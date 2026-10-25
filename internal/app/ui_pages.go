package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/player"
)

const pagePad = 40

// columns returns how many cards of at least min DIPs fit across the page.
func (a *app) columns(c *ui.Context, min float32) int {
	w, _ := c.Size()
	inner := w - sidebarWidth - 2*pagePad
	return max(2, int((inner+20)/(min+20)))
}

func (a *app) heading(c *ui.Context, title, sub string) {
	ui.Column(c).Gap(4).Margin(0, 0, 18, 0).Children(func() {
		if sub != "" {
			ui.Text(c, sub).FontSize(12).FontWeight(600).TextColor(a.pal.muted).LetterSpacing(0.4)
		}
		ui.Text(c, title).FontSize(30).FontWeight(700).LetterSpacing(-0.6)
	})
}

func (a *app) section(c *ui.Context, title string) {
	ui.Text(c, title).FontSize(19).FontWeight(700).LetterSpacing(-0.3).Margin(34, 0, 14, 0)
}

func (a *app) homePage(c *ui.Context) {
	if len(a.featured) == 0 && len(a.charts) == 0 {
		a.centered(c, a.homeLoading, a.homeErr, a.loadHome)
		return
	}
	now := c.Now()
	top, bottom := a.insets(c)
	ui.Scroll(c.Key("home")).TrackScroll(&a.pageScroll).Grow(1).Padding(top+pageGap, pagePad, bottom+48, pagePad).ScrollbarInsets(top, 0, bottom, 0).Children(func() {
		a.startMark(c)
		defer a.endMark(c)
		hello := greeting(now)
		if a.profile != nil {
			hello += "，" + a.profile.Nickname
		}
		a.heading(c, hello, fmt.Sprintf("%d月%d日 %s", now.Month(), now.Day(), weekday(now)))
		a.dailyCard(c, now)

		if len(a.recPlaylists) > 0 {
			a.section(c, "今日为你推荐")
			ui.Grid(c).Columns(a.columns(c, 168)).GapX(20).GapY(28).Children(func() {
				for i := range a.recPlaylists[:min(len(a.recPlaylists), a.columns(c, 168)*2)] {
					a.playlistCard(c.Key(a.recPlaylists[i].ID), &a.recPlaylists[i])
				}
			})
		}

		a.section(c, "官方榜单")
		official := a.charts[:min(4, len(a.charts))]
		cols := min(4, max(2, a.columns(c, 260)))
		ui.Grid(c).Columns(cols).Gap(20).Children(func() {
			for i := range official {
				a.chartCard(c.Key(official[i].ID), &official[i])
			}
		})

		a.section(c, "推荐歌单")
		ui.Grid(c).Columns(a.columns(c, 168)).GapX(20).GapY(28).Children(func() {
			for i := range a.featured {
				a.playlistCard(c.Key(a.featured[i].ID), &a.featured[i])
			}
		})
	})
}

func weekday(t time.Time) string {
	return [...]string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}[t.Weekday()]
}

func (a *app) chartsPage(c *ui.Context) {
	if len(a.charts) == 0 {
		a.centered(c, a.homeLoading, a.homeErr, a.loadHome)
		return
	}
	top, bottom := a.insets(c)
	ui.Scroll(c.Key("charts")).TrackScroll(&a.pageScroll).Grow(1).Padding(top+pageGap, pagePad, bottom+48, pagePad).ScrollbarInsets(top, 0, bottom, 0).Children(func() {
		a.startMark(c)
		defer a.endMark(c)
		a.heading(c, "排行榜", "每日更新")
		official := a.charts[:min(4, len(a.charts))]
		cols := min(4, max(2, a.columns(c, 260)))
		ui.Grid(c).Columns(cols).Gap(20).Children(func() {
			for i := range official {
				a.chartCard(c.Key(official[i].ID), &official[i])
			}
		})
		if len(a.charts) > 4 {
			a.section(c, "全部榜单")
			rest := a.charts[4:]
			ui.Grid(c).Columns(a.columns(c, 150)).GapX(20).GapY(28).Children(func() {
				for i := range rest {
					a.playlistCard(c.Key(rest[i].ID), &rest[i])
				}
			})
		}
	})
}

// playlistCard is a square cover with the playlist's name below: it lifts
// under the pointer, where a play button shows on it.
func (a *app) playlistCard(c *ui.Context, pl *netease.Playlist) {
	p := a.pal
	card := ui.ButtonBase(c).Column().Gap(10).AlignItems(ui.Stretch).FocusRing(false).Label(pl.Name)
	hovered := card.Hovered()
	card.Children(func() {
		art := ui.Box(c).AspectRatio(1).Radius(10)
		lift := ui.Pose{}
		if hovered {
			lift = ui.Pose{Y: -3}
			art.Shadow(0, 12, 28, -6, p.cardShade)
		} else {
			art.Shadow(0, 2, 8, -2, p.cardShade)
		}
		art.AnimatePose(lift, ui.Snappy)
		art.Children(func() {
			a.coverFill(c, pl.Cover(), 400, 10)
			if pl.PlayCount > 0 {
				ui.Row(c).Absolute().Top(8).Right(8).Padding(2, 7).Radius(999).Gap(3).AlignItems(ui.Center).
					Background(ui.RGBA(0, 0, 0, 0.35)).TextColor(ui.RGB(255, 255, 255)).Children(func() {
					ui.Icon(c, iconHeadset).Size(10, 10)
					ui.Text(c, plays(pl.PlayCount)).FontSize(10.5).FontWeight(600)
				})
			}
			a.coverPlayButton(c, pl.ID, hovered)
		})
		ui.Text(c, pl.Name).FontSize(13).FontWeight(500).MaxLines(2).LineHeight(1.35)
		if pl.UpdateFreq != "" {
			ui.Text(c, pl.UpdateFreq).FontSize(12).TextColor(p.muted).SingleLine().Margin(-6, 0, 0, 0)
		}
	})
	if card.Clicked() {
		a.open(route{page: pagePlaylist, playlist: pl.ID})
	}
}

// coverFill fills its parent with a cover, its corners rounded by radius.
// The cover rounds itself rather than its parent clipping it: a clip moved
// by a transform, as a card lifting under the pointer, is drawn on the CPU.
func (a *app) coverFill(c *ui.Context, url string, px int, radius float32) {
	art := a.art.get(netease.Sized(url, px), false)
	if art == nil || art.bmp == nil {
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(radius).Background(a.pal.hover)
		return
	}
	ui.Image(c, art.bmp).Absolute().Top(0).Left(0).Right(0).Bottom(0).Fit(ui.Cover).Radius(radius)
}

// coverPlayButton plays a playlist from its card, without opening it.
func (a *app) coverPlayButton(c *ui.Context, id int64, shown bool) {
	b := ui.ButtonBase(c.Key("play")).Absolute().Bottom(10).Right(10).Size(38, 38).Radius(999).Center().
		Background(a.pal.accent).TextColor(ui.RGB(255, 255, 255)).Label("播放").FocusRing(false).
		Shadow(0, 4, 12, 0, ui.RGBA(0, 0, 0, 0.3))
	pose := ui.Pose{Fade: 1, Scale: 0.8, Y: 6}
	if shown || b.Hovered() {
		pose = ui.Pose{Scale: 1}
		if b.Hovered() {
			pose.Scale = 1.08
		}
	}
	b.AnimatePose(pose, ui.Snappy)
	b.Children(func() { ui.Icon(c, iconPlay).Size(15, 15).Margin(0, 0, 0, 2) })
	if b.Clicked() {
		a.loadPlaylist(id, true)
	}
}

// chartCard shows a chart with its top three songs beside its cover.
func (a *app) chartCard(c *ui.Context, pl *netease.Playlist) {
	p := a.pal
	card := ui.ButtonBase(c).Gap(14).Padding(12).Radius(14).AlignItems(ui.Center).FocusRing(false).Label(pl.Name)
	hovered := card.Hovered()
	bg := p.hover
	if hovered {
		bg = p.press
	}
	card.Background(card.AnimateColor("bg", bg, ui.Animation{Duration: 140 * time.Millisecond}))
	card.Children(func() {
		ui.Box(c).Size(84, 84).Radius(9).Shadow(0, 3, 10, -2, p.cardShade).Children(func() {
			a.coverFill(c, pl.Cover(), 200, 9)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(5).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Baseline).Children(func() {
				ui.Text(c, pl.Name).FontSize(14).FontWeight(700).SingleLine()
				ui.Text(c, pl.UpdateFreq).FontSize(11).TextColor(p.faint).SingleLine()
			})
			for i, line := range pl.Preview {
				ui.Row(c).Gap(7).Children(func() {
					ui.Textf(c, "%d", i+1).FontSize(12).FontWeight(700).Width(10).FontFeatures("tnum").TextColor(p.muted)
					ui.RichText(c,
						ui.Span{Text: line.First, Weight: 500},
						ui.Span{Text: "  " + line.Second, Color: p.muted},
					).FontSize(12).SingleLine().Shrink(1).MinWidth(0)
				})
			}
		})
	})
	if card.Clicked() {
		a.open(route{page: pagePlaylist, playlist: pl.ID})
	}
}

func (a *app) playlistPage(c *ui.Context) {
	id := a.route.playlist
	pl := a.playlists[id]
	if pl == nil {
		a.centered(c, a.loadingPl[id], a.plErr[id], func() { a.loadPlaylist(id, false) })
		return
	}
	a.trackRows(c, pl.Tracks, false, func(c *ui.Context) { a.playlistHeader(c, pl) })
}

func (a *app) playlistHeader(c *ui.Context, pl *netease.Playlist) {
	p := a.pal
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 26, 0).Children(func() {
		ui.Box(c).Size(212, 212).Radius(14).Shadow(0, 18, 40, -10, p.cardShade).Children(func() {
			a.coverFill(c, pl.Cover(), 600, 14)
		})
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			kind := "歌单"
			if pl.UpdateFreq != "" {
				kind = "榜单 · " + pl.UpdateFreq
			}
			ui.Text(c, kind).FontSize(12).FontWeight(600).TextColor(p.accent)
			ui.Text(c, pl.Name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			var meta []string
			if pl.Creator.Nickname != "" {
				meta = append(meta, pl.Creator.Nickname)
			}
			meta = append(meta, fmt.Sprintf("%d 首", len(pl.Tracks)))
			if pl.PlayCount > 0 {
				meta = append(meta, plays(pl.PlayCount)+" 次播放")
			}
			ui.Text(c, strings.Join(meta, " · ")).FontSize(13).TextColor(p.muted)
			if d := strings.TrimSpace(pl.Description); d != "" {
				ui.Text(c, strings.ReplaceAll(d, "\n", " ")).FontSize(12.5).TextColor(p.muted).MaxLines(2).LineHeight(1.45).Margin(2, 0, 0, 0)
			}
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).Children(func() {
				if pillButton(c.Key("play"), iconPlay, "播放", p.accent, p.onAccent).Clicked() {
					if a.pl.Shuffle() {
						a.pl.ToggleShuffle()
					}
					a.pl.PlayQueue(pl.Tracks, firstPlayable(pl.Tracks))
				}
				if pillButton(c.Key("shuffle"), iconShuffle, "随机播放", p.hover.Over(p.bg), p.text).Clicked() {
					if !a.pl.Shuffle() {
						a.pl.ToggleShuffle()
					}
					a.pl.PlayQueue(pl.Tracks, firstPlayable(pl.Tracks))
				}
				if pl.SpecialType == 5 {
					label := "心动模式"
					if a.radioStarting == heartRadio {
						label = "正在开启…"
					}
					if pillButton(c.Key("heart"), iconHeartFill, label, p.hover.Over(p.bg), p.text).Clicked() {
						a.startHeartMode()
					}
				}
			})
		})
	})
}

func (a *app) searchPage(c *ui.Context) {
	if len(a.results) == 0 {
		msg := a.searchErr
		if msg == "" && !a.searching {
			msg = "没有找到相关歌曲"
		}
		top, bottom := a.insets(c)
		ui.Column(c).Grow(1).Padding(top+pageGap, pagePad, bottom, pagePad).Children(func() {
			a.heading(c, "“"+a.searched+"”", "搜索")
			a.centered(c, a.searching, msg, nil)
		})
		return
	}
	a.trackRows(c, a.results, true, func(c *ui.Context) {
		a.heading(c, "“"+a.searched+"”", "搜索")
	})
}

// trackRows lists tracks under a header, which scrolls with them; a click
// plays the list from the track clicked.
func (a *app) trackRows(c *ui.Context, tracks []netease.Track, covers bool, header func(c *ui.Context)) {
	p := a.pal
	cur := a.pl.Current()
	w, _ := c.Size()
	wide := w-sidebarWidth > 820
	top, bottom := a.insets(c)
	// Rows out of view are not built, their marks with them: a list
	// scrolled away from its start or end has content under the bars.
	a.underTop, a.underBottom = true, true
	ui.List(c.Key(a.route), &a.trackList, len(tracks)+1, func(i int) {
		if i == 0 {
			ui.Column(c).Children(func() {
				a.startMark(c)
				header(c)
			})
			return
		}
		if i == len(tracks) {
			defer a.endMark(c)
		}
		i--
		tr := &tracks[i]
		playing := cur != nil && cur.ID == tr.ID
		row := ui.ButtonBase(c.Key(tr.ID)).Height(54).PaddingX(12).Gap(14).Radius(10).AlignItems(ui.Center).FocusRing(false).Label(tr.Name)
		hovered := row.Hovered()
		bg := ui.Transparent
		switch {
		case playing:
			bg = p.accent.Alpha(0.09)
		case hovered:
			bg = p.hover
		}
		row.Background(row.AnimateColor("bg", bg, ui.Animation{Duration: 100 * time.Millisecond}))
		if !tr.Playable() {
			row.Opacity(0.42)
		}
		row.Children(func() {
			ui.Box(c).Width(26).Center().Children(func() {
				switch {
				case playing:
					a.nowPlayingBars(c)
				case hovered && tr.Playable():
					ui.Icon(c, iconPlay).Size(13, 13).TextColor(p.text)
				default:
					ui.Textf(c, "%d", i+1).FontSize(13).FontFeatures("tnum").TextColor(p.faint)
				}
			})
			if covers {
				a.cover(c, tr.Album.PicURL, 96, 38, 6)
			}
			ui.Column(c).Grow(3).Basis(0).MinWidth(0).Gap(3).Children(func() {
				name := p.text
				if playing {
					name = p.accent
				}
				spans := []ui.Span{{Text: tr.Name, Weight: 500, Color: name}}
				if len(tr.Alias) > 0 {
					spans = append(spans, ui.Span{Text: "（" + tr.Alias[0] + "）", Color: p.faint})
				}
				ui.RichText(c, spans...).FontSize(14).SingleLine()
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					if !tr.Playable() {
						ui.Text(c, "VIP").FontSize(9).FontWeight(800).TextColor(p.accent).Padding(0, 4).Radius(3).Border(1, p.accent.Alpha(0.6))
					}
					ui.Text(c, tr.ArtistNames()).FontSize(12.5).TextColor(p.muted).SingleLine().Shrink(1).MinWidth(0)
					if tr.Reason != "" {
						ui.Text(c, tr.Reason).FontSize(10.5).FontWeight(600).TextColor(p.accent).Padding(1, 6).Radius(999).Background(p.accent.Alpha(0.1)).SingleLine()
					}
				})
			})
			if wide {
				ui.Text(c, tr.Album.Name).Grow(2).Basis(0).MinWidth(0).FontSize(12.5).TextColor(p.muted).SingleLine()
			}
			// A liked song's heart shows; the others' as the pointer is on them.
			ui.Box(c).Size(32, 32).Center().Children(func() {
				if a.liked[tr.ID] || hovered {
					a.likeButton(c.Key("like"), tr, 15, p.muted)
				}
			})
			ui.Text(c, clock(tr.Length())).FontSize(12.5).FontFeatures("tnum").TextColor(p.faint).Width(44).TextAlign(ui.End)
		})
		if row.Clicked() && tr.Playable() {
			a.pl.PlayQueue(tracks, i)
		}
	}).Grow(1).Padding(top+pageGap, pagePad-12, bottom+32, pagePad-12).ScrollbarInsets(top, 0, bottom, 0)
}

// nowPlayingBars dance in the row of the track playing.
func (a *app) nowPlayingBars(c *ui.Context) {
	color := a.pal.accent
	ui.Box(c).Size(16, 14).Draw(func(p *ui.Painter, r ui.Rect) {
		lv, live := a.pl.Levels()
		a.rowBars.step(p.Now(), lv, live)
		drawBars(p, r, &a.rowBars, 4, color, 2.5)
		if live || a.rowBars.group(0, player.Bands) > 0.01 {
			p.AnimationFrame()
		}
	})
}
