package qqmusic

import (
	"encoding/json"
	"net/http"
	"testing"
)

// Lite search preserves account identifiers as strings and uses anonymity only without a valid local session.
func TestSearchSessionSelection(t *testing.T) {
	for _, tc := range []struct {
		name          string
		session       Session
		authenticated bool
	}{
		{"QQ", Session{MusicID: "123", MusicKey: "fixture-key", LoginType: 2}, true},
		{"WeChat large ID", Session{MusicID: "9007199254740993", MusicKey: "fixture-key", LoginType: 1, Device: &wechatDevice{Qimei: "fixture-device"}}, true},
		{"expired", Session{MusicID: "123", MusicKey: "expired-key", LoginType: 2, CreatedAt: 1, ExpiresIn: 1}, false},
		{"missing key", Session{MusicID: "123", LoginType: 2}, false},
		{"invalid login type", Session{MusicID: "123", MusicKey: "fixture-key", LoginType: 9}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Comm map[string]any
					Req  struct{ Param map[string]any } `json:"req_0"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				if tc.authenticated {
					if request.Comm["ct"] != float64(24) || request.Comm["uin"] != tc.session.MusicID || request.Comm["qq"] != tc.session.MusicID || request.Comm["authst"] != tc.session.MusicKey || request.Comm["tmeLoginType"] != float64(tc.session.LoginType) || request.Req.Param["search_type"] != float64(0) {
						t.Error("account search did not preserve the PC identity")
					}
				} else if request.Comm["tmeAppID"] != "qqmusiclight" || request.Comm["authst"] != nil || r.Header.Get("Cookie") != "tmeLoginType=-1" || request.Req.Param["search_type"] != float64(7) {
					t.Error("anonymous search sent an invalid or expired account")
				}
				rpcReply(w, map[string]any{"code": 0, "body": map[string]any{"item_song": []any{map[string]any{"id": 101, "mid": "songMid", "title": "Song"}}}, "meta": map[string]any{"sum": 1}})
			})
			setSession(tc.session)
			result, err := SearchPageSource("Song", SearchSongs, 20, 0, SearchLite)
			if err != nil || result.Len() != 1 {
				t.Fatalf("search failed: rows=%d error=%v", result.Len(), err)
			}
			if sessionSnapshot() != tc.session {
				t.Fatal("search changed the account session")
			}
		})
	}
}
