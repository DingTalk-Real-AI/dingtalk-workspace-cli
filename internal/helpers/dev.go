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

package helpers

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

func init() {
	RegisterPublic(func() Handler {
		return devHandler{}
	})
}

// devHandler mounts the developer umbrella command `dws dev`, grouping the
// three developer-facing pillars under one entry:
//
//	dev app      application lifecycle (CRUD / credentials / permission /
//	             member / security / webapp / robot / version)
//	dev connect  link an existing robot to the local agent for debugging
//	             (promoted out of the robot subtree: it is a local-dev entry,
//	             not a robot configuration)
//	dev doc      open-platform developer doc search (bridges the devdoc
//	             product; `dws devdoc` keeps working independently)
//
// Future developer capabilities (event subscription, MCP tooling, ...) join
// as new subtrees here instead of new top-level commands.
type devHandler struct{}

func (devHandler) Name() string {
	return "dev"
}

func (devHandler) Command(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "dev",
		Short:             "开放平台开发者能力",
		Long:              "钉钉开放平台开发者命令组：应用生命周期管理（app）、机器人本地调试建联（connect）、开发文档搜索（doc）。",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	doc := &cobra.Command{
		Use:               "doc",
		Short:             "开放平台文档搜索",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	doc.AddCommand(newDevdocArticleSearchCommand(runner))

	root.AddCommand(
		newDevAppCommand(runner),
		newDevAppRobotConnectCommand(runner),
		doc,
	)
	return root
}
