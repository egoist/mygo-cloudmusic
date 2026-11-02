package player

import (
	"errors"
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

// Reset drops the current song and queue so a platform switch cannot retain playback.
func TestResetDropsPlaybackContext(t *testing.T) {
	p := &Player{at: -1, shuffle: true, repeat: RepeatOne}
	p.Cue([]netease.Track{{ID: 1, Name: "一"}, {ID: 2, Name: "二"}})
	p.radio = "radio"
	p.feed = func(func([]netease.Track)) {}
	p.playing = true
	p.Reset()
	if p.Current() != nil || len(p.Queue()) != 0 || p.Radio() != "" || p.Playing() {
		t.Fatalf("reset retained playback: current=%v queue=%d radio=%q playing=%v", p.Current(), len(p.Queue()), p.Radio(), p.Playing())
	}
	if p.Shuffle() || p.Repeat() != RepeatOff {
		t.Fatalf("reset retained playback modes: shuffle=%v repeat=%v", p.Shuffle(), p.Repeat())
	}
}

// Appending a queue preserves the current songs and ignores duplicate IDs.
func TestAppendQueue(t *testing.T) {
	p := &Player{at: 0, queue: []netease.Track{{ID: 1}}, order: []int{0}, track: &netease.Track{ID: 1}}
	p.AppendQueue([]netease.Track{{ID: 1}, {ID: 2}})
	if got := p.Queue(); len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Fatalf("queue = %+v", got)
	}
	if len(p.order) != 2 || p.order[1] != 1 {
		t.Fatalf("order = %+v", p.order)
	}
}

// Songs that cannot play stop the player after a few in a row, instead of
// flying through the whole queue: a NAS gone away failed every song, which
// skipped one after another.
func TestFailedLoadsStopAfterAFew(t *testing.T) {
	ch := make(chan func(), 64)
	p := New(func(fn func()) { ch <- fn })
	tried := 0
	p.ResolveStream = func(*netease.Track, string) (*netease.Stream, error) {
		tried++
		return nil, errors.New("gone")
	}
	queue := make([]netease.Track, 10)
	for i := range queue {
		queue[i] = netease.Track{ID: int64(i + 1), Name: "song", Duration: 1000}
	}
	p.PlayQueue(queue, 0)
	for p.Loading() {
		select {
		case fn := <-ch:
			fn()
		case <-time.After(2 * time.Second):
			t.Fatal("still loading")
		}
	}
	if tried != maxSkipped || p.Playing() || p.Err() == "" {
		t.Fatalf("tried %d songs, playing=%v err=%q", tried, p.Playing(), p.Err())
	}
}
