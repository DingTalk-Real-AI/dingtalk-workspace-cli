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

package transport

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// MaxFrameBytes caps a single frame's wire size. 1 MiB accommodates large
// event payloads (kanban cards, doc snapshots) with headroom while still
// preventing a buggy/malicious peer from causing OOM via an unbounded line.
// Callers writing frames larger than this should split their work, not bump
// the cap — the SDK enforces its own cloud-side cap that is well below this.
const MaxFrameBytes = 1 << 20 // 1 MiB

// ErrFrameTooLarge is returned by Reader.Read when an incoming frame exceeds
// MaxFrameBytes. The underlying connection is left in an indeterminate
// state (the rest of the oversized frame is NOT drained) — callers should
// close the connection on receipt of this error.
var ErrFrameTooLarge = errors.New("transport: frame exceeds MaxFrameBytes")

// Reader reads \n-delimited JSON frames from an underlying byte stream.
// Wrap each connection (one per direction) in a Reader; Reader is not
// safe for concurrent use.
type Reader struct {
	br *bufio.Reader
}

// NewReader returns a Reader with a buffer sized to MaxFrameBytes so a
// single frame can fit in the buffer without growing.
func NewReader(r io.Reader) *Reader {
	return &Reader{br: bufio.NewReaderSize(r, MaxFrameBytes)}
}

// Read returns the next frame's raw bytes (the terminating \n is stripped).
// Returns io.EOF cleanly when the peer closes the connection between
// frames. Returns ErrFrameTooLarge when a single frame exceeds the cap.
func (r *Reader) Read() ([]byte, error) {
	line, err := r.br.ReadSlice('\n')
	if err == nil {
		// Strip trailing \n. Safe because ReadSlice always includes the
		// delimiter when err == nil.
		out := make([]byte, len(line)-1)
		copy(out, line[:len(line)-1])
		return out, nil
	}
	if errors.Is(err, bufio.ErrBufferFull) {
		// Read past current contents (without retaining) to skip the
		// remainder of the oversize frame would be the friendly path,
		// but plan says callers should close the connection — keep it
		// simple and surface ErrFrameTooLarge immediately.
		return nil, ErrFrameTooLarge
	}
	// EOF on a partial line: report it as EOF (clean close between frames
	// returns a fresh EOF with an empty line, already handled above).
	if errors.Is(err, io.EOF) && len(line) == 0 {
		return nil, io.EOF
	}
	return nil, err
}

// ReadJSON decodes the next frame into dst. dst must be a non-nil pointer.
func (r *Reader) ReadJSON(dst any) error {
	raw, err := r.Read()
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("transport: decode frame: %w", err)
	}
	return nil
}

// Writer marshals values to JSON and writes them as \n-terminated frames.
// Writer is not safe for concurrent use; callers wanting a fan-in writer
// must serialise externally.
type Writer struct {
	w io.Writer
}

// NewWriter returns a Writer wrapping w. The Writer does NOT buffer —
// every WriteJSON call performs a single Write to the underlying stream
// so backpressure is immediate.
func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

// WriteJSON serialises v and writes it as one frame. Refuses to write
// frames larger than MaxFrameBytes with ErrFrameTooLarge.
func (w *Writer) WriteJSON(v any) error {
	buf, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("transport: encode frame: %w", err)
	}
	if len(buf)+1 > MaxFrameBytes {
		return ErrFrameTooLarge
	}
	buf = append(buf, '\n')
	_, err = w.w.Write(buf)
	return err
}
