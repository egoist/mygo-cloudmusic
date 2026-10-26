package fnmusic

import (
	"context"
	"os"
	"testing"
	"time"
)

// liveClient is a client of the service FN_ADDR names, as FN_USER with
// FN_PASS. The live tests, which need a Feiniu NAS of someone's, skip unless
// LIVE is set, and the three are.
func liveClient(t *testing.T) *Client {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	addr, user, pass := os.Getenv("FN_ADDR"), os.Getenv("FN_USER"), os.Getenv("FN_PASS")
	if addr == "" || user == "" {
		t.Skip("LIVE is set, but FN_ADDR, FN_USER and FN_PASS do not name a service to try")
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

// The playlist calls against a real service, on a playlist made for the test
// and deleted after it, with songs of the library, which are only listed in
// it: none of the listener's own playlists or songs is touched.
func TestLivePlaylists(t *testing.T) {
	c := liveClient(t)
	songs, _, err := c.LibraryPage(SortRecent, 1, 3)
	if err != nil || len(songs) < 3 {
		t.Fatalf("%d songs, %v", len(songs), err)
	}
	pl, err := c.CreatePlaylist("测试用临时歌单（会自动删除）")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.DeletePlaylist(pl.Ref); err != nil {
			t.Errorf("the playlist made for the test was not deleted: %v", err)
		}
	}()
	names := func() []string {
		got, _, err := c.PlaylistPage(pl.Ref, 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, s := range got {
			out = append(out, s.Ref)
		}
		return out
	}
	if err := c.AddToPlaylist(pl.Ref, []string{songs[0].Ref, songs[1].Ref}); err != nil {
		t.Fatal(err)
	}
	if got := names(); len(got) != 2 {
		t.Fatalf("after adding two: %v", got)
	}
	// Adding one that is in is no error and no second copy; it moves first.
	if err := c.AddToPlaylist(pl.Ref, []string{songs[1].Ref}); err != nil {
		t.Fatal(err)
	}
	if got := names(); len(got) != 2 || got[0] != songs[1].Ref {
		t.Errorf("after adding one that was in: %v", got)
	}
	if err := c.RemoveFromPlaylist(pl.Ref, []string{songs[1].Ref}); err != nil {
		t.Fatal(err)
	}
	if got := names(); len(got) != 1 || got[0] != songs[0].Ref {
		t.Errorf("after removing one: %v", got)
	}
	if err := c.RenamePlaylist(pl.Ref, "测试用临时歌单（改名）"); err != nil {
		t.Fatal(err)
	}
	// A cover: any picture the service takes, to which the playlist's name goes.
	png := append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, make([]byte, 64)...)
	if _, err := c.SetPlaylistCover(pl.Ref, "测试用临时歌单（改名）", png); err != nil && err != ErrBadCover {
		t.Logf("a cover of no real picture: %v", err)
	}
	lists, err := c.Playlists()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, l := range lists {
		if l.Ref == pl.Ref {
			found = l.Name == "测试用临时歌单（改名）"
		}
	}
	if !found {
		t.Error("the playlist is not listed under its new name")
	}
}
