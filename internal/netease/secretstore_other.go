//go:build !linux

package netease

import "errors"

func startKDESecretService() error { return errors.ErrUnsupported }
