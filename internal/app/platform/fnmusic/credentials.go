package fnmusic

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"

	backend "netease-music-mygo/internal/platform/fnmusic"
)

// Feiniu credentials are kept out of settings.json. The platform settings
// namespace stores only connection metadata; the password hash and optional
// access code use the same MyGo secret store as other account credentials.
const fnCredentialSecret = "fnmusic-credentials"

type fnCredentials struct {
	PassHash   string `json:"passHash"`
	AccessCode string `json:"accessCode,omitempty"`
}

func credentialFile() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "fnmusic-credentials"), nil
}

func LoadCredentials(cfg *backend.Config) {
	if cfg == nil {
		return
	}
	var b []byte
	var err error
	if mygo.IsDev() {
		var path string
		if path, err = credentialFile(); err == nil {
			b, err = os.ReadFile(path)
		}
	} else {
		b, err = mygo.Secrets.Get(fnCredentialSecret)
	}
	var sec fnCredentials
	if err != nil || json.Unmarshal(b, &sec) != nil {
		return
	}
	cfg.PassHash, cfg.AccessCode = sec.PassHash, sec.AccessCode
}

func SaveCredentials(cfg backend.Config) error {
	if cfg == (backend.Config{}) {
		return nil
	}
	b, _ := json.Marshal(fnCredentials{PassHash: cfg.PassHash, AccessCode: cfg.AccessCode})
	if !mygo.IsDev() {
		return mygo.Secrets.Set(fnCredentialSecret, b)
	}
	path, err := credentialFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func DeleteCredentials() {
	if !mygo.IsDev() {
		mygo.Secrets.Delete(fnCredentialSecret)
		return
	}
	if path, err := credentialFile(); err == nil {
		_ = os.Remove(path)
	}
}
