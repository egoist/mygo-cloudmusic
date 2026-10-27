package app

import (
	"errors"
	"os"
	"strconv"
	"testing"
	"time"
)

// An animated WebP cover, as some playlists have, shows its first frame,
// in the bitmap too, which decodes its data again once it drops its pixels.
func TestAnimatedWebPCover(t *testing.T) {
	b, err := os.ReadFile("testdata/animated.webp")
	if err != nil {
		t.Fatal(err)
	}
	bmp, img, err := decodeArt(b)
	if err != nil {
		t.Fatal(err)
	}
	if r := img.Bounds(); r.Dx() != 24 || r.Dy() != 24 {
		t.Errorf("first frame is %v, want 24×24", r)
	}
	if w, h := bmp.Size(); w != 24 || h != 24 {
		t.Errorf("bitmap is %d×%d, want 24×24", w, h)
	}
}

// Covers not asked for in a while go, so that a long session does not
// keep every cover it showed; the ones still showing stay.
func TestArtCacheDropsUnusedCovers(t *testing.T) {
	now := time.Unix(1e9, 0)
	c := newArtCache(func(func()) {})
	c.now, c.swept = func() time.Time { return now }, now
	c.art["shown"] = &artwork{used: now}
	c.art["gone"] = &artwork{used: now}
	for range 7 {
		now = now.Add(time.Minute)
		c.get("shown", false)
	}
	if _, ok := c.art["gone"]; ok {
		t.Error("kept a cover not asked for in 7 minutes")
	}
	if _, ok := c.art["shown"]; !ok {
		t.Error("dropped a cover asked for every minute")
	}
}

// The covers showing when the window went idle stay for its next frame.
// They went: nothing asks for covers while the window draws nothing, so
// after 5 minutes away the first frame, as the window was focused again,
// dropped every cover on the page, which flashed as they loaded again.
func TestArtCacheKeepsCoversWhileIdle(t *testing.T) {
	now := time.Unix(1e9, 0)
	c := newArtCache(func(func()) {})
	c.now, c.swept = func() time.Time { return now }, now
	c.art["shown"] = &artwork{used: now}
	c.get("shown", false)
	now = now.Add(10 * time.Minute)
	if c.get("shown", false) == nil {
		t.Error("dropped the covers showing after 10 minutes idle")
	}
}

// The covers a page shows fetch together, not 8 at a time as they decode:
// far from the CDN each is a round trip most of a second long, and the
// wall's 40 took 3 seconds in rounds of 8.
func TestArtCacheFetchesCoversTogether(t *testing.T) {
	const n = 20
	started, release := make(chan struct{}, n), make(chan struct{})
	defer func(f func(string) ([]byte, error)) { fetchCover = f }(fetchCover)
	fetchCover = func(string) ([]byte, error) {
		started <- struct{}{}
		<-release
		return nil, errors.New("offline")
	}
	defer close(release)
	c := newArtCache(func(func()) {})
	for i := range n {
		c.get(strconv.Itoa(i), false)
	}
	for i := range n {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatalf("%d of %d covers fetching at once", i, n)
		}
	}
}
