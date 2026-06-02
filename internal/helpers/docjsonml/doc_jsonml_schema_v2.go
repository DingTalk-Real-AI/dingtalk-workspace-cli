package docjsonml

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed jsonml-schema-v2.json
var jsonmlSchemaV2Raw []byte

// TypeSpec represents a type constraint for an attribute value.
// Parsed from the schema JSON's unified object format: { "type": "...", ... }
type TypeSpec struct {
	Type      string              `json:"type"`                 // string|number|boolean|array|object|any|enum|union
	Min       *float64            `json:"min,omitempty"`        // for number
	Max       *float64            `json:"max,omitempty"`        // for number
	Values    []string            `json:"values,omitempty"`     // for enum
	Types     []string            `json:"types,omitempty"`      // for union: pass silently
	WarnTypes []string            `json:"warn_types,omitempty"` // for union: match → warning (not error)
	Fields    map[string]TypeSpec `json:"fields,omitempty"`     // for object (deep validation)
}

// TagSchema defines the schema for a single JSONML tag.
type TagSchema struct {
	AllowedChildren    []string            `json:"allowed_children"`
	Attrs              map[string]TypeSpec `json:"attrs"`
	allowedChildrenSet map[string]bool     // precomputed
}

// SchemaV2 is the top-level schema structure.
type SchemaV2 struct {
	Version     string                `json:"_version"`
	Description string                `json:"_description"`
	Tags        map[string]*TagSchema `json:"tags"`
	knownTags   map[string]bool       // precomputed: all tag names
}

// IsKnownTag returns true if the tag is declared in the schema.
func (s *SchemaV2) IsKnownTag(tag string) bool {
	return s.knownTags[tag]
}

// TagSchemaFor returns the schema for a tag, or nil if unknown.
func (s *SchemaV2) TagSchemaFor(tag string) *TagSchema {
	return s.Tags[tag]
}

// IsAllowedChild returns true if childTag is in the parent's allowed_children.
func (ts *TagSchema) IsAllowedChild(childTag string) bool {
	return ts.allowedChildrenSet[childTag]
}

func mustLoadSchemaV2(raw []byte) *SchemaV2 {
	var s SchemaV2
	if err := json.Unmarshal(raw, &s); err != nil {
		panic(fmt.Sprintf("jsonml-schema-v2.json parse failed: %v", err))
	}
	if len(s.Tags) == 0 {
		panic("jsonml-schema-v2.json: tags must be non-empty")
	}
	// Precompute sets
	s.knownTags = make(map[string]bool, len(s.Tags))
	for name, ts := range s.Tags {
		s.knownTags[name] = true
		ts.allowedChildrenSet = make(map[string]bool, len(ts.AllowedChildren))
		for _, c := range ts.AllowedChildren {
			ts.allowedChildrenSet[c] = true
		}
	}
	return &s
}

var schemaV2 = mustLoadSchemaV2(jsonmlSchemaV2Raw)

// validBlockTags is a set of block-level tags derived from the v2 schema.
// Inline tags (span, text, leaf) are excluded since they appear as children
// inside block nodes, not at body level.
var validBlockTags = func() map[string]bool {
	inline := map[string]bool{"span": true, "text": true, "leaf": true}
	m := make(map[string]bool)
	for tag := range schemaV2.Tags {
		if !inline[tag] {
			m[tag] = true
		}
	}
	return m
}()

// childStartIndex returns the index of the first child element in a JSONML
// node array, skipping the tag string and optional attrs object.
func childStartIndex(arr []any) int {
	if len(arr) > 1 {
		if _, ok := arr[1].(map[string]any); ok {
			return 2
		}
	}
	return 1
}
