// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

// Package authoring converts a small semantic intent into stable A2UI messages.
// It deliberately does not mirror the A2UI component tree: Agents choose an
// information structure here and inspect the generated protocol separately.
package authoring

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/a2ui/protocol"
)

type Recipe struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	UseWhen     []string `json:"useWhen"`
	Blocks      []string `json:"blocks"`
}

// SurfacePolicy is Agent-facing layout guidance. Width is currently a host
// surface capability rather than a public A2UI Card field, so these values
// guide recipe selection and renderer acceptance without leaking private wire
// properties into an A2UI message.
type SurfacePolicy struct {
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	UseWhen              []string `json:"useWhen"`
	MinWidth             int      `json:"minWidth"`
	ObservedHostMinWidth int      `json:"observedHostMinWidth"`
	ObservedHostMaxWidth int      `json:"observedHostMaxWidth"`
	WidthMode            string   `json:"widthMode"`
	NarrowViewport       string   `json:"narrowViewport"`
	WidenWhen            []string `json:"widenWhen,omitempty"`
	Enforcement          string   `json:"enforcement"`
}

type ComponentGuide struct {
	Name        string `json:"name"`
	Role        string `json:"role"`
	UseWhen     string `json:"useWhen"`
	AvoidWhen   string `json:"avoidWhen,omitempty"`
	Reliability string `json:"reliability,omitempty"`
}

// InformationPlanGuidance is design guidance for the Agent, not a protocol
// declaration or a recipe compiler. The reader's actual facts determine the
// number and order of blocks before a built-in Recipe is considered.
type InformationPlanGuidance struct {
	Artifact      InformationPlanArtifact `json:"artifact"`
	Roles         []InformationRole       `json:"roles"`
	GroupingRules []string                `json:"groupingRules"`
	VisualRules   []string                `json:"visualRules"`
	Review        []string                `json:"review"`
}

type InformationPlanArtifact struct {
	RequiredFields    []string `json:"requiredFields"`
	RequiredDecisions []string `json:"requiredDecisions"`
	PriorityValues    []string `json:"priorityValues"`
	Instructions      string   `json:"instructions"`
}

type InformationRole struct {
	Name      string `json:"name"`
	Question  string `json:"question"`
	Treatment string `json:"treatment"`
}

func InformationPlan() InformationPlanGuidance {
	return InformationPlanGuidance{
		Artifact: InformationPlanArtifact{
			RequiredFields:    []string{"fact", "readerNeed", "group", "priority", "treatment", "componentId", "verification"},
			RequiredDecisions: []string{"messageArchetype", "headerDecision"},
			PriorityValues:    []string{"focal", "supporting", "context"},
			Instructions:      "Create one entry for each preserved fact before selecting components. First classify the message's product job from its facts and user intent (for example notification, detail, approval, report, form, or progress), then choose the presentation. For notification-shaped content, prefer an unpainted title row and document any explicitly requested themed full-width header as a cross-platform exception. Classification informs composition but never selects a fixed template by keyword; do not add planning fields to A2UI wire.",
		},
		Roles: []InformationRole{
			{Name: "identity", Question: "What is this and where did it come from?", Treatment: "Subdued source and ID; a short semantic status tag when needed."},
			{Name: "conclusion", Question: "What happened and why does it matter?", Treatment: "One dominant headline and a concise impact statement if the headline does not carry it."},
			{Name: "explanation", Question: "What connects the event to its impact?", Treatment: "A short body line adjacent to the conclusion."},
			{Name: "evidence", Question: "What supports the conclusion?", Treatment: "Group related observations together, with quiet labels and readable values."},
			{Name: "next_step", Question: "What can the reader do?", Treatment: "One distinct action with a real destination or event; keep proposals labeled as proposals."},
			{Name: "context", Question: "What might be useful later?", Treatment: "Level-3 text, a detail link, or collapsed content; remove repetition."},
		},
		GroupingRules: []string{
			"Rank the actual facts before selecting components or a Recipe; a Recipe is an optional implementation shortcut.",
			"Keep cause and impact in the same reading sequence; keep evidence for the same claim in one group; separate evidence from a proposed action.",
			"Use proximity and spacing to show relationships. Do not turn every sentence into an equally prominent box or repeat the conclusion in several blocks.",
		},
		VisualRules: []string{
			"Use one strongest text focal point. Prefer level-1 for the conclusion, level-2 for explanation, and level-3 for source or incidental metadata.",
			"Use a semantic Tag or color for status and risk; reserve local fills for bounded semantic regions. Never rely on color alone to express status.",
			"Muted text must remain readable against its actual surface in both light and dark themes; do not lower contrast just to make metadata quiet.",
			"Avoid repeated high-contrast inline code pills that overpower the meaning. Use supported Tokens and component fields, not invented styles.",
		},
		Review: []string{
			"Was the message archetype inferred from the user's job, event semantics, expected reading behavior, and action needs before any Recipe or component was selected?",
			"Do preview and acceptance widths respect the selected SurfacePolicy? Notification and approval cards start at 360px; do not add 240/280px acceptance views unless diagnosing a separately documented host fallback.",
			"For a notification, alert, or status update, does an unpainted title row identify the message without duplicating the focal event title? If a themed full-width header is required, was it accepted on Mac, mobile, and Web?",
			"In three seconds, can the reader name the event and its impact?",
			"On a second glance, are related evidence and the next step easy to find without rereading the whole card?",
			"At a narrow conversation width and without color, does the grouping remain clear? Check the actual client; lint and structural preview cannot prove visual acceptance.",
		},
	}
}

type CompositionGuidance struct {
	SuggestedPath string `json:"suggestedPath"`
	Reason        string `json:"reason"`
	RecipeMatched bool   `json:"recipeMatched"`
	Recipe        Recipe `json:"recipe"`
}

func RecommendComposition(intent string) CompositionGuidance {
	recipe, matched := RecommendWithMatch(intent)
	if !matched {
		return CompositionGuidance{SuggestedPath: "custom", Reason: "No built-in Recipe intent matched. Build an information plan and compose raw public A2UI components, then use card lint/build.", RecipeMatched: false, Recipe: recipe}
	}
	return CompositionGuidance{SuggestedPath: "review_recipe", Reason: "A Recipe intent matched, but its fixed slots are only a candidate after reviewing the content relationships.", RecipeMatched: true, Recipe: recipe}
}

type Block struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Components  []string     `json:"components"`
	Layout      string       `json:"layout,omitempty"`
	Styles      []BlockStyle `json:"styles,omitempty"`
	Rules       []string     `json:"rules,omitempty"`
	Fallback    string       `json:"fallback,omitempty"`
}

// BlockStyle publishes a reviewed authoring style. Token values containing '*'
// are family patterns for author selection, not literal wire values.
type BlockStyle struct {
	Name                 string  `json:"name"`
	BackgroundColorToken string  `json:"backgroundColorToken"`
	BorderWidth          float64 `json:"borderWidth"`
	BorderColorToken     string  `json:"borderColorToken"`
	Constraint           string  `json:"constraint,omitempty"`
}

type Metric struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Theme string `json:"theme,omitempty"`
}

type Highlight struct {
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
	Status string `json:"status,omitempty"`
	Theme  string `json:"theme,omitempty"`
}

type FormOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type FormField struct {
	ID          string       `json:"id"`
	Label       string       `json:"label"`
	Kind        string       `json:"kind"`
	Placeholder string       `json:"placeholder,omitempty"`
	Options     []FormOption `json:"options,omitempty"`
}

var formFieldIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

type Spec struct {
	Recipe          string      `json:"recipe"`
	SurfaceID       string      `json:"surfaceId"`
	Title           string      `json:"title"`
	Subtitle        string      `json:"subtitle,omitempty"`
	Status          string      `json:"status,omitempty"`
	Body            string      `json:"body"`
	BackgroundColor string      `json:"backgroundColor,omitempty"`
	ImageURL        string      `json:"imageUrl,omitempty"`
	ImageFit        string      `json:"imageFit,omitempty"`
	ImageAlt        string      `json:"imageAlt,omitempty"`
	ImageCaption    string      `json:"imageCaption,omitempty"`
	ScheduleTime    string      `json:"scheduleTime,omitempty"`
	MeetingURL      string      `json:"meetingUrl,omitempty"`
	MeetingState    string      `json:"meetingState,omitempty"`
	MeetingNumber   string      `json:"meetingNumber,omitempty"`
	ActionDisabled  bool        `json:"actionDisabled,omitempty"`
	Metrics         []Metric    `json:"metrics,omitempty"`
	Highlights      []Highlight `json:"highlights,omitempty"`
	FileName        string      `json:"fileName,omitempty"`
	FileURL         string      `json:"fileUrl,omitempty"`
	FilePreviewURL  string      `json:"filePreviewUrl,omitempty"`
	FileDescription string      `json:"fileDescription,omitempty"`
	FileMIMEType    string      `json:"fileMimeType,omitempty"`
	FileSize        float64     `json:"fileSize,omitempty"`
	Details         string      `json:"details,omitempty"`
	DetailURL       string      `json:"detailUrl,omitempty"`
	PrimaryCTA      string      `json:"primaryCta,omitempty"`
	Secondary       string      `json:"secondaryCta,omitempty"`
	Options         []string    `json:"options,omitempty"`
	FormFields      []FormField `json:"formFields,omitempty"`
}

// A2UI layout values follow the public Catalog's four-point spacing rhythm:
// top-level sibling blocks use 8np, while surfaced or nested content uses a
// 12np inset. Keeping these defaults explicit prevents Recipe-specific magic
// numbers from drifting away from the renderer contract.
const (
	spacingTight   = 4.0
	spacingBlock   = 8.0
	spacingContent = 12.0
)

var recipes = []Recipe{
	{Name: "notification", Description: "一件事、一个状态、一个主要动作的通知", UseWhen: []string{"结果通知", "风险告警", "轻量提醒"}, Blocks: []string{"header", "summary", "actions"}},
	{Name: "information", Description: "Structured information and reports with a summary, evidence, and optional attachments", UseWhen: []string{"信息摘要", "内容推荐", "结果报告"}, Blocks: []string{"header", "summary", "hero", "metrics", "list", "attachments", "details", "actions"}},
	{Name: "task", Description: "任务状态、说明和处理动作", UseWhen: []string{"任务分派", "待办跟进"}, Blocks: []string{"compact-notification-header", "facts", "summary", "actions"}},
	{Name: "schedule", Description: "Meeting schedule with time, joining details, and response status", UseWhen: []string{"日程提醒", "会议邀请"}, Blocks: []string{"compact-notification-header", "meeting-title", "meeting-time", "joining-details", "response-status"}},
	{Name: "form", Description: "按字段组收集信息并统一提交的录入卡", UseWhen: []string{"澄清信息", "用户配置", "补充业务参数"}, Blocks: []string{"header", "form", "actions"}},
}

var surfacePolicies = []SurfacePolicy{
	{Name: "notification", Description: "紧凑通知、日程和任务提醒的单列消息面", UseWhen: []string{"notification", "schedule", "task"}, MinWidth: 360, ObservedHostMinWidth: 320, ObservedHostMaxWidth: 640, WidthMode: "content-adaptive", NarrowViewport: "fit-available-width", WidenWhen: []string{"标题或事实值需要多行", "存在两个长操作文案"}, Enforcement: "host-surface-required"},
	{Name: "standard", Description: "Standard single-column surface for information, reports, approvals, and forms", UseWhen: []string{"information", "approval", "form"}, MinWidth: 360, ObservedHostMinWidth: 320, ObservedHostMaxWidth: 640, WidthMode: "content-adaptive", NarrowViewport: "fit-available-width", WidenWhen: []string{"存在宽表格、媒体或较长结构化字段", "内容在 360px 下产生高密度断行"}, Enforcement: "host-surface-required"},
	{Name: "ai", Description: "思考区、流式 Markdown 和反馈操作组成的 AI 消息面", UseWhen: []string{"ai-streaming", "reasoning", "long-form-ai"}, MinWidth: 360, ObservedHostMinWidth: 320, ObservedHostMaxWidth: 640, WidthMode: "content-adaptive", NarrowViewport: "fit-available-width", WidenWhen: []string{"Markdown 含表格、代码块或宽媒体", "长篇输出在窄宽下明显影响扫读"}, Enforcement: "host-surface-required"},
}

var guides = map[string]ComponentGuide{
	"Button":           {Name: "Button", Role: "明确触发一次业务动作", UseWhen: "主要或次要操作", Reliability: "event.name 必须稳定；提交表单时 context 带回绑定值"},
	"File":             {Name: "File", Role: "展示可预览或下载的附件", UseWhen: "结果含文件实体"},
	"Markdown":         {Name: "Markdown", Role: "表达多段、有层次的正文", UseWhen: "摘要、列表、说明", AvoidWhen: "短标签或按钮文案"},
	"TextField":        {Name: "TextField", Role: "收集单行或多行文本", UseWhen: "审批意见、补充说明", Reliability: "绑定 DataModel；由提交按钮统一校验和回传"},
	"Link":             {Name: "Link", Role: "低强调导航或事件入口", UseWhen: "查看详情、帮助"},
	"Card":             {Name: "Card", Role: "提供视觉表面与内边距", UseWhen: "需要独立信息面"},
	"Row":              {Name: "Row", Role: "横向并列少量同级元素", UseWhen: "标签行、双按钮", AvoidWhen: "超过两列或长正文"},
	"Column":           {Name: "Column", Role: "组织主要纵向阅读顺序", UseWhen: "绝大多数卡片根布局"},
	"Tag":              {Name: "Tag", Role: "短状态或分类", UseWhen: "风险、进度、来源", AvoidWhen: "句子或主要动作"},
	"Divider":          {Name: "Divider", Role: "分隔语义区块", UseWhen: "正文与操作区边界"},
	"CollapsiblePanel": {Name: "CollapsiblePanel", Role: "收纳低频明细", UseWhen: "日志、详情、补充说明"},
	"Image":            {Name: "Image", Role: "强化人物、封面或关键视觉", UseWhen: "图片本身携带信息"},
	"Text":             {Name: "Text", Role: "标题、标签和短正文", UseWhen: "一到两行文本"},
	"ChoicePicker":     {Name: "ChoicePicker", Role: "单选或多选结构化输入", UseWhen: "固定选项", Reliability: "RadioButton 语义编译为 mutuallyExclusive + checkbox"},
}

var blocks = []Block{
	{Name: "header", Description: "标题、来源与可选状态入口，默认使用无底色标题行", Components: []string{"Column", "Row", "Text", "Tag"}, Layout: "Column(root insets) > Column(gap=12) > Row(title + optional status) + body", Rules: []string{"内置模板暂不使用带底色的通栏 CardHeader，避免 Mac 气泡边距使头部变成内缩色条", "标题与正文共用根布局边距；只有真实状态才添加右侧 Tag", "局部 content-panel 的背景、描边和圆角不继承到标题行"}},
	{Name: "compact-notification-header", Description: "日程、任务和轻提醒使用的无底色标题行", Components: []string{"Row", "Tag", "Column", "Text"}, Layout: "Column(root insets) > Column(gap=12) > Row(title + optional Tag) + body", Rules: []string{"标题行不使用通栏主题底色", "Tag 只表达短状态，不取代标题", "正文与标题共用根布局边距，避免重复 padding", "空备注、空事实行和无信息增益的 Divider 不进入 children"}},
	{Name: "content-panel", Description: "同一消息卡片内部的内容面板，带背景、描边和内边距；不是另一张消息卡片", Components: []string{"Card", "Column"}, Layout: "Card(padding=12,cornerRadius=8,borderWidth=0.5) > Column(gap=8) > content", Styles: []BlockStyle{{Name: "ordinary", BackgroundColorToken: "common_fg_z1_color", BorderWidth: 0.5, BorderColorToken: "common_line_hard_color"}, {Name: "highlight", BackgroundColorToken: "extended_*0_color", BorderWidth: 0.5, BorderColorToken: "extended_*1_color", Constraint: "背景与边框必须选择同一 extended 色系，并在最终 JSON 中写完整已注册 Token ID"}, {Name: "soft-tint", BackgroundColorToken: "extended_*0_color", BorderWidth: 0, Constraint: "低饱和色面板可不加描边；数字颜色使用相同色系的 extended_*6_color，状态仍需文字表达"}}, Rules: []string{"普通区块使用 backgroundColorToken=common_fg_z1_color、borderWidth=0.5、borderColorToken=common_line_hard_color", "高亮区块使用已注册的 extended_*0_color 作为 backgroundColorToken，并使用同色系 extended_*1_color 作为 borderColorToken，borderWidth=0.5", "Card 只有一个 child；多项内容必须先包在 Column 或 Row 中", "区块容器统一负责背景、描边、padding 和圆角；子内容不使用统一 margin 模拟容器 padding", "根 Body 继承宿主气泡背景；content-panel 只用于需要分组或强调的局部内容", "柔和色块本身已能分组时不要叠加描边；指标卡的不同色相必须对应有根据的状态或类别"}, Fallback: "无法使用 Card 时退化为 Column(padding=12,gap=8)，保留内容层次并移除背景和描边"},
	{Name: "hero", Description: "关键封面或人物视觉", Components: []string{"Image"}},
	{Name: "status", Description: "当前阶段或风险状态", Components: []string{"Tag", "Text"}},
	{Name: "facts", Description: "短标签与值的事实表", Components: []string{"Column", "Row", "Text"}},
	{Name: "summary", Description: "结论优先的正文摘要", Components: []string{"Markdown"}},
	{Name: "metrics", Description: "少量关键指标", Components: []string{"Row", "Column", "Text"}},
	{Name: "list", Description: "同构条目列表", Components: []string{"Column", "Card", "Text"}},
	{Name: "timeline", Description: "按时间或阶段组织的进展", Components: []string{"Column", "Row", "Text", "Tag"}},
	{Name: "image-text-item", Description: "文字主信息与辅助缩略图组成的紧凑图文项", Components: []string{"Row", "Column", "Text", "Image", "Link"}, Layout: "Row(align=center,gap=12) > Column(weight=1) + Image(variant=smallFeature)", Rules: []string{"文字列占据剩余宽度，标题最多 2 行、摘要最多 3 行", "smallFeature 提供 96×96 最大高度，Row 以该高度为基准并让文字列垂直居中", "不得使用 Row.align=stretch 冒充等高；当前公开端会把 stretch 降级为 start", "缩略图是辅助信息，默认关闭大图预览以避免扩大点击边界"}, Fallback: "宿主不能兑现 smallFeature 96×96 几何时移除缩略图，降级为紧凑纯文本项"},
	{Name: "attachments", Description: "文件结果和附件", Components: []string{"File"}},
	{Name: "form", Description: "文本与选择输入", Components: []string{"TextField", "ChoicePicker"}},
	{Name: "actions", Description: "一个主动作及可选次动作", Components: []string{"Row", "Button", "Text"}},
	{Name: "details", Description: "低频补充信息", Components: []string{"CollapsiblePanel", "Markdown", "Link"}},
	{Name: "progress", Description: "生成或执行中的阶段提示", Components: []string{"Tag", "Text"}},
	{Name: "terminal-status", Description: "完成、失败或取消后的稳定终态", Components: []string{"Tag", "Text", "Link"}},
}

func Recipes() []Recipe { return append([]Recipe(nil), recipes...) }

func Blocks() []Block { return append([]Block(nil), blocks...) }

func SurfacePolicies() []SurfacePolicy {
	return append([]SurfacePolicy(nil), surfacePolicies...)
}

func Guides() []ComponentGuide {
	out := make([]ComponentGuide, 0, len(guides))
	for _, guide := range guides {
		out = append(out, guide)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func Guide(name string) (ComponentGuide, bool) { guide, ok := guides[name]; return guide, ok }

func Recommend(intent string) Recipe {
	recipe, _ := RecommendWithMatch(intent)
	return recipe
}

// RecommendWithMatch reports whether the suggestion came from a recognized
// intent. A fallback is not evidence that the content fits a built-in Recipe.
func RecommendWithMatch(intent string) (Recipe, bool) {
	if hasIntentWord(intent, "notification") || strings.Contains(intent, "通知") {
		return recipes[0], true
	}
	for _, candidate := range []string{"approval", "schedule", "task", "form", "report", "information"} {
		if hasIntentWord(intent, candidate) || strings.Contains(intent, map[string]string{"approval": "审批", "schedule": "日程", "task": "任务", "form": "收集", "report": "报告", "information": "信息"}[candidate]) {
			if candidate == "report" {
				candidate = "information"
			} else if candidate == "approval" {
				candidate = "form"
			}
			for _, recipe := range recipes {
				if recipe.Name == candidate {
					return recipe, true
				}
			}
		}
	}
	return recipes[0], false
}

func hasIntentWord(intent, candidate string) bool {
	for _, word := range strings.FieldsFunc(strings.ToLower(intent), func(r rune) bool {
		return r < 'a' || r > 'z'
	}) {
		if word == candidate {
			return true
		}
	}
	return false
}

// ResolveRecipe returns the effective built-in Recipe without mutating the
// caller's CompositionSpec. Commands use it both for compilation and for the
// result envelope so an inferred Recipe is never reported as empty.
func ResolveRecipe(spec Spec) (string, error) {
	recipeName := strings.TrimSpace(spec.Recipe)
	if recipeName == "" {
		recipeName = Recommend(spec.Title + " " + spec.Body).Name
	}
	for _, recipe := range recipes {
		if recipe.Name == recipeName {
			return recipeName, nil
		}
	}
	return "", fmt.Errorf("unknown recipe %q", recipeName)
}

func Compile(spec Spec) ([]map[string]any, error) {
	if strings.TrimSpace(spec.SurfaceID) == "" || strings.TrimSpace(spec.Title) == "" || strings.TrimSpace(spec.Body) == "" {
		return nil, fmt.Errorf("surfaceId, title and body are required")
	}
	if spec.BackgroundColor != "" && spec.BackgroundColor != "#00FFFFFF" {
		return nil, fmt.Errorf("root Card backgroundColor must be #00FFFFFF; put opaque fills on child components")
	}
	effectiveRecipe, err := ResolveRecipe(spec)
	if err != nil {
		return nil, err
	}
	spec.Recipe = effectiveRecipe
	registry, err := protocol.Load()
	if err != nil {
		return nil, err
	}
	if spec.Recipe == "schedule" {
		return compileSchedule(spec, registry)
	}
	status := strings.TrimSpace(spec.Status)
	compactHeader := spec.Recipe == "task"
	quietHeader := compactHeader || spec.Recipe == "notification" || spec.Recipe == "information" || spec.Recipe == "form"
	if compactHeader && status == "" {
		status = "进行中"
	}
	children := []string{}
	contentGap := 0.0
	components := []any{}
	headerPadding := spacingContent
	if quietHeader {
		headerPadding = 0
		contentGap = spacingContent
	}
	header := map[string]any{"id": "header", "component": "Column", "children": []string{"header_row"}, "padding": headerPadding}
	headerChildren := []string{"title"}
	title := map[string]any{"id": "title", "component": "Text", "text": map[string]any{"path": "/content/title"}, "variant": "body", "colorToken": "common_level1_base_color", "bold": true, "maxLine": 1.0}
	if status != "" {
		headerChildren = append(headerChildren, "status")
		title["weight"] = 1.0
	}
	components = append(components,
		header,
		map[string]any{"id": "header_row", "component": "Row", "children": headerChildren, "align": "center", "gap": spacingBlock},
		title,
	)
	if status != "" {
		components = append(components, map[string]any{"id": "status", "component": "Tag", "text": map[string]any{"path": "/content/status"}, "theme": statusTagTheme(status)})
	}
	// A plain Column root receives platform insets from the translator. The Mac
	// message bubble supplies its own inset and strips the translated root inset.
	root := map[string]any{"id": "root", "component": "Card", "child": "content", "padding": 0.0, "cornerRadius": 12.0, "backgroundColor": "#00FFFFFF"}
	if quietHeader {
		root = map[string]any{"id": "root", "component": "Column", "children": []string{"content"}, "align": "stretch", "gap": 0.0}
	}
	components = append([]any{
		root,
		map[string]any{"id": "content", "component": "Column", "children": []string{}, "align": "stretch", "gap": contentGap},
	}, components...)
	data := map[string]any{"content": map[string]any{"title": spec.Title, "subtitle": spec.Subtitle, "status": status, "body": spec.Body}, "form": map[string]any{}}
	if spec.Recipe == "information" {
		children = append(children, "body")
		components = append(components, map[string]any{"id": "body", "component": "Markdown", "content": map[string]any{"path": "/content/body"}})
	}
	if spec.Subtitle != "" {
		children = append(children, "subtitle")
		components = append(components, map[string]any{"id": "subtitle", "component": "Text", "text": map[string]any{"path": "/content/subtitle"}, "variant": "caption", "colorToken": "common_level3_base_color", "maxLine": 3.0})
	}
	if spec.ImageURL != "" {
		children = append(children, "hero")
		imageFit := "cover"
		if spec.ImageFit == "contain" {
			imageFit = "contain"
		}
		components = append(components, map[string]any{"id": "hero", "component": "Image", "url": spec.ImageURL, "fit": imageFit, "variant": "header", "cornerRadius": 10.0, "previewEnabled": true, "accessibility": map[string]any{"label": defaultString(spec.ImageAlt, spec.Title)}})
		if spec.ImageCaption != "" {
			children = append(children, "image_caption")
			components = append(components, map[string]any{"id": "image_caption", "component": "Text", "text": spec.ImageCaption, "variant": "caption", "colorToken": "common_level3_base_color", "maxLine": 3.0})
		}
	}
	if len(spec.Metrics) > 0 {
		children = append(children, "metrics_title", "metrics")
		metricChildren := make([]string, 0, len(spec.Metrics))
		components = append(components, map[string]any{"id": "metrics_title", "component": "Text", "text": "关键指标", "variant": "body", "bold": true})
		for i, metric := range spec.Metrics {
			metricID := fmt.Sprintf("metric_%d", i+1)
			valueID := fmt.Sprintf("metric_value_%d", i+1)
			labelID := fmt.Sprintf("metric_label_%d", i+1)
			if spec.Recipe == "information" {
				panelID := fmt.Sprintf("metric_panel_%d", i+1)
				metricChildren = append(metricChildren, panelID)
				panel := ordinaryContentPanel(panelID, metricID)
				if theme := metricTheme(metric.Theme); theme != "" {
					panel["backgroundColorToken"] = "extended_" + theme + "0_color"
					panel["backgroundColor"] = metricBackground(theme)
					delete(panel, "borderWidth")
					delete(panel, "borderColorToken")
				}
				panel["weight"] = 1.0
				components = append(components, panel)
			} else {
				metricChildren = append(metricChildren, metricID)
			}
			metricColumn := map[string]any{"id": metricID, "component": "Column", "children": []string{valueID, labelID}, "gap": spacingBlock, "weight": 1.0}
			if spec.Recipe != "information" {
				metricColumn["padding"] = spacingContent
				metricColumn["cornerRadius"] = 8.0
			}
			if spec.Recipe == "information" {
				metricColumn["children"] = []string{labelID, valueID}
				metricColumn["gap"] = spacingTight
			}
			value := map[string]any{"id": valueID, "component": "Text", "text": metric.Value, "variant": "body", "bold": true, "maxLine": 2.0}
			if spec.Recipe == "information" {
				value["sizeToken"] = "common_h2_text_style__font_size"
				if theme := metricTheme(metric.Theme); theme != "" {
					value["colorToken"] = "extended_" + theme + "6_color"
					value["customLightColor"] = metricValueColor(theme)
					value["customDarkColor"] = metricDarkValueColor(theme)
				}
			}
			label := map[string]any{"id": labelID, "component": "Text", "text": metric.Label, "variant": "caption", "colorToken": "common_level2_base_color", "maxLine": 2.0}
			if spec.Recipe == "information" {
				label = map[string]any{"id": labelID, "component": "Text", "text": metric.Label, "variant": "body", "colorToken": "common_level2_base_color", "customLightColor": "#687078", "customDarkColor": "#A8B1BA", "maxLine": 2.0}
			}
			components = append(components,
				metricColumn,
				value,
				label,
			)
		}
		components = append(components, map[string]any{"id": "metrics", "component": "Row", "children": metricChildren, "align": "start", "gap": spacingBlock})
	}
	if spec.Recipe != "information" {
		children = append(children, "body")
		components = append(components, map[string]any{"id": "body", "component": "Markdown", "content": map[string]any{"path": "/content/body"}})
	}
	if len(spec.Highlights) > 0 {
		children = append(children, "highlights_title", "highlights")
		highlightChildren := make([]string, 0, len(spec.Highlights))
		components = append(components, map[string]any{"id": "highlights_title", "component": "Text", "text": "重点进展", "variant": "body", "bold": true})
		for i, highlight := range spec.Highlights {
			rowID := fmt.Sprintf("highlight_%d", i+1)
			copyID := fmt.Sprintf("highlight_copy_%d", i+1)
			titleID := fmt.Sprintf("highlight_title_%d", i+1)
			detailID := fmt.Sprintf("highlight_detail_%d", i+1)
			tagID := fmt.Sprintf("highlight_status_%d", i+1)
			copyChildren := []string{titleID}
			if spec.Recipe == "information" {
				panelID := fmt.Sprintf("highlight_panel_%d", i+1)
				highlightChildren = append(highlightChildren, panelID)
				components = append(components, ordinaryContentPanel(panelID, rowID))
			} else {
				highlightChildren = append(highlightChildren, rowID)
			}
			components = append(components, map[string]any{"id": titleID, "component": "Text", "text": highlight.Title, "variant": "body", "bold": true, "maxLine": 2.0})
			if highlight.Detail != "" {
				copyChildren = append(copyChildren, detailID)
				components = append(components, map[string]any{"id": detailID, "component": "Text", "text": highlight.Detail, "variant": "caption", "colorToken": "common_level2_base_color", "maxLine": 3.0})
			}
			components = append(components, map[string]any{"id": copyID, "component": "Column", "children": copyChildren, "gap": spacingTight, "weight": 1.0})
			rowChildren := []string{copyID}
			if highlight.Status != "" {
				rowChildren = append(rowChildren, tagID)
				components = append(components, map[string]any{"id": tagID, "component": "Tag", "text": highlight.Status, "theme": validTagTheme(highlight.Theme), "variant": "filled"})
			}
			row := map[string]any{"id": rowID, "component": "Row", "children": rowChildren, "align": "center", "gap": spacingBlock, "padding": spacingContent, "cornerRadius": 8.0}
			if spec.Recipe == "information" {
				delete(row, "padding")
				delete(row, "cornerRadius")
			}
			components = append(components, row)
		}
		components = append(components, map[string]any{"id": "highlights", "component": "Column", "children": highlightChildren, "gap": spacingBlock})
	}
	if spec.FileURL != "" {
		children = append(children, "divider_attachment", "attachment_title", "file")
		attachmentTitle := "交付产物"
		if spec.Recipe == "information" {
			attachmentTitle = "结果产物"
		}
		file := map[string]any{"id": "file", "component": "File", "fileName": defaultString(spec.FileName, "附件"), "url": spec.FileURL, "description": defaultString(spec.FileDescription, "相关附件"), "onPreview": eventAction("file_preview", spec.SurfaceID)}
		if spec.FilePreviewURL != "" {
			file["previewUrl"] = spec.FilePreviewURL
		}
		if spec.FileMIMEType != "" {
			file["mimeType"] = spec.FileMIMEType
		}
		if spec.FileSize > 0 {
			file["size"] = spec.FileSize
		}
		components = append(components,
			map[string]any{"id": "divider_attachment", "component": "Divider", "axis": "horizontal", "borderColorToken": "common_line_light_color"},
			map[string]any{"id": "attachment_title", "component": "Text", "text": attachmentTitle, "variant": "body", "bold": true},
			file,
		)
	}
	showDetailLink := spec.DetailURL != "" && spec.Recipe != "task" && !(spec.Recipe == "information" && spec.PrimaryCTA != "")
	if spec.Recipe == "task" && spec.Details != "" {
		children = append(children, "task_criteria")
		components = append(components, map[string]any{"id": "task_criteria", "component": "Markdown", "content": spec.Details})
	}
	if spec.Recipe != "task" && (showDetailLink || spec.Details != "") {
		children = append(children, "details")
		detailChildren := []string{}
		if spec.Details != "" {
			detailChildren = append(detailChildren, "details_body")
			content := spec.Details
			if spec.Recipe == "information" {
				content = quoteDetails("数据口径与补充说明", content)
			}
			components = append(components, map[string]any{"id": "details_body", "component": "Markdown", "content": content})
		}
		if showDetailLink {
			detailChildren = append(detailChildren, "detail_link")
			components = append(components, map[string]any{"id": "detail_link", "component": "Link", "text": "打开完整详情", "action": map[string]any{"functionCall": map[string]any{"call": "openUrl", "args": map[string]any{"url": spec.DetailURL}}}})
		}
		components = append(components, map[string]any{"id": "details_content", "component": "Column", "children": detailChildren, "align": "stretch", "gap": spacingBlock, "padding": spacingContent})
		if spec.Recipe == "information" {
			components = append(components, map[string]any{"id": "details", "component": "Column", "children": []string{"details_content"}, "align": "stretch"})
		} else {
			components = append(components, map[string]any{"id": "details", "component": "CollapsiblePanel", "title": "数据口径与补充说明", "variant": "indented", "defaultExpanded": false, "maxHeight": 240.0, "children": []string{"details_content"}, "fallbackMarkdown": "### 数据口径与补充说明\n\n请打开完整详情查看。"})
		}
	}
	if spec.Recipe == "form" {
		options := spec.Options
		if len(options) == 0 {
			options = []string{"选项一", "选项二"}
		}
		optionValues := make([]any, 0, len(options))
		for i, option := range options {
			optionValues = append(optionValues, map[string]any{"label": option, "value": fmt.Sprintf("option_%d", i+1)})
		}
		if len(spec.FormFields) > 0 {
			fieldChildren := make([]string, 0, len(spec.FormFields))
			formValues := make(map[string]any, len(spec.FormFields))
			for _, field := range spec.FormFields {
				if !formFieldIDPattern.MatchString(field.ID) || strings.TrimSpace(field.Label) == "" {
					return nil, fmt.Errorf("form field requires a stable id and label")
				}
				if _, exists := formValues[field.ID]; exists {
					return nil, fmt.Errorf("duplicate form field id %q", field.ID)
				}
				id := "field_" + field.ID
				component := map[string]any{"id": id, "value": map[string]any{"path": "/form/" + field.ID}}
				switch field.Kind {
				case "text", "longText":
					fieldChildren = append(fieldChildren, id)
					component["component"] = "TextField"
					component["label"] = field.Label
					component["variant"] = "shortText"
					if field.Kind == "longText" {
						component["variant"] = "longText"
					}
					if field.Placeholder != "" {
						component["placeholder"] = field.Placeholder
					}
					formValues[field.ID] = ""
				case "checkbox", "dropdown", "radio":
					labelID := id + "_label"
					groupID := id + "_group"
					fieldChildren = append(fieldChildren, groupID)
					components = append(components,
						map[string]any{"id": labelID, "component": "Text", "text": field.Label, "variant": "body", "bold": true},
						map[string]any{"id": groupID, "component": "Column", "children": []string{labelID, id}, "align": "stretch", "gap": spacingBlock},
					)
					if len(field.Options) == 0 {
						return nil, fmt.Errorf("choice field %q requires options", field.ID)
					}
					choices := make([]any, 0, len(field.Options))
					seen := map[string]bool{}
					for _, option := range field.Options {
						if strings.TrimSpace(option.Label) == "" || strings.TrimSpace(option.Value) == "" || seen[option.Value] {
							return nil, fmt.Errorf("choice field %q requires unique non-empty labels and values", field.ID)
						}
						seen[option.Value] = true
						choices = append(choices, map[string]any{"label": option.Label, "value": option.Value})
					}
					component["component"] = "ChoicePicker"
					component["options"] = choices
					component["variant"] = "mutuallyExclusive"
					component["displayStyle"] = "checkbox"
					if field.Kind == "checkbox" {
						component["variant"] = "multipleSelection"
					} else if field.Kind == "dropdown" {
						component["displayStyle"] = "dropdown"
						component["label"] = "请选择" + field.Label
					}
					formValues[field.ID] = []string{}
				default:
					return nil, fmt.Errorf("unsupported form field kind %q", field.Kind)
				}
				components = append(components, component)
			}
			children = append(children, "form_fields")
			components = append(components, map[string]any{"id": "form_fields", "component": "Column", "children": fieldChildren, "align": "stretch", "gap": 16.0})
			data["form"] = formValues
		} else {
			children = append(children, "form_intro", "form_fields")
			components = append(components,
				map[string]any{"id": "form_intro", "component": "Text", "text": "请完成以下信息", "variant": "body", "bold": true},
				map[string]any{"id": "form_fields", "component": "Column", "children": []string{"comment", "choice"}, "align": "stretch", "gap": spacingContent, "cornerRadius": 8.0},
				map[string]any{"id": "comment", "component": "TextField", "label": "补充信息", "value": map[string]any{"path": "/form/comment"}, "placeholder": "请输入 Agent 继续处理所需的信息", "variant": "longText"},
				map[string]any{"id": "choice", "component": "ChoicePicker", "label": "执行方式", "variant": "mutuallyExclusive", "displayStyle": "checkbox", "options": optionValues, "value": map[string]any{"path": "/form/choice"}},
			)
		}
		if len(spec.FormFields) == 0 {
			data["form"] = map[string]any{"comment": "", "choice": []string{}}
		}
	}
	if spec.PrimaryCTA != "" || spec.Secondary != "" {
		children = append(children, "divider_actions", "actions")
		actionChildren := []string{}
		components = append(components, map[string]any{"id": "divider_actions", "component": "Divider", "axis": "horizontal"})
		if spec.Secondary != "" {
			actionChildren = append(actionChildren, "secondary_button")
			secondaryVariant := "default"
			if spec.Recipe == "form" {
				secondaryVariant = "borderless"
			}
			components = append(components,
				map[string]any{"id": "secondary_label", "component": "Text", "text": spec.Secondary, "variant": "body"},
				map[string]any{"id": "secondary_button", "component": "Button", "child": "secondary_label", "variant": secondaryVariant, "weight": 1.0, "action": eventAction(actionEventName(spec.Recipe, false), spec.SurfaceID)},
			)
		}
		if spec.PrimaryCTA != "" {
			actionChildren = append(actionChildren, "primary_button")
			components = append(components,
				map[string]any{"id": "primary_label", "component": "Text", "text": spec.PrimaryCTA, "variant": "body"},
				map[string]any{"id": "primary_button", "component": "Button", "child": "primary_label", "variant": "primary", "weight": 1.0, "action": eventAction(actionEventName(spec.Recipe, true), spec.SurfaceID)},
			)
		}
		components = append(components, map[string]any{"id": "actions", "component": "Row", "children": actionChildren, "align": "center", "gap": spacingBlock})
	}
	if spec.Recipe != "information" && len(children) >= 2 && children[len(children)-2] == "divider_actions" && children[len(children)-1] == "actions" {
		children = append(children[:len(children)-2], "actions_footer")
		components = append(components, map[string]any{"id": "actions_footer", "component": "Column", "children": []string{"divider_actions", "actions"}, "align": "stretch", "gap": spacingContent, "padding": 4.0})
	}
	if compactHeader {
		components = append(components, map[string]any{"id": "compact_body", "component": "Column", "children": children, "align": "stretch", "gap": spacingBlock})
		components[1].(map[string]any)["children"] = []string{"header", "compact_body"}
	} else {
		bodyGap := spacingBlock
		if spec.Recipe == "information" {
			var sections []any
			children, sections = groupInformationBody(children)
			components = append(components, sections...)
			bodyGap = 16.0
		}
		bodyContent := map[string]any{"id": "body_content", "component": "Column", "children": children, "align": "stretch", "gap": bodyGap, "padding": spacingContent}
		if quietHeader {
			delete(bodyContent, "padding")
		}
		components = append(components, bodyContent)
		components[1].(map[string]any)["children"] = []string{"header", "body_content"}
	}
	messages := []map[string]any{
		{"version": "v1.0", "createSurface": map[string]any{"surfaceId": spec.SurfaceID, "catalogId": registry.Catalog().CatalogID, "sendDataModel": true}},
		{"version": "v1.0", "updateDataModel": map[string]any{"surfaceId": spec.SurfaceID, "path": "/", "value": data}},
		{"version": "v1.0", "updateComponents": map[string]any{"surfaceId": spec.SurfaceID, "components": components}},
	}
	validation := registry.Validate(messages, "create")
	if !validation.Valid {
		return nil, fmt.Errorf("generated recipe failed validation: %+v", validation.Diagnostics)
	}
	return messages, nil
}

func groupInformationBody(children []string) ([]string, []any) {
	sectionByChild := map[string]string{
		"body": "summary_section", "subtitle": "summary_section", "hero": "summary_section", "image_caption": "summary_section",
		"metrics_title": "metrics_section", "metrics": "metrics_section",
		"highlights_title": "highlights_section", "highlights": "highlights_section",
		"divider_attachment": "attachment_section", "attachment_title": "attachment_section", "file": "attachment_section",
		"divider_actions": "actions_section", "actions": "actions_section",
	}
	grouped := make([]string, 0, len(children))
	sections := make([]any, 0, 5)
	for i := 0; i < len(children); {
		sectionID := sectionByChild[children[i]]
		if sectionID == "" {
			grouped = append(grouped, children[i])
			i++
			continue
		}
		sectionChildren := []string{}
		for i < len(children) && sectionByChild[children[i]] == sectionID {
			sectionChildren = append(sectionChildren, children[i])
			i++
		}
		grouped = append(grouped, sectionID)
		section := map[string]any{"id": sectionID, "component": "Column", "children": sectionChildren, "align": "stretch", "gap": spacingBlock}
		if sectionID == "actions_section" {
			section["gap"] = spacingContent
			section["padding"] = 4.0
		}
		sections = append(sections, section)
	}
	return grouped, sections
}

func ordinaryContentPanel(id, child string) map[string]any {
	return map[string]any{
		"id": id, "component": "Card", "child": child,
		"padding": spacingContent, "cornerRadius": 8.0,
		"backgroundColorToken": "common_fg_z1_color",
		"borderWidth":          0.5, "borderColorToken": "common_line_hard_color",
	}
}

func metricTheme(theme string) string {
	switch theme {
	case "green", "blue", "orange":
		return theme
	default:
		return ""
	}
}

func metricBackground(theme string) string {
	switch theme {
	case "green":
		return "#E0F4E6"
	case "blue":
		return "#E0F0FF"
	case "orange":
		return "#FFF2E0"
	default:
		return ""
	}
}

func metricValueColor(theme string) string {
	switch theme {
	case "green":
		return "#008A2A"
	case "blue":
		return "#006ACC"
	case "orange":
		return "#B85C00"
	default:
		return ""
	}
}

func metricDarkValueColor(theme string) string {
	switch theme {
	case "green":
		return "#3EAA5F"
	case "blue":
		return "#3E91E4"
	case "orange":
		return "#E39D3E"
	default:
		return ""
	}
}

func compileSchedule(spec Spec, registry *protocol.Registry) ([]map[string]any, error) {
	children := []string{"meeting_title"}
	components := []any{
		map[string]any{"id": "root", "component": "Column", "children": []string{"content"}, "align": "stretch", "gap": 0.0},
		map[string]any{"id": "content", "component": "Column", "children": []string{"header", "schedule_body"}, "align": "stretch", "gap": spacingContent},
		map[string]any{"id": "header", "component": "Text", "text": map[string]any{"path": "/content/title"}, "variant": "body", "colorToken": "extended_green6_color", "bold": true, "maxLine": 1.0},
		map[string]any{"id": "meeting_title", "component": "Text", "text": map[string]any{"path": "/content/body"}, "variant": "body", "sizeToken": "common_h2_text_style__font_size", "colorToken": "common_level1_base_color", "bold": true},
	}
	if spec.ScheduleTime != "" {
		children = append(children, "meeting_time_row")
		components = append(components,
			map[string]any{"id": "meeting_time_row", "component": "Row", "children": []string{"time_icon", "meeting_time"}, "align": "center", "gap": 0.0},
			map[string]any{"id": "time_icon", "component": "Text", "text": "", "variant": "body", "colorToken": "common_level1_base_color", "customLightColor": "#1F2329", "customDarkColor": "#FFFFFF", "icon": map[string]any{"name": "icon_time_line"}},
			map[string]any{"id": "meeting_time", "component": "Text", "text": spec.ScheduleTime, "variant": "body", "maxLine": 2.0},
		)
	}
	if spec.MeetingURL != "" || spec.MeetingState != "" || spec.MeetingNumber != "" {
		children = append(children, "meeting_join_row")
		joinChildren := []string{"meeting_join"}
		join := map[string]any{"id": "meeting_join", "component": "Text", "text": "钉钉视频会议", "variant": "body", "colorToken": "extended_blue6_color"}
		if spec.MeetingURL != "" {
			join = map[string]any{"id": "meeting_join", "component": "Link", "text": "加入钉钉视频会议", "action": map[string]any{"functionCall": map[string]any{"call": "openUrl", "args": map[string]any{"url": spec.MeetingURL}}}}
		}
		components = append(components,
			map[string]any{"id": "meeting_join_row", "component": "Row", "children": []string{"meeting_icon", "meeting_join_details"}, "align": "start", "gap": 0.0},
			map[string]any{"id": "meeting_icon", "component": "Text", "text": "", "variant": "body", "colorToken": "extended_blue6_color", "customLightColor": "#007FFF", "customDarkColor": "#007FFF", "icon": map[string]any{"name": "icon_videoon_line"}},
			join,
		)
		parts := []string{}
		if spec.MeetingState != "" {
			parts = append(parts, spec.MeetingState)
		}
		if spec.MeetingNumber != "" {
			parts = append(parts, "会议号："+spec.MeetingNumber)
		}
		if len(parts) > 0 {
			joinChildren = append(joinChildren, "meeting_meta")
			components = append(components, map[string]any{"id": "meeting_meta", "component": "Text", "text": "● " + strings.Join(parts, " | "), "variant": "caption", "colorToken": "common_level3_base_color", "maxLine": 2.0})
		}
		components = append(components, map[string]any{"id": "meeting_join_details", "component": "Column", "children": joinChildren, "align": "stretch", "gap": spacingTight, "weight": 1.0})
	}
	if spec.PrimaryCTA != "" {
		children = append(children, "schedule_actions")
		buttonVariant := "primary"
		if spec.ActionDisabled {
			buttonVariant = "default"
		}
		components = append(components,
			map[string]any{"id": "schedule_actions", "component": "Row", "children": []string{"primary_button"}, "align": "stretch"},
			map[string]any{"id": "primary_label", "component": "Text", "text": spec.PrimaryCTA, "variant": "body"},
			map[string]any{"id": "primary_button", "component": "Button", "child": "primary_label", "variant": buttonVariant, "weight": 1.0, "disabled": spec.ActionDisabled, "action": eventAction("schedule_accept", spec.SurfaceID)},
		)
	}
	components = append(components, map[string]any{"id": "schedule_body", "component": "Column", "children": children, "align": "stretch", "gap": spacingContent})
	messages := []map[string]any{
		{"version": "v1.0", "createSurface": map[string]any{"surfaceId": spec.SurfaceID, "catalogId": registry.Catalog().CatalogID, "sendDataModel": true}},
		{"version": "v1.0", "updateDataModel": map[string]any{"surfaceId": spec.SurfaceID, "path": "/", "value": map[string]any{"content": map[string]any{"title": spec.Title, "body": spec.Body}, "form": map[string]any{}}}},
		{"version": "v1.0", "updateComponents": map[string]any{"surfaceId": spec.SurfaceID, "components": components}},
	}
	if validation := registry.Validate(messages, "create"); !validation.Valid {
		return nil, fmt.Errorf("generated schedule failed validation: %+v", validation.Diagnostics)
	}
	return messages, nil
}

func quoteDetails(title, body string) string {
	lines := []string{"> **" + title + "**", ">"}
	for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
		if line == "" {
			lines = append(lines, ">")
		} else {
			lines = append(lines, "> "+line)
		}
	}
	return strings.Join(lines, "\n")
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func validTagTheme(value string) string {
	switch value {
	case "black", "gray", "red", "orange", "green", "blue":
		return value
	default:
		return "blue"
	}
}

func statusTagTheme(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	switch {
	case containsAny(lower, "incomplete", "unsuccessful", "not complete", "not completed", "not successful", "failed", "failure", "error"),
		containsAny(value, "未完成", "未通过", "不通过", "失败", "异常", "风险"):
		return "red"
	case containsAny(lower, "complete", "success", "passed"), containsAny(value, "已完成", "成功", "已通过"):
		return "green"
	case containsAny(lower, "pending", "waiting", "in progress"), containsAny(value, "待", "进行"):
		return "orange"
	default:
		return "blue"
	}
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func eventAction(name, surfaceID string) map[string]any {
	return map[string]any{"event": map[string]any{"name": name, "context": map[string]any{"surfaceId": surfaceID, "form": map[string]any{"path": "/form"}}}}
}

func actionEventName(recipe string, primary bool) string {
	if primary {
		switch recipe {
		case "form":
			return "form_submit"
		default:
			return "primary"
		}
	}
	switch recipe {
	case "form":
		return "form_cancel"
	default:
		return "secondary"
	}
}
