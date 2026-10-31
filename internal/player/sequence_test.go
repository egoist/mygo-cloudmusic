package player

import (
	"io"
	"math"
	"testing"

	"netease-music-mygo/internal/netease"
)

type samplesStream struct {
	samples []float32
	rate    int
	pos     int64
}

func (s *samplesStream) SampleRate() int           { return s.rate }
func (*samplesStream) Channels() int               { return 2 }
func (s *samplesStream) Len() int64                { return int64(len(s.samples) / 2) }
func (s *samplesStream) SetPosition(f int64) error { s.pos = f; return nil }
func (s *samplesStream) Read(b []float32) (int, error) {
	if s.pos >= s.Len() {
		return 0, io.EOF
	}
	n := copy(b, s.samples[s.pos*2:])
	s.pos += int64(n / 2)
	if s.pos == s.Len() {
		return n, io.EOF
	}
	return n, nil
}
func (*samplesStream) Close() error                   { return nil }
func (*samplesStream) source() (*download, *spectrum) { return nil, nil }

func testPCM(t *testing.T, rate, frames int, value float32) *pcmTrack {
	t.Helper()
	b := make([]float32, frames*2)
	for i := range b {
		b[i] = value
	}
	p, err := preparePCM(&samplesStream{samples: b, rate: rate}, netease.Track{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.release)
	return p
}

// End callbacks previously left silence between tracks; a single read
// must now contain the exact tail and head, without losing or adding samples.
func TestSequenceJoinsWithinRead(t *testing.T) {
	for _, frames := range []int{1, 37, 4096, 5000} {
		a, b := testPCM(t, 48000, frames, .25), testPCM(t, 48000, 53, .5)
		calls := 0
		s := newSequence(a, func() { calls++ })
		s.queue(b)
		got := make([]float32, (frames+53+100)*2)
		n, err := s.Read(got)
		if n != (frames+53)*2 || err != io.EOF || calls != 1 {
			t.Fatalf("frames %d: n=%d err=%v switches=%d", frames, n, err, calls)
		}
		for i, v := range got[:n] {
			want := float32(.25)
			if i >= frames*2 {
				want = .5
			}
			if v != want {
				t.Fatalf("sample %d = %v, want %v", i, v, want)
			}
		}
		s.Close()
	}
}

// Songs with different sample rates retain their duration and fill the
// transition even when the main thread has not processed its notification.
func TestSequenceDifferentRates(t *testing.T) {
	a, b := testPCM(t, 44100, 4410, .25), testPCM(t, 96000, 9600, .5)
	s := newSequence(a, func() { /* deliberately do not update the UI */ })
	defer s.Close()
	s.queue(b)
	got := make([]float32, 20000)
	n, err := s.Read(got)
	if math.Abs(float64(n-19200)) > 2 || err != io.EOF {
		t.Fatalf("samples=%d err=%v", n, err)
	}
	for i := 9580; i < 9620; i++ {
		if got[i] == 0 {
			t.Fatalf("silence at transition sample %d", i)
		}
	}
	part, pending, _ := s.state(4700)
	if part.track != a || !pending {
		t.Fatal("changed visible song before it is heard")
	}
	part, _, _ = s.state(4900)
	if part.track != b {
		t.Fatal("did not change visible song after boundary")
	}
}

// Seeking back across decoded lookahead restores the following song;
// replacing the queue while seeking must instead play the newly selected one.
func TestSequenceSeekAndReplace(t *testing.T) {
	for _, replace := range []bool{false, true} {
		a, b, c := testPCM(t, 48000, 40, .25), testPCM(t, 48000, 80, .5), testPCM(t, 48000, 60, .75)
		s := newSequence(a, nil)
		s.queue(b)
		scratch := make([]float32, 100)
		s.Read(scratch)
		if replace {
			s.queue(c)
		}
		if err := s.SetPosition(30); err != nil {
			t.Fatal(err)
		}
		got := make([]float32, 400)
		n, err := s.Read(got)
		wantN, want := 180, float32(.5)
		if replace {
			wantN, want = 140, .75
		}
		if n != wantN || err != io.EOF {
			t.Fatalf("replace %v: n=%d err=%v", replace, n, err)
		}
		for i, v := range got[:n] {
			expected := want
			if i < 20 {
				expected = .25
			}
			if v != expected {
				t.Fatalf("replace %v sample %d=%v want %v", replace, i, v, expected)
			}
		}
		s.Close()
	}
}

// A removed successor is never read and its lifetime ends once both the
// preparation and the sequence let it go.
func TestSequenceDropsQueuedTrack(t *testing.T) {
	a, b := testPCM(t, 48000, 40, .25), testPCM(t, 48000, 80, .5)
	s := newSequence(a, nil)
	defer s.Close()
	s.queue(b)
	s.queue(nil)
	got := make([]float32, 400)
	n, err := s.Read(got)
	if n != 80 || err != io.EOF {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if b.refs.Load() != 1 {
		t.Fatal("sequence kept removed track")
	}
}

// Leaving a radio after lookahead crossed the boundary must also remove
// the consumed successor when the buffered audio is rewound.
func TestSequenceRemoveAfterLookahead(t *testing.T) {
	a, b := testPCM(t, 48000, 40, .25), testPCM(t, 48000, 80, .5)
	s := newSequence(a, nil)
	defer s.Close()
	s.queue(b)
	s.Read(make([]float32, 100))
	s.queue(nil)
	if err := s.SetPosition(30); err != nil {
		t.Fatal(err)
	}
	got := make([]float32, 400)
	n, err := s.Read(got)
	if n != 20 || err != io.EOF {
		t.Fatalf("removed successor played: n=%d err=%v", n, err)
	}
}

// Per-song loudness changes take effect at the sample boundary, even
// while the main thread is still displaying the preceding song.
func TestSequenceGainAtBoundary(t *testing.T) {
	a, b := testPCM(t, 48000, 3, .25), testPCM(t, 48000, 3, .25)
	a.setGain(2)
	b.setGain(.5)
	s := newSequence(a, nil)
	defer s.Close()
	s.queue(b)
	got := make([]float32, 12)
	n, _ := s.Read(got)
	if n != 12 {
		t.Fatal(n)
	}
	for i, v := range got {
		want := float32(.5)
		if i >= 6 {
			want = .125
		}
		if v != want {
			t.Fatalf("sample %d=%v want %v", i, v, want)
		}
	}
}

// Resampling must keep an exact frame count, and a seek must produce the
// same samples as decoding from the beginning, including fractional positions.
func TestResamplerLengthAndSeek(t *testing.T) {
	for _, rate := range []int{32000, 44100, 96000} {
		for _, frames := range []int{1, 37, 4410} {
			samples := make([]float32, frames*2)
			for i := range samples {
				samples[i] = float32(math.Sin(float64(i)/7)) * .25
			}
			r := newResampler(&samplesStream{samples: samples, rate: rate})
			ref := make([]float32, (int(r.Len())+2)*2)
			n, err := r.Read(ref)
			if n != int(r.Len())*2 || err != io.EOF {
				t.Fatalf("rate %d frames %d: n=%d len=%d err=%v", rate, frames, n, r.Len(), err)
			}
			ref = ref[:n]
			for _, at := range []int64{0, r.Len() / 2, r.Len() - 1} {
				if err := r.SetPosition(at); err != nil {
					t.Fatal(err)
				}
				got := make([]float32, len(ref))
				n, _ := r.Read(got)
				if n != len(ref)-int(at)*2 || maxDiff(got[:n], ref[at*2:]) > 1e-6 {
					t.Fatalf("rate %d seek %d differs from full decode", rate, at)
				}
			}
		}
	}
}
