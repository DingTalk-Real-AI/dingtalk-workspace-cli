// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

// A local --dry-run must never shadow the global barrier and reach a write.
// Keep the assembled command, ToolCaller adapter, runner and transport real;
// replace only HTTP and local token resolution, so overwrite may still read.
func TestCrossPlatformCoverageDocUpdateDryRunNeverWrites(t *testing.T) {
	for _, tc := range []struct {
		name        string
		args        []string
		wantReads   int
		wantWrites  int
		wantPreview bool
		wantErr     bool
	}{
		{"append local flag", []string{"doc", "update", "--node", "node", "--mode", "append", "--content", "new", "--dry-run"}, 0, 0, true, false},
		{"append indexed local flag", []string{"doc", "update", "--node", "node", "--mode", "append", "--content", "new", "--index", "0", "--dry-run"}, 0, 0, true, false},
		{"append global flag", []string{"--dry-run", "doc", "update", "--node", "node", "--mode", "append", "--content", "new"}, 0, 0, true, false},
		{"append explicit false writes", []string{"doc", "update", "--node", "node", "--mode", "append", "--content", "new", "--dry-run=false"}, 0, 1, false, false},
		{"overwrite retains read preview", []string{"doc", "update", "--node", "node", "--mode", "overwrite", "--content", "new", "--dry-run"}, 1, 0, true, false},
		{"jsonml overwrite retains read preview", []string{"doc", "update", "--node", "node", "--mode", "overwrite", "--content-format", "jsonml", "--content", `["root",{},["p",{},"new"]]`, "--dry-run"}, 1, 0, true, false},
		{"jsonml append local preview", []string{"doc", "update", "--node", "node", "--mode", "append", "--content-format", "jsonml", "--content", `["root",{},["p",{},["span",{},"new"]]]`, "--dry-run"}, 0, 0, true, false},
		{"jsonml append global preview", []string{"--dry-run", "doc", "update", "--node", "node", "--mode", "append", "--content-format", "jsonml", "--content", `["root",{},["p",{},["span",{},"new"]]]`}, 0, 0, true, false},
		{"jsonml append writes", []string{"doc", "update", "--node", "node", "--mode", "append", "--content-format", "jsonml", "--content", `["root",{},["p",{},["span",{},"new"]]]`, "--revision", "0", "--dry-run=false"}, 0, 1, false, false},
		{"jsonml append indexed writes", []string{"doc", "update", "--node", "node", "--mode", "append", "--content-format", "jsonml", "--content", `["root",{},["p",{},["span",{},"new"]]]`, "--revision", "0", "--index", "0", "--dry-run=false"}, 0, 1, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())
			t.Setenv("DO_NOT_TRACK", "1")
			t.Setenv("DINGTALK_DOC_MCP_URL", "https://mcp-gw.dingtalk.com/server/doc-dry-run")
			testseam.Swap(t, &os.Args, append([]string{"dws"}, tc.args...))
			helpers.InitDepsForTest(t, helpers.GetCaller())
			previousProfile := authpkg.RuntimeProfile()
			authpkg.SetRuntimeProfile("")
			t.Cleanup(func() { authpkg.SetRuntimeProfile(previousProfile) })
			testseam.Swap(t, &pluginAuthRegistry, map[string]*PluginAuth{})
			reads, writes := 0, 0
			prefetchDone := make(chan struct{})
			testseam.Swap(t, &runnerGetCachedRuntimeToken, func(context.Context) (string, error) {
				defer close(prefetchDone)
				return "test-token", nil
			})
			// Cleanup is LIFO: join runtime calls before restoring the getter.
			// Pure local previews never start a prefetch and need no join.
			t.Cleanup(func() {
				if tc.wantReads+tc.wantWrites == 0 && reads+writes == 0 {
					return
				}
				select {
				case <-prefetchDone:
				case <-time.After(5 * time.Second):
					t.Error("token prefetch did not finish before restoring its stub")
				}
			})
			testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
				return AccessTokenSnapshot{AccessToken: "test-token"}, nil
			})
			testseam.Swap(t, &runnerPreflightDocDownload, func(*runtimeRunner, context.Context, *transport.Client, string, executor.Invocation) error {
				return nil
			})
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				var request struct {
					ID     any    `json:"id"`
					Method string `json:"method"`
					Params struct {
						Name      string         `json:"name"`
						Arguments map[string]any `json:"arguments"`
					} `json:"params"`
				}
				if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
					return nil, err
				}
				text := `{"nodeId":"node","success":true}`
				if request.Method != "tools/call" {
					t.Errorf("unexpected method %q", request.Method)
				}
				switch request.Params.Name {
				case "get_document_content":
					reads++
					text = `{"markdown":"old"}`
				case "update_document":
					writes++
					if strings.HasPrefix(tc.name, "jsonml append") {
						a := request.Params.Arguments
						if a["format"] != "jsonml" || a["mode"] != "append" || a["jsonml"] != `["root",{},["p",{},["span",{},"new"]]]` || a["revision"] != float64(0) {
							t.Errorf("JSONML append arguments = %#v", a)
						}
						idx, present := a["index"]
						if present != strings.Contains(tc.name, "indexed") || (present && idx != float64(0)) {
							t.Errorf("index presence/value = %v/%v", present, idx)
						}
					}
				default:
					t.Errorf("unexpected tool %q", request.Params.Name)
				}
				body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}}})
				if err != nil {
					return nil, err
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(body)), Request: req}, nil
			})}
			testseam.Swap(t, &rootNewCommandRunnerWithFlags, func(flags *GlobalFlags) executor.Runner {
				return &runtimeRunner{transport: transport.NewClient(client), globalFlags: flags, auditSink: audit.NopSink{}}
			})
			root := NewRootCommand()
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			helpers.GetFormatter().SetWriters(&stdout, &stderr)
			root.SetArgs(tc.args)
			if _, err := pipeline.RunPreParseArgs(root, newPipelineEngine(), tc.args); err != nil {
				t.Fatal(err)
			}
			err := root.Execute()
			if (err != nil) != tc.wantErr {
				t.Errorf("execution error = %v; output=%s stderr=%s", err, stdout.String(), stderr.String())
			}
			t.Logf("observed HTTP reads=%d writes=%d", reads, writes)
			if reads != tc.wantReads || writes != tc.wantWrites {
				t.Errorf("wire reads/writes = %d/%d, want %d/%d", reads, writes, tc.wantReads, tc.wantWrites)
			}
			if tc.wantPreview && !strings.Contains(stdout.String(), "dry_run") && !strings.Contains(stdout.String(), "[dry-run]") {
				t.Errorf("missing dry-run preview: %s", stdout.String())
			}
			if strings.Contains(tc.name, "retains read preview") && !strings.Contains(stdout.String(), "old") {
				t.Errorf("overwrite lost its current-content diff: %s", stdout.String())
			}
		})
	}
}
