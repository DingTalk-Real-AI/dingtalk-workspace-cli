// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/syncdata"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageLoadRegistryInterfaceRefsCollectsIdentity(t *testing.T) {
	var stderr bytes.Buffer
	refs := loadRegistryInterfaceRefs(&stderr)
	if len(refs) == 0 {
		t.Fatal("loadRegistryInterfaceRefs() returned no collected commands")
	}

	got, ok := refs["calendar.list_calendars"]
	if !ok {
		t.Fatal("calendar.list_calendars missing from collected command identity")
	}
	if got["product_id"] != "calendar" || got["rpc_name"] != "list_calendars" {
		t.Fatalf("calendar.list_calendars ref = %#v", got)
	}

	direct, err := collectedIdentityInterfaceRefs()
	if err != nil {
		t.Fatalf("collectedIdentityInterfaceRefs() error = %v", err)
	}
	if len(direct) == 0 || direct["calendar.list_calendars"]["rpc_name"] != "list_calendars" {
		t.Fatalf("collectedIdentityInterfaceRefs() = %#v", direct["calendar.list_calendars"])
	}

	prevRegistry := registrySource
	prevCollect := collectIdentitySpecs
	t.Cleanup(func() {
		registrySource = prevRegistry
		collectIdentitySpecs = prevCollect
	})
	registrySource = func() (map[string]map[string]string, error) {
		return nil, errors.New("collect boom")
	}
	stderr.Reset()
	if got := loadRegistryInterfaceRefs(&stderr); len(got) != 0 || !strings.Contains(stderr.String(), "cannot collect command identity") {
		t.Fatalf("loadRegistryInterfaceRefs error path = %#v stderr=%q", got, stderr.String())
	}

	collectIdentitySpecs = func(*cobra.Command) ([]cli.CommandSpec, cli.IdentityCollectionReport, error) {
		return nil, cli.IdentityCollectionReport{}, errors.New("walk boom")
	}
	if _, err := collectedIdentityInterfaceRefs(); err == nil || !strings.Contains(err.Error(), "collect command identity") {
		t.Fatalf("collectedIdentityInterfaceRefs wrap error = %v", err)
	}
	collectIdentitySpecs = func(*cobra.Command) ([]cli.CommandSpec, cli.IdentityCollectionReport, error) {
		return []cli.CommandSpec{
			{CanonicalPath: ""},
			{CanonicalPath: "nodot"},
			{CanonicalPath: "doc.create"},
		}, cli.IdentityCollectionReport{}, nil
	}
	gotRefs, err := collectedIdentityInterfaceRefs()
	if err != nil || len(gotRefs) != 1 || gotRefs["doc.create"]["rpc_name"] != "create" {
		t.Fatalf("collectedIdentityInterfaceRefs skip = %#v err=%v", gotRefs, err)
	}
}

func TestBuildCrossServerRefs(t *testing.T) {
	registryMap := map[string]map[string]string{
		"aitable.advperm_enable":  {"product_id": "aitable", "rpc_name": "advperm_enable"},
		"aitable.advperm_disable": {"product_id": "aitable", "rpc_name": "advperm_disable"},
		"doc.copy_document":       {"product_id": "doc", "rpc_name": "copy_document"},
	}
	prevTools := map[string]map[string]any{
		// Fan-out: two canonicals share one live tool; insertion order must
		// not affect the sorted result.
		"aitable.advperm_enable": {
			"interface_ref": map[string]any{"product_id": "aitable-helper", "rpc_name": "set_advanced_permission"},
		},
		"aitable.advperm_disable": {
			"interface_ref": map[string]any{"product_id": "aitable-helper", "rpc_name": "set_advanced_permission"},
		},
		// Identity ref (live key == canonical) needs no cross entry.
		"doc.copy_document": {
			"interface_ref": map[string]any{"product_id": "doc", "rpc_name": "copy_document"},
		},
		// Not in the registry: must be ignored.
		"ghost.tool": {
			"interface_ref": map[string]any{"product_id": "ghost-helper", "rpc_name": "haunt"},
		},
	}
	got := buildCrossServerRefs(prevTools, registryMap)
	want := map[string][]string{
		"aitable-helper.set_advanced_permission": {"aitable.advperm_disable", "aitable.advperm_enable"},
	}
	if len(got) != len(want) {
		t.Fatalf("index = %#v, want %#v", got, want)
	}
	for k, v := range want {
		if gv := got[k]; len(gv) != len(v) || gv[0] != v[0] || gv[1] != v[1] {
			t.Fatalf("index[%q] = %v, want %v", k, gv, v)
		}
	}
}

func TestBuildCrossServerRefsSkipsMalformedRefs(t *testing.T) {
	registryMap := map[string]map[string]string{
		"a.x": {"product_id": "a", "rpc_name": "x"},
		"a.y": {"product_id": "a", "rpc_name": "y"},
		"a.z": {"product_id": "a", "rpc_name": "z"},
	}
	prevTools := map[string]map[string]any{
		"a.x": {"interface_ref": "not-a-map"},
		"a.y": {"interface_ref": map[string]any{"product_id": "", "rpc_name": "r"}},
		"a.z": {"title": "no ref at all"},
	}
	if got := buildCrossServerRefs(prevTools, registryMap); len(got) != 0 {
		t.Fatalf("index = %#v, want empty", got)
	}
}

func TestRunRefreshesCrossServerTools(t *testing.T) {
	registry := func() (map[string]map[string]string, error) {
		return map[string]map[string]string{
			"aitable.advperm_enable":  {"product_id": "aitable", "rpc_name": "advperm_enable"},
			"aitable.advperm_disable": {"product_id": "aitable", "rpc_name": "advperm_disable"},
		}, nil
	}
	servers := []syncdata.ServerInfo{{ID: "aitable-helper", Endpoint: "https://helper.example"}}
	lister := &fakeLister{
		results: map[string]transport.ToolsListResult{
			"https://helper.example": {Tools: []transport.ToolDescriptor{
				{Name: "set_advanced_permission", Title: "live title", Description: "live desc"},
			}},
		},
	}
	stubDeps(t, "env-token", nil, servers, lister, registry)

	output := filepath.Join(t.TempDir(), "snapshot.json")
	prev := `{"tools":{
		"aitable.advperm_enable":{"title":"stale","interface_ref":{"product_id":"aitable-helper","rpc_name":"set_advanced_permission"}},
		"aitable.advperm_disable":{"title":"stale","interface_ref":{"product_id":"aitable-helper","rpc_name":"set_advanced_permission"}}
	}}`
	if err := os.WriteFile(output, []byte(prev), 0o600); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	if code := run([]string{"--output", output}, &stderr); code != 0 {
		t.Fatalf("run() = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "cross-server ref index: 1 live keys") {
		t.Fatalf("stderr = %q, want cross-server index log", stderr.String())
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Tools map[string]map[string]any `json:"tools"`
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	for _, canonical := range []string{"aitable.advperm_enable", "aitable.advperm_disable"} {
		entry := snapshot.Tools[canonical]
		if entry["title"] != "live title" || entry["description"] != "live desc" {
			t.Fatalf("%s = %#v, want live refresh", canonical, entry)
		}
		ref := entry["interface_ref"].(map[string]any)
		if ref["product_id"] != "aitable-helper" || ref["rpc_name"] != "set_advanced_permission" {
			t.Fatalf("%s reviewed ref lost: %#v", canonical, ref)
		}
	}
}

// TestRunCrossOwnedCanonicalIgnoresNameCoincidence：canonical 拥有评审过的
// 跨 server 身份时，另一 server 上恰好同名的工具不得直连覆盖其元数据——
// 数据源只能是评审身份指向的 live 工具。
func TestRunCrossOwnedCanonicalIgnoresNameCoincidence(t *testing.T) {
	registry := func() (map[string]map[string]string, error) {
		return map[string]map[string]string{
			"aitable.advperm_enable": {"product_id": "aitable", "rpc_name": "advperm_enable"},
		}, nil
	}
	servers := []syncdata.ServerInfo{
		{ID: "aitable", Endpoint: "https://aitable.example"},
		{ID: "aitable-helper", Endpoint: "https://helper.example"},
	}
	lister := &fakeLister{
		results: map[string]transport.ToolsListResult{
			// 同名巧合：aitable server 上恰好也有 advperm_enable。
			"https://aitable.example": {Tools: []transport.ToolDescriptor{
				{Name: "advperm_enable", Title: "coincidence title", Description: "coincidence desc"},
			}},
			"https://helper.example": {Tools: []transport.ToolDescriptor{
				{Name: "set_advanced_permission", Title: "owner title", Description: "owner desc"},
			}},
		},
	}
	stubDeps(t, "env-token", nil, servers, lister, registry)

	output := filepath.Join(t.TempDir(), "snapshot.json")
	prev := `{"tools":{"aitable.advperm_enable":{"title":"stale","interface_ref":{"product_id":"aitable-helper","rpc_name":"set_advanced_permission"}}}}`
	if err := os.WriteFile(output, []byte(prev), 0o600); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	if code := run([]string{"--output", output}, &stderr); code != 0 {
		t.Fatalf("run() = %d, stderr=%s", code, stderr.String())
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Tools map[string]map[string]any `json:"tools"`
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	entry := snapshot.Tools["aitable.advperm_enable"]
	if entry["title"] != "owner title" || entry["description"] != "owner desc" {
		t.Fatalf("entry = %#v, want reviewed-identity source to win over name coincidence", entry)
	}
}

func TestMergeLiveMCPToolRefreshesExistingMetadata(t *testing.T) {
	const canonical = "calendar.list_calendars"
	reviewedRef := map[string]any{
		"product_id": "calendar-helper",
		"rpc_name":   "list_user_calendars",
	}
	allTools := map[string]map[string]any{
		canonical: {
			"title":         "old title",
			"description":   "old description",
			"interface_ref": reviewedRef,
			"parameters": map[string]any{
				"stale": map[string]any{"type": "string"},
			},
		},
	}
	live := transport.ToolDescriptor{
		Name:        "list_calendars",
		Title:       "new title",
		Description: "new description",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cursor": map[string]any{
					"type":        "string",
					"description": "next page cursor",
				},
			},
			"required": []any{"cursor"},
		},
	}
	fallbackRef := map[string]string{
		"product_id": "calendar",
		"rpc_name":   "list_calendars",
	}

	mergeLiveMCPTool(allTools, canonical, live, fallbackRef)

	got := allTools[canonical]
	if got["title"] != "new title" || got["description"] != "new description" {
		t.Fatalf("live metadata was not refreshed: %#v", got)
	}
	if !reflect.DeepEqual(got["interface_ref"], reviewedRef) {
		t.Fatalf("interface_ref = %#v, want reviewed mapping %#v", got["interface_ref"], reviewedRef)
	}
	params, ok := got["parameters"].(map[string]map[string]any)
	if !ok {
		t.Fatalf("parameters type = %T, want refreshed parameter map", got["parameters"])
	}
	if _, stale := params["stale"]; stale {
		t.Fatalf("stale parameter survived refresh: %#v", params)
	}
	if cursor := params["cursor"]; cursor["type"] != "string" || cursor["description"] != "next page cursor" || cursor["required"] != true {
		t.Fatalf("cursor parameter = %#v", cursor)
	}
}

func TestBuildCoverageReportsFailedServices(t *testing.T) {
	got := buildCoverage(26, []string{"doc", "sheet"}, 800, 813, 40)
	if got["source_services"] != 26 {
		t.Fatalf("source_services = %v, want 26", got["source_services"])
	}
	if got["snapshot_services"] != 24 {
		t.Fatalf("snapshot_services = %v, want 24 (26 sources - 2 failures)", got["snapshot_services"])
	}
	if !reflect.DeepEqual(got["missing_services"], []string{"doc", "sheet"}) {
		t.Fatalf("missing_services = %#v, want failed service IDs", got["missing_services"])
	}
	// matched 必须剔除 stub 占位，unmatched 据实等于 stub 数。
	if got["matched_tools"] != 773 || got["unmatched_tools"] != 40 {
		t.Fatalf("matched/unmatched = %v/%v, want 773/40 (813 surface - 40 stubs)", got["matched_tools"], got["unmatched_tools"])
	}
	if got["source_tools"] != 800 || got["surface_tools"] != 813 {
		t.Fatalf("tool counts = %#v", got)
	}
}

func TestBuildCoverageFullSnapshotHasNoMissingServices(t *testing.T) {
	got := buildCoverage(26, nil, 813, 813, 0)
	if got["snapshot_services"] != 26 {
		t.Fatalf("snapshot_services = %v, want 26", got["snapshot_services"])
	}
	if !reflect.DeepEqual(got["missing_services"], []string{}) {
		t.Fatalf("missing_services = %#v, want empty non-nil slice", got["missing_services"])
	}
	if got["matched_tools"] != 813 || got["unmatched_tools"] != 0 {
		t.Fatalf("matched/unmatched = %v/%v, want 813/0 for stub-free snapshot", got["matched_tools"], got["unmatched_tools"])
	}
}

// fakeLister returns canned tools/list results per endpoint.
type fakeLister struct {
	results map[string]transport.ToolsListResult
	errs    map[string]error
}

func (f *fakeLister) ListTools(_ context.Context, endpoint string) (transport.ToolsListResult, error) {
	if err := f.errs[endpoint]; err != nil {
		return transport.ToolsListResult{}, err
	}
	return f.results[endpoint], nil
}

// stubDeps swaps every injection point for the duration of one test.
func stubDeps(t *testing.T, token string, keychain func() (*auth.TokenData, error), servers []syncdata.ServerInfo, lister toolLister, registry func() (map[string]map[string]string, error)) {
	t.Helper()
	origGetenv, origLoad, origServers, origNew, origRegistry := getenv, loadTokenData, staticServers, newToolLister, registrySource
	t.Cleanup(func() {
		getenv, loadTokenData, staticServers, newToolLister, registrySource = origGetenv, origLoad, origServers, origNew, origRegistry
	})
	getenv = func(key string) string {
		if key == "DWS_ACCESS_TOKEN" {
			return token
		}
		return ""
	}
	loadTokenData = keychain
	staticServers = func() []syncdata.ServerInfo { return servers }
	newToolLister = func(string) toolLister { return lister }
	registrySource = registry
}

func testRegistryRefs() (map[string]map[string]string, error) {
	return map[string]map[string]string{
		"doc.copy_document": {"product_id": "doc", "rpc_name": "copy_document"},
		"doc.get_document":  {"product_id": "doc", "rpc_name": "get_document"},
	}, nil
}

func TestCrossPlatformCoverageRunRefusesRetiredPinnedMCPMetadataPath(t *testing.T) {
	stubDeps(t, "env-token", nil, nil, &fakeLister{}, testRegistryRefs)
	var stderr bytes.Buffer
	code := run([]string{"--output", "internal/cli/schema_mcp_metadata.json"}, &stderr)
	if code != 2 {
		t.Fatalf("run(retired pin) = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "refusing to write retired Schema pin") {
		t.Fatalf("stderr = %q, want retired-pin refusal", stderr.String())
	}
	if !retiredPinnedMCPMetadataPath("internal/cli/schema_mcp_metadata.json") ||
		!retiredPinnedMCPMetadataPath("/tmp/repo/internal/cli/schema_mcp_metadata.json") ||
		retiredPinnedMCPMetadataPath("artifacts/mcp_metadata_diagnostic.json") {
		t.Fatal("retiredPinnedMCPMetadataPath classification is incorrect")
	}
}

func TestRunNoTokenFails(t *testing.T) {
	stubDeps(t, "", func() (*auth.TokenData, error) { return nil, errors.New("no keychain") }, nil, &fakeLister{}, testRegistryRefs)
	var stderr bytes.Buffer
	if code := run(nil, &stderr); code != 1 {
		t.Fatalf("run() = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "no auth token") {
		t.Fatalf("stderr = %q, want no-auth-token hint", stderr.String())
	}
}

func TestRunInvalidFlagFails(t *testing.T) {
	stubDeps(t, "tok", nil, nil, &fakeLister{}, testRegistryRefs)
	var stderr bytes.Buffer
	if code := run([]string{"--nonexistent"}, &stderr); code != 2 {
		t.Fatalf("run() = %d, want 2", code)
	}
}

func TestResolveTokenKeychainFallback(t *testing.T) {
	stubDeps(t, "", func() (*auth.TokenData, error) {
		return &auth.TokenData{AccessToken: "kc-token"}, nil
	}, nil, &fakeLister{}, testRegistryRefs)
	var stderr bytes.Buffer
	if got := resolveToken(&stderr); got != "kc-token" {
		t.Fatalf("resolveToken() = %q, want kc-token", got)
	}
	if !strings.Contains(stderr.String(), "loaded token from keychain") {
		t.Fatalf("stderr = %q, want keychain log", stderr.String())
	}
}

func TestResolveTokenEmptyKeychainToken(t *testing.T) {
	stubDeps(t, "", func() (*auth.TokenData, error) { return &auth.TokenData{}, nil }, nil, &fakeLister{}, testRegistryRefs)
	var stderr bytes.Buffer
	if got := resolveToken(&stderr); got != "" {
		t.Fatalf("resolveToken() = %q, want empty", got)
	}
}

func TestCrossPlatformCoverageRunWritesSnapshotWithHonestCoverage(t *testing.T) {
	servers := []syncdata.ServerInfo{
		{ID: "doc", Endpoint: "https://doc.example"},
		{ID: "sheet", Endpoint: "https://sheet.example"},
		{ID: "blank", Endpoint: "   "},
	}
	lister := &fakeLister{
		results: map[string]transport.ToolsListResult{
			"https://doc.example": {Tools: []transport.ToolDescriptor{
				{Name: "copy_document", Title: "复制文档", Description: "copy", InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"doc_id": map[string]any{"type": "string", "description": "文档 ID", "default": "d", "enum": []any{"a", "b", 3}},
						"bogus":  "not-a-map",
					},
					"required": []any{"doc_id", 42},
				}},
				{Name: "   "},
				{Name: "not_in_registry"},
			}},
		},
		errs: map[string]error{"https://sheet.example": errors.New("boom")},
	}
	stubDeps(t, "env-token", nil, servers, lister, testRegistryRefs)

	dir := t.TempDir()
	output := filepath.Join(dir, "snapshot.json")
	prev := `{"tools":{"doc.get_document":{"interface_ref":{"product_id":"doc-helper","rpc_name":"fetch_document"}}}}`
	if err := os.WriteFile(output, []byte(prev), 0o600); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	if code := run([]string{"--output", output}, &stderr); code != 0 {
		t.Fatalf("run() = %d, stderr=%s", code, stderr.String())
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Version  int            `json:"version"`
		Coverage map[string]any `json:"coverage"`
		Tools    map[string]map[string]any
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != 1 {
		t.Fatalf("version = %d", snapshot.Version)
	}
	if got := snapshot.Coverage["snapshot_services"].(float64); got != 2 {
		t.Fatalf("snapshot_services = %v, want 2 (3 servers - 1 failed; blank endpoint not counted as failed)", got)
	}
	if got := snapshot.Coverage["missing_services"].([]any); len(got) != 1 || got[0] != "sheet" {
		t.Fatalf("missing_services = %v, want [sheet]", got)
	}
	live := snapshot.Tools["doc.copy_document"]
	if live == nil || live["title"] != "复制文档" {
		t.Fatalf("doc.copy_document = %#v, want live metadata", live)
	}
	params := live["parameters"].(map[string]any)
	docID := params["doc_id"].(map[string]any)
	if docID["type"] != "string" || docID["required"] != true || docID["default"] != "d" {
		t.Fatalf("doc_id = %#v", docID)
	}
	if enum := docID["enum"].([]any); len(enum) != 2 {
		t.Fatalf("enum = %v, want the 2 string members only", enum)
	}
	if _, ok := params["bogus"]; ok {
		t.Fatal("non-map property should be skipped")
	}
	prevRef := snapshot.Tools["doc.get_document"]["interface_ref"].(map[string]any)
	if prevRef["product_id"] != "doc-helper" {
		t.Fatalf("previous reviewed ref lost: %#v", prevRef)
	}
	if _, ok := snapshot.Tools["not_in_registry"]; ok {
		t.Fatal("tools outside the registry must be dropped")
	}
	if !strings.Contains(stderr.String(), "services unreachable: sheet") {
		t.Fatalf("stderr = %q, want unreachable log", stderr.String())
	}
}

func TestRunIgnoresCorruptPreviousSnapshot(t *testing.T) {
	stubDeps(t, "env-token", nil, nil, &fakeLister{}, testRegistryRefs)
	output := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(output, []byte("{corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if code := run([]string{"--output", output}, &stderr); code != 0 {
		t.Fatalf("run() = %d, stderr=%s", code, stderr.String())
	}
}

func TestCrossPlatformCoverageRunRegistryLoadFailureStillWritesStublessSnapshot(t *testing.T) {
	stubDeps(t, "env-token", nil, nil, &fakeLister{}, func() (map[string]map[string]string, error) { return nil, errors.New("no identity") })
	output := filepath.Join(t.TempDir(), "snapshot.json")
	var stderr bytes.Buffer
	if code := run([]string{"--output", output}, &stderr); code != 0 {
		t.Fatalf("run() = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "cannot collect command identity") {
		t.Fatalf("stderr = %q, want identity collection warning", stderr.String())
	}
}

func TestRunWriteFailure(t *testing.T) {
	stubDeps(t, "env-token", nil, nil, &fakeLister{}, testRegistryRefs)
	var stderr bytes.Buffer
	badPath := filepath.Join(t.TempDir(), "missing-dir", "snapshot.json")
	if code := run([]string{"--output", badPath}, &stderr); code != 1 {
		t.Fatalf("run() = %d, want 1 on write failure", code)
	}
}

func TestWriteMetadataMarshalFailure(t *testing.T) {
	err := writeMetadata(filepath.Join(t.TempDir(), "out.json"), map[string]any{"bad": math.NaN()})
	if err == nil || !strings.Contains(err.Error(), "marshal failed") {
		t.Fatalf("err = %v, want marshal failure", err)
	}
}

func TestMainDelegatesToRun(t *testing.T) {
	stubDeps(t, "env-token", nil, nil, &fakeLister{}, testRegistryRefs)
	origExit, origArgs := osExit, os.Args
	t.Cleanup(func() { osExit, os.Args = origExit, origArgs })
	exitCode := -1
	osExit = func(code int) { exitCode = code }
	os.Args = []string{"fetch_mcp_metadata", "--output", filepath.Join(t.TempDir(), "snapshot.json")}
	main()
	if exitCode != 0 {
		t.Fatalf("main() exited with %d, want 0", exitCode)
	}
}

func TestExtractParamsNilAndNonObjectSchemas(t *testing.T) {
	if got := extractParams(nil); got != nil {
		t.Fatalf("extractParams(nil) = %v, want nil", got)
	}
	if got := extractParams(map[string]any{"type": "object"}); got != nil {
		t.Fatalf("extractParams(no properties) = %v, want nil", got)
	}
}

func TestCrossPlatformCoverageNewToolListerBuildsAuthedClient(t *testing.T) {
	if lister := newToolLister("tok"); lister == nil {
		t.Fatal("newToolLister returned nil")
	}
}

func TestRunRecordsSourceRevision(t *testing.T) {
	stubDeps(t, "env-token", nil, nil, &fakeLister{}, testRegistryRefs)
	dir := t.TempDir()
	head := filepath.Join(dir, "HEAD")
	if err := os.WriteFile(head, []byte("ref: refs/heads/feature\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	origHead := gitHeadPath
	t.Cleanup(func() { gitHeadPath = origHead })
	gitHeadPath = head

	output := filepath.Join(dir, "snapshot.json")
	var stderr bytes.Buffer
	if code := run([]string{"--output", output}, &stderr); code != 0 {
		t.Fatalf("run() = %d, stderr=%s", code, stderr.String())
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		SourceRevision string `json:"source_revision"`
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.SourceRevision != "ref: refs/heads/feature" {
		t.Fatalf("source_revision = %q", snapshot.SourceRevision)
	}
}
