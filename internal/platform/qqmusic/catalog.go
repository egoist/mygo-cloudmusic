package qqmusic

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const chartIDBase int64 = 1 << 60
const songChartType int64 = 10005

var ErrUnsupported = errors.New("QQ 音乐暂不支持此功能")

func FeaturedPlaylists(limit int) ([]Playlist, error) {
	var data object
	if err := rpc("music.playlist.PlaylistSquare", "GetRecommendFeed", map[string]any{"From": 0, "Size": min(max(limit, 1), 60)}, &data); err != nil {
		return nil, err
	}
	var out []Playlist
	for _, entry := range objects(data, "List") {
		basic := obj(obj(entry["Playlist"])["basic"])
		p := Playlist{ID: number(basic, "tid"), Name: text(basic, "title"), Description: text(basic, "desc"), TrackCount: int(number(basic, "song_cnt")), PlayCount: float64(number(basic, "play_cnt"))}
		p.CoverURL = text(obj(basic["cover"]), "default_url", "big_url", "medium_url", "small_url")
		p.Creator.Nickname = text(obj(basic["creator"]), "nick")
		if p.ID > 0 {
			out = append(out, p)
		}
	}
	return out, nil
}

func Charts() ([]Playlist, error) {
	var data object
	// The mobile identity also exposes album, MV and native-only chart pages.
	comm := map[string]any{"ct": 24, "cv": 0}
	if err := rpcWithComm("musicToplist.ToplistInfoServer", "GetAll", map[string]any{}, &data, comm, nil); err != nil {
		return nil, err
	}
	var out []Playlist
	for _, group := range objects(data, "group") {
		for _, entry := range objects(group, "toplist") {
			id := number(entry, "topId")
			if id <= 0 || number(entry, "recType") != songChartType {
				continue
			}
			p := Playlist{ID: chartIDBase + id, Name: text(entry, "title"), CoverURL: text(entry, "headPicUrl", "frontPicUrl", "musichallPicUrl"), Description: text(entry, "intro"), TrackCount: int(number(entry, "totalNum")), PlayCount: float64(number(entry, "listenNum")), UpdateFreq: text(entry, "updateTime")}
			for _, song := range objects(entry, "song") {
				p.Preview = append(p.Preview, ChartLine{First: text(song, "title"), Second: text(song, "singerName")})
			}
			out = append(out, p)
		}
	}
	return out, nil
}

func PlaylistDetail(id int64) (*Playlist, error) {
	if id >= chartIDBase {
		var data object
		comm := map[string]any{"ct": 24, "cv": 0}
		if err := rpcWithComm("musicToplist.ToplistInfoServer", "GetDetail", map[string]any{"topid": id - chartIDBase, "offset": 0, "num": 100, "period": ""}, &data, comm, nil); err != nil {
			return nil, err
		}
		info := obj(data["data"])
		if number(info, "recType") != songChartType {
			return nil, ErrUnsupported
		}
		tracks := convertTracks(objects(data, "songInfoList"))
		if len(tracks) == 0 {
			return nil, fmt.Errorf("qqmusic: chart %d has no songs", id-chartIDBase)
		}
		p := &Playlist{ID: id, Name: text(info, "title"), CoverURL: text(info, "headPicUrl", "frontPicUrl"), Description: text(info, "intro"), TrackCount: int(number(info, "totalNum")), UpdateFreq: text(info, "updateTime"), Tracks: tracks}
		return p, nil
	}
	if isOwnedPlaylist(id) {
		return ownedPlaylist(id)
	}
	var data object
	err := cgi("/qzone/fcg-bin/fcg_ucc_getcdinfo_byids_cp.fcg", url.Values{"disstid": {strconv.FormatInt(id, 10)}, "type": {"1"}, "json": {"1"}, "utf8": {"1"}, "onlysong": {"0"}, "new_format": {"1"}}, &data)
	if err != nil {
		var protocol *ProtocolError
		if !errors.As(err, &protocol) || protocol.Code != 10 || sessionSnapshot().MusicKey == "" {
			return nil, err
		}
		return authenticatedPlaylist(id)
	}
	entries := objects(data, "cdlist")
	if len(entries) == 0 {
		if s := sessionSnapshot(); s.MusicKey != "" && s.EncryptUin != "" {
			return ownedPlaylist(id)
		}
		return nil, fmt.Errorf("qqmusic: playlist %d not found", id)
	}
	info := entries[0]
	p := &Playlist{ID: id, Name: text(info, "dissname", "title"), CoverURL: text(info, "logo", "picurl"), Description: text(info, "desc"), TrackCount: int(number(info, "total_song_num", "songnum")), PlayCount: float64(number(info, "visitnum", "listennum")), Tracks: convertTracks(objects(info, "songlist"))}
	p.Creator.Nickname = text(info, "nickname", "nick")
	p.Creator.UserID = number(info, "uin")
	if len(p.Tracks) == 0 && p.TrackCount > 0 {
		if s := sessionSnapshot(); s.MusicKey != "" && s.EncryptUin != "" {
			return ownedPlaylist(id)
		}
	}
	if p.TrackCount == 0 {
		p.TrackCount = len(p.Tracks)
	}
	return p, nil
}

// Private owned playlists can return a successful empty shell from the public CGI.
func ownedPlaylist(id int64) (*Playlist, error) {
	s := sessionSnapshot()
	var first object
	var tracks []Track
	total := 0
	for page := 0; page < 100; page++ {
		var data object
		if err := rpc("music.srfDissInfo.DissInfo", "CgiGetDiss", map[string]any{"disstid": id, "dirid": 0, "tag": true, "song_begin": len(tracks), "song_num": 100, "userinfo": true, "orderlist": true, "enc_host_uin": s.EncryptUin}, &data); err != nil {
			return nil, err
		}
		entries, err := playlistSongEntries(data, "music.srfDissInfo.DissInfo", "CgiGetDiss")
		if err != nil {
			return nil, err
		}
		if page == 0 {
			first = data
			total = int(number(data, "total_song_num"))
		}
		next := convertTracks(entries)
		if len(next) == 0 || len(tracks) > 0 && next[0].ID == tracks[0].ID {
			break
		}
		tracks = append(tracks, next...)
		if total > 0 && len(tracks) >= total || !playlistHasMore(data) && len(next) < 100 {
			break
		}
	}
	info := obj(first["dirinfo"])
	p := &Playlist{ID: id, Name: text(info, "title"), CoverURL: text(info, "picurl"), Description: text(info, "desc"), TrackCount: total, Tracks: tracks}
	p.Creator.Nickname = text(info, "host_nick")
	if p.TrackCount == 0 {
		p.TrackCount = len(tracks)
	}
	return p, nil
}

func playlistHasMore(data object) bool {
	var more bool
	_ = json.Unmarshal(data["hasmore"], &more)
	return more || number(data, "hasmore") == 1
}

// These RPCs can reject inside a successful musicu envelope; an absent list is not an empty library.
func playlistSongEntries(data object, module, method string) ([]object, error) {
	for _, key := range []string{"code", "subcode"} {
		if raw, ok := data[key]; ok {
			var code int
			if json.Unmarshal(raw, &code) != nil {
				return nil, fmt.Errorf("qqmusic: invalid playlist %s", key)
			}
			if code != 0 {
				return nil, &ProtocolError{0, code, module, method}
			}
		}
	}
	var entries []object
	raw, ok := data["songlist"]
	if !ok || string(raw) == "null" || json.Unmarshal(raw, &entries) != nil {
		return nil, fmt.Errorf("qqmusic: missing playlist song list")
	}
	return entries, nil
}

func authenticatedPlaylist(id int64) (*Playlist, error) {
	var first object
	var tracks []Track
	var total int
	for page := 0; page < 10; page++ {
		var data object
		if err := rpc("music.srfDissInfo.aiDissInfo", "uniform_get_Dissinfo", map[string]any{"disstid": id, "userinfo": 1, "tag": 1, "orderlist": 1, "song_begin": len(tracks), "song_num": 1000, "onlysonglist": 0, "enc_host_uin": ""}, &data); err != nil {
			return nil, err
		}
		entries, err := playlistSongEntries(data, "music.srfDissInfo.aiDissInfo", "uniform_get_Dissinfo")
		if err != nil {
			return nil, err
		}
		if page == 0 {
			first = data
			total = int(number(data, "total_song_num"))
		}
		next := convertTracks(entries)
		if len(next) == 0 {
			break
		}
		if len(tracks) > 0 && next[0].ID == tracks[0].ID {
			break
		}
		tracks = append(tracks, next...)
		if !playlistHasMore(data) || total > 0 && len(tracks) >= total {
			break
		}
	}
	info := obj(first["dirinfo"])
	p := &Playlist{ID: id, Name: text(info, "title"), CoverURL: text(info, "picurl"), Description: text(info, "desc"), TrackCount: total, Tracks: tracks}
	p.Creator.Nickname = text(info, "host_nick")
	if p.TrackCount == 0 {
		p.TrackCount = len(tracks)
	}
	return p, nil
}

func SongDetails(ids []int64) ([]Track, error) {
	out := make([]Track, 0, len(ids))
	for _, id := range ids {
		var data object
		t, _ := knownTrack(id)
		if err := rpc("music.pf_song_detail_svr", "get_song_detail_yqq", map[string]any{"song_type": 0, "song_mid": t.MID, "song_id": strconv.FormatInt(id, 10)}, &data); err != nil {
			return out, err
		}
		track := convertTrack(obj(data["track_info"]))
		if track.ID <= 0 || track.MID == "" {
			return out, fmt.Errorf("qqmusic: song %d not found", id)
		}
		if track.Volume != nil {
			songLoudnessCache.put(map[string]Loudness{track.MID: *track.Volume}, time.Now())
		}
		out = append(out, track)
	}
	return out, nil
}

func artistSongs(id int64, sort, limit int) (object, []Track, error) {
	mid := artistMID(id)
	if mid == "" {
		return nil, nil, fmt.Errorf("qqmusic: artist MID missing for %d", id)
	}
	var data object
	err := rpc("music.web_singer_info_svr", "get_singer_detail_info", map[string]any{"sort": sort, "singermid": mid, "sin": 0, "num": limit}, &data)
	return data, convertTracks(objects(data, "songlist")), err
}
func ArtistDetail(id int64) (*ArtistProfile, error) {
	data, songs, err := artistSongs(id, 5, 50)
	if err != nil {
		return nil, err
	}
	info := obj(data["singer_info"])
	a := convertArtist(info)
	if a.ID == 0 {
		mediaRegistry.RLock()
		a = mediaRegistry.artists[id]
		mediaRegistry.RUnlock()
	}
	p := &ArtistProfile{Artist: a, Avatar: artistCover(a.MID), PicURL: artistCover(a.MID), BriefDesc: text(info, "desc", "description"), MusicSize: int(number(data, "total_song")), AlbumSize: int(number(data, "total_album")), Songs: songs}
	return p, nil
}
func ArtistLatestSongs(id int64, limit int) ([]Track, error) {
	_, tracks, err := artistSongs(id, 2, limit)
	return tracks, err
}

func AlbumDetail(id int64) (*AlbumProfile, error) {
	q := url.Values{}
	if mid := albumMID(id); mid != "" {
		q.Set("albummid", mid)
	} else {
		q.Set("albumid", strconv.FormatInt(id, 10))
	}
	var body object
	if err := cgi("/v8/fcg-bin/fcg_v8_album_info_cp.fcg", q, &body); err != nil {
		return nil, err
	}
	data := obj(body["data"])
	a := convertAlbum(data)
	if a.ID == 0 {
		a.ID = id
	}
	p := &AlbumProfile{Album: a, Size: int(number(data, "total", "songnum")), Description: text(data, "desc"), Songs: convertTracks(objects(data, "list", "songlist"))}
	if p.Size == 0 {
		p.Size = len(p.Songs)
	}
	for _, s := range objects(data, "singer") {
		p.Artists = append(p.Artists, convertArtist(s))
	}
	if len(p.Artists) == 0 && len(p.Songs) > 0 {
		p.Artists = p.Songs[0].Artists
	}
	if date := text(data, "aDate", "time_public"); date != "" {
		if t, err := time.Parse("2006-01-02", date); err == nil {
			p.PublishTime = t.UnixMilli()
		}
	}
	return p, nil
}
func ArtistAlbums(id int64, limit, offset int) ([]AlbumProfile, bool, error) {
	var data object
	// This web catalog RPC rejects the logged-in mobile client identity with 104403.
	comm := map[string]any{"ct": 24, "cv": 0}
	if err := rpcWithComm("music.musichallAlbum.AlbumListServer", "GetAlbumList", map[string]any{"sort": 5, "singermid": artistMID(id), "begin": offset, "num": limit}, &data, comm, nil); err != nil {
		return nil, false, err
	}
	entries := objects(data, "albumList", "albumlist")
	out := make([]AlbumProfile, 0, len(entries))
	for _, o := range entries {
		a := AlbumProfile{Album: convertAlbum(o), Size: int(number(o, "song_count", "songnum", "totalNum"))}
		if date := text(o, "pub_time", "time_public", "publishDate"); date != "" {
			if t, err := time.Parse("2006-01-02", date); err == nil {
				a.PublishTime = t.UnixMilli()
			}
		}
		out = append(out, a)
	}
	return out, offset+len(out) < int(number(data, "total")), nil
}

func searchID() string {
	var b [18]byte
	_, _ = rand.Read(b[:])
	for i := range b {
		b[i] = '0' + b[i]%10
	}
	if b[0] == '0' {
		b[0] = '1'
	}
	return string(b[:])
}

// SearchFilterError distinguishes a suppressed response from a genuine empty search.
type SearchFilterError struct{ Filter int }

func (e *SearchFilterError) Error() string {
	return fmt.Sprintf("qqmusic: empty search response filtered (is_filter=%d)", e.Filter)
}

func searchMusicu(ctx context.Context, source SearchSource, query string, kind SearchType, limit, offset int, s Session) (SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return SearchResult{}, nil
	}
	if len([]rune(query)) > 60 {
		return SearchResult{}, fmt.Errorf("qqmusic: search exceeds 60 characters")
	}
	limit = min(max(limit, 1), 20)
	offset = max(offset, 0)
	code, key := 7, "item_song"
	switch kind {
	case SearchSongs:
	case SearchAlbums:
		code, key = 2, "item_album"
	case SearchArtists:
		code, key = 1, "singer"
	default:
		return SearchResult{}, ErrUnsupported
	}
	// Anonymous Lite search remains available without a session, but QQ can
	// reject it with 2001. Signed-in search needs PC comm: Android comm can
	// instead return success with is_filter=-9 and an empty list.
	comm := map[string]any{"ct": 11, "cv": "1003006", "v": "1003006", "tmeAppID": "qqmusiclight", "nettype": "NETWORK_WIFI", "udid": "0", "uid": "0"}
	headers := map[string]string{"Cookie": "tmeLoginType=-1", "User-Agent": "okhttp/3.14.9"}
	method := "DoSearchForQQMusicLite"
	if source == SearchDesktop {
		method = "DoSearchForQQMusicDesktop"
		comm = map[string]any{"ct": 11, "cv": 0, "uin": "0", "format": "json"}
		headers["User-Agent"] = "Mozilla/5.0"
		if kind == SearchSongs {
			code = 0
		}
	}
	if s.valid() {
		comm = map[string]any{"ct": 24, "cv": 0, "uin": s.MusicID, "qq": s.MusicID, "authst": s.MusicKey, "tmeLoginType": s.LoginType, "format": "json"}
		if source == SearchDesktop {
			comm["ct"] = 11
		}
		headers = map[string]string{"Cookie": searchCookie(s)}
		if kind == SearchSongs {
			code = 0
		}
	}
	param := map[string]any{"query": query, "search_type": code, "num_per_page": limit, "page_num": offset/limit + 1}
	if source != SearchDesktop {
		param["search_id"], param["remoteplace"] = searchID(), "search.android.keyboard"
		param["highlight"], param["nqc_flag"], param["page_id"], param["grp"] = 0, 0, 1, 1
	}
	var data object
	err := rpcWithCommContext(ctx, "music.search.SearchCgiService", method, param, &data, comm, headers)
	if err != nil {
		return SearchResult{}, err
	}
	if raw, ok := data["code"]; ok {
		var code int
		if json.Unmarshal(raw, &code) != nil {
			return SearchResult{}, fmt.Errorf("qqmusic: invalid search status")
		}
		if code != 0 {
			return SearchResult{}, &ProtocolError{0, code, "music.search.SearchCgiService", method}
		}
	}
	body := obj(data["body"])
	meta := obj(data["meta"])
	if source == SearchDesktop {
		switch kind {
		case SearchSongs:
			key = "song"
		case SearchArtists:
			key = "singer"
		case SearchAlbums:
			key = "album"
		}
	}
	rawEntries := body[key]
	if source == SearchDesktop {
		rawEntries = obj(rawEntries)["list"]
	}
	var entries []object
	if body == nil || meta == nil || json.Unmarshal(rawEntries, &entries) != nil || entries == nil {
		return SearchResult{}, fmt.Errorf("qqmusic: malformed search response")
	}
	for _, entry := range entries {
		if entry == nil {
			return SearchResult{}, fmt.Errorf("qqmusic: malformed search entry")
		}
	}
	if filter := int(number(meta, "is_filter")); len(entries) == 0 && filter < 0 {
		return SearchResult{}, &SearchFilterError{Filter: filter}
	}
	return decodeSearchEntries(entries, kind, int(number(meta, "sum")), source, limit, offset), nil
}

func decodeSearchEntries(entries []object, kind SearchType, total int, source SearchSource, limit, offset int) SearchResult {
	result := SearchResult{Total: total, Source: source, NextOffset: offset}
	if len(entries) > 0 {
		result.NextOffset = offset + limit
	}
	switch kind {
	case SearchSongs:
		result.Songs = searchTracks(entries)
	case SearchAlbums:
		for _, o := range entries {
			p := AlbumProfile{Album: convertAlbum(o), Size: int(number(o, "song_count"))}
			if p.ID <= 0 || p.MID == "" {
				continue
			}
			if pic := text(o, "albumPic"); pic != "" {
				p.PicURL = pic
			}
			if date, err := time.Parse("2006-01-02", text(o, "publicTime")); err == nil {
				p.PublishTime = date.UnixMilli()
			}
			for _, a := range objects(o, "singer", "singer_list") {
				p.Artists = append(p.Artists, convertArtist(a))
			}
			result.Albums = append(result.Albums, p)
		}
	case SearchArtists:
		for _, o := range entries {
			a := convertArtist(o)
			if a.ID <= 0 || a.MID == "" {
				continue
			}
			pic := text(o, "singerPic")
			if pic == "" {
				pic = artistCover(a.MID)
			}
			result.Artists = append(result.Artists, ArtistProfile{Artist: a, Avatar: pic, PicURL: pic, MusicSize: int(number(o, "songNum")), AlbumSize: int(number(o, "albumNum"))})
		}
	}
	return result
}
func Search(query string, limit int) ([]Track, error) {
	r, err := SearchPage(query, SearchSongs, limit, 0)
	return r.Songs, err
}
