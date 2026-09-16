// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package opennodes owns the local, side-effect-free OpenNodes V1 source
// representation shared by create guards and visual preview renderers.
package opennodes

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const (
	SchemaVersion  = "1.0"
	CatalogVersion = "dml-v1"
)

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-fA-F]{64}$`)

// Source is the stable source object sent to independent-whiteboard creation.
// Node fields remain open for catalog compatibility, except for explicit local
// guards against known server-invalid content. This is not a full schema validator.
type Source struct {
	SchemaVersion  string           `json:"schemaVersion"`
	CatalogVersion string           `json:"catalogVersion"`
	Nodes          []map[string]any `json:"nodes"`
}

type sourceEnvelope struct {
	Overwrite bool            `json:"overwrite,omitempty"`
	Source    json.RawMessage `json:"source"`
}

type rawSource struct {
	SchemaVersion  string          `json:"schemaVersion"`
	CatalogVersion string          `json:"catalogVersion"`
	Nodes          json.RawMessage `json:"nodes"`
}

// Parse accepts either a direct OpenNodes source object or the historical
// {"source": ...} update wrapper. Envelope/source unknown fields are rejected;
// individual node fields remain extensible for compatibility.
func Parse(data []byte) (*Source, error) {
	var top map[string]json.RawMessage
	if err := decodeOne(data, &top, false); err != nil {
		return nil, fmt.Errorf("OpenNodes source must be one JSON object: %w", err)
	}
	if top == nil {
		return nil, errors.New("OpenNodes source must be a JSON object")
	}

	sourceData := data
	if _, wrapped := top["source"]; wrapped {
		var envelope sourceEnvelope
		if err := decodeOne(data, &envelope, true); err != nil {
			return nil, fmt.Errorf("invalid OpenNodes source envelope: %w", err)
		}
		if len(envelope.Source) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Source), []byte("null")) {
			return nil, errors.New("source is required")
		}
		sourceData = envelope.Source
	}

	var raw rawSource
	if err := decodeOne(sourceData, &raw, true); err != nil {
		return nil, fmt.Errorf("invalid OpenNodes source: %w", err)
	}
	if raw.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("source.schemaVersion must be %q", SchemaVersion)
	}
	if raw.CatalogVersion != CatalogVersion {
		return nil, fmt.Errorf("source.catalogVersion must be %q", CatalogVersion)
	}
	if len(raw.Nodes) == 0 || !bytes.HasPrefix(bytes.TrimSpace(raw.Nodes), []byte("[")) {
		return nil, errors.New("source.nodes must be an array")
	}

	decoder := json.NewDecoder(bytes.NewReader(raw.Nodes))
	decoder.UseNumber()
	var values []any
	if err := decoder.Decode(&values); err != nil {
		return nil, fmt.Errorf("source.nodes must be an array: %w", err)
	}
	if values == nil {
		return nil, errors.New("source.nodes cannot be null")
	}
	if err := requireEOF(decoder); err != nil {
		return nil, fmt.Errorf("source.nodes must contain one JSON array: %w", err)
	}

	nodes := make([]map[string]any, len(values))
	for index, value := range values {
		node, ok := value.(map[string]any)
		if !ok || node == nil {
			return nil, fmt.Errorf("source.nodes[%d] must be an object", index)
		}
		nodes[index] = node
		if err := validateTextRuns(node["text"], fmt.Sprintf("/source/nodes/%d/text", index), node["id"]); err != nil {
			return nil, err
		}
		if title, ok := node["title"].(map[string]any); ok {
			if err := validateTextRuns(title["text"], fmt.Sprintf("/source/nodes/%d/title/text", index), node["id"]); err != nil {
				return nil, err
			}
		}
	}
	return &Source{SchemaVersion: SchemaVersion, CatalogVersion: CatalogVersion, Nodes: nodes}, nil
}

// OpenNodesUpdate.validateTextRun represents line boundaries with blocks.
// Do not normalize here: changing content invalidates an approved preview.
type TextRunValidationError struct {
	NodeID any
	Path   string
}

func (e *TextRunValidationError) Error() string {
	return fmt.Sprintf("节点 %v 的 %s 含非法换行符；请拆成独立 paragraph blocks（空行使用空文本段落），保留样式后重新 render 并确认预览", e.NodeID, e.Path)
}

func validateTextRuns(value any, path string, nodeID any) error {
	text, _ := value.(map[string]any)
	blocks, _ := text["blocks"].([]any)
	for blockIndex, item := range blocks {
		block, _ := item.(map[string]any)
		runs, _ := block["runs"].([]any)
		for runIndex, item := range runs {
			run, _ := item.(map[string]any)
			content, _ := run["text"].(string)
			if strings.ContainsAny(content, "\r\n\u2028\u2029") {
				return &TextRunValidationError{NodeID: nodeID, Path: fmt.Sprintf("%s/blocks/%d/runs/%d/text", path, blockIndex, runIndex)}
			}
		}
	}
	return nil
}

// CanonicalJSON returns the deterministic direct source representation used by
// independent-whiteboard creation and digest calculation.
func CanonicalJSON(source *Source) ([]byte, error) {
	if source == nil {
		return nil, errors.New("OpenNodes source is nil")
	}
	canonical := Source{
		SchemaVersion:  SchemaVersion,
		CatalogVersion: CatalogVersion,
		Nodes:          source.Nodes,
	}
	return json.Marshal(canonical)
}

// DigestSource binds a visual preview to the exact canonical source submitted
// by create-with-content. Object keys are sorted by encoding/json.
func DigestSource(source *Source) (string, error) {
	encoded, err := CanonicalJSON(source)
	if err != nil {
		return "", err
	}
	return digest(encoded), nil
}

// DigestUpdate binds +diff to +update and includes overwrite intent in addition
// to the source object.
func DigestUpdate(overwrite bool, nodes []map[string]any) (string, error) {
	canonical := struct {
		Overwrite bool   `json:"overwrite"`
		Source    Source `json:"source"`
	}{Overwrite: overwrite}
	canonical.Source = Source{
		SchemaVersion:  SchemaVersion,
		CatalogVersion: CatalogVersion,
		Nodes:          nodes,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	return digest(encoded), nil
}

func ValidDigest(value string) bool {
	return digestPattern.MatchString(strings.TrimSpace(value))
}

func EqualDigest(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func decodeOne(data []byte, target any, disallowUnknown bool) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if disallowUnknown {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return requireEOF(decoder)
}

func requireEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return errors.New("multiple JSON values are not allowed")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}
