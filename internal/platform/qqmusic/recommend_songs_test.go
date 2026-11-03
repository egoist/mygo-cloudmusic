package qqmusic

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// Song shelves retain the server's groups and complete song order after one batched detail request.
func TestRecommendedSongShelves(t *testing.T) {
	for _, loginType := range []int{1, 2} {
		t.Run(map[int]string{1: "WeChat", 2: "QQ"}[loginType], func(t *testing.T) {
			feedCalls, detailCalls := 0, 0
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Comm map[string]any
					Req  struct {
						Module, Method string
						Param          map[string]json.RawMessage
					} `json:"req_0"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if request.Comm["ct"] != float64(20) || request.Comm["cv"] != float64(2276) || request.Comm["platform"] != "wk_v17" || request.Comm["uin"] != "10001" || request.Comm["qq"] != "10001" || request.Comm["authst"] != "fixture-key" || request.Comm["tmeLoginType"] != float64(loginType) || request.Comm["QIMEI"] != nil {
					t.Error("song feed lost the PC identity or account")
				}
				if !strings.Contains(r.Header.Get("Cookie"), "qm_keyst=fixture-key") {
					t.Error("song feed lost the account cookie")
				}
				switch request.Req.Module + "/" + request.Req.Method {
				case "music.recommend.RecommendFeed/get_recommend_feed":
					feedCalls++
					if string(request.Req.Param["direction"]) != "0" || string(request.Req.Param["page"]) != "1" || string(request.Req.Param["s_num"]) != "0" || string(request.Req.Param["v_cache"]) != "[]" || string(request.Req.Param["v_uniq"]) != "[]" {
						t.Error("song feed did not request the first page")
					}
					rpcReply(w, json.RawMessage(`{"retcode":0,"v_shelf":[
						{"id":301,"v_niche":[{"v_card":[{"id":"700","type":200}]}]},
						{"id":207,"title_template":"听「{String}」也会喜欢💗","title_content":"种子曲\u2062","miscellany":{"songList":"30,20,99,30,500,400,0,-1,broken"},"v_niche":[{"v_card":[{"id":"20","type":200},{"id":"30","type":200},{"id":"500","type":500},{"id":"400","type":400}]}]},
						{"id":207,"title_template":"「{String}」，这是你的幸运好歌🍀","title_content":"Amamiya\u2062","miscellany":{"songList":"20,40"},"v_niche":[{"v_card":[{"id":"20","type":200},{"id":"40","type":200}]}]},
						{"id":207,"title_template":"","title_content":"为你推荐的宝藏单曲","miscellany":{"songList":"50,60"},"v_niche":[{"v_card":[{"id":"50","type":200}]}]},
						{"id":207,"title_template":"值得红心的宝藏歌曲","title_content":"无需插入的歌手","v_niche":[{"v_card":[{"id":"80","type":200},{"id":"80","type":200},{"id":"800","type":500},{"id":"not-a-song","type":200}]}]}
					]}`))
				case "music.trackInfo.UniformRuleCtrl/CgiGetTrackInfo":
					detailCalls++
					var ids []int64
					var types []int
					_ = json.Unmarshal(request.Req.Param["ids"], &ids)
					_ = json.Unmarshal(request.Req.Param["types"], &types)
					if !reflect.DeepEqual(ids, []int64{30, 20, 99, 40, 50, 60, 80}) || !reflect.DeepEqual(types, make([]int, 7)) {
						t.Errorf("batched wrong song IDs or types: %v %v", ids, types)
					}
					var tracks []any
					for _, id := range []int64{80, 40, 50, 99, 20, 30} {
						tracks = append(tracks, recommendedTrackFixture(id))
					}
					// Unavailable song 60 is omitted; no sparse card may impersonate its details.
					rpcReply(w, map[string]any{"tracks": tracks})
				default:
					t.Errorf("unexpected song feed request: %s/%s", request.Req.Module, request.Req.Method)
					http.Error(w, "unexpected request", http.StatusBadRequest)
				}
			})
			setSession(Session{MusicID: "10001", MusicKey: "fixture-key", LoginType: loginType, Device: &wechatDevice{Qimei: "fixture-device"}})
			shelves, err := RecommendedSongShelves()
			if err != nil {
				t.Fatal(err)
			}
			if feedCalls != 1 || detailCalls != 1 || len(shelves) != 4 {
				t.Fatalf("feed=%d detail=%d shelves=%d", feedCalls, detailCalls, len(shelves))
			}
			wantTitles := []string{"听「种子曲」也会喜欢💗", "「Amamiya」，这是你的幸运好歌🍀", "为你推荐的宝藏单曲", "值得红心的宝藏歌曲"}
			wantIDs := [][]int64{{30, 20, 99}, {20, 40}, {50}, {80}}
			for i, shelf := range shelves {
				var ids []int64
				for _, track := range shelf.Tracks {
					ids = append(ids, track.ID)
					if track.MID == "" || track.MediaMID == "" || track.Size128 == 0 || track.Duration != 210000 || len(track.Artists) != 1 || track.Album.ID == 0 || track.Album.PicURL == "" || track.Volume == nil || track.NeedsDetails {
						t.Errorf("song %d lacks complete playback metadata", track.ID)
					}
				}
				if shelf.Title != wantTitles[i] || !reflect.DeepEqual(ids, wantIDs[i]) {
					t.Errorf("shelf %d: title=%q IDs=%v", i, shelf.Title, ids)
				}
			}
		})
	}
}

func recommendedTrackFixture(id int64) map[string]any {
	return map[string]any{
		"id": id, "mid": fmt.Sprint("song-mid-", id), "title": fmt.Sprint("Song ", id), "interval": 210,
		"singer": []any{map[string]any{"id": 6, "mid": "artist-mid", "name": "Artist"}},
		"album":  map[string]any{"id": 7, "mid": "album-mid", "name": "Album"},
		"file":   map[string]any{"media_mid": fmt.Sprint("media-mid-", id), "size_128mp3": 4000000},
		"volume": map[string]any{"gain": -6.2, "peak": 0.98, "lra": 7.5},
	}
}

// Signed-out song recommendations must not contact QQ or relabel generic songs as personal.
func TestRecommendedSongShelvesRequiresAccount(t *testing.T) {
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) { t.Error("signed-out song feed contacted QQ") })
	if _, err := RecommendedSongShelves(); err == nil {
		t.Fatal("accepted a signed-out song recommendation request")
	}
}

// Missing feed status and detail metadata must fail instead of creating empty playable songs.
func TestRecommendedSongShelvesRejectsInvalidData(t *testing.T) {
	for _, tc := range []struct {
		name, feed, details string
		ok                  bool
	}{
		{"missing status", `{"v_shelf":[]}`, "", false},
		{"missing shelves", `{"retcode":0}`, "", false},
		{"rejected", `{"retcode":12913,"v_shelf":[]}`, "", false},
		{"empty feed", `{"retcode":0,"v_shelf":[]}`, "", true},
		{"only other types", `{"retcode":0,"v_shelf":[{"id":207,"miscellany":{"songList":"400,500"},"v_niche":[{"v_card":[{"id":"400","type":400},{"id":"500","type":500}]}]}]}`, "", true},
		{"missing details", `{"retcode":0,"v_shelf":[{"id":207,"miscellany":{"songList":"30"}}]}`, `{}`, false},
		{"empty details", `{"retcode":0,"v_shelf":[{"id":207,"miscellany":{"songList":"30"}}]}`, `{"tracks":[]}`, false},
		{"incomplete details", `{"retcode":0,"v_shelf":[{"id":207,"miscellany":{"songList":"30"}}]}`, `{"tracks":[{"id":30,"title":"Sparse"}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 1 {
					rpcReply(w, json.RawMessage(tc.feed))
				} else {
					if tc.details == "" {
						t.Error("feed without song IDs fetched details")
						http.Error(w, "unexpected details", http.StatusBadRequest)
						return
					}
					rpcReply(w, json.RawMessage(tc.details))
				}
			})
			setSession(Session{MusicID: "10001", MusicKey: "fixture-key", LoginType: 2})
			shelves, err := RecommendedSongShelves()
			if (err == nil) != tc.ok || len(shelves) != 0 {
				t.Fatalf("shelves=%d error=%v, want success=%v", len(shelves), err, tc.ok)
			}
			if tc.name == "rejected" {
				var protocol *ProtocolError
				if !errors.As(err, &protocol) || protocol.Code != 12913 {
					t.Fatalf("lost feed rejection: %v", err)
				}
			}
		})
	}
}
