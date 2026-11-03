package app

import (
	"os"
	"testing"
)

// Configured artwork fetchers handle platform-private cover URLs before decoding.
func TestLoadArtWithPlatformFetcher(t *testing.T) {
	b, err := os.ReadFile("testdata/animated.webp")
	if err != nil {
		t.Fatal(err)
	}
	called := ""
	_, img, _, err := loadArtWith("fnmusic://cover/abc", func(url string) ([]byte, error) { called = url; return b, nil })
	if err != nil {
		t.Fatal(err)
	}
	if called != "fnmusic://cover/abc" || img.Bounds().Dx() != 24 {
		t.Fatalf("fetch=%q image=%v", called, img.Bounds())
	}
}

// A cover response from the previous platform is discarded after a platform switch.
func TestArtCacheDropsLatePlatformResponse(t *testing.T) {
	b, err := os.ReadFile("testdata/animated.webp")
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	posted := make(chan func(), 1)
	generation := uint64(1)
	c := newArtCache(func(fn func()) { posted <- fn })
	c.generation = func() uint64 { return generation }
	c.fetch = func(string) ([]byte, error) {
		close(started)
		<-release
		return b, nil
	}
	c.get("fnmusic://cover/old", false)
	<-started
	generation = 2
	close(release)
	postedFn := <-posted
	postedFn()
	if _, ok := c.art["fnmusic://cover/old"]; ok {
		t.Fatal("late cover response populated the new platform cache")
	}
}
