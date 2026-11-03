package qqmusic

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type redirectTransport struct {
	target string
	base   http.RoundTripper
}

func (r redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	destination := *req.URL
	target, _ := http.NewRequest(http.MethodGet, r.target, nil)
	destination.Scheme = target.URL.Scheme
	destination.Host = target.URL.Host
	cloned.URL = &destination
	return r.base.RoundTrip(cloned)
}
func withQQServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	old := httpClient
	oldSession := sessionSnapshot()
	oldSearches := searches
	searches = newSearchEngine()
	searches.interval = 0
	setSession(Session{})
	httpClient = &http.Client{Transport: redirectTransport{server.URL, http.DefaultTransport}}
	t.Cleanup(func() { httpClient = old; searches = oldSearches; setSession(oldSession); server.Close() })
}
func rpcReply(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "req_0": map[string]any{"code": 0, "data": data}})
}

// Anonymous Lite search retains the IDs needed by later playback.
func TestAnonymousSearchKeepsMID(t *testing.T) {
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Comm map[string]any `json:"comm"`
			Req  struct {
				Module, Method string
				Param          map[string]any
			} `json:"req_0"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Req.Method != "DoSearchForQQMusicLite" || request.Req.Param["search_type"] != float64(7) {
			t.Errorf("wrong search: %+v", request)
		}
		if request.Comm["tmeAppID"] != "qqmusiclight" || r.Header.Get("Cookie") != "tmeLoginType=-1" {
			t.Errorf("wrong anonymous identity")
		}
		id, _ := request.Req.Param["search_id"].(string)
		if len(id) != 18 || strings.ContainsAny(id, "abcdef") {
			t.Errorf("invalid search id %q", id)
		}
		rpcReply(w, map[string]any{"body": map[string]any{"item_song": []any{map[string]any{"id": 101, "mid": "songMid", "title": "Song", "interval": 180, "singer": []any{map[string]any{"id": 201, "mid": "artistMid", "name": "Artist"}}, "album": map[string]any{"id": 301, "mid": "albumMid", "name": "Album"}, "file": map[string]any{"media_mid": "mediaMid"}}}}, "meta": map[string]any{"sum": 23}})
	})
	result, err := SearchPageSource("Song", SearchSongs, 20, 0, SearchLite)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 23 || len(result.Songs) != 1 {
		t.Fatalf("unexpected result %+v", result)
	}
	song := result.Songs[0]
	if song.MID != "songMid" || song.MediaMID != "mediaMid" || song.Duration != 180000 || song.Artists[0].MID != "artistMid" || song.Album.MID != "albumMid" {
		t.Fatalf("lost song fields: %+v", song)
	}
}

// Logged-in search needs the PC identity; Android comm returned filtered empty results and forced anonymity was rejected.
func TestSearchUsesPCIdentityWithSession(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind SearchType
		code int
		key  string
		item string
	}{
		{"songs", SearchSongs, 0, "item_song", `{"id":101,"mid":"songMid","title":"Song"}`},
		{"artists", SearchArtists, 1, "singer", `{"id":201,"mid":"artistMid","name":"Artist"}`},
		{"albums", SearchAlbums, 2, "item_album", `{"id":301,"mid":"albumMid","name":"Album"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Comm map[string]any
					Req  struct {
						Method string
						Param  map[string]any
					} `json:"req_0"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if request.Comm["ct"] != float64(24) || request.Comm["cv"] != float64(0) || request.Comm["uin"] != "123" || request.Comm["qq"] != "123" || request.Comm["authst"] != "fixture-credential" || request.Comm["tmeLoginType"] != float64(2) || request.Comm["format"] != "json" || request.Comm["tmeAppID"] != nil || r.UserAgent() != UserAgent || !strings.Contains(r.Header.Get("Cookie"), "qm_keyst=fixture-credential") {
					t.Error("logged-in search did not use the verified PC identity")
					// The old Android identity returned success with this filtered empty body.
					rpcReply(w, map[string]any{"code": 0, "body": map[string]any{tc.key: []any{}}, "meta": map[string]any{"sum": 0, "is_filter": -9}})
					return
				}
				if request.Req.Method != "DoSearchForQQMusicLite" || request.Req.Param["search_type"] != float64(tc.code) || request.Req.Param["page_num"] != float64(2) || request.Req.Param["num_per_page"] != float64(20) || request.Req.Param["query"] != "周杰伦" {
					t.Error("wrong search kind, query or pagination")
				}
				rpcReply(w, map[string]any{"code": 0, "body": map[string]any{tc.key: []json.RawMessage{json.RawMessage(tc.item)}}, "meta": map[string]any{"sum": 45}})
			})
			setSession(Session{MusicID: "123", MusicKey: "fixture-credential", LoginType: 2})
			result, err := SearchPageSource(" 周杰伦 ", tc.kind, 20, 20, SearchLite)
			if err != nil || result.Len() != 1 || result.Total != 45 {
				t.Fatalf("search became empty: length=%d total=%d error=%v", result.Len(), result.Total, err)
			}
			if sessionSnapshot().MusicKey != "fixture-credential" {
				t.Fatal("search changed the account session")
			}
		})
	}
}

// Inner search rejections and malformed lists used to appear as successful empty searches.
func TestSearchResponseValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
		ok   bool
	}{
		{"rejected", `{"code":2001,"body":{"singer":[]},"meta":{"sum":0}}`, false},
		{"invalid status", `{"code":"invalid","body":{"singer":[]},"meta":{"sum":0}}`, false},
		{"object list", `{"code":0,"body":{"singer":{}},"meta":{"sum":0}}`, false},
		{"null list", `{"code":0,"body":{"singer":null},"meta":{"sum":0}}`, false},
		{"null entry", `{"code":0,"body":{"singer":[null]},"meta":{"sum":1}}`, false},
		{"missing list", `{"code":0,"body":{},"meta":{"sum":0}}`, false},
		{"invalid meta", `{"code":0,"body":{"singer":[]},"meta":[]}`, false},
		{"filtered empty", `{"code":0,"body":{"singer":[]},"meta":{"sum":0,"is_filter":-9}}`, false},
		{"anonymous filtered empty", `{"code":0,"body":{"singer":[]},"meta":{"sum":0,"is_filter":-12}}`, false},
		{"empty search", `{"code":0,"body":{"singer":[]},"meta":{"sum":0}}`, true},
		{"unfiltered empty search", `{"code":0,"body":{"singer":[]},"meta":{"sum":0,"is_filter":0}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				rpcReply(w, json.RawMessage(tc.data))
			})
			result, err := SearchPageSource("周杰伦", SearchArtists, 20, 0, SearchLite)
			if (err == nil) != tc.ok {
				t.Fatalf("error=%v, want success=%v", err, tc.ok)
			}
			if tc.name == "rejected" {
				var protocol *ProtocolError
				if !errors.As(err, &protocol) || protocol.Code != 2001 {
					t.Fatalf("lost search rejection: %v", err)
				}
			}
			if tc.name == "filtered empty" || tc.name == "anonymous filtered empty" {
				var filtered *SearchFilterError
				if !errors.As(err, &filtered) || filtered.Filter >= 0 {
					t.Fatalf("lost server filtering: %v", err)
				}
			}
			if tc.ok && (result.Len() != 0 || result.Total != 0) {
				t.Fatalf("not an empty search: %+v", result)
			}
		})
	}
}

// A rejected RPC must be surfaced instead of turning into an empty successful search.
func TestRPCRejectsBusinessAndGatewayCodes(t *testing.T) {
	for _, body := range []string{`{"code":0,"req_0":{"code":2001,"data":{}}}`, `{"code":1000,"req_0":{"code":0,"data":{}}}`, `{"code":0,"req_0":{"code":0,"subcode":12,"data":{}}}`} {
		t.Run(body, func(t *testing.T) {
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) })
			var out object
			err := rpc("test", "read", map[string]any{}, &out)
			var protocol *ProtocolError
			if !errors.As(err, &protocol) {
				t.Fatalf("got %v, want protocol error", err)
			}
		})
	}
}

// Legacy song album IDs must not accidentally become the song's own ID.
func TestLegacyTrackAlbum(t *testing.T) {
	track := convertTrack(obj(json.RawMessage(`{"songid":777,"songmid":"legacySong","songname":"Song","albumid":888,"albummid":"legacyAlbum","albumname":"Album","interval":1}`)))
	if track.Album.ID != 888 || track.Album.Name != "Album" || track.Album.MID != "legacyAlbum" {
		t.Fatalf("wrong legacy album: %+v", track.Album)
	}
}

// Singer album responses use albumMid, which must survive to artwork and album navigation.
func TestSingerAlbumMID(t *testing.T) {
	album := convertAlbum(obj(json.RawMessage(`{"albumID":87495226,"albumMid":"fixtureAlbum","albumName":"Fixture"}`)))
	if album.ID != 87495226 || album.MID != "fixtureAlbum" || album.PicURL == "" {
		t.Fatalf("lost album MID: %+v", album)
	}
}

// Login used to send the mobile client identity to the web discography RPC, which rejects it with 104403.
func TestArtistAlbumsKeepsWebIdentity(t *testing.T) {
	const artistID = 62001
	convertArtist(obj(json.RawMessage(`{"id":62001,"mid":"fixtureSinger","name":"Singer"}`)))
	for _, account := range []struct {
		name    string
		session Session
	}{
		{"anonymous", Session{}},
		{"QQ", Session{MusicID: "123", MusicKey: "fixture-credential", LoginType: 2}},
		{"WeChat", Session{MusicID: "123", MusicKey: "fixture-credential", LoginType: 1}},
	} {
		t.Run(account.name, func(t *testing.T) {
			expectedOffset := 0
			withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Comm map[string]any
					Req  struct {
						Module, Method string
						Param          map[string]any
					} `json:"req_0"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if request.Comm["ct"] != float64(24) || request.Comm["cv"] != float64(0) || request.Comm["qq"] != nil || request.Comm["authst"] != nil {
					_, _ = io.WriteString(w, `{"code":0,"req_0":{"code":104403,"data":{"albumList":[],"total":0}}}`)
					return
				}
				if request.Req.Module != "music.musichallAlbum.AlbumListServer" || request.Req.Method != "GetAlbumList" || request.Req.Param["singermid"] != "fixtureSinger" || request.Req.Param["num"] != float64(30) || request.Req.Param["sort"] != float64(5) {
					t.Errorf("wrong discography request: %+v", request)
				}
				if offset := request.Req.Param["begin"]; offset != float64(expectedOffset) {
					t.Errorf("offset = %v, want %d", offset, expectedOffset)
				}
				rpcReply(w, json.RawMessage(`{"albumList":[{"albumID":62002,"albumMid":"fixtureAlbum","albumName":"Album","publishDate":"2026-10-30","totalNum":8}],"total":2}`))
			})
			setSession(account.session)
			for offset := 0; offset < 2; offset++ {
				expectedOffset = offset
				albums, more, err := ArtistAlbums(artistID, 30, offset)
				if err != nil {
					t.Fatal(err)
				}
				if len(albums) != 1 || albums[0].ID != 62002 || albums[0].MID != "fixtureAlbum" || albums[0].Name != "Album" || albums[0].PicURL == "" || albums[0].Size != 8 || albums[0].PublishTime == 0 || more != (offset == 0) {
					t.Fatalf("lost album metadata or pagination: %+v, more=%v", albums, more)
				}
			}
			if sessionSnapshot().MusicKey != account.session.MusicKey {
				t.Fatal("discography changed the account session")
			}
		})
	}
}

// A private owned playlist's empty public response must fall back to the signed owner RPC.
func TestPrivatePlaylistUsesOwnerRead(t *testing.T) {
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, `{"code":0,"cdlist":[]}`)
			return
		}
		var request struct {
			Req struct {
				Module, Method string
				Param          map[string]any
			} `json:"req_0"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request.Req.Method != "CgiGetDiss" || request.Req.Param["disstid"] != float64(123) || request.Req.Param["enc_host_uin"] != "encrypted-owner" {
			t.Errorf("wrong owner read: %+v", request)
		}
		rpcReply(w, map[string]any{"dirinfo": map[string]any{"title": "Private"}, "total_song_num": 1, "songlist": []any{map[string]any{"id": 9, "mid": "privateSong", "title": "Track"}}, "hasmore": 0})
	})
	setSession(Session{MusicID: "123", MusicKey: "credential", LoginType: 2, EncryptUin: "encrypted-owner"})
	p, err := PlaylistDetail(123)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Tracks) != 1 || p.Tracks[0].MID != "privateSong" {
		t.Fatalf("private playlist lost songs: %+v", p)
	}
}

// Inner playlist rejections and malformed lists must not appear as a successfully empty playlist.
func TestSessionPlaylistRejectsInnerStatus(t *testing.T) {
	for _, kind := range []string{"owner", "algorithmic"} {
		for _, body := range []string{`{"code":1000,"songlist":[]}`, `{"subcode":80120,"songlist":[]}`, `{"code":0}`, `{"code":0,"songlist":{}}`} {
			t.Run(kind+body, func(t *testing.T) {
				withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
					var data any
					_ = json.Unmarshal([]byte(body), &data)
					rpcReply(w, data)
				})
				setSession(Session{MusicID: "123", MusicKey: "credential", LoginType: 2, EncryptUin: "owner"})
				var err error
				if kind == "owner" {
					_, err = ownedPlaylist(123)
				} else {
					_, err = authenticatedPlaylist(123)
				}
				if err == nil {
					t.Fatal("rejected/malformed playlist accepted")
				}
			})
		}
	}
}

// A known owned list goes directly to its signed API because private public shells can omit totals.
func TestOwnedPlaylistBypassesPublicShell(t *testing.T) {
	withQQServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Error("owned playlist queried through public CGI")
			_, _ = io.WriteString(w, `{"code":0,"cdlist":[{"total_song_num":0,"songlist":[]}]}`)
			return
		}
		rpcReply(w, map[string]any{"dirinfo": map[string]any{"title": "Private"}, "total_song_num": 1, "songlist": []any{map[string]any{"id": 10, "mid": "ownedSong", "title": "Track"}}, "hasmore": 0})
	})
	setSession(Session{MusicID: "456", MusicKey: "credential", LoginType: 2, EncryptUin: "encrypted-owner"})
	RememberOwnedPlaylist(124)
	p, err := PlaylistDetail(124)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Tracks) != 1 {
		t.Fatal("private playlist became empty")
	}
	setSession(Session{MusicID: "789", MusicKey: "other", LoginType: 2, EncryptUin: "other-owner"})
	if isOwnedPlaylist(124) {
		t.Fatal("playlist ownership leaked between accounts")
	}
}

// Arbitrary UI sizes used to produce QQ CDN 404s: 96 and 600 failed where the player's 120 worked.
func TestSizedArtwork(t *testing.T) {
	for _, kind := range []string{"T001", "T002"} {
		for _, size := range []struct{ request, want int }{
			{38, 90}, {90, 90}, {96, 120}, {120, 120}, {150, 150},
			{160, 180}, {180, 180}, {200, 300}, {240, 300}, {300, 300},
			{320, 500}, {500, 500}, {600, 800}, {800, 800}, {1600, 800},
		} {
			t.Run(fmt.Sprintf("%s/%d", kind, size.request), func(t *testing.T) {
				pic := "http://y.gtimg.cn/music/photo_new/" + kind + "R500x500M000album.jpg?max_age=2592000"
				want := fmt.Sprintf("https://y.gtimg.cn/music/photo_new/%sR%dx%dM000album.jpg?max_age=2592000", kind, size.want, size.want)
				if got := Sized(pic, size.request); got != want {
					t.Errorf("Sized(%d) = %q, want %q", size.request, got, want)
				}
			})
		}
	}
}
