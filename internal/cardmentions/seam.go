// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package cardmentions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui"
	"io"
	"strings"
	"unicode/utf8"
)

type Diagnostic = a2ui.Diagnostic

func decode(raw []byte) (any, error) {
	raw = bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})
	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("JSON file must use valid UTF-8 encoding")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("file must contain exactly one JSON value")
	}
	return value, nil
}
func object(value any) map[string]any { result, _ := value.(map[string]any); return result }
func str(value any) string            { result, _ := value.(string); return result }
func pointer(parts []string) string {
	var result strings.Builder
	for _, part := range parts {
		result.WriteByte('/')
		result.WriteString(strings.ReplaceAll(strings.ReplaceAll(part, "~", "~0"), "/", "~1"))
	}
	return result.String()
}

// PreflightDiagnostics adds DWS notification diagnostics above the protocol.
func PreflightDiagnostics(messages []string, declared []string) []Diagnostic {
	diagnostics := []Diagnostic{}
	for _, token := range ExtractMentions(messages).IMPlaceholders {
		diagnostics = append(diagnostics, MentionIMPlaceholderDiagnostics(token.Token, token.Pointer)...)
	}
	if len(declared) > 0 {
		diagnostics = append(diagnostics, MentionDeclarationDiagnostics(messages, declared)...)
	}
	return diagnostics
}
