package app

import (
	"math"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"

	"netease-music-mygo/internal/netease"
)

// The cover of Now Playing is a thing to touch. It tilts toward the
// pointer in depth, a glare following the pointer over it as light over a
// card held up. Dragged or swiped across, it swings as it goes, and flung
// far enough it flies off: to the left for the next song, to the right
// for the one before, and the new cover comes in from the other side. A
// fling too weak springs back. Another song played by the buttons, the
// keys or the queue slides in the same way, the cover before sliding off.
// Asked for less motion, it neither tilts nor swings, and a song coming
// fades in over the one going.

// coverState is the cover as the pointer and the trackpad move it.
type coverState struct {
	off   ui.Point // where Draggable rests it, from its place
	gen   int      // counts the flings, each cover after one a new element
	flung int      // 1 flying off for the next song, -1 for the one before
	enter float32  // where the next cover comes in from, 0 for none
	// A trackpad's swipe: how far it has gone, when it last moved, and
	// whether it was decided, the momentum after it swallowed.
	swipe   float32
	swipeAt time.Time
	decided bool
	// The song the cover shows, where it is in the play order, and where
	// its cover goes as another comes (exit).
	track int64
	at    int
	exit  *ui.Motion
}

type coverKey struct {
	id  int64
	gen int
}

// glareSource paints the glare as the shadow of a point, grown by Core
// and blurred by Sigma, would be: the blur of a square Core from Center
// each way. It rounds the cover's corners itself, where a clip would be
// rasterized anew each frame the cover tilts in depth.
const glareSource = `package main

var Center vec2
var Core float
var Sigma float
var Alpha float

// erf approximates the error function, within 0.0002.
func erf(x float) float {
	x2 := x * x
	return sign(x) * sqrt(1.0-exp(-x2*(1.2732395+0.147*x2)/(1.0+0.147*x2)))
}

// spread is how much of a line of length 2*Core around 0, blurred, is at d.
func spread(d float) float {
	k := 0.70710678 / Sigma
	return 0.5 * (erf((d+Core)*k) - erf((d-Core)*k))
}

func Fragment(dst vec4, pos vec2) vec4 {
	d := pos - Center
	a := Alpha * spread(d.x) * spread(d.y)
	return vec4(a, a, a, a)
}
`

var glareShader = sync.OnceValue(func() *shader.Shader { return shader.MustCompile(glareSource) })

// coverSwing is how the cover goes to rest, or off.
var coverSwing = ui.Animation{Spring: ui.SpringOptions{Duration: 480 * time.Millisecond, Bounce: 0.18}}

// coverSlide is how a cover slides in, or off, as the song changes.
var coverSlide = ui.Animation{Spring: ui.SpringOptions{Duration: 520 * time.Millisecond, Bounce: 0.12}}

// swipeRest is how long a swipe is still before it is let go, for a
// trackpad that sends no momentum after it.
const swipeRest = 140 * time.Millisecond

// npCover shows the cover of tr, size across, which the pointer tilts,
// drags and flings.
func (a *app) npCover(c *ui.Context, tr *netease.Track, art *artwork, size float32) {
	cs, pl := &a.npCoverState, a.pl
	far := size * 1.5 // where a cover flies off to
	fling := func(dir int) {
		cs.flung = dir
		cs.off = ui.Point{X: -float32(dir) * far}
	}
	wrap := ui.Box(c.Key(coverKey{tr.ID, cs.gen})).Size(size, size).AlignSelf(ui.Center).Label("专辑封面")
	entering := cs.enter != 0
	if entering {
		// A new cover's first frame: it shows where it comes in from.
		cs.off, cs.enter = ui.Point{X: cs.enter}, 0
	}
	// Another song, not flung to, comes in from the side it is on in the
	// play order, and the cover before goes off the other way.
	var enter *ui.Motion
	if order, at := pl.PlayOrder(); cs.track != tr.ID {
		if cs.exit != nil && !entering {
			dir := float32(stepDir(cs.at, at, len(order)))
			// Its copy leaves from this frame on, which first knows the way.
			cs.exit.X = -dir * size * 0.5
			enter = &ui.Motion{X: dir * size * 0.5, Scale: 0.92}
		}
		cs.track, cs.at, cs.exit = tr.ID, at, &ui.Motion{Scale: 0.92}
	}
	exit := cs.exit
	if cs.flung != 0 {
		// Flown off already.
		exit = nil
	}
	wrap.Transition(ui.ElementTransition{Animation: &coverSlide, Colors: true, Enter: enter, Exit: exit})
	wrap.Draggable(&cs.off, ui.DraggableOptions{
		Axis: ui.Horizontal,
		Snap: func(p ui.Point) ui.Point {
			switch {
			case p.X < -size*0.4:
				fling(1)
			case p.X > size*0.4:
				fling(-1)
			default:
				return ui.Point{}
			}
			return cs.off
		},
		Animation: coverSwing,
	})
	if entering {
		cs.off = ui.Point{}
		c.AnimationFrame()
	}
	// Two fingers swiping across the trackpad move it as a drag does.
	wrap.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputScroll || !ev.Precise || cs.flung != 0 {
			return false
		}
		if ev.Momentum {
			if cs.swipe != 0 {
				a.letSwipeGo(size, fling)
			}
			return cs.decided
		}
		if cs.swipe == 0 && math.Abs(float64(ev.DX)) <= math.Abs(float64(ev.DY)) {
			return false
		}
		cs.decided = false
		cs.swipe -= ev.DX
		cs.swipeAt = ev.Time
		cs.off = ui.Point{X: rubber(cs.swipe, size)}
		return true
	})
	if cs.swipe != 0 {
		if c.Now().Sub(cs.swipeAt) >= swipeRest {
			a.letSwipeGo(size, fling)
		} else {
			c.After(swipeRest)
		}
	}

	at := wrap.DragOffset()
	if cs.flung != 0 && math.Abs(float64(at.X)) >= float64(far)*0.97 {
		// Gone: the next cover comes in from the other side.
		dir := cs.flung
		cs.flung, cs.gen, cs.enter = 0, cs.gen+1, float32(dir)*far
		if dir > 0 {
			pl.Next()
		} else {
			pl.Prev()
		}
	}

	// It tilts toward the pointer, and swings as it is dragged.
	px, py, over := wrap.PointerPosition()
	tx, ty := float32(0), float32(0)
	if over && !wrap.Dragging() {
		tx = clamp1(px/size*2 - 1)
		ty = clamp1(py/size*2 - 1)
	}
	swing := clamp1(at.X/size) * 16
	if c.Preferences().ReduceMotion {
		tx, ty, swing = 0, 0, 0
	}
	tilt := ui.Animation{Spring: ui.SpringOptions{Duration: 500 * time.Millisecond, Bounce: 0.2}}
	ry := wrap.AnimateTo("ry", tx*7+swing, tilt)
	rx := wrap.AnimateTo("rx", -ty*7, tilt)
	shine := float32(0)
	if over {
		shine = 1
	}
	shine = wrap.AnimateTo("shine", shine, ui.Animation{Duration: 300 * time.Millisecond})
	gx := wrap.AnimateTo("gx", px, tilt)
	gy := wrap.AnimateTo("gy", py, tilt)
	if over || wrap.Dragging() {
		wrap.Cursor(ui.CursorGrab)
	}
	// Flying off, it fades.
	if f := float32(math.Abs(float64(at.X))) / far; f > 0.01 {
		wrap.Opacity(1 - 0.85*min(f, 1))
	}

	wrap.Children(func() {
		// The cover breathes: smaller while paused, a little nearer under
		// the pointer.
		scale := float32(1)
		if !pl.Playing() {
			scale = 0.88
		}
		if over {
			scale *= 1.02
		}
		box := ui.Box(c).Fill().Radius(16).Shadow(0, 30, 60, -12, ui.RGBA(0, 0, 0, 0.55))
		if math.Abs(float64(rx))+math.Abs(float64(ry)) > 0.05 {
			// In depth only while tilted: flat, it draws as an image does.
			box.RotateX(rx).RotateY(ry).Perspective(900)
		}
		box.AnimatePose(ui.Pose{Scale: scale}, ui.Animation{Spring: ui.SpringOptions{Duration: 600 * time.Millisecond, Bounce: 0.25}})
		in := artIn(box, bitmap(art))
		if in < 1 {
			box.Background(white.Alpha(0.08))
		}
		box.Children(func() {
			artImage(c, bitmap(art), 16, in)
			if shine > 0.01 {
				ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
					glareShader().Draw(p, r, shader.Options{Radius: 16, Uniforms: map[string]any{
						"Center": [2]float32{gx, gy}, "Core": r.W * 0.16, "Sigma": r.W * 0.225, "Alpha": 0.26 * shine,
					}})
				})
			}
		})
	})
}

// letSwipeGo ends a trackpad's swipe: flung if it went far enough,
// springing back if not.
func (a *app) letSwipeGo(size float32, fling func(int)) {
	cs := &a.npCoverState
	switch {
	case cs.swipe < -size*0.3:
		fling(1)
	case cs.swipe > size*0.3:
		fling(-1)
	default:
		cs.off = ui.Point{}
	}
	cs.swipe, cs.decided = 0, true
}

// stepDir tells which way the play order went from from to to, of n
// songs: 1 on, -1 back, around its end as Next and Prev go.
func stepDir(from, to, n int) int {
	switch {
	case n > 2 && from == n-1 && to == 0:
		return 1
	case n > 2 && from == 0 && to == n-1:
		return -1
	case to < from:
		return -1
	}
	return 1
}

// rubber holds a swipe's travel back past where it would fling, as a
// drag past an edge.
func rubber(d, size float32) float32 {
	return ui.Rubberband(d, -size*0.5, size*0.5, size)
}

func clamp1(v float32) float32 { return max(-1, min(v, 1)) }
