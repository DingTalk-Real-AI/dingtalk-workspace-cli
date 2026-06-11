// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// dayLayout is the date stamp embedded in each rotated file name.
const dayLayout = "2006-01-02"

// DateRotatingWriter appends audit lines to a per-day file
// (`<dir>/<prefix>-YYYY-MM-DD.jsonl`), rolling to a new file when the local
// calendar day changes, and pruning files older than maxAgeDays. It is safe for
// concurrent use.
//
// Why date-based: audit/access trails are conventionally sliced by day so a
// single file never grows unbounded and retention is a simple per-file delete.
// For the common short-lived CLI process this just opens today's file; a
// long-running mode (e.g. the stdio server) rolls at midnight because the day is
// re-checked on every write.
type DateRotatingWriter struct {
	dir        string
	prefix     string
	maxAgeDays int // <= 0 keeps everything
	perm       os.FileMode
	dirPerm    os.FileMode
	now        func() time.Time // injectable for tests

	mu     sync.Mutex
	curDay string
	f      *os.File
}

// NewDateRotatingWriter builds a writer rooted at dir. Files are named
// "<prefix>-YYYY-MM-DD.jsonl"; files older than maxAgeDays are pruned on each
// roll (maxAgeDays <= 0 disables pruning).
func NewDateRotatingWriter(dir, prefix string, maxAgeDays int, perm, dirPerm os.FileMode) *DateRotatingWriter {
	return &DateRotatingWriter{
		dir:        dir,
		prefix:     prefix,
		maxAgeDays: maxAgeDays,
		perm:       perm,
		dirPerm:    dirPerm,
		now:        time.Now,
	}
}

// Write appends p to today's file, rolling first if the day changed.
func (w *DateRotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	day := w.now().Format(dayLayout)
	if w.f == nil || day != w.curDay {
		if err := w.openDay(day); err != nil {
			return 0, err
		}
	}
	return w.f.Write(p)
}

// Close closes the current file (safe to call multiple times).
func (w *DateRotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	err := w.f.Close()
	w.f = nil
	return err
}

// openDay closes any current file and opens the file for `day`, then prunes old
// files. Caller holds the lock.
func (w *DateRotatingWriter) openDay(day string) error {
	if w.f != nil {
		_ = w.f.Close()
		w.f = nil
	}
	if err := os.MkdirAll(w.dir, w.dirPerm); err != nil {
		return err
	}
	name := filepath.Join(w.dir, fmt.Sprintf("%s-%s.jsonl", w.prefix, day))
	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, w.perm)
	if err != nil {
		return err
	}
	w.f = f
	w.curDay = day
	w.prune(day) // best-effort; failures must not block auditing
	return nil
}

// prune removes "<prefix>-*.jsonl" files whose embedded date is older than
// maxAgeDays relative to `today`.
func (w *DateRotatingWriter) prune(today string) {
	if w.maxAgeDays <= 0 {
		return
	}
	t, err := time.Parse(dayLayout, today)
	if err != nil {
		return
	}
	cutoff := t.AddDate(0, 0, -w.maxAgeDays)

	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}
	pfx := w.prefix + "-"
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, pfx) || !strings.HasSuffix(n, ".jsonl") {
			continue
		}
		datePart := strings.TrimSuffix(strings.TrimPrefix(n, pfx), ".jsonl")
		d, err := time.Parse(dayLayout, datePart)
		if err != nil {
			continue // not a dated file we manage
		}
		if d.Before(cutoff) {
			_ = os.Remove(filepath.Join(w.dir, n))
		}
	}
}
