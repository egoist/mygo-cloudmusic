package app

import (
	"testing"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform"
	backend "netease-music-mygo/internal/platform/fnmusic"
)

// Feiniu Now Playing omits the NetEase comments pane because that platform has no comments API.
func TestFeiniuNowPlayingHidesComments(t *testing.T) {
	a := newPlatformTestApp()
	a.setPlatform(platform.FnmusicID)
	a.pl.Cue([]netease.Track{{ID: 1, Source: backend.Source, Name: "飞牛歌曲"}})
	for _, tab := range a.npTabs() {
		if tab.pane == paneComments {
			t.Fatal("Feiniu exposed the comments pane")
		}
	}
}
