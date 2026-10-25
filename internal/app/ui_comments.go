package app

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// commentState is the comments of the track Now Playing shows: the hot
// ones, and the latest, loaded a page at a time as they scroll.
type commentState struct {
	track   int64
	hot     []netease.Comment
	latest  []netease.Comment
	total   int
	more    bool
	loaded  bool
	loading bool
	err     string
	scroll  ui.ScrollState
}

// commentPage is how many comments a page loads.
const commentPage = 30

// commentsFor makes the comments those of tr, loading their first page
// once it changed: Now Playing does it as it shows, for the count on the
// pane's tab.
func (a *app) commentsFor(tr *netease.Track) *commentState {
	cs := &a.comments
	if cs.track != tr.ID {
		*cs = commentState{track: tr.ID}
		a.loadComments()
	}
	return cs
}

// loadComments loads the next page of comments.
func (a *app) loadComments() {
	cs := &a.comments
	if cs.loading {
		return
	}
	id, offset := cs.track, len(cs.latest)
	cs.loading, cs.err = true, ""
	go func() {
		page, err := netease.SongComments(id, offset, commentPage)
		a.post(func() {
			if cs.track != id {
				return
			}
			cs.loading = false
			if err != nil {
				cs.err = "评论加载失败"
				return
			}
			cs.loaded = true
			if offset == 0 {
				cs.hot = page.Hot
			}
			// New comments shift the pages: those already shown come again.
			seen := make(map[int64]bool, len(cs.latest))
			for _, cm := range cs.latest {
				seen[cm.ID] = true
			}
			for _, cm := range page.Latest {
				if !seen[cm.ID] {
					cs.latest = append(cs.latest, cm)
				}
			}
			cs.total, cs.more = page.Total, page.More && len(page.Latest) > 0
		})
	}()
}

// commentsTab names the comments' pane, with how many there are.
func (a *app) commentsTab() string {
	if cs := &a.comments; cs.loaded && cs.total > 0 {
		return "评论 " + count(cs.total)
	}
	return "评论"
}

// count formats a count as people in China read it, to a tenth of a 万.
func count(n int) string {
	if n >= 1e4 && n < 1e6 {
		return fmt.Sprintf("%.1f万", float64(n)/1e4)
	}
	return plays(float64(n))
}

type commentKey struct {
	hot bool
	id  int64
}

// commentsPane lists the comments of tr: the hot ones, then the latest,
// which load on as they scroll into view.
func (a *app) commentsPane(c *ui.Context, tr *netease.Track) {
	cs := a.commentsFor(tr)
	if len(cs.hot) == 0 && len(cs.latest) == 0 {
		ui.Column(c).Grow(1).Center().Gap(14).Children(func() {
			switch {
			case cs.loading || !cs.loaded && cs.err == "":
				ui.Spinner(c)
			case cs.err != "":
				ui.Text(c, cs.err).FontSize(15).TextColor(white.Alpha(0.5))
				if pillButton(c.Key("retry"), iconComment, "重试", white.Alpha(0.14), white).Clicked() {
					a.loadComments()
				}
			default:
				ui.Icon(c, iconComment).Size(40, 40).TextColor(white.Alpha(0.3))
				ui.Text(c, "还没有评论，快来抢沙发").FontSize(17).FontWeight(600).TextColor(white.Alpha(0.45))
			}
		})
		return
	}
	now := c.Now()
	scroll := ui.Scroll(c.Key("comments")).Grow(1).TrackScroll(&cs.scroll).Padding(4, 8, 48, 0)
	var end ui.Rect
	scroll.Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			if len(cs.hot) > 0 {
				commentHeading(c, "精彩评论")
				for i := range cs.hot {
					a.commentRow(c.Key(commentKey{true, cs.hot[i].ID}), &cs.hot[i], now)
				}
			}
			commentHeading(c, "最新评论 "+count(cs.total))
			for i := range cs.latest {
				a.commentRow(c.Key(commentKey{false, cs.latest[i].ID}), &cs.latest[i], now)
			}
			ui.Row(c.Key("foot")).Height(56).Center().Children(func() {
				switch {
				case cs.loading:
					ui.Spinner(c)
				case cs.err != "":
					retry := ui.ButtonBase(c.Key("retry")).Padding(6, 14).Radius(999).Background(white.Alpha(0.1)).Label("重试")
					retry.Children(func() {
						ui.Text(c, "加载失败，点按重试").FontSize(12.5).TextColor(white.Alpha(0.7))
					})
					if retry.Clicked() {
						a.loadComments()
					}
				case !cs.more:
					ui.Text(c, "没有更多评论了").FontSize(12).TextColor(white.Alpha(0.35))
				}
			})
			end = ui.Box(c.Key("end")).Height(0).Bounds()
		})
	})
	// The next page, once the foot is within a screen of coming into view.
	if box := scroll.Bounds(); cs.more && !cs.loading && cs.err == "" && end.W > 0 && end.Y < box.Y+box.H*2 {
		a.loadComments()
	}
}

func commentHeading(c *ui.Context, title string) {
	ui.Text(c, title).FontSize(13).FontWeight(700).TextColor(white.Alpha(0.55)).LetterSpacing(0.2).Padding(18, 12, 6, 12)
}

// commentRow is a comment: who wrote it, what, and what it replies to,
// when, from where, and how many liked it.
func (a *app) commentRow(c *ui.Context, cm *netease.Comment, now time.Time) {
	ui.Row(c).Padding(12).Gap(12).Radius(12).AlignItems(ui.Start).Children(func() {
		a.avatar(c, cm.User.AvatarURL, 34, white.Alpha(0.1))
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(6).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, cm.User.Nickname).FontSize(13).FontWeight(600).TextColor(white.Alpha(0.6)).SingleLine().Shrink(1).MinWidth(0)
				ui.Spacer(c)
				if cm.LikedCount > 0 {
					ui.Row(c).Gap(4).AlignItems(ui.Center).TextColor(white.Alpha(0.42)).Children(func() {
						ui.Icon(c, iconThumb).Size(13, 13)
						ui.Text(c, count(cm.LikedCount)).FontSize(12).FontFeatures("tnum")
					})
				}
			})
			ui.Text(c, cm.Content).FontSize(15).LineHeight(1.55).TextColor(white.Alpha(0.92))
			if len(cm.BeReplied) > 0 {
				r := cm.BeReplied[0]
				ui.Box(c).Padding(8, 12).Radius(10).Background(white.Alpha(0.07)).Children(func() {
					ui.RichText(c,
						ui.Span{Text: "@" + r.User.Nickname + "：", Color: white.Alpha(0.75), Weight: 600},
						ui.Span{Text: r.Content, Color: white.Alpha(0.6)},
					).FontSize(13).LineHeight(1.5).MaxLines(3)
				})
			}
			meta := commentTime(time.UnixMilli(cm.Time), now)
			if loc := cm.IPLocation.Location; loc != "" {
				meta += " · " + loc
			}
			ui.Text(c, meta).FontSize(11.5).TextColor(white.Alpha(0.38))
		})
	})
}

// avatar shows a listener's picture, round, over empty while it loads.
func (a *app) avatar(c *ui.Context, url string, size float32, empty ui.Color) {
	bmp := bitmap(a.art.get(netease.Sized(url, int(size*3)), false))
	box := ui.Box(c).Size(size, size).Radius(size / 2).Shrink(0)
	in := artIn(box, bmp)
	if in < 1 {
		box.Background(empty)
	}
	box.Children(func() { artImage(c, bmp, size/2, in) })
}

// commentTime says when a comment was written, as NetEase does: in
// minutes or hours while recent, then by the day.
func commentTime(t, now time.Time) string {
	d := now.Sub(t)
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d分钟前", int(d/time.Minute))
	case y1 == y2 && m1 == m2 && d1 == d2:
		return t.Format("15:04")
	case now.AddDate(0, 0, -1).Format("20060102") == t.Format("20060102"):
		return "昨天 " + t.Format("15:04")
	case y1 == y2:
		return fmt.Sprintf("%d月%d日", m1, d1)
	default:
		return fmt.Sprintf("%d年%d月%d日", y1, m1, d1)
	}
}
