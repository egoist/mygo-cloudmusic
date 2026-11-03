package qqmusic

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

// Favorite albums use the credential-bearing profile CGI and inclusive page ranges, preserving album identity.
func TestLikedAlbumsPages(t *testing.T) {
	requests := 0
	fakePurchasedCGI(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != likedAlbumsPath || q.Get("reqtype") != "2" || q.Get("userid") != "10001" || q.Get("sin") != fmt.Sprint(requests) || q.Get("ein") != fmt.Sprint(requests) {
			t.Errorf("unexpected album page: %s", r.URL)
		}
		if cookie, err := r.Cookie("qm_keyst"); err != nil || cookie.Value != "fake-music-key" {
			t.Error("missing account credential")
		}
		requests++
		_, _ = fmt.Fprintf(w, `{"code":0,"data":{"totalalbum":2,"albumlist":[{"albumid":%d,"albummid":"AlbumMID%d","albumname":"Album","singername":"Singer","pubtime":"2025-01-02"}]}}`, requests, requests)
	})
	for offset := 0; offset < 2; offset++ {
		albums, more, err := LikedAlbums(1, offset)
		if err != nil || len(albums) != 1 || more != (offset == 0) {
			t.Fatalf("page %d: albums=%+v more=%v err=%v", offset, albums, more, err)
		}
		album := albums[0]
		if album.ID != int64(offset+1) || album.MID != fmt.Sprintf("AlbumMID%d", offset+1) || albumMID(album.ID) != album.MID || album.ArtistNames() != "Singer" || album.PublishTime == 0 {
			t.Fatalf("lost album metadata: %+v", album)
		}
	}
}

// Rejected or truncated favorite responses must not be presented as an empty collection.
func TestLikedAlbumsRejectsInvalidData(t *testing.T) {
	body := ""
	requests := 0
	fakePurchasedCGI(t, func(w http.ResponseWriter, r *http.Request) { requests++; _, _ = io.WriteString(w, body) })
	setSession(Session{})
	if _, _, err := LikedAlbums(20, 0); err == nil || requests != 0 {
		t.Fatal("signed-out album request reached the network")
	}
	setSession(Session{MusicID: "10001", MusicKey: "fake", LoginType: 2})
	for _, response := range []string{
		`{"code":4000}`, `{"code":0}`, `{"code":0,"data":{}}`,
		`{"code":0,"data":{"totalalbum":1,"albumlist":[]}}`,
		`{"code":0,"data":{"totalalbum":1,"albumlist":[null]}}`,
	} {
		body = response
		if _, _, err := LikedAlbums(20, 0); err == nil {
			t.Errorf("accepted malformed collection: %s", response)
		}
	}
	body = `{"code":0,"data":{"totalalbum":0,"albumlist":[]}}`
	if albums, more, err := LikedAlbums(20, 0); err != nil || len(albums) != 0 || more {
		t.Fatalf("rejected empty collection: %v", err)
	}
}

// Album writes use MIDs, avoid redundant mutations and confirm the result after exactly one write.
func TestLikeAlbumWritesAndVerifies(t *testing.T) {
	for _, want := range []bool{true, false} {
		t.Run(fmt.Sprint(want), func(t *testing.T) {
			liked, writes := !want, 0
			fakeLibraryModuleRPC(t, func(module, method string, param map[string]json.RawMessage) any {
				if string(param["v_albumMid"]) != `["AlbumMID"]` {
					t.Error("did not use album MID")
				}
				switch method {
				case "IsAlbumFan":
					if module != "music.musicasset.AlbumFavRead" || string(param["uin"]) != "10001" {
						t.Errorf("wrong status owner/module: %s %v", module, param)
					}
					return map[string]any{"m_fan": map[string]bool{"AlbumMID": liked}}
				case "FavAlbum", "CancelFavAlbum":
					if module != "music.musicasset.AlbumFavWrite" || string(param["uin"]) != `"10001"` || (method == "FavAlbum") != want {
						t.Errorf("wrong write: %s %s %v", module, method, param)
					}
					writes++
					liked = want
					return map[string]any{"result": 0, "v_failedAlbumId": []int{}, "v_failedAlbumMid": []string{}}
				default:
					t.Errorf("unexpected method %s", method)
					return nil
				}
			})
			for range 2 {
				if err := LikeAlbum("AlbumMID", want); err != nil {
					t.Fatal(err)
				}
			}
			if writes != 1 {
				t.Fatalf("writes = %d", writes)
			}
		})
	}
}

// Unknown status, incomplete write results and failed readback cannot report a successful album mutation.
func TestLikeAlbumRejectsUnconfirmedWrites(t *testing.T) {
	for _, mode := range []string{"unknown", "missing result", "failed album", "unchanged"} {
		t.Run(mode, func(t *testing.T) {
			writes := 0
			fakeLibraryRPC(t, func(method string, _ map[string]json.RawMessage) any {
				if method == "IsAlbumFan" {
					if mode == "unknown" {
						return map[string]any{"m_fan": map[string]bool{}}
					}
					return map[string]any{"m_fan": map[string]bool{"AlbumMID": false}}
				}
				writes++
				if mode == "missing result" {
					return map[string]any{"result": 0}
				}
				failed := []string{}
				if mode == "failed album" {
					failed = append(failed, "AlbumMID")
				}
				return map[string]any{"result": 0, "v_failedAlbumId": []int{}, "v_failedAlbumMid": failed}
			})
			if err := LikeAlbum("AlbumMID", true); err == nil {
				t.Fatal("accepted unconfirmed mutation")
			}
			if mode == "unknown" && writes != 0 || writes > 1 {
				t.Fatalf("writes = %d", writes)
			}
		})
	}
}
