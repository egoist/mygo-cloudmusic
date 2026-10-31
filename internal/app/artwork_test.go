package app

import (
	"os"
	"runtime"
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

// A cover swept while something still holds it comes back as it was. It
// loaded again: Now Playing's cover went after 5 minutes on other pages,
// though Now Playing still held it, and flashed in once it opened again.
func TestArtCacheKeepsHeldCovers(t *testing.T) {
	now := time.Unix(1e9, 0)
	c := newArtCache(func(func()) {})
	c.now, c.swept = func() time.Time { return now }, now
	held := &artwork{used: now}
	c.art["now playing"] = held
	c.art["other"] = &artwork{used: now}
	for range 7 {
		now = now.Add(time.Minute)
		c.get("other", false)
	}
	if _, ok := c.art["now playing"]; ok {
		t.Fatal("kept a cover not asked for in 7 minutes")
	}
	if got := c.get("now playing", false); got != held {
		t.Error("loaded again a cover something still held")
	}
	runtime.KeepAlive(held)
}
