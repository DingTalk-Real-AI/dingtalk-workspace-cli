// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package opennodes

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf16"
)

// Writable V1 fields follow OpenNodesUpdate.validateText/validateTextRun.
// Editor/checkpoint run fields have a different layout; do not infer from them.
// Full CSS color syntax and model compilation remain authoritative remotely.
var linkProtocol = regexp.MustCompile(`(?i)^([a-z][a-z0-9+.-]*):`)

func textIssue(path, code string) error {
	return &NodeValidationError{Code: code, Path: path}
}

func textFields(value map[string]any, path, allowed, readOnly string) error {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys) // Deterministic first issue, independent of map iteration.
	for _, key := range keys {
		if slices.Contains(strings.Fields(allowed), key) {
			continue
		}
		code := "unknownField"
		if slices.Contains(strings.Fields(readOnly), key) {
			code = "readOnlyField"
		}
		pointer := strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
		issue := &NodeValidationError{Code: code, Path: path + "/" + pointer}
		if key == "fontColor" && strings.HasSuffix(path, "/marks") {
			issue.Hint = "；文字颜色请使用 marks.color"
		} else if key == "fontSize" && strings.Contains(path, "/runs/") && !strings.HasSuffix(path, "/marks") {
			issue.Hint = "；字号请放在 run.marks.fontSize"
		}
		return issue
	}
	return nil
}

func textEnum(value map[string]any, key, path, choices string) error {
	if v, present := value[key]; present {
		s, ok := v.(string)
		if !ok || !slices.Contains(strings.Fields(choices), s) {
			return textIssue(path+"/"+key, "invalidValue")
		}
	}
	return nil
}

func validateTextObject(value any, path string, nodeID any) error {
	text, ok := value.(map[string]any)
	if !ok || text == nil {
		return textIssue(path, "invalidType")
	}
	if err := textFields(text, path, "blocks verticalAlign padding", "plainText writeSupport unsupportedFeatures"); err != nil {
		return err
	}
	blocks, ok := text["blocks"].([]any)
	if !ok || len(blocks) == 0 {
		return textIssue(path+"/blocks", "invalidValue")
	}
	for index, item := range blocks {
		blockPath := fmt.Sprintf("%s/blocks/%d", path, index)
		block, ok := item.(map[string]any)
		if !ok || block == nil {
			return textIssue(blockPath, "invalidType")
		}
		if _, present := block["verticalAlign"]; present {
			return &VerticalAlignPlacementError{NodeID: nodeID, Path: blockPath + "/verticalAlign", ExpectedPath: path + "/verticalAlign"}
		}
		if err := textFields(block, blockPath, "type horizontalAlign runs", ""); err != nil {
			return err
		}
		if _, present := block["type"]; !present {
			return textIssue(blockPath+"/type", "invalidValue")
		}
		if err := textEnum(block, "type", blockPath, "paragraph bulletList orderedList"); err != nil {
			return err
		}
		if err := textEnum(block, "horizontalAlign", blockPath, "left center right"); err != nil {
			return err
		}
		runs, ok := block["runs"].([]any)
		if !ok || len(runs) == 0 {
			return textIssue(blockPath+"/runs", "invalidValue")
		}
		for index, run := range runs {
			if err := validateTextRun(run, fmt.Sprintf("%s/runs/%d", blockPath, index), nodeID); err != nil {
				return err
			}
		}
	}
	if err := textEnum(text, "verticalAlign", path, "top center bottom"); err != nil {
		return err
	}
	if padding, present := text["padding"]; present {
		values := []any{padding}
		if pair, ok := padding.([]any); ok {
			if len(pair) != 2 {
				return textIssue(path+"/padding", "invalidValue")
			}
			values = pair
		}
		for _, value := range values {
			number, ok := finiteNumber(value)
			if !ok || number < 0 {
				return textIssue(path+"/padding", "invalidValue")
			}
		}
	}
	return nil
}

func validateTextRun(value any, path string, nodeID any) error {
	run, ok := value.(map[string]any)
	if !ok || run == nil {
		return textIssue(path, "invalidType")
	}
	if err := textFields(run, path, "text marks link", ""); err != nil {
		return err
	}
	content, ok := run["text"].(string)
	if !ok {
		return textIssue(path+"/text", "invalidType")
	}
	if strings.ContainsAny(content, "\r\n\u2028\u2029") {
		return &TextRunValidationError{NodeID: nodeID, Path: path + "/text"}
	}
	if value, present := run["link"]; present {
		link, ok := value.(map[string]any)
		if !ok || link == nil {
			return textIssue(path+"/link", "invalidType")
		}
		if err := textFields(link, path+"/link", "url", ""); err != nil {
			return err
		}
		if _, present := link["url"]; !present {
			return textIssue(path+"/link/url", "required")
		}
		if !writableTextLink(link["url"]) {
			return textIssue(path+"/link/url", "invalidValue")
		}
	}
	value, present := run["marks"]
	if !present {
		return nil
	}
	marks, ok := value.(map[string]any)
	if !ok || marks == nil {
		return textIssue(path+"/marks", "invalidType")
	}
	if err := textFields(marks, path+"/marks", "fontFamily fontSize bold italic underline strike color highlight", ""); err != nil {
		return err
	}
	if value, present := marks["fontFamily"]; present {
		if family, ok := value.(string); !ok || strings.TrimSpace(family) == "" {
			return textIssue(path+"/marks/fontFamily", "invalidValue")
		}
	}
	if value, present := marks["fontSize"]; present {
		if size, ok := finiteNumber(value); !ok || size <= 0 {
			return textIssue(path+"/marks/fontSize", "invalidValue")
		}
	}
	for _, key := range []string{"bold", "italic", "underline", "strike"} {
		if value, present := marks[key]; present {
			if _, ok := value.(bool); !ok {
				return textIssue(path+"/marks/"+key, "invalidType")
			}
		}
	}
	// Do not introduce an incomplete second CSS parser.
	for _, key := range []string{"color", "highlight"} {
		if value, present := marks[key]; present {
			if color, ok := value.(string); !ok || strings.TrimSpace(color) == "" {
				return textIssue(path+"/marks/"+key, "invalidValue")
			}
		}
	}
	return nil
}

// Mirrors OpenNodesText.isWritableOpenTextLinkUrl, including relative links
// and JS's UTF-16 length limit. Validation never fetches URLs.
func writableTextLink(value any) bool {
	url, ok := value.(string)
	if !ok || url == "" || len(utf16.Encode([]rune(url))) > 2048 {
		return false
	}
	for _, r := range url {
		if r <= 0x1f || r == 0x7f || r == '<' || r == '>' {
			return false
		}
	}
	protocol := linkProtocol.FindStringSubmatch(strings.TrimSpace(url))
	if protocol == nil {
		return true
	}
	switch strings.ToLower(protocol[1]) {
	case "http", "https", "mailto", "tel", "dingtalk":
		return true
	default:
		return false
	}
}
