package app

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// dailyArt is the cover of the day's songs: their first cover, blurred,
// with the date set large over it.
func (a *app) dailyArt(c *ui.Context, now time.Time, size float32) {
	var art *artwork
	if len(a.daily) > 0 {
		art = a.art.get(netease.Sized(a.daily[0].Album.PicURL, 600), true)
	}
	radius := size * 0.07
	box := ui.Box(c).Size(size, size).Radius(radius).Background(ui.Hex("#3a3a40")).
		Shadow(0, 18, 40, -10, a.pal.cardShade)
	in := artIn(box, backdrop(art))
	box.Children(func() {
		artImage(c, backdrop(art), radius, in)
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(radius).
			LinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
				{Color: ui.RGBA(0, 0, 0, 0), At: 0.35},
				{Color: ui.RGBA(0, 0, 0, 0.4), At: 1},
			}})
		ui.Column(c).Absolute().Left(size * 0.09).Bottom(size * 0.08).TextColor(white).Children(func() {
			ui.Textf(c, "%d", now.Day()).FontSize(size * 0.44).FontWeight(300).LetterSpacing(-size * 0.02).FontFeatures("tnum").LineHeight(1)
			ui.Textf(c, "%d月 · %s", now.Month(), weekday(now)).FontSize(max(11, size*0.075)).FontWeight(600).TextColor(white.Alpha(0.85))
		})
	})
}

// dailyCard leads the home page: the day's songs, over their first cover
// blurred, with the next few covers fanned out on the right.
func (a *app) dailyCard(c *ui.Context, now time.Time) {
	songs := a.daily
	if len(songs) == 0 {
		return
	}
	art := a.art.get(netease.Sized(songs[0].Album.PicURL, 600), true)
	card := ui.ButtonBase(c.Key("daily")).Height(196).Radius(20).FocusRing(false).Label("每日推荐").
		Background(ui.Hex("#2a2a30")).TextColor(white)
	hovered := card.Hovered()
	lift := ui.Pose{}
	if hovered {
		lift.Scale = 1.006
	}
	card.AnimatePose(lift, ui.Snappy)
	card.Shadow(0, 14, 34, -14, a.pal.cardShade)
	in := artIn(card, backdrop(art))
	card.Children(func() {
		artImage(c, backdrop(art), 20, in)
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(20).
			LinearGradient(ui.LinearGradient{Angle: 90, Stops: []ui.GradientStop{
				{Color: ui.RGBA(0, 0, 0, 0.45), At: 0},
				{Color: ui.RGBA(0, 0, 0, 0.12), At: 1},
			}})
		// Over the backdrop, which is positioned too: positioned elements
		// paint over those in the flow.
		ui.Row(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Padding(28, 36).Gap(24).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(6).Children(func() {
				ui.Textf(c, "%d月%d日 · %s", now.Month(), now.Day(), weekday(now)).FontSize(12).FontWeight(600).TextColor(white.Alpha(0.7))
				ui.Text(c, "每日推荐").FontSize(30).FontWeight(800).LetterSpacing(-0.6)
				ui.Text(c, a.dailyBlurb()).FontSize(13).TextColor(white.Alpha(0.75))
				ui.Row(c).Gap(10).Margin(12, 0, 0, 0).Children(func() {
					play := pillButton(c.Key("play"), iconPlay, "播放", ui.RGB(255, 255, 255), ui.Hex("#1d1d1f"))
					if play.Clicked() {
						a.pl.PlayQueue(songs, firstPlayable(songs))
					}
					if a.profile == nil && pillButton(c.Key("login"), iconUser, "登录", white.Alpha(0.18), white).Clicked() {
						a.openLogin()
					}
				})
			})
			// The next covers, fanned.
			w, _ := c.Size()
			if w-sidebarWidth < 760 {
				return
			}
			ui.Row(c).Children(func() {
				for i := range min(4, len(songs)) {
					cov := ui.Box(c.Key(i)).Size(108, 108).Radius(12).Margin(0, 0, 0, map0(i, -46)).
						Shadow(-6, 10, 24, -6, ui.RGBA(0, 0, 0, 0.45))
					fan := ui.Pose{Rotate: float32(i-1) * 4, Y: float32(i%2) * 6}
					if hovered {
						fan = ui.Pose{Rotate: float32(i-1) * 7, Y: float32(i%2)*6 - 4, X: float32(i-2) * 6}
					}
					cov.AnimatePose(fan, ui.Animation{Spring: ui.SpringOptions{Duration: 500 * time.Millisecond, Bounce: 0.3}})
					cov.Children(func() { a.coverFill(c, songs[i].Album.PicURL, 240, 12) })
				}
			})
		})
	})
	if card.Clicked() {
		a.open(route{page: pageDaily})
	}
}

// dailyBlurb says whose taste the day's songs follow.
func (a *app) dailyBlurb() string {
	if a.profile == nil {
		return "大家都在听的好歌，登录后为你专属生成"
	}
	return "根据你的音乐口味生成，每天 6:00 更新"
}

// map0 returns v for every item but the first.
func map0(i int, v float32) float32 {
	if i == 0 {
		return 0
	}
	return v
}

func (a *app) dailyPage(c *ui.Context) {
	if len(a.daily) == 0 {
		a.centered(c, a.homeLoading, "暂时没有推荐", a.loadHome)
		return
	}
	now := c.Now()
	songs := a.daily
	a.trackRows(c, songs, true, func(c *ui.Context) {
		p := a.pal
		ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 26, 0).Children(func() {
			a.dailyArt(c, now, 212)
			ui.Column(c).Gap(8).Children(func() {
				ui.Text(c, "每日推荐").FontSize(12).FontWeight(600).TextColor(p.accent)
				ui.Text(c, "每日歌曲推荐").FontSize(30).FontWeight(800).LetterSpacing(-0.6)
				ui.Text(c, fmt.Sprintf("%d 首 · %s", len(songs), a.dailyBlurb())).FontSize(13).TextColor(p.muted)
				ui.Row(c).Gap(10).Margin(8, 0, 0, 0).Children(func() {
					if pillButton(c.Key("play"), iconPlay, "播放全部", p.accent, p.onAccent).Clicked() {
						if a.pl.Shuffle() {
							a.pl.ToggleShuffle()
						}
						a.pl.PlayQueue(songs, firstPlayable(songs))
					}
					if pillButton(c.Key("shuffle"), iconShuffle, "随机播放", p.hover.Over(p.bg), p.text).Clicked() {
						if !a.pl.Shuffle() {
							a.pl.ToggleShuffle()
						}
						a.pl.PlayQueue(songs, firstPlayable(songs))
					}
				})
			})
		})
	})
}
