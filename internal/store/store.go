// Package store keeps a local copy of the last menu published for each id.
//
// The copy lives on the data volume and is keyed by the opaque menu id, never
// by Telegram user ID. It lets the bot tell a user whether they have already
// published, and gives the operator something to re-publish from if the site
// repository is ever rebuilt.
package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/luis-serrano-l/decarta-bot/internal/menuid"
)

// ErrInvalidID is returned for ids that are not well-formed menu ids.
var ErrInvalidID = errors.New("store: invalid menu id")

// ErrNotFound is returned by Load when nothing is stored for the id.
var ErrNotFound = errors.New("store: not found")

// Store saves and looks up one document per menu id.
type Store interface {
	Save(id string, content []byte) error
	// Load returns the stored copy for id, or ErrNotFound.
	Load(id string) ([]byte, error)
	Has(id string) bool
	// Delete removes the copy for id; it is not an error if there is none.
	Delete(id string) error
}

// File is a Store backed by a directory with one <id>.json per menu.
type File struct {
	dir string
}

// NewFile returns a File store rooted at dir, creating it (mode 0700) if
// needed.
func NewFile(dir string) (*File, error) {
	if dir == "" {
		return nil, errors.New("store: directory is empty")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("store: create directory: %w", err)
	}
	return &File{dir: dir}, nil
}

// Save atomically replaces the stored copy for id.
func (f *File) Save(id string, content []byte) error {
	path, err := f.path(id)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(f.dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("store: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) //nolint:errcheck // best effort; a no-op after a successful rename

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("store: write: %w", err)
	}
	// Flushed before the rename, so a crash cannot leave an empty file
	// under the final name.
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("store: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("store: close: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("store: rename: %w", err)
	}
	return nil
}

// Load returns the stored copy for id, or ErrNotFound if there is none.
func (f *File) Load(id string) ([]byte, error) {
	path, err := f.path(id)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(path) //nolint:gosec // path is built from a validated base62 id
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("store: read: %w", err)
	}
	return content, nil
}

// Prune removes every stored copy last written before the given time and
// reports how many were removed. Files that are not stored copies are left
// alone.
func (f *File) Prune(before time.Time) (int, error) {
	entries, err := os.ReadDir(f.dir)
	if err != nil {
		return 0, fmt.Errorf("store: list: %w", err)
	}
	removed := 0
	for _, e := range entries {
		id, isCopy := strings.CutSuffix(e.Name(), ".json")
		if !isCopy || !e.Type().IsRegular() || !menuid.Valid(id) {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.ModTime().Before(before) {
			continue
		}
		if err := f.Delete(id); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

// Has reports whether a copy is stored for id.
func (f *File) Has(id string) bool {
	path, err := f.path(id)
	if err != nil {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// Delete removes the stored copy for id, if any.
func (f *File) Delete(id string) error {
	path, err := f.path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("store: remove: %w", err)
	}
	return nil
}

// path maps an id to its file. Ids are restricted to base62, so the result
// cannot escape dir.
func (f *File) path(id string) (string, error) {
	if !menuid.Valid(id) {
		return "", ErrInvalidID
	}
	return filepath.Join(f.dir, id+".json"), nil
}
