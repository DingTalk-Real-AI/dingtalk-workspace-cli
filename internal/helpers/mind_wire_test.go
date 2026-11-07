// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageMindProductionWireEmptyPatchAndRecovery(t *testing.T) {
	response := `{"success":true,"requestId":"req","nodeId":"file","pageId":"page","topicId":"topic","status":"APPLIED","applied":true,"readVersion":"0","committedVersion":"1","text":"","index":0,"childCount":0,"unsupportedFeatures":[],"nodeIdMap":{},"logId":"trace"}`
	caller := &mindTestCaller{response: response}
	if err := mindExecute(t, caller, "node", "update", "--node-id", "file", "--page-id", "page", "--topic-id", "topic", "--request-id", "req", "--text", "", "--yes"); err != nil {
		t.Fatal(err)
	}
	if patch, ok := caller.args["patch"].(string); !ok || patch != `{"text":""}` {
		t.Fatalf("wire patch: %#v", caller.args)
	}
	receipt, err := decodeMindReply(response)
	if err != nil {
		t.Fatal(err)
	}
	args := mindArgs(t, `{"nodeId":"file","pageId":"page","topicId":"topic","requestId":"req","patch":{"text":""}}`)
	if mindWriteResult("update_mind_node", args, receipt).Outcome() != output.OutcomeSuccess {
		t.Fatal("empty confirmed text rejected")
	}
	if receipt["traceId"] != "trace" {
		t.Fatal("production logId lost")
	}
	if !strings.Contains(mindResumeCommand("update_mind_node", args), "--text ''") {
		t.Fatal("empty text lost from recovery")
	}
}

func TestCrossPlatformCoverageMindProductionQueryNativeArraysPreservePageAndZero(t *testing.T) {
	raw := `{"success":true,"nodeId":"file","readVersion":"0","schemaVersion":"1.0","catalogVersion":"mind-v1","hasMore":false,"truncated":false,"pages":[{"id":"p","title":"P","rootId":"r"}],"nodes":[{"pageId":"p","id":"r","type":"topic","text":"","index":0,"childCount":0,"unsupportedFeatures":[]}]}`
	result, err := decodeMindReply(raw)
	if err != nil {
		t.Fatal(err)
	}
	node := result["items"].([]any)[0].(map[string]any)["nodes"].([]any)[0].(map[string]any)
	if node["text"] != "" || node["index"] != json.Number("0") || len(node["unsupportedFeatures"].([]any)) != 0 {
		t.Fatalf("node: %#v", node)
	}
	if mindQueryResult("list_mind_nodes", map[string]any{"nodeId": "file"}, result).Outcome() != output.OutcomeSuccess {
		t.Fatal("flat query rejected")
	}
}

func TestCrossPlatformCoverageMindProductionWireRejectsMalformedAndConflictingCollections(t *testing.T) {
	for _, raw := range []string{
		`{"pages":[]}`,
		`{"pages":[],"nodes":[{"pageId":"missing"}]}`,
		`{"pages":{},"nodes":[]}`,
		`{"pages":"[]","nodes":[]}`,
		`{"pages":[{"id":"p"},{"id":"p"}],"nodes":[]}`,
		`{"pages":[{"id":"p","nodes":[]}],"nodes":[]}`,
		`{"pages":[],"nodes":[],"items":[]}`,
		`{"nodeIdMap":null}`,
		`{"nodeIdMap":[]}`,
		`{"nodeIdMap":"{}"}`,
		`{"nodeIdMap":{"x":1}}`,
		`{"text":null}`,
		`{"text":"","node":{}}`,
		`{"text":"","unsupportedFeatures":{}}`,
		`{"text":"","unsupportedFeatures":[1]}`,
	} {
		if _, err := decodeMindReply(raw); err == nil {
			t.Fatalf("accepted invalid flat projection: %s", raw)
		}
	}
}

func TestCrossPlatformCoverageMindCurrentFailureDiagnosticsThroughSharedMCPParser(t *testing.T) {
	for _, code := range []string{"mind.permission.denied", "mind.document.notReady", "mind.version.conflict", "mind.request.inProgress", "mind.protected", "mind.feature.disabled"} {
		for _, wrapped := range []bool{false, true} {
			t.Run(code+fmt.Sprint(wrapped), func(t *testing.T) {
				diagnostic := map[string]any{"errorStage": "processContent", "errorCategory": "DOWNSTREAM", "logId": "trace-current", "requestId": "client-idempotency-key", "errorOperationId": "operation"}
				var response map[string]any
				if wrapped {
					response = map[string]any{"success": false, "errorCode": code, "errorMsg": "拒绝操作", "requestId": "client-idempotency-key", "result": diagnostic}
				} else {
					response = diagnostic
					response["success"] = false
					response["errorCode"] = code
					response["errorMessage"] = "拒绝操作"
					response["pages"] = nil
					response["nodes"] = nil
				}
				raw, err := json.Marshal(response)
				if err != nil {
					t.Fatal(err)
				}
				caller := &mindTestCaller{response: string(raw)}
				installMindCaller(t, caller)
				result, err := callMindResult(&cobra.Command{}, "update_mind_node", map[string]any{})
				if err != nil {
					t.Fatal(err)
				}
				env, err := output.EnvelopeFromResult(result)
				if err != nil {
					t.Fatal(err)
				}
				if result.Outcome() != output.OutcomeFailure || env.Error == nil || env.Error.UpstreamCode != code || env.Error.TraceID != "trace-current" || env.Error.RequestID != "client-idempotency-key" || env.Error.Stage != "processContent" || !strings.Contains(env.Error.Message, "拒绝操作") {
					t.Fatalf("lost diagnostics: %#v", env.Error)
				}
				receipt := env.Error.Details["receipt"].(map[string]any)
				if receipt["errorDetail"].(map[string]any)["category"] != "DOWNSTREAM" || caller.calls != 1 {
					t.Fatalf("category or call count lost: %#v", receipt)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageMindCurrentCreateKeepsMessageAndJSONStringForTenTopics(t *testing.T) {
	nodes := []map[string]any{{"id": "r", "type": "topic", "text": "根主题"}}
	mapping := map[string]any{"r": "root"}
	for i := 1; i < 10; i++ {
		id := fmt.Sprintf("n%d", i)
		nodes = append(nodes, map[string]any{"id": id, "type": "topic", "text": "", "parentId": "r"})
		mapping[id] = "real-" + id
	}
	source, _ := json.Marshal(map[string]any{"schemaVersion": "1.0", "catalogVersion": "mind-v1", "pages": []any{map[string]any{"id": "p", "title": "计划", "nodes": nodes}}})
	response, _ := json.Marshal(map[string]any{"success": true, "requestId": "req", "nodeId": "file", "status": "READY", "requestedContentApplied": true, "revision": "0", "pageIdMap": map[string]any{"p": "page"}, "nodeIdMap": mapping, "message": "后续读取仍校验权限"})
	caller := &mindTestCaller{response: string(response)}
	buf := installMindCaller(t, caller)
	cmd := newMindCommand()
	cmd.PersistentFlags().Bool("yes", true, "confirm")
	cmd.SetArgs([]string{"create", "--name", " 计划 ", "--source", string(source), "--request-id", "req"})
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if err := corecmd.ExecuteForTest(cmd); err != nil {
		t.Fatal(err)
	}
	if caller.args["source"] != string(source) || caller.args["name"] != " 计划 " || caller.calls != 1 {
		t.Fatalf("arguments changed: %#v", caller.args)
	}
	receipt, err := decodeMindReply(string(response))
	if err != nil {
		t.Fatal(err)
	}
	result := mindWriteResult("create_mind", map[string]any{"source": string(source), "requestId": "req"}, receipt)
	env, err := output.EnvelopeFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	data := env.Data.(map[string]any)
	if result.Outcome() != output.OutcomeSuccess || data["message"] != "后续读取仍校验权限" {
		t.Fatalf("lost creation message: %#v", env)
	}
}

func TestCrossPlatformCoverageMindCurrentCursorValidationBeforeRemoteCall(t *testing.T) {
	for _, token := range []string{"", "mindc1_" + strings.Repeat("a", 43), "opaque", " "} {
		caller := &mindTestCaller{response: `{}`}
		args := []string{"node", "list", "--node-id", "file", "--mode", "SUBTREE", "--page-id", "page", "--topic-id", "root", "--next-token", token}
		err := mindExecute(t, caller, args...)
		valid := token == "" || strings.HasPrefix(token, "mindc1_")
		if valid && (err != nil || caller.calls != 1) {
			t.Fatalf("valid token blocked: %q %v", token, err)
		}
		if !valid && (err == nil || caller.calls != 0) {
			t.Fatalf("invalid token sent: %q", token)
		}
	}
}

func TestCrossPlatformCoverageMindNameLimitRejectsBeforeRPC(t *testing.T) {
	for _, name := range []string{strings.Repeat("中", 129), strings.Repeat("A", 129)} {
		caller := &mindTestCaller{}
		err := mindExecute(t, caller, "create", "--name", name, "--source", mindTestSource, "--request-id", "req", "--yes")
		if err == nil || apperrors.ExitCode(err) != apperrors.ExitCodeValidation || caller.calls != 0 {
			t.Fatalf("invalid name reached server: err=%v calls=%d", err, caller.calls)
		}
	}
}

func TestCrossPlatformCoverageMindPendingRecoveryPreservesMultilineAndProfile(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	response := `{"requestId":"req","nodeId":"file","status":"INITIALIZING","requestedContentApplied":false}`
	caller := &mindTestCaller{response: response}
	installMindCaller(t, caller)
	cmd := &cobra.Command{}
	cmd.Flags().String("profile", "corp:user", "profile")
	source := "\n" + strings.ReplaceAll(mindTestSource, ",", ",\n") + "\n"
	args := map[string]any{"requestId": "req", "name": "计划", "source": source}
	result, err := callMindResult(cmd, "create_mind", args)
	if err != nil {
		t.Fatal(err)
	}
	env, err := output.EnvelopeFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if env.Meta == nil || env.Meta.Operation == nil {
		t.Fatal("pending operation missing")
	}
	command := env.Meta.Operation.NextCommand
	if !strings.Contains(command, "--profile corp:user") || strings.Contains(command, "--yes") {
		t.Fatalf("identity or confirmation changed: %s", command)
	}

	// A harmless shell function replaces dws; no CLI or remote calls execute.
	text := "line 1\nO'Brien $HOME `whoami` \\ \r\n"
	recovery := mindResumeCommand("update_mind_node", map[string]any{"patch": map[string]any{"text": text}})
	if strings.ContainsAny(recovery, "\n\r") {
		t.Fatal("multiline recovery command")
	}
	for _, shell := range []string{"bash", "zsh"} {
		path, err := exec.LookPath(shell)
		if err != nil {
			t.Logf("%s unavailable; retaining structural checks", shell)
			continue
		}
		for _, tc := range []struct{ command, flag, want string }{{command, "--source", source}, {recovery, "--text", text}} {
			script := `dws() { printf '%s\0' "$@"; }; ` + tc.command
			out, err := exec.Command(path, "-c", script).Output()
			if err != nil {
				t.Fatal(err)
			}
			argv := strings.Split(string(out), "\x00")
			found := false
			for i := 0; i+1 < len(argv); i++ {
				if argv[i] == tc.flag {
					found = true
					if argv[i+1] != tc.want {
						t.Fatalf("%s %s changed: %q", shell, tc.flag, argv[i+1])
					}
				}
			}
			if !found {
				t.Fatalf("%s missing", tc.flag)
			}
		}
	}

}

func TestCrossPlatformCoverageMindPendingRecoveryPinsCurrentProfileMetadata(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", dir)
	raw := `{"version":2,"currentProfile":"corp:user","profiles":[{"corpId":"corp","userId":"user","name":"User"}]}`
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	caller := &mindTestCaller{response: `{"requestId":"req","nodeId":"file","status":"INITIALIZING","requestedContentApplied":false}`}
	installMindCaller(t, caller)
	result, err := callMindResult(&cobra.Command{}, "create_mind", map[string]any{"requestId": "req", "name": "x", "source": mindTestSource})
	if err != nil {
		t.Fatal(err)
	}
	env, err := output.EnvelopeFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(env.Meta.Operation.NextCommand, "--profile corp:user") {
		t.Fatalf("current identity lost: %s", env.Meta.Operation.NextCommand)
	}
}
