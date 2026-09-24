// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package authoring

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui/protocol"
)

func TestNotificationVisualLintDistinguishesBadAndRepairedLayout(t *testing.T) {
	components := []any{
		map[string]any{"id": "root", "component": "Column", "children": []any{"header", "evidence"}},
		map[string]any{"id": "header", "component": "Row", "children": []any{"title", "status"}},
		map[string]any{"id": "title", "component": "Text", "text": "GitHub Issue #1438", "variant": "caption", "colorToken": "common_level3_base_color"},
		map[string]any{"id": "status", "component": "Tag", "text": "OPEN"},
		map[string]any{"id": "evidence", "component": "Card", "child": "detail", "backgroundColorToken": "common_bg_color", "padding": float64(0)},
		map[string]any{"id": "detail", "component": "Text", "text": "参数非法"},
	}
	messages := []map[string]any{{"version": "v1.0", "createSurface": map[string]any{"surfaceId": "visual-lint", "components": components}}}
	context := VisualContext{Archetype: "notification", MinimumValidationWidth: 240, DebugHostBadge: true}
	bad := VisualDiagnostics(messages, context)
	for _, code := range []string{"HEADER_TITLE_WEAK_COLOR", "HEADER_TRAILING_MAY_CLIP", "DEBUG_BADGE_OVERLAP_RISK", "LOCAL_PANEL_MISSING_PADDING", "LOCAL_PANEL_LOW_CONTRAST", "NOTIFICATION_WIDTH_TOO_NARROW"} {
		if !hasVisualDiagnostic(bad, code) {
			t.Errorf("bad notification is missing %s: %+v", code, bad)
		}
	}
	components[2] = map[string]any{"id": "title", "component": "Text", "text": "GitHub Issue #1438", "variant": "body", "colorToken": "common_level1_base_color", "bold": true, "weight": float64(1), "maxLine": float64(1)}
	components[4] = map[string]any{"id": "evidence", "component": "Card", "child": "detail", "backgroundColorToken": "common_fg_z1_color", "borderWidth": float64(0.5), "borderColorToken": "common_line_hard_color", "padding": float64(12)}
	context.MinimumValidationWidth = 360
	context.DebugHostBadge = false
	if got := VisualDiagnostics(messages, context); len(got) != 0 {
		t.Fatalf("repaired notification still has visual diagnostics: %+v", got)
	}
}

func TestNotificationVisualLintKeepsHeaderFullWidthWithoutPanelBorder(t *testing.T) {
	components := []any{
		map[string]any{"id": "root", "component": "Card", "child": "layout", "padding": float64(12)},
		map[string]any{"id": "layout", "component": "Column", "children": []any{"header", "body"}},
		map[string]any{"id": "header", "component": "Card", "child": "identity", "backgroundColorToken": "extended_blue0_color", "borderWidth": float64(0.5), "borderColorToken": "extended_blue1_color", "padding": float64(12)},
		map[string]any{"id": "identity", "component": "Text", "text": "GitHub Issue #1438"},
		map[string]any{"id": "body", "component": "Column", "children": []any{"identity"}, "padding": float64(12)},
	}
	messages := []map[string]any{{"createSurface": map[string]any{"components": components}}}
	context := VisualContext{Archetype: "notification", MinimumValidationWidth: 360}
	bad := VisualDiagnostics(messages, context)
	for _, code := range []string{"HEADER_UNREQUESTED_BORDER", "HEADER_NOT_FULL_WIDTH", "HEADER_BOTTOM_RADIUS_UNWANTED", "ROOT_CARD_NOT_TRANSPARENT"} {
		if !hasVisualDiagnostic(bad, code) {
			t.Errorf("inset, bordered header is missing %s: %+v", code, bad)
		}
	}
	delete(components[2].(map[string]any), "borderWidth")
	delete(components[2].(map[string]any), "borderColorToken")
	components[2].(map[string]any)["cornerRadius"] = float64(0)
	components[0].(map[string]any)["padding"] = float64(0)
	components[0].(map[string]any)["backgroundColor"] = "#00FFFFFF"
	if got := VisualDiagnostics(messages, context); len(got) != 0 {
		t.Fatalf("full-width borderless header still has visual diagnostics: %+v", got)
	}
}

func TestRootCardVisualLintRequiresExplicitTransparency(t *testing.T) {
	root := map[string]any{"id": "root", "component": "Card", "child": "body"}
	body := map[string]any{"id": "body", "component": "Text", "text": "内容"}
	messages := []map[string]any{{"createSurface": map[string]any{"components": []any{root, body}}}}
	for _, background := range []any{nil, "#FFFFFFFF", "#00FFFFFF"} {
		if background == nil {
			delete(root, "backgroundColor")
		} else {
			root["backgroundColor"] = background
		}
		got := VisualDiagnostics(messages, VisualContext{})
		if hasVisualDiagnostic(got, "ROOT_CARD_NOT_TRANSPARENT") != (background != "#00FFFFFF") {
			t.Errorf("background=%v diagnostics=%+v", background, got)
		}
	}
	root["backgroundColorToken"] = "common_fg_z1_color"
	if got := VisualDiagnostics(messages, VisualContext{}); !hasVisualDiagnostic(got, "ROOT_CARD_NOT_TRANSPARENT") {
		t.Fatalf("token must not override root transparency: %+v", got)
	} else if got[0].Severity != "warning" {
		t.Fatalf("an intentional full-card fill should not fail protocol lint: %+v", got)
	}
}

func TestKeyValueVisualLintRequiresSeparatorAndCentersCompactRow(t *testing.T) {
	components := []any{
		map[string]any{"id": "root", "component": "Row", "children": []any{"iso_label", "iso_result"}, "align": "start"},
		map[string]any{"id": "iso_label", "component": "Text", "text": "ISO 8601", "variant": "caption"},
		map[string]any{"id": "iso_result", "component": "Text", "text": "后端返回参数非法", "weight": float64(1)},
	}
	messages := []map[string]any{{"createSurface": map[string]any{"components": components}}}
	bad := VisualDiagnostics(messages, VisualContext{})
	for _, code := range []string{"KEY_VALUE_LABEL_MISSING_COLON", "KEY_VALUE_ROW_NOT_CENTERED"} {
		if !hasVisualDiagnostic(bad, code) {
			t.Errorf("key-value row is missing %s: %+v", code, bad)
		}
	}
	components[0].(map[string]any)["align"] = "center"
	components[1].(map[string]any)["text"] = "ISO 8601："
	if got := VisualDiagnostics(messages, VisualContext{}); len(got) != 0 {
		t.Fatalf("repaired key-value row still has diagnostics: %+v", got)
	}
}

func TestKeyValueVisualLintAlignsFirstLinesWhenValueWraps(t *testing.T) {
	components := []any{
		map[string]any{"id": "root", "component": "Row", "children": []any{"space_label", "space_result"}, "align": "center"},
		map[string]any{"id": "space_label", "component": "Text", "text": "空格时间：", "variant": "caption"},
		map[string]any{"id": "space_result", "component": "Text", "text": "CLI 返回 runAtText is required", "variant": "body", "maxLine": float64(3)},
	}
	messages := []map[string]any{{"createSurface": map[string]any{"components": components}}}
	if got := VisualDiagnostics(messages, VisualContext{}); !hasVisualDiagnostic(got, "KEY_VALUE_FIRST_LINE_MISALIGNED") {
		t.Fatalf("wrapping value must flag whole-row centering: %+v", got)
	}
	components[0].(map[string]any)["align"] = "start"
	components[1].(map[string]any)["variant"] = "body"
	if got := VisualDiagnostics(messages, VisualContext{}); len(got) != 0 {
		t.Fatalf("first-line alignment still has diagnostics: %+v", got)
	}
}

func hasVisualDiagnostic(diagnostics []protocol.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
