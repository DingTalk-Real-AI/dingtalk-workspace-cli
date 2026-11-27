// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"strings"
	"testing"
)

func TestCrossPlatformCoverageAitableAttachmentFileModeDelivery(t *testing.T) {
	for _, compact := range []bool{false, true} {
		for _, tc := range []struct{ path, parameter string }{
			{"aitable record create", ""},
			{"aitable record update", ""},
			{"aitable record upsert", ""},
			{"aitable record create", "records"},
			{"aitable record update", "records"},
			{"aitable record upsert", "records"},
			{"aitable +record-update", "records"},
			{"aitable +record-upsert", "records"},
		} {
			args := []string{"--cli-path", tc.path}
			if compact {
				args = append(args, "--compact")
			}
			tool := executeShortcutSchemaQuery(t, args...)
			description, _ := tool["description"].(string)
			if tc.parameter != "" {
				parameter := tool["parameters"].(map[string]any)[tc.parameter].(map[string]any)
				description, _ = parameter["description"].(string)
			}
			for _, required := range []string{`"isLink":false`, "默认链接模式", "JSON 布尔 false", "不自动回退链接模式", "不要编造 filename/type", "不要依赖 null 清空"} {
				if !strings.Contains(description, required) {
					t.Fatalf("attachment file mode missing from %s compact=%t: %s", tc.path, compact, required)
				}
			}
			if tc.path != "aitable record create" {
				for _, fact := range []string{"--records 的值是记录数组", "不能把", "整个对象传给 --records"} {
					if !strings.Contains(description, fact) {
						t.Fatalf("%s compact=%t parameter=%s missing CLI records boundary %q", tc.path, compact, tc.parameter, fact)
					}
				}
				if tc.parameter == "" && !strings.Contains(description, "不能仅因非空附件只传 URL") {
					t.Fatalf("%s compact=%t loses atomic/Shortcut result boundary", tc.path, compact)
				}
				if tc.parameter == "" {
					for _, fact := range []string{
						"不能要求它必须返回 attachmentVerification=verified",
						"不只适用于 [] 清空", "不证明首次写入由原请求造成",
						"不以 attachmentVerification 为必要条件",
						"值级幂等与并发安全分别判断",
						"不为任一历史经过分配概率", "读回失败不转为申请新授权后重发",
					} {
						if !strings.Contains(description, fact) {
							t.Fatalf("%s compact=%t loses current-value evidence boundary %q", tc.path, compact, fact)
						}
					}
				}
			}
		}
	}
}

func TestCrossPlatformCoverageAitableUpdateRecoveryDiscovery(t *testing.T) {
	for _, test := range []struct {
		entry    string
		required []string
	}{
		{"+record-update", []string{"PARTIALLY_COMPLETED", "retryable=false", "不重放"}},
		{"+record-upsert", []string{"clientToken 仅作用于创建分组", "更新分组", "禁止重放", "创建结果不能证明更新已生效", "纯更新不能由 +record-write-result 证明生效"}},
	} {
		t.Run(test.entry, func(t *testing.T) {
			root := NewRootCommand()
			cmd, _, err := root.Find([]string{"aitable", test.entry})
			if err != nil {
				t.Fatal(err)
			}
			descriptions := []string{cmd.Long}
			for _, compact := range []bool{false, true} {
				args := []string{"--cli-path", "aitable " + test.entry}
				if compact {
					args = append(args, "--compact")
				}
				tool := executeShortcutSchemaQuery(t, args...)
				descriptions = append(descriptions, tool["description"].(string))
			}
			required := append([]string{
				"dws aitable +record-query --base-id <BASE_ID> --table-id <TABLE_ID> --record-ids <RECORD_ID>",
				"尚未执行",
				"原写入入口与用于查询契约的 Schema 入口分别记录",
				"仍可按 ID 核对当前记录",
				"禁止重放或重新上传",
				"缺少原请求的期望 cells 时，只报告当前记录是否存在及可读值",
				"不能仅凭该次读回确认原更新生效",
				"clientToken 只作用于创建分组，不保护更新",
			}, test.required...)
			for _, description := range descriptions {
				for _, fragment := range required {
					if !strings.Contains(description, fragment) {
						t.Fatalf("%s recovery discovery missing %q: %s", test.entry, fragment, description)
					}
				}
			}
		})
	}
}

func TestCrossPlatformCoverageAitableShortcutAttachmentRecoveryBoundary(t *testing.T) {
	for _, path := range []string{
		"aitable +record-update", "aitable +record-upsert", "aitable +record-upsert-by-key",
		"aitable +record-batch-create", "aitable +record-bulk-patch",
	} {
		t.Run(path, func(t *testing.T) {
			root := NewRootCommand()
			cmd, _, err := root.Find(strings.Fields(path))
			if err != nil {
				t.Fatal(err)
			}
			descriptions := []string{cmd.Long}
			for _, compact := range []bool{false, true} {
				args := []string{"--cli-path", path}
				if compact {
					args = append(args, "--compact")
				}
				tool := executeShortcutSchemaQuery(t, args...)
				descriptions = append(descriptions, tool["description"].(string))
			}
			for _, description := range descriptions {
				for _, fact := range []string{
					"fieldId 或当前表内精确匹配的唯一字段名",
					"不代表一定是附件字段的问题", "mismatchedFieldId", "非附件字段也可能不一致",
					"非空附件输入若只给 url", "现有记录 Shortcut 的稳定身份校验仍不能确认 verified",
					"unknown、ATTACHMENT_WRITE_UNCONFIRMED、非零退出、retryable=false",
					"不能由诊断脱敏推断业务读回必定没有 fileToken",
					"不能凭英文、短名或合成代称的字面形状",
					"只能说明对应未确认，不证明该 resourceId 无效",
					"不外推为原子 record update/upsert 的固定返回值",
					"checkpoint.attachmentReadbackCommand", "不能只查创建结果就宣布附件确认",
					"不能要求它必须返回 attachmentVerification=verified", "缺少该诊断字段本身不构成核验失败",
					"不只适用于 [] 清空", "不证明首次写入由原请求造成",
					"不以 attachmentVerification 为必要条件",
					"值级幂等与并发安全分别判断",
					"不为任一历史经过分配概率",
					"读回失败不转为申请新授权后重发",
					"禁止重放或重新上传", "原 Base/Table 和记录 ID/唯一键只读核对",
				} {
					if !strings.Contains(description, fact) {
						t.Fatalf("%s Help/Schema missing attachment recovery fact %q", path, fact)
					}
				}
			}
		})
	}
}

func TestCrossPlatformCoverageAitableWriteIntentRejectionDiscovery(t *testing.T) {
	const path = "aitable +record-write-result"
	root := NewRootCommand()
	cmd, _, err := root.Find(strings.Fields(path))
	if err != nil {
		t.Fatal(err)
	}
	descriptions := []string{cmd.Long}
	for _, compact := range []bool{false, true} {
		args := []string{"--cli-path", path}
		if compact {
			args = append(args, "--compact")
		}
		tool := executeShortcutSchemaQuery(t, args...)
		descriptions = append(descriptions, tool["description"].(string))
	}
	for _, description := range descriptions {
		for _, fact := range []string{
			"独立 UUID 文件名", "文件名与 clientToken 是独立标识", "重复更新仍可能覆盖并发修改",
			"保持原 token、只换文件名", "不能据此断言创建去重失效或成功",
			"风险不等于必然重复创建", "不能把“换 token 或换文件”合并成同一结论",
			"另一次写入授权不构成原写入已核实", "校验失败在查询写入结果之前返回 validation",
			"只有显式传入 --write-intent 才校验该文件",
			"省略本身不会因缺文件返回 validation",
			"该入口只对账创建分组，与是否缺少恢复文件无关",
			"纯更新不能由 +record-write-result 证明生效",
			"不会发出 get_record_write_result", "不能据此判断原写入不存在或写入失败",
			"缺少原请求的期望 cells 时，只报告当前记录是否存在及可读值",
			"不能仅凭该次读回确认原更新生效",
		} {
			if !strings.Contains(description, fact) {
				t.Fatalf("%s Help/Schema missing write-intent fact %q", path, fact)
			}
		}
	}
}
