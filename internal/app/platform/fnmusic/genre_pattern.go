package fnmusic

// Generated style covers, for the styles without a drawn scene. A style's
// name picks one of the layouts and its colors, turn and rhythm, so each
// style looks its own and the same wherever it shows.

import (
	"math"

	"github.com/egoist/mygo/ui"
)

// genreRand draws numbers from a style's seed: the same name, the same
// numbers, in turn.
type genreRand struct{ s uint32 }

func (r *genreRand) next() uint32 {
	// xorshift32: small, and the same everywhere.
	r.s ^= r.s << 13
	r.s ^= r.s >> 17
	r.s ^= r.s << 5
	return r.s
}

// f is a number from 0 to 1.
func (r *genreRand) f() float32 { return float32(r.next()%10000) / 10000 }

// between is a number from lo to hi.
func (r *genreRand) between(lo, hi float32) float32 { return lo + (hi-lo)*r.f() }

// genrePalette is a style's colors: a deep ground, a lighter one, and two
// accents, from a hue its name gives.
type genrePalette struct{ deep, ground, accent, spark ui.Color }

func paletteOf(r *genreRand) genrePalette {
	hue := r.between(0, 360)
	turn := r.between(30, 150) // how far the accent is around the wheel
	return genrePalette{
		deep:   ui.Oklch(0.28, 0.09, hue),
		ground: ui.Oklch(0.46, 0.13, hue+18),
		accent: ui.Oklch(0.74, 0.15, hue+turn),
		spark:  ui.Oklch(0.9, 0.07, hue+turn+40),
	}
}

// genreLayouts are the patterns a generated cover can have.
var genreLayouts = []func(p *ui.Painter, g geo, r *genreRand, c genrePalette){
	layoutOrbits, layoutStripes, layoutGrid, layoutWaves, layoutShards,
}

func drawGeneratedArt(p *ui.Painter, g geo, name string, radius float32) {
	seed := genreSeed(name)
	r := &genreRand{s: seed | 1}
	c := paletteOf(r)
	p.FillGradient(g.r, ui.LinearGradient{From: c.ground, To: c.deep, Angle: r.between(120, 240), Oklab: true}, radius)
	p.Clip(g.r, radius, func() {
		genreLayouts[seed%uint32(len(genreLayouts))](p, g, r, c)
		// The style's first letter, large and faint in a corner, tells
		// apart two styles that came out alike.
		p.RichText(g.x(0.08), g.y(0.62), g.s(0.9), ui.Span{
			Text: monogram(name), Size: g.s(0.34), Weight: 800, Color: white.Alpha(0.16),
		})
	})
}

// layoutOrbits is a planet and the rings around it.
func layoutOrbits(p *ui.Painter, g geo, r *genreRand, c genrePalette) {
	cx, cy := r.between(0.45, 0.7), r.between(0.3, 0.5)
	for i := 0; i < 4; i++ {
		g.ring(p, cx, cy, 0.16+float32(i)*r.between(0.08, 0.12), 0.006, c.spark.Alpha(0.35-float32(i)*0.06))
	}
	g.circle(p, cx, cy, r.between(0.12, 0.18), c.accent)
	a := float64(r.between(0, 2*math.Pi))
	g.circle(p, cx+0.3*cos(a), cy+0.3*sin(a), 0.035, c.spark)
}

// layoutStripes is bands across the cover at a turn.
func layoutStripes(p *ui.Painter, g geo, r *genreRand, c genrePalette) {
	angle := r.between(-40, 40)
	p.Transform(ui.Rotation(angle), g.x(0.5), g.y(0.5), func() {
		y := float32(-0.3)
		for i := 0; y < 1.3; i++ {
			h := r.between(0.04, 0.14)
			col := c.accent.Alpha(r.between(0.25, 0.75))
			if i%3 == 2 {
				col = c.spark.Alpha(0.5)
			}
			g.rect(p, -0.3, y, 1.6, h, 0, col)
			y += h + r.between(0.03, 0.1)
		}
	})
}

// layoutGrid is dots of a grid, some lit and larger.
func layoutGrid(p *ui.Painter, g geo, r *genreRand, c genrePalette) {
	n := 5 + int(r.next()%3)
	step := 1 / float32(n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			x, y := (float32(i)+0.5)*step, (float32(j)+0.5)*step
			v := r.f()
			switch {
			case v > 0.86:
				g.circle(p, x, y, step*0.38, c.spark)
			case v > 0.55:
				g.circle(p, x, y, step*0.26, c.accent.Alpha(0.8))
			default:
				g.circle(p, x, y, step*0.1, white.Alpha(0.25))
			}
		}
	}
}

// layoutWaves is lines rolling across, as a sound's shape.
func layoutWaves(p *ui.Painter, g geo, r *genreRand, c genrePalette) {
	freq, phase := r.between(1.5, 3.5), float64(r.between(0, 6))
	for k := 0; k < 6; k++ {
		base := 0.25 + float32(k)*0.1
		amp := r.between(0.03, 0.09)
		path := new(ui.Path)
		for i := 0; i <= 40; i++ {
			t := float32(i) / 40
			y := base + amp*sin(float64(t*freq)*2*math.Pi+phase+float64(k)*0.6)
			if i == 0 {
				path.MoveTo(g.x(t), g.y(y))
			} else {
				path.LineTo(g.x(t), g.y(y))
			}
		}
		col := c.accent.Alpha(0.85 - float32(k)*0.1)
		if k == 2 {
			col = c.spark
		}
		p.StrokePath(path, g.s(0.012), col)
	}
}

// layoutShards is triangles pieced into a corner.
func layoutShards(p *ui.Painter, g geo, r *genreRand, c genrePalette) {
	for i := 0; i < 7; i++ {
		x, y := r.between(0.1, 0.95), r.between(0.05, 0.75)
		s := r.between(0.15, 0.4)
		col := c.accent.Alpha(r.between(0.35, 0.8))
		if i == 3 {
			col = c.spark.Alpha(0.85)
		}
		g.poly(p, col, x, y, x+s*r.between(-0.6, 0.6), y+s, x-s*r.between(0.3, 0.9), y+s*r.between(0.3, 0.9))
	}
}
