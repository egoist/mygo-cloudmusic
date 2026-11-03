package qqmusic

import (
	"encoding/json"
	"net/http"
	"testing"
)

// Login used to expose album, MV and native-only charts as song playlists that opened empty.
func TestChartsKeepsOnlySongChartsWithWebIdentity(t *testing.T) {
	for _, account := range []struct {
		name    string
		session Session
	}{
		{"anonymous", Session{}},
		{"QQ", Session{MusicID: "123", MusicKey: "fixture-credential", LoginType: 2}},
		{"WeChat", Session{MusicID: "123", MusicKey: "fixture-credential", LoginType: 1}},
	} {
		t.Run(account.name, func(t *testing.T) {
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Comm map[string]any
					Req  struct{ Module, Method string } `json:"req_0"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if request.Comm["ct"] != float64(24) || request.Comm["cv"] != float64(0) || request.Comm["qq"] != nil || request.Comm["authst"] != nil {
					t.Error("chart catalog inherited the account's mobile identity")
				}
				if request.Req.Module != "musicToplist.ToplistInfoServer" || request.Req.Method != "GetAll" {
					t.Errorf("wrong chart request: %+v", request)
				}
				rpcReply(w, json.RawMessage(`{"group":[{"toplist":[
					{"topId":62,"recType":10005,"title":"Song chart","totalNum":100,"song":[{"title":"Song","singerName":"Artist"}]},
					{"topId":23,"recType":10010,"title":"Album chart"},
					{"topId":506,"recType":10010,"title":"Global-K Chart"},
					{"topId":201,"recType":10010,"title":"MV chart"},
					{"topId":75,"recType":10099,"title":"Audio chart"},
					{"topId":135,"recType":10005,"title":"Weekly song chart","period":""}
				]}]}`))
			})
			setSession(account.session)
			charts, err := Charts()
			if err != nil {
				t.Fatal(err)
			}
			if len(charts) != 2 || charts[0].ID != chartIDBase+62 || charts[1].ID != chartIDBase+135 {
				t.Fatalf("non-song charts entered the playlist catalog: %+v", charts)
			}
			if charts[0].TrackCount != 100 || len(charts[0].Preview) != 1 || charts[0].Preview[0].First != "Song" {
				t.Fatalf("song chart lost metadata: %+v", charts[0])
			}
		})
	}
}

// Song charts must load the latest tracks with the web identity even when an account is present.
func TestChartDetailKeepsTracksWithSession(t *testing.T) {
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Comm map[string]any
			Req  struct {
				Module, Method string
				Param          map[string]any
			} `json:"req_0"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Comm["ct"] != float64(24) || request.Comm["cv"] != float64(0) || request.Comm["authst"] != nil {
			t.Error("chart detail inherited the account's mobile identity")
		}
		if request.Req.Module != "musicToplist.ToplistInfoServer" || request.Req.Method != "GetDetail" || request.Req.Param["topid"] != float64(62) || request.Req.Param["period"] != "" || request.Req.Param["num"] != float64(100) {
			t.Errorf("wrong chart detail request: %+v", request)
		}
		rpcReply(w, json.RawMessage(`{"data":{"topId":62,"recType":10005,"title":"Song chart","totalNum":1,"updateTime":"Weekly"},"songInfoList":[{"id":901,"mid":"chartSong","title":"Song","singer":[{"id":902,"mid":"chartArtist","name":"Artist"}],"album":{"id":903,"mid":"chartAlbum","name":"Album"}}]}`))
	})
	setSession(Session{MusicID: "123", MusicKey: "fixture-credential", LoginType: 2})
	chart, err := PlaylistDetail(chartIDBase + 62)
	if err != nil {
		t.Fatal(err)
	}
	if len(chart.Tracks) != 1 || chart.Tracks[0].ID != 901 || chart.Tracks[0].MID != "chartSong" || chart.TrackCount != 1 || chart.UpdateFreq != "Weekly" {
		t.Fatalf("lost chart contents: %+v", chart)
	}
}

// Custom chart metadata and missing song lists used to be cached as successful zero-song playlists.
func TestChartDetailRejectsEmptyShells(t *testing.T) {
	for _, data := range []string{
		`{"data":{"topId":23,"recType":10010,"totalNum":20},"songInfoList":[]}`,
		`{"data":{"topId":506,"recType":10010,"totalNum":3},"songInfoList":[]}`,
		`{"data":{"topId":62,"recType":10005,"totalNum":100},"songInfoList":[]}`,
		`{"data":{"topId":62,"recType":10005,"totalNum":100}}`,
	} {
		t.Run(data, func(t *testing.T) {
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				rpcReply(w, json.RawMessage(data))
			})
			chart, err := PlaylistDetail(chartIDBase + 62)
			if err == nil || chart != nil {
				t.Fatalf("accepted an empty chart shell: %+v, %v", chart, err)
			}
		})
	}
}
