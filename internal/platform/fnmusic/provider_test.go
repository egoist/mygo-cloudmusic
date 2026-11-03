package fnmusic

import (
	"context"
	"errors"
	"testing"

	"netease-music-mygo/internal/platform"
)

// A provider without a client rejects requests before touching the network.
func TestProviderRequiresClient(t *testing.T) {
	p := Wrap(nil)
	if p.ID() != platform.FnmusicID || p.Name() != "飞牛音乐" {
		t.Fatalf("identity = %q/%q", p.ID(), p.Name())
	}
	if _, err := p.Page(context.Background(), platform.PageRequest{Kind: platform.ListLibrary, Size: 10}); !errors.Is(err, ErrSignedOut) {
		t.Fatalf("Page error = %v, want ErrSignedOut", err)
	}
	if _, err := p.Search(context.Background(), "q", platform.SearchSongs, 0, 10); !errors.Is(err, ErrSignedOut) {
		t.Fatalf("Search error = %v, want ErrSignedOut", err)
	}
}

// A canceled request is rejected before a connected client is used.
func TestProviderHonorsCanceledContext(t *testing.T) {
	c, err := New(Config{Address: "127.0.0.1:1", Username: "u", PassHash: "p"})
	if err != nil {
		t.Fatal(err)
	}
	p := Wrap(c)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Page(ctx, platform.PageRequest{Kind: platform.ListLibrary, Size: 10}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Page error = %v, want context.Canceled", err)
	}
}

// Invalid page values fail at the provider seam instead of reaching the service.
func TestProviderRejectsInvalidPage(t *testing.T) {
	c, err := New(Config{Address: "127.0.0.1:1", Username: "u", PassHash: "p"})
	if err != nil {
		t.Fatal(err)
	}
	p := Wrap(c)
	if _, err := p.Page(context.Background(), platform.PageRequest{Kind: platform.ListLibrary, Page: -1, Size: 10}); err == nil {
		t.Fatal("negative page succeeded")
	}
	if _, err := p.Search(context.Background(), "q", platform.SearchSongs, 0, 0); err == nil {
		t.Fatal("zero page size succeeded")
	}
}
