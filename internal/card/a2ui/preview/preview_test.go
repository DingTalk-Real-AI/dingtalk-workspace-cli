// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package preview

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui/authoring"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui/state"
)

func TestCrossPlatformCoverageRenderReferencePreview(t *testing.T) {
	messages, err := authoring.Compile(authoring.Spec{Recipe: "form", SurfaceID: "preview", Title: "信息收集", Status: "待填写", Body: "请补充发布信息。", PrimaryCTA: "提交", Secondary: "取消"})
	if err != nil {
		t.Fatal(err)
	}
	surface, err := state.Reduce(state.Surface{}, messages)
	if err != nil {
		t.Fatal(err)
	}
	html, err := Render(surface)
	if err != nil {
		t.Fatal(err)
	}
	text := string(html)
	for _, want := range []string{"reference_preview", "信息收集", "请补充发布信息", `data-component="Button"`, `type="radio"`, `class="FieldLabel">补充信息`, `placeholder="请输入 Agent 继续处理所需的信息"`, `class="FieldLabel">执行方式`} {
		if !strings.Contains(text, want) {
			t.Errorf("preview missing %q", want)
		}
	}
}

func TestCrossPlatformCoverageReferencePreviewInteractionContract(t *testing.T) {
	messages, err := authoring.Compile(authoring.Spec{
		Recipe:     "form",
		SurfaceID:  "interactive-preview",
		Title:      "交互验收",
		Body:       "填写信息并选择处理方式。",
		PrimaryCTA: "提交",
		Secondary:  "取消",
	})
	if err != nil {
		t.Fatal(err)
	}
	surface, err := state.Reduce(state.Surface{}, messages)
	if err != nil {
		t.Fatal(err)
	}
	html, err := Render(surface)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(html)
	for _, want := range []string{
		`<textarea class="TextFieldControl" placeholder="请输入 Agent 继续处理所需的信息" data-binding-path="/form/comment">`,
		`type="radio" name="choice" value="option_1" data-binding-path="/form/choice"`,
		`<button type="button" class="ButtonControl borderless" data-event-name="form_cancel" data-surface-id="interactive-preview">`,
		`<button type="button" class="ButtonControl primary" data-event-name="form_submit" data-surface-id="interactive-preview">`,
		`id="interaction-result"`,
		`new CustomEvent("dws-a2ui-preview-action"`,
		`form:model.form??{}`,
		`不发送真实请求`,
		`new URLSearchParams(location.search).has("embed")`,
		`.embed .notice{display:none}`,
		`result.setAttribute("role","status")`,
		`document.querySelectorAll('[data-component="ChoicePicker"]')`,
		`group.classList.contains("multipleSelection")?"group":"radiogroup"`,
		`class="ChoicePickerControl"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("interactive preview missing %q", want)
		}
	}
	for _, forbidden := range []string{" disabled", "fetch(", "XMLHttpRequest", "navigator.sendBeacon"} {
		if strings.Contains(rendered, forbidden) {
			t.Errorf("interactive preview contains forbidden behavior %q", forbidden)
		}
	}
	if strings.Contains(rendered, `<textarea class="TextField"`) {
		t.Fatal("TextField component wrapper and native control must not share the same style class")
	}
	if strings.Contains(rendered, `<div class="ChoicePicker">`) {
		t.Fatal("ChoicePicker component wrapper and internal control must not duplicate layout classes")
	}
}

func TestCrossPlatformCoverageRenderReleaseFormChoiceStyles(t *testing.T) {
	messages, err := authoring.Compile(authoring.Spec{
		Recipe: "form", SurfaceID: "release-preview", Title: "Release details", Body: "Complete the release details.",
		FormFields: []authoring.FormField{
			{ID: "version", Label: "Version", Kind: "text"},
			{ID: "platforms", Label: "Platforms", Kind: "checkbox", Options: []authoring.FormOption{{Label: "Web", Value: "web"}, {Label: "iOS", Value: "ios"}}},
			{ID: "environment", Label: "Environment", Kind: "dropdown", Options: []authoring.FormOption{{Label: "Staging", Value: "staging"}}},
			{ID: "strategy", Label: "Strategy", Kind: "radio", Options: []authoring.FormOption{{Label: "Gradual", Value: "gradual"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	surface, err := state.Reduce(state.Surface{}, messages)
	if err != nil {
		t.Fatal(err)
	}
	html, err := Render(surface)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`data-binding-path="/form/version"`,
		`type="checkbox" name="field_platforms"`,
		`<select data-binding-path="/form/environment">`,
		`<option value="staging">Staging</option>`,
		`type="radio" name="field_strategy"`,
		`selected.get(path).push(control.value)`,
	} {
		if !strings.Contains(string(html), want) {
			t.Errorf("release form preview missing %q", want)
		}
	}
}

func TestCrossPlatformCoverageRenderRichInformationComponents(t *testing.T) {
	messages, err := authoring.Compile(authoring.Spec{
		Recipe:          "information",
		SurfaceID:       "rich-preview",
		Title:           "季度报告",
		Subtitle:        "2026 Q3 · 联合复盘",
		Body:            "## 核心结论\n\n指标符合预期。",
		ImageURL:        "https://example.com/cover.png",
		ImageCaption:    "报告封面",
		Metrics:         []authoring.Metric{{Label: "完成率", Value: "96%"}, {Label: "风险", Value: "0"}},
		Highlights:      []authoring.Highlight{{Title: "核心交付", Detail: "已完成验收", Status: "已完成", Theme: "green"}},
		FileName:        "report.pdf",
		FileURL:         "https://example.com/report.pdf",
		FilePreviewURL:  "https://example.com/report/preview",
		FileDescription: "正式版",
		FileMIMEType:    "application/pdf",
		FileSize:        2488320,
		Details:         "### 数据口径\n\n按验收结果计算。",
		DetailURL:       "https://example.com/details",
		PrimaryCTA:      "查看报告",
	})
	if err != nil {
		t.Fatal(err)
	}
	surface, err := state.Reduce(state.Surface{}, messages)
	if err != nil {
		t.Fatal(err)
	}
	html, err := Render(surface)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(html)
	for _, want := range []string{
		`data-component="Column"`, `data-component-id="header_row"`,
		`data-component="Image"`, `class="ImagePreview"`, `aria-label="预览：季度报告"`, `alt="季度报告"`, `data-component="File"`, "report.pdf",
		`class="FileMain" href="https://example.com/report/preview"`, `data-preview-event="file_preview"`,
		`class="FileAction" href="https://example.com/report.pdf"`, `class="FileMeta">正式版`, `class="FileIcon"><img src="https://g.alicdn.com/dingding/card_a2ui/0.1.0/img/ding-file-icons/pdf_light.Dsh5wUh.png"`, `aria-label="打开 report.pdf"`,
		`.FileControl{height:64px;min-height:64px;gap:0;padding:12px;border:.5px solid rgba(24,28,31,.2);border-radius:12px`,
		`.FileIcon{width:40px;height:40px`, `.FileMain{gap:12px;padding:0}`, `.FileName{font-size:14px;line-height:20px`, `.FileMeta{margin-top:2px;color:rgba(20,20,20,.35);font-size:12px;line-height:16px`,
		`.FileAction{width:16px;min-width:16px;height:16px;min-height:16px;margin-left:12px;padding:0`,
		`data-component-id="metric_panel_1"`, `data-component-id="metric_panel_2"`, `data-component-id="metrics"`, "关键指标", "96%", `data-component-id="highlight_panel_1"`, `data-component-id="highlights"`, "核心交付", "已完成验收",
		`data-max-line="2"`, "<blockquote>", "<strong>数据口径与补充说明</strong>", "<h3>数据口径</h3>",
		`data-component="Button"`, "<h2>核心结论</h2>",
		"图片暂不可用", `image.naturalWidth<=1`,
		`--space-2:8px`, `--space-3:12px`, `.Card>.Column{gap:var(--space-2)}`,
		`[data-padding="12"]{padding:var(--space-3)}`,
		`border-width:0.5px`,
		`data-component-id="details_content"`, `data-padding="12"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rich preview missing %q", want)
		}
	}
	if strings.Contains(rendered, `data-component="CollapsiblePanel"`) {
		t.Fatal("information details must remain visible as a quote")
	}
	if strings.Contains(rendered, `data-component-id="detail_link"`) || strings.Contains(rendered, "打开完整详情") {
		t.Fatal("information preview must not duplicate the primary report action")
	}
}

func TestCrossPlatformCoverageRenderMarkdownStructureAndEscaping(t *testing.T) {
	messages, err := authoring.Compile(authoring.Spec{
		Recipe:    "notification",
		SurfaceID: "markdown-preview",
		Title:     "Markdown 预览",
		Body:      "### 变更摘要\n\n- 第一项\n- **第二项**\n\n<script>alert('xss')</script>\n\n[危险链接](javascript:alert(1))",
	})
	if err != nil {
		t.Fatal(err)
	}
	surface, err := state.Reduce(state.Surface{}, messages)
	if err != nil {
		t.Fatal(err)
	}
	html, err := Render(surface)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(html)
	for _, want := range []string{"<h3>变更摘要</h3>", "<ul>", "<li>第一项</li>", "<strong>第二项</strong>"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered Markdown missing %q: %s", want, rendered)
		}
	}
	for _, unsafe := range []string{"<script>alert('xss')</script>", `href="javascript:`} {
		if strings.Contains(rendered, unsafe) {
			t.Fatalf("rendered Markdown contains unsafe output %q: %s", unsafe, rendered)
		}
	}
}

func TestCrossPlatformCoverageRenderMarkdownTableStructureAndResponsiveStyle(t *testing.T) {
	messages, err := authoring.Compile(authoring.Spec{
		Recipe:    "notification",
		SurfaceID: "markdown-table-preview",
		Title:     "运行周报",
		Body:      "| 可用性 | P95 | P1 事件 |\n| :--- | :--- | :--- |\n| **99.96%** | **418ms** | **3** |",
	})
	if err != nil {
		t.Fatal(err)
	}
	surface, err := state.Reduce(state.Surface{}, messages)
	if err != nil {
		t.Fatal(err)
	}
	html, err := Render(surface)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(html)
	for _, want := range []string{
		"<table>", "<thead>", "<tbody>", "<th", ">可用性</th>", "<strong>99.96%</strong>",
		`.Markdown table{width:100%;border-collapse:collapse;table-layout:fixed`,
		`.Markdown th,.Markdown td{padding:8px;border:1px solid #e3e7ec`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered Markdown table missing %q", want)
		}
	}
}

func TestCrossPlatformCoverageRenderRejectsUnsafeImageURL(t *testing.T) {
	messages, err := authoring.Compile(authoring.Spec{
		Recipe:    "information",
		SurfaceID: "unsafe-image-preview",
		Title:     "图片安全验收",
		Body:      "图片地址必须使用受控协议。",
		ImageURL:  "javascript:alert(1)",
	})
	if err != nil {
		t.Fatal(err)
	}
	surface, err := state.Reduce(state.Surface{}, messages)
	if err != nil {
		t.Fatal(err)
	}
	html, err := Render(surface)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(html)
	if strings.Contains(rendered, `src="javascript:`) || strings.Contains(rendered, `src="#ZgotmplZ`) {
		t.Fatalf("unsafe image URL reached rendered src: %s", rendered)
	}
	if !strings.Contains(rendered, `src=""`) {
		t.Fatalf("unsafe image URL must render an empty source for the fallback state")
	}
}

func TestCrossPlatformCoverageRenderHonorsUnboundedPanelAndLargeLineLimit(t *testing.T) {
	surface := state.Surface{
		SurfaceID: "numeric-preview",
		Components: map[string]map[string]any{
			"root":        {"component": "Card", "child": "content"},
			"content":     {"component": "Column", "children": []any{"copy", "details"}},
			"copy":        {"component": "Text", "text": "长文本", "maxLine": 1200.0},
			"details":     {"component": "CollapsiblePanel", "title": "不限高详情", "maxHeight": 0.0, "children": []any{"detail_copy"}},
			"detail_copy": {"component": "Text", "text": "完整内容"},
		},
	}
	html, err := Render(surface)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(html)
	for _, want := range []string{`data-max-line="1200" style="-webkit-line-clamp:1200"`, `data-max-height="0"`} {
		if !strings.Contains(rendered, want) {
			t.Errorf("numeric preview missing %q", want)
		}
	}
	if strings.Contains(rendered, "max-height:0px") {
		t.Fatal("maxHeight 0 means unlimited and must not collapse panel content")
	}
}

func TestCrossPlatformCoverageRenderProjectsPublicBoxModelAndImageVariants(t *testing.T) {
	surface := state.Surface{
		SurfaceID: "visual-contract-preview",
		Components: map[string]map[string]any{
			"root":    {"component": "Card", "child": "content", "padding": 0.0, "borderWidth": 0.0},
			"content": {"component": "Column", "children": []any{"banner", "avatar"}, "gap": 16.0, "padding": 12.0, "backgroundColor": "#FFE8F3FF", "cornerRadius": 8.0, "borderWidth": 1.0, "borderColor": "#FFB7D8FF"},
			"banner":  {"component": "Image", "url": "https://example.com/banner.png", "variant": "header", "fit": "cover"},
			"avatar":  {"component": "Image", "url": "https://example.com/avatar.png", "variant": "avatar", "fit": "cover"},
		},
	}
	html, err := Render(surface)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(html)
	for _, want := range []string{
		`data-component-id="root" data-component="Card"`,
		`style="padding:0px;border-width:0px"`,
		`style="padding:12px;gap:16px;border-radius:8px;border-width:1px;background-color:#E8F3FFFF;border-color:#B7D8FFFF;border-style:solid"`,
		`.ImageFrame.avatar{width:40px;height:40px;border-radius:50%}`,
		`.ImageFrame.header{width:100%;height:200px}`,
		`.ImageFrame.header.contain{background:#fff}`,
		`.ImageFrame>.Image{display:block;width:100%;height:100%;object-fit:cover}`,
		`.Row>.Image{flex:none}`,
		`.Row>.Image.smallFeature{width:96px}`,
		`.Row>[data-weight="1"]{width:0}`,
		`class="ImageFrame header cover"`,
		`class="ImageFrame avatar cover"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("visual contract preview missing %q", want)
		}
	}
	if strings.Contains(rendered, `}.Image{display:block;width:100%;height:100%;object-fit:cover}`) {
		t.Fatal("Image component wrapper must not inherit native image height rules")
	}
}

func TestCrossPlatformCoverageRenderProjectsTextIconColorTypographyAndButtonShape(t *testing.T) {
	surface := state.Surface{
		SurfaceID: "visual-token-preview",
		Components: map[string]map[string]any{
			"root":         {"component": "Card", "child": "content"},
			"content":      {"component": "Column", "children": []any{"title", "custom", "standalone", "action"}},
			"title":        {"component": "Text", "text": "发布完成", "bold": true, "colorToken": "common_green1_color", "sizeToken": "common_h1_text_style__font_size", "icon": map[string]any{"name": "Check_L_outlined"}},
			"custom":       {"component": "Text", "text": "自定义主题色", "customLightColor": "#123456", "customDarkColor": "#ABCDEF"},
			"standalone":   {"component": "Icon", "name": "Search_L_outlined", "color": "blue"},
			"action":       {"component": "Button", "child": "action_label", "variant": "primary", "action": map[string]any{"event": map[string]any{"name": "open", "context": map[string]any{"surfaceId": "visual-token-preview"}}}},
			"action_label": {"component": "Text", "text": "查看详情", "icon": map[string]any{"name": "Search_L_outlined"}},
		},
	}
	html, err := Render(surface)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(html)
	for _, want := range []string{
		`token-common_green1_color`,
		`class="Text custom-color`,
		`style="font-size:18px;-webkit-line-clamp:3"`,
		`<span class="InlineIcon" aria-hidden="true"><svg viewBox="0 0 24 24"`,
		`data-component="Icon"`,
		` blue `,
		`style="--text-light:#123456;--text-dark:#ABCDEF;color:var(--text-light);-webkit-line-clamp:3"`,
		`.InlineIcon{display:inline-flex;width:16px;height:16px`,
		`.Icon>.InlineIcon{width:20px;height:20px`,
		`.ButtonControl{border-radius:999px`,
		`.ButtonControl .Text{color:inherit}`,
		`.Text.custom-color,.Icon.custom-color{color:var(--text-dark)!important}`,
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("visual token preview missing %q", want)
		}
	}
	if strings.Contains(rendered, "✓") || strings.Contains(rendered, "🔍") {
		t.Fatal("reference preview must use the icon system instead of Unicode glyph substitutes")
	}
}

func TestCrossPlatformCoverageRenderCardHeaderIncludesTrailingComponent(t *testing.T) {
	surface := state.Surface{Components: map[string]map[string]any{
		"root":   {"id": "root", "component": "Card", "child": "header"},
		"header": {"id": "header", "component": "CardHeader", "title": "任务提醒", "theme": "blue", "trailing": "status"},
		"status": {"id": "status", "component": "Tag", "text": "进行中", "theme": "orange"},
	}}
	html, err := Render(surface)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`data-component="CardHeader"`, `data-component-id="status"`, "任务提醒", "进行中"} {
		if !strings.Contains(string(html), want) {
			t.Fatalf("preview missing %q", want)
		}
	}
}
