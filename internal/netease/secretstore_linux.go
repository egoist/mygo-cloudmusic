//go:build linux

package netease

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const kdeSecretService = "org.kde.secretservicecompat"

func startKDESecretService() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	args := make([]string, 0, 10)
	if address := os.Getenv("DBUS_SESSION_BUS_ADDRESS"); address != "" {
		args = append(args, "--address="+address)
	} else {
		args = append(args, "--user")
	}
	args = append(args,
		"call",
		"org.freedesktop.DBus",
		"/org/freedesktop/DBus",
		"org.freedesktop.DBus",
		"StartServiceByName",
		"su",
		kdeSecretService,
		"0",
	)
	output, err := exec.CommandContext(ctx, "busctl", args...).CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail != "" {
			return fmt.Errorf("busctl: %w: %s", err, detail)
		}
		return fmt.Errorf("busctl: %w", err)
	}
	return nil
}
