package qqmusic

import (
	"encoding/json"
	"net/http"
	"testing"
)

// The old CGI fails; public comment RPCs need a 25-item cap and the preceding page's opaque sequence.
func TestCommentsWebRPCAndCursor(t *testing.T) {
	for _, account := range []Session{{}, {MusicID: "123", MusicKey: "fixture-key", LoginType: 2}, {MusicID: "456", MusicKey: "fixture-key", LoginType: 1}} {
		t.Run(map[int]string{0: "anonymous", 2: "QQ", 1: "WeChat"}[account.LoginType], func(t *testing.T) {
			newCalls, hotCalls := 0, 0
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				var request struct {
					Comm map[string]any
					Req  struct {
						Module, Method string
						Param          map[string]any
					} `json:"req_0"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				p := request.Req.Param
				if request.Req.Module != "music.globalComment.CommentRead" || p["BizId"] != "42" || p["BizType"] != float64(1) || p["PageSize"] != float64(25) {
					t.Errorf("wrong comments request: %+v", request)
				}
				if len(request.Comm) != 2 || request.Comm["ct"] != float64(24) || request.Comm["cv"] != float64(0) || r.Header.Get("Cookie") != "" {
					t.Error("public comments inherited account identity")
				}
				entries := []map[string]any{}
				more := any(false)
				switch request.Req.Method {
				case "GetHotCommentList":
					hotCalls++
					if p["HotType"] != float64(1) || p["WithAirborne"] != float64(0) {
						t.Error("wrong hot comment parameters")
					}
					entries = append(entries, map[string]any{"CmId": "hot!opaque", "Content": "hot"})
				case "GetNewCommentList":
					newCalls++
					if p["WithHot"] != float64(0) || p["FromCommentId"] != "" {
						t.Error("hot comments mixed into latest")
					}
					if newCalls == 1 {
						if p["PageNum"] != float64(0) || p["LastCommentSeqNo"] != "" {
							t.Error("first page has a stale cursor")
						}
						entries = append(entries, map[string]any{"CmId": "1!opaque", "SeqNo": "18446744073709551615", "Content": "one &amp; 二", "Nick": "listener", "Avatar": "https://fixture.invalid/avatar", "PubTime": 1700000000, "PraiseNum": 7})
						more = 1
					} else {
						if p["PageNum"] != float64(1) || p["LastCommentSeqNo"] != "18446744073709551615" || p["LastTotal"] != float64(2) || p["LastTotalVer"] != "1793343305312403406" {
							t.Errorf("lost next-page state: %+v", p)
						}
						entries = append(entries, map[string]any{"CmId": "2!opaque", "SeqNo": "last-sequence", "Content": "two"})
					}
				default:
					t.Errorf("unexpected method %s", request.Req.Method)
				}
				rpcReply(w, map[string]any{"TotalVer": "1793343305312403406", "CommentList": map[string]any{"Comments": entries, "Total": 2, "HasMore": more}})
			})
			setSession(account)
			first, err := ThreadComments("42", 0, 30)
			if err != nil {
				t.Fatal(err)
			}
			if !first.More || first.Total != 2 || len(first.Latest) != 1 || len(first.Hot) != 1 {
				t.Fatalf("first page: %+v", first)
			}
			cm := first.Latest[0]
			if cm.ID == 0 || cm.RemoteID != "1!opaque" || cm.Content != "one & 二" || cm.Time != 1700000000000 || cm.LikedCount != 7 || cm.User.Nickname != "listener" || cm.User.AvatarURL == "" {
				t.Fatalf("lost comment metadata: %+v", cm)
			}
			next, err := ThreadComments("42", 1, 30)
			if err != nil {
				t.Fatal(err)
			}
			if next.More || len(next.Latest) != 1 || next.Latest[0].Content != "two" || next.Latest[0].ID == cm.ID || len(next.Hot) != 0 || newCalls != 2 || hotCalls != 1 {
				t.Fatalf("next page: %+v; calls new=%d hot=%d", next, newCalls, hotCalls)
			}
			if sessionSnapshot() != account {
				t.Error("public read changed the session")
			}
		})
	}
}

// An empty comment list is valid, but rejected or malformed replies must remain errors.
func TestCommentsEmptyAndMalformed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		data    any
		code    int
		wantErr bool
	}{
		{"empty", map[string]any{"CommentList": map[string]any{"Comments": []any{}, "Total": 0, "HasMore": 0}}, 0, false},
		{"missing list", map[string]any{}, 0, true},
		{"malformed list", map[string]any{"CommentList": map[string]any{"Comments": "bad"}}, 0, true},
		{"missing IDs", map[string]any{"CommentList": map[string]any{"Comments": []any{map[string]any{"Content": "bad"}}}}, 0, true},
		{"rejected", map[string]any{}, 10000, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "req_0": map[string]any{"code": tc.code, "data": tc.data}})
			})
			page, err := ThreadComments("43", 0, 25)
			if (err != nil) != tc.wantErr {
				t.Fatalf("page=%+v error=%v", page, err)
			}
			if !tc.wantErr && (page.More || page.Total != 0 || len(page.Latest) != 0 || len(page.Hot) != 0) {
				t.Fatalf("empty page: %+v", page)
			}
		})
	}
}

// Personal radio must keep the QQ track MID so the existing player can resolve it.
func TestPersonalRadioTrackIdentity(t *testing.T) {
	fakeLibraryRPC(t, func(method string, param map[string]json.RawMessage) any {
		if method != "get_radio_track" || string(param["id"]) != "99" {
			t.Errorf("unexpected radio %s: %v", method, param)
		}
		return map[string]any{"tracks": []map[string]any{{"id": 42, "mid": "qqRadioMid", "title": "Radio", "interval": 120}}}
	})
	tracks, err := PersonalFM(FMDefault)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 || tracks[0].MID != "qqRadioMid" || tracks[0].Duration != 120000 {
		t.Fatalf("radio tracks: %+v", tracks)
	}
}
