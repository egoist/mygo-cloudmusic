package player

import (
	"io"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo/audio"
	"netease-music-mygo/internal/netease"
)

const outputRate = 48000

// pcmTrack keeps a short decoded head ready. The sequence and a pending
// load each hold a reference, so changing the queue cannot close audio
// the decoder has already started reading.
type pcmTrack struct {
	raw   stream
	track netease.Track
	info  *netease.Stream
	dec   audio.Decoder
	head  []float32
	refs  atomic.Int32
	gain  atomic.Uint64
}

func preparePCM(s stream, t netease.Track, info *netease.Stream) (*pcmTrack, error) {
	p := &pcmTrack{raw: s, track: t, info: info, dec: s}
	p.refs.Store(1)
	p.setGain(1)
	if s.SampleRate() != outputRate {
		p.dec = newResampler(s)
	}
	p.head = make([]float32, 4096*2)
	n, err := p.dec.Read(p.head)
	p.head = p.head[:n]
	if err != nil && err != io.EOF {
		s.Close()
		return nil, err
	}
	return p, nil
}
func (p *pcmTrack) retain() { p.refs.Add(1) }
func (p *pcmTrack) release() {
	if p.refs.Add(-1) == 0 {
		p.raw.Close()
	}
}
func (p *pcmTrack) read(b []float32) (int, error) {
	n := copy(b, p.head)
	p.head = p.head[n:]
	if n > 0 {
		return n, nil
	}
	return p.dec.Read(b)
}
func (p *pcmTrack) seek(f int64) error { p.head = nil; return p.dec.SetPosition(f) }

type audioPart struct {
	track    *pcmTrack
	start    int64
	revision uint64
}

// sequence is one decoder for the lifetime of continuous playback. At
// EOF it fills the remainder of the SAME Read with the next song: neither
// the UI thread nor an end callback sits between the two sets of samples.
// Only MyGo's decoder goroutine calls Read and SetPosition.
type sequence struct {
	mu            sync.Mutex
	parts         []audioPart
	next          *pcmTrack
	revision      uint64
	pos           int64
	ended, closed bool
	changed       func()
}

func newSequence(p *pcmTrack, changed func()) *sequence {
	p.retain()
	return &sequence{parts: []audioPart{{track: p}}, changed: changed}
}
func (*sequence) SampleRate() int { return outputRate }
func (*sequence) Channels() int   { return 2 }
func (*sequence) Len() int64      { return -1 }

func (s *sequence) queue(p *pcmTrack) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	old := s.next
	if old == p && p != nil {
		s.mu.Unlock()
		return
	}
	s.revision++
	s.next = p
	if p != nil {
		p.retain()
	}
	s.mu.Unlock()
	if old != nil {
		old.release()
	}
}

func (s *sequence) Read(b []float32) (int, error) {
	total := 0
	for total < len(b) {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return total, io.EOF
		}
		p := s.parts[len(s.parts)-1].track
		s.mu.Unlock()
		n, err := p.read(b[total:])
		gain := p.volume()
		for i := total; i < total+n; i++ {
			b[i] *= gain
		}
		total += n
		s.mu.Lock()
		s.pos += int64(n / 2)
		if s.closed {
			s.mu.Unlock()
			return total, io.EOF
		}
		if err != nil || n == 0 {
			if err != nil && err != io.EOF || s.next == nil {
				s.ended = true
				s.mu.Unlock()
				return total, io.EOF
			}
			s.parts = append(s.parts, audioPart{track: s.next, start: s.pos, revision: s.revision})
			s.next = nil
			s.mu.Unlock()
			if s.changed != nil {
				s.changed()
			}
			continue
		}
		s.mu.Unlock()
	}
	return total, nil
}

func (s *sequence) SetPosition(frame int64) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return io.EOF
	}
	i := len(s.parts) - 1
	for i > 0 && frame < s.parts[i].start {
		i--
	}
	part := s.parts[i]
	removed := append([]audioPart(nil), s.parts[i+1:]...)
	// Seeking back before a decoded transition restores the following song,
	// unless a queue change has already selected a different successor.
	if len(removed) > 0 && s.next == nil && removed[0].revision == s.revision {
		s.next = removed[0].track
		s.next.retain()
	}
	s.parts = s.parts[:i+1]
	s.pos, s.ended = frame, false
	next := s.next
	s.mu.Unlock()
	for _, p := range removed {
		if p.track == next {
			p.track.seek(0)
		}
		p.track.release()
	}
	return part.track.seek(max(frame-part.start, 0))
}

// state reports the song heard at frame, and whether another boundary
// or the end still needs to be reflected by the UI.
func (s *sequence) state(frame int64) (audioPart, bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	part := s.parts[0]
	for _, p := range s.parts[1:] {
		if p.start > frame {
			break
		}
		part = p
	}
	pending := s.parts[len(s.parts)-1].start > frame || s.ended && frame < s.pos
	return part, pending, s.ended && frame >= s.pos
}
func (s *sequence) started(p *pcmTrack) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, part := range s.parts {
		if part.track == p {
			return true
		}
	}
	return false
}
func (s *sequence) ahead(start int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.parts[len(s.parts)-1].start > start
}
func (s *sequence) discardBefore(start int64) {
	s.mu.Lock()
	var old []*pcmTrack
	for len(s.parts) > 1 && s.parts[1].start <= start {
		old = append(old, s.parts[0].track)
		s.parts = s.parts[1:]
	}
	s.mu.Unlock()
	for _, p := range old {
		p.release()
	}
}
func (s *sequence) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	parts := append([]audioPart(nil), s.parts...)
	next := s.next
	s.next = nil
	s.mu.Unlock()
	for _, p := range parts {
		p.track.release()
	}
	if next != nil {
		next.release()
	}
	return nil
}
