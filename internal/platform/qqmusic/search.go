package qqmusic

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SearchSource pins pagination to the backend that returned the first page.
type SearchSource string

const (
	SearchAuto    SearchSource = ""
	SearchLite    SearchSource = "lite"
	SearchLegacy  SearchSource = "legacy"
	SearchDesktop SearchSource = "desktop"
)

// Desktop ct=11 passed signed-in songs, artists, albums and pagination checks.
const preferredSearchSource = SearchDesktop

type SearchCooldownError struct {
	RetryAfter time.Duration
	Cause      error
}

func (e *SearchCooldownError) Error() string {
	return "qqmusic: search temporarily unavailable; retry after " + e.RetryAfter.Round(time.Second).String()
}
func (e *SearchCooldownError) Unwrap() error { return e.Cause }

type searchFetcher func(context.Context, SearchSource, string, SearchType, int, int, Session) (SearchResult, error)

type searchEngine struct {
	requests requestGroup[SearchResult]
	mu       sync.Mutex
	gates    map[string]*searchGate
	now      func() time.Time
	interval time.Duration
	fetch    searchFetcher
}

func newSearchEngine() *searchEngine {
	return &searchEngine{gates: make(map[string]*searchGate), now: time.Now, interval: time.Second, fetch: fetchSearchPage}
}

var searches = newSearchEngine()

// SearchPage starts a search and returns its source for subsequent pages.
func SearchPage(query string, kind SearchType, limit, offset int) (SearchResult, error) {
	return SearchPageSource(query, kind, limit, offset, SearchAuto)
}

// SearchPageSource keeps a later page on the same source, even if that source is cooling down.
func SearchPageSource(query string, kind SearchType, limit, offset int, source SearchSource) (SearchResult, error) {
	return SearchPageContext(context.Background(), query, kind, limit, offset, source)
}

func SearchPageContext(ctx context.Context, query string, kind SearchType, limit, offset int, source SearchSource) (SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return SearchResult{}, nil
	}
	if len([]rune(query)) > 60 {
		return SearchResult{}, errors.New("qqmusic: search exceeds 60 characters")
	}
	if kind != SearchSongs && kind != SearchArtists && kind != SearchAlbums {
		return SearchResult{}, ErrUnsupported
	}
	if source != SearchAuto && source != SearchLite && source != SearchLegacy && source != SearchDesktop {
		return SearchResult{}, ErrUnsupported
	}
	if source == SearchLegacy && !legacySearchSupported(kind) {
		return SearchResult{}, ErrUnsupported
	}
	limit, offset = min(max(limit, 1), 20), max(offset, 0)
	session := sessionSnapshot()
	if !session.valid() {
		session = Session{}
	}
	// Match the existing network timeout across both sources, including pacing.
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	scope := searchSessionScope(session)
	engine := searches
	return engine.search(ctx, source, query, kind, limit, offset, session, scope)
}

func searchSessionScope(s Session) string {
	if !s.valid() {
		return "anonymous"
	}
	// The key isolates accounts and refreshed credentials without retaining credentials in diagnostics.
	digest := sha256.Sum256([]byte(s.MusicID + ":" + strconv.Itoa(s.LoginType) + ":" + s.MusicKey))
	return fmt.Sprintf("account:%x", digest)
}

func (e *searchEngine) search(ctx context.Context, source SearchSource, query string, kind SearchType, limit, offset int, s Session, scope string) (SearchResult, error) {
	if source != SearchAuto {
		return e.call(ctx, source, query, kind, limit, offset, s, scope)
	}
	candidates := []SearchSource{preferredSearchSource}
	if legacySearchSupported(kind) {
		candidates = append(candidates, SearchLegacy)
	}
	var failures []error
	for _, candidate := range candidates {
		result, err := e.call(ctx, candidate, query, kind, limit, offset, s, scope)
		if err == nil {
			return result, nil
		}
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			return SearchResult{}, err
		}
		failures = append(failures, err)
	}
	return SearchResult{}, errors.Join(failures...)
}

func (e *searchEngine) gate(source SearchSource, scope string) *searchGate {
	endpoint := "musicu"
	if source == SearchLegacy {
		endpoint = "legacy"
	}
	key := endpoint + ":" + scope
	e.mu.Lock()
	defer e.mu.Unlock()
	if gate := e.gates[key]; gate != nil {
		return gate
	}
	gate := &searchGate{interval: e.interval}
	e.gates[key] = gate
	return gate
}

func (e *searchEngine) call(ctx context.Context, source SearchSource, query string, kind SearchType, limit, offset int, s Session, scope string) (SearchResult, error) {
	key := searchRequestKey(scope, source, query, kind, limit, offset)
	return e.requests.do(ctx, key, func(shared context.Context) (SearchResult, error) {
		return e.load(shared, source, query, kind, limit, offset, s, scope)
	})
}

func searchRequestKey(scope string, source SearchSource, query string, kind SearchType, limit, offset int) string {
	return fmt.Sprintf("%s:%s:%d:%d:%d:%q", scope, source, kind, limit, offset, query)
}

func (e *searchEngine) load(ctx context.Context, source SearchSource, query string, kind SearchType, limit, offset int, s Session, scope string) (SearchResult, error) {
	gate := e.gate(source, scope)
	for {
		delay, err := gate.reserve(e.now())
		if err != nil {
			return SearchResult{}, err
		}
		if delay <= 0 {
			break
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return SearchResult{}, ctx.Err()
		case <-timer.C:
		}
	}
	if err := ctx.Err(); err != nil {
		return SearchResult{}, err
	}
	result, err := e.fetch(ctx, source, query, kind, limit, offset, s)
	if err != nil {
		if ctx.Err() == nil && !errors.Is(err, context.Canceled) {
			gate.failed(e.now(), err)
		}
		return SearchResult{}, err
	}
	gate.succeeded(e.now())
	result.Source = source
	return result, nil
}

func fetchSearchPage(ctx context.Context, source SearchSource, query string, kind SearchType, limit, offset int, s Session) (SearchResult, error) {
	if source == SearchLegacy {
		return searchLegacy(ctx, query, kind, limit, offset, s)
	}
	return searchMusicu(ctx, source, query, kind, limit, offset, s)
}

func legacySearchSupported(kind SearchType) bool {
	return kind == SearchSongs || kind == SearchAlbums || kind == SearchArtists
}

func searchLegacy(ctx context.Context, query string, kind SearchType, limit, offset int, s Session) (SearchResult, error) {
	if !legacySearchSupported(kind) {
		return SearchResult{}, ErrUnsupported
	}
	code, key := "0", "song"
	switch kind {
	case SearchAlbums:
		code, key = "8", "album"
	case SearchArtists:
		code, key = "9", "singer"
	}
	values := url.Values{"p": {strconv.Itoa(offset/limit + 1)}, "n": {strconv.Itoa(limit)}, "w": {query}, "t": {code}, "format": {"json"}, "cr": {"1"}, "new_json": {"1"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://c.y.qq.com/soso/fcgi-bin/client_search_cp?"+values.Encode(), nil)
	if err != nil {
		return SearchResult{}, err
	}
	req.Header.Set("Referer", "https://y.qq.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	if s.valid() {
		req.Header.Set("Cookie", searchCookie(s))
	}
	b, err := readHTTP(req)
	if err != nil {
		return SearchResult{}, err
	}
	var reply struct {
		Code    *int   `json:"code"`
		Subcode int    `json:"subcode"`
		Data    object `json:"data"`
	}
	if json.Unmarshal(b, &reply) != nil || reply.Code == nil {
		return SearchResult{}, errors.New("qqmusic: malformed legacy search response")
	}
	if *reply.Code != 0 || reply.Subcode != 0 {
		code := *reply.Code
		if code == 0 {
			code = reply.Subcode
		}
		return SearchResult{}, &ProtocolError{Code: code, Module: "legacy.search", Method: "client_search_cp"}
	}
	section := obj(reply.Data[key])
	var entries []object
	if section == nil || json.Unmarshal(section["list"], &entries) != nil || entries == nil {
		return SearchResult{}, errors.New("qqmusic: malformed legacy search list")
	}
	for _, entry := range entries {
		if entry == nil {
			return SearchResult{}, errors.New("qqmusic: malformed legacy search entry")
		}
	}
	total := int(number(section, "totalnum"))
	// A requested continuation cannot become a fresh zero-result search.
	// Keep the previous page and cool down instead of replacing its total with zero.
	if offset > 0 && total == 0 && len(entries) == 0 {
		return SearchResult{}, errors.New("qqmusic: legacy search lost its pagination total")
	}
	return decodeSearchEntries(entries, kind, total, SearchLegacy, limit, offset), nil
}

// Sparse fallback results need details before supplying a playback filename.
func searchTracks(entries []object) []Track {
	tracks := make([]Track, 0, len(entries))
	for _, entry := range entries {
		t := parseTrack(entry)
		if t.ID <= 0 || t.MID == "" {
			continue
		}
		t.NeedsDetails = t.MediaMID == ""
		if old, ok := knownTrack(t.ID); ok && old.MID == t.MID && old.MediaMID != "" && t.NeedsDetails {
			t.MediaMID = old.MediaMID
			if t.Duration <= 0 {
				t.Duration = old.Duration
			}
			if t.Name == "" {
				t.Name = old.Name
			}
			if len(t.Artists) == 0 {
				t.Artists = old.Artists
			}
			if t.Album.ID == 0 {
				t.Album = old.Album
			}
			if t.Size128 == 0 {
				t.Size128 = old.Size128
			}
			if t.Size320 == 0 {
				t.Size320 = old.Size320
			}
			if t.SizeFLAC == 0 {
				t.SizeFLAC = old.SizeFLAC
			}
			if t.Volume == nil {
				t.Volume = old.Volume
			}
			t.NeedsDetails = false
		}
		RememberTracks([]Track{t})
		tracks = append(tracks, t)
	}
	return tracks
}

func searchCookie(s Session) string {
	return "uin=" + s.MusicID + "; qqmusic_uin=" + s.MusicID + "; qm_keyst=" + s.MusicKey + "; qqmusic_key=" + s.MusicKey
}
