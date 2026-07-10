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

package wukonghints

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportFiltersAndReconcilesWukongEnvelopes(t *testing.T) {
	root := t.TempDir()
	surfacePath := filepath.Join(root, "surface.json")
	writeTestFile(t, surfacePath, `{
  "version": 1,
  "products": [{
    "id": "calendar",
    "tools": [
      {"canonical_path": "calendar.get_calendar_detail", "cli_path": "calendar event get"},
      {"canonical_path": "calendar.delete_calendar_event", "cli_path": "calendar event delete", "aliases": ["calendar event remove"]}
    ]
  }]
}`)
	envelopeDir := filepath.Join(root, "prod")
	writeTestFile(t, filepath.Join(envelopeDir, "calendar.json"), `{
  "server": {"name": "calendar", "description": "日历服务"},
  "_meta": {
    "com.dingtalk.mcp.registry/metadata": {"status": "active", "isLatest": true},
    "com.dingtalk.mcp.registry/cli": {
      "id": "calendar",
      "command": "calendar",
      "description": "日历日程与会议室",
      "toolOverrides": {
        "get_calendar_detail": {
          "cliName": "get",
          "group": "event",
          "description": "获取日程详情",
          "example": "  dws calendar event get --id EVENT_ID\n  ignored example"
        },
        "delete_calendar_event": {
          "cliName": "remove",
          "cliAliases": ["delete"],
          "group": "event",
          "description": "删除日程",
          "example": "dws calendar event delete --id EVENT_ID --yes",
          "isSensitive": true
        },
        "list_calendar_events": {
          "cliName": "list",
          "group": "event",
          "description": "列出日程"
        },
        "hidden_tool": {"cliName": "internal", "hidden": true},
        "legacy_tool": {"description": "旧工具"}
      }
    }
  }
}`)

	result, err := Import(Options{
		EnvelopeDir: envelopeDir,
		SurfacePath: surfacePath,
		Repository:  "https://example.invalid/dws-wukong",
		Revision:    "1234567890abcdef",
		Channel:     "prod",
		MaxExamples: 2,
	})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	coverage := result.Hints.Coverage
	if coverage.SourceProducts != 1 || coverage.MatchedProducts != 1 || coverage.SourceTools != 5 || coverage.EligibleTools != 3 || coverage.MatchedTools != 2 || coverage.UnmatchedTools != 1 {
		t.Fatalf("coverage = %#v", coverage)
	}
	if len(result.Hints.Tools) != 2 || len(result.Audit.Skipped) != 2 || len(result.Audit.Unmatched) != 1 {
		t.Fatalf("result = %#v", result)
	}
	get := result.Hints.Tools["calendar.get_calendar_detail"]
	if get.AgentSummary != "获取日程详情" || len(get.Examples) != 1 || get.Examples[0] != "dws calendar event get --id EVENT_ID" {
		t.Fatalf("get hint = %#v", get)
	}
	if get.InterfaceRef == nil || get.InterfaceRef.ProductID != "calendar" || get.InterfaceRef.RPCName != "get_calendar_detail" {
		t.Fatalf("get interface ref = %#v", get.InterfaceRef)
	}
	deleteHint := result.Hints.Tools["calendar.delete_calendar_event"]
	if deleteHint.Risk != "high" || deleteHint.Confirmation != "user_required" || len(deleteHint.Examples) != 1 {
		t.Fatalf("delete hint = %#v", deleteHint)
	}
	if result.Hints.Products["calendar"].AgentSummary != "日历日程与会议室" {
		t.Fatalf("product hint = %#v", result.Hints.Products["calendar"])
	}
	if result.Hints.Source.SourceHash == "" || result.Hints.Source.Revision != "1234567890abcdef" {
		t.Fatalf("source = %#v", result.Hints.Source)
	}
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}
