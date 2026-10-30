package app

import (
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/player"
)

func newTestApp() *app {
	a := &app{
		playlists: map[int64]*netease.Playlist{}, loadingPl: map[int64]bool{}, plErr: map[int64]string{},
		artists: map[int64]*artistData{}, albums: map[int64]*albumData{}, users: map[int64]*userData{},
		podcasts: map[int64]*podcastData{}, podcastTab: &podcastTab{}, searches: map[route]*searchData{},
		lyricAt: -1, queueAt: -1,
	}
	a.art = newArtCache(func(func()) {}, "")
	a.pl = player.New(func(func()) {})
	a.pl.SetVolume(0.5)
	return a
}
