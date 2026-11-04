package app

import "github.com/egoist/mygo/ui"

type cacheDirectoryConfirmation struct {
	open      bool
	directory string // empty means the system default
	target    string // displayed path, including the system default
	err       string
}

// cacheDirectoryDialog confirms the change in the settings window's own theme.
func (a *app) cacheDirectoryDialog(c *ui.Context, p palette) {
	d := &a.cacheConfirm
	ui.DialogBase(c.Key("cache-directory-confirm"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
		panel.Width(420).Padding(24).Radius(18).Gap(12).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		// Keep the actions and validation error visible even with a long path.
		_, height := c.Size()
		detailsHeight := height - 136 // panel padding, gaps, actions and window margin
		if d.err != "" {
			detailsHeight -= 48
		}
		ui.Scroll(c.Key("details")).MaxHeight(detailsHeight).Gap(12).Children(func() {
			ui.Text(c, "更改歌曲缓存目录").FontSize(17).FontWeight(700).TextColor(p.text)
			ui.Text(c, "确认后立即生效，无需重启。").FontSize(13).TextColor(p.muted)
			ui.Column(c).Padding(12).Radius(10).Gap(5).Background(p.sidebar).Children(func() {
				ui.Text(c, "新目录").FontSize(11).FontWeight(600).TextColor(p.muted)
				ui.Text(c, d.target).FontSize(12.5).TextColor(p.text).LineHeight(1.45)
			})
			ui.Text(c, "迁移并更改：搬移已有的完整歌曲缓存。\n仅更改目录：保留旧文件，之后使用新目录。").FontSize(13).TextColor(p.text).LineHeight(1.5)
			ui.Text(c, "请选择空目录。更改期间暂时停止播放，完成后恢复进度；请勿同时运行其他应用实例。").FontSize(12).TextColor(p.muted).LineHeight(1.45)
		})
		if d.err != "" {
			ui.Text(c, d.err).FontSize(12).TextColor(p.accent).LineHeight(1.45).MaxLines(2).Tooltip(d.err)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Margin(4, 0, 0, 0).Children(func() {
			if pillButton(c.Key("cancel"), nil, "取消", p.hover.Over(p.bg), p.text).Clicked() {
				d.open = false
			}
			if pillButton(c.Key("switch"), nil, "仅更改目录", p.hover.Over(p.bg), p.text).Clicked() {
				a.submitCacheDirectory(false)
			}
			if pillButton(c.Key("migrate"), nil, "迁移并更改", p.accent, p.onAccent).Clicked() {
				a.submitCacheDirectory(true)
			}
		})
	})
}
