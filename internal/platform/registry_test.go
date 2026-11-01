package platform

import (
	"context"
	"testing"

	"netease-music-mygo/internal/netease"
)

type testProvider struct{ id ID }

func (p testProvider) ID() ID       { return p.id }
func (p testProvider) Name() string { return string(p.id) }
func (p testProvider) Page(context.Context, PageRequest) (Page, error) {
	return Page{}, nil
}
func (p testProvider) Search(context.Context, string, SearchKind, int, int) (Page, error) {
	return Page{}, nil
}
func (p testProvider) Stream(context.Context, *netease.Track) (*netease.Stream, error) {
	return nil, nil
}
func (p testProvider) Lyrics(context.Context, *netease.Track) (Lyrics, error) {
	return Lyrics{}, nil
}
func (p testProvider) Cover(context.Context, string, int) ([]byte, error) {
	return nil, nil
}

// A registry rejects duplicate platform IDs and returns a copy of its order.
func TestRegistryRegistration(t *testing.T) {
	a := testProvider{id: NeteaseID}
	b := testProvider{id: FnmusicID}
	r, err := NewRegistry(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Get(FnmusicID); got != b {
		t.Fatalf("Get(fnmusic) = %#v, want %#v", got, b)
	}
	ids := r.IDs()
	ids[0] = FnmusicID
	if got := r.IDs()[0]; got != NeteaseID {
		t.Fatalf("IDs returned internal storage: %v", r.IDs())
	}
	if err := r.Register(testProvider{id: NeteaseID}); err == nil {
		t.Fatal("Register duplicate succeeded")
	}
}
