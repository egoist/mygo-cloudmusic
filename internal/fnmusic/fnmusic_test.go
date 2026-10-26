package fnmusic

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"netease-music-mygo/internal/netease"
)

// The service may be mounted anywhere a reverse proxy puts it, and is under
// /music/api/v1 there.
func TestAddress(t *testing.T) {
	for in, want := range map[string]string{
		"192.168.1.2:5666":                "http://192.168.1.2:5666/music/api/v1",
		"https://nas.example.com/":        "https://nas.example.com/music/api/v1",
		"http://nas/music":                "http://nas/music/api/v1",
		"http://nas/x/music/api/v1":       "http://nas/x/music/api/v1",
		"https://nas.example.com/proxy/p": "https://nas.example.com/proxy/p/music/api/v1",
	} {
		u, err := parseAddress(in, false)
		if err != nil || u.String() != want {
			t.Errorf("%q: got %v, %v; want %s", in, u, err, want)
		}
	}
	if _, err := parseAddress("  ", false); err == nil {
		t.Error("an empty address was taken")
	}
}

// server is a service that keeps one token: a new login replaces it.
type server struct {
	token   atomic.Value
	logins  atomic.Int32
	signed  atomic.Int32
	missing atomic.Int32
	// total, if set, is how many songs the library has, listed by page.
	total int
	// code is the access code the NAS asks for, if any.
	code string
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reply := func(code int, data any) {
		json.NewEncoder(w).Encode(map[string]any{"code": code, "msg": "", "data": data})
	}
	if r.Header.Get("authx") != "" {
		s.signed.Add(1)
	} else {
		s.missing.Add(1)
	}
	if s.code != "" {
		got, _ := base64.StdEncoding.DecodeString(r.Header.Get("x-access-code"))
		if string(got) != s.code {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}
	if r.URL.Path == "/access_code_verify" {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/music/api/v1")
	if path == "/sys/config" {
		reply(0, map[string]string{})
		return
	}
	if path == "/user/password-login" {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["password"] != HashPassword("secret") {
			reply(120003, nil)
			return
		}
		n := s.logins.Add(1)
		tok := "tok" + string(rune('0'+n))
		s.token.Store(tok)
		reply(0, map[string]string{"userToken": tok})
		return
	}
	if c, err := r.Cookie("music-token"); err != nil || c.Value != s.token.Load() {
		reply(99999, nil)
		return
	}
	switch path {
	case "/track/list":
		if s.total > 0 {
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			size, _ := strconv.Atoi(r.URL.Query().Get("size"))
			var list []map[string]any
			for i := (page - 1) * size; i < min(page*size, s.total); i++ {
				list = append(list, map[string]any{"guid": strconv.Itoa(i), "title": strconv.Itoa(i),
					"album": map[string]any{"guid": "al" + strconv.Itoa(i/10), "name": "A"}, "trackNo": i % 10,
					"audioSpec": map[string]any{"format": "mp3"}})
			}
			reply(0, map[string]any{"total": s.total, "list": list})
			return
		}
		reply(0, map[string]any{"total": 2, "list": []map[string]any{
			{"guid": "a", "title": "晴天", "artists": []map[string]any{{"name": "周杰伦"}}, "album": map[string]any{"name": "叶惠美", "coverId": "c1"},
				"audioSpec": map[string]any{"duration": 269000, "format": "FLAC"}},
			{"guid": "b", "title": "歌", "artists": []string{"甲", "乙"}, "album": "专辑", "duration": 1000, "audioSpec": map[string]any{"path": "x/y.m4a"}},
		}})
	default:
		http.NotFound(w, r)
	}
}

// Songs come back as the app's, tracks of a format the player cannot decode
// shown but not playable.
func TestTracks(t *testing.T) {
	s := &server{}
	ts := httptest.NewServer(s)
	defer ts.Close()
	c, _ := New(Config{Address: ts.URL, Username: "me", PassHash: HashPassword("secret")})
	tracks, err := c.allTracks()
	if err != nil || len(tracks) != 2 {
		t.Fatalf("%v, %d tracks", err, len(tracks))
	}
	a, b := tracks[0], tracks[1]
	if a.Name != "晴天" || a.ArtistNames() != "周杰伦" || a.Album.Name != "叶惠美" || a.Length().Seconds() != 269 {
		t.Errorf("wrong track: %+v", a)
	}
	if a.Format != "flac" || !a.Playable() || a.Ref != "a" || a.Source != Source || a.ID >= 0 {
		t.Errorf("wrong source: %+v", a)
	}
	if b.ArtistNames() != "甲, 乙" || b.Album.Name != "专辑" || b.Format != "m4a" || !b.Playable() || !Transcoded(&b) || Transcoded(&a) {
		t.Errorf("wrong second track: %+v", b)
	}
	if s.missing.Load() != 0 {
		t.Error("a request went unsigned")
	}
}

// Regression guard: a session another device ended (a new login takes the
// service's one token) gave errors until the app restarted, rather than a
// fresh login.
func TestSessionEnded(t *testing.T) {
	s := &server{}
	ts := httptest.NewServer(s)
	defer ts.Close()
	c, _ := New(Config{Address: ts.URL, Username: "me", PassHash: HashPassword("secret")})
	if _, err := c.allTracks(); err != nil {
		t.Fatal(err)
	}
	s.token.Store("another device")
	if _, err := c.allTracks(); err != nil {
		t.Fatalf("not signed in again: %v", err)
	}
	if s.logins.Load() != 2 {
		t.Errorf("%d logins, want 2", s.logins.Load())
	}
}

func TestWrongPassword(t *testing.T) {
	ts := httptest.NewServer(&server{})
	defer ts.Close()
	c, _ := New(Config{Address: ts.URL, Username: "me", PassHash: HashPassword("wrong")})
	if err := c.Login(); err == nil {
		t.Error("a wrong password was taken")
	}
}

// The web player's rules: the preferred lyrics if they read, else the timed
// ones from the best source, the untimed only when nothing is timed; and the
// offset the listener set moves the lines.
func TestChooseLyrics(t *testing.T) {
	synced := "[00:01.00]一\n[00:05.50]二"
	plain := "没有时间轴"
	var l lyricList
	add := func(guid, content string, source int, offset float64) {
		l.List = append(l.List, struct {
			GUID      string  `json:"guid"`
			Content   string  `json:"content"`
			Source    int     `json:"source"`
			Offset    float64 `json:"offset"`
			UpdatedAt float64 `json:"updatedAt"`
			CreatedAt float64 `json:"createdAt"`
		}{GUID: guid, Content: content, Source: source, Offset: offset})
	}
	add("p", plain, 1, 0)
	add("m", "[00:02.00]手动", 4, 0)
	add("e", synced, 2, 0)
	if got := chooseLyrics(l); got != synced {
		t.Errorf("the LRC file (source 2) should outrank typed in (4), and timed ones the untimed: %q", got)
	}
	l.Preferred = "m"
	if got := chooseLyrics(l); got != "[00:02.00]手动" {
		t.Errorf("preferred ignored: %q", got)
	}
	l.Preferred = "gone"
	l.List = l.List[:1]
	if got := chooseLyrics(l); got != plain {
		t.Errorf("untimed lyrics are the last resort: %q", got)
	}
	// Offset 500: lines show half a second sooner.
	if got := shift(synced, 500); got != "[00:00.500]一\n[00:05.000]二" {
		t.Errorf("shifted: %q", got)
	}
}

// lookupFor makes a lookup service that says the NAS at nas is at home,
// at its port, as FN Connect does, and points the package at it.
func lookupFor(t *testing.T, nas *httptest.Server, extra map[string]any) {
	u, _ := url.Parse(nas.URL)
	port, _ := strconv.Atoi(u.Port())
	data := map[string]any{"ipv4": []string{u.Hostname()}, "port": map[string]int{"httpPort": port}}
	for k, v := range extra {
		data[k] = v
	}
	var asked atomic.Value
	lookup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		asked.Store(body["fnId"])
		if r.Header.Get("authx") == "" {
			t.Error("the lookup was not signed")
		}
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
	}))
	old := lookupURL
	lookupURL = lookup.URL
	t.Cleanup(func() { lookupURL = old; lookup.Close() })
}

func TestFNID(t *testing.T) {
	for in, want := range map[string]string{
		"abc123":                    "abc123",
		" MyNas-01 ":                "mynas-01",
		"mynas01.5ddd.com":          "mynas01",
		"https://mynas01.5ddd.com/": "mynas01",
	} {
		if got, ok := FNID(in); !ok || got != want {
			t.Errorf("%q: %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "abc", "-abcdef", "abcdef-", "ab cdef", "nas.example.com", "https://5ddd.com/x/y"} {
		if got, ok := FNID(in); ok {
			t.Errorf("%q was taken as %q", in, got)
		}
	}
}

// With an FN ID the NAS is found by FN Connect's lookup, at the address of
// its own network that answers as the music service, and then used as an
// address would be.
func TestFNConnect(t *testing.T) {
	s := &server{}
	nas := httptest.NewServer(s)
	defer nas.Close()
	// An address that does not answer comes first and is passed over.
	lookupFor(t, nas, map[string]any{"ipv4": []string{"127.0.0.1"}, "ipv6": []string{"::1"}})
	c, err := New(Config{Mode: ModeFNConnect, Address: "MyNas01.5ddd.com", Username: "me", PassHash: HashPassword("secret")})
	if err != nil {
		t.Fatal(err)
	}
	if c.Route() != "" {
		t.Error("a route before one was looked for")
	}
	tracks, err := c.allTracks()
	if err != nil || len(tracks) != 2 {
		t.Fatalf("%v, %d tracks", err, len(tracks))
	}
	if c.Route() != "直连" {
		t.Errorf("route %q", c.Route())
	}
}

// A NAS that asks for an access code is found only with the right one, and
// the code goes with every request after.
func TestFNConnectAccessCode(t *testing.T) {
	s := &server{code: "1234"}
	nas := httptest.NewServer(s)
	defer nas.Close()
	lookupFor(t, nas, nil)
	cfg := Config{Mode: ModeFNConnect, Address: "mynas01", Username: "me", PassHash: HashPassword("secret")}

	c, _ := New(cfg)
	if err := c.Connect(); err != ErrAccessCodeRequired {
		t.Errorf("without a code: %v", err)
	}
	cfg.AccessCode = "nope"
	c, _ = New(cfg)
	if err := c.Connect(); err != ErrAccessCodeRejected {
		t.Errorf("with a wrong code: %v", err)
	}
	cfg.AccessCode = "1234"
	c, _ = New(cfg)
	if _, err := c.allTracks(); err != nil {
		t.Errorf("with the code: %v", err)
	}
}

// Regression guard: a NAS that moved (the phone changed networks) stayed
// lost until the app restarted; a request that cannot get through finds it
// again.
func TestFNConnectFindsMovedNAS(t *testing.T) {
	first := httptest.NewServer(&server{})
	lookupFor(t, first, nil)
	c, _ := New(Config{Mode: ModeFNConnect, Address: "mynas01", Username: "me", PassHash: HashPassword("secret")})
	if _, err := c.allTracks(); err != nil {
		t.Fatal(err)
	}
	first.Close()
	second := httptest.NewServer(&server{})
	defer second.Close()
	lookupFor(t, second, nil)
	if _, err := c.allTracks(); err != nil {
		t.Fatalf("did not find the NAS where it moved: %v", err)
	}
}

func TestFNConnectUnreachable(t *testing.T) {
	nas := httptest.NewServer(&server{})
	lookupFor(t, nas, nil)
	nas.Close()
	c, _ := New(Config{Mode: ModeFNConnect, Address: "mynas01", Username: "me", PassHash: HashPassword("secret")})
	if err := c.Connect(); err != ErrUnreachable {
		t.Errorf("got %v", err)
	}
}

// mp3Frames stands for audio of MP3: the bytes are no matter, only that they
// come out as they went in.
func mp3Frames(n int, fill byte) []byte { return bytes.Repeat([]byte{0xFF, 0xFB, fill}, n) }

// box makes a box of MP4.
func box(typ string, payload []byte) []byte {
	b := make([]byte, 8, 8+len(payload))
	binary.BigEndian.PutUint32(b, uint32(8+len(payload)))
	copy(b[4:], typ)
	return append(b, payload...)
}

// tsPackets wraps audio in the packets of an MPEG transport stream, as a
// PES of audio in one stream, and some of another stream that is no audio.
func tsPackets(audio []byte) []byte {
	var out []byte
	pes := append([]byte{0, 0, 1, 0xC0, 0, 0, 0x80, 0, 0}, audio...)
	for first := true; len(pes) > 0; first = false {
		pkt := make([]byte, 188)
		pkt[0], pkt[2], pkt[3] = 0x47, 0x11, 0x10
		if first {
			pkt[1] = 0x40
		}
		if len(pes) >= 184 {
			copy(pkt[4:], pes[:184])
			pes = pes[184:]
		} else {
			// The last packet is filled out by an adaptation field, as a
			// stream does, not by bytes that would pass for audio.
			pkt[3] = 0x30
			pkt[4] = byte(183 - len(pes))
			if pkt[4] > 0 {
				pkt[5] = 0
				for i := 6; i < 5+int(pkt[4]); i++ {
					pkt[i] = 0xFF
				}
			}
			copy(pkt[5+int(pkt[4]):], pes)
			pes = nil
		}
		out = append(out, pkt...)
		// A packet of the PAT between, not audio.
		other := make([]byte, 188)
		other[0], other[1], other[3] = 0x47, 0x00, 0x10
		out = append(out, other...)
	}
	return out
}

// The audio comes out of a piece of an HLS stream whether it is wrapped as
// MP4 fragments, as transport stream packets, or is the audio itself.
func TestAudioOf(t *testing.T) {
	audio := mp3Frames(300, 7)
	for name, wrapped := range map[string][]byte{
		"raw": audio,
		"mp4": append(box("styp", []byte("msdh")), append(box("moof", []byte("....")), box("mdat", audio)...)...),
		"ts":  tsPackets(audio),
	} {
		if got := audioOf(wrapped); !bytes.Equal(got, audio) {
			t.Errorf("%s: %d bytes came out, %d went in", name, len(got), len(audio))
		}
	}
}

func TestParsePlaylist(t *testing.T) {
	pl, err := parsePlaylist("#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:6,\nseg0.m4s\n#EXTINF:6,\n/abs/seg1.m4s\n#EXT-X-ENDLIST\n", "http://nas/music/api/v1/hls/x/index.m3u8")
	if err != nil || !pl.end || len(pl.segments) != 2 || pl.durations[0] != 6*time.Second ||
		pl.segments[0] != "http://nas/music/api/v1/hls/x/seg0.m4s" || pl.segments[1] != "http://nas/abs/seg1.m4s" {
		t.Errorf("%+v, %v", pl, err)
	}
	pl, _ = parsePlaylist("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\nlow/index.m3u8\n#EXT-X-STREAM-INF:BANDWIDTH=2\nhigh/index.m3u8\n", "http://nas/a/m.m3u8")
	if pl.variant != "http://nas/a/low/index.m3u8" {
		t.Errorf("variant %q", pl.variant)
	}
	if _, err := parsePlaylist("<html>", "http://nas/"); err == nil {
		t.Error("a page was taken for a playlist")
	}
}

// A track the player cannot decode is transcoded by the service: the app
// asks for MP3, gets the list of the pieces with how long each plays, and
// can ask for any one of them alone; and lets the service go of it when done.
func TestTranscodeHLS(t *testing.T) {
	hlsPoll = time.Millisecond
	defer func() { hlsPoll = time.Second }()
	s := &server{}
	var quit atomic.Int32
	var asked atomic.Value
	var polls atomic.Int32
	var fetched sync.Map
	seg := [][]byte{mp3Frames(100, 1), mp3Frames(100, 2), mp3Frames(100, 3)}
	mux := http.NewServeMux()
	mux.HandleFunc("/music/api/v1/track/transcode", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		asked.Store(body)
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]string{"url": "/music/api/v1/hls/abc/index.m3u8"}})
	})
	mux.HandleFunc("/music/api/v1/track/transcode/quit", func(w http.ResponseWriter, r *http.Request) {
		quit.Add(1)
		json.NewEncoder(w).Encode(map[string]any{"code": 0})
	})
	mux.HandleFunc("/music/api/v1/hls/abc/index.m3u8", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("authx") == "" || r.Header.Get("Cookie") == "" {
			http.Error(w, "unsigned", 401)
			return
		}
		// Still being made at first, then whole.
		if polls.Add(1) < 3 {
			w.Write([]byte("#EXTM3U\n#EXTINF:2.0,\nseg0.ts\n"))
			return
		}
		w.Write([]byte("#EXTM3U\n#EXTINF:2.0,\nseg0.ts\n#EXTINF:2.0,\nseg1.ts\n#EXTINF:1.5,\nseg2.ts\n#EXT-X-ENDLIST\n"))
	})
	for i := range seg {
		mux.HandleFunc(fmt.Sprintf("/music/api/v1/hls/abc/seg%d.ts", i), func(w http.ResponseWriter, r *http.Request) {
			fetched.Store(i, true)
			w.Write(tsPackets(seg[i]))
		})
	}
	mux.Handle("/", s)
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c, _ := New(Config{Address: ts.URL, Username: "me", PassHash: HashPassword("secret")})
	tr := &netease.Track{Ref: "g1", Format: "m4a", Source: Source}
	if !Transcoded(tr) {
		t.Fatal("an M4A is played as it is")
	}
	h, err := c.TranscodeHLS(context.Background(), tr)
	if err != nil {
		t.Fatal(err)
	}
	out := asked.Load().(map[string]any)
	if out["guid"] != "g1" || out["output"].(map[string]any)["codec"] != "mp3" {
		t.Errorf("asked %v", out)
	}
	if len(h.Segments) != 3 || h.Total != 5500*time.Millisecond || h.Segments[2].Duration != 1500*time.Millisecond {
		t.Fatalf("%d pieces, %v in all", len(h.Segments), h.Total)
	}
	// The last piece alone, without the ones before it.
	got, err := h.Fetch(context.Background(), 2)
	if err != nil || !bytes.Equal(got, seg[2]) {
		t.Fatalf("%v: %d bytes, want %d", err, len(got), len(seg[2]))
	}
	if _, was := fetched.Load(0); was {
		t.Error("the first piece was fetched to get the last")
	}
	if _, err := h.Fetch(context.Background(), 3); err == nil {
		t.Error("a piece past the end was had")
	}
	h.Release()
	h.Release()
	for i := 0; i < 200 && quit.Load() == 0; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	if quit.Load() != 1 {
		t.Errorf("the service was let go of %d times", quit.Load())
	}
}

// The service picks the songs of a roam one after another, each asked for
// with the roam ID of the one before.
func TestRoam(t *testing.T) {
	var rel atomic.Value
	mux := http.NewServeMux()
	step := func(id, title string) map[string]any {
		return map[string]any{"roamId": id, "track": map[string]any{"guid": id, "title": title, "audioSpec": map[string]any{"format": "mp3"}}}
	}
	mux.HandleFunc("/music/api/v1/track/roam-start", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"current": step("r1", "一"), "next": step("r2", "二")}})
	})
	mux.HandleFunc("/music/api/v1/track/roam-next", func(w http.ResponseWriter, r *http.Request) {
		rel.Store(r.URL.Query().Get("relativeRoamId") + "/" + r.URL.Query().Get("deviceId"))
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"previous": step("r1", "一"), "current": step("r3", "三")}})
	})
	mux.Handle("/", &server{})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c, _ := New(Config{Address: ts.URL, Username: "me", PassHash: HashPassword("secret"), DeviceID: "dev"})
	tr, id, err := c.RoamStart()
	if err != nil || tr.Name != "一" || id != "r1" || tr.Ref != "r1" {
		t.Fatalf("%+v %q %v", tr, id, err)
	}
	tr, id, err = c.RoamNext(id)
	if err != nil || tr.Name != "三" || id != "r3" || rel.Load() != "r1/dev" {
		t.Fatalf("%+v %q %v, asked %v", tr, id, err, rel.Load())
	}
}

// A track of a CUE sheet is as long as its part of the file, and is
// transcoded, which has the service cut it out.
func TestCueTrack(t *testing.T) {
	var r rawTrack
	// The audio spec's duration is the whole file's; the track's is its own.
	json.Unmarshal([]byte(`{"guid":"c","title":"一","isCue":true,"duration":140500,"audioSpec":{"duration":2400000,"format":"flac"}}`), &r)
	tr, ok := r.track()
	if !ok || tr.Length().Seconds() != 140.5 || !tr.Cue || !Transcoded(&tr) || !tr.Playable() {
		t.Errorf("%v, %+v", ok, tr)
	}
	// Without its own, where it starts and ends in the file says.
	var r2 rawTrack
	json.Unmarshal([]byte(`{"guid":"d","title":"二","isCue":true,"startTime":60,"endTime":200.5,"audioSpec":{"duration":2400000,"format":"flac"}}`), &r2)
	if tr, _ := r2.track(); tr.Length().Seconds() != 140.5 {
		t.Errorf("length %v", tr.Length())
	}
}

// allTracks reads the whole library, a page at a time: for tests.
func (c *Client) allTracks() ([]netease.Track, error) {
	var out []netease.Track
	for pg := 1; ; pg++ {
		tracks, total, err := c.LibraryPage(SortRecent, pg, 500)
		if err != nil {
			return nil, err
		}
		out = append(out, tracks...)
		if len(out) >= total || len(tracks) == 0 {
			return out, nil
		}
	}
}

// A page of the library is the page asked for, with how many songs there
// are in all, whatever its size: what a list loaded as it is scrolled
// needs to size its scrollbar before most of it is in.
func TestLibraryPage(t *testing.T) {
	ts := httptest.NewServer(&server{total: 2301})
	defer ts.Close()
	c, _ := New(Config{Address: ts.URL, Username: "me", PassHash: HashPassword("secret")})
	tracks, total, err := c.LibraryPage(SortRecent, 3, 100)
	if err != nil || total != 2301 || len(tracks) != 100 || tracks[0].Name != "200" || tracks[99].Name != "299" {
		t.Fatalf("%v, total %d, %d songs", err, total, len(tracks))
	}
	last, _, _ := c.LibraryPage(SortRecent, 24, 100)
	if len(last) != 1 || last[0].Name != "2300" {
		t.Errorf("last page has %d songs", len(last))
	}
	if tracks[0].Album.ID == 0 || tracks[5].TrackNo != 5 {
		t.Errorf("album or track number lost: %+v", tracks[5])
	}
}

// The service tells which songs the listener likes with them, so no list of
// the songs liked has to be read to draw the hearts.
func TestFavoriteComesWithTheTrack(t *testing.T) {
	var r rawTrack
	json.Unmarshal([]byte(`{"guid":"a","title":"x","isFavorite":true,"audioSpec":{"format":"flac"}}`), &r)
	if tr, _ := r.track(); !tr.Fav {
		t.Error("liked song not marked")
	}
}

// An address with no scheme takes the protocol the listener chose, and one
// with a scheme its own, whatever was chosen.
func TestAddressProtocol(t *testing.T) {
	for _, c := range []struct {
		addr  string
		https bool
		want  string
	}{
		{"192.168.1.2:5666", false, "http://192.168.1.2:5666/music/api/v1"},
		{"192.168.1.2:5667", true, "https://192.168.1.2:5667/music/api/v1"},
		{"http://192.168.1.2:5666", true, "http://192.168.1.2:5666/music/api/v1"},
		{"https://nas.lan:5667/", false, "https://nas.lan:5667/music/api/v1"},
		// A service the listener put on a domain of their own: no port, the
		// protocol's own; and behind a reverse proxy, a path before it.
		{"music.example.com", false, "http://music.example.com/music/api/v1"},
		{"music.example.com", true, "https://music.example.com/music/api/v1"},
		{"music.example.com:8443", true, "https://music.example.com:8443/music/api/v1"},
		{"https://example.com/nas/", false, "https://example.com/nas/music/api/v1"},
		{"example.com/nas/music", true, "https://example.com/nas/music/api/v1"},
		{"[fd00::1]:5666", false, "http://[fd00::1]:5666/music/api/v1"},
	} {
		u, err := parseAddress(c.addr, c.https)
		if err != nil || u.String() != c.want {
			t.Errorf("%q (https %v): %v, %v; want %s", c.addr, c.https, u, err, c.want)
		}
	}
}

func addressClient(t *testing.T, addr string, https bool, code string) *Client {
	c, err := New(Config{Address: addr, HTTPS: https, Username: "me", PassHash: HashPassword("secret"), AccessCode: code})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// A NAS that asks for an access code is reached by its address only with
// the right one, and the dialog is told which it is that is wrong, not that
// the password is.
func TestAddressAccessCode(t *testing.T) {
	ts := httptest.NewServer(&server{code: "1234"})
	defer ts.Close()
	host := strings.TrimPrefix(ts.URL, "http://")
	if err := addressClient(t, host, false, "").Connect(); err != ErrAccessCodeRequired {
		t.Errorf("without a code: %v", err)
	}
	if err := addressClient(t, host, false, "nope").Connect(); err != ErrAccessCodeRejected {
		t.Errorf("with a wrong code: %v", err)
	}
	c := addressClient(t, host, false, "1234")
	if err := c.Connect(); err != nil {
		t.Fatalf("with the code: %v", err)
	}
	if _, err := c.allTracks(); err != nil {
		t.Errorf("the code does not go with the requests after: %v", err)
	}
	// A NAS with none needs none given.
	open := httptest.NewServer(&server{})
	defer open.Close()
	if err := addressClient(t, strings.TrimPrefix(open.URL, "http://"), false, "").Connect(); err != nil {
		t.Errorf("a NAS with no access code: %v", err)
	}
}

// The protocol is the listener's to choose, and is told when it is the
// wrong one: HTTP to a port that speaks TLS, and HTTPS to one that does not,
// each as its own message rather than a bare 400 or a Go error.
func TestAddressWrongProtocol(t *testing.T) {
	plain := httptest.NewServer(&server{})
	defer plain.Close()
	tls := httptest.NewTLSServer(&server{})
	defer tls.Close()
	plainHost, tlsHost := strings.TrimPrefix(plain.URL, "http://"), strings.TrimPrefix(tls.URL, "https://")

	if err := addressClient(t, plainHost, false, "").Connect(); err != nil {
		t.Errorf("http to a plain port: %v", err)
	}
	// A certificate of its own (httptest's) on an address given as an IP is
	// taken, as a NAS at home has one.
	if err := addressClient(t, tlsHost, true, "").Connect(); err != nil {
		t.Errorf("https to a port with a self-signed certificate: %v", err)
	}
	if err := addressClient(t, tlsHost, false, "").Connect(); err != ErrNeedHTTPS {
		t.Errorf("http to a TLS port: %v", err)
	}
	if err := addressClient(t, plainHost, true, "").Connect(); err != ErrNeedHTTP {
		t.Errorf("https to a plain port: %v", err)
	}
}

// A certificate nobody vouches for is taken on the local network, where a
// NAS has one of its own, and not on a name on the internet: there it is
// said not to be trusted.
type tlsState = tls.ConnectionState

func TestCertificateOnlyOnTheLocalNetwork(t *testing.T) {
	for host, local := range map[string]bool{
		"192.168.1.10": true, "10.0.0.2": true, "172.16.1.10": true, "127.0.0.1": true, "::1": true,
		"localhost": true, "nas": true, "nas.local": true, "nas.lan": true, "nas.home": true, "nas.internal": true,
		"NAS.LOCAL": true, "nas.lan.": true,
		"nas.example.com": false, "example.com": false, "my-nas.duckdns.org": false, "locally.com": false,
	} {
		if got := localHost(host); got != local {
			t.Errorf("localHost(%q) = %v, want %v", host, got, local)
		}
	}
	tls := httptest.NewTLSServer(&server{})
	defer tls.Close()
	cert := tls.Certificate()
	state := func(name string) tlsState {
		return tlsState{ServerName: name, PeerCertificates: []*x509.Certificate{cert}}
	}
	if err := verifyNamed(state("nas.lan")); err != nil {
		t.Errorf("a self-signed certificate on the local network: %v", err)
	}
	err := verifyNamed(state("nas.example.com"))
	if err == nil {
		t.Fatal("a self-signed certificate on a name on the internet was taken")
	}
	if got := addressError(err); got != ErrCertUntrusted {
		t.Errorf("said %v", got)
	}
	// And a name on the local network connects over HTTPS, end to end.
	host := strings.Replace(strings.TrimPrefix(tls.URL, "https://"), "127.0.0.1", "localhost", 1)
	if err := addressClient(t, host, true, "").Connect(); err != nil {
		t.Errorf("https to localhost with a self-signed certificate: %v", err)
	}
}

// What answers but is not the music service is told so, and what does not
// answer is told apart from it.
func TestAddressNotTheMusicService(t *testing.T) {
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>some other service</html>"))
	}))
	defer web.Close()
	if err := addressClient(t, strings.TrimPrefix(web.URL, "http://"), false, "").Connect(); err != ErrMusicUnavailable {
		t.Errorf("another service: %v", err)
	}
	gone := httptest.NewServer(&server{})
	host := strings.TrimPrefix(gone.URL, "http://")
	gone.Close()
	if err := addressClient(t, host, false, "").Connect(); err != ErrUnreachable {
		t.Errorf("nothing there: %v", err)
	}
}

// A service behind a reverse proxy, on a domain with a path before it, is
// reached and signed in to: the signature is of the path from /music/api/v1
// on, which is what the NAS checks, not the whole of it.
func TestAddressBehindAPathPrefix(t *testing.T) {
	var asked atomic.Value
	inner := &server{}
	mux := http.NewServeMux()
	mux.Handle("/nas/", http.StripPrefix("/nas", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Store(r.URL.Path)
		inner.ServeHTTP(w, r)
	})))
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c := addressClient(t, strings.TrimPrefix(ts.URL, "http://")+"/nas", false, "")
	if err := c.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := c.Login(); err != nil {
		t.Fatalf("login: %v", err)
	}
	if tracks, err := c.allTracks(); err != nil || len(tracks) != 2 {
		t.Fatalf("%v, %d tracks", err, len(tracks))
	}
}

// playlistService is a service that keeps playlists, as the real one does
// for the calls tried against it: the song added last is first, one that is in
// already moves to the front, and what is asked of a playlist that is not is
// refused.
type playlistService struct {
	mu      sync.Mutex
	lists   map[string][]string
	names   map[string]string
	covers  map[string]string
	creates int
	signed  []string // what the cover upload was signed over
}

func (p *playlistService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reply := func(code int, data any) {
		json.NewEncoder(w).Encode(map[string]any{"code": code, "msg": "invalid arguments", "data": data})
	}
	path := strings.TrimPrefix(r.URL.Path, "/music/api/v1")
	if path == "/user/password-login" {
		reply(0, map[string]string{"userToken": "t"})
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	var body struct {
		GUID, Name, CoverID string
		TrackGUIDs          []string
	}
	if path == "/static/cover/playlist" {
		r.ParseMultipartForm(1 << 20)
		f, h, err := r.FormFile("file")
		if err != nil {
			reply(1, nil)
			return
		}
		defer f.Close()
		p.signed = append(p.signed, r.Header.Get("authx"), h.Header.Get("Content-Type"))
		reply(0, map[string]string{"coverId": "playlist_cover1"})
		return
	}
	json.NewDecoder(r.Body).Decode(&body)
	switch path {
	case "/playlist/create":
		p.creates++
		if body.Name == "" {
			reply(1, nil)
			return
		}
		guid := "pl" + strconv.Itoa(p.creates)
		p.lists[guid], p.names[guid] = []string{}, body.Name
		reply(0, map[string]any{"guid": guid, "name": body.Name, "coverId": nil})
	case "/playlist/add-track":
		if _, ok := p.lists[body.GUID]; !ok {
			reply(1, nil)
			return
		}
		for _, g := range body.TrackGUIDs {
			rest := p.lists[body.GUID][:0:0]
			for _, x := range p.lists[body.GUID] {
				if x != g {
					rest = append(rest, x)
				}
			}
			p.lists[body.GUID] = append([]string{g}, rest...)
		}
		reply(0, nil)
	case "/playlist/remove-track":
		var rest []string
		for _, x := range p.lists[body.GUID] {
			if !slices.Contains(body.TrackGUIDs, x) {
				rest = append(rest, x)
			}
		}
		p.lists[body.GUID] = rest
		reply(0, nil)
	case "/playlist/edit":
		// As the real one: an edit with no name is refused, of a cover too.
		if body.Name == "" {
			reply(1, nil)
			return
		}
		if body.Name != "" {
			p.names[body.GUID] = body.Name
		}
		if body.CoverID != "" {
			p.covers[body.GUID] = body.CoverID
		}
		reply(0, nil)
	case "/playlist/delete":
		delete(p.lists, body.GUID)
		delete(p.names, body.GUID)
		reply(0, nil)
	default:
		reply(1, nil)
	}
}

// The calls that edit playlists say what they should to the service, and what
// the service refuses is an error.
func TestPlaylistWrites(t *testing.T) {
	svc := &playlistService{lists: map[string][]string{}, names: map[string]string{}, covers: map[string]string{}}
	ts := httptest.NewServer(svc)
	defer ts.Close()
	c, _ := New(Config{Address: ts.URL, Username: "me", PassHash: HashPassword("secret")})

	pl, err := c.CreatePlaylist("通勤")
	if err != nil || pl.Ref != "pl1" || pl.Name != "通勤" || pl.ID != ID("pl1") {
		t.Fatalf("%+v, %v", pl, err)
	}
	if _, err := c.CreatePlaylist(""); err == nil {
		t.Error("a playlist with no name was made")
	}
	if err := c.AddToPlaylist(pl.Ref, []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	// One that is in moves to the front, and none is twice.
	c.AddToPlaylist(pl.Ref, []string{"a"})
	if got := svc.lists["pl1"]; len(got) != 2 || got[0] != "a" {
		t.Errorf("songs %v", got)
	}
	if err := c.AddToPlaylist("nope", []string{"a"}); err == nil {
		t.Error("songs were put in a playlist that is not")
	}
	c.RemoveFromPlaylist(pl.Ref, []string{"a"})
	if got := svc.lists["pl1"]; len(got) != 1 || got[0] != "b" {
		t.Errorf("songs after removing %v", got)
	}
	c.RenamePlaylist(pl.Ref, "上班路上")
	if svc.names["pl1"] != "上班路上" {
		t.Errorf("name %q", svc.names["pl1"])
	}
	if err := c.DeletePlaylist(pl.Ref); err != nil || len(svc.lists) != 0 {
		t.Errorf("delete: %v, %d left", err, len(svc.lists))
	}
}

// A cover is uploaded as a form of one field, signed over {} as the web player
// signs it, and then made the playlist's; what is no picture is refused before
// anything is sent.
func TestPlaylistCover(t *testing.T) {
	svc := &playlistService{lists: map[string][]string{}, names: map[string]string{}, covers: map[string]string{}}
	ts := httptest.NewServer(svc)
	defer ts.Close()
	c, _ := New(Config{Address: ts.URL, Username: "me", PassHash: HashPassword("secret")})
	pl, _ := c.CreatePlaylist("x")
	png := append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, make([]byte, 100)...)
	id, err := c.SetPlaylistCover(pl.Ref, "x", png)
	if err != nil || id != "playlist_cover1" || svc.covers[pl.Ref] != "playlist_cover1" {
		t.Fatalf("%q, %v, cover %q", id, err, svc.covers[pl.Ref])
	}
	if len(svc.signed) != 2 || svc.signed[1] != "image/png" || !strings.Contains(svc.signed[0], "sign=") {
		t.Errorf("upload: %v", svc.signed)
	}
	for name, b := range map[string][]byte{"text": []byte("not a picture"), "empty": nil, "too big": make([]byte, MaxCoverBytes+1)} {
		if _, err := c.SetPlaylistCover(pl.Ref, "x", b); err != ErrBadCover {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Regression guard: making a playlist over FN Connect, when the route
// failed after the request had gone, asked again over another, and the
// answer that never came may have been to the first: two playlists were made.
// Adding songs is asked again, which is safe.
func TestCreateIsNotAskedTwice(t *testing.T) {
	var creates, adds atomic.Int32
	lookup := func(nas *httptest.Server) { lookupFor(t, nas, nil) }
	mux := http.NewServeMux()
	cut := func(counter *atomic.Int32) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// The first is taken and the connection dropped, as a route that
			// failed after the request went.
			if counter.Add(1) == 1 {
				conn, _, _ := w.(http.Hijacker).Hijack()
				conn.Close()
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"guid": "g", "name": "n"}})
		}
	}
	mux.HandleFunc("/music/api/v1/playlist/create", cut(&creates))
	mux.HandleFunc("/music/api/v1/playlist/add-track", cut(&adds))
	mux.Handle("/", &server{})
	nas := httptest.NewServer(mux)
	defer nas.Close()
	lookup(nas)
	c, _ := New(Config{Mode: ModeFNConnect, Address: "mynas01", Username: "me", PassHash: HashPassword("secret")})
	if _, err := c.CreatePlaylist("n"); err == nil {
		t.Error("a create that was cut was asked again")
	}
	if creates.Load() != 1 {
		t.Errorf("%d creates", creates.Load())
	}
	if err := c.AddToPlaylist("g", []string{"a"}); err != nil || adds.Load() != 2 {
		t.Errorf("adding: %v, asked %d times", err, adds.Load())
	}
}

// A play is told to the service as the event the web player sends, and the
// play history is read and taken from as the service lists it.
func TestPlayHistoryCalls(t *testing.T) {
	var bodies []string
	var mu sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("/music/api/v1/event/report", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, "report "+string(b))
		mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"code": 0})
	})
	mux.HandleFunc("/music/api/v1/play-history/delete", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, "delete "+string(b))
		mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"code": 0})
	})
	mux.HandleFunc("/music/api/v1/play-history/list", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"total": 2, "list": []map[string]any{
			{"guid": "b", "title": "后听的", "audioSpec": map[string]any{"format": "mp3"}},
			{"guid": "a", "title": "先听的", "audioSpec": map[string]any{"format": "flac"}},
		}}})
	})
	mux.Handle("/", &server{})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c, _ := New(Config{Address: ts.URL, Username: "me", PassHash: HashPassword("secret")})

	if err := c.ReportPlay("guid1"); err != nil {
		t.Fatal(err)
	}
	var ev struct {
		Events []struct {
			EventType  string
			OccurredAt int64
			Payload    map[string]string
		}
	}
	mu.Lock()
	json.Unmarshal([]byte(strings.TrimPrefix(bodies[0], "report ")), &ev)
	mu.Unlock()
	if len(ev.Events) != 1 || ev.Events[0].EventType != "track_play" || ev.Events[0].Payload["trackGUID"] != "guid1" ||
		time.Since(time.UnixMilli(ev.Events[0].OccurredAt)) > 5*time.Second {
		t.Errorf("event %+v", ev)
	}
	tracks, total, err := c.RecentPage(1, 50)
	if err != nil || total != 2 || len(tracks) != 2 || tracks[0].Name != "后听的" {
		t.Errorf("%v, %d, %v", err, total, tracks)
	}
	if err := c.RemoveFromHistory([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if bodies[1] != `delete {"trackGUIDs":["a"]}` {
		t.Errorf("asked %q", bodies[1])
	}
}
