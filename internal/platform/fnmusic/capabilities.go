package fnmusic

// FolderCapabilities describes optional features reported by the music
// service. Older Feiniu Music versions omit these fields.
type FolderCapabilities struct {
	FolderView       bool `json:"folderView"`
	LibraryReconnect bool `json:"libraryReconnect"`
}

// Capabilities returns the server capabilities used by the platform UI.
func (c *Client) Capabilities() (FolderCapabilities, error) {
	var out struct {
		Capabilities FolderCapabilities `json:"capabilities"`
	}
	if err := c.call("GET", "/sys/config", nil, nil, &out); err != nil {
		return FolderCapabilities{}, err
	}
	return out.Capabilities, nil
}
