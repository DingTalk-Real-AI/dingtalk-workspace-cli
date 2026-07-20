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
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/registry"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

var (
	marshalEvent       = json.Marshal
	marshalEventIndent = json.MarshalIndent
	marshalCompact     = json.Marshal
)

// Format identifies the wire shape `dws event consume` writes per event.
// Values mirror dws's global -f/--format flag vocabulary (defined in
// internal/output) with the subset that makes sense for streaming.
type Format string

const (
	// FormatNDJSON is the default: one compact JSON object per line.
	// Pipe-friendly; one event per `read` line. Recommended for agents.
	FormatNDJSON Format = "ndjson"
	// FormatJSON pretty-prints each event as multi-line JSON. NOT a
	// JSON array — still NDJSON-style (one document per output unit),
	// just with indentation. See plan §3.1 输出约束 note about why we
	// do not emit a JSON array for an unbounded stream.
	FormatJSON Format = "json"
	// FormatPretty is the same as FormatJSON in v1; reserved for future
	// human-friendly colorisation. Kept distinct so we never silently
	// degrade `--format pretty` to compact-ndjson.
	FormatPretty Format = "pretty"
	// FormatRaw writes only the SDK's original Data string (one per
	// event, newline-terminated). Useful when piping into jq / a tool
	// that wants the cloud payload verbatim without our envelope.
	FormatRaw Format = "raw"
	// FormatCompact emits one compact JSON line. Personal streams use their
	// configured business projection; other streams use the registry processor.
	FormatCompact Format = "compact"
)

// NormalizeFormat maps a raw flag value to a supported Format. Values
// outside the event command's supported set fall back to NDJSON with the
// fallback flag set true — callers SHOULD warn on stderr when fallback is
// true and the original value was non-empty (e.g. user passed
// --format table which has no meaning for an event stream).
//
// Empty input maps to NDJSON without a fallback warning.
func NormalizeFormat(raw string) (f Format, fellback bool) {
	switch raw {
	case "":
		return FormatNDJSON, false
	case string(FormatNDJSON):
		return FormatNDJSON, false
	case string(FormatJSON):
		return FormatJSON, false
	case string(FormatPretty):
		return FormatPretty, false
	case string(FormatRaw):
		return FormatRaw, false
	case string(FormatCompact):
		return FormatCompact, false
	default:
		// Includes table/csv from the global -f vocabulary, plus any
		// typo. Fall back to ndjson (the safe stream default) and let
		// the caller stderr-WARN.
		return FormatNDJSON, true
	}
}

// Formatter renders a transport.Event into the byte stream the sink writes
// out. Implementations append their own line terminator when appropriate
// (NDJSON / Raw add '\n'; Pretty/JSON embed newlines in the JSON itself).
type Formatter interface {
	Render(ev transport.Event) ([]byte, error)
}

// Projector maps a transport envelope to the public value rendered by
// structured formats. Returning a value together with an error means the
// value is a safe fallback and should still be emitted after a warning.
type Projector func(ev transport.Event) (any, error)

type formatterConfig struct {
	projector Projector
	warnings  io.Writer
	warnOnce  sync.Once
}

// FormatterOption configures structured event rendering. Raw output always
// bypasses these options and preserves the source Data string.
type FormatterOption func(*formatterConfig)

func WithProjector(projector Projector) FormatterOption {
	return func(cfg *formatterConfig) { cfg.projector = projector }
}

func WithProjectionWarnings(w io.Writer) FormatterOption {
	return func(cfg *formatterConfig) {
		if w != nil {
			cfg.warnings = w
		}
	}
}

// NewFormatter returns a Formatter for the given Format. When no projector
// is configured, compact dispatches through registry.LookupProcessor. Returns
// an error only if format is internally unsupported.
func NewFormatter(format Format, opts ...FormatterOption) (Formatter, error) {
	cfg := &formatterConfig{warnings: io.Discard}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	switch format {
	case FormatNDJSON:
		return &structuredFormatter{config: cfg}, nil
	case FormatJSON, FormatPretty:
		return &structuredFormatter{config: cfg, pretty: true}, nil
	case FormatRaw:
		return &rawFormatter{}, nil
	case FormatCompact:
		return &structuredFormatter{config: cfg, compact: true}, nil
	default:
		return nil, fmt.Errorf("consume: unsupported format %q", format)
	}
}

// structuredFormatter renders either the transport envelope or a projected
// business value. The same projector is used by ndjson/json/pretty/compact.
type structuredFormatter struct {
	config  *formatterConfig
	pretty  bool
	compact bool
}

func (f *structuredFormatter) Render(ev transport.Event) ([]byte, error) {
	value := any(ev)
	if f.config.projector != nil {
		projected, err := f.config.projector(ev)
		if projected != nil {
			value = projected
		}
		if err != nil {
			f.config.warnOnce.Do(func() {
				fmt.Fprintf(f.config.warnings,
					"WARN: personal event output projection failed; using raw envelope event_id=%q event_type=%q: %v\n",
					ev.EventID, ev.EventType, err)
			})
		}
	} else if f.compact {
		value = registry.LookupProcessor(ev.EventType)(ev)
	}
	var (
		b   []byte
		err error
	)
	switch {
	case f.pretty:
		b, err = marshalEventIndent(value, "", "  ")
	case f.compact:
		b, err = marshalCompact(value)
	default:
		b, err = marshalEvent(value)
	}
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// The basic formatters remain available for focused encoder tests. The public
// construction path uses structuredFormatter so personal event projections are
// applied consistently across all structured formats.
type ndjsonFormatter struct{}

func (ndjsonFormatter) Render(ev transport.Event) ([]byte, error) {
	b, err := marshalEvent(ev)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

type prettyFormatter struct{}

func (prettyFormatter) Render(ev transport.Event) ([]byte, error) {
	b, err := marshalEventIndent(ev, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

type compactFormatter struct{}

func (compactFormatter) Render(ev transport.Event) ([]byte, error) {
	value := registry.LookupProcessor(ev.EventType)(ev)
	b, err := marshalCompact(value)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// rawFormatter writes ev.Data verbatim. If Data is already JSON it stays
// JSON; if it's some other string format it stays that. A trailing newline
// is appended so successive raw events are separable.
type rawFormatter struct{}

func (rawFormatter) Render(ev transport.Event) ([]byte, error) {
	out := make([]byte, 0, len(ev.Data)+1)
	out = append(out, ev.Data...)
	if len(ev.Data) == 0 || ev.Data[len(ev.Data)-1] != '\n' {
		out = append(out, '\n')
	}
	return out, nil
}
