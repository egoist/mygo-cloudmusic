package fnmusic

// Style covers. Most styles have no cover of their own, so each is drawn:
// the styles most libraries have get a scene of their own, and every other
// style a pattern and colors its name decides, the same each time it shows.

import (
	"hash/fnv"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// genreTheme is a drawn cover: the names it is for, its background from top
// to bottom, and the scene over it.
type genreTheme struct {
	name     string
	keys     []string
	from, to ui.Color
	draw     func(p *ui.Painter, g geo)
}

// geo places shapes in a cover, in fractions of its width and height.
type geo struct{ r ui.Rect }

func (g geo) x(f float32) float32 { return g.r.X + g.r.W*f }
func (g geo) y(f float32) float32 { return g.r.Y + g.r.H*f }
func (g geo) s(f float32) float32 { return g.r.W * f }

func (g geo) circle(p *ui.Painter, x, y, r float32, c ui.Color) {
	p.FillPath(new(ui.Path).Circle(g.x(x), g.y(y), g.s(r)), c)
}

func (g geo) ring(p *ui.Painter, x, y, r, w float32, c ui.Color) {
	p.StrokePath(new(ui.Path).Circle(g.x(x), g.y(y), g.s(r)), g.s(w), c)
}

// poly fills the polygon of points, given as x, y fractions in turn.
func (g geo) poly(p *ui.Painter, c ui.Color, pts ...float32) {
	path := new(ui.Path).MoveTo(g.x(pts[0]), g.y(pts[1]))
	for i := 2; i+1 < len(pts); i += 2 {
		path.LineTo(g.x(pts[i]), g.y(pts[i+1]))
	}
	p.FillPath(path.Close(), c)
}

func (g geo) line(p *ui.Painter, x0, y0, x1, y1, w float32, c ui.Color) {
	p.Line(g.x(x0), g.y(y0), g.x(x1), g.y(y1), g.s(w), c)
}

func (g geo) rect(p *ui.Painter, x, y, w, h, radius float32, c ui.Color) {
	p.Fill(ui.Rect{X: g.x(x), Y: g.y(y), W: g.s(w), H: g.r.H * h}, c, g.s(radius))
}

func sin(a float64) float32 { return float32(math.Sin(a)) }
func cos(a float64) float32 { return float32(math.Cos(a)) }

var white = ui.RGB(255, 255, 255)

// genreThemeFor is the drawn cover of a style, by its name, or nil for one
// to make a pattern for. The first theme a name has a key of wins, so the
// narrower ones, as J-Pop, come before what they are part of, Pop.
func genreThemeFor(name string) *genreTheme {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return nil
	}
	words := strings.FieldsFunc(n, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '&' })
	for i := range genreThemes {
		for _, k := range genreThemes[i].keys {
			if genreKeyIn(n, words, k) {
				return &genreThemes[i]
			}
		}
	}
	return nil
}

// genreKeyIn reports whether a style's name n has key k. A short Latin key,
// as "ost" or "rap", has to be a word of its own, or "post-rock" would be a
// soundtrack; longer keys and Chinese ones are found anywhere in it.
func genreKeyIn(n string, words []string, k string) bool {
	if len(k) > 3 || k[0] >= utf8.RuneSelf {
		return strings.Contains(n, k)
	}
	for _, w := range words {
		if w == k {
			return true
		}
	}
	return false
}

// drawGenreArt paints the cover of the style called name into r, its
// corners rounded by radius.
func drawGenreArt(p *ui.Painter, r ui.Rect, name string, radius float32) {
	g := geo{r}
	if t := genreThemeFor(name); t != nil {
		p.FillGradient(r, ui.LinearGradient{From: t.from, To: t.to, Angle: 160, Oklab: true}, radius)
		p.Clip(r, radius, func() { t.draw(p, g) })
		return
	}
	drawGeneratedArt(p, g, name, radius)
}

// genreArt is a box of size, its corners rounded by radius, showing the
// cover of the style called name.
func genreArt(c *ui.Context, name string, size, radius float32) ui.Element {
	return ui.Box(c).Size(size, size).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
		drawGenreArt(p, r, name, radius)
	})
}

// genreSeed is a number of a style's name, the same on every machine.
func genreSeed(name string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(strings.TrimSpace(name))))
	return h.Sum32()
}

// monogram is the first letter of a name that is one, upper case.
func monogram(name string) string {
	for _, r := range strings.TrimSpace(name) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return string(unicode.ToUpper(r))
		}
	}
	if r, _ := utf8.DecodeRuneInString(name); r != utf8.RuneError {
		return string(r)
	}
	return "♪"
}
