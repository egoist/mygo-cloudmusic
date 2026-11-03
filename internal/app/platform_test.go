package app

import (
	"errors"
	"testing"

	appplatform "netease-music-mygo/internal/app/platform"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform"
	"netease-music-mygo/internal/platform/paging"

	"github.com/egoist/mygo/ui"
)

func newPlatformTestApp() *app {
	a := newTestApp()
	a.platforms = map[platform.ID]appplatform.Adapter{}
	a.platformID = platform.NeteaseID
	a.settings = defaultSettings()
	newPlatformAdapters(a)
	a.configurePlatformPlayer()
	return a
}

// Switching platforms changes only the platform-owned context and restores the upstream home when switched back.
func TestPlatformContextSwitch(t *testing.T) {
	a := newPlatformTestApp()
	if a.platformID != platform.NeteaseID {
		t.Fatalf("default platform = %q", a.platformID)
	}
	a.pl.Cue([]netease.Track{{ID: 1, Name: "网易云歌曲"}})
	a.setPlatform(platform.FnmusicID)
	if a.pl.Current() != nil || len(a.pl.Queue()) != 0 || a.pl.Radio() != "" {
		t.Fatal("platform switch retained playback context")
	}
	if a.platformID != platform.FnmusicID || a.settings.Platform != string(platform.FnmusicID) {
		t.Fatalf("Feiniu platform was not selected: id=%q setting=%q", a.platformID, a.settings.Platform)
	}
	if a.pl.ResolveStream == nil {
		t.Fatal("Feiniu player hooks were not installed")
	}
	if got := a.activeHome().Route().Page; got != "library" {
		t.Fatalf("Feiniu home route = %q, want library", got)
	}
	a.setPlatform(platform.NeteaseID)
	if a.platformID != platform.NeteaseID || a.settings.Platform != string(platform.NeteaseID) || a.route.page != pageHome {
		t.Fatalf("NetEase platform was not restored: id=%q setting=%q route=%v", a.platformID, a.settings.Platform, a.route)
	}
	if a.pl.ResolveStream != nil || a.pl.Artwork != nil || a.pl.ResolveLyrics != nil {
		t.Fatal("provider player hooks survived switching back to NetEase")
	}
}

// Selecting QQ Music installs its player hooks and starts on its home page.
func TestQQMusicPlatformContextSwitch(t *testing.T) {
	a := newPlatformTestApp()
	a.setPlatform(platform.QQMusicID)
	if a.platformID != platform.QQMusicID || a.settings.Platform != string(platform.QQMusicID) {
		t.Fatalf("QQ Music was not selected: id=%q setting=%q", a.platformID, a.settings.Platform)
	}
	if got := a.activeHome().Route().Page; got != "home" {
		t.Fatalf("QQ Music home route = %q", got)
	}
	if a.pl.ResolveStream == nil || a.pl.Artwork == nil || a.pl.ResolveLyrics == nil {
		t.Fatal("QQ Music player hooks were not installed")
	}
	a.setPlatform(platform.NeteaseID)
	if a.pl.ResolveStream != nil {
		t.Fatal("QQ Music player hooks survived switching back to NetEase")
	}
}

// Shared platform settings enumerate adapter identities instead of hardcoding provider names.
func TestPlatformInfosComeFromAdapters(t *testing.T) {
	a := newPlatformTestApp()
	infos := a.platformInfos()
	if len(infos) != 3 {
		t.Fatalf("platform count = %d, want 3", len(infos))
	}
	if infos[0].ID != platform.NeteaseID || infos[0].Name != "网易云音乐" {
		t.Fatalf("default platform = %#v", infos[0])
	}
	for _, info := range infos[1:] {
		if info.ID == platform.NeteaseID || info.Name == "" {
			t.Fatalf("adapter identity = %#v", info)
		}
	}
}

// Platform capabilities keep NetEase-only settings out of adapters that do not
// support them, while QQ Music keeps the playback rows it does share.
func TestPlatformCapabilities(t *testing.T) {
	a := newPlatformTestApp()
	infos := a.platformInfos()
	if !infos[0].Quality || !infos[0].Normalize || !infos[0].Podcasts || !infos[0].Comments {
		t.Fatalf("NetEase capabilities = %#v", infos[0])
	}
	for _, info := range infos[1:] {
		if info.Podcasts {
			t.Fatalf("NetEase-only capability leaked into adapter %#v", info)
		}
		if info.ID == platform.FnmusicID && (info.Quality || info.Normalize || info.Comments) {
			t.Fatalf("optional capabilities leaked into Feiniu %#v", info)
		}
		if info.ID == platform.QQMusicID && (!info.Quality || !info.Normalize || !info.Comments) {
			t.Fatalf("QQ Music lost its playback or comment settings %#v", info)
		}
	}
}

// A detail page waiting for its hero keeps every real label behind the same full skeleton.
func TestPlatformHeroWaitKeepsFullSkeleton(t *testing.T) {
	a := newTestApp()
	a.art.fetch = func(string) ([]byte, error) { return nil, errors.New("not ready") }
	track := netease.Track{ID: 1, Name: "真实歌曲"}
	list := &appplatform.TrackList{
		Total: func() int { return 1 },
		Len:   func() int { return 1 },
		At:    func(int) *netease.Track { return &track },
	}
	view := appplatform.TrackView{
		Cover: "fnmusic://cover",
		Header: func(c *ui.Context) {
			ui.Text(c, "真实标题")
		},
		HeaderSkeleton: func(c *ui.Context) {
			a.playlistHeaderSkeleton(c, a.pageShimmer(c))
		},
		ShowAlbum: true,
	}
	h := platformHost{a: a}
	tt := ui.NewTester(func(c *ui.Context) {
		t := *c.Theme()
		a.pal = paletteFor(&t)
		h.RenderTracks(c, list, view)
	}, 1200, 800)
	tt.Frame()
	if tt.HasText("真实标题") || tt.HasText("真实歌曲") {
		t.Fatal("real content showed through the hero-loading skeleton")
	}
}

// Search keeps loaded songs behind its full skeleton until album and artist results settle.
func TestPlatformSearchWaitsForAllFirstPages(t *testing.T) {
	a := newTestApp()
	track := netease.Track{ID: 1, Name: "真实歌曲"}
	songs := &appplatform.TrackList{
		Total: func() int { return 1 },
		Len:   func() int { return 1 },
		At:    func(int) *netease.Track { return &track },
	}
	albums := paging.New[netease.Playlist](60, 0, nil, nil)
	artists := paging.New[netease.Playlist](60, 0, nil, nil)
	h := platformHost{a: a}
	results := appplatform.SearchResults{Query: "春日影", Songs: songs, Albums: albums, Artists: artists}
	tt := ui.NewTester(func(c *ui.Context) {
		t := *c.Theme()
		a.pal = paletteFor(&t)
		h.RenderSearch(c, results, appplatform.SearchView{})
	}, 1200, 800)
	tt.Frame()
	if tt.HasText("真实歌曲") || tt.HasText("“春日影”") {
		t.Fatal("search revealed content before its first pages settled")
	}
	albums.Land(0, nil, 0)
	tt.Frame()
	if tt.HasText("真实歌曲") {
		t.Fatal("search revealed songs while artists were still loading")
	}
	artists.Land(0, nil, 0)
	tt.Frame()
	if !tt.HasText("真实歌曲") || !tt.HasText("“春日影”") {
		t.Fatal("search did not reveal its results after every first page settled")
	}
}

// A failed first list page shows its retry state instead of a permanent skeleton.
func TestPlatformListFailureLeavesSkeleton(t *testing.T) {
	a := newTestApp()
	list := &appplatform.TrackList{
		Total:  func() int { return -1 },
		Len:    func() int { return 0 },
		At:     func(int) *netease.Track { return nil },
		Failed: func() bool { return true },
	}
	h := platformHost{a: a}
	tt := ui.NewTester(func(c *ui.Context) {
		t := *c.Theme()
		a.pal = paletteFor(&t)
		h.RenderTracks(c, list, appplatform.TrackView{ShowAlbum: true})
	}, 1200, 800)
	tt.Frame()
	if !tt.HasText("加载失败，请稍后重试") {
		t.Fatal("failed list remained behind its loading skeleton")
	}
}
