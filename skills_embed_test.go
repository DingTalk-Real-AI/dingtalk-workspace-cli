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

package dws

import (
	"encoding/json"
	"errors"
	"io/fs"
	"testing"
)

func TestEmbeddedSkillsContainsOnlyInstallableTrees(t *testing.T) {
	for _, path := range []string{
		"skills/mono/SKILL.md",
		"skills/multi/dingtalk-shared/SKILL.md",
		"skills/multi/dingtalk-sport/SKILL.md",
		"skills/multi/dingtalk-sport/references/tool-catalog.md",
		"skills/multi/dingtalk-sport/scripts/call_mcp.py",
		"skills/multi/dingtalk-sport/mcp-config.json",
		"skills/multi/dingtalk-sport/.env.example",
		"skills/multi/dingtalk-sport/.skillignore",
	} {
		if _, err := fs.Stat(EmbeddedSkills, path); err != nil {
			t.Fatalf("expected installable skill path %q in embedded FS: %v", path, err)
		}
	}

	if _, err := fs.Stat(EmbeddedSkills, "internal/cli/schema_hints"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("build-only internal/cli/schema_hints must not be present in embedded FS: %v", err)
	}
}

func TestEmbeddedSportSkillMCPConfig(t *testing.T) {
	data, err := EmbeddedSkills.ReadFile("skills/multi/dingtalk-sport/mcp-config.json")
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]struct {
		URL   string `json:"url"`
		MCPID string `json:"mcpId"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("运动 Skill 的 MCP 配置不是有效 JSON: %v", err)
	}
	service, ok := config["DINGTALK_SPORT_HEALTH_URL"]
	if !ok || len(config) != 1 {
		t.Fatal("运动 Skill 必须仅声明 DINGTALK_SPORT_HEALTH_URL 服务")
	}
	if service.MCPID != "10816" {
		t.Fatalf("运动 Skill 的 MCP 服务标识应为 10816，实际为 %q", service.MCPID)
	}
	if service.URL != "" {
		t.Fatal("预制运动 Skill 不得包含真实 MCP 连接 URL")
	}
}
