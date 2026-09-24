// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package authoring

import (
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui/protocol"
)

// VisualContext supplies authoring intent that cannot be inferred from A2UI wire.
// These checks complement the public protocol validator; they do not define wire fields.
type VisualContext struct {
	Archetype              string
	MinimumValidationWidth int
	DebugHostBadge         bool
}

func VisualDiagnostics(messages []map[string]any, context VisualContext) []protocol.Diagnostic {
	components := map[string]map[string]any{}
	paths := map[string]string{}
	for i, message := range messages {
		for _, operation := range []string{"createSurface", "updateComponents"} {
			body, ok := message[operation].(map[string]any)
			if !ok {
				continue
			}
			items, _ := body["components"].([]any)
			for j, item := range items {
				component, ok := item.(map[string]any)
				if !ok {
					continue
				}
				id, _ := component["id"].(string)
				if id == "" {
					continue
				}
				components[id] = component
				paths[id] = fmt.Sprintf("/%d/%s/components/%d", i, operation, j)
			}
		}
	}
	diagnostics := []protocol.Diagnostic{}
	add := func(severity, code, id, field, message, suggestion string) {
		path := paths[id]
		if field != "" {
			path += "/" + field
		}
		diagnostics = append(diagnostics, protocol.Diagnostic{Severity: severity, Code: code, Phase: "visual", InstancePath: path, ComponentID: id, Message: message, Suggestion: suggestion})
	}
	root := components["root"]
	if root != nil && root["component"] == "Card" {
		if root["backgroundColor"] != "#00FFFFFF" || root["backgroundColorToken"] != nil {
			add("warning", "ROOT_CARD_NOT_TRANSPARENT", "root", "backgroundColor", "the message root Card may paint over the host surface", "use backgroundColor=#00FFFFFF and omit backgroundColorToken unless a full-card fill is intentional")
		}
	} else if root != nil && hasVisualFill(root) {
		add("warning", "ROOT_BACKGROUND_UNJUSTIFIED", "root", "backgroundColorToken", "root background overrides the host surface", "omit the root Column fill and put visual fills on child components")
	}
	for id, component := range components {
		if id == "root" || !hasVisualFill(component) {
			continue
		}
		if component["component"] == "Card" || component["component"] == "Column" {
			padding, ok := component["padding"].(float64)
			if !ok || padding <= 0 {
				add("warning", "LOCAL_PANEL_MISSING_PADDING", id, "padding", "filled content section has no explicit inset", "set positive padding on the container when its content needs an inset")
			}
		}
		if component["backgroundColorToken"] == "common_bg_color" {
			add("warning", "LOCAL_PANEL_LOW_CONTRAST", id, "backgroundColorToken", "local panel may blend into the host bubble", "use the ordinary content-panel surface and border tokens, then inspect the client")
		}
	}
	for id, component := range components {
		if component["component"] != "Row" {
			continue
		}
		children := componentIDs(component["children"])
		if len(children) != 2 || !strings.HasSuffix(children[0], "_label") {
			continue
		}
		label, value := components[children[0]], components[children[1]]
		if label["component"] != "Text" || value["component"] != "Text" {
			continue
		}
		if literal, ok := label["text"].(string); ok && literal != "" && !strings.HasSuffix(literal, ":") && !strings.HasSuffix(literal, "：") {
			add("warning", "KEY_VALUE_LABEL_MISSING_COLON", children[0], "text", "key-value label has no separator", "use a colon when the label and value need an explicit textual separator")
		}
		if lines, _ := value["maxLine"].(float64); lines > 1 {
			if component["align"] != "start" || label["variant"] != value["variant"] || label["sizeToken"] != value["sizeToken"] {
				add("warning", "KEY_VALUE_FIRST_LINE_MISALIGNED", id, "align", "a wrapping value can center the label against the whole paragraph instead of its first line", "use align=start and the same variant/sizeToken for label and value; mute the label with colorToken")
			}
		} else if component["align"] != "center" {
			add("warning", "KEY_VALUE_ROW_NOT_CENTERED", id, "align", "single-line key-value row is not vertically centered", "use align=center for compact single-line values")
		}
	}
	if (context.Archetype == "notification" || context.Archetype == "approval") && context.MinimumValidationWidth > 0 && context.MinimumValidationWidth < 360 {
		add("error", "NOTIFICATION_WIDTH_TOO_NARROW", "root", "", "notification and approval acceptance width must start at 360px", "set minimum validation width to at least 360")
	}
	if context.Archetype != "notification" && context.Archetype != "approval" {
		return diagnostics
	}
	// A colored message header is a top-level surface, not a local content
	// panel. The latter's border contract must not leak into the header.
	if root != nil && root["component"] == "Card" {
		layout := components[stringValue(root["child"])]
		children := componentIDs(layout["children"])
		if len(children) > 0 {
			header := components[children[0]]
			if header["component"] == "Card" && hasVisualFill(header) {
				if width, _ := header["borderWidth"].(float64); width > 0 {
					add("warning", "HEADER_UNREQUESTED_BORDER", children[0], "borderWidth", "colored header inherits a content-panel border", "omit header borderWidth and borderColorToken unless the header design explicitly needs a border")
				}
				if radius, ok := header["cornerRadius"].(float64); !ok || radius > 0 {
					add("warning", "HEADER_BOTTOM_RADIUS_UNWANTED", children[0], "cornerRadius", "a colored Card header uses rounded bottom corners (8np by default)", "prefer an unpainted title row; if a colored header is explicitly required, verify Mac, mobile, and Web and set its radius deliberately")
				}
				if padding, _ := root["padding"].(float64); padding > 0 {
					add("warning", "HEADER_NOT_FULL_WIDTH", "root", "padding", "root padding insets the colored header from the message edges", "set root Card padding=0 when the header should span the full message width")
				}
			}
		}
	}
	for id, component := range components {
		if component["component"] != "Row" {
			continue
		}
		children := componentIDs(component["children"])
		if len(children) != 2 || components[children[1]]["component"] != "Tag" {
			continue
		}
		left := components[children[0]]
		if left["component"] != "Text" {
			continue
		}
		if left["colorToken"] == "common_level3_base_color" || left["variant"] == "caption" {
			add("warning", "HEADER_TITLE_WEAK_COLOR", children[0], "colorToken", "notification header identity is styled as supporting description", "use common_level1_base_color and a bold body-size title when the header is the primary identity")
		}
		weight, ok := left["weight"].(float64)
		if !ok || weight <= 0 || left["maxLine"] != float64(1) {
			add("warning", "HEADER_TRAILING_MAY_CLIP", id, "children", "header title does not yield remaining width to the trailing Tag", "give the title weight=1 and maxLine=1; let the Tag keep its natural width")
		}
		if context.DebugHostBadge {
			add("warning", "DEBUG_BADGE_OVERLAP_RISK", id, "children", "the host Debug badge may overlap the authored Tag", "verify the Tag in a real debug client screenshot and use a separate status row if obscured")
		}
	}
	return diagnostics
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}

func hasVisualFill(component map[string]any) bool {
	for _, key := range []string{"backgroundColor", "backgroundColorToken"} {
		if value, ok := component[key]; ok && value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" {
			return true
		}
	}
	return false
}

func componentIDs(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		id, _ := item.(string)
		ids = append(ids, id)
	}
	return ids
}
