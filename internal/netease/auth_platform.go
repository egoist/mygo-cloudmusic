package netease

// SessionConfigured reports whether a saved NetEase session exists. It is a
// read-only onboarding check; restoring or validating the session remains the
// responsibility of the normal boot flow.
func SessionConfigured() bool {
	b, err := readSession()
	return err == nil && len(b) > 0
}
