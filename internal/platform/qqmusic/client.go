package qqmusic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"

	"netease-music-mygo/internal/network"
)

const UserAgent = "QQMusic 14090008(android 10)"
const musicuURL = "https://u.y.qq.com/cgi-bin/musicu.fcg"

var httpClient = &http.Client{Transport: network.Transport(), Timeout: 25 * time.Second}
var mediaRegistry = struct {
	sync.RWMutex
	tracks         map[int64]Track
	artists        map[int64]Artist
	albums         map[int64]Album
	ownedPlaylists map[int64]string
}{tracks: map[int64]Track{}, artists: map[int64]Artist{}, albums: map[int64]Album{}, ownedPlaylists: map[int64]string{}}

// RememberOwnedPlaylist routes the account's private lists through QQ's owner API.
func RememberOwnedPlaylist(id int64) {
	s := sessionSnapshot()
	if id <= 0 || s.MusicID == "" {
		return
	}
	mediaRegistry.Lock()
	mediaRegistry.ownedPlaylists[id] = s.MusicID
	mediaRegistry.Unlock()
}

// OwnedPlaylist reports whether id belongs to the signed-in account, which
// the owner API reads differently from public lists.
func OwnedPlaylist(id int64) bool { return isOwnedPlaylist(id) }

func isOwnedPlaylist(id int64) bool {
	s := sessionSnapshot()
	if s.MusicKey == "" || s.EncryptUin == "" {
		return false
	}
	mediaRegistry.RLock()
	defer mediaRegistry.RUnlock()
	return mediaRegistry.ownedPlaylists[id] == s.MusicID
}

// RememberTracks restores QQ's MID values alongside the numeric IDs used by the player.
func RememberTracks(tracks []Track) {
	mediaRegistry.Lock()
	defer mediaRegistry.Unlock()
	for _, t := range tracks {
		if t.ID > 0 && t.MID != "" {
			mediaRegistry.tracks[t.ID] = t
		}
		if t.Album.ID > 0 && t.Album.MID != "" {
			mediaRegistry.albums[t.Album.ID] = t.Album
		}
		for _, a := range t.Artists {
			if a.ID > 0 && a.MID != "" {
				mediaRegistry.artists[a.ID] = a
			}
		}
	}
}
func knownTrack(id int64) (Track, bool) {
	mediaRegistry.RLock()
	defer mediaRegistry.RUnlock()
	t, ok := mediaRegistry.tracks[id]
	return t, ok
}
func artistMID(id int64) string {
	mediaRegistry.RLock()
	defer mediaRegistry.RUnlock()
	return mediaRegistry.artists[id].MID
}
func albumMID(id int64) string {
	mediaRegistry.RLock()
	defer mediaRegistry.RUnlock()
	return mediaRegistry.albums[id].MID
}

type ProtocolError struct {
	GlobalCode int
	Code       int
	Module     string
	Method     string
}

func (e *ProtocolError) Error() string {
	return fmt.Sprintf("qqmusic: %s/%s: gateway=%d code=%d", e.Module, e.Method, e.GlobalCode, e.Code)
}

func rpc(module, method string, param any, out any) error {
	return rpcWithComm(module, method, param, out, nil, nil)
}
func rpcWithComm(module, method string, param any, out any, comm map[string]any, headers map[string]string) error {
	return rpcWithCommContext(context.Background(), module, method, param, out, comm, headers)
}

func rpcWithCommContext(ctx context.Context, module, method string, param any, out any, comm map[string]any, headers map[string]string) error {
	s := sessionSnapshot()
	if comm == nil {
		comm = map[string]any{"ct": 24, "cv": 0}
		if s.MusicKey != "" {
			comm = authenticatedComm(s)
		}
	}
	b, err := json.Marshal(map[string]any{"comm": comm, "req_0": map[string]any{"module": module, "method": method, "param": param}})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, musicuURL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Referer", "https://y.qq.com/")
	if s.MusicKey != "" {
		req.Header.Set("Cookie", "uin="+s.MusicID+"; qqmusic_uin="+s.MusicID+"; qm_keyst="+s.MusicKey+"; qqmusic_key="+s.MusicKey)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	b, err = readHTTP(req)
	if err != nil {
		return err
	}
	var body struct {
		Code *int `json:"code"`
		Req  *struct {
			Code    *int            `json:"code"`
			Subcode int             `json:"subcode"`
			Data    json.RawMessage `json:"data"`
		} `json:"req_0"`
	}
	if err = json.Unmarshal(b, &body); err != nil {
		return fmt.Errorf("qqmusic: malformed RPC response: %w", err)
	}
	if body.Code == nil || body.Req == nil || body.Req.Code == nil {
		return fmt.Errorf("qqmusic: missing RPC status")
	}
	if *body.Code != 0 || *body.Req.Code != 0 || body.Req.Subcode != 0 {
		code := *body.Req.Code
		if code == 0 {
			code = body.Req.Subcode
		}
		return &ProtocolError{*body.Code, code, module, method}
	}
	if out == nil {
		return nil
	}
	if len(body.Req.Data) == 0 {
		return fmt.Errorf("qqmusic: missing RPC data")
	}
	return json.Unmarshal(body.Req.Data, out)
}

func readHTTP(req *http.Request) ([]byte, error) {
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("qqmusic: HTTP %d", res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, 32<<20))
}

var jsonpPattern = regexp.MustCompile(`(?s)^\w+\((.*)\);?\s*$`)

func cgi(path string, q url.Values, out any) error {
	if q == nil {
		q = url.Values{}
	}
	q.Set("format", "json")
	q.Set("outCharset", "utf-8")
	req, err := http.NewRequest(http.MethodGet, "https://c.y.qq.com"+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Referer", "https://y.qq.com/")
	req.Header.Set("User-Agent", UserAgent)
	s := sessionSnapshot()
	if s.MusicKey != "" {
		req.Header.Set("Cookie", "uin="+s.MusicID+"; qqmusic_uin="+s.MusicID+"; qm_keyst="+s.MusicKey+"; qqmusic_key="+s.MusicKey)
	}
	b, err := readHTTP(req)
	if err != nil {
		return err
	}
	if !utf8.Valid(b) {
		b, err = simplifiedchinese.GB18030.NewDecoder().Bytes(b)
		if err != nil {
			return err
		}
	}
	b = bytes.TrimSpace(b)
	if match := jsonpPattern.FindSubmatch(b); match != nil {
		b = match[1]
	}
	var status struct {
		Code    *int `json:"code"`
		Subcode int  `json:"subcode"`
	}
	if err = json.Unmarshal(b, &status); err != nil {
		return err
	}
	if status.Code == nil {
		return fmt.Errorf("qqmusic: missing CGI status")
	}
	if *status.Code != 0 || status.Subcode != 0 {
		code := *status.Code
		if code == 0 {
			code = status.Subcode
		}
		return &ProtocolError{0, code, "cgi", path}
	}
	return json.Unmarshal(b, out)
}

func Fetch(rawURL string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	return readHTTP(req)
}

var imageSizePattern = regexp.MustCompile(`(T00[12])R\d+x\d+M000`)

func Sized(pic string, n int) string {
	if pic == "" {
		return ""
	}
	// QQ serves fixed thumbnail sizes; arbitrary UI sizes such as 96 or 600
	// return 404. Round up to retain detail, capped at the largest supported size.
	px := 800
	for _, size := range [...]int{90, 120, 150, 180, 300, 500, 800} {
		if n <= size {
			px = size
			break
		}
	}
	return imageSizePattern.ReplaceAllString(strings.Replace(pic, "http://", "https://", 1), fmt.Sprintf("${1}R%dx%dM000", px, px))
}
func albumCover(mid string) string {
	if mid == "" {
		return ""
	}
	return "https://y.gtimg.cn/music/photo_new/T002R500x500M000" + mid + ".jpg?max_age=2592000"
}
func artistCover(mid string) string {
	if mid == "" {
		return ""
	}
	return "https://y.gtimg.cn/music/photo_new/T001R500x500M000" + mid + ".jpg?max_age=2592000"
}

type object map[string]json.RawMessage

func obj(raw json.RawMessage) object { var o object; _ = json.Unmarshal(raw, &o); return o }
func text(o object, keys ...string) string {
	for _, k := range keys {
		var s string
		if json.Unmarshal(o[k], &s) == nil && s != "" {
			return s
		}
	}
	return ""
}
func number(o object, keys ...string) int64 {
	for _, k := range keys {
		var n json.Number
		if json.Unmarshal(o[k], &n) == nil {
			if i, err := strconv.ParseInt(n.String(), 10, 64); err == nil {
				return i
			}
		}
		var s string
		if json.Unmarshal(o[k], &s) == nil {
			if i, err := strconv.ParseInt(s, 10, 64); err == nil {
				return i
			}
		}
	}
	return 0
}
func objects(o object, keys ...string) []object {
	for _, k := range keys {
		var out []object
		if json.Unmarshal(o[k], &out) == nil && out != nil {
			return out
		}
	}
	return nil
}
func convertArtist(o object) Artist {
	a := Artist{ID: number(o, "id", "singerid", "singer_id", "singerID"), MID: text(o, "mid", "singermid", "singer_mid", "singerMid", "singerMID"), Name: text(o, "name", "singername", "singer_name", "singerName")}
	mediaRegistry.Lock()
	if a.ID > 0 && a.MID != "" {
		mediaRegistry.artists[a.ID] = a
	}
	mediaRegistry.Unlock()
	return a
}
func convertAlbum(o object) Album {
	a := Album{ID: number(o, "id", "albumid", "albumID", "album_id"), MID: text(o, "mid", "albummid", "albumMID", "albumMid", "album_mid"), Name: text(o, "name", "title", "albumname", "albumName", "album_name")}
	a.PicURL = albumCover(a.MID)
	mediaRegistry.Lock()
	if a.ID > 0 && a.MID != "" {
		mediaRegistry.albums[a.ID] = a
	}
	mediaRegistry.Unlock()
	return a
}
func convertTrack(o object) Track {
	t := parseTrack(o)
	RememberTracks([]Track{t})
	return t
}

// parseTrack decodes a song without replacing the playback registry entry.
func parseTrack(o object) Track {
	if nested := obj(o["musicData"]); nested != nil {
		o = nested
	}
	if nested := obj(o["songInfo"]); nested != nil {
		o = nested
	}
	if nested := obj(o["track_info"]); nested != nil {
		o = nested
	}
	t := Track{ID: number(o, "id", "songid", "songId"), MID: text(o, "mid", "songmid", "songMid"), Name: text(o, "title", "name", "songname", "songName"), Duration: number(o, "interval") * 1000}
	if raw := o["volume"]; len(raw) > 0 {
		var volume *Loudness
		if json.Unmarshal(raw, &volume) == nil && volume != nil && volume.Peak >= 0 {
			t.Volume = volume
		}
	}
	for _, a := range objects(o, "singer", "singers") {
		t.Artists = append(t.Artists, convertArtist(a))
	}
	t.Album = convertAlbum(obj(o["album"]))
	if t.Album.ID == 0 {
		t.Album = Album{ID: number(o, "albumid", "album_id"), MID: text(o, "albummid", "album_mid"), Name: text(o, "albumname", "album_name")}
		t.Album.PicURL = albumCover(t.Album.MID)
	}
	file := obj(o["file"])
	t.MediaMID = text(file, "media_mid")
	t.Size128 = number(file, "size_128mp3")
	t.Size320 = number(file, "size_320mp3")
	t.SizeFLAC = number(file, "size_flac")
	if t.Size128 == 0 {
		t.Size128 = number(o, "size128")
	}
	if t.Size320 == 0 {
		t.Size320 = number(o, "size320")
	}
	if t.SizeFLAC == 0 {
		t.SizeFLAC = number(o, "sizeflac")
	}
	if t.MediaMID == "" {
		t.MediaMID = text(o, "strMediaMid", "media_mid")
	}
	return t
}
func convertTracks(entries []object) []Track {
	tracks := make([]Track, 0, len(entries))
	for _, o := range entries {
		t := convertTrack(o)
		if t.ID > 0 && t.MID != "" {
			tracks = append(tracks, t)
		}
	}
	return tracks
}
