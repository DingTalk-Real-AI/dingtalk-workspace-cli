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
	"io/fs"
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
	ToolsWithSummary     int `json:"tools_with_agent_summary,omitempty"`
	UnmatchedSkillTools  int `json:"unmatched_skill_tools,omitempty"`
}

type ProductMetadata struct {
	AgentSummary       string   `json:"agent_summary,omitempty"`
	AgentSummarySource string   `json:"agent_summary_source,omitempty"`
	UseWhen            []string `json:"use_when,omitempty"`
	AvoidWhen          []string `json:"avoid_when,omitempty"`
	SourceRefs         []string `json:"source_refs,omitempty"`
}

// InterfaceRef links a stable public command to the MCP operation that
// implements it. It is interface identity, not runtime endpoint discovery.
type InterfaceRef struct {
	ProductID string `json:"product_id"`
	RPCName   string `json:"rpc_name"`
}

type ToolMetadata struct {
	AgentSummary       string        `json:"agent_summary,omitempty"`
	AgentSummarySource string        `json:"agent_summary_source,omitempty"`
	UseWhen            []string      `json:"use_when,omitempty"`
	AvoidWhen          []string      `json:"avoid_when,omitempty"`
	Prerequisites      []string      `json:"prerequisites,omitempty"`
	Tips               []string      `json:"tips,omitempty"`
	Effect             string        `json:"effect,omitempty"`
	EffectSource       string        `json:"effect_source,omitempty"`
	Risk               string        `json:"risk,omitempty"`
	Confirmation       string        `json:"confirmation,omitempty"`
	Idempotency        string        `json:"idempotency,omitempty"`
	WorkflowRefs       []string      `json:"workflow_refs,omitempty"`
	Examples           []string      `json:"examples,omitempty"`
	Reviewed           *bool         `json:"reviewed,omitempty"`
	SourceRefs         []string      `json:"source_refs,omitempty"`
	InterfaceRef       *InterfaceRef `json:"interface_ref,omitempty"`
}

type Options struct {
	Root                     string
	SkillPath                string
	ProductsDir              string
	IntentGuidePath          string
	HintsDir                 string
	InterfaceMetadataPath    string
	MaxExamples              int
	MaxInterfaceSummaryRunes int
	ToolPaths                map[string]string
	ProductIDs               map[string]bool
	SurfaceHash              string
	SurfaceToolCount         int
}

type Stats struct {
	SourceFiles                   int
	Products                      int
	Tools                         int
	ToolIntents                   int
	Examples                      int
	RiskRules                     int
	HintFiles                     int
	HintProducts                  int
	HintTools                     int
	InterfaceMetadata             *InterfaceMetadataAudit
	UnmatchedTools                int
	SourceProducts                []string
	SkillProductsOutsideSurface   []string
	SurfaceProductsWithoutRouting []string
	UnmatchedReferences           []UnmatchedReference
}

// UnmatchedReference identifies one Skill command reference that cannot be
// resolved against the versioned command surface. It is emitted only in the
// build-time audit and is not embedded in the runtime Agent schema.
type UnmatchedReference struct {
	ToolPath   string   `json:"tool_path"`
	Source     string   `json:"source,omitempty"`
	Line       int      `json:"line,omitempty"`
	Candidates []string `json:"candidates,omitempty"`
}

// Audit contains build-time diagnostics that are intentionally kept separate
// from the runtime Agent metadata contract.
type Audit struct {
	Version                       int                     `json:"version"`
	SourceHash                    string                  `json:"source_hash"`
	SurfaceHash                   string                  `json:"surface_hash,omitempty"`
	SourceFiles                   int                     `json:"source_files"`
	HintFiles                     int                     `json:"hint_files,omitempty"`
	HintProducts                  int                     `json:"hint_products,omitempty"`
	HintTools                     int                     `json:"hint_tools,omitempty"`
	InterfaceMetadata             *InterfaceMetadataAudit `json:"interface_metadata,omitempty"`
	Coverage                      Coverage                `json:"coverage"`
	SourceProducts                []string                `json:"source_products,omitempty"`
	SkillProductsOutsideSurface   []string                `json:"skill_products_outside_surface,omitempty"`
	SurfaceProductsWithoutRouting []string                `json:"surface_products_without_routing_metadata,omitempty"`
	UnmatchedReferences           []UnmatchedReference    `json:"unmatched_references,omitempty"`
}

type sourceFile struct {
	path    string
	display string
	data    []byte
}

type commandReference struct {
	text          string
	line          int
	commentIntent string
}

type sourceLocation struct {
	source string
	line   int
}

type sourceTracker map[string]map[string]sourceLocation

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
	origins := sourceTracker{}

	skillDisplay := displayPath(opts.Root, resolvePath(opts.Root, opts.SkillPath))
	if skill, ok := byDisplay[skillDisplay]; ok {
		parseProductRouting(&out, string(skill.data), skill.display)
		parseDangerRules(&out, string(skill.data), skill.display, &stats, origins)
	}

	intentDisplay := displayPath(opts.Root, resolvePath(opts.Root, opts.IntentGuidePath))
	if guide, ok := byDisplay[intentDisplay]; ok {
		parseIntentGuide(&out, string(guide.data), guide.display, origins)
	}

	productFiles := make([]sourceFile, 0)
	productsRoot := filepath.Clean(resolvePath(opts.Root, opts.ProductsDir))
	productsPrefix := productsRoot + string(filepath.Separator)
	for _, file := range files {
		if strings.HasPrefix(filepath.Clean(file.path), productsPrefix) {
			productFiles = append(productFiles, file)
		}
	}
	sort.Slice(productFiles, func(i, j int) bool { return productFiles[i].display < productFiles[j].display })
	sourceProducts := map[string]bool{}
	for _, file := range productFiles {
		body := string(file.data)
		references := collectCommandReferences(body)
		productIDs := sourceProductIDs(file, productsRoot, references, opts.ProductIDs)
		for _, productID := range productIDs {
			sourceProducts[productID] = true
		}
		known := collectCommandPaths(productIDs, references)
		parseToolIntents(&out, productIDs, known, body, file.display, origins)
		parseExamples(&out, known, references, file.display, opts.MaxExamples, origins)
	}
	if err := parseHintSources(&out, files, opts, &stats, origins); err != nil {
		return File{}, Stats{}, err
	}
	if err := applyInterfaceMetadataFallback(&out, byDisplay, opts, &stats, origins); err != nil {
		return File{}, Stats{}, err
	}

	normalizeFile(&out, opts.MaxExamples)
	for productID := range out.Products {
		sourceProducts[productID] = true
	}
	for toolPath := range out.Tools {
		if productID := metadataProductID(toolPath); productID != "" {
			sourceProducts[productID] = true
		}
	}
	stats.SourceProducts = sortedBoolKeys(sourceProducts)
	if len(opts.ProductIDs) > 0 {
		for _, productID := range stats.SourceProducts {
			if !opts.ProductIDs[productID] {
				stats.SkillProductsOutsideSurface = append(stats.SkillProductsOutsideSurface, productID)
			}
		}
	}
	reconcileSurface(&out, opts, &stats, origins)
	normalizeFile(&out, opts.MaxExamples)
	if len(opts.ProductIDs) > 0 {
		for productID := range opts.ProductIDs {
			if _, ok := out.Products[productID]; !ok {
				stats.SurfaceProductsWithoutRouting = append(stats.SurfaceProductsWithoutRouting, productID)
			}
		}
		sort.Strings(stats.SurfaceProductsWithoutRouting)
	}
	stats.Products = len(out.Products)
	stats.Tools = len(out.Tools)
	toolsWithSummary := 0
	for _, metadata := range out.Tools {
		stats.ToolIntents += len(metadata.UseWhen)
		stats.Examples += len(metadata.Examples)
		if strings.TrimSpace(metadata.AgentSummary) != "" {
			toolsWithSummary++
		}
	}
	out.Coverage = Coverage{
		SurfaceProducts:      len(opts.ProductIDs),
		ProductsWithMetadata: len(out.Products),
		SurfaceTools:         opts.SurfaceToolCount,
		ToolsWithMetadata:    len(out.Tools),
		ToolsWithSummary:     toolsWithSummary,
		UnmatchedSkillTools:  stats.UnmatchedTools,
	}
	return out, stats, nil
}

func BuildAudit(file File, stats Stats) Audit {
	return Audit{
		Version:                       CurrentVersion,
		SourceHash:                    file.SourceHash,
		SurfaceHash:                   file.SurfaceHash,
		SourceFiles:                   stats.SourceFiles,
		HintFiles:                     stats.HintFiles,
		HintProducts:                  stats.HintProducts,
		HintTools:                     stats.HintTools,
		InterfaceMetadata:             stats.InterfaceMetadata,
		Coverage:                      file.Coverage,
		SourceProducts:                append([]string(nil), stats.SourceProducts...),
		SkillProductsOutsideSurface:   append([]string(nil), stats.SkillProductsOutsideSurface...),
		SurfaceProductsWithoutRouting: append([]string(nil), stats.SurfaceProductsWithoutRouting...),
		UnmatchedReferences:           append([]UnmatchedReference(nil), stats.UnmatchedReferences...),
	}
}

func reconcileSurface(file *File, opts Options, stats *Stats, origins sourceTracker) {
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
	sourcePaths := make([]string, 0, len(file.Tools))
	for skillPath := range file.Tools {
		sourcePaths = append(sourcePaths, skillPath)
	}
	sort.Strings(sourcePaths)
	for _, skillPath := range sourcePaths {
		metadata := file.Tools[skillPath]
		livePath, ok := opts.ToolPaths[skillPath]
		if !ok {
			stats.UnmatchedTools++
			locations := origins.locations(skillPath)
			if len(locations) == 0 {
				locations = []sourceLocation{{}}
			}
			candidates := candidateToolPaths(skillPath, opts.ToolPaths, 3)
			for _, location := range locations {
				stats.UnmatchedReferences = append(stats.UnmatchedReferences, UnmatchedReference{
					ToolPath:   skillPath,
					Source:     location.source,
					Line:       location.line,
					Candidates: append([]string(nil), candidates...),
				})
			}
			continue
		}
		existing := reconciled[livePath]
		reconciled[livePath] = mergeToolMetadata(existing, metadata)
	}
	file.Tools = reconciled
	sort.Slice(stats.UnmatchedReferences, func(i, j int) bool {
		left, right := stats.UnmatchedReferences[i], stats.UnmatchedReferences[j]
		if left.Source != right.Source {
			return left.Source < right.Source
		}
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		return left.ToolPath < right.ToolPath
	})
}

func mergeToolMetadata(left, right ToolMetadata) ToolMetadata {
	if left.AgentSummary == "" {
		left.AgentSummary = right.AgentSummary
		left.AgentSummarySource = right.AgentSummarySource
	}
	left.UseWhen = append(left.UseWhen, right.UseWhen...)
	left.AvoidWhen = append(left.AvoidWhen, right.AvoidWhen...)
	left.Prerequisites = append(left.Prerequisites, right.Prerequisites...)
	left.Tips = append(left.Tips, right.Tips...)
	left.WorkflowRefs = append(left.WorkflowRefs, right.WorkflowRefs...)
	left.Examples = append(left.Examples, right.Examples...)
	left.SourceRefs = append(left.SourceRefs, right.SourceRefs...)
	if left.Effect == "" || effectSourceRank(right.EffectSource) > effectSourceRank(left.EffectSource) {
		left.Effect = right.Effect
		left.EffectSource = right.EffectSource
	}
	if left.Risk == "" {
		left.Risk = right.Risk
	}
	if left.Confirmation == "" {
		left.Confirmation = right.Confirmation
	}
	if left.Idempotency == "" {
		left.Idempotency = right.Idempotency
	}
	if left.Reviewed == nil {
		left.Reviewed = right.Reviewed
	}
	if left.InterfaceRef == nil {
		left.InterfaceRef = right.InterfaceRef
	}
	left.UseWhen = normalizedStrings(left.UseWhen)
	left.AvoidWhen = normalizedStrings(left.AvoidWhen)
	left.Prerequisites = normalizedStrings(left.Prerequisites)
	left.Tips = normalizedStrings(left.Tips)
	left.WorkflowRefs = normalizedStrings(left.WorkflowRefs)
	left.Examples = uniqueStringsInOrder(left.Examples)
	left.SourceRefs = normalizedStrings(left.SourceRefs)
	return left
}

func loadSources(opts Options) ([]sourceFile, error) {
	paths := []string{
		resolvePath(opts.Root, opts.SkillPath),
		resolvePath(opts.Root, opts.IntentGuidePath),
	}
	productPaths := []string{}
	productsRoot := resolvePath(opts.Root, opts.ProductsDir)
	err := filepath.WalkDir(productsRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			return nil
		}
		productPaths = append(productPaths, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk product references: %w", err)
	}
	paths = append(paths, productPaths...)
	if strings.TrimSpace(opts.HintsDir) != "" {
		hintsRoot := resolvePath(opts.Root, opts.HintsDir)
		hintPaths := []string{}
		err := filepath.WalkDir(hintsRoot, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
				return nil
			}
			hintPaths = append(hintPaths, path)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk Agent hint sources: %w", err)
		}
		paths = append(paths, hintPaths...)
	}
	if strings.TrimSpace(opts.InterfaceMetadataPath) != "" {
		paths = append(paths, resolvePath(opts.Root, opts.InterfaceMetadataPath))
	}
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

func sourceProductIDs(file sourceFile, productsRoot string, references []commandReference, surfaceProducts map[string]bool) []string {
	relative, err := filepath.Rel(productsRoot, file.path)
	if err == nil {
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) > 1 && strings.TrimSpace(parts[0]) != "" {
			return []string{strings.TrimSpace(parts[0])}
		}
	}

	base := strings.TrimSuffix(filepath.Base(file.path), filepath.Ext(file.path))
	commandProducts := map[string]bool{}
	for _, reference := range references {
		if productID := firstCommandToken(normalizeCommandPath(reference.text)); productID != "" {
			commandProducts[productID] = true
		}
	}
	if commandProducts[base] {
		return []string{base}
	}
	if len(surfaceProducts) > 0 {
		matches := []string{}
		for productID := range surfaceProducts {
			if base == productID || strings.HasPrefix(base, productID+"-") {
				matches = append(matches, productID)
			}
		}
		if len(matches) > 0 {
			sort.Slice(matches, func(i, j int) bool {
				if len(matches[i]) != len(matches[j]) {
					return len(matches[i]) > len(matches[j])
				}
				return matches[i] < matches[j]
			})
			return matches[:1]
		}
	}
	if len(commandProducts) > 0 {
		return sortedBoolKeys(commandProducts)
	}
	if base == "" {
		return nil
	}
	return []string{base}
}

func collectCommandReferences(body string) []commandReference {
	lines := strings.Split(body, "\n")
	references := []commandReference{}
	inFence := false
	shellFence := false
	commentIntent := ""
	for index := 0; index < len(lines); index++ {
		line := strings.TrimSpace(lines[index])
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
		if !strings.HasPrefix(line, "dws ") {
			continue
		}

		startLine := index + 1
		parts := []string{}
		for {
			part := strings.TrimSpace(lines[index])
			continued := strings.HasSuffix(part, "\\")
			if continued {
				part = strings.TrimSpace(strings.TrimSuffix(part, "\\"))
			}
			if part != "" {
				parts = append(parts, part)
			}
			if !continued || index+1 >= len(lines) {
				break
			}
			index++
		}
		command := strings.TrimSpace(strings.Join(parts, " "))
		if command != "" {
			references = append(references, commandReference{
				text:          command,
				line:          startLine,
				commentIntent: commentIntent,
			})
		}
	}
	return references
}

func sortedBoolKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key, included := range values {
		if included && strings.TrimSpace(key) != "" {
			keys = append(keys, strings.TrimSpace(key))
		}
	}
	sort.Strings(keys)
	return keys
}

func (tracker sourceTracker) add(toolPath, source string, line int) {
	toolPath = normalizeCommandPath(toolPath)
	if len(strings.Fields(toolPath)) < 2 {
		return
	}
	if tracker[toolPath] == nil {
		tracker[toolPath] = map[string]sourceLocation{}
	}
	location := sourceLocation{source: strings.TrimSpace(source), line: line}
	key := location.source + "\x00" + fmt.Sprintf("%09d", location.line)
	tracker[toolPath][key] = location
}

func (tracker sourceTracker) locations(toolPath string) []sourceLocation {
	byKey := tracker[toolPath]
	locations := make([]sourceLocation, 0, len(byKey))
	for _, location := range byKey {
		locations = append(locations, location)
	}
	sort.Slice(locations, func(i, j int) bool {
		if locations[i].source != locations[j].source {
			return locations[i].source < locations[j].source
		}
		return locations[i].line < locations[j].line
	})
	return locations
}

func candidateToolPaths(skillPath string, toolPaths map[string]string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	canonical := map[string]bool{}
	for _, path := range toolPaths {
		if path = strings.TrimSpace(path); path != "" {
			canonical[path] = true
		}
	}
	type scoredPath struct {
		path  string
		score int
	}
	scored := []scoredPath{}
	for path := range canonical {
		score := commandPathSimilarity(skillPath, path)
		if score > 0 {
			scored = append(scored, scoredPath{path: path, score: score})
		}
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].path < scored[j].path
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}
	out := make([]string, 0, len(scored))
	for _, item := range scored {
		out = append(out, item.path)
	}
	return out
}

func commandPathSimilarity(left, right string) int {
	leftParts := strings.Fields(left)
	rightParts := strings.Fields(right)
	if len(leftParts) == 0 || len(rightParts) == 0 {
		return 0
	}
	score := 0
	if leftParts[0] == rightParts[0] {
		score += 6
	}
	for leftIndex, rightIndex := len(leftParts)-1, len(rightParts)-1; leftIndex >= 0 && rightIndex >= 0; leftIndex, rightIndex = leftIndex-1, rightIndex-1 {
		if leftParts[leftIndex] != rightParts[rightIndex] {
			break
		}
		score += 5
	}
	rightTokens := map[string]bool{}
	for _, token := range rightParts {
		rightTokens[token] = true
	}
	for _, token := range leftParts {
		if rightTokens[token] {
			score++
		}
	}
	if score < 5 {
		return 0
	}
	return score
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

func parseIntentGuide(out *File, body, source string, origins sourceTracker) {
	section, startLine := markdownSectionAt(body, "## 易混淆场景快速对照表")
	for index, line := range strings.Split(section, "\n") {
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
			origins.add(normalizeCommandPath(target), source, startLine+index)
		}
		for _, target := range codeSpans(columns[3]) {
			addTargetAvoid(out, target, scenario, source)
			origins.add(normalizeCommandPath(target), source, startLine+index)
		}
	}
}

func parseToolIntents(out *File, productIDs, known []string, body, source string, origins sourceTracker) {
	for _, heading := range []string{"## 意图判断", "## 使用场景"} {
		section, startLine := markdownSectionAt(body, heading)
		if section == "" {
			continue
		}
		currentIntent := ""
		scanner := bufio.NewScanner(strings.NewReader(section))
		lineIndex := 0
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			lineNumber := startLine + lineIndex
			lineIndex++
			if match := quotedIntentRE.FindStringSubmatch(line); len(match) > 1 {
				currentIntent = cleanIntent(match[1])
				if strings.Contains(line, "→") || strings.Contains(line, "->") {
					if target := routeCodeTarget(line); target != "" {
						if path := resolveToolPath(productIDs, target, known); path != "" {
							addToolUse(out, path, currentIntent, source)
							origins.add(path, source, lineNumber)
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
			if path := resolveToolPath(productIDs, target, known); path != "" {
				addToolUse(out, path, intent, source)
				origins.add(path, source, lineNumber)
			}
		}
	}
}

func parseExamples(out *File, known []string, references []commandReference, source string, maxExamples int, origins sourceTracker) {
	if len(known) == 0 {
		return
	}
	for _, reference := range references {
		line := reference.text
		if strings.Contains(line, "[flags]") || len(line) > 320 {
			continue
		}
		path := longestKnownPrefix(commandTokens(line), known)
		if path == "" {
			continue
		}
		metadata := out.Tools[path]
		if reference.commentIntent != "" {
			metadata.UseWhen = append(metadata.UseWhen, reference.commentIntent)
			metadata.SourceRefs = append(metadata.SourceRefs, source)
		}
		if len(metadata.Examples) < maxExamples {
			metadata.Examples = append(metadata.Examples, line)
			metadata.SourceRefs = append(metadata.SourceRefs, source)
		}
		ensureEffect(&metadata, path)
		applyExplicitCommentSafety(&metadata, reference.commentIntent)
		out.Tools[path] = metadata
		origins.add(path, source, reference.line)
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

func parseDangerRules(out *File, body, source string, stats *Stats, origins sourceTracker) {
	section, startLine := markdownSectionAt(body, "## 危险操作确认")
	for index, line := range strings.Split(section, "\n") {
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
			origins.add(path, source, startLine+index)
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

func collectCommandPaths(productIDs []string, references []commandReference) []string {
	allowedProducts := map[string]bool{}
	for _, productID := range productIDs {
		if productID = strings.TrimSpace(productID); productID != "" {
			allowedProducts[productID] = true
		}
	}
	paths := map[string]bool{}
	for _, reference := range references {
		path := normalizeCommandPath(reference.text)
		if allowedProducts[firstCommandToken(path)] {
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

func resolveToolPath(productIDs []string, raw string, known []string) string {
	candidate := normalizeCommandPath(raw)
	if candidate == "" {
		return ""
	}
	for _, path := range known {
		if candidate == path {
			return path
		}
	}
	productSet := map[string]bool{}
	for _, productID := range productIDs {
		productID = strings.TrimSpace(productID)
		if productID == "" {
			continue
		}
		productSet[productID] = true
		prefixed := strings.TrimSpace(productID + " " + candidate)
		for _, path := range known {
			if prefixed == path {
				return path
			}
		}
	}
	if productSet[firstCommandToken(candidate)] {
		return candidate
	}
	matches := []string{}
	for _, path := range known {
		if strings.HasSuffix(path, " "+candidate) {
			matches = append(matches, path)
		}
	}
	if len(matches) == 1 {
		return matches[0]
	}
	if len(productIDs) == 1 {
		return strings.TrimSpace(productIDs[0] + " " + candidate)
	}
	return ""
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
	if effect := classifyEffectPath(path); effect != "" {
		metadata.Effect = effect
		metadata.EffectSource = "command-verb"
	}
}

func classifyEffectPath(path string) string {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(path)))
	for _, part := range parts[1:] {
		if effect := classifyEffectVerb(part); effect != "" {
			return effect
		}
	}
	return ""
}

func classifyEffectVerb(verb string) string {
	verb = strings.ToLower(strings.TrimSpace(verb))
	read := map[string]bool{
		"list": true, "get": true, "search": true, "read": true, "query": true,
		"detail": true, "status": true, "download": true, "export": true,
		"info": true, "summary": true, "check": true, "inspect": true,
		"diagnose": true, "types": true, "records": true, "tasks": true,
		"find": true, "result": true, "resolve": true, "suggest": true,
		"fields": true, "stats": true, "rules": true, "keywords": true,
		"transcription": true, "todos": true, "audio": true, "mine": true,
		"person": true, "enterprise": true, "behavior": true, "bots": true,
		"invite-url": true, "conversation-info": true, "read-status": true,
		"upload-info": true, "search-options": true, "history-list": true,
		"share-url": true, "rag-pretest": true, "widgets-example": true,
		"config-example": true, "legacy-search-open-platform": true,
	}
	write := map[string]bool{
		"create": true, "update": true, "delete": true, "send": true, "submit": true,
		"approve": true, "reject": true, "revoke": true, "add": true, "remove": true,
		"insert": true, "upload": true, "move": true, "rename": true, "reply": true,
		"recall": true, "publish": true, "enable": true, "disable": true, "save": true,
		"replace": true, "respond": true, "redirect-task": true, "oa-comments": true,
		"oa-cc-noticer": true, "config": true, "connect": true, "reset": true,
		"start": true, "stop": true, "subscribe": true, "unsubscribe": true,
		"browser-policy": true, "chmod": true, "copy": true, "sort": true,
		"forward": true, "fill": true, "upsert": true, "transfer": true,
		"resume": true, "pause": true, "reorder": true, "mute": true,
		"mkdir": true, "duplicate": true, "complete": true, "commit": true,
		"cancel": true, "arrange": true, "append": true, "done": true,
		"import": true, "new": true, "lock": true, "dismiss": true,
		"quit": true, "clear": true, "csv-put": true,
	}
	if read[verb] || hasAnyPrefix(verb, "list-", "get-", "query-", "search-", "read-", "download-", "export-", "inspect-", "check-") || hasAnySuffix(verb, "-list", "-get", "-query", "-search", "-read", "-download", "-export") {
		return "read"
	}
	if write[verb] || hasAnyPrefix(verb,
		"create-", "update-", "delete-", "send-", "add-", "remove-",
		"set-", "unset-", "move-", "copy-", "insert-", "upload-",
		"replace-", "reset-", "enable-", "disable-", "start-", "stop-",
		"subscribe-", "unsubscribe-", "recall-", "reply-", "forward-",
		"clear-", "merge-", "unmerge-", "write-", "append-", "commit-",
		"complete-", "cancel-", "transfer-", "batch-", "role-create",
		"role-update", "role-delete") || hasAnySuffix(verb,
		"-create", "-update", "-delete", "-send", "-add", "-remove",
		"-set", "-move", "-copy", "-insert", "-upload", "-replace",
		"-enable", "-disable", "-forward", "-clear", "-write", "-put",
		"-mute") || hasHyphenToken(verb, "mute") {
		return "write"
	}
	return ""
}

func hasHyphenToken(value string, tokens ...string) bool {
	parts := strings.Split(value, "-")
	for _, part := range parts {
		for _, token := range tokens {
			if part == token {
				return true
			}
		}
	}
	return false
}

func hasAnyPrefix(value string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func hasAnySuffix(value string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(value, suffix) {
			return true
		}
	}
	return false
}

func applyDefaultSafety(metadata *ToolMetadata) {
	if metadata == nil || metadata.Effect == "" {
		return
	}
	if metadata.Effect == "destructive" {
		metadata.Risk = "high"
		metadata.Confirmation = "user_required"
	}
	if metadata.Risk == "" {
		if metadata.Effect == "read" {
			metadata.Risk = "low"
		} else {
			metadata.Risk = "medium"
		}
	}
	if metadata.Confirmation == "" {
		if metadata.Risk == "high" {
			metadata.Confirmation = "user_required"
		} else {
			metadata.Confirmation = "not_required"
		}
	}
	if metadata.Idempotency == "" {
		if metadata.Effect == "read" {
			metadata.Idempotency = "idempotent"
		} else {
			metadata.Idempotency = "unknown"
		}
	}
}

// effectSourceRank orders effect provenance so explicit hints and Skill danger
// rules take precedence over command-verb inference when reconciling duplicate
// command paths (see plan section 5.2: explicit hint > skill parse > verb).
func effectSourceRank(source string) int {
	switch strings.TrimSpace(source) {
	case "skill-explicit", "agent-hint":
		return 3
	case "skill-comment":
		return 2
	case "command-verb":
		return 1
	default:
		return 0
	}
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

func normalizeFile(file *File, maxExamples int) {
	for key, metadata := range file.Products {
		metadata.AgentSummary = strings.TrimSpace(metadata.AgentSummary)
		metadata.AgentSummarySource = strings.TrimSpace(metadata.AgentSummarySource)
		metadata.UseWhen = normalizedStrings(metadata.UseWhen)
		metadata.AvoidWhen = normalizedStrings(metadata.AvoidWhen)
		metadata.SourceRefs = normalizedStrings(metadata.SourceRefs)
		file.Products[key] = metadata
	}
	for key, metadata := range file.Tools {
		ensureEffect(&metadata, key)
		applyDefaultSafety(&metadata)
		metadata.AgentSummary = strings.TrimSpace(metadata.AgentSummary)
		metadata.AgentSummarySource = strings.TrimSpace(metadata.AgentSummarySource)
		metadata.UseWhen = normalizedStrings(metadata.UseWhen)
		metadata.AvoidWhen = normalizedStrings(metadata.AvoidWhen)
		metadata.Prerequisites = normalizedStrings(metadata.Prerequisites)
		metadata.Tips = normalizedStrings(metadata.Tips)
		metadata.WorkflowRefs = normalizedStrings(metadata.WorkflowRefs)
		metadata.Examples = uniqueStringsInOrder(metadata.Examples)
		if maxExamples > 0 && len(metadata.Examples) > maxExamples {
			metadata.Examples = metadata.Examples[:maxExamples]
		}
		metadata.SourceRefs = normalizedStrings(metadata.SourceRefs)
		if metadata.InterfaceRef != nil {
			metadata.InterfaceRef.ProductID = strings.TrimSpace(metadata.InterfaceRef.ProductID)
			metadata.InterfaceRef.RPCName = strings.TrimSpace(metadata.InterfaceRef.RPCName)
		}
		file.Tools[key] = metadata
	}
}

func uniqueStringsInOrder(values []string) []string {
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
	if len(out) == 0 {
		return nil
	}
	return out
}

func metadataProductID(path string) string {
	first := firstCommandToken(path)
	if index := strings.Index(first, "."); index > 0 {
		return first[:index]
	}
	return first
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
	section, _ := markdownSectionAt(body, heading)
	return section
}

func markdownSectionAt(body, heading string) (string, int) {
	lines := strings.Split(body, "\n")
	start := -1
	for index, line := range lines {
		if strings.TrimSpace(line) == heading {
			start = index + 1
			break
		}
	}
	if start < 0 {
		return "", 0
	}
	end := len(lines)
	for index := start; index < len(lines); index++ {
		if strings.HasPrefix(strings.TrimSpace(lines[index]), "## ") {
			end = index
			break
		}
	}
	for start < end && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return strings.Join(lines[start:end], "\n"), start + 1
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
