// Package storage is the driver interface from overview.md §7: Put/Get/
// Delete/URL behind an interface so the local-disk implementation here
// can be swapped for an S3-compatible driver later as a config change,
// with zero changes to the chat/automation/file business logic that
// calls it.
package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Driver interface {
	Put(uuid string, reader io.Reader) (path string, err error)
	Get(path string) (io.ReadCloser, error)
	Delete(path string) error
}

type LocalDriver struct {
	Root string
}

func NewLocalDriver(root string) *LocalDriver {
	return &LocalDriver{Root: root}
}

func (d *LocalDriver) Put(uuid string, reader io.Reader) (string, error) {
	if err := os.MkdirAll(d.Root, 0755); err != nil {
		return "", err
	}
	relPath := uuid
	fullPath := filepath.Join(d.Root, relPath)

	f, err := os.Create(fullPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, reader); err != nil {
		return "", err
	}
	return relPath, nil
}

func (d *LocalDriver) Get(relPath string) (io.ReadCloser, error) {
	full := filepath.Join(d.Root, relPath)
	if !isWithinRoot(d.Root, full) {
		return nil, fmt.Errorf("path escapes storage root")
	}
	return os.Open(full)
}

func (d *LocalDriver) Delete(relPath string) error {
	full := filepath.Join(d.Root, relPath)
	if !isWithinRoot(d.Root, full) {
		return fmt.Errorf("path escapes storage root")
	}
	return os.Remove(full)
}

func isWithinRoot(root, path string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}
