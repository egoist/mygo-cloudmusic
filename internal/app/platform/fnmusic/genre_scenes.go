package fnmusic

// The drawn scenes of the styles most libraries have. Keys are matched
// within a style's name in lower case, in order, so a narrower style's keys
// come before those of the style it is part of.

import (
	"math"

	"github.com/egoist/mygo/ui"
)

var genreThemes = []genreTheme{
	{name: "jpop", keys: []string{"j-pop", "jpop", "日本", "日语", "anime", "动漫", "二次元", "acg"},
		from: ui.Hex("#ffb7c5"), to: ui.Hex("#7b5ea7"), draw: sceneSakura},
	{name: "kpop", keys: []string{"k-pop", "kpop", "韩"},
		from: ui.Hex("#ff7eb3"), to: ui.Hex("#5a3fd1"), draw: sceneStage},
	{name: "chinese", keys: []string{"华语", "国语", "中文", "c-pop", "cpop", "mandopop", "粤语", "cantopop", "流行"},
		from: ui.Hex("#f4a259"), to: ui.Hex("#8c2f39"), draw: sceneLantern},
	{name: "folk", keys: []string{"民谣", "folk", "acoustic", "原声", "country", "乡村", "bluegrass"},
		from: ui.Hex("#e9c46a"), to: ui.Hex("#6b4226"), draw: sceneGuitar},
	{name: "classical", keys: []string{"classical", "古典", "orchestra", "交响", "symphony", "opera", "歌剧", "baroque", "chamber", "室内乐", "piano", "钢琴"},
		from: ui.Hex("#f1e3c6"), to: ui.Hex("#5c4b3b"), draw: sceneClassical},
	{name: "chinese-traditional", keys: []string{"古风", "国风", "中国风", "民族", "戏曲", "chinese traditional"},
		from: ui.Hex("#d8e2dc"), to: ui.Hex("#3d5a4f"), draw: sceneMountains},
	{name: "jazz", keys: []string{"jazz", "爵士", "swing", "bossa"},
		from: ui.Hex("#2a4b7c"), to: ui.Hex("#0b1a30"), draw: sceneJazz},
	{name: "blues", keys: []string{"blues", "布鲁斯", "蓝调"},
		from: ui.Hex("#3a6ea5"), to: ui.Hex("#10203c"), draw: sceneBlues},
	{name: "soul", keys: []string{"soul", "灵魂", "r&b", "rnb", "节奏布鲁斯", "funk", "放克", "motown", "gospel"},
		from: ui.Hex("#c0587e"), to: ui.Hex("#3b1c32"), draw: sceneVinyl},
	{name: "metal", keys: []string{"metal", "金属"},
		from: ui.Hex("#4a4a4a"), to: ui.Hex("#0d0d0d"), draw: sceneMetal},
	{name: "punk", keys: []string{"punk", "朋克", "hardcore", "grunge"},
		from: ui.Hex("#e63946"), to: ui.Hex("#1d1d1d"), draw: scenePunk},
	{name: "rock", keys: []string{"rock", "摇滚", "alternative", "另类", "indie", "独立"},
		from: ui.Hex("#ef476f"), to: ui.Hex("#26132b"), draw: sceneRock},
	{name: "hiphop", keys: []string{"hip-hop", "hip hop", "hiphop", "rap", "说唱", "嘻哈", "trap"},
		from: ui.Hex("#ffd166"), to: ui.Hex("#2b2d42"), draw: sceneHipHop},
	{name: "electronic", keys: []string{"electronic", "电子", "edm", "house", "techno", "trance", "dance", "舞曲", "dubstep", "drum", "电音", "synth"},
		from: ui.Hex("#00f5d4"), to: ui.Hex("#3a0ca3"), draw: sceneSynth},
	{name: "ambient", keys: []string{"ambient", "氛围", "new age", "新世纪", "轻音乐", "lounge", "chill", "lo-fi", "lofi", "meditation", "冥想"},
		from: ui.Hex("#a8dadc"), to: ui.Hex("#1d3557"), draw: sceneAmbient},
	{name: "reggae", keys: []string{"reggae", "雷鬼", "ska", "latin", "拉丁", "salsa", "world", "世界"},
		from: ui.Hex("#2a9d8f"), to: ui.Hex("#e76f51"), draw: sceneSun},
	{name: "soundtrack", keys: []string{"soundtrack", "原声带", "ost", "影视", "film", "movie", "score", "配乐", "game", "游戏"},
		from: ui.Hex("#264653"), to: ui.Hex("#0b0f14"), draw: sceneFilm},
	{name: "children", keys: []string{"儿歌", "children", "kids", "童谣", "儿童"},
		from: ui.Hex("#90e0ef"), to: ui.Hex("#ffd6a5"), draw: sceneBalloons},
	{name: "pop", keys: []string{"pop"},
		from: ui.Hex("#ff8fab"), to: ui.Hex("#6a4c93"), draw: scenePop},
}

// sceneSakura is a blossom branch under a pale sun.
func sceneSakura(p *ui.Painter, g geo) {
	g.circle(p, 0.7, 0.3, 0.2, white.Alpha(0.35))
	g.line(p, 0.0, 0.85, 0.55, 0.45, 0.025, ui.Hex("#4a2c2a"))
	g.line(p, 0.3, 0.63, 0.45, 0.75, 0.015, ui.Hex("#4a2c2a"))
	for _, b := range [][2]float32{{0.55, 0.45}, {0.42, 0.52}, {0.3, 0.6}, {0.47, 0.76}, {0.2, 0.68}, {0.62, 0.4}} {
		for k := 0; k < 5; k++ {
			a := float64(k)*2*math.Pi/5 - math.Pi/2
			g.circle(p, b[0]+0.035*cos(a), b[1]+0.035*sin(a), 0.03, ui.Hex("#ffe5ec"))
		}
		g.circle(p, b[0], b[1], 0.015, ui.Hex("#ff8fab"))
	}
	for _, d := range [][2]float32{{0.75, 0.62}, {0.85, 0.78}, {0.62, 0.88}} {
		g.circle(p, d[0], d[1], 0.018, ui.Hex("#ffe5ec").Alpha(0.8))
	}
}

// sceneStage is spotlights crossing over a stage.
func sceneStage(p *ui.Painter, g geo) {
	for i, x := range []float32{0.15, 0.5, 0.85} {
		col := []ui.Color{ui.Hex("#ffd6ff"), ui.Hex("#bde0fe"), ui.Hex("#ffc8dd")}[i]
		g.poly(p, col.Alpha(0.28), x-0.03, 0, x+0.03, 0, 0.5+(x-0.5)*0.3+0.18, 0.85, 0.5+(x-0.5)*0.3-0.18, 0.85)
	}
	g.rect(p, 0, 0.82, 1, 0.18, 0, ui.RGBA(0, 0, 0, 0.35))
	for i := 0; i < 9; i++ {
		x := 0.08 + float32(i)*0.105
		g.circle(p, x, 0.9+0.02*sin(float64(i)), 0.04, ui.RGBA(0, 0, 0, 0.5))
	}
}

// sceneLantern is red lanterns hanging in the evening.
func sceneLantern(p *ui.Painter, g geo) {
	g.line(p, 0, 0.12, 1, 0.2, 0.008, ui.Hex("#3b1f1f"))
	for _, l := range [][3]float32{{0.25, 0.14, 0.13}, {0.6, 0.18, 0.17}, {0.85, 0.2, 0.1}} {
		x, top, r := l[0], l[1], l[2]
		g.line(p, x, top, x, top+0.08, 0.006, ui.Hex("#3b1f1f"))
		cy := top + 0.08 + r
		g.circle(p, x, cy, r+0.04, ui.Hex("#ffb703").Alpha(0.22))
		p.FillPath(new(ui.Path).MoveTo(g.x(x-r), g.y(cy)).
			CubeTo(g.x(x-r), g.y(cy-r*1.1), g.x(x+r), g.y(cy-r*1.1), g.x(x+r), g.y(cy)).
			CubeTo(g.x(x+r), g.y(cy+r*1.1), g.x(x-r), g.y(cy+r*1.1), g.x(x-r), g.y(cy)).Close(), ui.Hex("#d62828"))
		g.rect(p, x-r*0.35, cy-r*0.95, r*0.7, 0.025, 0.005, ui.Hex("#fcbf49"))
		g.rect(p, x-r*0.35, cy+r*0.8, r*0.7, 0.025, 0.005, ui.Hex("#fcbf49"))
		g.line(p, x, cy+r*0.9, x, cy+r*0.9+0.08, 0.008, ui.Hex("#fcbf49"))
	}
}

// sceneGuitar is an acoustic guitar against a low sun.
func sceneGuitar(p *ui.Painter, g geo) {
	g.circle(p, 0.25, 0.3, 0.15, ui.Hex("#fff3b0").Alpha(0.6))
	wood, dark := ui.Hex("#bc6c25"), ui.Hex("#3d2314")
	p.Transform(ui.Rotation(-30), g.x(0.55), g.y(0.6), func() {
		g.circle(p, 0.55, 0.72, 0.17, wood)
		g.circle(p, 0.55, 0.52, 0.13, wood)
		g.circle(p, 0.55, 0.62, 0.05, dark)
		g.rect(p, 0.53, 0.05, 0.04, 0.45, 0.01, dark)
		g.rect(p, 0.515, 0.0, 0.07, 0.08, 0.015, dark)
		for k := 0; k < 4; k++ {
			x := 0.537 + float32(k)*0.009
			g.line(p, x, 0.04, x, 0.78, 0.0025, ui.Hex("#f2e8cf"))
		}
	})
}

// sceneClassical is a violin's scroll and a staff of notes.
func sceneClassical(p *ui.Painter, g geo) {
	ink := ui.Hex("#3b2f2f")
	for k := 0; k < 5; k++ {
		y := 0.3 + float32(k)*0.05
		g.line(p, 0.08, y, 0.92, y, 0.004, ink.Alpha(0.5))
	}
	for i, n := range [][2]float32{{0.2, 0.42}, {0.35, 0.37}, {0.5, 0.45}, {0.65, 0.32}, {0.8, 0.4}} {
		g.circle(p, n[0], n[1], 0.028, ink)
		g.line(p, n[0]+0.025, n[1], n[0]+0.025, n[1]-0.16, 0.007, ink)
		if i%2 == 1 {
			g.line(p, n[0]+0.025, n[1]-0.16, n[0]+0.09, n[1]-0.12, 0.012, ink)
		}
	}
	g.circle(p, 0.8, 0.8, 0.12, ui.Hex("#8c5a3c"))
	g.circle(p, 0.8, 0.8, 0.06, ui.Hex("#f1e3c6"))
	g.ring(p, 0.8, 0.8, 0.09, 0.012, ui.Hex("#5c3a21"))
}

// sceneMountains is ink mountains under a moon.
func sceneMountains(p *ui.Painter, g geo) {
	g.circle(p, 0.72, 0.25, 0.1, ui.Hex("#fefae0").Alpha(0.85))
	g.poly(p, ui.Hex("#52796f").Alpha(0.55), 0, 0.75, 0.2, 0.42, 0.38, 0.62, 0.55, 0.35, 0.8, 0.6, 1, 0.45, 1, 1, 0, 1)
	g.poly(p, ui.Hex("#2f3e46").Alpha(0.85), 0, 0.85, 0.25, 0.6, 0.45, 0.78, 0.7, 0.55, 1, 0.8, 1, 1, 0, 1)
	g.line(p, 0.1, 0.92, 0.35, 0.92, 0.006, white.Alpha(0.4))
	g.line(p, 0.55, 0.95, 0.85, 0.95, 0.006, white.Alpha(0.3))
}

// sceneJazz is a saxophone in a blue-lit club.
func sceneJazz(p *ui.Painter, g geo) {
	g.circle(p, 0.5, 0.35, 0.3, ui.Hex("#4895ef").Alpha(0.18))
	brass := ui.Hex("#e9c46a")
	p.StrokePath(new(ui.Path).MoveTo(g.x(0.42), g.y(0.12)).
		CubeTo(g.x(0.42), g.y(0.5), g.x(0.4), g.y(0.75), g.x(0.55), g.y(0.78)).
		CubeTo(g.x(0.68), g.y(0.8), g.x(0.7), g.y(0.62), g.x(0.66), g.y(0.55)).Cap(ui.RoundCap), g.s(0.07), brass)
	g.circle(p, 0.66, 0.53, 0.07, brass)
	g.circle(p, 0.66, 0.53, 0.045, ui.Hex("#0b1a30"))
	g.line(p, 0.42, 0.12, 0.33, 0.08, 0.025, brass)
	for k := 0; k < 4; k++ {
		g.circle(p, 0.42, 0.3+float32(k)*0.1, 0.018, ui.Hex("#fefae0"))
	}
	for _, n := range [][2]float32{{0.75, 0.22}, {0.85, 0.32}} {
		g.circle(p, n[0], n[1], 0.025, white.Alpha(0.6))
		g.line(p, n[0]+0.022, n[1], n[0]+0.022, n[1]-0.1, 0.006, white.Alpha(0.6))
	}
}

// sceneBlues is rain falling past a lone window.
func sceneBlues(p *ui.Painter, g geo) {
	g.rect(p, 0.58, 0.18, 0.28, 0.36, 0.01, ui.Hex("#ffd166").Alpha(0.85))
	g.line(p, 0.72, 0.18, 0.72, 0.54, 0.012, ui.Hex("#10203c"))
	g.line(p, 0.58, 0.36, 0.86, 0.36, 0.012, ui.Hex("#10203c"))
	r := genreRand{s: 7}
	for i := 0; i < 26; i++ {
		x, y := r.f(), r.f()
		g.line(p, x, y, x-0.03, y+0.09, 0.005, ui.Hex("#a9d6e5").Alpha(0.5))
	}
	g.rect(p, 0, 0.82, 1, 0.18, 0, ui.Hex("#0b1424").Alpha(0.7))
}

// sceneVinyl is a record turning, its label lit.
func sceneVinyl(p *ui.Painter, g geo) {
	g.circle(p, 0.5, 0.5, 0.4, ui.Hex("#141014"))
	for k := 0; k < 6; k++ {
		g.ring(p, 0.5, 0.5, 0.18+float32(k)*0.035, 0.003, white.Alpha(0.08))
	}
	g.circle(p, 0.5, 0.5, 0.13, ui.Hex("#ef8354"))
	g.circle(p, 0.5, 0.5, 0.015, ui.Hex("#141014"))
	p.StrokePath(new(ui.Path).MoveTo(g.x(0.88), g.y(0.08)).LineTo(g.x(0.8), g.y(0.5)).LineTo(g.x(0.66), g.y(0.6)).Cap(ui.RoundCap), g.s(0.018), ui.Hex("#d9d9d9"))
}

// sceneMetal is a lightning bolt through a dark sky.
func sceneMetal(p *ui.Painter, g geo) {
	g.poly(p, ui.Hex("#2b2b2b"), 0, 0.7, 0.3, 0.55, 0.6, 0.7, 1, 0.5, 1, 1, 0, 1)
	g.poly(p, ui.Hex("#f8f9fa").Alpha(0.15), 0.38, 0.0, 0.62, 0.0, 0.5, 0.32, 0.66, 0.32, 0.36, 0.86, 0.44, 0.44, 0.3, 0.44)
	g.poly(p, ui.Hex("#c0c0c0"), 0.45, 0.04, 0.6, 0.04, 0.5, 0.34, 0.62, 0.34, 0.38, 0.8, 0.45, 0.42, 0.35, 0.42)
}

// scenePunk is a torn safety-pin poster.
func scenePunk(p *ui.Painter, g geo) {
	g.poly(p, ui.Hex("#f1faee"), 0.12, 0.15, 0.88, 0.1, 0.92, 0.8, 0.1, 0.88)
	g.poly(p, ui.Hex("#e63946"), 0.12, 0.15, 0.88, 0.1, 0.85, 0.35, 0.6, 0.3, 0.4, 0.42, 0.14, 0.35)
	g.ring(p, 0.5, 0.62, 0.12, 0.022, ui.Hex("#1d1d1d"))
	g.line(p, 0.42, 0.53, 0.58, 0.71, 0.022, ui.Hex("#1d1d1d"))
	g.line(p, 0.58, 0.53, 0.42, 0.71, 0.022, ui.Hex("#1d1d1d"))
}

// sceneRock is an electric guitar's body and a burst behind it.
func sceneRock(p *ui.Painter, g geo) {
	for k := 0; k < 12; k++ {
		a := float64(k) * math.Pi / 6
		g.poly(p, ui.Hex("#ffd166").Alpha(0.18), 0.5, 0.5, 0.5+0.8*cos(a), 0.5+0.8*sin(a), 0.5+0.8*cos(a+0.25), 0.5+0.8*sin(a+0.25))
	}
	p.Transform(ui.Rotation(35), g.x(0.5), g.y(0.5), func() {
		body := ui.Hex("#1a1a1a")
		g.circle(p, 0.45, 0.68, 0.16, body)
		g.circle(p, 0.55, 0.52, 0.12, body)
		g.rect(p, 0.48, 0.05, 0.05, 0.45, 0.01, ui.Hex("#6f4518"))
		g.rect(p, 0.42, 0.62, 0.17, 0.04, 0.01, ui.Hex("#e0e0e0"))
	})
}

// sceneHipHop is a boombox with its speakers going.
func sceneHipHop(p *ui.Painter, g geo) {
	g.rect(p, 0.1, 0.32, 0.8, 0.44, 0.04, ui.Hex("#1b1b2f"))
	g.line(p, 0.3, 0.32, 0.38, 0.18, 0.02, ui.Hex("#1b1b2f"))
	g.line(p, 0.7, 0.32, 0.62, 0.18, 0.02, ui.Hex("#1b1b2f"))
	g.line(p, 0.38, 0.18, 0.62, 0.18, 0.02, ui.Hex("#1b1b2f"))
	for _, x := range []float32{0.28, 0.72} {
		g.circle(p, x, 0.56, 0.13, ui.Hex("#ffd166"))
		g.circle(p, x, 0.56, 0.08, ui.Hex("#1b1b2f"))
		g.circle(p, x, 0.56, 0.03, ui.Hex("#ffd166"))
	}
	g.rect(p, 0.43, 0.4, 0.14, 0.06, 0.01, ui.Hex("#ef476f"))
}

// sceneSynth is a neon grid running to a striped sun.
func sceneSynth(p *ui.Painter, g geo) {
	g.circle(p, 0.5, 0.48, 0.25, ui.Hex("#f72585"))
	for k := 0; k < 4; k++ {
		g.rect(p, 0.2, 0.5+float32(k)*0.05, 0.6, 0.02, 0, ui.Hex("#3a0ca3"))
	}
	g.rect(p, 0, 0.62, 1, 0.38, 0, ui.Hex("#10002b"))
	for k := 0; k <= 10; k++ {
		x := float32(k) / 10
		g.line(p, 0.5+(x-0.5)*0.4, 0.62, x*1.6-0.3, 1, 0.006, ui.Hex("#00f5d4").Alpha(0.7))
	}
	for k := 0; k < 5; k++ {
		y := 0.62 + float32(k*k)*0.022
		g.line(p, 0, y, 1, y, 0.006, ui.Hex("#00f5d4").Alpha(0.7))
	}
}

// sceneAmbient is soft clouds drifting in light.
func sceneAmbient(p *ui.Painter, g geo) {
	g.circle(p, 0.3, 0.3, 0.3, white.Alpha(0.12))
	g.circle(p, 0.75, 0.55, 0.35, white.Alpha(0.08))
	for _, c := range [][3]float32{{0.25, 0.62, 0.1}, {0.38, 0.58, 0.13}, {0.52, 0.63, 0.1}, {0.62, 0.36, 0.07}, {0.72, 0.33, 0.09}, {0.82, 0.37, 0.06}} {
		g.circle(p, c[0], c[1], c[2], white.Alpha(0.55))
	}
	g.circle(p, 0.5, 0.18, 0.05, ui.Hex("#fefae0").Alpha(0.9))
}

// sceneSun is a big warm sun over palms.
func sceneSun(p *ui.Painter, g geo) {
	g.circle(p, 0.5, 0.55, 0.28, ui.Hex("#f4a261"))
	g.rect(p, 0, 0.7, 1, 0.3, 0, ui.Hex("#264653").Alpha(0.85))
	for _, x := range []float32{0.18, 0.82} {
		g.line(p, x, 0.95, x+0.04, 0.45, 0.022, ui.Hex("#1b2a2f"))
		for k := 0; k < 5; k++ {
			a := -math.Pi/2 + (float64(k)-2)*0.55
			g.line(p, x+0.04, 0.45, x+0.04+0.16*cos(a), 0.45+0.1*sin(a)+0.06, 0.02, ui.Hex("#1b2a2f"))
		}
	}
}

// sceneFilm is a strip of film and a clapper.
func sceneFilm(p *ui.Painter, g geo) {
	p.Transform(ui.Rotation(-14), g.x(0.5), g.y(0.5), func() {
		g.rect(p, -0.1, 0.33, 1.2, 0.34, 0, ui.Hex("#111"))
		for k := 0; k < 12; k++ {
			x := -0.08 + float32(k)*0.11
			g.rect(p, x, 0.35, 0.05, 0.035, 0.005, ui.Hex("#e9ecef"))
			g.rect(p, x, 0.615, 0.05, 0.035, 0.005, ui.Hex("#e9ecef"))
		}
		for k := 0; k < 4; k++ {
			g.rect(p, 0.05+float32(k)*0.25, 0.41, 0.2, 0.18, 0.01, ui.Hex("#2a9d8f").Alpha(0.6+float32(k)*0.1))
		}
	})
}

// sceneBalloons is balloons rising into a light sky.
func sceneBalloons(p *ui.Painter, g geo) {
	for _, b := range [][4]float32{{0.3, 0.35, 0.12, 0}, {0.55, 0.28, 0.14, 1}, {0.75, 0.42, 0.1, 2}, {0.42, 0.55, 0.09, 3}} {
		col := []ui.Color{ui.Hex("#ff595e"), ui.Hex("#ffca3a"), ui.Hex("#8ac926"), ui.Hex("#1982c4")}[int(b[3])]
		g.line(p, b[0], b[1]+b[2], b[0]-0.02, 0.95, 0.005, ui.Hex("#555").Alpha(0.6))
		g.circle(p, b[0], b[1], b[2], col)
		g.circle(p, b[0]-b[2]*0.35, b[1]-b[2]*0.35, b[2]*0.2, white.Alpha(0.5))
	}
}

// scenePop is a microphone in a ring of light.
func scenePop(p *ui.Painter, g geo) {
	g.ring(p, 0.5, 0.42, 0.3, 0.02, white.Alpha(0.35))
	g.circle(p, 0.5, 0.36, 0.14, ui.Hex("#e0e0e0"))
	for k := 0; k < 4; k++ {
		y := 0.27 + float32(k)*0.05
		g.line(p, 0.39, y, 0.61, y, 0.006, ui.Hex("#9e9e9e"))
	}
	g.rect(p, 0.47, 0.49, 0.06, 0.3, 0.02, ui.Hex("#2d2d2d"))
	for _, s := range [][2]float32{{0.2, 0.2}, {0.82, 0.25}, {0.18, 0.75}, {0.85, 0.7}} {
		g.circle(p, s[0], s[1], 0.02, white.Alpha(0.8))
	}
}
