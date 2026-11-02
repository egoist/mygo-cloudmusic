package fnmusic

import (
	"fmt"
	backend "netease-music-mygo/internal/platform/fnmusic"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (h *Home) listHeader(c *ui.Context, l *List) {
	t := c.Theme()
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 26, 0).Children(func() {
		if l.Spec.Kind == ListGenre && l.CoverURL == "" {
			// A style has no cover; its own art stands for it, not its
			// first song's.
			genreArt(c, l.Name, 212, 14).Shadow(0, 18, 40, -10, ui.RGBA(0, 0, 0, 0.24))
		} else {
			h.host.Cover(c, l.Cover(), 212, 14).Shadow(0, 18, 40, -10, ui.RGBA(0, 0, 0, 0.24))
		}
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			if l.HeaderKind != "" && l.HeaderKind != l.Name {
				ui.Text(c, l.HeaderKind).FontSize(12).FontWeight(600).TextColor(t.Accent)
			}
			ui.Text(c, l.Name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			meta := []string{"飞牛音乐"}
			if l.Sub != "" {
				meta = append(meta, l.Sub)
			}
			if l.Pages.Total() >= 0 {
				meta = append(meta, fmt.Sprintf("%d 首", l.Pages.Total()))
			}
			if len(meta) > 0 {
				ui.Text(c, strings.Join(meta, " · ")).FontSize(13).TextColor(t.TextMuted)
			}
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				if h.host.PillButton(c, "fnmusic-list-play", iconPlay, "播放", true).Clicked() {
					PlayList(h.host.Player(), l, 0)
				}
				shuffleLabel := "随机播放"
				if l.Shuffle {
					shuffleLabel = "正在准备…"
				}
				if h.host.PillButton(c, "fnmusic-list-shuffle", iconShuffle, shuffleLabel, false).Clicked() {
					ShuffleList(h.host.Player(), l, h.host.Post)
				}
				h.host.MoreButton(c, "fnmusic-list-more", func(m *ui.Menu) { h.listMenu(m, l) })
				ui.Box(c).Grow(1)
				if control, ok := h.listSort(l); ok {
					control.button(c, "fnmusic-list-sort")
				}
			})
		})
	})
}

// artistListHeader keeps the artist-view switch in the same row on both the
// song and album pages. The song actions sit below it.
func (h *Home) artistListHeader(c *ui.Context, l *List) {
	t := c.Theme()
	ui.Row(c).Gap(28).AlignItems(ui.Start).Padding(0, 12, 26, 12).Children(func() {
		h.host.Cover(c, l.Cover(), 212, 14).Shadow(0, 18, 40, -10, ui.RGBA(0, 0, 0, 0.24))
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			if l.HeaderKind != "" && l.HeaderKind != l.Name {
				ui.Text(c, l.HeaderKind).FontSize(12).FontWeight(600).TextColor(t.Accent)
			}
			ui.Text(c, l.Name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			meta := []string{"飞牛音乐"}
			if l.Sub != "" {
				meta = append(meta, l.Sub)
			}
			if l.Pages.Total() >= 0 {
				meta = append(meta, fmt.Sprintf("%d 首", l.Pages.Total()))
			}
			ui.Text(c, strings.Join(meta, " · ")).FontSize(13).TextColor(t.TextMuted)
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				h.artistTabs(c, l.ID, l.Spec.Ref, 0)
				ui.Box(c).Grow(1)
				if control, ok := h.listSort(l); ok {
					control.button(c, "fnmusic-artist-track-sort")
				}
			})
			ui.Row(c).Gap(10).Margin(2, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				if h.host.PillButton(c, "fnmusic-list-play", iconPlay, "播放", true).Clicked() {
					PlayList(h.host.Player(), l, 0)
				}
				shuffleLabel := "随机播放"
				if l.Shuffle {
					shuffleLabel = "正在准备…"
				}
				if h.host.PillButton(c, "fnmusic-list-shuffle", iconShuffle, shuffleLabel, false).Clicked() {
					ShuffleList(h.host.Player(), l, h.host.Post)
				}
				h.host.MoreButton(c, "fnmusic-list-more", func(m *ui.Menu) { h.listMenu(m, l) })
			})
		})
	})
}

func (h *Home) artistListHeaderSkeleton(c *ui.Context, l *List) {
	ui.Row(c).Gap(28).AlignItems(ui.Start).Padding(0, 12, 26, 12).Children(func() {
		h.host.Bone(c, 14).Size(212, 212)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			h.host.TextBone(c, 12, 1.3).Width(36)
			h.host.TextBone(c, 30, 1.2).WidthPercent(48)
			h.host.TextBone(c, 13, 1.3).Width(180)
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				h.host.Bone(c, 12).Height(24).Width(116)
				ui.Box(c).Grow(1)
				if _, ok := h.listSort(l); ok {
					h.host.Bone(c, 15).Height(30).Width(140)
				}
			})
			ui.Row(c).Gap(10).Margin(2, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				h.host.Bone(c, 18).Size(92, 36)
				h.host.Bone(c, 18).Size(120, 36)
				h.host.Bone(c, 17).Size(34, 34)
			})
		})
	})
}

func (h *Home) listPage(c *ui.Context) {
	l := h.state.Catalog().ListForRoute(h.route.ID, h.route.Kind, h.route.Query)
	if h.route.Page == PageLibrary {
		l = h.state.Catalog().ListFor(backend.LibraryID)
	}
	if l != nil && l.Pages != nil {
		h.host.RenderTracks(c, l.View(), TrackView{
			Title: l.Name, Sub: l.Kind, Cover: listHeroCover(l), Total: l.Pages.Total(),
			OnPlay:    func(i int) { PlayList(h.host.Player(), l, i) },
			OnShuffle: func() { ShuffleList(h.host.Player(), l, h.host.Post) },
			OnRefresh: func() { l.Refresh(); h.host.Invalidate() },
			Header: func(c *ui.Context) {
				if l.Spec.Kind == ListArtist {
					h.artistListHeader(c, l)
				} else {
					h.listHeader(c, l)
				}
			},
			HeaderSkeleton: func(c *ui.Context) {
				if l.Spec.Kind == ListArtist {
					h.artistListHeaderSkeleton(c, l)
				} else {
					h.listHeaderSkeleton(c, l)
				}
			},
			Menu:     h.trackMenu,
			Liked:    h.state.Liked,
			Favorite: h.RequestFavorite,
			// Keep the same wide-row album metadata as the archived Feiniu
			// list; the shared renderer only hides it on narrow windows.
			ShowAlbum: l.Spec.Kind != ListAlbum && l.Spec.Kind != ListArtist,
		})
	} else {
		fallback := h.routeListSkeleton()
		h.host.TrackSkeleton(c, TrackView{
			ShowAlbum: fallback.Spec.Kind != ListAlbum && fallback.Spec.Kind != ListArtist,
			HeaderSkeleton: func(c *ui.Context) {
				if fallback.Spec.Kind == ListArtist {
					h.artistListHeaderSkeleton(c, fallback)
				} else {
					h.listHeaderSkeleton(c, fallback)
				}
			},
		})
	}
}

func (h *Home) routeListSkeleton() *List {
	kind := ListPlaylist
	switch h.route.Page {
	case PageLibrary:
		kind = ListLibrary
	case PagePlaylist:
		switch {
		case h.route.ID == backend.FavoritesID:
			kind = ListFavorites
		case h.route.ID == backend.RecentID:
			kind = ListRecent
		case h.route.Kind == backend.KindAlbum:
			kind = ListAlbum
		case h.route.Kind == backend.KindArtist:
			kind = ListArtist
		case h.route.Kind == backend.KindGenre:
			kind = ListGenre
		}
	}
	return &List{Spec: Spec{Kind: kind, Ref: h.route.Query}}
}

// listHeaderSkeleton reserves the whole list header without exposing the
// labels or actions already known to the page.
func (h *Home) listHeaderSkeleton(c *ui.Context, l *List) {
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 0, 26, 0).Children(func() {
		h.host.Bone(c, 14).Size(212, 212)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			if l == nil || l.HeaderKind != "" && l.HeaderKind != l.Name {
				h.host.TextBone(c, 12, 1.3).Width(36)
			}
			h.host.TextBone(c, 30, 1.2).WidthPercent(48)
			h.host.TextBone(c, 13, 1.3).Width(180)
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				h.host.Bone(c, 18).Size(92, 36)
				h.host.Bone(c, 18).Size(120, 36)
				h.host.Bone(c, 17).Size(34, 34)
				ui.Box(c).Grow(1)
				if _, ok := h.listSort(l); ok {
					h.host.Bone(c, 15).Height(30).Width(126)
				}
			})
		})
	})
}

// listHeroCover is the cover the list's header shows, which the page waits
// a moment for; a style's drawn art needs no waiting.
func listHeroCover(l *List) string {
	if l.Spec.Kind == ListGenre && l.CoverURL == "" {
		return ""
	}
	return l.Cover()
}
