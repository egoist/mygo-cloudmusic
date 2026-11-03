package fnmusic

// Feiniu folder pages. The libraries and each folder in them are pages of
// their own, drawn like the upstream lists: a header with the folder's tile,
// its path and the play buttons, then its folders and songs as list rows.

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
	backend "netease-music-mygo/internal/platform/fnmusic"
)

func (h *Home) folderPage(c *ui.Context) {
	if h.state == nil || !h.state.FolderViewEnabled() {
		h.route = Route{Page: PageLibrary}
		h.listPage(c)
		return
	}
	f := h.syncFolders()
	h.ensureFolderRoots()
	if h.route.Query == "" {
		h.folderRootsPage(c, f)
		return
	}
	h.folderContentPage(c, f, h.route.Query)
}

// folderRootsPage lists the libraries, the top of the folders.
func (h *Home) folderRootsPage(c *ui.Context, f *folderViewState) {
	if f.rootsErr != nil {
		h.folderMessage(c, "音乐库加载失败，请稍后重试", func() { h.refreshFolder("") })
		return
	}
	if f.roots == nil {
		h.folderRootsSkeleton(c)
		return
	}
	roots := make([]backend.SharedLibrary, 0, len(f.roots))
	for _, root := range f.roots {
		if strings.TrimSpace(root.Path) != "" {
			roots = append(roots, root)
		}
	}
	h.host.PageList(c, "fnmusic-folder-roots", f.listState(""), len(roots)+1, func(i int) {
		if i == 0 {
			ui.Column(c).PaddingX(12).Children(func() {
				h.cardsHeader(c, "文件夹", len(roots))
				if len(roots) == 0 {
					ui.Text(c, "飞牛音乐还没有设置音乐库").FontSize(13).TextColor(c.Theme().TextMuted)
				}
			})
			return
		}
		root := roots[i-1]
		node := backend.FolderNode{Name: f.rootName(root), Path: strings.TrimRight(root.Path, "/")}
		h.folderRow(c, node, folderRootNote(root), root.AccessStatus == 0)
	})
}

func folderRootNote(root backend.SharedLibrary) string {
	switch root.AccessStatus {
	case 0:
		return root.Path
	case 1:
		return "目录不存在"
	case 2:
		return "没有访问权限"
	default:
		return "尚未授权"
	}
}

// folderContentPage shows one folder: its folders first, then its songs.
func (h *Home) folderContentPage(c *ui.Context, f *folderViewState, folderPath string) {
	h.loadFolderPage(folderPath)
	page, ok := f.pages[folderPath]
	if !ok {
		if f.errs[folderPath] != nil {
			h.folderMessage(c, "文件夹加载失败，请稍后重试", func() { h.refreshFolder(folderPath) })
			return
		}
		h.folderContentSkeleton(c)
		return
	}
	tracks := make([]netease.Track, len(page.Tracks))
	for i, item := range page.Tracks {
		tracks[i] = item.Track
	}
	rows := 1 + len(page.Folders) + len(page.Tracks)
	h.host.PageList(c, "fnmusic-folder-"+folderPath, f.listState(folderPath), rows, func(i int) {
		if i == 0 {
			h.folderHeader(c, f, page)
			return
		}
		i--
		if i < len(page.Folders) {
			node := page.Folders[i]
			note := fmt.Sprintf("%d 项", node.ItemCount)
			if node.IsAdminDeleted {
				note = "已移到回收站"
			}
			h.folderRow(c, node, note, true)
			return
		}
		i -= len(page.Folders)
		item := page.Tracks[i]
		tr := item.Track
		row := tr
		h.host.TrackRow(c.Key("fnmusic-folder-track"), i, &row, TrackView{
			OnPlay: func(at int) {
				if p := h.host.Player(); p != nil {
					p.PlayQueue(tracks, at)
				}
			},
			Menu:      func(m *ui.Menu, t *netease.Track, _ int64) { h.folderTrackMenu(m, t, item) },
			Liked:     h.state.Liked,
			Favorite:  h.RequestFavorite,
			ShowAlbum: true,
		})
	})
}

func (h *Home) folderContentSkeleton(c *ui.Context) {
	h.host.TrackSkeleton(c, TrackView{
		ShowAlbum:      true,
		HeaderSkeleton: h.folderHeaderSkeleton,
	})
}

// folderHeaderSkeleton reserves the breadcrumbs and folder actions without
// exposing their labels before the folder is ready.
func (h *Home) folderHeaderSkeleton(c *ui.Context) {
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 12, 26, 12).Children(func() {
		h.host.Bone(c, 14).Size(212, 212)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			h.host.TextBone(c, 12, 1.3).Width(120)
			h.host.TextBone(c, 30, 1.2).WidthPercent(48)
			h.host.TextBone(c, 13, 1.3).Width(180)
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				h.host.Bone(c, 18).Size(92, 36)
				h.host.Bone(c, 18).Size(120, 36)
				h.host.Bone(c, 17).Size(34, 34)
				ui.Box(c).Grow(1)
				h.host.Bone(c, 15).Height(30).Width(76)
			})
		})
	})
}

// folderHeader is the folder's tile, its path, name and contents, and the
// buttons that play it, as a playlist's header.
func (h *Home) folderHeader(c *ui.Context, f *folderViewState, page backend.FolderPage) {
	t := c.Theme()
	name := strings.TrimSpace(page.Name)
	if name == "" {
		name = path.Base(page.Path)
	}
	if root, ok := f.folderRoot(page.Path); ok && strings.TrimRight(root.Path, "/") == page.Path {
		name = f.rootName(root)
	}
	ui.Row(c).Gap(28).AlignItems(ui.End).Padding(0, 12, 26, 12).Children(func() {
		folderTile(c, 212, 14, 72).Shadow(0, 18, 40, -10, ui.RGBA(0, 0, 0, 0.18))
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(8).Children(func() {
			h.folderCrumbBar(c, f, page.Path)
			ui.Text(c, name).FontSize(30).FontWeight(800).LetterSpacing(-0.6).MaxLines(2).LineHeight(1.2)
			meta := []string{}
			if n := len(page.Folders); n > 0 {
				meta = append(meta, fmt.Sprintf("%d 个文件夹", n))
			}
			if n := len(page.Tracks); n > 0 {
				meta = append(meta, fmt.Sprintf("%d 首", n))
			}
			if len(meta) == 0 {
				meta = append(meta, "空文件夹")
			}
			if f.loading[page.Path] {
				meta = append(meta, "正在刷新…")
			}
			ui.Text(c, strings.Join(meta, " · ")).FontSize(13).TextColor(t.TextMuted)
			ui.Row(c).Gap(10).Margin(10, 0, 0, 0).AlignItems(ui.Center).Children(func() {
				if h.host.PillButton(c, "fnmusic-folder-play", iconPlay, "播放", true).Clicked() {
					h.playFolders([]string{page.Path}, false)
				}
				if h.host.PillButton(c, "fnmusic-folder-shuffle", iconShuffle, "随机播放", false).Clicked() {
					h.playFolders([]string{page.Path}, true)
				}
				h.host.MoreButton(c, "fnmusic-folder-more", func(m *ui.Menu) { h.folderMenu(m, page.Path, false, true) })
				ui.Box(c).Grow(1)
				h.folderSortButton(c, f)
			})
		})
	})
}

// folderCrumbBar names the folders over this one; each opens its folder.
func (h *Home) folderCrumbBar(c *ui.Context, f *folderViewState, folderPath string) {
	t := c.Theme()
	crumbs := append([]folderCrumb{{Name: "文件夹"}}, f.folderCrumbs(folderPath)...)
	ui.Row(c).Gap(4).AlignItems(ui.Center).MinWidth(0).Children(func() {
		for i, crumb := range crumbs {
			if i > 0 {
				ui.Text(c, "›").FontSize(12).TextColor(t.TextMuted)
			}
			b := ui.ButtonBase(c.Key("fnmusic-folder-crumb-" + crumb.Path)).FocusRing(false).Label(crumb.Name).Shrink(1).MinWidth(0)
			color := t.Accent
			if b.Hovered() {
				color = t.Text
			}
			b.Children(func() {
				ui.Text(c, crumb.Name).FontSize(12).FontWeight(600).TextColor(color).SingleLine()
			})
			if b.Clicked() {
				h.openFolder(crumb.Path)
			}
		}
	})
}

// folderSortButton orders the folders and songs; the menu marks the order.
func (h *Home) folderSortButton(c *ui.Context, f *folderViewState) {
	t := c.Theme()
	b := ui.ButtonBase(c.Key("fnmusic-folder-sort")).Height(30).PaddingX(12).Radius(999).Gap(6).AlignItems(ui.Center).FocusRing(false).Label("排序")
	if b.Hovered() {
		b.Background(t.Text.Alpha(0.06))
	}
	b.Children(func() {
		ui.Text(c, folderSortLabel(f.sort)).FontSize(12.5).TextColor(t.TextMuted)
		ui.Text(c, "⌄").FontSize(12).TextColor(t.TextMuted)
	})
	b.Menu(func(m *ui.Menu) {
		for _, sort := range backend.FolderSortValues() {
			if m.Item(folderSortLabel(sort)).Checked(sort == f.sort).Chosen() {
				h.setFolderSort(sort)
			}
		}
	})
}

// folderRow is a folder in a list: it opens on a click, plays from its
// button, and its menu plays, adds or removes it.
func (h *Home) folderRow(c *ui.Context, node backend.FolderNode, note string, open bool) {
	t := c.Theme()
	row := ui.ButtonBase(c.Key("fnmusic-folder-row-" + node.Path)).Height(54).PaddingX(12).Gap(14).Radius(10).
		AlignItems(ui.Center).FocusRing(false).Label(node.Name)
	hovered := row.Hovered()
	bg := ui.Transparent
	if hovered && open {
		bg = t.Text.Alpha(0.05)
	}
	row.Background(row.AnimateColor("bg", bg, ui.Animation{Duration: 100 * time.Millisecond}))
	if !open || node.IsAdminDeleted {
		row.Opacity(0.5)
	}
	if open {
		row.ContextMenu(func(m *ui.Menu) { h.folderMenu(m, node.Path, node.IsAdminDeleted, false) })
	}
	row.Children(func() {
		ui.Box(c).Width(26)
		folderTile(c, 38, 6, 18)
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, node.Name).FontSize(14).FontWeight(500).SingleLine()
			ui.Text(c, note).FontSize(12.5).TextColor(t.TextMuted).SingleLine()
		})
		ui.Box(c).Size(32, 32).Center().Children(func() {
			if hovered && open && !node.IsAdminDeleted {
				play := ui.ButtonBase(c.Key("play")).Size(30, 30).Radius(999).Center().FocusRing(false).Label("播放")
				if play.Hovered() {
					play.Background(t.Text.Alpha(0.08))
				}
				play.Children(func() { ui.Icon(c, iconPlay).Size(13, 13).TextColor(t.Text) })
				if play.Clicked() {
					h.playFolders([]string{node.Path}, false)
				}
			}
		})
		ui.Text(c, "›").FontSize(18).TextColor(t.TextMuted).Width(44).TextAlign(ui.End)
	})
	if row.Clicked() && open && !node.IsAdminDeleted {
		h.openFolder(node.Path)
	}
}

// folderTile stands for a folder where a cover would be.
func folderTile(c *ui.Context, size, radius, icon float32) ui.Element {
	t := c.Theme()
	return ui.Box(c).Size(size, size).Shrink(0).Radius(radius).Background(t.Accent.Alpha(0.12)).Center().Children(func() {
		ui.Icon(c, iconFolder).Size(icon, icon).TextColor(t.Accent)
	})
}

func (h *Home) folderMenu(m *ui.Menu, folderPath string, deleted, current bool) {
	if deleted {
		if m.Item("恢复文件夹").Chosen() {
			h.recoverFolders([]string{folderPath})
		}
		return
	}
	if !current {
		if m.Item("播放").Chosen() {
			h.playFolders([]string{folderPath}, false)
		}
		if m.Item("随机播放").Chosen() {
			h.playFolders([]string{folderPath}, true)
		}
		m.Separator()
	}
	m.Submenu("全部添加到歌单", func(sub *ui.Menu) {
		for _, pl := range h.state.Playlists() {
			if sub.Item(pl.Name).Chosen() {
				h.addFolderToPlaylist(folderPath, pl)
			}
		}
	})
	if m.Item("复制路径").Chosen() {
		h.host.CopyText(folderPath)
	}
	if current {
		if m.Item("刷新").Chosen() {
			h.refreshFolder(folderPath)
		}
		return
	}
	m.Separator()
	if m.Item("移到回收站").Chosen() {
		h.deleteFolders([]string{folderPath}, false)
	}
}

func (h *Home) folderTrackMenu(m *ui.Menu, tr *netease.Track, item backend.FolderTrack) {
	h.trackMenu(m, tr, 0)
	m.Separator()
	if item.Path != "" && m.Item("复制文件路径").Chosen() {
		h.host.CopyText(item.Path)
	}
	if item.IsAdminDeleted {
		if m.Item("恢复歌曲").Chosen() {
			h.recoverTracks([]string{tr.Ref})
		}
	} else if m.Item("移到回收站").Chosen() {
		h.deleteTracks([]string{tr.Ref}, false)
	}
}

// folderRootsSkeleton is the libraries' page while they load: its title,
// and rows of blocks shaped as folderRow's.
func (h *Home) folderRootsSkeleton(c *ui.Context) {
	var st ui.ListState
	h.host.PageList(c, "fnmusic-folder-roots-loading", &st, 5, func(i int) {
		if i == 0 {
			ui.Column(c).PaddingX(12).Gap(4).Margin(0, 0, 18, 0).Children(func() {
				h.host.TextBone(c, 30, 1.2).WidthPercent(36)
				h.host.TextBone(c, 13, 1.3).Width(52)
			})
			return
		}
		ui.Row(c).Height(54).PaddingX(12).Gap(14).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Width(26)
			h.host.Bone(c, 6).Size(38, 38)
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
				h.host.TextBone(c, 14, 1.3).WidthPercent([]float32{22, 30, 18, 26}[i%4])
				h.host.TextBone(c, 12.5, 1.3).WidthPercent([]float32{40, 34, 46, 38}[i%4])
			})
			ui.Box(c).Size(32, 32)
			ui.Row(c).Width(44).Justify(ui.End).Children(func() {
				h.host.TextBone(c, 18, 1.3).Width(10)
			})
		})
	})
}

func (h *Home) folderMessage(c *ui.Context, msg string, retry func()) {
	ui.Column(c).Fill().Center().Gap(12).Children(func() {
		ui.Text(c, msg).FontSize(13).TextColor(c.Theme().TextMuted)
		if retry != nil && h.host.PillButton(c, "fnmusic-folder-retry", iconReload, "重试", false).Clicked() {
			retry()
		}
	})
}
