package app

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	_ "golang.org/x/image/webp"

	"netease-music-mygo/internal/fnmusic"
)

// artwork is a cover, decoded and ready to draw, with what the player
// derives from it: its color, and a blurred copy to fill a backdrop.
type artwork struct {
	bmp      *ui.Bitmap
	backdrop *ui.Bitmap
	tint     ui.Color // the cover's most vivid color
	failed   bool
	used     time.Time // when it was last asked for
}

// artCache loads covers on goroutines and keeps them while they show, and
// for a while after: their pixels on the GPU, and on the CPU only their
// data, which takes a fraction of the pixels. Its methods run on the main
// thread; post runs a function there.
type artCache struct {
	art   map[string]*artwork
	busy  map[string]bool
	sem   chan struct{}
	post  func(func())
	now   func() time.Time
	swept time.Time
	asked time.Time // when a cover was last asked for
}

// artKeep is how long a cover not asked for stays: the pages scrolled
// through over a session show thousands.
const artKeep = 5 * time.Minute

func newArtCache(post func(func())) *artCache {
	return &artCache{art: map[string]*artwork{}, busy: map[string]bool{}, sem: make(chan struct{}, 8), post: post, now: time.Now, swept: time.Now()}
}

// sweep drops the covers not asked for in artKeep before the last one
// was, once a minute. The time the window draws nothing, idle or in the
// background, doesn't count: the first frame after it would drop every
// cover it shows, and they would flash as they load again.
func (c *artCache) sweep(now time.Time) {
	if now.Sub(c.swept) < time.Minute {
		return
	}
	c.swept = now
	for key, a := range c.art {
		if c.asked.Sub(a.used) > artKeep {
			delete(c.art, key)
		}
	}
}

// get returns the cover at url, or nil while it loads.
func (c *artCache) get(url string, backdrop bool) *artwork {
	if url == "" {
		return nil
	}
	key := url
	if backdrop {
		key += "#backdrop"
	}
	now := c.now()
	c.sweep(now)
	c.asked = now
	if a, ok := c.art[key]; ok {
		a.used = now
		return a
	}
	if c.busy[key] {
		return nil
	}
	c.busy[key] = true
	go func() {
		c.sem <- struct{}{}
		bmp, img, err := loadArt(url)
		<-c.sem
		var blurred *image.RGBA
		var tint ui.Color
		if err == nil {
			tint = dominantColor(img)
			if backdrop {
				blurred = blurImage(img)
			}
		}
		c.post(func() {
			delete(c.busy, key)
			if err != nil {
				c.art[key] = &artwork{failed: true, used: c.now()}
				return
			}
			a := &artwork{bmp: bmp, tint: tint, used: c.now()}
			if blurred != nil {
				a.backdrop = ui.NewBitmap(blurred)
			}
			c.art[key] = a
			// Shown by the frame post draws, the cover drops its pixels,
			// which its data decodes again if they are needed.
			time.AfterFunc(artRelease, func() { mygo.RunOnMain(bmp.ReleasePixels) })
		})
	}()
	return nil
}

// artRelease is how long a cover keeps its pixels once loaded: past the
// frame that first shows it, and the smaller copies it shows from there.
const artRelease = 2 * time.Second

// loadArt fetches the cover at url, as a bitmap to draw and an image to
// measure.
func loadArt(url string) (*ui.Bitmap, image.Image, error) {
	b, err := fnmusic.Fetch(url)
	if err != nil {
		return nil, nil, err
	}
	return decodeArt(b)
}

// decodeArt decodes a cover twice: as a bitmap, which keeps the data to
// decode its pixels again after it drops them, and as an image.
func decodeArt(b []byte) (*ui.Bitmap, image.Image, error) {
	b = stillCover(b)
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, nil, err
	}
	bmp, err := ui.DecodeBitmap(b)
	return bmp, img, err
}

func fetchImage(url string) (image.Image, error) {
	b, err := fnmusic.Fetch(url)
	if err != nil {
		return nil, err
	}
	return decodeImage(b)
}

func decodeImage(b []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(stillCover(b)))
	return img, err
}

// stillCover returns the data of a cover that the decoders read. Some
// covers are animated WebP, whatever their name says, which they cannot:
// those show their first frame.
func stillCover(b []byte) []byte {
	if still := firstWebPFrame(b); still != nil {
		return still
	}
	return b
}

// firstWebPFrame returns the first frame of an animated WebP as a still
// WebP of its own, or nil if b is not one.
func firstWebPFrame(b []byte) []byte {
	if len(b) < 12 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
		return nil
	}
	chunk := func(b []byte) (name string, body, rest []byte, ok bool) {
		if len(b) < 8 {
			return "", nil, nil, false
		}
		n := int(binary.LittleEndian.Uint32(b[4:8]))
		if n > len(b)-8 {
			return "", nil, nil, false
		}
		end := min(len(b), 8+n+n&1)
		return string(b[:4]), b[8 : 8+n], b[end:], true
	}
	riff := func(chunks ...[]byte) []byte {
		body := []byte("WEBP")
		for _, c := range chunks {
			body = append(body, c...)
		}
		out := binary.LittleEndian.AppendUint32([]byte("RIFF"), uint32(len(body)))
		return append(out, body...)
	}
	for rest := b[12:]; ; {
		name, body, next, ok := chunk(rest)
		if !ok {
			return nil
		}
		rest = next
		if name != "ANMF" || len(body) < 16 {
			continue
		}
		// A frame: where it sits, its size, how long it shows, then its
		// image as the chunks of a still WebP, its alpha first if any.
		w, h := body[6:9], body[9:12]
		frame := body[16:]
		var alpha []byte
		for len(frame) > 0 {
			name, _, after, ok := chunk(frame)
			if !ok {
				return nil
			}
			whole := frame[:len(frame)-len(after)]
			switch name {
			case "ALPH":
				alpha = whole
			case "VP8L":
				return riff(whole)
			case "VP8 ":
				if alpha == nil {
					return riff(whole)
				}
				// Alpha needs the extended header, which carries the size.
				vp8x := []byte{'V', 'P', '8', 'X', 10, 0, 0, 0, 0x10, 0, 0, 0}
				vp8x = append(append(vp8x, w...), h...)
				return riff(vp8x, alpha, whole)
			}
			frame = after
		}
		return nil
	}
}

// shrink averages img down to n×n pixels.
func shrink(img image.Image, n int) *image.RGBA {
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := range n {
		for x := range n {
			x0, x1 := b.Min.X+x*b.Dx()/n, b.Min.X+(x+1)*b.Dx()/n
			y0, y1 := b.Min.Y+y*b.Dy()/n, b.Min.Y+(y+1)*b.Dy()/n
			var r, g, bl, cnt uint32
			for yy := y0; yy < max(y1, y0+1); yy += max(1, (y1-y0)/4) {
				for xx := x0; xx < max(x1, x0+1); xx += max(1, (x1-x0)/4) {
					cr, cg, cb, _ := img.At(xx, yy).RGBA()
					r, g, bl, cnt = r+cr>>8, g+cg>>8, bl+cb>>8, cnt+1
				}
			}
			out.SetRGBA(x, y, color.RGBA{uint8(r / cnt), uint8(g / cnt), uint8(bl / cnt), 255})
		}
	}
	return out
}

// dominantColor returns the most vivid color that covers much of img: the
// hue that its saturated pixels share most, at their mean.
func dominantColor(img image.Image) ui.Color {
	small := shrink(img, 24)
	const buckets = 18
	var weight [buckets]float64
	var sum [buckets][3]float64
	var gray [3]float64
	for i := 0; i < len(small.Pix); i += 4 {
		r, g, b := float64(small.Pix[i])/255, float64(small.Pix[i+1])/255, float64(small.Pix[i+2])/255
		h, s, v := hsv(r, g, b)
		gray[0], gray[1], gray[2] = gray[0]+r, gray[1]+g, gray[2]+b
		w := s * s * v
		if s < 0.15 || v < 0.15 {
			continue
		}
		k := int(h/360*buckets) % buckets
		weight[k] += w
		sum[k][0], sum[k][1], sum[k][2] = sum[k][0]+r*w, sum[k][1]+g*w, sum[k][2]+b*w
	}
	best := 0
	for k := range weight {
		if weight[k] > weight[best] {
			best = k
		}
	}
	if weight[best] < 2 {
		n := float64(len(small.Pix) / 4)
		return ui.RGB(uint8(gray[0]/n*255), uint8(gray[1]/n*255), uint8(gray[2]/n*255))
	}
	w := weight[best]
	return ui.RGB(uint8(sum[best][0]/w*255), uint8(sum[best][1]/w*255), uint8(sum[best][2]/w*255))
}

func hsv(r, g, b float64) (h, s, v float64) {
	mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	v = mx
	if mx == 0 {
		return 0, 0, 0
	}
	s = (mx - mn) / mx
	d := mx - mn
	switch {
	case d == 0:
		h = 0
	case mx == r:
		h = math.Mod((g-b)/d, 6) * 60
	case mx == g:
		h = ((b-r)/d + 2) * 60
	default:
		h = ((r-g)/d + 4) * 60
	}
	if h < 0 {
		h += 360
	}
	return h, s, v
}

// blurImage returns img shrunk and blurred, more saturated, to fill a
// backdrop behind what plays, scaled up as smooth as it is.
func blurImage(img image.Image) *image.RGBA {
	const n = 48
	small := shrink(img, n)
	px := make([][3]float64, n*n)
	for i := range px {
		r, g, b := float64(small.Pix[4*i]), float64(small.Pix[4*i+1]), float64(small.Pix[4*i+2])
		// More saturated, as light through colored glass.
		l := 0.3*r + 0.59*g + 0.11*b
		px[i] = [3]float64{l + (r-l)*1.5, l + (g-l)*1.5, l + (b-l)*1.5}
	}
	tmp := make([][3]float64, n*n)
	for range 3 {
		boxBlur(px, tmp, n, 5)
	}
	out := image.NewRGBA(image.Rect(0, 0, n, n))
	for i, p := range px {
		for c := range 3 {
			out.Pix[4*i+c] = uint8(math.Max(0, math.Min(255, p[c])))
		}
		out.Pix[4*i+3] = 255
	}
	return out
}

// boxBlur blurs px of n×n pixels by a box of radius r, across then down,
// clamping at the edges.
func boxBlur(px, tmp [][3]float64, n, r int) {
	at := func(i int) int { return max(0, min(n-1, i)) }
	for y := range n {
		for x := range n {
			var s [3]float64
			for k := -r; k <= r; k++ {
				p := px[y*n+at(x+k)]
				s[0], s[1], s[2] = s[0]+p[0], s[1]+p[1], s[2]+p[2]
			}
			d := float64(2*r + 1)
			tmp[y*n+x] = [3]float64{s[0] / d, s[1] / d, s[2] / d}
		}
	}
	for y := range n {
		for x := range n {
			var s [3]float64
			for k := -r; k <= r; k++ {
				p := tmp[at(y+k)*n+x]
				s[0], s[1], s[2] = s[0]+p[0], s[1]+p[1], s[2]+p[2]
			}
			d := float64(2*r + 1)
			px[y*n+x] = [3]float64{s[0] / d, s[1] / d, s[2] / d}
		}
	}
}
