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
