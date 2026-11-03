package qqmusic

import (
	"context"
	"fmt"
	"html"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform/paging"
	backend "netease-music-mygo/internal/platform/qqmusic"

	"github.com/egoist/mygo/ui"
)

func (h *Home) columns(c *ui.Context, min float32) int {
	w, _ := c.Size()
	inner := w - 232 - 80
	return max(2, int((float32(inner)+20)/(min+20)))
}

func (h *Home) homePage(c *ui.Context) {
	if h.homeLoading && len(h.featured) == 0 && len(h.charts) == 0 {
		h.host.TrackSkeleton(c, TrackView{})
		return
	}
	if h.homeErr != nil && len(h.featured) == 0 && len(h.charts) == 0 {
		h.message(c, backend.LoginErrorMessage(h.homeErr), h.loadHome)
		return
	}
	var rows []func(*ui.Context)
	rows = append(rows, func(c *ui.Context) {
		now := time.Now()
		hello := greeting(now)
		if p := h.state.Profile(); p != nil {
			hello += "，" + p.Nickname
		}
		ui.Row(c).AlignItems(ui.Center).Gap(16).Margin(0, 0, 18, 0).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				ui.Text(c, strconv.Itoa(int(now.Month()))+"月"+strconv.Itoa(now.Day())+"日 "+weekday(now)).FontSize(13).TextColor(c.Theme().TextMuted)
				ui.Text(c, hello).FontSize(30).FontWeight(800).LetterSpacing(-0.6).SingleLine()
			})
			if h.state.Profile() != nil {
				label := "换一批"
				if h.shelfLoading {
					label = "更新中…"
				}
				b := h.host.PillButton(c, "qq-home-refresh", iconReload, label, false)
				if h.shelfLoading {
					b.Disabled(true)
				}
				if b.Clicked() {
					h.loadShelves()
				}
			}
		})
	})
	if len(h.daily) > 0 {
		rows = append(rows, h.dailyCard)
	}
	if h.state.Profile() == nil {
		rows = append(rows, func(c *ui.Context) {
			ui.Row(c).Margin(28, 0, 0, 0).AlignItems(ui.Center).Gap(12).Children(func() {
				ui.Text(c, "歌曲推荐").FontSize(19).FontWeight(700).LetterSpacing(-0.3).Grow(1)
				if h.host.PillButton(c, "qq-shelves-login", iconUser, "登录查看推荐", false).Clicked() {
					h.openLogin()
				}
			})
		})
	}
	if h.state.Profile() != nil && len(h.shelves) == 0 && (h.shelfLoading || h.homeLoading) {
		rows = append(rows, h.shelfSkeleton)
	}
	for _, shelf := range h.shelves {
		shelf := shelf
		rows = append(rows, func(c *ui.Context) { h.shelfSection(c, shelf) })
	}
	if len(h.rec) > 0 {
		rec := h.rec[:min(len(h.rec), h.columns(c, 168)*2)]
		rows = append(rows, func(c *ui.Context) {
			h.section(c, "今日为你推荐", rec, func(id int64) { h.Open(Route{Page: PagePlaylist, ID: id}) })
		})
	}
	rows = append(rows, func(c *ui.Context) {
		h.section(c, "精选歌单", h.featured, func(id int64) { h.Open(Route{Page: PagePlaylist, ID: id}) })
	})
	h.host.PageList(c, "qq-home", h.listState(), len(rows), func(i int) { rows[i](c) })
}

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

func weekday(now time.Time) string {
	return [...]string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}[now.Weekday()]
}

// loadShelves replaces the song recommendations, leaving the rest of Home
// as it is.
func (h *Home) loadShelves() {
	if h.shelfLoading || h.state.Profile() == nil {
		return
	}
	h.shelfLoading = true
	h.host.Invalidate()
	go func() {
		shelves, err := backend.RecommendedSongShelves()
		h.Post(func() {
			h.shelfLoading = false
			if err == nil {
				h.shelves = shelves
				h.shelfPagers = nil
			} else {
				h.host.Toast("歌曲推荐加载失败，请稍后再试")
			}
			h.host.Invalidate()
		})
	}()
}

// shelfSkeleton stands in for the song shelves while they load.
func (h *Home) shelfSkeleton(c *ui.Context) {
	ui.Text(c, "歌曲推荐").FontSize(19).FontWeight(700).LetterSpacing(-0.3).Margin(28, 0, 0, 12)
	cols := min(3, max(1, h.columns(c, 240)))
	ui.Grid(c).Columns(cols).GapX(16).GapY(4).Children(func() {
		for i := 0; i < cols*2; i++ {
			ui.Row(c).Height(76).Padding(6).Gap(10).AlignItems(ui.Center).Children(func() {
				h.host.Bone(c, 6).Size(48, 48)
				ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
					h.host.Bone(c, 4).Width(120).Height(12)
					h.host.Bone(c, 4).Width(80).Height(10)
				})
			})
		}
	})
}

func (h *Home) shelfSection(c *ui.Context, shelf backend.SongShelf) {
	if h.shelfPagers == nil {
		h.shelfPagers = map[string]*songShelfPager{}
	}
	pager := h.shelfPagers[shelf.Title]
	if pager == nil {
		pager = &songShelfPager{}
		h.shelfPagers[shelf.Title] = pager
	}
	cols := min(3, max(1, h.columns(c, 240)))
	pager.layout(len(shelf.Tracks), cols*2)
	section := ui.Column(c).Gap(0)
	section.HandleInput(pager.scroll)
	if pager.pages() > 1 {
		section.OnShortcut(0, ui.KeyLeft, func() {
			if pager.move(-1) {
				h.host.Invalidate()
			}
		}).OnShortcut(0, ui.KeyRight, func() {
			if pager.move(1) {
				h.host.Invalidate()
			}
		})
	}
	section.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Margin(34, 0, 10, 0).Gap(6).Children(func() {
			ui.Text(c, shelf.Title).FontSize(19).FontWeight(700).LetterSpacing(-0.3).Grow(1)
			if h.host.PillButton(c, "qq-shelf-play-"+shelf.Title, iconPlay, "播放", false).Clicked() {
				h.play(tracks(shelf.Tracks), pager.start())
			}
			prev := ui.ButtonBase(c.Key("qq-shelf-prev")).Size(28, 28).Radius(8).Center().Label("上一组歌曲").Disabled(pager.page <= 0)
			prev.Children(func() { ui.Icon(c, iconBack).Size(14, 14) })
			if prev.Clicked() {
				pager.move(-1)
			}
			next := ui.ButtonBase(c.Key("qq-shelf-next")).Size(28, 28).Radius(8).Center().Label("下一组歌曲").Disabled(pager.page >= pager.pages()-1)
			next.Children(func() { ui.Icon(c, iconForward).Size(14, 14) })
			if next.Clicked() {
				pager.move(1)
			}
		})
		if len(shelf.Tracks) == 0 {
			ui.Text(c, "暂无内容").FontSize(13).TextColor(c.Theme().TextMuted)
			return
		}
		start, end := pager.start(), min(pager.start()+pager.size, len(shelf.Tracks))
		cur := h.currentTrack()
		ui.Grid(c).Columns(cols).GapX(16).GapY(4).Children(func() {
			for i := start; i < end; i++ {
				tr := track(shelf.Tracks[i])
				current := cur != nil && cur.ID == tr.ID
				h.shelfRow(c, shelf, tr, i, current)
			}
		})
	})
}

// shelfRow is one song of a shelf: its cover plays it, with the current
// song marked, and a heart collects it.
func (h *Home) shelfRow(c *ui.Context, shelf backend.SongShelf, tr netease.Track, i int, current bool) {
	t := c.Theme()
	row := ui.Row(c.Key("qq-shelf-" + shelf.Title + "-" + strconv.FormatInt(tr.ID, 10))).Height(76).Padding(6).Gap(4).Radius(8).AlignItems(ui.Center)
	row.ContextMenu(func(m *ui.Menu) { h.trackMenu(m, &tr, 0) })
	hovered := row.Hovered()
	if current {
		row.Background(t.Accent.Alpha(0.10))
	} else if hovered {
		row.Background(t.Text.Alpha(0.04))
	}
	row.Children(func() {
		b := ui.ButtonBase(c.Key("song")).Grow(1).Shrink(1).MinWidth(0).Gap(10).AlignItems(ui.Center).FocusRing(false).Label(tr.Name)
		b.Children(func() {
			ui.Box(c).Size(48, 48).Radius(6).Children(func() {
				h.host.Cover(c, tr.Album.PicURL, 48, 6)
				if hovered || current {
					ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(6).Center().
						Background(ui.RGBA(0, 0, 0, 0.32)).PassThrough().Children(func() {
						ui.Icon(c, iconPlay).Size(15, 15).TextColor(ui.RGBA(1, 1, 1, 1))
					})
				}
			})
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
				name := t.Text
				if current {
					name = t.Accent
				}
				ui.Text(c, tr.Name).FontSize(13.5).FontWeight(500).TextColor(name).SingleLine()
				ui.Text(c, tr.ArtistNames()).FontSize(12).TextColor(t.TextMuted).SingleLine()
			})
		})
		if b.Clicked() {
			h.play(tracks(shelf.Tracks), i)
		}
		liked := h.state.Liked(tr.ID)
		hb := ui.ButtonBase(c.Key("like")).Size(24, 24).Radius(12).Center().Label("喜欢")
		hb.Children(func() {
			icon, color := iconHeart, t.TextMuted
			if liked {
				icon, color = iconHeartFill, ui.Hex("#e5484d")
			}
			ui.Icon(c, icon).Size(14, 14).TextColor(color)
		})
		if hb.Clicked() {
			h.favorite(tr, !liked)
		}
	})
}

// currentTrack reports what plays when it belongs to QQ Music.
func (h *Home) currentTrack() *netease.Track {
	p := h.host.Player()
	if p == nil {
		return nil
	}
	tr := p.Current()
	if tr == nil || tr.Source != Source {
		return nil
	}
	return tr
}

func (h *Home) dailyCard(c *ui.Context) {
	if len(h.daily) == 0 {
		return
	}
	songs := h.daily
	now := time.Now()
	url := songs[0].Album.PicURL
	deep, rich, glow := ui.RGBA(26, 28, 32, 1), ui.RGBA(48, 52, 58, 1), ui.RGBA(126, 168, 146, 1)
	if d, r, g, ok := h.host.ArtworkColors(url); ok {
		deep, rich, glow = d, r, g
	}
	blur := h.host.ArtworkBackdrop(url)
	white := ui.RGBA(255, 255, 255, 1)
	card := ui.ButtonBase(c.Key("qq-daily")).Height(212).Radius(24).FocusRing(false).Label("每日推荐").TextColor(white)
	fade := ui.Animation{Duration: 600 * time.Millisecond}
	deep, rich, glow = card.AnimateColor("deep", deep, fade), card.AnimateColor("rich", rich, fade), card.AnimateColor("glow", glow, fade)
	still := c.Preferences().ReduceMotion
	hovered := card.Hovered()
	ours := h.dailyCurrent()
	playing := ours && h.dailyActuallyPlaying()
	sweep := float32(0)
	if hovered && !still {
		sweep = card.AnimateTo("sweep", 1, ui.Animation{Duration: 1100 * time.Millisecond, Ease: ui.EaseInOut})
	} else {
		card.Jump("sweep", 0)
	}
	out := float32(0)
	if playing {
		out = 1
	} else if hovered {
		out = 0.6
	}
	out = card.AnimateTo("out", out, ui.Animation{Spring: ui.SpringOptions{Duration: 700 * time.Millisecond, Bounce: 0.25}})
	spinning := (playing || hovered) && !still
	var inner float32
	if w, _ := c.Size(); w > 0 {
		inner = w - 232 - 80
	}
	showStack := inner >= 620
	showPicks := inner >= 1040
	showDay := inner >= 900
	if showStack {
		card.Draw(func(p *ui.Painter, r ui.Rect) {
			angle := float32(0)
			if spinning {
				angle = float32(float64(p.Now().UnixMilli()%360000) * 0.1)
				p.AnimationFrame()
			}
			cx := r.X + r.W - 32 - 300 + 64 + 156/2 + dailyRecordOut(out)
			drawDailyBackdrop(p, r, blur, deep, rich, glow, cx, angle, sweep)
			if showDay {
				day := ui.Span{Text: strconv.Itoa(now.Day()), Size: 240, Weight: 800, LetterSpacing: -12, Features: "tnum", Color: ui.RGBA(255, 255, 255, 0.055)}
				p.Clip(r, 24, func() { p.RichText(r.X+32+300, r.Y+r.H*0.22, 0, day) })
			}

		})
	} else {
		card.Background(deep)
	}
	card.Children(func() {
		ui.Row(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Padding(0, 32).Gap(28).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(6).Children(func() {
				ui.Text(c, strconv.Itoa(int(now.Month()))+"月"+strconv.Itoa(now.Day())+"日 "+weekday(now)+" · "+h.dailyNote()).
					FontSize(12).FontWeight(600).LetterSpacing(0.3).TextColor(ui.RGBA(255, 255, 255, 0.66)).MaxLines(1)
				ui.Text(c, "每日推荐").FontSize(32).FontWeight(800).LetterSpacing(-0.8).LineHeight(1.15).TextColor(white)
				ui.Text(c, dailySummary(songs)).FontSize(13).TextColor(ui.RGBA(255, 255, 255, 0.8)).MaxLines(1)
				ui.Row(c).Gap(10).Margin(12, 0, 0, 0).AlignItems(ui.Center).Children(func() {
					h.dailyPlayButton(c, songs, ours, playing)
					if h.state.Profile() == nil {
						if h.host.PillButton(c, "qq-home-daily-login", iconUser, "登录", false).Clicked() {
							h.openLogin()
						}
					}
				})
			})
			if showPicks {
				h.dailyPicks(c, songs)
			}
			if showStack {
				h.dailyStack(c, songs, hovered, out, spinning)
			}
		})
	})
	if card.Clicked() {
		h.Open(Route{Page: PageDaily})
	}
}

// dailySummary names the first artists of the day's songs and how many.
func dailySummary(list []netease.Track) string {
	var names []string
	seen := map[string]bool{}
	for _, tr := range list {
		for _, ar := range tr.Artists {
			if ar.Name == "" || seen[ar.Name] {
				continue
			}
			seen[ar.Name] = true
			names = append(names, ar.Name)
			if len(names) == 3 {
				break
			}
		}
		if len(names) == 3 {
			break
		}
	}
	if len(names) == 0 {
		return strconv.Itoa(len(list)) + " 首"
	}
	return strings.Join(names, "、") + " 等 " + strconv.Itoa(len(list)) + " 首"
}

// dailyNote says, shortly, how the day's songs are picked.
func (h *Home) dailyNote() string {
	if h.state.Profile() == nil {
		return "登录后为你专属生成"
	}
	return "QQ 音乐每日推荐"
}

// dailyCurrent reports whether the song playing belongs to the day's list.
func (h *Home) dailyCurrent() bool {
	cur := h.currentTrack()
	if cur == nil {
		return false
	}
	for _, tr := range h.daily {
		if tr.ID == cur.ID {
			return true
		}
	}
	return false
}

// dailyActuallyPlaying reports whether the day's songs are playing now.
func (h *Home) dailyActuallyPlaying() bool {
	if p := h.host.Player(); p != nil && p.Playing() {
		return h.dailyCurrent()
	}
	return false
}

// dailyPlayButton plays the day's songs, or pauses and resumes them.
func (h *Home) dailyPlayButton(c *ui.Context, songs []netease.Track, ours, playing bool) {
	ink := ui.RGBA(24, 30, 28, 1)
	b := ui.ButtonBase(c.Key("qq-home-play-daily")).Height(38).PaddingX(20).Radius(999).Gap(8).AlignItems(ui.Center).
		TextColor(ink).Shadow(0, 6, 16, -6, ui.RGBA(0, 0, 0, 0.35))
	fill := ui.RGBA(255, 255, 255, 1)
	if b.Hovered() {
		fill = ui.RGBA(240, 246, 242, 1)
	}
	b.Background(b.AnimateColor("bg", fill, ui.Animation{Duration: 120 * time.Millisecond}))
	pose := ui.Pose{}
	if b.Pressed() {
		pose.Scale = 0.95
	}
	b.AnimatePose(pose, ui.Snappy)
	label := "播放"
	switch {
	case playing && b.Hovered():
		label = "暂停播放"
	case playing:
		label = "正在播放"
	case ours:
		label = "继续播放"
	}
	b.Label(label)
	b.Children(func() {
		if playing && !b.Hovered() {
			ui.Box(c).Size(14, 13).Draw(func(p *ui.Painter, r ui.Rect) {
				p.AnimationFrame()
				t := float64(p.Now().UnixMilli()) / 260
				for i := 0; i < 4; i++ {
					hgt := float32(3 + 8*(0.5+0.5*math.Sin(t+float64(i)*1.7)))
					p.Fill(ui.Rect{X: r.X + float32(i)*3.5, Y: r.Y + r.H - hgt, W: 2, H: hgt}, ink, 1)
				}
			})
		} else {
			icon := iconPlay
			if playing || ours {
				icon = iconPause
			}
			ui.Icon(c, icon).Size(14, 14).TextColor(ink)
		}
		ui.Text(c, label).FontSize(14).FontWeight(600).TextColor(ink)
	})
	if b.Clicked() {
		if ours {
			if p := h.host.Player(); p != nil {
				p.TogglePlay()
			}
		} else {
			h.play(songs, 0)
		}
	}
}

// dailyPicks lists the day's first songs beside the words on wide pages.
func (h *Home) dailyPicks(c *ui.Context, songs []netease.Track) {
	cur := h.currentTrack()
	ui.Column(c.Key("qq-daily-picks")).Width(236).Shrink(0).Gap(2).Children(func() {
		for i := range min(3, len(songs)) {
			i := i
			tr := songs[i]
			current := cur != nil && cur.ID == tr.ID
			row := ui.ButtonBase(c.Key(strconv.FormatInt(tr.ID, 10))).Height(46).Gap(12).Padding(4, 10).Radius(12).AlignItems(ui.Center).FocusRing(false).Label(tr.Name)
			if row.Hovered() {
				row.Background(ui.RGBA(255, 255, 255, 0.10))
			}
			row.Children(func() {
				ui.Box(c).Width(18).Center().Children(func() {
					if current {
						ui.Icon(c, iconPause).Size(11, 11).TextColor(ui.RGBA(255, 255, 255, 0.9))
					} else {
						ui.Text(c, fmt.Sprintf("%02d", i+1)).FontSize(12).FontWeight(700).TextColor(ui.RGBA(255, 255, 255, 0.45))
					}
				})
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, tr.Name).FontSize(13).FontWeight(600).MaxLines(1).TextColor(ui.RGBA(255, 255, 255, 1))
					ui.Text(c, tr.ArtistNames()).FontSize(11.5).MaxLines(1).TextColor(ui.RGBA(255, 255, 255, 0.6))
				})
			})
			if row.Clicked() {
				if current {
					if p := h.host.Player(); p != nil {
						p.TogglePlay()
					}
				} else {
					h.play(songs, i)
				}
			}
		}
	})
}

// The daily card's measures; the stack of covers is laid out by hand.
const (
	dailySleeveSize = 156
	dailyRecordSize = 146
	dailyStackWidth = 300
	dailySleeveLeft = 64
)

func dailyRecordOut(out float32) float32 { return 30 + 46*out }

// dailyStack is the day's first cover, as a sleeve with its record out as
// far as out, the two covers after it behind, fanned under the pointer.
func (h *Home) dailyStack(c *ui.Context, songs []netease.Track, fanned bool, out float32, spinning bool) {
	spring := ui.Animation{Spring: ui.SpringOptions{Duration: 600 * time.Millisecond, Bounce: 0.3}}
	ui.Box(c.Key("qq-daily-stack")).Size(dailyStackWidth, dailySleeveSize).Shrink(0).Children(func() {
		for i := min(2, len(songs)-1); i >= 1; i-- {
			size := float32(dailySleeveSize - 18*i)
			pose := ui.Pose{Rotate: -5 * float32(i), Y: 2}
			if fanned {
				pose = ui.Pose{Rotate: -9 * float32(i), X: -10 * float32(i), Y: -2}
			}
			cov := ui.Box(c.Key(strconv.Itoa(i))).Absolute().Left(float32(dailySleeveLeft)-30*float32(i)).Top((dailySleeveSize-size)/2).Size(size, size).Radius(10).
				Opacity(1-0.12*float32(i)).Shadow(0, 10, 22, -6, ui.RGBA(0, 0, 0, 0.45))
			cov.AnimatePose(pose, spring)
			cov.Children(func() { h.host.Cover(c, songs[i].Album.PicURL, size, 10) })
		}
		mid := float32(dailySleeveLeft + dailySleeveSize/2)
		ui.Box(c.Key("qq-daily-record")).Absolute().Left(mid+dailyRecordOut(out)-dailyRecordSize/2).Top((dailySleeveSize-dailyRecordSize)/2).
			Size(dailyRecordSize, dailyRecordSize).Radius(dailyRecordSize/2).Shadow(0, 8, 20, -4, ui.RGBA(0, 0, 0, 0.5)).
			Draw(func(p *ui.Painter, r ui.Rect) {
				angle := float32(0)
				if spinning {
					angle = float32(float64(p.Now().UnixMilli()%360000) * 0.1)
					p.AnimationFrame()
				}
				drawDailyRecord(p, r, h.host.CoverBitmap(songs[0].Album.PicURL), angle)
			})
		sleeve := ui.Box(c.Key("qq-daily-sleeve")).Absolute().Left(dailySleeveLeft).Top(0).Size(dailySleeveSize, dailySleeveSize).Radius(12).
			Shadow(0, 16, 30, -8, ui.RGBA(0, 0, 0, 0.5))
		sleevePose := ui.Pose{}
		if fanned {
			sleevePose = ui.Pose{Rotate: -2, Y: -3}
		}
		sleeve.AnimatePose(sleevePose, spring)
		sleeve.Children(func() {
			h.host.Cover(c, songs[0].Album.PicURL, dailySleeveSize, 12)
			ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(12).PassThrough().Border(1, ui.RGBA(255, 255, 255, 0.14)).
				LinearGradient(ui.LinearGradient{Angle: 145, Stops: []ui.GradientStop{
					{Color: ui.RGBA(255, 255, 255, 0.22), At: 0}, {Color: ui.RGBA(255, 255, 255, 0), At: 0.42},
					{Color: ui.RGBA(0, 0, 0, 0), At: 0.7}, {Color: ui.RGBA(0, 0, 0, 0.18), At: 1},
				}})
		})
	})
}

// drawDailyRecord paints a record, its grooves and label turned to angle.
func drawDailyRecord(p *ui.Painter, r ui.Rect, bmp *ui.Bitmap, angle float32) {
	d := r.W
	cx, cy := r.X+d/2, r.Y+d/2
	p.Fill(r, ui.RGBA(16, 17, 19, 1), d/2)
	for i, rr := 0, d*0.21; rr < d*0.48; i, rr = i+1, rr+2.2 {
		a := float32(0.035)
		if i%3 == 0 {
			a = 0.07
		}
		p.Stroke(ui.Rect{X: cx - rr, Y: cy - rr, W: 2 * rr, H: 2 * rr}, ui.RGBA(255, 255, 255, a), rr, 0.7)
	}
	p.FillConicGradient(r, ui.ConicGradient{Angle: 20, Stops: []ui.GradientStop{
		{Color: ui.RGBA(255, 255, 255, 0), At: 0}, {Color: ui.RGBA(255, 255, 255, 0.13), At: 0.08}, {Color: ui.RGBA(255, 255, 255, 0), At: 0.2},
		{Color: ui.RGBA(255, 255, 255, 0), At: 0.5}, {Color: ui.RGBA(255, 255, 255, 0.13), At: 0.58}, {Color: ui.RGBA(255, 255, 255, 0), At: 0.7},
		{Color: ui.RGBA(255, 255, 255, 0), At: 1},
	}}, d/2)
	p.Stroke(r, ui.RGBA(255, 255, 255, 0.1), d/2, 1)
	lr := d * 0.18
	label := ui.Rect{X: cx - lr, Y: cy - lr, W: 2 * lr, H: 2 * lr}
	p.Fill(label, ui.Hex("#30a46c"), lr)
	if bmp != nil {
		p.Clip(label, lr, func() {
			p.Transform(ui.Rotation(angle), cx, cy, func() { p.Image(bmp, label, ui.Cover) })
		})
	} else {
		p.Transform(ui.Rotation(angle), cx, cy, func() {
			p.Line(cx-lr*0.7, cy, cx+lr*0.7, cy, 1.5, ui.RGBA(255, 255, 255, 0.35))
		})
	}

	p.Stroke(label, ui.RGBA(0, 0, 0, 0.35), lr, 1.5)
	p.Fill(ui.Rect{X: cx - 2.5, Y: cy - 2.5, W: 5, H: 5}, ui.RGBA(24, 26, 28, 1), 2.5)
}

// coverRect fits a w by h region of r, as a cover does.
func coverRect(r ui.Rect, w, h float32) ui.Rect {
	s := max(r.W/w, r.H/h)
	return ui.Rect{X: r.X + (r.W-w*s)/2, Y: r.Y + (r.H-h*s)/2, W: w * s, H: h * s}
}

// drawDailyBackdrop paints the day's colours, its cover blurred through them,
// and the record turning at cx.
func drawDailyBackdrop(p *ui.Painter, r ui.Rect, blurred *ui.Bitmap, deep, rich, glow ui.Color, cx, angle, sweep float32) {
	cy := r.Y + r.H/2
	p.Clip(r, 24, func() {
		p.FillGradient(r, ui.LinearGradient{Angle: 100, Oklab: true, Stops: []ui.GradientStop{
			{Color: deep, At: 0.1}, {Color: rich, At: 0.85}, {Color: rich.Mix(deep, 0.3), At: 1},
		}}, 0)
		if blurred != nil {
			p.ImageRegion(blurred, ui.Rect{W: 48, H: 48}, coverRect(r, 48, 48), ui.ImageOptions{Tint: ui.RGBA(255, 255, 255, 0.28)})
		}
		p.FillGradient(r, ui.LinearGradient{Angle: 90, Stops: []ui.GradientStop{
			{Color: deep.Alpha(0.85), At: 0}, {Color: deep.Alpha(0), At: 0.6},
		}}, 0)
		p.Shadow(ui.Rect{X: cx - 60, Y: cy - 60, W: 120, H: 120}, 60, 0, 0, 140, 30, glow.Alpha(0.32))
		p.Shadow(ui.Rect{X: r.X + r.W*0.3, Y: r.Y - 80, W: r.W * 0.4, H: 40}, 20, 0, 0, 120, 0, ui.RGBA(255, 255, 255, 0.08))
		const gap = 18
		off := float32(math.Round(float64(angle/360*gap*p.Scale()))) / p.Scale()
		for rr := float32(dailyRecordSize/2+6) + off; rr < r.W; rr += gap {
			a := 0.05 * (1 - (rr-dailyRecordSize/2)/(r.H*1.6))
			if a <= 0.004 {
				break
			}
			a *= min(1, (rr-dailyRecordSize/2-6)/gap)
			p.Stroke(ui.Rect{X: cx - rr, Y: cy - rr, W: 2 * rr, H: 2 * rr}, ui.RGBA(255, 255, 255, a), rr, 1)
		}
		if sweep > 0 && sweep < 1 {
			at := -0.25 + 1.5*sweep
			p.FillGradient(r, ui.LinearGradient{Angle: 110, Stops: []ui.GradientStop{
				{Color: ui.RGBA(255, 255, 255, 0), At: clamp01(at - 0.12)},
				{Color: ui.RGBA(255, 255, 255, 0.13*(1-sweep*0.5)), At: clamp01(at)},
				{Color: ui.RGBA(255, 255, 255, 0), At: clamp01(at + 0.12)},
			}}, 0)
		}
	})
	p.Stroke(r, ui.RGBA(255, 255, 255, 0.1), 24, 1)
}

func clamp01(v float32) float32 { return min(1, max(0, v)) }

func (h *Home) section(c *ui.Context, title string, cards []netease.Playlist, open func(int64)) {
	ui.Text(c, title).FontSize(19).FontWeight(700).LetterSpacing(-0.3).Margin(34, 0, 14, 0)
	if len(cards) == 0 {
		ui.Text(c, "暂无内容").FontSize(13).TextColor(c.Theme().TextMuted)
		return
	}
	cols := h.columns(c, 168)
	n := len(cards)
	w, _ := c.Size()
	inner := w - 232 - 80
	cardW := (inner - float32(20*(cols-1))) / float32(cols)
	ui.Grid(c).Columns(cols).GapX(20).GapY(24).Children(func() {
		for i := 0; i < n; i++ {
			h.playlistCard(c, cards[i], cardW, open)
		}
	})
}

func (h *Home) dailyPage(c *ui.Context) {
	if h.homeLoading && len(h.daily) == 0 {
		h.host.TrackSkeleton(c, TrackView{})
		return
	}
	if len(h.daily) == 0 {
		h.message(c, "暂时没有推荐", h.loadHome)
		return
	}
	h.host.RenderTracks(c, staticTracks(h.daily), TrackView{
		Title: "每日歌曲推荐", Sub: "每日推荐", Total: len(h.daily),
		OnPlay:    func(i int) { h.play(h.daily, i) },
		OnShuffle: func() { h.shuffle(h.daily) },
		Header:    h.dailyHeader,
		Menu:      h.trackMenu, Liked: h.state.Liked, Favorite: h.favorite,
		ShowAlbum: true,
	})
}

func (h *Home) dailyHeader(c *ui.Context) {
	t := c.Theme()
	now := time.Now()
	url := ""
	if len(h.daily) > 0 {
		url = h.daily[0].Album.PicURL
	}
	deep, rich := ui.RGBA(26, 28, 32, 1), ui.RGBA(48, 52, 58, 1)
	if d, r, _, ok := h.host.ArtworkColors(url); ok {
		deep, rich = d, r
	}
	blur := h.host.ArtworkBackdrop(url)
	ui.Row(c).Gap(28).AlignItems(ui.Center).Padding(0, 0, 26, 0).Children(func() {
		art := ui.Box(c.Key("qq-daily-art")).Size(212, 212).Radius(14)
		art.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Clip(r, 14, func() {
				p.FillGradient(r, ui.LinearGradient{Angle: 160, Oklab: true, Stops: []ui.GradientStop{
					{Color: rich, At: 0}, {Color: deep, At: 1},
				}}, 0)
				if blur != nil {
					p.ImageRegion(blur, ui.Rect{W: 48, H: 48}, coverRect(r, 48, 48), ui.ImageOptions{Tint: ui.RGBA(255, 255, 255, 0.18)})
				}
				p.FillGradient(r, ui.LinearGradient{Angle: 90, Stops: []ui.GradientStop{
					{Color: deep.Alpha(0), At: 0.3}, {Color: deep.Alpha(0.55), At: 1},
				}}, 0)
			})
		})
		art.Children(func() {
			ui.Column(c).Absolute().Left(18).Bottom(14).Gap(0).Children(func() {
				ui.Text(c, strconv.Itoa(now.Day())).FontSize(72).FontWeight(300).LetterSpacing(-2).
					TextColor(ui.RGBA(255, 255, 255, 1)).LineHeight(1)
				ui.Text(c, strconv.Itoa(int(now.Month()))+"月 · "+weekday(now)).FontSize(11).FontWeight(600).
					TextColor(ui.RGBA(255, 255, 255, 0.82))
			})
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			ui.Text(c, "每日推荐").FontSize(12).FontWeight(600).TextColor(t.Accent)
			ui.Text(c, "每日歌曲推荐").FontSize(30).FontWeight(800).LetterSpacing(-0.6)
			blurb := "登录 QQ 音乐查看每日推荐"
			if h.state.Profile() != nil {
				blurb = "根据你的音乐口味生成"
			}
			ui.Text(c, strconv.Itoa(len(h.daily))+" 首 · "+blurb).FontSize(13).TextColor(t.TextMuted)
			ui.Row(c).Gap(10).Margin(8, 0, 0, 0).Children(func() {
				if h.host.PillButton(c, "qq-daily-play", iconPlay, "播放全部", true).Clicked() {
					h.play(h.daily, 0)
				}
				if h.host.PillButton(c, "qq-daily-shuffle", iconShuffle, "随机播放", false).Clicked() {
					h.shuffle(h.daily)
				}
			})
		})
	})
}

func (h *Home) fmPage(c *ui.Context) {
	if h.state.Profile() == nil {
		h.loginPrompt(c)
		return
	}
	h.host.PageList(c, "qq-fm", h.listState(), 2, func(i int) {
		if i == 0 {
			h.heading(c, "电台", "无尽推荐")
			return
		}
		h.fmCard(c)
	})
}

const fmRadio = "fm:" + backend.FMDefault

// fmCard is the radio: what plays, large, with its own controls.
func (h *Home) fmCard(c *ui.Context) {
	p := h.host.Player()
	onFM := p != nil && p.Radio() == fmRadio
	var tr *netease.Track
	if onFM {
		tr = h.currentTrack()
	}
	deep, rich := ui.Hex("#12352b"), ui.Hex("#1e5a45")
	var blur *ui.Bitmap
	if tr != nil {
		if d, r, _, ok := h.host.ArtworkColors(tr.Album.PicURL); ok {
			deep, rich = d, r
		}
		blur = h.host.ArtworkBackdrop(tr.Album.PicURL)
	}
	white := ui.RGBA(255, 255, 255, 1)
	card := ui.Row(c.Key("qq-fm-card")).Height(320).Radius(24).Padding(36, 40).Gap(40).AlignItems(ui.Center).TextColor(white)
	card.Draw(func(p *ui.Painter, r ui.Rect) {
		p.Clip(r, 24, func() {
			p.FillGradient(r, ui.LinearGradient{Angle: 115, Oklab: true, Stops: []ui.GradientStop{
				{Color: deep, At: 0}, {Color: rich, At: 1},
			}}, 0)
			if blur != nil {
				p.ImageRegion(blur, ui.Rect{W: 48, H: 48}, coverRect(r, 48, 48), ui.ImageOptions{Tint: ui.RGBA(255, 255, 255, 0.22)})
			}
			p.FillGradient(r, ui.LinearGradient{Angle: 90, Stops: []ui.GradientStop{
				{Color: deep.Alpha(0.7), At: 0}, {Color: deep.Alpha(0), At: 0.7},
			}}, 0)
		})
	})
	card.Children(func() {
		h.fmCovers(c, tr)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconRadio).Size(14, 14).TextColor(ui.RGBA(255, 255, 255, 0.75))
				ui.Text(c, "电台 · 推荐").FontSize(12).FontWeight(600).TextColor(ui.RGBA(255, 255, 255, 0.75))
			})
			if tr == nil {
				ui.Text(c, "听见更多好音乐").FontSize(30).FontWeight(800).LetterSpacing(-0.6).TextColor(white)
				ui.Text(c, "根据你的口味，一首接一首，不停推荐").FontSize(13.5).TextColor(ui.RGBA(255, 255, 255, 0.72))
				ui.Row(c).Margin(18, 0, 0, 0).Children(func() {
					label := "开始收听"
					if h.fmStarting {
						label = "正在调频…"
					}
					b := ui.ButtonBase(c.Key("qq-fm-start")).Height(40).PaddingX(20).Radius(999).Gap(8).AlignItems(ui.Center).
						Background(white).FocusRing(false).Label(label)
					b.Children(func() {
						ui.Icon(c, iconPlay).Size(14, 14).TextColor(ui.RGBA(18, 53, 43, 1))
						ui.Text(c, label).FontSize(14).FontWeight(600).TextColor(ui.RGBA(18, 53, 43, 1))
					})
					if h.fmStarting {
						b.Disabled(true)
					}
					if b.Clicked() {
						h.startFM()
					}
				})
				return
			}
			ui.Text(c, tr.Name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2).TextColor(white)
			switch {
			case p != nil && p.Loading():
				ui.Text(c, "正在缓冲…").FontSize(14).TextColor(ui.RGBA(255, 255, 255, 0.7)).SingleLine()
			case p != nil && p.Err() != "":
				ui.Text(c, p.Err()).FontSize(14).TextColor(white).SingleLine()
			default:
				ui.Text(c, tr.ArtistNames()).FontSize(14).TextColor(ui.RGBA(255, 255, 255, 0.72)).SingleLine()
			}
			ui.Row(c).Gap(14).AlignItems(ui.Center).Margin(20, 0, 0, -8).Children(func() {
				liked := h.state.Liked(tr.ID)
				likeIcon := iconHeart
				if liked {
					likeIcon = iconHeartFill
				}
				lb := ui.ButtonBase(c.Key("qq-fm-like")).Size(28, 28).Radius(999).Center().Label("喜欢")
				lb.Children(func() {
					color := ui.RGBA(255, 255, 255, 0.78)
					if liked {
						color = ui.Hex("#e5484d")
					}
					ui.Icon(c, likeIcon).Size(20, 20).TextColor(color)
				})
				if lb.Clicked() {
					h.favorite(*tr, !liked)
				}
				pb := ui.ButtonBase(c.Key("qq-fm-play")).Size(56, 56).Center().Radius(999).Background(white).Label("播放/暂停")
				pb.Children(func() {
					icon := iconPlay
					if p != nil && p.Playing() {
						icon = iconPause
					}
					ui.Icon(c, icon).Size(22, 22).TextColor(ui.RGBA(18, 53, 43, 1))
				})
				if pb.Clicked() {
					if p != nil {
						p.TogglePlay()
					}
				}
				nb := ui.ButtonBase(c.Key("qq-fm-next")).Size(32, 32).Radius(999).Center().Label("下一首")
				nb.Children(func() { ui.Icon(c, iconNext).Size(20, 20).TextColor(ui.RGBA(255, 255, 255, 0.78)) })
				if nb.Clicked() {
					if p != nil {
						p.Next()
					}
				}
			})
		})
	})
}

// fmCovers stacks the cover playing over the next song's, which peeks out
// behind it, tilted, as the radio's own player shows them.
func (h *Home) fmCovers(c *ui.Context, tr *netease.Track) {
	const size = 232
	ui.Box(c).Size(size+36, size).Shrink(0).Children(func() {
		p := h.host.Player()
		var next *netease.Track
		if tr != nil && p != nil {
			order, at := p.PlayOrder()
			queue := p.Queue()
			if at >= 0 && at+1 < len(order) {
				if j := order[at+1]; j >= 0 && j < len(queue) && queue[j].Source == Source {
					n := queue[j]
					next = &n
				}
			}
		}
		if next != nil {
			back := ui.Box(c.Key("qq-fm-next-cover")).Absolute().Top(16).Left(36).Size(size-32, size-32).Radius(14).
				Opacity(0.55).Background(ui.RGBA(255, 255, 255, 0.12))
			back.AnimatePose(ui.Pose{Rotate: 6, X: 14}, ui.Animation{Spring: ui.SpringOptions{Duration: 500 * time.Millisecond, Bounce: 0.2}})
			back.Children(func() { h.host.Cover(c, next.Album.PicURL, size-32, 14) })
		}
		front := ui.Box(c.Key("qq-fm-cover")).Absolute().Top(0).Left(0).Size(size, size).Radius(16)
		if tr == nil {
			front.Center().Children(func() {
				ui.Icon(c, iconRadio).Size(64, 64).TextColor(ui.RGBA(255, 255, 255, 0.85))
			})
			return
		}
		front.Children(func() { h.host.Cover(c, tr.Album.PicURL, size, 16) })
	})
}

// startFM loads the first songs of the personal radio and starts it; the
// radio then keeps queueing more as it plays.
func (h *Home) startFM() {
	p := h.host.Player()
	if p == nil || h.fmStarting {
		return
	}
	h.fmStarting = true
	h.host.Invalidate()
	go func() {
		list, err := backend.PersonalFM(backend.FMDefault)
		h.Post(func() {
			h.fmStarting = false
			if err != nil || len(list) == 0 {
				h.host.Toast("电台暂不可用，请稍后再试")
				h.host.Invalidate()
				return
			}
			first := tracks(list)
			p.PlayRadio(fmRadio, first, func(got func([]netease.Track)) {
				go func() {
					more, ferr := backend.PersonalFM(backend.FMDefault)
					h.Post(func() {
						if ferr != nil {
							got(nil)
							return
						}
						got(tracks(more))
					})
				}()
			})
			h.host.Invalidate()
		})
	}()
}

func (h *Home) chartsPage(c *ui.Context) {
	if h.homeLoading && len(h.charts) == 0 {
		h.host.CardsSkeleton(c, CardView{})
		return
	}
	if len(h.charts) == 0 {
		h.message(c, "暂无排行榜", h.loadHome)
		return
	}
	official := h.charts[:min(4, len(h.charts))]
	rest := h.charts[len(official):]
	var rows []func(*ui.Context)
	rows = append(rows, func(c *ui.Context) {
		h.heading(c, "排行榜", "每日更新")
	})
	rows = append(rows, func(c *ui.Context) {
		cols := min(4, max(2, h.columns(c, 260)))
		ui.Grid(c).Columns(cols).Gap(20).Children(func() {
			for _, pl := range official {
				h.chartCard(c, pl)
			}
		})
	})
	if len(rest) > 0 {
		rows = append(rows, func(c *ui.Context) {
			h.section(c, "全部榜单", rest, func(id int64) { h.Open(Route{Page: PagePlaylist, ID: id}) })
		})
	}
	h.host.PageList(c, "qq-charts", h.listState(), len(rows), func(i int) { rows[i](c) })
}

// chartCard is a ranking: its cover beside the songs leading it.
func (h *Home) chartCard(c *ui.Context, pl netease.Playlist) {
	t := c.Theme()
	b := ui.ButtonBase(c.Key("qq-chart-" + strconv.FormatInt(pl.ID, 10))).Gap(14).Padding(12).Radius(14).AlignItems(ui.Center).FocusRing(false).Label(pl.Name)
	bg := t.Text.Alpha(0.03)
	if b.Hovered() {
		bg = t.Text.Alpha(0.06)
	}
	b.Background(bg)
	b.Children(func() {
		h.host.Cover(c, playlistCover(pl), 84, 9)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(5).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, pl.Name).FontSize(14).FontWeight(700).SingleLine()
				ui.Text(c, pl.UpdateFreq).FontSize(11).TextColor(t.TextMuted).SingleLine()
			})
			for i, line := range pl.Preview {
				ui.Row(c).Gap(7).Children(func() {
					ui.Text(c, strconv.Itoa(i+1)).FontSize(12).FontWeight(700).Width(10).TextColor(t.TextMuted)
					ui.RichText(c,
						ui.Span{Text: line.First, Weight: 500},
						ui.Span{Text: "  " + line.Second, Color: t.TextMuted},
					).FontSize(12).SingleLine().Shrink(1).MinWidth(0)
				})
			}
		})
	})
	if b.Clicked() {
		h.Open(Route{Page: PagePlaylist, ID: pl.ID})
	}
}

func (h *Home) likedPage(c *ui.Context) {
	if h.state.Profile() == nil {
		h.loginPrompt(c)
		return
	}
	if h.route.Page == PageLikedAlbums {
		if !h.likedAlbumsLoaded {
			h.likedEmpty(c, "正在加载收藏专辑…")
			h.loadLikedAlbums()
			return
		}
		if len(h.likedAlbums) == 0 {
			h.likedEmpty(c, "暂无收藏专辑")
			return
		}
		h.host.RenderCards(c, staticCards(h.likedAlbums), CardView{
			Title: "我喜欢", Count: len(h.likedAlbums), NoPlay: true,
			Header: h.likedHeader,
			OnOpen: func(id int64) { h.Open(Route{Page: PageAlbum, ID: id}) },
			Footer: func(c *ui.Context) {
				h.moreFooter(c, h.likedAlbumsMore, h.likedAlbumsBusy, h.moreLikedAlbums)
			},
		})
		return
	}
	if !h.likedLoaded {
		h.likedEmpty(c, "正在加载喜欢的歌曲…")
		h.loadLiked()
		return
	}
	if h.likedErr != "" && len(h.liked) == 0 {
		h.likedRetry(c, h.likedErr)
		return
	}
	if len(h.liked) == 0 {
		h.likedEmpty(c, "还没有喜欢的歌曲")
		return
	}
	h.host.RenderTracks(c, staticTracks(h.liked), TrackView{
		Title: "我喜欢", Total: len(h.liked),
		OnPlay:    func(i int) { h.play(h.liked, i) },
		OnShuffle: func() { h.shuffle(h.liked) },
		Header:    h.likedHeader,
		Menu:      h.trackMenu, Liked: h.state.Liked, Favorite: h.favorite,
		ShowAlbum: true,
	})
}

func (h *Home) likedHeader(c *ui.Context) {
	t := c.Theme()
	albums := h.route.Page == PageLikedAlbums
	cover := ""
	if len(h.liked) > 0 {
		cover = h.liked[0].Album.PicURL
	} else if len(h.likedAlbums) > 0 {
		cover = playlistCover(h.likedAlbums[0])
	}
	meta := strconv.Itoa(len(h.liked)) + " 首单曲"
	if albums {
		meta = strconv.Itoa(len(h.likedAlbums)) + " 张专辑"
	}
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 8, 0).Children(func() {
		h.coverFrame(c, cover, 212, 14)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			ui.Text(c, "收藏").FontSize(12).FontWeight(600).TextColor(t.Accent)
			ui.Text(c, "我喜欢").FontSize(30).FontWeight(800).LetterSpacing(-0.6)
			ui.Text(c, meta).FontSize(13).TextColor(t.TextMuted)
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				if albums {
					if h.host.PillButton(c, "qq-liked-refresh", iconReload, "刷新", false).Clicked() {
						h.likedAlbumsLoaded = false
						h.host.Invalidate()
						h.loadLikedAlbums()
					}
					return
				}
				b := h.host.PillButton(c, "qq-liked-play", iconPlay, "播放", true)
				if len(h.liked) == 0 {
					b.Disabled(true)
				}
				if b.Clicked() {
					h.play(h.liked, 0)
				}
				sb := h.host.PillButton(c, "qq-liked-shuffle", iconShuffle, "随机播放", false)
				if len(h.liked) == 0 {
					sb.Disabled(true)
				}
				if sb.Clicked() {
					h.shuffle(h.liked)
				}
			})
		})
	})
	ui.Row(c).Margin(28, 0, 18, 0).Children(func() {
		at := 0
		if albums {
			at = 1
		}
		if h.host.Segmented(c, "qq-liked-tabs", &at, "单曲", "专辑") {
			if at == 1 {
				h.Open(Route{Page: PageLikedAlbums})
			} else {
				h.Open(Route{Page: PageLiked})
			}
		}
	})
}

// likedRetry keeps the header and tabs above a failed load, with a retry.
func (h *Home) likedRetry(c *ui.Context, msg string) {
	h.host.PageList(c, "qq-liked-empty", h.listState(), 2, func(i int) {
		if i == 0 {
			h.likedHeader(c)
			return
		}
		ui.Column(c).Justify(ui.Center).AlignItems(ui.Center).Margin(40, 0, 40, 0).Gap(12).Children(func() {
			ui.Text(c, msg).FontSize(13).TextColor(c.Theme().TextMuted)
			if h.host.PillButton(c, "qq-liked-retry", iconReload, "重试", false).Clicked() {
				h.likedErr, h.likedLoaded, h.likedLoading = "", false, false
				h.loadLiked()
			}
		})
	})
}

// likedEmpty keeps the header and tabs above an empty list.
func (h *Home) likedEmpty(c *ui.Context, msg string) {
	h.host.PageList(c, "qq-liked-empty", h.listState(), 2, func(i int) {
		if i == 0 {
			h.likedHeader(c)
			return
		}
		ui.Row(c).Justify(ui.Center).Margin(64, 0, 64, 0).Children(func() {
			ui.Text(c, msg).FontSize(13).TextColor(c.Theme().TextMuted)
		})
	})
}

// coverFrame is a collection's artwork with the shadow the reference gives it.
func (h *Home) coverFrame(c *ui.Context, url string, size, radius float32) {
	box := ui.Box(c).Size(size, size).Radius(radius).Shadow(0, 18, 40, -10, ui.RGBA(0, 0, 0, 0.35))
	box.Children(func() { h.host.Cover(c, url, size, radius) })
}

func (h *Home) purchasedPage(c *ui.Context) {
	if h.state.Profile() == nil {
		h.loginPrompt(c)
		return
	}
	empty := len(h.purchasedSongs) == 0 && len(h.purchasedAlbums) == 0
	if h.purchasedLoading && !h.purchasedLoaded && empty {
		h.host.TrackSkeleton(c, TrackView{})
		return
	}
	if h.purchasedErr != nil && !h.purchasedLoaded && empty {
		h.message(c, "加载失败，请稍后重试", h.loadPurchased)
		return
	}
	// The tabs live in the page header so an empty tab still keeps the way
	// back to the other one.
	if h.purchasedTab == 1 {
		if len(h.purchasedSongs) == 0 {
			h.purchasedEmpty(c, "暂无已购单曲")
			return
		}
		h.host.RenderTracks(c, staticTracks(h.purchasedSongs), TrackView{
			Title: "已购音乐", Sub: "QQ 音乐", Total: len(h.purchasedSongs),
			OnPlay:    func(i int) { h.play(h.purchasedSongs, i) },
			OnShuffle: func() { h.shuffle(h.purchasedSongs) },
			Header:    h.purchasedHeader,
			Menu:      h.trackMenu, Liked: h.state.Liked, Favorite: h.favorite,
			ShowAlbum: true,
			Footer: func(c *ui.Context) {
				h.moreFooter(c, h.purchasedSongsMore, h.purchasedMoreBusy, func() { h.morePurchased(false) })
			},
		})
		return
	}
	if h.purchasedTab == 0 {
		if len(h.purchasedAlbums) == 0 {
			h.purchasedEmpty(c, "暂无已购专辑")
			return
		}
		h.host.RenderCards(c, staticCards(h.purchasedAlbums), CardView{
			Title: "已购音乐", Count: len(h.purchasedAlbums), NoPlay: true,
			Header: h.purchasedHeader,
			OnOpen: func(id int64) { h.Open(Route{Page: PageAlbum, ID: id}) },
			Footer: func(c *ui.Context) {
				h.moreFooter(c, h.purchasedAlbumsMore, h.purchasedMoreBusy, func() { h.morePurchased(true) })
			},
		})
		return
	}
}

func (h *Home) purchasedHeader(c *ui.Context) {
	h.heading(c, "已购音乐", "")
	ui.Row(c).AlignItems(ui.Center).Gap(12).Margin(0, 0, 18, 0).Children(func() {
		at := h.purchasedTab
		if h.host.Segmented(c, "qq-purchased-tabs", &at, "专辑", "单曲") {
			h.purchasedTab = at
			h.host.Invalidate()
		}
		ui.Box(c).Grow(1)
		if len(h.purchasedSongs) > 0 {
			if h.host.PillButton(c, "qq-purchased-play", iconPlay, "播放", true).Clicked() {
				h.play(h.purchasedSongs, 0)
			}
		}
		if h.host.PillButton(c, "qq-purchased-refresh", iconReload, "刷新", false).Clicked() {
			h.refreshPurchased()
		}
	})
}

// moreFooter is the foot of a page that loads more, or says there is no more.
func (h *Home) moreFooter(c *ui.Context, more, busy bool, load func()) {
	if !more {
		ui.Text(c, "没有更多了").FontSize(12).TextColor(c.Theme().TextMuted)
		return
	}
	label := "加载更多"
	if busy {
		label = "正在加载…"
	}
	b := h.host.PillButton(c, "qq-more", iconReload, label, false)
	if busy {
		b.Disabled(true)
	}
	if b.Clicked() {
		load()
	}
}

// purchasedEmpty keeps the header and tabs above an empty result instead of
// handing zero rows to the track and card lists.
func (h *Home) purchasedEmpty(c *ui.Context, msg string) {
	h.host.PageList(c, "qq-purchased-empty", h.listState(), 2, func(i int) {
		if i == 0 {
			h.purchasedHeader(c)
			return
		}
		ui.Row(c).Justify(ui.Center).Margin(64, 0, 64, 0).Children(func() {
			ui.Text(c, msg).FontSize(13).TextColor(c.Theme().TextMuted)
		})
	})
}

func (h *Home) playlistPage(c *ui.Context) {
	h.mu.Lock()
	p := h.playlists[h.route.ID]
	h.mu.Unlock()
	if p == nil || p.loading {
		h.host.TrackSkeleton(c, TrackView{})
		return
	}
	if p.err != nil {
		h.message(c, "加载失败，请稍后重试", func() {
			h.mu.Lock()
			delete(h.playlists, h.route.ID)
			h.mu.Unlock()
			h.loadPlaylist(h.route.ID)
		})
		return
	}
	pl := p.pl
	h.host.RenderTracks(c, staticTracks(pl.Tracks), TrackView{
		Title: pl.Name, Sub: "歌单", Cover: pl.Cover(), Total: len(pl.Tracks),
		OnPlay:    func(i int) { h.play(pl.Tracks, i) },
		OnShuffle: func() { h.shuffle(pl.Tracks) },
		Header:    func(c *ui.Context) { h.playlistHeader(c, pl) },
		Menu:      h.trackMenu, Liked: h.state.Liked, Favorite: h.favorite,
		ShowAlbum: true,
	})
}

func (h *Home) playlistHeader(c *ui.Context, pl netease.Playlist) {
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 26, 0).Children(func() {
		h.host.Cover(c, playlistCover(pl), 212, 14)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			kind := "歌单"
			if pl.UpdateFreq != "" {
				kind = "榜单 · " + pl.UpdateFreq
			}
			ui.Text(c, kind).FontSize(12).FontWeight(600).TextColor(c.Theme().Accent)
			ui.Text(c, pl.Name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			var meta []string
			if pl.Creator.Nickname != "" {
				meta = append(meta, pl.Creator.Nickname)
			}
			meta = append(meta, strconv.Itoa(len(pl.Tracks))+" 首")
			if pl.PlayCount > 0 {
				meta = append(meta, plays(pl.PlayCount)+" 次播放")
			}
			ui.Text(c, strings.Join(meta, " · ")).FontSize(13).TextColor(c.Theme().TextMuted)
			h.description(c, pl.Description)
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				if h.host.PillButton(c, "qq-list-play", iconPlay, "播放", true).Clicked() {
					h.play(pl.Tracks, 0)
				}
				if h.host.PillButton(c, "qq-list-shuffle", iconShuffle, "随机播放", false).Clicked() {
					h.shuffle(pl.Tracks)
				}
				h.playlistMore(c, pl)
			})
		})
	})
}

// playlistMore offers the actions the account has on this list: renaming or
// deleting the ones it made, unfavouriting the ones it saved.
func (h *Home) playlistMore(c *ui.Context, pl netease.Playlist) {
	if backend.OwnedPlaylist(pl.ID) {
		h.host.MoreButton(c, "qq-list-more", func(m *ui.Menu) { h.playlistMenu(m, pl) })
		return
	}
	for _, item := range h.state.Playlists() {
		if item.ID == pl.ID {
			h.host.MoreButton(c, "qq-list-more", func(m *ui.Menu) {
				if m.Item("取消收藏…").Chosen() {
					h.unfavoritePlaylist(pl)
				}
			})
			return
		}
	}
}

func (h *Home) unfavoritePlaylist(pl netease.Playlist) {
	h.confirmRemove(pl, true)
}

// removeFromPlaylist takes a song out of one the account made.
func (h *Home) removeFromPlaylist(pid, trackID int64) {
	go func() {
		err := backend.RemoveFromPlaylist(pid, trackID)
		h.Post(func() {
			if err != nil {
				h.host.Toast("移出歌单失败，请稍后再试")
				return
			}
			h.mu.Lock()
			if p := h.playlists[pid]; p != nil {
				out := p.pl.Tracks[:0]
				for _, tr := range p.pl.Tracks {
					if tr.ID != trackID {
						out = append(out, tr)
					}
				}
				p.pl.Tracks = out
				p.pl.TrackCount = len(out)
			}
			h.mu.Unlock()
			h.host.Toast("已移出歌单")
			h.host.Invalidate()
		})
	}()
}

// description shows two lines of prose; prose longer than that opens a
// card with the whole of it after a moment under the pointer.
func (h *Home) description(c *ui.Context, raw string) {
	text := cleanDescription(raw)
	if text == "" {
		return
	}
	t := c.Theme()
	preview := strings.ReplaceAll(text, "\n", " ")
	anchor := ui.Text(c.Key("qq-description"), preview).FontSize(12.5).TextColor(t.TextMuted).
		MaxLines(2).LineHeight(1.45).Margin(2, 0, 0, 0).Label("简介").Description(text).Focusable()
	width := anchor.LayoutBounds().W
	if width <= 0 {
		c.Invalidate()
		return
	}
	font := ui.Font{Family: t.Font, Size: 12.5, LineHeight: 1.45, Lang: c.Locale()}
	_, height := ui.ShapeText(preview, font, width).Size()
	if height <= 2*font.Size*font.LineHeight+0.5 {
		return
	}
	s := ui.Local(anchor, "description", func() descHover { return descHover{} })
	now := c.Now()
	hovered, focused := anchor.Hovered(), anchor.FocusVisible()
	if hovered || focused {
		if s.entered.IsZero() {
			s.entered = now
		}
		if focused {
			s.open = true
		} else if !s.open {
			if wait := 500*time.Millisecond - now.Sub(s.entered); wait > 0 {
				c.After(wait)
			} else {
				s.open = true
			}
		}
	} else {
		s.entered = time.Time{}
		s.open = false
	}
	ui.PopoverBase(c.Key("qq-description-card"), anchor, &s.open, func(panel ui.Element) {
		panel.Width(440).Padding(14, 16).Radius(12).Margin(8, 0, 0, 0).
			Background(t.Background).Border(1, t.Border).Shadow(0, 10, 30, -8, ui.RGBA(0, 0, 0, 0.3)).Label("完整简介")
		ui.Scroll(c.Key("qq-description-scroll")).Height(220).Children(func() {
			ui.Text(c, text).FontSize(12.5).LineHeight(1.55)
		})
	})
}

type descHover struct {
	entered time.Time
	open    bool
}

// cleanDescription turns QQ's HTML prose into the plain lines the page shows.
func cleanDescription(s string) string {
	s = html.UnescapeString(s)
	for _, tag := range []string{"<br>", "<br/>", "<br />", "<BR>", "<BR/>", "<BR />"} {
		s = strings.ReplaceAll(s, tag, "\n")
	}
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

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

func (h *Home) albumPage(c *ui.Context) {
	h.mu.Lock()
	p := h.albums[h.route.ID]
	h.mu.Unlock()
	if p == nil || p.loading {
		h.host.TrackSkeleton(c, TrackView{})
		return
	}
	if p.err != nil {
		h.message(c, "加载失败，请稍后重试", func() { h.retryAlbum(h.route.ID) })
		return
	}
	h.host.RenderTracks(c, staticTracks(p.tracks), TrackView{
		Title: p.al.Name, Sub: "专辑", Cover: p.al.PicURL, Total: len(p.tracks),
		OnPlay:    func(i int) { h.play(p.tracks, i) },
		OnShuffle: func() { h.shuffle(p.tracks) },
		Header: func(c *ui.Context) {
			ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 26, 0).Children(func() {
				h.coverFrame(c, p.al.PicURL, 212, 14)
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
					ui.Text(c, "专辑").FontSize(12).FontWeight(600).TextColor(c.Theme().Accent)
					ui.Text(c, p.al.Name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
					t := c.Theme()
					ui.RichText(c).FontSize(13).TextColor(t.TextMuted).Children(func() {
						for i := range p.al.Artists {
							ar := p.al.Artists[i]
							if i > 0 {
								ui.Text(c, " / ")
							}
							link := ui.Link(c, ar.Name, "").TextColor(t.TextMuted)
							if link.Hovered() {
								link.TextColor(t.Text)
							}
							link.OnClick(func() { h.Open(Route{Page: PageArtist, ID: ar.ID}) })
						}
						meta := " · " + strconv.Itoa(len(p.tracks)) + " 首"
						if p.al.PublishTime > 0 {
							meta = " · " + time.UnixMilli(p.al.PublishTime).Format("2006-01-02") + meta
						}
						ui.Text(c, meta)
					})
					h.description(c, p.al.Description)
					ui.Row(c).Gap(10).Margin(10, 0, 0, 0).AlignItems(ui.Center).Children(func() {
						if h.host.PillButton(c, "qq-album-play", iconPlay, "播放", true).Clicked() {
							h.play(p.tracks, 0)
						}
						h.albumLike(c, p)
					})
				})
			})
		},
		Menu: h.trackMenu, Liked: h.state.Liked, Favorite: h.favorite,
		ShowAlbum: false,
	})
}

// retryAlbum drops a failed album so its page loads it again.
func (h *Home) retryAlbum(id int64) {
	h.mu.Lock()
	delete(h.albums, id)
	h.mu.Unlock()
	h.loadAlbum(id)
}

// retryArtist drops a failed artist so its page loads again.
func (h *Home) retryArtist(id int64) {
	h.mu.Lock()
	delete(h.artists, id)
	h.mu.Unlock()
	h.loadArtist(id)
}

// albumLike toggles the album's collection, which QQ keys by album MID.
func (h *Home) albumLike(c *ui.Context, p *albumPage) {
	t := c.Theme()
	icon, label, color := iconHeart, "收藏专辑", t.TextMuted
	if p.liked {
		icon, label, color = iconHeartFill, "取消收藏专辑", ui.Hex("#e5484d")
	}
	b := ui.ButtonBase(c.Key("qq-album-like")).Size(36, 36).Radius(999).Center().FocusRing(false).Label(label)
	if b.Hovered() {
		b.Background(t.Text.Alpha(0.06))
	}
	b.Children(func() { ui.Icon(c, icon).Size(20, 20).TextColor(color) })
	if p.likeBusy || p.al.MID == "" {
		b.Disabled(true)
	}
	if b.Clicked() {
		if h.state.Profile() == nil {
			h.openLogin()
			return
		}
		if !p.likeKnown {
			return
		}
		want := !p.liked
		p.likeBusy = true
		go func() {
			err := backend.LikeAlbum(p.al.MID, want)
			h.Post(func() {
				p.likeBusy = false
				if err != nil {
					h.host.Toast("操作失败，请稍后再试")
					return
				}
				p.liked = want
				h.loadLikedAlbums()
				h.host.Invalidate()
			})
		}()
	}
}

func (h *Home) artistPage(c *ui.Context) {
	h.mu.Lock()
	p := h.artists[h.route.ID]
	h.mu.Unlock()
	if p == nil || p.loading {
		h.host.TrackSkeleton(c, TrackView{})
		return
	}
	if p.err != nil {
		h.message(c, "加载失败，请稍后重试", func() { h.retryArtist(h.route.ID) })
		return
	}
	switch h.artistTab {
	case 2:
		if len(p.albums) == 0 {
			h.artistEmpty(c, p, "暂无专辑")
			return
		}
		h.host.RenderCards(c, staticCards(p.albums), CardView{
			Title: p.ar.Name, Count: len(p.albums), NoPlay: true,
			Header: func(c *ui.Context) { h.artistHeader(c, p) },
			OnOpen: func(id int64) { h.Open(Route{Page: PageAlbum, ID: id}) },
		})
	case 1:
		if len(p.latest) == 0 {
			h.artistEmpty(c, p, "暂无最新歌曲")
			return
		}
		h.host.RenderTracks(c, staticTracks(p.latest), TrackView{
			Title: p.ar.Name, Sub: "最新", Total: len(p.latest),
			OnPlay:    func(i int) { h.play(p.latest, i) },
			OnShuffle: func() { h.shuffle(p.latest) },
			Header:    func(c *ui.Context) { h.artistHeader(c, p) },
			Menu:      h.trackMenu, Liked: h.state.Liked, Favorite: h.favorite,
			ShowAlbum: true,
		})
	default:
		if len(p.tracks) == 0 {
			h.artistEmpty(c, p, "暂无歌曲")
			return
		}
		h.host.RenderTracks(c, staticTracks(p.tracks), TrackView{
			Title: p.ar.Name, Sub: "歌手", Total: len(p.tracks),
			OnPlay:    func(i int) { h.play(p.tracks, i) },
			OnShuffle: func() { h.shuffle(p.tracks) },
			Header:    func(c *ui.Context) { h.artistHeader(c, p) },
			Menu:      h.trackMenu, Liked: h.state.Liked, Favorite: h.favorite,
			ShowAlbum: false,
		})
	}
}

// artistEmpty keeps the header and tabs above an empty tab.
func (h *Home) artistEmpty(c *ui.Context, p *artistPage, msg string) {
	h.host.PageList(c, "qq-artist-empty", h.listState(), 2, func(i int) {
		if i == 0 {
			h.artistHeader(c, p)
			return
		}
		ui.Row(c).Justify(ui.Center).Margin(64, 0, 64, 0).Children(func() {
			ui.Text(c, msg).FontSize(13).TextColor(c.Theme().TextMuted)
		})
	})
}

func (h *Home) artistHeader(c *ui.Context, p *artistPage) {
	ar := p.ar
	photo := ar.Avatar
	if photo == "" {
		photo = ar.PicURL
	}
	songs := p.tracks
	if h.artistTab == 1 {
		songs = p.latest
	}
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 8, 0).Children(func() {
		h.coverFrame(c, photo, 212, 106)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			ui.Text(c, "歌手").FontSize(12).FontWeight(600).TextColor(c.Theme().Accent)
			ui.Text(c, ar.Name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			var meta []string
			if aka := distinctStrings(append([]string{ar.Trans}, ar.Alias...)); len(aka) > 0 {
				meta = append(meta, strings.Join(aka, " / "))
			}
			if ar.MusicSize > 0 {
				meta = append(meta, strconv.Itoa(ar.MusicSize)+" 首歌曲")
			}
			if ar.AlbumSize > 0 {
				meta = append(meta, strconv.Itoa(ar.AlbumSize)+" 张专辑")
			}
			ui.Text(c, strings.Join(meta, " · ")).FontSize(13).TextColor(c.Theme().TextMuted)
			if desc := strings.TrimSpace(ar.BriefDesc); desc != "" {
				ui.Text(c, strings.ReplaceAll(desc, "\n", " ")).FontSize(12.5).TextColor(c.Theme().TextMuted).MaxLines(2).LineHeight(1.45).Margin(2, 0, 0, 0)
			}
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).Children(func() {
				if h.host.PillButton(c, "qq-artist-play", iconPlay, "播放", true).Clicked() {
					h.play(songs, 0)
				}
				if h.host.PillButton(c, "qq-artist-shuffle", iconShuffle, "随机播放", false).Clicked() {
					h.shuffle(songs)
				}
			})
		})
	})
	ui.Row(c).AlignItems(ui.Center).Gap(12).Margin(34, 0, 14, 0).Children(func() {
		ui.Text(c, []string{"热门歌曲", "最新歌曲", "专辑"}[h.artistTab]).FontSize(19).FontWeight(700).LetterSpacing(-0.3)
		ui.Box(c).Grow(1)
		at := h.artistTab
		if h.host.Segmented(c, "qq-artist-tabs", &at, "热门", "最新", "专辑") {
			h.artistTab = at
			h.host.Invalidate()
		}
	})
}

func distinctStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func (h *Home) userPage(c *ui.Context) {
	prof := h.state.Profile()
	if prof == nil {
		h.loginPrompt(c)
		return
	}
	h.mu.Lock()
	p := h.users[h.route.ID]
	if p == nil {
		p = &userPage{}
		h.users[h.route.ID] = p
	}
	h.mu.Unlock()
	if p.user == nil && p.loading == 0 {
		h.loadUser(prof.UserID)
	}
	h.host.PageList(c, "qq-user", h.listState(), 2, func(i int) {
		if i == 0 {
			h.userHeader(c, prof, p)
			return
		}
		h.userPlaylists(c, p)
	})
}

func (h *Home) userHeader(c *ui.Context, prof *backend.Profile, p *userPage) {
	name, avatar, signature := prof.Nickname, prof.AvatarURL, ""
	if p != nil && p.user != nil {
		name, avatar, signature = p.user.Nickname, p.user.AvatarURL, p.user.Signature
	}
	ui.Row(c).Gap(28).AlignItems(ui.Center).Padding(0, 0, 26, 0).Children(func() {
		h.coverFrame(c, avatar, 212, 106)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Start).Gap(8).Children(func() {
			ui.Text(c, name).MinWidth(0).FontSize(30).FontWeight(700).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			if p != nil {
				h.membershipBadge(c, p)
			}
			if signature != "" {
				ui.Text(c, signature).FontSize(12.5).TextColor(c.Theme().TextMuted).MaxLines(2).LineHeight(1.45)
			}
		})
	})
}

func (h *Home) membershipBadge(c *ui.Context, p *userPage) {
	t := c.Theme()
	label, ink, bg := "会员状态未知", t.TextMuted, t.Text.Alpha(0.06)
	switch p.membership {
	case backend.MembershipSuper:
		label, ink, bg = "超级会员", t.Accent, t.Accent.Alpha(0.16)
	case backend.MembershipGreen:
		label, ink, bg = "绿钻会员", t.Accent, t.Accent.Alpha(0.10)
	case backend.MembershipFree:
		label = "免费账号"
	}
	if p.memberErr != nil {
		label, ink, bg = "会员状态暂不可用", t.TextMuted, t.Text.Alpha(0.06)
	} else if p.memberBusy && p.membership == "" {
		label = "正在查询会员状态"
	}
	ui.Box(c).Padding(3, 8).Radius(4).Background(bg).Children(func() {
		ui.Text(c, label).FontSize(11).FontWeight(500).TextColor(ink).SingleLine()
	})
}

func (h *Home) userPlaylists(c *ui.Context, p *userPage) {
	ui.Text(c, "创建的歌单").FontSize(19).FontWeight(700).LetterSpacing(-0.3).Margin(0, 0, 14, 0)
	if len(p.playlists) == 0 {
		label := "暂无公开歌单"
		if p != nil && p.loading > 0 {
			label = "正在加载歌单…"
		}
		ui.Row(c).Justify(ui.Center).Margin(24, 0, 48, 0).Children(func() {
			ui.Text(c, label).FontSize(13).TextColor(c.Theme().TextMuted)
		})
		return
	}
	cols := h.columns(c, 168)
	w, _ := c.Size()
	inner := w - 232 - 80
	cardW := (inner - 20*float32(cols-1)) / float32(cols)
	ui.Grid(c).Columns(cols).GapX(20).GapY(28).Children(func() {
		for _, pl := range p.playlists {
			h.playlistCard(c, pl, cardW, func(id int64) { h.Open(Route{Page: PagePlaylist, ID: id}) })
		}
	})
}

// playlistCard is a cover card: its count over the cover, a play button
// under the pointer, and its name below.
func (h *Home) playlistCard(c *ui.Context, pl netease.Playlist, cardW float32, open func(int64)) {
	b := ui.ButtonBase(c.Key("qq-card-" + strconv.FormatInt(pl.ID, 10))).Column().Gap(8).AlignItems(ui.Stretch).FocusRing(false).Label(pl.Name)
	hovered := b.Hovered()
	b.Children(func() {
		art := ui.Box(c).Size(cardW, cardW).Radius(10)
		if hovered {
			art.AnimatePose(ui.Pose{Y: -3}, ui.Snappy)
		}
		art.Children(func() {
			h.host.Cover(c, playlistCover(pl), cardW, 10)
			if pl.PlayCount > 0 {
				ui.Row(c).Absolute().Top(8).Right(8).Padding(2, 7).Radius(999).Gap(3).AlignItems(ui.Center).
					Background(ui.RGBA(0, 0, 0, 0.38)).Children(func() {
					ui.Icon(c, iconHeadset).Size(10, 10).TextColor(ui.RGBA(255, 255, 255, 0.95))
					ui.Text(c, plays(pl.PlayCount)).FontSize(10.5).FontWeight(600).TextColor(ui.RGBA(255, 255, 255, 0.95))
				})
			}
			if hovered {
				pb := ui.ButtonBase(c.Key("play")).Absolute().Right(10).Bottom(10).Size(36, 36).Radius(18).Center().
					Background(ui.RGBA(255, 255, 255, 0.92)).FocusRing(false).Label("播放")
				pb.Children(func() { ui.Icon(c, iconPlay).Size(14, 14).TextColor(ui.RGBA(20, 20, 22, 1)) })
				if pb.Clicked() {
					h.playPlaylist(pl.ID)
				}
			}
		})
		ui.Text(c, pl.Name).FontSize(13).FontWeight(500).MaxLines(1)
		if pl.UpdateFreq != "" {
			ui.Text(c, pl.UpdateFreq).FontSize(12).TextColor(c.Theme().TextMuted).SingleLine()
		}
	})
	if b.Clicked() {
		open(pl.ID)
	}
}

const (
	searchSongs = iota
	searchArtists
	searchAlbums
)

func (h *Home) searchPage(c *ui.Context) {
	if h.search.query == "" {
		h.message(c, "输入关键词搜索 QQ 音乐", nil)
		return
	}
	switch h.searchKind {
	case searchArtists:
		p := h.search.artists
		if p == nil {
			h.host.SearchSkeleton(c)
			return
		}
		if p.Total() == 0 {
			h.searchEmpty(c, "没有找到相关歌手")
			return
		}
		count := 0
		if p.Total() > 0 {
			count = p.Total()
		}
		h.host.RenderCards(c, p, CardView{
			Title: "“" + h.search.query + "”", Count: count, Round: true,
			Header: h.searchHeader,
			OnOpen: func(id int64) { h.Open(Route{Page: PageArtist, ID: id}) },
		})
	case searchAlbums:
		p := h.search.albums
		if p == nil {
			h.host.SearchSkeleton(c)
			return
		}
		if p.Total() == 0 {
			h.searchEmpty(c, "没有找到相关专辑")
			return
		}
		count := 0
		if p.Total() > 0 {
			count = p.Total()
		}
		h.host.RenderCards(c, p, CardView{
			Title: "“" + h.search.query + "”", Count: count, NoPlay: true,
			Header: h.searchHeader,
			OnOpen: func(id int64) { h.Open(Route{Page: PageAlbum, ID: id}) },
		})
	default:
		p := h.search.songs
		if p == nil {
			h.host.SearchSkeleton(c)
			return
		}
		if p.Total() == 0 {
			h.searchEmpty(c, "没有找到相关单曲")
			return
		}
		h.host.RenderTracks(c, lazyTracks(p, 20), TrackView{
			Title:     "“" + h.search.query + "”",
			OnPlay:    h.playSearch,
			OnShuffle: h.shuffleSearch,
			Header:    h.searchHeader,
			Menu:      h.trackMenu, Liked: h.state.Liked, Favorite: h.favorite,
			ShowAlbum: true,
		})
	}
}

// searchHeader shows what was searched for, how many it found, and the tabs
// of the kinds it may look for instead.
func (h *Home) searchHeader(c *ui.Context) {
	sub := "搜索"
	switch h.searchKind {
	case searchArtists:
		sub = "搜索 · 找到 " + strconv.Itoa(pagerTotal(h.search.artists)) + " 位歌手"
	case searchAlbums:
		sub = "搜索 · 找到 " + strconv.Itoa(pagerTotal(h.search.albums)) + " 张专辑"
	default:
		n := 0
		if p := h.search.songs; p != nil && p.Total() > 0 {
			n = p.Total()
		}
		sub = "搜索 · 找到 " + strconv.Itoa(n) + " 首单曲"
	}
	h.heading(c, "“"+h.search.query+"”", sub)
	ui.Row(c).Margin(0, 0, 18, 0).Children(func() {
		at := h.searchKind
		if h.host.Segmented(c, "qq-search-tabs", &at, "单曲", "歌手", "专辑") {
			h.searchKind = at
			h.host.Invalidate()
		}
	})
}

func pagerTotal[T any](p *paging.Pager[T]) int {
	if p == nil || p.Total() < 0 {
		return 0
	}
	return p.Total()
}

// searchEmpty keeps the header and tabs above an empty kind instead of
// handing zero rows to the track or card lists.
func (h *Home) searchEmpty(c *ui.Context, msg string) {
	h.host.PageList(c, "qq-search-empty", h.listState(), 2, func(i int) {
		if i == 0 {
			h.searchHeader(c)
			return
		}
		ui.Row(c).Justify(ui.Center).Margin(64, 0, 64, 0).Children(func() {
			ui.Text(c, msg).FontSize(13).TextColor(c.Theme().TextMuted)
		})
	})
}

func (h *Home) startSearch(q string) {
	p := paging.New[netease.Track](20, 0, h.Post, func(page int) ([]netease.Track, int, error) {
		res, err := backend.SearchPage(q, backend.SearchSongs, 20, (page-1)*20)
		if err != nil {
			return nil, 0, err
		}
		return tracks(res.Songs), res.Total, nil
	})
	artists := paging.New[netease.Playlist](20, 0, h.Post, func(page int) ([]netease.Playlist, int, error) {
		res, err := backend.SearchPage(q, backend.SearchArtists, 20, (page-1)*20)
		if err != nil {
			return nil, 0, err
		}
		return artistCards(res.Artists), res.Total, nil
	})
	albums := paging.New[netease.Playlist](20, 0, h.Post, func(page int) ([]netease.Playlist, int, error) {
		res, err := backend.SearchPage(q, backend.SearchAlbums, 20, (page-1)*20)
		if err != nil {
			return nil, 0, err
		}
		return albumCards(res.Albums), res.Total, nil
	})
	h.search = searchPage{query: q, songs: p, albums: albums, artists: artists}
	_ = p.At(0)
	_ = albums.At(0)
	_ = artists.At(0)
}

// playSearch plays the loaded pages from at, then keeps queueing the pages
// that come after them.
func (h *Home) playSearch(at int) {
	p := h.search.songs
	player := h.host.Player()
	if p == nil || player == nil {
		return
	}
	var first []netease.Track
	for i := 0; i < p.Len(); i++ {
		if tr := p.At(i); tr != nil {
			first = append(first, *tr)
		}
	}
	if len(first) == 0 {
		return
	}
	if at < 0 || at >= len(first) {
		at = 0
	}
	q := h.search.query
	offset := len(first)
	var mu sync.Mutex
	player.PlayList(first, at, func(got func([]netease.Track)) {
		mu.Lock()
		from := offset
		mu.Unlock()
		go func() {
			res, err := backend.SearchPage(q, backend.SearchSongs, 20, from)
			h.Post(func() {
				if err != nil || len(res.Songs) == 0 {
					got(nil)
					return
				}
				more := tracks(res.Songs)
				mu.Lock()
				offset += len(more)
				mu.Unlock()
				got(more)
			})
		}()
	})
}

func (h *Home) shuffleSearch() {
	q := h.search.query
	go func() {
		var all []netease.Track
		for page := 1; page <= 3; page++ {
			res, err := backend.SearchPage(q, backend.SearchSongs, 20, (page-1)*20)
			if err != nil {
				break
			}
			all = append(all, tracks(res.Songs)...)
			if len(res.Songs) < 20 {
				break
			}
		}
		h.Post(func() {
			if len(all) > 0 {
				h.shuffle(all)
			}
		})
	}()
}

func (h *Home) trackPage(c *ui.Context, title, sub string, list []netease.Track) {
	h.host.RenderTracks(c, staticTracks(list), TrackView{
		Title: title, Sub: sub, Total: len(list),
		OnPlay:    func(i int) { h.play(list, i) },
		OnShuffle: func() { h.shuffle(list) },
		Header:    func(c *ui.Context) { h.heading(c, title, sub) },
		Menu:      h.trackMenu, Liked: h.state.Liked, Favorite: h.favorite,
		ShowAlbum: true,
	})
}

func (h *Home) heading(c *ui.Context, title, sub string) {
	ui.Column(c).Gap(4).Margin(0, 0, 18, 0).Children(func() {
		if sub != "" {
			ui.Text(c, sub).FontSize(12).FontWeight(600).TextColor(c.Theme().TextMuted).LetterSpacing(0.4)
		}
		ui.Text(c, title).FontSize(30).FontWeight(700).LetterSpacing(-0.6)
	})
}

func (h *Home) cardsPage(c *ui.Context, title string, cards []netease.Playlist, open, play func(int64)) {
	if len(cards) == 0 {
		h.message(c, "暂无内容", nil)
		return
	}
	header := func(c *ui.Context) { h.heading(c, title, "QQ 音乐") }
	h.host.RenderCards(c, staticCards(cards), CardView{
		Title: title, Count: len(cards), Header: header,
		OnOpen: open,
		OnPlay: play,
	})
}

func (h *Home) message(c *ui.Context, msg string, retry func()) {
	ui.Column(c).Fill().Center().Gap(12).Children(func() {
		ui.Text(c, msg).FontSize(13).TextColor(c.Theme().TextMuted)
		if retry != nil && h.host.PillButton(c, "qq-retry", iconReload, "重试", false).Clicked() {
			retry()
		}
	})
}

func (h *Home) loginPrompt(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(12).Children(func() {
		ui.Text(c, "登录 QQ 音乐").FontSize(20).FontWeight(700)
		ui.Text(c, "登录后可查看我喜欢、收藏专辑、已购音乐和个人歌单").FontSize(13).TextColor(c.Theme().TextMuted)
		if h.host.PillButton(c, "qq-login-prompt", iconUser, "登录 QQ 音乐", true).Clicked() {
			h.openLogin()
		}
	})
}

func (h *Home) favorite(tr netease.Track, liked bool) {
	go func() {
		err := h.state.SetFavorite(context.Background(), &tr, liked)
		h.Post(func() {
			if err != nil {
				h.host.Toast("操作失败，请稍后再试")
				h.host.Invalidate()
				return
			}
			h.mu.Lock()
			if liked {
				found := false
				for _, item := range h.liked {
					if item.ID == tr.ID {
						found = true
						break
					}
				}
				if !found {
					h.liked = append([]netease.Track{tr}, h.liked...)
				}
			} else {
				out := h.liked[:0]
				for _, item := range h.liked {
					if item.ID != tr.ID {
						out = append(out, item)
					}
				}
				h.liked = out
			}
			h.mu.Unlock()
			h.host.Invalidate()
		})
	}()
}

func (h *Home) trackMenu(m *ui.Menu, tr *netease.Track, _ int64) {
	if tr == nil {
		return
	}
	if m.Item("复制歌名").Chosen() {
		h.host.CopyText(tr.Name)
	}
	if m.Item("复制歌名和歌手").Chosen() {
		h.host.CopyText(tr.Name + " - " + tr.ArtistNames())
	}
	m.Separator()
	if m.Item("复制链接").Chosen() {
		h.host.CopyText("https://y.qq.com/n/ryqq/songDetail/" + tr.Ref)
	}
	for _, ar := range tr.Artists {
		if ar.ID != 0 && m.Item("查看歌手："+ar.Name).Chosen() {
			h.Open(Route{Page: PageArtist, ID: ar.ID})
		}
	}
	if tr.Album.ID != 0 && m.Item("查看专辑："+tr.Album.Name).Chosen() {
		h.Open(Route{Page: PageAlbum, ID: tr.Album.ID})
	}
	if h.state.Profile() == nil {
		return
	}
	m.Separator()
	m.Submenu("添加到歌单", func(sub *ui.Menu) {
		if sub.Item("新建歌单…").Chosen() {
			h.nameOpen, h.name, h.editing, h.pending, h.nameErr = true, "", nil, []netease.Track{*tr}, ""
		}
		for _, pl := range h.state.Playlists() {
			pl := pl
			if pl.SpecialType == 5 || h.route.Page == PagePlaylist && h.route.ID == pl.ID {
				continue
			}
			if sub.Item(pl.Name).Chosen() {
				h.requestAddToPlaylist(pl.ID, *tr)
			}
		}
	})
	if h.route.Page == PagePlaylist && backend.OwnedPlaylist(h.route.ID) {
		m.Separator()
		if m.Item("从歌单中删除").Chosen() {
			h.removeFromPlaylist(h.route.ID, tr.ID)
		}
	}
}

func (h *Home) Started(tr *netease.Track) { h.started(tr) }

func (h *Home) Settings(c *ui.Context) {
	t := c.Theme()
	ui.Scroll(c).Fill().Padding(24, 28, 32, 28).Gap(8).Children(func() {
		ui.Text(c, "QQ 音乐").FontSize(12).FontWeight(600).TextColor(t.TextMuted).Margin(14, 4, 4, 4)
		ui.Column(c).Radius(12).Background(t.Background).Border(1, t.Border).Padding(12, 16).Gap(8).Children(func() {
			name := "未登录"
			if p := h.state.Profile(); p != nil {
				name = p.Nickname
			}
			ui.Text(c, name).FontSize(13.5).FontWeight(500)
			if h.state.Profile() == nil {
				if h.host.PillButton(c, "qq-settings-login", iconUser, "登录 QQ 音乐", true).Clicked() {
					h.openLogin()
				}
			} else if h.host.PillButton(c, "qq-settings-logout", iconSignOut, "退出登录", false).Clicked() {
				backend.Logout()
				h.signedIn(nil)
			}
		})
		h.host.SharedSettings(c)
	})
}
