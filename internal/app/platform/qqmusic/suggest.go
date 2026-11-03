package qqmusic

import (
	"context"
	"strings"
	"time"

	backend "netease-music-mygo/internal/platform/qqmusic"

	"github.com/egoist/mygo/ui"
)

const suggestDelay = 220 * time.Millisecond

type suggestItem struct {
	key, name, detail, kind string
	route                   Route
}

type suggestState struct {
	query   string
	items   []suggestItem
	loading bool
	gen     uint64
	timer   *time.Timer
	cancel  context.CancelFunc
}

func (h *Home) suggestFor(q string) {
	if q == h.suggest.query {
		return
	}
	h.dismissSuggestions()
	h.suggest.query, h.suggest.loading = q, true
	ctx, cancel := context.WithCancel(context.Background())
	h.suggest.cancel = cancel
	gen := h.suggest.gen
	h.suggest.timer = time.AfterFunc(suggestDelay, func() {
		res, err := backend.SearchSuggestContext(ctx, q)
		h.Post(func() {
			if gen != h.suggest.gen || q != h.suggest.query {
				return
			}
			h.suggest.loading = false
			if err == nil {
				h.suggest.items = suggestionList(res)
			}
			h.host.Invalidate()
		})
	})
}

func (h *Home) dismissSuggestions() {
	if h.suggest.timer != nil {
		h.suggest.timer.Stop()
	}
	if h.suggest.cancel != nil {
		h.suggest.cancel()
	}
	h.suggest = suggestState{gen: h.suggest.gen + 1}
}

func suggestionList(res backend.Suggestions) []suggestItem {
	byKind := map[string][]suggestItem{}
	for _, t := range res.Songs {
		byKind["songs"] = append(byKind["songs"], suggestItem{
			key: "song:" + t.MID, name: t.Name, detail: t.ArtistNames(), kind: "单曲",
			route: Route{Page: PageSearch, Query: t.Name},
		})
	}
	for _, ar := range res.Artists {
		byKind["artists"] = append(byKind["artists"], suggestItem{
			key: "artist:" + ar.MID, name: ar.Name, kind: "歌手",
			route: Route{Page: PageArtist, ID: ar.ID},
		})
	}
	for _, al := range res.Albums {
		byKind["albums"] = append(byKind["albums"], suggestItem{
			key: "album:" + al.MID, name: al.Name, detail: al.ArtistNames(), kind: "专辑",
			route: Route{Page: PageAlbum, ID: al.ID},
		})
	}
	for _, pl := range res.Playlists {
		byKind["playlists"] = append(byKind["playlists"], suggestItem{
			key: "playlist:" + strconvID(pl.ID), name: pl.Name, detail: pl.Creator.Nickname, kind: "歌单",
			route: Route{Page: PagePlaylist, ID: pl.ID},
		})
	}
	var out []suggestItem
	for _, kind := range append(res.Order, "songs", "artists", "albums", "playlists") {
		out = append(out, byKind[kind]...)
		delete(byKind, kind)
	}
	return out[:min(len(out), 10)]
}

func (h *Home) searchField(c *ui.Context) {
	t := c.Theme()
	field := ui.Row(c).Height(32).PaddingX(10).Gap(7).Radius(9).AlignItems(ui.Center).Margin(0, 0, 14, 0)
	if c.Focused(h.searchBox) {
		field.Background(t.Background).Shadow(0, 0, 0, 3, t.Accent.Alpha(0.25)).Border(1, t.Accent.Alpha(0.6))
	} else {
		field.Background(t.Text.Alpha(0.035)).Border(1, t.Border)
	}
	field.Children(func() {
		ui.Icon(c, iconSearch).Size(14, 14).TextColor(t.TextMuted)
		cb := ui.ComboboxBase(c.Key("qq-search"), &h.query)
		in := cb.Input.Bind(&h.searchBox).Placeholder("搜索 QQ 音乐").Label("搜索").FontSize(13).Grow(1).MinWidth(0)
		q := strings.TrimSpace(h.query)
		history := h.host.SearchHistory()
		if q == "" {
			h.dismissSuggestions()
			if len(history) == 0 || !c.Focused(h.searchBox) {
				cb.SetOpen(false)
			}
		} else {
			h.suggestFor(q)
		}
		items := map[string]suggestItem{}
		if q != "" {
			for _, item := range h.suggest.items {
				items[item.key] = item
			}
		}
		cb.Popup(func(panel ui.Element) {
			panel.AttachTo(field, ui.AnchorBottomStart, ui.AnchorTopStart).
				Width(360).Padding(6).Margin(8, 0, 0, 0).Radius(12).
				Background(t.Background).Border(1, t.Border).Shadow(0, 8, 24, -4, ui.RGBA(0, 0, 0, 0.2))
			option := func(key, label string) ui.Element {
				row := cb.Item(key).MinHeight(36).Padding(6, 10).Gap(10).Radius(7).AlignItems(ui.Center).Label(label)
				if row.Highlighted() {
					row.Background(t.Text.Alpha(0.06))
				}
				return row
			}
			if q == "" {
				if len(history) > 0 {
					panel.Label("最近搜索")
					for _, past := range history {
						past := past
						option("history:"+past, past).Children(func() {
							ui.Icon(c, iconSearch).Size(13, 13).TextColor(t.TextMuted)
							ui.Text(c, past).FontSize(13).SingleLine().Grow(1)
						})
					}
				}
				return
			}
			option("query", "搜索“"+q+"”").Children(func() {
				ui.Icon(c, iconSearch).Size(13, 13).TextColor(t.TextMuted)
				ui.Text(c, "搜索“"+q+"”").FontSize(13).FontWeight(500).SingleLine().Shrink(1).MinWidth(0)
			})
			if len(h.suggest.items) > 0 {
				panel.Label("搜索建议")
			}
			for _, item := range h.suggest.items {
				item := item
				option(item.key, item.name).Children(func() {
					ui.Text(c, item.name).FontSize(13).FontWeight(500).SingleLine().Shrink(1).MinWidth(0).Grow(1)
					if item.detail != "" {
						ui.Text(c, item.detail).FontSize(12).TextColor(t.TextMuted).SingleLine().Shrink(1)
					}
					ui.Text(c, item.kind).FontSize(11).TextColor(t.TextMuted)
				})
			}
		})
		if key, ok := cb.Chosen(); ok {
			cb.SetOpen(false)
			if item, found := items[key]; found {
				h.Open(item.route)
				if item.route.Page == PageSearch {
					h.startSearch(item.name)
				}
				h.host.Navigate(h.route)
				return
			}
			if strings.HasPrefix(key, "history:") {
				h.submitSearch(strings.TrimPrefix(key, "history:"))
			} else if key == "query" {
				h.submitSearch(q)
			}
		} else if in.Submitted() {
			h.submitSearch(q)
		}
	})
}

func (h *Home) submitSearch(q string) {
	q = strings.TrimSpace(q)
	if q == "" {
		return
	}
	h.query = q
	h.host.RememberSearch(q)
	h.dismissSuggestions()
	h.startSearch(q)
	h.Open(Route{Page: PageSearch, Query: q})
	h.host.Navigate(h.route)
}
