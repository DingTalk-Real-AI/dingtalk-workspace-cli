// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestInterfaceSpecValidateDispositionMatrix(t *testing.T) {
	ref := &InterfaceRefSpec{ProductID: "calendar", RPCName: "get_event"}
	for _, test := range []struct {
		name       string
		spec       InterfaceSpec
		wantError  string
		executable bool
	}{
		{name: "mcp available", spec: InterfaceSpec{Mode: InterfaceModeMCP, Availability: InterfaceAvailable, Ref: ref}, executable: true},
		{name: "local available", spec: InterfaceSpec{Mode: InterfaceModeLocal, Availability: InterfaceAvailable}, executable: true},
		{name: "composite available", spec: InterfaceSpec{Mode: InterfaceModeComposite, Availability: InterfaceAvailable, Reason: "orchestrates operations"}, executable: true},
		{name: "mcp unavailable", spec: InterfaceSpec{Mode: InterfaceModeMCP, Availability: InterfaceUnavailable, Reason: "RPC temporarily disabled"}},
		{name: "local unavailable", spec: InterfaceSpec{Mode: InterfaceModeLocal, Availability: InterfaceUnavailable, Reason: "compatibility command frozen"}},
		{name: "composite unavailable", spec: InterfaceSpec{Mode: InterfaceModeComposite, Availability: InterfaceUnavailable, Reason: "workflow retired"}},
		{name: "legacy unavailable mode", spec: InterfaceSpec{Mode: InterfaceUnavailable, Availability: InterfaceUnavailable, Reason: "legacy"}, wantError: "legacy interface_mode=unavailable; migrate"},
		{name: "mcp available missing ref", spec: InterfaceSpec{Mode: InterfaceModeMCP, Availability: InterfaceAvailable}, wantError: "has no interface_ref"},
		{name: "local available with ref", spec: InterfaceSpec{Mode: InterfaceModeLocal, Availability: InterfaceAvailable, Ref: ref}, wantError: "must not declare interface_ref"},
		{name: "composite available missing reason", spec: InterfaceSpec{Mode: InterfaceModeComposite, Availability: InterfaceAvailable}, wantError: "must declare interface_reason"},
		{name: "unavailable with ref", spec: InterfaceSpec{Mode: InterfaceModeMCP, Availability: InterfaceUnavailable, Reason: "retired", Ref: ref}, wantError: "unavailable interface must not declare interface_ref"},
		{name: "unavailable missing reason", spec: InterfaceSpec{Mode: InterfaceModeLocal, Availability: InterfaceUnavailable}, wantError: "must declare interface_reason"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.spec.Validate("calendar.event_get")
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Validate() error = %v, want %q", err, test.wantError)
			}
			if got := test.spec.AgentExecutable(); got != test.executable {
				t.Fatalf("AgentExecutable() = %v, want %v", got, test.executable)
			}
		})
	}
}

func TestToolSpecFromRuntimeBuildsOneTypedResolvedContract(t *testing.T) {
	selected := true
	reviewed := false
	spec, err := ToolSpecFromRuntime(RuntimeToolSpecInput{
		Identity: ToolIdentitySpec{
			ProductID: " calendar ",
			Name:      " attendee_delete ",
			CLIName:   "delete",
			CLIPath:   " calendar   attendee  delete ",
			Aliases:   []string{"calendar attendee remove", "calendar attendee remove", ""},
			Source:    "native_annotation",
		},
		Display:     "Calendar",
		Title:       "Delete attendee",
		Description: "Deletes one attendee.",
		Parameters: []ParameterSpec{
			{Name: "yes", Type: "boolean", Required: false, Default: json.RawMessage("false")},
			{
				Name:        "event-id",
				Type:        "string",
				Property:    "eventId",
				Required:    false,
				CLIRequired: true,
				FieldProvenance: map[string]FieldProvenance{
					"required": {
						Value:      json.RawMessage("false"),
						Source:     "reviewed_manual_hint",
						Precedence: "reviewed_manual",
						Resolution: "highest_precedence",
						Candidates: []FieldCandidateProvenance{{
							Value:      json.RawMessage("false"),
							Source:     "reviewed_manual_hint",
							Precedence: "reviewed_manual",
							Selected:   &selected,
						}},
					},
				},
			},
		},
		Safety: SafetySpec{
			Effect:       "write",
			Risk:         "low",
			Confirmation: "not_required",
			Idempotency:  "non_idempotent",
		},
		Interface: InterfaceSpec{
			Ref:          &InterfaceRefSpec{ProductID: "calendar", RPCName: "delete_attendee"},
			Mode:         "mcp",
			Availability: "available",
		},
		Selection: SelectionSpec{
			UseWhen:        []string{"remove attendee", "delete attendee", "remove attendee"},
			AvoidWhen:      []string{"keep attendee"},
			Reviewed:       &reviewed,
			MetadataSource: "resolved-agent-metadata",
		},
	})
	if err != nil {
		t.Fatalf("ToolSpecFromRuntime() error = %v", err)
	}

	if got, want := spec.Identity.CanonicalPath, "calendar.attendee_delete"; got != want {
		t.Fatalf("canonical path = %q, want %q", got, want)
	}
	if got, want := spec.Identity.CLIPath, "calendar attendee delete"; got != want {
		t.Fatalf("CLI path = %q, want %q", got, want)
	}
	if got, want := spec.Identity.PrimaryCLIPath, spec.Identity.CLIPath; got != want {
		t.Fatalf("primary CLI path = %q, want %q", got, want)
	}
	if got, want := []string{spec.Parameters[0].Name, spec.Parameters[1].Name}, []string{"event-id", "yes"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("parameter order = %#v, want %#v", got, want)
	}
	if got, want := spec.Selection.UseWhen, []string{"remove attendee", "delete attendee"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("selection normalization = %#v, want %#v", got, want)
	}
	// The typed model is policy-neutral: a reviewed source may lower safety or
	// projected required semantics. Resolution policy is not re-applied here.
	if spec.Safety.Risk != "low" || spec.Safety.Confirmation != "not_required" || spec.Parameters[0].Required {
		t.Fatalf("typed assembly changed resolved values: %#v", spec)
	}
}

func TestToolSpecDryRunCapabilityProjectsAndRoundTripsAtomically(t *testing.T) {
	dryRun := &DryRunSpec{PreviewKind: DryRunPreviewDiff, RemoteReads: true}
	spec, err := ToolSpecFromRuntime(RuntimeToolSpecInput{
		Identity: ToolIdentitySpec{
			ProductID: "doc",
			Name:      "update",
			CLIName:   "update",
			CLIPath:   "doc update",
		},
		DryRun: dryRun,
		FieldProvenance: map[string]FieldProvenance{
			"dry_run": resolvedFieldProvenance(
				dryRun,
				"reviewed_manual_hint",
				"schema_manual_hints.json",
				"reviewed_manual",
				"highest_precedence",
				"reviewed dry-run capability",
			),
		},
	})
	if err != nil {
		t.Fatalf("ToolSpecFromRuntime() error = %v", err)
	}

	full, err := spec.ToPayload()
	if err != nil {
		t.Fatalf("ToPayload() error = %v", err)
	}
	delivered, ok := full["dry_run"].(map[string]any)
	if !ok {
		t.Fatalf("dry_run = %#v", full["dry_run"])
	}
	if got := schemaString(delivered["preview_kind"]); got != DryRunPreviewDiff {
		t.Fatalf("dry_run.preview_kind = %q", got)
	}
	if got, ok := delivered["remote_reads"].(bool); !ok || !got {
		t.Fatalf("dry_run.remote_reads = %#v", delivered["remote_reads"])
	}

	summary, err := spec.ToSummaryPayload()
	if err != nil {
		t.Fatalf("ToSummaryPayload() error = %v", err)
	}
	if !schemaJSONEqual(summary["dry_run"], full["dry_run"]) {
		t.Fatalf("summary dry_run = %#v, want %#v", summary["dry_run"], full["dry_run"])
	}

	registry, err := SchemaRegistryFromRuntime("runtime-command", []ProductSpec{{
		ID:    "doc",
		Name:  "Document",
		Tools: []ToolSpec{spec},
	}})
	if err != nil {
		t.Fatalf("SchemaRegistryFromRuntime() error = %v", err)
	}
	snapshotPayload, err := registry.ToSnapshotPayload()
	if err != nil {
		t.Fatalf("ToSnapshotPayload() error = %v", err)
	}
	loaded, _, err := schemaRegistryFromSnapshot(SchemaCatalogSnapshot{
		Catalog: snapshotPayload.Catalog,
		Tools:   snapshotPayload.Tools,
	})
	if err != nil {
		t.Fatalf("schemaRegistryFromSnapshot() error = %v", err)
	}
	loadedTool := loaded.Products[0].Tools[0]
	if loadedTool.DryRun == nil || *loadedTool.DryRun != *dryRun {
		t.Fatalf("round-tripped dry_run = %#v, want %#v", loadedTool.DryRun, dryRun)
	}
}

func TestToolSpecDryRunCapabilityIsPositiveOnlyAndStrict(t *testing.T) {
	base := RuntimeToolSpecInput{Identity: ToolIdentitySpec{
		ProductID: "sample",
		Name:      "run",
		CLIName:   "run",
		CLIPath:   "sample run",
	}}

	withoutCapability, err := ToolSpecFromRuntime(base)
	if err != nil {
		t.Fatalf("ToolSpecFromRuntime() error = %v", err)
	}
	payload, err := withoutCapability.ToPayload()
	if err != nil {
		t.Fatalf("ToPayload() error = %v", err)
	}
	if _, ok := payload["dry_run"]; ok {
		t.Fatalf("nil capability unexpectedly emitted dry_run: %#v", payload["dry_run"])
	}

	base.DryRun = &DryRunSpec{PreviewKind: "unsupported"}
	if _, err := ToolSpecFromRuntime(base); err == nil || !strings.Contains(err.Error(), "unknown preview_kind") {
		t.Fatalf("invalid preview_kind error = %v", err)
	}

	base.DryRun = &DryRunSpec{PreviewKind: DryRunPreviewInvocation}
	withCapability, err := ToolSpecFromRuntime(base)
	if err != nil {
		t.Fatalf("ToolSpecFromRuntime(valid dry_run) error = %v", err)
	}
	payload, err = withCapability.ToPayload()
	if err != nil {
		t.Fatalf("ToPayload(valid dry_run) error = %v", err)
	}
	dryRun := payload["dry_run"].(map[string]any)
	if _, ok := dryRun["remote_reads"]; ok {
		t.Fatalf("false remote_reads should be omitted: %#v", dryRun)
	}
	dryRun["unexpected"] = true
	if _, err := schemaToolSpecFromPayload(payload); err == nil || !strings.Contains(err.Error(), `unknown field "unexpected"`) {
		t.Fatalf("strict dry_run decode error = %v", err)
	}
}

func TestToolSpecDryRunProvenanceRejectsAtomicDrift(t *testing.T) {
	selected := true
	dryRun := &DryRunSpec{PreviewKind: DryRunPreviewRequest}
	_, err := ToolSpecFromRuntime(RuntimeToolSpecInput{
		Identity: ToolIdentitySpec{ProductID: "sample", Name: "run", CLIPath: "sample run"},
		DryRun:   dryRun,
		FieldProvenance: map[string]FieldProvenance{
			"dry_run": {
				Value:      json.RawMessage(`{"preview_kind":"plan"}`),
				Source:     "reviewed_manual_hint",
				Precedence: "reviewed_manual",
				Resolution: "highest_precedence",
				Candidates: []FieldCandidateProvenance{{
					Value:      json.RawMessage(`{"preview_kind":"plan"}`),
					Source:     "reviewed_manual_hint",
					Precedence: "reviewed_manual",
					Selected:   &selected,
				}},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "provenance winner does not equal final value") {
		t.Fatalf("dry_run provenance drift error = %v", err)
	}
}

func TestToolSpecToPayloadKeepsCompatibleFlatShape(t *testing.T) {
	selected := true
	reviewed := true
	spec, err := ToolSpecFromRuntime(RuntimeToolSpecInput{
		Identity: ToolIdentitySpec{
			ProductID:      "chat",
			Name:           "category_create_smart",
			CLIName:        "create-smart",
			CLIPath:        "chat category create-smart",
			PrimaryCLIPath: "chat category create-smart",
			Source:         "reviewed_manual_hint",
		},
		Display:     "Chat",
		Title:       "Create smart category",
		Description: "Create a smart category.",
		Parameters: []ParameterSpec{{
			Name:          "dry-run",
			Type:          "boolean",
			Description:   "Preview only.",
			Property:      "dryRun",
			Required:      false,
			Default:       json.RawMessage("false"),
			Example:       json.RawMessage("true"),
			Enum:          []string{"true", "false"},
			InterfaceType: "boolean",
			FieldProvenance: map[string]FieldProvenance{
				"required": {
					Value:        json.RawMessage("false"),
					Source:       "reviewed_manual_hint",
					Precedence:   "reviewed_manual",
					Resolution:   "highest_precedence",
					ReviewReason: "preview remains optional",
					Candidates: []FieldCandidateProvenance{{
						Value:      json.RawMessage("false"),
						Source:     "reviewed_manual_hint",
						Precedence: "reviewed_manual",
						Selected:   &selected,
					}},
				},
			},
		}},
		Safety: SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required"},
		Interface: InterfaceSpec{
			Ref:          &InterfaceRefSpec{ProductID: "chat", RPCName: "create_smart_category"},
			Mode:         "mcp",
			Availability: "available",
			Reason:       "reviewed interface",
		},
		Selection: SelectionSpec{
			AgentSummary:   "Create a category from rules.",
			UseWhen:        []string{"create smart category"},
			Examples:       []string{"dws chat category create-smart --dry-run"},
			Reviewed:       &reviewed,
			MetadataSource: "resolved-agent-metadata",
		},
		FieldProvenance: map[string]FieldProvenance{
			"canonical_path": {
				Value:      json.RawMessage(`"chat.category_create_smart"`),
				Source:     "reviewed_manual_hint",
				Precedence: "reviewed_manual",
				Resolution: "highest_precedence",
				Candidates: []FieldCandidateProvenance{{
					Value:      json.RawMessage(`"chat.category_create_smart"`),
					Source:     "reviewed_manual_hint",
					Precedence: "reviewed_manual",
					Selected:   &selected,
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("ToolSpecFromRuntime() error = %v", err)
	}
	payload, err := spec.ToPayload()
	if err != nil {
		t.Fatalf("ToPayload() error = %v", err)
	}

	for _, nested := range []string{"identity", "safety", "interface", "selection"} {
		if _, ok := payload[nested]; ok {
			t.Fatalf("payload unexpectedly changed wire shape with %q: %#v", nested, payload)
		}
	}
	if got := schemaString(payload["canonical_path"]); got != "chat.category_create_smart" {
		t.Fatalf("canonical_path = %q", got)
	}
	if got := schemaString(payload["risk"]); got != "medium" {
		t.Fatalf("risk = %q", got)
	}
	if got := schemaString(payload["confirmation"]); got != "user_required" {
		t.Fatalf("confirmation = %q", got)
	}
	ref, ok := payload["interface_ref"].(map[string]any)
	if !ok || schemaString(ref["rpc_name"]) != "create_smart_category" {
		t.Fatalf("interface_ref = %#v", payload["interface_ref"])
	}
	parameters, ok := payload["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters type = %T", payload["parameters"])
	}
	dryRun, ok := parameters["dry-run"].(map[string]any)
	if !ok {
		t.Fatalf("dry-run type = %T", parameters["dry-run"])
	}
	if value, ok := dryRun["default"].(bool); !ok || value {
		t.Fatalf("default = %#v", dryRun["default"])
	}
	if value, ok := dryRun["required"].(bool); !ok || value {
		t.Fatalf("required = %#v", dryRun["required"])
	}
	if _, ok := dryRun["field_provenance"].(map[string]any); !ok {
		t.Fatalf("parameter provenance = %#v", dryRun["field_provenance"])
	}
	if _, ok := payload["field_provenance"].(map[string]any); !ok {
		t.Fatalf("tool provenance = %#v", payload["field_provenance"])
	}
}

func TestSchemaRegistryToPayloadIsDeterministic(t *testing.T) {
	tool := func(product, name, cliPath string) ToolSpec {
		spec, err := ToolSpecFromRuntime(RuntimeToolSpecInput{Identity: ToolIdentitySpec{
			ProductID: product,
			Name:      name,
			CLIName:   name,
			CLIPath:   cliPath,
			Source:    "native_annotation",
		}})
		if err != nil {
			t.Fatalf("ToolSpecFromRuntime(%s.%s) error = %v", product, name, err)
		}
		return spec
	}
	registry := SchemaRegistry{
		Source: "runtime-command",
		Products: []ProductSpec{
			{ID: "zeta", Name: "Zeta", Tools: []ToolSpec{tool("zeta", "beta", "zeta beta"), tool("zeta", "alpha", "zeta alpha")}},
			{ID: "alpha", Name: "Alpha", Tools: []ToolSpec{tool("alpha", "read", "alpha read")}},
		},
	}

	first, err := registry.ToPayload()
	if err != nil {
		t.Fatalf("ToPayload() error = %v", err)
	}
	second, err := registry.ToPayload()
	if err != nil {
		t.Fatalf("second ToPayload() error = %v", err)
	}
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("payload is not deterministic:\n%s\n%s", firstJSON, secondJSON)
	}
	products := schemaMapSlice(first["products"])
	if got := []string{schemaString(products[0]["id"]), schemaString(products[1]["id"])}; !reflect.DeepEqual(got, []string{"alpha", "zeta"}) {
		t.Fatalf("product order = %#v", got)
	}
	zetaTools := schemaMapSlice(products[1]["tools"])
	if got := []string{schemaString(zetaTools[0]["canonical_path"]), schemaString(zetaTools[1]["canonical_path"])}; !reflect.DeepEqual(got, []string{"zeta.alpha", "zeta.beta"}) {
		t.Fatalf("tool order = %#v", got)
	}
}

func TestProductFieldProvenanceSurvivesTypedPayloadAndFailsOnDrift(t *testing.T) {
	provenance := resolvedFieldProvenance("Calendar operations", "manual", "schema_manual_hints.json", "reviewed_manual", "highest_precedence", "reviewed")
	product := ProductSpec{
		ID: "calendar",
		Selection: SelectionSpec{
			AgentSummary: "Calendar operations",
		},
		FieldProvenance: map[string]FieldProvenance{"agent_summary": provenance},
	}
	registry, err := SchemaRegistryFromRuntime("runtime-command", []ProductSpec{product})
	if err != nil {
		t.Fatalf("SchemaRegistryFromRuntime() error = %v", err)
	}
	payload, err := registry.Products[0].ToSummaryPayload()
	if err != nil {
		t.Fatalf("ToSummaryPayload() error = %v", err)
	}
	delivered, ok := payload["field_provenance"].(map[string]any)
	if !ok || delivered["agent_summary"] == nil {
		t.Fatalf("product field_provenance = %#v", payload["field_provenance"])
	}

	bad := product
	bad.FieldProvenance = cloneFieldProvenance(product.FieldProvenance)
	badWinner, _ := json.Marshal("Different summary")
	entry := bad.FieldProvenance["agent_summary"]
	entry.Value = badWinner
	bad.FieldProvenance["agent_summary"] = entry
	if _, err := SchemaRegistryFromRuntime("runtime-command", []ProductSpec{bad}); err == nil || !strings.Contains(err.Error(), "provenance winner does not equal final value") {
		t.Fatalf("drift error = %v", err)
	}
}

func TestSchemaRegistryIndexResolvesCanonicalCLIAndAlias(t *testing.T) {
	spec, err := ToolSpecFromRuntime(RuntimeToolSpecInput{Identity: ToolIdentitySpec{
		ProductID:      "calendar",
		Name:           "attendee_delete",
		CLIName:        "delete",
		CLIPath:        "calendar attendee delete",
		PrimaryCLIPath: "calendar attendee delete",
		Aliases:        []string{"calendar attendee remove"},
		Source:         "native_annotation",
	}})
	if err != nil {
		t.Fatalf("ToolSpecFromRuntime() error = %v", err)
	}
	registry, err := SchemaRegistryFromRuntime("runtime-command", []ProductSpec{{
		ID:    "calendar",
		Name:  "Calendar",
		Tools: []ToolSpec{spec},
	}})
	if err != nil {
		t.Fatalf("SchemaRegistryFromRuntime() error = %v", err)
	}
	index, err := registry.Index()
	if err != nil {
		t.Fatalf("Index() error = %v", err)
	}
	for _, path := range []string{
		"calendar.attendee_delete",
		"calendar attendee delete",
		" calendar   attendee remove ",
	} {
		resolved, ok := index.Resolve(path)
		if !ok || resolved.Identity.CanonicalPath != "calendar.attendee_delete" {
			t.Fatalf("Resolve(%q) = %#v, %v", path, resolved.Identity, ok)
		}
	}
	if got := index.CanonicalPaths(); !reflect.DeepEqual(got, []string{"calendar.attendee_delete"}) {
		t.Fatalf("CanonicalPaths() = %#v", got)
	}
	if _, ok := index.Resolve("calendar attendee unknown"); ok {
		t.Fatal("unknown CLI path unexpectedly resolved")
	}
}

func TestSchemaRegistryIndexRejectsAmbiguousCLIIdentity(t *testing.T) {
	build := func(name, cliPath string, aliases ...string) ToolSpec {
		spec, err := ToolSpecFromRuntime(RuntimeToolSpecInput{Identity: ToolIdentitySpec{
			ProductID: "chat",
			Name:      name,
			CLIName:   name,
			CLIPath:   cliPath,
			Aliases:   aliases,
		}})
		if err != nil {
			t.Fatalf("ToolSpecFromRuntime(%s) error = %v", name, err)
		}
		return spec
	}
	registry := SchemaRegistry{Products: []ProductSpec{{
		ID: "chat",
		Tools: []ToolSpec{
			build("create", "chat create", "chat legacy"),
			build("delete", "chat delete", "chat legacy"),
		},
	}}}
	if _, err := registry.Index(); err == nil || !strings.Contains(err.Error(), "resolves to both") {
		t.Fatalf("ambiguous index error = %v", err)
	}
}

func TestSchemaRegistrySnapshotUsesSameToolForSummaryAndFullExport(t *testing.T) {
	spec, err := ToolSpecFromRuntime(RuntimeToolSpecInput{
		Identity: ToolIdentitySpec{
			ProductID: "agoal",
			Name:      "strategy_list",
			CLIName:   "list",
			CLIPath:   "agoal strategy list",
			Source:    "reviewed_manual_hint",
		},
		Description: "List strategies.",
		Parameters: []ParameterSpec{{
			Name:        "page-size",
			Type:        "integer",
			Description: "Page size.",
			Property:    "pageSize",
			Default:     json.RawMessage("20"),
		}},
	})
	if err != nil {
		t.Fatalf("ToolSpecFromRuntime() error = %v", err)
	}
	registry, err := SchemaRegistryFromRuntime("embedded-command-catalog", []ProductSpec{{
		ID:    "agoal",
		Name:  "AGoal",
		Tools: []ToolSpec{spec},
	}})
	if err != nil {
		t.Fatalf("SchemaRegistryFromRuntime() error = %v", err)
	}
	snapshot, err := registry.ToSnapshotPayload()
	if err != nil {
		t.Fatalf("ToSnapshotPayload() error = %v", err)
	}
	full := snapshot.Tools["agoal.strategy_list"]
	if full == nil {
		t.Fatalf("full tool map = %#v", snapshot.Tools)
	}
	parameters, ok := full["parameters"].(map[string]any)
	if !ok || parameters["page-size"] == nil {
		t.Fatalf("full parameters = %#v", full["parameters"])
	}
	products := schemaMapSlice(snapshot.Catalog["products"])
	summaries := schemaMapSlice(products[0]["tools"])
	if len(summaries) != 1 || schemaString(summaries[0]["canonical_path"]) != schemaString(full["canonical_path"]) {
		t.Fatalf("summary/full identity drift: summary=%#v full=%#v", summaries, full)
	}
	if _, exists := summaries[0]["parameters"]; exists {
		t.Fatalf("summary unexpectedly contains full parameters: %#v", summaries[0])
	}
	if got := snapshot.Catalog["tool_count"]; got != 1 {
		t.Fatalf("catalog tool_count = %#v", got)
	}
}

func TestToolSpecStructuralValidationDoesNotEncodePrecedencePolicy(t *testing.T) {
	_, err := ToolSpecFromRuntime(RuntimeToolSpecInput{Identity: ToolIdentitySpec{
		ProductID:     "chat",
		Name:          "read",
		CanonicalPath: "chat.wrong",
		CLIPath:       "chat read",
	}})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("canonical mismatch error = %v", err)
	}

	_, err = ToolSpecFromRuntime(RuntimeToolSpecInput{
		Identity: ToolIdentitySpec{ProductID: "chat", Name: "read", CLIPath: "chat read"},
		Parameters: []ParameterSpec{{
			Name:    "query",
			Default: json.RawMessage("not-json"),
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid JSON default") {
		t.Fatalf("invalid literal error = %v", err)
	}
}

func TestFinalFieldProvenancePreservesFalseAndEmptyJSONValues(t *testing.T) {
	selected := true
	tests := []struct {
		name  string
		value any
		raw   json.RawMessage
	}{
		{name: "false", value: false, raw: json.RawMessage("false")},
		{name: "empty string", value: "", raw: json.RawMessage(`""`)},
		{name: "empty array", value: []string{}, raw: json.RawMessage(`[]`)},
		{name: "null", value: nil, raw: json.RawMessage(`null`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provenance := FieldProvenance{
				Value:      append(json.RawMessage(nil), test.raw...),
				Source:     "reviewed",
				Precedence: "reviewed_explicit",
				Resolution: "highest_precedence",
				Candidates: []FieldCandidateProvenance{{
					Value:      append(json.RawMessage(nil), test.raw...),
					Source:     "reviewed",
					Precedence: "reviewed_explicit",
					Selected:   &selected,
				}},
			}
			if err := validateFinalFieldProvenance("sample.run", "field", provenance, test.value); err != nil {
				t.Fatalf("validateFinalFieldProvenance() error = %v", err)
			}
		})
	}
}

func TestFinalFieldProvenanceIgnoresFormattingWhitespaceOnly(t *testing.T) {
	selected := true
	formatted := json.RawMessage("[\n  \"first\",\n  \"second\"\n]")
	provenance := FieldProvenance{
		Value:      formatted,
		Source:     "manual",
		Precedence: "reviewed_manual",
		Resolution: "highest_precedence",
		Candidates: []FieldCandidateProvenance{{
			Value: formatted, Source: "manual", Precedence: "reviewed_manual", Selected: &selected,
		}},
	}
	if err := validateFinalFieldProvenance("sample.run", "use_when", provenance, []string{"first", "second"}); err != nil {
		t.Fatalf("formatted provenance should validate: %v", err)
	}
	escaped := resolvedFieldProvenance("<none>", "manual", "manual.json", "reviewed_manual", "highest_precedence", "reviewed")
	escaped.Value = json.RawMessage(`"\u003cnone\u003e"`)
	escaped.Candidates[0].Value = escaped.Value
	if err := validateFinalFieldProvenance("sample.run", "interface_ref", escaped, "<none>"); err != nil {
		t.Fatalf("equivalent string escapes should validate: %v", err)
	}
	provenance.Value = json.RawMessage(`[1.0]`)
	provenance.Candidates[0].Value = provenance.Value
	if err := validateFinalFieldProvenance("sample.run", "use_when", provenance, []int{1}); err == nil {
		t.Fatal("numeric token drift unexpectedly validated")
	}
}

func TestEqualJSONValuesFastPathPreservesSemanticComparison(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
		want  bool
	}{
		{name: "identical valid", left: `{"value":false}`, right: `{"value":false}`, want: true},
		{name: "identical invalid", left: `{`, right: `{`, want: false},
		{name: "formatting and key order", left: `{"first":1,"second":[true]}`, right: "{\n  \"second\": [true],\n  \"first\": 1\n}", want: true},
		{name: "equivalent escape", left: `"<none>"`, right: `"\u003cnone\u003e"`, want: true},
		{name: "numeric lexical drift", left: `[1]`, right: `[1.0]`, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := equalJSONValues([]byte(test.left), []byte(test.right)); got != test.want {
				t.Fatalf("equalJSONValues(%q, %q) = %v, want %v", test.left, test.right, got, test.want)
			}
		})
	}
}

func TestToolSpecFromRuntimeDoesNotRepairResolvedProvenance(t *testing.T) {
	selected := true
	base := func() RuntimeToolSpecInput {
		return RuntimeToolSpecInput{
			Identity: ToolIdentitySpec{
				ProductID:     "sample",
				Name:          "run",
				CanonicalPath: "sample.run",
				CLIPath:       "sample run",
			},
			FieldProvenance: map[string]FieldProvenance{
				"canonical_path": {
					Value:      json.RawMessage(`"sample.run"`),
					Source:     "reviewed_command_registry",
					Precedence: "command_registry",
					Resolution: "registry_identity",
					Candidates: []FieldCandidateProvenance{{
						Value:      json.RawMessage(`"sample.run"`),
						Source:     "reviewed_command_registry",
						Precedence: "command_registry",
						Selected:   &selected,
					}},
				},
			},
		}
	}

	tests := []struct {
		name    string
		mutate  func(*FieldProvenance)
		wantErr string
	}{
		{
			name: "winner value is not synthesized",
			mutate: func(provenance *FieldProvenance) {
				provenance.Value = nil
			},
			wantErr: "winner does not equal final value",
		},
		{
			name: "selected value is not rewritten",
			mutate: func(provenance *FieldProvenance) {
				provenance.Candidates[0].Value = json.RawMessage(`"sample.other"`)
			},
			wantErr: "selected provenance candidate does not equal final value",
		},
		{
			name: "precedence is not inferred",
			mutate: func(provenance *FieldProvenance) {
				provenance.Precedence = ""
			},
			wantErr: "winner is incomplete",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := base()
			provenance := input.FieldProvenance["canonical_path"]
			test.mutate(&provenance)
			input.FieldProvenance["canonical_path"] = provenance
			_, err := ToolSpecFromRuntime(input)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("ToolSpecFromRuntime() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestFinalFieldProvenanceRequiresExactlyOneTypedWinner(t *testing.T) {
	selected := true
	unselected := false
	valid := func() FieldProvenance {
		return FieldProvenance{
			Value:      json.RawMessage("false"),
			Source:     "reviewed",
			Precedence: "reviewed_explicit",
			Resolution: "highest_precedence",
			Candidates: []FieldCandidateProvenance{{
				Value:      json.RawMessage("false"),
				Source:     "reviewed",
				Precedence: "reviewed_explicit",
				Selected:   &selected,
			}},
		}
	}
	tests := []struct {
		name    string
		mutate  func(*FieldProvenance)
		wantErr string
	}{
		{
			name: "JSON type mismatch",
			mutate: func(provenance *FieldProvenance) {
				provenance.Value = json.RawMessage(`"false"`)
			},
			wantErr: "winner does not equal final value",
		},
		{
			name: "no candidates",
			mutate: func(provenance *FieldProvenance) {
				provenance.Candidates = nil
			},
			wantErr: "has no candidates",
		},
		{
			name: "no selected candidate",
			mutate: func(provenance *FieldProvenance) {
				provenance.Candidates[0].Selected = &unselected
			},
			wantErr: "0 selected candidates",
		},
		{
			name: "two selected candidates",
			mutate: func(provenance *FieldProvenance) {
				provenance.Candidates = append(provenance.Candidates, provenance.Candidates[0])
			},
			wantErr: "2 selected candidates",
		},
		{
			name: "selected overridden candidate",
			mutate: func(provenance *FieldProvenance) {
				provenance.OverriddenCandidates = []FieldCandidateProvenance{{
					Value: json.RawMessage("true"), Source: "imported", Precedence: "imported", Selected: &selected,
				}}
			},
			wantErr: "selected overridden provenance candidate",
		},
		{
			name: "invalid unselected candidate value",
			mutate: func(provenance *FieldProvenance) {
				provenance.Candidates = append(provenance.Candidates, FieldCandidateProvenance{
					Value: json.RawMessage("not-json"), Source: "bad", Precedence: "imported", Selected: &unselected,
				})
			},
			wantErr: "invalid value",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provenance := valid()
			test.mutate(&provenance)
			err := validateFinalFieldProvenance("sample.run", "required", provenance, false)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateFinalFieldProvenance() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestProductionSnapshotLoaderDoesNotSynthesizeRequiredProvenance(t *testing.T) {
	for _, field := range requiredToolProvenanceFields {
		t.Run("tool "+field, func(t *testing.T) {
			snapshot := schemaDeliveryTestSnapshot(schemaDeliveryTestTool{Canonical: "sample.run", CLIPath: "sample run"})
			provenance := snapshot.Tools["sample.run"]["field_provenance"].(map[string]any)
			delete(provenance, field)
			snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)

			_, err := loadSchemaCatalogSnapshot(snapshot)
			if err == nil || !strings.Contains(err.Error(), "has no provenance for "+field) {
				t.Fatalf("loadSchemaCatalogSnapshot() error = %v, want fail-closed missing %s provenance", err, field)
			}
		})
	}

	t.Run("conditional interface_reason", func(t *testing.T) {
		snapshot := schemaDeliveryTestSnapshot(schemaDeliveryTestTool{Canonical: "sample.run", CLIPath: "sample run"})
		provenance := snapshot.Tools["sample.run"]["field_provenance"].(map[string]any)
		delete(provenance, "interface_reason")
		snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)

		_, err := loadSchemaCatalogSnapshot(snapshot)
		if err == nil || !strings.Contains(err.Error(), "has no provenance for interface_reason") {
			t.Fatalf("loadSchemaCatalogSnapshot() error = %v, want fail-closed conditional interface_reason provenance", err)
		}
	})

	selectionCases := []struct {
		field     string
		selection SelectionSpec
	}{
		{field: "use_when", selection: SelectionSpec{UseWhen: []string{}}},
		{field: "avoid_when", selection: SelectionSpec{AvoidWhen: []string{"avoid"}}},
		{field: "prerequisites", selection: SelectionSpec{Prerequisites: []string{"token"}}},
		{field: "tips", selection: SelectionSpec{Tips: []string{"tip"}}},
		{field: "workflow_refs", selection: SelectionSpec{WorkflowRefs: []string{"sample.other"}}},
		{field: "examples", selection: SelectionSpec{Examples: []string{"dws sample run"}}},
	}
	reviewed := false
	selectionCases = append(selectionCases, struct {
		field     string
		selection SelectionSpec
	}{field: "reviewed", selection: SelectionSpec{Reviewed: &reviewed}})
	for _, test := range selectionCases {
		t.Run("selection "+test.field, func(t *testing.T) {
			snapshot := schemaDeliveryTestSnapshot(schemaDeliveryTestTool{
				Canonical: "sample.run",
				CLIPath:   "sample run",
				Selection: test.selection,
			})
			provenance := snapshot.Tools["sample.run"]["field_provenance"].(map[string]any)
			delete(provenance, test.field)
			snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)

			_, err := loadSchemaCatalogSnapshot(snapshot)
			if err == nil || !strings.Contains(err.Error(), "has no provenance for "+test.field) {
				t.Fatalf("loadSchemaCatalogSnapshot() error = %v, want fail-closed selection %s provenance", err, test.field)
			}
		})
	}

	parameterProvenance := schemaDeliveryTestProvenance(map[string]any{
		"property":      "query",
		"type":          "string",
		"description":   "",
		"required":      false,
		"required_when": "",
	})
	for _, field := range requiredParameterProvenanceFields {
		t.Run("parameter "+field, func(t *testing.T) {
			parameter := ParameterSpec{
				Name:            "query",
				Type:            "string",
				Property:        "query",
				FieldProvenance: cloneFieldProvenance(parameterProvenance),
			}
			delete(parameter.FieldProvenance, field)
			snapshot := schemaDeliveryTestSnapshot(schemaDeliveryTestTool{
				Canonical:  "sample.run",
				CLIPath:    "sample run",
				Parameters: []ParameterSpec{parameter},
			})
			snapshot.SourceHash = schemaCatalogSnapshotHash(snapshot)

			_, err := loadSchemaCatalogSnapshot(snapshot)
			if err == nil || !strings.Contains(err.Error(), "has no provenance for "+field) {
				t.Fatalf("loadSchemaCatalogSnapshot() error = %v, want fail-closed missing parameter %s provenance", err, field)
			}
		})
	}
}

func TestFinalProvenanceCoverageDoesNotInventOptionalInterfaceReason(t *testing.T) {
	registry := schemaDeliveryTestRegistry(schemaDeliveryTestTool{Canonical: "sample.run", CLIPath: "sample run"})
	tool := &registry.Products[0].Tools[0]
	tool.Interface.Reason = ""
	delete(tool.FieldProvenance, "interface_reason")

	if err := validateFinalSchemaProvenanceCoverage(registry); err != nil {
		t.Fatalf("optional local interface_reason should not require invented provenance: %v", err)
	}
}
