package qqmusic

import (
	"os"
	"testing"
)

// QQ's public song detail must return usable measurements without account credentials.
func TestLiveSongLoudness(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to contact QQ Music")
	}
	isolateLoudnessCache(t)
	previous := sessionSnapshot()
	t.Cleanup(func() { setSession(previous) })
	setSession(Session{})
	volume, err := SongLoudness(Track{MID: "0039MnYb0qxYhV"})
	if err != nil || volume.Peak <= 0 || volume.Gain == 0 {
		t.Fatalf("missing loudness measurement: %+v, error=%v", volume, err)
	}
	t.Logf("gain=%g dB peak=%g lra=%g LU", volume.Gain, volume.Peak, volume.LRA)
}
