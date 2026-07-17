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
	"encoding/json"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

// ShareDoc: send a document link to a person by NAME, no ID juggling.
//
// Steps: resolve name → single user (disambiguate on multiple matches) →
// build a Markdown message that links to the document → send it to the user's
// openDingTalkId as a single-chat message. This is a pure "share a link" flow:
// it does not touch any doc tool, it just delivers the URL you already have
// straight to the recipient's inbox.
//
//	dws doc +share-doc --to 张三 --url https://docs.dingtalk.com/xxx --note "帮忙过一下"
var ShareDoc = shortcut.Shortcut{
	Service:     "doc",
	Command:     "+share-doc",
	Product:     "chat",
	Description: "按姓名把文档链接私信发给某人（自动解析 userId）",
	Intent: "当你手上已经有一个文档链接、想直接私信发给某个人而不必先查 userId 时使用；" +
		"内部先按姓名搜通讯录解析出唯一用户，再用 openDingTalkId 把链接拼成一条 Markdown 消息发出去，" +
		"姓名匹配到多人时会列出候选让你区分。只发链接、不读取或改动文档本身，会真实发出消息。",
	Risk: shortcut.RiskWrite,
	Flags: []shortcut.Flag{
		{Name: "to", Type: shortcut.FlagString, Desc: "收件人姓名/花名", Required: true},
		{Name: "url", Type: shortcut.FlagString, Desc: "文档链接", Required: true},
		{Name: "note", Type: shortcut.FlagString, Desc: "附言（可选）"},
		shortcut.AIMessageTagFlag(),
	},
	Tips: []string{`dws doc +share-doc --to 张三 --url https://docs.dingtalk.com/xxx --note "帮忙过一下"`},
	Execute: func(rt *shortcut.RuntimeContext) error {
		url := rt.Str("url")
		note := rt.Str("note")

		// Step 1 — resolve the recipient name to a unique userId.
		user, err := resolveUser(rt, rt.Str("to"))
		if err != nil {
			return err
		}
		if user.openDingTalkID == "" {
			return apperrors.NewValidation("通讯录结果缺少 openDingTalkId，无法发送文档分享消息；请改用 chat +messages-send --open-dingtalk-id")
		}

		// Step 2 — build a Markdown message linking to the document.
		text := shareDocBuildText(url, note)
		title := "文档分享"
		content, _ := json.Marshal(map[string]string{"title": title, "text": text})

		// Step 3 — deliver it as a single-chat message.
		return rt.CallMCP("send_personal_message", rt.AddAIMessageTag(map[string]any{
			"receiverOpenDingTalkId": user.openDingTalkID,
			"msgType":                "markdown",
			"content":                string(content),
		}))
	},
}

// shareDocBuildText assembles the Markdown body: a clickable link plus an
// optional note on its own line.
func shareDocBuildText(url, note string) string {
	var b strings.Builder
	b.WriteString("[文档](")
	b.WriteString(url)
	b.WriteString(")")
	if strings.TrimSpace(note) != "" {
		b.WriteString("\n\n")
		b.WriteString(note)
	}
	return b.String()
}

func init() {
	shortcut.Register(ShareDoc)
}
