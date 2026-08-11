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
	testseam.Swap(t, &httpGetFile, func(context.Context, string, map[string]string, string) error {
		return nil
	})
	testseam.Swap(t, &helperAfter, func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	})

	tests := []struct {
		name  string
		build func() *cobra.Command
		args  []string
		steps []scriptedToolStep
	}{
		{
			name:  "latest drive download",
			build: newDriveCommand,
			args:  []string{"download", "--node", "node-latest", "--output", filepath.Join(t.TempDir(), "latest.txt")},
			steps: []scriptedToolStep{{text: `{"downloadUrl":"https://example.test/latest.txt","fileSize":7,"version":9}`}},
		},
		{
			name:  "versioned drive download",
			build: newDriveCommand,
			args:  []string{"download", "--node", "node-versioned", "--version", "4", "--output", filepath.Join(t.TempDir(), "versioned.txt")},
			steps: []scriptedToolStep{{text: `{"downloadUrl":"https://example.test/versioned.txt","fileSize":7}`}},
		},
		{
			name:  "doc export",
			build: newDocCommand,
			args:  []string{"export", "--node", "doc-node", "--export-format", "markdown", "--output", filepath.Join(t.TempDir(), "export.md")},
			steps: []scriptedToolStep{
				{text: `{"jobId":"export-job-1"}`},
				{text: `{"status":"SUCCESS","downloadUrl":"https://example.test/export.md"}`},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := executeJSONOutputContractCommand(t, &scriptedToolCaller{format: "json", steps: tt.steps}, tt.build, tt.args...)
			if err == nil || !strings.Contains(err.Error(), "读取") {
				t.Fatalf("expected missing local artifact error, got %v", err)
			}
		})
	}
}
