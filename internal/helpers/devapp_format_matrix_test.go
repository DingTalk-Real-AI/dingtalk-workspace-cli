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
	"encoding/json"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

// devAppFormatMatrixCases 是 B117 分发表驱动用例集：读叶子（成功路径）与
// 写叶子（--dry-run pending 路径）各取一个代表，覆盖 json/table/pretty/
// csv/ndjson/raw 六种 format。契约规范 §5.2：json = 完整信封（唯一 JSON
// 契约）；table/pretty/csv/ndjson/raw = 仅 data 通道（不输出信封外壳）。
func TestDevAppFormatMatrixDispatching(t *testing.T) {
	readArgs := []string{"dev", "app", "get", "--unified-app-id", "u-1"}
	writeArgs := []string{"dev", "app", "update", "--unified-app-id", "u-1", "--name", "NewName", "--dry-run"}

	readContent := map[string]any{
		"unifiedAppId": "u-1",
		"name":         "DemoApp",
		"appStatus":    "ENABLED",
	}
	writeContent := map[string]any{"unifiedAppId": "u-1", "name": "NewName"}

	// 每个 format 的断言：'envelope' 表示 stdout 必须是完整信封（默认 json、
	// 未知降级）；'data' 表示必须只渲染 data（无信封外壳键）。
	cases := []struct {
		name     string
		format   string
		wantKind string // "envelope" | "data"
	}{
		{"json", "json", "envelope"},
		{"table", "table", "data"},
		{"pretty", "pretty", "data"},
		{"csv", "csv", "data"},
		{"ndjson", "ndjson", "data"},
		{"raw", "raw", "data"},
	}

	for _, mode := range []struct {
		name       string
		args       []string
		content    map[string]any
		wantMarker string
	}{
		{"read", readArgs, readContent, "DemoApp"},
		{"write-dryrun", writeArgs, writeContent, "NewName"},
	} {
		for _, tc := range cases {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				args := append([]string{}, mode.args...)
				args = append(args, "--format", tc.format)
				out, errBuf, err := runDevAppFamily(t, devAppFamilyContentRunner(mode.content), args...)
				if err != nil {
					t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errBuf.String())
				}
				s := out.String()
				if !strings.Contains(s, mode.wantMarker) {
					t.Fatalf("stdout missing business value %q:\n%s", mode.wantMarker, s)
				}
				switch tc.wantKind {
				case "envelope":
					var env map[string]any
					if err := json.Unmarshal(out.Bytes(), &env); err != nil {
						t.Fatalf("-f %s stdout is not a JSON envelope: %v\n%s", tc.format, err, s)
					}
					if env["ok"] != true {
						t.Fatalf("-f %s envelope ok = %#v, want true", tc.format, env["ok"])
					}
				case "data":
					// 非 json 只渲染 data：不得泄漏信封外壳键。
					for _, banned := range []string{`"ok"`, `"outcome"`, `"dry_run"`} {
						if strings.Contains(s, banned) {
							t.Fatalf("-f %s leaked envelope wrapper key %s:\n%s", tc.format, banned, s)
						}
					}
				}
				// 成功/dry-run 数据路径 stderr 应无噪声（除 dry-run 外的
				// 无 format warning 场景）。
				if strings.Contains(errBuf.String(), "[WARN]") {
					t.Fatalf("-f %s unexpected warning on stderr: %q", tc.format, errBuf.String())
				}
			})
		}
	}
}

// TestDevAppFormatMatrixUnknownDegradesToJSON 是 B117 的未知值降级断言：
// 未知 --format 走 AC-09 降级为完整 JSON 信封 + stderr 单行 warning，数据不丢。
// 与轮12 全链路降级回归（TestUnknownFormatFullChainDegradesOverDevAppCommand）
// 同口径，此处作为分发表驱动矩阵的一行固化。
func TestDevAppFormatMatrixUnknownDegradesToJSON(t *testing.T) {
	out, errBuf, err := runDevAppFamily(t,
		devAppFamilyContentRunner(map[string]any{"unifiedAppId": "u-1", "name": "DemoApp"}),
		"dev", "app", "get", "--unified-app-id", "u-1", "--format", "bogus")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errBuf.String())
	}
	var env regressionEnvelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("unknown format must degrade to JSON envelope: %v\n%s", err, out.String())
	}
	if env.OK == nil || !*env.OK || env.Outcome != "success" {
		t.Fatalf("degraded envelope malformed: %s", out.String())
	}
	if env.Data["name"] != "DemoApp" {
		t.Fatalf("business payload lost in degradation: %s", out.String())
	}
	warning := errBuf.String()
	if !strings.Contains(warning, "[WARN]") || !strings.Contains(warning, "bogus") {
		t.Fatalf("stderr warning missing: %q", warning)
	}
}

// TestDevAppFormatMatrixRawPassThrough 是 B117 raw 语义断言：raw 直接透传
// data 载荷（不输出信封外壳）。dev app 成功载荷为 map，raw 即其紧凑 JSON。
func TestDevAppFormatMatrixRawPassThrough(t *testing.T) {
	out, errBuf, err := runDevAppFamily(t,
		devAppFamilyContentRunner(map[string]any{"unifiedAppId": "u-1", "name": "DemoApp"}),
		"dev", "app", "get", "--unified-app-id", "u-1", "--format", "raw")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errBuf.String())
	}
	s := out.String()
	if !strings.Contains(s, "DemoApp") {
		t.Fatalf("raw output missing business value:\n%s", s)
	}
	for _, banned := range []string{`"ok"`, `"outcome"`} {
		if strings.Contains(s, banned) {
			t.Fatalf("raw leaked envelope wrapper key %s:\n%s", banned, s)
		}
	}
	_ = errBuf
}

var _ output.Format = output.FormatJSON
