package fnmusic

// Feiniu style pages. The styles are cards, as the albums are, and a style
// opens as a song list like an album's; both go through the shared card and
// list renderers, so only the sorts and the menu are the style's own.

import (
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

func (h *Home) genrePage(c *ui.Context) {
	catalog := h.state.Catalog()
	cards := catalog.GenresList()
	if cards == nil {
		h.listPage(c)
		return
	}
	control := h.genreSort()
	h.host.RenderCards(c, cards, CardView{
		Title: "风格", Count: cards.Total(),
		Header:         func(c *ui.Context) { h.cardsHeaderWithSort(c, "风格", cards.Total(), control) },
		HeaderSkeleton: func(c *ui.Context) { h.cardsHeaderSkeleton(c, true) },
		Meta:           true,
		OnOpen:         func(id int64) { h.openCardFrom(cards, id) },
		OnPlay:         func(id int64) { h.playCardFrom(cards, id) },
		Art: func(p *ui.Painter, r ui.Rect, card *netease.Playlist, radius float32) {
			drawGenreArt(p, r, card.Name, radius)
		},
	})
}
