package app

import (
	"context"
	"sort"

	appplatform "netease-music-mygo/internal/app/platform"
	"netease-music-mygo/internal/netease"
	"netease-music-mygo/internal/platform"
	"netease-music-mygo/internal/player"
)

// Platform extension seam.
//
// Keep provider-neutral adapter lookup, capability checks and dispatch here.
// Concrete provider state, protocol types and provider UI belong in the
// provider adapter directory. Changes to the upstream app should use this
// seam instead of adding provider-specific fields or branches.

func (a *app) activePlatform() appplatform.Adapter {
	if a == nil || a.platforms == nil {
		return nil
	}
	return a.platforms[a.platformID]
}

func (a *app) initPlatformState() {
	newPlatformAdapters(a)
	a.platformID = platform.ID(a.settings.Platform)
	if !a.hasPlatform(a.platformID) {
		a.platformID = platform.NeteaseID
		a.settings.Platform = string(platform.NeteaseID)
	}
}

func (a *app) configurePlatformPlayer() {
	if a.art != nil {
		a.art.fetch = platformHost{a: a}.FetchArtwork
		a.art.generation = func() uint64 { return a.platformGeneration }
	}
	if a.pl == nil {
		return
	}
	// Player hooks belong to the active platform only. Clearing the embedded
	// seam first prevents a previous provider from handling a later platform's
	// track after a switch.
	a.pl.PlatformPlayer = player.PlatformPlayer{}
	if adapter := a.activePlatform(); adapter != nil {
		adapter.ConfigurePlayer(a.pl)
	}
}

func (a *app) startPlatform() {
	if a.platforms == nil {
		a.initPlatformState()
		a.configurePlatformPlayer()
	}
	if a.platformID == platform.NeteaseID {
		a.boot()
	} else {
		a.homeLoading = false
	}
	if a.activeHome() == nil {
		return
	}
	rt := a.activeRuntime()
	if rt == nil || !rt.Configured() {
		return
	}
	go func() {
		err := rt.Connect(context.Background(), rt.Config())
		a.post(func() {
			if err != nil && err != context.Canceled {
				a.toast = err.Error()
			}
		})
	}()
}

func (a *app) activeHome() appplatform.Home {
	if p := a.activePlatform(); p != nil {
		return p.Home()
	}
	return nil
}

func (a *app) activeRuntime() appplatform.Runtime {
	if p := a.activePlatform(); p != nil {
		return p.Runtime()
	}
	return nil
}

// platformInfos returns the identities available to the shared platform
// selector. NetEase is the upstream shell and therefore has no adapter; all
// other entries come from the registered adapter seam.
func (a *app) platformInfos() []appplatform.Info {
	infos := []appplatform.Info{neteasePlatformInfo()}
	for _, adapter := range a.platforms {
		if adapter == nil {
			continue
		}
		info := adapter.Info()
		if info.ID == "" || info.ID == platform.NeteaseID {
			continue
		}
		infos = append(infos, info)
	}
	others := infos[1:]
	sort.SliceStable(others, func(i, j int) bool { return others[i].Name < others[j].Name })
	return infos
}

func neteasePlatformInfo() appplatform.Info {
	return appplatform.Info{ID: platform.NeteaseID, Name: "网易云音乐", Comments: true, Podcasts: true, Quality: true, Normalize: true}
}

func (a *app) commentsEnabled() bool {
	if a == nil {
		return true
	}
	if p := a.activePlatform(); p != nil {
		return p.Info().Comments
	}
	return true
}

func (a *app) hasPlatform(id platform.ID) bool {
	if id == platform.NeteaseID {
		return true
	}
	_, ok := a.platforms[id]
	return ok
}

func (a *app) trackPlatform(tr *netease.Track) (platform.ID, bool) {
	if tr == nil {
		return "", false
	}
	if tr.Source == "" {
		return platform.NeteaseID, true
	}
	for id, adapter := range a.platforms {
		if adapter != nil && adapter.OwnsTrack(tr) {
			return id, true
		}
	}
	return "", false
}

func (a *app) trackAdapter(tr *netease.Track) appplatform.Adapter {
	if tr == nil {
		return nil
	}
	for _, adapter := range a.platforms {
		if adapter != nil && adapter.OwnsTrack(tr) {
			return adapter
		}
	}
	return nil
}
