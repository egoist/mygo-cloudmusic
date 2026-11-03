package qqmusic

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

type songPagingReply struct {
	data   string
	status int
	wire   bool
}

type songPagingRequest struct {
	Direction int `json:"direction"`
	Page      int `json:"page"`
	SNum      int `json:"s_num"`
}

type songPagingResult struct {
	shelves     []SongShelf
	err         error
	requests    []songPagingRequest
	detailCalls int
	detailIDs   []int64
}

const songPagingFirst = `{"retcode":0,"load_mark":0,"v_shelf":[{"id":301},{"id":207,"title_content":"First","miscellany":{"songList":"30,20"}},{"id":272},{"id":271}]}`
const songPagingNext = `{"retcode":0,"load_mark":-1,"v_shelf":[{"id":205},{"id":207,"title_content":"Second","miscellany":{"songList":"20,40"}}]}`

func runSongPaging(t *testing.T, replies []songPagingReply) songPagingResult {
	t.Helper()
	var result songPagingResult
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Req struct {
				Module string
				Method string
				Param  json.RawMessage
			} `json:"req_0"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		switch request.Req.Module + "/" + request.Req.Method {
		case "music.recommend.RecommendFeed/get_recommend_feed":
			var param struct {
				songPagingRequest
				Cache json.RawMessage `json:"v_cache"`
				Uniq  json.RawMessage `json:"v_uniq"`
			}
			if err := json.Unmarshal(request.Req.Param, &param); err != nil {
				t.Error(err)
			}
			if string(param.Cache) != "[]" || string(param.Uniq) != "[]" {
				t.Errorf("feed changed the empty cache arguments: %s %s", param.Cache, param.Uniq)
			}
			index := len(result.requests)
			result.requests = append(result.requests, param.songPagingRequest)
			if index >= len(replies) {
				t.Errorf("unexpected feed request: %+v", param.songPagingRequest)
				http.Error(w, "unexpected page", http.StatusBadRequest)
				return
			}
			reply := replies[index]
			if reply.status != 0 {
				w.WriteHeader(reply.status)
			}
			if reply.wire {
				_, _ = w.Write([]byte(reply.data))
			} else {
				rpcReply(w, json.RawMessage(reply.data))
			}
		case "music.trackInfo.UniformRuleCtrl/CgiGetTrackInfo":
			result.detailCalls++
			var param struct {
				IDs []int64 `json:"ids"`
			}
			if err := json.Unmarshal(request.Req.Param, &param); err != nil {
				t.Error(err)
			}
			result.detailIDs = append([]int64(nil), param.IDs...)
			var tracks []any
			for i := len(param.IDs) - 1; i >= 0; i-- {
				tracks = append(tracks, recommendedTrackFixture(param.IDs[i]))
			}
			rpcReply(w, map[string]any{"tracks": tracks})
		default:
			t.Errorf("unexpected method: %s/%s", request.Req.Module, request.Req.Method)
			http.Error(w, "unexpected method", http.StatusBadRequest)
		}
	})
	setSession(Session{MusicID: "10001", MusicKey: "fixture-key", LoginType: 2})
	result.shelves, result.err = RecommendedSongShelves()
	return result
}

func songPagingIDs(shelves []SongShelf) [][]int64 {
	var ids [][]int64
	for _, shelf := range shelves {
		var group []int64
		for _, track := range shelf.Tracks {
			group = append(group, track.ID)
		}
		ids = append(ids, group)
	}
	return ids
}

// Home used to omit the continuation batch, losing its second song shelf entirely.
func TestRecommendedSongShelvesLoadsContinuation(t *testing.T) {
	got := runSongPaging(t, []songPagingReply{{data: songPagingFirst}, {data: songPagingNext}})
	if got.err != nil {
		t.Fatal(got.err)
	}
	if want := []songPagingRequest{{0, 1, 0}, {1, 2, 4}}; !reflect.DeepEqual(got.requests, want) {
		t.Fatalf("feed requests=%+v, want %+v", got.requests, want)
	}
	if want := [][]int64{{30, 20}, {20, 40}}; !reflect.DeepEqual(songPagingIDs(got.shelves), want) {
		t.Fatalf("shelf song order=%v, want %v", songPagingIDs(got.shelves), want)
	}
	if got.detailCalls != 1 || !reflect.DeepEqual(got.detailIDs, []int64{30, 20, 40}) {
		t.Fatalf("detail calls=%d IDs=%v; shared songs must be requested once but retained in both shelves", got.detailCalls, got.detailIDs)
	}
}

// Feed termination comes from the server marker or an empty batch, including older replies without a marker.
func TestRecommendedSongShelvesPagingStops(t *testing.T) {
	for _, tt := range []struct {
		name    string
		replies []songPagingReply
		calls   int
		ids     [][]int64
	}{
		{"negative_marker", []songPagingReply{{data: `{"retcode":0,"load_mark":-1,"v_shelf":[{"id":207,"miscellany":{"songList":"30,20"}}]}`}}, 1, [][]int64{{30, 20}}},
		{"positive_marker", []songPagingReply{{data: `{"retcode":0,"load_mark":1,"v_shelf":[{"id":207,"miscellany":{"songList":"30,20"}}]}`}}, 1, [][]int64{{30, 20}}},
		{"missing_marker", []songPagingReply{{data: `{"retcode":0,"v_shelf":[{"id":207,"miscellany":{"songList":"30,20"}}]}`}}, 1, [][]int64{{30, 20}}},
		{"empty_first_batch", []songPagingReply{{data: `{"retcode":0,"load_mark":0,"v_shelf":[]}`}}, 1, nil},
		{"empty_continuation", []songPagingReply{{data: songPagingFirst}, {data: `{"retcode":0,"load_mark":0,"v_shelf":[]}`}}, 2, [][]int64{{30, 20}}},
		{"continuation_without_marker", []songPagingReply{{data: songPagingFirst}, {data: `{"retcode":0,"v_shelf":[{"id":207,"miscellany":{"songList":"40"}}]}`}}, 2, [][]int64{{30, 20}, {40}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := runSongPaging(t, tt.replies)
			if got.err != nil {
				t.Fatal(got.err)
			}
			if len(got.requests) != tt.calls || !reflect.DeepEqual(songPagingIDs(got.shelves), tt.ids) {
				t.Fatalf("feed calls=%d shelves=%v, want calls=%d shelves=%v", len(got.requests), songPagingIDs(got.shelves), tt.calls, tt.ids)
			}
			wantDetails := 0
			if len(tt.ids) > 0 {
				wantDetails = 1
			}
			if got.detailCalls != wantDetails {
				t.Fatalf("detail calls=%d, want %d", got.detailCalls, wantDetails)
			}
		})
	}
}

// Non-song continuation batches still advance the feed instead of imposing a fixed batch-count limit.
func TestRecommendedSongShelvesContinuesPastNonSongBatches(t *testing.T) {
	got := runSongPaging(t, []songPagingReply{
		{data: songPagingFirst},
		{data: `{"retcode":0,"load_mark":0,"v_shelf":[{"id":205}]}`},
		{data: `{"retcode":0,"load_mark":0,"v_shelf":[{"id":272}]}`},
		{data: `{"retcode":0,"load_mark":0,"v_shelf":[{"id":271}]}`},
		{data: songPagingNext},
	})
	if got.err != nil {
		t.Fatal(got.err)
	}
	if want := []songPagingRequest{{0, 1, 0}, {1, 2, 4}, {1, 3, 5}, {1, 4, 6}, {1, 5, 7}}; !reflect.DeepEqual(got.requests, want) {
		t.Fatalf("feed requests=%+v, want %+v", got.requests, want)
	}
	if want := [][]int64{{30, 20}, {20, 40}}; !reflect.DeepEqual(songPagingIDs(got.shelves), want) || got.detailCalls != 1 {
		t.Fatalf("shelves=%v detail calls=%d, want shelves=%v and one detail request", songPagingIDs(got.shelves), got.detailCalls, want)
	}
}

// The home limit counts usable song shelves and retains every song shelf already returned in the final batch.
func TestRecommendedSongShelvesStopsAtHomeLimit(t *testing.T) {
	for _, tt := range []struct {
		name string
		last string
		ids  [][]int64
	}{
		{"exact_limit", `{"retcode":0,"load_mark":0,"v_shelf":[{"id":207,"miscellany":{"songList":"50"}}]}`, [][]int64{{30, 20}, {40}, {50}}},
		{"complete_final_batch", `{"retcode":0,"load_mark":0,"v_shelf":[{"id":207,"miscellany":{"songList":"50"}},{"id":207,"miscellany":{"songList":"60"}}]}`, [][]int64{{30, 20}, {40}, {50}, {60}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := runSongPaging(t, []songPagingReply{
				{data: songPagingFirst},
				{data: `{"retcode":0,"load_mark":0,"v_shelf":[{"id":207,"miscellany":{"songList":"0,invalid"}},{"id":205},{"id":207,"miscellany":{"songList":"40"}}]}`},
				{data: tt.last},
			})
			if got.err != nil {
				t.Fatal(got.err)
			}
			if want := []songPagingRequest{{0, 1, 0}, {1, 2, 4}, {1, 3, 7}}; !reflect.DeepEqual(got.requests, want) {
				t.Fatalf("feed requests=%+v, want %+v", got.requests, want)
			}
			if !reflect.DeepEqual(songPagingIDs(got.shelves), tt.ids) || got.detailCalls != 1 {
				t.Fatalf("shelves=%v detail calls=%d, want shelves=%v and one detail request", songPagingIDs(got.shelves), got.detailCalls, tt.ids)
			}
		})
	}
}

func songPagingFailures() map[string]songPagingReply {
	return map[string]songPagingReply{
		"http":            {status: http.StatusServiceUnavailable, data: "unavailable", wire: true},
		"decode":          {data: "{", wire: true},
		"envelope_status": {data: `{"code":1000}`, wire: true},
		"rpc_status":      {data: `{"code":0,"req_0":{"code":12913,"data":{}}}`, wire: true},
		"feed_status":     {data: `{"retcode":12913,"load_mark":-1,"v_shelf":[]}`},
		"missing_retcode": {data: `{"load_mark":-1,"v_shelf":[]}`},
		"missing_shelves": {data: `{"retcode":0,"load_mark":-1}`},
		"null_shelves":    {data: `{"retcode":0,"load_mark":-1,"v_shelf":null}`},
		"invalid_shelves": {data: `{"retcode":0,"load_mark":-1,"v_shelf":"invalid"}`},
	}
}

// A failed continuation must keep the playable first batch and still resolve its song details.
func TestRecommendedSongShelvesContinuationFailureKeepsEarlierSongs(t *testing.T) {
	for name, failure := range songPagingFailures() {
		t.Run(name, func(t *testing.T) {
			got := runSongPaging(t, []songPagingReply{{data: songPagingFirst}, failure})
			if got.err != nil {
				t.Fatal(got.err)
			}
			if len(got.requests) != 2 || !reflect.DeepEqual(songPagingIDs(got.shelves), [][]int64{{30, 20}}) {
				t.Fatalf("feed calls=%d shelves=%v; first-batch songs were not retained", len(got.requests), songPagingIDs(got.shelves))
			}
			if got.detailCalls != 1 || !reflect.DeepEqual(got.detailIDs, []int64{30, 20}) {
				t.Fatalf("detail calls=%d IDs=%v, want one request for [30 20]", got.detailCalls, got.detailIDs)
			}
		})
	}
}

// A continuation failure without earlier usable songs must remain an error instead of an empty successful feed.
func TestRecommendedSongShelvesContinuationFailureWithoutSongs(t *testing.T) {
	for name, failure := range songPagingFailures() {
		t.Run(name, func(t *testing.T) {
			got := runSongPaging(t, []songPagingReply{
				{data: `{"retcode":0,"load_mark":0,"v_shelf":[{"id":301},{"id":207,"miscellany":{"songList":"0,invalid"}}]}`},
				failure,
			})
			if got.err == nil || len(got.requests) != 2 || got.detailCalls != 0 || len(got.shelves) != 0 {
				t.Fatalf("error=%v feed calls=%d detail calls=%d shelves=%d; expected a continuation error without songs", got.err, len(got.requests), got.detailCalls, len(got.shelves))
			}
		})
	}
}

// First-batch failures must surface directly because there are no earlier shelves to fall back to.
func TestRecommendedSongShelvesFirstBatchFailure(t *testing.T) {
	for name, failure := range songPagingFailures() {
		t.Run(name, func(t *testing.T) {
			got := runSongPaging(t, []songPagingReply{failure})
			if got.err == nil || len(got.requests) != 1 || got.detailCalls != 0 || len(got.shelves) != 0 {
				t.Fatalf("error=%v feed calls=%d detail calls=%d shelves=%d; expected the first-batch error", got.err, len(got.requests), got.detailCalls, len(got.shelves))
			}
		})
	}
}
