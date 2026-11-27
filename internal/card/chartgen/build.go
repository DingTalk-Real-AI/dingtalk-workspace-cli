// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package chartgen

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const catalogID = "https://dingtalk.com/card/a2ui/catalogs/public/catalog.json"

var (
	chartRandomRead = rand.Read
	chartWireEncode = wireContent
)

type Input struct {
	SpecVersion string          `json:"specVersion"`
	PresetID    string          `json:"presetId"`
	Title       string          `json:"title"`
	Data        json.RawMessage `json:"data"`
}

type Annotation struct {
	SurfaceID   string `json:"surfaceId"`
	ComponentID string `json:"componentId"`
	Type        string `json:"type"`
}

type Artifact struct {
	SpecVersion       string           `json:"specVersion"`
	PresetID          string           `json:"presetId"`
	PresetAlias       string           `json:"presetAlias"`
	SurfaceID         string           `json:"surfaceId"`
	Summary           string           `json:"summary"`
	CreateMessages    []map[string]any `json:"createMessages"`
	FinishMessages    []map[string]any `json:"finishMessages"`
	FinishAnnotations []Annotation     `json:"finishAnnotations"`
	CreateContent     string           `json:"createContent"`
	FinishContent     string           `json:"finishContent"`
}

func validateLabel(value, field string, limit int) error {
	if value == "" || strings.TrimSpace(value) != value || utf8.RuneCountInString(value) > limit {
		return fmt.Errorf("%s must be nonempty, trimmed, and at most %d characters", field, limit)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("%s contains a control character", field)
		}
		if chartTransportUnsafeUnicode(r) {
			return fmt.Errorf("%s contains a Unicode character unsupported by delivery", field)
		}
	}
	return nil
}

// Keep labels compatible with the public DWS transport guard without depending
// on a host package. Joined emoji containing U+200D share this delivery limit.
func chartTransportUnsafeUnicode(r rune) bool {
	return r >= 0x200b && r <= 0x200d || r == 0xfeff ||
		r >= 0x202a && r <= 0x202e || r >= 0x2028 && r <= 0x2029 ||
		r >= 0x2066 && r <= 0x2069
}

func newSurfaceID() (string, error) {
	var value [16]byte
	if _, err := chartRandomRead(value[:]); err != nil {
		return "", err
	}
	return "chart-" + hex.EncodeToString(value[:]), nil
}

func validateSurfaceID(value string) error {
	if value == "" || len(value) > 80 {
		return fmt.Errorf("surface ID must contain 1–80 URL-safe characters")
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return fmt.Errorf("surface ID must contain only ASCII letters, digits, hyphen, or underscore")
		}
	}
	return nil
}

func escapeMarkdown(value string) string {
	var out strings.Builder
	for _, r := range value {
		if strings.ContainsRune("\\`*_{}[]()#+-.!<>|~", r) {
			out.WriteByte('\\')
		}
		out.WriteRune(r)
	}
	return out.String()
}

func wireContent(messages []map[string]any) (string, error) {
	items := make([]string, 0, len(messages))
	for _, message := range messages {
		data, err := json.Marshal(message)
		if err != nil {
			return "", err
		}
		items = append(items, string(data))
	}
	data, err := json.Marshal(items)
	return string(data), err
}

// Build validates an internal ChartSpec and returns complete create and finish
// artifacts. It does not resolve recipients or send a card. surfaceID is an
// internal deterministic test/retry override; pass an empty string for a new ID.
func Build(raw []byte, surfaceID string) (Artifact, error) {
	if len(raw) == 0 || len(raw) > 256*1024 {
		return Artifact{}, fmt.Errorf("ChartSpec must contain 1–262144 bytes")
	}
	raw = bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})
	var input Input
	if err := decodeStrict(raw, &input); err != nil {
		return Artifact{}, fmt.Errorf("invalid ChartSpec: %w", err)
	}
	if input.SpecVersion != "1.0" {
		return Artifact{}, fmt.Errorf("unsupported ChartSpec version %q", input.SpecVersion)
	}
	if err := validateLabel(input.Title, "title", 120); err != nil {
		return Artifact{}, err
	}
	preset, err := Lookup(input.PresetID)
	if err != nil {
		return Artifact{}, err
	}
	if len(input.Data) == 0 {
		return Artifact{}, fmt.Errorf("data is required")
	}
	option, summary, err := buildOption(preset, input.Title, input.Data)
	if err != nil {
		return Artifact{}, err
	}
	if surfaceID == "" {
		surfaceID, err = newSurfaceID()
		if err != nil {
			return Artifact{}, err
		}
	}
	if err := validateSurfaceID(surfaceID); err != nil {
		return Artifact{}, err
	}
	components := []map[string]any{
		{"id": "root", "component": "Column", "children": []string{"chart", "summary"}},
		{"id": "chart", "component": "Chart", "height": 480, "data": map[string]any{"path": "/chart"}, "fallbackMarkdown": escapeMarkdown(summary)},
		{"id": "summary", "component": "Text", "text": map[string]any{"path": "/summary"}, "maxLine": 8},
	}
	create := []map[string]any{{"version": "v1.0", "createSurface": map[string]any{
		"surfaceId": surfaceID, "catalogId": catalogID, "components": components,
		"dataModel": map[string]any{"chart": option, "summary": summary},
	}}}
	finish := []map[string]any{{"version": "v1.0", "updateDataModel": map[string]any{
		"surfaceId": surfaceID, "path": "/summary", "value": summary,
	}}}
	createContent, err := chartWireEncode(create)
	if err != nil {
		return Artifact{}, err
	}
	finishContent, err := chartWireEncode(finish)
	if err != nil {
		return Artifact{}, err
	}
	return Artifact{
		SpecVersion: input.SpecVersion, PresetID: preset.ID, PresetAlias: preset.Alias,
		SurfaceID: surfaceID, Summary: summary, CreateMessages: create,
		FinishMessages: finish, FinishAnnotations: []Annotation{{surfaceID, "summary", "artifact"}},
		CreateContent: createContent, FinishContent: finishContent,
	}, nil
}
