package app

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/player"
)

// palette holds the app's colors, for the light and the dark appearance.
type palette struct {
	dark      bool
	bg        ui.Color // the content's background
	sidebar   ui.Color // under the sidebar where no material shows
	text      ui.Color
	muted     ui.Color
	faint     ui.Color
	hairline  ui.Color
	hover     ui.Color
	press     ui.Color
	accent    ui.Color
	onAccent  ui.Color
	cardShade ui.Color
}

func paletteFor(t *ui.Theme) palette {
	if t.Dark {
		return palette{
			dark:      true,
			bg:        ui.Hex("#141416"),
			sidebar:   ui.Hex("#1c1c1f"),
			text:      ui.Hex("#f5f5f7"),
			muted:     ui.RGBA(235, 235, 245, 0.56),
			faint:     ui.RGBA(235, 235, 245, 0.3),
			hairline:  ui.RGBA(255, 255, 255, 0.07),
			hover:     ui.RGBA(255, 255, 255, 0.06),
			press:     ui.RGBA(255, 255, 255, 0.1),
			accent:    ui.Oklch(0.66, 0.2, 22),
			onAccent:  ui.Hex("#ffffff"),
			cardShade: ui.RGBA(0, 0, 0, 0.45),
		}
	}
	return palette{
		bg:        ui.Hex("#ffffff"),
		sidebar:   ui.Hex("#f4f4f6"),
		text:      ui.Hex("#1d1d1f"),
		muted:     ui.RGBA(60, 60, 67, 0.62),
		faint:     ui.RGBA(60, 60, 67, 0.32),
		hairline:  ui.RGBA(0, 0, 0, 0.07),
		hover:     ui.RGBA(0, 0, 0, 0.04),
		press:     ui.RGBA(0, 0, 0, 0.08),
		accent:    ui.Oklch(0.6, 0.21, 24),
		onAccent:  ui.Hex("#ffffff"),
		cardShade: ui.RGBA(0, 0, 0, 0.16),
	}
}

// iconButton is a borderless button showing an icon, which a soft circle
// marks under the pointer and which gives a little as it is pressed.
func iconButton(c *ui.Context, icon *ui.SVG, size float32, color ui.Color, label string) ui.Element {
	b := roundButton(c, size, color, label)
	b.Children(func() { ui.Icon(c, icon).Size(size, size) })
	return b
}

// playButton is the icon button that plays and pauses, its icon turning
// from one into the other.
func playButton(c *ui.Context, playing bool, size float32, color ui.Color) ui.Element {
	b := roundButton(c, size, color, "播放/暂停")
	b.Children(func() { playPause(c, b, playing, size) })
	return b
}

// playPause draws the icon of b, a button that plays and pauses: as it
// goes from one to the other, the icon going shrinks and fades as the one
// coming grows in, with a little spring.
func playPause(c *ui.Context, b ui.Element, playing bool, size float32) {
	to := float32(0)
	if playing {
		to = 1
	}
	v := b.AnimateTo("playing", to, ui.Animation{Spring: ui.SpringOptions{Duration: 340 * time.Millisecond, Bounce: 0.3}})
	ui.Box(c).Size(size, size).Children(func() {
		for _, icon := range []struct {
			svg   *ui.SVG
			shown float32
		}{{iconPlay, 1 - v}, {iconPause, v}} {
			if icon.shown <= 0.01 {
				continue
			}
			s := 0.4 + 0.6*icon.shown
			ui.Icon(c, icon.svg).Absolute().Top(0).Left(0).Size(size, size).Scale(s, s).Opacity(min(icon.shown, 1))
		}
	})
}

// roundButton is the button of iconButton, without its icon.
func roundButton(c *ui.Context, size float32, color ui.Color, label string) ui.Element {
	b := ui.ButtonBase(c).Size(size+16, size+16).Center().Radius(999).Label(label).TextColor(color)
	bg := ui.Transparent
	if b.Hovered() {
		bg = ui.RGBA(128, 128, 128, 0.14)
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	pose := ui.Pose{}
	if b.Pressed() {
		pose.Scale = 0.88
	}
	b.AnimatePose(pose, ui.Snappy)
	return b
}

// pillButton is a rounded button with an icon and a label.
func pillButton(c *ui.Context, icon *ui.SVG, label string, bg, fg ui.Color) ui.Element {
	b := ui.ButtonBase(c).Height(36).PaddingX(18).Gap(8).Radius(999).AlignItems(ui.Center).TextColor(fg)
	fill := bg
	if b.Hovered() {
		fill = bg.Mix(ui.RGB(255, 255, 255), 0.12)
	}
	b.Background(b.AnimateColor("bg", fill, ui.Animation{Duration: 120 * time.Millisecond}))
	pose := ui.Pose{}
	if b.Pressed() {
		pose.Scale = 0.96
	}
	b.AnimatePose(pose, ui.Snappy)
	b.Children(func() {
		if icon != nil {
			ui.Icon(c, icon).Size(14, 14)
		}
		ui.Text(c, label).FontSize(14).FontWeight(600).SingleLine()
	})
	return b
}

// cover shows an artwork, or a placeholder of the same size while it loads.
func (a *app) cover(c *ui.Context, url string, px int, size, radius float32) ui.Element {
	p := a.pal
	box := ui.Box(c).Size(size, size).Radius(radius).Center()
	bmp := bitmap(a.art.get(netease.Sized(url, px), false))
	in := artIn(box, bmp)
	if in < 1 {
		box.Background(p.hover)
	}
	box.Children(func() {
		if in < 1 {
			ui.Icon(c, iconNote).Size(size*0.3, size*0.3).TextColor(p.faint).Opacity(1 - in)
		}
		artImage(c, bmp, radius, in)
	})
	return box
}

// bitmap returns the cover of art, nil while it loads.
func bitmap(art *artwork) *ui.Bitmap {
	if art == nil {
		return nil
	}
	return art.bmp
}

// backdrop returns the blurred cover of art, nil while it loads.
func backdrop(art *artwork) *ui.Bitmap {
	if art == nil {
		return nil
	}
	return art.backdrop
}

// artFade is how an artwork fades in over its placeholder.
var artFade = ui.Animation{Duration: 320 * time.Millisecond}

// artIn returns how far bmp has faded in on el, the element that shows it
// or its placeholder: over a moment once it loads while el shows, and at
// once if it was ready as el first showed.
func artIn(el ui.Element, bmp *ui.Bitmap) float32 {
	if bmp == nil {
		return el.Jump("art", 0)
	}
	return el.AnimateTo("art", 1, artFade)
}

// artImage fills its parent with bmp, its corners rounded by radius, as
// far faded in as in.
func artImage(c *ui.Context, bmp *ui.Bitmap, radius, in float32) {
	if bmp == nil || in <= 0 {
		return
	}
	img := ui.Image(c, bmp).Absolute().Top(0).Left(0).Right(0).Bottom(0).Fit(ui.Cover).Radius(radius)
	if in < 1 {
		img.Opacity(in)
	}
}

// clock formats a duration as minutes and seconds.
func clock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d.Round(time.Second) / time.Second)
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// plays formats a play count as people in China read it.
func plays(n float64) string {
	switch {
	case n >= 1e8:
		return fmt.Sprintf("%.1f亿", n/1e8)
	case n >= 1e4:
		return fmt.Sprintf("%.0f万", n/1e4)
	default:
		return fmt.Sprintf("%.0f", n)
	}
}

// greeting returns what to say at this hour.
func greeting(now time.Time) string {
	switch h := now.Hour(); {
	case h < 5:
		return "夜深了"
	case h < 11:
		return "早上好"
	case h < 13:
		return "中午好"
	case h < 18:
		return "下午好"
	default:
		return "晚上好"
	}
}

// bars draws n bars of an equalizer in r, from the levels of what plays,
// smoothed in st between frames; idle bars rest low.
type barState struct {
	h    [player.Bands]float32
	last time.Time
}

func (st *barState) step(now time.Time, target [player.Bands]float32, live bool) {
	dt := float32(now.Sub(st.last).Seconds())
	if st.last.IsZero() || dt > 0.2 {
		dt = 0.016
	}
	st.last = now
	for i := range st.h {
		tg := target[i]
		if !live {
			tg = 0
		}
		k := float32(18) // rises fast
		if tg < st.h[i] {
			k = 6 // falls slowly
		}
		st.h[i] += (tg - st.h[i]) * min(1, k*dt)
	}
}

// group returns the mean of bands [from, to).
func (st *barState) group(from, to int) float32 {
	var s float32
	for i := from; i < to; i++ {
		s += st.h[i]
	}
	return s / float32(to-from)
}

func drawBars(p *ui.Painter, r ui.Rect, st *barState, n int, color ui.Color, minH float32) {
	gap := r.W / float32(n) * 0.38
	w := (r.W - gap*float32(n-1)) / float32(n)
	per := player.Bands / n
	for i := range n {
		// Shape the response: low bands are loud already.
		v := st.group(i*per, (i+1)*per)
		v = v * v * 1.15
		h := max(minH, min(r.H, r.H*v))
		x := r.X + float32(i)*(w+gap)
		p.Fill(ui.Rect{X: x, Y: r.Y + (r.H-h)/2, W: w, H: h}, color, w/2)
	}
}

// segmented is a segmented control as AppKit draws one: segments of one
// width in a small gray track, divided by hairlines, the one chosen raised
// on a capsule that slides to the next as it changes. It reports whether
// the user chose another.
func segmented(c *ui.Context, selected *int, labels ...string) bool {
	return segmentedIn(c, selected, 0, labels...)
}

// segmentedIn is segmented stretched to total DIPs across, if that is more
// than its labels take.
func segmentedIn(c *ui.Context, selected *int, total float32, labels ...string) bool {
	t := c.Theme()
	const height, inset, radius = 24, 2, 6
	width := float32(0)
	for _, l := range labels {
		w, _ := c.MeasureText(0, ui.Span{Text: l, Size: 13})
		width = max(width, w+26)
	}
	width = max(width, (total-2*inset)/float32(len(labels)))
	track, capsule, divider := ui.RGBA(0, 0, 0, 0.06), ui.RGB(255, 255, 255), ui.RGBA(0, 0, 0, 0.13)
	if t.Dark {
		track, capsule, divider = ui.RGBA(255, 255, 255, 0.08), ui.RGBA(255, 255, 255, 0.24), ui.RGBA(255, 255, 255, 0.13)
	}
	before := *selected
	parts := ui.SegmentedBase(c, selected, len(labels))
	at := parts.Track.AnimateTo("at", float32(*selected), ui.Animation{Spring: ui.SpringOptions{Duration: 280 * time.Millisecond, Bounce: 0.08}})
	n := len(labels)
	dark := t.Dark
	parts.Track.Height(height).Padding(inset).Radius(radius).Background(track).Draw(func(p *ui.Painter, r ui.Rect) {
		// Hairlines between segments, but beside the capsule.
		for i := 1; i < n; i++ {
			if d := float32(i) - at; d > -1 && d < 2 {
				continue
			}
			x := r.X + inset + float32(i)*width
			p.Fill(ui.Rect{X: x - 0.5, Y: r.Y + 6, W: 1, H: r.H - 12}, divider, 0)
		}
		pill := ui.Rect{X: r.X + inset + at*width, Y: r.Y + inset, W: width, H: r.H - 2*inset}
		if !dark {
			p.Shadow(pill, radius-1, 0, 0.5, 1, 0, ui.RGBA(0, 0, 0, 0.2))
			p.Shadow(pill, radius-1, 0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.06))
		}
		p.Fill(pill, capsule, radius-1)
	}).Children(func() {
		for i, l := range labels {
			seg := parts.Segment(i).Width(width).Height(height - 2*inset).Center().Radius(radius - 1)
			if seg.Pressed() && i != *selected {
				seg.Background(divider)
			}
			seg.Children(func() { ui.Text(c, l).FontSize(13).TextColor(t.Text).SingleLine() })
		}
	})
	return *selected != before
}

// paged is how a list loaded a page at a time is coming along.
type paged struct {
	loaded  bool // whether the first page came
	loading bool
	more    bool   // whether there is a page after those loaded
	err     string // why the last page didn't load
}

// pageFoot ends a list loaded a page at a time, asking next for the next
// page once it is within a screen of coming into view: where the last
// failed, it says so, with a button to try again.
func (a *app) pageFoot(c *ui.Context, pg *paged, next func()) {
	foot := ui.Row(c.Key("foot")).Height(56).Gap(12).Center()
	foot.Children(func() {
		switch {
		case pg.loading:
			ui.Text(c, "正在加载…").FontSize(12.5).TextColor(a.pal.muted)
		case pg.err != "":
			ui.Text(c, pg.err).FontSize(12.5).TextColor(a.pal.muted)
			if ui.Button(c.Key("retry"), "重试").Clicked() {
				next()
			}
		}
	})
	_, h := c.Size()
	if b := foot.Bounds(); pg.more && !pg.loading && pg.err == "" && b.W > 0 && b.Y < 2*h {
		next()
	}
}
