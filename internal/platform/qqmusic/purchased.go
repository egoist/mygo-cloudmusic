package qqmusic

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

const purchasedPath = "/shop/fcgi-bin/fcg_get_order"

// purchasedPage follows the offset-based order list used by QQ Music's web profile.
func purchasedPage(kind, list string, limit, offset int) ([]object, bool, error) {
	s, err := signedSession()
	if err != nil {
		return nil, false, err
	}
	limit = min(max(limit, 1), 100)
	offset = max(offset, 0)
	token := strconv.FormatUint(uint64(hash33(s.MusicKey, 5381)), 10)
	var body object
	err = cgi(purchasedPath, url.Values{
		"from": {"1"}, "cmd": {"sales_album"}, "type": {kind},
		"uin": {s.MusicID}, "start": {strconv.Itoa(offset)}, "num": {strconv.Itoa(limit)}, "platform": {"yqq"},
		"ct": {"24"}, "cv": {"4747474"}, "inCharset": {"utf-8"}, "notice": {"0"}, "needNewCode": {"1"},
		"g_tk": {token}, "g_tk_new_20200303": {token},
	}, &body)
	if err != nil {
		return nil, false, err
	}
	data := obj(body["data"])
	if data == nil {
		return nil, false, fmt.Errorf("qqmusic: missing purchased music data")
	}
	for _, key := range []string{"code", "subcode"} {
		if raw, ok := data[key]; ok {
			var code int
			if json.Unmarshal(raw, &code) != nil {
				return nil, false, fmt.Errorf("qqmusic: invalid purchased music %s", key)
			}
			if code != 0 {
				return nil, false, &ProtocolError{0, code, "cgi", purchasedPath}
			}
		}
	}
	var total json.Number
	if json.Unmarshal(data["total"], &total) != nil {
		return nil, false, fmt.Errorf("qqmusic: invalid purchased music total")
	}
	count, err := strconv.ParseInt(total.String(), 10, 64)
	if err != nil || count < 0 {
		return nil, false, fmt.Errorf("qqmusic: invalid purchased music total")
	}
	var entries []object
	raw, ok := data[list]
	if !ok || string(raw) == "null" {
		// The official profile treats an omitted empty list as []; only do so
		// after a successful CGI response explicitly reports no purchases.
		if count == 0 {
			return []object{}, false, nil
		}
		return nil, false, fmt.Errorf("qqmusic: missing purchased music list")
	}
	if json.Unmarshal(raw, &entries) != nil {
		return nil, false, fmt.Errorf("qqmusic: invalid purchased music list")
	}
	if len(entries) == 0 && int64(offset) < count {
		return nil, false, fmt.Errorf("qqmusic: incomplete purchased music list")
	}
	return entries, int64(offset)+int64(len(entries)) < count, nil
}

func PurchasedSongs(limit, offset int) ([]Track, bool, error) {
	entries, more, err := purchasedPage("2", "songlist", limit, offset)
	if err != nil {
		return nil, false, err
	}
	tracks := make([]Track, 0, len(entries))
	for _, entry := range entries {
		track := convertTrack(entry)
		if track.ID <= 0 || track.MID == "" {
			return nil, false, fmt.Errorf("qqmusic: missing purchased song identity")
		}
		tracks = append(tracks, track)
	}
	return tracks, more, nil
}

func PurchasedAlbums(limit, offset int) ([]AlbumProfile, bool, error) {
	entries, more, err := purchasedPage("1", "albumlist", limit, offset)
	if err != nil {
		return nil, false, err
	}
	albums := make([]AlbumProfile, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			return nil, false, fmt.Errorf("qqmusic: missing purchased album identity")
		}
		if text(entry, "mid", "albummid", "albumMID", "albumMid", "album_mid") == "" {
			entry["mid"] = entry["pmid"]
		}
		album := AlbumProfile{Album: convertAlbum(entry), Size: int(number(entry, "song_count", "songnum", "totalNum"))}
		if album.ID <= 0 || album.MID == "" {
			return nil, false, fmt.Errorf("qqmusic: missing purchased album identity")
		}
		for _, singer := range objects(entry, "singers") {
			album.Artists = append(album.Artists, convertArtist(singer))
		}
		if len(album.Artists) == 0 {
			singer := Artist{ID: number(entry, "singerid", "singerID", "singer_id"), MID: text(entry, "singermid", "singerMID", "singerMid", "singer_mid"), Name: text(entry, "singerName", "singername", "singer_name", "singer")}
			if singer.Name != "" {
				album.Artists = append(album.Artists, singer)
			}
		}
		if date := text(entry, "release_time", "publishDate"); date != "" {
			if parsed, err := time.Parse("2006-01-02", date); err == nil {
				album.PublishTime = parsed.UnixMilli()
			}
		}
		albums = append(albums, album)
	}
	return albums, more, nil
}
