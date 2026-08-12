// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package minutesdata

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// RequireWriteAcknowledgement accepts only an explicit successful envelope or
// a non-empty result object. A bare {} cannot prove a remote write succeeded.
func RequireWriteAcknowledgement(operation string, data map[string]any) error {
	if err := validateEnvelope(data); err != nil {
		return err
	}
	if success, exists := data["success"]; exists {
		value, ok := success.(bool)
		if !ok || !value {
			return fmt.Errorf("minutes %s response has invalid success acknowledgement", operation)
		}
		return nil
	}
	if result, ok := data["result"].(map[string]any); ok && len(result) > 0 {
		return nil
	}
	return fmt.Errorf("minutes %s response has no explicit successful acknowledgement", operation)
}

// RecordResult validates the gateway's observed listening-note command result.
func RecordResult(expectedCmd, taskUUID string, data map[string]any) (map[string]any, error) {
	if err := RequireWriteAcknowledgement("record "+expectedCmd, data); err != nil {
		return nil, err
	}
	result, ok := data["result"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("minutes record %s response has no result object", expectedCmd)
	}
	cmd := stringField(result, "cmd")
	if cmd != expectedCmd {
		return nil, fmt.Errorf("minutes record response cmd %q does not match %q", cmd, expectedCmd)
	}
	if expectedCmd != "create" {
		actual := TaskUUID(result)
		if actual == "" || actual != taskUUID {
			return nil, fmt.Errorf("minutes record %s response task UUID mismatch", expectedCmd)
		}
	}
	return result, nil
}

// MindGraphStatus extracts the documented 0=processing, 1=success, 2=failed
// state and rejects empty status responses.
func MindGraphStatus(data map[string]any) (int, map[string]any, error) {
	if err := validateEnvelope(data); err != nil {
		return 0, nil, err
	}
	result, ok := data["result"].(map[string]any)
	if !ok {
		return 0, nil, fmt.Errorf("minutes mind graph response has no result object")
	}
	value, exists := result["taskStatus"]
	if !exists {
		return 0, nil, fmt.Errorf("minutes mind graph response has no taskStatus")
	}
	status, err := intValue(value)
	if err != nil || status < 0 || status > 2 {
		return 0, nil, fmt.Errorf("minutes mind graph taskStatus is invalid: %v", value)
	}
	return status, result, nil
}

// SpeakerSummaryTask extracts the recovery handle returned by task creation.
func SpeakerSummaryTask(data map[string]any) (taskID, status string, err error) {
	if err := RequireWriteAcknowledgement("speaker summary create", data); err != nil {
		return "", "", err
	}
	result, ok := data["result"].(map[string]any)
	if !ok {
		return "", "", fmt.Errorf("minutes speaker summary create response has no result object")
	}
	taskID = stringField(result, "taskId", "taskID")
	status = strings.ToLower(stringField(result, "status"))
	if taskID == "" {
		return "", "", fmt.Errorf("minutes speaker summary create response has no taskId")
	}
	return taskID, status, nil
}

// SpeakerSummaryResult requires a concrete result payload. The API returns a
// business error while processing, so an empty object is never ready.
func SpeakerSummaryResult(data map[string]any) (any, error) {
	if err := validateEnvelope(data); err != nil {
		return nil, err
	}
	result, exists := data["result"]
	if !exists || result == nil {
		return nil, fmt.Errorf("minutes speaker summary response has no result")
	}
	switch value := result.(type) {
	case map[string]any:
		if len(value) == 0 {
			return nil, fmt.Errorf("minutes speaker summary result is empty")
		}
	case []any:
		if value == nil {
			return nil, fmt.Errorf("minutes speaker summary result is null")
		}
	default:
		return nil, fmt.Errorf("minutes speaker summary result has type %T", result)
	}
	return result, nil
}

// HotWords extracts the explicit current personal hot-word set. [] is valid;
// a missing/null/wrongly typed list is not.
func HotWords(data map[string]any) ([]string, error) {
	if err := validateEnvelope(data); err != nil {
		return nil, err
	}
	result, ok := data["result"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("minutes hot-word response has no result object")
	}
	raw, exists := result["hotWordList"]
	if !exists || raw == nil {
		return nil, fmt.Errorf("minutes hot-word response has no hotWordList array")
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("minutes hotWordList has type %T, want array", raw)
	}
	out := make([]string, 0, len(items))
	seen := map[string]bool{}
	for index, item := range items {
		word := ""
		switch value := item.(type) {
		case string:
			word = value
		case map[string]any:
			word = stringField(value, "hotWord", "word", "name", "content")
		}
		word = strings.TrimSpace(word)
		if word == "" {
			return nil, fmt.Errorf("minutes hotWordList item %d has no word", index)
		}
		if !seen[word] {
			seen[word] = true
			out = append(out, word)
		}
	}
	return out, nil
}

func intValue(value any) (int, error) {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || math.Trunc(typed) != typed || typed < 0 || typed > 2 {
			return 0, fmt.Errorf("invalid status number %v", typed)
		}
		return int(typed), nil
	case int:
		return typed, nil
	case json.Number:
		parsed, err := strconv.Atoi(string(typed))
		return parsed, err
	case string:
		return strconv.Atoi(strings.TrimSpace(typed))
	default:
		return 0, fmt.Errorf("unsupported number type %T", value)
	}
}
