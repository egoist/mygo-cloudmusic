package fnmusic

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
	backend "netease-music-mygo/internal/platform/fnmusic"
)

// pageHost draws the few shared primitives a folder page uses, and navigates
// the home it is given as the app does.
type pageHost struct {
	nopHost
	home *Home
}

func (h pageHost) Navigate(r Route) { h.home.Open(r) }
func (pageHost) Post(fn func())     { fn() }
func (pageHost) PageList(c *ui.Context, key string, st *ui.ListState, rows int, row func(int)) {
	ui.Column(c.Key(key)).Fill().Children(func() {
		for i := 0; i < rows; i++ {
			row(i)
		}
	})
}
func (pageHost) TrackRow(c *ui.Context, i int, tr *netease.Track, view TrackView) {
	ui.Text(c, tr.Name)
}
func (pageHost) Cover(c *ui.Context, url string, size, radius float32) ui.Element {
	return ui.Box(c).Size(size, size)
}
func (pageHost) PillButton(c *ui.Context, key string, icon *ui.SVG, label string, primary bool) ui.Element {
	return ui.Button(c.Key(key), label)
}
func (pageHost) MoreButton(c *ui.Context, key string, menu func(*ui.Menu)) ui.Element {
	return ui.Button(c.Key(key), "更多")
}

// Clicking a folder row opens it as its own page, and Back returns to the folder it was in.
func TestFolderRowOpensFolderAndBackReturns(t *testing.T) {
	s := NewState()
	client, err := backend.New(backend.Config{Address: "127.0.0.1:1", Username: "u", PassHash: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Attach(client); err != nil {
		t.Fatal(err)
	}
	s.capabilities, s.capabilitiesKnown = backend.FolderCapabilities{FolderView: true}, true
	h := NewHome(s, nil)
	h.host = pageHost{home: h}
	f := h.syncFolders()
	f.roots = []backend.SharedLibrary{{Name: "音乐", Path: "/music"}}
	f.pages["/music"] = backend.FolderPage{Name: "music", Path: "/music", Folders: []backend.FolderNode{{Name: "Adele", Path: "/music/Adele", ItemCount: 2}}}
	f.pages["/music/Adele"] = backend.FolderPage{Name: "Adele", Path: "/music/Adele", Tracks: []backend.FolderTrack{{Track: netease.Track{Name: "Hello"}}}}
	h.ResetNavigation(Route{Page: PageFolders, Query: "/music"})

	tt := ui.NewTester(h.Content, 1000, 900)
	tt.Frame()
	if err := tt.Click("Adele"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := h.Route(); got.Page != PageFolders || got.Query != "/music/Adele" {
		t.Fatalf("after click: route = %#v", got)
	}
	if _, ok := tt.Find("Hello"); !ok {
		t.Fatal("the opened folder does not list its song")
	}
	h.Back()
	tt.Frame()
	if got := h.Route(); got.Query != "/music" {
		t.Fatalf("after back: route = %#v", got)
	}
}
