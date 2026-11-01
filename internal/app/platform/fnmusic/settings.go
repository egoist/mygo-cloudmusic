package fnmusic

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	appplatform "netease-music-mygo/internal/app/platform"
)

// Settings is the persisted configuration owned by the Feiniu platform. The
// root app stores it under the platform namespace but does not interpret it.
type Settings struct {
	Config           appplatform.ConnectionConfig `json:"config,omitempty"`
	TranscodeMode    appplatform.TranscodeMode    `json:"transcodeMode,omitempty"`
	TranscodeBitrate int                          `json:"transcodeBitrate,omitempty"`
	LibrarySort      string                       `json:"librarySort,omitempty"`
}

func DefaultSettings() Settings { return Settings{TranscodeMode: appplatform.TranscodeAuto} }

func (s *Settings) Normalize() {
	if s == nil {
		return
	}
	if s.TranscodeMode != appplatform.TranscodeAuto && s.TranscodeMode != appplatform.TranscodeNative && s.TranscodeMode != appplatform.TranscodeAlways {
		s.TranscodeMode = appplatform.TranscodeAuto
	}
	if s.TranscodeBitrate < 0 {
		s.TranscodeBitrate = 0
	}
}

func (h *Home) Settings(c *ui.Context) {
	if h == nil || h.host == nil || h.state == nil {
		return
	}
	t := *c.Theme()
	c.Root().Background(t.Background)
	ui.Scroll(c).Fill().Padding(24, 28, 32, 28).Gap(8).Children(func() {
		ui.Text(c, "飞牛音乐").FontSize(12).FontWeight(600).TextColor(t.TextMuted).Margin(14, 4, 4, 4)
		ui.Column(c).Radius(12).Background(t.Background).Border(1, t.Border).Dividers(1, t.Border).Children(func() {
			var cfg appplatform.ConnectionConfig
			if h.runtime != nil {
				cfg = h.runtime.Config()
			}
			ui.Row(c).Padding(12, 16).Gap(16).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).Gap(3).Children(func() {
					name := "未连接"
					desc := "连接后可播放飞牛音乐库、歌单和喜欢的歌曲"
					if h.state.Connected() {
						name = cfg.Username
						if name == "" {
							name = "飞牛音乐"
						}
						desc = "已连接 · " + displayAddress(cfg)
					}
					if err := h.state.Error(); err != nil {
						desc = "连接失败：" + err.Error()
					}
					ui.Text(c, name).FontSize(13.5).FontWeight(500)
					ui.Text(c, desc).FontSize(12).TextColor(t.TextMuted)
				})
				label := "连接…"
				if h.state.Connected() {
					label = "重新连接…"
				}
				if h.host.PillButton(c, "fnmusic-connection-open", iconServer, label, true).Clicked() {
					h.openConnection()
				}
				if h.state.Connected() && h.host.PillButton(c, "fnmusic-disconnect", iconSignOut, "断开", false).Clicked() {
					h.disconnect()
				}
			})
			if h.state.Connected() {
				h.transcodeSettings(c, t)
			}
		})
		h.host.SharedSettings(c)
	})
	h.connectionDialog(c)
}

func (h *Home) transcodeSettings(c *ui.Context, t ui.Theme) {
	if h.runtime == nil {
		return
	}
	transcode := h.runtime.Transcode()
	mode, bitrate := transcode.Mode, transcode.Bitrate
	ui.Row(c).Padding(12, 16).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			ui.Text(c, "转码策略").FontSize(13.5).FontWeight(500)
			ui.Text(c, "自动模式只转换播放器不支持的格式").FontSize(12).TextColor(t.TextMuted)
		})
		labels := []string{"自动转码", "仅原始格式", "始终转码"}
		chosen := labels[0]
		if mode == appplatform.TranscodeNative {
			chosen = labels[1]
		} else if mode == appplatform.TranscodeAlways {
			chosen = labels[2]
		}
		if ui.Select(c.Key("fnmusic-transcode-mode"), &chosen, labels).Label("转码策略").Width(150).Changed() {
			m := appplatform.TranscodeAuto
			if chosen == labels[1] {
				m = appplatform.TranscodeNative
			} else if chosen == labels[2] {
				m = appplatform.TranscodeAlways
			}
			h.runtime.SetTranscode(appplatform.TranscodeSettings{Mode: m, Bitrate: bitrate})
		}
	})
	ui.Row(c).Padding(12, 16).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			ui.Text(c, "转码码率").FontSize(13.5).FontWeight(500)
			ui.Text(c, "仅服务端转码时生效").FontSize(12).TextColor(t.TextMuted)
		})
		values := h.runtime.TranscodeBitrates()
		labels := make([]string, len(values))
		chosen := "自动"
		for i, v := range values {
			labels[i] = "自动"
			if v > 0 {
				labels[i] = fmt.Sprintf("%d kbps", v)
			}
			if v == bitrate {
				chosen = labels[i]
			}
		}
		if ui.Select(c.Key("fnmusic-transcode-bitrate"), &chosen, labels).Label("转码码率").Width(150).Disabled(mode == appplatform.TranscodeNative).Changed() {
			for i, label := range labels {
				if label == chosen {
					h.runtime.SetTranscode(appplatform.TranscodeSettings{Mode: mode, Bitrate: values[i]})
					break
				}
			}
		}
	})
}
