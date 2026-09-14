// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
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

func TestDevConnectListPreservesPublishedTableAndJSONArray(t *testing.T) {
	connectDaemonDirOverride = t.TempDir()
	t.Cleanup(func() { connectDaemonDirOverride = "" })

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

	root := newDevAppTestRoot(&captureRunner{})
	tableOut, tableErr, err := runRootBuffered(t, root, "dev", "connect", "list")
	if err != nil {
		t.Fatalf("connect list error = %v\nstderr:\n%s", err, tableErr.String())
	}
	table := tableOut.String()
	for _, value := range []string{"STATE", "CLIENT", "PID", "CHANNEL", "dingAAA", "dingBBB", "codex", "opencode"} {
		if !strings.Contains(table, value) {
			t.Fatalf("legacy table missing %q:\n%s", value, table)
		}
	}
	if strings.Contains(table, `"outcome"`) || strings.Contains(table, `"ok"`) {
		t.Fatalf("legacy table was enveloped:\n%s", table)
	}

	jsonRoot := newDevAppTestRoot(&captureRunner{})
	jsonOut, jsonErr, err := runRootBuffered(t, jsonRoot, "dev", "connect", "list", "--json")
	if err != nil {
		t.Fatalf("connect list --json error = %v\nstderr:\n%s", err, jsonErr.String())
	}
	var reports []connectHealthReport
	if err := json.Unmarshal(jsonOut.Bytes(), &reports); err != nil {
		t.Fatalf("legacy --json is not a top-level array: %v\n%s", err, jsonOut.String())
	}
	if len(reports) != 2 || strings.Contains(jsonOut.String(), `"outcome"`) || strings.Contains(jsonOut.String(), `"data"`) {
		t.Fatalf("legacy --json changed shape: %s", jsonOut.String())
	}
}

func TestDevConnectListPreservesPublishedEmptyState(t *testing.T) {
	connectDaemonDirOverride = t.TempDir()
	t.Cleanup(func() { connectDaemonDirOverride = "" })

	root := newDevAppTestRoot(&captureRunner{})
	humanOut, humanErr, err := runRootBuffered(t, root, "dev", "connect", "list")
	if err != nil {
		t.Fatalf("connect list empty error = %v\nstderr:\n%s", err, humanErr.String())
	}
	if strings.TrimSpace(humanOut.String()) != "no connectors found" {
		t.Fatalf("legacy empty output=%q", humanOut.String())
	}

	jsonRoot := newDevAppTestRoot(&captureRunner{})
	jsonOut, jsonErr, err := runRootBuffered(t, jsonRoot, "dev", "connect", "list", "--json")
	if err != nil {
		t.Fatalf("connect list empty --json error = %v\nstderr:\n%s", err, jsonErr.String())
	}
	var reports []connectHealthReport
	if err := json.Unmarshal(jsonOut.Bytes(), &reports); err != nil {
		t.Fatalf("legacy empty --json is not an array: %v\n%s", err, jsonOut.String())
	}
	if reports == nil || len(reports) != 0 {
		t.Fatalf("legacy empty --json=%s, want []", jsonOut.String())
	}
}
