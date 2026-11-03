package qqmusic

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func isolateSuggestions(t *testing.T) {
	t.Helper()
	old := smartboxSuggestions
	smartboxSuggestions = &suggestionService{}
	t.Cleanup(func() { smartboxSuggestions = old })
}

const smartboxFixture = `{"code":0,"subcode":0,"data":{
  "song":{"order":2,"itemlist":[
    {"id":"91001","mid":"known-song","name":"歌曲","singer":"歌手文本"},
    {"id":"91002","mid":"known-song","name":"重复"},
    {"mid":"mid-only","name":"仅有MID"},
    {"name":"没有标识"},null,{"id":"91003","mid":"missing-name"}]},
  "singer":{"order":0,"itemlist":[
    {"id":"9007199254740993","mid":"artist-mid","name":"歌手"},
    {"id":"9007199254740993","mid":"artist-mid","name":"重复歌手"},
    {"mid":"artist-no-id","name":"不能路由"}]},
  "album":{"order":1,"itemlist":[
    {"docid":"92001","mid":"album-mid","name":"专辑","singer":"专辑歌手","pic":"https://example.invalid/album.png"}]},
  "mv":{"order":3,"itemlist":[{"id":"1","mid":"video","name":"忽略"}]}
}}`

// Smartbox uses its anonymous endpoint and parses sparse candidates without overwriting playback metadata.
func TestSmartboxCandidatesKeepPlaybackMetadata(t *testing.T) {
	isolateSuggestions(t)
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != smartboxPath || r.URL.Query().Get("key") != "候选" || r.URL.Query().Get("format") != "json" || r.URL.Query().Get("inCharset") != "utf-8" || r.URL.Query().Get("outCharset") != "utf-8" || r.URL.Query().Get("platform") != "yqq" || r.URL.Query().Get("needNewCode") != "0" {
			t.Errorf("wrong smartbox request: %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("Cookie") != "" || r.UserAgent() != "Mozilla/5.0" || r.Header.Get("Referer") != "https://y.qq.com/" {
			t.Error("smartbox inherited account identity")
		}
		_, _ = w.Write([]byte(smartboxFixture))
	})
	setSession(Session{MusicID: "123", MusicKey: "fixture-key", LoginType: 2})
	full := Track{ID: 91001, MID: "known-song", MediaMID: "media-mid", Size320: 123456, Name: "完整记录", Volume: &Loudness{Gain: -7.6, Peak: 0.94}}
	RememberTracks([]Track{full})
	result, err := SearchSuggest(" 候选 ")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Order, []string{"artists", "albums", "songs"}) || len(result.Songs) != 2 || len(result.Artists) != 1 || len(result.Albums) != 1 {
		t.Fatalf("candidate kinds/order: %+v", result)
	}
	if result.Songs[0].ID != 91001 || result.Songs[0].ArtistNames() != "歌手文本" || result.Songs[1].ID != 0 || result.Songs[1].MID != "mid-only" {
		t.Fatalf("song candidates lost sparse identity: %+v", result.Songs)
	}
	if result.Artists[0].ID != 9007199254740993 || artistMID(9007199254740993) != "artist-mid" || albumMID(92001) != "album-mid" || result.Albums[0].ArtistNames() != "专辑歌手" {
		t.Fatal("artist/album candidates lost routing metadata")
	}
	if got, ok := knownTrack(full.ID); !ok || !reflect.DeepEqual(got, full) {
		t.Fatal("sparse candidate overwrote the full track")
	}
	if _, ok := knownTrack(91002); ok {
		t.Fatal("duplicate candidate was registered as a full track")
	}
}

// Successful suggestions are reused for the same trimmed query, while failures are retried.
func TestSmartboxSuccessCacheAndFailureRetry(t *testing.T) {
	isolateSuggestions(t)
	var calls atomic.Int32
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Query().Get("key") == "retry" && calls.Load() == 2 {
			_, _ = w.Write([]byte(`{"code":2001,"data":{}}`))
			return
		}
		_, _ = w.Write([]byte(smartboxFixture))
	})
	first, err := SearchSuggest("cached")
	if err != nil {
		t.Fatal(err)
	}
	again, err := SearchSuggest(" cached ")
	if err != nil || !reflect.DeepEqual(again, first) || calls.Load() != 1 {
		t.Fatalf("success not cached: calls=%d err=%v", calls.Load(), err)
	}
	if _, err := SearchSuggest("retry"); err == nil {
		t.Fatal("rejected suggestions looked successful")
	}
	if result, err := SearchSuggest("retry"); err != nil || len(result.Songs) == 0 || calls.Load() != 3 {
		t.Fatalf("failed suggestions stayed cached: calls=%d err=%v", calls.Load(), err)
	}
}

// The short success cache expires at its TTL and remains bounded as new words arrive.
func TestSmartboxCacheExpiryAndBound(t *testing.T) {
	var service suggestionService
	now := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	service.put("first", Suggestions{}, now)
	if _, ok := service.get("first", now.Add(suggestionLifetime-time.Nanosecond)); !ok {
		t.Fatal("fresh empty candidates were not cacheable")
	}
	if _, ok := service.get("first", now.Add(suggestionLifetime)); ok {
		t.Fatal("expired suggestions were reused")
	}
	for i := range suggestionCacheSize + 1 {
		service.put(fmt.Sprint(i), Suggestions{}, now.Add(time.Duration(i)*time.Millisecond))
	}
	if len(service.entries) != suggestionCacheSize {
		t.Fatalf("cache has %d entries", len(service.entries))
	}
	if _, ok := service.get("0", now.Add(time.Second)); ok {
		t.Fatal("oldest cached word survived capacity eviction")
	}
}

// Concurrent callers for the same trimmed word share one HTTP request, and one cancellation leaves the other caller intact.
func TestSmartboxConcurrentCallersShareHTTP(t *testing.T) {
	isolateSuggestions(t)
	started, release := make(chan struct{}), make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	var calls atomic.Int32
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			_, _ = w.Write([]byte(smartboxFixture))
		case <-r.Context().Done():
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first, second := make(chan groupTestResult, 1), make(chan groupTestResult, 1)
	go func() {
		value, err := SearchSuggestContext(ctx, " shared ")
		first <- groupTestResult{len(value.Songs), err}
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP read did not start")
	}
	go func() {
		value, err := SearchSuggestContext(context.Background(), "shared")
		second <- groupTestResult{len(value.Songs), err}
	}()
	waitGroupWaiters(t, &smartboxSuggestions.requests, "shared", 2)
	cancel()
	if got := receiveGroupResult(t, first); !errors.Is(got.err, context.Canceled) {
		t.Fatalf("canceled caller: %+v", got)
	}
	close(release)
	released = true
	if got := receiveGroupResult(t, second); got.err != nil || got.value != 2 || calls.Load() != 1 {
		t.Fatalf("shared HTTP read: result=%+v calls=%d", got, calls.Load())
	}
}

// Canceling the last suggestion caller aborts its HTTP request, and the same word can then start a fresh read.
func TestSmartboxCanceledHTTPIsNotReused(t *testing.T) {
	isolateSuggestions(t)
	started, stopped := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
			<-r.Context().Done()
			close(stopped)
			return
		}
		_, _ = w.Write([]byte(smartboxFixture))
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := SearchSuggestContext(ctx, "cancel"); finished <- err }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP read did not start")
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled suggestion: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("caller did not cancel")
	}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP read kept running after its last caller canceled")
	}
	result, err := SearchSuggestContext(context.Background(), "cancel")
	if err != nil || len(result.Songs) == 0 || calls.Load() != 2 {
		t.Fatalf("fresh read: calls=%d err=%v", calls.Load(), err)
	}
}

// Missing status, rejected data and malformed candidate lists must not become cached empty success.
func TestSmartboxResponseValidation(t *testing.T) {
	for _, body := range []string{
		`not JSON`, `{}`, `{"code":2001,"data":{}}`, `{"code":0,"subcode":4,"data":{}}`,
		`{"code":0}`, `{"code":0,"data":[]}`, `{"code":0,"data":{"song":{"itemlist":{}}}}`,
	} {
		if _, err := decodeSuggestions([]byte(body)); err == nil {
			t.Errorf("invalid response accepted: %s", body)
		}
	}
	if result, err := decodeSuggestions([]byte(`{"code":0,"data":{"song":{"order":0,"itemlist":[]}}}`)); err != nil || len(result.Songs) != 0 {
		t.Fatalf("valid empty candidates: %+v %v", result, err)
	}
	var rejected *ProtocolError
	_, err := decodeSuggestions([]byte(`{"code":0,"subcode":4,"data":{}}`))
	if !errors.As(err, &rejected) || rejected.Code != 4 {
		t.Fatal("smartbox lost its business code")
	}
}

// Blank or already canceled suggestions do not reach any upstream endpoint.
func TestSmartboxSkipsEmptyAndCanceledReads(t *testing.T) {
	isolateSuggestions(t)
	withQQServer(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected suggestion request") })
	if _, err := SearchSuggest(" \t "); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := SearchSuggestContext(ctx, "canceled"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := SearchSuggest(string(make([]rune, 61))); err == nil {
		t.Fatal("overlong suggestion was sent upstream")
	}
}
