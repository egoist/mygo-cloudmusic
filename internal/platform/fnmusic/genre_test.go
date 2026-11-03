package fnmusic

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// Genre endpoints preserve the service's list shape, sort, page and full-load
// parameters while converting style tracks to shared songs.
func TestGenreEndpoints(t *testing.T) {
	s := &genreTestServer{}
	ts := httptest.NewServer(s)
	defer ts.Close()
	c, err := New(Config{Address: ts.URL, Username: "me", PassHash: HashPassword("secret")})
	if err != nil {
		t.Fatal(err)
	}

	list, err := c.GenresPage("", 1, 200)
	if err != nil || list.Total != 2 || len(list.List) != 1 || list.List[0].GUID != "g1" {
		t.Fatalf("genres = %+v, err = %v", list, err)
	}
	if list.Sort != SortGenreNameDesc {
		t.Fatalf("default genre sort = %q", list.Sort)
	}

	sorted, err := c.GenresPage(SortGenreNameDesc, 2, 200)
	if err != nil || sorted.Sort != SortGenreNameDesc {
		t.Fatalf("sorted genres = %+v, err = %v", sorted, err)
	}
	if got := s.queries[1].Get("sort"); got != SortGenreNameDesc {
		t.Errorf("genre sort query = %q", got)
	}

	detail, err := c.Genre("g1")
	if err != nil || detail.Name != "Blues" || detail.TrackCount != 1152 {
		t.Fatalf("genre detail = %+v, err = %v", detail, err)
	}

	tracks, err := c.GenreTracksPage("g1", "", 1, 300)
	if err != nil || tracks.Total != 1152 || len(tracks.List) != 1 {
		t.Fatalf("genre tracks = %+v, err = %v", tracks, err)
	}
	if tracks.Sort != SortGenreTrackCreatedDesc || tracks.List[0].Track.Name != "Hello" || tracks.List[0].Size != 640 {
		t.Fatalf("genre track conversion = %+v", tracks)
	}

	all, err := c.AllGenreTracks("g1", SortGenreTrackCreatedDesc)
	if err != nil || len(all) != 1 || all[0].Ref != "track-1" {
		t.Fatalf("all genre tracks = %+v, err = %v", all, err)
	}
	if got := s.queries[len(s.queries)-1].Get("size"); got != "-1" {
		t.Errorf("all tracks size = %q", got)
	}
}

// Invalid persisted values do not become requests the service cannot order.
func TestGenreSortValidation(t *testing.T) {
	if got := ParseGenreSort("wrong"); got != SortGenreNameDesc {
		t.Errorf("genre default sort = %q", got)
	}
	if got := ParseGenreTrackSort("wrong"); got != SortGenreTrackCreatedDesc {
		t.Errorf("track default sort = %q", got)
	}
	if _, err := (&Client{}).GenresPage("", 0, 200); err == nil {
		t.Error("zero genre page was accepted")
	}
	if _, err := (&Client{}).GenreTracksPage("g1", "", 1, 0); err == nil {
		t.Error("zero genre track size was accepted")
	}
}

type genreTestServer struct {
	server
	queries []url.Values
}

func (s *genreTestServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/music/api/v1")
	switch path {
	case "/genre/list":
		s.queries = append(s.queries, r.URL.Query())
		sort := r.URL.Query().Get("sort")
		if sort == "" {
			sort = SortGenreNameDesc
		}
		s.reply(w, map[string]any{
			"list":  []map[string]any{{"guid": "g1", "name": "Blues", "trackCount": 1152}},
			"total": 2,
			"sort":  sort,
		})
		return
	case "/genre/detail":
		s.reply(w, map[string]any{"guid": "g1", "name": "Blues", "trackCount": 1152})
		return
	case "/track/genre-detail/list":
		s.queries = append(s.queries, r.URL.Query())
		sort := r.URL.Query().Get("sort")
		if sort == "" {
			sort = SortGenreTrackCreatedDesc
		}
		s.reply(w, map[string]any{
			"list":  []map[string]any{{"guid": "track-1", "title": "Hello", "artists": []map[string]any{{"name": "Adele"}}, "audioSpec": map[string]any{"format": "flac", "size": 640}}},
			"total": 1152,
			"sort":  sort,
		})
		return
	}
	s.server.ServeHTTP(w, r)
}

func (s *genreTestServer) reply(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "msg": "", "data": data})
}
