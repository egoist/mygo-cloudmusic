package app

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"

	"netease-music-mygo/internal/fnmusic"
)

// What signs the listener in to the Feiniu service (its password's hash and
// the FN Connect access code) is kept in the system's keychain, apart from
// the settings. As with NetEase's session, development builds keep it in a
// private file instead: each rebuild is signed anew, and the keychain would
// ask before every one of them read it.

const fnSecret = "fn-credentials"

type fnSecrets struct {
	PassHash   string `json:"passHash"`
	AccessCode string `json:"accessCode,omitempty"`
}

func fnSecretFile() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "fn-credentials"), nil
}

// loadFnSecrets adds the secrets of the service's config, kept apart.
func loadFnSecrets(cfg *fnmusic.Config) {
	var b []byte
	var err error
	if mygo.IsDev() {
		var path string
		if path, err = fnSecretFile(); err == nil {
			b, err = os.ReadFile(path)
		}
	} else {
		b, err = mygo.Secrets.Get(fnSecret)
	}
	var s fnSecrets
	if err != nil || json.Unmarshal(b, &s) != nil {
		return
	}
	cfg.PassHash, cfg.AccessCode = s.PassHash, s.AccessCode
}

// saveFnSecrets keeps the secrets of cfg.
func saveFnSecrets(cfg fnmusic.Config) error {
	b, _ := json.Marshal(fnSecrets{PassHash: cfg.PassHash, AccessCode: cfg.AccessCode})
	if !mygo.IsDev() {
		return mygo.Secrets.Set(fnSecret, b)
	}
	path, err := fnSecretFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// deleteFnSecrets forgets them.
func deleteFnSecrets() {
	if !mygo.IsDev() {
		mygo.Secrets.Delete(fnSecret)
		return
	}
	if path, err := fnSecretFile(); err == nil {
		os.Remove(path)
	}
}
