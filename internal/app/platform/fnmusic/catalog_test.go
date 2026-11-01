package fnmusic

import (
	"testing"

	backend "netease-music-mygo/internal/platform/fnmusic"
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
