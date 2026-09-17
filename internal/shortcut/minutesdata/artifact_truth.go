// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package minutesdata

import (
	"fmt"
	"sort"
)

// ArtifactState is the machine-readable truth state for one requested Minutes
// artifact. The first rollout is intentionally limited to todos because that
// is the artifact for which raw Case evidence proves both non-empty and
// explicit-empty collections. Unknown response shapes remain unsupported.
type ArtifactState string

const (
	ArtifactReady            ArtifactState = "ready"
	ArtifactKnownEmpty       ArtifactState = "known_empty"
	ArtifactUnsupportedShape ArtifactState = "unsupported_shape"
	ArtifactFailed           ArtifactState = "failed"
)

// TodosFact is the normalized, compatibility-preserving interpretation of one
// list_minutes_todos response. Raw result fields are retained only for
// supported shapes; unsupported payload values are never copied into output.
type TodosFact struct {
	TaskUUID       string
	State          ArtifactState
	SourceField    string
	Items          []any
	RawResult      map[string]any
	ObservedFields []string
	ObservedTypes  map[string]string
	Message        string
}

// InspectTodos distinguishes an explicit empty collection from a ready result,
// a backend failure, and an unknown shape. It accepts only the two response
// fields already evidenced by Runtime and raw Case trajectories.
func InspectTodos(taskUUID string, data map[string]any) TodosFact {
	fact := TodosFact{TaskUUID: taskUUID, Items: []any{}, ObservedTypes: map[string]string{}}
	if err := validateEnvelope(data); err != nil {
		fact.State = ArtifactFailed
		fact.Message = err.Error()
		return fact
	}
	result, ok := data["result"].(map[string]any)
	if !ok {
		fact.State = ArtifactUnsupportedShape
		fact.Message = "minutes todos response has no result object"
		return fact
	}
	fact.ObservedFields, fact.ObservedTypes = observedShape(result)

	actions, actionsErr := mapSliceField(result, "actions")
	todos, todosErr := mapSliceField(result, "dingtalkTodoList")
	if actionsErr != nil && todosErr != nil {
		fact.State = ArtifactUnsupportedShape
		fact.Message = "minutes todos response has neither actions nor dingtalkTodoList array"
		return fact
	}

	switch {
	case todosErr == nil && len(todos) > 0:
		fact.SourceField, fact.Items = "dingtalkTodoList", todos
	case actionsErr == nil && len(actions) > 0:
		fact.SourceField, fact.Items = "actions", actions
	case todosErr == nil:
		fact.SourceField, fact.Items = "dingtalkTodoList", todos
	default:
		fact.SourceField, fact.Items = "actions", actions
	}
	fact.RawResult = result
	if len(fact.Items) == 0 {
		fact.State = ArtifactKnownEmpty
	} else {
		fact.State = ArtifactReady
	}
	return fact
}

// FailedTodos records a transport/RPC failure without pretending that a
// business collection was observed.
func FailedTodos(taskUUID string, err error) TodosFact {
	message := "minutes todos request failed"
	if err != nil {
		message = err.Error()
	}
	return TodosFact{
		TaskUUID:       taskUUID,
		State:          ArtifactFailed,
		Items:          []any{},
		ObservedFields: []string{},
		ObservedTypes:  map[string]string{},
		Message:        message,
	}
}

// Successful reports whether the response proves either a real collection or
// an explicit empty collection.
func (fact TodosFact) Successful() bool {
	return fact.State == ArtifactReady || fact.State == ArtifactKnownEmpty
}

// Err preserves the existing failure wording for compatibility while the
// structured payload carries the new state and shape diagnostics.
func (fact TodosFact) Err() error {
	if fact.Successful() {
		return nil
	}
	return fmt.Errorf("%s", fact.Message)
}

// Ledger returns non-sensitive machine evidence for one requested todos
// artifact. itemCount and complete are always present, including known empty.
func (fact TodosFact) Ledger() map[string]any {
	observedFields := make([]string, len(fact.ObservedFields))
	copy(observedFields, fact.ObservedFields)
	ledger := map[string]any{
		"artifact":       "todos",
		"taskUuid":       fact.TaskUUID,
		"state":          string(fact.State),
		"complete":       fact.Successful(),
		"itemCount":      len(fact.Items),
		"observedFields": observedFields,
		"observedTypes":  copyStringMap(fact.ObservedTypes),
		"retryable":      false,
	}
	if fact.SourceField != "" {
		ledger["sourceField"] = fact.SourceField
	}
	if fact.Message != "" {
		ledger["error"] = fact.Message
	}
	return ledger
}

// Payload preserves legacy successful result fields and adds normalized truth
// fields. Unsupported/failed payloads expose only non-sensitive shape facts.
func (fact TodosFact) Payload() map[string]any {
	payload := map[string]any{}
	if fact.Successful() {
		for key, value := range fact.RawResult {
			payload[key] = value
		}
		items := make([]any, len(fact.Items))
		copy(items, fact.Items)
		payload["items"] = items
	}
	for key, value := range fact.Ledger() {
		payload[key] = value
	}
	return payload
}

func observedShape(data map[string]any) ([]string, map[string]string) {
	fields := make([]string, 0, len(data))
	types := make(map[string]string, len(data))
	for key, value := range data {
		fields = append(fields, key)
		types[key] = jsonType(value)
	}
	sort.Strings(fields)
	return fields, types
}

func jsonType(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "number"
	default:
		return fmt.Sprintf("%T", value)
	}
}

func copyStringMap(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
