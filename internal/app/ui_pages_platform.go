package app

import (
	"fmt"
	"time"

	"netease-music-mygo/internal/netease"

	"github.com/egoist/mygo/ui"
)

// Platform UI extension seam.
//
// Keep generic options and callbacks needed to reuse the upstream list
// renderer for provider-owned data here. Provider menus, state and protocol
// behavior belong in the provider adapter, not in the upstream page code.

type trackRowsOptions struct {
	Menu      func(*ui.Menu, *netease.Track, int64)
	Liked     func(int64) bool
	Favorite  func(netease.Track, bool)
	Like      func(*ui.Context, *netease.Track, bool) ui.Element
	ShowAlbum bool
}

// cardWidth keeps platform card pages aligned with the upstream card layout:
// the column count is stable at the current width and each card uses the
// remaining space instead of a fixed width.
func (a *app) cardWidth(c *ui.Context) (int, float32) {
	const gap = 20
	w, _ := c.Size()
	cols := a.columns(c, 168)
	inner := w - sidebarWidth - 2*pagePad
	return cols, (inner - gap*float32(cols-1)) / float32(cols)
}

func (a *app) sharedLikeButton(c *ui.Context, tr *netease.Track, liked bool, color ui.Color, favorite func(netease.Track, bool)) ui.Element {
	icon, label := iconHeart, "喜欢"
	if liked {
		icon, label, color = iconHeartFill, "取消喜欢", a.pal.like
	}
	b := iconButton(c, icon, 15, color, label)
	if b.Clicked() && favorite != nil {
		favorite(*tr, !liked)
	}
	return b
}

// Provider list renderer.
//
// This virtual-list implementation is kept outside ui_pages.go so the upstream
// list renderer remains unchanged. It shares the upstream row primitives while
// accepting lazy data and provider-owned actions through trackRowsOptions.
func (a *app) trackRowsProvider(c *ui.Context, total int, get func(int) *netease.Track, play func(int), covers bool, header func(c *ui.Context), footer ...func(c *ui.Context)) {
	a.trackRowsProviderOptions(c, total, get, play, covers, trackRowsOptions{ShowAlbum: true, Like: func(c *ui.Context, tr *netease.Track, liked bool) ui.Element {
		return a.likeButton(c, tr, 15, a.pal.muted)
	}}, header, footer...)
}

func (a *app) trackRowsProviderOptions(c *ui.Context, total int, get func(int) *netease.Track, play func(int), covers bool, options trackRowsOptions, header func(c *ui.Context), footer ...func(c *ui.Context)) {
	cur := a.pl.Current()
	// A song's menu can take it out of the playlist it is listed in.
	var pid int64
	if a.route.page == pagePlaylist {
		pid = a.route.id
	}
	w, _ := c.Size()
	// An album's page has no column of the album its songs are on.
	wide := w-sidebarWidth > 820 && a.route.page != pageAlbum
	top, bottom := a.insets(c)
	// Rows out of view are not built, their marks with them: a list
	// scrolled away from its start or end has content under the bars.
	a.underTop, a.underBottom = true, true
	rows := total + 1 + len(footer)
	ui.List(c.Key(a.route), &a.trackList, rows, func(i int) {
		if i == 0 {
			ui.Column(c).Children(func() {
				a.startMark(c)
				header(c)
			})
			return
		}
		if i == rows-1 {
			defer a.endMark(c)
		}
		if i > total {
			footer[i-total-1](c)
			return
		}
		i--
		tr := get(i)
		if tr == nil {
			a.pendingTrackRow(c, i, wide, covers, options.ShowAlbum)
			return
		}
		a.providerTrackRow(c, i, tr, cur, pid, wide, covers, options, play)
	}).Grow(1).Padding(top+pageGap, pagePad-12, bottom+32, pagePad-12).ScrollbarInsets(top, 0, bottom, 0)
}

// providerTrackRow is one song of a provider list, as the upstream list
// shows it: its number or the bars while it plays, cover, name and artists,
// album, heart and length.
func (a *app) providerTrackRow(c *ui.Context, i int, tr *netease.Track, cur *netease.Track, pid int64, wide, covers bool, options trackRowsOptions, play func(int)) {
	p := a.pal
	playing := cur != nil && cur.ID == tr.ID
	row := ui.ButtonBase(c.Key(tr.ID)).Height(54).PaddingX(12).Gap(14).Radius(10).AlignItems(ui.Center).FocusRing(false).Label(tr.Name)
	if options.Menu != nil {
		row.ContextMenu(func(m *ui.Menu) { options.Menu(m, tr, pid) })
	} else {
		row.ContextMenu(a.songItems(tr, pid))
	}
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
				a.artistLinks(c, tr, p.muted, p.text).FontSize(12.5).SingleLine().Shrink(1).MinWidth(0)
				if tr.Reason != "" {
					ui.Text(c, tr.Reason).FontSize(10.5).FontWeight(600).TextColor(p.accent).Padding(1, 6).Radius(999).Background(p.accent.Alpha(0.1)).SingleLine()
				}
			})
		})
		if wide && options.ShowAlbum {
			album := ui.RichText(c).Grow(2).Basis(0).MinWidth(0).FontSize(12.5).TextColor(p.muted).SingleLine()
			album.Children(func() {
				if tr.Album.ID == 0 {
					ui.Text(c, tr.Album.Name)
				} else {
					a.link(c.Key("album"), tr.Album.Name, p.muted, p.text, route{page: pageAlbum, id: tr.Album.ID})
				}
			})
		}
		// A liked song's heart shows; the others' as the pointer is on them.
		ui.Box(c).Size(32, 32).Center().Children(func() {
			if options.Like != nil {
				liked := tr.Fav || options.Liked != nil && options.Liked(tr.ID)
				if liked || hovered {
					options.Like(c.Key("like"), tr, liked)
				}
			} else if options.Liked != nil {
				liked := tr.Fav || options.Liked(tr.ID)
				if liked || hovered {
					a.sharedLikeButton(c.Key("platform-like"), tr, liked, p.muted, options.Favorite)
				}
			}
		})
		ui.Text(c, clock(tr.Length())).FontSize(12.5).FontFeatures("tnum").TextColor(p.faint).Width(44).TextAlign(ui.End)
	})
	if row.Clicked() && tr.Playable() {
		play(i)
	}
}

func (a *app) pendingTrackRow(c *ui.Context, i int, wide, covers, showAlbum bool) {
	a.platformTrackRowSkeleton(c.Key(fmt.Sprintf("pending-%d", i)), a.pageShimmer(c), i, wide, covers, showAlbum)
}

// drawnCard is coverCard for a card whose cover is drawn, not loaded: it
// lifts, shades and shows its play button the same.
func (a *app) drawnCard(c *ui.Context, name, sub string, art func(p *ui.Painter, r ui.Rect, radius float32), play func()) bool {
	p := a.pal
	card := ui.ButtonBase(c).Column().Gap(10).AlignItems(ui.Stretch).Justify(ui.Start).FocusRing(false).Label(name)
	hovered := card.Hovered()
	card.Children(func() {
		box := ui.Box(c).AspectRatio(1).Radius(10)
		lift := ui.Pose{}
		if hovered {
			lift = ui.Pose{Y: -3}
			box.Shadow(0, 12, 28, -6, p.cardShade)
		} else {
			box.Shadow(0, 2, 8, -2, p.cardShade)
		}
		box.AnimatePose(lift, ui.Snappy)
		box.Children(func() {
			ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Draw(func(pt *ui.Painter, r ui.Rect) {
				art(pt, r, 10)
			})
			a.coverPlayButton(c, hovered, play)
		})
		ui.Text(c, name).FontSize(13).FontWeight(500).MaxLines(2).LineHeight(1.35)
		if sub != "" {
			ui.Text(c, sub).FontSize(12).TextColor(p.muted).SingleLine().Margin(-6, 0, 0, 0)
		}
	})
	return card.Clicked()
}
