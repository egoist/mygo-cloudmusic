package fnmusic

import (
	"context"
	"testing"

	backend "netease-music-mygo/internal/platform/fnmusic"
)

// Attaching and detaching a session clears account state instead of leaking it to the next login.
func TestStateSessionLifecycle(t *testing.T) {
	s := NewState()
	c, err := backend.New(backend.Config{Address: "127.0.0.1:1", Username: "u", PassHash: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Attach(c); err != nil {
		t.Fatal(err)
	}
	s.SetLiked(7, true)
	s.SetPlaylists(nil)
	s.SetRoamID("r1")
	if !s.Connected() || !s.Liked(7) || s.RoamID() != "r1" || s.Provider() == nil {
		t.Fatal("attached state was not published")
	}
	if got := s.Detach(); got != c {
		t.Fatal("Detach returned a different client")
	}
	if s.Connected() || s.Liked(7) || s.RoamID() != "" || s.Provider() != nil {
		t.Fatal("detached state retained session data")
	}
}

// Replacing a session keeps catalog completions on the application owner goroutine.
func TestStateCatalogKeepsPost(t *testing.T) {
	s := NewState()
	post := func(func()) {}
	s.SetPost(post)
	if got := s.Catalog().Post; got == nil {
		t.Fatal("catalog lost owner post callback")
	}
	c, err := backend.New(backend.Config{Address: "127.0.0.1:1", Username: "u", PassHash: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Attach(c); err != nil {
		t.Fatal(err)
	}
	if got := s.Catalog().Post; got == nil {
		t.Fatal("attached catalog lost owner post callback")
	}
	s.Detach()
	if got := s.Catalog().Post; got == nil {
		t.Fatal("detached catalog lost owner post callback")
	}
}

// Cancel invalidates asynchronous work without dropping the connected client.
func TestStateCancelKeepsClient(t *testing.T) {
	s := NewState()
	c, err := backend.New(backend.Config{Address: "127.0.0.1:1", Username: "u", PassHash: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Attach(c); err != nil {
		t.Fatal(err)
	}
	_, gen, err := s.activeClient(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s.Cancel()
	if !s.Connected() || s.clientCurrent(c, gen) {
		t.Fatal("cancel did not invalidate the old generation")
	}
}

// Generation changes when a session is canceled, so late platform UI work can be ignored.
func TestStateGenerationChangesOnCancel(t *testing.T) {
	s := NewState()
	before := s.Generation()
	s.Cancel()
	if after := s.Generation(); after == before {
		t.Fatalf("generation stayed %d after cancel", before)
	}
}
