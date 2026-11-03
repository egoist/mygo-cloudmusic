package qqmusic

import (
	"fmt"
	"strconv"
)

// RecommendedPlaylists reads the home feed, independently of the public playlist square.
func RecommendedPlaylists() ([]Playlist, error) {
	s, err := signedSession()
	if err != nil {
		return nil, err
	}
	// This feed identity includes the personal playlist shelf. The web identity
	// returns a small generic feed, while the Android identity needs a device.
	comm := map[string]any{"ct": 20, "cv": 13020008, "qq": s.MusicID, "authst": s.MusicKey, "tmeLoginType": s.LoginType}
	var data struct {
		Retcode *int `json:"retcode"`
		Shelves []struct {
			Niches []struct {
				Cards []struct {
					ID       string `json:"id"`
					Type     int    `json:"type"`
					Subtype  int    `json:"subtype"`
					JumpType int    `json:"jumptype"`
					Title    string `json:"title"`
					Cover    string `json:"cover"`
					Count    int64  `json:"cnt"`
				} `json:"v_card"`
			} `json:"v_niche"`
		} `json:"v_shelf"`
	}
	const module, method = "music.recommend.RecommendFeed", "get_recommend_feed"
	param := map[string]any{"direction": 0, "page": 1, "s_num": 0, "v_cache": []string{}}
	if err := rpcWithComm(module, method, param, &data, comm, nil); err != nil {
		return nil, err
	}
	if data.Retcode == nil {
		return nil, fmt.Errorf("qqmusic: missing home feed status")
	}
	if *data.Retcode != 0 {
		return nil, &ProtocolError{0, *data.Retcode, module, method}
	}
	if data.Shelves == nil {
		return nil, fmt.Errorf("qqmusic: missing home feed shelves")
	}
	var out []Playlist
	seen := map[int64]bool{}
	for _, shelf := range data.Shelves {
		for _, niche := range shelf.Niches {
			for _, card := range niche.Cards {
				// Daily mixes and navigation cards also use type 500, but
				// cannot be opened as an ordinary playlist.
				if card.Type != 500 || card.Subtype != 0 || card.JumpType != 10014 {
					continue
				}
				id, err := strconv.ParseInt(card.ID, 10, 64)
				if err != nil || id <= 0 || seen[id] {
					continue
				}
				seen[id] = true
				out = append(out, Playlist{ID: id, Name: card.Title, CoverURL: card.Cover, PlayCount: float64(card.Count)})
			}
		}
	}
	return out, nil
}
