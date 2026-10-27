package app

import "github.com/egoist/mygo/ui"

// Icons drawn on a 24-point grid, in the color of the text: transport
// controls filled, the rest in a 1.75-point stroke.

func strokeIcon(paths string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">` + paths + `</svg>`))
}

func fillIcon(paths string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor">` + paths + `</svg>`))
}

var (
	iconPlay  = fillIcon(`<path d="M7 4.6v14.8c0 .8.9 1.3 1.6.9l12-7.4c.6-.4.6-1.3 0-1.7l-12-7.4C7.9 3.3 7 3.8 7 4.6z"/>`)
	iconPause = fillIcon(`<rect x="5.5" y="4" width="4.5" height="16" rx="1.4"/><rect x="14" y="4" width="4.5" height="16" rx="1.4"/>`)
	iconNext  = fillIcon(`<path d="M3 5.6v12.8c0 .8.9 1.3 1.6.8l9.4-6.4c.6-.4.6-1.2 0-1.6L4.6 4.8C3.9 4.3 3 4.8 3 5.6z"/><path d="M12 5.6v12.8c0 .8.9 1.3 1.6.8l9.4-6.4c.6-.4.6-1.2 0-1.6l-9.4-6.4c-.7-.5-1.6 0-1.6.8z" transform="translate(-1.5 0)"/>`)
	iconPrev  = fillIcon(`<g transform="matrix(-1 0 0 1 24 0)"><path d="M3 5.6v12.8c0 .8.9 1.3 1.6.8l9.4-6.4c.6-.4.6-1.2 0-1.6L4.6 4.8C3.9 4.3 3 4.8 3 5.6z"/><path d="M12 5.6v12.8c0 .8.9 1.3 1.6.8l9.4-6.4c.6-.4.6-1.2 0-1.6l-9.4-6.4c-.7-.5-1.6 0-1.6.8z" transform="translate(-1.5 0)"/></g>`)

	iconShuffle       = strokeIcon(`<path d="M2 18h1.4c1.3 0 2.5-.6 3.3-1.7l6.1-8.6c.8-1.1 2-1.7 3.3-1.7H22"/><path d="m18 2 4 4-4 4"/><path d="M2 6h1.9c1.5 0 2.9.9 3.6 2.2"/><path d="M22 18h-5.9c-1.3 0-2.6-.7-3.3-1.8l-.5-.8"/><path d="m18 14 4 4-4 4"/>`)
	iconRepeat        = strokeIcon(`<path d="m17 2 4 4-4 4"/><path d="M3 11v-1a4 4 0 0 1 4-4h14"/><path d="m7 22-4-4 4-4"/><path d="M21 13v1a4 4 0 0 1-4 4H3"/>`)
	iconRepeat1       = strokeIcon(`<path d="m17 2 4 4-4 4"/><path d="M3 11v-1a4 4 0 0 1 4-4h14"/><path d="m7 22-4-4 4-4"/><path d="M21 13v1a4 4 0 0 1-4 4H3"/><path d="M11 10h1v4"/>`)
	iconVolume        = strokeIcon(`<path d="M11 4.7a.7.7 0 0 0-1.2-.5L6.4 7.6A1.4 1.4 0 0 1 5.4 8H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2.4a1.4 1.4 0 0 1 1 .4l3.4 3.4a.7.7 0 0 0 1.2-.5z"/><path d="M16 9a5 5 0 0 1 0 6"/><path d="M19.4 18.4a9 9 0 0 0 0-12.8"/>`)
	iconVolumeLow     = strokeIcon(`<path d="M11 4.7a.7.7 0 0 0-1.2-.5L6.4 7.6A1.4 1.4 0 0 1 5.4 8H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2.4a1.4 1.4 0 0 1 1 .4l3.4 3.4a.7.7 0 0 0 1.2-.5z"/><path d="M16 9a5 5 0 0 1 0 6"/>`)
	iconMute          = strokeIcon(`<path d="M11 4.7a.7.7 0 0 0-1.2-.5L6.4 7.6A1.4 1.4 0 0 1 5.4 8H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2.4a1.4 1.4 0 0 1 1 .4l3.4 3.4a.7.7 0 0 0 1.2-.5z"/><line x1="22" x2="16" y1="9" y2="15"/><line x1="16" x2="22" y1="9" y2="15"/>`)
	iconSearch        = strokeIcon(`<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>`)
	iconHome          = strokeIcon(`<path d="M12 3 4 9v11a1 1 0 0 0 1 1h4v-6h6v6h4a1 1 0 0 0 1-1V9z"/>`)
	iconDaily         = strokeIcon(`<rect x="3" y="4.5" width="18" height="16.5" rx="2.5"/><path d="M8 2.5v4"/><path d="M16 2.5v4"/><path d="M3 10h18"/>`)
	iconCharts        = strokeIcon(`<path d="M4 20V10"/><path d="M10 20V4"/><path d="M16 20v-7"/><path d="M22 20H2"/>`)
	iconQueue         = strokeIcon(`<path d="M16 5H3"/><path d="M11 12H3"/><path d="M11 19H3"/><path d="M21 16V5"/><circle cx="18" cy="16" r="3"/>`)
	iconLyrics        = strokeIcon(`<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/><path d="M8 9h8"/><path d="M8 13h5"/>`)
	iconDesktopLyrics = strokeIcon(`<rect x="3" y="4" width="18" height="14" rx="2"/><path d="M8 21h8"/><path d="M8 8h8"/><path d="M8 12h5"/>`)
	iconLock          = strokeIcon(`<rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/>`)
	iconUnlock        = strokeIcon(`<rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 7-2"/>`)
	iconDown          = strokeIcon(`<path d="m6 9 6 6 6-6"/>`)
	iconBack          = strokeIcon(`<path d="m15 18-6-6 6-6"/>`)
	iconForward       = strokeIcon(`<path d="m9 18 6-6-6-6"/>`)
	iconNote          = strokeIcon(`<path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/>`)
	iconHeart         = strokeIcon(`<path d="M19 14c1.5-1.5 3-3.2 3-5.5A5.5 5.5 0 0 0 16.5 3c-1.8 0-3 .5-4.5 2-1.5-1.5-2.7-2-4.5-2A5.5 5.5 0 0 0 2 8.5c0 2.3 1.5 4 3 5.5l7 7z"/>`)
	iconHeartFill     = fillIcon(`<path d="M19 14c1.5-1.5 3-3.2 3-5.5A5.5 5.5 0 0 0 16.5 3c-1.8 0-3 .5-4.5 2-1.5-1.5-2.7-2-4.5-2A5.5 5.5 0 0 0 2 8.5c0 2.3 1.5 4 3 5.5l7 7z"/>`)
	iconUser          = strokeIcon(`<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>`)
	iconPodcast       = strokeIcon(`<rect x="9" y="2" width="6" height="12" rx="3"/><path d="M19 10v1a7 7 0 0 1-14 0v-1"/><path d="M12 18v4"/>`)
	iconHeadset       = strokeIcon(`<path d="M3 14h3a2 2 0 0 1 2 2v3a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-7a9 9 0 0 1 18 0v7a2 2 0 0 1-2 2h-1a2 2 0 0 1-2-2v-3a2 2 0 0 1 2-2h3"/>`)
)

var (
	iconRadio   = strokeIcon(`<circle cx="12" cy="12" r="2"/><path d="M16.24 7.76a6 6 0 0 1 0 8.49"/><path d="M7.76 16.24a6 6 0 0 1 0-8.49"/><path d="M19.07 4.93a10 10 0 0 1 0 14.14"/><path d="M4.93 19.07a10 10 0 0 1 0-14.14"/>`)
	iconTrash   = strokeIcon(`<path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>`)
	iconThumb   = strokeIcon(`<path d="M7 10v12"/><path d="M15 5.88 14 10h5.83a2 2 0 0 1 1.92 2.56l-2.33 8A2 2 0 0 1 17.5 22H4a2 2 0 0 1-2-2v-8a2 2 0 0 1 2-2h2.76a2 2 0 0 0 1.79-1.11L12 2a3.13 3.13 0 0 1 3 3.88Z"/>`)
	iconComment = strokeIcon(`<path d="M7.9 20A9 9 0 1 0 4 16.1L2 22Z"/>`)
	iconPlus    = strokeIcon(`<path d="M12 5v14"/><path d="M5 12h14"/>`)
	iconCheck   = strokeIcon(`<path d="m5 12 4 4L19 6"/>`)
	iconX       = strokeIcon(`<path d="M18 6 6 18"/><path d="m6 6 12 12"/>`)
	iconMore    = fillIcon(`<circle cx="5" cy="12" r="1.9"/><circle cx="12" cy="12" r="1.9"/><circle cx="19" cy="12" r="1.9"/>`)
)

// iconThumbFill is iconThumb filled, its cuff and hand apart as the outline
// draws them.
var iconThumbFill = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor" stroke="currentColor" stroke-width="1.75" stroke-linejoin="round"><path d="M2.9 10.9h2.3v10.2H2.9z"/><path d="M9 10.4 12 2a3.13 3.13 0 0 1 3 3.88L14 10h5.83a2 2 0 0 1 1.92 2.56l-2.33 8A2 2 0 0 1 17.5 22H9z"/></svg>`))

var (
	iconReload   = strokeIcon(`<path d="M20 7v5h-5"/><path d="M20 12a8 8 0 1 0-2.3 5.7"/>`)
	iconUpDown   = strokeIcon(`<path d="m7 15 5 5 5-5"/><path d="m7 9 5-5 5 5"/>`)
	iconSettings = strokeIcon(`<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/>`)
	iconSignOut  = strokeIcon(`<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="m16 17 5-5-5-5"/><path d="M21 12H9"/>`)
)
