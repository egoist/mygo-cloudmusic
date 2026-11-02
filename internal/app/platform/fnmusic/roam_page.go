package fnmusic

import (
	"context"
	"netease-music-mygo/internal/netease"

	"github.com/egoist/mygo/ui"
)

func (h *Home) roamSkeleton(c *ui.Context) {
	ui.Column(c).Fill().Padding(40, 40, 100, 40).Gap(12).Children(func() {
		h.host.TextBone(c, 30, 1.2).Width(90)
		h.host.TextBone(c, 14, 1.3).WidthPercent(58)
		h.host.Bone(c, 18).Size(112, 36)
	})
}

func (h *Home) roamPage(c *ui.Context) {
	t := c.Theme()
	player := h.host.Player()
	on := player != nil && player.Radio() == "fnmusic-roam"
	ui.Column(c).Fill().Padding(40, 40, 100, 40).Gap(12).Children(func() {
		ui.Text(c, "漫游").FontSize(30).FontWeight(700)
		ui.Text(c, "由 NAS 随机挑选曲库歌曲，一首接一首播放。").FontSize(14).TextColor(t.TextMuted)
		if on {
			if tr := player.Current(); tr != nil {
				ui.Text(c, "正在播放："+tr.Name+" · "+tr.ArtistNames()).FontSize(14)
			}
			if h.host.PillButton(c, "stop-fnmusic-roam", iconX, "退出漫游", false).Clicked() {
				player.EndRadio()
			}
			return
		}
		if h.host.PillButton(c, "start-fnmusic-roam", iconRadio, "开始漫游", true).Clicked() {
			gen := h.state.Generation()
			go func() {
				tr, err := h.state.StartRoam(context.Background())
				h.postCurrent(gen, func() {
					if err != nil {
						h.state.SetError(err)
						return
					}
					if p := h.host.Player(); p != nil {
						p.PlayRadio("fnmusic-roam", []netease.Track{tr}, func(got func([]netease.Track)) {
							go func() {
								next, nextErr := h.state.NextRoam(context.Background())
								h.host.Post(func() {
									if h.state.Generation() != gen {
										got(nil)
										return
									}
									if nextErr == nil {
										got([]netease.Track{next})
									} else {
										if nextErr != context.Canceled {
											h.state.SetError(nextErr)
										}
										got(nil)
									}
								})
							}()
						})
					}
				})
			}()
		}
	})
}
