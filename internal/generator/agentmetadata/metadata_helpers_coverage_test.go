// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package agentmetadata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageMetadataMarshalAndProjectionEdges(t *testing.T) {
	bad := map[string]FieldProvenance{"bad": {Value: make(chan int)}}
	if _, err := json.Marshal(ProductMetadata{FieldProvenance: bad}); err == nil {
		t.Fatal("product metadata with unsupported provenance value succeeded")
	}
	if _, err := json.Marshal(ToolMetadata{FieldProvenance: bad}); err == nil {
		t.Fatal("tool metadata with unsupported provenance value succeeded")
	}

	seedEffectiveToolProjection(nil, nil)
	file := &File{}
	seedEffectiveToolProjection(file, map[string]string{
		"sample.tool": " dws sample item get ",
		"blank":       " ",
	})
	if _, ok := file.Tools["sample item get"]; !ok {
		t.Fatalf("seeded tools = %#v", file.Tools)
	}
	seedEffectiveToolProjection(file, map[string]string{"sample.tool": "sample item get"})
	if err := validateEffectiveToolProjection(File{}, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := validateEffectiveToolProjection(File{Tools: map[string]ToolMetadata{"unexpected": {}}}, Options{ToolPaths: map[string]string{"sample": "sample get"}}); err == nil {
		t.Fatal("unexpected tool projection succeeded")
	}
	if _, _, err := Generate(Options{
		HintsDir:           "hints",
		CanonicalToolPaths: map[string]string{"sample": "sample get"},
		ToolPaths:          map[string]string{"sample": "sample get"},
		ProductIDs:         map[string]bool{"sample": true},
	}); err == nil || !strings.Contains(err.Error(), "Cobra-bound") {
		t.Fatalf("bound registry error = %v", err)
	}
}

func TestCrossPlatformCoverageMetadataCandidateAndMergeEdges(t *testing.T) {
	seedToolFieldCandidates(nil)
	effectiveSelectionScalarRank("", true, selectionRankDefault)
	effectiveSelectionListRank(nil, true, selectionRankDefault)
	if err := mergeRankedInterfaceRef(nil, ToolMetadata{}, "sample"); err != nil {
		t.Fatal(err)
	}
	if err := mergeRankedInterfaceRef(&ToolMetadata{}, ToolMetadata{}, "sample"); err != nil {
		t.Fatal(err)
	}
	refTarget := ToolMetadata{}
	if err := mergeRankedInterfaceRef(&refTarget, ToolMetadata{InterfaceRef: &InterfaceRef{ProductID: "sample", RPCName: "get"}}, "sample"); err != nil {
		t.Fatal(err)
	}
	seededRef := ToolMetadata{InterfaceRef: &InterfaceRef{ProductID: "sample", RPCName: "get"}}
	seedToolFieldCandidates(&seededRef)

	preserveExplicitEmptyToolLists(nil)
	tool := ToolMetadata{
		useWhenPresent: true, avoidWhenPresent: true, prerequisitesPresent: true,
		tipsPresent: true, workflowRefsPresent: true, examplesPresent: true,
	}
	preserveExplicitEmptyToolLists(&tool)
	if tool.UseWhen == nil || tool.AvoidWhen == nil || tool.Prerequisites == nil || tool.Tips == nil || tool.WorkflowRefs == nil || tool.Examples == nil {
		t.Fatalf("explicit empty tool lists were not preserved: %#v", tool)
	}
	preserveExplicitEmptyProductLists(nil)
	product := ProductMetadata{useWhenPresent: true, avoidWhenPresent: true}
	preserveExplicitEmptyProductLists(&product)
	if product.UseWhen == nil || product.AvoidWhen == nil {
		t.Fatalf("explicit empty product lists were not preserved: %#v", product)
	}

	if got := fieldValueKey(make(chan int)); !strings.HasPrefix(got, "0x") {
		t.Fatalf("fallback field key = %q", got)
	}
	normalizeListFieldCandidates(nil, 1)
	tool.fieldCandidates = map[string][]FieldCandidateProvenance{
		"scalar":   {{Value: "value"}},
		"examples": {{Value: []string{" one ", "two"}}},
	}
	normalizeListFieldCandidates(&tool, 1)
	if got := tool.fieldCandidates["examples"][0].Value.([]string); len(got) != 1 || got[0] != "one" {
		t.Fatalf("normalized examples = %#v", got)
	}

	if err := mergeEffectValue(nil, "read", true, "x", 1, "x", "", "sample"); err != nil {
		t.Fatal(err)
	}
	if err := mergeEffectValue(&tool, "read", false, "x", 1, "x", "", "sample"); err != nil {
		t.Fatal(err)
	}
	if scalarIncomingWon("", false, 0, "", "x", false, 1, "x", "x") {
		t.Fatal("absent incoming value won")
	}
	conflictingEffect := ToolMetadata{Effect: "read", effectPresent: true, effectRank: selectionRankSkill, effectOrigin: "old"}
	if err := mergeEffectValue(&conflictingEffect, "write", true, "new", selectionRankSkill, "new", "", "sample"); err == nil {
		t.Fatal("effect conflict succeeded")
	}

	recordProductStringCandidate(nil, "summary", "x", true, 1, "x")
	recordProductStringCandidate(&product, "summary", "x", false, 1, "x")
	recordTypedFieldCandidateValue(nil, "field", "x", true, 1, "x", "")
	recordTypedFieldCandidateValue(&tool, "field", "x", false, 1, "x", "")
	recordTypedFieldCandidateValue(&tool, "field", "x", true, 1, "x", "")
	recordTypedFieldCandidateValue(&tool, "field", "x", true, 1, "x", "reviewed")
	if tool.fieldCandidates["field"][0].ReviewReason != "reviewed" {
		t.Fatalf("candidate review reason = %#v", tool.fieldCandidates["field"])
	}
}

func TestCrossPlatformCoverageMetadataConflictAndProvenanceEdges(t *testing.T) {
	conflict := map[string][]FieldCandidateProvenance{
		"field": {
			{Value: "b", Source: "same", Precedence: selectionPrecedenceSkill},
			{Value: "a", Source: "same", Precedence: selectionPrecedenceSkill},
		},
	}
	if err := validateFieldCandidateConflicts("sample", conflict); err == nil {
		t.Fatal("same-rank field conflict succeeded")
	}
	if err := validateFieldCandidateConflicts("sample", map[string][]FieldCandidateProvenance{
		"field": {
			{Value: "same", Source: "same", Precedence: "unknown-default"},
			{Value: "same", Source: "same", Precedence: selectionPrecedenceDefault},
		},
	}); err != nil {
		t.Fatalf("equivalent default candidates conflict: %v", err)
	}
	file := File{Products: map[string]ProductMetadata{"sample": {fieldCandidates: conflict}}}
	if err := validateToolFieldCandidateConflicts(file); err == nil {
		t.Fatal("product field conflict succeeded")
	}

	syncToolFieldProvenance(nil)
	syncProductFieldProvenance(nil)
	emptyTool := ToolMetadata{}
	syncToolFieldProvenance(&emptyTool)
	if emptyTool.FieldProvenance != nil {
		t.Fatalf("empty tool provenance = %#v", emptyTool.FieldProvenance)
	}
	emptyProduct := ProductMetadata{}
	syncProductFieldProvenance(&emptyProduct)
	if emptyProduct.FieldProvenance != nil {
		t.Fatalf("empty product provenance = %#v", emptyProduct.FieldProvenance)
	}

	product := ProductMetadata{
		AgentSummary:        "winner",
		agentSummaryPresent: true,
		agentSummaryRank:    selectionRankReviewedExplicit,
		agentSummaryOrigin:  "winner",
		UseWhen:             []string{"winner"},
		useWhenPresent:      true,
		useWhenRank:         selectionRankReviewedExplicit,
		useWhenOrigin:       "winner",
		fieldCandidates: map[string][]FieldCandidateProvenance{
			"agent_summary": {
				{Value: "winner", Source: "winner", Precedence: selectionPrecedenceReviewedExplicit},
				{Value: "low", Source: "z", Precedence: selectionPrecedenceSkill},
				{Value: "other", Source: "a", Precedence: selectionPrecedenceSkill},
				{Value: "ranked", Source: "rank", Precedence: selectionPrecedenceImported},
				{Value: "value-z", Source: "same", Precedence: selectionPrecedenceUnreviewedExplicit},
				{Value: "value-a", Source: "same", Precedence: selectionPrecedenceUnreviewedExplicit},
			},
			"use_when": {
				{Value: []string{"winner"}, Source: "winner", Precedence: selectionPrecedenceReviewedExplicit},
				{Value: []string{"low"}, Source: "z", Precedence: selectionPrecedenceSkill},
				{Value: []string{"other"}, Source: "a", Precedence: selectionPrecedenceSkill},
				{Value: []string{"ranked"}, Source: "rank", Precedence: selectionPrecedenceImported},
				{Value: []string{"source-z"}, Source: "source-z", Precedence: selectionPrecedenceUnreviewedExplicit},
				{Value: []string{"source-a"}, Source: "source-a", Precedence: selectionPrecedenceUnreviewedExplicit},
				{Value: []string{"value-z"}, Source: "same", Precedence: selectionPrecedenceUnreviewedExplicit},
				{Value: []string{"value-a"}, Source: "same", Precedence: selectionPrecedenceUnreviewedExplicit},
			},
		},
	}
	syncProductFieldProvenance(&product)
	if len(product.FieldProvenance) != 2 {
		t.Fatalf("product provenance = %#v", product.FieldProvenance)
	}

	tool := ToolMetadata{
		Effect:         "read",
		effectPresent:  true,
		effectRank:     selectionRankReviewedExplicit,
		effectOrigin:   "winner",
		UseWhen:        []string{"winner"},
		useWhenPresent: true,
		useWhenRank:    selectionRankReviewedExplicit,
		useWhenOrigin:  "winner",
		fieldCandidates: map[string][]FieldCandidateProvenance{
			"effect": {
				{Value: "read", Source: "winner", Precedence: selectionPrecedenceReviewedExplicit},
				{Value: "write", Source: "z", Precedence: selectionPrecedenceSkill},
				{Value: "destructive", Source: "a", Precedence: selectionPrecedenceSkill},
				{Value: "same-z", Source: "same", Precedence: selectionPrecedenceImported},
				{Value: "same-a", Source: "same", Precedence: selectionPrecedenceImported},
			},
			"use_when": {
				{Value: []string{"winner"}, Source: "winner", Precedence: selectionPrecedenceReviewedExplicit},
				{Value: []string{"low"}, Source: "z", Precedence: selectionPrecedenceImported},
				{Value: []string{"other"}, Source: "a", Precedence: selectionPrecedenceSkill},
				{Value: []string{"source-z"}, Source: "source-z", Precedence: selectionPrecedenceImported},
				{Value: []string{"source-a"}, Source: "source-a", Precedence: selectionPrecedenceImported},
				{Value: []string{"same-z"}, Source: "same", Precedence: selectionPrecedenceUnreviewedExplicit},
				{Value: []string{"same-a"}, Source: "same", Precedence: selectionPrecedenceUnreviewedExplicit},
			},
		},
	}
	syncToolFieldProvenance(&tool)
	if len(tool.FieldProvenance) != 2 {
		t.Fatalf("tool provenance = %#v", tool.FieldProvenance)
	}

	boolValue := true
	for _, tc := range []struct {
		name  string
		left  ToolMetadata
		right ToolMetadata
	}{
		{"summary", ToolMetadata{AgentSummary: "a", agentSummaryPresent: true, agentSummaryRank: selectionRankExplicit, agentSummaryOrigin: "a"}, ToolMetadata{AgentSummary: "b", agentSummaryPresent: true, agentSummaryRank: selectionRankExplicit, agentSummaryOrigin: "b"}},
		{"effect", ToolMetadata{Effect: "read", effectPresent: true, effectRank: selectionRankExplicit, effectOrigin: "a"}, ToolMetadata{Effect: "write", effectPresent: true, effectRank: selectionRankExplicit, effectOrigin: "b"}},
		{"risk", ToolMetadata{Risk: "low", riskPresent: true, riskRank: selectionRankExplicit, riskOrigin: "a"}, ToolMetadata{Risk: "high", riskPresent: true, riskRank: selectionRankExplicit, riskOrigin: "b"}},
		{"confirmation", ToolMetadata{Confirmation: "not_required", confirmationPresent: true, confirmationRank: selectionRankExplicit, confirmationOrigin: "a"}, ToolMetadata{Confirmation: "user_required", confirmationPresent: true, confirmationRank: selectionRankExplicit, confirmationOrigin: "b"}},
		{"idempotency", ToolMetadata{Idempotency: "idempotent", idempotencyPresent: true, idempotencyRank: selectionRankExplicit, idempotencyOrigin: "a"}, ToolMetadata{Idempotency: "unknown", idempotencyPresent: true, idempotencyRank: selectionRankExplicit, idempotencyOrigin: "b"}},
		{"reviewed equal", ToolMetadata{Reviewed: &boolValue, reviewedRank: selectionRankExplicit, reviewedOrigin: "z"}, ToolMetadata{Reviewed: &boolValue, reviewedRank: selectionRankExplicit, reviewedOrigin: "a", AgentSummary: "x"}},
	} {
		t.Run("merge "+tc.name, func(t *testing.T) {
			_, err := mergeToolMetadata(tc.left, tc.right, "sample")
			if tc.name == "reviewed equal" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatal("same-rank merge conflict succeeded")
			}
		})
	}

	reconcileFile := &File{Tools: map[string]ToolMetadata{
		"a target": {AgentSummary: "a", agentSummaryPresent: true, agentSummaryRank: selectionRankExplicit, agentSummaryOrigin: "a"},
		"z alias":  {AgentSummary: "z", agentSummaryPresent: true, agentSummaryRank: selectionRankExplicit, agentSummaryOrigin: "z"},
	}}
	reconcileStats := &Stats{referenceReviews: map[string]ReferenceReview{
		"z alias": {Status: "alias", Target: "canonical", Reason: "fixture"},
	}}
	if err := reconcileSurface(reconcileFile, Options{ToolPaths: map[string]string{
		"a target": "primary", "canonical": "primary",
	}}, reconcileStats, sourceTracker{}); err == nil {
		t.Fatal("alias reconciliation conflict succeeded")
	}
}

func TestCrossPlatformCoverageMetadataSourceAndPathEdges(t *testing.T) {
	if _, _, err := generateFromSources(Options{ProductsDir: "definitely-missing-agentmetadata-products"}); err == nil {
		t.Fatal("default-root missing products succeeded")
	}
	root := t.TempDir()
	if _, err := loadSources(Options{Root: root, ProductsDir: "missing"}); err == nil || !strings.Contains(err.Error(), "walk product") {
		t.Fatalf("missing product directory error = %v", err)
	}
	products := filepath.Join(root, "products")
	if err := os.MkdirAll(products, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSources(Options{Root: root, ProductsDir: "products", HintsDir: "missing"}); err == nil || !strings.Contains(err.Error(), "walk Agent hint") {
		t.Fatalf("missing hints directory error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(products, "skip.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	same := filepath.Join(root, "same.md")
	if err := os.WriteFile(same, []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := loadSources(Options{Root: root, ProductsDir: "products", SkillPath: "same.md", IntentGuidePath: "same.md"})
	if err != nil || len(files) != 1 {
		t.Fatalf("deduplicated sources = %#v, %v", files, err)
	}
	if _, err := loadSources(Options{Root: root, ProductsDir: "products", ManualHintsPath: "missing.json"}); err == nil || !strings.Contains(err.Error(), "read") {
		t.Fatalf("missing source error = %v", err)
	}

	if got := sourceProductIDs(sourceFile{path: filepath.Join(root, "products", "sample", "guide.md")}, products, nil, nil); len(got) != 1 || got[0] != "sample" {
		t.Fatalf("nested product ids = %#v", got)
	}
	if got := sourceProductIDs(sourceFile{path: filepath.Join(products, "sample.md")}, products, []commandReference{{text: "dws sample get"}}, nil); len(got) != 1 || got[0] != "sample" {
		t.Fatalf("command product ids = %#v", got)
	}
	if got := sourceProductIDs(sourceFile{path: filepath.Join(products, "sample-long.md")}, products, nil, map[string]bool{"sample": true, "sample-long": true}); len(got) != 1 || got[0] != "sample-long" {
		t.Fatalf("surface product ids = %#v", got)
	}
	if got := sourceProductIDs(sourceFile{path: filepath.Join(products, ".md")}, products, nil, nil); got != nil {
		t.Fatalf("empty product ids = %#v", got)
	}
	if got := sourceProductIDs(sourceFile{path: filepath.Join(products, "fallback.md")}, products, nil, nil); len(got) != 1 || got[0] != "fallback" {
		t.Fatalf("fallback product ids = %#v", got)
	}

	if got := candidateToolPaths("sample", nil, 0); got != nil {
		t.Fatalf("zero-limit candidates = %#v", got)
	}
	if got := commandPathSimilarity("", "sample"); got != 0 {
		t.Fatalf("empty path similarity = %d", got)
	}
	abs := resolvePath(root, filepath.Join(root, "x"))
	if !filepath.IsAbs(abs) {
		t.Fatalf("absolute resolved path = %q", abs)
	}
	if got := displayPath(root, filepath.Dir(root)); !filepath.IsAbs(got) {
		t.Fatalf("outside display path = %q", got)
	}
	if got := firstCommandToken("dws"); got != "" {
		t.Fatalf("empty command token = %q", got)
	}
	if got := firstCommandToken("dws sample get"); got != "sample" {
		t.Fatalf("command token = %q", got)
	}
	if got := resolveToolPath([]string{"", "sample"}, "get", []string{"sample get"}); got != "sample get" {
		t.Fatalf("resolved tool path = %q", got)
	}
	if got := resolveToolPath([]string{"one", "two"}, "unknown", []string{"one get", "two get"}); got != "" {
		t.Fatalf("ambiguous tool path = %q", got)
	}
	if got := normalizeToolPath("sample", " "); got != "" {
		t.Fatalf("empty normalized tool path = %q", got)
	}
	if got := normalizeToolPath("sample", "dws sample get"); got != "sample get" {
		t.Fatalf("prefixed normalized tool path = %q", got)
	}
}

func TestCrossPlatformCoverageMetadataListAndDispositionEdges(t *testing.T) {
	var values []string
	present := false
	rank := 0
	origin := ""
	if err := appendRankedStringListValue(&values, &present, &rank, &origin, " ", 1, "x", "sample", "use_when"); err != nil {
		t.Fatal(err)
	}
	if err := appendToolListValue(nil, "use_when", &values, &present, &rank, &origin, "x", 1, "x", "sample"); err != nil {
		t.Fatal(err)
	}
	if err := appendProductListValue(nil, "use_when", &values, &present, &rank, &origin, "x", 1, "x", "sample"); err != nil {
		t.Fatal(err)
	}
	upsertToolListCandidate(nil, "field", []string{"x"}, 1, "x")
	upsertProductListCandidate(nil, "field", []string{"x"}, 1, "x")
	if got := upsertListCandidate(nil, nil, 1, "x"); got != nil {
		t.Fatalf("empty candidate = %#v", got)
	}
	candidates := []FieldCandidateProvenance{{Value: []string{"old"}, Source: "old", Precedence: selectionPrecedenceImported}}
	candidates = upsertListCandidate(candidates, []string{"skill"}, selectionRankSkill, "skill")
	candidates = upsertListCandidate(candidates, []string{"new"}, selectionRankImported, "old")
	candidates = upsertListCandidate(candidates, []string{"append"}, selectionRankExplicit, "new")
	if len(candidates) != 3 {
		t.Fatalf("upserted candidates = %#v", candidates)
	}
	toolConflict := ToolMetadata{UseWhen: []string{"old"}, useWhenPresent: true, useWhenRank: selectionRankExplicit, useWhenOrigin: "old"}
	if err := appendToolListValue(&toolConflict, "use_when", &toolConflict.UseWhen, &toolConflict.useWhenPresent, &toolConflict.useWhenRank, &toolConflict.useWhenOrigin, "new", selectionRankExplicit, "new", "sample"); err == nil {
		t.Fatal("append tool list conflict succeeded")
	}
	productConflict := ProductMetadata{UseWhen: []string{"old"}, useWhenPresent: true, useWhenRank: selectionRankExplicit, useWhenOrigin: "old"}
	if err := appendProductListValue(&productConflict, "use_when", &productConflict.UseWhen, &productConflict.useWhenPresent, &productConflict.useWhenRank, &productConflict.useWhenOrigin, "new", selectionRankExplicit, "new", "sample"); err == nil {
		t.Fatal("append product list conflict succeeded")
	}

	out := &File{Products: map[string]ProductMetadata{}, Tools: map[string]ToolMetadata{}}
	for _, target := range []string{"", "先追问"} {
		addTargetUse(out, target, "intent", "source")
	}
	addTargetAvoid(out, "", "intent", "source")
	addProductUse(out, "", "intent", "source")
	addToolUse(out, "sample get", "", "source")

	file := File{Tools: map[string]ToolMetadata{
		"plain": {},
		"local": {
			InterfaceMode: "local", interfaceModeOrigin: "mode", interfaceModeRank: selectionRankReviewedExplicit,
			FieldProvenance: map[string]FieldProvenance{"interface_ref": {Candidates: []FieldCandidateProvenance{
				{Value: nil, Source: "mode", Precedence: selectionPrecedenceReviewedExplicit},
				{Value: "<none>", Source: "other", Precedence: selectionPrecedenceSkill},
				{Value: "sample.get", Source: "mcp", Precedence: selectionPrecedenceMCPFallback},
			}}},
		},
		"unavailable": {InterfaceMode: "mcp", Availability: "unavailable", InterfaceReason: "offline", availabilityOrigin: "availability"},
	}}
	finalizeInterfaceDispositions(nil)
	finalizeInterfaceDispositions(&file)
	if len(file.Tools["local"].FieldProvenance["interface_ref"].OverriddenCandidates) != 1 {
		t.Fatalf("interface disposition = %#v", file.Tools["local"].FieldProvenance)
	}

	invalid := File{Tools: map[string]ToolMetadata{
		"bad-mode":    {InterfaceMode: "other", Availability: "available"},
		"legacy":      {InterfaceMode: "unavailable", Availability: "unavailable"},
		"bad-avail":   {InterfaceMode: "mcp", Availability: "other"},
		"missing-ref": {InterfaceMode: "mcp", Availability: "available"},
	}}
	if err := validateInterfaceDispositions(invalid); err == nil {
		t.Fatal("invalid interface matrix succeeded")
	}

	for _, effect := range []string{"destructive", "write"} {
		metadata := ToolMetadata{Effect: effect}
		applyDefaultSafety(&metadata)
		if metadata.Risk == "" || metadata.Confirmation == "" || metadata.Idempotency == "" {
			t.Fatalf("default safety for %s = %#v", effect, metadata)
		}
	}
	normalized := File{Products: map[string]ProductMetadata{}, Tools: map[string]ToolMetadata{
		"sample": {Examples: []string{"one", "two"}},
	}}
	normalizeFile(&normalized, 1)
	if len(normalized.Tools["sample"].Examples) != 1 {
		t.Fatalf("trimmed examples = %#v", normalized.Tools["sample"].Examples)
	}
}

func TestCrossPlatformCoverageMetadataParserConflictEdges(t *testing.T) {
	out := &File{Products: map[string]ProductMetadata{}, Tools: map[string]ToolMetadata{}}
	parseProductRouting(out, "## 意图判断决策树\n用户提到“empty” → `dws`", "source")

	dangerBody := "## 危险操作确认\n| 产品 | 命令 | 风险 |\n|---|---|---|\n| `sample` | `get` | 删除且不可逆 |"
	for _, tc := range []struct {
		name string
		tool ToolMetadata
	}{
		{"effect", ToolMetadata{Effect: "write", effectPresent: true, effectRank: selectionRankSkill, effectOrigin: "old"}},
		{"risk", ToolMetadata{Effect: "destructive", effectPresent: true, effectRank: selectionRankSkill, effectOrigin: "source", Risk: "low", riskPresent: true, riskRank: selectionRankSkill, riskOrigin: "old"}},
		{"confirmation", ToolMetadata{Effect: "destructive", effectPresent: true, effectRank: selectionRankSkill, effectOrigin: "source", Risk: "high", riskPresent: true, riskRank: selectionRankSkill, riskOrigin: "source", Confirmation: "not_required", confirmationPresent: true, confirmationRank: selectionRankSkill, confirmationOrigin: "old"}},
	} {
		t.Run("danger "+tc.name, func(t *testing.T) {
			file := &File{Products: map[string]ProductMetadata{}, Tools: map[string]ToolMetadata{"sample get": tc.tool}}
			err := parseDangerRules(file, dangerBody, "source", &Stats{}, sourceTracker{})
			if err == nil {
				t.Fatal("danger conflict succeeded")
			}
		})
	}
	if err := parseDangerRules(out, "## 危险操作确认\n| `sample` | no-command | high |\n| `dws` | `dws` | high |", "source", &Stats{}, sourceTracker{}); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		tool ToolMetadata
	}{
		{"effect", ToolMetadata{Effect: "destructive", effectPresent: true, effectRank: selectionRankSkill, effectOrigin: "old"}},
		{"risk", ToolMetadata{Effect: "write", effectPresent: true, effectRank: selectionRankSkill, effectOrigin: "source", Risk: "low", riskPresent: true, riskRank: selectionRankSkill, riskOrigin: "old"}},
		{"confirmation", ToolMetadata{Effect: "write", effectPresent: true, effectRank: selectionRankSkill, effectOrigin: "source", Risk: "high", riskPresent: true, riskRank: selectionRankSkill, riskOrigin: "source", Confirmation: "not_required", confirmationPresent: true, confirmationRank: selectionRankSkill, confirmationOrigin: "old"}},
	} {
		t.Run("comment "+tc.name, func(t *testing.T) {
			metadata := tc.tool
			err := applyExplicitCommentSafety(&metadata, "高风险且需要确认", "source", "sample get")
			if err == nil {
				t.Fatal("comment safety conflict succeeded")
			}
		})
	}

	file := &File{Tools: map[string]ToolMetadata{"sample get": {
		Effect: "destructive", effectPresent: true, effectRank: selectionRankSkill, effectOrigin: "old",
	}}}
	err := parseExamples(file, []string{"sample get"}, []commandReference{{text: "dws sample get", commentIntent: "高风险需要确认"}}, "source", 2, sourceTracker{})
	if err == nil {
		t.Fatal("example safety conflict succeeded")
	}
}

func TestCrossPlatformCoverageGenerateFromSourcesFailureEdges(t *testing.T) {
	base := func(t *testing.T) (string, Options) {
		t.Helper()
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "products"), 0o700); err != nil {
			t.Fatal(err)
		}
		writeManualFixtureFile(t, root, "empty.md", "# empty")
		return root, Options{Root: root, ProductsDir: "products", SkillPath: "empty.md", IntentGuidePath: "empty.md"}
	}

	t.Run("danger rules", func(t *testing.T) {
		root, opts := base(t)
		opts.SkillPath = "skill.md"
		writeManualFixtureFile(t, root, "skill.md", "## 危险操作确认\n| 产品 | 命令 | 风险 |\n|---|---|---|\n| `sample` | `get` | 删除 |\n| `sample` | `get` | 高风险写入 |")
		if _, _, err := generateFromSources(opts); err == nil {
			t.Fatal("conflicting danger source succeeded")
		}
	})

	t.Run("examples", func(t *testing.T) {
		root, opts := base(t)
		opts.SkillPath = "skill.md"
		writeManualFixtureFile(t, root, "skill.md", "## 危险操作确认\n| 产品 | 命令 | 风险 |\n|---|---|---|\n| `sample` | `delete` | 删除且不可逆 |")
		body := "## 使用场景\n- 用户提到“delete” → `sample delete`\n\n```bash\n# 高风险需要确认\ndws sample delete\n```"
		writeManualFixtureFile(t, root, "products/sample/guide.md", body)
		if _, _, err := generateFromSources(opts); err == nil {
			t.Fatal("conflicting example source succeeded")
		}
	})

	t.Run("interface metadata", func(t *testing.T) {
		root, opts := base(t)
		opts.InterfaceMetadataPath = "interface.json"
		writeManualFixtureFile(t, root, "interface.json", "{")
		if _, _, err := generateFromSources(opts); err == nil {
			t.Fatal("invalid interface metadata succeeded")
		}
	})

	t.Run("projected count", func(t *testing.T) {
		_, opts := base(t)
		opts.ToolPaths = map[string]string{"sample.tool": "sample get"}
		opts.SurfaceToolCount = 2
		if _, _, err := generateFromSources(opts); err == nil {
			t.Fatal("invalid projected count succeeded")
		}
	})

	t.Run("reviewed delivery", func(t *testing.T) {
		root := t.TempDir()
		writeSelectionFixture(t, root, true, `["dws sample item search --query value"]`)
		writeManualFixtureFile(t, root, "skills/mono/SKILL.md", "# empty")
		writeManualFixtureFile(t, root, "skills/mono/references/intent-guide.md", "# empty")
		if err := os.MkdirAll(filepath.Join(root, "skills/mono/references/products"), 0o700); err != nil {
			t.Fatal(err)
		}
		opts := selectionFixtureOptions(root)
		opts.ToolPaths = map[string]string{"other.tool": "other get"}
		opts.SurfaceToolCount = 1
		if _, _, err := generateFromSources(opts); err == nil || !strings.Contains(err.Error(), "selection Agent delivery") {
			t.Fatalf("reviewed delivery error = %v", err)
		}
	})

	t.Run("interface disposition", func(t *testing.T) {
		root := t.TempDir()
		writeSelectionFixture(t, root, true, `["dws sample item search --query value"]`)
		writeManualFixtureFile(t, root, "skills/mono/SKILL.md", "# empty")
		writeManualFixtureFile(t, root, "skills/mono/references/intent-guide.md", "# empty")
		if err := os.MkdirAll(filepath.Join(root, "skills/mono/references/products"), 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, "internal/cli/schema_hints/metadata/sample.json")
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		body = []byte(strings.Replace(string(body), `"runtime_gate":"none",`, `"runtime_gate":"none","interface_mode":"mcp","availability":"available",`, 1))
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := generateFromSources(selectionFixtureOptions(root)); err == nil || !strings.Contains(err.Error(), "invalid final Agent interface disposition") {
			t.Fatalf("interface disposition error = %v", err)
		}
	})
}
