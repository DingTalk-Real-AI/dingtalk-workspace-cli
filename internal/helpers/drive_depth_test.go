package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func driveDepthTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().String("cursor", "", "")
	cmd.Flags().String("next-token", "", "")
	cmd.Flags().Int("limit", 0, "")
	cmd.Flags().Int("max", 0, "")
	return cmd
}

func TestCrossPlatformCoverageDriveDepthCancelledError(t *testing.T) {
	err := &driveDepthCancelledError{}
	if err.Error() != driveDepthCancelledMsg || err.RawStderr() != driveDepthCancelledMsg {
		t.Fatalf("messages = %q/%q", err.Error(), err.RawStderr())
	}
	if err.ExitCode() != 130 {
		t.Fatalf("ExitCode = %d, want 130", err.ExitCode())
	}
}

func TestCrossPlatformCoverageValidateDriveListDepth(t *testing.T) {
	newCmd := func() *cobra.Command { return driveDepthTestCmd() }

	if err := validateDriveListDepth(newCmd(), 0); err == nil {
		t.Fatal("depth 0 returned nil")
	}
	if err := validateDriveListDepth(newCmd(), driveDepthMax+1); err == nil {
		t.Fatal("depth above max returned nil")
	}

	shallow := newCmd()
	_ = shallow.Flags().Set("cursor", "c1")
	_ = shallow.Flags().Set("limit", "5")
	if err := validateDriveListDepth(shallow, 1); err != nil {
		t.Fatalf("depth 1 with cursor/limit returned %v", err)
	}

	withCursor := newCmd()
	_ = withCursor.Flags().Set("cursor", "c1")
	if err := validateDriveListDepth(withCursor, 2); err == nil {
		t.Fatal("depth 2 with cursor returned nil")
	}

	withNextToken := newCmd()
	_ = withNextToken.Flags().Set("next-token", "t1")
	if err := validateDriveListDepth(withNextToken, 2); err == nil {
		t.Fatal("depth 2 with next-token returned nil")
	}

	withLimit := newCmd()
	_ = withLimit.Flags().Set("limit", "5")
	if err := validateDriveListDepth(withLimit, 2); err == nil {
		t.Fatal("depth 2 with limit returned nil")
	}

	withMax := newCmd()
	_ = withMax.Flags().Set("max", "5")
	if err := validateDriveListDepth(withMax, 2); err == nil {
		t.Fatal("depth 2 with max returned nil")
	}

	if err := validateDriveListDepth(newCmd(), 2); err != nil {
		t.Fatalf("depth 2 clean returned %v", err)
	}
	if err := validateDriveListDepth(newCmd(), driveDepthMax); err != nil {
		t.Fatalf("depth max clean returned %v", err)
	}
}

func TestCrossPlatformCoverageDriveDepthRoutesBuildArgs(t *testing.T) {
	pan := newDrivePanDepthRoute()
	args := pan.buildArgs(map[string]any{"spaceId": "s1"}, "folder1", "tok")
	if args["maxResults"] != float64(driveDepthPageSize) || args["spaceId"] != "s1" ||
		args["parentId"] != "folder1" || args["nextToken"] != "tok" {
		t.Fatalf("pan args = %#v", args)
	}
	empty := pan.buildArgs(nil, "", "")
	if _, ok := empty["parentId"]; ok {
		t.Fatalf("pan empty folder args = %#v", empty)
	}
	if _, ok := empty["nextToken"]; ok {
		t.Fatalf("pan empty token args = %#v", empty)
	}
	if id := pan.itemID(map[string]any{"fileId": "f1"}); id != "f1" {
		t.Fatalf("pan itemID = %q", id)
	}
	if id := pan.itemID(map[string]any{}); id != "" {
		t.Fatalf("pan missing itemID = %q", id)
	}

	doc := newDocDepthRoute()
	dargs := doc.buildArgs(map[string]any{"workspaceId": "w1"}, "node1", "pt")
	if dargs["pageSize"] != float64(docDepthPageSize) || dargs["workspaceId"] != "w1" ||
		dargs["folderId"] != "node1" || dargs["pageToken"] != "pt" {
		t.Fatalf("doc args = %#v", dargs)
	}
	dempty := doc.buildArgs(nil, "", "")
	if _, ok := dempty["folderId"]; ok {
		t.Fatalf("doc empty folder args = %#v", dempty)
	}
	if id := doc.itemID(map[string]any{"nodeId": "n1"}); id != "n1" {
		t.Fatalf("doc itemID = %q", id)
	}
	if id := doc.itemID(map[string]any{}); id != "" {
		t.Fatalf("doc missing itemID = %q", id)
	}
}

func TestCrossPlatformCoverageParseDriveDepthPage(t *testing.T) {
	if items, next, hasMore := parseDriveDepthPage(`{invalid`); items != nil || next != "" || hasMore {
		t.Fatalf("invalid JSON = %#v %q %v", items, next, hasMore)
	}
	if items, _, _ := parseDriveDepthPage(`{"foo":1}`); items != nil {
		t.Fatalf("missing items key = %#v", items)
	}
	items, next, hasMore := parseDriveDepthPage(`{"items":[{"name":"a"},42],"nextToken":"t","hasMore":true}`)
	if len(items) != 1 || items[0]["name"] != "a" || next != "t" || !hasMore {
		t.Fatalf("top-level items = %#v %q %v", items, next, hasMore)
	}
	items, next, _ = parseDriveDepthPage(`{"result":{"dentryList":[{"name":"b"}],"nextToken":"t2"}}`)
	if len(items) != 1 || items[0]["name"] != "b" || next != "t2" {
		t.Fatalf("result dentryList = %#v %q", items, next)
	}
	if items, _, _ := parseDriveDepthPage(`{"result":{"other":1}}`); items != nil {
		t.Fatalf("result without items = %#v", items)
	}
}

func TestCrossPlatformCoverageDriveDepthListItemsKey(t *testing.T) {
	if k := driveDepthListItemsKey(map[string]any{"items": []any{}}); k != "items" {
		t.Fatalf("items key = %q", k)
	}
	if k := driveDepthListItemsKey(map[string]any{"dentryList": []any{1}}); k != "dentryList" {
		t.Fatalf("dentryList key = %q", k)
	}
	if k := driveDepthListItemsKey(map[string]any{"items": nil}); k != "" {
		t.Fatalf("nil items key = %q", k)
	}
	if k := driveDepthListItemsKey(map[string]any{}); k != "" {
		t.Fatalf("missing key = %q", k)
	}
}

func TestCrossPlatformCoverageParseDocDepthPage(t *testing.T) {
	if items, next, hasMore := parseDocDepthPage(`{bad`); items != nil || next != "" || hasMore {
		t.Fatalf("invalid JSON = %#v %q %v", items, next, hasMore)
	}
	items, next, hasMore := parseDocDepthPage(`{"nodes":[{"name":"a"},"x"],"nextPageToken":"nt","hasMore":true}`)
	if len(items) != 1 || next != "nt" || !hasMore {
		t.Fatalf("nodes = %#v %q %v", items, next, hasMore)
	}
}

func TestCrossPlatformCoverageMatchDriveNamePattern(t *testing.T) {
	if !matchDriveNamePattern("anything", "") {
		t.Fatal("empty pattern should match")
	}
	if !matchDriveNamePattern("report.xlsx", "*.xlsx") || matchDriveNamePattern("report.csv", "*.xlsx") {
		t.Fatal("glob match failed")
	}
	if !matchDriveNamePattern("weekly-report.xlsx", "report") || matchDriveNamePattern("a.txt", "report") {
		t.Fatal("substring wrap failed")
	}
	if !matchDriveNamePattern("a[b", "[") {
		t.Fatal("invalid pattern should fall back to substring")
	}
}

func TestCrossPlatformCoverageDriveDepthItemClassifiers(t *testing.T) {
	if !isDriveDepthFolder(map[string]any{"type": "FOLDER"}) {
		t.Fatal("type FOLDER not detected")
	}
	if !isDriveDepthFolder(map[string]any{"dentryType": "folder"}) {
		t.Fatal("dentryType folder not detected")
	}
	if isDriveDepthFolder(map[string]any{"type": "FILE"}) || isDriveDepthFolder(map[string]any{}) {
		t.Fatal("non-folder detected as folder")
	}
	if !isDocDepthFolder(map[string]any{"nodeType": "Folder"}) {
		t.Fatal("doc folder not detected")
	}
	if isDocDepthFolder(map[string]any{"nodeType": "doc"}) {
		t.Fatal("doc detected as folder")
	}
	if id := driveDepthItemID(map[string]any{"fileId": "f"}); id != "f" {
		t.Fatalf("fileId = %q", id)
	}
	if id := driveDepthItemID(map[string]any{"nodeId": "n"}); id != "n" {
		t.Fatalf("nodeId = %q", id)
	}
	if id := driveDepthItemID(map[string]any{"fileId": ""}); id != "" {
		t.Fatalf("empty = %q", id)
	}
}

func TestCrossPlatformCoverageDriveDepthUnrecoverable(t *testing.T) {
	for _, code := range []string{CodeAuthTokenExpired, CodeAuthNotConfigured, CodeNetworkTimeout, CodeNetworkUnreachable} {
		if !driveDepthUnrecoverable(&CLIError{Code: code}) {
			t.Fatalf("%s not unrecoverable", code)
		}
	}
	if driveDepthUnrecoverable(&CLIError{Code: CodeMCPToolError}) {
		t.Fatal("tool error marked unrecoverable")
	}
	if driveDepthUnrecoverable(errors.New("plain")) {
		t.Fatal("plain error marked unrecoverable")
	}
}

func TestCrossPlatformCoverageDriveDepthErrorCode(t *testing.T) {
	if c := driveDepthErrorCode(errors.New("plain")); c != "" {
		t.Fatalf("plain = %q", c)
	}
	if c := driveDepthErrorCode(&CLIError{Message: "not json"}); c != "" {
		t.Fatalf("non-JSON = %q", c)
	}
	for _, tc := range []struct{ body, want string }{
		{`{"errorCode":"a.b"}`, "a.b"},
		{`{"error_code":"c.d"}`, "c.d"},
		{`{"code":"e.f"}`, "e.f"},
		{`{"errorCode":""}`, ""},
	} {
		if c := driveDepthErrorCode(&CLIError{Message: tc.body}); c != tc.want {
			t.Fatalf("%s = %q, want %q", tc.body, c, tc.want)
		}
	}
}

func TestCrossPlatformCoverageClassifyDriveDepthReason(t *testing.T) {
	if r := classifyDriveDepthReason(driveDepthRateLimitedCode); r != "rate_limited" {
		t.Fatalf("rate limit = %q", r)
	}
	if r := classifyDriveDepthReason("forbidden.noPermission"); r != "permission_denied" {
		t.Fatalf("forbidden = %q", r)
	}
	if r := classifyDriveDepthReason("other.x"); r != "api_error" {
		t.Fatalf("other = %q", r)
	}
}

func TestCrossPlatformCoverageDriveDepthErrorMessage(t *testing.T) {
	if m := driveDepthErrorMessage(&CLIError{Message: `{"errorMsg":"boom"}`}); m != "boom" {
		t.Fatalf("errorMsg = %q", m)
	}
	if m := driveDepthErrorMessage(&CLIError{Message: `{"message":"m2"}`}); m != "m2" {
		t.Fatalf("message = %q", m)
	}
	if m := driveDepthErrorMessage(&CLIError{Message: "raw text"}); m != "raw text" {
		t.Fatalf("raw = %q", m)
	}
	if m := driveDepthErrorMessage(errors.New("plain boom")); m != "plain boom" {
		t.Fatalf("plain = %q", m)
	}
}

func TestCrossPlatformCoverageNewDriveDepthError(t *testing.T) {
	folder := driveDepthFolder{id: "f1", name: "folder one", depth: 2}
	derr := newDriveDepthError(folder, &CLIError{Code: CodeMCPToolError, Message: `{"errorCode":"forbidden.x","errorMsg":"denied"}`})
	if derr.Depth != 2 || derr.FolderID != "f1" || derr.FolderName != "folder one" ||
		derr.Reason != "permission_denied" || derr.Message != "denied" {
		t.Fatalf("driveDepthError = %#v", derr)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func installDepthCaller(t *testing.T, caller *scriptedToolCaller) *bytes.Buffer {
	t.Helper()
	installScriptedCaller(t, caller)
	buf := &bytes.Buffer{}
	deps.Out.w = buf
	return buf
}

func decodeDepthResult(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, buf.String())
	}
	return out
}

func TestCrossPlatformCoverageEmitDriveDepthResult(t *testing.T) {
	out := installDepthCaller(t, &scriptedToolCaller{})
	items := []map[string]any{
		{"fileName": "b-file.xlsx", "rel_path": "b", "depth": 1, "fileId": "f2"},
		{"name": "a-file.xlsx", "rel_path": "a", "depth": 2, "fileId": "f1"},
		{"name": "skip-me.csv", "rel_path": "c", "depth": 1, "fileId": "f3"},
	}
	if err := emitDriveDepthResult(items, nil, false, "*.xlsx", 0, 0); err != nil {
		t.Fatal(err)
	}
	result := decodeDepthResult(t, out)
	got := result["items"].([]any)
	if len(got) != 2 {
		t.Fatalf("filtered items = %#v", got)
	}
	if got[0].(map[string]any)["rel_path"] != "a" || got[1].(map[string]any)["rel_path"] != "b" {
		t.Fatalf("sort order = %#v", got)
	}
	if result["maxDepth"] != float64(2) {
		t.Fatalf("maxDepth = %#v", result["maxDepth"])
	}
	if result["truncated"] != false {
		t.Fatalf("truncated = %#v", result["truncated"])
	}

	out.Reset()
	if err := emitDriveDepthResult(nil, nil, false, "", 0, 0); err != nil {
		t.Fatal(err)
	}
	result = decodeDepthResult(t, out)
	if items, ok := result["items"].([]any); !ok || len(items) != 0 {
		t.Fatalf("nil items = %#v", result["items"])
	}

	samePath := []map[string]any{
		{"name": "dup", "rel_path": "p/dup", "fileId": "z9"},
		{"name": "dup", "rel_path": "p/dup", "fileId": "a1"},
	}
	out.Reset()
	if err := emitDriveDepthResult(samePath, nil, false, "", 0, 0); err != nil {
		t.Fatal(err)
	}
	got = decodeDepthResult(t, out)["items"].([]any)
	if got[0].(map[string]any)["fileId"] != "a1" {
		t.Fatalf("tie-break order = %#v", got)
	}

	deps.Out.w = failingWriter{}
	if err := emitDriveDepthResult(nil, nil, false, "", 0, 0); err == nil {
		t.Fatal("failing writer returned nil")
	}
}

func TestCrossPlatformCoveragePrintDriveDepthDryRun(t *testing.T) {
	jsonOut := installDepthCaller(t, &scriptedToolCaller{format: "json", dry: true})
	if err := printDriveDepthDryRun(newDrivePanDepthRoute(), map[string]any{"spaceId": "s1"}, 3); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(jsonOut.Bytes(), &payload); err != nil {
		t.Fatalf("json dry-run output: %v", err)
	}
	if payload["dry_run"] != true || payload["tool"] != "list_files" || payload["maxDepth"] != float64(3) {
		t.Fatalf("json dry-run = %#v", payload)
	}

	installDepthCaller(t, &scriptedToolCaller{format: "table", dry: true})
	if err := printDriveDepthDryRun(newDocDepthRoute(), map[string]any{"workspaceId": "w1"}, 2); err != nil {
		t.Fatal(err)
	}
}

func useDriveDepthArgs(t *testing.T) {
	t.Helper()
	old := os.Args
	os.Args = []string{"dws", "drive", "list", "--depth", "2"}
	t.Cleanup(func() { os.Args = old })
}

func runDepthBFS(t *testing.T, caller *scriptedToolCaller, route driveDepthRoute, root string, maxDepth int, pattern string) (map[string]any, error) {
	t.Helper()
	out := installDepthCaller(t, caller)
	cmd := &cobra.Command{Use: "list"}
	err := runDriveListDepth(cmd, route, map[string]any{}, root, maxDepth, pattern, true, 0)
	return decodeDepthResult(t, out), err
}

func TestCrossPlatformCoverageRunDriveListDepthDryRun(t *testing.T) {
	caller := &scriptedToolCaller{format: "json", dry: true}
	out := installDepthCaller(t, caller)
	cmd := &cobra.Command{Use: "list"}
	if err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{}, "", 2, "", true, 0); err != nil {
		t.Fatal(err)
	}
	if caller.calls != 0 {
		t.Fatalf("dry-run calls = %d", caller.calls)
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("dry-run output: %v", err)
	}
	if payload["dry_run"] != true {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageRunDriveListDepthPanBFS(t *testing.T) {
	useDriveDepthArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"fileId":"fA","name":"dirA","type":"FOLDER"},{"fileId":"fX","name":"x.txt","type":"FILE"}]}`},
		{text: `{"items":[{"fileId":"fY","name":"y.txt","type":"FILE"}]}`},
	}}
	out := installDepthCaller(t, caller)
	cmd := &cobra.Command{Use: "list"}
	if err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{}, "", 3, "", false, 0); err != nil {
		t.Fatal(err)
	}
	if caller.calls != 2 {
		t.Fatalf("calls = %d, want 2", caller.calls)
	}
	items := decodeDepthResult(t, out)["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("items = %#v", items)
	}
	var dirA, nested map[string]any
	for _, raw := range items {
		item := raw.(map[string]any)
		switch item["rel_path"] {
		case "dirA":
			dirA = item
		case "dirA/y.txt":
			nested = item
		}
	}
	if dirA == nil || nested == nil {
		t.Fatalf("rel_path tree = %#v", items)
	}
	if dirA["depth"] != float64(1) || dirA["parentId"] != "" {
		t.Fatalf("root item = %#v", dirA)
	}
	if nested["depth"] != float64(2) || nested["parentId"] != "fA" {
		t.Fatalf("nested item = %#v", nested)
	}
}

func TestCrossPlatformCoverageRunDriveListDepthMaxDepthSkipsEnqueue(t *testing.T) {
	useDriveDepthArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"fileId":"fA","name":"dirA","type":"FOLDER"}]}`},
	}}
	result, err := runDepthBFS(t, caller, newDrivePanDepthRoute(), "", 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if caller.calls != 1 {
		t.Fatalf("calls = %d, want 1", caller.calls)
	}
	if len(result["items"].([]any)) != 1 {
		t.Fatalf("items = %#v", result["items"])
	}
}

func TestCrossPlatformCoverageRunDriveListDepthDedup(t *testing.T) {
	useDriveDepthArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[
			{"fileId":"fA","name":"dirA","type":"FOLDER"},
			{"fileId":"fA","name":"dirA-copy","type":"FOLDER"},
			{"fileId":"","name":"no-id","type":"FOLDER"},
			{"fileId":"fB","name":"dirB","type":"FOLDER"}]}`},
		{text: `{"items":[{"fileId":"f1","name":"a.txt","type":"FILE"}]}`},
		{text: `{"items":[{"fileId":"f2","name":"b.txt","type":"FILE"}]}`},
	}}
	result, err := runDepthBFS(t, caller, newDrivePanDepthRoute(), "", 3, "")
	if err != nil {
		t.Fatal(err)
	}
	if caller.calls != 3 {
		t.Fatalf("calls = %d, want 3 (dedup + empty id skipped)", caller.calls)
	}
	if len(result["items"].([]any)) != 6 {
		t.Fatalf("items = %#v", result["items"])
	}
}

func TestCrossPlatformCoverageRunDriveListDepthDocRouteAnomaly(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"nodes":[{"nodeId":"nB","name":"folderB","nodeType":"folder"},{"nodeId":"nD","name":"doc1","nodeType":"doc"}],"hasMore":false,"nextPageToken":"ignored"}`},
		{text: `{"nodes":[{"nodeId":"nE","name":"inner","nodeType":"doc"}],"hasMore":true}`},
	}}
	result, err := runDepthBFS(t, caller, newDocDepthRoute(), "root1", 3, "")
	if err != nil {
		t.Fatal(err)
	}
	if caller.calls != 2 {
		t.Fatalf("calls = %d, want 2 (hasMore=false stops authoritative route)", caller.calls)
	}
	errs := result["errors"].([]any)
	if len(errs) != 1 {
		t.Fatalf("errors = %#v", result["errors"])
	}
	entry := errs[0].(map[string]any)
	if entry["folderId"] != "nB" || entry["reason"] != "api_error" {
		t.Fatalf("anomaly entry = %#v", entry)
	}
	if !strings.Contains(entry["message"].(string), "hasMore=true but nextToken is empty") {
		t.Fatalf("anomaly message = %#v", entry)
	}
	if len(result["items"].([]any)) != 3 {
		t.Fatalf("items = %#v", result["items"])
	}
}

func TestCrossPlatformCoverageRunDriveListDepthRateLimitRetry(t *testing.T) {
	useDriveDepthArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"fileId":"fA","name":"dirA","type":"FOLDER"}]}`},
		{text: `{"errorCode":"invalidRequest.rateLimited","errorMsg":"slow down"}`},
		{text: `{"items":[{"fileId":"f1","name":"a.txt","type":"FILE"}]}`},
	}}
	result, err := runDepthBFS(t, caller, newDrivePanDepthRoute(), "", 3, "")
	if err != nil {
		t.Fatal(err)
	}
	if caller.calls != 3 {
		t.Fatalf("calls = %d, want 3", caller.calls)
	}
	if errs := result["errors"].([]any); len(errs) != 0 {
		t.Fatalf("errors = %#v", errs)
	}
	if len(result["items"].([]any)) != 2 {
		t.Fatalf("items = %#v", result["items"])
	}
}

func TestCrossPlatformCoverageRunDriveListDepthRateLimitResumesFromFailedPage(t *testing.T) {
	useDriveDepthArgs(t)
	caller := &depthArgsRecordingCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"fileId":"f1","name":"page1.txt","type":"FILE"}],"nextToken":"p2"}`},
		{text: `{"errorCode":"invalidRequest.rateLimited","errorMsg":"slow down"}`},
		{text: `{"items":[{"fileId":"f2","name":"page2.txt","type":"FILE"}]}`},
	}}
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })
	InitDeps(caller)
	out := &bytes.Buffer{}
	deps.Out.w = out
	deps.Out.errW = io.Discard
	cmd := &cobra.Command{Use: "list"}
	if err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{}, "", 3, "", true, 0); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 3 {
		t.Fatalf("calls = %d, want 3", len(caller.calls))
	}
	if tok, _ := caller.calls[2]["nextToken"].(string); tok != "p2" {
		t.Fatalf("retry call args = %#v, want resume with nextToken=p2", caller.calls[2])
	}
	items := decodeDepthResult(t, out)["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items = %#v, want 2 without duplicates", items)
	}
}

func TestCrossPlatformCoverageRunDriveListDepthRateLimitExhausted(t *testing.T) {
	useDriveDepthArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"fileId":"fA","name":"dirA","type":"FOLDER"}]}`},
		{text: `{"errorCode":"invalidRequest.rateLimited","errorMsg":"slow down"}`},
	}}
	result, err := runDepthBFS(t, caller, newDrivePanDepthRoute(), "", 3, "")
	if err != nil {
		t.Fatal(err)
	}
	if caller.calls != 3 {
		t.Fatalf("calls = %d, want 3 (one retry)", caller.calls)
	}
	errs := result["errors"].([]any)
	if len(errs) != 1 {
		t.Fatalf("errors = %#v", errs)
	}
	entry := errs[0].(map[string]any)
	if entry["reason"] != "rate_limited" || entry["folderId"] != "fA" {
		t.Fatalf("rate limit entry = %#v", entry)
	}
}

func TestCrossPlatformCoverageRunDriveListDepthRootFailure(t *testing.T) {
	useDriveDepthArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"errorCode":"invalidRequest.rateLimited","errorMsg":"slow down"}`},
	}}
	installDepthCaller(t, caller)
	cmd := &cobra.Command{Use: "list"}
	err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{}, "", 3, "", true, 0)
	if err == nil {
		t.Fatal("root failure returned nil")
	}
	if caller.calls != 2 {
		t.Fatalf("calls = %d, want 2 (single retry then fail)", caller.calls)
	}
}

func TestCrossPlatformCoverageRunDriveListDepthUnrecoverable(t *testing.T) {
	useDriveDepthArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"fileId":"fA","name":"dirA","type":"FOLDER"},{"fileId":"fX","name":"x.txt","type":"FILE"}]}`},
		{text: `{"errorCode":"DWS_SERVICE_UNAUTHORIZED"}`},
	}}
	out := installDepthCaller(t, caller)
	cmd := &cobra.Command{Use: "list"}
	err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{}, "", 3, "", true, 0)
	if err == nil {
		t.Fatal("unrecoverable returned nil")
	}
	var cliErr *CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != CodeAuthTokenExpired {
		t.Fatalf("err = %T %v", err, err)
	}
	result := decodeDepthResult(t, out)
	if len(result["items"].([]any)) != 2 {
		t.Fatalf("partial items = %#v", result["items"])
	}
	if len(result["errors"].([]any)) != 1 {
		t.Fatalf("errors = %#v", result["errors"])
	}
}

func TestCrossPlatformCoverageRunDriveListDepthUnrecoverableEmitFailure(t *testing.T) {
	useDriveDepthArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"errorCode":"DWS_SERVICE_UNAUTHORIZED"}`},
	}}
	installDepthCaller(t, caller)
	deps.Out.w = failingWriter{}
	cmd := &cobra.Command{Use: "list"}
	err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{}, "", 3, "", true, 0)
	if err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("err = %v, want emit failure", err)
	}
}

func TestCrossPlatformCoverageRunDriveListDepthPaginationLoop(t *testing.T) {
	useDriveDepthArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[],"nextToken":"t","hasMore":true}`},
	}}
	installDepthCaller(t, caller)
	cmd := &cobra.Command{Use: "list"}
	err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{}, "", 3, "", true, 0)
	if err == nil || !strings.Contains(err.Error(), "cursor loop suspected") {
		t.Fatalf("err = %v, want pagination anomaly", err)
	}
	maxPages := driveDepthMaxItems/driveDepthPageSize + 1
	if caller.calls != maxPages {
		t.Fatalf("calls = %d, want %d", caller.calls, maxPages)
	}
}

func TestCrossPlatformCoverageRunDriveListDepthTruncation(t *testing.T) {
	useDriveDepthArgs(t)
	var sb strings.Builder
	sb.WriteString(`{"items":[`)
	for i := 0; i < driveDepthMaxItems; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"fileId":"f%d","name":"file-%d.txt","type":"FILE"}`, i, i)
	}
	sb.WriteString(`]}`)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: sb.String()}}}
	result, err := runDepthBFS(t, caller, newDrivePanDepthRoute(), "", 3, "")
	if err != nil {
		t.Fatal(err)
	}
	if result["truncated"] != true {
		t.Fatalf("truncated = %#v", result["truncated"])
	}
	if len(result["items"].([]any)) != driveDepthMaxItems {
		t.Fatalf("items len = %d", len(result["items"].([]any)))
	}
}

func TestCrossPlatformCoverageRunDriveListDepthCancelled(t *testing.T) {
	useDriveDepthArgs(t)
	caller := &scriptedToolCaller{}
	out := installDepthCaller(t, caller)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := &cobra.Command{Use: "list"}
	cmd.SetContext(ctx)
	err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{}, "", 3, "", true, 0)
	var cancelErr *driveDepthCancelledError
	if !errors.As(err, &cancelErr) {
		t.Fatalf("err = %T %v, want driveDepthCancelledError", err, err)
	}
	result := decodeDepthResult(t, out)
	if result["truncated"] != true {
		t.Fatalf("cancelled result = %#v", result)
	}
}

func TestCrossPlatformCoverageRunDriveListDepthCancelledEmitFailure(t *testing.T) {
	useDriveDepthArgs(t)
	caller := &scriptedToolCaller{}
	installDepthCaller(t, caller)
	deps.Out.w = failingWriter{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := &cobra.Command{Use: "list"}
	cmd.SetContext(ctx)
	err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{}, "", 3, "", true, 0)
	if err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("err = %v, want emit failure", err)
	}
}

func TestCrossPlatformCoverageRunDriveListDepthFinalEmitFailure(t *testing.T) {
	useDriveDepthArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"fileId":"fX","name":"x.txt","type":"FILE"}]}`},
	}}
	installDepthCaller(t, caller)
	deps.Out.w = failingWriter{}
	cmd := &cobra.Command{Use: "list"}
	err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{}, "", 3, "", true, 0)
	if err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("err = %v, want emit failure", err)
	}
}

func TestCrossPlatformCoverageRunDriveListDepthPatternFilter(t *testing.T) {
	useDriveDepthArgs(t)
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"fileId":"fA","name":"dirA","type":"FOLDER"},{"fileId":"fX","name":"x.txt","type":"FILE"}]}`},
		{text: `{"items":[{"fileId":"fY","name":"keep.xlsx","type":"FILE"}]}`},
	}}
	result, err := runDepthBFS(t, caller, newDrivePanDepthRoute(), "", 3, "*.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	items := result["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["name"] != "keep.xlsx" {
		t.Fatalf("filtered = %#v", items)
	}
}

type cancelOnCallCaller struct {
	scriptedToolCaller
	cancel context.CancelFunc
}

func (c *cancelOnCallCaller) CallTool(ctx context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	c.cancel()
	return c.scriptedToolCaller.CallTool(ctx, productID, toolName, args)
}

func TestCrossPlatformCoverageRunDriveListDepthCancelledInsidePagination(t *testing.T) {
	useDriveDepthArgs(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	route := newDrivePanDepthRoute()
	baseParse := route.parsePage
	route.parsePage = func(text string) ([]map[string]any, string, bool) {
		cancel()
		return baseParse(text)
	}
	caller := &scriptedToolCaller{steps: []scriptedToolStep{
		{text: `{"items":[{"fileId":"fX","name":"x.txt","type":"FILE"}],"nextToken":"more"}`},
	}}
	out := installDepthCaller(t, caller)
	cmd := &cobra.Command{Use: "list"}
	cmd.SetContext(ctx)
	err := runDriveListDepth(cmd, route, map[string]any{}, "", 3, "", true, 0)
	var cancelErr *driveDepthCancelledError
	if !errors.As(err, &cancelErr) {
		t.Fatalf("err = %T %v, want driveDepthCancelledError", err, err)
	}
	result := decodeDepthResult(t, out)
	if len(result["items"].([]any)) != 1 || result["truncated"] != true {
		t.Fatalf("cancelled partial = %#v", result)
	}
}

func TestCrossPlatformCoverageRunDriveListDepthCancelledAfterFetch(t *testing.T) {
	useDriveDepthArgs(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	caller := &cancelOnCallCaller{
		scriptedToolCaller: scriptedToolCaller{steps: []scriptedToolStep{
			{text: `{"items":[{"fileId":"fX","name":"x.txt","type":"FILE"}]}`},
		}},
		cancel: cancel,
	}
	previousDeps := deps
	t.Cleanup(func() { deps = previousDeps })
	InitDeps(caller)
	out := &bytes.Buffer{}
	deps.Out.w = out
	deps.Out.errW = io.Discard
	cmd := &cobra.Command{Use: "list"}
	cmd.SetContext(ctx)
	err := runDriveListDepth(cmd, newDrivePanDepthRoute(), map[string]any{}, "", 3, "", true, 0)
	var cancelErr *driveDepthCancelledError
	if !errors.As(err, &cancelErr) {
		t.Fatalf("err = %T %v, want driveDepthCancelledError", err, err)
	}
	result := decodeDepthResult(t, out)
	if result["truncated"] != true {
		t.Fatalf("cancelled result = %#v", result)
	}
}
