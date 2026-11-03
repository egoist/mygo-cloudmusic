package qqmusic

import (
	"fmt"
	"strconv"
	"strings"
)

// SongShelf is one ordered group of songs selected by the account's home feed.
type SongShelf struct {
	Title  string
	Tracks []Track
}

type songFeedShelf struct {
	ID            int64  `json:"id"`
	TitleTemplate string `json:"title_template"`
	TitleContent  string `json:"title_content"`
	Miscellany    struct {
		SongList string `json:"songList"`
	} `json:"miscellany"`
	Niches []struct {
		Cards []struct {
			ID   string `json:"id"`
			Type int    `json:"type"`
		} `json:"v_card"`
	} `json:"v_niche"`
}

type songFeedBatch struct {
	Retcode  *int            `json:"retcode"`
	LoadMark *int            `json:"load_mark"`
	Shelves  []songFeedShelf `json:"v_shelf"`
}

func songFeedComm(s Session) map[string]any {
	return map[string]any{
		"format": "json", "ct": 20, "cv": 2276, "platform": "wk_v17",
		"inCharset": "utf-8", "outCharset": "utf-8", "notice": 0, "needNewCode": 1,
		"uin": s.MusicID, "qq": s.MusicID, "authst": s.MusicKey, "tmeLoginType": s.LoginType,
		"g_tk": 0, "g_tk_new_20200303": 0,
	}
}

// RecommendedSongShelves keeps the feed's groups and order, with full playback
// metadata fetched once for the distinct song IDs across all shelves.
func RecommendedSongShelves() ([]SongShelf, error) {
	s, err := signedSession()
	if err != nil {
		return nil, err
	}
	comm := songFeedComm(s)
	feed, err := songFeedShelves(comm)
	if err != nil {
		return nil, err
	}
	var shelves []SongShelf
	var shelfIDs [][]int64
	var ids []int64
	seen := map[int64]bool{}
	for _, shelf := range feed {
		if shelf.ID != 207 {
			continue
		}
		ordered := shelf.songIDs()
		if len(ordered) == 0 {
			continue
		}
		title := shelf.TitleTemplate
		if title == "" {
			title = shelf.TitleContent
		} else {
			title = strings.ReplaceAll(title, "{String}", shelf.TitleContent)
		}
		title = strings.TrimSpace(strings.ReplaceAll(title, "\u2062", ""))
		if title == "" {
			title = "为你推荐的歌曲"
		}
		shelves = append(shelves, SongShelf{Title: title})
		shelfIDs = append(shelfIDs, ordered)
		for _, id := range ordered {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	tracks, err := songFeedTracks(ids, comm)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]Track, len(tracks))
	for _, track := range tracks {
		byID[track.ID] = track
	}
	out := make([]SongShelf, 0, len(shelves))
	for i, shelf := range shelves {
		for _, id := range shelfIDs[i] {
			if track, ok := byID[id]; ok {
				shelf.Tracks = append(shelf.Tracks, track)
			}
		}
		if len(shelf.Tracks) > 0 {
			out = append(out, shelf)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("qqmusic: song feed returned no song details")
	}
	return out, nil
}

func songFeedShelves(comm map[string]any) ([]songFeedShelf, error) {
	// Home shows up to three song groups. Keep every group in a received batch,
	// but stop requesting more once the display is full.
	const shelfLimit = 3
	var shelves []songFeedShelf
	groups := 0
	for page := 1; ; page++ {
		batch, err := readSongFeedBatch(comm, page, len(shelves))
		if err != nil {
			if groups == 0 {
				return nil, err
			}
			// A continuation failure must not discard songs already received.
			break
		}
		shelves = append(shelves, batch.Shelves...)
		for _, shelf := range batch.Shelves {
			if shelf.ID == 207 && len(shelf.songIDs()) > 0 {
				groups++
			}
		}
		if len(batch.Shelves) == 0 || batch.LoadMark == nil || *batch.LoadMark != 0 || groups >= shelfLimit {
			break
		}
	}
	return shelves, nil
}

func readSongFeedBatch(comm map[string]any, page, shelfCount int) (songFeedBatch, error) {
	const module, method = "music.recommend.RecommendFeed", "get_recommend_feed"
	direction := 1
	if page == 1 {
		direction = 0
	}
	param := map[string]any{"direction": direction, "page": page, "s_num": shelfCount, "v_cache": []string{}, "v_uniq": []string{}}
	var data songFeedBatch
	if err := rpcWithComm(module, method, param, &data, comm, nil); err != nil {
		return data, err
	}
	if data.Retcode == nil {
		return data, fmt.Errorf("qqmusic: missing song feed status")
	}
	if *data.Retcode != 0 {
		return data, &ProtocolError{0, *data.Retcode, module, method}
	}
	if data.Shelves == nil {
		return data, fmt.Errorf("qqmusic: missing song feed shelves")
	}
	return data, nil
}

func (s songFeedShelf) songIDs() []int64 {
	var cards []int64
	songs, other := map[int64]bool{}, map[int64]bool{}
	for _, niche := range s.Niches {
		for _, card := range niche.Cards {
			id, err := strconv.ParseInt(card.ID, 10, 64)
			if err != nil || id <= 0 {
				continue
			}
			if card.Type == 200 {
				songs[id] = true
				cards = append(cards, id)
			} else {
				other[id] = true
			}
		}
	}
	// songList includes songs beyond the visible cards. An explicitly different
	// card type must not turn a playlist or programme ID into a song request.
	var out []int64
	seen := map[int64]bool{}
	add := func(id int64) {
		if id > 0 && !seen[id] && (!other[id] || songs[id]) {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, raw := range strings.Split(s.Miscellany.SongList, ",") {
		if id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil {
			add(id)
		}
	}
	if len(out) == 0 {
		for _, id := range cards {
			add(id)
		}
	}
	return out
}

func songFeedTracks(ids []int64, comm map[string]any) ([]Track, error) {
	var data struct {
		Tracks []object `json:"tracks"`
	}
	param := map[string]any{"ids": ids, "types": make([]int, len(ids))}
	if err := rpcWithComm("music.trackInfo.UniformRuleCtrl", "CgiGetTrackInfo", param, &data, comm, nil); err != nil {
		return nil, err
	}
	if data.Tracks == nil {
		return nil, fmt.Errorf("qqmusic: missing song feed track details")
	}
	return convertTracks(data.Tracks), nil
}
