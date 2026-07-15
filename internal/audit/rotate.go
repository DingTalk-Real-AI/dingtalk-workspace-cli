package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	auditLockFile    = ".audit.lock"
	auditLockTimeout = 3 * time.Second
	auditLockRetry   = 20 * time.Millisecond
)

var (
	rotateLockFile    = lockFile
	rotateLockTimeout = auditLockTimeout
)

type DateRotatingWriter struct {
	mu        sync.Mutex
	dir       string
	curDate   string
	file      *os.File
	lock      *os.File
	retention int
}

func NewDateRotatingWriter(dir string, retentionDays int) (*DateRotatingWriter, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("audit: create dir: %w", err)
	}
	lock, err := os.OpenFile(filepath.Join(dir, auditLockFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("audit: open lock file: %w", err)
	}
	w := &DateRotatingWriter{
		dir:       dir,
		retention: retentionDays,
		lock:      lock,
	}
	go w.pruneOldFiles()
	return w, nil
}

// beginAppend serializes writers within this process (mu) and across processes
// (flock), rotates to today's file, and returns the open handle plus a release
// func that unlocks in reverse order. The file is opened O_RDWR|O_APPEND so the
// chain can read the tail while every write still lands atomically at EOF even
// when another dws process appends concurrently.
func (w *DateRotatingWriter) beginAppend() (*os.File, func(), error) {
	w.mu.Lock()
	if err := w.acquireLock(); err != nil {
		w.mu.Unlock()
		return nil, nil, err
	}

	today := time.Now().Format("20060102")
	if today != w.curDate || w.file == nil {
		if w.file != nil {
			_ = w.file.Close()
			w.file = nil
		}
		path := filepath.Join(w.dir, fmt.Sprintf("audit-%s.jsonl", today))
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
		if err != nil {
			unlockFile(w.lock)
			w.mu.Unlock()
			return nil, nil, fmt.Errorf("audit: open file: %w", err)
		}
		w.file = f
		w.curDate = today
	}

	release := func() {
		unlockFile(w.lock)
		w.mu.Unlock()
	}
	return w.file, release, nil
}

func (w *DateRotatingWriter) acquireLock() error {
	deadline := time.Now().Add(rotateLockTimeout)
	for {
		if err := rotateLockFile(w.lock); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("audit: timeout acquiring file lock after %v (another dws process may be writing)", rotateLockTimeout)
		}
		time.Sleep(auditLockRetry)
	}
}

func (w *DateRotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	var firstErr error
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			firstErr = err
		}
		w.file = nil
	}
	if w.lock != nil {
		if err := w.lock.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		w.lock = nil
	}
	return firstErr
}

func (w *DateRotatingWriter) pruneOldFiles() {
	if w.retention <= 0 {
		return
	}
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -w.retention).Format("20060102")
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "audit-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		dateStr := strings.TrimPrefix(name, "audit-")
		dateStr = strings.TrimSuffix(dateStr, ".jsonl")
		if len(dateStr) != 8 {
			continue
		}
		if dateStr < cutoff {
			_ = os.Remove(filepath.Join(w.dir, name))
		}
	}
}

func (w *DateRotatingWriter) Dir() string {
	return w.dir
}

func LatestAuditFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var files []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "audit-") && strings.HasSuffix(e.Name(), ".jsonl") {
			files = append(files, e.Name())
		}
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no audit files found in %s", dir)
	}
	sort.Strings(files)
	return filepath.Join(dir, files[len(files)-1]), nil
}

func AuditFilesInRange(dir, since, until string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "audit-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		dateStr := strings.TrimPrefix(name, "audit-")
		dateStr = strings.TrimSuffix(dateStr, ".jsonl")
		if len(dateStr) != 8 {
			continue
		}
		if (since == "" || dateStr >= since) && (until == "" || dateStr <= until) {
			files = append(files, filepath.Join(dir, name))
		}
	}
	sort.Strings(files)
	return files, nil
}
