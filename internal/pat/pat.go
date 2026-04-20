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

// Package pat implements the "dws pat" command group for PAT (Personal Action
// Token) authorization management.
package pat

import (
	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

var caller edition.ToolCaller

// RegisterCommands adds the pat command tree to rootCmd.
func RegisterCommands(root *cobra.Command, c edition.ToolCaller) {
	caller = c
	patCmd := &cobra.Command{
		Use:   "pat",
		Short: "行为授权管理",
		Long: `管理行为授权（PAT）。

命令结构:
  dws pat chmod     <scope>...   授予指定权限
  dws pat callback  <command>    宿主 / Agent PAT 接管模式的回调接口

第三方业务开发者通过 DINGTALK_AGENT 指定自己的业务 Agent 名称。
生效请求头固定为：
  claw-type: <business-agent-name 或 default>

当 DINGTALK_AGENT 为空或为 default 时，走默认 DWS 行为。
当 claw-type != default 且命中 PAT 时，PAT 返回 JSON，
由宿主处理全部 UI / 交互 / 回调节奏 / 重试逻辑。

DWS_CHANNEL 只用于上游 channelCode。`,
		RunE: cmdutil.GroupRunE,
	}

	patCmd.AddCommand(chmodCmd)
	patCmd.AddCommand(callbackCmd)
	root.AddCommand(patCmd)
}
