package qqmusic

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var ErrInPlaylist = errors.New("歌曲已在歌单中")

func signedSession() (Session, error) {
	s := sessionSnapshot()
	if !s.valid() {
		return s, errors.New("请先登录 QQ 音乐")
	}
	return s, nil
}
func ownedPlaylists() (object, error) {
	s, err := signedSession()
	if err != nil {
		return nil, err
	}
	var data object
	err = rpc("music.musicasset.PlaylistBaseRead", "GetPlaylistByUin", map[string]any{"uin": s.MusicID}, &data)
	return data, err
}
func libraryPlaylist(o object) Playlist {
	p := Playlist{ID: number(o, "tid", "dissid", "id"), Name: text(o, "dirName", "title", "dissname", "name"), CoverURL: text(o, "bigpicUrl", "picUrl", "picurl", "coverUrl", "logo", "albumPicUrl"), Description: text(o, "desc"), TrackCount: int(number(o, "songNum", "songnum", "song_cnt")), PlayCount: float64(number(o, "play_cnt", "playCount"))}
	p.Creator.UserID = number(o, "uin")
	p.Creator.Nickname = text(o, "nick", "nickname")
	if number(o, "dirId", "dirid") == 201 {
		p.SpecialType = 5
	}
	if number(o, "dirShow", "dir_show") == 2 {
		p.Privacy = 10
	}
	return p
}

func UserPlaylists(uid int64) (made, saved []Playlist, err error) {
	s, err := signedSession()
	if err != nil {
		return nil, nil, err
	}
	if strconv.FormatInt(uid, 10) != s.MusicID {
		return nil, nil, ErrUnsupported
	}
	data, err := ownedPlaylists()
	if err != nil {
		return nil, nil, err
	}
	for _, o := range objects(data, "v_playlist") {
		p := libraryPlaylist(o)
		p.Creator.UserID = uid
		if p.ID > 0 {
			RememberOwnedPlaylist(p.ID)
			if p.SpecialType == 5 {
				made = append([]Playlist{p}, made...)
			} else {
				made = append(made, p)
			}
		}
	}
	if s.EncryptUin == "" {
		return made, nil, nil
	}
	seen := map[int64]bool{}
	for offset := 0; offset < 10000; {
		var page object
		if err = rpc("music.musicasset.PlaylistFavRead", "CgiGetPlaylistFavInfo", map[string]any{"uin": s.EncryptUin, "offset": offset, "size": 100}, &page); err != nil {
			return made, saved, err
		}
		items := objects(page, "v_list")
		fresh := 0
		for _, o := range items {
			p := libraryPlaylist(o)
			if p.ID > 0 && !seen[p.ID] {
				seen[p.ID] = true
				saved = append(saved, p)
				fresh++
			}
		}
		if !libraryMore(page) || len(items) == 0 || fresh == 0 {
			return made, saved, nil
		}
		offset += len(items)
	}
	return made, saved, errors.New("QQ 音乐收藏歌单过多，未能完整读取")
}

func libraryMore(o object) bool { return number(o, "hasmore") == 1 || string(o["hasmore"]) == "true" }
func DailySongs() ([]Track, error) {
	if _, err := signedSession(); err != nil {
		return nil, err
	}
	var data object
	if err := rpc("music.srfDissInfo.aiDissInfo", "uniform_get_Dissinfo", map[string]any{"dirid": 202, "optype": 3, "tag": 1, "userinfo": 0, "pic_dpi": 300}, &data); err != nil {
		return nil, err
	}
	if code, ok := data["code"]; ok && string(code) != "0" {
		return nil, fmt.Errorf("qqmusic: daily songs code %s", code)
	}
	return convertTracks(objects(data, "songlist")), nil
}

func LikedIDs(uid int64) ([]int64, error) {
	s, err := signedSession()
	if err != nil {
		return nil, err
	}
	if strconv.FormatInt(uid, 10) != s.MusicID {
		return nil, ErrUnsupported
	}
	if s.EncryptUin == "" {
		return nil, errors.New("QQ 音乐登录凭证缺少账号标识，请重新登录")
	}
	var ids []int64
	seen := map[int64]bool{}
	for offset := 0; offset < 100000; {
		var page object
		if err = rpc("music.srfDissInfo.DissInfo", "CgiGetDiss", map[string]any{"disstid": 0, "dirid": 201, "tag": true, "song_begin": offset, "song_num": 100, "userinfo": true, "orderlist": true, "enc_host_uin": s.EncryptUin}, &page); err != nil {
			return nil, err
		}
		if code, ok := page["code"]; ok && string(code) != "0" {
			return nil, fmt.Errorf("qqmusic: liked songs code %s", code)
		}
		items := objects(page, "songlist")
		fresh := 0
		for _, track := range convertTracks(items) {
			if track.ID > 0 && !seen[track.ID] {
				seen[track.ID] = true
				ids = append(ids, track.ID)
				fresh++
			}
		}
		if !libraryMore(page) || number(page, "total_song_num") > 0 && int64(len(ids)) >= number(page, "total_song_num") {
			return ids, nil
		}
		if len(items) == 0 || fresh == 0 {
			return nil, errors.New("QQ 音乐喜欢歌曲列表未能完整读取")
		}
		offset += len(items)
	}
	return nil, errors.New("QQ 音乐喜欢歌曲列表过多，未能完整读取")
}

// LikedTracks reads the account's hearted songs, with the details the liked
// songs page shows. Unlike LikedIDs followed by SongDetails it does not ask
// for every song again, which made large collections load for minutes.
func LikedTracks(uid int64) ([]Track, error) {
	s, err := signedSession()
	if err != nil {
		return nil, err
	}
	if strconv.FormatInt(uid, 10) != s.MusicID {
		return nil, ErrUnsupported
	}
	if s.EncryptUin == "" {
		return nil, errors.New("QQ 音乐登录凭证缺少账号标识，请重新登录")
	}
	var out []Track
	seen := map[int64]bool{}
	for offset := 0; offset < 100000; {
		var page object
		if err = rpc("music.srfDissInfo.DissInfo", "CgiGetDiss", map[string]any{"disstid": 0, "dirid": 201, "tag": true, "song_begin": offset, "song_num": 100, "userinfo": true, "orderlist": true, "enc_host_uin": s.EncryptUin}, &page); err != nil {
			return out, err
		}
		if code, ok := page["code"]; ok && string(code) != "0" {
			return out, fmt.Errorf("qqmusic: liked songs code %s", code)
		}
		items := objects(page, "songlist")
		fresh := 0
		for _, track := range convertTracks(items) {
			if track.ID > 0 && !seen[track.ID] {
				seen[track.ID] = true
				out = append(out, track)
				fresh++
			}
		}
		if !libraryMore(page) || number(page, "total_song_num") > 0 && int64(len(out)) >= number(page, "total_song_num") {
			return out, nil
		}
		// A page that repeats itself means the collection has been read;
		// keep what came before instead of dropping the whole list.
		if len(items) == 0 || fresh == 0 {
			return out, nil
		}
		offset += len(items)
	}
	return out, nil
}

const likedAlbumsPath = "/fav/fcgi-bin/fcg_get_profile_order_asset.fcg"

// LikedAlbums reads the account's album collection. This legacy CGI uses an
// inclusive range; the musicu album collection method does not return the list.
func LikedAlbums(limit, offset int) ([]AlbumProfile, bool, error) {
	s, err := signedSession()
	if err != nil {
		return nil, false, err
	}
	limit, offset = min(max(limit, 1), 100), max(offset, 0)
	var body struct {
		Data *struct {
			Albums []object `json:"albumlist"`
			Total  *int64   `json:"totalalbum"`
		} `json:"data"`
	}
	if err := cgi(likedAlbumsPath, url.Values{
		"ct": {"20"}, "cid": {"205360956"}, "userid": {s.MusicID},
		"reqtype": {"2"}, "sin": {strconv.Itoa(offset)}, "ein": {strconv.Itoa(offset + limit - 1)},
	}, &body); err != nil {
		return nil, false, err
	}
	if body.Data == nil || body.Data.Total == nil || *body.Data.Total < 0 {
		return nil, false, errors.New("QQ 音乐未返回收藏专辑列表")
	}
	data := body.Data
	if data.Albums == nil && *data.Total > 0 || len(data.Albums) == 0 && int64(offset) < *data.Total {
		return nil, false, errors.New("QQ 音乐收藏专辑列表不完整")
	}
	albums := make([]AlbumProfile, 0, len(data.Albums))
	for _, entry := range data.Albums {
		album := AlbumProfile{Album: convertAlbum(entry), Size: int(number(entry, "song_count", "songnum", "totalNum"))}
		if album.ID <= 0 {
			return nil, false, errors.New("QQ 音乐未返回收藏专辑标识")
		}
		if album.PicURL == "" {
			album.PicURL = text(entry, "picurl", "picUrl", "albumPicUrl")
		}
		for _, singer := range objects(entry, "singers", "singer") {
			album.Artists = append(album.Artists, convertArtist(singer))
		}
		if len(album.Artists) == 0 {
			singer := Artist{ID: number(entry, "singerid", "singerID", "singer_id"), MID: text(entry, "singermid", "singerMID", "singerMid", "singer_mid"), Name: text(entry, "singerName", "singername", "singer_name")}
			if singer.Name != "" {
				album.Artists = append(album.Artists, singer)
			}
		}
		if date := text(entry, "release_time", "publishDate", "pubtime"); date != "" {
			if parsed, err := time.Parse("2006-01-02", date); err == nil {
				album.PublishTime = parsed.UnixMilli()
			}
		}
		albums = append(albums, album)
	}
	return albums, int64(offset)+int64(len(albums)) < *data.Total, nil
}

func albumFavoriteOwner(mid string) (Session, any, error) {
	s, err := signedSession()
	if err != nil {
		return s, nil, err
	}
	if len(mid) == 0 || len(mid) > 64 || strings.IndexFunc(mid, func(r rune) bool {
		return r < '0' || r > '9' && r < 'A' || r > 'Z' && r < 'a' || r > 'z'
	}) >= 0 {
		return s, nil, errors.New("qqmusic: invalid album MID")
	}
	owner, err := strconv.ParseInt(s.MusicID, 10, 64)
	if err != nil || owner <= 0 {
		return s, nil, errors.New("QQ 音乐登录凭证缺少账号标识，请重新登录")
	}
	var statusOwner any = s.MusicID
	if owner <= 1<<53-1 {
		statusOwner = owner
	}
	return s, statusOwner, nil
}

// AlbumLiked reads the collection status by album MID, not its numeric ID.
func AlbumLiked(mid string) (bool, error) {
	_, owner, err := albumFavoriteOwner(mid)
	if err != nil {
		return false, err
	}
	var data object
	if err := rpc("music.musicasset.AlbumFavRead", "IsAlbumFan", map[string]any{"uin": owner, "v_albumMid": []string{mid}}, &data); err != nil {
		return false, err
	}
	var liked bool
	raw, ok := obj(data["m_fan"])[mid]
	if !ok || string(raw) == "null" || json.Unmarshal(raw, &liked) != nil {
		return false, errors.New("QQ 音乐未返回专辑收藏状态")
	}
	return liked, nil
}

// LikeAlbum writes only when needed, then confirms the album's resulting status.
func LikeAlbum(mid string, liked bool) error {
	s, _, err := albumFavoriteOwner(mid)
	if err != nil {
		return err
	}
	current, err := AlbumLiked(mid)
	if err != nil || current == liked {
		return err
	}
	method := "CancelFavAlbum"
	if liked {
		method = "FavAlbum"
	}
	var data struct {
		Result     *int              `json:"result"`
		FailedIDs  []json.RawMessage `json:"v_failedAlbumId"`
		FailedMIDs []json.RawMessage `json:"v_failedAlbumMid"`
	}
	if err := rpc("music.musicasset.AlbumFavWrite", method, map[string]any{"uin": s.MusicID, "v_albumMid": []string{mid}}, &data); err != nil {
		return err
	}
	if data.Result == nil || *data.Result != 0 || data.FailedIDs == nil || data.FailedMIDs == nil || len(data.FailedIDs)+len(data.FailedMIDs) != 0 {
		return errors.New("QQ 音乐未确认专辑收藏的更改")
	}
	current, err = AlbumLiked(mid)
	if err != nil {
		return err
	}
	if current != liked {
		return errors.New("QQ 音乐未确认专辑收藏状态")
	}
	return nil
}

func writeResult(module, method string, param any) (object, error) {
	var data object
	if err := rpc(module, method, param, &data); err != nil {
		return nil, err
	}
	if raw, ok := data["retCode"]; !ok || string(raw) != "0" {
		return nil, fmt.Errorf("qqmusic: %s rejected (retCode %s)", method, data["retCode"])
	}
	result := obj(data["result"])
	if result == nil {
		return nil, fmt.Errorf("qqmusic: %s has no result", method)
	}
	return result, nil
}
func Like(id int64, liked bool) error {
	if _, err := signedSession(); err != nil {
		return err
	}
	tracks, err := SongDetails([]int64{id})
	if err != nil {
		return err
	}
	if len(tracks) != 1 || tracks[0].ID != id || tracks[0].MID == "" {
		return errors.New("qqmusic: song identity not found")
	}
	mid := tracks[0].MID
	read := func() (bool, error) {
		var data object
		if err := rpc("music.musicasset.SongFavRead", "IsSongFanByMid", map[string]any{"v_songMid": []string{mid}}, &data); err != nil {
			return false, err
		}
		var value bool
		raw, ok := obj(data["m_fan"])[mid]
		if !ok || json.Unmarshal(raw, &value) != nil {
			return false, errors.New("qqmusic: invalid liked status")
		}
		return value, nil
	}
	current, err := read()
	if err != nil || current == liked {
		return err
	}
	method := "DelSonglist"
	if liked {
		method = "AddSonglist"
	}
	if _, err = writeResult("music.musicasset.PlaylistDetailWrite", method, map[string]any{"dirId": 201, "v_songInfo": []map[string]any{{"songId": id, "songType": 0}}}); err != nil {
		return err
	}
	current, err = read()
	if err != nil {
		return err
	}
	if current != liked {
		return errors.New("QQ 音乐未确认喜欢状态的更改")
	}
	return nil
}

func playlistName(name string) error {
	if strings.TrimSpace(name) == "" || len([]rune(name)) > 20 {
		return errors.New("歌单名称应为 1–20 个字符")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return errors.New("歌单名称含无效字符")
		}
	}
	return nil
}
func ordinaryPlaylist(id int64) (object, error) {
	data, err := ownedPlaylists()
	if err != nil {
		return nil, err
	}
	var found object
	matches := 0
	for _, o := range objects(data, "v_playlist") {
		if number(o, "tid") == id {
			found = o
			matches++
		}
	}
	if matches != 1 || number(found, "dirId") <= 0 || number(found, "dirId") >= 200 || number(found, "dirType") != 0 {
		return nil, errors.New("只能编辑自己创建的普通歌单")
	}
	if uin := text(found, "uin"); uin != "" && uin != sessionSnapshot().MusicID {
		return nil, errors.New("不能编辑其他用户的歌单")
	}
	return found, nil
}

func CreatePlaylist(name string) (*Playlist, error) {
	if _, err := signedSession(); err != nil {
		return nil, err
	}
	if err := playlistName(name); err != nil {
		return nil, err
	}
	result, err := writeResult("music.musicasset.PlaylistBaseWrite", "AddPlaylist", map[string]any{"dirName": name})
	if err != nil {
		return nil, err
	}
	id := number(result, "tid")
	dir := number(result, "dirId")
	if id <= 0 || dir <= 0 || dir >= 200 {
		return nil, errors.New("QQ 音乐未返回新歌单标识")
	}
	entry, err := ordinaryPlaylist(id)
	if err != nil {
		return nil, fmt.Errorf("歌单创建后未能确认，请刷新歌单后检查: %w", err)
	}
	if number(entry, "dirId") != dir {
		return nil, errors.New("QQ 音乐新歌单标识不一致")
	}
	p := libraryPlaylist(entry)
	p.Creator.UserID, _ = strconv.ParseInt(sessionSnapshot().MusicID, 10, 64)
	RememberOwnedPlaylist(p.ID)
	return &p, nil
}

func RenamePlaylist(id int64, name string) error {
	if err := playlistName(name); err != nil {
		return err
	}
	entry, err := ordinaryPlaylist(id)
	if err != nil {
		return err
	}
	dir := number(entry, "dirId")
	read := func() (object, error) {
		var data object
		err := rpc("music.srfDissInfo.aiDissInfo", "uniform_get_Dissinfo", map[string]any{"dirid": dir, "optype": 3, "tag": 1, "userinfo": 0, "pic_dpi": 300}, &data)
		if err != nil {
			return nil, err
		}
		info := obj(data["dirinfo"])
		if info == nil || number(info, "dirid") != dir {
			return nil, errors.New("QQ 音乐未返回歌单设置")
		}
		for _, field := range []string{"title", "desc", "picurl", "picurl2"} {
			var value string
			if json.Unmarshal(info[field], &value) != nil {
				return nil, errors.New("QQ 音乐歌单设置不完整，无法保留后更名")
			}
		}
		var tags []json.RawMessage
		if json.Unmarshal(info["tag"], &tags) != nil || len(tags) > 3 {
			return nil, errors.New("QQ 音乐歌单标签设置不完整")
		}
		return info, nil
	}
	before, err := read()
	if err != nil {
		return err
	}
	show := number(before, "dir_show")
	if show != 1 && show != 2 || text(before, "picurl2") != "" {
		return errors.New("当前歌单设置暂不支持保留后重命名")
	}
	var tags []string
	for _, tag := range objects(before, "tag") {
		tagID := number(tag, "id")
		if tagID <= 0 {
			return errors.New("QQ 音乐歌单标签无效")
		}
		tags = append(tags, strconv.FormatInt(tagID, 10))
	}
	result, err := writeResult("music.musicasset.PlaylistBaseWrite", "EditPlaylist", map[string]any{"dirId": dir, "mask": 95, "dirNewName": name, "dirNewDesc": text(before, "desc"), "dirNewPicUrl": text(before, "picurl"), "dirNewPicUrl2": "", "dirNewtaglist": strings.Join(tags, ","), "dirShow": show})
	if err != nil {
		return err
	}
	if number(result, "tid") != id || number(result, "dirId") != dir {
		return errors.New("QQ 音乐未确认歌单更名")
	}
	after, err := read()
	if err != nil {
		return err
	}
	if text(after, "title") != name || text(after, "desc") != text(before, "desc") || text(after, "picurl") != text(before, "picurl") || number(after, "dir_show") != show {
		return errors.New("QQ 音乐未确认歌单设置，请刷新后检查")
	}
	return nil
}

func AddToPlaylist(id int64, songs ...int64) error      { return editPlaylistSongs(id, true, songs) }
func RemoveFromPlaylist(id int64, songs ...int64) error { return editPlaylistSongs(id, false, songs) }
func editPlaylistSongs(id int64, add bool, songs []int64) error {
	if len(songs) == 0 || len(songs) > 100 {
		return errors.New("一次请选择 1–100 首歌曲")
	}
	entry, err := ordinaryPlaylist(id)
	if err != nil {
		return err
	}
	dir := number(entry, "dirId")
	values := make([]map[string]any, len(songs))
	for i, song := range songs {
		if song <= 0 {
			return errors.New("qqmusic: invalid song ID")
		}
		values[i] = map[string]any{"songId": song, "songType": 0}
	}
	method := "DelSonglist"
	if add {
		method = "AddSonglist"
	}
	result, err := writeResult("music.musicasset.PlaylistDetailWrite", method, map[string]any{"dirId": dir, "v_songInfo": values})
	if err != nil {
		return err
	}
	if number(result, "tid") == id && number(result, "dirId") == dir {
		return nil
	}
	if number(result, "tid") == 0 && number(result, "dirId") == 0 {
		return errors.New("QQ 音乐未确认歌单歌曲的更改，请刷新后检查")
	}
	return errors.New("QQ 音乐歌单写入结果不一致")
}

func DeletePlaylist(id int64) error {
	entry, err := ordinaryPlaylist(id)
	if err != nil {
		return err
	}
	dir := number(entry, "dirId")
	result, err := writeResult("music.musicasset.PlaylistBaseWrite", "DelPlaylist", map[string]any{"dirId": dir})
	if err != nil {
		return err
	}
	if number(result, "tid") != id || number(result, "dirId") != dir || number(result, "updateTime") != 0 {
		return errors.New("QQ 音乐未确认删除歌单")
	}
	data, err := ownedPlaylists()
	if err != nil {
		return err
	}
	for _, o := range objects(data, "v_playlist") {
		if number(o, "tid") == id || number(o, "dirId") == dir {
			return errors.New("QQ 音乐删除后仍返回此歌单")
		}
	}
	return nil
}

// UnfavoritePlaylist removes another creator's playlist from the listener's library.
func UnfavoritePlaylist(id int64) error {
	s, err := signedSession()
	if err != nil {
		return err
	}
	if id <= 0 {
		return errors.New("qqmusic: invalid playlist ID")
	}
	var data struct {
		Result *int              `json:"result"`
		Failed []json.RawMessage `json:"v_failedPlaylistId"`
	}
	if err := rpc("music.musicasset.PlaylistFavWrite", "CancelFavPlaylist", map[string]any{"uin": s.MusicID, "v_playlistId": []int64{id}}, &data); err != nil {
		return err
	}
	if data.Result == nil || data.Failed == nil {
		return errors.New("QQ 音乐未确认取消收藏歌单")
	}
	if *data.Result != 0 || len(data.Failed) != 0 {
		return errors.New("QQ 音乐取消收藏歌单失败")
	}
	return nil
}
