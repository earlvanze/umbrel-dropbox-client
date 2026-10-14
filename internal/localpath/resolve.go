// Package localpath maps Dropbox display paths onto existing local spelling.
package localpath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrUnsafe = errors.New("unsafe or ambiguous local path")

// Resolve preserves the spelling of each unique existing component. Missing
// components retain their cloud display spelling. Symlinks are never traversed.
func Resolve(root, displayPath string) (string, error) {
	relative := strings.TrimPrefix(displayPath, "/")
	parts := strings.Split(relative, "/")
	for _, p := range parts {
		if p == "" || p == "." || p == ".." || strings.ContainsAny(p, "\\\x00") {
			return "", fmt.Errorf("%w: invalid path %q", ErrUnsafe, displayPath)
		}
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%w: sync root is not a real directory", ErrUnsafe)
	}
	parent := absolute
	for i, part := range parts {
		entries, err := os.ReadDir(parent)
		if err != nil {
			return "", err
		}
		name := ""
		for _, entry := range entries {
			if strings.ToLower(entry.Name()) != strings.ToLower(part) {
				continue
			}
			if name != "" {
				return "", fmt.Errorf("%w: case collision at %q", ErrUnsafe, part)
			}
			name = entry.Name()
		}
		if name == "" {
			return filepath.Join(append([]string{parent}, parts[i:]...)...), nil
		}
		parent = filepath.Join(parent, name)
		info, err := os.Lstat(parent)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%w: symlink at %q", ErrUnsafe, name)
		}
		if i < len(parts)-1 && !info.IsDir() {
			return "", fmt.Errorf("%w: parent %q is not a directory", ErrUnsafe, name)
		}
	}
	return parent, nil
}
