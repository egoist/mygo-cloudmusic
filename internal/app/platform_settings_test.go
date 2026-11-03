package app

import (
	"testing"

	"netease-music-mygo/internal/platform"
)

// QQ keeps its playback preferences in its own namespace, leaving the shared
// ones, and NetEase's, alone.
func TestQQSettingsStayInPlatformNamespace(t *testing.T) {
	a := newPlatformTestApp()
	shared := a.settings
	a.setPlatform(platform.QQMusicID)

	if got := a.qualityLevel(); got != "exhigh" {
		t.Fatalf("default QQ quality = %q, want exhigh", got)
	}
	if !a.normalizeEnabled() {
		t.Fatal("QQ normalisation defaults to off")
	}
	if a.hotCommentsEnabled() {
		t.Fatal("QQ hot comments default to on")
	}

	a.setQualityLevel("lossless")
	a.setNormalize(false)
	a.setHotComments(true)

	if a.settings.QQMusic.Quality != "lossless" {
		t.Fatalf("QQ quality = %q, want lossless in the namespace", a.settings.QQMusic.Quality)
	}
	if a.settings.QQMusic.Normalize == nil || *a.settings.QQMusic.Normalize {
		t.Fatal("QQ normalisation was not written to the namespace")
	}
	if !a.settings.QQMusic.HotComments {
		t.Fatal("QQ hot comments were not written to the namespace")
	}
	if a.settings.Quality != shared.Quality || a.settings.Normalize != shared.Normalize || a.settings.HotComments != shared.HotComments {
		t.Fatal("writing QQ settings changed the shared ones")
	}
}

// The player follows the active platform's quality and normalisation.
func TestPlayerQualityFollowsPlatform(t *testing.T) {
	a := newPlatformTestApp()
	a.settings.Quality, a.settings.Normalize = "lossless", false
	a.setPlatform(platform.QQMusicID)
	a.setQualityLevel("standard")
	a.setNormalize(true)
	a.configurePlatformPlayer()
	if got := a.pl.Quality(); got != "standard" {
		t.Fatalf("player quality on QQ = %q, want standard", got)
	}
	if !a.pl.Normalize() {
		t.Fatal("player normalisation on QQ = off, want on")
	}
	a.setPlatform(platform.NeteaseID)
	a.configurePlatformPlayer()
	if got := a.pl.Quality(); got != "lossless" {
		t.Fatalf("player quality on NetEase = %q, want lossless", got)
	}
	if a.pl.Normalize() {
		t.Fatal("player normalisation on NetEase = on, want off")
	}
}

// Search history of a platform is kept in that platform's namespace.
func TestPlatformSearchHistoryNamespaced(t *testing.T) {
	a := newPlatformTestApp()
	host := platformHost{a: a}
	a.setPlatform(platform.QQMusicID)
	host.RememberSearch("周杰伦")
	host.RememberSearch("邓紫棋")
	host.RememberSearch("周杰伦")
	got := host.SearchHistory()
	if len(got) != 2 || got[0] != "周杰伦" || got[1] != "邓紫棋" {
		t.Fatalf("QQ search history = %v, want 周杰伦 then 邓紫棋 after searching it again", got)
	}
	if len(a.settings.QQMusic.SearchHistory) != 2 {
		t.Fatalf("QQ namespace history = %v", a.settings.QQMusic.SearchHistory)
	}
	if len(a.settings.SearchHistory) != 0 {
		t.Fatalf("shared history was written: %v", a.settings.SearchHistory)
	}
	a.setPlatform(platform.FnmusicID)
	host.RememberSearch("飞牛")
	if len(host.SearchHistory()) != 1 || host.SearchHistory()[0] != "飞牛" {
		t.Fatalf("Feiniu history = %v", host.SearchHistory())
	}
	if len(a.settings.QQMusic.SearchHistory) != 2 {
		t.Fatal("writing Feiniu history touched QQ's")
	}
}
