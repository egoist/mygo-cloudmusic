package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/netease"
)

// Desktop lyrics recover to the primary display when their saved monitor is disconnected.
func TestDesktopLyricsPositionRecoversDisconnectedDisplay(t *testing.T) {
	displays := []mygo.Display{
		{Primary: true, WorkArea: mygo.Rectangle{X: 0, Y: 0, Width: 1920, Height: 1040}},
		{WorkArea: mygo.Rectangle{X: 1920, Y: 0, Width: 2560, Height: 1400}},
	}
	r := desktopLyricsPosition(mygo.Rectangle{X: 2200, Y: 900, Width: desktopLyricsWidth, Height: desktopLyricsHeight}, displays[:1])
	if r.X != 510 || r.Y != 856 {
		t.Fatalf("disconnected display position became %+v, want primary bottom center", r)
	}
}

// Desktop lyrics keep valid negative coordinates for a display left of the primary one.
func TestDesktopLyricsPositionKeepsNegativeDisplayCoordinates(t *testing.T) {
	displays := []mygo.Display{
		{Primary: true, WorkArea: mygo.Rectangle{X: 0, Y: 0, Width: 1920, Height: 1040}},
		{WorkArea: mygo.Rectangle{X: -1920, Y: 0, Width: 1920, Height: 1080}},
	}
	want := mygo.Rectangle{X: -1800, Y: 100, Width: desktopLyricsWidth, Height: desktopLyricsHeight}
	if got := desktopLyricsPosition(want, displays); got != want {
		t.Fatalf("valid negative display position became %+v, want %+v", got, want)
	}
}

// Desktop lyrics clamp a partially visible saved position back into its display work area.
func TestDesktopLyricsPositionClampsPartialWindow(t *testing.T) {
	displays := []mygo.Display{{Primary: true, WorkArea: mygo.Rectangle{X: 0, Y: 0, Width: 1920, Height: 1040}}}
	r := desktopLyricsPosition(mygo.Rectangle{X: 1800, Y: 900, Width: desktopLyricsWidth, Height: desktopLyricsHeight}, displays)
	if r.X != 1020 || r.Y != 880 {
		t.Fatalf("partially visible position became %+v, want clamped position", r)
	}
}

// Desktop lyric shadows preserve every spaced word run from the foreground.
func TestDesktopLyricsShadowKeepsSpacedRuns(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	words := []lyrics.Word{
		{Text: "hello ", Start: 0, End: time.Second},
		{Text: "world ", Start: time.Second, End: 2 * time.Second},
		{Text: "again", Start: 2 * time.Second, End: 3 * time.Second},
	}
	foreground := a.desktopLyricsWordSpans(words, 1500*time.Millisecond, 30)
	shadow := desktopLyricsShadowSpans(foreground)
	if len(shadow) != len(foreground) {
		t.Fatalf("shadow spans %d, foreground spans %d", len(shadow), len(foreground))
	}
	for i := range foreground {
		if shadow[i].Text != foreground[i].Text {
			t.Fatalf("shadow run %d text %q, foreground %q", i, shadow[i].Text, foreground[i].Text)
		}
		if shadow[i].Size != foreground[i].Size || shadow[i].Weight != foreground[i].Weight {
			t.Fatalf("shadow run %d layout style differs: %+v vs %+v", i, shadow[i], foreground[i])
		}
	}
}

// Desktop lyrics keep the translation in its own row instead of painting it over the original.
func TestDesktopLyricsTranslationHasSeparateRow(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.desktopLyricsShowTranslation = true
	a.desktopLyricsFontSize = 30
	a.pl.Cue([]netease.Track{{ID: 7, Name: "Foreign song", Duration: 180000}})
	a.pl.SetLyrics(lyrics.Build("[00:00.00]hello world", "[00:00.00]你好 世界", ""))

	tt := ui.NewTester(a.desktopLyricsView, desktopLyricsWidth, desktopLyricsHeight)
	tt.Frame()
	original, ok := tt.Find("桌面歌词原文")
	if !ok {
		t.Fatal("desktop lyric original was not laid out")
	}
	translation, ok := tt.Find("桌面歌词译文")
	if !ok {
		t.Fatal("desktop lyric translation was not laid out")
	}
	if original.Y+original.H > translation.Y {
		t.Fatalf("original row %v overlaps translation row %v", original, translation)
	}
}

// Desktop lyrics keep style, lock state, and the dragged position in settings across runs.
func TestDesktopLyricsSettingsRememberStyle(t *testing.T) {
	a := newTestApp()
	a.settings = defaultSettings()
	a.settings.path = filepath.Join(t.TempDir(), "settings.json")
	a.desktopLyricsLocked = true
	a.desktopLyricsShowTranslation = false
	a.desktopLyricsFontSize = 36
	a.desktopLyricsColor = 5
	prefs := defaultDesktopLyricsSettings()
	prefs.PositionSet, prefs.X, prefs.Y = true, 123, 456
	a.settings.DesktopLyrics = &prefs
	a.saveDesktopLyricsState()

	b, err := os.ReadFile(a.settings.path)
	if err != nil {
		t.Fatal(err)
	}
	var got settings
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.DesktopLyrics == nil {
		t.Fatal("desktop lyric settings were not saved")
	}
	if got.DesktopLyrics.Locked != true || got.DesktopLyrics.ShowTranslation || got.DesktopLyrics.FontSize != 36 || got.DesktopLyrics.Color != 5 {
		t.Fatalf("desktop lyric style was not saved: %+v", *got.DesktopLyrics)
	}
	if !got.DesktopLyrics.PositionSet || got.DesktopLyrics.X != 123 || got.DesktopLyrics.Y != 456 {
		t.Fatalf("desktop lyric position was not saved: %+v", *got.DesktopLyrics)
	}
}
