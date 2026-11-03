package qqmusic

import (
	"strings"

	"netease-music-mygo/internal/netease"
	backend "netease-music-mygo/internal/platform/qqmusic"

	"github.com/egoist/mygo/ui"
)

func (h *Home) openNewPlaylist() {
	h.nameOpen, h.name, h.editing, h.pending, h.nameErr = true, "", nil, nil, ""
}

func (h *Home) requestAddToPlaylist(plID int64, tr netease.Track) {
	go func() {
		err := backend.AddToPlaylist(plID, tr.ID)
		h.Post(func() {
			if err != nil {
				h.host.Toast("加入歌单失败，请稍后再试")
				return
			}
			// The list's cached songs are now one short of what it has.
			h.mu.Lock()
			delete(h.playlists, plID)
			h.mu.Unlock()
			if h.route.Page == PagePlaylist && h.route.ID == plID {
				h.loadPlaylist(plID)
			}
			h.host.Toast("已加入歌单")
			h.host.Invalidate()
		})
	}()
}

func (h *Home) playlistMenu(m *ui.Menu, pl netease.Playlist) {
	if m.Item("重命名…").Chosen() {
		h.nameOpen, h.name, h.editing, h.pending, h.nameErr = true, pl.Name, &pl, nil, ""
		return
	}
	if m.Item("删除歌单…").Chosen() {
		h.confirmRemove(pl, false)
	}
}

// confirmRemove asks before deleting a made playlist or unfavouriting a
// saved one, as the reference does.
func (h *Home) confirmRemove(pl netease.Playlist, saved bool) {
	h.removing, h.removeSaved = &pl, saved
	h.removeOpen, h.removeErr, h.removeBusy = true, "", false
}

func (h *Home) removeDialog(c *ui.Context) {
	if !h.removeOpen || h.removing == nil {
		return
	}
	t := c.Theme()
	pl := *h.removing
	title, action, desc := "删除歌单", "删除", "删除后无法恢复，歌单里的歌曲不会被删除。"
	if h.removeSaved {
		title, action, desc = "取消收藏", "取消收藏", "取消后，这个歌单会从「收藏的歌单」里移除。"
	}
	ui.DialogBase(c.Key("qq-remove-playlist"), &h.removeOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(360).Padding(24).Radius(18).Gap(14).Background(t.Background).Border(1, t.Border).
			Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4))
		ui.Text(c, title).FontSize(17).FontWeight(700)
		ui.Text(c, "「"+pl.Name+"」").FontSize(13.5)
		ui.Text(c, desc).FontSize(12.5).TextColor(t.TextMuted).LineHeight(1.45)
		if h.removeBusy {
			ui.Text(c, "正在处理…").FontSize(12.5).TextColor(t.TextMuted)
			h.host.Bone(c, 2).Width(312).Height(4)
		}
		if h.removeErr != "" {
			ui.Text(c, h.removeErr).FontSize(12.5).TextColor(t.Accent)
		}
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if h.host.PillButton(c, "qq-remove-cancel", nil, "取消", false).Clicked() {
				h.removeOpen = false
			}
			b := h.host.PillButton(c, "qq-remove-confirm", nil, action, true)
			if h.removeBusy {
				b.Disabled(true)
			}
			if b.Clicked() {
				h.finishRemove(pl, h.removeSaved)
			}
		})
	})
}

func (h *Home) finishRemove(pl netease.Playlist, saved bool) {
	if h.removeBusy {
		return
	}
	h.removeBusy, h.removeErr = true, ""
	go func() {
		var err error
		if saved {
			err = backend.UnfavoritePlaylist(pl.ID)
		} else {
			err = backend.DeletePlaylist(pl.ID)
		}
		h.Post(func() {
			h.removeBusy = false
			if err != nil {
				h.removeErr = "操作失败，请稍后再试"
				h.host.Invalidate()
				return
			}
			h.removeOpen = false
			lists := h.state.Playlists()
			out := lists[:0]
			for _, item := range lists {
				if item.ID != pl.ID {
					out = append(out, item)
				}
			}
			h.state.SetPlaylists(out)
			h.mu.Lock()
			delete(h.playlists, pl.ID)
			h.mu.Unlock()
			if h.route.Page == PagePlaylist && h.route.ID == pl.ID {
				h.Open(Route{Page: PageHome})
			}
			if saved {
				h.host.Toast("已取消收藏")
			} else {
				h.host.Toast("已删除歌单")
			}
			h.host.Invalidate()
		})
	}()
}

func (h *Home) playlistDialog(c *ui.Context) {
	if !h.nameOpen {
		return
	}
	t := c.Theme()
	ui.DialogBase(c.Key("qq-playlist-name"), &h.nameOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(340).Padding(24).Radius(18).Gap(14).Background(t.Background).Border(1, t.Border).
			Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4))
		title := "新建歌单"
		if h.editing != nil {
			title = "重命名歌单"
		}
		ui.Text(c, title).FontSize(17).FontWeight(700)
		in := ui.TextInput(c.Key("qq-playlist-name-input"), &h.name).Placeholder("歌单名称").Label("歌单名称").FontSize(14).AutoFocus()
		if h.nameErr != "" {
			ui.Text(c, h.nameErr).FontSize(12.5).TextColor(t.Accent)
		}
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if h.host.PillButton(c, "qq-playlist-name-cancel", nil, "取消", false).Clicked() {
				h.nameOpen = false
			}
			save := h.host.PillButton(c, "qq-playlist-name-save", nil, "保存", true)
			if save.Clicked() || in.Submitted() {
				h.savePlaylistName()
			}
		})
	})
}

func (h *Home) savePlaylistName() {
	name := strings.TrimSpace(h.name)
	if name == "" {
		h.nameErr = "请输入歌单名称"
		return
	}
	h.nameErr = ""
	if h.editing != nil {
		pl := *h.editing
		go func() {
			err := backend.RenamePlaylist(pl.ID, name)
			h.Post(func() {
				if err != nil {
					h.nameErr = "重命名失败，请稍后再试"
					h.host.Invalidate()
					return
				}
				lists := h.state.Playlists()
				for i := range lists {
					if lists[i].ID == pl.ID {
						lists[i].Name = name
					}
				}
				h.state.SetPlaylists(lists)
				h.nameOpen = false
				h.host.Invalidate()
			})
		}()
		return
	}
	pending := h.pending
	go func() {
		pl, err := backend.CreatePlaylist(name)
		if err == nil && len(pending) > 0 {
			ids := make([]int64, len(pending))
			for i := range pending {
				ids[i] = pending[i].ID
			}
			err = backend.AddToPlaylist(pl.ID, ids...)
		}
		h.Post(func() {
			if err != nil || pl == nil {
				h.nameErr = "新建歌单失败，请稍后再试"
				h.host.Invalidate()
				return
			}
			lists := h.state.Playlists()
			h.state.SetPlaylists(append([]netease.Playlist{playlist(*pl)}, lists...))
			h.nameOpen = false
			h.host.Invalidate()
		})
	}()
}
