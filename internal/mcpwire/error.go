// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package mcpwire parses the reviewed inner MCP error envelope embedded in
// legacy CLI errors. It intentionally exposes only fields needed for safe
// branching; callers must not classify arbitrary message text as success.
package mcpwire

import (
	"encoding/json"
	"strings"
)

type ErrorDetails struct {
	Code      string
	Message   string
	Type      string
	Retryable *bool
}

func ParseError(err error) (ErrorDetails, bool) {
	if err == nil {
		return ErrorDetails{}, false
	}
	raw := err.Error()
	start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return ErrorDetails{}, false
	}
	var envelope struct {
		Error struct {
			Code      any    `json:"code"`
			Message   string `json:"message"`
			Type      string `json:"type"`
			Retryable *bool  `json:"retryable"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(raw[start:end+1]), &envelope) != nil {
		return ErrorDetails{}, false
	}
	details := ErrorDetails{
		Message:   strings.TrimSpace(envelope.Error.Message),
		Type:      strings.TrimSpace(envelope.Error.Type),
		Retryable: envelope.Error.Retryable,
	}
	if code, ok := envelope.Error.Code.(string); ok {
		details.Code = strings.TrimSpace(code)
	}
	if details.Code == "" && details.Message == "" && details.Type == "" && details.Retryable == nil {
		return ErrorDetails{}, false
	}
	return details, true
}

// IsPrimaryDocAbsent matches the one reviewed legacy wire shape currently
// used by get_primary_doc for a valid record without a primary document. A
// generic SYSTEM_ERROR or arbitrary "no record" text is deliberately not
// enough on its own.
func IsPrimaryDocAbsent(err error) bool {
	details, ok := ParseError(err)
	return ok && details.Code == "-1" &&
		strings.EqualFold(details.Type, "SYSTEM_ERROR") &&
		strings.EqualFold(details.Message, "no record")
}
