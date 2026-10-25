package player

import (
	"math"
	"sync"
	"time"

	"github.com/egoist/mygo/audio"
)

// How long pausing and resuming fade the music out and in.
const (
	fadeOut = 180 * time.Millisecond
	fadeIn  = 260 * time.Millisecond
)

// fader sets the volume of the player playing: the listener's volume and
// the track's normalizing gain, times a level that fades as playback
// pauses and resumes. It ramps the level on a goroutine of its own, since
// package audio lets any goroutine change a player, and the main thread
// would redraw the windows with each step.
type fader struct {
	mu    sync.Mutex
	out   *audio.Player
	gain  float64 // the listener's volume and the track's gain, together
	from  float64 // the level faded from at start, to to over dur
	to    float64
	start time.Time
	dur   time.Duration
	gen   int // counts fades, to stop the goroutine of one overtaken
}

// level returns the level at now, easing in and out.
func (f *fader) level(now time.Time) float64 {
	t := 1.0
	if f.dur > 0 {
		t = min(1, now.Sub(f.start).Seconds()/f.dur.Seconds())
	}
	return f.from + (f.to-f.from)*(0.5-0.5*math.Cos(math.Pi*t))
}

func (f *fader) apply(now time.Time) {
	if f.out != nil {
		f.out.SetVolume(f.gain * f.level(now))
	}
}

// attach makes out the player faded, at level.
func (f *fader) attach(out *audio.Player, level float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gen++
	f.out, f.from, f.to, f.dur = out, level, level, 0
	f.apply(time.Now())
}

// setGain changes the volume and gain, at once.
func (f *fader) setGain(g float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gain = g
	f.apply(time.Now())
}

// silent reports whether the level is at nothing, faded out.
func (f *fader) silent() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.level(time.Now()) == 0
}

// fade ramps the level from where it is to to, over d at most, then calls
// done on its goroutine, unless another fade overtook it.
func (f *fader) fade(to float64, d time.Duration, done func()) {
	f.mu.Lock()
	now := time.Now()
	from := f.level(now)
	f.gen++
	gen := f.gen
	// Part of the way already: the rest of the time.
	if span := math.Abs(to - from); span < 1 {
		d = time.Duration(float64(d) * span)
	}
	f.from, f.to, f.start, f.dur = from, to, now, d
	f.mu.Unlock()
	go func() {
		tick := time.NewTicker(8 * time.Millisecond)
		defer tick.Stop()
		for {
			f.mu.Lock()
			if f.gen != gen {
				f.mu.Unlock()
				return
			}
			now := time.Now()
			f.apply(now)
			finished := now.Sub(f.start) >= f.dur
			f.mu.Unlock()
			if finished {
				if done != nil {
					done()
				}
				return
			}
			<-tick.C
		}
	}()
}
