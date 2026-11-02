package fnmusic

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// Each list endpoint sends the normalized sort the page asked for.
func TestListSortRequests(t *testing.T) {
	s := &sortQueryServer{}
	ts := httptest.NewServer(s)
	defer ts.Close()
	c, err := New(Config{Address: ts.URL, Username: "me", PassHash: HashPassword("secret")})
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := c.FavoritesPage("", 1, 50); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.AlbumPage("album", "", 1, 50); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.ArtistPage("artist", "", 1, 50); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.ArtistAlbumsPage("artist", "", 1, 50); err != nil {
		t.Fatal(err)
	}

	want := []struct {
		sort, extra string
	}{
		{FavoriteSortPolicy.Default, ""},
		{AlbumTrackSortPolicy.Default, "albumGUID=album"},
		{ArtistTrackSortPolicy.Default, "artistGUID=artist"},
		{ArtistAlbumSortPolicy.Default, "artistGUID=artist"},
	}
	if len(s.queries) != len(want) {
		t.Fatalf("recorded %d sort requests", len(s.queries))
	}
	for i, w := range want {
		if got := s.queries[i].Get("sort"); got != w.sort {
			t.Errorf("request %d sort = %q, want %q", i, got, w.sort)
		}
		if w.extra != "" && !strings.Contains(s.queries[i].Encode(), w.extra) {
			t.Errorf("request %d query = %q, missing %q", i, s.queries[i].Encode(), w.extra)
		}
	}
}

// Sorting policies fall back to the web player's defaults and reject fields an
// endpoint cannot order by.
func TestSortPolicyDefaults(t *testing.T) {
	for _, tc := range []struct {
		name   string
		policy SortPolicy
		want   string
	}{
		{"favorites", FavoriteSortPolicy, "favoriteAt,desc"},
		{"albums", AlbumListSortPolicy, "newTrackAddedAt,desc"},
		{"album tracks", AlbumTrackSortPolicy, "trackNo,asc"},
		{"artists", ArtistListSortPolicy, "name,desc"},
		{"artist tracks", ArtistTrackSortPolicy, "createdAt,desc"},
		{"artist albums", ArtistAlbumSortPolicy, "newTrackAddedAt,desc"},
		{"genres", GenreListSortPolicy, "name,desc"},
		{"genre tracks", GenreTrackSortPolicy, "createdAt,desc"},
	} {
		if got := tc.policy.Normalize(""); got != tc.want {
			t.Errorf("%s empty sort = %q, want %q", tc.name, got, tc.want)
		}
		if got := tc.policy.Normalize("not,a,sort"); got != tc.want {
			t.Errorf("%s invalid sort = %q, want %q", tc.name, got, tc.want)
		}
	}
	if got := AlbumTrackSortPolicy.Normalize("title,asc"); got != "title,asc" {
		t.Errorf("valid album track sort = %q", got)
	}
}

type sortQueryServer struct {
	server
	queries []url.Values
}

func (s *sortQueryServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/music/api/v1")
	switch path {
	case "/favorite-track/list", "/track/album-detail/list", "/track/artist-detail/list", "/album/artist-detail/list":
		s.queries = append(s.queries, r.URL.Query())
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "msg": "", "data": map[string]any{
			"list": []any{}, "total": 0, "sort": r.URL.Query().Get("sort"),
		}})
		return
	}
	s.server.ServeHTTP(w, r)
}
