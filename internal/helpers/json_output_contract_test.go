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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func executeJSONOutputContractCommand(t *testing.T, caller *scriptedToolCaller, build func() *cobra.Command, args ...string) (string, string, error) {
	t.Helper()
	testseam.Protect(t, &deps)
	testseam.Protect(t, &os.Args)
	InitDeps(caller)
	var stdout, stderr bytes.Buffer
	deps.Out.w = &stdout
	deps.Out.errW = &stderr

	root := build()
	installExampleGlobalFlags(root)
	os.Args = append([]string{"dws", root.Name()}, args...)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		return stdout.String(), stderr.String(), err
	}
	return stdout.String(), stderr.String(), nil
}

func assertJSONOutputPayload(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	return payload
}

func TestCrossPlatformCoverageJSONOutputContractForCompletedFileTransfers(t *testing.T) {
	testseam.Swap(t, &httpGetFile, func(_ context.Context, _ string, _ map[string]string, destination string) error {
		return os.WriteFile(destination, []byte("payload"), 0o600)
	})

	t.Run("drive latest download", func(t *testing.T) {
		outputPath := filepath.Join(t.TempDir(), "latest.txt")
		stdout, stderr, err := executeJSONOutputContractCommand(t,
			&scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: `{"downloadUrl":"https://example.test/latest.txt","fileSize":7,"version":9}`}}},
			newDriveCommand,
			"download", "--node", "node-latest", "--output", outputPath)
		if err != nil {
			t.Fatal(err)
		}
		payload := assertJSONOutputPayload(t, stdout)
		if payload["nodeId"] != "node-latest" || payload["savedPath"] != outputPath || payload["sizeBytes"] != float64(7) || payload["version"] != float64(9) {
			t.Fatalf("payload = %#v", payload)
		}
		if !strings.Contains(stderr, "下载完成") && !strings.Contains(stderr, "下载文件到") {
			t.Fatalf("expected progress on stderr, got %q", stderr)
		}
	})

	t.Run("drive historical download through compatibility flag", func(t *testing.T) {
		outputPath := filepath.Join(t.TempDir(), "versioned.txt")
		stdout, _, err := executeJSONOutputContractCommand(t,
			&scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: `{"downloadUrl":"https://example.test/versioned.txt","fileSize":7}`}}},
			newDriveCommand,
			"download", "--node", "node-versioned", "--version", "4", "--output", outputPath)
		if err != nil {
			t.Fatal(err)
		}
		payload := assertJSONOutputPayload(t, stdout)
		if payload["nodeId"] != "node-versioned" || payload["version"] != float64(4) || payload["sizeBytes"] != float64(7) {
			t.Fatalf("payload = %#v", payload)
		}
	})

	t.Run("drive latest download with default output directory", func(t *testing.T) {
		t.Chdir(t.TempDir())
		stdout, _, err := executeJSONOutputContractCommand(t,
			&scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: `{"downloadUrl":"https://example.test/latest.txt","fileName":"inferred-latest.txt","fileSize":7,"version":9}`}}},
			newDriveCommand,
			"download", "--node", "node-default-output")
		if err != nil {
			t.Fatal(err)
		}
		payload := assertJSONOutputPayload(t, stdout)
		savedPath, _ := payload["savedPath"].(string)
		if filepath.Base(savedPath) != "inferred-latest.txt" {
			t.Fatalf("expected inferred filename in savedPath, got %#v", payload)
		}
		if _, statErr := os.Stat(savedPath); statErr != nil {
			t.Fatalf("expected downloaded file at %q: %v", savedPath, statErr)
		}
	})

	t.Run("drive historical download with default output directory", func(t *testing.T) {
		t.Chdir(t.TempDir())
		stdout, _, err := executeJSONOutputContractCommand(t,
			&scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: `{"downloadUrl":"https://example.test/versioned.txt","fileName":"inferred-versioned.txt","fileSize":7}`}}},
			newDriveCommand,
			"download", "--node", "node-versioned-default", "--version", "4")
		if err != nil {
			t.Fatal(err)
		}
		payload := assertJSONOutputPayload(t, stdout)
		savedPath, _ := payload["savedPath"].(string)
		if filepath.Base(savedPath) != "inferred-versioned.txt" {
			t.Fatalf("expected inferred filename in savedPath, got %#v", payload)
		}
		if _, statErr := os.Stat(savedPath); statErr != nil {
			t.Fatalf("expected downloaded file at %q: %v", savedPath, statErr)
		}
	})

	t.Run("drive download rejects existing target without --overwrite", func(t *testing.T) {
		t.Chdir(t.TempDir())
		caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: `{"downloadUrl":"https://example.test/latest.txt","fileName":"conflict.txt","fileSize":7}`}}}
		if _, _, err := executeJSONOutputContractCommand(t, caller, newDriveCommand, "download", "--node", "node-conflict"); err != nil {
			t.Fatal(err)
		}
		_, _, err := executeJSONOutputContractCommand(t, caller, newDriveCommand, "download", "--node", "node-conflict")
		if err == nil {
			t.Fatal("expected conflict rejection on second identical download")
		}
		var cliErr *CLIError
		if !errors.As(err, &cliErr) || cliErr.Code != CodeFileAlreadyExists || cliErr.Operation != "drive download" {
			t.Fatalf("expected INPUT_FILE_ALREADY_EXISTS CLIError for drive download, got %v", err)
		}
		_, stderr, err := executeJSONOutputContractCommand(t, caller, newDriveCommand, "download", "--node", "node-conflict", "--overwrite")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stderr, "将覆盖") {
			t.Fatalf("expected overwrite warning on stderr, got %q", stderr)
		}
	})

	t.Run("drive download-version rejects existing target without --overwrite", func(t *testing.T) {
		t.Chdir(t.TempDir())
		caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: `{"downloadUrl":"https://example.test/versioned.txt","fileName":"conflict-versioned.txt","fileSize":7}`}}}
		if _, _, err := executeJSONOutputContractCommand(t, caller, newDriveCommand, "download", "--node", "node-conflict-v", "--version", "4"); err != nil {
			t.Fatal(err)
		}
		_, _, err := executeJSONOutputContractCommand(t, caller, newDriveCommand, "download", "--node", "node-conflict-v", "--version", "4")
		if err == nil {
			t.Fatal("expected conflict rejection on second identical versioned download")
		}
		var cliErr *CLIError
		if !errors.As(err, &cliErr) || cliErr.Code != CodeFileAlreadyExists || cliErr.Operation != "drive download-version" {
			t.Fatalf("expected INPUT_FILE_ALREADY_EXISTS CLIError for drive download-version, got %v", err)
		}
		if _, _, err := executeJSONOutputContractCommand(t, caller, newDriveCommand, "download", "--node", "node-conflict-v", "--version", "4", "--overwrite"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("doc export", func(t *testing.T) {
		testseam.Swap(t, &helperAfter, func(time.Duration) <-chan time.Time {
			ch := make(chan time.Time, 1)
			ch <- time.Now()
			return ch
		})
		outputPath := filepath.Join(t.TempDir(), "export.md")
		stdout, stderr, err := executeJSONOutputContractCommand(t,
			&scriptedToolCaller{format: "json", steps: []scriptedToolStep{
				{text: `{"jobId":"export-job-1"}`},
				{text: `{"status":"SUCCESS","downloadUrl":"https://example.test/export.md"}`},
			}},
			newDocCommand,
			"export", "--node", "doc-node", "--export-format", "markdown", "--output", outputPath)
		if err != nil {
			t.Fatal(err)
		}
		payload := assertJSONOutputPayload(t, stdout)
		if payload["nodeId"] != "doc-node" || payload["exportFormat"] != "markdown" || payload["jobId"] != "export-job-1" || payload["taskId"] != "export-job-1" || payload["status"] != "SUCCESS" || payload["sizeBytes"] != float64(7) {
			t.Fatalf("payload = %#v", payload)
		}
		if !strings.Contains(stderr, "提交导出任务") {
			t.Fatalf("expected export progress on stderr, got %q", stderr)
		}
	})
}

func TestCrossPlatformCoverageJSONOutputContractDryRunIsMachineReadable(t *testing.T) {
	stdout, _, err := executeJSONOutputContractCommand(t,
		&scriptedToolCaller{format: "json", dry: true},
		newDriveCommand,
		"download", "--node", "node-dry-run", "--output", filepath.Join(t.TempDir(), "out.txt"), "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	payload := assertJSONOutputPayload(t, stdout)
	if payload["dry_run"] != true || payload["executed"] != false || payload["nodeId"] != "node-dry-run" {
		t.Fatalf("payload = %#v", payload)
	}

	t.Run("text mode default output annotation", func(t *testing.T) {
		stdout, _, err := executeJSONOutputContractCommand(t,
			&scriptedToolCaller{format: "text", dry: true},
			newDriveCommand,
			"download", "--node", "node-dry-run-default", "--dry-run")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stdout, "当前目录（自动推断文件名）") {
			t.Fatalf("expected default-output annotation in text preview, got %q", stdout)
		}
	})

	stdout, _, err = executeJSONOutputContractCommand(t,
		&scriptedToolCaller{format: "json", dry: true},
		newDocCommand,
		"export", "--node", "doc-dry-run", "--export-format", "markdown", "--output", filepath.Join(t.TempDir(), "export.md"), "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	payload = assertJSONOutputPayload(t, stdout)
	if payload["dry_run"] != true || payload["executed"] != false || payload["nodeId"] != "doc-dry-run" || payload["operation"] != "doc_export" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCrossPlatformCoverageJSONOutputContractReportsMissingLocalArtifact(t *testing.T) {
	testseam.Swap(t, &httpGetFile, func(_ context.Context, _ string, _ map[string]string, destPath string) error {
		// 模拟“下载引擎成功返回但本地产物缺失”：drive download 的临时文件
		// 由 downloadViaTemp 预创建，移除后原子发布点 fail closed；doc export
		// 从未写入产物，在读取产物时 fail closed。两个路径都必须失败。
		_ = os.Remove(destPath)
		return nil
	})
	testseam.Swap(t, &helperAfter, func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	})

	// wantErr 为空表示任意非空错误（fail-closed）：drive 下载引擎先写临时文件
	// 再原子发布，stub 移除临时文件时在发布点失败；doc export 在读取产物时失败。
	tests := []struct {
		name    string
		wantErr string
		build   func() *cobra.Command
		args    []string
		steps   []scriptedToolStep
	}{
		{
			name:    "latest drive download",
			wantErr: "",
			build:   newDriveCommand,
			args:    []string{"download", "--node", "node-latest", "--output", filepath.Join(t.TempDir(), "latest.txt")},
			steps:   []scriptedToolStep{{text: `{"downloadUrl":"https://example.test/latest.txt","fileSize":7,"version":9}`}},
		},
		{
			name:    "versioned drive download",
			wantErr: "",
			build:   newDriveCommand,
			args:    []string{"download", "--node", "node-versioned", "--version", "4", "--output", filepath.Join(t.TempDir(), "versioned.txt")},
			steps:   []scriptedToolStep{{text: `{"downloadUrl":"https://example.test/versioned.txt","fileSize":7}`}},
		},
		{
			name:    "doc export",
			wantErr: "读取",
			build:   newDocCommand,
			args:    []string{"export", "--node", "doc-node", "--export-format", "markdown", "--output", filepath.Join(t.TempDir(), "export.md")},
			steps: []scriptedToolStep{
				{text: `{"jobId":"export-job-1"}`},
				{text: `{"status":"SUCCESS","downloadUrl":"https://example.test/export.md"}`},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := executeJSONOutputContractCommand(t, &scriptedToolCaller{format: "json", steps: tt.steps}, tt.build, tt.args...)
			if err == nil {
				t.Fatal("expected missing local artifact to fail closed")
			}
			if tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
