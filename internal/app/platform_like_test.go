package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// The filled heart in a provider row really sends an unlike, not another like.
func TestProviderTrackUnlikeClick(t *testing.T) {
	a := newTestApp()
	tr := netease.Track{ID: 1, Name: "收藏歌曲", Duration: 200000}
	var got bool
	var called bool
	tt := ui.NewTester(func(c *ui.Context) {
		t := *c.Theme()
		a.pal = paletteFor(&t)
		a.providerTrackRow(c, 0, &tr, nil, 0, true, true, trackRowsOptions{
			Liked: func(int64) bool { return true },
			Favorite: func(_ netease.Track, liked bool) {
				called, got = true, liked
			},
		}, func(int) {})
	}, 800, 100)
	tt.Frame()
	if err := tt.Click("取消喜欢"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !called || got {
		t.Fatalf("unlike callback called=%v liked=%v", called, got)
	}
}
