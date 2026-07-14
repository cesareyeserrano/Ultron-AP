package server

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cesareyeserrano/ultron-ap/internal/database"
)

// backupFileRe-shaped naming: the automated backup writes ultron-<ts>.db and,
// when encryption is on, ultron-<ts>.db.enc. Only those artefacts are listed or
// served — the same "only our own files" rule the retention job applies (M11),
// so a stray file in a shared LocalPath directory can never be downloaded
// through this route.
const backupFilePrefix = "ultron-"

// backupFile is one row of the Settings backup listing (FR-084 / AC-015-004).
type backupFile struct {
	Name      string
	SizeBytes int64
	Size      string
	CreatedAt time.Time
	Created   string
	Encrypted bool
}

// backupDir resolves the directory the automated backup writes into. It mirrors
// performAutomatedBackup's resolution, including the run-time re-validation of
// a configured LocalPath override (a symlink component planted after the config
// was saved must not redirect us outside BackupRoot).
func (s *Server) backupDir() (string, error) {
	dir := filepath.Join(filepath.Dir(s.cfg.DBPath), "backups")

	cfg, err := s.db.GetBackupConfig()
	if err != nil {
		return dir, nil // fall back to the default location; listing is read-only
	}
	if strings.TrimSpace(cfg.LocalPath) == "" {
		return dir, nil
	}

	validated, err := database.ValidateBackupPath(cfg.LocalPath, s.cfg.BackupRoot)
	if err != nil {
		return "", fmt.Errorf("backup local path invalid: %w", err)
	}
	return validated, nil
}

// listBackupFiles returns the backup artefacts on disk, newest first.
func (s *Server) listBackupFiles() ([]backupFile, error) {
	dir, err := s.backupDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil // no backups yet — an empty list, not an error
	}
	if err != nil {
		return nil, fmt.Errorf("read backup dir: %w", err)
	}

	var files []backupFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), backupFilePrefix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, backupFile{
			Name:      e.Name(),
			SizeBytes: info.Size(),
			Size:      formatBytes(uint64(info.Size())),
			CreatedAt: info.ModTime(),
			Created:   info.ModTime().Format("2006-01-02 15:04"),
			Encrypted: strings.HasSuffix(e.Name(), ".enc"),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].CreatedAt.After(files[j].CreatedAt)
	})
	return files, nil
}

// handleBackupDownload handles GET /api/settings/backups/{name} (FR-084).
//
// It serves the STORED file — encrypted when it was written encrypted — rather
// than generating a fresh plaintext snapshot the way the legacy
// GET /api/settings/backup route does. The encryption key never leaves the
// server: this route only ever reads bytes that are already on disk.
func (s *Server) handleBackupDownload(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	// The filename is client-supplied, so it is never joined to a directory
	// until it has been proven to be a bare basename of one of OUR artefacts.
	// filepath.Base alone is not enough — it would happily accept "..".
	if name == "" || name != filepath.Base(name) || !strings.HasPrefix(name, backupFilePrefix) {
		http.Error(w, "Invalid backup name", http.StatusBadRequest)
		return
	}

	dir, err := s.backupDir()
	if err != nil {
		log.Printf("settings: backup dir unavailable: %v", err)
		http.Error(w, "Backup directory unavailable", http.StatusInternalServerError)
		return
	}

	path := filepath.Join(dir, name)

	// Re-check containment after resolving symlinks: a symlink planted inside
	// the backup dir must not become a read primitive for arbitrary files.
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		http.Error(w, "Backup not found", http.StatusNotFound)
		return
	}
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		http.Error(w, "Backup directory unavailable", http.StatusInternalServerError)
		return
	}
	if !strings.HasPrefix(resolved, resolvedDir+string(os.PathSeparator)) {
		log.Printf("settings: rejecting backup download outside the backup dir: %q", name)
		http.Error(w, "Invalid backup name", http.StatusBadRequest)
		return
	}

	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() || !info.Mode().IsRegular() {
		http.Error(w, "Backup not found", http.StatusNotFound)
		return
	}

	f, err := os.Open(resolved)
	if err != nil {
		log.Printf("settings: failed to open backup %q: %v", name, err)
		http.Error(w, "Backup not found", http.StatusNotFound)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Disposition", "attachment; filename="+name)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	if _, err := io.Copy(w, f); err != nil {
		log.Printf("settings: backup download interrupted for %q: %v", name, err)
	}

	s.auditLog(r, "settings", "backup_download", name, "", true)
}
