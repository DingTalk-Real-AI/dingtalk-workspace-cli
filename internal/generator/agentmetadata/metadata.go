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
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
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
	SurfaceProducts        int `json:"surface_products,omitempty"`
	ProductsWithMetadata   int `json:"products_with_metadata"`
	SurfaceTools           int `json:"surface_tools,omitempty"`
	ToolsWithMetadata      int `json:"tools_with_metadata"`
	ToolsWithSummary       int `json:"tools_with_agent_summary,omitempty"`
	ToolsWithUseWhen       int `json:"tools_with_use_when,omitempty"`
	ToolsWithAvoidWhen     int `json:"tools_with_avoid_when,omitempty"`
	ToolsWithExamples      int `json:"tools_with_examples,omitempty"`
	ToolsWithInterfaceMode int `json:"tools_with_interface_mode,omitempty"`
	UnmatchedSkillTools    int `json:"unmatched_skill_tools,omitempty"`
	UnreviewedSkillTools   int `json:"unreviewed_skill_tools,omitempty"`
}

type ProductMetadata struct {
	AgentSummary        string                     `json:"agent_summary,omitempty"`
	AgentSummarySource  string                     `json:"agent_summary_source,omitempty"`
	UseWhen             []string                   `json:"use_when,omitempty"`
	AvoidWhen           []string                   `json:"avoid_when,omitempty"`
	SourceRefs          []string                   `json:"source_refs,omitempty"`
	FieldProvenance     map[string]FieldProvenance `json:"field_provenance,omitempty"`
	agentSummaryPresent bool
	agentSummaryRank    int
	agentSummaryOrigin  string
	useWhenRank         int
	useWhenOrigin       string
	useWhenPresent      bool
	avoidWhenRank       int
	avoidWhenOrigin     string
	avoidWhenPresent    bool
	fieldCandidates     map[string][]FieldCandidateProvenance
}

// MarshalJSON preserves the distinction between an omitted authored list and
// an explicitly authored empty list. The latter is a real precedence value: it
// intentionally clears a lower-ranked non-empty list and must be emitted as
// [] rather than being removed by omitempty.
func (metadata ProductMetadata) MarshalJSON() ([]byte, error) {
	type wire ProductMetadata
	encoded, err := json.Marshal(wire(metadata))
	if err != nil {
		return nil, err
	}
	var object map[string]json.RawMessage
	_ = json.Unmarshal(encoded, &object)
	for _, list := range []struct {
		name    string
		value   []string
		present bool
	}{
		{"use_when", metadata.UseWhen, metadata.useWhenPresent},
		{"avoid_when", metadata.AvoidWhen, metadata.avoidWhenPresent},
	} {
		if !listIsPresent(list.value, list.present) {
			continue
		}
		value, _ := json.Marshal(normalizeAuthoredStrings(list.value))
		object[list.name] = value
	}
	return json.Marshal(object)
}

// InterfaceRef links a stable public command to the MCP operation that
// implements it. It is interface identity, not runtime endpoint discovery.
type InterfaceRef struct {
	ProductID string `json:"product_id"`
	RPCName   string `json:"rpc_name"`
}

// FieldProvenance identifies the winning source and the deterministic rule
// used to resolve one final Agent-facing field.
type FieldProvenance struct {
	Value                any                        `json:"value"`
	Source               string                     `json:"source"`
	Precedence           string                     `json:"precedence"`
	Resolution           string                     `json:"resolution"`
	ReviewReason         string                     `json:"review_reason,omitempty"`
	Candidates           []FieldCandidateProvenance `json:"candidates"`
	OverriddenCandidates []FieldCandidateProvenance `json:"overridden_candidates,omitempty"`
}

// FieldCandidateProvenance preserves a non-winning input so precedence
// decisions remain auditable in the generated Agent contract.
type FieldCandidateProvenance struct {
	Value        any    `json:"value"`
	Source       string `json:"source"`
	Precedence   string `json:"precedence"`
	Selected     bool   `json:"selected"`
	ReviewReason string `json:"review_reason,omitempty"`
}

type ToolMetadata struct {
	AgentSummary           string                     `json:"agent_summary,omitempty"`
	AgentSummarySource     string                     `json:"agent_summary_source,omitempty"`
	UseWhen                []string                   `json:"use_when,omitempty"`
	AvoidWhen              []string                   `json:"avoid_when,omitempty"`
	Prerequisites          []string                   `json:"prerequisites,omitempty"`
	Tips                   []string                   `json:"tips,omitempty"`
	Effect                 string                     `json:"effect,omitempty"`
	EffectSource           string                     `json:"effect_source,omitempty"`
	Risk                   string                     `json:"risk,omitempty"`
	Confirmation           string                     `json:"confirmation,omitempty"`
	Idempotency            string                     `json:"idempotency,omitempty"`
	WorkflowRefs           []string                   `json:"workflow_refs,omitempty"`
	Examples               []string                   `json:"examples,omitempty"`
	Reviewed               *bool                      `json:"reviewed,omitempty"`
	SourceRefs             []string                   `json:"source_refs,omitempty"`
	InterfaceRef           *InterfaceRef              `json:"interface_ref,omitempty"`
	InterfaceMode          string                     `json:"interface_mode,omitempty"`
	Availability           string                     `json:"availability,omitempty"`
	InterfaceReason        string                     `json:"interface_reason,omitempty"`
	FieldProvenance        map[string]FieldProvenance `json:"field_provenance,omitempty"`
	agentSummaryPresent    bool
	agentSummaryRank       int
	agentSummaryOrigin     string
	useWhenRank            int
	useWhenOrigin          string
	useWhenPresent         bool
	avoidWhenRank          int
	avoidWhenOrigin        string
	avoidWhenPresent       bool
	prerequisitesRank      int
	prerequisitesOrigin    string
	prerequisitesPresent   bool
	tipsRank               int
	tipsOrigin             string
	tipsPresent            bool
	workflowRefsRank       int
	workflowRefsOrigin     string
	workflowRefsPresent    bool
	examplesRank           int
	examplesOrigin         string
	examplesPresent        bool
	effectPresent          bool
	riskPresent            bool
	confirmationPresent    bool
	idempotencyPresent     bool
	idempotencyRank        int
	idempotencyOrigin      string
	reviewedRank           int
	reviewedOrigin         string
	interfaceRefPresent    bool
	interfaceRefRank       int
	interfaceRefOrigin     string
	interfaceModePresent   bool
	interfaceModeRank      int
	interfaceModeOrigin    string
	availabilityPresent    bool
	availabilityRank       int
	availabilityOrigin     string
	interfaceReasonPresent bool
	interfaceReasonRank    int
	interfaceReasonOrigin  string
	effectRank             int
	effectOrigin           string
	riskRank               int
	riskOrigin             string
	confirmationRank       int
	confirmationOrigin     string
	fieldCandidates        map[string][]FieldCandidateProvenance
}

// MarshalJSON preserves explicit empty authored lists for the same reason as
// ProductMetadata.MarshalJSON. Unset lists remain omitted.
func (metadata ToolMetadata) MarshalJSON() ([]byte, error) {
	type wire ToolMetadata
	encoded, err := json.Marshal(wire(metadata))
	if err != nil {
		return nil, err
	}
	var object map[string]json.RawMessage
	_ = json.Unmarshal(encoded, &object)
	for _, list := range []struct {
		name    string
		value   []string
		present bool
	}{
		{"use_when", metadata.UseWhen, metadata.useWhenPresent},
		{"avoid_when", metadata.AvoidWhen, metadata.avoidWhenPresent},
		{"prerequisites", metadata.Prerequisites, metadata.prerequisitesPresent},
		{"tips", metadata.Tips, metadata.tipsPresent},
		{"workflow_refs", metadata.WorkflowRefs, metadata.workflowRefsPresent},
		{"examples", metadata.Examples, metadata.examplesPresent},
	} {
		if !listIsPresent(list.value, list.present) {
			continue
		}
		value, _ := json.Marshal(normalizeAuthoredStrings(list.value))
		object[list.name] = value
	}
	return json.Marshal(object)
}

const (
	// Scalar fields use one source-only precedence for selection, safety, and
	// interface disposition. Values never influence precedence: a reviewed
	// explicit hint may intentionally upgrade or downgrade any scalar.
	selectionRankDefault            = 0
	selectionRankMCPFallback        = 10
	selectionRankSkill              = 100
	selectionRankUnreviewedExplicit = 150
	selectionRankImported           = 200
	selectionRankExplicit           = 300
	selectionRankReviewedExplicit   = 400
	selectionRankReviewedManual     = 500

	selectionPrecedenceDefault            = "inference_or_default"
	selectionPrecedenceMCPFallback        = "mcp_fallback"
	selectionPrecedenceSkill              = "skill"
	selectionPrecedenceUnreviewedExplicit = "unreviewed_explicit"
	selectionPrecedenceImported           = "imported"
	selectionPrecedenceExplicit           = "explicit"
	selectionPrecedenceReviewedExplicit   = "reviewed_explicit"
	selectionPrecedenceReviewedManual     = "reviewed_manual"
)

type Options struct {
	Root                     string
	SkillPath                string
	ProductsDir              string
	IntentGuidePath          string
	HintsDir                 string
	ManualHintsPath          string
	InterfaceMetadataPath    string
	MaxExamples              int
	MaxInterfaceSummaryRunes int
	ToolPaths                map[string]string
	CanonicalToolPaths       map[string]string
	BoundCommands            cli.BoundCommandRegistry
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
	referenceReviews              map[string]ReferenceReview
	unreviewedSkillTools          int
}

// UnmatchedReference identifies one Skill command reference that cannot be
// resolved against the versioned command surface. It is emitted only in the
// build-time audit and is not embedded in the runtime Agent schema.
type UnmatchedReference struct {
	ToolPath   string           `json:"tool_path"`
	Source     string           `json:"source,omitempty"`
	Line       int              `json:"line,omitempty"`
	Candidates []string         `json:"candidates,omitempty"`
	Review     *ReferenceReview `json:"review,omitempty"`
}

// ReferenceReview is the fixed disposition of a Skill command reference that
// is not a current public leaf. It prevents fuzzy matching from silently
// binding stale prose or command groups to an unrelated tool.
type ReferenceReview struct {
	Status string `json:"status"`
	Target string `json:"target,omitempty"`
	Reason string `json:"reason"`
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
	if strings.TrimSpace(opts.HintsDir) == "" {
		return File{}, Stats{}, fmt.Errorf("agent hint directory is required")
	}
	if len(opts.CanonicalToolPaths) == 0 || len(opts.ToolPaths) == 0 || len(opts.ProductIDs) == 0 {
		return File{}, Stats{}, fmt.Errorf("complete Effective CommandRegistry projection is required")
	}
	if len(opts.BoundCommands.ByCanonical) != len(opts.CanonicalToolPaths) {
		return File{}, Stats{}, fmt.Errorf("complete Cobra-bound CommandRegistry is required")
	}
	return generateFromSources(opts)
}

// generateFromSources retains the lower-precedence evidence parsers as a
// package-internal seam for focused tests. Production callers must use Generate,
// which requires the reviewed selection + metadata hint sources under HintsDir.
func generateFromSources(opts Options) (File, Stats, error) {
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
	stats := Stats{SourceFiles: len(files), referenceReviews: map[string]ReferenceReview{}}
	origins := sourceTracker{}

	skillDisplay := displayPath(opts.Root, resolvePath(opts.Root, opts.SkillPath))
	if skill, ok := byDisplay[skillDisplay]; ok {
		parseProductRouting(&out, string(skill.data), skill.display)
		if err := parseDangerRules(&out, string(skill.data), skill.display, &stats, origins); err != nil {
			return File{}, Stats{}, err
		}
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
		if err := parseExamples(&out, known, references, file.display, opts.MaxExamples, origins); err != nil {
			return File{}, Stats{}, err
		}
	}
	usedSelection, err := parseHintSources(&out, files, opts, &stats, origins)
	if err != nil {
		return File{}, Stats{}, err
	}
	if usedSelection {
		if err := validateSelectionAuthoringContracts(opts); err != nil {
			return File{}, Stats{}, err
		}
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
	if err := reconcileSurface(&out, opts, &stats, origins); err != nil {
		return File{}, Stats{}, err
	}
	seedEffectiveToolProjection(&out, opts.ToolPaths)
	if err := validateEffectiveToolProjection(out, opts); err != nil {
		return File{}, Stats{}, err
	}
	if usedSelection {
		retainReviewedSelectionCandidates(&out)
	}
	normalizeFile(&out, opts.MaxExamples)
	if err := validateToolFieldCandidateConflicts(out); err != nil {
		return File{}, Stats{}, err
	}
	if usedSelection {
		if err := validateReviewedSelectionDelivery(out, opts); err != nil {
			return File{}, Stats{}, err
		}
	}
	finalizeInterfaceDispositions(&out)
	if err := validateInterfaceDispositions(out); err != nil {
		return File{}, Stats{}, err
	}
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
	toolsWithSummary, toolsWithUseWhen, toolsWithAvoidWhen := 0, 0, 0
	toolsWithExamples, toolsWithInterfaceMode := 0, 0
	for _, metadata := range out.Tools {
		stats.ToolIntents += len(metadata.UseWhen)
		stats.Examples += len(metadata.Examples)
		if strings.TrimSpace(metadata.AgentSummary) != "" {
			toolsWithSummary++
		}
		// Coverage is about a resolved field, not truthiness. A reviewed [] is
		// an authored winner that intentionally clears lower-priority guidance.
		if listIsPresent(metadata.UseWhen, metadata.useWhenPresent) {
			toolsWithUseWhen++
		}
		if listIsPresent(metadata.AvoidWhen, metadata.avoidWhenPresent) {
			toolsWithAvoidWhen++
		}
		if listIsPresent(metadata.Examples, metadata.examplesPresent) {
			toolsWithExamples++
		}
		if strings.TrimSpace(metadata.InterfaceMode) != "" {
			toolsWithInterfaceMode++
		}
	}
	out.Coverage = Coverage{
		SurfaceProducts:        len(opts.ProductIDs),
		ProductsWithMetadata:   len(out.Products),
		SurfaceTools:           opts.SurfaceToolCount,
		ToolsWithMetadata:      len(out.Tools),
		ToolsWithSummary:       toolsWithSummary,
		ToolsWithUseWhen:       toolsWithUseWhen,
		ToolsWithAvoidWhen:     toolsWithAvoidWhen,
		ToolsWithExamples:      toolsWithExamples,
		ToolsWithInterfaceMode: toolsWithInterfaceMode,
		UnmatchedSkillTools:    stats.UnmatchedTools,
		UnreviewedSkillTools:   stats.unreviewedSkillTools,
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

func reconcileSurface(file *File, opts Options, stats *Stats, origins sourceTracker) error {
	if len(opts.ProductIDs) > 0 {
		for productID := range file.Products {
			if !opts.ProductIDs[productID] {
				delete(file.Products, productID)
			}
		}
	}
	if len(opts.ToolPaths) == 0 {
		return nil
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
			review, reviewed := stats.referenceReviews[skillPath]
			if reviewed && review.Status == "alias" {
				if target, targetOK := opts.ToolPaths[normalizeCommandPath(review.Target)]; targetOK {
					existing := reconciled[target]
					merged, err := mergeToolMetadata(existing, metadata, target)
					if err != nil {
						return err
					}
					reconciled[target] = merged
					continue
				}
			}
			stats.UnmatchedTools++
			if !reviewed {
				stats.unreviewedSkillTools++
			}
			locations := origins.locations(skillPath)
			if len(locations) == 0 {
				locations = []sourceLocation{{}}
			}
			candidates := candidateToolPaths(skillPath, opts.ToolPaths, 3)
			for _, location := range locations {
				reference := UnmatchedReference{
					ToolPath:   skillPath,
					Source:     location.source,
					Line:       location.line,
					Candidates: append([]string(nil), candidates...),
				}
				if reviewed {
					value := review
					reference.Review = &value
				}
				stats.UnmatchedReferences = append(stats.UnmatchedReferences, reference)
			}
			continue
		}
		existing := reconciled[livePath]
		merged, err := mergeToolMetadata(existing, metadata, livePath)
		if err != nil {
			return err
		}
		reconciled[livePath] = merged
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
	return nil
}

// seedEffectiveToolProjection materializes one Agent metadata record for every
// effective public command, including reviewed manual-only commands. Empty
// records remain subject to the normal defaulting and final release gates; the
// important identity invariant here is that absence can no longer be hidden
// behind a surface count.
func seedEffectiveToolProjection(file *File, toolPaths map[string]string) {
	if file == nil {
		return
	}
	if file.Tools == nil {
		file.Tools = map[string]ToolMetadata{}
	}
	for _, rawPrimary := range toolPaths {
		primary := normalizeCommandPath(rawPrimary)
		if primary == "" {
			continue
		}
		if _, exists := file.Tools[primary]; !exists {
			file.Tools[primary] = ToolMetadata{}
		}
	}
}

// validateEffectiveToolProjection makes the EffectiveCommandRegistry
// projection fail closed. ToolPaths contains canonical paths and reviewed
// aliases mapped to their one primary CLI path; therefore its unique values
// are the exact public command set that Agent metadata must deliver.
func validateEffectiveToolProjection(file File, opts Options) error {
	if len(opts.ToolPaths) == 0 {
		return nil
	}
	expected := make(map[string]bool, len(opts.ToolPaths))
	for _, rawPrimary := range opts.ToolPaths {
		if primary := normalizeCommandPath(rawPrimary); primary != "" {
			expected[primary] = true
		}
	}
	missing := make([]string, 0)
	for primary := range expected {
		if _, ok := file.Tools[primary]; !ok {
			missing = append(missing, primary)
		}
	}
	unexpected := make([]string, 0)
	for primary := range file.Tools {
		if !expected[normalizeCommandPath(primary)] {
			unexpected = append(unexpected, primary)
		}
	}
	sort.Strings(missing)
	sort.Strings(unexpected)
	if len(missing) > 0 || len(unexpected) > 0 {
		return fmt.Errorf(
			"agent metadata does not exactly project the effective CommandRegistry: missing=%v unexpected=%v",
			missing,
			unexpected,
		)
	}
	if opts.SurfaceToolCount > 0 && opts.SurfaceToolCount != len(expected) {
		return fmt.Errorf(
			"agent metadata effective CommandRegistry count %d disagrees with unique projected tools %d",
			opts.SurfaceToolCount,
			len(expected),
		)
	}
	return nil
}

func mergeToolMetadata(left, right ToolMetadata, path string) (ToolMetadata, error) {
	seedToolFieldCandidates(&left)
	seedToolFieldCandidates(&right)
	mergeFieldCandidateHistory(&left, right)
	previousSummaryPresent := scalarIsPresent(left.AgentSummary, left.agentSummaryPresent)
	previousSummaryRank := left.agentSummaryRank
	previousSummaryOrigin := left.agentSummaryOrigin
	incomingSummaryPresent := scalarIsPresent(right.AgentSummary, right.agentSummaryPresent)
	incomingSummaryRank := effectiveSelectionScalarRank(right.AgentSummary, incomingSummaryPresent, right.agentSummaryRank)
	incomingSummaryOrigin := firstNonEmpty(right.agentSummaryOrigin, right.AgentSummarySource)
	recordStringFieldCandidate(&left, "agent_summary", right.AgentSummary, incomingSummaryPresent, incomingSummaryRank, incomingSummaryOrigin,
		fieldWinnerReviewReason(right, "agent_summary", right.AgentSummary, incomingSummaryOrigin, incomingSummaryRank))
	if err := mergeRankedStringValue(
		&left.AgentSummary, &left.agentSummaryPresent, &left.agentSummaryRank, &left.agentSummaryOrigin,
		right.AgentSummary, incomingSummaryPresent, incomingSummaryRank, incomingSummaryOrigin, path, "agent_summary",
	); err != nil {
		return ToolMetadata{}, err
	}
	incomingSummaryWon := incomingSummaryPresent &&
		(!previousSummaryPresent || incomingSummaryRank > previousSummaryRank ||
			(incomingSummaryRank == previousSummaryRank && left.agentSummaryOrigin != previousSummaryOrigin && left.agentSummaryOrigin == strings.TrimSpace(incomingSummaryOrigin)))
	if incomingSummaryWon {
		left.AgentSummarySource = right.AgentSummarySource
	}
	for _, list := range []struct {
		name         string
		leftValue    *[]string
		leftPresent  *bool
		leftRank     *int
		leftOrigin   *string
		rightValue   []string
		rightPresent bool
		rightRank    int
		rightOrigin  string
	}{
		{"use_when", &left.UseWhen, &left.useWhenPresent, &left.useWhenRank, &left.useWhenOrigin, right.UseWhen, listIsPresent(right.UseWhen, right.useWhenPresent), right.useWhenRank, right.useWhenOrigin},
		{"avoid_when", &left.AvoidWhen, &left.avoidWhenPresent, &left.avoidWhenRank, &left.avoidWhenOrigin, right.AvoidWhen, listIsPresent(right.AvoidWhen, right.avoidWhenPresent), right.avoidWhenRank, right.avoidWhenOrigin},
		{"prerequisites", &left.Prerequisites, &left.prerequisitesPresent, &left.prerequisitesRank, &left.prerequisitesOrigin, right.Prerequisites, listIsPresent(right.Prerequisites, right.prerequisitesPresent), right.prerequisitesRank, right.prerequisitesOrigin},
		{"tips", &left.Tips, &left.tipsPresent, &left.tipsRank, &left.tipsOrigin, right.Tips, listIsPresent(right.Tips, right.tipsPresent), right.tipsRank, right.tipsOrigin},
		{"workflow_refs", &left.WorkflowRefs, &left.workflowRefsPresent, &left.workflowRefsRank, &left.workflowRefsOrigin, right.WorkflowRefs, listIsPresent(right.WorkflowRefs, right.workflowRefsPresent), right.workflowRefsRank, right.workflowRefsOrigin},
		{"examples", &left.Examples, &left.examplesPresent, &left.examplesRank, &left.examplesOrigin, right.Examples, listIsPresent(right.Examples, right.examplesPresent), right.examplesRank, right.examplesOrigin},
	} {
		incomingRank := effectiveSelectionListRank(list.rightValue, list.rightPresent, list.rightRank)
		if err := mergeRankedStringList(list.leftValue, list.leftPresent, list.leftRank, list.leftOrigin, list.rightValue, list.rightPresent, incomingRank, list.rightOrigin, path, list.name); err != nil {
			return ToolMetadata{}, err
		}
	}
	left.SourceRefs = append(left.SourceRefs, right.SourceRefs...)
	if err := mergeEffectValue(&left, right.Effect, scalarIsPresent(right.Effect, right.effectPresent), right.EffectSource, right.effectRank, right.effectOrigin,
		fieldWinnerReviewReason(right, "effect", right.Effect, right.effectOrigin, right.effectRank), path); err != nil {
		return ToolMetadata{}, err
	}
	if err := mergeRiskValue(&left, right.Risk, scalarIsPresent(right.Risk, right.riskPresent), right.riskRank, right.riskOrigin,
		fieldWinnerReviewReason(right, "risk", right.Risk, right.riskOrigin, right.riskRank), path); err != nil {
		return ToolMetadata{}, err
	}
	if err := mergeConfirmationValue(&left, right.Confirmation, scalarIsPresent(right.Confirmation, right.confirmationPresent), right.confirmationRank, right.confirmationOrigin,
		fieldWinnerReviewReason(right, "confirmation", right.Confirmation, right.confirmationOrigin, right.confirmationRank), path); err != nil {
		return ToolMetadata{}, err
	}
	idempotencyPresent := scalarIsPresent(right.Idempotency, right.idempotencyPresent)
	recordStringFieldCandidate(&left, "idempotency", right.Idempotency, idempotencyPresent, right.idempotencyRank, right.idempotencyOrigin,
		fieldWinnerReviewReason(right, "idempotency", right.Idempotency, right.idempotencyOrigin, right.idempotencyRank))
	if err := mergeRankedStringValue(
		&left.Idempotency, &left.idempotencyPresent, &left.idempotencyRank, &left.idempotencyOrigin,
		right.Idempotency, idempotencyPresent, right.idempotencyRank, right.idempotencyOrigin, path, "idempotency",
	); err != nil {
		return ToolMetadata{}, err
	}
	if right.Reviewed != nil {
		recordTypedFieldCandidateValue(&left, "reviewed", *right.Reviewed, true, right.reviewedRank, right.reviewedOrigin,
			fieldWinnerReviewReasonValue(right, "reviewed", *right.Reviewed, right.reviewedOrigin, right.reviewedRank))
		if left.Reviewed == nil || right.reviewedRank > left.reviewedRank {
			value := *right.Reviewed
			left.Reviewed = &value
			left.reviewedRank = right.reviewedRank
			left.reviewedOrigin = right.reviewedOrigin
		} else if right.reviewedRank == left.reviewedRank && *left.Reviewed == *right.Reviewed {
			left.reviewedOrigin = stableSource(left.reviewedOrigin, right.reviewedOrigin)
		}
	}
	if err := mergeRankedInterfaceRef(&left, right, path); err != nil {
		return ToolMetadata{}, err
	}
	for _, field := range []struct {
		name         string
		leftValue    *string
		leftPresent  *bool
		leftRank     *int
		leftOrigin   *string
		rightValue   string
		rightPresent bool
		rightRank    int
		rightOrigin  string
	}{
		{"interface_mode", &left.InterfaceMode, &left.interfaceModePresent, &left.interfaceModeRank, &left.interfaceModeOrigin, right.InterfaceMode, scalarIsPresent(right.InterfaceMode, right.interfaceModePresent), right.interfaceModeRank, right.interfaceModeOrigin},
		{"availability", &left.Availability, &left.availabilityPresent, &left.availabilityRank, &left.availabilityOrigin, right.Availability, scalarIsPresent(right.Availability, right.availabilityPresent), right.availabilityRank, right.availabilityOrigin},
		{"interface_reason", &left.InterfaceReason, &left.interfaceReasonPresent, &left.interfaceReasonRank, &left.interfaceReasonOrigin, right.InterfaceReason, scalarIsPresent(right.InterfaceReason, right.interfaceReasonPresent), right.interfaceReasonRank, right.interfaceReasonOrigin},
	} {
		rank := effectiveSelectionScalarRank(field.rightValue, field.rightPresent, field.rightRank)
		recordStringFieldCandidate(&left, field.name, field.rightValue, field.rightPresent, rank, field.rightOrigin,
			fieldWinnerReviewReason(right, field.name, field.rightValue, field.rightOrigin, rank))
		if err := mergeRankedStringValue(
			field.leftValue, field.leftPresent, field.leftRank, field.leftOrigin,
			field.rightValue, field.rightPresent, rank, field.rightOrigin,
			path, field.name,
		); err != nil {
			return ToolMetadata{}, err
		}
	}
	left.UseWhen = normalizeAuthoredStrings(left.UseWhen)
	left.AvoidWhen = normalizeAuthoredStrings(left.AvoidWhen)
	left.Prerequisites = normalizeAuthoredStrings(left.Prerequisites)
	left.Tips = normalizeAuthoredStrings(left.Tips)
	left.WorkflowRefs = normalizeAuthoredStrings(left.WorkflowRefs)
	left.Examples = normalizeAuthoredStrings(left.Examples)
	preserveExplicitEmptyToolLists(&left)
	left.SourceRefs = normalizedStrings(left.SourceRefs)
	syncToolFieldProvenance(&left)
	return left, nil
}

func seedToolFieldCandidates(metadata *ToolMetadata) {
	if metadata == nil {
		return
	}
	recordStringFieldCandidate(metadata, "agent_summary", metadata.AgentSummary, scalarIsPresent(metadata.AgentSummary, metadata.agentSummaryPresent), effectiveSelectionScalarRank(metadata.AgentSummary, metadata.agentSummaryPresent, metadata.agentSummaryRank), firstNonEmpty(metadata.agentSummaryOrigin, metadata.AgentSummarySource), "")
	recordStringFieldCandidate(metadata, "effect", metadata.Effect, scalarIsPresent(metadata.Effect, metadata.effectPresent), metadata.effectRank, firstNonEmpty(metadata.effectOrigin, metadata.EffectSource), "")
	recordStringFieldCandidate(metadata, "risk", metadata.Risk, scalarIsPresent(metadata.Risk, metadata.riskPresent), metadata.riskRank, metadata.riskOrigin, "")
	recordStringFieldCandidate(metadata, "confirmation", metadata.Confirmation, scalarIsPresent(metadata.Confirmation, metadata.confirmationPresent), metadata.confirmationRank, metadata.confirmationOrigin, "")
	recordStringFieldCandidate(metadata, "idempotency", metadata.Idempotency, scalarIsPresent(metadata.Idempotency, metadata.idempotencyPresent), metadata.idempotencyRank, metadata.idempotencyOrigin, "")
	if metadata.InterfaceRef != nil || metadata.interfaceRefPresent {
		rank := metadata.interfaceRefRank
		if rank == selectionRankDefault {
			rank = selectionRankSkill
		}
		var value any
		if metadata.InterfaceRef != nil {
			value = metadata.InterfaceRef.ProductID + "." + metadata.InterfaceRef.RPCName
		}
		recordTypedFieldCandidateValue(metadata, "interface_ref", value, true, rank, metadata.interfaceRefOrigin, "")
	}
	recordStringFieldCandidate(metadata, "interface_mode", metadata.InterfaceMode, scalarIsPresent(metadata.InterfaceMode, metadata.interfaceModePresent), effectiveSelectionScalarRank(metadata.InterfaceMode, metadata.interfaceModePresent, metadata.interfaceModeRank), metadata.interfaceModeOrigin, "")
	recordStringFieldCandidate(metadata, "availability", metadata.Availability, scalarIsPresent(metadata.Availability, metadata.availabilityPresent), effectiveSelectionScalarRank(metadata.Availability, metadata.availabilityPresent, metadata.availabilityRank), metadata.availabilityOrigin, "")
	recordStringFieldCandidate(metadata, "interface_reason", metadata.InterfaceReason, scalarIsPresent(metadata.InterfaceReason, metadata.interfaceReasonPresent), effectiveSelectionScalarRank(metadata.InterfaceReason, metadata.interfaceReasonPresent, metadata.interfaceReasonRank), metadata.interfaceReasonOrigin, "")
	if metadata.Reviewed != nil {
		recordTypedFieldCandidateValue(metadata, "reviewed", *metadata.Reviewed, true, metadata.reviewedRank, metadata.reviewedOrigin, "")
	}
	for _, list := range []struct {
		name    string
		value   []string
		present bool
		rank    int
		origin  string
	}{
		{"use_when", metadata.UseWhen, listIsPresent(metadata.UseWhen, metadata.useWhenPresent), metadata.useWhenRank, metadata.useWhenOrigin},
		{"avoid_when", metadata.AvoidWhen, listIsPresent(metadata.AvoidWhen, metadata.avoidWhenPresent), metadata.avoidWhenRank, metadata.avoidWhenOrigin},
		{"prerequisites", metadata.Prerequisites, listIsPresent(metadata.Prerequisites, metadata.prerequisitesPresent), metadata.prerequisitesRank, metadata.prerequisitesOrigin},
		{"tips", metadata.Tips, listIsPresent(metadata.Tips, metadata.tipsPresent), metadata.tipsRank, metadata.tipsOrigin},
		{"workflow_refs", metadata.WorkflowRefs, listIsPresent(metadata.WorkflowRefs, metadata.workflowRefsPresent), metadata.workflowRefsRank, metadata.workflowRefsOrigin},
		{"examples", metadata.Examples, listIsPresent(metadata.Examples, metadata.examplesPresent), metadata.examplesRank, metadata.examplesOrigin},
	} {
		recordListFieldCandidate(metadata, list.name, list.value, list.present, effectiveSelectionListRank(list.value, list.present, list.rank), list.origin, "")
	}
}

func mergeRankedString(target *string, targetRank *int, targetOrigin *string, incoming string, incomingRank int, incomingOrigin, path, field string) error {
	return mergeRankedStringValue(
		target, nil, targetRank, targetOrigin,
		incoming, strings.TrimSpace(incoming) != "", incomingRank, incomingOrigin, path, field,
	)
}

// mergeRankedStringValue keeps authored presence separate from the string
// value. An explicit "" is therefore a real candidate which can clear a
// lower-ranked non-empty value. Callers that do not expose authored presence
// continue to use mergeRankedString, where an empty string means absence.
func mergeRankedStringValue(target *string, targetPresent *bool, targetRank *int, targetOrigin *string, incoming string, incomingPresent bool, incomingRank int, incomingOrigin, path, field string) error {
	if !incomingPresent {
		return nil
	}
	incoming = strings.TrimSpace(incoming)
	incomingOrigin = strings.TrimSpace(incomingOrigin)
	current := strings.TrimSpace(*target)
	currentPresent := current != ""
	if targetPresent != nil {
		currentPresent = currentPresent || *targetPresent
	}
	if !currentPresent || incomingRank > *targetRank {
		*target = incoming
		if targetPresent != nil {
			*targetPresent = true
		}
		*targetRank = incomingRank
		*targetOrigin = incomingOrigin
		return nil
	}
	if incomingRank < *targetRank {
		return nil
	}
	if current != incoming {
		return fmt.Errorf("agent metadata conflict for %s field %s at precedence %d: %q from %s conflicts with %q from %s",
			path, field, incomingRank, *target, firstNonEmpty(*targetOrigin, "<unknown>"), incoming, firstNonEmpty(incomingOrigin, "<unknown>"))
	}
	if targetPresent != nil {
		*targetPresent = true
	}
	*targetOrigin = stableSource(*targetOrigin, incomingOrigin)
	return nil
}

func mergeRankedStringList(target *[]string, targetPresent *bool, targetRank *int, targetOrigin *string, incoming []string, incomingPresent bool, incomingRank int, incomingOrigin, path, field string) error {
	if !incomingPresent {
		return nil
	}
	incoming = normalizeAuthoredStrings(incoming)
	incomingOrigin = strings.TrimSpace(incomingOrigin)
	current := normalizeAuthoredStrings(*target)
	currentPresent := listIsPresent(*target, *targetPresent)
	if !currentPresent || incomingRank > *targetRank {
		*target = cloneStringList(incoming)
		*targetPresent = true
		*targetRank = incomingRank
		*targetOrigin = incomingOrigin
		return nil
	}
	if incomingRank < *targetRank {
		return nil
	}
	if incomingRank == selectionRankSkill {
		*target = normalizeAuthoredStrings(append(current, incoming...))
		*targetPresent = true
		*targetOrigin = combinedSkillSource(*targetOrigin, incomingOrigin)
		return nil
	}
	if !fieldValueEqual(current, incoming) {
		return fmt.Errorf("agent metadata conflict for %s field %s at precedence %d: %s from %s conflicts with %s from %s",
			path, field, incomingRank,
			fieldValueDisplay(current), firstNonEmpty(*targetOrigin, "<unknown>"),
			fieldValueDisplay(incoming), firstNonEmpty(incomingOrigin, "<unknown>"))
	}
	*target = cloneStringList(current)
	*targetPresent = true
	*targetOrigin = stableSource(*targetOrigin, incomingOrigin)
	return nil
}

func effectiveSelectionRank(value string, rank int) int {
	return effectiveSelectionScalarRank(value, strings.TrimSpace(value) != "", rank)
}

func effectiveSelectionScalarRank(value string, present bool, rank int) int {
	if scalarIsPresent(value, present) && rank == selectionRankDefault {
		return selectionRankSkill
	}
	return rank
}

func scalarIsPresent(value string, explicit bool) bool {
	return explicit || strings.TrimSpace(value) != ""
}

func effectiveSelectionListRank(values []string, present bool, rank int) int {
	if listIsPresent(values, present) && rank == selectionRankDefault {
		return selectionRankSkill
	}
	return rank
}

func mergeRankedInterfaceRef(left *ToolMetadata, right ToolMetadata, path string) error {
	if left == nil || (right.InterfaceRef == nil && !right.interfaceRefPresent) {
		return nil
	}
	rank := right.interfaceRefRank
	if rank == selectionRankDefault {
		rank = selectionRankSkill
	}
	rightValue := interfaceRefCandidateValue(right.InterfaceRef)
	recordTypedFieldCandidateValue(left, "interface_ref", rightValue, true, rank, right.interfaceRefOrigin,
		fieldWinnerReviewReasonValue(right, "interface_ref", rightValue, right.interfaceRefOrigin, rank))
	leftPresent := left.InterfaceRef != nil || left.interfaceRefPresent
	if !leftPresent || rank > left.interfaceRefRank {
		left.InterfaceRef = cloneInterfaceRef(right.InterfaceRef)
		left.interfaceRefPresent = true
		left.interfaceRefRank = rank
		left.interfaceRefOrigin = right.interfaceRefOrigin
		return nil
	}
	if rank < left.interfaceRefRank {
		return nil
	}
	leftValue := interfaceRefCandidateValue(left.InterfaceRef)
	if !fieldValueEqual(leftValue, rightValue) {
		return fmt.Errorf("agent metadata conflict for %s field interface_ref at precedence %d: %s from %s conflicts with %s from %s",
			path, rank,
			fieldValueDisplay(leftValue), firstNonEmpty(left.interfaceRefOrigin, "<unknown>"),
			fieldValueDisplay(rightValue), firstNonEmpty(right.interfaceRefOrigin, "<unknown>"))
	}
	left.interfaceRefPresent = true
	left.interfaceRefOrigin = stableSource(left.interfaceRefOrigin, right.interfaceRefOrigin)
	return nil
}

func interfaceRefCandidateValue(ref *InterfaceRef) any {
	if ref == nil {
		return nil
	}
	return strings.TrimSpace(ref.ProductID) + "." + strings.TrimSpace(ref.RPCName)
}

func cloneInterfaceRef(ref *InterfaceRef) *InterfaceRef {
	if ref == nil {
		return nil
	}
	value := *ref
	return &value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func stableSource(left, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || (right != "" && right < left) {
		return right
	}
	return left
}

func listIsPresent(values []string, explicit bool) bool {
	return explicit || values != nil
}

func cloneStringList(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string{}, values...)
}

func preserveExplicitEmptyToolLists(metadata *ToolMetadata) {
	if metadata == nil {
		return
	}
	for _, list := range []struct {
		value   *[]string
		present bool
	}{
		{&metadata.UseWhen, metadata.useWhenPresent},
		{&metadata.AvoidWhen, metadata.avoidWhenPresent},
		{&metadata.Prerequisites, metadata.prerequisitesPresent},
		{&metadata.Tips, metadata.tipsPresent},
		{&metadata.WorkflowRefs, metadata.workflowRefsPresent},
		{&metadata.Examples, metadata.examplesPresent},
	} {
		if list.present && *list.value == nil {
			*list.value = []string{}
		}
	}
}

func preserveExplicitEmptyProductLists(metadata *ProductMetadata) {
	if metadata == nil {
		return
	}
	if metadata.useWhenPresent && metadata.UseWhen == nil {
		metadata.UseWhen = []string{}
	}
	if metadata.avoidWhenPresent && metadata.AvoidWhen == nil {
		metadata.AvoidWhen = []string{}
	}
}

func cloneFieldValue(value any) any {
	switch typed := value.(type) {
	case []string:
		return cloneStringList(typed)
	default:
		return typed
	}
}

func fieldValueKey(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(encoded)
}

func fieldValueDisplay(value any) string {
	return fieldValueKey(value)
}

func fieldStringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func fieldValueEqual(left, right any) bool {
	return fieldValueKey(left) == fieldValueKey(right)
}

func fieldCandidateEqual(left, right FieldCandidateProvenance) bool {
	return fieldValueEqual(left.Value, right.Value) &&
		left.Source == right.Source &&
		left.Precedence == right.Precedence &&
		left.Selected == right.Selected &&
		left.ReviewReason == right.ReviewReason
}

func normalizeListFieldCandidates(metadata *ToolMetadata, maxExamples int) {
	if metadata == nil {
		return
	}
	for field, candidates := range metadata.fieldCandidates {
		for index := range candidates {
			values, ok := candidates[index].Value.([]string)
			if !ok {
				continue
			}
			values = normalizeAuthoredStrings(values)
			if field == "examples" && maxExamples > 0 && len(values) > maxExamples {
				values = values[:maxExamples]
			}
			candidates[index].Value = cloneStringList(values)
		}
		metadata.fieldCandidates[field] = candidates
	}
}

func mergeEffect(metadata *ToolMetadata, incoming, effectSource string, rank int, origin, reviewReason, path string) error {
	return mergeEffectValue(metadata, incoming, strings.TrimSpace(incoming) != "", effectSource, rank, origin, reviewReason, path)
}

func mergeEffectValue(metadata *ToolMetadata, incoming string, incomingPresent bool, effectSource string, rank int, origin, reviewReason, path string) error {
	if metadata == nil || !incomingPresent {
		return nil
	}
	previousValue := strings.TrimSpace(metadata.Effect)
	previousPresent := scalarIsPresent(metadata.Effect, metadata.effectPresent)
	previousRank := metadata.effectRank
	previousOrigin := metadata.effectOrigin
	recordStringFieldCandidate(metadata, "effect", incoming, true, rank, origin, reviewReason)
	if err := mergeRankedStringValue(
		&metadata.Effect, &metadata.effectPresent, &metadata.effectRank, &metadata.effectOrigin,
		incoming, true, rank, origin, path, "effect",
	); err != nil {
		return err
	}
	if scalarIncomingWon(previousValue, previousPresent, previousRank, previousOrigin, incoming, true, rank, origin, metadata.effectOrigin) {
		metadata.EffectSource = strings.TrimSpace(effectSource)
	}
	return nil
}

func mergeRisk(metadata *ToolMetadata, incoming string, rank int, origin, reviewReason, path string) error {
	return mergeRiskValue(metadata, incoming, strings.TrimSpace(incoming) != "", rank, origin, reviewReason, path)
}

func mergeRiskValue(metadata *ToolMetadata, incoming string, incomingPresent bool, rank int, origin, reviewReason, path string) error {
	if metadata == nil || !incomingPresent {
		return nil
	}
	recordStringFieldCandidate(metadata, "risk", incoming, true, rank, origin, reviewReason)
	return mergeRankedStringValue(
		&metadata.Risk, &metadata.riskPresent, &metadata.riskRank, &metadata.riskOrigin,
		incoming, true, rank, origin, path, "risk",
	)
}

func mergeConfirmation(metadata *ToolMetadata, incoming string, rank int, origin, reviewReason, path string) error {
	return mergeConfirmationValue(metadata, incoming, strings.TrimSpace(incoming) != "", rank, origin, reviewReason, path)
}

func mergeConfirmationValue(metadata *ToolMetadata, incoming string, incomingPresent bool, rank int, origin, reviewReason, path string) error {
	if metadata == nil || !incomingPresent {
		return nil
	}
	recordStringFieldCandidate(metadata, "confirmation", incoming, true, rank, origin, reviewReason)
	return mergeRankedStringValue(
		&metadata.Confirmation, &metadata.confirmationPresent, &metadata.confirmationRank, &metadata.confirmationOrigin,
		incoming, true, rank, origin, path, "confirmation",
	)
}

func scalarIncomingWon(previousValue string, previousPresent bool, previousRank int, previousOrigin, incoming string, incomingPresent bool, incomingRank int, incomingOrigin, finalOrigin string) bool {
	if !incomingPresent {
		return false
	}
	incoming = strings.TrimSpace(incoming)
	if !previousPresent || incomingRank > previousRank {
		return true
	}
	return incomingRank == previousRank && previousValue == incoming &&
		strings.TrimSpace(finalOrigin) == strings.TrimSpace(incomingOrigin) &&
		strings.TrimSpace(previousOrigin) != strings.TrimSpace(finalOrigin)
}

func selectionPrecedence(rank int) string {
	switch rank {
	case selectionRankReviewedManual:
		return selectionPrecedenceReviewedManual
	case selectionRankReviewedExplicit:
		return selectionPrecedenceReviewedExplicit
	case selectionRankExplicit:
		return selectionPrecedenceExplicit
	case selectionRankImported:
		return selectionPrecedenceImported
	case selectionRankUnreviewedExplicit:
		return selectionPrecedenceUnreviewedExplicit
	case selectionRankSkill:
		return selectionPrecedenceSkill
	case selectionRankMCPFallback:
		return selectionPrecedenceMCPFallback
	default:
		return selectionPrecedenceDefault
	}
}

func recordFieldCandidate(metadata *ToolMetadata, field, value string, rank int, source, reviewReason string) {
	recordStringFieldCandidate(metadata, field, value, strings.TrimSpace(value) != "", rank, source, reviewReason)
}

func recordStringFieldCandidate(metadata *ToolMetadata, field, value string, present bool, rank int, source, reviewReason string) {
	if !present {
		return
	}
	recordTypedFieldCandidateValue(metadata, field, strings.TrimSpace(value), true, rank, source, reviewReason)
}

func recordListFieldCandidate(metadata *ToolMetadata, field string, value []string, present bool, rank int, source, reviewReason string) {
	if !listIsPresent(value, present) {
		return
	}
	value = normalizeAuthoredStrings(value)
	recordTypedFieldCandidate(metadata, field, cloneStringList(value), rank, source, reviewReason)
}

func recordProductListCandidate(metadata *ProductMetadata, field string, value []string, present bool, rank int, source string) {
	if metadata == nil || !listIsPresent(value, present) {
		return
	}
	value = normalizeAuthoredStrings(value)
	if metadata.fieldCandidates == nil {
		metadata.fieldCandidates = map[string][]FieldCandidateProvenance{}
	}
	candidate := FieldCandidateProvenance{
		Value:      cloneStringList(value),
		Source:     firstNonEmpty(source, "unknown"),
		Precedence: selectionPrecedence(rank),
	}
	for _, existing := range metadata.fieldCandidates[field] {
		if fieldCandidateEqual(existing, candidate) {
			return
		}
	}
	metadata.fieldCandidates[field] = append(metadata.fieldCandidates[field], candidate)
}

func recordProductStringCandidate(metadata *ProductMetadata, field, value string, present bool, rank int, source string) {
	if metadata == nil || !present {
		return
	}
	value = strings.TrimSpace(value)
	if metadata.fieldCandidates == nil {
		metadata.fieldCandidates = map[string][]FieldCandidateProvenance{}
	}
	candidate := FieldCandidateProvenance{
		Value:      value,
		Source:     firstNonEmpty(source, "unknown"),
		Precedence: selectionPrecedence(rank),
	}
	for _, existing := range metadata.fieldCandidates[field] {
		if fieldCandidateEqual(existing, candidate) {
			return
		}
	}
	metadata.fieldCandidates[field] = append(metadata.fieldCandidates[field], candidate)
}

func recordTypedFieldCandidate(metadata *ToolMetadata, field string, value any, rank int, source, reviewReason string) {
	recordTypedFieldCandidateValue(metadata, field, value, value != nil, rank, source, reviewReason)
}

func recordTypedFieldCandidateValue(metadata *ToolMetadata, field string, value any, present bool, rank int, source, reviewReason string) {
	if metadata == nil || !present {
		return
	}
	if metadata.fieldCandidates == nil {
		metadata.fieldCandidates = map[string][]FieldCandidateProvenance{}
	}
	candidate := FieldCandidateProvenance{
		Value:        cloneFieldValue(value),
		Source:       firstNonEmpty(source, "unknown"),
		Precedence:   selectionPrecedence(rank),
		ReviewReason: strings.TrimSpace(reviewReason),
	}
	for index, existing := range metadata.fieldCandidates[field] {
		if fieldValueEqual(existing.Value, candidate.Value) && existing.Source == candidate.Source && existing.Precedence == candidate.Precedence {
			if existing.ReviewReason == "" && candidate.ReviewReason != "" {
				metadata.fieldCandidates[field][index].ReviewReason = candidate.ReviewReason
			}
			return
		}
	}
	metadata.fieldCandidates[field] = append(metadata.fieldCandidates[field], candidate)
}

func mergeFieldCandidateHistory(target *ToolMetadata, incoming ToolMetadata) {
	if target == nil || len(incoming.fieldCandidates) == 0 {
		return
	}
	if target.fieldCandidates == nil {
		target.fieldCandidates = map[string][]FieldCandidateProvenance{}
	}
	for field, candidates := range incoming.fieldCandidates {
		for _, candidate := range candidates {
			exists := false
			for _, current := range target.fieldCandidates[field] {
				if fieldCandidateEqual(current, candidate) {
					exists = true
					break
				}
			}
			if !exists {
				target.fieldCandidates[field] = append(target.fieldCandidates[field], candidate)
			}
		}
	}
}

// validateToolFieldCandidateConflicts checks every scalar and authored-list
// precedence layer, including non-winning layers. A higher-ranked candidate
// selects the final value, but it must not hide contradictory inputs at a
// lower rank.
func validateToolFieldCandidateConflicts(file File) error {
	productIDs := make([]string, 0, len(file.Products))
	for productID := range file.Products {
		productIDs = append(productIDs, productID)
	}
	sort.Strings(productIDs)
	for _, productID := range productIDs {
		if err := validateFieldCandidateConflicts("product "+productID, file.Products[productID].fieldCandidates); err != nil {
			return err
		}
	}
	paths := make([]string, 0, len(file.Tools))
	for path := range file.Tools {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := validateFieldCandidateConflicts(path, file.Tools[path].fieldCandidates); err != nil {
			return err
		}
	}
	return nil
}

func validateFieldCandidateConflicts(path string, history map[string][]FieldCandidateProvenance) error {
	fields := make([]string, 0, len(history))
	for field := range history {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	for _, field := range fields {
		candidates := append([]FieldCandidateProvenance(nil), history[field]...)
		sort.Slice(candidates, func(i, j int) bool {
			left, right := candidates[i], candidates[j]
			leftRank, rightRank := precedenceRank(left.Precedence), precedenceRank(right.Precedence)
			if leftRank != rightRank {
				return leftRank > rightRank
			}
			if left.Source != right.Source {
				return left.Source < right.Source
			}
			if !fieldValueEqual(left.Value, right.Value) {
				return fieldValueKey(left.Value) < fieldValueKey(right.Value)
			}
			return left.Precedence < right.Precedence
		})
		for start := 0; start < len(candidates); {
			rank := precedenceRank(candidates[start].Precedence)
			end := start + 1
			for end < len(candidates) && precedenceRank(candidates[end].Precedence) == rank {
				end++
			}
			reference := candidates[start]
			for _, candidate := range candidates[start+1 : end] {
				if !fieldValueEqual(candidate.Value, reference.Value) {
					return fmt.Errorf(
						"agent metadata conflict for %s field %s at precedence %d: %s from %s conflicts with %s from %s",
						path, field, rank,
						fieldValueDisplay(reference.Value), firstNonEmpty(reference.Source, "<unknown>"),
						fieldValueDisplay(candidate.Value), firstNonEmpty(candidate.Source, "<unknown>"),
					)
				}
			}
			start = end
		}
	}
	return nil
}

func fieldWinnerReviewReason(metadata ToolMetadata, field, value, source string, rank int) string {
	return fieldWinnerReviewReasonValue(metadata, field, strings.TrimSpace(value), source, rank)
}

func fieldWinnerReviewReasonValue(metadata ToolMetadata, field string, value any, source string, rank int) string {
	precedence := selectionPrecedence(rank)
	for _, candidate := range metadata.fieldCandidates[field] {
		if fieldValueEqual(candidate.Value, value) && candidate.Source == firstNonEmpty(source, "unknown") && candidate.Precedence == precedence {
			return candidate.ReviewReason
		}
	}
	return ""
}

func syncToolFieldProvenance(metadata *ToolMetadata) {
	if metadata == nil {
		return
	}
	for _, field := range []string{"use_when", "avoid_when", "prerequisites", "tips", "workflow_refs", "examples"} {
		coalesceSkillListCandidates(metadata.fieldCandidates, field)
	}
	provenance := map[string]FieldProvenance{}
	addValue := func(field string, value any, present bool, source string, rank int) {
		if !present {
			return
		}
		source = firstNonEmpty(source, "unknown")
		reviewReason := fieldWinnerReviewReasonValue(*metadata, field, value, source, rank)
		recordTypedFieldCandidateValue(metadata, field, value, true, rank, source, reviewReason)
		winner := FieldCandidateProvenance{
			Value:        cloneFieldValue(value),
			Source:       source,
			Precedence:   selectionPrecedence(rank),
			ReviewReason: reviewReason,
		}
		candidates := make([]FieldCandidateProvenance, 0, len(metadata.fieldCandidates[field]))
		for _, candidate := range metadata.fieldCandidates[field] {
			candidate.Selected = fieldValueEqual(candidate.Value, winner.Value) && candidate.Source == winner.Source && candidate.Precedence == winner.Precedence
			candidates = append(candidates, candidate)
		}
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].Selected != candidates[j].Selected {
				return candidates[i].Selected
			}
			leftRank := precedenceRank(candidates[i].Precedence)
			rightRank := precedenceRank(candidates[j].Precedence)
			if leftRank != rightRank {
				return leftRank > rightRank
			}
			if candidates[i].Source != candidates[j].Source {
				return candidates[i].Source < candidates[j].Source
			}
			return fieldValueKey(candidates[i].Value) < fieldValueKey(candidates[j].Value)
		})
		provenance[field] = FieldProvenance{
			Value:        cloneFieldValue(winner.Value),
			Source:       source,
			Precedence:   selectionPrecedence(rank),
			Resolution:   "highest_precedence",
			ReviewReason: reviewReason,
			Candidates:   candidates,
		}
	}
	addString := func(field, value string, present bool, source string, rank int) {
		addValue(field, strings.TrimSpace(value), present, source, rank)
	}
	addList := func(field string, value []string, present bool, source string, rank int) {
		if !listIsPresent(value, present) {
			return
		}
		value = normalizeAuthoredStrings(value)
		rank = effectiveSelectionListRank(value, true, rank)
		source = firstNonEmpty(source, "unknown")
		reviewReason := fieldWinnerReviewReasonValue(*metadata, field, value, source, rank)
		recordListFieldCandidate(metadata, field, value, true, rank, source, reviewReason)
		winner := FieldCandidateProvenance{
			Value:        cloneStringList(value),
			Source:       source,
			Precedence:   selectionPrecedence(rank),
			ReviewReason: reviewReason,
		}
		candidates := make([]FieldCandidateProvenance, 0, len(metadata.fieldCandidates[field]))
		for _, candidate := range metadata.fieldCandidates[field] {
			candidate.Selected = fieldValueEqual(candidate.Value, winner.Value) && candidate.Source == winner.Source && candidate.Precedence == winner.Precedence
			candidates = append(candidates, candidate)
		}
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].Selected != candidates[j].Selected {
				return candidates[i].Selected
			}
			leftRank := precedenceRank(candidates[i].Precedence)
			rightRank := precedenceRank(candidates[j].Precedence)
			if leftRank != rightRank {
				return leftRank > rightRank
			}
			if candidates[i].Source != candidates[j].Source {
				return candidates[i].Source < candidates[j].Source
			}
			return fieldValueKey(candidates[i].Value) < fieldValueKey(candidates[j].Value)
		})
		provenance[field] = FieldProvenance{
			Value:        winner.Value,
			Source:       source,
			Precedence:   selectionPrecedence(rank),
			Resolution:   "highest_precedence",
			ReviewReason: reviewReason,
			Candidates:   candidates,
		}
	}
	addString("effect", metadata.Effect, scalarIsPresent(metadata.Effect, metadata.effectPresent), firstNonEmpty(metadata.effectOrigin, metadata.EffectSource), metadata.effectRank)
	addString("risk", metadata.Risk, scalarIsPresent(metadata.Risk, metadata.riskPresent), metadata.riskOrigin, metadata.riskRank)
	addString("confirmation", metadata.Confirmation, scalarIsPresent(metadata.Confirmation, metadata.confirmationPresent), metadata.confirmationOrigin, metadata.confirmationRank)
	addValue("interface_ref", interfaceRefCandidateValue(metadata.InterfaceRef), metadata.InterfaceRef != nil || metadata.interfaceRefPresent, metadata.interfaceRefOrigin, metadata.interfaceRefRank)
	addString("interface_mode", metadata.InterfaceMode, scalarIsPresent(metadata.InterfaceMode, metadata.interfaceModePresent), metadata.interfaceModeOrigin, metadata.interfaceModeRank)
	addString("availability", metadata.Availability, scalarIsPresent(metadata.Availability, metadata.availabilityPresent), metadata.availabilityOrigin, metadata.availabilityRank)
	addString("interface_reason", metadata.InterfaceReason, scalarIsPresent(metadata.InterfaceReason, metadata.interfaceReasonPresent), metadata.interfaceReasonOrigin, metadata.interfaceReasonRank)
	addString("agent_summary", metadata.AgentSummary, scalarIsPresent(metadata.AgentSummary, metadata.agentSummaryPresent), firstNonEmpty(metadata.agentSummaryOrigin, metadata.AgentSummarySource), metadata.agentSummaryRank)
	addString("idempotency", metadata.Idempotency, scalarIsPresent(metadata.Idempotency, metadata.idempotencyPresent), metadata.idempotencyOrigin, metadata.idempotencyRank)
	if metadata.Reviewed != nil {
		addValue("reviewed", *metadata.Reviewed, true, metadata.reviewedOrigin, metadata.reviewedRank)
	}
	addList("use_when", metadata.UseWhen, metadata.useWhenPresent, metadata.useWhenOrigin, metadata.useWhenRank)
	addList("avoid_when", metadata.AvoidWhen, metadata.avoidWhenPresent, metadata.avoidWhenOrigin, metadata.avoidWhenRank)
	addList("prerequisites", metadata.Prerequisites, metadata.prerequisitesPresent, metadata.prerequisitesOrigin, metadata.prerequisitesRank)
	addList("tips", metadata.Tips, metadata.tipsPresent, metadata.tipsOrigin, metadata.tipsRank)
	addList("workflow_refs", metadata.WorkflowRefs, metadata.workflowRefsPresent, metadata.workflowRefsOrigin, metadata.workflowRefsRank)
	addList("examples", metadata.Examples, metadata.examplesPresent, metadata.examplesOrigin, metadata.examplesRank)
	if len(provenance) == 0 {
		metadata.FieldProvenance = nil
		return
	}
	metadata.FieldProvenance = provenance
}

func syncProductFieldProvenance(metadata *ProductMetadata) {
	if metadata == nil {
		return
	}
	for _, field := range []string{"use_when", "avoid_when"} {
		coalesceSkillListCandidates(metadata.fieldCandidates, field)
	}
	provenance := map[string]FieldProvenance{}
	if scalarIsPresent(metadata.AgentSummary, metadata.agentSummaryPresent) {
		value := strings.TrimSpace(metadata.AgentSummary)
		rank := metadata.agentSummaryRank
		source := firstNonEmpty(metadata.agentSummaryOrigin, metadata.AgentSummarySource, "unknown")
		recordProductStringCandidate(metadata, "agent_summary", value, true, rank, source)
		winner := FieldCandidateProvenance{
			Value:      value,
			Source:     source,
			Precedence: selectionPrecedence(rank),
		}
		candidates := make([]FieldCandidateProvenance, 0, len(metadata.fieldCandidates["agent_summary"]))
		for _, candidate := range metadata.fieldCandidates["agent_summary"] {
			candidate.Selected = fieldValueEqual(candidate.Value, winner.Value) && candidate.Source == winner.Source && candidate.Precedence == winner.Precedence
			candidates = append(candidates, candidate)
		}
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].Selected != candidates[j].Selected {
				return candidates[i].Selected
			}
			leftRank, rightRank := precedenceRank(candidates[i].Precedence), precedenceRank(candidates[j].Precedence)
			if leftRank != rightRank {
				return leftRank > rightRank
			}
			if candidates[i].Source != candidates[j].Source {
				return candidates[i].Source < candidates[j].Source
			}
			return fieldValueKey(candidates[i].Value) < fieldValueKey(candidates[j].Value)
		})
		provenance["agent_summary"] = FieldProvenance{
			Value:      winner.Value,
			Source:     source,
			Precedence: selectionPrecedence(rank),
			Resolution: "highest_precedence",
			Candidates: candidates,
		}
	}
	for _, list := range []struct {
		field   string
		value   []string
		present bool
		source  string
		rank    int
	}{
		{"use_when", metadata.UseWhen, metadata.useWhenPresent, metadata.useWhenOrigin, metadata.useWhenRank},
		{"avoid_when", metadata.AvoidWhen, metadata.avoidWhenPresent, metadata.avoidWhenOrigin, metadata.avoidWhenRank},
	} {
		if !listIsPresent(list.value, list.present) {
			continue
		}
		value := normalizeAuthoredStrings(list.value)
		rank := effectiveSelectionListRank(value, true, list.rank)
		source := firstNonEmpty(list.source, "unknown")
		recordProductListCandidate(metadata, list.field, value, true, rank, source)
		winner := FieldCandidateProvenance{
			Value:      cloneStringList(value),
			Source:     source,
			Precedence: selectionPrecedence(rank),
		}
		candidates := make([]FieldCandidateProvenance, 0, len(metadata.fieldCandidates[list.field]))
		for _, candidate := range metadata.fieldCandidates[list.field] {
			candidate.Selected = fieldValueEqual(candidate.Value, winner.Value) && candidate.Source == winner.Source && candidate.Precedence == winner.Precedence
			candidates = append(candidates, candidate)
		}
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].Selected != candidates[j].Selected {
				return candidates[i].Selected
			}
			leftRank, rightRank := precedenceRank(candidates[i].Precedence), precedenceRank(candidates[j].Precedence)
			if leftRank != rightRank {
				return leftRank > rightRank
			}
			if candidates[i].Source != candidates[j].Source {
				return candidates[i].Source < candidates[j].Source
			}
			return fieldValueKey(candidates[i].Value) < fieldValueKey(candidates[j].Value)
		})
		provenance[list.field] = FieldProvenance{
			Value:      winner.Value,
			Source:     source,
			Precedence: selectionPrecedence(rank),
			Resolution: "highest_precedence",
			Candidates: candidates,
		}
	}
	if len(provenance) == 0 {
		metadata.FieldProvenance = nil
		return
	}
	metadata.FieldProvenance = provenance
}

func precedenceRank(precedence string) int {
	switch strings.TrimSpace(precedence) {
	case selectionPrecedenceReviewedManual:
		return selectionRankReviewedManual
	case selectionPrecedenceReviewedExplicit:
		return selectionRankReviewedExplicit
	case selectionPrecedenceExplicit:
		return selectionRankExplicit
	case selectionPrecedenceImported:
		return selectionRankImported
	case selectionPrecedenceUnreviewedExplicit:
		return selectionRankUnreviewedExplicit
	case selectionPrecedenceSkill:
		return selectionRankSkill
	case selectionPrecedenceMCPFallback:
		return selectionRankMCPFallback
	default:
		return selectionRankDefault
	}
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
	if strings.TrimSpace(opts.ManualHintsPath) != "" {
		paths = append(paths, resolvePath(opts.Root, opts.ManualHintsPath))
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
				return len(matches[i]) > len(matches[j])
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

func parseExamples(out *File, known []string, references []commandReference, source string, maxExamples int, origins sourceTracker) error {
	if len(known) == 0 {
		return nil
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
			_ = appendToolListValue(&metadata, "use_when", &metadata.UseWhen, &metadata.useWhenPresent, &metadata.useWhenRank, &metadata.useWhenOrigin, reference.commentIntent, selectionRankSkill, source, path)
			metadata.SourceRefs = append(metadata.SourceRefs, source)
		}
		if len(metadata.Examples) < maxExamples {
			_ = appendToolListValue(&metadata, "examples", &metadata.Examples, &metadata.examplesPresent, &metadata.examplesRank, &metadata.examplesOrigin, line, selectionRankSkill, source, path)
			metadata.SourceRefs = append(metadata.SourceRefs, source)
		}
		ensureEffect(&metadata, path)
		if err := applyExplicitCommentSafety(&metadata, reference.commentIntent, source, path); err != nil {
			return err
		}
		out.Tools[path] = metadata
		origins.add(path, source, reference.line)
	}
	return nil
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

func applyExplicitCommentSafety(metadata *ToolMetadata, intent, source, path string) error {
	if metadata == nil || metadata.Effect == "" || metadata.Effect == "read" {
		return nil
	}
	explicitRisk := false
	for _, marker := range []string{"不可逆", "不可恢复", "不能撤销", "二次确认", "立即失效", "需确认", "需要确认", "高风险", "高影响"} {
		if strings.Contains(intent, marker) {
			explicitRisk = true
			break
		}
	}
	if !explicitRisk {
		return nil
	}
	const reason = "Skill command comment explicitly marks the operation as risky"
	if err := mergeEffect(metadata, dangerEffect(intent), "skill-comment", selectionRankSkill, source, reason, path); err != nil {
		return err
	}
	if err := mergeRisk(metadata, "high", selectionRankSkill, source, reason, path); err != nil {
		return err
	}
	return mergeConfirmation(metadata, "user_required", selectionRankSkill, source, reason, path)
}

func parseDangerRules(out *File, body, source string, stats *Stats, origins sourceTracker) error {
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
			const reason = "Skill danger table explicitly reviews this operation"
			if err := mergeEffect(&metadata, dangerEffect(columns[2]), "skill-explicit", selectionRankSkill, source, reason, path); err != nil {
				return err
			}
			if err := mergeRisk(&metadata, "high", selectionRankSkill, source, reason, path); err != nil {
				return err
			}
			if err := mergeConfirmation(&metadata, "user_required", selectionRankSkill, source, reason, path); err != nil {
				return err
			}
			metadata.SourceRefs = append(metadata.SourceRefs, source)
			out.Tools[path] = metadata
			origins.add(path, source, startLine+index)
			stats.RiskRules++
		}
	}
	return nil
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
	if metadata == nil || scalarIsPresent(metadata.Effect, metadata.effectPresent) {
		return
	}
	if effect := classifyEffectPath(path); effect != "" {
		_ = mergeEffect(metadata, effect, "command-verb", selectionRankDefault, "command-verb", "", path)
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
	if metadata == nil || strings.TrimSpace(metadata.Effect) == "" {
		return
	}
	if !scalarIsPresent(metadata.Risk, metadata.riskPresent) {
		switch metadata.Effect {
		case "read":
			metadata.Risk = "low"
		case "destructive":
			metadata.Risk = "high"
		default:
			metadata.Risk = "medium"
		}
		metadata.riskRank = selectionRankDefault
		metadata.riskOrigin = "effect-default"
		metadata.riskPresent = true
		recordFieldCandidate(metadata, "risk", metadata.Risk, metadata.riskRank, metadata.riskOrigin, "")
	}
	if !scalarIsPresent(metadata.Confirmation, metadata.confirmationPresent) {
		if metadata.Risk == "high" {
			metadata.Confirmation = "user_required"
		} else {
			metadata.Confirmation = "not_required"
		}
		metadata.confirmationRank = selectionRankDefault
		metadata.confirmationOrigin = "risk-default"
		metadata.confirmationPresent = true
		recordFieldCandidate(metadata, "confirmation", metadata.Confirmation, metadata.confirmationRank, metadata.confirmationOrigin, "")
	}
	if !scalarIsPresent(metadata.Idempotency, metadata.idempotencyPresent) {
		if metadata.Effect == "read" {
			metadata.Idempotency = "idempotent"
		} else {
			metadata.Idempotency = "unknown"
		}
		metadata.idempotencyRank = selectionRankDefault
		metadata.idempotencyOrigin = "effect-default"
		metadata.idempotencyPresent = true
		recordFieldCandidate(metadata, "idempotency", metadata.Idempotency, metadata.idempotencyRank, metadata.idempotencyOrigin, "")
	}
}

func appendRankedStringListValue(target *[]string, targetPresent *bool, targetRank *int, targetOrigin *string, value string, rank int, source, path, field string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	current := normalizeAuthoredStrings(*target)
	if listIsPresent(*target, *targetPresent) && *targetRank == rank &&
		(strings.TrimSpace(*targetOrigin) == strings.TrimSpace(source) || rank == selectionRankSkill) {
		*target = normalizeAuthoredStrings(append(current, value))
		*targetPresent = true
		if rank == selectionRankSkill {
			// Parsed Skill rows are fragments of one logical authored array, not
			// independent whole-field candidates. Aggregate them before the
			// precedence resolver; same-rank conflict remains strict for every
			// complete candidate (hints and reconciled ToolMetadata).
			*targetOrigin = combinedSkillSource(*targetOrigin, source)
		}
		return nil
	}
	return mergeRankedStringList(target, targetPresent, targetRank, targetOrigin, []string{value}, true, rank, source, path, field)
}

func appendToolListValue(metadata *ToolMetadata, field string, target *[]string, targetPresent *bool, targetRank *int, targetOrigin *string, value string, rank int, source, path string) error {
	if metadata == nil {
		return nil
	}
	if err := appendRankedStringListValue(target, targetPresent, targetRank, targetOrigin, value, rank, source, path, field); err != nil {
		upsertToolListCandidate(metadata, field, []string{value}, rank, source)
		return err
	}
	candidate := []string{value}
	candidateSource := source
	if *targetRank == rank && (strings.TrimSpace(*targetOrigin) == strings.TrimSpace(source) || rank == selectionRankSkill) {
		candidate = *target
		candidateSource = *targetOrigin
	}
	upsertToolListCandidate(metadata, field, candidate, rank, candidateSource)
	return nil
}

func appendProductListValue(metadata *ProductMetadata, field string, target *[]string, targetPresent *bool, targetRank *int, targetOrigin *string, value string, rank int, source, path string) error {
	if metadata == nil {
		return nil
	}
	if err := appendRankedStringListValue(target, targetPresent, targetRank, targetOrigin, value, rank, source, path, field); err != nil {
		upsertProductListCandidate(metadata, field, []string{value}, rank, source)
		return err
	}
	candidate := []string{value}
	candidateSource := source
	if *targetRank == rank && (strings.TrimSpace(*targetOrigin) == strings.TrimSpace(source) || rank == selectionRankSkill) {
		candidate = *target
		candidateSource = *targetOrigin
	}
	upsertProductListCandidate(metadata, field, candidate, rank, candidateSource)
	return nil
}

func upsertToolListCandidate(metadata *ToolMetadata, field string, values []string, rank int, source string) {
	if metadata == nil {
		return
	}
	if metadata.fieldCandidates == nil {
		metadata.fieldCandidates = map[string][]FieldCandidateProvenance{}
	}
	metadata.fieldCandidates[field] = upsertListCandidate(metadata.fieldCandidates[field], values, rank, source)
}

func upsertProductListCandidate(metadata *ProductMetadata, field string, values []string, rank int, source string) {
	if metadata == nil {
		return
	}
	if metadata.fieldCandidates == nil {
		metadata.fieldCandidates = map[string][]FieldCandidateProvenance{}
	}
	metadata.fieldCandidates[field] = upsertListCandidate(metadata.fieldCandidates[field], values, rank, source)
}

func upsertListCandidate(candidates []FieldCandidateProvenance, values []string, rank int, source string) []FieldCandidateProvenance {
	values = normalizeAuthoredStrings(values)
	if len(values) == 0 {
		return candidates
	}
	source = firstNonEmpty(source, "unknown")
	precedence := selectionPrecedence(rank)
	candidate := FieldCandidateProvenance{Value: append([]string(nil), values...), Source: source, Precedence: precedence}
	if rank == selectionRankSkill {
		filtered := candidates[:0]
		for _, existing := range candidates {
			if existing.Precedence != precedence {
				filtered = append(filtered, existing)
			}
		}
		return append(filtered, candidate)
	}
	for index := range candidates {
		if candidates[index].Source == source && candidates[index].Precedence == precedence {
			candidates[index] = candidate
			return candidates
		}
	}
	return append(candidates, candidate)
}

func combinedSkillSource(existing, incoming string) string {
	parts := make([]string, 0, 2)
	for _, source := range []string{existing, incoming} {
		for _, item := range strings.Split(source, " + ") {
			if item = strings.TrimSpace(item); item != "" {
				parts = append(parts, item)
			}
		}
	}
	return strings.Join(normalizedStrings(parts), " + ")
}

func coalesceSkillListCandidates(history map[string][]FieldCandidateProvenance, field string) {
	if len(history) == 0 {
		return
	}
	candidates := history[field]
	indices := make([]int, 0, len(candidates))
	for index, candidate := range candidates {
		if candidate.Precedence == selectionPrecedenceSkill {
			indices = append(indices, index)
		}
	}
	if len(indices) <= 1 {
		return
	}
	sort.Slice(indices, func(i, j int) bool {
		return candidates[indices[i]].Source < candidates[indices[j]].Source
	})
	values := make([]string, 0)
	source := ""
	for _, index := range indices {
		if list, ok := candidates[index].Value.([]string); ok {
			values = append(values, list...)
		}
		source = combinedSkillSource(source, candidates[index].Source)
	}
	aggregated := FieldCandidateProvenance{
		Value:      normalizeAuthoredStrings(values),
		Source:     source,
		Precedence: selectionPrecedenceSkill,
	}
	filtered := candidates[:0]
	for _, candidate := range candidates {
		if candidate.Precedence != selectionPrecedenceSkill {
			filtered = append(filtered, candidate)
		}
	}
	history[field] = append(filtered, aggregated)
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
		_ = appendProductListValue(&metadata, "avoid_when", &metadata.AvoidWhen, &metadata.avoidWhenPresent, &metadata.avoidWhenRank, &metadata.avoidWhenOrigin, intent, selectionRankSkill, source, target)
		metadata.SourceRefs = append(metadata.SourceRefs, source)
		out.Products[target] = metadata
		return
	}
	metadata := out.Tools[target]
	_ = appendToolListValue(&metadata, "avoid_when", &metadata.AvoidWhen, &metadata.avoidWhenPresent, &metadata.avoidWhenRank, &metadata.avoidWhenOrigin, intent, selectionRankSkill, source, target)
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
	_ = appendProductListValue(&metadata, "use_when", &metadata.UseWhen, &metadata.useWhenPresent, &metadata.useWhenRank, &metadata.useWhenOrigin, intent, selectionRankSkill, source, productID)
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
	_ = appendToolListValue(&metadata, "use_when", &metadata.UseWhen, &metadata.useWhenPresent, &metadata.useWhenRank, &metadata.useWhenOrigin, intent, selectionRankSkill, source, path)
	metadata.SourceRefs = append(metadata.SourceRefs, source)
	ensureEffect(&metadata, path)
	out.Tools[path] = metadata
}

func normalizeFile(file *File, maxExamples int) {
	for key, metadata := range file.Products {
		metadata.AgentSummary = strings.TrimSpace(metadata.AgentSummary)
		metadata.AgentSummarySource = strings.TrimSpace(metadata.AgentSummarySource)
		metadata.UseWhen = normalizeAuthoredStrings(metadata.UseWhen)
		metadata.AvoidWhen = normalizeAuthoredStrings(metadata.AvoidWhen)
		preserveExplicitEmptyProductLists(&metadata)
		metadata.SourceRefs = normalizedStrings(metadata.SourceRefs)
		syncProductFieldProvenance(&metadata)
		file.Products[key] = metadata
	}
	for key, metadata := range file.Tools {
		ensureEffect(&metadata, key)
		applyDefaultSafety(&metadata)
		metadata.AgentSummary = strings.TrimSpace(metadata.AgentSummary)
		metadata.AgentSummarySource = strings.TrimSpace(metadata.AgentSummarySource)
		metadata.UseWhen = normalizeAuthoredStrings(metadata.UseWhen)
		metadata.AvoidWhen = normalizeAuthoredStrings(metadata.AvoidWhen)
		metadata.Prerequisites = normalizeAuthoredStrings(metadata.Prerequisites)
		metadata.Tips = normalizeAuthoredStrings(metadata.Tips)
		metadata.WorkflowRefs = normalizeAuthoredStrings(metadata.WorkflowRefs)
		metadata.Examples = normalizeAuthoredStrings(metadata.Examples)
		if maxExamples > 0 && len(metadata.Examples) > maxExamples {
			metadata.Examples = metadata.Examples[:maxExamples]
		}
		preserveExplicitEmptyToolLists(&metadata)
		normalizeListFieldCandidates(&metadata, maxExamples)
		metadata.SourceRefs = normalizedStrings(metadata.SourceRefs)
		if metadata.InterfaceRef != nil {
			metadata.InterfaceRef.ProductID = strings.TrimSpace(metadata.InterfaceRef.ProductID)
			metadata.InterfaceRef.RPCName = strings.TrimSpace(metadata.InterfaceRef.RPCName)
		}
		syncToolFieldProvenance(&metadata)
		file.Tools[key] = metadata
	}
}

// finalizeInterfaceDispositions applies the cross-field interface matrix only
// after every source and alias has been reconciled. Keeping it out of
// normalizeFile makes resolution associative: an early local candidate cannot
// erase a lower-ranked MCP ref before a later higher-ranked mode is merged.
func finalizeInterfaceDispositions(file *File) {
	if file == nil {
		return
	}
	for key, metadata := range file.Tools {
		mode := strings.TrimSpace(metadata.InterfaceMode)
		availability := strings.TrimSpace(metadata.Availability)
		if mode != "local" && mode != "composite" && availability != "unavailable" {
			continue
		}
		current := metadata.FieldProvenance["interface_ref"]
		allCandidates := append([]FieldCandidateProvenance(nil), current.Candidates...)
		allCandidates = append(allCandidates, current.OverriddenCandidates...)

		source := firstNonEmpty(metadata.interfaceModeOrigin, "interface_mode")
		rank := effectiveSelectionRank(metadata.InterfaceMode, metadata.interfaceModeRank)
		reason := "final interface mode " + mode + " forbids a direct MCP interface_ref"
		if availability == "unavailable" {
			source = firstNonEmpty(metadata.availabilityOrigin, "availability")
			rank = effectiveSelectionRank(metadata.Availability, metadata.availabilityRank)
			reason = "final availability unavailable forbids Agent invocation and a direct MCP interface_ref"
		}
		winner := FieldCandidateProvenance{
			Value:        nil,
			Source:       source,
			Precedence:   selectionPrecedence(rank),
			Selected:     true,
			ReviewReason: reason,
		}
		candidates := []FieldCandidateProvenance{winner}
		overridden := make([]FieldCandidateProvenance, 0, len(allCandidates))
		for _, candidate := range allCandidates {
			candidate.Selected = false
			if candidate.Value == nil || fieldStringValue(candidate.Value) == "" || fieldStringValue(candidate.Value) == "<none>" {
				candidate.Value = nil
				// The matrix winner already represents an authored nil from the
				// same source/rank. Keep one selected candidate, but retain a nil
				// authored at another source/rank as an auditable candidate.
				if candidate.Source != winner.Source || candidate.Precedence != winner.Precedence {
					candidates = append(candidates, candidate)
				}
				continue
			}
			overridden = append(overridden, candidate)
		}
		metadata.InterfaceRef = nil
		metadata.interfaceRefPresent = true
		if metadata.FieldProvenance == nil {
			metadata.FieldProvenance = map[string]FieldProvenance{}
		}
		metadata.FieldProvenance["interface_ref"] = FieldProvenance{
			Value:                winner.Value,
			Source:               winner.Source,
			Precedence:           winner.Precedence,
			Resolution:           "interface_disposition_matrix",
			ReviewReason:         reason,
			Candidates:           candidates,
			OverriddenCandidates: overridden,
		}
		file.Tools[key] = metadata
	}
}

// validateInterfaceDispositions keeps implementation mechanism and execution
// availability orthogonal. Only mcp/local/composite are modes; unavailable is
// an Agent execution gate and therefore always needs a reviewed explanation.
func validateInterfaceDispositions(file File) error {
	var problems []string
	for key, metadata := range file.Tools {
		mode := strings.TrimSpace(metadata.InterfaceMode)
		availability := strings.TrimSpace(metadata.Availability)
		reason := strings.TrimSpace(metadata.InterfaceReason)
		// The generator can be used for partial metadata audits which have not
		// selected an interface disposition yet. Once any interface field is
		// declared, however, it must satisfy the complete final matrix. The
		// release Catalog gate separately requires every effective command to
		// carry such a disposition.
		if mode == "" && availability == "" && reason == "" && metadata.InterfaceRef == nil {
			continue
		}
		if mode == "unavailable" {
			problems = append(problems, fmt.Sprintf("Agent metadata tool %s uses legacy interface_mode=unavailable; migrate to interface_mode=mcp, local, or composite with availability=unavailable", key))
			continue
		}
		if mode != "mcp" && mode != "local" && mode != "composite" {
			problems = append(problems, fmt.Sprintf("Agent metadata tool %s has unsupported interface_mode %q", key, mode))
			continue
		}
		if availability != "available" && availability != "unavailable" {
			problems = append(problems, fmt.Sprintf("Agent metadata tool %s has unsupported availability %q", key, availability))
			continue
		}
		if availability == "unavailable" {
			if metadata.InterfaceRef != nil {
				problems = append(problems, fmt.Sprintf("Agent metadata tool %s is unavailable but declares interface_ref", key))
			}
			if reason == "" {
				problems = append(problems, fmt.Sprintf("Agent metadata tool %s is unavailable without interface_reason", key))
			}
			continue
		}
		switch mode {
		case "mcp":
			if metadata.InterfaceRef == nil {
				problems = append(problems, fmt.Sprintf("Agent metadata tool %s is available mcp without interface_ref", key))
			}
		case "local":
			if metadata.InterfaceRef != nil {
				problems = append(problems, fmt.Sprintf("Agent metadata tool %s is available local but declares interface_ref", key))
			}
		case "composite":
			if metadata.InterfaceRef != nil {
				problems = append(problems, fmt.Sprintf("Agent metadata tool %s is available composite but declares a single interface_ref", key))
			}
			if reason == "" {
				problems = append(problems, fmt.Sprintf("Agent metadata tool %s is available composite without interface_reason", key))
			}
		}
	}
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("invalid final Agent interface disposition: %s", strings.Join(problems, "; "))
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

func normalizeAuthoredStrings(values []string) []string {
	if values == nil {
		return nil
	}
	values = uniqueStringsInOrder(values)
	if values == nil {
		return []string{}
	}
	return values
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
