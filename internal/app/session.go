package app

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// keepSession saves the session every half minute.
func (a *app) keepSession() {
	go func() {
		for range time.Tick(30 * time.Second) {
			a.win.Update(a.saveSession)
		}
	}()
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
}
