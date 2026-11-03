package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// coverApp is Now Playing showing the second of three songs.
func coverApp(t *testing.T) (*app, *ui.Tester, ui.Rect) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 1, Name: "一"}, {ID: 2, Name: "二"}, {ID: 3, Name: "三"}})
	a.pl.Next()
	a.pl.SetLyrics(nil)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 860, 560)
	// Without the tilt and the swing, which the CPU draws slowly in depth:
	// the gestures do as they would.
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	tt.Advance(500 * time.Millisecond)
	r, ok := tt.Find("专辑封面")
	if !ok {
		t.Fatal("no cover")
	}
	return a, tt, r
}

// drag drags from the middle of r by dx, over a few frames, and lets go
// after hold, still.
func drag(tt *ui.Tester, r ui.Rect, dx float32, hold time.Duration) {
	x, y := r.X+r.W/2, r.Y+r.H/2
	tt.Press(x, y)
	for i := 1; i <= 6; i++ {
		tt.Advance(16 * time.Millisecond)
		tt.Move(x+dx*float32(i)/6, y)
	}
	if hold > 0 {
		tt.Advance(hold)
	}
	tt.Release(x+dx, y)
	tt.Advance(time.Second)
	tt.Frame()
}

// Flung far to the left, the cover flies off for the next song; to the
// right, for the one before, however far this one has played.
func TestFlingCover(t *testing.T) {
	a, tt, r := coverApp(t)
	if a.pl.Current().ID != 2 {
		t.Fatalf("playing %d", a.pl.Current().ID)
	}
	drag(tt, r, -r.W*0.8, 0)
	if id := a.pl.Current().ID; id != 3 {
		t.Fatalf("flung left: playing %d", id)
	}
	if r2, ok := tt.Find("专辑封面"); !ok || r2.X < r.X-2 || r2.X > r.X+2 {
		t.Errorf("the next cover rests at %v, not %v", r2, r)
	}
	drag(tt, r, r.W*0.8, 0)
	if id := a.pl.Current().ID; id != 2 {
		t.Errorf("flung right: playing %d", id)
	}
}

// A drag too short, let go still, springs back, the song playing on.
func TestCoverSpringsBack(t *testing.T) {
	a, tt, r := coverApp(t)
	drag(tt, r, -r.W*0.3, 300*time.Millisecond)
	if id := a.pl.Current().ID; id != 2 {
		t.Fatalf("a short drag changed the song to %d", id)
	}
	if r2, _ := tt.Find("专辑封面"); r2.X < r.X-2 || r2.X > r.X+2 {
		t.Errorf("the cover rests at %v, not %v", r2, r)
	}
}

// Two fingers swiping across the trackpad fling it too.
func TestSwipeCover(t *testing.T) {
	a, tt, r := coverApp(t)
	x, y := r.X+r.W/2, r.Y+r.H/2
	for range 10 {
		tt.Scroll(x, y, r.W*0.06, 0)
		tt.Advance(16 * time.Millisecond)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if id := a.pl.Current().ID; id != 3 {
		t.Errorf("swiped left: playing %d", id)
	}
}

// Played from the menu or the keys, the next song's cover slides in from
// the right and the one before's from the left, each coming to rest in its
// place.
func TestCoverSlidesWithTheSong(t *testing.T) {
	a, tt, r := coverApp(t)
	tt.SetPreferences(ui.Preferences{})
	for _, step := range []struct {
		name string
		play func()
		id   int64
		dir  float32
	}{{"next", a.pl.Next, 3, 1}, {"prev", a.pl.Prev, 2, -1}} {
		step.play()
		tt.Frame()
		tt.Advance(60 * time.Millisecond)
		tt.Frame()
		if id := a.pl.Current().ID; id != step.id {
			t.Fatalf("%s: playing %d", step.name, id)
		}
		if r2, _ := tt.Find("专辑封面"); (r2.X-r.X)*step.dir < r.W*0.1 {
			t.Errorf("%s: the cover comes in at %v, from %v", step.name, r2.X, r.X)
		}
		tt.Advance(time.Second)
		tt.Frame()
		if r2, _ := tt.Find("专辑封面"); r2.X < r.X-2 || r2.X > r.X+2 {
			t.Errorf("%s: the cover rests at %v, not %v", step.name, r2, r)
		}
	}
}

// A cover shorter than the controls keeps them their width, and used to sit
// at that column's left edge; it stays centered over them.
func TestShortCoverStaysCenteredOverTheControls(t *testing.T) {
	_, _, r := coverApp(t)
	if r.W >= npControlsWidth-1 {
		t.Fatalf("the cover is %v wide, no shorter than the controls, so nothing is centered", r)
	}
	want := npSideX(860) + (npControlsWidth-r.W)/2
	if r.X < want-1 || r.X > want+1 {
		t.Errorf("the cover sits at %v, not centered over the controls at %v", r.X, want)
	}
}
