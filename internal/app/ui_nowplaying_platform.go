package app

import "netease-music-mygo/internal/netease"

// Now Playing platform extension seam.
//
// Keep the upstream pane layout and rendering in ui_nowplaying.go. Platform
// capabilities may prepare or filter that layout through these small hooks;
// this file must not contain provider protocol types or provider UI.

func (a *app) preparePlatformNowPlaying(tr *netease.Track) {
	if a.commentsEnabled() {
		a.commentsFor(tr)
	}
	if !a.commentsEnabled() && a.npPane == paneComments {
		a.npPane = paneLyrics
	}
}

func (a *app) appendPlatformNPTabs(tabs []npTab) []npTab {
	if a.commentsEnabled() {
		return append(tabs, npTab{paneComments, "评论"})
	}
	return tabs
}
