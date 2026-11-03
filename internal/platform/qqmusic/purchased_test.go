package qqmusic

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"testing"
)

func fakePurchasedCGI(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	fakeAuthServer(t, handler)
	old := httpClient
	httpClient = authHTTP
	t.Cleanup(func() { httpClient = old })
	setSession(Session{MusicID: "10001", MusicKey: "fake-music-key", LoginType: 2})
}

// Purchased songs use the music session and continue by item offset while preserving playable identity.
func TestPurchasedSongsPages(t *testing.T) {
	requests := 0
	fakePurchasedCGI(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		q := r.URL.Query()
		if r.Method != http.MethodGet || r.Host != "c.y.qq.com" || r.URL.Path != purchasedPath {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		for key, want := range map[string]string{"from": "1", "cmd": "sales_album", "type": "2", "uin": "10001", "num": "1", "platform": "yqq", "format": "json", "g_tk": "665448288", "g_tk_new_20200303": "665448288"} {
			if q.Get(key) != want {
				t.Errorf("%s=%q; want %q", key, q.Get(key), want)
			}
		}
		for key, want := range map[string]string{"uin": "10001", "qqmusic_uin": "10001", "qm_keyst": "fake-music-key", "qqmusic_key": "fake-music-key"} {
			cookie, err := r.Cookie(key)
			if err != nil || cookie.Value != want {
				t.Errorf("missing account cookie %s", key)
			}
		}
		offset, _ := strconv.Atoi(q.Get("start"))
		if offset != requests-1 {
			t.Errorf("offset=%d; want %d", offset, requests-1)
		}
		_, _ = fmt.Fprintf(w, `{"code":0,"data":{"total":2,"songlist":[{"songid":%d,"songmid":"purchased-song-%d","songname":"Song","albummid":"album-mid","albumid":71,"albumname":"Album","interval":123,"singer":[{"id":3,"mid":"artist-mid","name":"Singer"}],"file":{"media_mid":"media-mid","size_320mp3":12345}}]}}`, 501+offset, offset)
	})
	for offset := 0; offset < 2; offset++ {
		songs, more, err := PurchasedSongs(1, offset)
		if err != nil || len(songs) != 1 || more != (offset == 0) {
			t.Fatalf("page %d: songs=%v more=%v err=%v", offset, songs, more, err)
		}
		song := songs[0]
		if song.ID != int64(501+offset) || song.MID != fmt.Sprintf("purchased-song-%d", offset) || song.MediaMID != "media-mid" || song.Size320 != 12345 || song.Duration != 123000 || song.Album.MID != "album-mid" || len(song.Artists) != 1 {
			t.Fatalf("lost song metadata: %+v", song)
		}
		remembered, ok := knownTrack(song.ID)
		if !ok || remembered.MediaMID != song.MediaMID {
			t.Fatal("purchased song identity not registered for playback")
		}
	}
}

// Purchased albums retain the MID needed to open their existing detail page, including legacy order fields.
func TestPurchasedAlbumsIdentity(t *testing.T) {
	fakePurchasedCGI(t, func(w http.ResponseWriter, r *http.Request) {
		if q := r.URL.Query(); q.Get("type") != "1" || q.Get("start") != "3" || q.Get("num") != "2" {
			t.Errorf("incorrect album page: %s", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"code":0,"data":{"total":5,"albumlist":[{"albumID":701,"albumMid":"album-order-mid","albumName":"Album","singerid":3,"singermid":"artist-mid","singerName":"Singer","publishDate":"2025-01-02"},{"id":702,"pmid":"photo-fallback-mid","name":"Second","singers":[{"id":4,"mid":"artist-two","name":"Two"}]}]}}`)
	})
	albums, more, err := PurchasedAlbums(2, 3)
	if err != nil || len(albums) != 2 || more {
		t.Fatalf("albums=%v more=%v err=%v", albums, more, err)
	}
	if albums[0].ID != 701 || albums[0].MID != "album-order-mid" || albumMID(701) != "album-order-mid" || albums[0].Artists[0].Name != "Singer" || albums[0].PublishTime == 0 || albums[1].MID != "photo-fallback-mid" || albums[1].Artists[0].MID != "artist-two" {
		t.Fatalf("lost album metadata: %+v", albums)
	}
}

// An unauthenticated or rejected purchase request must never look like an empty library.
func TestPurchasedRequiresLoginAndRejectsInvalidData(t *testing.T) {
	responses := []string{
		`{"code":1000,"subcode":1000}`,
		`{"code":0,"data":{"code":104401,"songlist":[],"total":0}}`,
		`{"code":0}`,
		`{"code":0,"data":{"total":1}}`,
		`{"code":0,"data":{"total":1,"songlist":null}}`,
		`{"code":0,"data":{"total":0,"songlist":{}}}`,
		`{"code":0,"data":{"songlist":[]}}`,
		`{"code":0,"data":{"total":-1,"songlist":[]}}`,
		`{"code":0,"data":{"total":1,"songlist":[]}}`,
		`{"code":0,"data":{"total":1,"songlist":[null]}}`,
		`{"code":0,"data":{"total":1,"songlist":[{"songid":1}]}}`,
	}
	requests := 0
	body := responses[0]
	fakePurchasedCGI(t, func(w http.ResponseWriter, r *http.Request) { requests++; _, _ = io.WriteString(w, body) })
	setSession(Session{})
	if _, _, err := PurchasedSongs(30, 0); err == nil || requests != 0 {
		t.Fatal("unauthenticated purchase request reached the network")
	}
	if _, _, err := PurchasedAlbums(30, 0); err == nil || requests != 0 {
		t.Fatal("unauthenticated album request reached the network")
	}
	setSession(Session{MusicID: "10001", MusicKey: "fake-music-key", LoginType: 2})
	for i, response := range responses {
		body = response
		if _, _, err := PurchasedSongs(30, 0); err == nil {
			t.Errorf("accepted invalid response %d", i)
		} else if i == 0 {
			var protocol *ProtocolError
			if !errors.As(err, &protocol) || protocol.Code != 1000 {
				t.Errorf("lost authentication rejection: %v", err)
			}
		}
	}
	for _, response := range []string{`{"code":0,"data":{"total":1,"albumlist":null}}`, `{"code":0,"data":{"total":1}}`, `{"code":0,"data":{"total":0,"albumlist":{}}}`, `{"code":0,"data":{"total":1,"albumlist":[null]}}`, `{"code":0,"data":{"total":1,"albumlist":[{"id":1}]}}`} {
		body = response
		if _, _, err := PurchasedAlbums(30, 0); err == nil {
			t.Error("accepted invalid album response")
		}
	}
	for _, response := range []string{`{"code":0,"data":{"total":0,"songlist":[],"albumlist":[]}}`, `{"code":0,"data":{"total":0}}`, `{"code":0,"data":{"total":0,"songlist":null,"albumlist":null}}`} {
		body = response
		if songs, more, err := PurchasedSongs(30, 0); err != nil || len(songs) != 0 || more {
			t.Fatalf("valid empty purchases rejected: %v", err)
		}
		if albums, more, err := PurchasedAlbums(30, 0); err != nil || len(albums) != 0 || more {
			t.Fatalf("valid empty albums rejected: %v", err)
		}
	}
}
