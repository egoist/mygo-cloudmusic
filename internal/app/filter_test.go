package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// filterTestApp shows a playlist whose songs differ in name, album and
// artists, and one whose album is named after it.
func filterTestApp() *app {
	a := editTestApp()
	a.playlists[10].Tracks = []netease.Track{
		{ID: 1, Name: "一张相片", Artists: []netease.Artist{{Name: "林宥嘉"}}, Album: netease.Album{Name: "今日营业中"}, Duration: 200000},
		{ID: 2, Name: "宝船", Artists: []netease.Artist{{Name: "DOUDOU"}}, Album: netease.Album{Name: "宝船"}, Duration: 200000},
		{ID: 3, Name: "山海", Artists: []netease.Artist{{Name: "林宥嘉"}}, Album: netease.Album{Name: "贾想"}, Duration: 200000},
	}
	return a
}

// The field over a playlist narrows it to the songs finding its words in
// their name, album or artists, a click on one of them plays it, and
// Escape shows every song again.
func TestPlaylistFilter(t *testing.T) {
	a := filterTestApp()
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("搜索歌单"); err != nil {
		t.Fatal(err)
	}
	tt.Type("林宥嘉")
	tt.Frame()
	if err := tt.Click("宝船"); err == nil {
		t.Error("listed a song the filter leaves out")
	}
	if err := tt.Click("山海"); err != nil {
		t.Fatalf("left out the song whose artist the filter finds: %v", err)
	}
	if cur := a.pl.Current(); cur == nil || cur.ID != 3 {
		t.Errorf("played %v, want the song clicked", cur)
	}

	// ⌘F puts the pointer back in the field from anywhere on the page, and
	// Escape gives up what it holds, with every song back.
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if err := tt.Click("宝船"); err != nil {
		t.Error("Escape left the page narrowed")
	}
}

// A page's filter finds a song by its name, its album, its aliases or its
// artists, holding every word of what was typed, whatever the case.
func TestFilterTracks(t *testing.T) {
	tracks := []netease.Track{
		{ID: 1, Name: "昨天", Artists: []netease.Artist{{Name: "Jay Chou"}}, Album: netease.Album{Name: "范特西"}},
		{ID: 2, Name: "晴天", Artists: []netease.Artist{{Name: "周杰伦"}}, Album: netease.Album{Name: "叶惠美"}},
		{ID: 3, Name: "搁浅", Artists: []netease.Artist{{Name: "周杰伦"}}, Album: netease.Album{Name: "七里香"}, Alias: []string{"Step Aside"}},
	}
	for _, c := range []struct {
		query string
		want  []int64
	}{
		{"晴天", []int64{2}},
		{"叶惠美", []int64{2}},
		{"周杰伦", []int64{2, 3}},
		{"周杰伦 七里香", []int64{3}},
		{"jay", []int64{1}},
		{"step aside", []int64{3}},
		{"台风", nil},
	} {
		var got []int64
		for _, tr := range filterTracks(tracks, strings.Fields(strings.ToLower(c.query))) {
			got = append(got, tr.ID)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%q found %v, want %v", c.query, got, c.want)
		}
	}
}
