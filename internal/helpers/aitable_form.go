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
	"strconv"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/spf13/cobra"
)

func newAitableFormCommand(runner executor.Runner) *cobra.Command {
	form := newAitableFormGroup("form", "表单管理")
	form.Hidden = true
	field := newAitableFormGroup("field", "表单字段管理")
	share := newAitableFormGroup("share", "表单分享管理")
	questions := newAitableFormGroup("questions", "表单题目管理（等价于 field create / delete）")

	field.AddCommand(
		newAitableFormFieldListCommand(runner),
		newAitableFormFieldUpdateCommand(runner),
		newAitableFormFieldHideCommand(runner),
	)
	share.AddCommand(
		newAitableFormShareGetCommand(runner),
		newAitableFormShareUpdateCommand(runner),
	)
	questions.AddCommand(
		newAitableFormQuestionsCreateCommand(runner),
		newAitableFormQuestionsDeleteCommand(runner),
	)
	form.AddCommand(
		newAitableFormListCommand(runner),
		newAitableFormGetCommand(runner),
		newAitableFormCreateCommand(runner),
		newAitableFormUpdateCommand(runner),
		newAitableFormDeleteCommand(runner),
		field,
		share,
		questions,
	)
	return form
}

func newAitableFormGroup(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:               use,
		Short:             i18n.T(short),
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
}

func newAitableFormListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             i18n.T("列出表单视图"),
		Example:           "  dws aitable form list --base-id BASE_ID --table-id TABLE_ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, err := requiredAitableBaseTable(cmd)
			if err != nil {
				return err
			}
			return runAitableFormTool(cmd, runner, "list_form_views", map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableFlags(cmd)
	return cmd
}

func newAitableFormGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             i18n.T("获取单个表单视图详情"),
		Example:           "  dws aitable form get --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, err := requiredAitableBaseTable(cmd)
			if err != nil {
				return err
			}
			viewID, err := aitableRequiredFlag(cmd, "view-id")
			if err != nil {
				return err
			}
			return runAitableFormTool(cmd, runner, "list_form_views", map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
				"viewIds": []string{viewID},
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableFlags(cmd)
	cmd.Flags().String("view-id", "", i18n.T("View ID (必填)"))
	return cmd
}

func newAitableFormCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create",
		Short:             i18n.T("创建表单视图"),
		Example:           "  dws aitable form create --base-id BASE_ID --table-id TABLE_ID --name 员工信息收集",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, err := requiredAitableBaseTable(cmd)
			if err != nil {
				return err
			}
			name, err := aitableRequiredFlag(cmd, "name")
			if err != nil {
				return err
			}
			params := map[string]any{
				"baseId":   baseID,
				"tableId":  tableID,
				"viewType": "FormDesigner",
				"viewName": name,
			}
			// create_view does not currently declare a description parameter.
			// Keep the flag for Wukong CLI compatibility, but do not send it.
			return runAitableTool(cmd, runner, "create_view", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableFlags(cmd)
	cmd.Flags().String("name", "", i18n.T("表单名称 (必填)"))
	cmd.Flags().String("description", "", i18n.T("表单描述（兼容保留）"))
	return cmd
}

func newAitableFormUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "update",
		Short:             i18n.T("更新表单配置"),
		Example:           "  dws aitable form update --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --title 新标题",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, err := requiredAitableBaseTable(cmd)
			if err != nil {
				return err
			}
			viewID, err := aitableRequiredFlag(cmd, "view-id")
			if err != nil {
				return err
			}
			title := aitableFlagOrFallback(cmd, "title", "name")
			description := aitableStringFlag(cmd, "description")
			if title == "" && description == "" {
				return apperrors.NewValidation("--title (or --name) and --description must specify at least one")
			}
			params := map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
				"viewId":  viewID,
			}
			if title != "" {
				params["title"] = title
			}
			if description != "" {
				params["description"] = description
			}
			return runAitableFormTool(cmd, runner, "update_form_info", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableFlags(cmd)
	cmd.Flags().String("view-id", "", i18n.T("View ID (必填)"))
	cmd.Flags().String("title", "", i18n.T("表单标题"))
	cmd.Flags().String("name", "", i18n.T("--title 的别名"))
	cmd.Flags().String("description", "", i18n.T("表单描述"))
	return cmd
}

func newAitableFormDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "delete",
		Short:             i18n.T("删除表单"),
		Example:           "  dws aitable form delete --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --yes",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, err := requiredAitableBaseTable(cmd)
			if err != nil {
				return err
			}
			viewID, err := aitableRequiredFlag(cmd, "view-id")
			if err != nil {
				return err
			}
			if !confirmDeletePrompt(cmd, i18n.T("表单"), viewID) {
				return nil
			}
			return runAitableFormTool(cmd, runner, "delete_form_view", map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
				"viewId":  viewID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableFlags(cmd)
	cmd.Flags().String("view-id", "", i18n.T("View ID (必填)"))
	return cmd
}

func newAitableFormQuestionsCreateCommand(runner executor.Runner) *cobra.Command {
	cmd := newAitableFieldCreateCommand(runner)
	cmd.Use = "create"
	cmd.Short = i18n.T("向表单添加题目（等价于 field create）")
	cmd.Example = "  dws aitable form questions create --base-id BASE_ID --table-id TABLE_ID --name 电话 --type text"
	return cmd
}

func newAitableFormQuestionsDeleteCommand(runner executor.Runner) *cobra.Command {
	cmd := newAitableFieldDeleteCommand(runner)
	cmd.Use = "delete"
	cmd.Short = i18n.T("从表单删除题目（等价于 field delete）")
	cmd.Example = "  dws aitable form questions delete --base-id BASE_ID --table-id TABLE_ID --field-id FIELD_ID --yes"
	return cmd
}

func newAitableFormFieldListCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "list",
		Short:             i18n.T("列出表单字段"),
		Example:           "  dws aitable form field list --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, viewID, err := requiredAitableBaseTableView(cmd)
			if err != nil {
				return err
			}
			return runAitableFormTool(cmd, runner, "list_form_fields", map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
				"viewId":  viewID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableViewFlags(cmd)
	return cmd
}

func newAitableFormFieldUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "update",
		Short:             i18n.T("更新表单字段"),
		Example:           "  dws aitable form field update --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --field-id FIELD_ID --required true",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, viewID, err := requiredAitableBaseTableView(cmd)
			if err != nil {
				return err
			}
			fieldID, err := aitableRequiredFlag(cmd, "field-id")
			if err != nil {
				return err
			}
			params := map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
				"viewId":  viewID,
				"fieldId": fieldID,
			}
			if required, ok, err := optionalAitableBoolStringFlag(cmd, "required"); err != nil {
				return err
			} else if ok {
				params["required"] = required
			}
			if description := aitableStringFlag(cmd, "field-description"); description != "" {
				params["fieldDescription"] = description
			}
			if _, hasRequired := params["required"]; !hasRequired {
				if _, hasDescription := params["fieldDescription"]; !hasDescription {
					return apperrors.NewValidation("at least one of --required or --field-description is required")
				}
			}
			return runAitableFormTool(cmd, runner, "update_form_field", params)
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableViewFlags(cmd)
	cmd.Flags().String("field-id", "", i18n.T("Field ID (必填)"))
	cmd.Flags().String("required", "", i18n.T("是否必填: true/false"))
	cmd.Flags().String("field-description", "", i18n.T("字段描述"))
	return cmd
}

func newAitableFormFieldHideCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "hide",
		Short:             i18n.T("切换表单字段隐藏"),
		Example:           "  dws aitable form field hide --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --field-id FIELD_ID --hidden true",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, viewID, err := requiredAitableBaseTableView(cmd)
			if err != nil {
				return err
			}
			fieldID, err := aitableRequiredFlag(cmd, "field-id")
			if err != nil {
				return err
			}
			hidden, err := requiredAitableBoolStringFlag(cmd, "hidden")
			if err != nil {
				return err
			}
			return runAitableFormTool(cmd, runner, "update_form_field_hidden", map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
				"viewId":  viewID,
				"fieldId": fieldID,
				"hidden":  hidden,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableViewFlags(cmd)
	cmd.Flags().String("field-id", "", i18n.T("Field ID (必填)"))
	cmd.Flags().String("hidden", "", i18n.T("是否隐藏: true/false (必填)"))
	return cmd
}

func newAitableFormShareGetCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "get",
		Short:             i18n.T("获取表单分享配置"),
		Example:           "  dws aitable form share get --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, viewID, err := requiredAitableBaseTableView(cmd)
			if err != nil {
				return err
			}
			return runAitableFormTool(cmd, runner, "get_share_form_config", map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
				"viewId":  viewID,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableViewFlags(cmd)
	return cmd
}

func newAitableFormShareUpdateCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "update",
		Short:             i18n.T("开启/关闭分享表单"),
		Example:           "  dws aitable form share update --base-id BASE_ID --table-id TABLE_ID --view-id VIEW_ID --enabled true",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			baseID, tableID, viewID, err := requiredAitableBaseTableView(cmd)
			if err != nil {
				return err
			}
			enabledRaw, err := requiredAitableBoolStringFlagRaw(cmd, "enabled")
			if err != nil {
				return err
			}
			return runAitableFormTool(cmd, runner, "update_share_form", map[string]any{
				"baseId":  baseID,
				"tableId": tableID,
				"viewId":  viewID,
				"enabled": enabledRaw,
			})
		},
	}
	preferLegacyLeaf(cmd)
	addAitableBaseTableViewFlags(cmd)
	cmd.Flags().String("enabled", "", i18n.T("是否开启分享: true/false (必填)"))
	return cmd
}

func addAitableBaseTableFlags(cmd *cobra.Command) {
	cmd.Flags().String("base-id", "", i18n.T("Base ID (必填)"))
	addAitableHiddenStringFlag(cmd, "base", "--base-id 的兼容别名")
	cmd.Flags().String("table-id", "", i18n.T("Table ID (必填)"))
}

func addAitableBaseTableViewFlags(cmd *cobra.Command) {
	addAitableBaseTableFlags(cmd)
	cmd.Flags().String("view-id", "", i18n.T("View ID (必填)"))
}

func requiredAitableBaseTable(cmd *cobra.Command) (string, string, error) {
	baseID, err := aitableRequiredFlagOrFallback(cmd, "base-id", "base")
	if err != nil {
		return "", "", err
	}
	tableID, err := aitableRequiredFlag(cmd, "table-id")
	if err != nil {
		return "", "", err
	}
	return baseID, tableID, nil
}

func requiredAitableBaseTableView(cmd *cobra.Command) (string, string, string, error) {
	baseID, tableID, err := requiredAitableBaseTable(cmd)
	if err != nil {
		return "", "", "", err
	}
	viewID, err := aitableRequiredFlag(cmd, "view-id")
	if err != nil {
		return "", "", "", err
	}
	return baseID, tableID, viewID, nil
}

func optionalAitableBoolStringFlag(cmd *cobra.Command, name string) (bool, bool, error) {
	raw := aitableStringFlag(cmd, name)
	if raw == "" {
		return false, false, nil
	}
	value, err := parseAitableBoolString(raw, name)
	if err != nil {
		return false, false, err
	}
	return value, true, nil
}

func requiredAitableBoolStringFlag(cmd *cobra.Command, name string) (bool, error) {
	raw, err := requiredAitableBoolStringFlagRaw(cmd, name)
	if err != nil {
		return false, err
	}
	return parseAitableBoolString(raw, name)
}

func requiredAitableBoolStringFlagRaw(cmd *cobra.Command, name string) (string, error) {
	raw := aitableStringFlag(cmd, name)
	if raw == "" {
		return "", apperrors.NewValidation(fmt.Sprintf("--%s is required", name))
	}
	if _, err := parseAitableBoolString(raw, name); err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimSpace(raw)), nil
}

func parseAitableBoolString(raw, name string) (bool, error) {
	value, err := strconv.ParseBool(strings.ToLower(strings.TrimSpace(raw)))
	if err != nil {
		return false, apperrors.NewValidation(fmt.Sprintf("--%s must be true or false", name))
	}
	return value, nil
}
