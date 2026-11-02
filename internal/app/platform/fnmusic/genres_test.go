package fnmusic

import (
	"testing"

	backend "netease-music-mygo/internal/platform/fnmusic"
)

// A style card opens its songs in the chosen order, not as a playlist of the same GUID.
func TestGenreCardOpensGenreList(t *testing.T) {
	s := NewState()
	client, err := backend.New(backend.Config{Address: "127.0.0.1:1", Username: "u", PassHash: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Attach(client); err != nil {
		t.Fatal(err)
	}
	c := s.Catalog()
	c.GenreTrackSort = backend.SortGenreTrackTitleAsc
	card := backend.Genre{GUID: "g1", TrackCount: 3}.Card()
	h := &Home{state: s}
	r := h.cardRoute(card.ID)
	l := c.ListForRoute(r.ID, card.Kind, card.Ref)
	if l == nil || l.Spec.Kind != ListGenre || l.Spec.Ref != "g1" || l.Spec.Sort != backend.SortGenreTrackTitleAsc {
		t.Fatalf("list = %#v", l)
	}
	if card.Name != "未命名风格" {
		t.Fatalf("unnamed style card name = %q", card.Name)
	}
}
