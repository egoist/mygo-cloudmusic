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

func TestArtistDetailLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	ar, err := ArtistDetail(12138269)
	if err != nil {
		t.Fatal(err)
	}
	if ar.Name == "" || ar.Avatar == "" || len(ar.Songs) == 0 || ar.Songs[0].Privilege == nil {
		t.Fatalf("artist %+v with %d songs", ar.Artist, len(ar.Songs))
	}
	t.Logf("%s: %d songs, %d albums, first %q", ar.Name, ar.MusicSize, ar.AlbumSize, ar.Songs[0].Name)
}

func TestArtistLatestSongsLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	// The brilliant green's come in the old shape, 毛不易's in the new.
	for _, id := range []int64{21823, 12138269} {
		songs, err := ArtistLatestSongs(id, 100)
		if err != nil || len(songs) == 0 {
			t.Fatalf("artist %d: %d songs, %v", id, len(songs), err)
		}
		s := songs[0]
		if len(s.Artists) == 0 || s.Album.PicURL == "" || s.Duration == 0 || s.Privilege == nil {
			t.Fatalf("artist %d: newest song %+v", id, s)
		}
		t.Logf("artist %d: %d songs, newest %q", id, len(songs), s.Name)
	}
}

// Each kind of search finds something for a common query, and says how
// many it found in all; podcasts come 10 at most, on one page.
func TestSearchPageLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	for _, kind := range []SearchType{SearchSongs, SearchAlbums, SearchArtists, SearchUsers, SearchPodcasts} {
		res, err := SearchPage("音乐", kind, 5, 0)
		if err != nil || res.Len() == 0 || res.Total < res.Len() {
			t.Fatalf("kind %d: %d of %d, %v", kind, res.Len(), res.Total, err)
		}
	}
	s, err := SearchSuggest("周杰")
	if err != nil || len(s.Songs) == 0 || len(s.Songs[0].Artists) == 0 || len(s.Albums) == 0 || len(s.Albums[0].Artists) == 0 {
		t.Fatalf("suggestions %+v, %v", s, err)
	}
}

// An album's page has its artists and songs in the shape Track reads; an
// artist's albums page on.
func TestAlbumLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	al, err := AlbumDetail(32311)
	if err != nil || al.Name == "" || len(al.Artists) == 0 || al.PublishTime == 0 || len(al.Songs) == 0 {
		t.Fatalf("album %+v, %v", al, err)
	}
	if s := al.Songs[0]; len(s.Artists) == 0 || s.Album.PicURL == "" || s.Duration == 0 || s.Privilege == nil {
		t.Fatalf("first song %+v", s)
	}
	albums, more, err := ArtistAlbums(6452, 5, 0)
	if err != nil || len(albums) != 5 || !more || albums[0].PicURL == "" {
		t.Fatalf("%d albums, more %v, %v", len(albums), more, err)
	}
}

// An artist with an account of their own has its events, a page at a
// time, and a podcast has its episodes, each with audio to play.
func TestEventsAndPodcastLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	ar, err := ArtistDetail(5781)
	if err != nil || ar.AccountID == 0 {
		t.Fatalf("artist %+v, %v", ar, err)
	}
	events, last, _, err := Events(ar.AccountID, 5, -1)
	if err != nil || len(events) == 0 || last == 0 || events[0].User.Nickname == "" {
		t.Fatalf("%d events, last %d, %v", len(events), last, err)
	}
	res, err := SearchPage("故事", SearchPodcasts, 1, 0)
	if err != nil || len(res.Podcasts) == 0 {
		t.Fatalf("podcasts %+v, %v", res, err)
	}
	pod, err := PodcastDetail(res.Podcasts[0].ID)
	if err != nil || pod.Name == "" || pod.ProgramCount == 0 {
		t.Fatalf("podcast %+v, %v", pod, err)
	}
	programs, _, err := PodcastPrograms(pod.ID, 5, 0)
	if err != nil || len(programs) == 0 || programs[0].MainSong.ID == 0 {
		t.Fatalf("%d episodes, %v", len(programs), err)
	}
}

// The podcasts tab's lists come a page at a time, with names and covers.
func TestPodcastListsLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	rec, err := RecommendedPodcasts()
	if err != nil || len(rec) == 0 || rec[0].Name == "" {
		t.Fatalf("%d recommended, %v", len(rec), err)
	}
	hot, more, err := HotPodcasts(5, 0)
	if err != nil || len(hot) != 5 || !more || hot[0].PicURL == "" {
		t.Fatalf("%d hot, more %v, %v", len(hot), more, err)
	}
	next, _, err := HotPodcasts(5, 5)
	if err != nil || len(next) == 0 || next[0].ID == hot[0].ID {
		t.Fatalf("next page %+v, %v", next, err)
	}
}

// An episode has comments of its own, not its audio's, and NetEase answers
// of each episode asked about whether the listener likes it.
func TestProgramCommentsLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip()
	}
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	res, err := SearchPage("故事", SearchPodcasts, 1, 0)
	if err != nil || len(res.Podcasts) == 0 {
		t.Fatalf("podcasts %+v, %v", res, err)
	}
	pod := &res.Podcasts[0]
	programs, _, err := PodcastPrograms(pod.ID, 2, 0)
	if err != nil || len(programs) < 2 {
		t.Fatalf("%d episodes, %v", len(programs), err)
	}
	tr := programs[0].Track(pod)
	page, err := ThreadComments(tr.Thread(), 0, 5)
	if err != nil || page.Total == 0 || len(page.Latest) == 0 {
		t.Fatalf("comments %+v, %v", page, err)
	}
	ids := []int64{programs[0].ID, programs[1].ID}
	liked, err := ProgramsLiked(ids)
	if err != nil || len(liked) != 2 {
		t.Fatalf("liked %v, %v", liked, err)
	}
	if _, ok := liked[ids[1]]; !ok {
		t.Errorf("no answer of %d: %v", ids[1], liked)
	}
}
