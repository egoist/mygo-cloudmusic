package qqmusic

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/egoist/mygo"
)

type songShelfFeedCapture struct {
	base    http.RoundTripper
	shelves []songFeedShelf
	calls   int
	marks   []int
}

func (c *songShelfFeedCapture) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := c.base.RoundTrip(r)
	if err != nil {
		return response, err
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		return nil, err
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	var envelope struct {
		Req struct {
			Data struct {
				Shelves  []songFeedShelf `json:"v_shelf"`
				LoadMark *int            `json:"load_mark"`
			} `json:"data"`
		} `json:"req_0"`
	}
	if json.Unmarshal(body, &envelope) == nil && envelope.Req.Data.Shelves != nil {
		c.shelves = append(c.shelves, envelope.Req.Data.Shelves...)
		c.calls++
		if mark := envelope.Req.Data.LoadMark; mark != nil {
			c.marks = append(c.marks, *mark)
		}
	}
	return response, nil
}

func logSongShelfDiagnostic(t *testing.T, page string, raw []songFeedShelf, parsed []SongShelf) {
	t.Helper()
	shelfTypes := map[int64]int{}
	rawGroups, songCards, orderedIDs, tracks, complete := 0, 0, 0, 0, 0
	for _, shelf := range raw {
		shelfTypes[shelf.ID]++
		if shelf.ID != 207 {
			continue
		}
		rawGroups++
		orderedIDs += len(shelf.songIDs())
		for _, niche := range shelf.Niches {
			for _, card := range niche.Cards {
				if card.Type == 200 {
					songCards++
				}
			}
		}
	}
	for _, shelf := range parsed {
		for _, track := range shelf.Tracks {
			tracks++
			if track.ID > 0 && track.MID != "" && track.MediaMID != "" && track.Duration > 0 && len(track.Artists) > 0 {
				complete++
			}
		}
	}
	t.Logf("%s: shelf_types=%v raw_207=%d song_cards=%d ordered_song_ids=%d parsed_shelves=%d tracks=%d complete=%d", page, shelfTypes, rawGroups, songCards, orderedIDs, len(parsed), tracks, complete)
}

// Saved accounts must receive song shelves with complete tracks from the PC feed.
func TestLiveRecommendedSongShelves(t *testing.T) {
	if os.Getenv("LIVE") == "" || os.Getenv("LIVE_SONG_SHELVES_ACCOUNT") != "1" {
		t.Skip("set LIVE=1 LIVE_SONG_SHELVES_ACCOUNT=1 to read Lilt's saved account for song recommendations")
	}
	// Test binaries do not carry the packaged app identifier. Use the same
	// explicit credential service as the account search integration tests.
	b, err := mygo.Secrets.Service("dev.amamiya.qqmusic").Get(qqSessionSecret)
	if err != nil {
		t.Fatal("cannot read the saved Lilt session")
	}
	var s Session
	if json.Unmarshal(b, &s) != nil || !s.valid() {
		t.Skip("sign in to Lilt before testing song recommendations")
	}
	previous := sessionSnapshot()
	t.Cleanup(func() { setSession(previous) })
	setSession(s)
	oldClient := httpClient
	client := *oldClient
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	capture := &songShelfFeedCapture{base: base}
	client.Transport = capture
	httpClient = &client
	t.Cleanup(func() { httpClient = oldClient })
	shelves, err := RecommendedSongShelves()
	if err != nil {
		t.Fatal(err)
	}
	tracks, complete := 0, 0
	for _, shelf := range shelves {
		for _, track := range shelf.Tracks {
			tracks++
			if track.ID > 0 && track.MID != "" && track.MediaMID != "" && track.Duration > 0 && len(track.Artists) > 0 {
				complete++
			}
		}
	}
	if len(shelves) == 0 || complete != tracks {
		t.Fatalf("shelves=%d tracks=%d complete=%d", len(shelves), tracks, complete)
	}
	if capture.calls == 0 {
		t.Fatal("no feed response was observed")
	}
	logSongShelfDiagnostic(t, "merged feed", capture.shelves, shelves)
	t.Logf("feed_batches=%d load_marks=%v shelves=%d tracks=%d complete=%d", capture.calls, capture.marks, len(shelves), tracks, complete)
}
