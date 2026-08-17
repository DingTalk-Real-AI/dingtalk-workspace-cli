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

package helpers

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
)

func TestCrossPlatformCoverageDriveFolderContractsPublishResultAndDryRun(t *testing.T) {
	drive := newDriveCommand()
	want := map[string]struct {
		canonical  string
		dryRun     bool
		outcomes   []contract.ResultOutcome
		properties []string
	}{
		"status": {
			canonical:  "drive.folder_status",
			outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
			properties: []string{"detection", "modified", "new_local", "new_remote", "unchanged", "unknown"},
		},
		"pull": {
			canonical:  "drive.folder_pull",
			dryRun:     true,
			outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomePartialFailure, contract.ResultOutcomeFailure},
			properties: []string{"dry_run", "executed", "if_exists", "items", "operation", "plan", "preview_kind", "summary"},
		},
		"push": {
			canonical:  "drive.folder_push",
			dryRun:     true,
			outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomePartialFailure, contract.ResultOutcomeFailure},
			properties: []string{"dry_run", "executed", "if_exists", "items", "operation", "plan", "preview_kind", "summary"},
		},
		"sync": {
			canonical:  "drive.folder_sync",
			dryRun:     true,
			outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomePartialFailure, contract.ResultOutcomeFailure},
			properties: []string{"detection", "diff", "dry_run", "executed", "items", "operation", "plan", "preview_kind", "summary"},
		},
	}

	for name, expectation := range want {
		t.Run(name, func(t *testing.T) {
			leaf, _, err := drive.Find([]string{name})
			if err != nil || leaf == nil {
				t.Fatalf("find drive %s: command=%v err=%v", name, leaf, err)
			}
			final, ok := contractfinal.RuntimeContractFinal(leaf)
			if !ok || final.Identity == nil || final.Identity.CanonicalPath != expectation.canonical {
				t.Fatalf("drive %s ContractFinal identity = %#v", name, final.Identity)
			}
			if final.Result == nil || !reflect.DeepEqual(final.Result.Outcomes, expectation.outcomes) {
				t.Fatalf("drive %s result = %#v, want outcomes %#v", name, final.Result, expectation.outcomes)
			}
			properties := resultSchemaProperties(t, final.Result.DataSchema)
			if got := sortedContractSchemaKeys(properties); !reflect.DeepEqual(got, expectation.properties) {
				t.Fatalf("drive %s result properties = %#v, want %#v", name, got, expectation.properties)
			}
			if expectation.dryRun {
				if final.DryRun == nil || final.DryRun.PreviewKind != contract.DryRunPreviewPlan || !final.DryRun.RemoteReads {
					t.Fatalf("drive %s dry_run = %#v, want remote-reading plan", name, final.DryRun)
				}
				if final.Selection == nil || len(final.Selection.ExampleDispositions) != len(final.Selection.Examples) {
					t.Fatalf("drive %s stateful example dispositions = %#v", name, final.Selection)
				}
				for _, disposition := range final.Selection.ExampleDispositions {
					if disposition.Mode != contract.ExampleDispositionModeContractOnly ||
						disposition.ReasonCode != contract.ExampleDispositionReasonStatefulPreflight || !disposition.Reviewed {
						t.Fatalf("drive %s invalid example disposition: %#v", name, disposition)
					}
				}
			} else if final.DryRun != nil {
				t.Fatalf("drive %s unexpectedly publishes dry_run: %#v", name, final.DryRun)
			}

			if name == "sync" {
				assertDriveSyncResultSchema(t, final.Result.DataSchema)
			}
		})
	}
}

func resultSchemaProperties(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("result data_schema is not JSON: %v\n%s", err, raw)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("result data_schema.properties = %#v, want object", schema["properties"])
	}
	return properties
}

func sortedContractSchemaKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func assertDriveSyncResultSchema(t *testing.T, raw json.RawMessage) {
	t.Helper()
	properties := resultSchemaProperties(t, raw)
	diff := schemaObjectProperty(t, properties, "diff")
	assertSchemaRequired(t, diff, "new_local", "new_remote", "modified", "unchanged", "unknown")
	if got := sortedContractSchemaKeys(schemaProperties(t, diff)); !reflect.DeepEqual(got, []string{"modified", "new_local", "new_remote", "unchanged", "unknown"}) {
		t.Fatalf("sync diff properties = %#v", got)
	}

	plan := schemaObjectProperty(t, properties, "plan")
	assertSchemaRequired(t, plan, "detection", "diff", "summary", "items")
	planProperties := schemaProperties(t, plan)
	planDiff := schemaObjectProperty(t, planProperties, "diff")
	assertSchemaRequired(t, planDiff, "new_local", "new_remote", "modified", "unchanged", "unknown")

	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	oneOf, ok := schema["oneOf"].([]any)
	if !ok || len(oneOf) != 2 {
		t.Fatalf("sync result oneOf = %#v, want execution/dry-run alternatives", schema["oneOf"])
	}
}

func schemaObjectProperty(t *testing.T, properties map[string]any, name string) map[string]any {
	t.Helper()
	property, ok := properties[name].(map[string]any)
	if !ok || property["type"] != "object" {
		t.Fatalf("schema property %s = %#v, want object", name, properties[name])
	}
	return property
}

func schemaProperties(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties = %#v, want object", schema["properties"])
	}
	return properties
}

func assertSchemaRequired(t *testing.T, schema map[string]any, want ...string) {
	t.Helper()
	raw, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("schema required = %#v", schema["required"])
	}
	got := make([]string, 0, len(raw))
	for _, value := range raw {
		text, ok := value.(string)
		if !ok {
			t.Fatalf("schema required value = %#v, want string", value)
		}
		got = append(got, text)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("schema required = %#v, want %#v", got, want)
	}
}
