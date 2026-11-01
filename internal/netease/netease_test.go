package netease

import (
	"encoding/json"
	"slices"
	"sync"
	"testing"
)

func TestPlayableFollowsPrivilege(t *testing.T) {
	tracks := []Track{{ID: 1, Fee: 1}, {ID: 2, Fee: 8}, {ID: 3, Fee: 8}, {ID: 4, Fee: 1}}
	if tracks[0].Playable() || !tracks[1].Playable() {
		t.Fatal("without privileges, VIP songs do not play and free ones do")
	}
	withPrivileges(tracks, []Privilege{
		{ID: 1, Pl: 320000},           // VIP song, the listener subscribes
		{ID: 2, Pl: 0},                // nothing to play
		{ID: 3, St: -200, Pl: 128000}, // taken down
	})
	want := []bool{true, false, false, false}
	for i, tr := range tracks {
		if tr.Playable() != want[i] {
			t.Errorf("track %d: playable %v", tr.ID, tr.Playable())
		}
	}
}

// A playlist whose detail came with only some of its tracks, the rest by ID:
// they are fetched and put back where the playlist has them, not after it.
func TestPlaylistTracksFollowOrder(t *testing.T) {
	old := songDetails
	defer func() { songDetails = old }()
	var asked []int64
	songDetails = func(ids []int64) ([]Track, error) {
		asked = append(asked, ids...)
		tracks := make([]Track, len(ids))
		for i, id := range ids {
			tracks[i] = Track{ID: id}
		}
		return tracks, nil
	}
	// Five tracks, of which the detail carried the first and the third.
	var p Playlist
	err := json.Unmarshal([]byte(`{
		"trackCount": 5,
		"tracks": [{"id": 1}, {"id": 3}],
		"trackIds": [{"id": 1}, {"id": 2}, {"id": 3}, {"id": 4}, {"id": 5}]
	}`), &p)
	if err != nil {
		t.Fatal(err)
	}

	tracks := allTracks(&p)
	want := []int64{1, 2, 3, 4, 5}
	if len(tracks) != len(want) {
		t.Fatalf("%d tracks, want %d", len(tracks), len(want))
	}
	for i, tr := range tracks {
		if tr.ID != want[i] {
			t.Errorf("track %d is %d, want %d", i, tr.ID, want[i])
		}
	}
	if !slices.Equal(asked, []int64{2, 4, 5}) {
		t.Errorf("fetched %v, want the missing ones", asked)
	}
}

// Thousands of tracks are fetched a thousand IDs to a request, and come back
// in their order.
func TestSongDetailsFollowOrder(t *testing.T) {
	old := songDetails
	defer func() { songDetails = old }()
	var mu sync.Mutex
	var batches []int
	songDetails = func(ids []int64) ([]Track, error) {
		mu.Lock()
		batches = append(batches, len(ids))
		mu.Unlock()
		tracks := make([]Track, len(ids))
		for i, id := range ids {
			tracks[i] = Track{ID: id}
		}
		return tracks, nil
	}

	ids := make([]int64, 2500)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	tracks := songDetailsAll(ids)
	if len(tracks) != len(ids) {
		t.Fatalf("%d tracks, want %d", len(tracks), len(ids))
	}
	for i, tr := range tracks {
		if tr.ID != ids[i] {
			t.Fatalf("track %d is %d, want %d", i, tr.ID, ids[i])
		}
	}
	slices.Sort(batches)
	if !slices.Equal(batches, []int{500, 1000, 1000}) {
		t.Errorf("batches %v, want three of a thousand at most", batches)
	}
}
