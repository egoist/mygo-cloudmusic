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
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).Children(func() {
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
			OnRefresh: func() { l.Pages.Retry(); l.Pages.At(0); h.host.Invalidate() },
			Header:    func(c *ui.Context) { h.listHeader(c, l) },
			Menu:      h.trackMenu,
			Liked:     h.state.Liked,
			Favorite:  h.RequestFavorite,
			// Keep the same wide-row album metadata as the archived Feiniu
			// list; the shared renderer only hides it on narrow windows.
			ShowAlbum: l.Spec.Kind != ListAlbum && l.Spec.Kind != ListArtist,
		})
	} else {
		h.host.TrackSkeleton(c, true)
	}
}

// listHeroCover is the cover the list's header shows, which the page waits
// a moment for; a style's drawn art needs no waiting.
func listHeroCover(l *List) string {
	if l.Spec.Kind == ListGenre && l.CoverURL == "" {
		return ""
	}
	return l.Cover()
}
