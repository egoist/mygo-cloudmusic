package app

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
)

// lyricApp plays a song whose lines have the romaji and the translation
// given, and shows its lyrics.
func lyricApp(t *testing.T, translation, roma string) (*app, *ui.Tester) {
	t.Helper()
	a, tt, _ := playingApp()
	a.nowPlaying, a.npPane = true, paneLyrics
	lines := lyrics.Build("[00:01.00]ラララ\n[00:11.00]ルルル\n", translation, roma, "")
	a.pl.SetLyrics(lines)
	a.pl.Seek(2 * time.Second)
	tt.Frame()
	return a, tt
}

// The lyrics' toggle starts on the translation, and a click on it goes
// through the lyrics alone, then the romaji, then back, showing what it
// says it does.
func TestLyricsToggle(t *testing.T) {
	a, tt := lyricApp(t, "[00:01.00]啦啦啦\n[00:11.00]噜噜噜", "[00:01.00]ra ra ra\n[00:11.00]ru ru ru")
	if !tt.HasText("啦啦啦") || a.settings.LyricsText != lyricTranslation {
		t.Fatalf("at first: the translation shows %v, the choice %q", tt.HasText("啦啦啦"), a.settings.LyricsText)
	}
	for _, c := range []struct {
		choice              string
		translation, romaji bool
	}{
		{lyricRoma, false, true},
		{lyricOnly, false, false},
		{lyricTranslation, true, false},
	} {
		if err := tt.Click("歌词选项"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.settings.LyricsText != c.choice {
			t.Fatalf("the toggle went to %q, want %q", a.settings.LyricsText, c.choice)
		}
		if translation, romaji := tt.HasText("啦啦啦"), tt.HasText("ra ra ra"); translation != c.translation || romaji != c.romaji {
			t.Errorf("%q: translation %v, romaji %v", c.choice, translation, romaji)
		}
	}
}

// The toggle goes through what the song has, and shows nothing where it has
// neither a translation nor romaji.
func TestLyricsToggleAvailable(t *testing.T) {
	a, tt := lyricApp(t, "[00:01.00]啦啦啦\n", "")
	for _, want := range []string{lyricOnly, lyricTranslation} {
		if err := tt.Click("歌词选项"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.settings.LyricsText != want {
			t.Fatalf("a song with a translation only: chose %q, want %q", a.settings.LyricsText, want)
		}
	}
	// The romaji, where the song has that alone: it shows in place of the
	// translation chosen, and the toggle goes on from there.
	a.pl.SetLyrics(lyrics.Build("[00:01.00]ラララ\n", "", "[00:01.00]ra ra ra", ""))
	tt.Frame()
	if !tt.HasText("ra ra ra") {
		t.Fatalf("a song with romaji only shows none of it: %q", tt.Texts())
	}
	for _, want := range []string{lyricOnly, lyricRoma} {
		if err := tt.Click("歌词选项"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.settings.LyricsText != want {
			t.Fatalf("a song with romaji only: chose %q, want %q", a.settings.LyricsText, want)
		}
	}
	a.pl.SetLyrics(lyrics.Build("[00:01.00]ラララ\n", "", "", ""))
	tt.Frame()
	if err := tt.Click("歌词选项"); err == nil {
		t.Error("a song with neither a translation nor romaji still has a toggle")
	}
}

// A line shows the kind of second line chosen, and falls back to the
// other where it has none of it, showing nothing where it has neither.
func TestUnderLine(t *testing.T) {
	both := lyrics.Line{Text: "ラララ", Translation: "啦啦啦", Roma: "ra ra ra"}
	translation := lyrics.Line{Text: "ラララ", Translation: "啦啦啦"}
	roma := lyrics.Line{Text: "ラララ", Roma: "ra ra ra"}
	bare := lyrics.Line{Text: "ラララ"}
	for _, c := range []struct {
		chosen, text, kind string
		line               lyrics.Line
	}{
		{lyricTranslation, "啦啦啦", lyricTranslation, both},
		{lyricRoma, "ra ra ra", lyricRoma, both},
		// The other where the line has none of the kind chosen.
		{lyricTranslation, "ra ra ra", lyricRoma, roma},
		{lyricRoma, "啦啦啦", lyricTranslation, translation},
		// And nothing where it has neither.
		{lyricTranslation, "", "", bare},
		{lyricRoma, "", "", bare},
		// The lyrics alone, though it has both.
		{lyricOnly, "", "", both},
	} {
		a := newTestApp()
		a.settings = defaultSettings()
		a.settings.LyricsText = c.chosen
		if text, kind := a.underLine(&c.line); text != c.text || text != "" && kind != c.kind {
			t.Errorf("%s of %+v: %q of kind %q, want %q of kind %q", c.chosen, c.line, text, kind, c.text, c.kind)
		}
	}
}

// lyricLines are lines a second apart, each with a translation long enough
// to wrap, and a romaji short enough not to, so that what the lyrics show
// changes how tall they are.
func lyricLines(n int) []lyrics.Line {
	out := make([]lyrics.Line, n)
	for i := range out {
		at := time.Duration(i) * time.Second
		out[i] = lyrics.Line{
			At:          at,
			End:         at + 900*time.Millisecond,
			Text:        fmt.Sprintf("ラララ %d", i),
			Translation: fmt.Sprintf("%d 这是一句很长很长的翻译，长到折成两行才放得下，好让歌词的高度随它变化", i),
			Roma:        fmt.Sprintf("ra ra ra %d", i),
		}
	}
	return out
}

// lyricSong plays a song of twenty such lines, showing the tenth, with the
// lyrics settled on it.
func lyricSong(t *testing.T) (*app, *ui.Tester, string) {
	t.Helper()
	a, tt, _ := playingApp()
	lines := lyricLines(20)
	a.pl.SetLyrics(lines)
	a.pl.Seek(10*time.Second + 100*time.Millisecond)
	a.nowPlaying, a.npPane = true, paneLyrics
	settle(tt)
	return a, tt, lines[10].Text
}

// settle runs the frames the lyrics glide and the scrolls take to come to
// rest.
func settle(tt *ui.Tester) {
	for range 120 {
		tt.Advance(16 * time.Millisecond)
		tt.Frame()
	}
}

// Changing what the lyrics show under each line brings the line sung back
// to where the lyrics put it, though the lines are of another height now:
// the offset follows them, rather than leaving the line where they were.
func TestLyricsChoiceRecenters(t *testing.T) {
	a, tt, line := lyricSong(t)
	if !a.lyricFocused {
		t.Fatalf("the lyrics do not follow the line sung: %+v", a.lyricScroll)
	}
	was := a.lyricScroll.Y
	if err := tt.Click("歌词选项"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if found := a.lyricScroll.Y; found == was {
		t.Errorf("the lyrics stayed at %.0f, where the shorter lines left them", found)
	}
	if !a.lyricFocused {
		t.Errorf("the line sung shows %+v, not where the lyrics put it: %+v", mustFind(tt, line), a.lyricScroll)
	}
	if after := mustFind(tt, line); after.H == 0 {
		t.Errorf("the line sung is gone from the view: %q", tt.Texts())
	}
}

// A listener who scrolled away from the line sung keeps their place as
// what the lyrics show changes.
func TestLyricsChoiceKeepsScrolledPlace(t *testing.T) {
	a, tt, line := lyricSong(t)
	// Up the lyrics, towards the start of the song, as a touchpad does.
	tt.Wheel(900, 400, 0, -400)
	settle(tt)
	if a.lyricFocused {
		t.Fatalf("scrolled away, the lyrics still show the line sung where they put it: %+v, %+v", a.lyricScroll, mustFind(tt, line))
	}
	scrolled := a.lyricScroll.Y

	if err := tt.Click("歌词选项"); err != nil {
		t.Fatal(err)
	}
	settle(tt)
	if a.settings.LyricsText != lyricRoma {
		t.Fatalf("the choice did not change: %q", a.settings.LyricsText)
	}
	if d := math.Abs(float64(a.lyricScroll.Y - scrolled)); d > 8 {
		t.Errorf("the lyrics jumped %.0f DIPs from where the listener left them", d)
	}
}

func mustFind(tt *ui.Tester, label string) ui.Rect {
	r, _ := tt.Find(label)
	return r
}
