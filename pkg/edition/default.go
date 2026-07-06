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

package edition

// DefaultOSSClawType is the wire value for request header claw-type in
// the open-source build. It is intentionally hard-wired — the open-source
// CLI does NOT derive claw-type from DINGTALK_AGENT or any other caller
// input, so third-party hosts get a predictable header regardless of
// their environment.
const DefaultOSSClawType = "openClaw"

// defaultHooks returns the open-source edition defaults.
//
// MergeHeaders is the only hook that ships with behaviour: it pins the
// `claw-type` request header to DefaultOSSClawType so every open-source
// MCP request carries the same stable routing tag. All other fields are
// nil — the internal code interprets nil as "use standard open-source
// behaviour".
func defaultHooks() *Hooks {
	return &Hooks{
		Name: "open",
		MergeHeaders: func(base map[string]string) map[string]string {
			if base == nil {
				base = make(map[string]string)
			}
			base["claw-type"] = DefaultOSSClawType
			return base
		},
		StaticServers: openStaticServers,
	}
}

func openStaticServers() []ServerInfo {
	return []ServerInfo{
		{ID: "aitable", Name: "AI 多维表", Endpoint: "https://mcp-gw.dingtalk.com/server/5f0d121611f14e878f7d42c3e32bf6c4a790d433066adae38c062a657c397047", Prefixes: []string{"table", "record", "field", "base", "attachment", "view", "dashboard", "chart", "export", "import"}},
		{ID: "aitable-helper", Name: "AI 多维表(辅助)", Endpoint: "https://mcp-gw.dingtalk.com/server/bb2984ee6b10c1560b4fe943ca620f646bed31f215c551a53abf040b52591a95", Prefixes: []string{"form", "share_form"}},
		{ID: "calendar", Name: "日历", Endpoint: "https://mcp-gw.dingtalk.com/server/3cb83d4ac411227c44c1abde4e4bfbae0ea2c172b83a78a33ffc3821d0d1be47", Prefixes: []string{"calendar", "event"}},
		{ID: "contact", Name: "通讯录", Endpoint: "https://mcp-gw.dingtalk.com/server/db4b26cb38ea6a8739ad55d1997fa1da608cd36b33a6cf0f77884f70c49382fe", Prefixes: []string{"contact", "user", "dept"}},
		{ID: "todo", Name: "待办", Endpoint: "https://mcp-gw.dingtalk.com/server/0f51140eddcd913106c5821a4d0cd577b2d1a0b6cb452dd0e51ab41facf3a83c", Prefixes: []string{"todo", "task"}},
		{ID: "doc", Name: "文档", Endpoint: "https://mcp-gw.dingtalk.com/server/91e17caf44f6ca1ed9c6ce614221a518ac93300ece63ca8d7e9b133f912e0607", Prefixes: []string{"doc"}},
		{ID: "chat", Name: "群聊/会话", Endpoint: "https://mcp-gw.dingtalk.com/server/0a1609437385696b77fc4771c3ddaf5656b487f809966c0cc8d4755e7b1d3b74", Prefixes: []string{"chat", "group"}},
		{ID: "im", Name: "im", Endpoint: "https://mcp-gw.dingtalk.com/server/450eede6b54d83e030140e66ec77c98a2e89a0869ef4db481f8217a98a42f821", Prefixes: []string{"im"}},
		{ID: "teambition", Name: "Teambition", Endpoint: "https://mcp-gw.dingtalk.com/server/f64b4dc8164ab62ffd8ac10fe04f9f6d3af7a35f8f2a4b2a9626570022145853", Prefixes: []string{"tb", "project"}},
		{ID: "oa", Name: "审批", Endpoint: "https://mcp-gw.dingtalk.com/server/8faff71bdfc3cb5437894ada5305b48214eb56408ca31e378f4be2773ba4500c", Prefixes: []string{"oa", "approval"}},
		{ID: "mail", Name: "邮箱", Endpoint: "https://mcp-gw.dingtalk.com/server/81395b96cfd92fd40858094064c1ed2f7f36eca3d7229d22aae426f74261a286", Prefixes: []string{"mail"}},
		{ID: "ding", Name: "DING", Endpoint: "https://mcp-gw.dingtalk.com/server/404106cbb828de22de78bd390e7af4b2b24ec0cdc5088440ce10a41614fa328d", Prefixes: []string{"ding"}},
		{ID: "bot", Name: "机器人消息", Endpoint: "https://mcp-gw.dingtalk.com/server/4717d5cbb92ecdebd89c174e4331dc17207208a97622e2004cac49c0fbedc9d1", Prefixes: []string{"bot", "robot"}},
		{ID: "notify", Name: "工作通知", Endpoint: "https://mcp.dingtalk.com/notify", Prefixes: []string{"notify", "message"}},
		{ID: "devdoc", Name: "开放平台文档", Endpoint: "https://mcp-gw.dingtalk.com/server/47ec90fc0db1e68d84fdd2280129c219873b51e81a23adf9fe7fa29ee9b579b3", Prefixes: []string{"devdoc", "wiki"}},
		{ID: "attendance", Name: "考勤打卡", Endpoint: "https://mcp-gw.dingtalk.com/server/72c8e63fa17cae0ea5bf507e2594d56c7b286122a747a9a28d4c30ac430cc774", Prefixes: []string{"attendance"}},
		{ID: "conference", Name: "会议", Endpoint: "https://mcp-gw.dingtalk.com/server/8bdad53455a0f4fe88bda77ecca5147d12d1bdef1cf6aa8ed41e01d8111d5d9b", Prefixes: []string{"conference", "meeting"}},
		{ID: "live", Name: "直播", Endpoint: "https://mcp-gw.dingtalk.com/server/57fb66889659c56df8f0a1838026b9004d8f4257fbef54fd04a028426253a033", Prefixes: []string{"live"}},
		{ID: "aiapp", Name: "AI 应用", Endpoint: "https://mcp-gw.dingtalk.com/server/d5ea9e57768bd9b8c44bca271e4109fd1bc45ef5449dc66c2b8682bc25ba75f8", Prefixes: []string{"aiapp"}},
		{ID: "minutes", Name: "AI 听记", Endpoint: "https://mcp-gw.dingtalk.com/server/1e798e16a79e82eb7933050fbd58ed3ba8934170efb7c92565e39a1fb1c888e1", Prefixes: []string{"minutes"}},
		{ID: "aisearch", Name: "AI 搜问", Endpoint: "https://mcp-gw.dingtalk.com/server/ai-search", Prefixes: []string{"aisearch", "enterprise"}},
		{ID: "report", Name: "钉钉日志", Endpoint: "https://mcp-gw.dingtalk.com/server/01d5a7b815babb03626bf3e505bad4c1e36ecf66876eaf6a7a466d9d5ccc9900", Prefixes: []string{"report"}},
		{ID: "drive", Name: "钉盘", Endpoint: "https://mcp-gw.dingtalk.com/server/536f3b329ee774322b14361c666d6e9471e5bbb281b91ded8ca033b3ce7189af", Prefixes: []string{"drive"}},
		{ID: "blackboard", Name: "工作台公告", Endpoint: "https://mcp-gw.dingtalk.com/server/6d1206b4978544ad057151a9cb2d31569b251b79bbc99c34a4bb990b102b5654", Prefixes: []string{"blackboard"}},
		{ID: "sheet", Name: "钉钉表格", Endpoint: "https://mcp-gw.dingtalk.com/server/f7340bef5170f3baf97815989fc2bff68f4c293be82b2106c5d3e9cbbb14a17f", Prefixes: []string{"sheet"}},
		{ID: "wiki", Name: "知识库", Endpoint: "https://mcp-gw.dingtalk.com/server/wiki", Prefixes: []string{"wiki"}},
		{ID: "ai-sincere-hire", Name: "AI诚聘", Endpoint: "https://mcp-gw.dingtalk.com/server/fae626e7b5ea5902defd23aee6563bd18816943ec97e741c59d5db3c73a4a05c", Prefixes: []string{"ai-sincere-hire"}},
		{ID: "discovery", Name: "发现", Endpoint: "https://mcp-gw.dingtalk.com/server/0258710af6719df6ae3ef1f6a3f7eaab6002dcceb213ead7cc5d038b6d41ea13", Prefixes: []string{"discovery"}},
		{ID: "yida", Name: "宜搭", Endpoint: "https://mcp-gw.dingtalk.com/server/9fb0c65384a256e4f7ef7e640c6a09ed87f2f2e53684f3201e748f9fcf991b1b", Prefixes: []string{"yida"}},
		{ID: "pat", Name: "行为授权", Endpoint: "https://mcp-gw.dingtalk.com/server/abc3c880fb90f04b52d1426aaf093766e5fc9ec38411688cbb74df42a584d374", Prefixes: []string{"pat"}},
		{ID: "mcp-meta", Name: "MCP 元服务", Endpoint: "https://mcp-gw.dingtalk.com/server/89833ea5debf30c260a07ffcb5127ffa3bf0c830cd76babadb293d9861485d44"},
	}
}
