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

package pipeline

// Phase represents a named stage in the CLI execution pipeline.
// Handlers are grouped by phase and executed in chain order within
// each phase. Phases themselves execute in a fixed order defined
// by the engine.
type Phase int

const (
	// Register runs at CLI startup when the Cobra command tree is
	// being built. Handlers can add, remove, or modify commands.
	Register Phase = iota

	// PreParse runs before Cobra parses the raw argv. Handlers
	// receive the raw argument slice and can rewrite it — for
	// example to fix flag-name typos or split glued values.
	PreParse

	// PostParse runs after Cobra has successfully parsed flags and
	// args. Handlers receive structured parameters plus the tool
	// schema, enabling value-level corrections such as date format
	// normalisation.
	PostParse

	// PreRequest runs after validation, just before the JSON-RPC
	// call is dispatched. Handlers can inspect or mutate the final
	// payload.
	PreRequest

	// PostResponse runs after the transport returns a result and
	// before the output is written to stdout.
	PostResponse
)

// String returns the human-readable name of the phase.
func (p Phase) String() string {
	switch p {
	case Register:
		return "register"
	case PreParse:
		return "pre-parse"
	case PostParse:
		return "post-parse"
	case PreRequest:
		return "pre-request"
	case PostResponse:
		return "post-response"
	default:
		return "unknown"
	}
}

// phases returns all phases in execution order.
func phases() []Phase {
	return []Phase{Register, PreParse, PostParse, PreRequest, PostResponse}
}

// Context carries mutable state through the handler chain. Each phase
// populates additional fields; earlier-phase fields remain available in
// later phases so that handlers can correlate raw input with structured
// parameters.
type Context struct {
	// Args is the raw argv slice (available from PreParse onward).
	// PreParse handlers may rewrite this in place.
	Args []string

	// Command is the resolved product.tool canonical path
	// (available from PostParse onward).
	Command string

	// Params holds structured key→value parameters after Cobra
	// parsing (available from PostParse onward). Handlers may
	// mutate values or add/remove keys.
	Params map[string]any

	// Schema is the JSON Schema for the resolved tool's input
	// (available from PostParse onward). Handlers must treat this
	// as read-only.
	Schema map[string]any

	// Payload is the merged, validated payload ready to be sent
	// over the wire (available from PreRequest onward).
	Payload map[string]any

	// Response is the JSON-RPC result returned by the server
	// (available from PostResponse onward). Handlers may mutate
	// the response before it is written to stdout.
	Response map[string]any

	// FlagSpecs provides the list of known flag names for the
	// current tool, derived from the input schema. PreParse
	// handlers use this to match against raw argv tokens.
	FlagSpecs []FlagInfo

	// Corrections records every correction applied by handlers,
	// enabling downstream logging and debugging.
	Corrections []Correction
}

// FlagInfo describes a single CLI flag derived from a tool's input
// schema. PreParse handlers use this to recognise valid flag names
// when performing fuzzy matching or alias resolution.
type FlagInfo struct {
	// Name is the canonical kebab-case flag name (e.g. "user-id").
	Name string

	// PropertyName is the original schema property key (e.g. "userId").
	PropertyName string

	// Type is the JSON Schema type ("string", "integer", etc.).
	Type string

	// Shorthand is the single-character short flag (e.g. "h" for --help).
	Shorthand string
}

// Correction records a single input correction applied by a handler.
type Correction struct {
	// Handler is the name of the handler that applied the correction.
	Handler string

	// Phase is the pipeline phase in which the correction occurred.
	Phase Phase

	// Field identifies the affected flag or parameter name.
	Field string

	// Original is the value before correction.
	Original string

	// Corrected is the value after correction.
	Corrected string

	// Kind classifies the correction (e.g. "alias", "sticky", "case").
	Kind string
}

// AddCorrection appends a correction record to the context.
func (c *Context) AddCorrection(handler string, phase Phase, field, original, corrected, kind string) {
	c.Corrections = append(c.Corrections, Correction{
		Handler:   handler,
		Phase:     phase,
		Field:     field,
		Original:  original,
		Corrected: corrected,
		Kind:      kind,
	})
}
