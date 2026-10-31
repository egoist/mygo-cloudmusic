package player

import (
	"errors"
	"sync"
	"testing"

	"github.com/egoist/mygo/audio"

	"netease-music-mygo/internal/netease"
)

type preparedStream struct {
	silence
	read, closed        chan struct{}
	readOnce, closeOnce sync.Once
}

func newPreparedStream() *preparedStream {
	return &preparedStream{read: make(chan struct{}), closed: make(chan struct{})}
}

func (s *preparedStream) Read(b []float32) (int, error) {
	s.readOnce.Do(func() { close(s.read) })
	return s.silence.Read(b)
}

func (s *preparedStream) Close() error {
	s.closeOnce.Do(func() { close(s.closed) })
	return nil
}

func (s *preparedStream) source() (*download, *spectrum) { return nil, nil }

type openRequest struct {
	track  netease.Track
	level  string
	stream *preparedStream
	result chan error
}

// preparationPlayer holds each open until the test supplies its result.
func preparationPlayer(t *testing.T, ids ...int64) (*Player, chan func(), chan openRequest) {
	t.Helper()
	posts := make(chan func(), 16)
	requests := make(chan openRequest, 16)
	old := openTrack
	openTrack = func(_ *Player, track netease.Track, level string) (*netease.Stream, stream, error) {
		r := openRequest{track, level, newPreparedStream(), make(chan error, 1)}
		requests <- r
		if err := <-r.result; err != nil {
			return nil, nil, err
		}
		return &netease.Stream{Level: level}, r.stream, nil
	}
	p := &Player{post: func(fn func()) { posts <- fn }, volume: 0.8, gain: 1}
	tracks := make([]netease.Track, len(ids))
	for i, id := range ids {
		tracks[i] = netease.Track{ID: id, Duration: 600000}
	}
	p.Cue(tracks)
	p.stream = newPreparedStream()
	p.out = audio.NewPlayer(p.stream)
	t.Cleanup(func() {
		openTrack = old
		p.fade.attach(nil, 1)
		if p.out != nil {
			p.out.Close()
		}
		if p.nextJob != nil {
			p.nextJob.close()
		}
		if p.loadJob != nil {
			p.loadJob.close()
		}
	})
	return p, posts, requests
}

// Switching used to open the next song only after the old one ended;
// a prepared song now decodes beforehand and is attached without another open.
func TestPreparedTrackStartsWithoutLoading(t *testing.T) {
	p, posts, requests := preparationPlayer(t, 1, 2)
	p.prepareNext()
	r := <-requests
	if r.track.ID != 2 {
		t.Fatalf("prepared %d", r.track.ID)
	}
	r.result <- nil
	next(t, posts)
	<-r.stream.read
	pcm := p.nextJob.pcm
	p.stop()
	p.at, p.track, p.loading, p.playing = 1, &p.queue[1], true, true
	p.loadAudio(*p.track, p.at, 0)
	if p.loading || p.stream != pcm.raw || !p.out.Playing() || p.nextJob != nil {
		t.Fatal("did not immediately play the prepared audio")
	}
	if len(requests) != 0 {
		t.Fatal("opened the same song again")
	}
}

// Skipping while preparation is in flight reuses it and honors a pause
// requested before the download finishes.
func TestPreparedTrackInFlightIsReused(t *testing.T) {
	p, posts, requests := preparationPlayer(t, 1, 2)
	p.prepareNext()
	r := <-requests
	job := p.nextJob
	p.stop()
	p.at, p.track, p.loading = 1, &p.queue[1], true
	p.loadAudio(*p.track, p.at, 0)
	if p.loadJob != job || !p.loading {
		t.Fatal("restarted preparation")
	}
	r.result <- nil
	next(t, posts)
	if p.loading || p.stream != job.stream || p.out.Playing() {
		t.Fatal("lost the prepared song or ignored the pause")
	}
	if len(requests) != 0 {
		t.Fatal("opened the same song again")
	}
}

// A foreground load overtaken by a skip must release its audio and never
// become the current song when its result arrives later.
func TestOvertakenPreparationCloses(t *testing.T) {
	p, posts, requests := preparationPlayer(t, 1, 2)
	p.prepareNext()
	r := <-requests
	p.stop()
	p.at, p.track = 1, &p.queue[1]
	p.loadAudio(*p.track, p.at, 0)
	p.stop()
	r.result <- nil
	next(t, posts)
	<-r.stream.closed
	if p.out != nil || p.loadJob != nil {
		t.Fatal("attached a stale song")
	}
}

// A new play order discards the old preparation, including a late result.
func TestPreparationFollowsOrder(t *testing.T) {
	p, posts, requests := preparationPlayer(t, 1, 2, 3)
	p.prepareNext()
	old := <-requests
	p.order[1], p.order[2] = p.order[2], p.order[1]
	p.prepareNext()
	current := <-requests
	if current.track.ID != 3 {
		t.Fatalf("prepared %d", current.track.ID)
	}
	old.result <- nil
	next(t, posts)
	<-old.stream.closed
	current.result <- nil
	next(t, posts)
	if p.nextJob.id != 3 || !p.nextJob.ready {
		t.Fatal("lost the new preparation")
	}
}

// Changing quality must not consume audio prepared at the previous setting.
func TestPreparationRespectsQuality(t *testing.T) {
	p, posts, requests := preparationPlayer(t, 1, 2)
	p.prepareNext()
	old := <-requests
	old.result <- nil
	next(t, posts)
	p.Quality = func() string { return "lossless" }
	p.prepareNext()
	<-old.stream.closed
	r := <-requests
	if r.level != "lossless" {
		t.Fatalf("quality %q", r.level)
	}
	r.result <- nil
	next(t, posts)
}

// Repeating prepares the appropriate song again, and leaving a radio
// drops the songs it had queued.
func TestPreparationRepeatAndRadio(t *testing.T) {
	p, posts, requests := preparationPlayer(t, 1, 2)
	p.at, p.track = 1, &p.queue[1]
	p.prepareNext()
	if p.nextJob != nil {
		t.Fatal("prepared past the queue's end")
	}
	p.CycleRepeat()
	r := <-requests
	if r.track.ID != 1 {
		t.Fatal("did not prepare the queue's first song")
	}
	r.result <- nil
	next(t, posts)
	p.CycleRepeat()
	<-r.stream.closed
	r = <-requests
	if r.track.ID != 2 {
		t.Fatal("did not prepare the repeated song")
	}
	r.result <- nil
	next(t, posts)
	p.repeat, p.at, p.track = RepeatOff, 0, &p.queue[0]
	p.AttachRadio("test", func(got func([]netease.Track)) { got(nil) })
	p.EndRadio()
	<-r.stream.closed
	if p.nextJob != nil {
		t.Fatal("kept the departed radio's next song")
	}
}

// A failed preparation leaves the current song playing and reports the
// failure only when that prepared song is actually selected.
func TestPreparationFailureWaitsForSelection(t *testing.T) {
	p, posts, requests := preparationPlayer(t, 1, 2)
	p.prepareNext()
	r := <-requests
	r.result <- errors.New("download failed")
	next(t, posts)
	if p.err != "" || p.Current().ID != 1 {
		t.Fatal("failed preparation interrupted playback")
	}
	p.stop()
	p.at, p.track, p.loading = 1, &p.queue[1], true
	// All other songs have already failed in this attempted traversal.
	p.loadAudio(*p.track, p.at, 1)
	if p.loading || p.playing || p.err != "download failed" {
		t.Fatal("did not report the selected song's error")
	}
}
