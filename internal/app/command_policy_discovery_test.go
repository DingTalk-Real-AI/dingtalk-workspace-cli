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

package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cmdpolicy"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/spf13/pflag"
)

// reportReadOnlyRules allows the report product except its submit operation,
// which is reachable through both "report entry submit" and "report create".
func reportReadOnlyRules() func() []edition.CommandRule {
	return staticRules(edition.CommandRule{Name: "report-readonly", Allow: []string{"report/**"}, MaxEffect: edition.EffectRead})
}

func schemaJSON(t *testing.T, args ...string) map[string]any {
	t.Helper()
	out, err := executePolicyRoot(t, append([]string{"schema"}, args...)...)
	if err != nil {
		t.Fatalf("dws schema %v: %v\n%s", args, err, out)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("dws schema %v is not JSON: %v\n%.300s", args, err, out)
	}
	return payload
}

func schemaTools(container map[string]any) []map[string]any {
	raw, _ := container["tools"].([]any)
	tools := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		tools = append(tools, item.(map[string]any))
	}
	return tools
}

func schemaProducts(payload map[string]any) []map[string]any {
	raw, _ := payload["products"].([]any)
	products := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		products = append(products, item.(map[string]any))
	}
	return products
}

// Every way of naming a denied command in a single-path Schema query is
// refused with the denial, across compact and full, and never as "not found".
func TestCrossPlatformCoverageCommandPolicySchemaLeafQueries(t *testing.T) {
	restrictForTest(t, reportReadOnlyRules())
	deniedQueries := map[string][]string{
		"canonical":        {"report.create_report"},
		"primary cli path": {"report entry submit"},
		"alias":            {"report create"},
		"--cli-path":       {"--cli-path", "report entry submit"},
		"--cli-path alias": {"--cli-path", "report create"},
	}
	for name, query := range deniedQueries {
		for _, detail := range [][]string{nil, {"--compact"}} {
			args := append(append([]string{"schema"}, query...), detail...)
			args = append(args, "-f", "json")
			t.Run(name+strings.Join(detail, ""), func(t *testing.T) {
				out, err := executePolicyRoot(t, args...)
				requireDenied(t, err, cmdpolicy.ReasonRiskTooHigh)
				if strings.Contains(out, "canonical_path") {
					t.Fatalf("a denied leaf query emitted Schema content:\n%.300s", out)
				}
			})
		}
	}
	for _, detail := range [][]string{nil, {"--compact"}} {
		leaf := schemaJSON(t, append([]string{"report entry get", "-f", "json"}, detail...)...)
		if leaf["canonical_path"] == nil {
			t.Fatalf("a permitted leaf (%v) must still be served: %v", detail, leaf)
		}
	}
}

// Range queries are projected: denied tools disappear, counts are recomputed,
// empty containers are dropped, and the projection is identified.
func TestCrossPlatformCoverageCommandPolicySchemaRangeQueries(t *testing.T) {
	baseline := schemaJSON(t, "-f", "json")
	baselineProduct := schemaJSON(t, "report", "-f", "json")
	baselineTools := len(schemaTools(baselineProduct["product"].(map[string]any)))

	restrictForTest(t, reportReadOnlyRules())

	for _, detail := range [][]string{nil, {"--compact"}} {
		name := "full"
		if detail != nil {
			name = "compact"
		}
		t.Run("product/"+name, func(t *testing.T) {
			payload := schemaJSON(t, append([]string{"report", "-f", "json"}, detail...)...)
			product := payload["product"].(map[string]any)
			tools := schemaTools(product)
			if len(tools) == 0 || len(tools) >= baselineTools {
				t.Fatalf("product view has %d tools, want fewer than %d and more than 0", len(tools), baselineTools)
			}
			for _, tool := range tools {
				if tool["effect"] != "read" {
					t.Errorf("product view contains a non-read tool: %v (%v)", tool["canonical_path"], tool["effect"])
				}
			}
			if fmt.Sprint(product["tool_count"]) != fmt.Sprint(len(tools)) || fmt.Sprint(payload["count"]) != fmt.Sprint(len(tools)) {
				t.Fatalf("counts %v/%v do not match %d listed tools", product["tool_count"], payload["count"], len(tools))
			}
		})
		t.Run("group/"+name, func(t *testing.T) {
			payload := schemaJSON(t, append([]string{"report entry", "-f", "json"}, detail...)...)
			tools := schemaTools(payload)
			for _, tool := range tools {
				if tool["canonical_path"] == "report.create_report" {
					t.Fatal("group view lists the denied submit tool")
				}
			}
			if len(tools) == 0 || fmt.Sprint(payload["count"]) != fmt.Sprint(len(tools)) {
				t.Fatalf("group count %v does not match %d tools", payload["count"], len(tools))
			}
		})
		t.Run("all/"+name, func(t *testing.T) {
			payload := schemaJSON(t, append([]string{"--all", "-f", "json"}, detail...)...)
			products := schemaProducts(payload)
			// config carries only the always-available policy diagnostic.
			if got := strings.Join(productIDs(products), ","); got != "config,report" {
				t.Fatalf("--all view lists products %q, want config,report", got)
			}
			total := 0
			for _, product := range products {
				tools := schemaTools(product)
				if len(tools) == 0 || fmt.Sprint(product["tool_count"]) != fmt.Sprint(len(tools)) {
					t.Fatalf("product %v tool_count %v does not match %d listed tools", product["id"], product["tool_count"], len(tools))
				}
				total += len(tools)
			}
			if fmt.Sprint(payload["tool_count"]) != fmt.Sprint(total) || fmt.Sprint(payload["count"]) != "2" {
				t.Fatalf("--all counts %v/%v do not match %d tools in 2 products", payload["tool_count"], payload["count"], total)
			}
		})
		t.Run("overview/"+name, func(t *testing.T) {
			payload := schemaJSON(t, append([]string{"-f", "json"}, detail...)...)
			products := schemaProducts(payload)
			if got := strings.Join(productIDs(products), ","); got != "config,report" {
				t.Fatalf("overview lists products %q, want config,report", got)
			}
			// The overview carries no tool list; its counts must agree with
			// what each product view actually lists.
			total := 0
			for _, entry := range products {
				product := schemaJSON(t, fmt.Sprint(entry["id"]), "-f", "json")["product"].(map[string]any)
				if fmt.Sprint(entry["tool_count"]) != fmt.Sprint(product["tool_count"]) {
					t.Fatalf("overview count %v for %v disagrees with its product view %v", entry["tool_count"], entry["id"], product["tool_count"])
				}
				total += len(schemaTools(product))
			}
			if fmt.Sprint(payload["tool_count"]) != fmt.Sprint(total) || fmt.Sprint(payload["count"]) != "2" {
				t.Fatalf("overview totals %v/%v do not match %d tools in 2 products", payload["tool_count"], payload["count"], total)
			}
		})
	}

	// A product or group with nothing permitted is refused, not shown empty.
	for _, query := range []string{"calendar", "calendar event"} {
		_, err := executePolicyRoot(t, "schema", query, "-f", "json")
		requireDenied(t, err, cmdpolicy.ReasonAllChildrenDenied)
	}

	// The base catalog identity is unchanged; the view carries its own.
	overview := schemaJSON(t, "-f", "json")
	if overview["catalog_hash"] != baseline["catalog_hash"] {
		t.Fatalf("catalog_hash changed under a policy: %v -> %v", baseline["catalog_hash"], overview["catalog_hash"])
	}
	viewHash, _ := overview["policy_view_hash"].(string)
	if !strings.HasPrefix(viewHash, "sha256:") {
		t.Fatalf("overview lacks policy_view_hash: %v", overview["policy_view_hash"])
	}
	if _, present := baseline["policy_view_hash"]; present {
		t.Fatal("policy_view_hash must be absent while the policy is disabled")
	}
	restrictForTest(t, staticRules(edition.CommandRule{Name: "report-all", Allow: []string{"report/**"}}))
	if other := schemaJSON(t, "-f", "json")["policy_view_hash"]; other == viewHash {
		t.Fatal("a different rule set produced the same policy_view_hash")
	}
}

func productIDs(products []map[string]any) []string {
	ids := make([]string, 0, len(products))
	for _, product := range products {
		ids = append(ids, fmt.Sprint(product["id"]))
	}
	return ids
}

// Projecting a view must not write through to shared delivery state: after a
// restricted tree has served --all, an unrestricted tree in the same process
// still sees the complete catalog.
func TestCrossPlatformCoverageCommandPolicySchemaViewDoesNotMutateDelivery(t *testing.T) {
	before := schemaJSON(t, "--all", "-f", "json")
	func() {
		restrictForTest(t, reportReadOnlyRules())
		restricted := schemaJSON(t, "--all", "-f", "json")
		if fmt.Sprint(restricted["tool_count"]) == fmt.Sprint(before["tool_count"]) {
			t.Fatal("the restricted view was not narrowed")
		}
		schemaJSON(t, "-f", "json")
		schemaJSON(t, "report", "-f", "json")
	}()
	previous := edition.Get()
	open := *previous
	open.Restrict = nil
	edition.Override(&open)
	after := schemaJSON(t, "--all", "-f", "json")
	if fmt.Sprint(after["tool_count"]) != fmt.Sprint(before["tool_count"]) || len(schemaProducts(after)) != len(schemaProducts(before)) {
		t.Fatalf("delivery state was mutated by a policy view: %v tools/%d products, want %v/%d",
			after["tool_count"], len(schemaProducts(after)), before["tool_count"], len(schemaProducts(before)))
	}
}

// Help and completion are discovery surfaces too.
func TestCrossPlatformCoverageCommandPolicyHelpAndCompletion(t *testing.T) {
	restrictForTest(t, reportReadOnlyRules())

	for _, args := range [][]string{
		{"help", "report", "entry", "submit"},
		{"help", "report", "create"},
		{"help", "calendar", "event", "delete"},
	} {
		out, err := executePolicyRoot(t, args...)
		requireDenied(t, err, "")
		if strings.Contains(out, "--template-id") || strings.Contains(out, "Usage:") || strings.Contains(out, "Flags:") {
			t.Fatalf("dws %v leaked help for a denied command:\n%s", args, out)
		}
	}
	// A group with nothing permitted: both spellings refuse to render help.
	if _, err := executePolicyRoot(t, "help", "calendar"); err == nil {
		t.Fatal("dws help calendar must be refused when no calendar command is permitted")
	} else {
		requireDenied(t, err, cmdpolicy.ReasonAllChildrenDenied)
	}
	out, err := executePolicyRoot(t, "calendar", "--help")
	if err != nil {
		t.Fatalf("dws calendar --help: %v", err)
	}
	if !strings.Contains(out, "command_denied") && !strings.Contains(out, "denied by the command restriction policy") {
		t.Fatalf("dws calendar --help did not report the denial:\n%s", out)
	}
	if strings.Contains(out, "event") {
		t.Fatalf("dws calendar --help listed subcommands of a denied product:\n%s", out)
	}

	// Permitted help still works and lists only permitted children.
	out, err = executePolicyRoot(t, "help", "report", "entry")
	if err != nil {
		t.Fatalf("dws help report entry: %v\n%s", err, out)
	}
	// The group's own description is static prose and may still name submit;
	// the command listing must not.
	if !strings.Contains(out, "\n  get ") || strings.Contains(out, "\n  submit ") {
		t.Fatalf("report entry help must list get and omit submit:\n%s", out)
	}

	// Shell completion offers neither denied command names nor their flags.
	out, err = executePolicyRoot(t, "__complete", "report", "entry", "")
	if err != nil {
		t.Fatalf("__complete: %v\n%s", err, out)
	}
	if !strings.Contains(out, "get") || strings.Contains(out, "submit") {
		t.Fatalf("completion under report entry must offer get and not submit:\n%s", out)
	}
	out, err = executePolicyRoot(t, "__complete", "")
	if err != nil {
		t.Fatalf("__complete root: %v\n%s", err, out)
	}
	if strings.Contains(out, "calendar") || !strings.Contains(out, "report") {
		t.Fatalf("root completion must offer report and not calendar:\n%s", out)
	}
	// Completion resolves its target and enumerates flags without running the
	// denied command, so the stub itself must expose none. Cover both the
	// described and the no-description request, and both the primary and the
	// compatibility entry.
	for _, request := range []string{"__complete", "__completeNoDesc"} {
		for _, target := range [][]string{{"report", "entry", "submit"}, {"report", "create"}} {
			for _, prefix := range []string{"--", "--t", "-"} {
				args := append(append([]string{request}, target...), prefix)
				out, _ := executePolicyRoot(t, args...)
				if strings.Contains(out, "template-id") || strings.Contains(out, "to-user-ids") {
					t.Fatalf("dws %v leaked flags of a denied command:\n%s", args, out)
				}
			}
		}
	}
	// A permitted sibling still completes its own flags.
	out, _ = executePolicyRoot(t, "__complete", "report", "entry", "get", "--")
	if !strings.Contains(out, "--") {
		t.Fatalf("completion for a permitted command offers no flags:\n%s", out)
	}
}

// A denied hybrid group keeps navigating to its permitted children, but its
// own business flags are offered neither by completion nor by help.
func TestCrossPlatformCoverageCommandPolicyDeniedHybridHidesBusinessFlags(t *testing.T) {
	const self, child = "dingtalk-tag/connect", "dingtalk-tag/connect/status"
	unrestricted := findPolicyCommand(t, NewRootCommand(), self)
	var businessFlag string
	unrestricted.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) {
		if businessFlag == "" && !flag.Hidden && flag.Name != "help" {
			businessFlag = flag.Name
		}
	})
	if businessFlag == "" {
		t.Skip("dingtalk-tag connect declares no visible business flag")
	}

	restrictForTest(t, staticRules(edition.CommandRule{Name: "status", Allow: []string{child}, AllowUnannotated: true}))
	out, _ := executePolicyRoot(t, "__complete", "dingtalk-tag", "connect", "--")
	if strings.Contains(out, "--"+businessFlag) {
		t.Fatalf("completion offered --%s of a denied hybrid group:\n%s", businessFlag, out)
	}
	out, _ = executePolicyRoot(t, "__complete", "dingtalk-tag", "connect", "")
	if !strings.Contains(out, "status") {
		t.Fatalf("completion no longer navigates to the permitted child:\n%s", out)
	}
	out, err := executePolicyRoot(t, "dingtalk-tag", "connect", "--help")
	if err != nil {
		t.Fatalf("dws dingtalk-tag connect --help: %v", err)
	}
	if strings.Contains(out, "--"+businessFlag) || !strings.Contains(out, "status") {
		t.Fatalf("help of a denied hybrid group must list its permitted child and omit --%s:\n%s", businessFlag, out)
	}
}

// config policy show is part of the Agent surface: its return contract is
// published through full and compact Schema, and the live report conforms.
func TestCrossPlatformCoverageCommandPolicyShowSchemaContract(t *testing.T) {
	full := schemaJSON(t, "--cli-path", "config policy show", "-f", "json")
	compact := schemaJSON(t, "config.policy_show", "--compact", "-f", "json")
	for name, leaf := range map[string]map[string]any{"full": full, "compact": compact} {
		if leaf["canonical_path"] != "config.policy_show" || leaf["effect"] != "read" || leaf["risk"] != "low" {
			t.Fatalf("%s leaf identity/safety = %v/%v/%v", name, leaf["canonical_path"], leaf["effect"], leaf["risk"])
		}
		result, _ := leaf["result"].(map[string]any)
		if result == nil || result["data_schema"] == nil || len(result["outcomes"].([]any)) == 0 {
			t.Fatalf("%s leaf publishes no result contract: %v", name, leaf["result"])
		}
	}
	fullResult, _ := json.Marshal(full["result"])
	compactResult, _ := json.Marshal(compact["result"])
	if string(fullResult) != string(compactResult) {
		t.Fatalf("compact result differs from the full leaf:\n full    %s\n compact %s", fullResult, compactResult)
	}

	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("policy-show.json", full["result"].(map[string]any)["data_schema"]); err != nil {
		t.Fatalf("add data_schema: %v", err)
	}
	schema, err := compiler.Compile("policy-show.json")
	if err != nil {
		t.Fatalf("compile data_schema: %v", err)
	}
	validate := func(t *testing.T) {
		t.Helper()
		out, err := executePolicyRoot(t, "config", "policy", "show", "--format", "json")
		if err != nil {
			t.Fatalf("config policy show: %v\n%s", err, out)
		}
		envelope, err := jsonschema.UnmarshalJSON(strings.NewReader(out))
		if err != nil {
			t.Fatalf("output is not JSON: %v\n%s", err, out)
		}
		data := envelope.(map[string]any)["data"]
		if err := schema.Validate(data); err != nil {
			t.Fatalf("live report does not conform to the published data_schema: %v\n%s", err, out)
		}
	}
	t.Run("disabled", validate)
	t.Run("active", func(t *testing.T) {
		restrictForTest(t, staticRules(
			edition.CommandRule{Name: "no-delete", Allow: []string{"calendar/**"}, Deny: []string{"calendar/event/delete"}},
			edition.CommandRule{Name: "everything"},
		))
		validate(t)
	})
	t.Run("invalid", func(t *testing.T) {
		restrictForTest(t, staticRules(edition.CommandRule{Name: "bad", MaxRisk: "nope"}))
		validate(t)
	})
}

// A mistyped command is never corrected into a denied one.
func TestCrossPlatformCoverageCommandPolicySuggestionsOmitDenied(t *testing.T) {
	restrictForTest(t, reportReadOnlyRules())
	out, err := executePolicyRoot(t, "report", "entry", "submi")
	if err == nil {
		t.Fatalf("a mistyped subcommand must fail:\n%s", out)
	}
	if strings.Contains(out+err.Error(), "submit") {
		t.Fatalf("a denied command was suggested: %v\n%s", err, out)
	}
	out, err = executePolicyRoot(t, "calenda")
	if err == nil {
		t.Fatalf("a mistyped product must fail:\n%s", out)
	}
	if strings.Contains(out+err.Error(), "calendar") {
		t.Fatalf("a denied product was suggested: %v\n%s", err, out)
	}
}
