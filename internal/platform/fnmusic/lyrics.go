package fnmusic

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// lyricList is the lyrics the service has for a song: several, from
// the file itself, from LRC files beside it, or typed in.
type lyricList struct {
	List []struct {
		GUID      string  `json:"guid"`
		Content   string  `json:"content"`
		Source    int     `json:"source"` // 1 the file's, 2 an LRC file, 4 typed in
		Offset    float64 `json:"offset"` // milliseconds, positive to show lines sooner
		UpdatedAt float64 `json:"updatedAt"`
		CreatedAt float64 `json:"createdAt"`
	} `json:"list"`
	Preferred string `json:"preferred"`
}

var (
	stamp     = regexp.MustCompile(`[\[［](\d{1,3}):(\d{2})(?:[.:](\d{1,3}))?[\]］]`)
	leading   = regexp.MustCompile(`^(?:\s*[\[［]\d{1,3}:\d{2}(?:[.:]\d{1,3})?[\]］])+`)
	offsetTag = regexp.MustCompile(`(?i)^\s*\[\s*offset\s*:\s*([+-]?\d+)\s*\]\s*$`)
)

// synced counts the lines that are timed and say something.
func synced(content string) int {
	n := 0
	for _, line := range strings.Split(content, "\n") {
		if loc := leading.FindStringIndex(line); loc != nil {
			rest := stamp.ReplaceAllString(line[loc[1]:], "")
			if strings.TrimSpace(rest) != "" {
				n++
			}
		}
	}
	return n
}

// usable is false for lyrics that are timed, and yet no line reads: a
// broken file.
func usable(content string) bool {
	return !stamp.MatchString(content) || synced(content) > 0
}

// priority ranks where lyrics came from: the file's own first.
func priority(source int) int {
	switch source {
	case 1:
		return 4
	case 2:
		return 3
	case 4:
		return 2
	}
	return 1
}

// chooseLyrics picks what to show as the web player does: the one it
// prefers if it reads; else the best of the timed ones, by where it is
// from and how new it is; else the best untimed one.
func chooseLyrics(l lyricList) string {
	type cand struct {
		content string
		guid    string
		source  int
		offset  int
		updated float64
		created float64
	}
	var cands []cand
	for _, c := range l.List {
		if strings.TrimSpace(c.Content) == "" {
			continue
		}
		cands = append(cands, cand{c.Content, c.GUID, priority(c.Source), int(c.Offset), c.UpdatedAt, c.CreatedAt})
	}
	slices.SortStableFunc(cands, func(a, b cand) int {
		switch {
		case a.source != b.source:
			return b.source - a.source
		case a.updated != b.updated:
			if a.updated > b.updated {
				return -1
			}
			return 1
		case a.created != b.created:
			if a.created > b.created {
				return -1
			}
			return 1
		}
		return 0
	})
	build := func(c cand) string { return shift(c.content, c.offset) }
	if l.Preferred != "" {
		for _, c := range cands {
			if c.guid == l.Preferred && usable(c.content) {
				return build(c)
			}
		}
	}
	var plain *cand
	for i, c := range cands {
		if !stamp.MatchString(c.content) {
			if plain == nil {
				plain = &cands[i]
			}
			continue
		}
		if synced(c.content) > 0 {
			return build(c)
		}
	}
	if plain != nil {
		return build(*plain)
	}
	return ""
}

// shift moves the lines of LRC by what the listener adjusted them: the
// service's offset, which shows lines sooner when positive, less the
// lines' own, which shows them later.
func shift(content string, offset int) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	own := 0
	kept := lines[:0:0]
	for _, line := range lines {
		if m := offsetTag.FindStringSubmatch(line); m != nil {
			own, _ = strconv.Atoi(m[1])
			continue
		}
		kept = append(kept, line)
	}
	// A timestamp moves later by the lines' own offset, sooner by the
	// service's.
	delta := own - offset
	if delta == 0 {
		return strings.Join(kept, "\n")
	}
	for i, line := range kept {
		kept[i] = stamp.ReplaceAllStringFunc(line, func(s string) string {
			m := stamp.FindStringSubmatch(s)
			min, _ := strconv.Atoi(m[1])
			sec, _ := strconv.Atoi(m[2])
			ms := 0
			if m[3] != "" {
				ms, _ = strconv.Atoi(m[3])
				for k := len(m[3]); k < 3; k++ {
					ms *= 10
				}
			}
			t := max(0, (min*60+sec)*1000+ms+delta)
			return fmt.Sprintf("[%02d:%02d.%03d]", t/60000, t/1000%60, t%1000)
		})
	}
	return strings.Join(kept, "\n")
}
