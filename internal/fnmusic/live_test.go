package fnmusic

import (
	"context"
	"os"
	"testing"
	"time"
)

// liveClient is a client of the service FN_ADDR names, as FN_USER with
// FN_PASS; the live tests skip without them, as the others that need a
// service of someone's.
func liveClient(t *testing.T) *Client {
	addr, user, pass := os.Getenv("FN_ADDR"), os.Getenv("FN_USER"), os.Getenv("FN_PASS")
	if addr == "" || user == "" {
		t.Skip("FN_ADDR, FN_USER and FN_PASS name a service to try")
	}
	c, err := New(Config{Address: addr, Username: user, PassHash: HashPassword(pass)})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// What the paged lists of a real service give, and how fast: a page is
// what opens a list, whatever the library's size.
func TestLivePages(t *testing.T) {
	c := liveClient(t)
	timed := func(name string, f func() (int, int, error)) {
		start := time.Now()
		n, total, err := f()
		if err != nil {
			t.Errorf("%s: %v", name, err)
			return
		}
		t.Logf("%-22s %3d of %5d in %v", name, n, total, time.Since(start).Round(time.Millisecond))
	}
	timed("library recent", func() (int, int, error) {
		tr, total, err := c.LibraryPage(SortRecent, 1, 100)
		return len(tr), total, err
	})
	timed("library title", func() (int, int, error) {
		tr, total, err := c.LibraryPage(SortTitle, 1, 100)
		return len(tr), total, err
	})
	timed("library last page", func() (int, int, error) {
		tr, total, err := c.LibraryPage(SortRecent, 46, 100)
		return len(tr), total, err
	})
	timed("favorites", func() (int, int, error) {
		tr, total, err := c.FavoritesPage(1, 100)
		return len(tr), total, err
	})
	timed("search 周杰伦", func() (int, int, error) {
		tr, total, err := c.SearchPage("周杰伦", 1, 100)
		if len(tr) > 0 {
			t.Logf("  first hit %q by %q", tr[0].Name, tr[0].ArtistNames())
		}
		return len(tr), total, err
	})
	albums, total, err := c.AlbumsPage(AlbumsPath, AlbumsSize, "", 1, 60)
	if err != nil || len(albums) == 0 {
		t.Fatalf("albums: %v", err)
	}
	t.Logf("albums by size: %d of %d; first %q by %q, %d songs, cover %s", len(albums), total, albums[0].Name, albums[0].Sub, albums[0].TrackCount, albums[0].CoverURL)
	timed("album songs", func() (int, int, error) {
		tr, total, err := c.AlbumPage(albums[0].Ref, 1, 100)
		return len(tr), total, err
	})
	artists, total, err := c.ArtistsPage(ArtistsPath, ArtistsSize, "", 1, 60)
	if err != nil || len(artists) == 0 {
		t.Fatalf("artists: %v", err)
	}
	t.Logf("artists by size: %d of %d; first %q (%s)", len(artists), total, artists[0].Name, artists[0].Sub)
	timed("artist songs", func() (int, int, error) {
		tr, total, err := c.ArtistPage(artists[0].Ref, 1, 100)
		return len(tr), total, err
	})
	if b, err := c.Cover(artists[0].CoverURL[len(coverScheme):], 200); err != nil || len(b) < 500 {
		t.Errorf("artist cover: %d bytes, %v", len(b), err)
	}
}

// Whether the service transcodes, and what its stream is made of.
func TestLiveTranscode(t *testing.T) {
	c := liveClient(t)
	for pg := 1; ; pg++ {
		tracks, total, err := c.LibraryPage(SortRecent, pg, 500)
		if err != nil {
			t.Fatal(err)
		}
		for _, tr := range tracks {
			if !Transcoded(&tr) {
				continue
			}
			t.Logf("transcoding %q (%s)", tr.Name, tr.Format)
			h, err := c.TranscodeHLS(context.Background(), &tr)
			if err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			far, err := h.Fetch(context.Background(), len(h.Segments)-3)
			t.Logf("%d pieces, %v; a piece near the end alone: %d bytes in %v (%v)", len(h.Segments), h.Total, len(far), time.Since(start).Round(time.Millisecond), err)
			h.Release()
			return
		}
		if pg*500 >= total {
			t.Skip("nothing in the library needs transcoding")
		}
	}
}
