package player

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"

	"netease-music-mygo/internal/netease"
)

func TestPlayEmptyQueue(t *testing.T) {
	p := &Player{at: -1}
	p.PlayQueue(nil, 0) // an empty playlist's play button
	if p.Current() != nil {
		t.Error("playing from an empty queue")
	}
}

// silence is a long song of nothing.
type silence struct{}

func (silence) SampleRate() int               { return 48000 }
func (silence) Channels() int                 { return 2 }
func (silence) Len() int64                    { return 48000 * 600 }
func (silence) SetPosition(int64) error       { return nil }
func (silence) Read(b []float32) (int, error) { clear(b); return len(b), nil }

// posted collects what the player posts to the main thread, for the test
// to run there.
func posted() (chan func(), *Player) {
	ch := make(chan func(), 16)
	p := New(func(fn func()) { ch <- fn })
	p.Cue([]netease.Track{{ID: 1, Name: "一", Duration: 600000}})
	out := audio.NewPlayer(silence{})
	out.SetVolume(0)
	out.Play()
	p.Attach(out)
	p.fade.attach(out, 1)
	return ch, p
}

func next(t *testing.T, ch chan func()) {
	t.Helper()
	select {
	case fn := <-ch:
		fn()
	case <-time.After(2 * time.Second):
		t.Fatal("nothing posted")
	}
}

// Pausing fades out before the music stops; playing again fades it in.
func TestPauseFades(t *testing.T) {
	ch, p := posted()
	defer p.out.Close()
	p.TogglePlay()
	if p.Playing() || !p.out.Playing() {
		t.Fatal("paused at once, or not at all")
	}
	next(t, ch)
	if p.out.Playing() {
		t.Fatal("still playing after fading out")
	}
	if !p.fade.silent() {
		t.Error("faded out to something")
	}
	p.TogglePlay()
	if !p.Playing() || !p.out.Playing() {
		t.Fatal("did not play again")
	}
	time.Sleep(fadeIn + 50*time.Millisecond)
	p.fade.mu.Lock()
	level := p.fade.level(time.Now())
	p.fade.mu.Unlock()
	if level != 1 {
		t.Errorf("faded in to %v", level)
	}
}

// A seek playing pauses, which fades out over a few milliseconds, and
// plays again once there, its position the one asked for meanwhile.
func TestSeekWithoutClick(t *testing.T) {
	ch, p := posted()
	defer p.out.Close()
	p.Seek(90 * time.Second)
	if p.out.Playing() {
		t.Error("jumped while playing")
	}
	if p.Position() != 90*time.Second {
		t.Errorf("position %v while seeking", p.Position())
	}
	next(t, ch)
	if !p.out.Playing() || p.out.Position() < 90*time.Second {
		t.Errorf("after the seek: playing %v at %v", p.out.Playing(), p.out.Position())
	}
}

// Seeks one after the other, as the progress bar asks when let go, play
// on from the last.
func TestSeekTwicePlaysOn(t *testing.T) {
	ch, p := posted()
	defer p.out.Close()
	p.Seek(90 * time.Second)
	p.Seek(95 * time.Second)
	next(t, ch)
	for len(ch) > 0 {
		next(t, ch)
	}
	time.Sleep(seekGap * 2)
	for len(ch) > 0 {
		next(t, ch)
	}
	if !p.out.Playing() || p.out.Position() < 95*time.Second {
		t.Errorf("after two seeks: playing %v at %v", p.out.Playing(), p.out.Position())
	}
}

// A radio asks for more as its queue nears its end, and adds what is not
// in it already.
func TestRadioRefills(t *testing.T) {
	p := &Player{at: -1}
	p.Cue([]netease.Track{{ID: 1}, {ID: 2}})
	asked := 0
	p.AttachRadio("fm:DEFAULT", func(got func([]netease.Track)) {
		asked++
		got([]netease.Track{{ID: 2}, {ID: 3}})
	})
	if asked != 1 || len(p.Queue()) != 3 || p.Queue()[2].ID != 3 {
		t.Fatalf("asked %d, queue %v", asked, p.Queue())
	}
	if p.Radio() != "fm:DEFAULT" {
		t.Errorf("radio %q", p.Radio())
	}
	// Playing something else ends the radio.
	p.endRadio()
	if p.Radio() != "" || p.feed != nil {
		t.Error("radio still on")
	}
}

// A list too long to have been loaded goes on with the pages its feed
// gives, and is no radio: it has no name, and once the feed has nothing
// more it is let go of, the queue ending where the list does.
func TestListContinues(t *testing.T) {
	p := &Player{at: -1}
	p.Cue([]netease.Track{{ID: 1}, {ID: 2}})
	pages := [][]netease.Track{{{ID: 3}, {ID: 4}}, nil}
	asked := 0
	p.feed = func(got func([]netease.Track)) {
		asked++
		got(pages[0])
		pages = pages[1:]
	}
	p.refill()
	if asked != 1 || len(p.Queue()) != 4 || p.Radio() != "" {
		t.Fatalf("asked %d, queue %v, radio %q", asked, p.Queue(), p.Radio())
	}
	// Near its end again: the list has no more.
	p.at = 3
	p.refill()
	if asked != 2 || p.feed != nil || len(p.Queue()) != 4 {
		t.Errorf("asked %d, feed set %v, queue %v", asked, p.feed != nil, p.Queue())
	}
}

func TestNormGain(t *testing.T) {
	for _, c := range []struct {
		st   *netease.Stream
		want float64
	}{
		{nil, 1},
		{&netease.Stream{}, 1},
		{&netease.Stream{Gain: -6.6, Peak: 1}, math.Pow(10, -2.6/20)},
		// Quiet, it is lifted, but not past its peak.
		{&netease.Stream{Gain: 3, Peak: 0.8}, 1.25},
		{&netease.Stream{Gain: -4, Peak: 0}, 1},
	} {
		if g := normGain(c.st); math.Abs(g-c.want) > 1e-9 {
			t.Errorf("%+v: %v, want %v", c.st, g, c.want)
		}
	}
}

// The cache gives back what it kept, at the level asked for or a better
// one, and drops what was played longest ago once over its limit.
func TestCache(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir, 400)
	store := func(id int64, st *netease.Stream, n int) {
		f := c.part(st)
		f.Write(make([]byte, n))
		f.Close()
		c.keep(id, st, f.Name(), int64(n))
	}
	store(1, &netease.Stream{Level: "lossless", Type: "flac", Gain: -3}, 100)
	dl, st, ok := c.lookup(1, "exhigh")
	if !ok || dl.size != 100 || st.Type != "flac" || st.Gain != -3 {
		t.Fatalf("lookup: %v %+v", ok, st)
	}
	dl.Close()
	if _, _, ok := c.lookup(1, "hires"); ok {
		t.Error("found a lossless song for Hi-Res")
	}
	old := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(dir, "1-lossless.flac"), old, old)
	store(2, &netease.Stream{Level: "exhigh", Type: "mp3"}, 100)
	store(3, &netease.Stream{Level: "exhigh", Type: "mp3"}, 100)
	store(4, &netease.Stream{Level: "exhigh", Type: "mp3"}, 100)
	store(5, &netease.Stream{Level: "exhigh", Type: "mp3"}, 100)
	if _, _, ok := c.lookup(1, "standard"); ok {
		t.Error("kept the song played longest ago")
	}
	if dl, _, ok := c.lookup(5, "standard"); !ok {
		t.Error("dropped the song just kept")
	} else {
		dl.Close()
	}
	if n := c.Size(); n > 400 {
		t.Errorf("takes %d of 400", n)
	}
	c.Clear()
	if n := c.Size(); n != 0 {
		t.Errorf("cleared to %d", n)
	}
}

// Leaving a radio keeps the song playing and those before it, drops those
// it queued after, and asks it for no more.
func TestEndRadio(t *testing.T) {
	p := &Player{at: -1}
	p.Cue([]netease.Track{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}})
	p.at, p.track = 1, &p.queue[1]
	asked := 0
	p.AttachRadio("heart", func(got func([]netease.Track)) { asked++ })
	p.EndRadio()
	if p.Radio() != "" || p.Current().ID != 2 {
		t.Fatalf("radio %q, current %v", p.Radio(), p.Current())
	}
	if q := p.Queue(); len(q) != 2 || q[0].ID != 1 || q[1].ID != 2 {
		t.Fatalf("queue %v", q)
	}
	p.refill()
	if asked != 0 {
		t.Error("asked the radio after leaving it")
	}
}

// Where a queue read as it is played comes from goes in the session, for
// the app to go on reading it after a restart; and playing something that
// has no such origin takes it away, as it would take a stale one to the
// wrong list.
func TestOriginIsKeptInTheSession(t *testing.T) {
	p := &Player{at: -1}
	p.Cue([]netease.Track{{ID: 1}, {ID: 2}})
	p.origin = func() string { return `{"T":"album"}` }
	s, ok := p.Session()
	if !ok || s.Origin != `{"T":"album"}` || !p.HasOrigin() {
		t.Fatalf("session %+v, has origin %v", s, p.HasOrigin())
	}
	p.endRadio()
	p.origin = nil
	if s, _ := p.Session(); s.Origin != "" || p.HasOrigin() {
		t.Errorf("origin %q kept", s.Origin)
	}
}

// Shuffle switched on or off over a queue with an origin is the app's to
// handle, which makes the queue over from the song playing; without an
// origin the queue is shuffled as ever.
func TestShuffleHook(t *testing.T) {
	p := &Player{at: -1}
	p.Cue([]netease.Track{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5}, {ID: 6}})
	told := 0
	p.OnShuffle = func(on bool) bool { told++; return true }
	p.ToggleShuffle() // no origin: the player's own
	if told != 0 || !p.Shuffle() {
		t.Fatalf("told %d without an origin, shuffle %v", told, p.Shuffle())
	}
	p.ToggleShuffle()
	p.origin = func() string { return "x" }
	before := append([]int(nil), p.order...)
	p.ToggleShuffle()
	if told != 1 || !p.Shuffle() {
		t.Fatalf("told %d, shuffle %v", told, p.Shuffle())
	}
	for i, v := range before {
		if p.order[i] != v {
			t.Fatal("the player shuffled a queue the app handles")
		}
	}
}

// A queue read from the song playing on has that song alone, then what the
// list gives; it is no radio.
func TestListFrom(t *testing.T) {
	p := &Player{at: -1}
	p.Cue([]netease.Track{{ID: 1}, {ID: 2}, {ID: 3}})
	p.at, p.track = 2, &p.queue[2]
	p.ListFrom(func(got func([]netease.Track)) { got([]netease.Track{{ID: 7}, {ID: 3}, {ID: 8}}) }, func() string { return "o" })
	q := p.Queue()
	if len(q) != 3 || q[0].ID != 3 || q[1].ID != 7 || q[2].ID != 8 || p.Radio() != "" || !p.HasOrigin() {
		t.Errorf("queue %v, radio %q", q, p.Radio())
	}
}
