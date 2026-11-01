package app

import (
	"testing"

	appplatform "netease-music-mygo/internal/app/platform"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform"
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

// Shared platform settings enumerate adapter identities instead of hardcoding provider names.
func TestPlatformInfosComeFromAdapters(t *testing.T) {
	a := newPlatformTestApp()
	infos := a.platformInfos()
	if len(infos) != 2 {
		t.Fatalf("platform count = %d, want 2", len(infos))
	}
	if infos[0].ID != platform.NeteaseID || infos[0].Name != "网易云音乐" {
		t.Fatalf("default platform = %#v", infos[0])
	}
	if infos[1].ID == platform.NeteaseID || infos[1].Name == "" {
		t.Fatalf("adapter identity = %#v", infos[1])
	}
}

// Platform capabilities keep NetEase-only settings out of Feiniu's settings page.
func TestPlatformCapabilities(t *testing.T) {
	a := newPlatformTestApp()
	infos := a.platformInfos()
	if !infos[0].Quality || !infos[0].Normalize || !infos[0].Podcasts || !infos[0].Comments {
		t.Fatalf("NetEase capabilities = %#v", infos[0])
	}
	if infos[1].Quality || infos[1].Normalize || infos[1].Podcasts || infos[1].Comments {
		t.Fatalf("Feiniu capabilities = %#v", infos[1])
	}
}
