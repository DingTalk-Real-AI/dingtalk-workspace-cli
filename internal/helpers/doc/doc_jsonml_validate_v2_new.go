//go:build goexperiment.jsonv2

package doc

import (
	"encoding/json"
)

// ValidateJsonMLBodyV2 validates a JSONML body using schema-v2.
// This implementation delegates to the new Parse → ValidateWithSchema pipeline,
// providing position-aware diagnostics. The signature is unchanged for backward
// compatibility (SPEC §8.4).
//
// Note: Since input is already []any, we marshal it back to JSON bytes to feed
// the parser. Positions are relative to the marshaled output. The full pipeline
// (parsing raw input directly) provides original-source positions.
//
// Behavioral note: The old implementation interprets a body whose first element
// is an unknown tag (not "root", not a known schema tag) as an "array of blocks"
// and would error on each non-array element. The new Parse-based implementation
// correctly interprets ["unknownTag", ...] as a single JSONML element with an
// unknown tag (warning). This is semantically more correct for JSONML.
func ValidateJsonMLBodyV2(body []any) *JsonMLValidationResult {
	if len(body) == 0 {
		return &JsonMLValidationResult{}
	}

	// Marshal body back to JSON bytes for the parser
	src, err := json.Marshal(body)
	if err != nil {
		r := &JsonMLValidationResult{}
		r.Errors = append(r.Errors, "internal: failed to marshal body for validation: "+err.Error())
		return r
	}

	// Layer 0 + Layer 1: Parse
	doc := Parse(src)

	// Layer 2: Schema validation
	schemaDiags := ValidateWithSchema(doc, schemaV2)

	// Merge all diagnostics
	allDiags := append(doc.Diagnostics, schemaDiags...)

	return allDiags.ToLegacyResult()
}

// ValidateJsonMLNodeV2 validates a single JSONML node using schema-v2.
// Delegates to the new Parse → ValidateWithSchema pipeline.
func ValidateJsonMLNodeV2(node any) *JsonMLValidationResult {
	// Marshal single node to JSON bytes
	src, err := json.Marshal(node)
	if err != nil {
		r := &JsonMLValidationResult{}
		r.Errors = append(r.Errors, "internal: failed to marshal node for validation: "+err.Error())
		return r
	}

	// Layer 0 + Layer 1: Parse
	doc := Parse(src)

	// Layer 2: Schema validation
	schemaDiags := ValidateWithSchema(doc, schemaV2)

	// Merge all diagnostics
	allDiags := append(doc.Diagnostics, schemaDiags...)

	return allDiags.ToLegacyResult()
}

// ValidateJsonMLSource validates raw JSONML source bytes directly,
// preserving original line/column positions in diagnostics.
// This should be preferred over ValidateJsonMLBodyV2 when the original
// source bytes are available (e.g., from a file).
//
// The input can be:
//   - A bare JSONML array: ["root", {}, ...]
//   - A wrapped object: {"jsonml": [...]}
func ValidateJsonMLSource(src []byte) *JsonMLValidationResult {
	if len(src) == 0 {
		return &JsonMLValidationResult{}
	}

	// Layer 0 + Layer 1: Parse
	doc := Parse(src)

	// Layer 2: Schema validation
	schemaDiags := ValidateWithSchema(doc, schemaV2)

	// Merge all diagnostics
	allDiags := append(doc.Diagnostics, schemaDiags...)

	return allDiags.ToLegacyResult()
}
