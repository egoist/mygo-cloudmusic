package qqmusic

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo"
)

func liveSearchAccount(t *testing.T) Session {
	t.Helper()
	if os.Getenv("LIVE") == "" || os.Getenv("LIVE_SEARCH_ACCOUNT") != "1" {
		t.Skip("set LIVE=1 LIVE_SEARCH_ACCOUNT=1 to read Lilt's saved session for QQ search")
	}
	b, err := mygo.Secrets.Service("dev.amamiya.qqmusic").Get(qqSessionSecret)
	if err != nil {
		t.Fatal("read Lilt session:", err)
	}
	var session Session
	if json.Unmarshal(b, &session) != nil || !session.valid() {
		t.Skip("sign in to Lilt before running account search")
	}
	previous := sessionSnapshot()
	t.Cleanup(func() { setSession(previous) })
	setSession(session)
	return session
}

// Opt-in account testing verifies real search, suggestions and pagination without printing or changing the saved session.
func TestLiveAuthenticatedSearch(t *testing.T) {
	session := liveSearchAccount(t)
	firstSongs := map[int64]bool{}
	for _, tc := range []struct {
		name, query string
		kind        SearchType
		offset      int
	}{
		{"songs", "周杰伦", SearchSongs, 0},
		{"next songs", "周杰伦", SearchSongs, 20},
		{"artists", "周杰伦", SearchArtists, 0},
		{"albums", "周杰伦", SearchAlbums, 0},
		{"another query", "小酒窝", SearchSongs, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := SearchPageSource(tc.query, tc.kind, 20, tc.offset, SearchLite)
			if err != nil {
				t.Fatal(err)
			}
			if result.Len() == 0 || result.Total == 0 {
				t.Fatal("empty account search")
			}
			switch tc.kind {
			case SearchSongs:
				for _, track := range result.Songs {
					if track.ID <= 0 || track.MID == "" {
						t.Fatal("missing song identity")
					}
				}
				if tc.name == "songs" {
					for _, track := range result.Songs {
						firstSongs[track.ID] = true
					}
				}
				if tc.offset > 0 {
					newSong := false
					for _, track := range result.Songs {
						if !firstSongs[track.ID] {
							newSong = true
						}
					}
					if !newSong {
						t.Fatal("second page repeated the first")
					}
				}
			case SearchArtists:
				for _, artist := range result.Artists {
					if artist.ID <= 0 || artist.MID == "" {
						t.Fatal("missing artist identity")
					}
				}
			case SearchAlbums:
				for _, album := range result.Albums {
					if album.ID <= 0 || album.MID == "" {
						t.Fatal("missing album identity")
					}
				}
			}
			t.Logf("%d rows of %d, offset=%d", result.Len(), result.Total, tc.offset)
		})
		time.Sleep(500 * time.Millisecond)
	}
	suggestions, err := SearchSuggest("周杰伦")
	if err != nil || len(suggestions.Songs) == 0 {
		t.Fatalf("suggestions: rows=%d error=%v", len(suggestions.Songs), err)
	}
	if sessionSnapshot() != session {
		t.Fatal("search changed the account session")
	}
	t.Logf("suggestions: %d songs", len(suggestions.Songs))
}

// Real Desktop parity must pass every category and paging before it can replace the existing main method.
func TestLiveDesktopSearch(t *testing.T) {
	liveSearchAccount(t)
	for _, tc := range []struct {
		name   string
		kind   SearchType
		offset int
	}{
		{"songs", SearchSongs, 0}, {"artists", SearchArtists, 0}, {"albums", SearchAlbums, 0}, {"songs next", SearchSongs, 20}, {"albums next", SearchAlbums, 20},
	} {
		if !t.Run(tc.name, func(t *testing.T) {
			result, err := SearchPageSource("周杰伦", tc.kind, 20, tc.offset, SearchDesktop)
			if err != nil {
				t.Fatal(err)
			}
			if result.Len() == 0 || result.Total == 0 || result.Source != SearchDesktop || result.NextOffset != tc.offset+20 {
				t.Fatalf("incomplete Desktop result: rows=%d total=%d offset=%d source=%s", result.Len(), result.Total, result.NextOffset, result.Source)
			}
			switch tc.kind {
			case SearchSongs:
				if result.Songs[0].ID <= 0 || result.Songs[0].MID == "" || result.Songs[0].MediaMID == "" {
					t.Fatal("Desktop lost playable song metadata")
				}
			case SearchArtists:
				if result.Artists[0].ID <= 0 || result.Artists[0].MID == "" {
					t.Fatal("Desktop lost artist identity")
				}
			case SearchAlbums:
				if result.Albums[0].ID <= 0 || result.Albums[0].MID == "" {
					t.Fatal("Desktop lost album identity")
				}
			}
			t.Logf("rows=%d total=%d offset=%d", result.Len(), result.Total, tc.offset)
		}) {
			return
		}
		time.Sleep(1200 * time.Millisecond)
	}
}

// The old public endpoint must keep standard playback metadata and page boundaries when used as backup.
func TestLiveLegacySearch(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE=1 to contact QQ Music")
	}
	previous := sessionSnapshot()
	setSession(Session{})
	t.Cleanup(func() { setSession(previous) })
	for _, tc := range []struct {
		name string
		kind SearchType
	}{
		{"songs", SearchSongs}, {"albums", SearchAlbums}, {"artists", SearchArtists},
	} {
		if !t.Run(tc.name, func(t *testing.T) {
			var firstID int64
			for _, offset := range []int{0, 5} {
				result, err := SearchPageSource("周杰伦", tc.kind, 5, offset, SearchLegacy)
				if err != nil {
					t.Fatal(err)
				}
				if result.Len() == 0 || result.Total == 0 || result.Source != SearchLegacy || result.NextOffset != offset+5 {
					t.Fatalf("incomplete backup page: rows=%d total=%d offset=%d source=%s", result.Len(), result.Total, result.NextOffset, result.Source)
				}
				var id int64
				switch tc.kind {
				case SearchSongs:
					track := result.Songs[0]
					id = track.ID
					if track.MID == "" || track.MediaMID == "" {
						t.Fatal("backup lost playback metadata")
					}
				case SearchAlbums:
					album := result.Albums[0]
					id = album.ID
					if album.MID == "" || len(album.Artists) == 0 || album.PublishTime <= 0 {
						t.Fatal("backup lost album metadata")
					}
				case SearchArtists:
					artist := result.Artists[0]
					id = artist.ID
					if artist.MID == "" || artist.Avatar == "" {
						t.Fatal("backup lost artist metadata")
					}
				}
				if id <= 0 || offset > 0 && id == firstID {
					t.Fatal("backup lost identity or repeated its first page")
				}
				firstID = id
				t.Logf("offset=%d rows=%d total=%d", offset, result.Len(), result.Total)
				time.Sleep(1200 * time.Millisecond)
			}
		}) {
			return
		}
	}
}
