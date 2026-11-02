package fnmusic

// SharedLibrary is one configured music-library root.
type SharedLibrary struct {
	GUID                 string `json:"guid"`
	Name                 string `json:"name"`
	Path                 string `json:"path"`
	AutoDownloadLyric    bool   `json:"autoDownloadLyric"`
	MetadataPreference   string `json:"metadataPreference"`
	ContentLastChangedAt int64  `json:"contentLastChangedAt"`
	AccessStatus         int    `json:"accessStatus"`
}

// SharedLibraries returns all configured music-library roots.
func (c *Client) SharedLibraries() ([]SharedLibrary, error) {
	var out struct {
		List []SharedLibrary `json:"list"`
	}
	if err := c.call("GET", "/shared-library/list", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.List, nil
}

// AuthorizedDirectory is a directory the music app may browse. Remote mounts
// carry the display metadata the web player uses for their root label.
type AuthorizedDirectory struct {
	Path             string `json:"path"`
	StorageType      int    `json:"storageType"`
	CloudStorageType int    `json:"cloudStorageType"`
	Comment          string `json:"comment"`
	Username         string `json:"username"`
	Address          string `json:"address"`
	Uname            string `json:"uname"`
}

// AuthorizedDirectories returns the authorized directory roots.
func (c *Client) AuthorizedDirectories() ([]AuthorizedDirectory, error) {
	var out struct {
		List []AuthorizedDirectory `json:"list"`
	}
	if err := c.call("GET", "/app-center/authed-dir/list", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.List, nil
}
