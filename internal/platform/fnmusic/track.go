package fnmusic

import (
	"encoding/json"
	"strings"

	"netease-music-mygo/internal/netease"
)

// names reads what the service gives for people or albums: a list of
// objects with names, or of plain names.
type names []string

func (n *names) UnmarshalJSON(b []byte) error {
	var objs []struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(b, &objs) == nil {
		for _, o := range objs {
			if s := strings.TrimSpace(o.Name); s != "" {
				*n = append(*n, s)
			}
		}
		return nil
	}
	var strs []string
	if json.Unmarshal(b, &strs) == nil {
		for _, s := range strs {
			if s = strings.TrimSpace(s); s != "" {
				*n = append(*n, s)
			}
		}
	}
	return nil
}

// album is an album as an object, or as its name.
type album struct {
	GUID    string `json:"guid"`
	Name    string `json:"name"`
	CoverID string `json:"coverId"`
}

func (a *album) UnmarshalJSON(b []byte) error {
	var name string
	if json.Unmarshal(b, &name) == nil {
		a.Name = name
		return nil
	}
	type plain album
	return json.Unmarshal(b, (*plain)(a))
}

// rawTrack is a song as the service lists it.
type rawTrack struct {
	GUID     string `json:"guid"`
	Title    string `json:"title"`
	Filename string `json:"filename"`
	CoverID  string `json:"coverId"`
	Album    album  `json:"album"`
	Artists  names  `json:"artists"`
	Duration int64  `json:"duration"`
	IsCue    bool   `json:"isCue"`
	// StartTime and EndTime, in seconds, say where in its file a track of a
	// CUE sheet is.
	StartTime    float64 `json:"startTime"`
	EndTime      float64 `json:"endTime"`
	CueStartTime float64 `json:"cueStartTime"`
	CueEndTime   float64 `json:"cueEndTime"`
	TrackNo      int     `json:"trackNo"`
	DiscNo       int     `json:"discNo"`
	Favorite     bool    `json:"isFavorite"`
	// Format and Path are where a file's kind shows, if its audio spec
	// does not say.
	Format    string `json:"format"`
	Extension string `json:"extension"`
	Path      string `json:"path"`
	AudioSpec struct {
		Duration  int64  `json:"duration"` // milliseconds
		Format    string `json:"format"`
		Extension string `json:"extension"`
		Container string `json:"container"`
		Codec     string `json:"codec"`
		Path      string `json:"path"`
	} `json:"audioSpec"`
}

// format names the kind of file a song is, by its extension: mp3, flac,
// m4a and so on, or "" if the service does not say.
func (r *rawTrack) format() string {
	spec := &r.AudioSpec
	for _, s := range []string{
		spec.Extension, spec.Format, spec.Container, spec.Codec,
		r.Extension, r.Format, ext(spec.Path), ext(r.Path),
	} {
		if f := normalizeFormat(s); f != "" {
			return f
		}
	}
	return ""
}

func ext(path string) string {
	if i := strings.LastIndex(path, "."); i >= 0 {
		return path[i+1:]
	}
	return ""
}

// normalizeFormat names a kind of file as its extension is, from what
// the service calls it.
func normalizeFormat(s string) string {
	s = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), ".")))
	switch s {
	case "", "audio", "cue", "cue-track":
		return ""
	case "mpeg", "audio/mpeg", "audio/mp3":
		return "mp3"
	case "mp4a", "audio/mp4", "audio/m4a":
		return "m4a"
	case "audio/flac", "x-flac":
		return "flac"
	case "vorbis", "audio/ogg":
		return "ogg"
	case "wave", "audio/wav", "audio/x-wav":
		return "wav"
	}
	return s
}

// track makes a song of the app's from one the service lists, or false for
// one with no GUID.
func (r *rawTrack) track() (netease.Track, bool) {
	if r.GUID == "" {
		return netease.Track{}, false
	}
	title := strings.TrimSpace(r.Title)
	if title == "" {
		title = strings.TrimSpace(r.Filename)
	}
	if title == "" {
		title = "未知歌曲"
	}
	ms := r.AudioSpec.Duration
	if ms == 0 {
		ms = r.Duration
	}
	// A track of a CUE sheet is as long as its own part of the file, which
	// its own duration says; the audio spec's is of the whole file. Failing
	// that, where it starts and ends in the file does.
	if r.IsCue {
		from, to := max(r.StartTime, r.CueStartTime), max(r.EndTime, r.CueEndTime)
		switch {
		case r.Duration > 0:
			ms = r.Duration
		case to > from:
			ms = int64((to - from) * 1000)
		}
	}
	cover := r.CoverID
	if cover == "" {
		cover = r.Album.CoverID
	}
	format := r.format()
	t := netease.Track{
		ID:       ID(r.GUID),
		Name:     title,
		Album:    netease.Album{Name: r.Album.Name, PicURL: coverURL(cover)},
		DiscNo:   r.DiscNo,
		TrackNo:  r.TrackNo,
		Cue:      r.IsCue,
		Fav:      r.Favorite,
		Duration: ms,
		Source:   Source,
		Ref:      r.GUID,
		Format:   format,
	}
	if r.Album.GUID != "" {
		// Tracks of an album are told by its GUID, not by a name that
		// several albums share.
		t.Album.ID = ID(r.Album.GUID)
	}
	for _, name := range r.Artists {
		t.Artists = append(t.Artists, netease.Artist{Name: name})
	}
	// What the player cannot decode, the service transcodes (Transcoded).
	t.Privilege = &netease.Privilege{ID: t.ID, Pl: 1}
	return t, true
}
