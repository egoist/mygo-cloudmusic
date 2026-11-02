package fnmusic

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"netease-music-mygo/internal/netease"
)

// FolderNode is a directory returned as a child of a folder page.
type FolderNode struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	ItemCount      int    `json:"itemCount"`
	IsAdminDeleted bool   `json:"isAdminDeleted"`
	ChildrenLoaded bool   `json:"-"`
}

// FolderTrack keeps file metadata alongside the shared track model.
type FolderTrack struct {
	Track          netease.Track
	Path           string
	Size           int64
	CreatedAt      int64
	UpdatedAt      int64
	IsAdminDeleted bool
}

// FolderPage is the direct content of one directory. ItemCount may include
// descendants and must not be used as the number of rows in this page.
type FolderPage struct {
	Name      string
	Path      string
	ItemCount int
	Folders   []FolderNode
	Tracks    []FolderTrack
	Sort      string
}

type folderPageResponse struct {
	Folder struct {
		Name      string       `json:"name"`
		Path      string       `json:"path"`
		ItemCount int          `json:"itemCount"`
		Folders   []FolderNode `json:"folders"`
		Tracks    []rawTrack   `json:"tracks"`
	} `json:"folder"`
	Sort string `json:"sort"`
}

// FolderPage returns the direct folders and tracks below parentPath. Paths
// are sent unchanged because they identify the library root and its tree.
func (c *Client) FolderPage(parentPath, sort string) (FolderPage, error) {
	parentPath = strings.TrimSpace(parentPath)
	if parentPath == "" {
		return FolderPage{}, errors.New("fnmusic: empty folder path")
	}
	q := url.Values{"parentPath": {parentPath}}
	if sort != "" && sort != SortFolderTitleAsc {
		q.Set("sort", sort)
	}
	var out folderPageResponse
	if err := c.call("GET", "/folder-view/list", q, nil, &out); err != nil {
		return FolderPage{}, err
	}
	page := FolderPage{
		Name: out.Folder.Name, Path: out.Folder.Path, ItemCount: out.Folder.ItemCount,
		Folders: out.Folder.Folders, Sort: out.Sort,
	}
	if page.Sort == "" {
		page.Sort = SortFolderTitleAsc
	}
	for _, raw := range out.Folder.Tracks {
		track, ok := raw.track()
		if !ok {
			continue
		}
		path := raw.AudioSpec.Path
		if path == "" {
			path = raw.Path
		}
		page.Tracks = append(page.Tracks, FolderTrack{
			Track: track, Path: path, Size: raw.AudioSpec.Size,
			CreatedAt: raw.CreatedAt, UpdatedAt: raw.UpdatedAt,
			IsAdminDeleted: raw.IsAdminDeleted,
		})
	}
	return page, nil
}

const (
	SortFolderTitleAsc    = "title,asc"
	SortFolderTitleDesc   = "title,desc"
	SortFolderCreatedAsc  = "createdAt,asc"
	SortFolderCreatedDesc = "createdAt,desc"
	SortFolderUpdatedAsc  = "updatedAt,asc"
	SortFolderUpdatedDesc = "updatedAt,desc"
)

// PlayableFolderTracks asks the service to recursively resolve folders and
// returns the tracks it can play. The service owns recursion and limits.
func (c *Client) PlayableFolderTracks(folderPaths, trackGUIDs []string) ([]netease.Track, error) {
	var raw json.RawMessage
	payload := map[string]any{"folderPaths": folderPaths, "trackGUIDs": trackGUIDs}
	if err := c.call("POST", "/folder-view/playable-tracks", nil, payload, &raw); err != nil {
		return nil, err
	}
	return decodeFolderTracks(raw)
}

// AddFoldersToPlaylist adds direct tracks and recursively resolved folders to
// a Feiniu playlist.
func (c *Client) AddFoldersToPlaylist(playlistGUID string, folderPaths, trackGUIDs []string) error {
	payload := map[string]any{"folderPaths": folderPaths, "playlistGUID": playlistGUID, "trackGUIDs": trackGUIDs}
	return c.call("POST", "/folder-view/add-to-playlist", nil, payload, nil)
}

// DeleteFolderItems deletes selected folders or tracks. Permanent controls
// whether the service skips its recycle-bin behavior.
func (c *Client) DeleteFolderItems(folderPaths, trackGUIDs []string, permanent bool) error {
	payload := map[string]any{"folderPaths": folderPaths, "trackGUIDs": trackGUIDs, "isPermanent": permanent}
	return c.call("POST", "/folder-view/delete", nil, payload, nil)
}

// RecoverFolderItems restores selected folders or tracks from the recycle bin.
func (c *Client) RecoverFolderItems(folderPaths, trackGUIDs []string) error {
	payload := map[string]any{"folderPaths": folderPaths, "trackGUIDs": trackGUIDs}
	return c.call("POST", "/folder-view/recover", nil, payload, nil)
}

func decodeFolderTracks(raw json.RawMessage) ([]netease.Track, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var list []rawTrack
	if raw[0] == '[' {
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, err
		}
	} else {
		var wrapper struct {
			Tracks []rawTrack `json:"tracks"`
			List   []rawTrack `json:"list"`
			Data   struct {
				Tracks []rawTrack `json:"tracks"`
				List   []rawTrack `json:"list"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &wrapper); err != nil {
			return nil, err
		}
		list = wrapper.Tracks
		if len(list) == 0 {
			list = wrapper.List
		}
		if len(list) == 0 {
			list = wrapper.Data.Tracks
		}
		if len(list) == 0 {
			list = wrapper.Data.List
		}
	}
	out := make([]netease.Track, 0, len(list))
	for _, item := range list {
		if track, ok := item.track(); ok {
			out = append(out, track)
		}
	}
	return out, nil
}

// FolderSortValues returns the service-supported sort values for UI controls.
func FolderSortValues() []string {
	return []string{SortFolderTitleAsc, SortFolderTitleDesc, SortFolderCreatedAsc, SortFolderCreatedDesc, SortFolderUpdatedAsc, SortFolderUpdatedDesc}
}

func folderSortField(sort string) (string, string, bool) {
	parts := strings.Split(sort, ",")
	if len(parts) != 2 || (parts[1] != "asc" && parts[1] != "desc") {
		return "", "", false
	}
	switch parts[0] {
	case "title", "createdAt", "updatedAt":
		return parts[0], parts[1], true
	default:
		return "", "", false
	}
}

// ParseFolderSort normalizes persisted sort values to the server's accepted
// form, defaulting invalid values to title ascending.
func ParseFolderSort(sort string) string {
	if _, _, ok := folderSortField(sort); ok {
		return sort
	}
	return SortFolderTitleAsc
}
