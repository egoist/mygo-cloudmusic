package fnmusic

import (
	"testing"

	"netease-music-mygo/internal/netease"
	backend "netease-music-mygo/internal/platform/fnmusic"
)

// Card routes retain the provider ref so opening an album or artist cannot fall back to an invalid list ID.
func TestCardRouteKeepsProviderReference(t *testing.T) {
	s := NewState()
	c := s.Catalog()
	const id int64 = 42
	c.Meta[id] = netease.Playlist{ID: id, Ref: "album-guid", Kind: backend.KindAlbum}
	h := &Home{state: s}
	r := h.cardRoute(id)
	if r.Page != PagePlaylist || r.ID != id || r.Kind != backend.KindAlbum || r.Query != "album-guid" {
		t.Fatalf("card route = %#v", r)
	}
}

// Platform navigation keeps a detail page reachable without touching the upstream route history.
func TestPlatformNavigationBackAndForward(t *testing.T) {
	h := NewHome(NewState(), nil)
	h.ResetNavigation(Route{Page: PageLibrary})
	h.Open(Route{Page: PageAlbums})
	h.Open(Route{Page: PagePlaylist, ID: 42, Kind: backend.KindAlbum, Query: "album-guid"})
	if !h.CanBack() || h.CanForward() {
		t.Fatalf("navigation state after open: back=%v forward=%v", h.CanBack(), h.CanForward())
	}
	h.Back()
	if got := h.Route(); got.Page != PageAlbums || !h.CanForward() {
		t.Fatalf("after back: route=%#v forward=%v", got, h.CanForward())
	}
	h.Forward()
	if got := h.Route(); got.Page != PagePlaylist || got.Kind != backend.KindAlbum || !h.CanBack() {
		t.Fatalf("after forward: route=%#v back=%v", got, h.CanBack())
	}
}

// Address mode strips an explicitly typed scheme while preserving the selected protocol.
func TestSplitScheme(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		secure bool
		want   string
		wantOn bool
	}{
		{name: "explicit https", input: "https://nas.example/music/", want: "nas.example/music", wantOn: true},
		{name: "explicit http", input: "http://192.168.1.10:5666/", secure: true, want: "192.168.1.10:5666", wantOn: false},
		{name: "implicit", input: "nas.example:5666", secure: true, want: "nas.example:5666", wantOn: true},
	}
	for _, tt := range tests {
		got, secure := splitScheme(tt.input, tt.secure)
		if got != tt.want || secure != tt.wantOn {
			t.Errorf("splitScheme(%q, %v) = (%q, %v), want (%q, %v)", tt.input, tt.secure, got, secure, tt.want, tt.wantOn)
		}
	}
}

// The account menu changes from connect to reconnect/disconnect when a session is attached.
func TestAccountActionsFollowConnectionState(t *testing.T) {
	s := NewState()
	h := NewHome(s, nil)
	if actions := h.accountActions(); len(actions) != 2 || actions[1].Label != "连接飞牛音乐…" {
		t.Fatalf("disconnected actions = %#v", actions)
	}
	c, err := backend.New(backend.Config{Address: "127.0.0.1:1", Username: "u", PassHash: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Attach(c); err != nil {
		t.Fatal(err)
	}
	actions := h.accountActions()
	if len(actions) != 3 || actions[1].Label != "重新连接…" || actions[2].Label != "断开飞牛音乐" {
		t.Fatalf("connected actions = %#v", actions)
	}
}
