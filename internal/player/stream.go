package player

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/cmplx"
	"net/http"
	"sync"

	"github.com/egoist/mygo/audio"
	gomp3 "github.com/hajimehoshi/go-mp3"

	"netease-music-mygo/internal/netease"
)

// A download fetches a song into memory as it plays: readers wait for the
// bytes they need, so playback starts with the first ones.
type download struct {
	mu     sync.Mutex
	cond   sync.Cond
	data   []byte
	size   int64 // expected, or 0 if unknown
	done   bool
	err    error
	cancel context.CancelFunc
	// whole, if set, takes the song once it has all come in.
	whole func([]byte)
}

// startDownload fetches url, of size bytes if known, and gives whole the
// song once it has all come in.
func startDownload(url string, size int64, whole func([]byte)) *download {
	ctx, cancel := context.WithCancel(context.Background())
	d := &download{size: size, cancel: cancel, whole: whole}
	d.cond.L = &d.mu
	if size > 0 {
		d.data = make([]byte, 0, size)
	}
	go d.run(ctx, url)
	return d
}

// downloaded returns a download that has all of b in already, as a song
// from the cache.
func downloaded(b []byte) *download {
	d := &download{data: b, size: int64(len(b)), done: true, cancel: func() {}}
	d.cond.L = &d.mu
	return d
}

func (d *download) run(ctx context.Context, url string) {
	finish := func(err error) {
		d.mu.Lock()
		d.done, d.err = true, err
		d.cond.Broadcast()
		d.mu.Unlock()
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		finish(err)
		return
	}
	req.Header.Set("User-Agent", netease.UserAgent)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		finish(err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		finish(errors.New(res.Status))
		return
	}
	buf := make([]byte, 64<<10)
	for {
		n, err := res.Body.Read(buf)
		if n > 0 {
			d.mu.Lock()
			d.data = append(d.data, buf[:n]...)
			d.cond.Broadcast()
			d.mu.Unlock()
		}
		if err == io.EOF {
			finish(nil)
			// Nothing appends to the data any more: it is the song's, whole
			// if as long as promised.
			if d.whole != nil && (d.size <= 0 || int64(len(d.data)) == d.size) {
				d.whole(d.data)
			}
			return
		}
		if err != nil {
			finish(err)
			return
		}
	}
}

// Close stops the download and wakes its readers.
func (d *download) Close() {
	d.cancel()
	d.mu.Lock()
	if !d.done {
		d.done, d.err = true, context.Canceled
	}
	d.cond.Broadcast()
	d.mu.Unlock()
}

// Progress returns the share of the song downloaded so far.
func (d *download) Progress() float64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.done && d.err == nil {
		return 1
	}
	if d.size <= 0 {
		return 0
	}
	return float64(len(d.data)) / float64(d.size)
}

// waitFor waits until n bytes are in, or the download ended, and returns
// how many there are.
func (d *download) waitFor(n int64) int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	for int64(len(d.data)) < n && !d.done {
		d.cond.Wait()
	}
	return int64(len(d.data))
}

// bytesAt returns the bytes from off as far as they are in.
func (d *download) bytesAt(off int64) []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	if off >= int64(len(d.data)) {
		return nil
	}
	return d.data[off:len(d.data):len(d.data)]
}

// A downloadReader reads a download from an offset, waiting for its bytes.
// It is no io.Seeker, so go-mp3 does not read the whole file up front to
// find where each frame starts.
type downloadReader struct {
	d   *download
	pos int64
}

func (r *downloadReader) Read(p []byte) (int, error) {
	d := r.d
	d.mu.Lock()
	defer d.mu.Unlock()
	for int64(len(d.data)) <= r.pos && !d.done {
		d.cond.Wait()
	}
	if int64(len(d.data)) <= r.pos {
		if d.err != nil {
			return 0, d.err
		}
		return 0, io.EOF
	}
	n := copy(p, d.data[r.pos:])
	r.pos += int64(n)
	return n, nil
}

// A stream decodes a download for package audio, and measures the
// spectrum of what it decodes, for the visualizers: an MP3's or a FLAC's.
type stream interface {
	audio.Decoder
	Close() error
	// source returns the download decoded, and its spectrum.
	source() (*download, *spectrum)
}

// openStream opens the song dl fetches, MP3 or FLAC, as its first bytes
// say. An MP3 says nothing of its length: durationMs is it.
func openStream(dl *download, durationMs int64) (stream, error) {
	off := id3Size(dl)
	if dl.waitFor(off+4) >= off+4 && string(dl.bytesAt(off)[:4]) == "fLaC" {
		return openFLACStream(dl)
	}
	return openMP3Stream(dl, durationMs)
}

// mp3Stream decodes a download for package audio, seeking by bytes: songs
// are encoded at a constant bit rate, so time and bytes are in proportion.
// It also measures the spectrum of what it decodes, for the visualizers.
type mp3Stream struct {
	dl      *download
	dec     *gomp3.Decoder
	rate    int
	frames  int64 // the song's length in frames, from its duration
	start   int64 // the first audio frame's byte, after the ID3 tag
	pos     int64 // the frame decoded next
	skip    int   // bytes of decoded audio to drop after a seek
	buf     []byte
	spec    *spectrum
	closeMu sync.Once
}

func openMP3Stream(dl *download, durationMs int64) (*mp3Stream, error) {
	s := &mp3Stream{dl: dl}
	dec, err := gomp3.NewDecoder(&downloadReader{d: dl})
	if err != nil {
		return nil, err
	}
	s.dec, s.rate = dec, dec.SampleRate()
	s.frames = durationMs * int64(s.rate) / 1000
	s.start = id3Size(dl)
	s.spec = newSpectrum(s.rate, s.frames)
	return s, nil
}

// id3Size returns the length of the ID3v2 tag at the start of the file.
func id3Size(dl *download) int64 {
	dl.waitFor(10)
	b := dl.bytesAt(0)
	if len(b) < 10 || string(b[:3]) != "ID3" {
		return 0
	}
	size := int64(b[6]&0x7f)<<21 | int64(b[7]&0x7f)<<14 | int64(b[8]&0x7f)<<7 | int64(b[9]&0x7f)
	return 10 + size
}

func (s *mp3Stream) SampleRate() int { return s.rate }
func (s *mp3Stream) Channels() int   { return 2 }
func (s *mp3Stream) Len() int64      { return s.frames }

func (s *mp3Stream) Read(dst []float32) (int, error) {
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
	frames := int64(samples / 2)
	s.spec.feed(s.pos, dst[:samples])
	s.pos += frames
	if samples > 0 {
		return samples, nil
	}
	if err == io.ErrUnexpectedEOF || err == nil {
		err = io.EOF
	}
	return 0, err
}

// preroll is how many frames of sound a seek decodes before the one asked
// for, and drops. An MP3 frame borrows bits from the frames before it, up
// to 511 bytes back, which at 128 kbps is two frames, and its sound
// overlaps theirs: the first few decoded from the middle of a file are
// noise, as loud as the loudest sound.
const preroll = 4 * 1152

func (s *mp3Stream) SetPosition(frame int64) error {
	var off int64
	if from := frame - preroll; from > 0 && s.frames > 0 {
		size := s.dl.size
		if size <= 0 {
			size = s.dl.waitFor(math.MaxInt64)
		}
		off = s.start + int64(float64(size-s.start)*float64(from)/float64(s.frames))
		off = findFrame(s.dl, off)
	}
	dec, err := gomp3.NewDecoder(&downloadReader{d: s.dl, pos: off})
	if err != nil {
		return err
	}
	s.dec, s.pos = dec, frame
	// Drop what comes before frame: the preroll, or near the start, all
	// of it from there.
	s.skip = int(min(frame, preroll)) * 4
	return nil
}

func (s *mp3Stream) source() (*download, *spectrum) { return s.dl, s.spec }

func (s *mp3Stream) Close() error {
	s.closeMu.Do(s.dl.Close)
	return nil
}

// findFrame returns the start of the first MP3 frame at or after off whose
// header the next frame's confirms, so that bytes that only look like one
// within the audio do not count.
func findFrame(dl *download, off int64) int64 {
	for tries := 0; tries < 64<<10; tries++ {
		have := dl.waitFor(off + 4)
		if have < off+4 {
			return off
		}
		b := dl.bytesAt(off)
		size := frameSize(b)
		if size > 0 {
			have = dl.waitFor(off + int64(size) + 4)
			next := dl.bytesAt(off + int64(size))
			if len(next) < 4 || frameSize(next) > 0 {
				return off
			}
		}
		off++
	}
	return off
}

var (
	bitratesV1 = [16]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
	bitratesV2 = [16]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0}
	rates      = [4]int{44100, 48000, 32000, 0}
)

// frameSize returns the length of the MPEG layer III frame whose header b
// starts with, or 0 if it starts with none.
func frameSize(b []byte) int {
	if len(b) < 4 || b[0] != 0xff || b[1]&0xe0 != 0xe0 {
		return 0
	}
	version := (b[1] >> 3) & 3 // 3: MPEG 1, 2: MPEG 2, 0: MPEG 2.5
	layer := (b[1] >> 1) & 3   // 1: layer III
	if version == 1 || layer != 1 {
		return 0
	}
	bi, ri := b[2]>>4, (b[2]>>2)&3
	if bi == 0 || bi == 15 || ri == 3 {
		return 0
	}
	pad := int(b[2]>>1) & 1
	rate := rates[ri]
	switch version {
	case 3:
		return 144*bitratesV1[bi]*1000/rate + pad
	case 2:
		return 72*bitratesV2[bi]*1000/(rate/2) + pad
	default:
		return 72*bitratesV2[bi]*1000/(rate/4) + pad
	}
}

// The spectrum of a song, measured in chunks of fftSize frames as it
// decodes, a second ahead of what plays: the visualizers look up the
// chunk playing.

const (
	fftSize = 1024
	Bands   = 24
)

type spectrum struct {
	mu     sync.RWMutex
	rate   int
	chunks [][Bands]float32
	have   []bool
	window [fftSize]float32
	acc    []float32 // mono samples of the chunk being filled
	accAt  int64     // its first frame
	edges  [Bands + 1]int
	fft    []complex128
}

func newSpectrum(rate int, frames int64) *spectrum {
	n := int(frames/fftSize) + 2
	s := &spectrum{rate: rate, chunks: make([][Bands]float32, n), have: make([]bool, n), accAt: -1}
	for i := range s.window {
		s.window[i] = float32(0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(fftSize-1)))
	}
	// Bands spaced evenly in pitch, from 40 Hz to 14 kHz.
	lo, hi := math.Log(40), math.Log(14000)
	for i := range s.edges {
		f := math.Exp(lo + (hi-lo)*float64(i)/Bands)
		s.edges[i] = max(1, min(fftSize/2-1, int(f*fftSize/float64(rate))))
	}
	for i := 1; i < len(s.edges); i++ {
		s.edges[i] = max(s.edges[i], s.edges[i-1]+1)
	}
	s.fft = make([]complex128, fftSize)
	return s
}

// feed takes stereo samples decoded from frame at.
func (s *spectrum) feed(at int64, stereo []float32) {
	if s.accAt < 0 || at != s.accAt+int64(len(s.acc)) {
		// The start, or a seek: begin a chunk at a chunk's boundary.
		s.acc = s.acc[:0]
		skip := (fftSize - at%fftSize) % fftSize
		if int(skip)*2 >= len(stereo) {
			s.accAt = -1
			return
		}
		stereo = stereo[skip*2:]
		at += skip
		s.accAt = at
	}
	for i := 0; i+1 < len(stereo); i += 2 {
		s.acc = append(s.acc, (stereo[i]+stereo[i+1])/2)
		if len(s.acc) == fftSize {
			s.analyze(s.accAt / fftSize)
			s.accAt += fftSize
			s.acc = s.acc[:0]
		}
	}
}

func (s *spectrum) analyze(chunk int64) {
	if chunk < 0 || chunk >= int64(len(s.chunks)) {
		return
	}
	for i, v := range s.acc {
		s.fft[i] = complex(float64(v*s.window[i]), 0)
	}
	fft(s.fft)
	var bands [Bands]float32
	for b := range Bands {
		var sum float64
		for k := s.edges[b]; k < s.edges[b+1]; k++ {
			sum += cmplx.Abs(s.fft[k])
		}
		mag := sum / float64(s.edges[b+1]-s.edges[b]) / (fftSize / 4)
		// Higher bands carry less energy: tilt them up, as the ear hears.
		mag *= 1 + float64(b)/6
		db := 20 * math.Log10(mag+1e-9)
		bands[b] = float32(math.Max(0, math.Min(1, (db+60)/52)))
	}
	s.mu.Lock()
	s.chunks[chunk] = bands
	s.have[chunk] = true
	s.mu.Unlock()
}

// at returns the bands of frame, or false if not measured.
func (s *spectrum) at(frame int64) ([Bands]float32, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c := frame / fftSize
	if c < 0 || c >= int64(len(s.chunks)) || !s.have[c] {
		return [Bands]float32{}, false
	}
	return s.chunks[c], true
}

// fft transforms x in place; len(x) is a power of two.
func fft(x []complex128) {
	n := len(x)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		w := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			wk := complex(1, 0)
			for k := range size / 2 {
				a, b := x[start+k], x[start+k+size/2]*wk
				x[start+k], x[start+k+size/2] = a+b, a-b
				wk *= w
			}
		}
	}
}
