// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"
)

const employeeCardPayloadLimit = 64 * 1024

func encodeEmployeeCardMessages(messages []map[string]any) ([]byte, error) {
	encoded := make([]string, 0, len(messages))
	for _, message := range messages {
		data, err := json.Marshal(message)
		if err != nil {
			return nil, employeeTerminal("invalid_card_payload")
		}
		encoded = append(encoded, string(data))
	}
	content, err := json.Marshal(encoded)
	if err != nil || len(content) > employeeCardPayloadLimit {
		return nil, employeeTerminal("card_payload_too_large")
	}
	return content, nil
}

// Limit the twice-encoded value, reserving room for component definitions and
// parent references. Prefer paragraph boundaries; never split a UTF-8 code point.
// TODO(A2UI-MARKDOWN-CHUNKING): use Markdown block-aware pagination. A byte-safe
// split can still break fences, lists, tables or reference links. Acceptance must
// cover blocks larger than a batch and preserve their rendered structure; raw
// text reconstruction and protocol lint alone do not establish that guarantee.
func employeeCardAnswerParts(answer string) []string {
	const budget = 24 * 1024
	var parts []string
	for len(answer) > 0 {
		hi := min(len(answer), budget)
		lo := 0
		for lo < hi {
			mid := (lo + hi + 1) / 2
			first, _ := json.Marshal(answer[:mid])
			second, _ := json.Marshal(string(first))
			if len(second) <= budget {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		end := lo
		for end > 0 && end < len(answer) && !utf8.RuneStart(answer[end]) {
			end--
		}
		if end < len(answer) {
			if paragraph := strings.LastIndex(answer[:end], "\n\n"); paragraph >= end/2 {
				end = paragraph + 2
			}
		}
		parts = append(parts, answer[:end])
		answer = answer[end:]
	}
	return parts
}

// Small replies keep the original single-frame shape. Large replies publish
// separate data paths on the same surface. Stage a new revision off the visible
// tree, then switch the stable answer container only after every part succeeds.
// Failed/unknown writes must not shorten or mix the last visible answer. All
// batches are preflighted before any write; only the final batch publishes them.
func (v *employeeCardView) batches(surface, title string, running bool) ([][]map[string]any, error) {
	messages := v.messages(surface, title, running)
	if _, err := encodeEmployeeCardMessages(messages); err == nil {
		return [][]map[string]any{messages}, nil
	}
	parts := employeeCardAnswerParts(v.answer)
	v.revision++
	children := make([]string, 0, len(parts))
	var batches [][]map[string]any
	for i, text := range parts {
		id := "answer-" + strconv.Itoa(v.revision) + "-" + strconv.Itoa(i)
		children = append(children, id)
		batch := []map[string]any{
			employeeCardMessage("updateDataModel", surface, map[string]any{"path": "/" + id, "value": text}),
			employeeCardMessage("updateComponents", surface, map[string]any{"components": []map[string]any{
				{"id": id, "component": "Markdown", "content": map[string]any{"path": "/" + id}},
			}}),
		}
		batches = append(batches, batch)
	}
	// The last frame publishes the complete revision through the stable artifact.
	panelView := *v
	panelView.answer = ""
	panel := panelView.messages(surface, title, running)[1]
	components := panel["updateComponents"].(map[string]any)["components"].([]map[string]any)
	components[len(components)-1]["children"] = []string{"execution", "answer"}
	panel["updateComponents"].(map[string]any)["components"] = append(components,
		map[string]any{"id": "answer", "component": "Column", "children": children})
	batches = append(batches, []map[string]any{panel})
	for _, batch := range batches {
		if _, err := encodeEmployeeCardMessages(batch); err != nil {
			return nil, err
		}
	}
	return batches, nil
}
