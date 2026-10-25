package app

import (
	"image"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// A cover that loads while its place shows fades in; one ready as its
// place first shows is there at once.
func TestArtFadesIn(t *testing.T) {
	var bmp *ui.Bitmap
	var in float32
	tt := ui.NewTester(func(c *ui.Context) { in = artIn(ui.Box(c).Size(40, 40), bmp) }, 100, 100)
	tt.Frame()
	if in != 0 {
		t.Fatalf("shown %v while loading", in)
	}
	bmp = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 4, 4)))
	tt.Frame()
	tt.Advance(artFade.Duration / 3)
	if in <= 0 || in >= 1 {
		t.Errorf("shown %v a third of the way", in)
	}
	tt.Advance(artFade.Duration)
	if in != 1 {
		t.Errorf("shown %v once faded in", in)
	}

	in = 0
	ready := ui.NewTester(func(c *ui.Context) { in = artIn(ui.Box(c).Size(40, 40), bmp) }, 100, 100)
	ready.Frame()
	if in != 1 {
		t.Errorf("a cover ready at once shown %v", in)
	}
}

// The play button's icon turns into the pause icon: both show on the
// way, then only the pause icon.
func TestPlayPauseMorphs(t *testing.T) {
	playing := false
	var v float32
	tt := ui.NewTester(func(c *ui.Context) {
		b := playButton(c, playing, 24, white)
		to := float32(0)
		if playing {
			to = 1
		}
		// Asked for the same, the button gives the value it animates.
		v = b.AnimateTo("playing", to, ui.Animation{Spring: ui.SpringOptions{Duration: 340 * time.Millisecond, Bounce: 0.3}})
	}, 100, 100)
	tt.Frame()
	if v != 0 {
		t.Fatalf("paused at %v", v)
	}
	playing = true
	tt.Frame()
	tt.Advance(80 * time.Millisecond)
	if v <= 0 || v >= 1 {
		t.Errorf("at %v on the way", v)
	}
	tt.Advance(time.Second)
	if v != 1 {
		t.Errorf("at %v once playing", v)
	}
}
