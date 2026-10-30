package app

import (
	"math"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"

	"netease-music-mygo/internal/player"
)

// The backdrop of Now Playing paints the cover's colors behind it, as
// NetEase's own player does: a deep gradient of its main color, lighter at
// the top, with the cover itself, blurred, faintly through it. It stays
// still, so it is painted once a song, into a canvas of its own, the
// window's DIPs on the GPU and an eighth of them on the CPU, where a shader
// over every pixel would be too slow: what moves over it then draws one
// image under it. The canvas holds eight bits a channel, so the backdrop is
// painted with half a code of dither, a pixel at a time: without it its
// steps show as bands, even on a screen that could show more than eight
// bits, as neither the canvas nor the window's frames use a wider format.
//
// The experimental fog (settings.Fog) makes it alive, as NetEase's mobile
// player's is: fog of the cover's color, billowing as it rises slowly
// through a deep shade of it, with specks of dust glinting in it, brighter
// and thicker with the bass. A shader paints it into a canvas, half the
// window across on the GPU, thirty times a second while the music plays;
// the CPU paints it still. Paused, the fog glides to a stop, and the
// canvas stays as it is.
//
// Another song's backdrop fades in over the last one's, which stays as it
// was, in a canvas of its own, until it is gone.

// fogSource paints the fog: noise of five octaves, warped by noise of
// its own so that it billows rather than slides, rising with Time, Fog
// over Deep where it is thick; thickest a little above the middle, and
// thinning toward the top and the controls at the foot.
const fogSource = `package main

var Time float
var Bass float
var Deep vec4
var Fog vec4

func hash(p vec2) float {
	return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453)
}

func noise(p vec2) float {
	i := floor(p)
	f := fract(p)
	u := f * f * (3.0 - 2.0*f)
	a := hash(i)
	b := hash(i + vec2(1, 0))
	c := hash(i + vec2(0, 1))
	d := hash(i + vec2(1, 1))
	return mix(mix(a, b, u.x), mix(c, d, u.x), u.y)
}

func fbm(p vec2) float {
	v := 0.0
	amp := 0.5
	for i := 0; i < 5; i++ {
		v += amp * noise(p)
		p = p*2.03 + vec2(1.7, 9.2)
		amp *= 0.5
	}
	return v
}

func Fragment(dst vec4, pos vec2) vec4 {
	size := rectSize()
	uv := pos / size
	p := pos / size.y * 2.4
	t := Time
	rise := vec2(0, t*0.1)
	q := vec2(fbm(p+rise+vec2(0, t*0.04)), fbm(p+rise+vec2(5.2, 1.3)-vec2(t*0.05, 0)))
	f := fbm(p + rise + 0.9*q)
	// Broad banks of it, and the puffs within them.
	bank := fbm(p*0.45 + rise*0.6 + vec2(3.7, 0))
	cloud := smoothstep(0.3-0.06*Bass, 0.78, f) * (0.45 + 0.75*smoothstep(0.3, 0.65, bank))
	// Thickest a little above the middle.
	m := smoothstep(-0.1, 0.4, uv.y) * (1.0 - 0.5*smoothstep(0.65, 1.0, uv.y))
	c := mix(Deep.rgb, Fog.rgb, min(cloud*m*(0.85+0.35*Bass), 1.0))
	// A broad glow behind it all, darker at the corners.
	g := length((uv - vec2(0.5, 0.42)) * vec2(1.0, 1.25))
	c *= 1.08 - 0.45*smoothstep(0.2, 0.85, g)
	// Specks of dust, rising with the fog, each glinting in its own time.
	cell := size.y / 22
	sp := (pos + vec2(0, t*cell*0.9)) / cell
	id := floor(sp)
	h := hash(id + vec2(3.1, 7.7))
	if h > 0.93 {
		at := vec2(hash(id+vec2(1.3, 2.9)), hash(id+vec2(8.1, 4.3)))*0.8 + 0.1
		d := length((fract(sp)-at)*cell) / max(cell*0.05, 0.7)
		glint := 0.5 + 0.5*sin(t*(1.5+2.0*h)+h*40.0)
		c += vec3(0.75, 0.82, 1.0) * exp(-d*d) * glint * (0.25 + 0.3*m)
	}
	return vec4(c, 1)
}
`

var fogShader = sync.OnceValue(func() *shader.Shader { return shader.MustCompile(fogSource) })

// stillSource paints the still backdrop: LabTop at the top, LabFoot at the
// foot, mixed in Oklab, with the cover's blurred wash through it and darker
// toward the foot, all before the dither that keeps its eight bits from
// banding. The dither is at the canvas's pixels, before their eight bits
// round the color: that is what keeps a smooth backdrop smooth, where
// dithering the canvas after it was rounded could only noise the bands.
//
// The two ends come in Oklab, converted once in Go: the shader would convert
// the same colors at every pixel otherwise.
const stillSource = `package main

var LabTop vec3
var LabFoot vec3
var Cover float
var CoverScale float
var CoverAt vec2

func srgb(c float) float {
	a := abs(c)
	if a <= 0.0031308 {
		return c * 12.92
	}
	return sign(c) * (1.055*pow(a, 1.0/2.4) - 0.055)
}

func fromOklab(lab vec3) vec3 {
	l := lab.x + 0.3963377774*lab.y + 0.2158037573*lab.z
	m := lab.x - 0.1055613458*lab.y - 0.0638541728*lab.z
	s := lab.x - 0.0894841775*lab.y - 1.2914855480*lab.z
	l = l * l * l
	m = m * m * m
	s = s * s * s
	return vec3(srgb(4.0767416621*l-3.3077115913*m+0.2309699292*s),
		srgb(-1.2684380046*l+2.6097574011*m-0.3413193965*s),
		srgb(-0.0041960863*l-0.7034186147*m+1.7076147010*s))
}

func hash(p vec2) float {
	p = fract(p * vec2(0.1031, 0.1030))
	p = p + dot(p, p+vec2(33.33, 33.33))
	return fract((p.x+p.y)*p.x)
}

func Fragment(dst vec4, pos vec2) vec4 {
	t := clamp(pos.y/rectSize().y, 0.0, 1.0)
	c := fromOklab(mix(LabTop, LabFoot, t))
	if Cover > 0.5 {
		// The cover, blurred, as a faint wash of its own colors. Its
		// image is sampled between its pixels, which keeps the wash smooth
		// where its own eight bits would step.
		cs := image0Size()
		uv := (pos - CoverAt) / CoverScale
		if uv.x >= 0.0 && uv.y >= 0.0 && uv.x < cs.x && uv.y < cs.y {
			c = mix(c, image0Sample(uv).rgb, 0.1)
		}
	}
	// Darker toward the foot, under the controls.
	a := 0.18 * clamp((t-0.55)/0.45, 0.0, 1.0)
	c = c * (1.0 - a)
	// Two hashes make the noise triangular, so that it settles around
	// the color instead of drifting from it.
	p := floor(vec2(dst.x, dst.y))
	n := hash(p) + hash(p+vec2(37.0, 17.0)) - 1.0
	d := n / 510.0
	return vec4(clamp(c.x+d, 0.0, 1.0), clamp(c.y+d, 0.0, 1.0), clamp(c.z+d, 0.0, 1.0), 1.0)
}
`

var stillShader = sync.OnceValue(func() *shader.Shader { return shader.MustCompile(stillSource) })

// backdropFrame is how often the fog is painted while it moves.
const backdropFrame = time.Second / 30

// backdropFade is how long another song's backdrop takes to fade in.
const backdropFade = 700 * time.Millisecond

// backdropCache is the backdrop as last painted, and where the fog's
// drift and its pulse are.
type backdropCache struct {
	canvas *ui.Canvas
	before *ui.Canvas // the song before's, fading out over it
	fadeAt time.Time  // when it began to
	art    *artwork
	tint   ui.Color
	fog    bool
	at     time.Time // when it was painted
	drift  float64   // the clouds' time, in seconds, which stops on pause
	speed  float64   // how fast drift goes: 1 playing, 0 paused
	level  barState
	last   time.Time
}

// backdrop paints the backdrop behind Now Playing, still or fog.
func (a *app) backdrop(c *ui.Context, art *artwork, tint ui.Color) {
	pl := a.pl
	fog, still := a.settings.Fog, c.Preferences().ReduceMotion
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
		// The fog paints into a canvas of its own, smaller on the CPU, and
		// those once a song: it stays still. The still backdrop is at the
		// window's DIPs on the GPU, where a DIP is two pixels of a Retina
		// screen, and an eighth of that on the CPU.
		gpu := p.Accelerated()
		div := float32(1)
		if !gpu {
			div = 8
		} else if fog {
			div = 2
		}
		cw := max(1, int(math.Ceil(float64(r.W/div))))
		ch := max(1, int(math.Ceil(float64(r.H/div))))
		bd := &a.backdropCache
		if bd.canvas != nil {
			if w, h := bd.canvas.Size(); w != cw || h != ch {
				bd.canvas, bd.before = nil, nil
			}
		}
		now := p.Now()
		if bd.canvas != nil && (bd.art != art || bd.tint != tint) {
			// Another song: the last one's stays as it was, fading out.
			bd.canvas, bd.before, bd.fadeAt = bd.before, bd.canvas, now
		}
		moving := fog && gpu && !still
		lv, live := pl.Levels()
		moved := bd.step(now, pl.Playing() && moving, lv, live && moving)
		due := now.Sub(bd.at) >= backdropFrame-2*time.Millisecond
		if bd.canvas == nil || bd.art != art || bd.tint != tint || bd.fog != fog || moved && due {
			if bd.canvas == nil {
				bd.canvas = ui.NewCanvas(cw, ch)
			}
			bd.art, bd.tint, bd.fog, bd.at = art, tint, fog, now
			bass := bd.level.group(0, 6)
			bd.canvas.Draw(p, func(cp *ui.Painter) {
				box := ui.Rect{W: float32(cw), H: float32(ch)}
				if fog {
					drawFog(cp, box, tint, float32(bd.drift), bass)
				} else {
					drawBackdrop(cp, box, art, tint)
				}
			})
		}
		p.Image(bd.canvas.Bitmap(), r, ui.FillBox)
		if bd.before != nil {
			if k := float32(now.Sub(bd.fadeAt)) / float32(backdropFade); k < 1 {
				// Eased out, as it fades.
				k = 1 - (1-k)*(1-k)*(1-k)
				p.ImageRegion(bd.before.Bitmap(), ui.Rect{}, r, ui.ImageOptions{Tint: white.Alpha(1 - k)})
				p.AnimationFrame()
			} else {
				bd.before = nil
			}
		}
		if moved {
			p.After(backdropFrame)
		}
	})
}

// step moves the drift and the pulse on to now, and reports whether
// either still moves.
func (bd *backdropCache) step(now time.Time, playing bool, lv [player.Bands]float32, live bool) bool {
	dt := now.Sub(bd.last).Seconds()
	if bd.last.IsZero() || dt > 0.2 {
		dt = 1.0 / 30
	}
	bd.last = now
	to := 0.0
	if playing {
		to = 1
	}
	// The drift eases in as the music plays, and glides to a stop.
	bd.speed += (to - bd.speed) * min(1, 2.5*dt)
	if math.Abs(to-bd.speed) < 0.005 {
		bd.speed = to
	}
	bd.drift += bd.speed * dt
	bd.level.step(now, lv, live)
	return bd.speed > 0 || bd.level.group(0, 6) > 0.004
}

// drawBackdrop paints the still backdrop in r.
func drawBackdrop(p *ui.Painter, r ui.Rect, art *artwork, tint ui.Color) {
	top, foot := backdropColors(tint)
	o := shader.Options{Uniforms: map[string]any{
		"LabTop": oklabColor(top), "LabFoot": oklabColor(foot), "Cover": float32(0),
	}}
	if art != nil && art.backdrop != nil {
		if w, h := art.backdrop.Size(); w > 0 && h > 0 {
			fw, fh := float32(w), float32(h)
			s := max(r.W/fw, r.H/fh)
			o.Images[0] = art.backdrop
			o.Uniforms["Cover"] = float32(1)
			o.Uniforms["CoverScale"] = s
			o.Uniforms["CoverAt"] = [2]float32{(r.W - fw*s) / 2, (r.H - fh*s) / 2}
		}
	}
	stillShader().Draw(p, r, o)
}

// oklabColor returns c in Oklab, as stillSource mixes its two ends.
func oklabColor(c ui.Color) [3]float32 {
	linear := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	r, g, b := linear(c.R), linear(c.G), linear(c.B)
	l := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	return [3]float32{
		float32(0.2104542553*l + 0.7936177850*m - 0.0040720468*s),
		float32(1.9779984951*l - 2.4285922050*m + 0.4505937099*s),
		float32(0.0259040371*l + 0.7827717662*m - 0.8086757660*s),
	}
}

// coverRect returns where an image of w×h goes to cover r, centered.
func coverRect(r ui.Rect, w, h float32) ui.Rect {
	s := max(r.W/w, r.H/h)
	return ui.Rect{X: r.X + (r.W-w*s)/2, Y: r.Y + (r.H-h*s)/2, W: w * s, H: h * s}
}

// backdropColors returns the top and the foot of the backdrop's gradient
// for a cover of tint: its hue, deep and rich, at the same depth whatever
// the cover, so that white reads over it, as NetEase's player paints it.
// A gray cover stays gray.
func backdropColors(tint ui.Color) (top, foot ui.Color) {
	h, s, _ := hsv(float64(tint.R)/255, float64(tint.G)/255, float64(tint.B)/255)
	if s > 0.12 {
		s = min(max(s*1.3, 0.45), 0.66)
	}
	return hsvColor(h, s, 0.35), hsvColor(h, min(s*1.05, 0.7), 0.24)
}

// drawFog paints the fog of tint in r at drift seconds, lit by bass.
func drawFog(p *ui.Painter, r ui.Rect, tint ui.Color, drift, bass float32) {
	deep, fog := fogColors(tint)
	fogShader().Draw(p, r, shader.Options{Uniforms: map[string]any{
		"Time": drift, "Bass": min(bass, 1), "Deep": deep, "Fog": fog,
	}})
}

// fogColors returns the deep shade behind the fog and the fog's own for a
// cover of tint: its hue, rich, at the same depths whatever the cover, so
// that white reads over them. A gray cover stays gray.
func fogColors(tint ui.Color) (deep, fog ui.Color) {
	h, s, _ := hsv(float64(tint.R)/255, float64(tint.G)/255, float64(tint.B)/255)
	if s > 0.12 {
		s = min(max(s*1.3, 0.6), 0.85)
	}
	return hsvColor(h, s, 0.3), hsvColor(h, s*0.6, 0.72)
}

// hsvColor is the color of hue h (degrees), saturation s and value v.
func hsvColor(h, s, v float64) ui.Color {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	var r, g, b float64
	switch {
	case h < 60:
		r, g = c, x
	case h < 120:
		r, g = x, c
	case h < 180:
		g, b = c, x
	case h < 240:
		g, b = x, c
	case h < 300:
		r, b = x, c
	default:
		r, b = c, x
	}
	m := v - c
	return ui.RGB(uint8((r+m)*255+0.5), uint8((g+m)*255+0.5), uint8((b+m)*255+0.5))
}

// backdropOver paints the backdrop again over band, a strip of win, the
// box the backdrop fills, as opaque at each height in band as alpha says
// for it, from 0 at its top to 1 at its foot: what moves under the strip
// fades into the backdrop with no edge, the very pixels around it where
// it is opaque.
func (a *app) backdropOver(p *ui.Painter, win, band ui.Rect, alpha func(t float32) float32) {
	bd := &a.backdropCache
	if bd.canvas == nil || win.W <= 0 || win.H <= 0 || band.H <= 0 {
		return
	}
	cw, ch := bd.canvas.Size()
	sx, sy := float32(cw)/win.W, float32(ch)/win.H
	// Its song's fading in over the song before's, as the backdrop.
	before := float32(0)
	if bd.before != nil {
		if k := float32(p.Now().Sub(bd.fadeAt)) / float32(backdropFade); k < 1 {
			before = (1 - k) * (1 - k) * (1 - k)
		}
	}
	const strip = 2
	for y := float32(0); y < band.H; y += strip {
		o := alpha((y + strip/2) / band.H)
		if o <= 0.004 {
			continue
		}
		dst := ui.Rect{X: band.X, Y: band.Y + y, W: band.W, H: min(strip, band.H-y)}
		src := ui.Rect{X: (dst.X - win.X) * sx, Y: (dst.Y - win.Y) * sy, W: dst.W * sx, H: dst.H * sy}
		p.ImageRegion(bd.canvas.Bitmap(), src, dst, ui.ImageOptions{Tint: white.Alpha(o)})
		if before > 0 {
			p.ImageRegion(bd.before.Bitmap(), src, dst, ui.ImageOptions{Tint: white.Alpha(o * before)})
		}
	}
}
