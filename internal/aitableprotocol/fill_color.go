// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitableprotocol

import (
	"encoding/json"
	"fmt"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// ConditionalFormatHelp is shared by the atomic leaf and shortcut declarations.
const ConditionalFormatHelp = `全量覆盖指定 Grid 视图的条件填色规则，最多 40 条，每条 filters 恰好 1 项。
规则结构：{type: cell|row|column|preRow, formatFieldId, format: {color}, filters: [{fieldId, symbol, value, link: "AND", version: 1}]}。
原生 checkbox 使用 symbol="EQ"、value=true 或 false（JSON 布尔，不加引号）。条件列由 filters[0].fieldId 决定，可与着色列 formatFieldId 不同；文本 "True" 保持文本含义。
formula/lookup/filterUp 的计算结果为 checkbox 时也可传 JSON 布尔，由服务端按字段 Schema 转换；不要根据显示文本猜类型。类型未就绪返回 FILL_COLOR_FIELD_SCHEMA_UNAVAILABLE，整批不提交；保留原请求，等待 Schema 就绪，不改成文本规则或自动重放。
颜色使用英文语义名称。先选色系，再按用户要求的强调程度选样式。
背景色格式为 {色系}_{强度}，强度共 5 档：
faint=极淡半透明底；pale=较淡半透明底；light=浅实色底；medium=中等实色底；strong=最浓实色底。前两档会透出单元格底色，实际观感随明暗主题变化。
色系可选 red（红）、orange（橙）、yellow（黄）、lime（黄绿/青柠）、green（绿）、teal（蓝绿/青）、lightblue（浅蓝）、blue（蓝）、purple（紫）、pink（粉）、grey（灰）。
选色示例：浅红底用 red_light，极淡粉底用 pink_faint，醒目绿底用 green_strong。只要求背景填色时使用上述背景色。
明确要求改文字时：text_{色系}=该色文字、透明背景；text_bg_{色系}=该色文字加同色极淡半透明底。例如 text_red 表示红字，text_bg_blue 表示蓝字配淡蓝底。
使用上述英文名称，不支持 HEX 或自由文本颜色。
symbol 取 GT/LT/GTE/LTE/EQ/NE/CONTAIN/EXCLUSIVE/EXIST/UN_EXIST/ALL_OF/ANY_OF/NONE_OF/BEFORE/AFTER/NOT_BEFORE/NOT_AFTER/DATE_EQ/FROM_NOW/DATE_BETWEEN。
传 [] 清空当前视图全部规则；更新前先读取并保留需要继续使用的原规则。
设置回执与视图查询的已知颜色也使用英文语义名。填色回执 verified=true 才表示独立回读确认；false 或缺失时用 view get 或 +view-get 只读核对，不据请求回显声称已完成，也不自动重放。`

// ConditionalFormatReadHelp lets callers verify colors without learning palette codes.
const ConditionalFormatReadHelp = "条件填色的已知颜色返回英文语义名，例如 red_light、orange_strong；其他规则属性保留。"

// AttachmentReferenceHelp describes the shared record create/update input boundary.
const AttachmentReferenceHelp = `附件数组整体替换；更新时附件清空优先使用 []。上游链路可能丢弃显式 null，不要依赖 null 清空；未传入的字段保持原值。
上传文件用 [{"fileToken":"ft_xxx"}]；普通可下载文件用 [{"url":"https://..."}]（异步转存，需回读确认可用）。
钉钉文档节点链接用 [{"url":"https://alidocs.dingtalk.com/i/nodes/节点ID"}]，省略 isLink 时为默认链接模式；服务端校验源权限后按真实名称、类型保存链接，不导出、不转存、不扩大权限。
显式文件模式使用 [{"url":"https://alidocs.dingtalk.com/i/nodes/节点ID","isLink":false}]，isLink 必须保持 JSON 布尔 false，不得省略、改成 true 或字符串 "false"。服务端校验源权限与下载/转存结果；权限或下载失败时保留错误，不自动回退链接模式或扩大权限。不要编造 filename/type 或资源 ID。未声明的 {nodeId:...} 不受支持。
记录 Shortcut 直接写入 fileToken 时，仅在独立读回的稳定 fileToken/resourceId 可对应时确认 verified；只有 filename/resourceId 而无法对应输入 token 时返回 unknown、ATTACHMENT_WRITE_UNCONFIRMED、retryable=false 和只读核对命令，禁止重放。读回不一致诊断不回显附件值或 fileToken。
记录 Shortcut 的附件 [] 清空后读回失败时也禁止重放；字段目录不可用时不能据空数组排除附件清空。未确认项不计为失败，failedCount 为零时可省略。
含在线附件时 attachmentVerification=verified 才表示独立回读已确认；ATTACHMENT_WRITE_UNCONFIRMED 保留真实记录 ID，禁止自动重放。+attachment-put 仅接受本地文件，append 仍要求已有项可安全重写。`

// ParseConditionalFormatsJSON preserves JSON value types; field-aware conversion belongs to HSF.
func ParseConditionalFormatsJSON(raw string) ([]any, error) {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, apperrors.NewValidation(fmt.Sprintf("--json 解析失败: %v", err))
	}
	items, ok := value.([]any)
	if !ok {
		return nil, apperrors.NewValidation("--json 必须是 conditionalFormats JSON 数组")
	}
	if len(items) > 40 {
		return nil, apperrors.NewValidation("conditionalFormats 最多 40 条")
	}
	for i, item := range items {
		if _, ok := item.(map[string]any); !ok {
			return nil, apperrors.NewValidation(fmt.Sprintf("conditionalFormats[%d] 必须是 JSON 对象", i))
		}
	}
	return items, nil
}
