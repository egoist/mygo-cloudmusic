package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/egoist/mygo"

	"netease-music-mygo/internal/player"
)

// Where playback was left is kept in session.json in the user data
// directory: on quitting, and every little while as it plays, in case the
// app does not get to quit.

func sessionPath() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "session.json"), nil
}

// saveSession keeps where playback is, if it moved on since last kept.
func (a *app) saveSession() {
	s, ok := a.pl.Session()
	if !ok {
		return
	}
	b, err := json.Marshal(s)
	if err != nil || string(b) == a.savedSession {
		return
	}
	path, err := sessionPath()
	if err != nil {
		return
	}
	if os.WriteFile(path+".part", b, 0o600) == nil && os.Rename(path+".part", path) == nil {
		a.savedSession = string(b)
	}
}

// saveSessionSoon is saveSession for while the app runs on: a queue of a
// library's thousands of songs is megabytes of JSON, which would hold up
// a frame or two on the main thread each time, so it is encoded and written
// on another. Its songs are not changed in place, but its order is, by
// shuffling: that is copied.
func (a *app) saveSessionSoon() {
	s, ok := a.pl.Session()
	if !ok {
		return
	}
	s.Order = slices.Clone(s.Order)
	path, err := sessionPath()
	if err != nil {
		return
	}
	go func() {
		b, err := json.Marshal(s)
		if err != nil {
			return
		}
		a.post(func() {
			if string(b) == a.savedSession {
				return
			}
			a.savedSession = string(b)
			go func() {
				if os.WriteFile(path+".part", b, 0o600) == nil {
					os.Rename(path+".part", path)
				}
			}()
		})
	}()
}

// sessionEvery is how long after playback changed, and how often as it
// plays, the session is kept.
const sessionEvery = 30 * time.Second

// keepSession saves the session a while after playback changed, and every
// while again as it plays. Paused, nothing is due, and the app sleeps.
func (a *app) keepSession() {
	if a.sessionDue {
		return
	}
	a.sessionDue = true
	sessionAfter(sessionEvery, func() {
		a.sessionDue = false
		a.saveSessionSoon()
		if a.pl.Playing() {
			a.keepSession()
		}
	})
}

// sessionAfter runs fn on the main thread after d, without redrawing a
// window as Update would.
var sessionAfter = func(d time.Duration, fn func()) {
	time.AfterFunc(d, func() { mygo.RunOnMain(fn) })
}

// restoreSession picks up playback where it was left, paused, once the
// listener's session is back: the songs they may play depend on it.
func (a *app) restoreSession() {
	if a.restored {
		return
	}
	a.restored = true
	path, err := sessionPath()
	if err != nil {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var s player.Session
	if json.Unmarshal(b, &s) != nil {
		return
	}
	a.savedSession = string(b)
	a.pl.Restore(s)
	if s.Radio != "" && a.pl.Current() != nil {
		a.pl.AttachRadio(s.Radio, a.radioFeed(s.Radio))
	}
	// A queue read from a list of the Feiniu service goes on reading it.
	if s.Origin != "" {
		a.restoreFnOrigin(s.Origin)
	}
}
