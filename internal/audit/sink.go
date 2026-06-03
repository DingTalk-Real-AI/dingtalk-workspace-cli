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
	"encoding/json"
	"io"
	"sync"
)

// Sink consumes audit events. Implementations must be safe for concurrent use
// and must never block the command on slow I/O for longer than they have to —
// auditing is a side effect, not a gate on the user's command.
type Sink interface {
	Emit(e *Event) error
}

// FileSink appends one JSON object per line (JSONL) to a writer the operator
// owns. This is the transparent, always-available channel: the source of truth
// the user/customer can inspect with grep. It writes the FULL event verbatim —
// the local file is inside the operator's own trust boundary.
type FileSink struct {
	mu sync.Mutex
	w  io.Writer
}

// NewFileSink wraps w (typically a rotating file writer).
func NewFileSink(w io.Writer) *FileSink {
	return &FileSink{w: w}
}

// Emit serializes e as a single JSONL line. Marshal happens under the lock-free
// section; only the write is serialized.
func (s *FileSink) Emit(e *Event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.w.Write(data)
	return err
}

// RedactingSink wraps another Sink, applying a RedactLevel before forwarding.
// Use this to point a forwarder at a remote endpoint while guaranteeing the
// content tier shipped off-box matches policy — the wrapped sink never sees the
// raw event.
type RedactingSink struct {
	Inner Sink
	Level RedactLevel
	Salt  string
}

// Emit redacts then delegates.
func (s *RedactingSink) Emit(e *Event) error {
	return s.Inner.Emit(e.Redact(s.Level, s.Salt))
}

// MultiSink fans an event out to several sinks, collecting the first error but
// always attempting every sink (one failing forwarder must not starve the
// local file).
type MultiSink struct {
	Sinks []Sink
}

// Emit delivers to all sinks.
func (m *MultiSink) Emit(e *Event) error {
	var firstErr error
	for _, s := range m.Sinks {
		if s == nil {
			continue
		}
		if err := s.Emit(e); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// NopSink discards events. Default when auditing is disabled — emitting is
// always safe, so callers never need a nil check.
type NopSink struct{}

// Emit does nothing.
func (NopSink) Emit(*Event) error { return nil }
