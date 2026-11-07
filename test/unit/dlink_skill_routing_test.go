package unit_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDLinkSkillRoutingContract(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))

	tests := []struct {
		path      string
		required  []string
		forbidden []string
	}{
		{
			path: filepath.Join("skills", "multi", "dingtalk-shared", "SKILL.md"),
			required: []string{
				"来源未验证的 nodeId", "extension=dlink", "result.fileId",
				"linkSourceInfo.nodeId", "最终目标 ID",
			},
		},
		{
			path: filepath.Join("skills", "multi", "dingtalk-doc", "SKILL.md"),
			required: []string{
				"extension=dlink", "linkSourceInfo.nodeId", "逐跳", "循环即停",
				"入口移动/改名/删除", "顶层 ID",
			},
		},
		{
			path: filepath.Join("skills", "multi", "dingtalk-doc", "references", "intent-guide.md"),
			required: []string{
				"extension=dlink", "result.fileId", "dws doc info", "linkSourceInfo",
			},
			forbidden: []string{"快捷方式nodeId"},
		},
		{
			path: filepath.Join("skills", "multi", "dingtalk-drive", "SKILL.md"),
			required: []string{
				"extension=dlink", "result.fileId", "dws doc info", "linkSourceInfo.nodeId", "逐跳",
				"移动、重命名或删除快捷方式入口", "最初的 `result.fileId`",
			},
			forbidden: []string{"快捷方式nodeId", "顶层 nodeId"},
		},
		{
			path: filepath.Join("skills", "multi", "dingtalk-shared", "references", "url-patterns.md"),
			required: []string{
				"产品意图不能替代节点类型证据", "快捷方式解析边界", "result.fileId",
				"entryFileId", "linkSourceInfo.nodeId", "请求失败", "nodeId 重复", "快捷方式入口本身",
			},
			forbidden: []string{"当用户指令中已明确指定产品", "快捷方式nodeId", "返回的顶层 nodeId"},
		},
		{
			path: filepath.Join("skills", "multi", "dingtalk-shared", "references", "intent-guide.md"),
			required: []string{
				"来源未验证的 nodeId", "extension=dlink", "result.fileId",
				"目标 `linkSourceInfo`", "入口自身移动/重命名/删除仍用最初的 `result.fileId`",
			},
			forbidden: []string{"快捷方式nodeId", "仍用顶层 nodeId"},
		},
		{
			path: filepath.Join("skills", "multi", "dingtalk-misc", "references", "sheet.md"),
			required: []string{
				"extension=dlink", "result.fileId", "linkSourceInfo.nodeId",
				"最终目标 `extension=axls`", "ID 重复", "快捷方式入口",
			},
			forbidden: []string{"快捷方式nodeId", "入口管理仍用顶层 nodeId"},
		},
		{
			path: filepath.Join("skills", "multi", "dingtalk-misc", "references", "sheet-intent-guide.md"),
			required: []string{
				"extension=dlink", "result.fileId", "逐跳读取目标 `linkSourceInfo`", "最终目标为 axls",
			},
		},
		{
			path: filepath.Join("skills", "multi", "dingtalk-aitable", "SKILL.md"),
			required: []string{
				"来源未验证的 nodeId", "extension=dlink", "result.fileId", "linkSourceInfo.nodeId",
				"最终类型不是 able", "快捷方式入口本身", "最初的 `result.fileId`", "不远程解析 dlink",
			},
			forbidden: []string{"快捷方式nodeId", "顶层 nodeId"},
		},
		{
			path: filepath.Join("skills", "multi", "dingtalk-aitable", "references", "intent-guide.md"),
			required: []string{
				"extension=dlink", "result.fileId", "不能把入口 ID 当 baseId", "最终目标 `extension=able`",
			},
			forbidden: []string{"先用 `+url-resolve` 取稳定 ID"},
		},
		{
			path: filepath.Join("skills", "multi", "dingtalk-aitable", "references", "url-patterns.md"),
			required: []string{
				"完整且权威", "产品意图不能替代节点类型证据", "extension=dlink",
				"result.fileId", "entryFileId", "linkSourceInfo.nodeId", "快捷方式入口 ID",
			},
			forbidden: []string{"当用户指令中已明确指定产品", "快捷方式nodeId", "返回的顶层 nodeId"},
		},
		{
			path: filepath.Join("skills", "multi", "dingtalk-misc", "references", "url-patterns.md"),
			required: []string{
				"完整且权威", "产品意图不能替代节点类型证据", "extension=dlink",
				"result.fileId", "entryFileId", "linkSourceInfo.nodeId", "快捷方式入口 ID",
			},
			forbidden: []string{"当用户指令中已明确指定产品", "快捷方式nodeId", "返回的顶层 nodeId"},
		},
		{
			path: filepath.Join("skills", "mono", "references", "url-patterns.md"),
			required: []string{
				"快捷方式解析边界", "linkSourceInfo.nodeId", "请求失败", "nodeId 重复",
				"快捷方式入口本身", "顶层 nodeId",
			},
			forbidden: []string{"当用户指令中已明确指定产品"},
		},
		{
			path: filepath.Join("skills", "mono", "references", "products", "doc", "doc-info.md"),
			required: []string{
				"linkSourceInfo.nodeId", "只返回一跳目标", "逐跳记录已访问 nodeId",
				"移动、重命名或删除入口本身", "顶层 nodeId",
			},
		},
		{
			path: filepath.Join("skills", "mono", "references", "products", "drive.md"),
			required: []string{
				"`dlink` 不能按普通文件下载", "linkSourceInfo.nodeId", "逐跳", "nodeId 重复",
				"移动、重命名或删除快捷方式入口本身", "顶层 nodeId",
			},
		},
		{
			path: filepath.Join("skills", "mono", "references", "products", "sheet.md"),
			required: []string{
				"extension=dlink", "result.fileId", "linkSourceInfo.nodeId",
				"最终目标 `extension=axls`", "ID 重复", "快捷方式入口",
			},
			forbidden: []string{"快捷方式nodeId", "入口管理仍用顶层 nodeId"},
		},
		{
			path: filepath.Join("skills", "mono", "references", "products", "aitable.md"),
			required: []string{
				"URL/节点 ID → baseId 规范化", "extension=dlink", "linkSourceInfo",
				"禁止直接把 dlink 快捷方式入口",
			},
			forbidden: []string{"提取 `/nodes/` 后的路径段作为 `baseId`"},
		},
	}

	for _, test := range tests {
		t.Run(filepath.ToSlash(test.path), func(t *testing.T) {
			path := filepath.Join(root, test.path)
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			text := string(content)
			for _, required := range test.required {
				if !strings.Contains(text, required) {
					t.Errorf("%s missing dlink routing contract %q", path, required)
				}
			}
			for _, forbidden := range test.forbidden {
				if strings.Contains(text, forbidden) {
					t.Errorf("%s retains contradictory dlink routing guidance %q", path, forbidden)
				}
			}
		})
	}
}
