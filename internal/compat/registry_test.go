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

package compat

import (
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	"github.com/spf13/cobra"
)

func TestCollectBindingsParsesTypedValuesAndAcceptsAliasFlags(t *testing.T) {
	t.Parallel()

	bindings := []FlagBinding{
		{FlagName: "dept-ids", Alias: "deptIds", Property: "deptIds", Kind: ValueFloatSlice},
		{FlagName: "ratio", Property: "ratio", Kind: ValueFloat},
		{FlagName: "enabled-flags", Property: "enabledFlags", Kind: ValueBoolSlice},
		{FlagName: "base-id", Alias: "baseId", Property: "baseId", Kind: ValueString},
	}

	cmd := &cobra.Command{Use: "test"}
	ApplyBindings(cmd, bindings)

	if err := cmd.Flags().Set("deptIds", "1,2.5"); err != nil {
		t.Fatalf("Set(deptIds) error = %v", err)
	}
	if err := cmd.Flags().Set("ratio", "1.25"); err != nil {
		t.Fatalf("Set(ratio) error = %v", err)
	}
	if err := cmd.Flags().Set("enabled-flags", "true,false"); err != nil {
		t.Fatalf("Set(enabled-flags) error = %v", err)
	}
	if err := cmd.Flags().Set("baseId", "B1"); err != nil {
		t.Fatalf("Set(baseId) error = %v", err)
	}

	params, err := CollectBindings(cmd, bindings, nil)
	if err != nil {
		t.Fatalf("CollectBindings() error = %v", err)
	}

	if params["baseId"] != "B1" {
		t.Fatalf("baseId = %#v, want B1", params["baseId"])
	}
	if params["ratio"] != 1.25 {
		t.Fatalf("ratio = %#v, want 1.25", params["ratio"])
	}
	if want := []any{1.0, 2.5}; !reflect.DeepEqual(params["deptIds"], want) {
		t.Fatalf("deptIds = %#v, want %#v", params["deptIds"], want)
	}
	if want := []any{true, false}; !reflect.DeepEqual(params["enabledFlags"], want) {
		t.Fatalf("enabledFlags = %#v, want %#v", params["enabledFlags"], want)
	}

	aliasFlag := cmd.Flags().Lookup("baseId")
	if aliasFlag == nil || !aliasFlag.Hidden {
		t.Fatalf("baseId alias flag hidden = false, want true")
	}
}

func TestHiddenAliasFlagsDoNotInflateLeafCount(t *testing.T) {
	t.Parallel()

	// overlay leaf: 2 primary + 2 hidden aliases = 4 total, but only 2 visible
	overlay := &cobra.Command{Use: "list"}
	ApplyBindings(overlay, []FlagBinding{
		{FlagName: "start-time", Alias: "startTime", Property: "startTime", Kind: ValueString},
		{FlagName: "end-time", Alias: "endTime", Property: "endTime", Kind: ValueString},
	})

	// curated compat leaf: 3 primary, 0 aliases = 3 visible
	curated := &cobra.Command{Use: "list"}
	ApplyBindings(curated, []FlagBinding{
		{FlagName: "start", Property: "start", Kind: ValueString},
		{FlagName: "end", Property: "end", Kind: ValueString},
		{FlagName: "calendar-id", Property: "calendarId", Kind: ValueString},
	})

	overlayCount := cobracmd.LocalFlagCount(overlay)
	curatedCount := cobracmd.LocalFlagCount(curated)

	// overlay has 2 visible flags (start-time, end-time) + json + params = 4
	// curated has 3 visible flags (start, end, calendar-id) + json + params = 5
	if overlayCount >= curatedCount {
		t.Fatalf("overlay visible flags (%d) >= curated visible flags (%d); hidden aliases should not be counted",
			overlayCount, curatedCount)
	}

	// shouldReplaceCompatLeaf should NOT replace curated with overlay
	if cobracmd.ShouldReplaceLeaf(curated, overlay) {
		t.Fatal("cobracmd.ShouldReplaceLeaf(curated, overlay) = true; overlay should not displace curated")
	}
}

func TestCollectBindingsParsesJSONFlagValue(t *testing.T) {
	t.Parallel()

	bindings := []FlagBinding{
		{FlagName: "fields", Property: "fields", Kind: ValueJSON},
		{FlagName: "config", Property: "config", Kind: ValueJSON},
	}

	cmd := &cobra.Command{Use: "test"}
	ApplyBindings(cmd, bindings)

	if err := cmd.Flags().Set("fields", `[{"fieldName":"title","type":"text"}]`); err != nil {
		t.Fatalf("Set(fields) error = %v", err)
	}
	if err := cmd.Flags().Set("config", `{"options":[{"name":"high"}]}`); err != nil {
		t.Fatalf("Set(config) error = %v", err)
	}

	params, err := CollectBindings(cmd, bindings, nil)
	if err != nil {
		t.Fatalf("CollectBindings() error = %v", err)
	}

	fields, ok := params["fields"].([]any)
	if !ok || len(fields) != 1 {
		t.Fatalf("fields = %#v, want array of 1 element", params["fields"])
	}
	firstField, ok := fields[0].(map[string]any)
	if !ok || firstField["fieldName"] != "title" {
		t.Fatalf("fields[0] = %#v, want {fieldName:title, type:text}", fields[0])
	}

	config, ok := params["config"].(map[string]any)
	if !ok {
		t.Fatalf("config = %#v, want map", params["config"])
	}
	options, ok := config["options"].([]any)
	if !ok || len(options) != 1 {
		t.Fatalf("config.options = %#v, want array of 1", config["options"])
	}
}

func TestCollectSchemaFlagsPicksUpUnboundFlags(t *testing.T) {
	t.Parallel()

	// Simulate a plugin command with schema-generated flags but no bindings.
	cmd := &cobra.Command{Use: "greet"}
	cmd.Flags().String("name", "", "Name of person")
	cmd.Flags().String("language", "en", "Language")
	cmd.Flags().Int("count", 0, "Repeat count")
	cmd.Flags().Bool("loud", false, "Loud mode")
	cmd.Flags().String("json", "", "")
	cmd.Flags().String("params", "", "")

	// User sets --name and --count but not --language
	_ = cmd.Flags().Set("name", "Alice")
	_ = cmd.Flags().Set("count", "3")
	_ = cmd.Flags().Set("loud", "true")

	params := make(map[string]any)
	collectSchemaFlags(cmd, nil, params)

	if params["name"] != "Alice" {
		t.Errorf("name = %v, want Alice", params["name"])
	}
	if params["count"] != 3 {
		t.Errorf("count = %v, want 3", params["count"])
	}
	if params["loud"] != true {
		t.Errorf("loud = %v, want true", params["loud"])
	}
	// language was not set by user, should not appear
	if _, exists := params["language"]; exists {
		t.Errorf("language should not be in params (not set by user)")
	}
	// json/params are reserved, should not appear
	if _, exists := params["json"]; exists {
		t.Error("json should be skipped")
	}
}

func TestCollectSchemaFlagsSkipsBoundFlags(t *testing.T) {
	t.Parallel()

	bindings := []FlagBinding{
		{FlagName: "dept-id", Property: "deptId", Kind: ValueString},
	}

	cmd := &cobra.Command{Use: "test"}
	ApplyBindings(cmd, bindings)
	// Also add a schema-generated flag
	cmd.Flags().String("title", "", "Title")

	_ = cmd.Flags().Set("dept-id", "D001")
	_ = cmd.Flags().Set("title", "Hello")

	params := make(map[string]any)
	collectSchemaFlags(cmd, bindings, params)

	// dept-id is bound, should NOT be collected by collectSchemaFlags
	if _, exists := params["dept_id"]; exists {
		t.Error("dept-id should be skipped (already has binding)")
	}
	// title is unbound, should be collected
	if params["title"] != "Hello" {
		t.Errorf("title = %v, want Hello", params["title"])
	}
}

func TestCollectSchemaFlagsSkipsGlobalFlags(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("name", "", "Name")
	cmd.Flags().Bool("debug", false, "Debug")
	cmd.Flags().Bool("verbose", false, "Verbose")
	cmd.Flags().Bool("dry-run", false, "Dry run")
	cmd.Flags().String("format", "json", "Format")
	cmd.Flags().String("json", "", "")
	cmd.Flags().String("params", "", "")

	_ = cmd.Flags().Set("name", "Bob")
	_ = cmd.Flags().Set("debug", "true")
	_ = cmd.Flags().Set("verbose", "true")
	_ = cmd.Flags().Set("dry-run", "true")
	_ = cmd.Flags().Set("format", "table")

	params := make(map[string]any)
	collectSchemaFlags(cmd, nil, params)

	if params["name"] != "Bob" {
		t.Errorf("name = %v, want Bob", params["name"])
	}
	// Global flags should be skipped
	for _, skip := range []string{"debug", "verbose", "dry_run", "format"} {
		if _, exists := params[skip]; exists {
			t.Errorf("%s should be skipped (global flag)", skip)
		}
	}
}
