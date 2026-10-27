package app

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// These load a page of a thread's comments, and like a comment or unlike
// it, with NetEase; tests replace them.
var (
	threadComments = netease.ThreadComments
	likeComment    = netease.LikeComment
)

// commentState is the comments of the song or episode Now Playing shows:
// the hot ones, and the latest, loaded a page at a time as they scroll.
type commentState struct {
	track   int64
	thread  string // whose comments they are, as NetEase names it
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
		*cs = commentState{track: tr.ID, thread: tr.Thread()}
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
	thread, offset := cs.thread, len(cs.latest)
	cs.loading, cs.err = true, ""
	go func() {
		page, err := threadComments(thread, offset, commentPage)
		a.post(func() {
			if cs.thread != thread {
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
	if n := a.commentCount(); n > 0 {
		return "评论 " + count(n)
	}
	return "评论"
}

// commentCount is the count the comments' tab shows: the last song's
// while the next one's loads, so that it rolls from one to the other.
func (a *app) commentCount() int {
	switch cs := &a.comments; {
	case cs.loaded:
		return cs.total
	case cs.err != "":
		return 0
	}
	return a.tabCount.n
}

// rollState is a count as it rolls from one value to another.
type rollState struct {
	n    int
	up   bool       // whether it went up, last it changed
	exit *ui.Motion // where the characters shown go as it changes
}

// rollLine is how far a character rolls, about a line of the tab's text.
const rollLine = 16

// rollCount shows n as text, each character in a slot of its own, as
// the wheels of a counter: one that changes rolls out as the next rolls
// in, upward as n grows and downward as it shrinks, the others staying.
func rollCount(c *ui.Context, st *rollState, n int, text string, fg ui.Color) {
	if n != st.n {
		st.up = n > st.n
		dir := float32(1)
		if !st.up {
			dir = -1
		}
		if st.exit != nil {
			// The characters shown leave from this frame on, which first
			// knows the way.
			st.exit.Y = -dir * rollLine
		}
		st.n, st.exit = n, &ui.Motion{}
	}
	dir := float32(1)
	if !st.up {
		dir = -1
	}
	chars := []rune(text)
	// Clipped to its line, a character leaves out of sight.
	ui.Row(c).ClipY().Children(func() {
		for i, ch := range chars {
			// Slots go by their place from the right, as the wheels of a
			// counter do: the units are always the last.
			k := len(chars) - 1 - i
			roll := ui.Animation{Spring: ui.SpringOptions{Duration: 420 * time.Millisecond, Bounce: 0.15}, Delay: time.Duration(k) * 30 * time.Millisecond}
			slot := ui.Box(c.Key(k)).Transition(ui.ElementTransition{
				Animation: &roll, Size: true,
				Enter: &ui.Motion{Y: dir * rollLine, Collapse: true},
				Exit:  &ui.Motion{Collapse: true},
			})
			slot.Children(func() {
				ui.Text(c.Key(ch), string(ch)).FontSize(13).FontWeight(600).FontFeatures("tnum").TextColor(fg).
					Transition(ui.ElementTransition{Animation: &roll, Colors: true, Enter: &ui.Motion{Y: dir * rollLine}, Exit: st.exit})
			})
		}
	})
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
		if cs.loading || !cs.loaded && cs.err == "" {
			ui.Column(c).Grow(1).Padding(4, 8, 0, 0).Gap(2).Children(func() {
				s := npShimmer(c)
				s.text(c, 13, 1.3).Width(56).Margin(18, 12, 6, 12)
				commentSkeleton(c, s, 6)
			})
			return
		}
		ui.Column(c).Grow(1).Center().Gap(14).Children(func() {
			switch {
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
			if cs.loading {
				commentSkeleton(c.Key("more"), npShimmer(c), 2)
			}
			ui.Row(c.Key("foot")).Height(56).Center().Children(func() {
				switch {
				case cs.loading:
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
// when, from where, and how many liked it, with a thumb to like it too.
func (a *app) commentRow(c *ui.Context, cm *netease.Comment, now time.Time) {
	ui.Row(c).Padding(12).Gap(12).Radius(12).AlignItems(ui.Start).Children(func() {
		a.avatar(c, cm.User.AvatarURL, 34, white.Alpha(0.1))
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(6).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, cm.User.Nickname).FontSize(13).FontWeight(600).TextColor(white.Alpha(0.6)).SingleLine().Shrink(1).MinWidth(0)
				ui.Spacer(c)
				a.commentLike(c.Key("like"), cm)
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

// commentLike is a comment's thumb, with how many liked it, which likes it
// or unlikes it: in red once liked.
func (a *app) commentLike(c *ui.Context, cm *netease.Comment) {
	label, fg := "赞", white.Alpha(0.42)
	if cm.Liked {
		label, fg = "取消赞", a.pal.accent
	}
	b := ui.ButtonBase(c).Gap(4).Padding(3, 6).Margin(-3, -6).Radius(999).AlignItems(ui.Center).FocusRing(false).Label(label)
	if b.Hovered() && !cm.Liked {
		fg = white.Alpha(0.75)
	}
	b.TextColor(b.AnimateColor("fg", fg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Children(func() {
		icon := iconThumb
		if cm.Liked {
			icon = iconThumbFill
		}
		ui.Icon(c, icon).Size(13, 13)
		if cm.LikedCount > 0 {
			ui.Text(c, count(cm.LikedCount)).FontSize(12).FontFeatures("tnum")
		}
	})
	if b.Clicked() {
		a.toggleCommentLike(cm.ID)
	}
}

// toggleCommentLike likes a comment, or unlikes it, at once in the app and
// then with NetEase, which may refuse: it goes back then. Signed out, it
// asks the listener to sign in.
func (a *app) toggleCommentLike(id int64) {
	if a.profile == nil {
		a.openLogin()
		return
	}
	cs := &a.comments
	thread, like := cs.thread, true
	if cm := cs.find(id); cm != nil {
		like = !cm.Liked
	}
	cs.setLiked(id, like)
	go func() {
		err := likeComment(thread, id, like)
		a.post(func() {
			if err != nil && cs.thread == thread {
				cs.setLiked(id, !like)
				a.toast = "操作失败，请稍后再试"
			}
		})
	}()
}

// find returns the comment id, hot or latest, or nil.
func (cs *commentState) find(id int64) *netease.Comment {
	for _, list := range [][]netease.Comment{cs.hot, cs.latest} {
		for i := range list {
			if list[i].ID == id {
				return &list[i]
			}
		}
	}
	return nil
}

// setLiked marks the comment id liked, or not, counting the listener in or
// out, both where it is hot and where it is latest.
func (cs *commentState) setLiked(id int64, like bool) {
	for _, list := range [][]netease.Comment{cs.hot, cs.latest} {
		for i := range list {
			if cm := &list[i]; cm.ID == id && cm.Liked != like {
				cm.Liked = like
				if like {
					cm.LikedCount++
				} else {
					cm.LikedCount = max(cm.LikedCount-1, 0)
				}
			}
		}
	}
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
