// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageInsertImageCommandRejectsSVGBeforeRemoteCalls(t *testing.T) {
	// Include active SVG content and raster-looking filenames: the command must
	// reject the bytes locally without sending them to any upload or read tool.
	const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64" onload="alert(1)"><script>alert(2)</script><rect width="64" height="64" fill="blue"/></svg>`
	for _, target := range []struct {
		name string
		args []string
	}{
		{"embedded", []string{"--node", "doc", "--part-id", "part"}},
		{"standalone", []string{"--node", "board", "--page-id", "page"}},
	} {
		for _, name := range []string{"image.svg", "image.SVG", "renamed.png", "renamed.jpg"} {
			for _, mode := range []string{"--dry-run", "--yes"} {
				t.Run(target.name+"/"+name+"/"+mode, func(t *testing.T) {
					t.Setenv("DWS_CONFIG_DIR", t.TempDir())
					t.Setenv("DO_NOT_TRACK", "1")
					helpers.InitDepsForTest(t, helpers.GetCaller())
					t.Cleanup(CloseFileLogger)
					file := filepath.Join(t.TempDir(), name)
					if err := os.WriteFile(file, []byte(svg), 0600); err != nil {
						t.Fatal(err)
					}
					backend := &imageDryRunCommandRunner{}
					testseam.Swap(t, &rootNewCommandRunnerWithFlags, func(f *GlobalFlags) executor.Runner {
						return &runtimeRunner{globalFlags: f, fallback: backend}
					})
					root := newRootCommandWithEngine(t.Context(), nil, false, false)
					var stdout, stderr bytes.Buffer
					root.SetOut(&stdout)
					root.SetErr(&stderr)
					helpers.GetFormatter().SetWriters(&stdout, &stderr)
					args := append([]string{"whiteboard", "+insert-image"}, target.args...)
					root.SetArgs(append(args, "--file", file, mode, "--format", "json"))
					err := root.Execute()
					if err == nil || apperrors.ExitCode(err) != apperrors.ExitCodeValidation || !strings.Contains(err.Error(), "不支持 SVG") {
						t.Fatalf("SVG must fail as validation/3: %v; stdout=%s stderr=%s", err, &stdout, &stderr)
					}
					if len(backend.calls) != 0 {
						t.Fatalf("SVG reached remote execution: %#v", backend.calls)
					}
				})
			}
		}
	}
}

type imageDryRunCommandRunner struct {
	wantTool string
	calls    []executor.Invocation
}

func (r *imageDryRunCommandRunner) Run(_ context.Context, inv executor.Invocation) (executor.Result, error) {
	r.calls = append(r.calls, inv)
	if inv.CanonicalProduct != "whiteboard" || inv.Tool != r.wantTool || inv.DryRun {
		return executor.Result{}, fmt.Errorf("unexpected backend call: %#v", inv)
	}
	if inv.Tool == "read_whiteboard_content" && (inv.Params["nodeId"] != "doc" || inv.Params["partId"] != "part") {
		return executor.Result{}, fmt.Errorf("read changed the embedded target: %#v", inv.Params)
	}
	if inv.Tool == "get_whiteboard_detail" && (inv.Params["nodeId"] != "board" || inv.Params["pageId"] != "page" || inv.Params["view"] != "page") {
		return executor.Result{}, fmt.Errorf("read changed the standalone target: %#v", inv.Params)
	}
	response := map[string]any{
		"success":       true,
		"resultJson":    map[string]any{"schemaVersion": "1.0", "catalogVersion": "dml-v1", "pages": []any{map[string]any{"id": "page", "nodes": []any{}}}},
		"resultSummary": map[string]any{"nodeCount": 0, "pageCount": 1, "readOnlyNodeCount": 0, "unknownNodeCount": 0, "resultBytes": 100, "resultSha256": "fixture-digest"},
	}
	if inv.Tool == "get_whiteboard_detail" {
		response["nodeId"], response["revision"], response["view"] = "board", 3, "page"
	}
	return executor.Result{Invocation: inv, Response: response}, nil
}

func TestCrossPlatformCoverageInsertImageCommandDryRunReadsWithoutUploading(t *testing.T) {
	for _, tc := range []struct {
		name, tool string
		target     []string
	}{
		{"embedded", "read_whiteboard_content", []string{"--node", "doc", "--part-id", "part"}},
		{"standalone", "get_whiteboard_detail", []string{"--node", "board", "--page-id", "page"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())
			t.Setenv("DO_NOT_TRACK", "1")
			helpers.InitDepsForTest(t, helpers.GetCaller())
			t.Cleanup(CloseFileLogger)
			path := filepath.Join(t.TempDir(), "image.png")
			var encoded bytes.Buffer
			if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, encoded.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			backend := &imageDryRunCommandRunner{wantTool: tc.tool}
			var flags *GlobalFlags
			testseam.Swap(t, &rootNewCommandRunnerWithFlags, func(f *GlobalFlags) executor.Runner {
				flags = f
				return &runtimeRunner{globalFlags: f, fallback: backend}
			})
			root := newRootCommandWithEngine(t.Context(), nil, false, false)
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			helpers.GetFormatter().SetWriters(&stdout, &stderr)
			args := append([]string{"whiteboard", "+insert-image"}, tc.target...)
			args = append(args, "--file", path, "--dry-run", "--format", "json")
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatalf("image preview failed: %v; stdout=%s stderr=%s", err, &stdout, &stderr)
			}
			if len(backend.calls) != 1 || flags == nil || !flags.DryRun {
				t.Fatalf("preview bypassed the read-only boundary: calls=%#v flags=%#v", backend.calls, flags)
			}
			var result struct {
				OK   bool           `json:"ok"`
				Data map[string]any `json:"data"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatalf("invalid preview JSON: %v; %s", err, &stdout)
			}
			if !result.OK || result.Data["dryRun"] != true || result.Data["uploaded"] != false || result.Data["committed"] != false {
				t.Fatalf("unexpected preview state: %#v", result)
			}
		})
	}
}
