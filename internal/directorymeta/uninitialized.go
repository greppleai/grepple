package directorymeta

import (
	"os"
	"path/filepath"
)

// UninitializedCheckout finds the enclosing Git boundary using ancestor probes
// only. A missing .grepple is conclusive inside a checkout, but not in a parent
// directory which may contain independent annotated checkouts. Existing markers
// (including broken symlinks) and inspection errors retain the normal path.
func UninitializedCheckout(start string) (string, bool) {
	directory, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Lstat(filepath.Join(directory, ".grepple")); !os.IsNotExist(err) {
			return "", false
		}
		if _, err := os.Stat(filepath.Join(directory, ".git")); err == nil {
			return directory, true
		} else if !os.IsNotExist(err) {
			return "", false
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", false
		}
		directory = parent
	}
}
