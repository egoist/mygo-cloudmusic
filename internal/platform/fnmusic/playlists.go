package fnmusic

import (
	"bytes"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"

	"netease-music-mygo/internal/netease"
)

// The listener's playlists on the service: making them, naming them, giving
// them a cover, putting songs in and taking songs out. The service puts the
// song added last first, and takes a song that is in already for one that is
// to move there, so that adding is safe to ask twice; making a playlist is
// not, and is never asked over another route.

// CreatePlaylist makes a playlist named name, with no cover, and returns it.
func (c *Client) CreatePlaylist(name string) (netease.Playlist, error) {
	var out struct {
		GUID string `json:"guid"`
		Name string `json:"name"`
	}
	if err := c.callWith(false, "POST", "/playlist/create", nil, map[string]string{"name": name}, &out); err != nil {
		return netease.Playlist{}, err
	}
	if out.GUID == "" {
		return netease.Playlist{}, errors.New("fnmusic: the service made no playlist")
	}
	return netease.Playlist{ID: ID(out.GUID), Ref: out.GUID, Name: out.Name}, nil
}

// RenamePlaylist gives the playlist another name.
func (c *Client) RenamePlaylist(ref, name string) error {
	return c.call("POST", "/playlist/edit", nil, map[string]string{"guid": ref, "name": name}, nil)
}

// DeletePlaylist deletes the playlist, and not the songs in it.
func (c *Client) DeletePlaylist(ref string) error {
	return c.call("POST", "/playlist/delete", nil, map[string]string{"guid": ref}, nil)
}

// AddToPlaylist puts songs in a playlist.
func (c *Client) AddToPlaylist(ref string, songs []string) error {
	return c.call("POST", "/playlist/add-track", nil, map[string]any{"guid": ref, "trackGUIDs": songs}, nil)
}

// RemoveFromPlaylist takes songs out of a playlist.
func (c *Client) RemoveFromPlaylist(ref string, songs []string) error {
	return c.call("POST", "/playlist/remove-track", nil, map[string]any{"guid": ref, "trackGUIDs": songs}, nil)
}

// MaxCoverBytes is the largest picture taken for a cover.
const MaxCoverBytes = 8 << 20

// ErrBadCover is what a file that is no picture of a kind the service takes
// is refused with.
var ErrBadCover = errors.New("请选择 PNG、JPEG、WebP 或 GIF 图片（不超过 8 MB）")

// SetPlaylistCover uploads a picture and makes it the cover of the playlist,
// which has the name given: the service takes an edit only with the name,
// whatever is edited.
func (c *Client) SetPlaylistCover(ref, name string, picture []byte) (string, error) {
	kind := http.DetectContentType(picture)
	ext := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp", "image/gif": "gif"}[kind]
	if ext == "" || len(picture) == 0 || len(picture) > MaxCoverBytes {
		return "", ErrBadCover
	}
	// A form of one field, file, as the web player sends it.
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="cover.%s"`, ext))
	h.Set("Content-Type", kind)
	part, err := w.CreatePart(h)
	if err != nil {
		return "", err
	}
	part.Write(picture)
	w.Close()
	data := body.Bytes()

	var out struct {
		CoverID string `json:"coverId"`
		GUID    string `json:"guid"`
	}
	err = c.callBody(false, "POST", "/static/cover/playlist", nil, func(token string) (*http.Request, error) {
		return c.requestWith("POST", "/static/cover/playlist", nil, data, w.FormDataContentType(), "{}", token)
	}, &out)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(out.CoverID)
	if id == "" {
		id = strings.TrimSpace(out.GUID)
	}
	if id == "" {
		return "", errors.New("fnmusic: the service gave no cover")
	}
	if err := c.call("POST", "/playlist/edit", nil, map[string]string{"guid": ref, "name": name, "coverId": id}, nil); err != nil {
		return "", err
	}
	return id, nil
}

// CoverURL is the address a cover is fetched by, as the app asks for one.
func CoverURL(id string) string { return coverURL(id) }
