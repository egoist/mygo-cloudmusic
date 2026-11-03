package qqmusic

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

// Personal recommendations used to call the playlist square and repeat the public featured lists.
func TestRecommendedPlaylistsUsesAccountHomeFeed(t *testing.T) {
	for _, loginType := range []int{1, 2} {
		t.Run(map[int]string{1: "WeChat", 2: "QQ"}[loginType], func(t *testing.T) {
			calls := 0
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
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
				if request.Req.Module != "music.recommend.RecommendFeed" || request.Req.Method != "get_recommend_feed" {
					t.Errorf("wrong recommendation source: %s/%s", request.Req.Module, request.Req.Method)
				}
				if request.Comm["ct"] != float64(20) || request.Comm["cv"] != float64(13020008) || request.Comm["qq"] != "10001" || request.Comm["authst"] != "fixture-key" || request.Comm["tmeLoginType"] != float64(loginType) || request.Comm["QIMEI"] != nil {
					t.Error("home feed lost its client identity or account")
				}
				if string(request.Req.Param["direction"]) != "0" || string(request.Req.Param["page"]) != "1" || string(request.Req.Param["s_num"]) != "0" || string(request.Req.Param["v_cache"]) != "[]" {
					t.Errorf("wrong first feed page: %v", request.Req.Param)
				}
				rpcReply(w, json.RawMessage(`{"retcode":0,"v_shelf":[
					{"id":301,"v_niche":[{"v_card":[
						{"id":"0","type":500,"subtype":510,"jumptype":10014,"title":"Daily"},
						{"id":"11","type":500,"subtype":513,"jumptype":10014,"title":"Navigation"}
					]}]},
					{"id":271,"v_niche":[{"v_card":[
						{"id":"9001","type":500,"subtype":0,"jumptype":10014,"title":"Personal A","cover":"https://example.com/a.jpg","cnt":2085000},
						{"id":"42","type":200,"subtype":0,"jumptype":10014,"title":"Song"}
					]},{"v_card":[
						{"id":"9002","type":500,"subtype":0,"jumptype":10014,"title":"Personal B","cover":"https://example.com/b.jpg","cnt":1493000},
						{"id":"9001","type":500,"subtype":0,"jumptype":10014,"title":"Repeated"},
						{"id":"video","type":300,"subtype":311,"jumptype":10012},
						{"id":"not-a-playlist","type":500,"subtype":0,"jumptype":10014},
						{"id":"9003","type":500,"subtype":0,"jumptype":10012}
					]}]}
				]}`))
			})
			setSession(Session{MusicID: "10001", MusicKey: "fixture-key", LoginType: loginType, Device: &wechatDevice{Qimei: "fixture-device"}})
			lists, err := RecommendedPlaylists()
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || len(lists) != 2 {
				t.Fatalf("got %d playlists from %d requests", len(lists), calls)
			}
			if lists[0].ID != 9001 || lists[0].Name != "Personal A" || lists[0].Cover() != "https://example.com/a.jpg" || lists[0].PlayCount != 2085000 || lists[1].ID != 9002 {
				t.Fatalf("lost feed order, playlist identity or metadata: %+v", lists)
			}
		})
	}
}

// A signed-out request must not relabel generic recommendations as personal ones.
func TestRecommendedPlaylistsRequiresAccount(t *testing.T) {
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("signed-out recommendations contacted QQ")
	})
	if _, err := RecommendedPlaylists(); err == nil {
		t.Fatal("accepted a signed-out personal recommendation request")
	}
}

// Rejected or missing feed data must not silently fall back to the public playlist square.
func TestRecommendedPlaylistsRejectsInvalidFeed(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
		ok   bool
	}{
		{"rejected", `{"retcode":12913,"v_shelf":[]}`, false},
		{"missing status", `{"v_shelf":[]}`, false},
		{"missing shelves", `{"retcode":0}`, false},
		{"empty feed", `{"retcode":0,"v_shelf":[]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				rpcReply(w, json.RawMessage(tc.data))
			})
			setSession(Session{MusicID: "10001", MusicKey: "fixture-key", LoginType: 2})
			lists, err := RecommendedPlaylists()
			if (err == nil) != tc.ok || len(lists) != 0 || calls != 1 {
				t.Fatalf("playlists=%d requests=%d error=%v, want success=%v", len(lists), calls, err, tc.ok)
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
