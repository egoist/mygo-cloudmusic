package qqmusic

import (
	"os"
	"strconv"
	"testing"
)

// Every advertised song chart must return usable tracks rather than a successful empty custom page.
func TestLiveSongCharts(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("set LIVE to contact QQ Music")
	}
	previous := sessionSnapshot()
	t.Cleanup(func() { setSession(previous) })
	// Exercise public chart reads while an account is present without using real credentials.
	setSession(Session{MusicID: "123", MusicKey: "fixture-credential", LoginType: 2})
	charts, err := Charts()
	if err != nil || len(charts) == 0 {
		t.Fatalf("chart catalog: count=%d, error=%v", len(charts), err)
	}
	for _, chart := range charts {
		t.Run(strconv.FormatInt(chart.ID-chartIDBase, 10), func(t *testing.T) {
			detail, err := PlaylistDetail(chart.ID)
			if err != nil {
				t.Fatalf("%s: %v", chart.Name, err)
			}
			if detail.Name == "" || len(detail.Tracks) == 0 {
				t.Fatalf("missing chart contents: %+v", detail)
			}
			for _, track := range detail.Tracks {
				if track.ID <= 0 || track.MID == "" || track.Name == "" {
					t.Fatalf("unusable chart track: %+v", track)
				}
			}
			t.Logf("%s: %d songs", detail.Name, len(detail.Tracks))
		})
	}
}
