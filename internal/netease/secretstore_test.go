package netease

import (
	"errors"
	"testing"

	"github.com/egoist/mygo"
)

func TestRetrySecretStoreActivatesAndRetriesUnsupported(t *testing.T) {
	// A missing SteamOS Secret Service is activated before retrying the session operation.
	calls, activations := 0, 0
	err := retrySecretStore(func() error {
		calls++
		if calls == 1 {
			return errors.ErrUnsupported
		}
		return nil
	}, func() error {
		activations++
		return nil
	})
	if err != nil || calls != 2 || activations != 1 {
		t.Fatalf("retrySecretStore() = %v after %d calls and %d activations, want success after 2 calls and 1 activation", err, calls, activations)
	}
}

func TestRetrySecretStoreDoesNotActivateForOtherErrors(t *testing.T) {
	// An ordinary keyring error does not activate or repeat the operation.
	want := errors.New("permission denied")
	calls, activations := 0, 0
	err := retrySecretStore(func() error {
		calls++
		return want
	}, func() error {
		activations++
		return nil
	})
	if err != want || calls != 1 || activations != 0 {
		t.Fatalf("retrySecretStore() = %v after %d calls and %d activations, want original error and no activation", err, calls, activations)
	}
}

func TestRetrySecretStoreDoesNotActivateForMissingSecret(t *testing.T) {
	// A missing stored session is different from an unavailable Secret Service.
	calls, activations := 0, 0
	err := retrySecretStore(func() error {
		calls++
		return mygo.ErrSecretNotFound
	}, func() error {
		activations++
		return nil
	})
	if !errors.Is(err, mygo.ErrSecretNotFound) || calls != 1 || activations != 0 {
		t.Fatalf("retrySecretStore() = %v after %d calls and %d activations, want the missing-secret error without activation", err, calls, activations)
	}
}

func TestRetrySecretStorePreservesUnsupportedWhenPlatformHasNoActivator(t *testing.T) {
	// A platform without KDE activation keeps the original unsupported-store error.
	calls := 0
	err := retrySecretStore(func() error {
		calls++
		return errors.ErrUnsupported
	}, func() error { return errors.ErrUnsupported })
	if err != errors.ErrUnsupported || calls != 1 {
		t.Fatalf("retrySecretStore() = %v after %d calls, want original unsupported error without retry", err, calls)
	}
}

func TestRetrySecretStorePreservesUnsupportedWhenActivationFails(t *testing.T) {
	// A missing busctl or KDE service must not erase MyGo's unsupported-store error.
	startErr := errors.New("busctl unavailable")
	calls := 0
	err := retrySecretStore(func() error {
		calls++
		return errors.ErrUnsupported
	}, func() error { return startErr })
	if !errors.Is(err, errors.ErrUnsupported) || !errors.Is(err, startErr) || calls != 1 {
		t.Fatalf("retrySecretStore() = %v after %d calls, want both errors and no retry", err, calls)
	}
}
