// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package render implements a deterministic, network-free semantic SVG
// preview for DWS OpenNodes V1.
package render

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard/opennodes"
)

const (
	Version          = "opennodes-svg-v1"
	DefaultPadding   = 24.0
	maximumNodeCount = 5000
	maximumSVGBytes  = 8 * 1024 * 1024
)

type Bounds struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type Warning struct {
	Code     string `json:"code"`
	NodeID   string `json:"nodeId,omitempty"`
	NodeType string `json:"nodeType,omitempty"`
	Message  string `json:"message"`
	Fidelity string `json:"fidelity"`
}

type Result struct {
	SVG              []byte
	Bounds           Bounds
	Fidelity         string
	NodeCount        int
	RenderedCount    int
	ExactCount       int
	ApproximateCount int
	PlaceholderCount int
	Warnings         []Warning
}

type renderer struct {
	source    *opennodes.Source
	items     []*item
	byID      map[string]*item
	warnings  []Warning
	exact     int
	approx    int
	placehold int
}

type item struct {
	index       int
	node        map[string]any
	id          string
	nodeType    string
	x           float64
	y           float64
	width       float64
	height      float64
	worldX      float64
	worldY      float64
	worldReady  bool
	worldBusy   bool
	hidden      bool
	placeholder bool
}

type point struct{ x, y float64 }

var (
	colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{3}([0-9a-fA-F]{3})?([0-9a-fA-F]{2})?$`)
	pathPattern  = regexp.MustCompile(`^[MmLlHhVvCcSsQqTtAaZz0-9+eE., \t\r\n-]+$`)
)

func SVG(source *opennodes.Source) (*Result, error) {
	if source == nil {
		return nil, errors.New("OpenNodes source is nil")
	}
	if len(source.Nodes) > maximumNodeCount {
		return nil, fmt.Errorf("source.nodes contains %d nodes; maximum is %d", len(source.Nodes), maximumNodeCount)
	}
	r := &renderer{source: source, byID: make(map[string]*item, len(source.Nodes))}
	r.index()
	for _, current := range r.items {
		r.worldPosition(current)
	}
	bounds := r.sceneBounds()

	var body bytes.Buffer
	body.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" role="img" aria-label="DWS OpenNodes semantic preview" viewBox="`)
	body.WriteString(number(bounds.X) + " " + number(bounds.Y) + " " + number(bounds.Width) + " " + number(bounds.Height))
	body.WriteString(`" width="` + number(bounds.Width) + `" height="` + number(bounds.Height) + `">` + "\n")
	body.WriteString("  <title>DWS OpenNodes semantic preview</title>\n")
	body.WriteString("  <desc>Approximate local preview. Final DingTalk whiteboard rendering may differ.</desc>\n")
	body.WriteString("  <defs>\n")
	body.WriteString("    <marker id=\"arrow-filled\" markerWidth=\"10\" markerHeight=\"10\" refX=\"9\" refY=\"3\" orient=\"auto\" markerUnits=\"strokeWidth\"><path d=\"M0,0 L0,6 L9,3 z\" fill=\"context-stroke\"/></marker>\n")
	body.WriteString("    <marker id=\"arrow-open\" markerWidth=\"10\" markerHeight=\"10\" refX=\"9\" refY=\"3\" orient=\"auto\" markerUnits=\"strokeWidth\"><path d=\"M0,0 L9,3 L0,6\" fill=\"none\" stroke=\"context-stroke\"/></marker>\n")
	body.WriteString("  </defs>\n")
	body.WriteString("  <rect x=\"")
	body.WriteString(number(bounds.X) + `" y="` + number(bounds.Y) + `" width="` + number(bounds.Width) + `" height="` + number(bounds.Height) + `" fill="#ffffff"/>` + "\n")
	for _, current := range r.items {
		if current.hidden {
			continue
		}
		body.WriteString(r.renderItem(current))
		if body.Len() > maximumSVGBytes {
			return nil, fmt.Errorf("rendered SVG exceeds %d bytes", maximumSVGBytes)
		}
	}
	body.WriteString("</svg>\n")

	fidelity := "exact"
	if r.placehold > 0 {
		fidelity = "placeholder"
	} else if r.approx > 0 {
		fidelity = "approximate"
	}
	return &Result{
		SVG: body.Bytes(), Bounds: bounds, Fidelity: fidelity,
		NodeCount: len(source.Nodes), RenderedCount: r.exact + r.approx + r.placehold,
		ExactCount: r.exact, ApproximateCount: r.approx, PlaceholderCount: r.placehold,
		Warnings: r.warnings,
	}, nil
}

func (r *renderer) index() {
	for index, node := range r.source.Nodes {
		current := &item{index: index, node: node}
		current.id, _ = stringValue(node["id"])
		current.nodeType, _ = stringValue(node["type"])
		current.nodeType = strings.ToLower(current.nodeType)
		current.x, _ = finiteNumber(node["x"])
		current.y, _ = finiteNumber(node["y"])
		current.width, _ = positiveNumber(node["width"])
		current.height, _ = positiveNumber(node["height"])
		current.hidden, _ = node["hidden"].(bool)
		applyDefaultSize(current)
		r.items = append(r.items, current)
		if current.id == "" {
			r.warn(current, "missing_node_id", "节点缺少稳定 id；预览使用输入顺序定位", "placeholder")
			current.placeholder = true
			continue
		}
		if _, exists := r.byID[current.id]; exists {
			r.warn(current, "duplicate_node_id", "节点 id 重复；parent/connector 引用可能不准确", "placeholder")
			current.placeholder = true
			continue
		}
		r.byID[current.id] = current
	}
}

func applyDefaultSize(current *item) {
	if current.width > 0 && current.height > 0 {
		return
	}
	switch current.nodeType {
	case "text":
		if current.width <= 0 {
			current.width = 240
		}
		if current.height <= 0 {
			current.height = 48
		}
	case "icon":
		if current.width <= 0 {
			current.width = 48
		}
		if current.height <= 0 {
			current.height = 48
		}
	default:
		if current.width <= 0 {
			current.width = 160
		}
		if current.height <= 0 {
			current.height = 80
		}
	}
}

func (r *renderer) worldPosition(current *item) (float64, float64) {
	if current.worldReady {
		return current.worldX, current.worldY
	}
	if current.worldBusy {
		r.warn(current, "parent_cycle", "parentId 形成循环；使用节点自身坐标", "placeholder")
		return current.x, current.y
	}
	current.worldBusy = true
	current.worldX, current.worldY = current.x, current.y
	if parentID, ok := stringValue(current.node["parentId"]); ok {
		if parent, exists := r.byID[parentID]; exists {
			if parent.nodeType == "frame" || parent.nodeType == "group" {
				px, py := r.worldPosition(parent)
				current.worldX += px
				current.worldY += py
			} else {
				r.warn(current, "invalid_parent_type", "parentId 仅支持引用 frame/group；使用节点自身坐标", "approximate")
			}
		} else {
			r.warn(current, "missing_parent", "parentId 未指向本次 source 中的节点；使用节点自身坐标", "approximate")
		}
	}
	current.worldBusy = false
	current.worldReady = true
	return current.worldX, current.worldY
}

func (r *renderer) sceneBounds() Bounds {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	add := func(x, y float64) {
		if !isFinite(x) || !isFinite(y) {
			return
		}
		minX, minY = math.Min(minX, x), math.Min(minY, y)
		maxX, maxY = math.Max(maxX, x), math.Max(maxY, y)
	}
	for _, current := range r.items {
		if current.hidden {
			continue
		}
		if current.nodeType == "connector" {
			points, _ := r.connectorPoints(current)
			for _, p := range points {
				add(p.x, p.y)
			}
			continue
		}
		add(current.worldX, current.worldY)
		add(current.worldX+current.width, current.worldY+current.height)
	}
	if math.IsInf(minX, 1) {
		return Bounds{X: 0, Y: 0, Width: 640, Height: 360}
	}
	return Bounds{
		X: minX - DefaultPadding, Y: minY - DefaultPadding,
		Width:  math.Max(1, maxX-minX+2*DefaultPadding),
		Height: math.Max(1, maxY-minY+2*DefaultPadding),
	}
}

func (r *renderer) renderItem(current *item) string {
	if current.placeholder {
		return r.placeholder(current, "节点身份无效，无法安全解析引用")
	}
	switch current.nodeType {
	case "shape", "sticky":
		return r.renderShape(current)
	case "text":
		return r.renderTextNode(current)
	case "frame":
		return r.renderFrame(current)
	case "group":
		return r.renderGroup(current)
	case "connector":
		return r.renderConnector(current)
	case "path":
		return r.renderPath(current)
	case "icon":
		return r.renderIcon(current)
	case "image", "vector":
		return r.placeholder(current, current.nodeType+" 资源在本地预览中不访问网络")
	default:
		return r.placeholder(current, "不支持的节点类型 "+printableType(current.nodeType))
	}
}

func (r *renderer) renderShape(current *item) string {
	geometry, _ := stringValue(current.node["geometry"])
	fill, stroke, strokeWidth, styleApproximate := style(current.node, "#f8fafc", "#64748b", 1.5)
	if current.nodeType == "sticky" && fill == "#f8fafc" {
		fill = "#fef3c7"
	}
	var element string
	switch geometry {
	case "", "dml:rect":
		element = rect(current, 0, fill, stroke, strokeWidth, "")
		r.exact++
	case "dml:roundRect":
		element = rect(current, math.Min(16, math.Min(current.width, current.height)/5), fill, stroke, strokeWidth, "")
		r.approx++
		r.warn(current, "approximate_geometry", "roundRect 圆角使用本地近似值", "approximate")
	case "dml:diamond":
		cx, cy := current.worldX+current.width/2, current.worldY+current.height/2
		points := fmt.Sprintf("%s,%s %s,%s %s,%s %s,%s", number(cx), number(current.worldY), number(current.worldX+current.width), number(cy), number(cx), number(current.worldY+current.height), number(current.worldX), number(cy))
		element = fmt.Sprintf("  <polygon points=\"%s\" fill=\"%s\" stroke=\"%s\" stroke-width=\"%s\"%s/>\n", points, fill, stroke, number(strokeWidth), rotation(current))
		r.approx++
	default:
		return r.placeholder(current, "暂不支持 geometry "+geometry)
	}
	if styleApproximate {
		if r.exact > 0 && geometry != "dml:roundRect" && geometry != "dml:diamond" {
			r.exact--
			r.approx++
		}
		r.warn(current, "style_approximate", "渐变、效果或不安全样式未原样进入 SVG，已使用安全近似值", "approximate")
	}
	if len(textLines(current.node["text"])) > 0 {
		if geometry != "dml:roundRect" && geometry != "dml:diamond" {
			r.exact--
			r.approx++
		}
		r.warn(current, "font_metrics_approximate", "SVG 字体度量和换行可能与钉钉白板不同", "approximate")
	}
	return element + r.nodeText(current, current.node["text"])
}

func (r *renderer) renderTextNode(current *item) string {
	r.approx++
	r.warn(current, "font_metrics_approximate", "SVG 字体度量和换行可能与钉钉白板不同", "approximate")
	return r.nodeText(current, current.node["text"])
}

func (r *renderer) renderFrame(current *item) string {
	r.exact++
	result := rect(current, 0, "#ffffff", "#94a3b8", 1.5, ` stroke-dasharray="8 5"`)
	title := current.node["title"]
	if object, ok := title.(map[string]any); ok {
		title = object["text"]
	}
	if len(textLines(title)) > 0 {
		r.exact--
		r.approx++
		r.warn(current, "font_metrics_approximate", "Frame 标题字体度量可能与钉钉白板不同", "approximate")
	}
	return result + r.nodeTextAt(current, title, current.worldX+12, current.worldY+22, "start")
}

func (r *renderer) renderGroup(current *item) string {
	r.approx++
	r.warn(current, "group_bounds_approximate", "group 边界按本地节点尺寸显示，最终白板可能重新规范化", "approximate")
	return rect(current, 0, "none", "#cbd5e1", 1, ` stroke-dasharray="4 4"`)
}

func (r *renderer) renderConnector(current *item) string {
	points, ok := r.connectorPoints(current)
	if !ok || len(points) < 2 {
		return r.placeholder(current, "connector 端点无法在本地解析")
	}
	_, stroke, strokeWidth, styleApproximate := style(current.node, "none", "#475569", 2)
	markerStart, markerStartApproximate := markerAttribute(current.node["start"], "marker-start")
	markerEnd, markerEndApproximate := markerAttribute(current.node["end"], "marker-end")
	routing, _ := stringValue(current.node["routing"])
	var element string
	if routing == "curve" && len(points) == 2 {
		midX := (points[0].x + points[1].x) / 2
		d := fmt.Sprintf("M %s %s C %s %s, %s %s, %s %s", number(points[0].x), number(points[0].y), number(midX), number(points[0].y), number(midX), number(points[1].y), number(points[1].x), number(points[1].y))
		element = fmt.Sprintf("  <path d=\"%s\" fill=\"none\" stroke=\"%s\" stroke-width=\"%s\"%s%s/>\n", d, stroke, number(strokeWidth), markerStart, markerEnd)
		r.approx++
		r.warn(current, "connector_routing_approximate", "curve connector 使用本地贝塞尔近似路径", "approximate")
	} else {
		if routing == "orthogonal" && len(points) == 2 {
			midX := (points[0].x + points[1].x) / 2
			points = []point{points[0], {x: midX, y: points[0].y}, {x: midX, y: points[1].y}, points[1]}
			r.approx++
			r.warn(current, "connector_routing_approximate", "orthogonal connector 未执行钉钉避障算法", "approximate")
		} else {
			r.exact++
		}
		parts := make([]string, len(points))
		for index, p := range points {
			parts[index] = number(p.x) + "," + number(p.y)
		}
		element = fmt.Sprintf("  <polyline points=\"%s\" fill=\"none\" stroke=\"%s\" stroke-width=\"%s\"%s%s/>\n", strings.Join(parts, " "), stroke, number(strokeWidth), markerStart, markerEnd)
	}
	if styleApproximate || markerStartApproximate || markerEndApproximate {
		if r.exact > 0 && routing != "curve" && routing != "orthogonal" {
			r.exact--
			r.approx++
		}
		r.warn(current, "connector_style_approximate", "connector 的样式或 marker 使用了安全近似值", "approximate")
	}
	return element
}

func (r *renderer) renderPath(current *item) string {
	pathObject, _ := current.node["path"].(map[string]any)
	data, _ := stringValue(pathObject["data"])
	if data == "" || !pathPattern.MatchString(data) {
		return r.placeholder(current, "path.data 缺失或包含不安全字符")
	}
	intrinsicWidth, okWidth := positiveNumber(pathObject["intrinsicWidth"])
	intrinsicHeight, okHeight := positiveNumber(pathObject["intrinsicHeight"])
	if !okWidth || !okHeight {
		return r.placeholder(current, "path 缺少正数 intrinsicWidth/intrinsicHeight")
	}
	fill, stroke, strokeWidth, _ := style(current.node, "none", "#475569", 2)
	transform := fmt.Sprintf("translate(%s %s) scale(%s %s)", number(current.worldX), number(current.worldY), number(current.width/intrinsicWidth), number(current.height/intrinsicHeight))
	r.approx++
	r.warn(current, "path_approximate", "path 使用本地 SVG 缩放，笔触和最终渲染可能不同", "approximate")
	return fmt.Sprintf("  <path d=\"%s\" transform=\"%s\" fill=\"%s\" stroke=\"%s\" stroke-width=\"%s\"/>\n", data, transform, fill, stroke, number(strokeWidth))
}

func (r *renderer) renderIcon(current *item) string {
	catalogID, _ := stringValue(current.node["catalogId"])
	r.approx++
	switch catalogID {
	case "task/task-done":
		cx, cy := current.worldX+current.width/2, current.worldY+current.height/2
		radius := math.Min(current.width, current.height) / 2
		return fmt.Sprintf("  <circle cx=\"%s\" cy=\"%s\" r=\"%s\" fill=\"#dcfce7\" stroke=\"#16a34a\"/><path d=\"M %s %s L %s %s L %s %s\" fill=\"none\" stroke=\"#16a34a\" stroke-width=\"3\"/>\n", number(cx), number(cy), number(radius), number(current.worldX+current.width*.24), number(current.worldY+current.height*.52), number(current.worldX+current.width*.43), number(current.worldY+current.height*.7), number(current.worldX+current.width*.78), number(current.worldY+current.height*.3))
	case "task/task":
		return rect(current, 4, "#ffffff", "#64748b", 2, "")
	default:
		return r.placeholderAlreadyCounted(current, "暂不支持 icon catalogId "+catalogID)
	}
}

func (r *renderer) placeholder(current *item, message string) string {
	r.placehold++
	r.warn(current, "placeholder_rendered", message, "placeholder")
	return placeholderSVG(current, message)
}

func (r *renderer) placeholderAlreadyCounted(current *item, message string) string {
	r.approx--
	return r.placeholder(current, message)
}

func placeholderSVG(current *item, message string) string {
	label := printableType(current.nodeType)
	if current.id != "" {
		label += " · " + current.id
	}
	return rect(current, 0, "#f8fafc", "#ef4444", 1.5, ` stroke-dasharray="6 4"`) +
		fmt.Sprintf("  <text x=\"%s\" y=\"%s\" fill=\"#991b1b\" font-family=\"system-ui,-apple-system,sans-serif\" font-size=\"12\"><tspan>%s</tspan><tspan x=\"%s\" dy=\"18\">%s</tspan></text>\n", number(current.worldX+8), number(current.worldY+20), escape(label), number(current.worldX+8), escape(truncate(message, 80)))
}

func (r *renderer) nodeText(current *item, value any) string {
	return r.nodeTextAt(current, value, current.worldX+current.width/2, current.worldY+current.height/2, "middle")
}

func (r *renderer) nodeTextAt(current *item, value any, x, y float64, anchor string) string {
	lines := textLines(value)
	if len(lines) == 0 {
		return ""
	}
	fontSize := 14.0
	if styleObject, ok := current.node["style"].(map[string]any); ok {
		if size, ok := positiveNumber(styleObject["fontSize"]); ok && size <= 256 {
			fontSize = size
		}
	}
	lineHeight := fontSize * 1.25
	startY := y - float64(len(lines)-1)*lineHeight/2
	var result strings.Builder
	result.WriteString(fmt.Sprintf("  <text x=\"%s\" y=\"%s\" text-anchor=\"%s\" dominant-baseline=\"middle\" fill=\"#0f172a\" font-family=\"system-ui,-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif\" font-size=\"%s\"%s>\n", number(x), number(startY), anchor, number(fontSize), rotation(current)))
	for index, line := range lines {
		dy := "0"
		if index > 0 {
			dy = number(lineHeight)
		}
		result.WriteString(fmt.Sprintf("    <tspan x=\"%s\" dy=\"%s\">%s</tspan>\n", number(x), dy, escape(line)))
	}
	result.WriteString("  </text>\n")
	return result.String()
}

func (r *renderer) connectorPoints(current *item) ([]point, bool) {
	start, okStart := r.endpoint(current.node["start"])
	end, okEnd := r.endpoint(current.node["end"])
	if !okStart || !okEnd {
		return nil, false
	}
	points := []point{start}
	if waypoints, ok := current.node["waypoints"].([]any); ok {
		for _, raw := range waypoints {
			p, valid := pointValue(raw)
			if !valid {
				return nil, false
			}
			points = append(points, p)
		}
	}
	points = append(points, end)
	return points, true
}

func (r *renderer) endpoint(value any) (point, bool) {
	endpoint, ok := value.(map[string]any)
	if !ok {
		return point{}, false
	}
	kind, _ := stringValue(endpoint["type"])
	if kind == "point" {
		return pointValue(endpoint["point"])
	}
	if kind != "node" {
		return point{}, false
	}
	ref, _ := endpoint["nodeRef"].(map[string]any)
	id, _ := stringValue(ref["id"])
	target := r.byID[id]
	if target == nil {
		return point{}, false
	}
	x, y := target.worldX+target.width/2, target.worldY+target.height/2
	anchor, _ := endpoint["anchor"].(map[string]any)
	side, _ := stringValue(anchor["side"])
	switch side {
	case "top":
		y = target.worldY
	case "bottom":
		y = target.worldY + target.height
	case "left":
		x = target.worldX
	case "right":
		x = target.worldX + target.width
	}
	return point{x: x, y: y}, true
}

func (r *renderer) warn(current *item, code, message, fidelity string) {
	r.warnings = append(r.warnings, Warning{Code: code, NodeID: current.id, NodeType: current.nodeType, Message: message, Fidelity: fidelity})
}

func style(node map[string]any, defaultFill, defaultStroke string, defaultWidth float64) (string, string, float64, bool) {
	fill, stroke, width := defaultFill, defaultStroke, defaultWidth
	approximate := false
	styleObject, _ := node["style"].(map[string]any)
	if fillObject, ok := styleObject["fill"].(map[string]any); ok {
		fillType, _ := stringValue(fillObject["type"])
		if fillType == "none" {
			fill = "none"
		} else if fillType == "solid" || fillType == "" {
			if color, ok := safeColor(fillObject["color"]); ok {
				fill = color
			} else if _, exists := fillObject["color"]; exists {
				approximate = true
			}
		} else {
			approximate = true
		}
	}
	if strokeObject, ok := styleObject["stroke"].(map[string]any); ok {
		if paint, ok := strokeObject["paint"].(map[string]any); ok {
			paintType, _ := stringValue(paint["type"])
			if paintType == "solid" || paintType == "" {
				if color, ok := safeColor(paint["color"]); ok {
					stroke = color
				} else if _, exists := paint["color"]; exists {
					approximate = true
				}
			} else {
				approximate = true
			}
		}
		if value, ok := positiveNumber(strokeObject["width"]); ok && value <= 128 {
			width = value
		} else if _, exists := strokeObject["width"]; exists {
			approximate = true
		}
	}
	if _, exists := styleObject["effects"]; exists {
		approximate = true
	}
	return fill, stroke, width, approximate
}

func markerAttribute(value any, name string) (string, bool) {
	endpoint, _ := value.(map[string]any)
	marker, _ := endpoint["marker"].(map[string]any)
	catalogID, _ := stringValue(marker["catalogId"])
	switch catalogID {
	case "arrow.filled":
		return ` ` + name + `="url(#arrow-filled)"`, false
	case "arrow.open":
		return ` ` + name + `="url(#arrow-open)"`, false
	case "", "none":
		return "", false
	default:
		return "", true
	}
}

func rect(current *item, radius float64, fill, stroke string, strokeWidth float64, extra string) string {
	return fmt.Sprintf("  <rect x=\"%s\" y=\"%s\" width=\"%s\" height=\"%s\" rx=\"%s\" fill=\"%s\" stroke=\"%s\" stroke-width=\"%s\"%s%s/>\n", number(current.worldX), number(current.worldY), number(current.width), number(current.height), number(radius), fill, stroke, number(strokeWidth), extra, rotation(current))
}

func rotation(current *item) string {
	angle, ok := finiteNumber(current.node["angle"])
	if !ok || angle == 0 {
		return ""
	}
	cx, cy := current.worldX+current.width/2, current.worldY+current.height/2
	return fmt.Sprintf(` transform="rotate(%s %s %s)"`, number(angle), number(cx), number(cy))
}

func textLines(value any) []string {
	switch current := value.(type) {
	case string:
		return splitLines(current)
	case map[string]any:
		if nested, exists := current["text"]; exists {
			return textLines(nested)
		}
		blocks, _ := current["blocks"].([]any)
		var lines []string
		for _, rawBlock := range blocks {
			block, _ := rawBlock.(map[string]any)
			runs, _ := block["runs"].([]any)
			var line strings.Builder
			for _, rawRun := range runs {
				run, _ := rawRun.(map[string]any)
				text, _ := stringValue(run["text"])
				line.WriteString(text)
			}
			lines = append(lines, line.String())
		}
		return lines
	default:
		return nil
	}
}

func splitLines(value string) []string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return strings.Split(value, "\n")
}

func pointValue(value any) (point, bool) {
	object, ok := value.(map[string]any)
	if !ok {
		return point{}, false
	}
	x, okX := finiteNumber(object["x"])
	y, okY := finiteNumber(object["y"])
	return point{x: x, y: y}, okX && okY
}

func stringValue(value any) (string, bool) {
	text, ok := value.(string)
	text = strings.TrimSpace(text)
	return text, ok && text != ""
}

func finiteNumber(value any) (float64, bool) {
	var result float64
	var err error
	switch current := value.(type) {
	case json.Number:
		result, err = current.Float64()
	case float64:
		result = current
	case float32:
		result = float64(current)
	case int:
		result = float64(current)
	case int64:
		result = float64(current)
	default:
		return 0, false
	}
	return result, err == nil && isFinite(result)
}

func positiveNumber(value any) (float64, bool) {
	result, ok := finiteNumber(value)
	return result, ok && result > 0
}

func safeColor(value any) (string, bool) {
	color, ok := stringValue(value)
	return color, ok && colorPattern.MatchString(color)
}

func isFinite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func number(value float64) string {
	if value == 0 {
		return "0"
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func escape(value string) string { return html.EscapeString(value) }

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit-1]) + "…"
}

func printableType(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

// SortedWarningCodes is useful to callers that need a compact stable summary.
func SortedWarningCodes(warnings []Warning) []string {
	seen := make(map[string]struct{}, len(warnings))
	for _, warning := range warnings {
		seen[warning.Code] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for code := range seen {
		result = append(result, code)
	}
	sort.Strings(result)
	return result
}
