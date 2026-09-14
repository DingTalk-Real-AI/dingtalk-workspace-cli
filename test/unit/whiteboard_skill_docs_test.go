// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package unit_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWhiteboardReferencesAreDeliveredToBothSkillSurfaces(t *testing.T) {
	pairs := [][2]string{
		{"../../skills/mono/references/products/whiteboard.md", "../../skills/multi/dingtalk-misc/references/whiteboard.md"},
		{"../../skills/mono/references/products/whiteboard/compose.md", "../../skills/multi/dingtalk-misc/references/whiteboard/compose.md"},
		{"../../skills/mono/references/products/whiteboard/vector.md", "../../skills/multi/dingtalk-misc/references/whiteboard/vector.md"},
		{"../../skills/mono/references/products/whiteboard/replace.md", "../../skills/multi/dingtalk-misc/references/whiteboard/replace.md"},
		{"../../skills/mono/references/products/whiteboard/open-nodes-v1.md", "../../skills/multi/dingtalk-misc/references/whiteboard/open-nodes-v1.md"},
	}
	for _, pair := range pairs {
		mono, err := os.ReadFile(pair[0])
		if err != nil {
			t.Fatal(err)
		}
		multi, err := os.ReadFile(pair[1])
		if err != nil {
			t.Fatal(err)
		}
		if string(mono) != string(multi) {
			t.Errorf("mono and multi whiteboard reference differ: %s", filepath.Base(pair[0]))
		}
	}

	root, err := os.ReadFile(pairs[0][0])
	if err != nil {
		t.Fatal(err)
	}
	content := string(root)
	for _, required := range []string{
		"./whiteboard/compose.md",
		"./whiteboard/vector.md",
		"./whiteboard/replace.md",
		"每个普通任务最多读取一份操作",
		"append 和 overwrite 的 `verified=true` 均包含独立读回证据",
		"## 调用与上下文预算",
		"投影只减少重复上下文，不删除业务信息",
		"成功结果只返回稳定目标、mode、验证节点",
		"不返回 `source.pages[].nodes` 完整快照",
		"更新后完整快照时，再执行一次 `+query`",
		"几何、样式、文本、关系必须保留",
		"query/export/audit 需要完整快照时原样",
		"不得因丢字段而重发远端请求",
		"## 坐标读回与稳定结构",
		"0.5 像素",
		"已有成功回执或已读到真实节点时，停止重提并只读对账",
		"append 会创建新节点，不会修正已有节点",
		"commitState=committed",
		"**提交状态不明**",
		"都不能证明未提交",
		"**明确未提交**",
		"须先对账并另行确认范围和授权",
		"不是已落库节点的修复手段",
	} {
		if !strings.Contains(content, required) {
			t.Errorf("whiteboard root reference missing %q", required)
		}
	}
	for _, forbidden := range []string{"recipes.md", "## Golden Routes", "立即改用", "稳定结构只再提交一次", "第一次出现相同节点"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("whiteboard root contains retired routing %q", forbidden)
		}
	}
}

func TestWhiteboardProtocolDocumentsLocalAndServerValidationBoundaries(t *testing.T) {
	for _, path := range []string{
		"../../skills/mono/references/products/whiteboard/open-nodes-v1.md",
		"../../skills/multi/dingtalk-misc/references/whiteboard/open-nodes-v1.md",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		content := string(data)
		for _, required := range []string{
			"原子 `whiteboard update` 本地预检",
			"`whiteboard +update` 还在调用服务前校验",
			"端点类型/有限坐标", "`nodeRef`", "anchor、marker、routing", "waypoints",
			"本地校验失败不会调用服务", "仍由白板服务完整校验",
			"读回失败不代表未写入", "`data.receipt`", "`dryRun=true/executed=false`",
		} {
			if !strings.Contains(content, required) {
				t.Errorf("%s missing validation boundary %q", path, required)
			}
		}
		for _, stale := range []string{"CLI 只预检", "任一层失败都不会保留部分更新"} {
			if strings.Contains(content, stale) {
				t.Errorf("%s retains misleading validation guarantee %q", path, stale)
			}
		}
	}
}

func TestWhiteboardOperationReferencesPublishValidEnvelopes(t *testing.T) {
	cases := []struct {
		path      string
		overwrite bool
		nodeType  string
	}{
		{"../../skills/multi/dingtalk-misc/references/whiteboard/compose.md", false, "shape"},
		{"../../skills/multi/dingtalk-misc/references/whiteboard/vector.md", false, "vector"},
		{"../../skills/multi/dingtalk-misc/references/whiteboard/replace.md", true, ""},
	}
	for _, tc := range cases {
		t.Run(filepath.Base(tc.path), func(t *testing.T) {
			data, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			payload := firstJSONFence(t, string(data))
			if payload["overwrite"] != tc.overwrite {
				t.Fatalf("overwrite = %#v, want %t", payload["overwrite"], tc.overwrite)
			}
			source, ok := payload["source"].(map[string]any)
			if !ok || source["schemaVersion"] != "1.0" || source["catalogVersion"] != "dml-v1" {
				t.Fatalf("invalid OpenNodes source envelope: %#v", payload["source"])
			}
			nodes, ok := source["nodes"].([]any)
			if !ok {
				t.Fatalf("source.nodes = %#v, want array", source["nodes"])
			}
			if tc.nodeType == "" {
				if len(nodes) != 0 {
					t.Fatalf("clear envelope nodes = %#v, want empty", nodes)
				}
			} else {
				if len(nodes) == 0 {
					t.Fatal("operation envelope has no nodes")
				}
				for index, rawNode := range nodes {
					node, ok := rawNode.(map[string]any)
					if !ok || strings.TrimSpace(stringValue(node["id"])) == "" || strings.TrimSpace(stringValue(node["type"])) == "" {
						t.Fatalf("source.nodes[%d] must have stable id and type: %#v", index, rawNode)
					}
				}
				node, _ := nodes[0].(map[string]any)
				if node["type"] != tc.nodeType {
					t.Fatalf("first node type = %#v, want %q", node["type"], tc.nodeType)
				}
			}
			var commands []string
			for _, fence := range bashFences(string(data)) {
				for _, line := range strings.Split(fence, "\n") {
					fields := strings.Fields(line)
					if len(fields) >= 3 && fields[0] == "dws" && fields[1] == "whiteboard" {
						commands = append(commands, fields[2])
					}
				}
				if strings.Contains(fence, "--yes") {
					t.Errorf("stored write example must not pre-authorize execution:\n%s", fence)
				}
			}
			if tc.overwrite && strings.Join(commands, " ") != "+query +update" {
				t.Errorf("overwrite default commands = %v, want one pre-write snapshot then one internally verified update", commands)
			}
		})
	}
}

func TestWhiteboardOperationReferencesKeepReviewedCapabilities(t *testing.T) {
	required := map[string][]string{
		"../../skills/multi/dingtalk-misc/references/whiteboard/compose.md": {
			"## 首次远端写入前检查",
			"connector `nodeRef` 和 `parentId`",
			"绝对 `M` + 显式绝对",
			"connector 不放入 Frame",
			`"type": "linearGradient"`,
			`"type": "shadow"`,
			`"parentId": "stage"`,
			`"type": "connector"`,
			`"id": "start-to-finish"`,
			"禁止 waypoints",
			"同一请求的",
			`"type": "frame"`,
			`"catalogId": "task/task-done"`,
			`"data": "M0,18 Q60,2 125,16 Q190,30 250,10"`,
		},
		"../../skills/multi/dingtalk-misc/references/whiteboard/vector.md": {
			"上传只准备资源，不插入文档正文",
			`"kind": "managed"`,
			`"resourceId": "<RESOURCE_ID>"`,
			`"url": "<RESOURCE_URL>"`,
		},
		"../../skills/multi/dingtalk-misc/references/whiteboard/replace.md": {
			`"overwrite": true`,
			`"nodes": []`,
			"不要先清空再追加",
			"不再追加 `+query`",
			"仅用户明确要求更新后完整快照时",
			"分别报告更新已验证成功、快照获取失败",
			"不能报告完整成功",
			"最多再对同一目标 `+query` 一次只读对账",
			"不自动重放 overwrite、清空或追加",
		},
	}
	for path, markers := range required {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(data), marker) {
				t.Errorf("%s missing %q", filepath.Base(path), marker)
			}
		}
	}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func bashFences(markdown string) []string {
	const marker = "```bash\n"
	var fences []string
	for {
		start := strings.Index(markdown, marker)
		if start < 0 {
			return fences
		}
		markdown = markdown[start+len(marker):]
		end := strings.Index(markdown, "\n```")
		if end < 0 {
			return fences
		}
		fences = append(fences, markdown[:end])
		markdown = markdown[end+len("\n```"):]
	}
}

func firstJSONFence(t *testing.T, markdown string) map[string]any {
	t.Helper()
	const marker = "```json\n"
	start := strings.Index(markdown, marker)
	if start < 0 {
		t.Fatal("markdown has no JSON fence")
	}
	start += len(marker)
	end := strings.Index(markdown[start:], "\n```")
	if end < 0 {
		t.Fatal("JSON fence is not closed")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(markdown[start:start+end]), &payload); err != nil {
		t.Fatalf("decode first JSON fence: %v", err)
	}
	return payload
}
