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
)

// TestDevAppAllLeavesStdoutZeroLogBytes 是队列 B119 的 stdout 零日志字节总
// 断言：dev app 全树 32 叶子的 stdout 必须恰为一个 JSON 信封（json解包严格
// 成功），不得混入任何日志/人读杂音字节。读叶子走成功信封、写叶子走
// --dry-run success 信封。任何把日志打到 stdout 的叶子都会在此暴露
// （json.Unmarshal 对前后冗余字节报错）。
func TestDevAppAllLeavesStdoutZeroLogBytes(t *testing.T) {
	for _, entry := range devAppEnvelopeRegressionChecklist() {
		t.Run(entry.path, func(t *testing.T) {
			args := append([]string{}, entry.args...)
			if entry.write {
				args = append(args, "--dry-run")
			}
			out, errBuf, err := runDevAppFamily(t,
				devAppFamilyContentRunner(map[string]any{
					"unifiedAppId": "u-1",
					"name":         "DemoApp",
					"appStatus":    "ENABLED",
				}),
				args...)
			if err != nil {
				t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errBuf.String())
			}
			// 严格：整个 stdout 缓冲必须能被 json 完整消费（无头尾杂音）。
			dec := json.NewDecoder(out)
			var env regressionEnvelope
			if derr := dec.Decode(&env); derr != nil {
				t.Fatalf("stdout is not a single JSON envelope (log bytes leaked?): %v\n%s", derr, out.String())
			}
			if dec.More() {
				t.Fatalf("stdout has trailing content after envelope (log bytes leaked?): %s", out.String())
			}
			if env.OK == nil || !*env.OK {
				t.Fatalf("envelope ok missing/false: %s", out.String())
			}
		})
	}
}

// TestDevAppFieldsProjectionCombinesWithEnvelope 是队列 B120 的 --fields 过滤
// 与信封组合断言。实测收敛现状（W3 接线后固化）：json 模式下 --fields 作用于
// **信封顶层键**（renderEnvelopeInto 的 json 分支：WriteFiltered(json, env,
// fields)），即选 `data`/`ok`/`outcome` 等信封键而非 data 内字段；
// `--fields data` 保留完整 data 载荷、剔除 ok/outcome 外壳。此行为与队列
// 描述（data 内字段过滤）不同，按纪律固化真实行为 + findings 上报。
func TestDevAppFieldsProjectionCombinesWithEnvelope(t *testing.T) {
	content := map[string]any{
		"unifiedAppId": "u-1",
		"name":         "DemoApp",
		"appStatus":    "ENABLED",
	}
	// --fields data：只保留信封顶层 data 键（完整载荷），外壳键被剔除。
	out, errBuf, err := runDevAppFamilyProdAligned(t, devAppFamilyContentRunner(content),
		"dev", "app", "get", "--unified-app-id", "u-1", "--fields", "data")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errBuf.String())
	}
	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("--fields stdout is not JSON: %v\n%s", err, out.String())
	}
	if _, hasOK := env["ok"]; hasOK {
		t.Fatalf("--fields data must drop envelope shell ok (json top-level key filtering): %#v", env)
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("--fields data data not an object: %#v", env["data"])
	}
	if data["name"] != "DemoApp" || data["appStatus"] != "ENABLED" {
		t.Fatalf("--fields data preserved payload = %#v", data)
	}

	// --fields ok：只保留信封顶层 ok 键（布尔值）。
	outOK, _, err := runDevAppFamilyProdAligned(t, devAppFamilyContentRunner(content),
		"dev", "app", "get", "--unified-app-id", "u-1", "--fields", "ok")
	if err != nil {
		t.Fatalf("Execute(--fields ok) error = %v", err)
	}
	var envOK map[string]any
	if err := json.Unmarshal(outOK.Bytes(), &envOK); err != nil {
		t.Fatalf("--fields ok stdout not JSON: %v\n%s", err, outOK.String())
	}
	if envOK["ok"] != true {
		t.Fatalf("--fields ok = %#v, want ok:true", envOK)
	}
	if _, hasData := envOK["data"]; hasData {
		t.Fatalf("--fields ok must drop data key: %#v", envOK)
	}
}

// TestDevAppFieldsTableViewCombines 是队列 B120 的 --fields 与 table 组合：
// 非 json 下 --fields 先投影 data 再按 format 渲染（§5.2 联动），table 视图
// 只含选定列、不含丢弃字段。
func TestDevAppFieldsTableViewCombines(t *testing.T) {
	content := map[string]any{
		"unifiedAppId": "u-1",
		"name":         "DemoApp",
		"appStatus":    "ENABLED",
	}
	out, errBuf, err := runDevAppFamilyProdAligned(t, devAppFamilyContentRunner(content),
		"dev", "app", "get", "--unified-app-id", "u-1", "--fields", "name", "--format", "table")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errBuf.String())
	}
	s := out.String()
	if !strings.Contains(s, "DemoApp") {
		t.Fatalf("table --fields=name missing value:\n%s", s)
	}
	for _, banned := range []string{"ENABLED", "\"appStatus\""} {
		if strings.Contains(s, banned) {
			t.Fatalf("table --fields=name must drop non-selected field %q:\n%s", banned, s)
		}
	}
}
