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

宿主接管 PAT 只由 CLAW_TYPE 选择。支持值：
  host-control
  rewind-desktop
  dws-wukong
  wukong

DWS_CHANNEL 只用于上游 channelCode，
不提供 PAT 宿主接管的任何回退路径。`,
		RunE: cmdutil.GroupRunE,
	}

	patCmd.AddCommand(chmodCmd)
	patCmd.AddCommand(callbackCmd)
	root.AddCommand(patCmd)
}
