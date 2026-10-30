package app

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/lyrics"
)

// The translations and the romaji of the lyrics are the song's own: some
// have either, some both, most neither. The toggle of them shows while Now
// Playing shows the lyrics, and only where the song playing has something
// to show.

// lyricsOptionsRow is where the lyrics' toggle sits: along the top, level
// with the tabs, at the right edge of the view, where they move neither
// the tabs nor the stage's styles.
func lyricsOptionsRow(c *ui.Context, w float32) ui.Element {
	return ui.Row(c.Key("lyrics-options")).Absolute().Top(topBar(c)).Right(npSideX(w)).Height(npTabsHeight).AlignItems(ui.Center)
}

// lyricRecenterFrames is how many frames the lyrics follow the line sung
// for after what they show of them changes: the lines under it take a frame
// or two to place at their new height, and the lyrics a few more to glide
// back.
const lyricRecenterFrames = 20

// lyricsChoices toggles what the lyrics show under each line, of what the
// song has: the lyrics alone, their translation, or their romaji.
func (a *app) lyricsChoices(c *ui.Context) {
	lines, _ := a.pl.Lyrics()
	opts := lyricOptions(lines)
	if len(opts) < 2 {
		// The lyrics alone, which is all there is to show of this song.
		return
	}
	now := a.lyricOption(lines)
	next := opts[(max(slices.Index(opts, now), -1)+1)%len(opts)]
	b := ui.ButtonBase(c.Key("lyrics-choices")).Padding(6, 10, 6, 12).Radius(999).Gap(6).AlignItems(ui.Center).
		FocusRing(false).Label("歌词选项").Tooltip("切换歌词显示")
	bg, fg := ui.Transparent, white.Alpha(0.55)
	if b.Hovered() {
		bg, fg = white.Alpha(0.08), white.Alpha(0.85)
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Children(func() {
		ui.Icon(c, iconLyrics).Size(14, 14).TextColor(fg)
		ui.Text(c, lyricLabel(now)).FontSize(12.5).FontWeight(600).TextColor(fg)
	})
	if b.Clicked() {
		a.chooseLyricsText(next)
	}
}

// chooseLyricsText shows kind under the lyrics. Another kind of line is
// another height, which moves the lines under the one sung: the lyrics find
// it again, unless the listener scrolled away from it, whose place is
// theirs.
func (a *app) chooseLyricsText(kind string) {
	a.settings.LyricsText = kind
	a.settings.save()
	if a.lyricFocused {
		a.lyricRecenter, a.lyricRecenterMax = lyricRecenterFrames, a.lyricScroll.MaxY
	}
	a.redraw()
}

// lyricOption is what shows under the lyrics now: the kind chosen, or the
// other where the song has none of that one, and nothing where it has
// neither, or where the listener chose the lyrics alone.
func (a *app) lyricOption(lines []lyrics.Line) string {
	chosen := a.settings.lyricsText()
	if chosen == lyricOnly {
		return lyricOnly
	}
	has := func(kind string) bool {
		if kind == lyricRoma {
			return lyrics.HasRoma(lines)
		}
		return lyrics.HasTranslation(lines)
	}
	if has(chosen) {
		return chosen
	}
	other := lyricTranslation
	if chosen == lyricTranslation {
		other = lyricRoma
	}
	if has(other) {
		return other
	}
	return lyricOnly
}

// lyricOptions are the kinds of line the song playing may show, in the
// order the toggle goes through them.
func lyricOptions(lines []lyrics.Line) []string {
	opts := []string{lyricOnly}
	if lyrics.HasTranslation(lines) {
		opts = append(opts, lyricTranslation)
	}
	if lyrics.HasRoma(lines) {
		opts = append(opts, lyricRoma)
	}
	return opts
}

// lyricLabel names a kind of line, as the toggle says it.
func lyricLabel(kind string) string {
	for _, t := range lyricsTexts {
		if t.id == kind {
			return t.label
		}
	}
	return ""
}

// underLine is what shows under a line of lyrics, and which kind it is:
// nothing where the listener chose the lyrics alone, else the kind they
// chose, its translation or its romaji, or the other where the line has
// none of that kind, so that it says something in either, and nothing
// where it has neither.
func (a *app) underLine(l *lyrics.Line) (text, kind string) {
	switch a.settings.lyricsText() {
	case lyricOnly:
		return "", ""
	case lyricRoma:
		if l.Roma != "" {
			return l.Roma, lyricRoma
		}
		return l.Translation, lyricTranslation
	}
	if l.Translation != "" {
		return l.Translation, lyricTranslation
	}
	return l.Roma, lyricRoma
}
