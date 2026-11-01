package app

import (
	"context"

	"netease-music-mygo/internal/netease"
)

// Platform like extension seam.
//
// Keep provider-owned favorite operations behind the shared adapter. The
// upstream like flow should only call these helpers and must not know a
// provider's session or protocol types.

func (a *app) togglePlatformLike(tr *netease.Track) bool {
	adapter := a.trackAdapter(tr)
	if adapter == nil {
		return false
	}
	liked := adapter.Liked(tr)
	go func() {
		err := adapter.Favorite(context.Background(), tr, !liked)
		a.post(func() {
			if err != nil {
				a.toast = "操作失败，请稍后再试"
			}
		})
	}()
	return true
}

func (a *app) platformLiked(tr *netease.Track) (bool, bool) {
	adapter := a.trackAdapter(tr)
	if adapter == nil {
		return false, false
	}
	return adapter.Liked(tr), true
}

func (a *app) platformLikeAllowed(tr *netease.Track) bool {
	return a.trackAdapter(tr) != nil
}
