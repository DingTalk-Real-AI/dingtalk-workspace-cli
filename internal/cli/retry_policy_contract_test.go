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
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

func conditionalRetryTool() RuntimeToolSpecInput {
	return RuntimeToolSpecInput{
		Identity: contract.ToolIdentitySpec{ProductID: "chat", Name: "send", CLIPath: "chat send"},
		Parameters: []ParameterSpec{{
			Name: "uuid", Type: "string", Property: "requestUuid",
			FieldProvenance: map[string]contract.FieldProvenance{
				"property": resolvedFieldProvenance(
					"requestUuid", "native_annotation", "contract.ParamDecl", "native_annotation",
					"contract_pass_through", "explicit retry key property",
				),
			},
		}},
		Safety: contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "not_required", Idempotency: "conditional"},
		RetryPolicy: &contract.RetryPolicySpec{
			Mode:                contract.RetryModeDeduplicationKey,
			KeyParameter:        "uuid",
			SamePayloadRequired: true,
		},
		Interface: contract.InterfaceSpec{
			Mode:         contract.InterfaceModeMCP,
			Availability: contract.InterfaceAvailable,
			Ref:          &contract.InterfaceRefSpec{ProductID: "chat", RPCName: "send"},
		},
		FieldProvenance: map[string]contract.FieldProvenance{
			"retry_policy": resolvedFieldProvenance(
				contract.RetryPolicySpec{Mode: contract.RetryModeDeduplicationKey, KeyParameter: "uuid", SamePayloadRequired: true},
				"corecmd.contract", "corecmd.ContractDecl", "contract_final", "contract_pass_through", "reviewed retry policy",
			),
		},
	}
}

func TestCrossPlatformCoverageConditionalRetryPolicyValidatesAndProjects(t *testing.T) {
	spec, err := ToolSpecFromRuntime(conditionalRetryTool())
	if err != nil {
		t.Fatalf("ToolSpecFromRuntime() error = %v", err)
	}
	full, err := spec.ToPayload()
	if err != nil {
		t.Fatalf("ToPayload() error = %v", err)
	}
	policy, ok := full["retry_policy"].(map[string]any)
	if !ok || policy["mode"] != contract.RetryModeDeduplicationKey || policy["key_parameter"] != "uuid" || policy["same_payload_required"] != true {
		t.Fatalf("retry_policy = %#v", full["retry_policy"])
	}
	compact := stripSchemaPayloadCompact(full)
	if !schemaJSONEqual(compact["retry_policy"], full["retry_policy"]) {
		t.Fatalf("compact retry_policy = %#v, want %#v", compact["retry_policy"], full["retry_policy"])
	}
	summary, err := spec.ToSummaryPayload()
	if err != nil {
		t.Fatalf("ToSummaryPayload() error = %v", err)
	}
	if _, exists := summary["retry_policy"]; exists {
		t.Fatalf("navigation summary must omit retry_policy: %#v", summary)
	}
	wire, err := schemaToolWireFromPayload(full)
	if err != nil {
		t.Fatalf("schemaToolWireFromPayload() error = %v", err)
	}
	roundTrip, err := schemaToolSpecFromWire(wire)
	if err != nil || roundTrip.RetryPolicy == nil || roundTrip.RetryPolicy.KeyParameter != "uuid" {
		t.Fatalf("snapshot retry_policy = %#v, error = %v", roundTrip.RetryPolicy, err)
	}

	registry := SchemaRegistry{Products: []ProductSpec{{ID: "chat", Tools: []ToolSpec{spec}}}}
	meta := buildMetaByCLIPathFromRegistry(registry)["chat send"]
	if meta.Safety.RetryPolicy == nil || meta.Safety.RetryPolicy.KeyParameter != "uuid" {
		t.Fatalf("ResolveMeta projection lost retry_policy: %#v", meta.Safety)
	}
	if decision := meta.Safety.Resolve(map[string]any{"uuid": "request-1"}); !decision.SafeToRetry || decision.EffectiveIdempotency != "idempotent" {
		t.Fatalf("invocation decision = %#v", decision)
	}
	if decision := meta.Safety.Resolve(nil); decision.SafeToRetry || decision.EffectiveIdempotency != "non_idempotent" {
		t.Fatalf("missing-key invocation decision = %#v", decision)
	}
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)
	storeSchemaSourceRootFn(func() *cobra.Command { return &cobra.Command{Use: "dws"} })
	assembleDeliverySchemaCatalogFn = func(*cobra.Command) (loadedSchemaCatalog, error) {
		return loadedSchemaCatalog{Registry: registry}, nil
	}
	resetSchemaDeliveryState()
	decision, ok := ResolveInvocationSafety("chat send", map[string]any{"uuid": "request-1"})
	if !ok || !decision.SafeToRetry || decision.EffectiveIdempotency != "idempotent" {
		t.Fatalf("ResolveInvocationSafety() = %#v, ok=%v", decision, ok)
	}
	if _, ok := ResolveInvocationSafety("chat missing", nil); ok {
		t.Fatal("ResolveInvocationSafety() must not invent missing commands")
	}
	transportDecision := ResolveToolCallRetry("chat", "send", map[string]any{"requestUuid": "request-1"})
	if !transportDecision.SafeToRetry || transportDecision.EffectiveIdempotency != "idempotent" {
		t.Fatalf("ResolveToolCallRetry() = %#v", transportDecision)
	}
	if missing := ResolveToolCallRetry("chat", "missing", nil); missing.SafeToRetry || missing.Reason != "interface_not_declared" {
		t.Fatalf("missing interface decision = %#v", missing)
	}
	lookup := buildToolCallRetryLookup(loadedSchemaCatalog{Registry: registry})
	resolution := lookup[retryInterfaceRefKey("chat", "send")]
	resolvedDecision := contract.ResolveRetryDecision(resolution.idempotency, resolution.policy, map[string]any{"requestUuid": "request-1"}, resolution.argumentKey)
	if !resolvedDecision.SafeToRetry {
		t.Fatalf("interface retry decision = %#v", resolvedDecision)
	}
	fullTool, _ := spec.ToPayload()
	metaSnapshot := SchemaCatalogSnapshot{Version: SchemaCatalogSnapshotVersion, SourceHash: "retry-source", Tools: map[string]map[string]any{"chat.send": fullTool}}
	metaIndex, err := BuildSchemaMetaIndex(metaSnapshot)
	if err != nil || len(metaIndex.Entries) != 1 || metaIndex.Entries[0].RetryPolicy == nil {
		t.Fatalf("BuildSchemaMetaIndex() = %#v, error = %v", metaIndex, err)
	}
	encodedIndex, err := EncodeSchemaMetaIndex(metaIndex)
	if err != nil {
		t.Fatalf("EncodeSchemaMetaIndex() error = %v", err)
	}
	decodedIndex, err := DecodeSchemaMetaIndex(encodedIndex)
	if err != nil || decodedIndex.Entries[0].RetryPolicy == nil || decodedIndex.Entries[0].RetryPolicy.KeyParameter != "uuid" {
		t.Fatalf("DecodeSchemaMetaIndex() = %#v, error = %v", decodedIndex, err)
	}
	if err := ValidateSchemaMetaIndexAgainstSnapshot(metaIndex, metaSnapshot); err != nil {
		t.Fatalf("ValidateSchemaMetaIndexAgainstSnapshot() error = %v", err)
	}
	snapshotLookup := buildToolCallRetryLookup(loadedSchemaCatalog{Snapshot: SchemaCatalogSnapshot{Tools: map[string]map[string]any{
		"chat.send":  fullTool,
		"chat.local": {"interface_mode": contract.InterfaceModeLocal},
	}}})
	snapshotResolution := snapshotLookup[retryInterfaceRefKey("chat", "send")]
	if snapshotResolution.argumentKey != "requestUuid" || snapshotResolution.policy == nil {
		t.Fatalf("snapshot retry resolution = %#v", snapshotResolution)
	}
	invalidSnapshot := map[string]any{
		"interface_mode": contract.InterfaceModeMCP,
		"interface_ref":  map[string]any{"product_id": "chat", "rpc_name": "invalid"},
		"idempotency":    "conditional",
		"retry_policy":   map[string]any{"mode": "inferred", "key_parameter": "uuid", "same_payload_required": true},
	}
	invalidLookup := buildToolCallRetryLookup(loadedSchemaCatalog{Snapshot: SchemaCatalogSnapshot{Tools: map[string]map[string]any{"chat.invalid": invalidSnapshot}}})
	invalidResolution := invalidLookup[retryInterfaceRefKey("chat", "invalid")]
	if invalidResolution.policy != nil || contract.ResolveRetryDecision(invalidResolution.idempotency, nil, nil, "").Reason != "invalid_retry_policy" {
		t.Fatalf("invalid snapshot retry resolution = %#v", invalidResolution)
	}
	missingBindingSnapshot := map[string]any{
		"interface_mode": contract.InterfaceModeMCP,
		"interface_ref":  map[string]any{"product_id": "chat", "rpc_name": "missing_binding"},
		"idempotency":    "conditional",
		"retry_policy":   map[string]any{"mode": contract.RetryModeDeduplicationKey, "key_parameter": "uuid", "same_payload_required": true},
		"parameters":     map[string]any{"uuid": map[string]any{"type": "string"}},
	}
	missingBindingLookup := buildToolCallRetryLookup(loadedSchemaCatalog{Snapshot: SchemaCatalogSnapshot{Tools: map[string]map[string]any{
		"chat.missing_binding": missingBindingSnapshot,
	}}})
	missingBindingResolution := missingBindingLookup[retryInterfaceRefKey("chat", "missing_binding")]
	missingBindingDecision := contract.ResolveRetryDecision(
		missingBindingResolution.idempotency,
		missingBindingResolution.policy,
		map[string]any{"uuid": "request-1"},
		missingBindingResolution.argumentKey,
	)
	if missingBindingDecision.SafeToRetry || missingBindingDecision.Reason != "deduplication_key_binding_missing" {
		t.Fatalf("snapshot missing property inferred a key binding: %#v", missingBindingDecision)
	}
	if retryPolicyFromSchemaValue(func() {}) != nil || retryPolicyFromSchemaValue("invalid") != nil || retryPolicyFromSchemaValue(nil) != nil {
		t.Fatal("invalid snapshot policy values must fail closed")
	}

	conflict := spec
	conflict.Identity.Name = "reply"
	conflict.Identity.CanonicalPath = "chat.reply"
	conflict.Identity.CLIPath = "chat reply"
	conflict.Identity.PrimaryCLIPath = "chat reply"
	conflict.Safety.Idempotency = "unknown"
	conflict.RetryPolicy = nil
	conflict.FieldProvenance = nil
	conflictRegistry := SchemaRegistry{Products: []ProductSpec{{ID: "chat", Tools: []ToolSpec{spec, conflict}}}}
	assembleDeliverySchemaCatalogFn = func(*cobra.Command) (loadedSchemaCatalog, error) {
		return loadedSchemaCatalog{Registry: conflictRegistry}, nil
	}
	resetSchemaDeliveryState()
	conflictDecision := ResolveToolCallRetry("chat", "send", map[string]any{"requestUuid": "request-1"})
	if conflictDecision.SafeToRetry || conflictDecision.Reason != "interface_retry_policy_conflict" {
		t.Fatalf("conflicting interface decision = %#v", conflictDecision)
	}
}

func TestCrossPlatformCoverageConditionalRetryPolicyFailsClosed(t *testing.T) {
	for name, mutate := range map[string]func(*RuntimeToolSpecInput){
		"missing policy":       func(in *RuntimeToolSpecInput) { in.RetryPolicy = nil },
		"missing parameter":    func(in *RuntimeToolSpecInput) { in.Parameters = nil },
		"missing property":     func(in *RuntimeToolSpecInput) { in.Parameters[0].Property = "" },
		"non string parameter": func(in *RuntimeToolSpecInput) { in.Parameters[0].Type = "integer" },
		"inferred property": func(in *RuntimeToolSpecInput) {
			in.Parameters[0].FieldProvenance["property"] = resolvedFieldProvenance(
				"requestUuid", "flag_name_inference", "cobra.flag", "inference", "fallback", "inferred",
			)
		},
		"non MCP interface": func(in *RuntimeToolSpecInput) {
			in.Interface = contract.InterfaceSpec{Mode: contract.InterfaceModeLocal, Availability: contract.InterfaceAvailable, Reason: "local"}
		},
		"policy on static idempotent": func(in *RuntimeToolSpecInput) { in.Safety.Idempotency = "idempotent" },
		"invalid policy mode":         func(in *RuntimeToolSpecInput) { in.RetryPolicy.Mode = "inferred" },
	} {
		t.Run(name, func(t *testing.T) {
			input := conditionalRetryTool()
			mutate(&input)
			if _, err := ToolSpecFromRuntime(input); err == nil {
				t.Fatal("invalid retry contract must fail closed")
			}
		})
	}
}

func TestCrossPlatformCoverageConditionalRetryPolicyRequiresFinalProvenance(t *testing.T) {
	spec, err := ToolSpecFromRuntime(conditionalRetryTool())
	if err != nil {
		t.Fatalf("ToolSpecFromRuntime() error = %v", err)
	}
	delete(spec.FieldProvenance, "retry_policy")
	err = validateFinalSchemaProvenanceCoverage(SchemaRegistry{Products: []ProductSpec{{
		ID: "chat", Tools: []ToolSpec{spec},
	}}})
	if err == nil || !strings.Contains(err.Error(), "has no provenance for retry_policy") {
		t.Fatalf("validateFinalSchemaProvenanceCoverage() error = %v", err)
	}
}

func TestCrossPlatformCoverageInterfaceRetryPolicyConflictFailsClosed(t *testing.T) {
	lookup := map[InterfaceRefKey]toolCallRetryResolution{}
	key := retryInterfaceRefKey("chat", "send")
	idempotent := toolCallRetryResolution{idempotency: "idempotent"}
	mergeToolCallRetryResolution(lookup, key, idempotent)
	mergeToolCallRetryResolution(lookup, key, idempotent)
	mergeToolCallRetryResolution(lookup, key, toolCallRetryResolution{idempotency: "unknown"})
	mergeToolCallRetryResolution(lookup, key, idempotent)
	if got := lookup[key]; !got.conflict {
		t.Fatalf("conflicting interface retry semantics = %#v", got)
	}
	decision := contract.RetryDecision{EffectiveIdempotency: "unknown", Reason: "interface_retry_policy_conflict"}
	if decision.SafeToRetry || !strings.Contains(decision.Reason, "conflict") {
		t.Fatalf("conflict decision = %#v", decision)
	}
}
