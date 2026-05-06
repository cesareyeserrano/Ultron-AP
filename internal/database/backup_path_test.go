package database

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateBackupPath_EmptyAllowed(t *testing.T) {
	root := t.TempDir()
	got, err := ValidateBackupPath("", root)
	if err != nil {
		t.Fatalf("empty input must be allowed (signals default), got err: %v", err)
	}
	if got != "" {
		t.Fatalf("empty input must round-trip to empty string, got %q", got)
	}
}

func TestValidateBackupPath_RootMustBeAbsolute(t *testing.T) {
	if _, err := ValidateBackupPath("/some/path", "relative/root"); err == nil {
		t.Fatal("expected error when root is relative")
	}
	if _, err := ValidateBackupPath("/some/path", ""); err == nil {
		t.Fatal("expected error when root is empty")
	}
}

func TestValidateBackupPath_AcceptsValidUnderRoot(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "backups", "daily")

	cleaned, err := ValidateBackupPath(candidate, root)
	if err != nil {
		t.Fatalf("expected valid path, got err: %v", err)
	}
	if cleaned != candidate {
		t.Fatalf("expected %q, got %q", candidate, cleaned)
	}
}

func TestValidateBackupPath_RejectsNUL(t *testing.T) {
	root := t.TempDir()
	bad := filepath.Join(root, "evil\x00name")
	if _, err := ValidateBackupPath(bad, root); err == nil || !errors.Is(err, ErrBackupPathInvalid) {
		t.Fatalf("expected ErrBackupPathInvalid for NUL byte, got %v", err)
	}
}

func TestValidateBackupPath_RejectsControlCharacter(t *testing.T) {
	root := t.TempDir()
	bad := filepath.Join(root, "evil\nname")
	if _, err := ValidateBackupPath(bad, root); err == nil || !errors.Is(err, ErrBackupPathInvalid) {
		t.Fatalf("expected ErrBackupPathInvalid for newline, got %v", err)
	}
	bad2 := filepath.Join(root, "evil\tname")
	if _, err := ValidateBackupPath(bad2, root); err == nil || !errors.Is(err, ErrBackupPathInvalid) {
		t.Fatalf("expected ErrBackupPathInvalid for tab, got %v", err)
	}
}

func TestValidateBackupPath_RejectsRelative(t *testing.T) {
	root := t.TempDir()
	if _, err := ValidateBackupPath("backups/daily", root); err == nil || !errors.Is(err, ErrBackupPathInvalid) {
		t.Fatal("expected ErrBackupPathInvalid for relative path")
	}
}

func TestValidateBackupPath_RejectsEscapeViaDotDot(t *testing.T) {
	root := t.TempDir()
	// Build an absolute path that, after Clean, escapes root.
	escape := filepath.Clean(filepath.Join(root, "..", "outside"))
	if _, err := ValidateBackupPath(escape, root); err == nil || !errors.Is(err, ErrBackupPathInvalid) {
		t.Fatalf("expected ErrBackupPathInvalid for path escaping root, got %v", err)
	}
}

func TestValidateBackupPath_AcceptsRootItself(t *testing.T) {
	root := t.TempDir()
	cleaned, err := ValidateBackupPath(root, root)
	if err != nil {
		t.Fatalf("expected root itself to validate, got %v", err)
	}
	if cleaned != filepath.Clean(root) {
		t.Fatalf("expected cleaned root, got %q", cleaned)
	}
}

func TestValidateBackupPath_RejectsSymlinkComponent(t *testing.T) {
	root := t.TempDir()
	realTarget := t.TempDir() // outside root

	linkInsideRoot := filepath.Join(root, "redirect")
	if err := os.Symlink(realTarget, linkInsideRoot); err != nil {
		t.Skipf("symlink unsupported in this environment: %v", err)
	}

	candidate := filepath.Join(linkInsideRoot, "stolen")
	if _, err := ValidateBackupPath(candidate, root); err == nil || !errors.Is(err, ErrBackupPathInvalid) {
		t.Fatalf("expected ErrBackupPathInvalid when a path component is a symlink, got %v", err)
	}
}

func TestValidateBackupPath_AllowsNonExistentLeafComponents(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "does", "not", "exist", "yet")
	cleaned, err := ValidateBackupPath(candidate, root)
	if err != nil {
		t.Fatalf("nonexistent leaf components must be allowed (created later), got %v", err)
	}
	if cleaned != candidate {
		t.Fatalf("expected %q, got %q", candidate, cleaned)
	}
}

func TestValidateBackupPath_RootIsSymlink(t *testing.T) {
	parent := t.TempDir()
	realRoot := filepath.Join(parent, "real")
	if err := os.Mkdir(realRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	linkRoot := filepath.Join(parent, "link")
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	candidate := filepath.Join(linkRoot, "x")
	if _, err := ValidateBackupPath(candidate, linkRoot); err == nil || !errors.Is(err, ErrBackupPathInvalid) {
		t.Fatalf("expected ErrBackupPathInvalid when configured root is itself a symlink, got %v", err)
	}
}

func TestValidateBackupPath_TrimsWhitespace(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "trimmed")
	cleaned, err := ValidateBackupPath("   "+candidate+"   ", root)
	if err != nil {
		t.Fatalf("expected whitespace to be trimmed and accepted, got %v", err)
	}
	if cleaned != candidate {
		t.Fatalf("expected %q, got %q", candidate, cleaned)
	}
}

// --- Defensive checks in DB.Backup ---

func TestDBBackup_RejectsControlCharsAndRelativeAndEmpty(t *testing.T) {
	db := newTestDB(t)
	cases := []struct {
		name string
		path string
	}{
		{"empty", ""},
		{"nul byte", "/tmp/evil\x00.db"},
		{"newline", "/tmp/evil\n.db"},
		{"relative", "ultron.db"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := db.Backup(tc.path)
			if err == nil {
				t.Fatalf("expected Backup(%q) to fail", tc.path)
			}
			// Ensure the error is a sanity-check rejection, not a downstream
			// VACUUM INTO error — those would mention "database backup failed".
			if strings.Contains(err.Error(), "database backup failed") {
				t.Fatalf("Backup reached VACUUM INTO with bad input %q: %v", tc.path, err)
			}
		})
	}
}
