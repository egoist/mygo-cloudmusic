package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The installed fonts leave out the @-prefixed vertical-writing aliases.
func TestSystemFontsHideVertical(t *testing.T) {
	for _, name := range systemFonts() {
		if name == "" || strings.HasPrefix(name, "@") {
			t.Fatalf("listed %q", name)
		}
	}
}

// The font row shows the system's default when no font is chosen, and a
// saved font no longer installed as chosen, the default still offered.
// (Choosing one applies the appearance, which needs the app running.)
func TestFontSetting(t *testing.T) {
	const gone = "No Such Font 字体"
	a := newTestApp()
	a.settings = defaultSettings()
	tt := ui.NewTester(a.settingsView, 560, 2000)
	tt.Frame()
	shows := func(s string) bool { return slices.Contains(tt.Texts(), s) }
	if !shows("系统默认") {
		t.Fatal("no default with no font chosen")
	}

	a.settings.Font = gone
	tt.Frame()
	if !shows(gone) || shows("系统默认") {
		t.Fatalf("texts %q with a font gone chosen", tt.Texts())
	}
	if err := tt.Click(gone); err != nil {
		t.Fatal(err)
	}
	if !shows("系统默认") {
		t.Fatal("the popup does not offer the default")
	}
}

// The chosen font is the family of the main window's, the settings
// window's and the notch's text.
func TestFontApplied(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.Font = " Microsoft YaHei UI "
	n := &notch{a: a, w0: 200, h0: 32, winW: notchOpenW + 60, winH: 240}
	for name, view := range map[string]func(*ui.Context){"main": a.view, "settings": a.settingsView, "notch": n.view} {
		var font string
		tt := ui.NewTester(func(c *ui.Context) {
			view(c)
			font = c.Theme().Font
		}, 560, 640)
		tt.Frame()
		if font != "Microsoft YaHei UI" {
			t.Errorf("%s: font %q", name, font)
		}
	}
}
