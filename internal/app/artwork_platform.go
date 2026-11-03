package app

import (
	"image"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// Artwork platform extension seam.
//
// Keep provider artwork fetching and request-generation state outside the
// upstream art cache declaration. The cache itself depends only on this
// small provider-neutral extension state.
type artPlatformState struct {
	fetch      func(string) ([]byte, error)
	generation func() uint64
}

func newArtPlatformState() artPlatformState {
	return artPlatformState{fetch: netease.Fetch, generation: func() uint64 { return 0 }}
}

func loadArtWith(url string, fetch func(string) ([]byte, error)) (*ui.Bitmap, image.Image, int, error) {
	if fetch == nil {
		fetch = netease.Fetch
	}
	b, err := fetch(url)
	if err != nil {
		return nil, nil, 0, err
	}
	bmp, img, err := decodeArt(b)
	return bmp, img, len(b), err
}

func (c *artCache) artworkGeneration() uint64 {
	if c.generation == nil {
		return 0
	}
	return c.generation()
}

func (c *artCache) discardStaleArtwork(gen uint64, bmp *ui.Bitmap) bool {
	if c.generation == nil || c.generation() == gen {
		return false
	}
	if bmp != nil {
		bmp.ReleasePixels()
	}
	return true
}
