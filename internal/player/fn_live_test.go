package player

import (
	"context"
	"os"
	"testing"
	"time"

	"netease-music-mygo/internal/fnmusic"
	"netease-music-mygo/internal/netease"
)

// liveFn is a client of the Feiniu service FN_ADDR names, as FN_USER with
// FN_PASS; without them the tests that play from one skip.
func liveFn(t *testing.T) *fnmusic.Client {
	addr, user, pass := os.Getenv("FN_ADDR"), os.Getenv("FN_USER"), os.Getenv("FN_PASS")
	if addr == "" || user == "" {
		t.Skip("FN_ADDR, FN_USER and FN_PASS name a service to play from")
	}
	c, err := fnmusic.New(fnmusic.Config{Address: addr, Username: user, PassHash: fnmusic.HashPassword(pass)})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// firstOf returns the first song of the library that is of format, and
// whether transcoded is what the service has to do to play it.
func firstOf(t *testing.T, c *fnmusic.Client, want func(*netease.Track) bool) netease.Track {
	for pg := 1; ; pg++ {
		tracks, total, err := c.LibraryPage(fnmusic.SortRecent, pg, 500)
		if err != nil {
			t.Fatal(err)
		}
		for _, tr := range tracks {
			if want(&tr) {
				return tr
			}
		}
		if pg*500 >= total {
			t.Skip("no such song in the library")
		}
	}
}

// decode reads everything the stream decodes, and returns how long it was.
func decode(t *testing.T, s stream) time.Duration {
	buf := make([]float32, 8192)
	var frames int64
	for {
		n, err := s.Read(buf)
		frames += int64(n / s.Channels())
		if err != nil || n == 0 {
			break
		}
	}
	return time.Duration(frames) * time.Second / time.Duration(s.SampleRate())
}

// A song the player cannot decode as it is (AAC) plays through the
// service's transcoding: it decodes to the end, as long as the song, and a
// seek into its middle is quick, as it asks for the piece it lands in.
func TestFnTranscodedPlays(t *testing.T) {
	c := liveFn(t)
	tr := firstOf(t, c, func(tr *netease.Track) bool { return fnmusic.Transcoded(tr) })
	start := time.Now()
	h, err := c.TranscodeHLS(context.Background(), &tr)
	if err != nil {
		t.Fatal(err)
	}
	s, err := openHLSStream(h)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	t.Logf("%q (%s): %d pieces, %v, opened in %v", tr.Name, tr.Format, len(h.Segments), h.Total, time.Since(start).Round(time.Millisecond))

	// A seek to the middle, before anything else is read.
	seek := time.Now()
	if err := s.SetPosition(s.frames / 2); err != nil {
		t.Fatal(err)
	}
	buf := make([]float32, 4096)
	if n, err := s.Read(buf); n == 0 || err != nil {
		t.Fatalf("read after the seek: %d %v", n, err)
	}
	took := time.Since(seek)
	t.Logf("seek to the middle: first sound after %v", took.Round(time.Millisecond))
	if took > 3*time.Second {
		t.Errorf("a seek took %v", took)
	}

	s.SetPosition(0)
	got := decode(t, s)
	t.Logf("decoded %v of %v in %v", got.Round(time.Millisecond), tr.Length(), time.Since(start).Round(time.Millisecond))
	if d := got - tr.Length(); d < -2*time.Second || d > 2*time.Second {
		t.Errorf("decoded %v of a song of %v", got, tr.Length())
	}
}

// A FLAC of the service streams as it is, and decodes as long as the song.
func TestFnFLACPlays(t *testing.T) {
	c := liveFn(t)
	tr := firstOf(t, c, func(tr *netease.Track) bool { return tr.Format == "flac" && !tr.Cue })
	st, err := c.Stream(&tr)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	dl := startDownloadWith(st.URL, st.Header, st.Size, nil, nil)
	s, err := openStream(dl, tr.Duration)
	if err != nil {
		t.Fatal(err)
	}
	got := decode(t, s)
	s.Close()
	t.Logf("%q: %d bytes, decoded %v of %v in %v", tr.Name, st.Size, got.Round(time.Millisecond), tr.Length(), time.Since(start).Round(time.Millisecond))
	if d := got - tr.Length(); d < -time.Second || d > time.Second {
		t.Errorf("decoded %v of a song of %v", got, tr.Length())
	}
}
