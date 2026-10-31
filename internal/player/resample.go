package player

import (
	"io"
	"math"

	"github.com/egoist/mygo/audio"
)

func (p *pcmTrack) setGain(g float64) { p.gain.Store(math.Float64bits(g)) }
func (p *pcmTrack) volume() float32   { return float32(math.Float64frombits(p.gain.Load())) }

// resampler presents stereo music at the output rate, with cubic
// interpolation as MyGo's mixer uses. A song with the output's rate
// bypasses it entirely.
type resampler struct {
	d     audio.Decoder
	buf   [4096]float32
	n, at int
	hist  [4][2]float32
	index int64
	frame int64
	end   int64
	err   error
}

func newResampler(d audio.Decoder) *resampler {
	return &resampler{d: d, index: -3, end: -1}
}
func (*resampler) SampleRate() int { return outputRate }
func (*resampler) Channels() int   { return 2 }
func (r *resampler) Len() int64 {
	n, rate := r.d.Len(), int64(r.d.SampleRate())
	if n < 0 {
		return -1
	}
	return (n*outputRate + rate - 1) / rate
}
func (r *resampler) next() {
	var v [2]float32
	if r.at == r.n && r.err == nil {
		r.n, r.err = r.d.Read(r.buf[:])
		r.at = 0
		if r.n == 0 && r.err == nil {
			r.err = io.ErrNoProgress
		}
	}
	if r.at < r.n {
		v = [2]float32{r.buf[r.at], r.buf[r.at+1]}
		r.at += 2
	} else if r.end < 0 {
		r.end = r.index + 3
	}
	r.hist[0], r.hist[1], r.hist[2], r.hist[3] = r.hist[1], r.hist[2], r.hist[3], v
	r.index++
}
func (r *resampler) Read(b []float32) (int, error) {
	rate := int64(r.d.SampleRate())
	for i := 0; i+1 < len(b); i += 2 {
		pos := r.frame * rate
		for r.index < pos/outputRate {
			r.next()
		}
		if r.end >= 0 && pos >= r.end*outputRate {
			if r.err != nil {
				return i, r.err
			}
			return i, io.EOF
		}
		t := float32(pos%outputRate) / outputRate
		for ch := range 2 {
			a, c, d, e := r.hist[0][ch], r.hist[1][ch], r.hist[2][ch], r.hist[3][ch]
			b[i+ch] = c + 0.5*t*(d-a+t*(2*a-5*c+4*d-e+t*(3*(c-d)+e-a)))
		}
		r.frame++
	}
	return len(b) / 2 * 2, nil
}
func (r *resampler) SetPosition(frame int64) error {
	first := max(frame*int64(r.d.SampleRate())/outputRate-1, 0)
	if err := r.d.SetPosition(first); err != nil {
		return err
	}
	r.n, r.at, r.index, r.frame, r.end, r.err = 0, 0, first-3, frame, -1, nil
	r.hist = [4][2]float32{}
	return nil
}
