package qqmusic

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Desktop uses ct=11 with wrapped lists for all three categories, independently of the Lite identity.
func TestDesktopSearchCategories(t *testing.T) {
	for _, signedIn := range []bool{false, true} {
		for _, tc := range []struct {
			name, key string
			kind      SearchType
			code      int
			item      map[string]any
		}{
			{"songs", "song", SearchSongs, 0, map[string]any{"id": 8201, "mid": "desktop-song", "name": "Song", "file": map[string]any{"media_mid": "desktop-media"}}},
			{"artists", "singer", SearchArtists, 1, map[string]any{"id": 8202, "mid": "desktop-artist", "name": "Artist"}},
			{"albums", "album", SearchAlbums, 2, map[string]any{"id": 8203, "mid": "desktop-album", "name": "Album"}},
		} {
			t.Run(tc.name+map[bool]string{false: " anonymous", true: " account"}[signedIn], func(t *testing.T) {
				withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
					var request struct {
						Comm map[string]any
						Req  struct {
							Method string
							Param  map[string]any
						} `json:"req_0"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
						return
					}
					if request.Req.Method != "DoSearchForQQMusicDesktop" || request.Comm["ct"] != float64(11) || request.Comm["cv"] != float64(0) || request.Req.Param["search_type"] != float64(tc.code) || request.Req.Param["page_num"] != float64(2) || request.Req.Param["num_per_page"] != float64(20) {
						t.Error("incorrect Desktop identity or paging")
					}
					if signedIn {
						if request.Comm["uin"] != "123" || request.Comm["authst"] != "desktop-key" || !strings.Contains(r.Header.Get("Cookie"), "qm_keyst=desktop-key") {
							t.Error("Desktop lost account identity")
						}
					} else if request.Comm["uin"] != "0" || request.Comm["authst"] != nil || r.Header.Get("Cookie") != "tmeLoginType=-1" {
						t.Error("Desktop anonymous request inherited account state")
					}
					rpcReply(w, map[string]any{"code": 0, "body": map[string]any{tc.key: map[string]any{"list": []any{tc.item}}}, "meta": map[string]any{"sum": 61, "is_filter": 0}})
				})
				if signedIn {
					setSession(Session{MusicID: "123", MusicKey: "desktop-key", LoginType: 2})
				}
				result, err := SearchPage("Jay", tc.kind, 20, 20)
				if err != nil || result.Source != SearchDesktop || result.Len() != 1 || result.Total != 61 || result.NextOffset != 40 {
					t.Fatalf("Desktop result: %+v %v", result, err)
				}
			})
		}
	}
}
