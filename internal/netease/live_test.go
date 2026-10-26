package netease

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestQRLoginLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	// A device of the test's own, out of the keychain.
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	key, err := QRKey()
	if err != nil {
		t.Fatal(err)
	}
	code, msg, _, _, err := QRCheck(key)
	if err != nil || code != QRWaiting {
		t.Fatalf("check: %d %q %v", code, msg, err)
	}
	t.Logf("key %s: %d %s", key, code, msg)
}

func TestSongURLEapiLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	var res map[string]any
	err := eapiPost("/api/song/enhance/player/url/v1", map[string]any{"ids": "[2652820720]", "level": "exhigh", "encodeType": "mp3"}, &res)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(res)
	t.Log(string(b)[:min(400, len(b))])
}

func TestPrivilegesLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	pl, err := PlaylistDetail(3778678)
	if err != nil {
		t.Fatal(err)
	}
	with := 0
	for _, tr := range pl.Tracks {
		if tr.Privilege != nil {
			with++
		}
	}
	daily, err := DailySongs()
	if err != nil {
		t.Fatal(err)
	}
	dailyWith := 0
	for _, tr := range daily {
		if tr.Privilege != nil {
			dailyWith++
		}
	}
	t.Logf("playlist: %d of %d tracks with privileges; daily: %d of %d", with, len(pl.Tracks), dailyWith, len(daily))
	if with == 0 || dailyWith == 0 {
		t.Fatal("no privileges")
	}
	st, err := SongURL(2652820720, "exhigh")
	if err != nil || st.URL == "" || st.Size == 0 || st.Type != "mp3" {
		t.Fatalf("stream %+v %v", st, err)
	}
}

func TestRecentPlaysLive(t *testing.T) {
	// The session of a development build, which has the listener's plays.
	path := os.Getenv("NM_SESSION")
	if os.Getenv("LIVE") == "" || path == "" {
		t.Skip()
	}
	useSession(t, path)
	plays, err := RecentPlays(20)
	if err != nil || len(plays) == 0 {
		t.Fatalf("%d plays, %v", len(plays), err)
	}
	for _, p := range plays[:min(5, len(plays))] {
		t.Logf("%s %s %q playable %v", p.At.Format("01-02 15:04"), p.Device, p.Track.Name, p.Track.Playable())
	}
	if plays[0].Track.ID == 0 || plays[0].Device == "" || plays[0].At.Year() < 2020 {
		t.Fatalf("latest play %+v", plays[0])
	}
}

// useSession signs in with the session kept at path.
func useSession(t *testing.T, path string) {
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cookies []*http.Cookie
	for _, part := range strings.Split(strings.TrimSpace(string(b)), ";") {
		if name, value, ok := strings.Cut(strings.TrimSpace(part), "="); ok {
			cookies = append(cookies, &http.Cookie{Name: name, Value: value, Path: "/"})
		}
	}
	httpClient.Jar.SetCookies(siteURL, cookies)
}

func TestVIPSongPlayableLive(t *testing.T) {
	// A subscriber's VIP songs play: the web API called Stan taken down
	// (st -100) for a subscriber, and greyed it out, though the eapi
	// streamed it whole. NM_SESSION must be a subscriber's.
	path := os.Getenv("NM_SESSION")
	if os.Getenv("LIVE") == "" || path == "" {
		t.Skip()
	}
	useSession(t, path)
	songs, err := SongDetails([]int64{17572422})
	if err != nil || len(songs) != 1 {
		t.Fatalf("%d songs, %v", len(songs), err)
	}
	if !songs[0].Playable() {
		t.Fatalf("Stan not playable: %+v", songs[0].Privilege)
	}
}
