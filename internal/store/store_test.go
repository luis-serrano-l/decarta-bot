package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
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
