package fnmusic

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// Folder endpoints preserve full paths, decode direct tracks, and send the
// documented operation bodies.
func TestFolderEndpoints(t *testing.T) {
	s := &folderTestServer{}
	ts := httptest.NewServer(s)
	defer ts.Close()
	c, err := New(Config{Address: ts.URL, Username: "me", PassHash: HashPassword("secret")})
	if err != nil {
		t.Fatal(err)
	}

	caps, err := c.Capabilities()
	if err != nil || !caps.FolderView {
		t.Fatalf("capabilities = %+v, %v", caps, err)
	}
	roots, err := c.SharedLibraries()
	if err != nil || len(roots) != 1 || roots[0].Path != "/music" {
		t.Fatalf("roots = %+v, %v", roots, err)
	}
	page, err := c.FolderPage("/music/Adele", SortFolderTitleAsc)
	if err != nil || page.Path != "/music/Adele" || len(page.Folders) != 1 || len(page.Tracks) != 1 {
		t.Fatalf("page = %+v, %v", page, err)
	}
	if page.Tracks[0].Track.Name != "Hello" || page.Tracks[0].Size != 640 {
		t.Fatalf("track = %+v", page.Tracks[0])
	}
	tracks, err := c.PlayableFolderTracks([]string{"/music/Adele"}, []string{"track-1"})
	if err != nil || len(tracks) != 1 || tracks[0].Ref != "track-1" {
		t.Fatalf("playable = %+v, %v", tracks, err)
	}
	if err := c.AddFoldersToPlaylist("playlist-1", []string{"/music/Adele"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteFolderItems(nil, []string{"track-1"}, false); err != nil {
		t.Fatal(err)
	}
	if err := c.RecoverFolderItems([]string{"/music/Adele"}, nil); err != nil {
		t.Fatal(err)
	}
	if len(s.bodies) != 4 {
		t.Fatalf("recorded %d operation bodies", len(s.bodies))
	}
	var add map[string]any
	json.Unmarshal(s.bodies[1], &add)
	if add["playlistGUID"] != "playlist-1" {
		t.Fatalf("add body = %s", s.bodies[0])
	}
	var del map[string]any
	json.Unmarshal(s.bodies[2], &del)
	if del["isPermanent"] != false {
		t.Fatalf("delete body = %s", s.bodies[1])
	}
	if got := s.query.Get("parentPath"); got != "/music/Adele" {
		t.Errorf("parentPath = %q", got)
	}
}

// Playable folder responses may be returned as a direct list or nested in a
// data object by different Feiniu service versions.
func TestDecodeFolderTracksShapes(t *testing.T) {
	for name, raw := range map[string]string{
		"direct": `[{"guid":"track-1","title":"Hello"}]`,
		"tracks": `{"tracks":[{"guid":"track-1","title":"Hello"}]}`,
		"list":   `{"list":[{"guid":"track-1","title":"Hello"}]}`,
		"nested": `{"data":{"tracks":[{"guid":"track-1","title":"Hello"}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			tracks, err := decodeFolderTracks(json.RawMessage(raw))
			if err != nil || len(tracks) != 1 || tracks[0].Ref != "track-1" {
				t.Fatalf("tracks = %+v, err = %v", tracks, err)
			}
		})
	}
}

type folderTestServer struct {
	server
	query  url.Values
	bodies [][]byte
}

func (s *folderTestServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/music/api/v1")
	switch p {
	case "/sys/config":
		s.reply(w, map[string]any{"capabilities": map[string]bool{"folderView": true}})
		return
	case "/shared-library/list":
		s.reply(w, map[string]any{"list": []map[string]any{{"guid": "root", "path": "/music", "accessStatus": 0}}})
		return
	case "/folder-view/list":
		s.query = r.URL.Query()
		s.reply(w, map[string]any{"folder": map[string]any{
			"name": "Adele", "path": "/music/Adele", "itemCount": 2,
			"folders": []map[string]any{{"name": "Live", "path": "/music/Adele/Live", "itemCount": 1}},
			"tracks":  []map[string]any{{"guid": "track-1", "title": "Hello", "artists": []map[string]any{{"name": "Adele"}}, "audioSpec": map[string]any{"format": "flac", "path": "/music/Adele/Hello.flac", "size": 640}}},
		}, "sort": SortFolderTitleAsc})
		return
	case "/folder-view/playable-tracks":
		s.recordBody(w, r)
		s.reply(w, map[string]any{"tracks": []map[string]any{{"guid": "track-1", "title": "Hello", "audioSpec": map[string]any{"format": "flac"}}}})
		return
	case "/folder-view/add-to-playlist", "/folder-view/delete", "/folder-view/recover":
		s.recordBody(w, r)
		s.reply(w, map[string]any{})
		return
	}
	s.server.ServeHTTP(w, r)
}

func (s *folderTestServer) recordBody(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
	}
	s.bodies = append(s.bodies, body)
}

func (s *folderTestServer) reply(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"code": 0, "msg": "", "data": data})
}
