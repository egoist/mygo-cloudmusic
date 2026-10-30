package netease

import (
	"errors"
	"fmt"
)

func retrySecretStore(call, activate func() error) error {
	err := call()
	if !errors.Is(err, errors.ErrUnsupported) {
		return err
	}
	if activateErr := activate(); activateErr != nil {
		if errors.Is(activateErr, errors.ErrUnsupported) {
			return err
		}
		return errors.Join(err, fmt.Errorf("activate KWallet Secret Service: %w", activateErr))
	}
	return call()
}
