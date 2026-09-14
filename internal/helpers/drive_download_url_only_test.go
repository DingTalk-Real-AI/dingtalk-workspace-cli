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
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

// driveURLOnlySignedResponse 覆盖全字段：签名 URL、非空 headers、fileName、
// fileSize、version（download --url-only 从响应透出 version 的场景）。
const driveURLOnlySignedResponse = `{"result":{"downloadUrl":"https://oss.example.test/report.pdf?Expires=1&OSSAccessKeyId=2&Signature=3","headers":{"dentry-token":"token-1"},"fileName":"report.pdf","fileSize":2048,"version":9}}`

// driveURLOnlyMinimalResponse 覆盖最小字段：仅签名 URL（OSS 预签名场景，
// headers 为空对象、无 fileName/fileSize/version）。
const driveURLOnlyMinimalResponse = `{"downloadUrl":"https://oss.example.test/plain.txt?Expires=1&Signature=2"}`

func TestCrossPlatformCoverageDriveDownloadURLOnly(t *testing.T) {
	t.Run("latest json full payload", func(t *testing.T) {
		t.Chdir(t.TempDir())
		caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: driveURLOnlySignedResponse}}}
		stdout, _, err := executeJSONOutputContractCommand(t, caller, newDriveCommand,
			"download", "--node", "node-url-full", "--url-only")
		if err != nil {
			t.Fatal(err)
		}
		if caller.tool != "download_file" {
			t.Fatalf("called tool %q, want download_file", caller.tool)
		}
		if want := map[string]any{"fileId": "node-url-full"}; !reflect.DeepEqual(caller.args, want) {
			t.Fatalf("args = %#v, want %#v", caller.args, want)
		}
		payload := assertJSONOutputPayload(t, stdout)
		want := map[string]any{
			"success":     true,
			"urlOnly":     true,
			"nodeId":      "node-url-full",
			"downloadUrl": "https://oss.example.test/report.pdf?Expires=1&OSSAccessKeyId=2&Signature=3",
			"headers":     map[string]any{"dentry-token": "token-1"},
			"fileName":    "report.pdf",
			"fileSize":    float64(2048),
			"version":     float64(9),
		}
		for k, v := range want {
			if !reflect.DeepEqual(payload[k], v) {
				t.Fatalf("payload[%q] = %#v, want %#v (full payload: %#v)", k, payload[k], v, payload)
			}
		}
		if len(payload) != len(want) {
			t.Fatalf("payload has unexpected extra keys: %#v", payload)
		}
		// 签名 URL 的 & 分隔符不能被 HTML 转义，否则地址无法直接使用。
		if strings.Contains(stdout, `\u0026`) {
			t.Fatalf("stdout escapes signed query separator: %q", stdout)
		}
		// URL 模式不落盘：当前目录必须保持为空。
		entries, readErr := os.ReadDir(".")
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(entries) != 0 {
			t.Fatalf("url-only mode wrote local files: %v", entries)
		}
	})

	t.Run("latest json minimal payload", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: driveURLOnlyMinimalResponse}}}
		stdout, _, err := executeJSONOutputContractCommand(t, caller, newDriveCommand,
			"download", "--node", "node-url-minimal", "--url-only")
		if err != nil {
			t.Fatal(err)
		}
		payload := assertJSONOutputPayload(t, stdout)
		want := map[string]any{
			"success":     true,
			"urlOnly":     true,
			"nodeId":      "node-url-minimal",
			"downloadUrl": "https://oss.example.test/plain.txt?Expires=1&Signature=2",
			"headers":     map[string]any{},
		}
		if len(payload) != len(want) {
			t.Fatalf("payload = %#v, want exactly %#v", payload, want)
		}
		for k, v := range want {
			if !reflect.DeepEqual(payload[k], v) {
				t.Fatalf("payload[%q] = %#v, want %#v", k, payload[k], v)
			}
		}
	})

	t.Run("json dry run", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "json", dry: true}
		stdout, _, err := executeJSONOutputContractCommand(t, caller, newDriveCommand,
			"download", "--node", "node-url-dry", "--url-only", "--dry-run")
		if err != nil {
			t.Fatal(err)
		}
		if caller.calls != 0 {
			t.Fatalf("dry-run must not call the MCP tool, got %d calls", caller.calls)
		}
		payload := assertJSONOutputPayload(t, stdout)
		want := map[string]any{
			"dry_run":      true,
			"executed":     false,
			"preview_kind": "plan",
			"operation":    "drive_download",
			"nodeId":       "node-url-dry",
			"urlOnly":      true,
		}
		if len(payload) != len(want) {
			t.Fatalf("payload = %#v, want exactly %#v", payload, want)
		}
		for k, v := range want {
			if !reflect.DeepEqual(payload[k], v) {
				t.Fatalf("payload[%q] = %#v, want %#v", k, payload[k], v)
			}
		}
	})

	t.Run("text dry run latest omits version", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "text", dry: true}
		stdout, _, err := executeJSONOutputContractCommand(t, caller, newDriveCommand,
			"download", "--node", "node-url-dry-text", "--url-only", "--dry-run")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stdout, "获取下载地址（--url-only，不落盘）") {
			t.Fatalf("expected url-only operation in text preview, got %q", stdout)
		}
		if !strings.Contains(stdout, "node-url-dry-text") {
			t.Fatalf("expected node id in text preview, got %q", stdout)
		}
		if strings.Contains(stdout, "版本号") {
			t.Fatalf("latest download must not preview a version, got %q", stdout)
		}
	})

	t.Run("text dry run versioned shows version", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "text", dry: true}
		stdout, _, err := executeJSONOutputContractCommand(t, caller, newDriveCommand,
			"download", "--node", "node-url-dry-v", "--version", "5", "--url-only", "--dry-run")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stdout, "版本号:") || !strings.Contains(stdout, "5") {
			t.Fatalf("expected version in versioned text preview, got %q", stdout)
		}
	})

	t.Run("latest text full payload", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "text", steps: []scriptedToolStep{{text: driveURLOnlySignedResponse}}}
		stdout, _, err := executeJSONOutputContractCommand(t, caller, newDriveCommand,
			"download", "--node", "node-url-text", "--url-only")
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{
			"https://oss.example.test/report.pdf?Expires=1&OSSAccessKeyId=2&Signature=3",
			`"dentry-token":"token-1"`,
			"版本号:",
			"report.pdf",
			"2048 字节",
			"已获取下载地址（未落盘，下载由调用方自行执行）",
		} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("text output missing %q, got %q", want, stdout)
			}
		}
	})

	t.Run("latest text minimal payload", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "text", steps: []scriptedToolStep{{text: driveURLOnlyMinimalResponse}}}
		stdout, _, err := executeJSONOutputContractCommand(t, caller, newDriveCommand,
			"download", "--node", "node-url-text-min", "--url-only")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stdout, "（无：签名已内含在下载地址）") {
			t.Fatalf("expected empty-headers annotation, got %q", stdout)
		}
		if strings.Contains(stdout, "版本号:") || strings.Contains(stdout, "文件名:") || strings.Contains(stdout, "文件大小:") {
			t.Fatalf("minimal response must not render absent fields, got %q", stdout)
		}
	})

	t.Run("rejects conflicting flags", func(t *testing.T) {
		tests := []struct {
			name    string
			args    []string
			wantErr string
		}{
			{
				name:    "output",
				args:    []string{"download", "--node", "n1", "--url-only", "--output", "some.txt"},
				wantErr: "--url-only 与 --output 互斥",
			},
			{
				name:    "multiple transfer flags",
				args:    []string{"download", "--node", "n1", "--url-only", "--part-size", "8MB", "--parallel", "2"},
				wantErr: "--url-only 与 --part-size/--parallel 互斥",
			},
			{
				name:    "versioned overwrite",
				args:    []string{"download-version", "--node", "n1", "--version", "3", "--url-only", "--overwrite"},
				wantErr: "--url-only 与 --overwrite 互斥",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				caller := &scriptedToolCaller{format: "json"}
				_, _, err := executeJSONOutputContractCommand(t, caller, newDriveCommand, tt.args...)
				if err == nil {
					t.Fatalf("expected mutual-exclusion rejection for %v", tt.args)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
				}
				if caller.calls != 0 {
					t.Fatalf("conflicting flags must fail before any MCP call, got %d calls", caller.calls)
				}
			})
		}
	})

	t.Run("mcp error propagates", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{err: errors.New("drive rpc boom")}}}
		_, _, err := executeJSONOutputContractCommand(t, caller, newDriveCommand,
			"download", "--node", "node-url-rpc-err", "--url-only")
		if err == nil || !strings.Contains(err.Error(), "drive rpc boom") {
			t.Fatalf("expected rpc error to propagate, got %v", err)
		}
	})

	t.Run("parse error propagates", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: `{"result":{"status":"denied"}}`}}}
		_, _, err := executeJSONOutputContractCommand(t, caller, newDriveCommand,
			"download", "--node", "node-url-parse-err", "--url-only")
		if err == nil || !strings.Contains(err.Error(), "未返回下载链接") {
			t.Fatalf("expected missing downloadUrl rejection, got %v", err)
		}
	})

	t.Run("versioned via download compatibility routing", func(t *testing.T) {
		t.Chdir(t.TempDir())
		caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: `{"downloadUrl":"https://example.test/v4.txt?Expires=1&Signature=9","headers":{"dentry-token":"token-4"}}`}}}
		stdout, _, err := executeJSONOutputContractCommand(t, caller, newDriveCommand,
			"download", "--node", "node-route-v4", "--version", "4", "--url-only")
		if err != nil {
			t.Fatal(err)
		}
		// 路由防御：--version 转发到 download-version 后 --url-only 仍生效，
		// 调用的是 download_file_version 而非落盘下载。
		if caller.tool != "download_file_version" {
			t.Fatalf("called tool %q, want download_file_version", caller.tool)
		}
		if want := map[string]any{"nodeId": "node-route-v4", "version": 4}; !reflect.DeepEqual(caller.args, want) {
			t.Fatalf("args = %#v, want %#v", caller.args, want)
		}
		payload := assertJSONOutputPayload(t, stdout)
		if payload["urlOnly"] != true || payload["nodeId"] != "node-route-v4" || payload["version"] != float64(4) {
			t.Fatalf("payload = %#v", payload)
		}
		if _, ok := payload["savedPath"]; ok {
			t.Fatalf("url-only routing must not report a saved path: %#v", payload)
		}
		entries, readErr := os.ReadDir(".")
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(entries) != 0 {
			t.Fatalf("url-only routing must not write local files: %v", entries)
		}
	})

	t.Run("versioned direct invocation", func(t *testing.T) {
		caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: `{"result":{"downloadUrl":"https://example.test/v7.txt","headers":{"dentry-token":"token-7"}}}`}}}
		stdout, _, err := executeJSONOutputContractCommand(t, caller, newDriveCommand,
			"download-version", "--node", "node-v7", "--version", "7", "--url-only")
		if err != nil {
			t.Fatal(err)
		}
		if caller.tool != "download_file_version" {
			t.Fatalf("called tool %q, want download_file_version", caller.tool)
		}
		payload := assertJSONOutputPayload(t, stdout)
		want := map[string]any{
			"success":     true,
			"urlOnly":     true,
			"nodeId":      "node-v7",
			"version":     float64(7),
			"downloadUrl": "https://example.test/v7.txt",
			"headers":     map[string]any{"dentry-token": "token-7"},
		}
		if len(payload) != len(want) {
			t.Fatalf("payload = %#v, want exactly %#v", payload, want)
		}
		for k, v := range want {
			if !reflect.DeepEqual(payload[k], v) {
				t.Fatalf("payload[%q] = %#v, want %#v", k, payload[k], v)
			}
		}
	})
}

// TestCrossPlatformCoverageDriveDownloadURLOnlyPreservesExplicitOutputFlagSemantics
// 锁定：--url-only 不改变落盘路径的既有行为——不带 --url-only 时 output 语义
// 与变更前一致（缺省落盘当前目录）。
func TestCrossPlatformCoverageDriveDownloadURLOnlyPreservesExplicitOutputFlagSemantics(t *testing.T) {
	testseam.Swap(t, &httpGetFile, func(_ context.Context, _ string, _ map[string]string, destination string) error {
		return os.WriteFile(destination, []byte("payload"), 0o600)
	})
	t.Chdir(t.TempDir())
	caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: `{"downloadUrl":"https://example.test/legacy.txt","fileName":"legacy.txt","fileSize":6}`}}}
	stdout, _, err := executeJSONOutputContractCommand(t, caller, newDriveCommand,
		"download", "--node", "node-legacy")
	if err != nil {
		t.Fatal(err)
	}
	payload := assertJSONOutputPayload(t, stdout)
	if payload["urlOnly"] != nil {
		t.Fatalf("persist mode must not report urlOnly, got %#v", payload)
	}
	savedPath, _ := payload["savedPath"].(string)
	if filepath.Base(savedPath) != "legacy.txt" {
		t.Fatalf("expected default persist path, got %#v", payload)
	}
	if _, statErr := os.Stat(savedPath); statErr != nil {
		t.Fatalf("expected downloaded file at %q: %v", savedPath, statErr)
	}
}
