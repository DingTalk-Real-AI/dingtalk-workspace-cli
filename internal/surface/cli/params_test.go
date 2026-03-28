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
	"reflect"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/ir"
	"github.com/spf13/cobra"
)

func TestNormalizeCanonicalParamsMergesAliasesTransformsAndNesting(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"required": []any{
			"title",
		},
		"properties": map[string]any{
			"title":      map[string]any{"type": "string"},
			"tags":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"config":     map[string]any{"type": "object"},
			"publish_at": map[string]any{"type": "integer"},
			"visibility": map[string]any{"type": "integer"},
			"Body.query": map[string]any{"type": "string"},
		},
	}
	hints := map[string]ir.CLIFlagHint{
		"title": {
			Alias:    "name",
			Required: true,
		},
		"tags": {
			Transform: "csv_to_array",
		},
		"config": {
			Transform: "json_parse",
		},
		"publish_at": {
			Transform: "iso8601_to_millis",
		},
		"visibility": {
			Transform: "enum_map",
			TransformArgs: map[string]any{
				"private":  2,
				"_default": 9,
			},
		},
		"Body.query": {
			Alias: "query",
		},
	}

	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().String("json", "", "")
	cmd.Flags().String("params", "", "")
	specs := BuildFlagSpecs(schema, hints)
	applyFlagSpecs(cmd, specs)
	if err := cmd.ParseFlags([]string{
		"--json", `{"name":"json-title","tags":"alpha, beta","config":"{\"enabled\":true,\"count\":2}","publish_at":"2026-03-27T08:09:10Z","visibility":"unknown","query":"from-json"}`,
		"--params", `{"title":"params-title","visibility":"private"}`,
		"--name", "flag-title",
	}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}

	params, err := normalizeCanonicalParams(cmd, schema, specs, hints)
	if err != nil {
		t.Fatalf("normalizeCanonicalParams() error = %v", err)
	}

	if got := params["title"]; got != "flag-title" {
		t.Fatalf("params[title] = %#v, want flag-title", got)
	}
	if want := []any{"alpha", "beta"}; !reflect.DeepEqual(params["tags"], want) {
		t.Fatalf("params[tags] = %#v, want %#v", params["tags"], want)
	}
	if want := map[string]any{"enabled": true, "count": float64(2)}; !reflect.DeepEqual(params["config"], want) {
		t.Fatalf("params[config] = %#v, want %#v", params["config"], want)
	}
	wantMillis := time.Date(2026, time.March, 27, 8, 9, 10, 0, time.UTC).UnixMilli()
	if got := params["publish_at"]; got != wantMillis {
		t.Fatalf("params[publish_at] = %#v, want %d", got, wantMillis)
	}
	if got := params["visibility"]; got != 2 {
		t.Fatalf("params[visibility] = %#v, want 2", got)
	}
	body, ok := params["Body"].(map[string]any)
	if !ok {
		t.Fatalf("params[Body] = %#v, want nested map", params["Body"])
	}
	if got := body["query"]; got != "from-json" {
		t.Fatalf("params[Body][query] = %#v, want from-json", got)
	}
}

func TestNormalizeCanonicalParamsRequiresMissingFields(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"required": []any{
			"title",
		},
		"properties": map[string]any{
			"title": map[string]any{"type": "string"},
		},
	}
	hints := map[string]ir.CLIFlagHint{
		"title": {
			Required: true,
		},
	}

	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().String("json", "", "")
	cmd.Flags().String("params", "", "")
	specs := BuildFlagSpecs(schema, hints)
	applyFlagSpecs(cmd, specs)

	params, err := normalizeCanonicalParams(cmd, schema, specs, hints)
	if err == nil {
		t.Fatalf("normalizeCanonicalParams() params = %#v, want required error", params)
	}
	if got := err.Error(); got != "--title is required" {
		t.Fatalf("normalizeCanonicalParams() error = %q, want --title is required", got)
	}
}

func TestNormalizeCanonicalParamsIgnoresHiddenRequiredFields(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"baseId": map[string]any{"type": "string"},
			"aiConfig": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"outputType": map[string]any{"type": "string"},
				},
				"required": []any{"outputType"},
			},
		},
	}
	hints := map[string]ir.CLIFlagHint{
		"baseId": {
			Alias:    "base-id",
			Required: true,
		},
		"aiConfig.outputType": {
			Alias:    "outputType",
			Hidden:   true,
			Required: true,
		},
	}

	cmd := &cobra.Command{Use: "update"}
	cmd.Flags().String("json", "", "")
	cmd.Flags().String("params", "", "")
	specs := BuildFlagSpecs(schema, hints)
	applyFlagSpecs(cmd, specs)
	if err := cmd.ParseFlags([]string{"--base-id", "base-1"}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}

	params, err := normalizeCanonicalParams(cmd, schema, specs, hints)
	if err != nil {
		t.Fatalf("normalizeCanonicalParams() error = %v", err)
	}
	if got := params["baseId"]; got != "base-1" {
		t.Fatalf("params[baseId] = %#v, want base-1", got)
	}
	if _, exists := params["aiConfig"]; exists {
		t.Fatalf("params unexpectedly populated hidden aiConfig: %#v", params)
	}
}

func TestNormalizeCanonicalParamsPreservesPayloadPrecedenceAcrossAliases(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{"type": "string"},
		},
	}
	hints := map[string]ir.CLIFlagHint{
		"title": {
			Alias: "name",
		},
	}
	specs := BuildFlagSpecs(schema, hints)

	tests := []struct {
		name      string
		flagArgs  []string
		wantTitle string
	}{
		{
			name:      "params canonical beats json alias",
			flagArgs:  []string{"--json", `{"name":"from-json"}`, "--params", `{"title":"from-params"}`},
			wantTitle: "from-params",
		},
		{
			name:      "params alias beats json canonical",
			flagArgs:  []string{"--json", `{"title":"from-json"}`, "--params", `{"name":"from-params"}`},
			wantTitle: "from-params",
		},
		{
			name:      "canonical key wins within same payload",
			flagArgs:  []string{"--json", `{"title":"canonical","name":"alias"}`},
			wantTitle: "canonical",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := &cobra.Command{Use: "create"}
			cmd.Flags().String("json", "", "")
			cmd.Flags().String("params", "", "")
			applyFlagSpecs(cmd, specs)
			if err := cmd.ParseFlags(tt.flagArgs); err != nil {
				t.Fatalf("ParseFlags() error = %v", err)
			}

			params, err := normalizeCanonicalParams(cmd, schema, specs, hints)
			if err != nil {
				t.Fatalf("normalizeCanonicalParams() error = %v", err)
			}

			if got := params["title"]; got != tt.wantTitle {
				t.Fatalf("params[title] = %#v, want %q", got, tt.wantTitle)
			}
			if _, ok := params["name"]; ok {
				t.Fatalf("params unexpectedly kept alias key: %#v", params)
			}
		})
	}
}

func TestNormalizeCanonicalParamsResolvesFlattenedPayloadAliases(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"cursor":  map[string]any{"type": "string"},
			"keyword": map[string]any{"type": "string"},
		},
	}
	hints := map[string]ir.CLIFlagHint{
		"cursor":  {Alias: "cursor"},
		"keyword": {Alias: "query"},
	}
	specs := BuildFlagSpecs(schema, hints)

	tests := []struct {
		name        string
		flagArgs    []string
		wantKeyword string
	}{
		{
			name:        "json alias maps to canonical property",
			flagArgs:    []string{"--json", `{"query":"from-json"}`},
			wantKeyword: "from-json",
		},
		{
			name:        "params alias overrides json alias",
			flagArgs:    []string{"--json", `{"query":"from-json"}`, "--params", `{"query":"from-params"}`},
			wantKeyword: "from-params",
		},
		{
			name:        "canonical payload beats alias in same object",
			flagArgs:    []string{"--json", `{"keyword":"canonical","query":"alias"}`},
			wantKeyword: "canonical",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := &cobra.Command{Use: "search"}
			cmd.Flags().String("json", "", "")
			cmd.Flags().String("params", "", "")
			applyFlagSpecs(cmd, specs)
			if err := cmd.ParseFlags(tt.flagArgs); err != nil {
				t.Fatalf("ParseFlags() error = %v", err)
			}

			params, err := normalizeCanonicalParams(cmd, schema, specs, hints)
			if err != nil {
				t.Fatalf("normalizeCanonicalParams() error = %v", err)
			}
			if got := params["keyword"]; got != tt.wantKeyword {
				t.Fatalf("params[keyword] = %#v, want %#v", got, tt.wantKeyword)
			}
		})
	}
}

func TestNormalizeCanonicalParamsSupportsFlatAndNestedWrapperPayloads(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"OpenSearchRequest": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"cursor": map[string]any{"type": "string"},
					"query":  map[string]any{"type": "string"},
				},
				"required": []any{"query"},
			},
		},
	}
	hints := map[string]ir.CLIFlagHint{
		"OpenSearchRequest.cursor": {Alias: "cursor"},
		"OpenSearchRequest.query":  {Alias: "query"},
	}
	specs := BuildFlagSpecs(schema, hints)

	tests := []struct {
		name       string
		flagArgs   []string
		wantCursor string
		wantQuery  string
	}{
		{
			name:       "flat json payload",
			flagArgs:   []string{"--json", `{"query":"from-json","cursor":"page-1"}`},
			wantCursor: "page-1",
			wantQuery:  "from-json",
		},
		{
			name:       "nested json payload",
			flagArgs:   []string{"--json", `{"OpenSearchRequest":{"query":"from-json","cursor":"page-1"}}`},
			wantCursor: "page-1",
			wantQuery:  "from-json",
		},
		{
			name:       "flag overrides payload",
			flagArgs:   []string{"--json", `{"OpenSearchRequest":{"query":"from-json","cursor":"page-1"}}`, "--params", `{"query":"from-params"}`, "--query", "from-flag"},
			wantCursor: "page-1",
			wantQuery:  "from-flag",
		},
		{
			name:       "nested canonical beats flat public key",
			flagArgs:   []string{"--json", `{"query":"flat","OpenSearchRequest":{"query":"nested","cursor":"page-1"}}`},
			wantCursor: "page-1",
			wantQuery:  "nested",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := &cobra.Command{Use: "search"}
			cmd.Flags().String("json", "", "")
			cmd.Flags().String("params", "", "")
			applyFlagSpecs(cmd, specs)
			if err := cmd.ParseFlags(tt.flagArgs); err != nil {
				t.Fatalf("ParseFlags() error = %v", err)
			}

			params, err := normalizeCanonicalParams(cmd, schema, specs, hints)
			if err != nil {
				t.Fatalf("normalizeCanonicalParams() error = %v", err)
			}

			request, ok := params["OpenSearchRequest"].(map[string]any)
			if !ok {
				t.Fatalf("params[OpenSearchRequest] = %#v, want nested map", params["OpenSearchRequest"])
			}
			if got := request["cursor"]; got != tt.wantCursor {
				t.Fatalf("request[cursor] = %#v, want %#v", got, tt.wantCursor)
			}
			if got := request["query"]; got != tt.wantQuery {
				t.Fatalf("request[query] = %#v, want %#v", got, tt.wantQuery)
			}
		})
	}
}

func TestNormalizeCanonicalParamsKeepsEmptyObjectPayloads(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"viewDescription": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
				},
			},
			"config": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"fieldWidths": map[string]any{
						"type":                 "object",
						"additionalProperties": map[string]any{"type": "integer"},
					},
				},
			},
		},
	}
	hints := map[string]ir.CLIFlagHint{
		"config.fieldWidths": {Alias: "fieldWidths"},
	}

	cmd := &cobra.Command{Use: "update-view"}
	cmd.Flags().String("json", "", "")
	cmd.Flags().String("params", "", "")
	specs := BuildFlagSpecs(schema, hints)
	applyFlagSpecs(cmd, specs)
	if err := cmd.ParseFlags([]string{"--json", `{"viewDescription":{},"fieldWidths":{}}`}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}

	params, err := normalizeCanonicalParams(cmd, schema, specs, hints)
	if err != nil {
		t.Fatalf("normalizeCanonicalParams() error = %v", err)
	}

	if got, ok := params["viewDescription"].(map[string]any); !ok || len(got) != 0 {
		t.Fatalf("params[viewDescription] = %#v, want empty object", params["viewDescription"])
	}

	config, ok := params["config"].(map[string]any)
	if !ok {
		t.Fatalf("params[config] = %#v, want nested map", params["config"])
	}
	if got, ok := config["fieldWidths"].(map[string]any); !ok || len(got) != 0 {
		t.Fatalf("config[fieldWidths] = %#v, want empty object", config["fieldWidths"])
	}
}

func TestNormalizeCanonicalParamsCoercesEnvDefaultsToSchema(t *testing.T) {
	t.Setenv("TEST_LIMIT", "12")
	t.Setenv("TEST_ENABLED", "true")
	t.Setenv("TEST_TAGS", "alpha,beta")
	t.Setenv("TEST_REVIEWERS", "alice,bob")
	t.Setenv("TEST_METADATA", `{"owner":"ops"}`)
	t.Setenv("TEST_FILTER", `{"owner":"me"}`)

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"limit":   map[string]any{"type": "integer"},
			"enabled": map[string]any{"type": "boolean"},
			"tags": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"reviewers": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"metadata": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
			"filter": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"owner": map[string]any{"type": "string"},
				},
			},
		},
	}
	hints := map[string]ir.CLIFlagHint{
		"limit":   {EnvDefault: "TEST_LIMIT"},
		"enabled": {EnvDefault: "TEST_ENABLED"},
		"tags": {
			EnvDefault: "TEST_TAGS",
			Transform:  "csv_to_array",
		},
		"reviewers": {EnvDefault: "TEST_REVIEWERS"},
		"metadata":  {EnvDefault: "TEST_METADATA"},
		"filter": {
			EnvDefault: "TEST_FILTER",
			Transform:  "json_parse",
		},
	}

	cmd := &cobra.Command{Use: "search"}
	cmd.Flags().String("json", "", "")
	cmd.Flags().String("params", "", "")
	specs := []FlagSpec{
		{PropertyName: "limit", FlagName: "limit", Kind: flagInteger},
		{PropertyName: "enabled", FlagName: "enabled", Kind: flagBoolean},
		{PropertyName: "tags", FlagName: "tags", Kind: flagStringArray},
		{PropertyName: "reviewers", FlagName: "reviewers", Kind: flagStringArray},
		{PropertyName: "metadata", FlagName: "metadata", Kind: flagJSON},
		{PropertyName: "filter", FlagName: "filter", Kind: flagJSON},
	}
	applyFlagSpecs(cmd, specs)

	params, err := normalizeCanonicalParams(cmd, schema, specs, hints)
	if err != nil {
		t.Fatalf("normalizeCanonicalParams() error = %v", err)
	}

	if got := params["limit"]; got != 12 {
		t.Fatalf("params[limit] = %#v, want 12", got)
	}
	if got := params["enabled"]; got != true {
		t.Fatalf("params[enabled] = %#v, want true", got)
	}
	if want := []any{"alpha", "beta"}; !reflect.DeepEqual(params["tags"], want) {
		t.Fatalf("params[tags] = %#v, want %#v", params["tags"], want)
	}
	if want := []any{"alice", "bob"}; !reflect.DeepEqual(params["reviewers"], want) {
		t.Fatalf("params[reviewers] = %#v, want %#v", params["reviewers"], want)
	}
	if want := map[string]any{"owner": "ops"}; !reflect.DeepEqual(params["metadata"], want) {
		t.Fatalf("params[metadata] = %#v, want %#v", params["metadata"], want)
	}
	if want := map[string]any{"owner": "me"}; !reflect.DeepEqual(params["filter"], want) {
		t.Fatalf("params[filter] = %#v, want %#v", params["filter"], want)
	}
}

func TestNormalizeCanonicalParamsRenestsDeepDottedPaths(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"A": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"B": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"C": map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	}
	hints := map[string]ir.CLIFlagHint{
		"A.B.C": {Alias: "c"},
	}

	cmd := &cobra.Command{Use: "deep"}
	cmd.Flags().String("json", "", "")
	cmd.Flags().String("params", "", "")
	specs := BuildFlagSpecs(schema, hints)
	applyFlagSpecs(cmd, specs)
	if err := cmd.ParseFlags([]string{"--c", "value"}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}

	params, err := normalizeCanonicalParams(cmd, schema, specs, hints)
	if err != nil {
		t.Fatalf("normalizeCanonicalParams() error = %v", err)
	}

	levelA, ok := params["A"].(map[string]any)
	if !ok {
		t.Fatalf("params[A] = %#v, want nested map", params["A"])
	}
	levelB, ok := levelA["B"].(map[string]any)
	if !ok {
		t.Fatalf("levelA[B] = %#v, want nested map", levelA["B"])
	}
	if got := levelB["C"]; got != "value" {
		t.Fatalf("levelB[C] = %#v, want value", got)
	}
}

func TestNormalizeCanonicalParamsKeepsRealPropertyWhenAliasCollides(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{"type": "string"},
			"name":  map[string]any{"type": "string"},
		},
	}
	hints := map[string]ir.CLIFlagHint{
		"title": {
			Alias: "name",
		},
	}

	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().String("json", "", "")
	cmd.Flags().String("params", "", "")
	specs := BuildFlagSpecs(schema, hints)
	applyFlagSpecs(cmd, specs)
	if err := cmd.ParseFlags([]string{"--json", `{"name":"real-name"}`}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}

	params, err := normalizeCanonicalParams(cmd, schema, specs, hints)
	if err != nil {
		t.Fatalf("normalizeCanonicalParams() error = %v", err)
	}

	if got := params["name"]; got != "real-name" {
		t.Fatalf("params[name] = %#v, want real-name", got)
	}
	if _, ok := params["title"]; ok {
		t.Fatalf("params unexpectedly rewrote colliding property into alias target: %#v", params)
	}
}

func TestNormalizeCanonicalParamsCoercesCSVArrayFlagsToSchemaItemTypes(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"deptIds": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "number"},
			},
		},
	}
	hints := map[string]ir.CLIFlagHint{
		"deptIds": {
			Alias:     "ids",
			Transform: "csv_to_array",
			Required:  true,
		},
	}

	cmd := &cobra.Command{Use: "list-members"}
	cmd.Flags().String("json", "", "")
	cmd.Flags().String("params", "", "")
	specs := BuildFlagSpecs(schema, hints)
	applyFlagSpecs(cmd, specs)
	if err := cmd.ParseFlags([]string{"--ids", "[7,8]"}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}

	params, err := normalizeCanonicalParams(cmd, schema, specs, hints)
	if err != nil {
		t.Fatalf("normalizeCanonicalParams() error = %v", err)
	}

	want := []any{float64(7), float64(8)}
	if !reflect.DeepEqual(params["deptIds"], want) {
		t.Fatalf("params[deptIds] = %#v, want %#v", params["deptIds"], want)
	}
	if err := ValidateInputSchema(params, schema); err != nil {
		t.Fatalf("ValidateInputSchema() error = %v", err)
	}
}

func TestNormalizeCanonicalParamsCoercesJSONPayloadScalarsToStringSchema(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"isDone": map[string]any{"type": "string"},
			"taskId": map[string]any{"type": "string"},
		},
		"required": []any{"isDone", "taskId"},
	}
	hints := map[string]ir.CLIFlagHint{
		"isDone": {
			Alias:    "status",
			Required: true,
		},
		"taskId": {
			Alias:    "task-id",
			Required: true,
		},
	}

	cmd := &cobra.Command{Use: "done"}
	cmd.Flags().String("json", "", "")
	cmd.Flags().String("params", "", "")
	specs := BuildFlagSpecs(schema, hints)
	applyFlagSpecs(cmd, specs)
	if err := cmd.ParseFlags([]string{"--json", `{"taskId":51593402605,"isDone":true}`}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}

	params, err := normalizeCanonicalParams(cmd, schema, specs, hints)
	if err != nil {
		t.Fatalf("normalizeCanonicalParams() error = %v", err)
	}

	if got := params["isDone"]; got != "true" {
		t.Fatalf("params[isDone] = %#v, want \"true\"", got)
	}
	if got := params["taskId"]; got != "51593402605" {
		t.Fatalf("params[taskId] = %#v, want task id string", got)
	}
	if err := ValidateInputSchema(params, schema); err != nil {
		t.Fatalf("ValidateInputSchema() error = %v", err)
	}
}

func TestNormalizeCanonicalParamsCoercesJSONArrayObjectFieldsToStringSchema(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"contents": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"content":     map[string]any{"type": "string"},
						"contentType": map[string]any{"type": "string"},
						"key":         map[string]any{"type": "string"},
						"sort":        map[string]any{"type": "string"},
						"type":        map[string]any{"type": "string"},
					},
					"required": []any{"content", "contentType", "key", "sort", "type"},
				},
			},
		},
		"required": []any{"contents"},
	}
	hints := map[string]ir.CLIFlagHint{
		"contents": {
			Alias:     "contents",
			Transform: "json_parse",
			Required:  true,
		},
	}

	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().String("json", "", "")
	cmd.Flags().String("params", "", "")
	specs := BuildFlagSpecs(schema, hints)
	applyFlagSpecs(cmd, specs)
	if err := cmd.ParseFlags([]string{"--contents", `[{"content":"日报","contentType":"markdown","key":"日报内容","sort":1,"type":1}]`}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}

	params, err := normalizeCanonicalParams(cmd, schema, specs, hints)
	if err != nil {
		t.Fatalf("normalizeCanonicalParams() error = %v", err)
	}

	contents, ok := params["contents"].([]any)
	if !ok || len(contents) != 1 {
		t.Fatalf("params[contents] = %#v, want single-item []any", params["contents"])
	}
	item, ok := contents[0].(map[string]any)
	if !ok {
		t.Fatalf("contents[0] = %#v, want object", contents[0])
	}
	if got := item["sort"]; got != "1" {
		t.Fatalf("contents[0][sort] = %#v, want \"1\"", got)
	}
	if got := item["type"]; got != "1" {
		t.Fatalf("contents[0][type] = %#v, want \"1\"", got)
	}
	if err := ValidateInputSchema(params, schema); err != nil {
		t.Fatalf("ValidateInputSchema() error = %v", err)
	}
}

func TestNormalizeCanonicalParamsSkipsISOTransformWhenSchemaExpectsString(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"startTime": map[string]any{"type": "string"},
		},
	}
	hints := map[string]ir.CLIFlagHint{
		"startTime": {
			Alias:     "start",
			Transform: "iso8601_to_millis",
			Required:  true,
		},
	}

	cmd := &cobra.Command{Use: "search"}
	cmd.Flags().String("json", "", "")
	cmd.Flags().String("params", "", "")
	specs := BuildFlagSpecs(schema, hints)
	applyFlagSpecs(cmd, specs)
	if err := cmd.ParseFlags([]string{"--start", "2026-03-27T08:09:10Z"}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}

	params, err := normalizeCanonicalParams(cmd, schema, specs, hints)
	if err != nil {
		t.Fatalf("normalizeCanonicalParams() error = %v", err)
	}

	if got := params["startTime"]; got != "2026-03-27T08:09:10Z" {
		t.Fatalf("params[startTime] = %#v, want ISO-8601 string", got)
	}
	if err := ValidateInputSchema(params, schema); err != nil {
		t.Fatalf("ValidateInputSchema() error = %v", err)
	}
}
