package app

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

func TestSongMenuCopies(t *testing.T) {
	var copied string
	real := copyText
	copyText = func(s string) error { copied = s; return nil }
	defer func() { copyText = real }()

	a := newTestApp()
	a.settings = defaultSettings()
	tr := netease.Track{ID: 1859245776, Name: "宝船", Artists: []netease.Artist{{Name: "DOUDOU"}, {Name: "小明"}}, Duration: 200000}
	a.pl.Cue([]netease.Track{tr})
	a.pl.SetLyrics(nil)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()

	for _, c := range []struct{ item, want string }{
		{"复制歌名", "宝船"},
		{"复制歌名和歌手", "宝船 - DOUDOU, 小明"},
		{"复制链接", "https://music.163.com/song?id=1859245776"},
	} {
		if err := tt.Click("更多"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem(c.item); err != nil {
			t.Fatalf("%v; the menu shows %q", err, tt.Menu())
		}
		tt.Frame()
		if copied != c.want {
			t.Errorf("%s copied %q, want %q", c.item, copied, c.want)
		}
	}
}

func TestSongMenuOnRightClick(t *testing.T) {
	var copied string
	real := copyText
	copyText = func(s string) error { copied = s; return nil }
	defer func() { copyText = real }()

	a := newTestApp()
	a.settings = defaultSettings()
	a.playlists[42] = &netease.Playlist{ID: 42, Name: "飙升榜", Tracks: []netease.Track{
		{ID: 1, Name: "一张相片", Artists: []netease.Artist{{Name: "林宥嘉"}}, Duration: 200000},
		{ID: 2, Name: "宝船", Artists: []netease.Artist{{Name: "DOUDOU"}}, Duration: 200000},
	}}
	a.route = route{page: pagePlaylist, id: 42}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()

	if err := tt.RightClick("宝船"); err != nil {
		t.Fatal(err)
	}
	if got, want := tt.Menu(), []string{"下一首播放", "加入插播队列", "-", "复制歌名", "复制歌名和歌手", "-", "复制链接"}; !slices.Equal(got, want) {
		t.Fatalf("the menu shows %q, want %q", got, want)
	}
	if err := tt.ChooseMenuItem("复制链接"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if want := "https://music.163.com/song?id=2"; copied != want {
		t.Errorf("copied %q, want %q", copied, want)
	}
	if a.pl.Current() != nil {
		t.Error("the right click played the song")
	}
}
