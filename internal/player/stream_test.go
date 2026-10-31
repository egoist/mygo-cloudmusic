package player

import (
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

func TestFrameSize(t *testing.T) {
	// MPEG-1 layer III, 320 kbps, 44.1 kHz, no padding.
	if n := frameSize([]byte{0xff, 0xfb, 0xe0, 0x44}); n != 1044 {
		t.Errorf("frame size %d", n)
	}
	if n := frameSize([]byte{0xff, 0x00, 0xe0, 0x44}); n != 0 {
		t.Errorf("no frame: %d", n)
	}
}

// decodeAll decodes the whole of a song from its start.
func decodeAll(t *testing.T, b []byte) []float32 {
	t.Helper()
	s, err := openStream(downloaded(b), 600000)
	if err != nil {
		t.Fatal(err)
	}
	var out []float32
	buf := make([]float32, 4096)
	for {
		n, err := s.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil || n == 0 {
			return out
		}
	}
}

// maxDiff returns how far apart a and b are at most, over the length of
// the shorter.
func maxDiff(a, b []float32) float64 {
	d := 0.0
	for i := range min(len(a), len(b)) {
		d = math.Max(d, math.Abs(float64(a[i]-b[i])))
	}
	return d
}

// A seek into the middle of a 128 kbps MP3, whose frames borrow bits from
// two frames back, plays the music there, not a burst of noise.
func TestMP3SeekIsClean(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	ref := decodeAll(t, b)
	dl := downloaded(b)
	s, err := openMP3Stream(dl, int64(len(ref)/2)*1000/44100)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]float32, 4096)
	for i := 1; i < 40; i++ {
		frame := s.frames * int64(i) / 41
		if err := s.SetPosition(frame); err != nil {
			t.Fatal(err)
		}
		n, _ := s.Read(buf)
		at := frame * 2
		if d := maxDiff(buf[:n], ref[at:]); d > 1e-4 {
			t.Errorf("seek to frame %d: off by up to %.3f", frame, d)
		}
	}
}

// A FLAC plays as its samples are, and a seek lands on the very sample.
func TestFLACSeek(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.flac")
	if err != nil {
		t.Fatal(err)
	}
	ref := decodeAll(t, b)
	if len(ref) != 5*44100*2 {
		t.Fatalf("decoded %d samples", len(ref))
	}
	s, err := openStream(downloaded(b), 5000)
	if err != nil {
		t.Fatal(err)
	}
	if s.SampleRate() != 44100 || s.Len() != 5*44100 {
		t.Fatalf("rate %d, length %d", s.SampleRate(), s.Len())
	}
	buf := make([]float32, 4096)
	for _, frame := range []int64{0, 1, 4095, 4096, 50000, 123457, 200000} {
		if err := s.SetPosition(frame); err != nil {
			t.Fatal(err)
		}
		n, err := s.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		if d := maxDiff(buf[:n], ref[frame*2:]); d != 0 {
			t.Errorf("seek to %d: off by up to %.4f", frame, d)
		}
	}
}

// A FLAC whose metadata block is of a length with bit 2 set plays from its
// first frame. The block's length was ORed into the 4 bytes of its header
// rather than added to them, so most of NetEase's lossless songs started 4
// bytes early, failed to decode and were skipped.
func TestFLACMetadataLength(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.flac")
	if err != nil {
		t.Fatal(err)
	}
	ref := decodeAll(t, b)
	start, err := flacDataStart(downloaded(b))
	if err != nil {
		t.Fatal(err)
	}
	// The last block's header is no longer the last: a padding block of 6
	// bytes follows it.
	off := int64(4)
	for b[off]&0x80 == 0 {
		off += 4 + (int64(b[off+1])<<16 | int64(b[off+2])<<8 | int64(b[off+3]))
	}
	padded := append([]byte(nil), b[:start]...)
	padded[off] &^= 0x80
	padded = append(padded, 0x81, 0, 0, 6, 0, 0, 0, 0, 0, 0)
	padded = append(padded, b[start:]...)

	s, err := openStream(downloaded(padded), 5000)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]float32, 4096)
	n, err := s.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if d := maxDiff(buf[:n], ref); d != 0 {
		t.Errorf("off by up to %.4f", d)
	}
}

// A song plays as it streams into a file of the cache, rather than the
// app's memory, and the cache keeps it once it has all come in; one cut
// short leaves no file behind.
func TestStreamIntoCache(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	half := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cut" {
			w.Header().Set("Content-Length", strconv.Itoa(len(b)))
			w.Write(b[:len(b)/2])
			w.(http.Flusher).Flush()
			<-half
			return
		}
		w.Write(b)
	}))
	defer srv.Close()
	defer close(half)
	dir := t.TempDir()
	c := NewCache(dir, 1<<30)
	ref := decodeAll(t, b)

	st := &netease.Stream{URL: srv.URL + "/whole", Size: int64(len(b)), Level: "exhigh", Type: "mp3"}
	s, err := openStream(c.download(1, st), int64(len(ref)/2)*1000/44100)
	if err != nil {
		t.Fatal(err)
	}
	var got []float32
	buf := make([]float32, 4096)
	for {
		n, err := s.Read(buf)
		got = append(got, buf[:n]...)
		if err != nil || n == 0 {
			break
		}
	}
	s.Close()
	if d := maxDiff(got, ref); len(got) != len(ref) || d != 0 {
		t.Errorf("played %d of %d samples, off by up to %.4f", len(got), len(ref), d)
	}
	if dl, _, ok := c.lookup(1, "exhigh"); !ok {
		t.Error("not kept")
	} else {
		dl.Close()
	}

	cut := c.download(2, &netease.Stream{URL: srv.URL + "/cut", Size: int64(len(b)), Level: "exhigh", Type: "mp3"})
	cut.waitFor(int64(len(b) / 2))
	cut.Close()
	for start := time.Now(); time.Since(start) < 2*time.Second; time.Sleep(10 * time.Millisecond) {
		if parts, _ := filepath.Glob(filepath.Join(dir, "*.part")); len(parts) == 0 {
			break
		}
	}
	if parts, _ := filepath.Glob(filepath.Join(dir, "*.part")); len(parts) != 0 {
		t.Errorf("left %v", parts)
	}
	if _, _, ok := c.lookup(2, "exhigh"); ok {
		t.Error("kept a song cut short")
	}
}

// This five-second LAME fixture used to include its metadata frame,
// encoder delay and tail padding, adding audible silence at each transition.
func TestMP3GaplessLengthAndRestart(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	s, err := openMP3Stream(downloaded(b), 600000)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Len() != 5*44100 {
		t.Fatalf("audible length %d, want 220500", s.Len())
	}
	ref := decodeAll(t, b)
	if len(ref) != 5*44100*2 {
		t.Fatalf("decoded %d samples, want 441000", len(ref))
	}
	buf := make([]float32, 4096)
	for _, frame := range []int64{0, 1, 220000, 0} {
		if err := s.SetPosition(frame); err != nil {
			t.Fatal(err)
		}
		n, _ := s.Read(buf)
		if n != min(len(buf), len(ref)-int(frame*2)) {
			t.Fatalf("frame %d: %d samples", frame, n)
		}
		if d := maxDiff(buf[:n], ref[frame*2:]); d > 1e-4 {
			t.Fatalf("frame %d differs by %g", frame, d)
		}
	}
}

// A second stream of the song, as transcribing an episode opens, decodes
// it as the one that plays does. Closing it leaves the download to the
// one that plays, and the one that plays closing leaves it what is in.
func TestSharedStream(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(b) }))
	defer srv.Close()
	c := NewCache(t.TempDir(), 1<<30)
	ref := decodeAll(t, b)
	ms := int64(len(ref)/2) * 1000 / 44100
	readAll := func(s stream) []float32 {
		var got []float32
		buf := make([]float32, 4096)
		for {
			n, err := s.Read(buf)
			got = append(got, buf[:n]...)
			if err != nil || n == 0 {
				return got
			}
		}
	}

	dl := c.download(1, &netease.Stream{URL: srv.URL, Size: int64(len(b)), Level: "exhigh", Type: "mp3"})
	playing, err := openStream(dl, ms)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := openShared(dl, ms)
	if err != nil {
		t.Fatal(err)
	}
	shared.Close()
	if got := readAll(playing); len(got) != len(ref) || maxDiff(got, ref) != 0 {
		t.Errorf("played %d of %d samples after the second stream closed", len(got), len(ref))
	}
	playing.Close()

	kept, _, ok := c.lookup(1, "exhigh")
	if !ok {
		t.Fatal("not kept")
	}
	playing, _ = openStream(kept, ms)
	shared, err = openShared(kept, ms)
	if err != nil {
		t.Fatal(err)
	}
	playing.Close()
	if got := readAll(shared); len(got) != len(ref) || maxDiff(got, ref) != 0 {
		t.Errorf("decoded %d of %d samples after the stream playing closed", len(got), len(ref))
	}
	shared.Close()
	if _, err := openShared(kept, ms); err == nil {
		t.Error("shared a song let go")
	}
}

// A stream whose level would lead out of the cache's folder is played but
// not kept. The level named the file, taken as the API sent it, so a level
// of "/../../x" wrote the song and its JSON beside the cache instead.
func TestCacheKeepsOnlyKnownLevels(t *testing.T) {
	b, err := os.ReadFile("testdata/seek.mp3")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(b) }))
	defer srv.Close()
	root := t.TempDir()
	dir := filepath.Join(root, "cache")
	c := NewCache(dir, 1<<30)

	dl := c.download(1, &netease.Stream{URL: srv.URL, Size: int64(len(b)), Level: "/../../pwned", Type: "mp3"})
	dl.waitFor(int64(len(b)))
	dl.Close()
	for start := time.Now(); time.Since(start) < 2*time.Second; time.Sleep(10 * time.Millisecond) {
		if parts, _ := filepath.Glob(filepath.Join(dir, "*.part")); len(parts) == 0 {
			break
		}
	}
	var outside []string
	filepath.WalkDir(root, func(path string, e os.DirEntry, err error) error {
		if err == nil && !e.IsDir() && filepath.Dir(path) != dir {
			outside = append(outside, path)
		}
		return nil
	})
	if len(outside) != 0 {
		t.Errorf("wrote %v outside the cache", outside)
	}
	if songs := c.songs(); len(songs) != 0 {
		t.Errorf("kept %v", songs)
	}
}

// A song the cache won't keep sets aside no more memory than maxPrealloc,
// whatever size the API says it is, which it set aside in full before.
func TestStreamSizeIsNotTrusted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("short")) }))
	defer srv.Close()
	dl := startDownload(srv.URL, 1<<40, nil, nil)
	defer dl.Close()
	dl.mu.Lock()
	n := cap(dl.data)
	dl.mu.Unlock()
	if n > maxPrealloc {
		t.Errorf("set aside %d bytes", n)
	}
}
