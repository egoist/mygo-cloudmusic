package player

import (
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
	"netease-music-mygo/internal/netease"
)

// A decoded transition must preserve the output and pause state, and only
// change song metadata and relative position when the boundary is heard.
func TestAudibleTransitionKeepsOutput(t *testing.T) {
	oldArt, oldLyrics := fetchArtwork, fetchLyrics
	fetchArtwork = func(string) ([]byte, error) { return nil, errors.New("no artwork") }
	fetchLyrics = func(int64) (string, string, string, error) { return "", "", "", nil }
	defer func() { fetchArtwork, fetchLyrics = oldArt, oldLyrics }()
	a, b := testPCM(t, 48000, 48000, .25), testPCM(t, 48000, 48000, .5)
	a.track = netease.Track{ID: 1, Duration: 1000}
	b.track = netease.Track{ID: 2, Duration: 1000}
	seq := newSequence(a, nil)
	defer seq.Close()
	seq.queue(b)
	seq.Read(make([]float32, 50000*2))
	// A separate, paused audio clock lets the test place the heard position
	// exactly, without depending on the device or wall clock.
	out := audio.NewSound(48000, 2, make([]float32, 96000*2)).NewPlayer()
	defer out.Close()
	posts := make(chan func(), 4)
	p := &Player{post: func(fn func()) { posts <- fn }, out: out, sequence: seq, stream: a.raw}
	p.Cue([]netease.Track{a.track, b.track})
	out.Seek(900 * time.Millisecond)
	p.pollAudio(seq)
	if p.Current().ID != 1 {
		t.Fatal("metadata changed before boundary")
	}
	out.Seek(1010 * time.Millisecond)
	p.pollAudio(seq)
	next(t, posts) // lyrics completion
	if p.Current().ID != 2 || p.at != 1 || p.out != out || p.playing || out.Playing() {
		t.Fatal("transition replaced output, lost queue position or resumed playback")
	}
	if p.Position() != 10*time.Millisecond {
		t.Fatalf("relative position %v", p.Position())
	}
}
