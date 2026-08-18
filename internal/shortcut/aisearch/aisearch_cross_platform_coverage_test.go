// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package aisearch

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type aisearchCaller struct {
	product string
	tool    string
	args    map[string]any
	payload string
	calls   int
}

func (caller *aisearchCaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	caller.product, caller.tool, caller.args = product, tool, args
	caller.calls++
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: caller.payload}}}, nil
}

func (*aisearchCaller) Format() string { return "json" }
func (*aisearchCaller) DryRun() bool   { return false }
func (*aisearchCaller) Fields() string { return "" }
func (*aisearchCaller) JQ() string     { return "" }

func TestCrossPlatformCoverageAiSearchRejectsFalseSuccessAndBadCollections(t *testing.T) {
	valid := map[string]any{"success": true, "result": []any{map[string]any{"sourceType": "user", "userId": "u1"}}}
	items, err := projectSearch(valid, "aisearch/test", []string{"userId"})
	if err != nil || len(items) != 1 {
		t.Fatalf("valid response rejected: items=%v err=%v", items, err)
	}
	explicitEmpty := map[string]any{"success": true, "result": []any{}}
	items, err = projectSearch(explicitEmpty, "aisearch/test", []string{"userId"})
	if err != nil || len(items) != 0 {
		t.Fatalf("explicit empty rejected: items=%v err=%v", items, err)
	}

	broken := []map[string]any{
		{},
		{"result": []any{}},
		{"success": "true", "result": []any{}},
		{"success": false, "result": []any{}},
		{"success": true},
		{"success": true, "result": map[string]any{}},
		{"success": true, "result": []any{"bad"}},
		{"success": true, "result": []any{map[string]any{}}},
		{"success": true, "result": []any{map[string]any{"sourceType": "user"}}},
		{"success": true, "result": []any{map[string]any{"sourceType": "user", "userId": 1}}},
		{"success": true, "errorCode": "REMOTE_ERROR", "result": []any{}},
		{"success": true, "errorMsg": "conflict", "result": []any{}},
	}
	for index, data := range broken {
		if got, projectErr := projectSearch(data, "aisearch/test", []string{"userId"}); projectErr == nil {
			t.Errorf("broken response %d returned success: %v", index, got)
		}
	}
}

func TestCrossPlatformCoverageAiSearchSelectorsFailBeforeRemoteCall(t *testing.T) {
	caller := &aisearchCaller{payload: `{"success":true,"result":[]}`}
	helpers.InitDepsForTest(t, caller)
	cmd := &cobra.Command{Use: "+search-person"}
	cmd.Flags().StringSlice("dimensions", []string{"all", "name"}, "")
	if err := validatePerson(shortcut.RuntimeContextForTest(cmd, SearchPerson)); err == nil {
		t.Fatal("all plus another dimension must fail")
	}
	if caller.calls != 0 {
		t.Fatalf("invalid selector made %d remote calls", caller.calls)
	}

	behavior := &cobra.Command{Use: "+search-behavior"}
	behavior.Flags().StringSlice("types", []string{"document"}, "")
	behavior.Flags().String("chat-scope", "fixture-chat", "")
	if err := validateBehavior(shortcut.RuntimeContextForTest(behavior, SearchBehavior)); err == nil {
		t.Fatal("chat scope without im must fail")
	}
	if caller.calls != 0 {
		t.Fatalf("invalid behavior selector made %d remote calls", caller.calls)
	}

	enterprise := &cobra.Command{Use: "+search-enterprise"}
	enterprise.Flags().StringSlice("queries", []string{""}, "")
	enterprise.Flags().StringSlice("types", []string{"im"}, "")
	if err := validateEnterprise(shortcut.RuntimeContextForTest(enterprise, SearchEnterprise)); err == nil {
		t.Fatal("empty enterprise query must fail")
	}
	if caller.calls != 0 {
		t.Fatalf("empty enterprise query made %d remote calls", caller.calls)
	}
}

func TestCrossPlatformCoverageUnavailableAiSearchMakesNoRemoteCall(t *testing.T) {
	caller := &aisearchCaller{payload: `{"success":true,"result":[]}`}
	helpers.InitDepsForTest(t, caller)
	for _, declaration := range []shortcut.Shortcut{SearchEnterprise, SearchBehavior} {
		if err := declaration.Execute(shortcut.RuntimeContextForTest(&cobra.Command{Use: declaration.Command}, declaration)); err == nil || !strings.Contains(err.Error(), "cannot prove query relevance") {
			t.Errorf("%s unavailable error = %v", declaration.Command, err)
		}
	}
	if caller.calls != 0 {
		t.Fatalf("unavailable searches made %d remote calls", caller.calls)
	}
}

func TestCrossPlatformCoverageAiSearchExactShortcutMapping(t *testing.T) {
	caller := &aisearchCaller{payload: `{"success":true,"errorCode":null,"errorMsg":"","result":[{"sourceType":"user","userId":"stable-user"}]}`}
	helpers.InitDepsForTest(t, caller)

	declaration := SearchPerson
	declaration.OutputRollout = output.RolloutLegacyOnly
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	service := &cobra.Command{Use: "aisearch"}
	service.AddCommand(corecmd.New(shortcut.FromShortcut(declaration)))
	root.AddCommand(service)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"aisearch", "+search-person", "--query", "fixture", "--dimensions", "name,duty"})
	if err := root.Execute(); err != nil {
		t.Fatalf("exact shortcut execution: %v", err)
	}
	if caller.calls != 1 || caller.product != "aisearch" || caller.tool != "enterprise_person_search" {
		t.Fatalf("call = count:%d product:%q tool:%q", caller.calls, caller.product, caller.tool)
	}
	if caller.args["keyword"] != "fixture" {
		t.Fatalf("keyword = %#v", caller.args["keyword"])
	}
	dimensions, ok := caller.args["dimension"].([]string)
	if !ok || strings.Join(dimensions, ",") != "name,duty" {
		t.Fatalf("dimensions = %#v", caller.args["dimension"])
	}
}

func TestCrossPlatformCoverageAiSearchEnterpriseHardenedMapperSourceGuard(t *testing.T) {
	caller := &aisearchCaller{payload: `{"success":true,"result":[{"sourceType":"im","openConversationId":"cid-fixture"}]}`}
	helpers.InitDepsForTest(t, caller)

	declaration := SearchEnterprise
	declaration.OutputRollout = output.RolloutLegacyOnly
	command := &cobra.Command{Use: "+search-enterprise"}
	command.Flags().StringSlice("queries", []string{"fixture"}, "")
	command.Flags().StringSlice("types", []string{"im"}, "")
	command.Flags().String("time-range", "过去一周", "")
	runtime := shortcut.RuntimeContextForTest(command, declaration)
	if err := executeSearchForSource(runtime, "search_enterprise", map[string]any{
		"queries": []string{"fixture"}, "searchTypes": []string{"im"}, "timeRange": "过去一周",
	}, []string{"openConversationId", "url"}, "im"); err != nil {
		t.Fatalf("hardened enterprise mapper: %v", err)
	}
	if caller.calls != 1 || caller.product != "aisearch" || caller.tool != "search_enterprise" {
		t.Fatalf("call = count:%d product:%q tool:%q", caller.calls, caller.product, caller.tool)
	}
	if got := caller.args["searchTypes"]; len(got.([]string)) != 1 || got.([]string)[0] != "im" {
		t.Fatalf("searchTypes = %#v", got)
	}
	if caller.args["timeRange"] != "过去一周" {
		t.Fatalf("timeRange = %#v", caller.args["timeRange"])
	}

	caller.payload = `{"success":true,"result":[{"sourceType":"minute","openConversationId":"cid-fixture"}]}`
	badCommand := &cobra.Command{Use: "+search-enterprise"}
	badCommand.Flags().StringSlice("queries", []string{"fixture"}, "")
	badCommand.Flags().StringSlice("types", []string{"im"}, "")
	badCommand.Flags().String("time-range", "", "")
	if err := executeSearchForSource(
		shortcut.RuntimeContextForTest(badCommand, declaration),
		"search_enterprise",
		map[string]any{"queries": []string{"fixture"}, "searchTypes": []string{"im"}},
		[]string{"openConversationId", "url"},
		"im",
	); err == nil || !strings.Contains(err.Error(), "来源") {
		t.Fatalf("source drift error = %v", err)
	}
}

func TestCrossPlatformCoverageAiSearchCatalogAndContracts(t *testing.T) {
	registered := map[string]shortcut.Shortcut{}
	for _, item := range shortcut.All() {
		if item.Service == "aisearch" {
			registered[item.Command] = item
		}
	}
	if len(registered) != 3 {
		t.Fatalf("registered AiSearch shortcuts = %d, want 3", len(registered))
	}
	for _, command := range []string{"+search-person", "+search-enterprise", "+search-behavior"} {
		item := registered[command]
		if item.Command == "" {
			t.Fatalf("missing %s", command)
		}
		if item.OutputRollout != output.RolloutUnifiedActive || item.Contract.Result == nil || strings.TrimSpace(item.Safety.Effect) == "" {
			t.Errorf("%s lacks unified typed contract: %#v", command, item)
		}
		var schema map[string]any
		if err := json.Unmarshal(item.Contract.Result.DataSchema, &schema); err != nil || schema["type"] != "object" {
			t.Errorf("%s result schema invalid: schema=%v err=%v", command, schema, err)
		}
	}
	if registered["+search-person"].Hidden || registered["+search-person"].Availability != shortcut.AvailabilityAvailable {
		t.Fatalf("search-person visibility/availability = hidden:%v availability:%q", registered["+search-person"].Hidden, registered["+search-person"].Availability)
	}
	for _, command := range []string{"+search-enterprise", "+search-behavior"} {
		if !registered[command].Hidden || registered[command].Availability != shortcut.AvailabilityUnavailable {
			t.Errorf("%s must remain hidden/unavailable: hidden=%v availability=%q", command, registered[command].Hidden, registered[command].Availability)
		}
	}
}
