package player

import (
	"testing"

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
