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

package smart

import (
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// DocAppend: append a chunk of text to the END of a document in one command.
//
// Instead of asking you to first list the document blocks, figure out the
// child count / last-block index, then hand-build an insert_document_block
// element payload, this shortcut leans on update_document's built-in
// "append" mode — the helper documents mode=append as "追加 (在末尾追加，最安全)",
// i.e. it appends the given markdown to the document tail safely without
// touching existing content. Params (nodeId/markdown/mode) are copied verbatim
// from the update_document append call site in internal/helpers/doc.go.
//
//	dws doc +doc-append --doc DOC_ID --text "今天的会议结论：下周一上线。"
var DocAppend = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+doc-append",
	Product:     "doc",
	Description: "在文档末尾追加一段文本（安全追加，不改动原有内容）",
	Intent: "当你只想往一篇钉钉文档的最后面补一段文字、又不想动原有内容时使用；" +
		"内部用文档更新的“追加(append)”模式，把你给的文本安全地拼到文档末尾，" +
		"不需要你先去查文档块列表、算末尾位置或手工拼块结构。" +
		"会真实写入文档内容。",
	Risk: shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "doc", Type: shortcut.FlagString, Desc: "文档 documentId / nodeId（或文档 URL/token）", Required: true},
		{Name: "text", Type: shortcut.FlagString, Desc: "要追加到文档末尾的文本", Required: true},
	},
	Tips: []string{
		`dws doc +doc-append --doc DOC_ID --text "补充说明：本方案已评审通过。"`,
		`dws doc +doc-append --doc "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --text "追加一行备注"`,
	},
	Execute: func(rt *shortcut.RuntimeContext) error {
		nodeID := strings.TrimSpace(rt.Str("doc"))
		if nodeID == "" {
			return apperrors.NewValidation("--doc 不能为空，请提供文档 documentId/nodeId 或文档 URL")
		}
		text := rt.Str("text")
		if strings.TrimSpace(text) == "" {
			return apperrors.NewValidation("--text 不能为空，请提供要追加的文本")
		}

		// update_document append: params copied verbatim from the helper's
		// `doc update --mode append` call site — nodeId + markdown + mode.
		// mode=append 追加到文档末尾（最安全，不清空原内容）。
		return rt.CallMCP("update_document", map[string]any{
			"nodeId":   nodeID,
			"markdown": text,
			"mode":     "append",
		})
	},
}

func init() {
	shortcut.Register(DocAppend)
}
