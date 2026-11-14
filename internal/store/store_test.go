package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const id = "0Aa1Bb2Cc3Dd4Ee5F"

func TestFileSaveAndHas(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "data")
	s, err := NewFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Has(id) {
		t.Fatal("Has() = true before any save")
	}
	for _, content := range []string{"first", "second"} {
		if err := s.Save(id, []byte(content)); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(dir, id+".json"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != content {
			t.Errorf("stored %q, want %q", got, content)
		}
	}
	if !s.Has(id) {
		t.Error("Has() = false after save")
	}

	info, err := os.Stat(filepath.Join(dir, id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("file mode = %o, want no group/other access", perm)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("temp files left behind: %d entries in %s", len(entries), dir)
	}
}

func TestFileDelete(t *testing.T) {
	s, err := NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(id); err != nil {
		t.Errorf("Delete() of a missing copy = %v, want nil", err)
	}
	if err := s.Save(id, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(id); err != nil {
		t.Fatal(err)
	}
	if s.Has(id) {
		t.Error("Has() = true after Delete")
	}
	if err := s.Delete("../../../etc/pwd0"); !errors.Is(err, ErrInvalidID) {
		t.Errorf("Delete(bad id) error = %v, want ErrInvalidID", err)
	}
}

func TestFileLoad(t *testing.T) {
	s, err := NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(id); !errors.Is(err, ErrNotFound) {
		t.Errorf("Load() before any save: error = %v, want ErrNotFound", err)
	}
	if err := s.Save(id, []byte("menu")); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(id)
	if err != nil || string(got) != "menu" {
		t.Errorf("Load() = %q, %v; want %q, nil", got, err, "menu")
	}
	if _, err := s.Load("../../../etc/pwd0"); !errors.Is(err, ErrInvalidID) {
		t.Errorf("Load(bad id) error = %v, want ErrInvalidID", err)
	}
}

func TestFilePrune(t *testing.T) {
	const fresh = "1Aa1Bb2Cc3Dd4Ee5F"
	dir := t.TempDir()
	s, err := NewFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{id, fresh} {
		if err := s.Save(name, []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	// Not a stored copy: must survive however old it is.
	other := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(other, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	old := now.Add(-8 * 24 * time.Hour)
	for _, path := range []string{filepath.Join(dir, id+".json"), other} {
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}

	removed, err := s.Prune(now.Add(-7 * 24 * time.Hour))
	if err != nil || removed != 1 {
		t.Fatalf("Prune() = %d, %v; want 1, nil", removed, err)
	}
	if s.Has(id) {
		t.Error("the old copy was not removed")
	}
	if !s.Has(fresh) {
		t.Error("the fresh copy was removed")
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("an unrelated file was removed: %v", err)
	}
}

func TestFileRejectsBadIDs(t *testing.T) {
	s, err := NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "abc", "../../../etc/pwd0", "aaaaaaaa/aaaaaaaa", "1532825339"} {
		if err := s.Save(bad, []byte("x")); !errors.Is(err, ErrInvalidID) {
			t.Errorf("Save(%q) error = %v, want ErrInvalidID", bad, err)
		}
		if s.Has(bad) {
			t.Errorf("Has(%q) = true", bad)
		}
	}
}

func TestNewFileEmptyDir(t *testing.T) {
	if _, err := NewFile(""); err == nil {
		t.Error("expected an error for an empty directory")
	}
}
