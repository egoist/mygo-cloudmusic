package fnmusic

import (
	"context"
	"netease-music-mygo/internal/netease"
	backend "netease-music-mygo/internal/platform/fnmusic"
	"os"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (h *Home) RequestNewPlaylist(tracks []netease.Track) {
	h.nameOpen, h.name, h.rename, h.delete, h.deleteOpen, h.addTracks, h.addList = true, "", nil, nil, false, append([]netease.Track(nil), tracks...), nil
}

func (h *Home) RequestRename(pl netease.Playlist) {
	h.nameOpen, h.name, h.rename, h.delete, h.deleteOpen = true, pl.Name, &pl, nil, false
}

func (h *Home) RequestDelete(pl netease.Playlist) { h.delete, h.deleteOpen = &pl, true }

func (h *Home) RequestChangeCover(pl netease.Playlist) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: "选择歌单封面", Filters: []mygo.FileFilter{{Name: "图片", Extensions: []string{"png", "jpg", "jpeg", "webp", "gif"}}}})
	if err != nil || len(paths) == 0 {
		return
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		h.host.Toast("读取封面失败")
		return
	}
	if len(data) > backend.MaxCoverBytes {
		h.host.Toast(backend.ErrBadCover.Error())
		return
	}
	gen := h.state.Generation()
	go func() {
		_, err := h.state.ChangePlaylistCover(context.Background(), pl, data)
		h.postCurrent(gen, func() {
			if err != nil {
				h.host.Toast(err.Error())
				return
			}
			if h.state != nil && h.state.Catalog() != nil {
				h.state.Catalog().Refresh(pl.ID)
			}
			h.host.Toast("已更换封面")
			h.host.Invalidate()
		})
	}()
}

func (h *Home) RequestAddTracks(pl netease.Playlist, tracks []netease.Track) {
	if h == nil || h.state == nil {
		return
	}
	gen := h.state.Generation()
	go func() {
		err := h.state.AddToPlaylist(context.Background(), pl, tracks)
		h.postCurrent(gen, func() {
			if err != nil {
				h.host.Toast("添加失败，请稍后再试")
			} else {
				h.state.Catalog().Refresh(pl.ID)
				h.host.Toast("已添加到歌单")
			}
		})
	}()
}

func (h *Home) RequestRemoveTrack(pl netease.Playlist, tr netease.Track) {
	if h == nil || h.state == nil {
		return
	}
	gen := h.state.Generation()
	go func() {
		err := h.state.RemoveFromPlaylist(context.Background(), pl, tr)
		h.postCurrent(gen, func() {
			if err != nil {
				h.host.Toast("移除失败，请稍后再试")
			} else {
				h.state.Catalog().Refresh(pl.ID)
				h.host.Toast("已从歌单移除")
			}
		})
	}()
}

func (h *Home) submitName() {
	name := strings.TrimSpace(h.name)
	if name == "" || h.state == nil {
		return
	}
	if h.rename != nil {
		pl := *h.rename
		h.nameOpen, h.rename = false, nil
		gen := h.state.Generation()
		go func() {
			err := h.state.RenamePlaylist(context.Background(), pl, name)
			h.postCurrent(gen, func() {
				if err != nil {
					h.host.Toast("重命名失败，请稍后再试")
				}
			})
		}()
		return
	}
	tracks, list := append([]netease.Track(nil), h.addTracks...), h.addList
	h.nameOpen = false
	gen := h.state.Generation()
	go func() {
		pl, err := h.state.CreatePlaylist(context.Background(), name)
		h.postCurrent(gen, func() {
			if err != nil {
				h.host.Toast("创建歌单失败，请稍后再试")
				return
			}
			h.host.Navigate(Route{Page: PagePlaylist, ID: pl.ID})
			if list != nil {
				h.RequestAddList(pl, list)
			} else if len(tracks) > 0 {
				h.RequestAddTracks(pl, tracks)
			}
		})
	}()
}

func (h *Home) confirmDelete() {
	if h.delete == nil {
		return
	}
	pl := *h.delete
	h.delete, h.deleteOpen = nil, false
	gen := h.state.Generation()
	go func() {
		err := h.state.DeletePlaylist(context.Background(), pl)
		h.postCurrent(gen, func() {
			if err != nil {
				h.host.Toast("删除歌单失败，请稍后再试")
			} else {
				h.state.Catalog().Refresh(pl.ID)
				if h.route.ID == pl.ID {
					h.Open(Route{Page: PageLibrary})
					h.host.Navigate(h.route)
				}
			}
		})
	}()
}

func (h *Home) dialogs(c *ui.Context) {
	t := c.Theme()
	if h.nameOpen {
		ui.DialogBase(c.Key("fnmusic-playlist-name"), &h.nameOpen, func(backdrop, panel ui.Element) {
			backdrop.Background(ui.RGBA(0, 0, 0, .35))
			panel.Width(360).Padding(24).Radius(18).Gap(14).Background(t.Background).Border(1, t.Border)
			ui.Text(c, map[bool]string{true: "重命名歌单", false: "新建歌单"}[h.rename != nil]).FontSize(17).FontWeight(700)
			in := ui.TextInput(c.Key("fnmusic-playlist-name-input"), &h.name).Placeholder("歌单名称").Label("歌单名称").FontSize(14).AutoFocus()
			if in.Submitted() {
				h.submitName()
			}
			ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
				if ui.Button(c.Key("fnmusic-playlist-name-cancel"), "取消").Clicked() {
					h.nameOpen = false
				}
				ok := ui.PrimaryButton(c.Key("fnmusic-playlist-name-ok"), "确定")
				if strings.TrimSpace(h.name) == "" {
					ok.Disabled(true)
				}
				if ok.Clicked() {
					h.submitName()
				}
			})
		})
	}
	if h.delete != nil && h.deleteOpen {
		ui.DialogBase(c.Key("fnmusic-playlist-delete"), &h.deleteOpen, func(backdrop, panel ui.Element) {
			backdrop.Background(ui.RGBA(0, 0, 0, .35))
			panel.Width(360).Padding(24).Radius(18).Gap(12).Background(t.Background).Border(1, t.Border)
			ui.Text(c, "删除歌单").FontSize(17).FontWeight(700)
			ui.Text(c, "确定删除歌单“"+h.delete.Name+"”吗？删除后无法恢复。").FontSize(13).TextColor(t.TextMuted)
			ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
				if ui.Button(c.Key("fnmusic-delete-cancel"), "取消").Clicked() {
					h.delete, h.deleteOpen = nil, false
				}
				if ui.PrimaryButton(c.Key("fnmusic-delete-ok"), "删除").Clicked() {
					h.confirmDelete()
				}
			})
		})
	}
}
