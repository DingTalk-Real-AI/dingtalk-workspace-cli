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
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

type wikiProxyTarget int

const (
	wikiProxyTargetSpace wikiProxyTarget = iota
	wikiProxyTargetDoc
)

type wikiProxyOptions struct {
	workspaceToWorkspaceIDs bool
}

func addWikiProxyCommands(root *cobra.Command, runner executor.Runner) {
	root.AddCommand(
		newWikiProxyLeaf(runner, "list", wikiProxyTargetSpace, []string{"space", "list"}, wikiProxyOptions{}),
		newWikiProxyLeaf(runner, "search", wikiProxyTargetSpace, []string{"space", "search"}, wikiProxyOptions{}),
		newWikiProxyLeaf(runner, "create", wikiProxyTargetSpace, []string{"space", "create"}, wikiProxyOptions{}),
		newWikiProxyLeaf(runner, "get", wikiProxyTargetSpace, []string{"space", "get"}, wikiProxyOptions{}),
	)

	node := newWikiProxyGroup("node", "知识库节点兼容入口")
	node.AddCommand(
		newWikiProxyLeaf(runner, "list", wikiProxyTargetDoc, []string{"list"}, wikiProxyOptions{}),
		newWikiProxyLeaf(runner, "read", wikiProxyTargetDoc, []string{"read"}, wikiProxyOptions{}),
		newWikiProxyLeaf(runner, "info", wikiProxyTargetDoc, []string{"info"}, wikiProxyOptions{}),
		newWikiProxyLeaf(runner, "create", wikiProxyTargetDoc, []string{"create"}, wikiProxyOptions{}),
		newWikiProxyLeaf(runner, "search", wikiProxyTargetDoc, []string{"search"}, wikiProxyOptions{workspaceToWorkspaceIDs: true}),
	)

	file := newWikiProxyGroup("file", "知识库文件兼容入口")
	file.AddCommand(
		newWikiProxyLeaf(runner, "list", wikiProxyTargetDoc, []string{"list"}, wikiProxyOptions{}),
		newWikiProxyLeaf(runner, "search", wikiProxyTargetDoc, []string{"search"}, wikiProxyOptions{workspaceToWorkspaceIDs: true}),
	)

	doc := newWikiProxyGroup("doc", "知识库文档兼容入口")
	doc.AddCommand(
		newWikiProxyLeaf(runner, "list", wikiProxyTargetDoc, []string{"list"}, wikiProxyOptions{}),
		newWikiProxyLeaf(runner, "read", wikiProxyTargetDoc, []string{"read"}, wikiProxyOptions{}),
		newWikiProxyLeaf(runner, "search", wikiProxyTargetDoc, []string{"search"}, wikiProxyOptions{workspaceToWorkspaceIDs: true}),
	)

	root.AddCommand(node, file, doc)
}

func newWikiProxyGroup(use, short string) *cobra.Command {
	cmd := &cobra.Command{
		Use:               use,
		Short:             short,
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	preferLegacyLeaf(cmd)
	return cmd
}

func newWikiProxyLeaf(runner executor.Runner, use string, target wikiProxyTarget, targetPath []string, opts wikiProxyOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:                use,
		Short:              "兼容入口，透明转发到新命令",
		DisableFlagParsing: true,
		DisableAutoGenTag:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			forwardArgs := append([]string{}, targetPath...)
			forwardArgs = append(forwardArgs, rewriteWikiProxyArgs(args, opts)...)

			fmt.Fprintf(cmd.ErrOrStderr(), "redirecting to: dws %s\n", wikiProxyDisplayPath(target, targetPath))
			return executeWikiProxyTarget(cmd, runner, target, forwardArgs)
		},
	}
	preferLegacyLeaf(cmd)
	return cmd
}

func wikiProxyDisplayPath(target wikiProxyTarget, targetPath []string) string {
	prefix := "doc"
	if target == wikiProxyTargetSpace {
		prefix = "wiki"
	}
	parts := append([]string{prefix}, targetPath...)
	return strings.Join(parts, " ")
}

func rewriteWikiProxyArgs(args []string, opts wikiProxyOptions) []string {
	if !opts.workspaceToWorkspaceIDs {
		return append([]string{}, args...)
	}

	out := make([]string, 0, len(args))
	for _, arg := range args {
		switch {
		case arg == "--workspace":
			out = append(out, "--workspace-ids")
		case strings.HasPrefix(arg, "--workspace="):
			out = append(out, "--workspace-ids="+strings.TrimPrefix(arg, "--workspace="))
		default:
			out = append(out, arg)
		}
	}
	return out
}

func executeWikiProxyTarget(source *cobra.Command, runner executor.Runner, target wikiProxyTarget, args []string) error {
	var root *cobra.Command
	switch target {
	case wikiProxyTargetSpace:
		root = newWikiProxySpaceTargetRoot(runner)
	case wikiProxyTargetDoc:
		root = docHandler{}.Command(runner)
	default:
		return fmt.Errorf("unknown wiki proxy target: %d", target)
	}

	configureWikiProxyTargetRoot(source, root)
	root.SetArgs(args)
	return root.Execute()
}

func newWikiProxySpaceTargetRoot(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "wiki",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
	}
	space := &cobra.Command{
		Use:               "space",
		Short:             "知识库空间",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	space.AddCommand(
		newWikiSpaceCreateCommand(runner),
		newWikiSpaceGetCommand(runner),
		newWikiSpaceListCommand(runner),
		newWikiSpaceSearchCommand(runner),
	)
	root.AddCommand(space)
	return root
}

func configureWikiProxyTargetRoot(source, root *cobra.Command) {
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.SetOut(source.OutOrStdout())
	root.SetErr(source.ErrOrStderr())
	root.SetIn(source.InOrStdin())
	root.SetContext(source.Context())
	if root.PersistentFlags().Lookup("format") == nil {
		root.PersistentFlags().StringP("format", "f", "json", "输出格式: json|table|raw|pretty|ndjson|csv")
	}
	if root.PersistentFlags().Lookup("dry-run") == nil {
		root.PersistentFlags().Bool("dry-run", false, "仅打印即将发送的请求，不真正执行")
	}
}
