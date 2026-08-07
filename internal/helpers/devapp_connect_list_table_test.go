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
	"os"
	"strings"
	"testing"
)

// TestDevConnectListTableView 是队列 B127 的 `dev connect list` table 断言：
// 该命令走 writeConnectListTable 专用路径（不经 WriteEnvelope），-f table 下
// 渲染为带 STATE/APP NAME/CLIENT/PID/CHANNEL/UPTIME 列头的表视图；-f json 走
// 信封通道（data 数组 + meta.count）。本测试在临时 connect 目录写入两个
// 心跳（healthy + down），验证列头与行值。--format 是生产根命令的持久 flag，
// 故经 newDevAppTestRoot（注册 --format/--dry-run/--yes）挂 dev 子树端到端执行。
func TestDevConnectListTableView(t *testing.T) {
	connectDaemonDirOverride = t.TempDir()
	t.Cleanup(func() { connectDaemonDirOverride = "" })

	// 写入两个连接器心跳：一个 healthy（live pid + connected），一个 down
	//（dead pid）。
	healthyDir, err := connectDaemonDir(daemonDirKey("dingAAA", ""))
	if err != nil {
		t.Fatalf("connectDaemonDir(healthy): %v", err)
	}
	writeJSON(t, connectHeartbeatPath(healthyDir), connectHeartbeat{
		Pid: os.Getpid(), Channel: "codex", ClientID: "dingAAA",
		StartUnix: 1_000_000, ConnectedUnix: 1_000_010, UpdatedUnix: 2_000_000,
	})
	downDir, err := connectDaemonDir(daemonDirKey("dingBBB", ""))
	if err != nil {
		t.Fatalf("connectDaemonDir(down): %v", err)
	}
	writeJSON(t, connectHeartbeatPath(downDir), connectHeartbeat{
		Pid: deadPid(t), Channel: "opencode", ClientID: "dingBBB",
		StartUnix: 1_000_000, ConnectedUnix: 1_000_010, UpdatedUnix: 2_000_000,
	})

	// -f table：专用表视图，列头齐全。
	root := newDevAppTestRoot(&captureRunner{})
	tableOut, tableErr, err := runRootBuffered(t, root, "dev", "connect", "list", "--format", "table")
	if err != nil {
		t.Fatalf("connect list -f table error = %v\nstderr:\n%s", err, tableErr.String())
	}
	table := tableOut.String()
	for _, header := range []string{"STATE", "APP NAME", "CLIENT", "PID", "CHANNEL", "UPTIME"} {
		if !strings.Contains(table, header) {
			t.Fatalf("-f table missing column header %q:\n%s", header, table)
		}
	}
	for _, val := range []string{"dingAAA", "dingBBB", "codex", "opencode"} {
		if !strings.Contains(table, val) {
			t.Fatalf("-f table missing row value %q:\n%s", val, table)
		}
	}
	// 信封外壳不得出现（专用表路径不经 WriteEnvelope）。
	if strings.Contains(table, `"outcome"`) || strings.Contains(table, `"ok"`) {
		t.Fatalf("-f table leaked envelope shell:\n%s", table)
	}

	// -f json：信封通道，data 数组 + meta.count。
	jsonRoot := newDevAppTestRoot(&captureRunner{})
	jsonOut, jsonErr, err := runRootBuffered(t, jsonRoot, "dev", "connect", "list", "--format", "json")
	if err != nil {
		t.Fatalf("connect list -f json error = %v\nstderr:\n%s", err, jsonErr.String())
	}
	env := decodePhaseFConnListEnvelope(t, jsonOut.Bytes())
	if !env.OK || env.Outcome != "success" {
		t.Fatalf("connect list json envelope ok/outcome = %v/%q, want true/success: %s",
			env.OK, env.Outcome, jsonOut.String())
	}
	if len(env.Data) != 2 {
		t.Fatalf("connect list json data len = %d, want 2: %s", len(env.Data), jsonOut.String())
	}
}

// decodePhaseFConnListEnvelope 解析 `dev connect list -f json` 的信封：data 为
// 连接器数组（非 map），故用独立结构解码并对信封形态做基本校验。
func decodePhaseFConnListEnvelope(t *testing.T, raw []byte) *struct {
	OK      bool             `json:"ok"`
	Outcome string           `json:"outcome"`
	Data    []map[string]any `json:"data"`
} {
	t.Helper()
	var env struct {
		OK      bool             `json:"ok"`
		Outcome string           `json:"outcome"`
		Data    []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("stdout is not a single valid connect-list JSON envelope: %v\n%s", err, raw)
	}
	return &env
}

// TestDevConnectListTableEmptyState 是队列 B127 的空态 table 断言：无任何
// 连接器时 -f table 输出 "no connectors found"（专用路径空态合法文案），
// -f json 输出 data:[] + count:0 信封（AC-06）。
func TestDevConnectListTableEmptyState(t *testing.T) {
	connectDaemonDirOverride = t.TempDir()
	t.Cleanup(func() { connectDaemonDirOverride = "" })

	// -f table 空态：人读文案。
	tableRoot := newDevAppTestRoot(&captureRunner{})
	tableOut, tableErr, err := runRootBuffered(t, tableRoot, "dev", "connect", "list", "--format", "table")
	if err != nil {
		t.Fatalf("connect list empty -f table error = %v\nstderr:\n%s", err, tableErr.String())
	}
	if !strings.Contains(tableOut.String(), "no connectors found") {
		t.Fatalf("-f table empty state = %q, want 'no connectors found'", tableOut.String())
	}

	// -f json 空态：data:[] + count:0 信封。
	jsonRoot := newDevAppTestRoot(&captureRunner{})
	jsonOut, jsonErr, err := runRootBuffered(t, jsonRoot, "dev", "connect", "list", "--format", "json")
	if err != nil {
		t.Fatalf("connect list empty -f json error = %v\nstderr:\n%s", err, jsonErr.String())
	}
	if !strings.Contains(jsonOut.String(), `"count": 0`) {
		t.Fatalf("-f json empty state must carry count:0:\n%s", jsonOut.String())
	}
}
