package player

import (
	"math"
	"os"
	"testing"
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
		// Where the decoder started, counted in MP3 frames from the first,
		// and what it played from there on.
		var at int64
		if from := frame - preroll; from > 0 {
			off := s.start + int64(float64(dl.size-s.start)*float64(from)/float64(s.frames))
			off = findFrame(dl, off)
			k := int64(0)
			for p := s.start; p < off; p += int64(frameSize(dl.data[p:])) {
				k++
			}
			at = (k*1152 + preroll) * 2
		} else {
			at = frame * 2
		}
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
