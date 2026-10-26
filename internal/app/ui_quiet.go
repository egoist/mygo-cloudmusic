package app

import (
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// Through the quiet parts of a song, its intro, an interlude or its outro,
// the lyrics dim, and one of its hot comments (热评) rises in where they
// were, down the lyrics pane or in the middle of the stage, with who wrote
// it and how many liked it, giving way as the singing comes back. A long quiet shows a few, one after another; a song without
// lyrics shows them all along. A click on one opens the comments. The
// listener turns them on (settings.HotComments).

const (
	quietLeast = 8 * time.Second         // the shortest quiet a comment shows in
	quietLead  = 1200 * time.Millisecond // into the quiet before it shows
	quietTail  = 1000 * time.Millisecond // before the singing, it is gone
	quietSlot  = 10 * time.Second        // about how long each shows in a long quiet
	quietRunes = 140                     // the longest comment shown
)

// quietState is the quiet parts of the song Now Playing shows, and the
// comment shown in one, kept as it fades away.
type quietState struct {
	track  int64
	lines  int // how many lines they were found among
	quiets []lyrics.Quiet
	cm     *netease.Comment
}

// quietComment returns the hot comment to show at pos, nil for none, and
// how long until that changes, 0 for never.
func (a *app) quietComment(tr *netease.Track, lines []lyrics.Line, loaded bool, pos time.Duration) (*netease.Comment, time.Duration) {
	// Only NetEase's songs have comments.
	if !a.settings.HotComments || tr.Source != "" {
		return nil, 0
	}
	qs := &a.quiet
	if qs.track != tr.ID || qs.lines != len(lines) || qs.quiets == nil {
		qs.track, qs.lines = tr.ID, len(lines)
		qs.quiets = lyrics.Quiets(lines, tr.Length(), quietLeast)
		if len(lines) == 0 && loaded && tr.Length() >= quietLeast {
			qs.quiets = []lyrics.Quiet{{From: 0, To: tr.Length()}}
		}
		if qs.quiets == nil {
			qs.quiets = []lyrics.Quiet{}
		}
	}
	var hot []*netease.Comment
	if cs := &a.comments; cs.track == tr.ID {
		for i := range cs.hot {
			if n := utf8.RuneCountInString(cs.hot[i].Content); n > 0 && n <= quietRunes {
				hot = append(hot, &cs.hot[i])
			}
		}
	}
	if len(hot) == 0 {
		return nil, 0
	}
	shown := 0 // the comments the quiets before showed
	for _, q := range qs.quiets {
		from, to := q.From+quietLead, q.To-quietTail
		if pos < from {
			return nil, from - pos
		}
		// As many as fit about a slot each, sharing the quiet evenly.
		slots := max(1, int((to-from+quietSlot/2)/quietSlot))
		each := (to - from) / time.Duration(slots)
		if pos < to {
			slot := min(int((pos-from)/each), slots-1)
			next := to
			if slot+1 < slots {
				next = from + time.Duration(slot+1)*each
			}
			return hot[(shown+slot)%len(hot)], next - pos
		}
		shown += slots
	}
	return nil, 0
}

// quietShown returns how far a hot comment has come in on el, the
// lyrics, from 0 to 1, keeping it to show as it goes, and whether one
// shows now.
func (a *app) quietShown(c *ui.Context, el ui.Element, tr *netease.Track, lines []lyrics.Line, loaded bool) (float32, bool) {
	pl := a.pl
	cm, next := a.quietComment(tr, lines, loaded, pl.Position())
	if pl.Playing() && next > 0 {
		c.After(next + 20*time.Millisecond)
	}
	to := float32(0)
	if cm != nil {
		to = 1
	}
	dim := el.AnimateTo("quiet", to, ui.Animation{Duration: 600 * time.Millisecond})
	if cm != nil {
		a.quiet.cm = cm
	}
	return dim, cm != nil
}

// quietFaded is how much of the lyrics shows under a hot comment.
const quietFaded = 0.06

// quietUnder fades e, the lyrics, under a hot comment as far in as dim,
// and hides it once the comment is all in.
func quietUnder(e ui.Element, dim float32) ui.Element {
	if dim > 0.98 {
		e.Invisible()
	} else if dim > 0.01 {
		e.Opacity(1 - (1-quietFaded)*dim)
	}
	return e
}

// quietCard shows the hot comment kept, top down the pane, as far in as
// dim.
func (a *app) quietCard(c *ui.Context, dim, top float32) {
	a.hotCard(c, dim, 22, func(card ui.Element) { card.Absolute().Top(top).Left(0).Right(24) })
}

// stageQuiet shows the hot comment kept in the middle of the stage, w
// wide, its lines sung at f, as far in as dim.
func (a *app) stageQuiet(c *ui.Context, dim, f, w float32) {
	if dim <= 0.01 || a.quiet.cm == nil {
		return
	}
	ui.Column(c.Key("quiet-stage")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Center().PassThrough().Children(func() {
		a.hotCard(c, dim, min(34, max(24, f*0.55)), func(card ui.Element) { card.Width(min(760, w*0.6)) })
	})
}

// hotCard shows the hot comment kept, its words at size, as far in as dim;
// place puts it where it goes.
func (a *app) hotCard(c *ui.Context, dim, size float32, place func(ui.Element)) {
	cm := a.quiet.cm
	if dim <= 0.01 || cm == nil {
		return
	}
	card := ui.ButtonBase(c.Key("quiet")).Column().AlignItems(ui.Start).
		Padding(16, 12).Gap(14).Radius(14).FocusRing(false).Label("查看评论")
	place(card)
	// Each comment rises in on its own as it takes the last one's place.
	seen := ui.Local(card, "seen", func() int64 { return 0 })
	if *seen != cm.ID {
		*seen = cm.ID
		card.Jump("in", 0)
	}
	in := card.AnimateTo("in", 1, ui.Animation{Spring: ui.SpringOptions{Duration: 700 * time.Millisecond, Bounce: 0.1}})
	card.Opacity(min(dim, max(0, in))).Translate(0, (1-in)*16)
	if card.Hovered() {
		card.Background(white.Alpha(0.06))
	}
	card.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).TextColor(white.Alpha(0.5)).Children(func() {
			ui.Icon(c, iconComment).Size(14, 14)
			ui.Text(c, "热评").FontSize(12.5).FontWeight(700).LetterSpacing(0.6)
		})
		ui.Text(c, cm.Content).FontSize(size).FontWeight(600).LineHeight(1.5).TextColor(white.Alpha(0.92)).MaxLines(6)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			a.avatar(c, cm.User.AvatarURL, 22, white.Alpha(0.1))
			ui.Text(c, cm.User.Nickname).FontSize(13).TextColor(white.Alpha(0.6)).SingleLine().Shrink(1).MinWidth(0)
			if cm.LikedCount > 0 {
				ui.Row(c).Gap(4).AlignItems(ui.Center).Margin(0, 0, 0, 6).TextColor(white.Alpha(0.45)).Children(func() {
					ui.Icon(c, iconThumb).Size(13, 13)
					ui.Text(c, count(cm.LikedCount)).FontSize(12).FontFeatures("tnum")
				})
			}
		})
	})
	if card.Clicked() {
		a.npPane = paneComments
	}
}
