package app

import "netease-music-mygo/internal/netease"

// Platform artwork loading keeps the first visible surface responsive without
// changing the shared artwork cache. A provider page can ask for one hero
// image and preheat the small covers that will be built immediately after it.
func (a *app) platformArtworkReady(url string, px int) bool {
	if a == nil || a.art == nil || url == "" {
		return true
	}
	return a.art.get(netease.Sized(url, px), false) != nil
}

func (a *app) prefetchPlatformArtwork(urls []string, px int) {
	if a == nil || a.art == nil {
		return
	}
	for _, raw := range urls {
		if raw != "" {
			a.art.get(netease.Sized(raw, px), false)
		}
	}
}
