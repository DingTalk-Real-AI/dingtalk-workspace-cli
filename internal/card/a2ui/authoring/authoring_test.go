// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package authoring

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui/protocol"
)

func TestCrossPlatformCoverageRecipesCompileToValidA2UI(t *testing.T) {
	registry, err := protocol.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, recipe := range Recipes() {
		t.Run(recipe.Name, func(t *testing.T) {
			messages, err := Compile(Spec{Recipe: recipe.Name, SurfaceID: "test-" + recipe.Name, Title: "发布检查", Status: "待处理", Body: "**结论**：所有检查通过。", ImageURL: "https://example.com/cover.png", FileName: "report.pdf", FileURL: "https://example.com/report.pdf", DetailURL: "https://example.com/details", PrimaryCTA: "确认", Secondary: "稍后处理"})
			if err != nil {
				t.Fatal(err)
			}
			validation := registry.Validate(messages, "create")
			if !validation.Valid {
				t.Fatalf("invalid recipe: %+v", validation.Diagnostics)
			}
			components := messages[2]["updateComponents"].(map[string]any)["components"].([]any)
			footerID := "actions_footer"
			if recipe.Name == "information" {
				footerID = "actions_section"
			} else if recipe.Name == "schedule" {
				footerID = "schedule_body"
			}
			foundFooter := false
			for _, raw := range components {
				component := raw.(map[string]any)
				if component["id"] == footerID {
					foundFooter = true
					if recipe.Name == "schedule" {
						if _, ok := component["padding"]; ok {
							t.Errorf("schedule body duplicates the root inset: %v", component)
						}
					} else if component["padding"] != 4.0 || component["gap"] != spacingContent {
						t.Errorf("%s should use 4px inset and 12px gap: %v", footerID, component)
					}
				}
				if component["id"] == "actions" || component["id"] == "schedule_actions" {
					if component["padding"] != nil {
						t.Errorf("button row should not add another inset: %v", component)
					}
				}
			}
			if !foundFooter {
				t.Errorf("missing %s", footerID)
			}
		})
	}
}

func TestCrossPlatformCoverageRecommendWithMatchDoesNotInferRecipeFromWordFragments(t *testing.T) {
	for _, intent := range []string{
		"GitHub issue timestamp format mismatch",
		"scheduled notice failure",
		"Summarize service health with metrics, an incident, and one investigation action",
	} {
		if recipe, matched := RecommendWithMatch(intent); matched || recipe.Name != "notification" {
			t.Fatalf("intent %q: recipe=%q matched=%v", intent, recipe.Name, matched)
		}
	}
	if recipe, matched := RecommendWithMatch("notification about a bug"); !matched || recipe.Name != "notification" {
		t.Fatalf("explicit notification: recipe=%q matched=%v", recipe.Name, matched)
	}
	for _, intent := range []string{"service quality report", "服务质量报告"} {
		if recipe, matched := RecommendWithMatch(intent); !matched || recipe.Name != "information" {
			t.Fatalf("report intent %q should use the information recipe: recipe=%q matched=%v", intent, recipe.Name, matched)
		}
	}
}

func TestCrossPlatformCoverageInformationRecipeBuildsAReadableHierarchy(t *testing.T) {
	messages, err := Compile(Spec{
		Recipe:          "information",
		SurfaceID:       "information-hierarchy",
		Title:           "季度执行报告",
		Subtitle:        "2026 Q3 联合复盘",
		Status:          "已完成",
		Body:            "## 核心结论\n\n目标按计划完成。",
		ImageURL:        "https://example.com/cover.jpg",
		ImageFit:        "contain",
		ImageCaption:    "报告封面",
		Metrics:         []Metric{{Label: "完成率", Value: "96%"}, {Label: "风险", Value: "0"}},
		Highlights:      []Highlight{{Title: "核心能力", Detail: "完成验收", Status: "已完成", Theme: "green"}, {Title: "体验改善", Detail: "耗时下降", Status: "符合预期", Theme: "blue"}},
		FileName:        "report.pdf",
		FileURL:         "https://example.com/report.pdf",
		FilePreviewURL:  "https://example.com/report/preview",
		FileDescription: "正式版",
		FileMIMEType:    "application/pdf",
		FileSize:        1024,
		Details:         "### 数据口径\n\n按验收结果计算。",
		DetailURL:       "https://example.com/details",
		PrimaryCTA:      "查看完整报告",
	})
	if err != nil {
		t.Fatal(err)
	}
	updates := messages[2]["updateComponents"].(map[string]any)["components"].([]any)
	byID := map[string]map[string]any{}
	for _, raw := range updates {
		component := raw.(map[string]any)
		byID[fmt.Sprint(component["id"])] = component
	}
	for _, id := range []string{"subtitle", "hero", "metrics", "metric_panel_1", "metric_panel_2", "metric_1", "body", "highlights", "highlight_panel_1", "highlight_panel_2", "file", "details", "details_content", "actions"} {
		if byID[id] == nil {
			t.Errorf("compiled information card is missing hierarchy component %q", id)
		}
	}
	if got := byID["status"]["theme"]; got != "green" {
		t.Errorf("completed status theme = %v, want green", got)
	}
	if got := byID["details"]["component"]; got != "Column" {
		t.Errorf("information details component = %v, want always-visible Column", got)
	}
	if got := fmt.Sprint(byID["details"]["children"]); got != "[details_content]" {
		t.Errorf("details children = %v, want padded content wrapper", got)
	}
	if got := fmt.Sprint(byID["details_content"]["children"]); got != "[details_body]" || byID["detail_link"] != nil {
		t.Errorf("information details must not duplicate the primary report action: children=%v link=%v", got, byID["detail_link"])
	}
	if got := byID["details_content"]["padding"]; got != 12.0 {
		t.Errorf("details content padding = %v, want 12", got)
	}
	if got := byID["details_content"]["gap"]; got != 8.0 {
		t.Errorf("details content gap = %v, want 8", got)
	}
	if got := fmt.Sprint(byID["details_body"]["content"]); !strings.Contains(got, "> **数据口径与补充说明**") || !strings.Contains(got, "> 按验收结果计算。") {
		t.Errorf("information details must use a visible Markdown quote, got %q", got)
	}
	if got := byID["file"]["previewUrl"]; got != "https://example.com/report/preview" {
		t.Errorf("file previewUrl = %v", got)
	}
	if got := byID["hero"]["fit"]; got != "contain" {
		t.Errorf("information chart fit = %v, want contain", got)
	}
	if got := byID["root"]["component"]; got != "Column" {
		t.Errorf("root component = %v, want a plain Column", got)
	}
	if got := byID["content"]["gap"]; got != spacingContent {
		t.Errorf("top-level block gap = %v, want %v", got, spacingContent)
	}
	if _, ok := byID["body_content"]["padding"]; ok {
		t.Errorf("body must share the root inset, got %v", byID["body_content"]["padding"])
	}
	if header := byID["header"]; header["component"] != "Column" || header["padding"] != float64(0) || byID["header_row"]["component"] != "Row" {
		t.Errorf("information header should be an unpainted row: %v", header)
	}
	for _, field := range []string{"borderWidth", "borderColorToken", "cornerRadius"} {
		if _, ok := byID["header"][field]; ok {
			t.Errorf("header must not inherit content-panel field %s", field)
		}
	}
	for _, pair := range [][2]string{{"metric_panel_1", "metric_1"}, {"metric_panel_2", "metric_2"}, {"highlight_panel_1", "highlight_1"}, {"highlight_panel_2", "highlight_2"}} {
		panel := byID[pair[0]]
		if panel["component"] != "Card" || panel["child"] != pair[1] || panel["backgroundColorToken"] != "common_fg_z1_color" || panel["borderWidth"] != 0.5 || panel["borderColorToken"] != "common_line_hard_color" || panel["padding"] != 12.0 || panel["cornerRadius"] != 8.0 {
			t.Errorf("%s does not match the ordinary content-panel contract: %v", pair[0], panel)
		}
	}
	if got := fmt.Sprint(byID["body_content"]["children"]); got != "[summary_section metrics_section highlights_section attachment_section details actions_section]" {
		t.Errorf("information sections must follow conclusion, evidence, progress, and artifacts: %v", got)
	}
	if got := byID["body_content"]["gap"]; got != 16.0 {
		t.Errorf("information section gap = %v, want 16", got)
	}
	for _, section := range []struct {
		id       string
		children string
	}{
		{"summary_section", "[body subtitle hero image_caption]"},
		{"metrics_section", "[metrics_title metrics]"},
		{"highlights_section", "[highlights_title highlights]"},
		{"attachment_section", "[divider_attachment attachment_title file]"},
		{"actions_section", "[divider_actions actions]"},
	} {
		wantGap := 8.0
		if section.id == "actions_section" {
			wantGap = 12.0
		}
		if got := fmt.Sprint(byID[section.id]["children"]); got != section.children || byID[section.id]["gap"] != wantGap {
			t.Errorf("%s should keep related content together: %v", section.id, byID[section.id])
		}
	}
	if got := byID["attachment_title"]["text"]; got != "结果产物" {
		t.Errorf("information attachment heading = %v, want result artifacts", got)
	}
	if got := fmt.Sprint(byID["metrics"]["children"]); got != "[metric_panel_1 metric_panel_2]" {
		t.Errorf("metric panels must be separate siblings, got %v", got)
	}
	if got := fmt.Sprint(byID["highlights"]["children"]); got != "[highlight_panel_1 highlight_panel_2]" {
		t.Errorf("highlight panels must be separate siblings, got %v", got)
	}
	if got := byID["metric_panel_1"]["weight"]; got != 1.0 {
		t.Errorf("metric panel weight = %v, want 1", got)
	}
	if got := byID["highlight_1"]["gap"]; got != 8.0 {
		t.Errorf("highlight row gap = %v, want 8", got)
	}
	for _, id := range []string{"metric_1", "highlight_1"} {
		if _, ok := byID[id]["backgroundColorToken"]; ok {
			t.Errorf("%s must inherit its own panel surface", id)
		}
	}
	for id, token := range map[string]string{
		"subtitle": "common_level3_base_color", "image_caption": "common_level3_base_color",
		"metric_label_1": "common_level2_base_color", "highlight_detail_1": "common_level2_base_color",
	} {
		if _, invalid := byID[id]["color"]; invalid || byID[id]["colorToken"] != token {
			t.Errorf("%s must use the protocol color token %s: %v", id, token, byID[id])
		}
	}
}

func TestCrossPlatformCoverageRequestedComponentGuidesAreCovered(t *testing.T) {
	for _, name := range []string{"Button", "File", "Markdown", "TextField", "Link", "Card", "Row", "Column", "Tag", "Divider", "CollapsiblePanel", "Image", "Text", "ChoicePicker"} {
		if _, ok := Guide(name); !ok {
			t.Errorf("missing guide for %s", name)
		}
	}
}

func TestCrossPlatformCoverageSurfacePoliciesKeepCardsReadableWithoutInventingWireFields(t *testing.T) {
	policies := SurfacePolicies()
	if len(policies) != 3 {
		t.Fatalf("surface policies=%d, want 3", len(policies))
	}
	byName := make(map[string]SurfacePolicy, len(policies))
	for _, policy := range policies {
		byName[policy.Name] = policy
		if policy.MinWidth < 360 {
			t.Errorf("%s minWidth=%d, want at least 360", policy.Name, policy.MinWidth)
		}
		if policy.ObservedHostMinWidth != 320 || policy.ObservedHostMaxWidth != 640 {
			t.Errorf("%s observed host width range=%d..%d, want 320..640", policy.Name, policy.ObservedHostMinWidth, policy.ObservedHostMaxWidth)
		}
		if policy.WidthMode != "content-adaptive" || policy.NarrowViewport != "fit-available-width" {
			t.Errorf("%s has non-responsive policy: %+v", policy.Name, policy)
		}
		if policy.Enforcement != "host-surface-required" {
			t.Errorf("%s enforcement=%q", policy.Name, policy.Enforcement)
		}
	}
	if byName["notification"].MinWidth != 360 {
		t.Errorf("notification minWidth=%d, want 360", byName["notification"].MinWidth)
	}
	if byName["ai"].MinWidth != 360 {
		t.Errorf("ai minWidth=%d, want 360", byName["ai"].MinWidth)
	}
}

func TestCrossPlatformCoverageRootCardBackgroundMustBeTransparent(t *testing.T) {
	compileBackground := func(background string) (any, bool) {
		t.Helper()
		messages, err := Compile(Spec{Recipe: "form", SurfaceID: "background", Title: "通知", Body: "内容", BackgroundColor: background})
		if err != nil {
			t.Fatal(err)
		}
		components := messages[2]["updateComponents"].(map[string]any)["components"].([]any)
		got, ok := components[0].(map[string]any)["backgroundColor"]
		return got, ok
	}
	if got, ok := compileBackground(""); ok {
		t.Errorf("default Column root should inherit host background, got %v", got)
	}
	if got, ok := compileBackground("#00FFFFFF"); ok {
		t.Errorf("transparent Column root should inherit host background, got %v", got)
	}
	if _, err := Compile(Spec{Recipe: "form", SurfaceID: "opaque-root", Title: "通知", Body: "内容", BackgroundColor: "#FFE8F3FF"}); err == nil {
		t.Fatal("opaque root Card background must be rejected")
	}
}

func TestCrossPlatformCoverageSeventeenSemanticBlocksAreRegistered(t *testing.T) {
	if got := len(Blocks()); got != 17 {
		t.Fatalf("blocks=%d, want 17", got)
	}
}

func TestCrossPlatformCoverageContentSectionBlockUsesCanonicalCardTokens(t *testing.T) {
	var section *Block
	for _, block := range Blocks() {
		if block.Name == "content-panel" {
			candidate := block
			section = &candidate
			break
		}
	}
	if section == nil {
		t.Fatal("content-panel block is not registered")
	}
	if got := strings.Join(section.Components, ","); got != "Card,Column" {
		t.Errorf("components=%q, want Card,Column", got)
	}
	for _, want := range []string{"padding=12", "cornerRadius=8", "borderWidth=0.5"} {
		if !strings.Contains(section.Layout, want) {
			t.Errorf("layout %q is missing %q", section.Layout, want)
		}
	}
	if len(section.Styles) != 3 {
		t.Fatalf("styles=%d, want 3", len(section.Styles))
	}
	ordinary, highlight, softTint := section.Styles[0], section.Styles[1], section.Styles[2]
	if ordinary.Name != "ordinary" || ordinary.BackgroundColorToken != "common_fg_z1_color" || ordinary.BorderWidth != 0.5 || ordinary.BorderColorToken != "common_line_hard_color" {
		t.Errorf("ordinary style=%+v", ordinary)
	}
	if highlight.Name != "highlight" || highlight.BackgroundColorToken != "extended_*0_color" || highlight.BorderWidth != 0.5 || highlight.BorderColorToken != "extended_*1_color" || !strings.Contains(highlight.Constraint, "同一 extended 色系") {
		t.Errorf("highlight style=%+v", highlight)
	}
	if softTint.Name != "soft-tint" || softTint.BackgroundColorToken != "extended_*0_color" || softTint.BorderWidth != 0 || softTint.BorderColorToken != "" {
		t.Errorf("soft-tint style=%+v", softTint)
	}
	rules := strings.Join(section.Rules, "\n")
	for _, want := range []string{"common_fg_z1_color", "common_line_hard_color", "extended_*0_color", "extended_*1_color", "同色系", "borderWidth=0.5"} {
		if !strings.Contains(rules, want) {
			t.Errorf("rules are missing %q", want)
		}
	}
}

func TestCrossPlatformCoverageContentSectionStylesValidateAgainstA2UIProtocol(t *testing.T) {
	registry, err := protocol.Load()
	if err != nil {
		t.Fatal(err)
	}
	result := registry.Validate([]map[string]any{{
		"version": "v1.0",
		"createSurface": map[string]any{
			"surfaceId": "content-panel-styles",
			"catalogId": registry.Catalog().CatalogID,
			"components": []any{
				map[string]any{"id": "root", "component": "Column", "children": []any{"ordinary", "highlight"}, "gap": 8.0},
				map[string]any{"id": "ordinary", "component": "Card", "child": "ordinary-content", "padding": 12.0, "cornerRadius": 8.0, "backgroundColorToken": "common_fg_z1_color", "borderWidth": 0.5, "borderColorToken": "common_line_hard_color"},
				map[string]any{"id": "ordinary-content", "component": "Text", "text": "普通区块"},
				map[string]any{"id": "highlight", "component": "Card", "child": "highlight-content", "padding": 12.0, "cornerRadius": 8.0, "backgroundColorToken": "extended_blue0_color", "borderWidth": 0.5, "borderColorToken": "extended_blue1_color"},
				map[string]any{"id": "highlight-content", "component": "Text", "text": "高亮区块"},
			},
		},
	}}, "create")
	if !result.Valid {
		t.Fatalf("content-panel styles must match the public A2UI protocol: %+v", result.Diagnostics)
	}
}

func TestCrossPlatformCoverageNotificationRecipesUseCompactHeaderBlock(t *testing.T) {
	for _, recipe := range Recipes() {
		if recipe.Name != "schedule" && recipe.Name != "task" {
			continue
		}
		if !containsString(recipe.Blocks, "compact-notification-header") {
			t.Errorf("%s blocks=%v, want compact-notification-header", recipe.Name, recipe.Blocks)
		}
	}
	for _, block := range Blocks() {
		if block.Name == "compact-notification-header" && (strings.Contains(block.Layout, "CardHeader") || !strings.Contains(block.Layout, "Row")) {
			t.Errorf("compact notification layout=%q, want an unpainted title row", block.Layout)
		}
	}
	for _, recipe := range []string{"task"} {
		messages, err := Compile(Spec{Recipe: recipe, SurfaceID: "compact-" + recipe, Title: "待处理事项", Status: "进行中", Body: "请及时处理。"})
		if err != nil {
			t.Fatal(err)
		}
		updates := messages[2]["updateComponents"].(map[string]any)["components"].([]any)
		byID := map[string]map[string]any{}
		for _, raw := range updates {
			component := raw.(map[string]any)
			byID[fmt.Sprint(component["id"])] = component
		}
		if got := byID["header"]["component"]; got != "Column" {
			t.Errorf("%s header=%v, want Column", recipe, got)
		}
		if got := fmt.Sprint(byID["header_row"]["children"]); got != "[title status]" {
			t.Errorf("%s header row=%v, want title and status", recipe, got)
		}
		if got := byID["root"]["component"]; got != "Column" {
			t.Errorf("%s root component=%v, want Column", recipe, got)
		}
		if _, ok := byID["compact_body"]["padding"]; ok {
			t.Errorf("%s body duplicates the root inset", recipe)
		}
	}
}

func TestCrossPlatformCoverageScheduleRecipeUsesMeetingDetailsWithoutConflictScaffolding(t *testing.T) {
	messages, err := Compile(Spec{
		Recipe: "schedule", SurfaceID: "meeting-schedule", Title: "Schedule", Body: "Design review",
		ScheduleTime: "Sep 24, 01:00–02:00 (GMT+8)", MeetingURL: "https://example.com/meeting",
		MeetingState: "Not started", MeetingNumber: "249 643 090", PrimaryCTA: "Accept",
	})
	if err != nil {
		t.Fatal(err)
	}
	updates := messages[2]["updateComponents"].(map[string]any)["components"].([]any)
	byID := map[string]map[string]any{}
	for _, raw := range updates {
		component := raw.(map[string]any)
		byID[component["id"].(string)] = component
	}
	if byID["header"]["component"] != "Text" || byID["header"]["colorToken"] != "extended_green6_color" {
		t.Errorf("meeting header = %v", byID["header"])
	}
	if byID["meeting_title"]["sizeToken"] != "common_h2_text_style__font_size" {
		t.Errorf("meeting title should retain its heading size")
	}
	if got := fmt.Sprint(byID["schedule_body"]["children"]); got != "[meeting_title meeting_time_row meeting_join_row schedule_actions]" {
		t.Errorf("meeting content order = %s", got)
	}
	if byID["meeting_join"]["component"] != "Link" || byID["meeting_meta"] == nil || byID["primary_button"]["disabled"] != false || byID["primary_button"]["variant"] != "primary" {
		t.Errorf("meeting link, metadata, or enabled accept action is missing")
	}
	if byID["primary_label"]["text"] != "Accept" || byID["primary_button"]["action"].(map[string]any)["event"].(map[string]any)["name"] != "schedule_accept" {
		t.Errorf("meeting accept action is not wired")
	}
	for _, id := range []string{"time_icon", "meeting_icon"} {
		icon := byID[id]
		if icon["component"] != "Text" || icon["variant"] != "body" || icon["text"] != "" {
			t.Errorf("%s should inherit the body-text size: %v", id, icon)
		}
		if icon["colorToken"] != map[string]string{"time_icon": "common_level1_base_color", "meeting_icon": "extended_blue6_color"}[id] || icon["icon"].(map[string]any)["name"] != map[string]string{"time_icon": "icon_time_line", "meeting_icon": "icon_videoon_line"}[id] {
			t.Errorf("%s icon should match its adjacent text", id)
		}
	}
	if byID["meeting_time_row"]["gap"] != float64(0) || byID["meeting_join_row"]["gap"] != float64(0) {
		t.Errorf("meeting icons should rely on the text icon's built-in trailing gap")
	}
	for _, id := range []string{"subtitle", "details", "compact_body", "secondary_button"} {
		if byID[id] != nil {
			t.Errorf("meeting card still contains unrelated %s", id)
		}
	}
}

func TestCrossPlatformCoverageInformationMetricsUseLabeledThemedTiles(t *testing.T) {
	messages, err := Compile(Spec{
		Recipe: "information", SurfaceID: "themed-metrics", Title: "Quarterly review", Body: "The team met its goal.",
		Metrics: []Metric{{Label: "Completion", Value: "96%", Theme: "green"}, {Label: "On time", Value: "18 / 19", Theme: "blue"}, {Label: "Risk", Value: "0", Theme: "orange"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	updates := messages[2]["updateComponents"].(map[string]any)["components"].([]any)
	byID := map[string]map[string]any{}
	for _, raw := range updates {
		component := raw.(map[string]any)
		byID[component["id"].(string)] = component
	}
	for i, theme := range []string{"green", "blue", "orange"} {
		panel := byID[fmt.Sprintf("metric_panel_%d", i+1)]
		column := byID[fmt.Sprintf("metric_%d", i+1)]
		value := byID[fmt.Sprintf("metric_value_%d", i+1)]
		label := byID[fmt.Sprintf("metric_label_%d", i+1)]
		if panel["backgroundColorToken"] != "extended_"+theme+"0_color" || panel["backgroundColor"] != metricBackground(theme) || panel["cornerRadius"] != 8.0 || panel["borderWidth"] != nil || panel["borderColorToken"] != nil {
			t.Errorf("%s tile surface = %v", theme, panel)
		}
		if got := fmt.Sprint(column["children"]); got != fmt.Sprintf("[metric_label_%d metric_value_%d]", i+1, i+1) {
			t.Errorf("%s tile reading order = %s", theme, got)
		}
		if column["gap"] != spacingTight || label["variant"] != "body" {
			t.Errorf("%s tile text hierarchy = %v, %v", theme, column, label)
		}
		if value["colorToken"] != "extended_"+theme+"6_color" || value["customLightColor"] != metricValueColor(theme) || value["customDarkColor"] != metricDarkValueColor(theme) {
			t.Errorf("%s tile value color = %v", theme, value)
		}
	}
}

func TestCrossPlatformCoverageTaskRecipeKeepsCompletionCriteriaVisible(t *testing.T) {
	messages, err := Compile(Spec{
		Recipe: "task", SurfaceID: "task-criteria", Title: "Fix login", Status: "In progress",
		Body:      "### Progress\n\n- Regression checks are running",
		Details:   "### Completion criteria\n\n1. Login works on refresh",
		DetailURL: "https://example.com/task", PrimaryCTA: "Update progress",
	})
	if err != nil {
		t.Fatal(err)
	}
	updates := messages[2]["updateComponents"].(map[string]any)["components"].([]any)
	byID := map[string]map[string]any{}
	for _, raw := range updates {
		component := raw.(map[string]any)
		byID[fmt.Sprint(component["id"])] = component
	}
	if got := fmt.Sprint(byID["compact_body"]["children"]); !strings.Contains(got, "body task_criteria") {
		t.Errorf("task progress and completion criteria should be adjacent, got %s", got)
	}
	if got := byID["task_criteria"]["content"]; got != "### Completion criteria\n\n1. Login works on refresh" {
		t.Errorf("task completion criteria = %v", got)
	}
	for _, id := range []string{"details", "details_content", "detail_link"} {
		if byID[id] != nil {
			t.Errorf("task should not contain generic detail component %s", id)
		}
	}
}

func TestCrossPlatformCoverageResolveRecipeReportsTheEffectiveDefault(t *testing.T) {
	got, err := ResolveRecipe(Spec{Title: "普通通知", Body: "处理完成"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "notification" {
		t.Fatalf("recipe=%q, want notification", got)
	}
	if _, err := ResolveRecipe(Spec{Recipe: "missing"}); err == nil {
		t.Fatal("expected unknown recipe rejection")
	}
	if _, err := ResolveRecipe(Spec{Recipe: "report"}); err == nil {
		t.Fatal("the duplicate report recipe should not remain selectable")
	}
	if _, err := ResolveRecipe(Spec{Recipe: "approval"}); err == nil {
		t.Fatal("the redundant approval recipe should not remain selectable")
	}
}

func TestCrossPlatformCoverageBuiltInRecipesUseUnpaintedInsetHeader(t *testing.T) {
	for _, recipe := range []string{"notification", "information", "form"} {
		t.Run(recipe, func(t *testing.T) {
			messages, err := Compile(Spec{Recipe: recipe, SurfaceID: "header-" + recipe, Title: "标题", Status: "进行中", Body: "说明"})
			if err != nil {
				t.Fatal(err)
			}
			updates := messages[2]["updateComponents"].(map[string]any)["components"].([]any)
			byID := map[string]map[string]any{}
			for _, raw := range updates {
				component := raw.(map[string]any)
				byID[component["id"].(string)] = component
			}
			if byID["root"]["component"] != "Column" || byID["content"]["gap"] != spacingContent {
				t.Fatalf("%s should use one platform inset around an unpainted header", recipe)
			}
			if got := fmt.Sprint(byID["content"]["children"]); got != "[header body_content]" {
				t.Fatalf("%s content children = %s", recipe, got)
			}
			header := byID["header"]
			if header["component"] != "Column" || header["backgroundColorToken"] != nil {
				t.Fatalf("%s should use a quiet header without a painted surface: %+v", recipe, header)
			}
			for _, field := range []string{"borderWidth", "borderColorToken", "cornerRadius"} {
				if _, ok := header[field]; ok {
					t.Errorf("%s header has unsolicited %s", recipe, field)
				}
			}
			if _, ok := byID["body_content"]["padding"]; ok {
				t.Errorf("%s body duplicates the root inset", recipe)
			}
			if byID["title"]["colorToken"] != "common_level1_base_color" || byID["title"]["variant"] != "body" || byID["title"]["sizeToken"] != nil || byID["title"]["weight"] != float64(1) {
				t.Errorf("%s title styling = %+v", recipe, byID["title"])
			}
			if _, exists := byID["divider_top"]; exists {
				t.Errorf("%s contains redundant divider_top", recipe)
			}
		})
	}
}

func TestCrossPlatformCoverageImageTextItemBlockDeclaresCompactHorizontalContract(t *testing.T) {
	var found *Block
	for _, block := range Blocks() {
		if block.Name == "image-text-item" {
			blockCopy := block
			found = &blockCopy
			break
		}
	}
	if found == nil {
		t.Fatal("missing image-text-item block")
	}
	if found.Layout != "Row(align=center,gap=12) > Column(weight=1) + Image(variant=smallFeature)" {
		t.Errorf("layout=%q", found.Layout)
	}
	if !strings.Contains(found.Fallback, "纯文本") {
		t.Errorf("fallback=%q, want text-only degradation", found.Fallback)
	}
}

func TestCrossPlatformCoverageFormRecipeInformationArchitecture(t *testing.T) {
	compile := func(recipe string) map[string]map[string]any {
		t.Helper()
		messages, err := Compile(Spec{
			Recipe:     recipe,
			SurfaceID:  "distinct-" + recipe,
			Title:      "流程处理",
			Body:       "请检查内容并继续。",
			PrimaryCTA: "提交",
			Secondary:  "取消",
		})
		if err != nil {
			t.Fatal(err)
		}
		updates := messages[2]["updateComponents"].(map[string]any)["components"].([]any)
		result := make(map[string]map[string]any, len(updates))
		for _, raw := range updates {
			component := raw.(map[string]any)
			result[fmt.Sprint(component["id"])] = component
		}
		return result
	}

	form := compile("form")
	if form["form_intro"] == nil || form["form_fields"] == nil || form["decision_title"] != nil {
		t.Fatal("form must expose an instruction and grouped fields without approval decision chrome")
	}
	if got := form["form_fields"]["children"]; fmt.Sprint(got) != "[comment choice]" {
		t.Errorf("form field order = %v", got)
	}
	if got := form["form_fields"]["padding"]; got != nil {
		t.Errorf("form field container adds nested padding = %v", got)
	}
	if got := form["choice"]["label"]; got != "执行方式" {
		t.Errorf("form choice label = %v", got)
	}
	if got := form["secondary_button"]["variant"]; got != "borderless" {
		t.Errorf("form secondary variant = %v", got)
	}
	if got := eventName(form["primary_button"]); got != "form_submit" {
		t.Errorf("form primary event = %q", got)
	}
	if got := eventName(form["secondary_button"]); got != "form_cancel" {
		t.Errorf("form secondary event = %q", got)
	}
}

func TestCrossPlatformCoverageFormRecipeCompilesReleaseFieldsWithIndependentBindings(t *testing.T) {
	messages, err := Compile(Spec{
		Recipe: "form", SurfaceID: "release-form", Title: "Release details", Body: "Complete the release details.",
		PrimaryCTA: "Submit",
		FormFields: []FormField{
			{ID: "version", Label: "Version", Kind: "text"},
			{ID: "notes", Label: "Notes", Kind: "longText"},
			{ID: "platforms", Label: "Platforms", Kind: "checkbox", Options: []FormOption{{Label: "Web", Value: "web"}, {Label: "iOS", Value: "ios"}}},
			{ID: "environment", Label: "Environment", Kind: "dropdown", Options: []FormOption{{Label: "Staging", Value: "staging"}}},
			{ID: "strategy", Label: "Strategy", Kind: "radio", Options: []FormOption{{Label: "Gradual", Value: "gradual"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	updates := messages[2]["updateComponents"].(map[string]any)["components"].([]any)
	byID := map[string]map[string]any{}
	for _, raw := range updates {
		component := raw.(map[string]any)
		byID[fmt.Sprint(component["id"])] = component
	}
	if got := fmt.Sprint(byID["form_fields"]["children"]); got != "[field_version field_notes field_platforms_group field_environment_group field_strategy_group]" {
		t.Errorf("release field order = %s", got)
	}
	if got := byID["form_fields"]["padding"]; got != nil {
		t.Errorf("release field container adds nested padding = %v", got)
	}
	if got := byID["field_version"]["variant"]; got != "shortText" {
		t.Errorf("version variant = %v", got)
	}
	if got := byID["field_notes"]["variant"]; got != "longText" {
		t.Errorf("notes variant = %v", got)
	}
	for id, want := range map[string][2]string{
		"field_platforms":   {"multipleSelection", "checkbox"},
		"field_environment": {"mutuallyExclusive", "dropdown"},
		"field_strategy":    {"mutuallyExclusive", "checkbox"},
	} {
		if got := byID[id]; got["variant"] != want[0] || got["displayStyle"] != want[1] {
			t.Errorf("%s = %v, want variant=%s displayStyle=%s", id, got, want[0], want[1])
		}
		if byID[id+"_label"] == nil || byID[id+"_group"] == nil {
			t.Errorf("%s is missing its visible label group", id)
		}
	}
	form := messages[1]["updateDataModel"].(map[string]any)["value"].(map[string]any)["form"].(map[string]any)
	for _, id := range []string{"platforms", "environment", "strategy"} {
		if values, ok := form[id].([]string); !ok || len(values) != 0 {
			t.Errorf("choice %s must start with an empty string list, got %T %v", id, form[id], form[id])
		}
	}
	if got := eventName(byID["primary_button"]); got != "form_submit" {
		t.Errorf("submit event = %q", got)
	}
}

func eventName(component map[string]any) string {
	action, _ := component["action"].(map[string]any)
	event, _ := action["event"].(map[string]any)
	name, _ := event["name"].(string)
	return name
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func TestCrossPlatformCoverageRecommend(t *testing.T) {
	if got := Recommend("请生成一个审批确认卡片").Name; got != "form" {
		t.Fatalf("got %s", got)
	}
	if got := Recommend("一般提醒").Name; got != "notification" {
		t.Fatalf("got %s", got)
	}
}

func TestCrossPlatformCoverageStatusTagThemeHandlesNegativeFormsBeforeSuccessTokens(t *testing.T) {
	for _, value := range []string{"未完成", "未通过", "incomplete", "unsuccessful", "not completed", "failure"} {
		if got := statusTagTheme(value); got != "red" {
			t.Errorf("statusTagTheme(%q) = %q, want red", value, got)
		}
	}
	for _, value := range []string{"已完成", "已通过", "complete", "success"} {
		if got := statusTagTheme(value); got != "green" {
			t.Errorf("statusTagTheme(%q) = %q, want green", value, got)
		}
	}
	messages, err := Compile(Spec{Recipe: "notification", SurfaceID: "optional-status", Title: "无状态通知", Body: "无需虚构状态。"})
	if err != nil {
		t.Fatal(err)
	}
	components := messages[2]["updateComponents"].(map[string]any)["components"].([]any)
	for _, raw := range components {
		component := raw.(map[string]any)
		if component["id"] == "status" {
			t.Fatal("notification without a status must not invent a Tag")
		}
	}
}
