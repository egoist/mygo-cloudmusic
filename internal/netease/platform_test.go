package netease

import (
	"encoding/json"
	"testing"
)

// Provider metadata embedded in the shared models keeps its existing JSON field names.
func TestPlatformMetadataJSON(t *testing.T) {
	in := Track{PlatformTrack: PlatformTrack{Source: "fnmusic", Ref: "track-1", Format: "flac", Fav: true, DiscNo: 2, TrackNo: 4}}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Track
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Source != in.Source || out.Ref != in.Ref || out.Format != in.Format || !out.Fav || out.DiscNo != 2 || out.TrackNo != 4 {
		t.Fatalf("platform metadata changed during JSON round trip: %#v", out.PlatformTrack)
	}
}

// Provider-owned tracks do not get a misleading NetEase page URL or CDN query.
func TestPlatformTrackURLAndSized(t *testing.T) {
	tr := Track{PlatformTrack: PlatformTrack{Source: "fnmusic"}, ID: 7}
	if got := tr.URL(); got != "" {
		t.Fatalf("provider URL = %q, want empty", got)
	}
	if got := Sized("fnmusic://cover/7", 240); got != "fnmusic://cover/7?size=240" {
		t.Fatalf("provider cover URL = %q", got)
	}
	if got := Sized("http://music.example/cover", 240); got != "https://music.example/cover?param=240y240" {
		t.Fatalf("NetEase cover URL = %q", got)
	}
}
