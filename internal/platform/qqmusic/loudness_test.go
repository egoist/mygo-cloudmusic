package qqmusic

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func isolateLoudnessCache(t *testing.T) {
	t.Helper()
	oldCache, oldPath := songLoudnessCache, loudnessCacheFile
	path := filepath.Join(t.TempDir(), "loudness.json")
	loudnessCacheFile = func() (string, error) { return path, nil }
	songLoudnessCache = &loudnessCache{}
	t.Cleanup(func() { songLoudnessCache, loudnessCacheFile = oldCache, oldPath })
}

// A long queue must split at QQ's 30-song limit and reuse measurements after restarting.
func TestLoudnessBatchLimitAndPersistence(t *testing.T) {
	isolateLoudnessCache(t)
	calls := 0
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		var comm map[string]any
		_ = json.Unmarshal(payload["comm"], &comm)
		delete(payload, "comm")
		if len(payload) > 30 || len(payload) == 0 {
			t.Errorf("batch has %d songs", len(payload))
		}
		if comm["ct"] != float64(24) || comm["cv"] != float64(0) || comm["uin"] != "0" || comm["format"] != "json" || comm["authst"] != nil || r.Header.Get("Cookie") != "" {
			t.Errorf("loudness request inherited login identity: comm=%v", comm)
		}
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Referer") != "https://y.qq.com/" || r.UserAgent() != "Mozilla/5.0" {
			t.Errorf("wrong loudness HTTP request: %s %v", r.Method, r.Header)
		}
		replies := map[string]any{"code": 0}
		for key, raw := range payload {
			var request struct {
				Module, Method string
				Param          struct {
					MID string `json:"song_mid"`
				}
			}
			_ = json.Unmarshal(raw, &request)
			if request.Module != "music.pf_song_detail_svr" || request.Method != "get_song_detail_yqq" || request.Param.MID == "" {
				t.Errorf("wrong song request: %+v", request)
			}
			replies[key] = map[string]any{"code": 0, "data": map[string]any{"track_info": map[string]any{"mid": request.Param.MID, "volume": map[string]any{"gain": -7.603, "peak": 0.946, "lra": 3.712}, "vf": []float64{-2, 1, 5}}}}
		}
		_ = json.NewEncoder(w).Encode(replies)
	})
	setSession(Session{MusicID: "123", MusicKey: "fixture-credential", LoginType: 2})
	tracks := make([]Track, 65)
	for i := range tracks {
		tracks[i] = Track{MID: fmt.Sprintf("loudness%02d", i)}
	}
	tracks = append(tracks, tracks[0])
	volumes, err := SongLoudnesses(tracks)
	if err != nil || len(volumes) != 65 || calls != 3 {
		t.Fatalf("volumes=%d calls=%d error=%v", len(volumes), calls, err)
	}
	want := Loudness{Gain: -7.603, Peak: 0.946, LRA: 3.712}
	if volumes[tracks[0].MID] != want {
		t.Fatalf("standard-master measurement lost or replaced by vf: %+v", volumes[tracks[0].MID])
	}
	songLoudnessCache = &loudnessCache{}
	volume, err := SongLoudness(tracks[0])
	if err != nil || volume != want || calls != 3 {
		t.Fatalf("persisted measurement not reused: volume=%+v calls=%d error=%v", volume, calls, err)
	}
}

// Unmeasured songs are cacheable, while measurements expire after 30 days.
func TestLoudnessCacheExpiry(t *testing.T) {
	isolateLoudnessCache(t)
	now := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	songLoudnessCache.put(map[string]Loudness{"unmeasured": {}}, now)
	reopened := &loudnessCache{}
	if volume, ok := reopened.get("unmeasured", now.Add(loudnessLifetime-time.Second)); !ok || volume != (Loudness{}) {
		t.Fatalf("zero measurement not retained: %+v, %v", volume, ok)
	}
	if _, ok := reopened.get("unmeasured", now.Add(loudnessLifetime)); ok {
		t.Fatal("expired measurement used")
	}
}

// One failed detail must not discard the other songs or cache an invalid measurement.
func TestLoudnessPartialFailure(t *testing.T) {
	isolateLoudnessCache(t)
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":  0,
			"req_0": map[string]any{"code": 0, "data": map[string]any{"track_info": map[string]any{"mid": "measured", "volume": map[string]any{"gain": -6, "peak": 0.9}}}},
			"req_1": map[string]any{"code": 500003},
			"req_2": map[string]any{"code": 0, "data": map[string]any{"track_info": map[string]any{"mid": "malformed", "volume": map[string]any{"gain": "broken", "peak": 1}}}},
			"req_3": map[string]any{"code": 0, "data": map[string]any{"track_info": map[string]any{"mid": "different", "volume": map[string]any{"gain": -3, "peak": 1}}}},
			"req_4": map[string]any{"code": 0, "data": map[string]any{"track_info": map[string]any{"mid": "unmeasured"}}},
		})
	})
	tracks := []Track{{MID: "measured"}, {MID: "rejected"}, {MID: "malformed"}, {MID: "mismatch"}, {MID: "unmeasured"}}
	volumes, err := SongLoudnesses(tracks)
	if err == nil || len(volumes) != 2 || volumes["measured"].Gain != -6 {
		t.Fatalf("partial response: volumes=%+v error=%v", volumes, err)
	}
	for _, mid := range []string{"rejected", "malformed", "mismatch"} {
		if _, ok := songLoudnessCache.get(mid, time.Now()); ok {
			t.Errorf("failed song %s cached", mid)
		}
	}
	if _, ok := songLoudnessCache.get("unmeasured", time.Now()); !ok {
		t.Fatal("unmeasured song not cached")
	}
}

// A restored song with only its numeric ID uses the numeric detail method and learns its MID.
func TestLoudnessNumericID(t *testing.T) {
	isolateLoudnessCache(t)
	// This case needs an unknown MID; an earlier -count run may have learned it.
	mediaRegistry.Lock()
	previous, existed := mediaRegistry.tracks[901337]
	delete(mediaRegistry.tracks, 901337)
	mediaRegistry.Unlock()
	t.Cleanup(func() {
		mediaRegistry.Lock()
		defer mediaRegistry.Unlock()
		if existed {
			mediaRegistry.tracks[901337] = previous
		} else {
			delete(mediaRegistry.tracks, 901337)
		}
	})
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Req struct {
				Method string
				Param  map[string]any
			} `json:"req_0"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request.Req.Method != "get_song_detail" || request.Req.Param["song_id"] != float64(901337) || request.Req.Param["song_mid"] != nil {
			t.Errorf("wrong numeric request: %+v", request)
		}
		rpcReply(w, map[string]any{"track_info": map[string]any{"id": 901337, "mid": "numericLoudness", "volume": map[string]any{"gain": 0, "peak": 0}}})
	})
	volume, err := SongLoudness(Track{ID: 901337})
	if err != nil || volume != (Loudness{}) {
		t.Fatalf("numeric song: volume=%+v error=%v", volume, err)
	}
	if known, ok := knownTrack(901337); !ok || known.MID != "numericLoudness" {
		t.Fatalf("numeric song MID not remembered: %+v", known)
	}
}

// Details already fetched for playback supply loudness without another request.
func TestSongDetailLoudnessReused(t *testing.T) {
	isolateLoudnessCache(t)
	calls := 0
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		rpcReply(w, map[string]any{"track_info": map[string]any{"id": 901338, "mid": "detailLoudness", "volume": map[string]any{"gain": -7.603, "peak": 0.946, "lra": 3.712}}})
	})
	tracks, err := SongDetails([]int64{901338})
	if err != nil || len(tracks) != 1 || tracks[0].Volume == nil {
		t.Fatalf("details: tracks=%+v error=%v", tracks, err)
	}
	volume, err := SongLoudness(tracks[0])
	if err != nil || volume != *tracks[0].Volume || calls != 1 {
		t.Fatalf("details measurement not reused: volume=%+v calls=%d error=%v", volume, calls, err)
	}
}

// Gateway and transport failures return no fabricated zero measurement, so a later play can retry.
func TestLoudnessFailureRetry(t *testing.T) {
	for _, body := range []string{`{"code":500000}`, `{"req_0":{"code":0}}`, `{"code":0,"req_0":{"code":0,"data":{}}}`, `not JSON`} {
		t.Run(strings.ReplaceAll(body, "/", "_"), func(t *testing.T) {
			isolateLoudnessCache(t)
			calls := 0
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 1 {
					_, _ = w.Write([]byte(body))
					return
				}
				rpcReply(w, map[string]any{"track_info": map[string]any{"mid": "retryLoudness", "volume": map[string]any{"gain": -5, "peak": 1}}})
			})
			if _, err := SongLoudness(Track{MID: "retryLoudness"}); err == nil {
				t.Fatal("failed response accepted")
			}
			volume, err := SongLoudness(Track{MID: "retryLoudness"})
			if err != nil || volume.Gain != -5 || calls != 2 {
				t.Fatalf("retry: volume=%+v calls=%d error=%v", volume, calls, err)
			}
		})
	}
}
