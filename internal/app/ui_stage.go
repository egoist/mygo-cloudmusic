package app

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// The stage shows lyrics as Folia's classic visualizer does: one line at a
// time, its words scattered a little, each springing in as it is sung,
// lighting up with a glow, then settling and drifting. Everything follows
// from where playback is, so seeking shows the same frame playing would.

// wordShape is where a word sits and leans, the same for a line each time;
// last says it ends its line.
type wordShape struct {
	x, y, rotate, scale, drift float32
	margin                     float32
	last                       bool
}

// lineRandom is a small random number generator seeded by a line and a word,
// so a line is laid out the same way every time it shows.
func lineRandom(seed, offset float64) float32 {
	x := math.Sin(seed+offset) * 10000
	return float32(x - math.Floor(x))
}

// springStep is how far a critically-ish damped spring (stiffness 200,
// damping 20, Folia's) has gone towards its target t seconds after it set
// off: it overshoots a little and settles in about half a second.
func springStep(t float32) float32 {
	if t <= 0 {
		return 0
	}
	e := float32(math.Exp(-10 * float64(t)))
	return 1 - e*(float32(math.Cos(10*float64(t)))+float32(math.Sin(10*float64(t))))
}

func clamp01(v float32) float32 { return min(1, max(0, v)) }

func lerp(a, b, t float32) float32 { return a + (b-a)*t }

func easeOutCubic(t float32) float32 {
	t = clamp01(t)
	return 1 - (1-t)*(1-t)*(1-t)
}

// stageShapes lays out the words of a line at font size f.
func stageShapes(c *ui.Context, l *lyrics.Line, f float32) []wordShape {
	seed := float64(l.At / time.Millisecond)
	shapes := make([]wordShape, len(l.Words))
	widths := make([]float32, len(l.Words))
	for i, w := range l.Words {
		widths[i], _ = c.MeasureText(0, ui.Span{Text: w.Text, Size: f, Weight: 800})
	}
	for i := range l.Words {
		ws := seed + float64(i)
		shapes[i] = wordShape{
			x:      (lineRandom(ws, 1) - 0.5) * 36,
			y:      (lineRandom(ws, 2) - 0.5) * 36,
			rotate: (lineRandom(ws, 3) - 0.5) * 10,
			scale:  1.1 + lineRandom(ws, 4)*0.2,
			drift:  (lineRandom(ws, 5) - 0.5) * 10,
		}
	}
	spaceWords(shapes, widths, f*0.08)
	if len(shapes) > 0 {
		shapes[len(shapes)-1].last = true
	}
	return shapes
}

// spaceWords leaves room after each word, so that a word swelling as it
// is sung does not run into the next: half of each one's growth, and how
// far it is moved towards the next, as Folia spaces them, and gap.
func spaceWords(shapes []wordShape, widths []float32, gap float32) {
	for i := range shapes {
		grow := widths[i] * (shapes[i].scale*1.4 - 1) / 2
		next, toward := float32(0), float32(0)
		if i+1 < len(shapes) {
			next = widths[i+1] * (shapes[i+1].scale*1.4 - 1) / 2
			toward = shapes[i].x - shapes[i+1].x
		}
		shapes[i].margin = max(2*gap, grow+next+toward+gap)
	}
}

// stage shows the lyrics of tr on the whole stage.
func (a *app) stage(c *ui.Context, tr *netease.Track, accent ui.Color) {
	pl := a.pl
	lines, loaded := pl.Lyrics()
	w, h := c.Size()
	f := min(64, max(36, w*0.045))
	box := ui.Box(c.Key("stage")).Grow(1)
	// In a quiet part a hot comment takes the middle, the lyrics nearly
	// gone under it (ui_quiet.go).
	dim, quiet := a.quietShown(c, box, tr, lines, loaded)
	if len(lines) == 0 {
		box.Children(func() {
			quietUnder(ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Center(), dim).Children(func() {
				if !loaded {
					stageSkeleton(c, npShimmer(c), f, w)
					return
				}
				ui.Text(c, "纯音乐，请欣赏").FontSize(28).FontWeight(800).TextColor(white.Alpha(0.4))
			})
			a.stageQuiet(c, dim, f, w)
		})
		return
	}
	pos := pl.Position()
	cur := lyrics.Index(lines, pos)
	// Under a hot comment there is nothing to move: the quiet asks for
	// the frame where it ends.
	if pl.Playing() && !(quiet && dim > 0.98) {
		// 倾诉 shows a part of a line a little before it is sung.
		lead := 80 * time.Millisecond
		if a.settings.StageStyle == "tilt" {
			lead = tiltLead
		}
		stageNextFrame(c, lines, cur, pos, lead)
	}

	box.Children(func() {
		defer a.stageQuiet(c, dim, f, w)
		quietUnder(ui.Box(c.Key("sung")).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough(), dim).Children(func() {
			// The line before goes as the new one comes.
			if cur > 0 {
				gone := float32((pos - lines[cur].At).Seconds()) / 0.4
				if gone < 1 {
					a.stageLine(c, &lines[cur-1], pos, f, w, h, 1-easeOutCubic(gone), accent)
				}
			}
			switch {
			case cur < 0:
				a.interlude(c, pos, 0, lines[0].At)
			case cur+1 < len(lines) && pos > lines[cur].End+time.Second && lines[cur+1].At-lines[cur].End > 4*time.Second:
				// A long break before the next line.
				a.interlude(c, pos, lines[cur].End, lines[cur+1].At)
			default:
				a.stageLine(c, &lines[cur], pos, f, w, h, 1, accent)
			}

			// What the line says in words the listener reads, and the line
			// coming next, along the foot.
			ui.Column(c).Absolute().Left(0).Right(0).Bottom(24).AlignItems(ui.Center).Gap(8).PassThrough().Children(func() {
				if cur >= 0 {
					switch text, kind := a.underLine(&lines[cur]); {
					case text == "":
					case kind == lyricRoma:
						ui.Text(c.Key("under"), text).FontSize(16).FontWeight(500).TextColor(white.Alpha(0.6)).TextAlign(ui.Center)
					default:
						ui.Text(c.Key("under"), text).FontSize(18).FontWeight(600).TextColor(white.Alpha(0.72)).TextAlign(ui.Center)
					}
				}
				if cur+1 < len(lines) {
					ui.Text(c.Key("next"), lines[cur+1].Text).FontSize(14).TextColor(white.Alpha(0.32)).TextAlign(ui.Center).SingleLine()
				}
			})
		})
	})
}

// stageNextFrame asks for the next frame of the stage as soon as it
// changes: at once while a word springs in or its glow changes, a few
// times a second while words only drift, and else when the next word is
// due, lead before it. Most of a song is the slow kind.
func stageNextFrame(c *ui.Context, lines []lyrics.Line, cur int, pos, lead time.Duration) {
	if cur < 0 {
		if len(lines) > 0 {
			c.After(min(100*time.Millisecond, lines[0].At-pos))
		}
		return
	}
	next, drifting := time.Duration(-1), false
	for i := max(cur-1, 0); i <= min(cur+1, len(lines)-1); i++ {
		for _, w := range lines[i].Words {
			start, end := w.Start-lead, max(w.End, w.Start+150*time.Millisecond)
			switch {
			case pos >= start && pos < max(start+700*time.Millisecond, end+900*time.Millisecond):
				// Springing in, filling, or its glow fading.
				c.AnimationFrame()
				return
			case pos >= end && pos < end+5*time.Second:
				drifting = true
			case pos < start && (next < 0 || start-pos < next):
				next = start - pos
			}
		}
	}
	wait := 100 * time.Millisecond // the drift, and the breathing of a break
	if !drifting && next > 0 {
		wait = min(next, 250*time.Millisecond)
	}
	c.After(wait)
}

// stageLine shows the words of l as they are at pos, the whole line at
// opacity fade.
func (a *app) stageLine(c *ui.Context, l *lyrics.Line, pos time.Duration, f, w, h, fade float32, accent ui.Color) {
	switch a.settings.StageStyle {
	case "partita":
		a.partitaLine(c, l, pos, f, h, fade, accent)
		return
	case "tilt":
		tiltLine(c, l, pos, w, fade, accent, a.settings.StageSparks)
		return
	}
	seed := float64(l.At / time.Millisecond)
	shapes := stageShapes(c, l, f)
	// Where the line sits: to one side or the middle, a little up or down.
	justify := []ui.Align{ui.Start, ui.Center, ui.End}[int(seed)%3]
	lift := []float32{-0.14, 0, 0.12}[int(seed*2)%3] * h

	ui.Row(c.Key(l.At)).Absolute().Top(0).Left(0).Right(0).Bottom(0).Padding(0, w*0.1).
		Wrap().Justify(justify).AlignContent(ui.Center).AlignItems(ui.Center).Translate(0, lift).
		Opacity(fade).PassThrough().Children(func() {
		for i, word := range l.Words {
			a.stageWord(c.Key(i), word, shapes[i], pos, f, accent)
		}
	})
}

// stageWord shows a word as it is at pos: waiting to be sung, being sung,
// or sung.
func (a *app) stageWord(c *ui.Context, word lyrics.Word, s wordShape, pos time.Duration, f float32, accent ui.Color) {
	const lookahead = 80 * time.Millisecond
	start := word.Start - lookahead
	end := max(word.End, word.Start+150*time.Millisecond)
	since := float32((pos - start).Seconds())
	after := float32((pos - end).Seconds())
	singing := float32(max(0.08, (end - word.Start).Seconds()))

	// Waiting: small, off to the side, turned and unseen.
	x := s.x + float32(math.Sin(float64(s.y)))*100
	y := s.y + float32(math.Cos(float64(s.x)))*50
	scale, rotate, opacity, blur := float32(0.5), s.rotate+20, float32(0), float32(10)
	color := white.Alpha(0.5)
	glow := float32(0)
	if since > 0 {
		// Sung: springing to its place, larger, sharp, filling with light.
		e := springStep(since)
		x, y = lerp(x, s.x, e), lerp(y, s.y, e)
		scale, rotate = lerp(0.5, s.scale*1.4, e), lerp(s.rotate+20, s.rotate, e)
		opacity = clamp01(since / 0.1)
		blur = 10 * (1 - clamp01(since/0.2))
		color = white.Alpha(lerp(0.5, 1, clamp01(since/singing)))
		glow = clamp01(since / max(0.08, 0.3*singing))
	}
	if after > 0 {
		// Past: back to its size, a little dimmer, drifting as it rests.
		q := easeOutCubic(after / 0.5)
		scale = lerp(s.scale*1.4, s.scale, q)
		opacity = lerp(1, 0.82, q)
		rotate = s.rotate + s.drift*clamp01(after/5)
		color = white.Alpha(lerp(1, 0.5, easeOutCubic(after/0.8)))
		glow = 1 - clamp01(after/0.9)
	}

	box := ui.Box(c).MarginEnd(s.margin).Translate(x, y).Rotate(rotate).Scale(scale, scale).Opacity(opacity)
	box.Children(func() {
		text := ui.Text(c.Key("body"), word.Text).FontSize(f).FontWeight(800).NoWrap().TextColor(color)
		if blur > 0.3 {
			text.Blur(blur)
		}
		if glow > 0.01 {
			// The glow: the word again, in light, blurred around it.
			ui.Text(c.Key("glow"), word.Text).Absolute().Top(0).Left(0).FontSize(f).FontWeight(800).NoWrap().
				TextColor(accent.Alpha(0.85 * glow)).Blur(f * 0.2).PassThrough()
		}
	})
	if a.settings.StageSparks {
		stageSparks(box, float32((pos - word.Start).Seconds()), float64(word.Start/time.Millisecond), s.last, accent)
	}
}

// interlude shows three dots breathing through a break, filling as the
// next line nears.
func (a *app) interlude(c *ui.Context, pos, from, to time.Duration) {
	progress := clamp01(float32((pos - from).Seconds() / max(0.1, (to-from).Seconds())))
	breath := 1 + 0.12*float32(math.Sin(float64(c.Now().UnixMilli())/420))
	ui.Row(c.Key("interlude")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Center().Gap(18).PassThrough().Children(func() {
		for i := range 3 {
			lit := clamp01(progress*3 - float32(i))
			ui.Box(c.Key(i)).Size(16, 16).Radius(8).Background(white.Alpha(0.2+0.7*lit)).Scale(breath, breath)
		}
	})
}
