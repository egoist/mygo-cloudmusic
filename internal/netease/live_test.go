package netease

import (
	"encoding/json"
	"os"
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
