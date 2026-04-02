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

// Package logging provides file-based structured logging for diagnostics.
// Logs are written as JSON lines to ~/.dws/logs/dws.log with automatic
// size-based rotation. Sensitive values (tokens, secrets) are never logged.
package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
)

const (
	// logSubDir is the subdirectory under configDir for log files.
	logSubDir = "logs"
	// logFileName is the active log file name.
	logFileName = "dws.log"
	// maxLogSize is the maximum size of a single log file (5 MB).
	maxLogSize = 5 * 1024 * 1024
	// maxBackups is the number of rotated backup files to keep.
	maxBackups = 2
)

// FileLogger wraps a rotatingWriter and provides a structured slog.Logger
// that writes JSON lines to disk.
type FileLogger struct {
	writer *rotatingWriter
	Logger *slog.Logger
}

// Setup creates the log directory and returns a FileLogger. If directory
// creation fails, it returns a no-op logger that discards all output.
func Setup(configDir string) *FileLogger {
	logDir := filepath.Join(configDir, logSubDir)
	if err := os.MkdirAll(logDir, config.DirPerm); err != nil {
		return newNopFileLogger()
	}

	logPath := filepath.Join(logDir, logFileName)
	w := newRotatingWriter(logPath, maxLogSize, maxBackups)
	if err := w.open(); err != nil {
		return newNopFileLogger()
	}

	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	return &FileLogger{
		writer: w,
		Logger: slog.New(handler),
	}
}

// Writer returns the underlying io.Writer for use with slog handlers.
// Returns io.Discard if the logger is not initialized.
func (fl *FileLogger) Writer() io.Writer {
	if fl == nil || fl.writer == nil {
		return io.Discard
	}
	return fl.writer
}

// Close flushes and closes the underlying log file.
func (fl *FileLogger) Close() error {
	if fl == nil || fl.writer == nil {
		return nil
	}
	return fl.writer.close()
}

func newNopFileLogger() *FileLogger {
	return &FileLogger{
		Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}
}

// rotatingWriter is a simple size-based log file rotator.
// When the active file exceeds maxSize, it renames:
//
//	dws.log   → dws.log.1
//	dws.log.1 → dws.log.2
//	dws.log.2 → (deleted)
//
// Then opens a fresh dws.log.
type rotatingWriter struct {
	path       string
	maxSize    int64
	maxBackups int

	mu      sync.Mutex
	file    *os.File
	written int64
}

func newRotatingWriter(path string, maxSize int64, maxBackups int) *rotatingWriter {
	return &rotatingWriter{
		path:       path,
		maxSize:    maxSize,
		maxBackups: maxBackups,
	}
}

func (w *rotatingWriter) open() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, config.FilePerm)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	w.file = f
	w.written = info.Size()
	return nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Attempt to recover if file was lost (e.g. rotation failed due to full disk).
	if w.file == nil {
		if err := w.reopenLocked(); err != nil {
			return 0, err
		}
	}

	if w.written+int64(len(p)) > w.maxSize {
		w.rotate()
	}

	// rotate may have failed, check again.
	if w.file == nil {
		return 0, os.ErrClosed
	}

	n, err := w.file.Write(p)
	w.written += int64(n)
	return n, err
}

// reopenLocked tries to reopen the log file. Caller must hold w.mu.
func (w *rotatingWriter) reopenLocked() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, config.FilePerm)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	w.file = f
	w.written = info.Size()
	return nil
}

func (w *rotatingWriter) close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *rotatingWriter) rotate() {
	w.file.Close()
	w.file = nil

	// Shift existing backups: .2 → delete, .1 → .2, active → .1
	for i := w.maxBackups; i >= 1; i-- {
		src := w.backupName(i - 1)
		dst := w.backupName(i)
		if i == w.maxBackups {
			os.Remove(dst)
		}
		os.Rename(src, dst)
	}

	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, config.FilePerm)
	if err != nil {
		return
	}
	w.file = f
	w.written = 0
}

func (w *rotatingWriter) backupName(index int) string {
	if index == 0 {
		return w.path
	}
	return w.path + "." + itoa(index)
}

func itoa(i int) string {
	return strconv.Itoa(i)
}
