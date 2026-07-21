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

	"github.com/spf13/cobra"
)

type versionCommandConfig struct {
	groupShort   string
	groupLong    string
	resourceName string
	placeholder  string
}

func newDocVersionCmd() *cobra.Command {
	return newVersionCommand(versionCommandConfig{
		groupShort:   "文档历史版本管理",
		groupLong:    "管理钉钉在线文档（adoc）的历史版本：手动保存、查看版本列表、回滚到指定版本。",
		resourceName: "文档",
		placeholder:  "DOC_ID",
	})
}

func newSheetVersionCmd() *cobra.Command {
	command := newVersionCommand(versionCommandConfig{
		groupShort:   "表格历史版本管理",
		groupLong:    "管理钉钉在线电子表格的历史版本：手动保存、查看版本列表、回滚到指定版本。",
		resourceName: "表格",
		placeholder:  "SHEET_ID",
	})
	revertCommand, remaining, err := command.Find([]string{"revert"})
	if err != nil || revertCommand == nil || len(remaining) != 0 || revertCommand == command {
		panic(fmt.Sprintf("attach Sheet version revert guard: command not found (remaining=%v, err=%v)", remaining, err))
	}
	protectSheetMutationCommand(revertCommand, "回滚表格版本", "表格、目标版本及受影响内容")
	return command
}

func newVersionCommand(config versionCommandConfig) *cobra.Command {
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: config.groupShort,
		Long:  config.groupLong,
		RunE:  groupRunE,
	}

	saveCmd := &cobra.Command{
		Use:     "save",
		Short:   fmt.Sprintf("手动保存%s版本快照", config.resourceName),
		Example: fmt.Sprintf("  dws %s version save --node %s", versionCommandProduct(config), config.placeholder),
		RunE: func(cmd *cobra.Command, _ []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			return callMCPToolOnServer("doc", "save_doc_version", map[string]any{"nodeId": nodeID})
		},
	}
	saveCmd.Flags().String("node", "", fmt.Sprintf("%s ID 或 URL (必填)", config.resourceName))

	listCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   fmt.Sprintf("查看%s历史版本列表", config.resourceName),
		Example: fmt.Sprintf("  dws %s version list --node %s\n  dws %s version list --node %s --limit 10", versionCommandProduct(config), config.placeholder, versionCommandProduct(config), config.placeholder),
		RunE: func(cmd *cobra.Command, _ []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			toolArgs := map[string]any{"nodeId": nodeID}
			if value, _ := cmd.Flags().GetInt("limit"); value > 0 {
				toolArgs["maxResults"] = value
			}
			if value := flagOrFallback(cmd, "cursor", "page-token", "next-token"); value != "" {
				toolArgs["nextCursor"] = value
			}
			return callMCPToolOnServer("doc", "list_doc_versions", toolArgs)
		},
	}
	listCmd.Flags().String("node", "", fmt.Sprintf("%s ID 或 URL (必填)", config.resourceName))
	listCmd.Flags().Int("limit", 0, "返回版本数量上限")
	listCmd.Flags().String("cursor", "", "分页游标")

	revertCmd := &cobra.Command{
		Use:     "revert",
		Short:   fmt.Sprintf("回滚%s到指定版本", config.resourceName),
		Example: fmt.Sprintf("  dws %s version revert --node %s --version 3 --yes", versionCommandProduct(config), config.placeholder),
		RunE: func(cmd *cobra.Command, _ []string) error {
			nodeID, err := mustFlagOrFallback(cmd, "node", "url", "id", "node-id", "doc-id", "file-id")
			if err != nil {
				return err
			}
			if !cmd.Flags().Changed("version") {
				return fmt.Errorf("flag --version is required")
			}
			version, _ := cmd.Flags().GetInt("version")
			if version <= 0 {
				return fmt.Errorf("--version must be a positive integer")
			}
			// Dry-run must remain offline and preview only the final mutation.
			// Real execution keeps the defensive version-existence preflight.
			if deps == nil || deps.Caller == nil || !deps.Caller.DryRun() {
				exists, err := docVersionExists(cmd.Context(), nodeID, version)
				if err != nil {
					return err
				}
				if !exists {
					return fmt.Errorf("%s版本 %d 不存在；请先执行 dws %s version list --node %s --format json", config.resourceName, version, versionCommandProduct(config), nodeID)
				}
			}
			// Sheet uses the structural confirmation guard installed by
			// newSheetVersionCmd so Schema and runtime stay in lockstep.
			if config.resourceName != "表格" && !confirmDangerousAction(cmd, fmt.Sprintf("revert %s to version %d", versionCommandResourceEnglish(config), version), nodeID) {
				return nil
			}
			return callMCPToolOnServer("doc", "revert_doc_version", map[string]any{
				"nodeId":  nodeID,
				"version": version,
			})
		},
	}
	revertCmd.Flags().String("node", "", fmt.Sprintf("%s ID 或 URL (必填)", config.resourceName))
	revertCmd.Flags().Int("version", 0, "目标版本号 (必填，从 list 获取)")

	for _, command := range []*cobra.Command{saveCmd, listCmd, revertCmd} {
		addHiddenNodeAliases(command)
	}
	versionCmd.AddCommand(saveCmd, listCmd, revertCmd)
	return versionCmd
}

func versionCommandProduct(config versionCommandConfig) string {
	if config.resourceName == "表格" {
		return "sheet"
	}
	return "doc"
}

func versionCommandResourceEnglish(config versionCommandConfig) string {
	if config.resourceName == "表格" {
		return "sheet"
	}
	return "document"
}
