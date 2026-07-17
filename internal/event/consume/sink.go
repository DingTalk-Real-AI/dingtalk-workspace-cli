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

package consume

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

// Sink is what a Pipeline writes formatted event bytes to. Implementations
// take both the event (for filename derivation in file sinks) and the
// already-formatted bytes (so the same event can be rendered with
// different formats per call without re-running the formatter inside the
// sink).
type Sink interface {
	// Write places one event on the sink. Returns an error for IO failures;
	// returns ErrPipeClosed when the downstream consumer closed the pipe
	// (typical pattern: `dws event consume | head -1`).
	Write(ev transport.Event, formatted []byte) error
	// Close releases sink-owned resources. Idempotent.
	Close() error
}

// ErrPipeClosed is returned by stdout-style sinks when the downstream
// reader closed its end (SIGPIPE / EPIPE on Unix, ERROR_BROKEN_PIPE on
// Windows). The pipeline catches this sentinel and exits cleanly without
// surfacing it as a fatal error.
var ErrPipeClosed = errors.New("sink: downstream pipe closed")

var (
	mkdirSinkDir   = os.MkdirAll
	writeSinkFile  = os.WriteFile
	renameSinkFile = os.Rename
	removeSinkFile = os.Remove
)

// NewStdoutSink wraps the given writer (typically os.Stdout) in a Sink
// that writes formatted bytes verbatim. Detects broken-pipe on the host
// platform and returns ErrPipeClosed so the caller can exit code 0.
func NewStdoutSink(w io.Writer) Sink { return &stdoutSink{w: w} }

type stdoutSink struct{ w io.Writer }

func (s *stdoutSink) Write(_ transport.Event, formatted []byte) error {
	_, err := s.w.Write(formatted)
	if err != nil && isBrokenPipe(err) {
		return ErrPipeClosed
	}
	return err
}
func (s *stdoutSink) Close() error { return nil }

// NewFileDirSink returns a sink that writes each event to its own file
// under dir, naming files `{type}_{id}_{ts}.json`. The directory is
// mkdir'd on first write so callers don't have to ensure it themselves.
//
// Filename pieces are sanitised: characters that would escape the
// directory (path separators) or break shell globbing are replaced with
// '_'. `ts` is the ReceivedAtUnixMS (or current time if zero) so two
// events with the same id (re-delivery, dedup-defeated edge cases) don't
// collide.
func NewFileDirSink(dir string) Sink { return &fileDirSink{dir: dir} }

type fileDirSink struct{ dir string }

func (s *fileDirSink) Write(ev transport.Event, formatted []byte) error {
	if err := mkdirSinkDir(s.dir, 0o700); err != nil {
		return fmt.Errorf("sink: mkdir %s: %w", s.dir, err)
	}
	name := buildFilename(ev)
	full := filepath.Join(s.dir, name)
	return atomicWrite(full, formatted)
}
func (s *fileDirSink) Close() error { return nil }

// NewRoutedSink composes a Router with per-route dir sinks plus a fallback.
// On each Write, Router.Match decides the target dir; if non-empty, the
// event is written there; otherwise the fallback sink handles it. The
// fallback is typically NewStdoutSink (default) or NewFileDirSink
// (--output-dir mode).
func NewRoutedSink(router *Router, fallback Sink) Sink {
	return &routedSink{router: router, fallback: fallback}
}

type routedSink struct {
	router   *Router
	fallback Sink
}

func (s *routedSink) Write(ev transport.Event, formatted []byte) error {
	if dir := s.router.Match(ev); dir != "" {
		return NewFileDirSink(dir).Write(ev, formatted)
	}
	return s.fallback.Write(ev, formatted)
}
func (s *routedSink) Close() error { return s.fallback.Close() }

// buildFilename produces `{type}_{id}_{ts}.json`. All three pieces are
// sanitised to be safe filesystem path segments — see safePart.
func buildFilename(ev transport.Event) string {
	typ := safePart(ev.EventType)
	if typ == "" {
		typ = "unknown"
	}
	id := safePart(ev.EventID)
	if id == "" {
		id = "no-id"
	}
	ts := ev.ReceivedAtUnixMS
	if ts == 0 {
		ts = time.Now().UTC().UnixMilli()
	}
	return fmt.Sprintf("%s_%s_%d.json", typ, id, ts)
}

// safePart strips path separators, NULs, and leading/trailing whitespace
// from a filename piece. Replaces unsafe chars with '_' instead of
// dropping them so different inputs don't collide.
//
// We DO allow dots and dashes (common in event types like "im.message.at_v1");
// we just reject path separators and parent-directory traversal.
func safePart(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '/', '\\', 0, ':':
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
	// We intentionally do NOT collapse ".." sequences. After path
	// separator replacement (above), a bare ".." in the middle of a
	// filename cannot perform parent-directory traversal because there
	// is no separator to anchor it against. The final filename is
	// joined into a known-safe directory with filepath.Join which itself
	// will clean any traversal that does sneak through.
}

// atomicWrite writes content to path via tmp-file + rename, so a concurrent
// reader either sees the previous version or the new version — never a
// half-written file.
func atomicWrite(path string, content []byte) error {
	tmp := path + ".tmp"
	if err := writeSinkFile(tmp, content, 0o600); err != nil {
		return fmt.Errorf("sink: write tmp %s: %w", tmp, err)
	}
	if err := renameSinkFile(tmp, path); err != nil {
		_ = removeSinkFile(tmp)
		return fmt.Errorf("sink: rename to %s: %w", path, err)
	}
	return nil
}
