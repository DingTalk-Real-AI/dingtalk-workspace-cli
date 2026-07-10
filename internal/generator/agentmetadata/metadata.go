// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package agentmetadata

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const CurrentVersion = 1

type File struct {
	Version     int                        `json:"version"`
	SourceHash  string                     `json:"source_hash"`
	SurfaceHash string                     `json:"surface_hash,omitempty"`
	Coverage    Coverage                   `json:"coverage"`
	Products    map[string]ProductMetadata `json:"products"`
	Tools       map[string]ToolMetadata    `json:"tools"`
}

type Coverage struct {
	SurfaceProducts      int `json:"surface_products,omitempty"`
	ProductsWithMetadata int `json:"products_with_metadata"`
	SurfaceTools         int `json:"surface_tools,omitempty"`
	ToolsWithMetadata    int `json:"tools_with_metadata"`
	UnmatchedSkillTools  int `json:"unmatched_skill_tools,omitempty"`
}

type ProductMetadata struct {
	UseWhen    []string `json:"use_when,omitempty"`
	AvoidWhen  []string `json:"avoid_when,omitempty"`
	SourceRefs []string `json:"source_refs,omitempty"`
}

type ToolMetadata struct {
	UseWhen      []string `json:"use_when,omitempty"`
	AvoidWhen    []string `json:"avoid_when,omitempty"`
	Effect       string   `json:"effect,omitempty"`
	EffectSource string   `json:"effect_source,omitempty"`
	Risk         string   `json:"risk,omitempty"`
	Confirmation string   `json:"confirmation,omitempty"`
	Examples     []string `json:"examples,omitempty"`
	SourceRefs   []string `json:"source_refs,omitempty"`
}

type Options struct {
	Root             string
	SkillPath        string
	ProductsDir      string
	IntentGuidePath  string
	MaxExamples      int
	ToolPaths        map[string]string
	ProductIDs       map[string]bool
	SurfaceHash      string
	SurfaceToolCount int
}

type Stats struct {
	SourceFiles    int
	Products       int
	Tools          int
	ToolIntents    int
	Examples       int
	RiskRules      int
	UnmatchedTools int
}

type sourceFile struct {
	path    string
	display string
	data    []byte
}

var (
	quotedIntentRE = regexp.MustCompile(`用户(?:提到|说)["“]([^"”]+)["”]`)
	codeSpanRE     = regexp.MustCompile("`([^`]+)`")
	stepCommentRE  = regexp.MustCompile(`(?i)^step\s+[0-9]+\s*[:：]\s*`)
)

func Generate(opts Options) (File, Stats, error) {
	if opts.Root == "" {
		opts.Root = "."
	}
	if opts.MaxExamples <= 0 {
		opts.MaxExamples = 2
	}
	files, err := loadSources(opts)
	if err != nil {
		return File{}, Stats{}, err
	}
	byDisplay := make(map[string]sourceFile, len(files))
	for _, file := range files {
		byDisplay[file.display] = file
	}

	out := File{
		Version:     CurrentVersion,
		SourceHash:  hashSources(files),
		SurfaceHash: strings.TrimSpace(opts.SurfaceHash),
		Products:    map[string]ProductMetadata{},
		Tools:       map[string]ToolMetadata{},
	}
	stats := Stats{SourceFiles: len(files)}

	skillDisplay := displayPath(opts.Root, resolvePath(opts.Root, opts.SkillPath))
	if skill, ok := byDisplay[skillDisplay]; ok {
		parseProductRouting(&out, string(skill.data), skill.display)
		parseDangerRules(&out, string(skill.data), skill.display, &stats)
	}

	intentDisplay := displayPath(opts.Root, resolvePath(opts.Root, opts.IntentGuidePath))
	if guide, ok := byDisplay[intentDisplay]; ok {
		parseIntentGuide(&out, string(guide.data), guide.display)
	}

	productFiles := make([]sourceFile, 0)
	productsRoot := filepath.Clean(resolvePath(opts.Root, opts.ProductsDir)) + string(filepath.Separator)
	for _, file := range files {
		if strings.HasPrefix(filepath.Clean(file.path), productsRoot) && filepath.Dir(file.path) == filepath.Clean(strings.TrimSuffix(productsRoot, string(filepath.Separator))) {
			productFiles = append(productFiles, file)
		}
	}
	sort.Slice(productFiles, func(i, j int) bool { return productFiles[i].display < productFiles[j].display })
	for _, file := range productFiles {
		productID := strings.TrimSuffix(filepath.Base(file.path), filepath.Ext(file.path))
		known := collectCommandPaths(productID, string(file.data))
		parseToolIntents(&out, productID, known, string(file.data), file.display)
		parseExamples(&out, productID, known, string(file.data), file.display, opts.MaxExamples)
	}

	normalizeFile(&out)
	reconcileSurface(&out, opts, &stats)
	stats.Products = len(out.Products)
	stats.Tools = len(out.Tools)
	for _, metadata := range out.Tools {
		stats.ToolIntents += len(metadata.UseWhen)
		stats.Examples += len(metadata.Examples)
	}
	out.Coverage = Coverage{
		SurfaceProducts:      len(opts.ProductIDs),
		ProductsWithMetadata: len(out.Products),
		SurfaceTools:         opts.SurfaceToolCount,
		ToolsWithMetadata:    len(out.Tools),
		UnmatchedSkillTools:  stats.UnmatchedTools,
	}
	return out, stats, nil
}

func reconcileSurface(file *File, opts Options, stats *Stats) {
	if len(opts.ProductIDs) > 0 {
		for productID := range file.Products {
			if !opts.ProductIDs[productID] {
				delete(file.Products, productID)
			}
		}
	}
	if len(opts.ToolPaths) == 0 {
		return
	}
	reconciled := map[string]ToolMetadata{}
	for skillPath, metadata := range file.Tools {
		livePath, ok := opts.ToolPaths[skillPath]
		if !ok {
			stats.UnmatchedTools++
			continue
		}
		existing := reconciled[livePath]
		reconciled[livePath] = mergeToolMetadata(existing, metadata)
	}
	file.Tools = reconciled
}

func mergeToolMetadata(left, right ToolMetadata) ToolMetadata {
	left.UseWhen = append(left.UseWhen, right.UseWhen...)
	left.AvoidWhen = append(left.AvoidWhen, right.AvoidWhen...)
	left.Examples = append(left.Examples, right.Examples...)
	left.SourceRefs = append(left.SourceRefs, right.SourceRefs...)
	if left.Effect == "" || right.EffectSource == "skill-explicit" {
		left.Effect = right.Effect
		left.EffectSource = right.EffectSource
	}
	if left.Risk == "" {
		left.Risk = right.Risk
	}
	if left.Confirmation == "" {
		left.Confirmation = right.Confirmation
	}
	left.UseWhen = normalizedStrings(left.UseWhen)
	left.AvoidWhen = normalizedStrings(left.AvoidWhen)
	left.Examples = normalizedStrings(left.Examples)
	left.SourceRefs = normalizedStrings(left.SourceRefs)
	return left
}

func loadSources(opts Options) ([]sourceFile, error) {
	paths := []string{
		resolvePath(opts.Root, opts.SkillPath),
		resolvePath(opts.Root, opts.IntentGuidePath),
	}
	productPaths, err := filepath.Glob(filepath.Join(resolvePath(opts.Root, opts.ProductsDir), "*.md"))
	if err != nil {
		return nil, fmt.Errorf("glob product references: %w", err)
	}
	paths = append(paths, productPaths...)
	sort.Strings(paths)

	files := make([]sourceFile, 0, len(paths))
	seen := map[string]bool{}
	for _, path := range paths {
		path = filepath.Clean(path)
		if path == "." || seen[path] {
			continue
		}
		seen[path] = true
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		files = append(files, sourceFile{path: path, display: displayPath(opts.Root, path), data: data})
	}
	return files, nil
}

func resolvePath(root, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(root, path))
}

func displayPath(root, path string) string {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(filepath.Clean(path))
	}
	return filepath.ToSlash(rel)
}

func hashSources(files []sourceFile) string {
	h := sha256.New()
	for _, file := range files {
		_, _ = h.Write([]byte(file.display))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(file.data)
		_, _ = h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func parseProductRouting(out *File, body, source string) {
	section := markdownSection(body, "## 意图判断决策树")
	for _, line := range strings.Split(section, "\n") {
		match := quotedIntentRE.FindStringSubmatch(line)
		target := routeCodeTarget(line)
		if len(match) < 2 || target == "" {
			continue
		}
		productID := firstCommandToken(target)
		if productID == "" {
			continue
		}
		addProductUse(out, productID, cleanIntent(match[1]), source)
	}
}

func parseIntentGuide(out *File, body, source string) {
	section := markdownSection(body, "## 易混淆场景快速对照表")
	for _, line := range strings.Split(section, "\n") {
		columns := markdownTableColumns(line)
		if len(columns) < 5 || columns[0] == "用户说..." || strings.Trim(columns[0], "- ") == "" {
			continue
		}
		scenario := cleanIntent(columns[0])
		if intent := cleanIntent(columns[1]); intent != "" && intent != scenario {
			scenario += "；" + intent
		}
		for _, target := range codeSpans(columns[2]) {
			addTargetUse(out, target, scenario, source)
		}
		for _, target := range codeSpans(columns[3]) {
			addTargetAvoid(out, target, scenario, source)
		}
	}
}

func parseToolIntents(out *File, productID string, known []string, body, source string) {
	section := markdownSection(body, "## 意图判断")
	if section == "" {
		return
	}
	currentIntent := ""
	scanner := bufio.NewScanner(strings.NewReader(section))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if match := quotedIntentRE.FindStringSubmatch(line); len(match) > 1 {
			currentIntent = cleanIntent(match[1])
			if strings.Contains(line, "→") || strings.Contains(line, "->") {
				if target := routeCodeTarget(line); target != "" {
					if path := resolveToolPath(productID, target, known); path != "" {
						addToolUse(out, path, currentIntent, source)
					}
				}
				currentIntent = ""
			}
			continue
		}
		if currentIntent == "" || (!strings.Contains(line, "→") && !strings.Contains(line, "->")) {
			continue
		}
		target := routeCodeTarget(line)
		if target == "" {
			continue
		}
		action := strings.TrimSpace(strings.TrimLeft(strings.Split(strings.Split(line, "→")[0], "->")[0], "-* "))
		intent := currentIntent
		if action != "" {
			intent += "；" + action
		}
		if path := resolveToolPath(productID, target, known); path != "" {
			addToolUse(out, path, intent, source)
		}
	}
}

func parseExamples(out *File, productID string, known []string, body, source string, maxExamples int) {
	if len(known) == 0 {
		return
	}
	inFence := false
	shellFence := false
	commentIntent := ""
	for _, rawLine := range strings.Split(body, "\n") {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "```") {
			if inFence {
				inFence = false
				shellFence = false
			} else {
				inFence = true
				shellFence = isShellFence(strings.TrimSpace(strings.TrimPrefix(line, "```")))
			}
			commentIntent = ""
			continue
		}
		if shellFence {
			switch {
			case line == "":
				commentIntent = ""
			case strings.HasPrefix(line, "#"):
				commentIntent = shellCommentIntent(line)
				continue
			}
		}
		if !strings.HasPrefix(line, "dws ") || strings.Contains(line, "[flags]") || strings.HasSuffix(line, "\\") || len(line) > 320 {
			continue
		}
		path := longestKnownPrefix(commandTokens(line), known)
		if path == "" || !strings.HasPrefix(path, productID+" ") {
			continue
		}
		metadata := out.Tools[path]
		if commentIntent != "" {
			metadata.UseWhen = append(metadata.UseWhen, commentIntent)
			metadata.SourceRefs = append(metadata.SourceRefs, source)
		}
		if len(metadata.Examples) < maxExamples {
			metadata.Examples = append(metadata.Examples, line)
			metadata.SourceRefs = append(metadata.SourceRefs, source)
		}
		ensureEffect(&metadata, path)
		applyExplicitCommentSafety(&metadata, commentIntent)
		out.Tools[path] = metadata
	}
}

func isShellFence(language string) bool {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "", "bash", "console", "sh", "shell", "zsh":
		return true
	default:
		return false
	}
}

func shellCommentIntent(line string) string {
	intent := cleanIntent(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#")))
	intent = strings.TrimSpace(stepCommentRE.ReplaceAllString(intent, ""))
	if intent == "" || strings.HasPrefix(intent, "!") || strings.HasPrefix(intent, "→") || len(intent) > 200 {
		return ""
	}
	return intent
}

func applyExplicitCommentSafety(metadata *ToolMetadata, intent string) {
	if metadata == nil || metadata.Effect == "" || metadata.Effect == "read" {
		return
	}
	explicitRisk := false
	for _, marker := range []string{"不可逆", "不可恢复", "不能撤销", "二次确认", "立即失效", "需确认", "需要确认", "高风险", "高影响"} {
		if strings.Contains(intent, marker) {
			explicitRisk = true
			break
		}
	}
	if !explicitRisk {
		return
	}
	metadata.Effect = dangerEffect(intent)
	metadata.EffectSource = "skill-comment"
	metadata.Risk = "high"
	metadata.Confirmation = "user_required"
}

func parseDangerRules(out *File, body, source string, stats *Stats) {
	section := markdownSection(body, "## 危险操作确认")
	for _, line := range strings.Split(section, "\n") {
		columns := markdownTableColumns(line)
		if len(columns) < 3 || columns[0] == "产品" || strings.Trim(columns[0], "- ") == "" {
			continue
		}
		products := codeSpans(columns[0])
		commands := codeSpans(columns[1])
		if len(products) == 0 || len(commands) == 0 {
			continue
		}
		productID := firstCommandToken(products[0])
		for _, command := range commands {
			path := normalizeToolPath(productID, command)
			if path == "" {
				continue
			}
			metadata := out.Tools[path]
			metadata.Effect = dangerEffect(columns[2])
			metadata.EffectSource = "skill-explicit"
			metadata.Risk = "high"
			metadata.Confirmation = "user_required"
			metadata.SourceRefs = append(metadata.SourceRefs, source)
			out.Tools[path] = metadata
			stats.RiskRules++
		}
	}
}

func dangerEffect(description string) string {
	for _, token := range []string{"删除", "撤回", "拒绝", "移除", "替换", "不可逆", "清空"} {
		if strings.Contains(description, token) {
			return "destructive"
		}
	}
	return "write"
}

func collectCommandPaths(productID, body string) []string {
	paths := map[string]bool{}
	for _, line := range strings.Split(body, "\n") {
		candidate := strings.TrimSpace(line)
		if !strings.HasPrefix(candidate, "dws ") || strings.HasSuffix(candidate, "\\") {
			continue
		}
		path := normalizeCommandPath(candidate)
		if strings.HasPrefix(path, productID+" ") {
			paths[path] = true
		}
	}
	out := make([]string, 0, len(paths))
	for path := range paths {
		out = append(out, path)
	}
	sort.Slice(out, func(i, j int) bool {
		leftParts := len(strings.Fields(out[i]))
		rightParts := len(strings.Fields(out[j]))
		if leftParts != rightParts {
			return leftParts > rightParts
		}
		return out[i] < out[j]
	})
	return out
}

func routeCodeTarget(line string) string {
	arrow := strings.Index(line, "→")
	arrowWidth := len("→")
	if asciiArrow := strings.Index(line, "->"); arrow < 0 || (asciiArrow >= 0 && asciiArrow < arrow) {
		arrow = asciiArrow
		arrowWidth = len("->")
	}
	if arrow < 0 {
		return ""
	}
	spans := codeSpans(line[arrow+arrowWidth:])
	if len(spans) == 0 {
		return ""
	}
	return spans[0]
}

func resolveToolPath(productID, raw string, known []string) string {
	candidate := normalizeToolPath(productID, raw)
	if candidate == "" {
		return ""
	}
	for _, path := range known {
		if candidate == path {
			return path
		}
	}
	candidateParts := strings.Fields(candidate)
	if len(candidateParts) > 1 {
		suffix := strings.Join(candidateParts[1:], " ")
		matches := []string{}
		for _, path := range known {
			if path == productID+" "+suffix || strings.HasSuffix(path, " "+suffix) {
				matches = append(matches, path)
			}
		}
		if len(matches) == 1 {
			return matches[0]
		}
	}
	return candidate
}

func normalizeToolPath(productID, raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "dws ")
	path := normalizeCommandPath(raw)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, productID+" ") || path == productID {
		return path
	}
	return strings.TrimSpace(productID + " " + path)
}

func normalizeCommandPath(raw string) string {
	tokens := commandTokens(raw)
	if len(tokens) > 0 && tokens[0] == "dws" {
		tokens = tokens[1:]
	}
	return strings.Join(tokens, " ")
}

func commandTokens(raw string) []string {
	fields := strings.Fields(strings.TrimSpace(raw))
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, "`(),;:")
		if field == "" || strings.HasPrefix(field, "#") || strings.HasPrefix(field, "--") || strings.HasPrefix(field, "<") || strings.HasPrefix(field, "[") || strings.HasPrefix(field, "{") || strings.HasPrefix(field, "\"") || strings.HasPrefix(field, "'") || strings.Contains(field, "|") || strings.Contains(field, "=") {
			break
		}
		out = append(out, field)
	}
	return out
}

func longestKnownPrefix(tokens []string, known []string) string {
	if len(tokens) > 0 && tokens[0] == "dws" {
		tokens = tokens[1:]
	}
	joined := strings.Join(tokens, " ")
	for _, path := range known {
		if joined == path || strings.HasPrefix(joined, path+" ") {
			return path
		}
	}
	return ""
}

func ensureEffect(metadata *ToolMetadata, path string) {
	if metadata.Effect != "" {
		return
	}
	verb := ""
	parts := strings.Fields(path)
	if len(parts) > 0 {
		verb = parts[len(parts)-1]
	}
	if effect := classifyEffectVerb(verb); effect != "" {
		metadata.Effect = effect
		metadata.EffectSource = "command-verb"
	}
}

func classifyEffectVerb(verb string) string {
	verb = strings.ToLower(strings.TrimSpace(verb))
	read := map[string]bool{
		"list": true, "get": true, "search": true, "read": true, "query": true,
		"detail": true, "status": true, "download": true, "export": true,
		"info": true, "summary": true, "check": true, "inspect": true,
		"diagnose": true, "types": true, "records": true, "tasks": true,
		"find": true, "result": true, "resolve": true,
	}
	write := map[string]bool{
		"create": true, "update": true, "delete": true, "send": true, "submit": true,
		"approve": true, "reject": true, "revoke": true, "add": true, "remove": true,
		"insert": true, "upload": true, "move": true, "rename": true, "reply": true,
		"recall": true, "publish": true, "enable": true, "disable": true, "save": true,
		"replace": true, "respond": true, "redirect-task": true, "oa-comments": true,
		"oa-cc-noticer": true, "config": true, "connect": true, "reset": true,
		"start": true, "stop": true, "subscribe": true, "unsubscribe": true,
		"browser-policy": true, "chmod": true,
	}
	if read[verb] || strings.HasPrefix(verb, "list-") || strings.HasPrefix(verb, "get-") || strings.HasPrefix(verb, "query-") {
		return "read"
	}
	if write[verb] || strings.HasPrefix(verb, "create-") || strings.HasPrefix(verb, "update-") || strings.HasPrefix(verb, "delete-") || strings.HasPrefix(verb, "send-") {
		return "write"
	}
	return ""
}

func addTargetUse(out *File, target, intent, source string) {
	target = normalizeCommandPath(target)
	if target == "" || target == "先追问" {
		return
	}
	if len(strings.Fields(target)) == 1 {
		addProductUse(out, target, intent, source)
		return
	}
	addToolUse(out, target, intent, source)
}

func addTargetAvoid(out *File, target, intent, source string) {
	target = normalizeCommandPath(target)
	if target == "" {
		return
	}
	if len(strings.Fields(target)) == 1 {
		metadata := out.Products[target]
		metadata.AvoidWhen = append(metadata.AvoidWhen, intent)
		metadata.SourceRefs = append(metadata.SourceRefs, source)
		out.Products[target] = metadata
		return
	}
	metadata := out.Tools[target]
	metadata.AvoidWhen = append(metadata.AvoidWhen, intent)
	metadata.SourceRefs = append(metadata.SourceRefs, source)
	ensureEffect(&metadata, target)
	out.Tools[target] = metadata
}

func addProductUse(out *File, productID, intent, source string) {
	productID = strings.TrimSpace(productID)
	intent = cleanIntent(intent)
	if productID == "" || intent == "" {
		return
	}
	metadata := out.Products[productID]
	metadata.UseWhen = append(metadata.UseWhen, intent)
	metadata.SourceRefs = append(metadata.SourceRefs, source)
	out.Products[productID] = metadata
}

func addToolUse(out *File, path, intent, source string) {
	path = normalizeCommandPath(path)
	intent = cleanIntent(intent)
	if path == "" || intent == "" {
		return
	}
	metadata := out.Tools[path]
	metadata.UseWhen = append(metadata.UseWhen, intent)
	metadata.SourceRefs = append(metadata.SourceRefs, source)
	ensureEffect(&metadata, path)
	out.Tools[path] = metadata
}

func normalizeFile(file *File) {
	for key, metadata := range file.Products {
		metadata.UseWhen = normalizedStrings(metadata.UseWhen)
		metadata.AvoidWhen = normalizedStrings(metadata.AvoidWhen)
		metadata.SourceRefs = normalizedStrings(metadata.SourceRefs)
		file.Products[key] = metadata
	}
	for key, metadata := range file.Tools {
		ensureEffect(&metadata, key)
		metadata.UseWhen = normalizedStrings(metadata.UseWhen)
		metadata.AvoidWhen = normalizedStrings(metadata.AvoidWhen)
		metadata.Examples = normalizedStrings(metadata.Examples)
		metadata.SourceRefs = normalizedStrings(metadata.SourceRefs)
		file.Tools[key] = metadata
	}
}

func normalizedStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func markdownSection(body, heading string) string {
	start := strings.Index(body, heading)
	if start < 0 {
		return ""
	}
	section := body[start+len(heading):]
	if next := strings.Index(section, "\n## "); next >= 0 {
		section = section[:next]
	}
	return strings.TrimSpace(section)
}

func markdownTableColumns(line string) []string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
		return nil
	}
	raw := strings.Split(strings.Trim(line, "|"), "|")
	out := make([]string, 0, len(raw))
	for _, value := range raw {
		out = append(out, strings.TrimSpace(value))
	}
	return out
}

func codeSpans(value string) []string {
	matches := codeSpanRE.FindAllStringSubmatch(value, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 && strings.TrimSpace(match[1]) != "" {
			out = append(out, strings.TrimSpace(match[1]))
		}
	}
	return out
}

func firstCommandToken(value string) string {
	tokens := commandTokens(value)
	if len(tokens) > 0 && tokens[0] == "dws" {
		tokens = tokens[1:]
	}
	if len(tokens) == 0 {
		return ""
	}
	return tokens[0]
}

func cleanIntent(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"“”' ")
	value = strings.ReplaceAll(value, "**", "")
	value = strings.ReplaceAll(value, "`", "")
	return strings.TrimSpace(value)
}
