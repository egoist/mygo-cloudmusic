package netease

import (
	"fmt"
	"net/http"
	"strings"
)

// Shared model extension seam.
//
// Provider metadata is embedded into the upstream models so callers keep the
// existing Track, Playlist and Stream types while the original declarations
// only gain one extension field each. Keep these fields provider-neutral;
// protocol-specific models belong in the provider adapter.
type PlatformTrack struct {
	Source  string `json:"src,omitempty"`
	Ref     string `json:"ref,omitempty"`
	Format  string `json:"fmt,omitempty"`
	Cue     bool   `json:"cue,omitempty"`
	Fav     bool   `json:"fav,omitempty"`
	DiscNo  int    `json:"disc,omitempty"`
	TrackNo int    `json:"no,omitempty"`
}

type PlatformPlaylist struct {
	Ref  string `json:"-"`
	Kind string `json:"-"`
	Sub  string `json:"-"`
}

type PlatformStream struct {
	Header func() http.Header `json:"-"`
}

func usesNetEaseURL(t *Track) bool { return t == nil || t.Source == "" }

func platformSized(pic string, n int) (string, bool) {
	if pic == "" || strings.HasPrefix(pic, "http") {
		return "", false
	}
	return fmt.Sprintf("%s?size=%d", pic, n), true
}
