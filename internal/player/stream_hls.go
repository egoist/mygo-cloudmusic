package player

import (
	"context"
	"encoding/binary"
	"io"
	"sync"
	"time"

	gomp3 "github.com/hajimehoshi/go-mp3"
)

// hlsStream decodes an MP3 source that arrives in ordered pieces. A
// seek starts at the piece it lands in, rather than fetching all earlier
// pieces; the source supplies each piece's duration and the song's total.
//
// An MP3 frame borrows bits from the frames before it, so the decoder starts
// a piece early on a seek, and drops what that decodes (the preroll).
type hlsStream struct {
	h      SegmentedSource
	starts []int64 // the frame each piece starts at, by the lengths given
	rate   int
	frames int64

	rd   *pieceReader
	dec  *gomp3.Decoder
	pos  int64 // the frame decoded next
	skip int   // bytes of decoded audio to drop after a seek
	buf  []byte
	spec *spectrum
	prog *download    // how many pieces are in, for the progress bar
	seen map[int]bool // the pieces counted, which prog.mu guards

	closeOnce sync.Once
}

// hlsAhead is how many pieces are fetched ahead of the one decoded: each
// piece plays for two seconds, and buffering ahead protects against network
// jitter while keeping memory usage modest.
const hlsAhead = 32

func openSegmentedStream(h SegmentedSource) (*hlsStream, error) {
	if h == nil || h.SegmentCount() == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	s := &hlsStream{h: h, seen: map[int]bool{}}
	s.prog = &download{size: int64(h.SegmentCount())}
	s.prog.cond.L = &s.prog.mu
	if err := s.start(0); err != nil {
		return nil, err
	}
	s.rate = s.dec.SampleRate()
	s.frames = int64(h.TotalDuration().Seconds() * float64(s.rate))
	s.starts = make([]int64, h.SegmentCount()+1)
	var at float64
	for i := 0; i < h.SegmentCount(); i++ {
		s.starts[i] = int64(at * float64(s.rate))
		at += h.SegmentDuration(i).Seconds()
	}
	s.starts[h.SegmentCount()] = s.frames
	s.spec = newSpectrum(s.rate, s.frames)
	return s, nil
}

// start decodes from piece i on.
func (s *hlsStream) start(i int) error {
	if s.rd != nil {
		s.rd.close()
	}
	s.rd = newPieceReader(s.h, i, s.prog, s.seen)
	dec, err := gomp3.NewDecoder(s.rd)
	if err != nil {
		s.rd.close()
		return err
	}
	s.dec = dec
	return nil
}

func (s *hlsStream) SampleRate() int { return s.rate }
func (s *hlsStream) Channels() int   { return 2 }
func (s *hlsStream) Len() int64      { return s.frames }

func (s *hlsStream) Read(dst []float32) (int, error) {
	n := len(dst) / 2 * 4
	if cap(s.buf) < n {
		s.buf = make([]byte, n)
	}
	b := s.buf[:n]
	for s.skip > 0 {
		got, err := io.ReadFull(s.dec, b[:min(n, s.skip)])
		s.skip -= got
		if err != nil {
			s.skip = 0
		}
	}
	got, err := io.ReadFull(s.dec, b)
	got -= got % 4
	samples := got / 2
	for i := range samples {
		dst[i] = float32(int16(binary.LittleEndian.Uint16(b[2*i:]))) / (1 << 15)
	}
	s.spec.feed(s.pos, dst[:samples])
	s.pos += int64(samples / 2)
	if samples > 0 {
		return samples, nil
	}
	if err == io.ErrUnexpectedEOF || err == nil {
		err = io.EOF
	}
	return 0, err
}

// SetPosition moves to frame: it starts from the piece before the one the
// frame is in, and drops the sound up to the frame.
func (s *hlsStream) SetPosition(frame int64) error {
	frame = min(max(frame, 0), s.frames)
	k := 0
	for k+1 < s.h.SegmentCount() && s.starts[k+1] <= frame {
		k++
	}
	from := max(k-1, 0)
	if err := s.start(from); err != nil {
		return err
	}
	s.pos = s.starts[from]
	s.skip = int(frame-s.starts[from]) * 4
	return nil
}

func (s *hlsStream) source() (*download, *spectrum) { return s.prog, s.spec }

func (s *hlsStream) Close() error {
	s.closeOnce.Do(func() {
		s.rd.close()
		s.h.Close()
	})
	return nil
}

// pieceReader reads the audio of the pieces from i on, one after another.
// A goroutine fetches them in order, up to hlsAhead ahead of the one read.
//
// In order, and not all at once: the service transcodes as it is asked, from
// where it was last asked for on. The piece after the last is as good as
// made, a few milliseconds; one anywhere else starts the transcoding over
// there, a third of a second. Asking for four at once makes it start over
// for each, in turn, and a whole song then takes minutes.
type pieceReader struct {
	cancel context.CancelFunc
	pieces chan pieceResult
	cur    []byte
}

type pieceResult struct {
	b   []byte
	err error
}

func newPieceReader(h SegmentedSource, from int, prog *download, seen map[int]bool) *pieceReader {
	ctx, cancel := context.WithCancel(context.Background())
	capacity := max(h.SegmentCount()-from, hlsAhead)
	r := &pieceReader{cancel: cancel, pieces: make(chan pieceResult, capacity)}
	go func() {
		defer close(r.pieces)
		for i := from; i < h.SegmentCount(); i++ {
			var b []byte
			var err error
			for attempt := 0; attempt < 5; attempt++ {
				b, err = h.FetchSegment(ctx, i)
				if err == nil && len(b) > 0 {
					break
				}
				if attempt == 4 && err == nil && len(b) == 0 {
					err = io.ErrUnexpectedEOF
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Duration(100*(1<<attempt)) * time.Millisecond):
				}
			}
			if err == nil {
				countPiece(prog, seen, i)
			}
			select {
			case r.pieces <- pieceResult{b, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	return r
}

// countPiece counts piece i as in, once, for the progress bar.
func countPiece(prog *download, seen map[int]bool, i int) {
	prog.mu.Lock()
	defer prog.mu.Unlock()
	if seen[i] {
		return
	}
	seen[i] = true
	prog.n++
	if prog.n >= prog.size {
		prog.done = true
	}
}

func (r *pieceReader) Read(p []byte) (int, error) {
	for len(r.cur) == 0 {
		res, ok := <-r.pieces
		if !ok {
			return 0, io.EOF
		}
		if res.err != nil {
			return 0, res.err
		}
		r.cur = res.b
	}
	n := copy(p, r.cur)
	r.cur = r.cur[n:]
	return n, nil
}

func (r *pieceReader) close() { r.cancel() }
