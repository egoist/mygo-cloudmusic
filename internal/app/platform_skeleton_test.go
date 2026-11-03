package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	appplatform "netease-music-mygo/internal/app/platform"
	appfnmusic "netease-music-mygo/internal/app/platform/fnmusic"
	"netease-music-mygo/internal/netease"
	backend "netease-music-mygo/internal/platform/fnmusic"
	"netease-music-mygo/internal/platform/paging"

	"github.com/egoist/mygo/ui"
)

var fnmusicSkeletonScenes = []struct {
	name  string
	route appplatform.Route
}{
	{"library", appplatform.Route{Page: appfnmusic.PageLibrary}},
	{"favorites", appplatform.Route{Page: appfnmusic.PagePlaylist, ID: backend.FavoritesID}},
	{"albums", appplatform.Route{Page: appfnmusic.PageAlbums}},
	{"artists", appplatform.Route{Page: appfnmusic.PageArtists}},
	{"artist-songs", appplatform.Route{Page: appfnmusic.PagePlaylist, ID: 42, Kind: backend.KindArtist, Query: "artist"}},
	{"artist-albums", appplatform.Route{Page: appfnmusic.PageArtistAlbums, ID: 42, Query: "artist"}},
	{"genres", appplatform.Route{Page: appfnmusic.PageGenres}},
	{"search", appplatform.Route{Page: appfnmusic.PageSearch, Query: "春日影"}},
	{"folders", appplatform.Route{Page: appfnmusic.PageFolders}},
	{"folder", appplatform.Route{Page: appfnmusic.PageFolders, Query: "/music"}},
	{"roam", appplatform.Route{Page: appfnmusic.PageRoam}},
}

// With NM_FRAMES set, saves every Feiniu loading layout without making a
// network request, in the light and the dark.
func TestFnmusicSkeletonFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, s := range fnmusicSkeletonScenes {
		for _, dark := range []bool{false, true} {
			a := newTestApp()
			settings := &appplatform.PlatformSettings{Config: appplatform.ConnectionConfig{
				Address: "127.0.0.1:1", Username: "test", PassHash: "test",
			}}
			adapter := appfnmusic.NewAdapter(platformHost{a: a}, settings)
			home := adapter.Home()
			home.ResetNavigation(s.route)
			view := func(c *ui.Context) {
				t := *c.Theme()
				a.pal = paletteFor(&t)
				home.Content(c)
			}
			tt := ui.NewTester(view, 1200, 800)
			tt.SetDark(dark)
			tt.Advance(450 * time.Millisecond)
			tt.Frame()
			name := "fnmusic-" + s.name
			if dark {
				name += "-dark"
			}
			f, _ := os.Create(filepath.Join(dir, "skeleton-"+name+".png"))
			png.Encode(f, tt.Image())
			f.Close()
		}
	}
}

// With NM_FRAMES set, saves album cards whose covers are missing.
func TestFnmusicAlbumPlaceholderFrame(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	a := newTestApp()
	p := paging.New[netease.Playlist](60, 0, nil, nil)
	p.Land(0, []netease.Playlist{
		{ID: 1, Name: "无封面专辑"},
		{ID: 2, Name: "另一张无封面专辑"},
	}, 2)
	h := platformHost{a: a}
	tt := ui.NewTester(func(c *ui.Context) {
		t := *c.Theme()
		a.pal = paletteFor(&t)
		h.RenderCards(c, p, appplatform.CardView{
			Title: "专辑",
			Header: func(c *ui.Context) {
				ui.Text(c, "专辑").FontSize(30).FontWeight(700).Margin(0, 0, 18, 0)
			},
			Placeholder: iconNote,
		})
	}, 1200, 800)
	tt.Frame()
	f, _ := os.Create(filepath.Join(dir, "loaded-fnmusic-album-placeholder.png"))
	png.Encode(f, tt.Image())
	f.Close()
}
