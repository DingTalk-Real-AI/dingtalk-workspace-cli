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
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/spf13/cobra"
)

func init() {
	RegisterPublic(func() Handler { return minutesHandler{} })
}

// minutesHandler contributes the hardcoded `minutes list mine|shared|all`
// leaves. wukong lists minutes through a single tool
// (list_by_keyword_and_time_range) distinguished by belongingConditionId, and
// renames the raw MCP response fields (minutesDetails -> itemList,
// hasNext -> hasMore) for a stable CLI contract. The envelope/discovery layer
// maps these to older split tools and its outputFormat.rename cannot reach the
// nested result, so the canonical behaviour lives here. MergeCommandTree folds
// these leaves into the envelope-driven minutes tree.
type minutesHandler struct{}

func (minutesHandler) Name() string { return "minutes" }

func (minutesHandler) Command(runner executor.Runner) *cobra.Command {
	root := &cobra.Command{
		Use:               "minutes",
		Short:             i18n.T("听记"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE:              func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	list := &cobra.Command{
		Use:               "list",
		Short:             i18n.T("听记列表"),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE:              func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	list.AddCommand(
		newMinutesListCommand(runner, "mine", "created", i18n.T("查询我创建的听记列表")),
		newMinutesListCommand(runner, "shared", "shared", i18n.T("查询他人共享给我的听记列表")),
		newMinutesListCommand(runner, "all", "noLimit", i18n.T("查询我有权限访问的所有听记列表")),
	)
	root.AddCommand(list)
	return root
}

func newMinutesListCommand(runner executor.Runner, use, belonging, short string) *cobra.Command {
	cmd := &cobra.Command{
		Use:               use,
		Short:             short,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{"belongingConditionId": belonging}
			if v := strings.TrimSpace(firstNonEmptyFlag(cmd, "limit", "max")); v != "" {
				if n, err := strconv.ParseFloat(v, 64); err == nil {
					params["maxResults"] = n
				}
			}
			if v := strings.TrimSpace(firstNonEmptyFlag(cmd, "query", "keyword")); v != "" {
				params["keyword"] = v
			}
			if v := strings.TrimSpace(firstNonEmptyFlag(cmd, "cursor", "next-token", "offset")); v != "" {
				params["nextToken"] = v
			}
			if v := strings.TrimSpace(firstNonEmptyFlag(cmd, "start")); v != "" {
				ms, err := parseISOToMillis("start", v)
				if err != nil {
					return err
				}
				params["createTimeStart"] = float64(ms)
			}
			if v := strings.TrimSpace(firstNonEmptyFlag(cmd, "end")); v != "" {
				ms, err := parseISOToMillis("end", v)
				if err != nil {
					return err
				}
				params["createTimeEnd"] = float64(ms)
			}
			inv := executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "minutes", "list_by_keyword_and_time_range", params,
			)
			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, inv)
			}
			result, err := runner.Run(cmd.Context(), inv)
			if err != nil {
				return err
			}
			renameMinutesListFields(result.Response)
			return writeCommandPayload(cmd, result)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("limit", "", i18n.T("每页返回数量 (可选)"))
	cmd.Flags().String("max", "", i18n.T("--limit 的别名"))
	cmd.Flags().String("cursor", "", i18n.T("分页游标 (可选)"))
	cmd.Flags().String("query", "", i18n.T("关键字筛选 (可选)"))
	cmd.Flags().String("start", "", i18n.T("起始时间 ISO-8601 (可选)"))
	cmd.Flags().String("end", "", i18n.T("结束时间 ISO-8601 (可选)"))
	return cmd
}

// renameMinutesListFields renames the raw list fields inside the MCP response
// content (content.result.{minutesDetails->itemList, hasNext->hasMore}) so the
// CLI contract matches wukong. Safe no-op when the shape differs.
func renameMinutesListFields(resp map[string]any) {
	if resp == nil {
		return
	}
	content, ok := resp["content"].(map[string]any)
	if !ok {
		return
	}
	target := content
	if inner, ok := content["result"].(map[string]any); ok {
		target = inner
	}
	if v, ok := target["minutesDetails"]; ok {
		target["itemList"] = v
		delete(target, "minutesDetails")
	}
	if v, ok := target["hasNext"]; ok {
		target["hasMore"] = v
		delete(target, "hasNext")
	}
}

func parseISOToMillis(label, s string) (int64, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UnixMilli(), nil
		}
	}
	return 0, apperrors.NewValidation("--" + label + " 时间格式无效，请用 ISO-8601 (如 2026-03-10T14:00:00+08:00)")
}
