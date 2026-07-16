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
	// FormatCompact runs the per-event-type compact processor (see
	// registry.LookupProcessor) and emits one flattened JSON line.
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

// NewFormatter returns a Formatter for the given Format. Compact wraps
// registry.LookupProcessor so adding a new specialised compactor is just
// a registry-side change. Returns an error only if format is an internally
// unsupported value (defensive — NormalizeFormat guarantees the input is
// one of the constants).
func NewFormatter(format Format) (Formatter, error) {
	switch format {
	case FormatNDJSON:
		return &ndjsonFormatter{}, nil
	case FormatJSON, FormatPretty:
		return &prettyFormatter{}, nil
	case FormatRaw:
		return &rawFormatter{}, nil
	case FormatCompact:
		return &compactFormatter{}, nil
	default:
		return nil, fmt.Errorf("consume: unsupported format %q", format)
	}
}

// ndjsonFormatter encodes each Event as one compact JSON line + '\n'.
type ndjsonFormatter struct{}

func (ndjsonFormatter) Render(ev transport.Event) ([]byte, error) {
	b, err := marshalEvent(ev)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// prettyFormatter encodes each Event as multi-line indented JSON + '\n'.
// json.MarshalIndent does not append a trailing newline; we add one so
// successive events are visually separated in the output.
type prettyFormatter struct{}

func (prettyFormatter) Render(ev transport.Event) ([]byte, error) {
	b, err := marshalEventIndent(ev, "", "  ")
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

// compactFormatter dispatches to the registry per event_type and writes
// the flattened map as one compact JSON line + '\n'.
type compactFormatter struct{}

func (compactFormatter) Render(ev transport.Event) ([]byte, error) {
	p := registry.LookupProcessor(ev.EventType)
	v := p(ev)
	b, err := marshalCompact(v)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
