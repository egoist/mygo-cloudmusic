package fnmusic

import (
	"testing"

	"netease-music-mygo/internal/netease"
	backend "netease-music-mygo/internal/platform/fnmusic"
	"netease-music-mygo/internal/platform/paging"
)

// NewList maps platform list specs to stable IDs and keeps loading state outside the view.
func TestCatalogListMapping(t *testing.T) {
	s := NewState()
	c, err := backend.New(backend.Config{Address: "127.0.0.1:1", Username: "u", PassHash: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Attach(c); err != nil {
		t.Fatal(err)
	}
	catalog := NewCatalog(s, nil)
	l := catalog.NewList(Spec{Kind: ListLibrary})
	if l == nil || l.ID != backend.LibraryID || l.Pages == nil || l.Name != "音乐库" {
		t.Fatalf("library list = %#v", l)
	}
	if got := catalog.NewList(Spec{Kind: ListPlaylist, Ref: "playlist-1"}); got == nil || got.ID != backend.ID("playlist-1") {
		t.Fatalf("playlist list = %#v", got)
	}
}

// Unliking from the favorites page removes the loaded row without a refetch.
func TestRemoveFavoriteFromLoadedPage(t *testing.T) {
	catalog := NewCatalog(NewState(), nil)
	p := paging.New[netease.Track](60, 0, nil, nil)
	p.Land(0, []netease.Track{{ID: 1}, {ID: 2}, {ID: 3}}, 3)
	catalog.Lists[backend.FavoritesID] = &List{ID: backend.FavoritesID, Pages: p}
	if !catalog.RemoveFavorite(2) {
		t.Fatal("favorite was not removed")
	}
	if p.Total() != 2 {
		t.Fatalf("favorites after removal: total=%d", p.Total())
	}
	if catalog.RemoveFavorite(99) {
		t.Fatal("removed a song that was not loaded")
	}
}

// A new favorite invalidates the cached favorites list so the next visit fetches it.
func TestFavoriteChangedInvalidatesCache(t *testing.T) {
	catalog := NewCatalog(NewState(), nil)
	p := paging.New[netease.Track](60, 0, nil, nil)
	p.Land(0, []netease.Track{{ID: 1}}, 1)
	catalog.Lists[backend.FavoritesID] = &List{ID: backend.FavoritesID, Pages: p}

	catalog.FavoriteChanged(9, true)
	if _, ok := catalog.Lists[backend.FavoritesID]; ok {
		t.Fatal("liking left the favorites cache in place")
	}

	p = paging.New[netease.Track](60, 0, nil, nil)
	p.Land(0, []netease.Track{{ID: 1}, {ID: 2}}, 2)
	catalog.Lists[backend.FavoritesID] = &List{ID: backend.FavoritesID, Pages: p}
	catalog.FavoriteChanged(1, false)
	if _, ok := catalog.Lists[backend.FavoritesID]; !ok {
		t.Fatal("unliking dropped the loaded favorites list")
	}
	if p.Total() != 1 {
		t.Fatalf("favorites total = %d", p.Total())
	}
}

// Refresh bypasses loaded pages and asks the provider again.
func TestListRefreshReloadsLoadedPages(t *testing.T) {
	calls := make(chan struct{}, 2)
	p := paging.New[netease.Track](60, 0, func(fn func()) { fn() }, func(int) ([]netease.Track, int, error) {
		calls <- struct{}{}
		return []netease.Track{{ID: 2}}, 1, nil
	})
	p.Land(0, []netease.Track{{ID: 1}}, 1)
	l := &List{Pages: p}
	l.Refresh()
	<-calls
}
