package database

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// ErrBackupPathInvalid is returned by ValidateBackupPath for any rejection.
// Wrap with fmt.Errorf("...: %w", err) when surfacing to callers.
var ErrBackupPathInvalid = errors.New("invalid backup path")

// ValidateBackupPath enforces the BL-005 constraints on an admin-supplied
// backup destination directory. The empty string is allowed and means "use
// the default under root". The returned cleaned path is what callers should
// store/use; on error it is empty.
//
// Rules:
//   - no NUL bytes or other control characters in the input
//   - must be an absolute path
//   - after Clean, must be inside root (no ".." escape)
//   - none of the existing path components from root downward may be a
//     symlink (defense against admin pointing the dir at a symlink that
//     redirects VACUUM INTO outside root)
//
// The path itself does not need to exist yet — the backup job creates it.
func ValidateBackupPath(input, root string) (string, error) {
	if root == "" || !filepath.IsAbs(root) {
		return "", fmt.Errorf("%w: backup root not configured or not absolute", ErrBackupPathInvalid)
	}
	cleanRoot := filepath.Clean(root)

	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", nil
	}

	if strings.ContainsRune(trimmed, 0) {
		return "", fmt.Errorf("%w: contains NUL byte", ErrBackupPathInvalid)
	}
	for _, r := range trimmed {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("%w: contains control character", ErrBackupPathInvalid)
		}
	}

	if !filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("%w: must be an absolute path", ErrBackupPathInvalid)
	}

	clean := filepath.Clean(trimmed)

	rel, err := filepath.Rel(cleanRoot, clean)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: must be inside %s", ErrBackupPathInvalid, cleanRoot)
	}

	if err := rejectSymlinkChain(cleanRoot, clean); err != nil {
		return "", err
	}

	return clean, nil
}

// rejectSymlinkChain walks each existing component from root down to target
// and returns an error if any is a symlink. Components that don't exist yet
// are fine — the backup job will create them. Root itself must exist.
func rejectSymlinkChain(root, target string) error {
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("%w: cannot stat backup root %s: %v", ErrBackupPathInvalid, root, err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: backup root %s is a symlink", ErrBackupPathInvalid, root)
	}

	rel, err := filepath.Rel(root, target)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBackupPathInvalid, err)
	}
	if rel == "." {
		return nil
	}

	cur := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) {
				return nil // remaining components don't exist yet — safe.
			}
			return fmt.Errorf("%w: cannot stat %s: %v", ErrBackupPathInvalid, cur, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: component %s is a symlink", ErrBackupPathInvalid, cur)
		}
	}
	return nil
}
