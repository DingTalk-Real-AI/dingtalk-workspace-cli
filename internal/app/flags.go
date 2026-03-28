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

package app

import (
	"github.com/spf13/cobra"
)

// GlobalFlags contains the root-level persistent flags shared across the CLI.
type GlobalFlags struct {
	ClientID     string
	ClientSecret string
	Debug        bool
	DryRun       bool
	Format       string
	Mock         bool
	Output       string
	Timeout      int
	Token        string
	Verbose      bool
	Yes          bool
}

func bindPersistentFlags(cmd *cobra.Command, flags *GlobalFlags) {
	cmd.PersistentFlags().StringVar(&flags.ClientID, "client-id", "", "Override OAuth client ID (DingTalk AppKey)")
	cmd.PersistentFlags().StringVar(&flags.ClientSecret, "client-secret", "", "Override OAuth client secret (DingTalk AppSecret)")
	cmd.PersistentFlags().BoolVar(&flags.Debug, "debug", false, "显示调试日志")
	cmd.PersistentFlags().BoolVar(&flags.DryRun, "dry-run", false, "预览操作内容，不实际执行")
	cmd.PersistentFlags().StringVarP(&flags.Format, "format", "f", "json", "输出格式: json|table|raw")
	cmd.PersistentFlags().BoolVar(&flags.Mock, "mock", false, "使用 Mock 数据 (开发调试用)")
	cmd.PersistentFlags().StringVarP(&flags.Output, "output", "o", "", "Write command output to a file")
	_ = cmd.PersistentFlags().MarkHidden("output")
	cmd.PersistentFlags().IntVar(&flags.Timeout, "timeout", 30, "HTTP 请求超时时间 (秒)")
	cmd.PersistentFlags().StringVar(&flags.Token, "token", "", "Override the configured API token")
	_ = cmd.PersistentFlags().MarkHidden("token")
	cmd.PersistentFlags().BoolVarP(&flags.Verbose, "verbose", "v", false, "显示详细日志")
	cmd.PersistentFlags().BoolVarP(&flags.Yes, "yes", "y", false, "跳过确认提示 (AI Agent 模式)")
}
