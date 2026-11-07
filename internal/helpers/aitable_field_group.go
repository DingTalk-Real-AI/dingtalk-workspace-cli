// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

func newAitableFieldGroupCommand() *cobra.Command {
	group := newGroupCommand(&cobra.Command{Use: "field-group", Short: "字段编组管理", Long: "管理数据表的列编组；取消编组保留字段和数据。普通列遵循当前视图顺序与隐藏规则，不创建未分组分组。", RunE: groupRunE})
	base := aitableAppBaseIDFlag()
	table := LeafFlag{Name: "table-id", Bind: "tableId", Required: true, Trim: true, Usage: "所属数据表 ID"}
	groupID := LeafFlag{Name: "group-id", Bind: "groupId", Required: true, Trim: true, Usage: "字段组 ID"}
	name := LeafFlag{Name: "name", Bind: "name", Usage: "字段组名称；省略保持原值，显式传入时必须为非空白字符串", OmitEmpty: true}
	description := LeafFlag{Name: "description", Bind: "description", Usage: "字段组说明；显式空字符串清除，省略保持原值", OmitEmpty: true}
	commonParams := []contract.ParamDecl{aitableAppParam("base-id", "baseId", "string", true), aitableAppParam("table-id", "tableId", "string", true)}
	for _, entry := range []struct {
		use, tool, title, detail, positive, negative, example string
		flags                                                 []LeafFlag
		params                                                []contract.ParamDecl
		safety                                                contract.SafetySpec
	}{
		{use: "list", tool: "list_field_groups", title: "查询字段编组", detail: "查询数据表字段组及组内字段 ID；普通列仍按视图列顺序和隐藏规则展示。", positive: "需要读取数据表列编组，或只读核对编组写入状态时", negative: "按记录值分组用视图配置；读取字段配置用 field get", example: "dws aitable field-group list --base-id B --table-id T", safety: aitableSafetyRead()},
		{use: "create", tool: "create_field_group", title: "创建字段编组", detail: "为指定字段创建列编组；若移动字段会清空旧组，必须显式 --confirm-empty-group=true，确认取消空旧组；保留字段和数据。", positive: "需要把已有字段建立成新的列编组时", negative: "已有组改名用 field-group update；记录分组不走本入口", example: "dws aitable field-group create --base-id B --table-id T --name 地址 --field-ids F1,F2", flags: []LeafFlag{{Name: "name", Bind: "name", Required: true, Trim: true, Usage: "新字段组名称"}, description, aitableFieldGroupIDsFlag(), aitableFieldGroupConfirmEmptyFlag()}, params: []contract.ParamDecl{aitableAppParam("name", "name", "string", true), aitableAppParam("description", "description", "string", false), aitableAppParam("field-ids", "fieldIds", "array", true), aitableAppParam("confirm-empty-group", "confirmEmptyGroup", "boolean", false)}, safety: aitableImprovementCreateSafety()},
		{use: "update", tool: "update_field_group", title: "更新字段编组名称或说明", detail: "只更新显式传入的名称或说明；--name 显式传入时必须为非空白字符串，--description 空字符串清除说明，未传值保持原值。", positive: "已有字段组需要改名称或说明时", negative: "移入移出字段用 field-group move-fields；取消整组用 field-group ungroup", example: "dws aitable field-group update --base-id B --table-id T --group-id G --name 收件地址", flags: []LeafFlag{groupID, name, description}, params: []contract.ParamDecl{aitableAppParam("group-id", "groupId", "string", true), aitableAppParam("name", "name", "string", false), aitableAppParam("description", "description", "string", false)}, safety: aitableFieldGroupWriteSafety()},
		{use: "ungroup", tool: "ungroup_field_group", title: "取消字段编组并保留字段", detail: "仅取消指定编组，保留所有字段和数据；恢复普通列展示，沿用当前视图列顺序和隐藏规则。", positive: "需要取消整个字段编组并保留组内字段和数据时", negative: "只移出部分字段用 field-group move-fields；删除字段用 field delete", example: "dws aitable field-group ungroup --base-id B --table-id T --group-id G", flags: []LeafFlag{groupID}, params: []contract.ParamDecl{aitableAppParam("group-id", "groupId", "string", true)}, safety: aitableFieldGroupWriteSafety()},
		{use: "move-fields", tool: "move_field_group_fields", title: "将字段移入编组或移出编组", detail: "传 --target-group-id 移入目标组，省略表示移出成为普通列；显式传入时必须为非空白字符串。移空源组必须显式 --confirm-empty-group=true，沿用页面取消空组规则；保留字段和数据，不改变视图顺序或隐藏配置。", positive: "需要移动已有字段到指定编组，或移出成为普通列时", negative: "只改组名用 field-group update；取消整组用 field-group ungroup", example: "dws aitable field-group move-fields --base-id B --table-id T --field-ids F1,F2 --target-group-id G", flags: []LeafFlag{aitableFieldGroupIDsFlag(), {Name: "target-group-id", Bind: "targetGroupId", Trim: true, OmitEmpty: true, Usage: "目标字段组 ID；省略表示移出编组，显式传入时必须为非空白字符串"}, aitableFieldGroupConfirmEmptyFlag()}, params: []contract.ParamDecl{aitableAppParam("field-ids", "fieldIds", "array", true), aitableAppParam("target-group-id", "targetGroupId", "string", false), aitableAppParam("confirm-empty-group", "confirmEmptyGroup", "boolean", false)}, safety: aitableFieldGroupWriteSafety()},
	} {
		params := append(append([]contract.ParamDecl(nil), commonParams...), entry.params...)
		declaration := aitableAppContract("field_group_"+strings.ReplaceAll(entry.use, "-", "_"), "aitable field-group "+entry.use, entry.tool, entry.detail, entry.title, []string{entry.positive}, []string{entry.negative}, []string{entry.example}, params, aitableFieldGroupResultSpec(entry.use != "list"))
		declaration.DryRun = &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false}
		spec := LeafSpec{Use: entry.use, Short: entry.title, Long: entry.detail + "\n写入返回 verificationStatus=pending 时仅表示待核实，使用 field-group list 只读核对，不能重放写入。", Tool: entry.tool, Server: "aitable", Safety: entry.safety, Flags: append([]LeafFlag{base, table}, entry.flags...), OutputRollout: output.RolloutUnifiedActive, ResultCall: callAitableFieldGroupResult, Contract: declaration}
		if entry.use == "move-fields" {
			spec.Validate = validateAitableFieldGroupMoveTarget
			spec.Constraints = []LeafConstraint{{Kind: corecmd.Custom, Flags: []string{"target-group-id"}, Description: "显式传入 --target-group-id 时必须为非空白字符串；省略表示移出编组"}}
		}
		if entry.use == "update" {
			group.AddCommand(newAitableFieldGroupUpdateCommand(spec))
		} else {
			group.AddCommand(NewLeafCommand(spec))
		}
	}
	return group
}

// 省略目标组才表示移出；在 OmitEmpty 装配参数前拒绝显式空白，避免将移入误变为移出。
func validateAitableFieldGroupMoveTarget(cmd *cobra.Command, _ []string) error {
	if !cmd.Flags().Changed("target-group-id") {
		return nil
	}
	target, _ := cmd.Flags().GetString("target-group-id")
	if strings.TrimSpace(target) == "" {
		return apperrors.NewValidation("--target-group-id 显式传入时必须为非空白字符串；移出编组请省略此参数")
	}
	return nil
}

func newAitableFieldGroupUpdateCommand(spec LeafSpec) *cobra.Command {
	// PATCH must distinguish an omitted description from an explicitly empty
	// string. Keep argument assembly in this owning command's RunE, because
	// the generic managed binder cannot express that distinction.
	cmd := &cobra.Command{Use: spec.Use, Short: spec.Short, Long: spec.Long}
	flags := spec.Flags
	corecmd.RegisterFlags(cmd, flags)
	cmd.MarkFlagsOneRequired("name", "description")
	corecmd.AnnotateConstraints(cmd, []corecmd.Constraint{
		{Kind: corecmd.AtLeastOne, Flags: []string{"name", "description"}, PresenceOnly: true},
		{Kind: corecmd.Custom, Flags: []string{"name"}, Description: "显式传入 --name 时必须为非空白字符串；省略保持原值"},
	})
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		args := make(map[string]any, len(flags))
		for _, flag := range flags {
			if flag.Required || cmd.Flags().Changed(flag.Name) {
				args[flag.Bind] = corecmd.EffectiveValue(cmd, flag)
			}
		}
		result, err := callAitableFieldGroupResult(cmd, "update_field_group", args)
		if err != nil {
			return err
		}
		return output.StoreResult(cmd.Context(), result)
	}
	return DeclareLeafMetadata(cmd, LeafSpec{Safety: spec.Safety, OutputRollout: spec.OutputRollout, Contract: spec.Contract, Validate: func(cmd *cobra.Command, _ []string) error {
		if err := corecmd.ValidateRequired(cmd, flags); err != nil {
			return err
		}
		if cmd.Flags().Changed("name") {
			name, _ := cmd.Flags().GetString("name")
			if strings.TrimSpace(name) == "" {
				return apperrors.NewValidation("--name 显式传入时必须为非空白字符串")
			}
		}
		return nil
	}})
}

func aitableFieldGroupWriteSafety() contract.SafetySpec {
	return contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "idempotent"}
}

func aitableFieldGroupConfirmEmptyFlag() LeafFlag {
	return LeafFlag{Name: "confirm-empty-group", Bind: "confirmEmptyGroup", Kind: corecmd.KindBool, Usage: "确认移动后自动取消空的源组；默认不允许移空源组"}
}

func aitableFieldGroupIDsFlag() LeafFlag {
	return LeafFlag{Name: "field-ids", Bind: "fieldIds", Required: true, Trim: true, Usage: "1～100 个不重复的字段 ID，逗号分隔", Transform: func(raw string) (any, error) {
		ids := strings.Split(raw, ",")
		if len(ids) > 100 {
			return nil, apperrors.NewValidation("--field-ids 最多 100 项")
		}
		seen := make(map[string]bool, len(ids))
		for index := range ids {
			ids[index] = strings.TrimSpace(ids[index])
			if ids[index] == "" {
				return nil, apperrors.NewValidation("--field-ids 不能包含空字段 ID")
			}
			if seen[ids[index]] {
				return nil, apperrors.NewValidation("--field-ids 不能包含重复字段 ID")
			}
			seen[ids[index]] = true
		}
		return ids, nil
	}}
}

func aitableFieldGroupResultSpec(write bool) *contract.ResultSpec {
	outcomes := []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}
	status := ""
	required := `["baseId","tableId","groups"]`
	if write {
		outcomes = append(outcomes, contract.ResultOutcomePending)
		required = `["baseId","tableId","verificationStatus"]`
		status = `,"verificationStatus":{"type":"string","description":"verified 表示服务端已核验，pending 表示需继续只读核对；未核实的 groups 不返回","enum":["verified","pending"]},"groupId":{"type":"string","description":"本次操作的字段组 ID；操作适用时返回"},"warningMessage":{"type":"string","description":"读回未核实等需要保留的服务端提示"}`
	}
	return &contract.ResultSpec{Outcomes: outcomes, DataSchema: aitableResultSchemaWithDryRun("字段编组查询或变更回执；pending 不表示配置已生效", json.RawMessage(`{"type":"object","properties":{"baseId":{"type":"string","description":"所属 Base ID"},"tableId":{"type":"string","description":"所属数据表 ID"},"groups":{"type":"array","description":"服务端读取的字段组；写入读回未核实时省略，不伪造空集合","items":{"type":"object","properties":{"groupId":{"type":"string","description":"字段组 ID"},"name":{"type":"string","description":"字段组名称"},"description":{"type":"string","description":"字段组说明"},"fieldIds":{"type":"array","description":"组内字段 ID","items":{"type":"string"}}},"required":["groupId","name","fieldIds"],"additionalProperties":true}}`+status+`},"required":`+required+`,"additionalProperties":true}`))}
}

func callAitableFieldGroupResult(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error) {
	if preview, ok := aitableUnifiedDryRunResult(tool, args); ok {
		return preview, nil
	}
	data, err := callAitableUnifiedDataContext(cmd.Context(), tool, args)
	if err != nil {
		return nil, err
	}
	if tool == "list_field_groups" {
		return output.Success(data), nil
	}
	if object, ok := data.(map[string]any); ok {
		switch object["verificationStatus"] {
		case "verified":
			return output.Success(data), nil
		case "pending":
			return output.Pending(data, &output.OperationInfo{ID: fmt.Sprint(args["tableId"]), State: "pending", NextCommand: fmt.Sprintf("dws aitable field-group list --base-id %s --table-id %s --format json", ShellQuoteArg(fmt.Sprint(args["baseId"])), ShellQuoteArg(fmt.Sprint(args["tableId"])))}), nil
		}
	}
	return nil, apperrors.NewAPI("字段组写入返回无效回执；请使用 field-group list 只读核对，不要重放写入",
		apperrors.WithReason("invalid_field_group_receipt"), apperrors.WithOrigin("mcp"),
		apperrors.WithOperation("tools/call"), apperrors.WithServerKey("aitable"), apperrors.WithRetryable(false))
}
