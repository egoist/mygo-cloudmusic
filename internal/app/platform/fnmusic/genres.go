package fnmusic

// Feiniu style pages. The styles are cards, as the albums are, and a style
// opens as a song list like an album's; both go through the shared card and
// list renderers, so only the sorts and the menu are the style's own.

import (
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"

	backend "netease-music-mygo/internal/platform/fnmusic"
)

func (h *Home) genrePage(c *ui.Context) {
	catalog := h.state.Catalog()
	cards := catalog.GenresList()
	if cards == nil {
		h.listPage(c)
		return
	}
	current := backend.ParseGenreSort(catalog.GenreSort)
	h.host.RenderCards(c, cards, CardView{
		Title: "风格", Count: cards.Total(),
		Header: func(c *ui.Context) { h.cardsHeader(c, "风格", cards.Total()) },
		Sort: []SortChoice{
			{Value: backend.SortGenreTrackCountDesc, Label: "歌曲数"},
			{Value: backend.SortGenreNameAsc, Label: "名称"},
		},
		Current: current,
		Meta:    true,
		OnSort: func(v string) {
			catalog.GenreSort = v
			cards.Reload()
			h.host.Invalidate()
		},
		OnOpen: func(id int64) { h.openCardFrom(cards, id) },
		OnPlay: func(id int64) { h.playCardFrom(cards, id) },
		Art: func(p *ui.Painter, r ui.Rect, card *netease.Playlist, radius float32) {
			drawGenreArt(p, r, card.Name, radius)
		},
	})
}

// genreTrackSorts are the orders a style's songs can be listed in.
var genreTrackSorts = []SortChoice{
	{Value: backend.SortGenreTrackCreatedDesc, Label: "最近添加"},
	{Value: backend.SortGenreTrackTitleAsc, Label: "歌名"},
	{Value: backend.SortGenreTrackUpdatedDesc, Label: "最近修改"},
}

// genreSortMenu orders a style's songs anew; the list is asked for again.
func (h *Home) genreSortMenu(m *ui.Menu, l *List) {
	catalog := h.state.Catalog()
	current := backend.ParseGenreTrackSort(catalog.GenreTrackSort)
	m.Submenu("排序", func(sub *ui.Menu) {
		for _, s := range genreTrackSorts {
			if sub.Item(s.Label).Checked(s.Value == current).Chosen() && s.Value != current {
				catalog.GenreTrackSort = s.Value
				catalog.Refresh(l.ID)
				h.host.Invalidate()
			}
		}
	})
}
