package fnmusic

import (
	"fmt"
	backend "netease-music-mygo/internal/platform/fnmusic"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (h *Home) CloseTransient() {
	if h == nil {
		return
	}
	h.accountOpen = false
	h.connectionGen++
	h.connectionOpen, h.connectionBusy, h.connectionError = false, false, ""
	h.nameOpen, h.deleteOpen = false, false
}

func (h *Home) Sidebar(c *ui.Context) {
	if h == nil || h.host == nil || h.state == nil {
		return
	}
	h.host.PlatformSearchField(c, "搜索歌曲、歌手、播客", func(query string) {
		if query = strings.TrimSpace(query); query != "" {
			h.Open(Route{Page: PageSearch, Query: query, Kind: "songs"})
			h.host.Navigate(h.route)
		}
	})
	ui.Scroll(c.Key("fnmusic-nav")).TrackScroll(&h.navScroll).Grow(1).Margin(0, -6).Padding(0, 6).Gap(4).Children(func() {
		if h.state.Connecting() {
			h.sidebarLoading(c)
		} else if !h.state.Connected() {
			h.host.NavItem(c, iconServer, "连接飞牛音乐", Route{Page: PageLibrary})
		} else {
			h.host.NavItem(c, iconNote, "音乐库", Route{Page: PageLibrary})
			if h.state.FolderViewEnabled() {
				h.host.NavItem(c, iconFolder, "文件夹", Route{Page: PageFolders})
			}
			h.host.NavItem(c, iconGenre, "风格", Route{Page: PageGenres})
			h.host.NavItem(c, iconAlbum, "专辑", Route{Page: PageAlbums})
			h.host.NavItem(c, iconUser, "歌手", Route{Page: PageArtists})
			h.host.NavItem(c, iconHeart, "我喜欢", Route{Page: PagePlaylist, ID: backend.FavoritesID})
			h.host.NavItem(c, iconClock, "最近播放", Route{Page: PagePlaylist, ID: backend.RecentID})
			h.host.NavItem(c, iconRadio, "漫游", Route{Page: PageRoam})
		}
		if h.route.Page == PageSearch || h.route.Page == PageSearchAlbums || h.route.Page == PageSearchArtists {
			h.host.NavItem(c, iconSearch, "搜索结果", h.route)
		}
		if h.state.Connected() {
			h.host.PlaylistHeader(c, func() { h.RequestNewPlaylist(nil) })
			for _, pl := range h.state.Playlists() {
				r := Route{Page: PagePlaylist, ID: pl.ID}
				h.host.PlaylistItem(c, fmt.Sprintf("fnmusic-playlist-%d", pl.ID), pl, h.route == r, func(m *ui.Menu) { h.playlistMenu(m, pl) }, func() {
					h.host.Navigate(r)
				})
			}
		}
	})
	if player := h.host.Player(); player != nil {
		if tr := player.Current(); tr != nil {
			h.host.NowPlaying(c, tr)
		}
	}
	cfg := toConnectionConfig(h.state.Config())
	username, status := cfg.Username, "未连接"
	if h.state.Connecting() {
		if username == "" {
			username = "飞牛音乐"
		}
		status = "正在连接…"
	}
	if username == "" {
		username = "飞牛音乐（未连接）"
	}
	connected := h.state.Connected()
	if connected {
		if username == "飞牛音乐（未连接）" {
			username = "飞牛音乐"
		}
		status = "飞牛音乐"
		if err := h.state.Error(); err != nil {
			status = "连接失败"
		}
	} else if err := h.state.Error(); err != nil {
		status = "连接失败"
	}
	actions := h.accountActions()
	h.host.AccountRow(c, iconServer, username, status, &h.accountOpen, actions)
	h.host.PlatformSwitcher(c)
}

// sidebarLoading keeps the configured platform layout stable while the first
// connection and playlist request are in flight. Showing the connect CTA here
// made the sidebar flash to a different navigation tree on every launch.
func (h *Home) sidebarLoading(c *ui.Context) {
	// As many as the connected sidebar has, so it does not grow once in.
	for i, w := range []float32{56, 44, 36, 36, 36, 52, 64, 36} {
		ui.Row(c.Key(fmt.Sprintf("fnmusic-nav-loading-%d", i))).Height(34).PaddingX(10).Gap(10).AlignItems(ui.Center).Children(func() {
			h.host.Bone(c, 5).Size(17, 17)
			h.host.TextBone(c, 13.5, 1.3).Width(w)
		})
	}
	// The playlists' heading, and a few of them.
	ui.Row(c.Key("fnmusic-nav-loading-lists")).Height(30).PaddingX(10).Margin(14, 0, 0, 0).AlignItems(ui.Center).Children(func() {
		h.host.TextBone(c, 12, 1.3).Width(48)
	})
	for i, w := range []float32{88, 64, 104} {
		ui.Row(c.Key(fmt.Sprintf("fnmusic-nav-loading-list-%d", i))).Height(40).PaddingX(10).Gap(10).AlignItems(ui.Center).Children(func() {
			h.host.Bone(c, 6).Size(28, 28)
			h.host.TextBone(c, 13, 1.3).Width(w)
		})
	}
}
