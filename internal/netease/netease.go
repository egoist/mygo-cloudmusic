package netease

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The public NetEase Cloud Music API, as the web player calls it, without
// an account: what anyone may listen to.

const apiBase = "https://music.163.com"

const UserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15"

var httpClient = &http.Client{Timeout: 20 * time.Second}

type Artist struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Album struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	PicURL string `json:"picUrl"`
}

type Track struct {
	ID       int64    `json:"id"`
	Name     string   `json:"name"`
	Artists  []Artist `json:"ar"`
	Album    Album    `json:"al"`
	Duration int64    `json:"dt"` // milliseconds
	Fee      int      `json:"fee"`
	Alias    []string `json:"alia"`
	// Reason says why a daily recommendation was picked.
	Reason string `json:"reason"`
	// Privilege is what the listener may do with the track, if known.
	Privilege *Privilege `json:"privilege"`
	// A podcast's episode plays as a track of its audio, ID, with the
	// episode's ProgramID, the PodcastID it is in, and what it is about.
	ProgramID   int64  `json:"programId,omitempty"`
	PodcastID   int64  `json:"podcastId,omitempty"`
	Description string `json:"description,omitempty"`
	// Source is where the track is from, if not NetEase: the name of
	// another source (fnmusic.Source), whose Ref names the track there.
	Source string `json:"src,omitempty"`
	Ref    string `json:"ref,omitempty"`
	// Format is the kind of file a track of another source is: mp3, flac.
	Format string `json:"fmt,omitempty"`
	// DiscNo and TrackNo place a track of another source in its album.
	// Cue says the track is one of a CUE sheet's, which the service cuts out
	// of the one file.
	Cue     bool `json:"cue,omitempty"`
	Fav     bool `json:"fav,omitempty"`
	DiscNo  int  `json:"disc,omitempty"`
	TrackNo int  `json:"no,omitempty"`
}

// Privilege is what NetEase lets the listener signed in, or nobody, do
// with a track.
type Privilege struct {
	ID int64 `json:"id"`
	St int   `json:"st"` // below 0: taken down
	Pl int   `json:"pl"` // the best bit rate the listener may play; 0 for none
}

// withPrivileges gives tracks their privileges, matched by ID.
func withPrivileges(tracks []Track, privileges []Privilege) {
	byID := make(map[int64]*Privilege, len(privileges))
	for i := range privileges {
		byID[privileges[i].ID] = &privileges[i]
	}
	for i := range tracks {
		if p := byID[tracks[i].ID]; p != nil {
			tracks[i].Privilege = p
		}
	}
}

// ArtistNames joins the track's artists as the credits read.
func (t *Track) ArtistNames() string {
	names := make([]string, len(t.Artists))
	for i, a := range t.Artists {
		names[i] = a.Name
	}
	return strings.Join(names, ", ")
}

// Playable reports whether the listener can play the whole track: as their
// privilege says, or else as its fee does for anyone, VIP songs (1) and
// paid albums (4) giving only a preview.
func (t *Track) Playable() bool {
	if t.ProgramID > 0 {
		// An episode's audio has no privilege: whether it streams, as a
		// paid one bought, is up to its stream.
		return true
	}
	if t.Privilege != nil {
		return t.Privilege.St >= 0 && t.Privilege.Pl > 0
	}
	return t.Fee != 1 && t.Fee != 4
}

func (t *Track) Length() time.Duration { return time.Duration(t.Duration) * time.Millisecond }

// URL is the track's page on the NetEase website.
func (t *Track) URL() string {
	if t.Source != "" {
		return ""
	}
	if t.ProgramID > 0 {
		return apiBase + "/program?id=" + strconv.FormatInt(t.ProgramID, 10)
	}
	return apiBase + "/song?id=" + strconv.FormatInt(t.ID, 10)
}

// Thread names the track's comments, and what NetEase likes it by, for an
// episode: the episode's, not its audio's.
func (t *Track) Thread() string {
	if t.ProgramID > 0 {
		return programThread(t.ProgramID)
	}
	return "R_SO_4_" + strconv.FormatInt(t.ID, 10)
}

type Playlist struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	CoverURL    string  `json:"coverImgUrl"`
	PicURL      string  `json:"picUrl"`
	Description string  `json:"description"`
	PlayCount   float64 `json:"playCount"`
	TrackCount  int     `json:"trackCount"`
	UpdateFreq  string  `json:"updateFrequency"`
	SpecialType int     `json:"specialType"` // 5: the listener's liked songs
	Privacy     int     `json:"privacy"`     // 10: only its creator sees it
	Creator     struct {
		UserID   int64  `json:"userId"`
		Nickname string `json:"nickname"`
	} `json:"creator"`
	Tracks   []Track `json:"tracks"`
	TrackIDs []struct {
		ID int64 `json:"id"`
	} `json:"trackIds"`
	// Preview holds a chart's top songs, as its card shows them.
	Preview []ChartLine `json:"-"`
	// Ref names a playlist of another source there, where ID is only the
	// app's own number for it; Kind says what it is, if not a playlist.
	Ref  string `json:"-"`
	Kind string `json:"-"`
	// Sub is what cards show under the name, in place of UpdateFreq.
	Sub string `json:"-"`
}

// ChartLine is a song at the top of a chart: its name and its artist.
type ChartLine struct {
	First  string `json:"first"`
	Second string `json:"second"`
}

func (p *Playlist) Cover() string {
	if p.CoverURL != "" {
		return p.CoverURL
	}
	return p.PicURL
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Msg     string `json:"msg"`
}

func get(path string, query url.Values, out any) error {
	u := apiBase + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	return do(req, out)
}

func post(path string, form url.Values, out any) error {
	req, err := http.NewRequest("POST", apiBase+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return do(req, out)
}

func do(req *http.Request, out any) error {
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Referer", apiBase+"/")
	res, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	var status apiError
	if err := json.Unmarshal(body, &status); err != nil {
		return fmt.Errorf("netease: %s: %w", req.URL.Path, err)
	}
	if status.Code != 200 {
		msg := status.Message
		if msg == "" {
			msg = status.Msg
		}
		return fmt.Errorf("netease: %s: code %d %s", req.URL.Path, status.Code, msg)
	}
	return json.Unmarshal(body, out)
}

// FeaturedPlaylists returns the playlists the home page recommends.
func FeaturedPlaylists(limit int) ([]Playlist, error) {
	var res struct {
		Result []Playlist `json:"result"`
	}
	err := get("/api/personalized/playlist", url.Values{"limit": {strconv.Itoa(limit)}}, &res)
	return res.Result, err
}

// DailySongs returns the day's recommended songs: chosen for the listener
// once signed in, and what many listen to before.
func DailySongs() ([]Track, error) {
	var res struct {
		Data struct {
			DailySongs []Track `json:"dailySongs"`
		} `json:"data"`
	}
	if err := get("/api/v3/discovery/recommend/songs", nil, &res); err != nil {
		return nil, err
	}
	// They come without privileges: the songs' details have them.
	songs := res.Data.DailySongs
	detailPrivileges(songs)
	return songs, nil
}

// detailPrivileges gives songs the privileges their details carry for the
// listener, which are right where other lists have none, or wrong ones.
func detailPrivileges(songs []Track) {
	ids := make([]int64, len(songs))
	for i := range songs {
		ids[i] = songs[i].ID
	}
	details, err := SongDetails(ids)
	if err != nil {
		return
	}
	var privileges []Privilege
	for _, d := range details {
		if d.Privilege != nil {
			privileges = append(privileges, *d.Privilege)
		}
	}
	withPrivileges(songs, privileges)
}

// Charts returns the charts, each with its top three songs.
func Charts() ([]Playlist, error) {
	var res struct {
		List []struct {
			Playlist
			Tracks []ChartLine `json:"tracks"`
		} `json:"list"`
	}
	if err := get("/api/toplist/detail", nil, &res); err != nil {
		return nil, err
	}
	out := make([]Playlist, len(res.List))
	for i, l := range res.List {
		out[i] = l.Playlist
		out[i].Tracks = nil
		out[i].Preview = l.Tracks
	}
	return out, nil
}

// PlaylistDetail returns a playlist with up to 1000 of its tracks.
func PlaylistDetail(id int64) (*Playlist, error) {
	var res struct {
		Playlist   Playlist    `json:"playlist"`
		Privileges []Privilege `json:"privileges"`
	}
	q := url.Values{"id": {strconv.FormatInt(id, 10)}, "n": {"1000"}}
	if err := get("/api/v6/playlist/detail", q, &res); err != nil {
		return nil, err
	}
	p := &res.Playlist
	withPrivileges(p.Tracks, res.Privileges)
	// Its privileges mark VIP songs as taken down that a subscriber may
	// play, as Stan's: the songs' details say right.
	detailPrivileges(p.Tracks)
	// Long playlists list only their first tracks in full: the rest by id.
	if len(p.Tracks) < len(p.TrackIDs) {
		var ids []int64
		for _, t := range p.TrackIDs[len(p.Tracks):min(len(p.TrackIDs), 1000)] {
			ids = append(ids, t.ID)
		}
		more, err := SongDetails(ids)
		if err == nil {
			p.Tracks = append(p.Tracks, more...)
		}
	}
	return p, nil
}

// SongDetails returns the tracks of ids, in their order. It goes through
// the eapi: the web API's privileges call some VIP songs taken down for
// subscribers, whom the eapi streams them to.
func SongDetails(ids []int64) ([]Track, error) {
	var out []Track
	for len(ids) > 0 {
		batch := ids[:min(len(ids), 400)]
		ids = ids[len(batch):]
		c := make([]map[string]int64, len(batch))
		for i, id := range batch {
			c[i] = map[string]int64{"id": id}
		}
		b, _ := json.Marshal(c)
		var res struct {
			Code       int         `json:"code"`
			Songs      []Track     `json:"songs"`
			Privileges []Privilege `json:"privileges"`
		}
		if err := eapiPost("/api/v3/song/detail", map[string]any{"c": string(b)}, &res); err != nil {
			return out, err
		}
		if res.Code != 200 {
			return out, fmt.Errorf("netease: song detail: code %d", res.Code)
		}
		withPrivileges(res.Songs, res.Privileges)
		out = append(out, res.Songs...)
	}
	return out, nil
}

// ArtistProfile is who an artist is, as their page shows them, with the
// songs they are best known for.
type ArtistProfile struct {
	Artist
	Avatar    string   `json:"img1v1Url"` // square
	PicURL    string   `json:"picUrl"`    // wide
	Alias     []string `json:"alias"`
	Trans     string   `json:"trans"`
	BriefDesc string   `json:"briefDesc"`
	MusicSize int      `json:"musicSize"`
	AlbumSize int      `json:"albumSize"`
	Followed  bool     `json:"followed"` // by the listener signed in
	// AccountID is the artist's own account, which posts their events, or 0.
	AccountID int64   `json:"accountId"`
	Songs     []Track `json:"-"`
}

// ArtistDetail returns an artist with their 50 most popular songs.
func ArtistDetail(id int64) (*ArtistProfile, error) {
	var res struct {
		Artist   ArtistProfile `json:"artist"`
		HotSongs []Track       `json:"hotSongs"`
	}
	if err := get("/api/v1/artist/"+strconv.FormatInt(id, 10), nil, &res); err != nil {
		return nil, err
	}
	ar := &res.Artist
	ar.Songs = res.HotSongs
	detailPrivileges(ar.Songs)
	return ar, nil
}

// ArtistLatestSongs returns up to limit of an artist's songs, the newest
// first.
func ArtistLatestSongs(id int64, limit int) ([]Track, error) {
	var res struct {
		Songs []struct {
			ID int64 `json:"id"`
		} `json:"songs"`
	}
	form := url.Values{"id": {strconv.FormatInt(id, 10)}, "order": {"time"}, "limit": {strconv.Itoa(limit)}, "offset": {"0"}}
	if err := post("/api/v1/artist/songs", form, &res); err != nil {
		return nil, err
	}
	// Some artists' songs come in the old shape (artists, album, duration)
	// that Track doesn't read: their details have them in the new one, as
	// the listener's privileges.
	ids := make([]int64, len(res.Songs))
	for i, s := range res.Songs {
		ids[i] = s.ID
	}
	return SongDetails(ids)
}

var ErrUnavailable = errors.New("this song isn't available to stream")

// Stream is where a track streams from, and what it is.
type Stream struct {
	URL string `json:"-"`
	// Header, if set, makes the headers a request for the stream carries:
	// of another source, which signs each of its own.
	Header func() http.Header `json:"-"`
	Size   int64              `json:"size"` // in bytes
	// Type is the format: mp3, or flac for the lossless levels.
	Type string `json:"type"`
	// Level is the level served, which may be below the one asked for.
	Level string `json:"level"`
	// Gain is how many dB to change the track by to sound as loud as
	// others, and Peak its loudest sample, 1 at full scale, or 0 if
	// unknown.
	Gain float64 `json:"gain"`
	Peak float64 `json:"peak"`
}

// Levels are the stream levels, from the lowest.
var Levels = []string{"standard", "higher", "exhigh", "lossless", "hires"}

// Lossless reports whether level streams FLAC.
func Lossless(level string) bool { return level == "lossless" || level == "hires" }

// SongURL returns where to stream a track, MP3 or FLAC, at level (one of
// Levels) or the best below it the listener may play. It goes through
// the eapi, which carries the session, so VIP songs play for subscribers.
func SongURL(id int64, level string) (*Stream, error) {
	var res struct {
		Code int `json:"code"`
		Data []struct {
			Stream
			URL           string          `json:"url"`
			Peak          *float64        `json:"peak"`
			FreeTrialInfo json.RawMessage `json:"freeTrialInfo"`
		} `json:"data"`
	}
	encode := "mp3"
	if Lossless(level) {
		encode = "flac"
	}
	data := map[string]any{"ids": fmt.Sprintf("[%d]", id), "level": level, "encodeType": encode}
	if err := eapiPost("/api/song/enhance/player/url/v1", data, &res); err != nil {
		return nil, err
	}
	if len(res.Data) == 0 || res.Data[0].URL == "" {
		return nil, ErrUnavailable
	}
	d := res.Data[0]
	// A preview of a VIP song is no song.
	if len(d.FreeTrialInfo) > 0 && string(d.FreeTrialInfo) != "null" {
		return nil, ErrUnavailable
	}
	s := d.Stream
	s.Type = strings.ToLower(s.Type)
	if s.Type != "" && s.Type != "mp3" && s.Type != "flac" {
		return nil, fmt.Errorf("unsupported format %s", s.Type)
	}
	if d.Peak != nil {
		s.Peak = *d.Peak
	}
	// The level names the song's file in the cache: one not of Levels is
	// taken for the one asked for.
	if !slices.Contains(Levels, s.Level) {
		s.Level = level
	}
	s.URL = strings.Replace(d.URL, "http://", "https://", 1)
	return &s, nil
}

// Lyrics returns a track's timed lyrics, their translation if any, and
// its word-timed lyrics (YRC) where NetEase has them, which only the
// desktop client's API serves.
func Lyrics(id int64) (lrc, translation, yrc string, err error) {
	var res struct {
		Lrc struct {
			Lyric string `json:"lyric"`
		} `json:"lrc"`
		Tlyric struct {
			Lyric string `json:"lyric"`
		} `json:"tlyric"`
	}
	q := url.Values{"id": {strconv.FormatInt(id, 10)}, "lv": {"1"}, "tv": {"-1"}}
	if err = get("/api/song/lyric", q, &res); err != nil {
		return "", "", "", err
	}
	var words struct {
		Yrc struct {
			Lyric string `json:"lyric"`
		} `json:"yrc"`
	}
	data := map[string]any{"id": strconv.FormatInt(id, 10), "cp": false, "tv": 0, "lv": 0, "rv": 0, "kv": 0, "yv": 0, "ytv": 0, "yrv": 0}
	if eapiPost("/api/song/lyric/v1", data, &words) == nil {
		yrc = words.Yrc.Lyric
	}
	return res.Lrc.Lyric, res.Tlyric.Lyric, yrc, nil
}

// Sized asks the image CDN for a picture of n×n pixels.
func Sized(pic string, n int) string {
	if pic == "" {
		return ""
	}
	// A cover of another source asks for a size by itself.
	if !strings.HasPrefix(pic, "http") {
		return fmt.Sprintf("%s?size=%d", pic, n)
	}
	pic = strings.Replace(pic, "http://", "https://", 1)
	return fmt.Sprintf("%s?param=%dy%d", pic, n, n)
}

// Fetch returns what is at url, as a cover the image CDN serves.
func Fetch(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errors.New(res.Status)
	}
	return io.ReadAll(res.Body)
}
