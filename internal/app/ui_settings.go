package app

import (
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/fnmusic"
)

// openSettings shows the settings window, opening it the first time.
func (a *app) openSettings() {
	if a.settingsWin != nil {
		a.settingsWin.Show()
		a.settingsWin.Focus()
		a.measureCache()
		return
	}
	a.measureCache()
	a.settingsWin = mygo.NewWindow(mygo.WindowOptions{
		Title:           "设置",
		Width:           560,
		Height:          720,
		MinWidth:        480,
		MinHeight:       420,
		StateKey:        "settings",
		DisableMaximize: true,
		DisableMinimize: true,
		// The app's menu has nothing for this window: on Linux and
		// Windows it stays out of sight until Alt shows it.
		AutoHideMenuBar: true,
		BackgroundColor: "light-dark(#f4f4f6, #1c1c1f)",
		Content:         ui.View(a.settingsView),
	})
	a.settingsWin.OnClosed(func() {
		a.settingsWin = nil
		a.syncDock()
	})
	a.syncDock()
}

// changed applies the settings everywhere and keeps them.
func (a *app) changed() {
	a.settings.save()
	a.settings.applyAppearance()
	if a.notch != nil {
		a.notch.setEnabled(a.settings.Notch)
	}
	a.redraw()
}

func (a *app) settingsView(c *ui.Context) {
	t := *c.Theme()
	p := paletteFor(&t)
	t.Accent, t.AccentText = p.accent, p.onAccent
	c.SetTheme(&t)
	c.Root().Background(p.sidebar)
	c.OnShortcut(ui.Cmd, ui.KeyW, func() { a.settingsWin.Close() })
	s := &a.settings

	ui.Scroll(c).Fill().Padding(24, 28, 32, 28).Gap(8).Children(func() {
		group := func(title string, rows func()) {
			ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(p.muted).Margin(14, 4, 4, 4)
			ui.Column(c).Radius(12).Background(p.bg).Border(1, p.hairline).Dividers(1, p.hairline).Children(rows)
		}
		row := func(label, desc string, control func()) {
			ui.Row(c).Padding(12, 16).Gap(16).AlignItems(ui.Center).MinHeight(48).Children(func() {
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, label).FontSize(13.5).FontWeight(500)
					if desc != "" {
						ui.Text(c, desc).FontSize(12).TextColor(p.muted).LineHeight(1.4)
					}
				})
				control()
			})
		}

		// Only on macOS: elsewhere the icon is always in the notification
		// area.
		if runtime.GOOS == "darwin" {
			group("通用", func() {
				row("应用图标", "显示在菜单栏时，关闭窗口后可从菜单栏图标打开", func() {
					labels := make([]string, len(appIcons))
					chosen := appIcons[0].label
					for i, ic := range appIcons {
						labels[i] = ic.label
						if ic.id == s.AppIcon {
							chosen = ic.label
						}
					}
					if ui.Select(c.Key("app-icon"), &chosen, labels).Label("应用图标").Width(150).Changed() {
						for _, ic := range appIcons {
							if ic.label == chosen {
								s.AppIcon = ic.id
							}
						}
						a.changed()
						a.changeAppIcon()
					}
				})
			})
		}

		// Only on macOS, the one with a notch player.
		if a.notch != nil {
			group("刘海播放器", func() {
				row("显示刘海播放器", "在屏幕顶部的刘海处显示迷你播放器，鼠标悬停即可展开", func() {
					if ui.Switch(c.Key("notch"), &s.Notch).Label("显示刘海播放器").Changed() {
						a.changed()
					}
				})
				row("播放时显示实时活动", "在刘海两侧显示封面与音频波形，可能遮挡刘海旁的菜单栏图标", func() {
					if ui.Switch(c.Key("activity"), &s.NotchActivity).Label("播放时显示实时活动").Disabled(!s.Notch).Changed() {
						a.changed()
					}
				})
				row("在无刘海的屏幕上常驻显示", "关闭时，将鼠标移至屏幕顶部中央即可唤出，不会遮挡菜单栏", func() {
					if ui.Switch(c.Key("virtual"), &s.NotchVirtual).Label("在无刘海的屏幕上常驻显示").Disabled(!s.Notch).Changed() {
						a.changed()
					}
				})
			})
		}

		group("播放", func() {
			row("音质", "仅网易云音乐：无损与 Hi-Res 需要黑胶 VIP，没有时播放所能播放的最高音质；下一首歌曲开始生效。飞牛音乐按文件原样播放", func() {
				labels := make([]string, len(qualityLevels))
				chosen := qualityLevels[2].label
				for i, q := range qualityLevels {
					labels[i] = q.label
					if q.level == s.Quality {
						chosen = q.label
					}
				}
				if ui.Select(c.Key("quality"), &chosen, labels).Label("音质").Width(150).Changed() {
					for _, q := range qualityLevels {
						if q.label == chosen {
							s.Quality = q.level
						}
					}
					a.changed()
				}
			})
			row("间奏时显示热评", "前奏、间奏与尾奏时，歌词处换成这首歌的热门评论", func() {
				if ui.Switch(c.Key("hot-comments"), &s.HotComments).Label("间奏时显示热评").Changed() {
					a.changed()
				}
			})
			row("音量均衡", "按网易云音乐测得的响度调整每首歌的音量，让歌曲听起来一样响", func() {
				if ui.Switch(c.Key("normalize"), &s.Normalize).Label("音量均衡").Changed() {
					a.pl.SetVolume(a.volume)
					a.changed()
				}
			})
		})

		if a.captionsShown() {
			group("播客", func() { a.captionSettings(row, c) })
		}

		group("缓存", func() {
			row("歌曲缓存", "听过的歌曲保存在本机，再次播放无需等待，也不耗流量；"+a.cacheUsage(), func() {
				labels := make([]string, len(cacheLimits))
				chosen := cacheLimits[2].label
				for i, l := range cacheLimits {
					labels[i] = l.label
					if l.mb == s.CacheMB {
						chosen = l.label
					}
				}
				if ui.Select(c.Key("cache"), &chosen, labels).Label("歌曲缓存").Width(150).Changed() {
					for _, l := range cacheLimits {
						if l.label == chosen {
							s.CacheMB = l.mb
						}
					}
					a.changed()
					a.applyCacheLimit()
				}
			})
			row("清除缓存", "删除保存在本机的全部歌曲", func() {
				if ui.Button(c.Key("clear-cache"), "清除").Disabled(a.cacheUsed <= 0).Clicked() {
					a.clearCache()
				}
			})
		})

		group("外观", func() {
			row("外观", "", func() {
				modes := []string{"system", "light", "dark"}
				i := 0
				for k, m := range modes {
					if m == s.Appearance {
						i = k
					}
				}
				if segmented(c.Key("appearance"), &i, "跟随系统", "浅色", "深色") {
					s.Appearance = modes[i]
					a.changed()
				}
			})
		})

		group("实验性功能", func() {
			row("进度条烟雾特效", "播放时进度条化作拖着烟雾的光点，随音乐起伏；会多用一些电量", func() {
				if ui.Switch(c.Key("smoke"), &s.Smoke).Label("进度条烟雾特效").Changed() {
					a.changed()
				}
			})
			row("烟雾背景", "播放页的背景化作封面色调的烟雾，缓缓升起，随低音起伏；仍在打磨，会多用一些电量", func() {
				if ui.Switch(c.Key("fog"), &s.Fog).Label("烟雾背景").Changed() {
					a.changed()
				}
			})
		})

		group("账号", func() {
			if a.profile == nil {
				row("未登录", "登录后，每日推荐与歌单将根据你的口味生成", func() {
					if ui.PrimaryButton(c.Key("login"), "扫码登录").Clicked() {
						a.showMain()
						a.openLogin()
						a.win.Invalidate()
					}
				})
				return
			}
			prof := a.profile
			ui.Row(c).Padding(12, 16).Gap(12).AlignItems(ui.Center).Children(func() {
				a.avatar(c, prof.AvatarURL, 36, p.press)
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, prof.Nickname).FontSize(13.5).FontWeight(600)
					ui.Text(c, "已登录网易云音乐").FontSize(12).TextColor(p.muted)
				})
				if ui.Button(c.Key("logout"), "退出登录").Clicked() {
					a.signOut()
				}
			})
		})

		group("飞牛音乐", func() {
			if a.fn.client == nil {
				row("未连接", "连接后可播放飞牛 NAS 上的音乐库、歌单与喜欢的歌曲", func() {
					if ui.PrimaryButton(c.Key("fn-connect"), "连接…").Clicked() {
						a.showMain()
						a.openFnConnect()
						a.win.Invalidate()
					}
				})
				return
			}
			cfg := a.fn.client.Config()
			status := "已连接 · " + cfg.Address
			if cfg.Mode == fnmusic.ModeFNConnect {
				status = "已连接 · FN ID " + cfg.Address
				if r := a.fn.client.Route(); r != "" {
					status += "（" + r + "）"
				}
			}
			if a.fn.err != "" {
				status = a.fn.err
			}
			row(cfg.Username, status, func() {
				if ui.Button(c.Key("fn-reconnect"), "重新连接…").Clicked() {
					a.showMain()
					a.openFnConnect()
					a.win.Invalidate()
				}
				if ui.Button(c.Key("fn-disconnect"), "断开").Clicked() {
					a.disconnectFn()
				}
			})
		})

		if a.updates {
			group("更新", func() {
				desc := "每天检查一次是否有新版本"
				if t := lastUpdateCheck(); !t.IsZero() {
					desc += "；上次检查：" + commentTime(t, time.Now())
				}
				row("自动检查更新", desc, func() {
					on := updateChecks()
					if ui.Switch(c.Key("update-checks"), &on).Label("自动检查更新").Changed() {
						setUpdateChecks(on)
					}
				})
				row("自动下载并安装", "在后台下载新版本，下次打开时生效", func() {
					on := updateDownloads()
					if ui.Switch(c.Key("update-downloads"), &on).Label("自动下载并安装").Disabled(!updateChecks()).Changed() {
						setUpdateDownloads(on)
					}
				})
				row("当前版本 "+mygo.App.Version(), "", func() {
					if ui.Button(c.Key("check-updates"), "检查更新").Clicked() {
						checkForUpdates()
					}
				})
			})
		}
	})
}
