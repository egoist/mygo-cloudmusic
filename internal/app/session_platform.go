package app

import (
	"os"

	"netease-music-mygo/internal/player"
)

// Platform session extension seam.
//
// Keep only session behavior needed when the active provider changes here.
// The upstream session format and normal restore flow remain in session.go.

// discardSession removes the persisted playback context after a platform
// switch. Keeping it would let a later launch restore a queue from the wrong
// platform, even though the in-memory player was reset.
func (a *app) discardSession() {
	a.savedSession = ""
	a.sessionDue = false
	path, err := sessionPath()
	if err != nil {
		return
	}
	_ = os.Remove(path)
	_ = os.Remove(path + ".part")
}

func (a *app) sessionBelongsToActivePlatform(s player.Session) bool {
	if len(s.Queue) == 0 {
		return true
	}
	queuePlatform, ok := a.trackPlatform(&s.Queue[0])
	if !ok {
		return false
	}
	for i := 1; i < len(s.Queue); i++ {
		id, owned := a.trackPlatform(&s.Queue[i])
		if !owned || id != queuePlatform {
			return false
		}
	}
	return queuePlatform == a.platformID
}
