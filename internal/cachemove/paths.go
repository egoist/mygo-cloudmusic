// Package cachemove relocates a song cache whose readers and writers are stopped.
package cachemove

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Paths resolves links, including links in an existing parent, and rejects
// overlapping directories. It does not create either directory.
func Paths(source, target string) (string, string, error) {
	source, err := resolve(source)
	if err != nil {
		return "", "", err
	}
	target, err = resolve(target)
	if err != nil {
		return "", "", err
	}
	for _, pair := range [][2]string{{source, target}, {target, source}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))) {
			return "", "", fmt.Errorf("新旧缓存目录不能相同或互相包含")
		}
	}
	return source, target, nil
}

func resolve(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("缓存目录必须是绝对路径")
	}
	path = filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		info, err := os.Stat(resolved)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", fmt.Errorf("缓存路径不是目录：%s", resolved)
		}
		return resolved, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", err
	}
	parent, err = resolve(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(path)), nil
}

// Validate requires a separate, empty target. Existing user files are never
// merged, overwritten or removed, even when migration is not requested.
func Validate(source, target string) error {
	_, target, err := Paths(source, target)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("请选择空目录，目标目录中已有文件")
	}
	return nil
}
