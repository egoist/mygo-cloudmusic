package qqmusic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	suggestionLifetime  = 30 * time.Second
	suggestionCacheSize = 64
	smartboxPath        = "/splcloud/fcgi-bin/smartbox_new.fcg"
)

type suggestionEntry struct {
	result Suggestions
	at     time.Time
}

type suggestionService struct {
	requests requestGroup[Suggestions]
	mu       sync.Mutex
	entries  map[string]suggestionEntry
}

var smartboxSuggestions = &suggestionService{}

// SearchSuggest reads lightweight candidates without consuming a full search request.
func SearchSuggest(query string) (Suggestions, error) {
	return SearchSuggestContext(context.Background(), query)
}

// SearchSuggestContext shares identical reads and stops their HTTP request when
// the last waiting caller cancels. Smartbox never receives the account's cookies.
func SearchSuggestContext(ctx context.Context, query string) (Suggestions, error) {
	if err := ctx.Err(); err != nil {
		return Suggestions{}, err
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return Suggestions{}, nil
	}
	if len([]rune(query)) > 60 {
		return Suggestions{}, fmt.Errorf("qqmusic: suggestion exceeds 60 characters")
	}
	service := smartboxSuggestions
	if result, ok := service.get(query, time.Now()); ok {
		return result, nil
	}
	return service.requests.do(ctx, query, func(shared context.Context) (Suggestions, error) {
		// Another read may have completed between the first lookup and joining.
		if result, ok := service.get(query, time.Now()); ok {
			return result, nil
		}
		result, err := fetchSuggestions(shared, query)
		if err != nil {
			return Suggestions{}, err
		}
		if err := shared.Err(); err != nil {
			return Suggestions{}, err
		}
		service.put(query, result, time.Now())
		return result, nil
	})
}

func (s *suggestionService) get(query string, now time.Time) (Suggestions, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[query]
	if !ok {
		return Suggestions{}, false
	}
	if now.Sub(entry.at) >= suggestionLifetime {
		delete(s.entries, query)
		return Suggestions{}, false
	}
	return entry.result, true
}

func (s *suggestionService) put(query string, result Suggestions, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = make(map[string]suggestionEntry)
	}
	for key, entry := range s.entries {
		if now.Sub(entry.at) >= suggestionLifetime {
			delete(s.entries, key)
		}
	}
	if _, exists := s.entries[query]; !exists && len(s.entries) >= suggestionCacheSize {
		var oldest string
		var at time.Time
		for key, entry := range s.entries {
			if oldest == "" || entry.at.Before(at) {
				oldest, at = key, entry.at
			}
		}
		delete(s.entries, oldest)
	}
	s.entries[query] = suggestionEntry{result: result, at: now}
}

func fetchSuggestions(ctx context.Context, query string) (Suggestions, error) {
	params := url.Values{
		"key": {query}, "format": {"json"}, "inCharset": {"utf-8"},
		"outCharset": {"utf-8"}, "platform": {"yqq"}, "needNewCode": {"0"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://c.y.qq.com"+smartboxPath+"?"+params.Encode(), nil)
	if err != nil {
		return Suggestions{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://y.qq.com/")
	b, err := readHTTP(req)
	if err != nil {
		return Suggestions{}, err
	}
	return decodeSuggestions(b)
}

func decodeSuggestions(b []byte) (Suggestions, error) {
	var response struct {
		Code    *int            `json:"code"`
		Subcode int             `json:"subcode"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(b, &response); err != nil {
		return Suggestions{}, fmt.Errorf("qqmusic: malformed smartbox response: %w", err)
	}
	if response.Code == nil {
		return Suggestions{}, fmt.Errorf("qqmusic: missing smartbox status")
	}
	if *response.Code != 0 || response.Subcode != 0 {
		code := *response.Code
		if code == 0 {
			code = response.Subcode
		}
		return Suggestions{}, &ProtocolError{Code: code, Module: "cgi", Method: smartboxPath}
	}
	var data object
	if json.Unmarshal(response.Data, &data) != nil || data == nil {
		return Suggestions{}, fmt.Errorf("qqmusic: missing smartbox candidates")
	}
	type group struct {
		kind  string
		order int
		items []object
	}
	groups := make([]group, 0, 3)
	for i, descriptor := range []struct{ field, kind string }{{"song", "songs"}, {"singer", "artists"}, {"album", "albums"}} {
		raw, exists := data[descriptor.field]
		if !exists || string(raw) == "null" {
			continue
		}
		var candidate struct {
			Order    *int     `json:"order"`
			ItemList []object `json:"itemlist"`
		}
		if json.Unmarshal(raw, &candidate) != nil {
			return Suggestions{}, fmt.Errorf("qqmusic: malformed smartbox %s candidates", descriptor.field)
		}
		order := 100 + i
		if candidate.Order != nil {
			order = *candidate.Order
		}
		groups = append(groups, group{kind: descriptor.kind, order: order, items: candidate.ItemList})
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].order < groups[j].order })
	var result Suggestions
	for _, group := range groups {
		seen := make(map[string]bool, len(group.items))
		added := false
		for _, entry := range group.items {
			id := number(entry, "id", "docid")
			mid := strings.TrimSpace(text(entry, "mid"))
			name := strings.TrimSpace(text(entry, "name"))
			if name == "" || mid == "" && id <= 0 || group.kind != "songs" && (id <= 0 || mid == "") {
				continue
			}
			key := mid
			if key == "" {
				key = "id:" + strconv.FormatInt(id, 10)
			}
			if seen[key] {
				continue
			}
			seen[key], added = true, true
			singer := strings.TrimSpace(text(entry, "singer"))
			var artists []Artist
			if singer != "" {
				artists = []Artist{{Name: singer}}
			}
			switch group.kind {
			case "songs":
				// Candidates lack file/MediaMID: registering them as full tracks
				// would overwrite metadata needed by playback.
				result.Songs = append(result.Songs, Track{ID: id, MID: mid, Name: name, Artists: artists})
			case "artists":
				artist := Artist{ID: id, MID: mid, Name: name}
				mediaRegistry.Lock()
				mediaRegistry.artists[id] = artist
				mediaRegistry.Unlock()
				result.Artists = append(result.Artists, artist)
			case "albums":
				pic := text(entry, "pic")
				if pic == "" {
					pic = albumCover(mid)
				}
				album := Album{ID: id, MID: mid, Name: name, PicURL: pic}
				mediaRegistry.Lock()
				mediaRegistry.albums[id] = album
				mediaRegistry.Unlock()
				result.Albums = append(result.Albums, AlbumProfile{Album: album, Artists: artists})
			}
		}
		if added {
			result.Order = append(result.Order, group.kind)
		}
	}
	return result, nil
}
