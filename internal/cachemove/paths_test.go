package cachemove

import (
	"os"
	"path/filepath"
	"testing"
)

// Path validation rejects overlap and user files without creating or deleting anything.
func TestValidate(t *testing.T) {
	root := t.TempDir()
	source, occupied := filepath.Join(root, "old"), filepath.Join(root, "occupied")
	for _, dir := range []string{source, occupied} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(occupied, "personal.txt")
	if err := os.WriteFile(file, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{source, filepath.Join(source, "child"), root, occupied, file, filepath.Join(file, "child"), "relative"} {
		if err := Validate(source, target); err == nil {
			t.Fatalf("accepted unsafe path %s", target)
		}
	}
	target := filepath.Join(root, "new", "songs")
	if err := Validate(source, target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("validation created target: %v", err)
	}
	data, err := os.ReadFile(file)
	if err != nil || string(data) != "keep" {
		t.Fatal("changed user file")
	}
}

// Links to the source, including through a target parent, cannot bypass overlap checks.
func TestPathsResolveLinks(t *testing.T) {
	root := t.TempDir()
	source, link := filepath.Join(root, "old"), filepath.Join(root, "link")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	for _, target := range []string{link, filepath.Join(link, "child")} {
		if err := Validate(source, target); err == nil {
			t.Fatalf("accepted overlap via %s", target)
		}
	}
}
