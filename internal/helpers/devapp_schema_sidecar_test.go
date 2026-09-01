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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
)

// devSchemaAllDump 是 dev 子树的 schema --all 装配 dump 形态（只读投影）。
// 由 AssembleSchemaRegistryFromBound（bind 后装配）→ ToPayload（--all dump）
// 生成，与生产 registerSchemaSourceRoot → ResolveSchemaBuild →
// deliverySchemaCatalog 同一条装配语义，仅命令表面收敛为 dev 子树。
type devSchemaAllDump struct {
	Products []struct {
		ID    string `json:"id"`
		Tools []struct {
			CanonicalPath string `json:"canonical_path"`
			Name          string `json:"name"`
			CLIPath       string `json:"cli_path"`
		} `json:"tools"`
	} `json:"products"`
}

// assembleDevSchemaAllDump 在 dev 测试 root 上走 identity 收集 → bind → 装配 →
// --all dump 的旁路，返回轻量解析结构。只读，不改装配。
func assembleDevSchemaAllDump(t *testing.T) devSchemaAllDump {
	t.Helper()
	root := newDevAppTestRoot(&captureRunner{})
	effective, err := cli.BuildEffectiveCommandRegistry(root)
	if err != nil {
		t.Fatalf("BuildEffectiveCommandRegistry(dev): %v", err)
	}
	bound, err := cli.BindEffectiveCommandRegistry(root, effective)
	if err != nil {
		t.Fatalf("BindEffectiveCommandRegistry(dev): %v", err)
	}
	registry, err := cli.AssembleSchemaRegistryFromBound(bound)
	if err != nil {
		t.Fatalf("AssembleSchemaRegistryFromBound(dev): %v", err)
	}
	payload, err := registry.ToPayload()
	if err != nil {
		t.Fatalf("SchemaRegistry.ToPayload(dev): %v", err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal dev schema --all dump: %v", err)
	}
	var dump devSchemaAllDump
	if err := json.Unmarshal(raw, &dump); err != nil {
		t.Fatalf("dev schema --all dump is not parseable: %v\n%s", err, raw)
	}
	return dump
}

// TestDevSchemaAllDumpToolsParseable 是队列 B192 的 dev tool 可解析旁路断言：
// dev 子树 schema --all 装配 dump 中每个 tool 必须可解析（canonical_path 与
// name 非空、cli_path 非空），且代表 dev 域核心能力的 tool 必须存在。任何
// 装配后在 dump 中泄漏「空 canonical / 空 name」的 dev tool 都会在此暴露。
func TestDevSchemaAllDumpToolsParseable(t *testing.T) {
	dump := assembleDevSchemaAllDump(t)

	var devProduct *struct {
		ID    string `json:"id"`
		Tools []struct {
			CanonicalPath string `json:"canonical_path"`
			Name          string `json:"name"`
			CLIPath       string `json:"cli_path"`
		} `json:"tools"`
	}
	for i := range dump.Products {
		if dump.Products[i].ID == "dev" {
			devProduct = &dump.Products[i]
			break
		}
	}
	if devProduct == nil {
		t.Fatalf("dev schema --all dump missing 'dev' product: got products %#v", dump.Products)
	}
	if len(devProduct.Tools) == 0 {
		t.Fatal("dev schema --all dump has 0 tools")
	}

	canonicals := make(map[string]bool)
	for _, tool := range devProduct.Tools {
		toolName := tool.Name
		t.Run(toolName, func(t *testing.T) {
			if strings.TrimSpace(tool.CanonicalPath) == "" {
				t.Fatalf("dev tool %q has empty canonical_path (not parseable)", toolName)
			}
			if strings.TrimSpace(tool.Name) == "" {
				t.Fatalf("dev tool has empty name (canonical_path=%q)", tool.CanonicalPath)
			}
			if strings.TrimSpace(tool.CLIPath) == "" {
				t.Fatalf("dev tool %q has empty cli_path", toolName)
			}
			canonicals[tool.CanonicalPath] = true
		})
	}

	// dev 域核心能力必须出现在装配 dump（可解析旁路的目标集合）。
	for _, want := range []string{"dev.get_dev_app", "dev.list_dev_app", "dev.list_dev_app_versions", "dev.list_dev_app_events", "dev.list_dev_app_permissions", "dev.list_dev_app_members"} {
		if !canonicals[want] {
			t.Errorf("dev schema --all dump missing canonical tool %q", want)
		}
	}
}

// TestDevSchemaAllDumpCountStable 是队列 B192 的补充观察断言：dev 子树装配 dump
// 的 tool 数量稳定（当前 66；包含 29 条静态 dev mcp 开发命令），作为回归
// 基线快照。数量变化提示 dev 命令表面变更，需复核装配。
func TestDevSchemaAllDumpCountStable(t *testing.T) {
	dump := assembleDevSchemaAllDump(t)
	for _, p := range dump.Products {
		if p.ID == "dev" {
			if len(p.Tools) != 66 {
				t.Fatalf("dev schema --all tool count = %d, want 66 (baseline)", len(p.Tools))
			}
			return
		}
	}
	t.Fatal("dev product not found in schema --all dump")
}
