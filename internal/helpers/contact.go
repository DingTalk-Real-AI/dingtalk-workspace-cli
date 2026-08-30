package helpers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

// ──────────────────────────────────────────────────────────
// dws contact — 通讯录
// ──────────────────────────────────────────────────────────

func parseCSVValues(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		var values []string
		if err := json.Unmarshal([]byte(raw), &values); err == nil {
			return cleanStringValues(values)
		}
		raw = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(raw, "["), "]"))
	}
	parts := strings.Split(raw, ",")
	return cleanStringValues(parts)
}

func cleanStringValues(parts []string) []string {
	values := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.Trim(strings.TrimSpace(p), `"'`)
		if v != "" {
			values = append(values, v)
		}
	}
	return values
}

// contactUserIDFlagKeys 汇总 contact user get 支持的所有 flag 名（含 camelCase 派生与全小写写法），
// 在 RunE 中统一引用，避免每个调用点重复维护别名列表。
// camelCase 版本 --userId / --userIds 由 RegisterCamelCaseAliases 自动派生，--userid 为手写全小写别名。
var contactUserIDFlagKeys = []string{"ids", "user-id", "user-ids", "userId", "userIds", "userid"}

// contactRootDeptLikeTokens 是用户/模型常写错的"根部门占位符"。钉钉根部门 deptId 恒为 1。
// 在 contactParseInt64WithAliases 与 list-members 的 CSV 解析里命中这类值时，给出就近提示，避免调用方再去猜。
var contactRootDeptLikeTokens = map[string]struct{}{
	"self": {}, "me": {}, "root": {}, "0": {},
}

// contactFirstSetFlagName 返回 names 中第一个被用户显式传入的 flag 名（Changed=true）。
// 用于让报错文案显示用户实际输入的 flag 名，而不是主 flag 名，避免
// 出现 "用户传 --ids me 却被报 flag --id 不合法" 的错位。
func contactFirstSetFlagName(cmd *cobra.Command, names ...string) string {
	for _, n := range names {
		if f := cmd.Flag(n); f != nil && f.Changed {
			return n
		}
	}
	if len(names) > 0 {
		return names[0]
	}
	return ""
}

func contactAnyFlagChanged(cmd *cobra.Command, names ...string) bool {
	for _, n := range names {
		if f := cmd.Flag(n); f != nil && f.Changed {
			return true
		}
	}
	return false
}

func contactGetBoolWithAliases(cmd *cobra.Command, names ...string) (bool, bool) {
	for _, name := range names {
		if flag := cmd.Flag(name); flag != nil && flag.Changed {
			value, err := cmd.Flags().GetBool(name)
			return value, err == nil
		}
	}
	return false, false
}

func contactOptionalString(cmd *cobra.Command, primary string, aliases ...string) (string, bool) {
	names := append([]string{primary}, aliases...)
	if !contactAnyFlagChanged(cmd, names...) {
		return "", false
	}
	return strings.TrimSpace(flagOrFallback(cmd, primary, aliases...)), true
}

func contactOptionalDepartments(cmd *cobra.Command) ([]map[string]any, bool, error) {
	if !cmd.Flags().Changed("depts") {
		return nil, false, nil
	}
	raw := strings.TrimSpace(mustGetFlag(cmd, "depts"))
	if raw == "" {
		return nil, false, nil
	}
	var departments []map[string]any
	if err := json.Unmarshal([]byte(raw), &departments); err != nil {
		return nil, false, fmt.Errorf("--depts JSON 解析失败: %w\n  hint: 正确格式: [{\"deptId\":1}]", err)
	}
	return departments, true, nil
}

// contactParseInt64WithAliases 先在主 flag 与全部别名中找出用户实际传入的值（空则报 missing），
// 再走根部门占位符警告 + int64 解析，避免用户传别名时 RunE 读不到。
// 报错文案中使用用户实际输入的 flag 名（比如用户传 --ids me，错误里显示 --ids 而不是主 flag --id），
// 防止用户/LLM 被"我明明没传这个 flag 为啥报它"的错位文案带偏。
func contactParseInt64WithAliases(cmd *cobra.Command, primary string, aliases ...string) (int64, error) {
	if err := validateRequiredFlagWithAliases(cmd, primary, aliases...); err != nil {
		return 0, err
	}
	raw := strings.TrimSpace(flagOrFallback(cmd, primary, aliases...))
	setName := contactFirstSetFlagName(cmd, append([]string{primary}, aliases...)...)
	if _, ok := contactRootDeptLikeTokens[strings.ToLower(raw)]; ok {
		return 0, fmt.Errorf(
			"flag --%s 必须是整数；钉钉根部门 deptId=1，请使用 --%s 1", setName, setName)
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("flag --%s must be an integer: %w", setName, err)
	}
	return v, nil
}

// contactParseInt64Slice 将逗号分隔的整数字符串解析为 []int64，用于角色 ID / 部门 ID 列表。
func contactParseInt64Slice(raw string) ([]int64, error) {
	parts := parseCSVValues(raw)
	if len(parts) == 0 {
		return nil, errors.New("至少需要一个有效整数值")
	}
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		v, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%q 不是有效整数: %w", p, err)
		}
		out = append(out, v)
	}
	return out, nil
}

// contactParseInt64SliceWithAliases 从主 flag 或别名中读取逗号分隔整数列表。
func contactParseInt64SliceWithAliases(cmd *cobra.Command, primary string, aliases ...string) ([]int64, error) {
	if err := validateRequiredFlagWithAliases(cmd, primary, aliases...); err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(flagOrFallback(cmd, primary, aliases...))
	out, err := contactParseInt64Slice(raw)
	if err != nil {
		return nil, fmt.Errorf("--%s %w", contactFirstSetFlagName(cmd, append([]string{primary}, aliases...)...), err)
	}
	return out, nil
}

// contactParseBoolWithAliases 从主 flag 或别名中读取布尔值。
func contactParseBoolWithAliases(cmd *cobra.Command, primary string, aliases ...string) (bool, error) {
	if err := validateRequiredFlagWithAliases(cmd, primary, aliases...); err != nil {
		return false, err
	}
	raw := strings.TrimSpace(flagOrFallback(cmd, primary, aliases...))
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("--%s 必须是 boolean: %w", contactFirstSetFlagName(cmd, append([]string{primary}, aliases...)...), err)
	}
	return v, nil
}

func newContactDeptCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "创建部门",
		Long: `在当前企业下创建部门。--create-dept-group 必须显式传 true 或 false。
不传 --parent 时使用企业根部门。该写操作执行前需要确认，自动化场景在用户明确授权后传 --yes。`,
		Example: `  dws contact dept create --name "新产品部" --create-dept-group=true
  dws contact dept create --name "研发一组" --parent 12345 --create-dept-group=false`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "name", "dept-name", "deptName"); err != nil {
				return err
			}
			name := strings.TrimSpace(flagOrFallback(cmd, "name", "dept-name", "deptName"))
			if name == "" {
				return fmt.Errorf("--%s 不能为空", contactFirstSetFlagName(cmd, "name", "dept-name", "deptName"))
			}
			createGroup, supplied := contactGetBoolWithAliases(cmd, "create-dept-group", "createDeptGroup")
			if !supplied {
				return fmt.Errorf("--create-dept-group 是必填参数，请显式指定 true 或 false")
			}
			toolArgs := map[string]any{
				"deptName":        name,
				"createDeptGroup": createGroup,
			}
			if contactAnyFlagChanged(cmd, "parent", "super-dept-id", "super-dept", "superDeptId") {
				parentID, err := contactParseInt64WithAliases(cmd, "parent", "super-dept-id", "super-dept", "superDeptId")
				if err != nil {
					return err
				}
				toolArgs["superDeptId"] = parentID
			}
			return callMCPTool("department_create", toolArgs)
		},
	}
	cmd.Flags().String("name", "", "部门名称 (必填)")
	cmd.Flags().String("dept-name", "", "--name 的别名")
	_ = cmd.Flags().MarkHidden("dept-name")
	cmd.Flags().String("parent", "", "父部门 ID（可选，不传默认根部门）")
	cmd.Flags().String("super-dept-id", "", "--parent 的别名")
	cmd.Flags().String("super-dept", "", "--parent 的别名")
	_ = cmd.Flags().MarkHidden("super-dept-id")
	_ = cmd.Flags().MarkHidden("super-dept")
	cmd.Flags().Bool("create-dept-group", false, "是否创建部门群 (必填，需显式传 true 或 false)")
	cli.AnnotateRuntimeRequiredFlags(cmd, "name", "create-dept-group")
	return cmd
}

func newContactLabelCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "创建角色或角色组",
		Long: `创建通讯录角色（标签）或角色组，通过 --type 显式确认创建类型。

--type role  创建角色：必须通过 --parent-id 指定所属角色组 ID（可用 label list 查询）。
--type group 创建角色组：无需 --parent-id，服务端固定挂在根层级（parentId=-1）。
parentId 不接受 0；-1 仅表示根层级角色组，创建角色时必须传真实角色组 ID。

该写操作执行前需要确认，自动化场景在用户明确授权后传 --yes。`,
		Example: `  dws contact label create --name "管理员" --type role --parent-id 12345
  dws contact label create --name "管理层" --type group`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "name", "label-name", "labelName"); err != nil {
				return err
			}
			name := strings.TrimSpace(flagOrFallback(cmd, "name", "label-name", "labelName"))
			if name == "" {
				return fmt.Errorf("--%s 不能为空", contactFirstSetFlagName(cmd, "name", "label-name", "labelName"))
			}
			if err := validateRequiredFlagWithAliases(cmd, "type", "create-type", "label-type", "createType", "labelType"); err != nil {
				return err
			}
			createType := strings.TrimSpace(flagOrFallback(cmd, "type", "create-type", "label-type", "createType", "labelType"))
			switch createType {
			case "role", "group":
			default:
				return fmt.Errorf("--type 仅支持 role（角色）或 group（角色组），当前值 %q", createType)
			}
			parentIDSupplied := contactAnyFlagChanged(cmd, "parent-id", "parentId", "parent", "label-parent-id", "labelParentId")
			var parentID int64
			if createType == "role" {
				if !parentIDSupplied {
					return fmt.Errorf("创建角色（--type role）必须通过 --parent-id 指定所属角色组 ID；若要创建角色组请改用 --type group")
				}
				parentIDRaw := strings.TrimSpace(flagOrFallback(cmd, "parent-id", "parentId", "parent", "label-parent-id", "labelParentId"))
				var err error
				parentID, err = strconv.ParseInt(parentIDRaw, 10, 64)
				if err != nil {
					return fmt.Errorf("--%s must be an integer: %w", contactFirstSetFlagName(cmd, "parent-id", "parentId", "parent", "label-parent-id", "labelParentId"), err)
				}
				if parentID <= 0 {
					return fmt.Errorf("--parent-id 必须是有效的角色组 ID（正整数）；创建角色组请改用 --type group（无需 --parent-id）")
				}
			} else if parentIDSupplied {
				return fmt.Errorf("创建角色组（--type group）无需 --parent-id，服务端固定挂在根层级；若要在指定组下创建角色请改用 --type role")
			}
			if createType == "group" {
				parentID = -1
			}
			return callMCPTool("add_label", map[string]any{
				"parentId": parentID,
				"labelModel": map[string]any{
					"name": name,
				},
			})
		},
	}
	cmd.Flags().String("name", "", "角色或角色组名称 (必填)")
	cmd.Flags().String("label-name", "", "--name 的别名")
	_ = cmd.Flags().MarkHidden("label-name")
	cmd.Flags().String("type", "", "创建类型 (必填)：role 角色（需 --parent-id 指定所属角色组），group 角色组（挂在根层级）")
	cmd.Flags().String("create-type", "", "--type 的别名")
	cmd.Flags().String("label-type", "", "--type 的别名")
	_ = cmd.Flags().MarkHidden("create-type")
	_ = cmd.Flags().MarkHidden("label-type")
	cmd.Flags().String("parent-id", "", "所属角色组 ID（--type role 时必填，正整数）")
	cmd.Flags().String("parentId", "", "--parent-id 的别名")
	cmd.Flags().String("parent", "", "--parent-id 的别名")
	cmd.Flags().String("label-parent-id", "", "--parent-id 的别名")
	cmd.Flags().String("labelParentId", "", "--parent-id 的别名")
	_ = cmd.Flags().MarkHidden("parentId")
	_ = cmd.Flags().MarkHidden("parent")
	_ = cmd.Flags().MarkHidden("label-parent-id")
	_ = cmd.Flags().MarkHidden("labelParentId")
	cli.AnnotateRuntimeRequiredFlags(cmd, "name", "type")
	return cmd
}

func newContactLabelUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"modify", "edit"},
		Short:   "修改角色名称",
		Long:    `修改指定角色（标签）的名称。该写操作执行前需要确认，自动化场景在用户明确授权后传 --yes。`,
		Example: `  dws contact label update --id 12345 --name "新名称"`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			labelID, err := contactParseInt64WithAliases(cmd, "id", "label-id", "labelId", "role-id")
			if err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "name", "label-name", "labelName"); err != nil {
				return err
			}
			name := strings.TrimSpace(flagOrFallback(cmd, "name", "label-name", "labelName"))
			if name == "" {
				return fmt.Errorf("--%s 不能为空", contactFirstSetFlagName(cmd, "name", "label-name", "labelName"))
			}
			return callMCPTool("update_label", map[string]any{
				"labelId": labelID,
				"label": map[string]any{
					"name": name,
				},
			})
		},
	}
	cmd.Flags().String("id", "", "角色 ID (必填)")
	cmd.Flags().String("label-id", "", "--id 的别名")
	cmd.Flags().String("role-id", "", "--id 的别名")
	_ = cmd.Flags().MarkHidden("label-id")
	_ = cmd.Flags().MarkHidden("role-id")
	cmd.Flags().String("name", "", "角色新名称 (必填)")
	cmd.Flags().String("label-name", "", "--name 的别名")
	_ = cmd.Flags().MarkHidden("label-name")
	cli.AnnotateRuntimeRequiredFlags(cmd, "id", "name")
	return cmd
}

func newContactLabelDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete",
		Aliases: []string{"remove", "rm"},
		Short:   "删除角色或角色组",
		Long: `根据 ID 删除通讯录中的角色（标签）或角色组。删除角色组时会同时删除组下所有角色。
该写操作执行前需要确认，自动化场景在用户明确授权后传 --yes。`,
		Example: `  dws contact label delete --id 12345`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			id, err := contactParseInt64WithAliases(cmd, "id", "label-id", "labelId", "role-id")
			if err != nil {
				return err
			}
			return callMCPTool("delete_label", map[string]any{
				"id": id,
			})
		},
	}
	cmd.Flags().String("id", "", "要删除的角色或角色组 ID (必填)")
	cmd.Flags().String("label-id", "", "--id 的别名")
	cmd.Flags().String("role-id", "", "--id 的别名")
	_ = cmd.Flags().MarkHidden("label-id")
	_ = cmd.Flags().MarkHidden("role-id")
	cli.AnnotateRuntimeRequiredFlags(cmd, "id")
	return cmd
}

func newContactLabelAddMembersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-members",
		Short: "给成员添加角色",
		Long: `为指定成员批量添加角色（标签）。--id 为角色 ID，支持逗号分隔多个角色。
该写操作执行前需要确认，自动化场景在用户明确授权后传 --yes。`,
		Example: `  dws contact label add-members --id 12345 --users user1,user2`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			labelIDs, err := contactParseInt64SliceWithAliases(cmd, "id", "label-id", "labelId", "role-id")
			if err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "users", "user-ids", "userIds", "staff-ids", "staffIds"); err != nil {
				return err
			}
			staffIDs := parseCSVValues(flagOrFallback(cmd, "users", "user-ids", "userIds", "staff-ids", "staffIds"))
			if len(staffIDs) == 0 {
				return fmt.Errorf("--users 至少需要一个成员 ID")
			}
			return callMCPTool("add_label_members", map[string]any{
				"labelIds": labelIDs,
				"staffIds": staffIDs,
			})
		},
	}
	cmd.Flags().String("id", "", "角色 ID 列表，逗号分隔 (必填)")
	cmd.Flags().String("label-id", "", "--id 的别名")
	cmd.Flags().String("role-id", "", "--id 的别名")
	_ = cmd.Flags().MarkHidden("label-id")
	_ = cmd.Flags().MarkHidden("role-id")
	cmd.Flags().String("users", "", "成员 userId 列表，逗号分隔 (必填)")
	cmd.Flags().String("user-ids", "", "--users 的别名")
	cmd.Flags().String("userIds", "", "--users 的别名")
	cmd.Flags().String("staff-ids", "", "--users 的别名")
	cmd.Flags().String("staffIds", "", "--users 的别名")
	_ = cmd.Flags().MarkHidden("user-ids")
	_ = cmd.Flags().MarkHidden("userIds")
	_ = cmd.Flags().MarkHidden("staff-ids")
	_ = cmd.Flags().MarkHidden("staffIds")
	cli.AnnotateRuntimeRequiredFlags(cmd, "id", "users")
	return cmd
}

func newContactLabelRemoveMembersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove-members",
		Aliases: []string{"delete-members"},
		Short:   "移除成员角色",
		Long: `从指定成员身上批量移除角色（标签）。--id 为角色 ID，支持逗号分隔多个角色。
该写操作执行前需要确认，自动化场景在用户明确授权后传 --yes。`,
		Example: `  dws contact label remove-members --id 12345 --users user1,user2`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			labelIDs, err := contactParseInt64SliceWithAliases(cmd, "id", "label-id", "labelId", "role-id")
			if err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "users", "user-ids", "userIds", "staff-ids", "staffIds"); err != nil {
				return err
			}
			staffIDs := parseCSVValues(flagOrFallback(cmd, "users", "user-ids", "userIds", "staff-ids", "staffIds"))
			if len(staffIDs) == 0 {
				return fmt.Errorf("--users 至少需要一个成员 ID")
			}
			return callMCPTool("remove_label_members", map[string]any{
				"labelIds": labelIDs,
				"staffIds": staffIDs,
			})
		},
	}
	cmd.Flags().String("id", "", "角色 ID 列表，逗号分隔 (必填)")
	cmd.Flags().String("label-id", "", "--id 的别名")
	cmd.Flags().String("role-id", "", "--id 的别名")
	_ = cmd.Flags().MarkHidden("label-id")
	_ = cmd.Flags().MarkHidden("role-id")
	cmd.Flags().String("users", "", "成员 userId 列表，逗号分隔 (必填)")
	cmd.Flags().String("user-ids", "", "--users 的别名")
	cmd.Flags().String("userIds", "", "--users 的别名")
	cmd.Flags().String("staff-ids", "", "--users 的别名")
	cmd.Flags().String("staffIds", "", "--users 的别名")
	_ = cmd.Flags().MarkHidden("user-ids")
	_ = cmd.Flags().MarkHidden("userIds")
	_ = cmd.Flags().MarkHidden("staff-ids")
	_ = cmd.Flags().MarkHidden("staffIds")
	cli.AnnotateRuntimeRequiredFlags(cmd, "id", "users")
	return cmd
}

func newContactLabelUpdateMemberScopeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update-member-scope",
		Aliases: []string{"set-member-scope"},
		Short:   "修改角色管理范围",
		Long: `修改指定成员在某个角色下的管理范围（部门列表）。
该写操作执行前需要确认，自动化场景在用户明确授权后传 --yes。`,
		Example: `  dws contact label update-member-scope --user user1 --id 12345 --depts 1,2`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "user", "staff-id", "staffId", "user-id", "userId"); err != nil {
				return err
			}
			staffID := strings.TrimSpace(flagOrFallback(cmd, "user", "staff-id", "staffId", "user-id", "userId"))
			if staffID == "" {
				return fmt.Errorf("--%s 不能为空", contactFirstSetFlagName(cmd, "user", "staff-id", "staffId", "user-id", "userId"))
			}
			labelID, err := contactParseInt64WithAliases(cmd, "id", "label-id", "labelId", "role-id")
			if err != nil {
				return err
			}
			deptIDs, err := contactParseInt64SliceWithAliases(cmd, "depts", "dept-ids", "deptIds")
			if err != nil {
				return err
			}
			return callMCPTool("update_label_member_scope", map[string]any{
				"staffId": staffID,
				"labelId": labelID,
				"deptIds": deptIDs,
			})
		},
	}
	cmd.Flags().String("user", "", "成员 staffId / userId (必填)")
	cmd.Flags().String("staff-id", "", "--user 的别名")
	cmd.Flags().String("staffId", "", "--user 的别名")
	cmd.Flags().String("user-id", "", "--user 的别名")
	cmd.Flags().String("userId", "", "--user 的别名")
	_ = cmd.Flags().MarkHidden("staff-id")
	_ = cmd.Flags().MarkHidden("staffId")
	_ = cmd.Flags().MarkHidden("user-id")
	_ = cmd.Flags().MarkHidden("userId")
	cmd.Flags().String("id", "", "角色 ID (必填)")
	cmd.Flags().String("label-id", "", "--id 的别名")
	cmd.Flags().String("role-id", "", "--id 的别名")
	_ = cmd.Flags().MarkHidden("label-id")
	_ = cmd.Flags().MarkHidden("role-id")
	cmd.Flags().String("depts", "", "可管理部门 ID 列表，逗号分隔 (必填)")
	cmd.Flags().String("dept-ids", "", "--depts 的别名")
	cmd.Flags().String("deptIds", "", "--depts 的别名")
	_ = cmd.Flags().MarkHidden("dept-ids")
	_ = cmd.Flags().MarkHidden("deptIds")
	cli.AnnotateRuntimeRequiredFlags(cmd, "user", "id", "depts")
	return cmd
}

func newContactExtFieldCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "创建自定义字段",
		Long: `在通讯录中创建新的自定义成员字段。
该写操作执行前需要确认，自动化场景在用户明确授权后传 --yes。`,
		Example: `  dws contact ext-field create --name "职级"`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "name", "field-name", "fieldName"); err != nil {
				return err
			}
			name := strings.TrimSpace(flagOrFallback(cmd, "name", "field-name", "fieldName"))
			if name == "" {
				return fmt.Errorf("--%s 不能为空", contactFirstSetFlagName(cmd, "name", "field-name", "fieldName"))
			}
			return callMCPTool("add_org_ext_attrs", map[string]any{
				"orgEmpAttrModels": []map[string]any{
					{"name": name, "orgSelfTag": int64(1), "newAdd": true},
				},
			})
		},
	}
	cmd.Flags().String("name", "", "自定义字段显示名称 (必填)")
	cmd.Flags().String("field-name", "", "--name 的别名")
	_ = cmd.Flags().MarkHidden("field-name")
	cli.AnnotateRuntimeRequiredFlags(cmd, "name")
	return cmd
}

func newContactExtFieldUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"modify", "edit"},
		Short:   "更新自定义字段设置",
		Long: `更新指定自定义字段的属性设置，如 clientDisplay（是否在 profile 展示）、isSearch（是否支持搜索）等。
该写操作执行前需要确认，自动化场景在用户明确授权后传 --yes。`,
		Example: `  dws contact ext-field update --code "rank" --client-display true --is-search false`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "code", "field-code", "fieldCode"); err != nil {
				return err
			}
			code := strings.TrimSpace(flagOrFallback(cmd, "code", "field-code", "fieldCode"))
			if code == "" {
				return fmt.Errorf("--%s 不能为空", contactFirstSetFlagName(cmd, "code", "field-code", "fieldCode"))
			}
			orgSelfTagRaw := strings.TrimSpace(flagOrFallback(cmd, "org-self-tag", "field-type", "fieldType"))
			var orgSelfTag int64
			if orgSelfTagRaw != "" {
				var err error
				orgSelfTag, err = strconv.ParseInt(orgSelfTagRaw, 10, 64)
				if err != nil {
					return fmt.Errorf("--%s 必须是整数: %w", contactFirstSetFlagName(cmd, "org-self-tag", "field-type", "fieldType"), err)
				}
			} else {
				orgSelfTag = 1
			}
			clientDisplay, err := contactParseBoolWithAliases(cmd, "client-display", "clientDisplay")
			if err != nil {
				return err
			}
			isSearch, err := contactParseBoolWithAliases(cmd, "is-search", "isSearch")
			if err != nil {
				return err
			}
			return callMCPTool("update_org_ext_attrs", map[string]any{
				"orgEmpAttrModels": []map[string]any{
					{"code": code, "orgSelfTag": orgSelfTag, "clientDisplay": clientDisplay, "isSearch": isSearch},
				},
			})
		},
	}
	cmd.Flags().String("code", "", "自定义字段编码 (必填)")
	cmd.Flags().String("field-code", "", "--code 的别名")
	cmd.Flags().String("fieldCode", "", "--code 的别名")
	_ = cmd.Flags().MarkHidden("field-code")
	_ = cmd.Flags().MarkHidden("fieldCode")
	cmd.Flags().String("org-self-tag", "1", "字段类型：1 企业个性化字段，0 默认扩展字段")
	cmd.Flags().String("field-type", "", "--org-self-tag 的别名")
	cmd.Flags().String("fieldType", "", "--org-self-tag 的别名")
	_ = cmd.Flags().MarkHidden("field-type")
	_ = cmd.Flags().MarkHidden("fieldType")
	cmd.Flags().String("client-display", "", "是否在客户端展示：true / false (必填)")
	cmd.Flags().String("clientDisplay", "", "--client-display 的别名")
	_ = cmd.Flags().MarkHidden("clientDisplay")
	cmd.Flags().String("is-search", "", "是否支持搜索：true / false (必填)")
	cmd.Flags().String("isSearch", "", "--is-search 的别名")
	_ = cmd.Flags().MarkHidden("isSearch")
	cli.AnnotateRuntimeRequiredFlags(cmd, "code", "client-display", "is-search")
	return cmd
}

func newContactExtFieldDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete",
		Aliases: []string{"remove", "rm"},
		Short:   "删除自定义字段",
		Long:    `删除指定的自定义成员字段。该写操作执行前需要确认，自动化场景在用户明确授权后传 --yes。`,
		Example: `  dws contact ext-field delete --code "rank"`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "code", "field-code", "fieldCode"); err != nil {
				return err
			}
			code := strings.TrimSpace(flagOrFallback(cmd, "code", "field-code", "fieldCode"))
			if code == "" {
				return fmt.Errorf("--%s 不能为空", contactFirstSetFlagName(cmd, "code", "field-code", "fieldCode"))
			}
			orgSelfTagRaw := strings.TrimSpace(flagOrFallback(cmd, "org-self-tag", "field-type", "fieldType"))
			var orgSelfTag int64
			if orgSelfTagRaw != "" {
				var err error
				orgSelfTag, err = strconv.ParseInt(orgSelfTagRaw, 10, 64)
				if err != nil {
					return fmt.Errorf("--%s 必须是整数: %w", contactFirstSetFlagName(cmd, "org-self-tag", "field-type", "fieldType"), err)
				}
			} else {
				orgSelfTag = 1
			}
			return callMCPTool("remove_org_ext_attrs", map[string]any{
				"orgEmpAttrModels": []map[string]any{
					{"code": code, "orgSelfTag": orgSelfTag, "toDelete": true},
				},
			})
		},
	}
	cmd.Flags().String("code", "", "自定义字段编码 (必填)")
	cmd.Flags().String("field-code", "", "--code 的别名")
	cmd.Flags().String("fieldCode", "", "--code 的别名")
	_ = cmd.Flags().MarkHidden("field-code")
	_ = cmd.Flags().MarkHidden("fieldCode")
	cmd.Flags().String("org-self-tag", "1", "字段类型：1 企业个性化字段，0 默认扩展字段")
	cmd.Flags().String("field-type", "", "--org-self-tag 的别名")
	cmd.Flags().String("fieldType", "", "--org-self-tag 的别名")
	_ = cmd.Flags().MarkHidden("field-type")
	_ = cmd.Flags().MarkHidden("fieldType")
	cli.AnnotateRuntimeRequiredFlags(cmd, "code")
	return cmd
}

func newContactDeptUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"modify", "edit"},
		Short:   "更新部门信息",
		Long:    "更新部门名称，并可选择调整父部门。该写操作执行前需要确认，自动化场景在用户明确授权后传 --yes。",
		Example: `  dws contact dept update --dept 12345 --name "新部门名"
  dws contact dept update --dept 12345 --name "新名称" --parent 67890`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			deptID, err := contactParseInt64WithAliases(cmd, "dept", "id", "ids", "dept-id", "dept-ids", "deptId", "deptIds")
			if err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "name", "dept-name", "deptName"); err != nil {
				return err
			}
			name := strings.TrimSpace(flagOrFallback(cmd, "name", "dept-name", "deptName"))
			if name == "" {
				return fmt.Errorf("--%s 不能为空", contactFirstSetFlagName(cmd, "name", "dept-name", "deptName"))
			}
			toolArgs := map[string]any{"deptId": deptID, "deptName": name}
			if contactAnyFlagChanged(cmd, "parent", "super-dept-id", "super-dept", "superDeptId") {
				parentID, err := contactParseInt64WithAliases(cmd, "parent", "super-dept-id", "super-dept", "superDeptId")
				if err != nil {
					return err
				}
				toolArgs["superDeptId"] = parentID
			}
			return callMCPTool("department_update", toolArgs)
		},
	}
	cmd.Flags().String("dept", "", "部门 ID (必填)")
	cmd.Flags().String("name", "", "新部门名称 (必填)")
	cmd.Flags().String("dept-name", "", "--name 的别名")
	_ = cmd.Flags().MarkHidden("dept-name")
	cmd.Flags().String("parent", "", "新父部门 ID（可选）")
	cmd.Flags().String("super-dept-id", "", "--parent 的别名")
	cmd.Flags().String("super-dept", "", "--parent 的别名")
	_ = cmd.Flags().MarkHidden("super-dept-id")
	_ = cmd.Flags().MarkHidden("super-dept")
	cli.AnnotateRuntimeRequiredFlags(cmd, "dept", "name")
	return cmd
}

func newContactUserUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"modify", "edit"},
		Short:   "修改员工信息",
		Long:    "修改员工的企业内姓名、所属部门或直属主管。至少提供一个修改项，执行前需要确认。",
		Example: `  dws contact user update --user-id user001 --org-user-name "张三三"
  dws contact user update --user-id user001 --depts '[{"deptId":1}]'`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "user-id", "id", "userid", "userId"); err != nil {
				return err
			}
			userID := strings.TrimSpace(flagOrFallback(cmd, "user-id", "id", "userid", "userId"))
			if userID == "" {
				return fmt.Errorf("--user-id 不能为空")
			}
			toolArgs := map[string]any{"userId": userID}
			changed := false
			if value, supplied := contactOptionalString(cmd, "org-user-name", "orgUserName"); supplied && value != "" {
				toolArgs["orgUserName"] = value
				changed = true
			}
			departments, supplied, err := contactOptionalDepartments(cmd)
			if err != nil {
				return err
			}
			if supplied {
				toolArgs["depts"] = departments
				changed = true
			}
			if value, supplied := contactOptionalString(cmd, "master-user-id", "masterUserId"); supplied && value != "" {
				toolArgs["masterUserId"] = value
				changed = true
			}
			if !changed {
				return fmt.Errorf("至少需要一个修改项：--org-user-name、--depts 或 --master-user-id")
			}
			return callMCPTool("employee_update", toolArgs)
		},
	}
	cmd.Flags().String("user-id", "", "要修改的员工 userId (必填)")
	cmd.Flags().String("id", "", "--user-id 的别名")
	cmd.Flags().String("userid", "", "--user-id 的别名")
	_ = cmd.Flags().MarkHidden("id")
	_ = cmd.Flags().MarkHidden("userid")
	cmd.Flags().String("org-user-name", "", "员工在企业内的名称（可选）")
	cmd.Flags().String("depts", "", "员工所属部门列表 JSON 数组（可选），格式: [{\"deptId\":1}]")
	cmd.Flags().String("master-user-id", "", "直属主管 userId（可选）")
	cli.AnnotateRuntimeRequiredFlags(cmd, "user-id")
	cli.AnnotateRuntimeConstraints(cmd, cli.RuntimeSchemaConstraints{
		RequireOneOf: [][]string{{"org-user-name", "depts", "master-user-id"}},
	})
	return cmd
}

func newContactUserUpdateSelfCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update-self",
		Aliases: []string{"update-me", "update-self-profile", "edit-self", "modify-self"},
		Short:   "更新当前用户自己的 profile 信息",
		Long:    "更新当前用户的昵称或头像。头像需先上传到钉盘取得 fileId；执行前需要确认。",
		Example: `  dws contact user update-self --nick "新昵称"
  dws contact user update-self --avatar-file-id "file-id"`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			toolArgs := map[string]any{}
			if value, supplied := contactOptionalString(cmd, "nick"); supplied && value != "" {
				toolArgs["nick"] = value
			}
			if value, supplied := contactOptionalString(cmd, "avatar-file-id", "avatarFileId"); supplied && value != "" {
				toolArgs["avatarFileId"] = value
			}
			if len(toolArgs) == 0 {
				return fmt.Errorf("至少需要一个修改项：--nick 或 --avatar-file-id")
			}
			return callMCPTool("self_user_profile_update", toolArgs)
		},
	}
	cmd.Flags().String("nick", "", "新昵称（可选）")
	cmd.Flags().String("avatar-file-id", "", "新头像在钉盘的 fileId（可选）")
	cli.AnnotateRuntimeConstraints(cmd, cli.RuntimeSchemaConstraints{
		RequireOneOf: [][]string{{"nick", "avatar-file-id"}},
	})
	return cmd
}

func newContactUserUpdateOwnnessCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update-ownness",
		Aliases: []string{"set-ownness"},
		Short:   "更新用户个人状态",
		Long:    "更新指定用户的个人状态文本（展示在个人资料与聊天会话中，如「居家办公中」）。执行前需要确认，自动化场景在用户明确授权后传 --yes。",
		Example: `  dws contact user update-ownness --user-id user001 --ownness-text "居家办公中"`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "user-id", "id", "userid", "userId"); err != nil {
				return err
			}
			userID := strings.TrimSpace(flagOrFallback(cmd, "user-id", "id", "userid", "userId"))
			if userID == "" {
				return fmt.Errorf("--user-id 不能为空")
			}
			if err := validateRequiredFlagWithAliases(cmd, "ownness-text", "ownnessText"); err != nil {
				return err
			}
			ownnessText := strings.TrimSpace(flagOrFallback(cmd, "ownness-text", "ownnessText"))
			if ownnessText == "" {
				return fmt.Errorf("--ownness-text 不能为空")
			}
			return callMCPTool("user_ownness_update", map[string]any{
				"userId":      userID,
				"ownnessText": ownnessText,
			})
		},
	}
	cmd.Flags().String("user-id", "", "要更新个人状态的用户 userId (必填)")
	cmd.Flags().String("id", "", "--user-id 的别名")
	cmd.Flags().String("userid", "", "--user-id 的别名")
	_ = cmd.Flags().MarkHidden("id")
	_ = cmd.Flags().MarkHidden("userid")
	cmd.Flags().String("ownness-text", "", "个人状态文本 (必填)，如 \"居家办公中\"")
	cli.AnnotateRuntimeRequiredFlags(cmd, "user-id", "ownness-text")
	return cmd
}

func newContactAccountUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update",
		Aliases: []string{"modify", "edit"},
		Short:   "更新企业账号用户信息",
		Long:    "更新企业账号的员工姓名、部门、直属主管、昵称或头像。至少提供一个修改项，执行前需要确认。",
		Example: `  dws contact account update --user-id user001 --org-user-name "张三"
  dws contact account update --user-id user001 --nick "新昵称" --avatar-file-id "file-id"`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "user-id", "id", "userid", "userId"); err != nil {
				return err
			}
			userID := strings.TrimSpace(flagOrFallback(cmd, "user-id", "id", "userid", "userId"))
			if userID == "" {
				return fmt.Errorf("--user-id 不能为空")
			}
			toolArgs := map[string]any{"userId": userID}
			if value, supplied := contactOptionalString(cmd, "org-user-name", "orgUserName"); supplied && value != "" {
				toolArgs["orgUserName"] = value
			}
			departments, supplied, err := contactOptionalDepartments(cmd)
			if err != nil {
				return err
			}
			if supplied {
				toolArgs["depts"] = departments
			}
			if value, supplied := contactOptionalString(cmd, "master-user-id", "masterUserId"); supplied && value != "" {
				toolArgs["masterUserId"] = value
			}
			if value, supplied := contactOptionalString(cmd, "nick"); supplied && value != "" {
				toolArgs["nick"] = value
			}
			if value, supplied := contactOptionalString(cmd, "avatar-file-id", "avatarFileId"); supplied && value != "" {
				toolArgs["avatarFileId"] = value
			}
			if len(toolArgs) == 1 {
				return fmt.Errorf("至少需要一个修改项：--org-user-name、--depts、--master-user-id、--nick 或 --avatar-file-id")
			}
			return callMCPTool("exclusive_account_user_update", toolArgs)
		},
	}
	cmd.Flags().String("user-id", "", "被修改企业账号的 userId (必填)")
	cmd.Flags().String("id", "", "--user-id 的别名")
	cmd.Flags().String("userid", "", "--user-id 的别名")
	_ = cmd.Flags().MarkHidden("id")
	_ = cmd.Flags().MarkHidden("userid")
	cmd.Flags().String("org-user-name", "", "企业账号在企业内的员工姓名（可选）")
	cmd.Flags().String("depts", "", "部门列表 JSON 数组（可选），格式: [{\"deptId\":1}]")
	cmd.Flags().String("master-user-id", "", "直属主管 userId（可选）")
	cmd.Flags().String("nick", "", "企业账号自身昵称（可选）")
	cmd.Flags().String("avatar-file-id", "", "企业账号头像在钉盘的 fileId（可选）")
	cli.AnnotateRuntimeRequiredFlags(cmd, "user-id")
	cli.AnnotateRuntimeConstraints(cmd, cli.RuntimeSchemaConstraints{
		RequireOneOf: [][]string{{"org-user-name", "depts", "master-user-id", "nick", "avatar-file-id"}},
	})
	return cmd
}

func newContactCommand() *cobra.Command {
	// Product-level Agent routing Decl (migrated from selection/contact.json
	// products.contact). Catalog assembly stamps provenance contract_final.
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "contact",
		HelpReferences: contract.HelpReferences{
			RelatedSkills: []string{"dingtalk-contact"},
			Documentation: []contract.HelpDocumentation{
				contract.SkillDocumentation("通讯录深度指南", "dingtalk-contact", "references/contact.md"),
			},
		},
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "查询通讯录与花名册，并管理企业、部门、员工及企业账号",
			UseWhen: []string{
				"按姓名/手机号/userId/部门条件做通讯录精确查询，或明确执行企业与员工入企管理",
			},
			AvoidWhen: []string{
				"职责/上级等语义找人优先 aisearch person；不要用 contact 发消息；写操作前确认当前企业和目标信息",
			},
		},
	})
	root := newGroupCommand(&cobra.Command{
		Use:   "contact",
		Short: "通讯录 / 用户 / 部门 / 角色 / 人员关系",
		Long: `查询钉钉通讯录：用户搜索、手机号查找、部门搜索、子部门 / 成员列表、人员关系；用户花名册档案信息（学历、家庭、银行卡、合同等）与离职员工信息。

通讯录功能：
  - contact user get-self/search/search-mobile/get: 通讯录用户查询
  - contact user invite/update/update-self/update-ownness: 邀请与更新员工
  - contact dept search/get-info/list-children/list-members/create/update: 部门查询与管理
  - contact label create/list/get/list-members: 角色创建与查询
  - contact relation list-my-followings: 特别关注人查询

企业管理功能：
  - contact org create: 创建企业
  - contact account create/update: 创建与更新企业专属账号

基础人事功能（HR 花名册）：
  - contact user profile fields/get: 员工花名册档案查询（学历、家庭、银行卡等）
  - contact user dismission search: 离职员工列表查询`,
		RunE: groupRunE,
	})

	userCmd := newGroupCommand(&cobra.Command{
		Use:   "user",
		Short: "人员管理",
		Long: `人员管理：通讯录用户查询、修改员工信息、邀请员工加入企业、用户档案（花名册）查询、离职员工查询。

【何时用哪个命令】
  - 查询用户的部门、主管、管理员权限         → contact user get
  - 修改员工信息（姓名 / 部门 / 直属主管）   → contact user update
  - 更新当前用户自己的 profile（昵称 / 头像） → contact user update-self
  - 更新用户个人状态（如「居家办公中」）     → contact user update-ownness
  - 邀请员工加入企业                         → contact user invite
  - 查询用户的学历、家庭、银行卡、合同等档案 → contact user profile get
  - 查询离职员工列表                         → contact user dismission search`,
		RunE: groupRunE,
	})

	contactUserGetSelfCmd := &cobra.Command{
		Use:     "get-self",
		Aliases: []string{"self", "me", "whoami", "current"},
		Short:   "获取当前用户信息（我是谁 / 本人）",
		Long:    "获取当前登录用户的 userId 与基本信息。\n\n触发词：我是谁 / 我的信息 / 我的 userId / 当前用户 / 本人 / self / me / whoami / current。\n别名：self / me / whoami / current 均等价于 get-self。\n无需参数；禁止用 `dws contact user get --ids me/self` 代替（会返回空数据的假成功）。",
		Example: `  dws contact user get-self
  dws contact user self       # 别名（等价）
  dws contact user me         # 别名（等价）`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("get_current_user_profile", nil)
		},
	}
	DeclareLeafMetadata(contactUserGetSelfCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "get_current_user_profile",
				CanonicalPath:  "contact.get_current_user_profile",
				CLIPath:        "contact user get-self",
				PrimaryCLIPath: "contact user get-self",
			},
			Description: "获取当前登录用户资料与 userId",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "contact", RPCName: "get_current_user_profile"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取当前登录用户资料与 userId",
				UseWhen:      []string{"用户问我是谁/我的 userId/当前账号信息"},
				AvoidWhen:    []string{"查他人详情用 contact user get 或先搜索"},
				Examples:     []string{"dws contact user get-self --format json"},
			},
		},
	})

	relationCmd := newGroupCommand(&cobra.Command{Use: "relation",
		Short: "人员关系查询",
		Long:  `查询钉钉人员关系：特别关注人。`,
		RunE:  groupRunE})

	contactRelationListMyFollowingsCmd := &cobra.Command{
		Use:     "list-my-followings",
		Short:   "获取当前用户的特别关注列表",
		Example: `  dws contact relation list-my-followings`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("list_my_followings", nil)
		},
	}
	DeclareLeafMetadata(contactRelationListMyFollowingsCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "list_my_followings",
				CanonicalPath:  "contact.list_my_followings",
				CLIPath:        "contact relation list-my-followings",
				PrimaryCLIPath: "contact relation list-my-followings",
			},
			Description: "获取当前用户的特别关注列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "contact", RPCName: "list_my_followings"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取当前用户的特别关注列表",
				UseWhen:      []string{"需要查看当前用户特别关注或星标联系人时"},
				AvoidWhen:    []string{"需要搜索企业通讯录或查询任意员工详情时不要使用；这里只返回当前用户的特别关注列表。"},
				Examples:     []string{"dws contact relation list-my-followings --format json"},
			},
		},
	})

	contactUserSearchCmd := &cobra.Command{
		Use:     "search",
		Short:   "按关键词搜索用户",
		Example: `  dws contact user search --query "张三"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 兼容 --query / --keyword / --name 三种写法（--name 为隐藏别名，对齐 dept search）。
			if err := validateRequiredFlagWithAliases(cmd, "query", "keyword", "name"); err != nil {
				return err
			}
			kw := flagOrFallback(cmd, "query", "keyword", "name")
			return callMCPTool("search_contact_by_key_word", map[string]any{
				"keyword": kw,
			})
		},
	}
	DeclareLeafMetadata(contactUserSearchCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "search_contact_by_key_word",
				CanonicalPath:  "contact.search_contact_by_key_word",
				CLIPath:        "contact user search",
				PrimaryCLIPath: "contact user search",
			},
			Description: "按关键词搜索好友和同事，提取 userId/openDingTalkId",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "contact", RPCName: "search_contact_by_key_word"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "按关键词搜索好友和同事，提取 userId/openDingTalkId",
				UseWhen:      []string{"按姓名等关键词在通讯录里精确搜人，并需要 userId 或 openDingTalkId"},
				AvoidWhen: []string{
					"职责/上级/技能等语义找人优先 aisearch person",
					"已有 userId 查详情用 contact user get",
					"查自己用 contact user get-self",
				},
				Examples: []string{"dws contact user search --query \"张三\" --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "query", Property: "keyword"},
			},
		},
	})

	contactUserSearchMobileCmd := &cobra.Command{
		Use:     "search-mobile",
		Short:   "按手机号搜索用户",
		Example: `  dws contact user search-mobile --mobile 13800138000`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "mobile"); err != nil {
				return err
			}
			return callMCPTool("search_user_by_mobile", map[string]any{
				"mobile": mustGetFlag(cmd, "mobile"),
			})
		},
	}
	DeclareLeafMetadata(contactUserSearchMobileCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "search_user_by_mobile",
				CanonicalPath:  "contact.search_user_by_mobile",
				CLIPath:        "contact user search-mobile",
				PrimaryCLIPath: "contact user search-mobile",
			},
			Description: "按手机号搜索用户",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "contact", RPCName: "search_user_by_mobile"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "按手机号搜索用户",
				UseWhen:      []string{"已知手机号，需要精确搜索用户时"},
				AvoidWhen:    []string{"已有用户 ID 或需要批量用户详情时改用批量详情命令；该命令仅按手机号定位用户。"},
				Examples:     []string{"dws contact user search-mobile --mobile 13800138000 --format json"},
			},
		},
	})

	contactUserGetCmd := &cobra.Command{
		Use:   "get",
		Short: "批量获取用户详情（组织管理信息）",
		Long: `批量获取用户详情，返回用户的组织管理信息（来自通讯录领域）。

返回字段：
  - isAdmin: 是否为管理员
  - orgEmployeeModel.orgUserId / orgUserName: 用户 ID / 姓名
  - orgEmployeeModel.orgName / orgId: 所属组织名称 / ID
  - orgEmployeeModel.orgMasterUserId / orgMasterDisplayName: 直属主管
  - orgEmployeeModel.depts: 所属部门列表（含 deptId、deptName）
  - orgEmployeeModel.labels: 角色列表

【适用场景】
  - 想知道某个用户在哪个部门、上级是谁、是不是管理员

【不适用场景】
  - 查询学历、家庭、银行卡、合同、紧急联系人等档案信息 → 请用 contact user profile get`,
		Example: `  dws contact user get --ids userId1,userId2  # 查询 userId: dws contact user search --query "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, contactUserIDFlagKeys[0], contactUserIDFlagKeys[1:]...); err != nil {
				return err
			}
			raw := flagOrFallback(cmd, contactUserIDFlagKeys[0], contactUserIDFlagKeys[1:]...)
			// 拦截“假 userId”：me/self/current/whoami/i/me 代替真实 userId 会得到空数据的假成功。
			for _, part := range parseCSVValues(raw) {
				switch strings.ToLower(strings.TrimSpace(part)) {
				case "me", "self", "current", "whoami", "i":
					return fmt.Errorf("--ids 需要真实的 userId，不接受 %q 这类占位符\n  hint: 获取当前用户用: dws contact user get-self", part)
				}
			}
			return callMCPTool("get_user_info_by_user_ids", map[string]any{
				"user_id_list": parseCSVValues(raw),
			})
		},
	}
	DeclareLeafMetadata(contactUserGetCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "get_user_info_by_user_ids",
				CanonicalPath:  "contact.get_user_info_by_user_ids",
				CLIPath:        "contact user get",
				PrimaryCLIPath: "contact user get",
			},
			Description: "按 userId 批量获取员工详情（部门/主管等，受可见性限制）",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "contact", RPCName: "get_user_info_by_user_ids"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "按 userId 批量获取员工详情（部门/主管等，受可见性限制）",
				UseWhen:      []string{"已有一个或多个 userId，需要部门、主管等详情"},
				AvoidWhen: []string{
					"还没有 userId 时先 search / aisearch person",
					"查自己不要传 me/self，用 contact user get-self",
				},
				Examples: []string{"dws contact user get --ids userId1,userId2"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "ids", Property: "user_id_list"},
			},
		},
	})

	// ── label 角色 ──────────────────────────────────────────────────

	contactLabelCmd := newGroupCommand(&cobra.Command{
		Use:     "label",
		Aliases: []string{"role"},
		Short:   "角色查询与管理",
		Long: `角色查询与管理：创建/修改/删除角色、管理角色成员与成员在角色下的管理范围、查询角色列表与成员。

【何时用哪个命令】
  - 创建角色/角色组             → contact label create
  - 修改角色名称                 → contact label update
  - 删除角色/角色组             → contact label delete
  - 为成员批量添加角色           → contact label add-members
  - 从成员批量移除角色           → contact label remove-members
  - 修改成员在角色下的管理范围   → contact label update-member-scope
  - 获取企业所有角色列表         → contact label list
  - 根据角色名称查询角色ID       → contact label get
  - 根据角色ID查询角色下的成员   → contact label list-members

【典型场景：查询某类角色的人员（如主管、管理员、财务等）】
  1. contact label list          → 获取企业全部角色列表
  2. 从返回结果中匹配目标角色名称及 labelId
  3. contact label list-members --id <labelId>  → 获取该角色下的成员`,
		RunE: groupRunE,
	})

	runContactLabelList := func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return fmt.Errorf("contact label list 不接受位置参数: %s", strings.Join(args, " "))
		}
		return callMCPTool("get_org_labels", map[string]any{})
	}
	runContactLabelGet := func(cmd *cobra.Command, args []string) error {
		if err := validateRequiredFlagWithAliases(cmd, "names", "name", "query", "keyword"); err != nil {
			return err
		}
		raw := flagOrFallback(cmd, "names", "name", "query", "keyword")
		return callMCPTool("search_label_by_name", map[string]any{
			"labelNames": parseCSVValues(raw),
		})
	}
	runContactLabelMembers := func(cmd *cobra.Command, args []string) error {
		if err := validateRequiredFlagWithAliases(cmd, "id", "label-id", "role-id"); err != nil {
			return err
		}
		return callMCPTool("get_label_members_by_labelId", map[string]any{
			"labelId": flagOrFallback(cmd, "id", "label-id", "role-id"),
		})
	}

	contactLabelGetCmd := &cobra.Command{
		Use:   "get",
		Short: "根据角色名称查询角色",
		Long: `根据角色名称精确匹配查询角色信息（角色ID、名称等）。支持同时查询多个角色名称，逗号分隔。无需分页。

注意：精确匹配可能无结果（如用户输入"管理员"但企业只有"主管理员"和"子管理员"），
此时应降级使用 label list 获取全部角色列表，从中模糊匹配包含关键词的角色。`,
		Example: `  dws contact label get --names "管理员"
  dws contact label get --names "管理员,财务"`,
		RunE: runContactLabelGet,
	}

	contactLabelListMembersCmd := &cobra.Command{
		Use:     "list-members",
		Short:   "查询角色下的成员",
		Long:    `根据角色ID查询该角色下的成员列表。`,
		Example: `  dws contact label list-members --id 12345  # 查询 labelId: dws contact label get --names "角色名"`,
		RunE:    runContactLabelMembers,
	}

	contactLabelGetCmd.Flags().String("names", "", "角色名称，逗号分隔 (必填)")
	contactLabelGetCmd.Flags().String("name", "", "--names 的别名")
	contactLabelGetCmd.Flags().String("query", "", "--names 的别名")
	contactLabelGetCmd.Flags().String("keyword", "", "--names 的别名")
	_ = contactLabelGetCmd.Flags().MarkHidden("name")
	_ = contactLabelGetCmd.Flags().MarkHidden("query")
	_ = contactLabelGetCmd.Flags().MarkHidden("keyword")

	contactLabelListMembersCmd.Flags().String("id", "", "角色 ID (必填)")
	contactLabelListMembersCmd.Flags().String("label-id", "", "--id 的别名")
	contactLabelListMembersCmd.Flags().String("role-id", "", "--id 的别名")
	_ = contactLabelListMembersCmd.Flags().MarkHidden("label-id")
	_ = contactLabelListMembersCmd.Flags().MarkHidden("role-id")

	contactLabelListAllCmd := &cobra.Command{
		Use:   "list",
		Short: "获取企业所有角色列表",
		Long: `获取当前企业的所有角色（标签）列表，返回角色ID、角色名称等信息。无需参数。

用于不知道准确角色名称时，先列出全部角色，再根据需要选择目标角色查询成员。

【典型场景】
  - 用户说"企业所有主管/查所有管理员/财务人员有哪些"
    → 先 label list 浏览全部角色，匹配目标角色后 label list-members 获取成员`,
		Example: `  dws contact label list`,
		RunE:    runContactLabelList,
	}

	contactLabelCreateCmd := newContactLabelCreateCommand()
	DeclareLeafMetadata(contactLabelCreateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "add_label",
				CanonicalPath:  "contact.add_label",
				CLIPath:        "contact label create",
				PrimaryCLIPath: "contact label create",
			},
			Description: "创建通讯录角色（标签）或角色组：--type role 在指定角色组下创建角色，--type group 创建根层级角色组",
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: json.RawMessage(`{"type":"object","description":"标签创建结果","properties":{"result":{"type":"object","description":"调用结果","properties":{"id":{"type":"integer","description":"新建角色或角色组的标签 ID"}},"required":["id"]},"success":{"type":"boolean","description":"是否创建成功"},"errorCode":{"type":"string","description":"错误码"},"errorMsg":{"type":"string","description":"错误信息"}},"required":["success"],"additionalProperties":true}`),
			},
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "The executable CLI composes the add_label MCP tool call manually: label creation requires type-dependent parentId semantics (role requires a real group id, group passes -1) that the flag mapping layer owns.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建通讯录角色（标签）或角色组",
				UseWhen:      []string{"用户明确要求新建角色（--type role，需角色组 ID）或新建角色组（--type group，挂在根层级）"},
				AvoidWhen:    []string{"修改已有角色名称应使用 contact label update；仅查找角色应使用 contact label get 或 contact label list"},
				Examples: []string{
					"dws contact label create --name \"管理员\" --type role --parent-id 12345",
					"dws contact label create --name \"管理层\" --type group",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "name", Property: "labelModel.name", Required: boolPtr(true)},
				{Name: "type", Required: boolPtr(true), Enum: []string{"role", "group"}, Description: "创建类型：role 角色（--parent-id 必填），group 角色组（固定根层级 parentId=-1）"},
				{Name: "parent-id", Property: "parentId", Required: boolPtr(false), InterfaceType: "integer", RequiredWhen: "type=role", Description: "所属角色组 ID（正整数）；--type group 时禁止传"},
			},
		},
	})
	contactLabelUpdateCmd := newContactLabelUpdateCommand()
	DeclareLeafMetadata(contactLabelUpdateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "update_label",
				CanonicalPath:  "contact.update_label",
				CLIPath:        "contact label update",
				PrimaryCLIPath: "contact label update",
			},
			Description: "修改通讯录中指定角色（标签）的名称，其他属性保持原值",
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: json.RawMessage(`{"type":"object","description":"角色名称更新结果","properties":{"result":{"type":"object","description":"调用结果","properties":{"id":{"type":"integer","description":"角色 ID"},"name":{"type":"string","description":"角色新名称"}},"required":["id","name"]},"success":{"type":"boolean","description":"是否更新成功"},"errorCode":{"type":"string","description":"错误码"},"errorMsg":{"type":"string","description":"错误信息"}},"required":["success"],"additionalProperties":true}`),
			},
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "The executable CLI composes the update_label MCP tool call manually: the label.name field must be wrapped in a label object matching the service signature.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "修改通讯录角色（标签）名称",
				UseWhen:      []string{"用户明确要求修改已有角色的名称时"},
				AvoidWhen:    []string{"创建新角色应使用 contact label create；删除角色应使用 contact label delete"},
				Examples:     []string{`dws contact label update --id 12345 --name "新名称"`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "labelId", Required: boolPtr(true), InterfaceType: "integer", Description: "要修改的角色 ID"},
				{Name: "name", Property: "label.name", Required: boolPtr(true), Description: "角色新名称"},
			},
		},
	})

	contactLabelDeleteCmd := newContactLabelDeleteCommand()
	DeclareLeafMetadata(contactLabelDeleteCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "high",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "delete_label",
				CanonicalPath:  "contact.delete_label",
				CLIPath:        "contact label delete",
				PrimaryCLIPath: "contact label delete",
			},
			Description: "根据 ID 删除通讯录中的角色（标签）或角色组；删除角色组会同时删除组下所有角色",
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: json.RawMessage(`{"type":"object","description":"角色删除结果","properties":{"result":{"type":"object","description":"调用结果"},"success":{"type":"boolean","description":"是否删除成功"},"errorCode":{"type":"string","description":"错误码"},"errorMsg":{"type":"string","description":"错误信息"}},"required":["success"],"additionalProperties":true}`),
			},
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "contact", RPCName: "delete_label"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "删除通讯录角色（标签）或角色组",
				UseWhen:      []string{"用户明确要求删除某个角色或角色组时"},
				AvoidWhen:    []string{"修改角色名称应使用 contact label update；删除组成员关系应使用 contact label remove-members"},
				Examples:     []string{`dws contact label delete --id 12345`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "id", Required: boolPtr(true), InterfaceType: "integer", Description: "要删除的角色或角色组 ID"},
			},
		},
	})

	contactLabelAddMembersCmd := newContactLabelAddMembersCommand()
	DeclareLeafMetadata(contactLabelAddMembersCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "add_label_members",
				CanonicalPath:  "contact.add_label_members",
				CLIPath:        "contact label add-members",
				PrimaryCLIPath: "contact label add-members",
			},
			Description: "为指定成员批量添加角色（标签）",
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: json.RawMessage(`{"type":"object","description":"批量添加角色成员结果","properties":{"result":{"type":"object","description":"调用结果"},"success":{"type":"boolean","description":"是否成功"},"errorCode":{"type":"string","description":"错误码"},"errorMsg":{"type":"string","description":"错误信息"}},"required":["success"],"additionalProperties":true}`),
			},
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "contact", RPCName: "add_label_members"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "为成员批量添加角色（标签）",
				UseWhen:      []string{"需要把一批成员加入一个或多个角色时"},
				AvoidWhen:    []string{"创建新角色应使用 contact label create；移除成员角色应使用 contact label remove-members"},
				Examples:     []string{`dws contact label add-members --id 12345 --users user1,user2`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "labelIds", Required: boolPtr(true), InterfaceType: "array", Description: "角色 ID 列表，逗号分隔"},
				{Name: "users", Property: "staffIds", Required: boolPtr(true), InterfaceType: "array", Description: "成员 userId 列表，逗号分隔"},
			},
		},
	})

	contactLabelRemoveMembersCmd := newContactLabelRemoveMembersCommand()
	DeclareLeafMetadata(contactLabelRemoveMembersCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "remove_label_members",
				CanonicalPath:  "contact.remove_label_members",
				CLIPath:        "contact label remove-members",
				PrimaryCLIPath: "contact label remove-members",
			},
			Description: "从指定成员身上批量移除角色（标签）",
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: json.RawMessage(`{"type":"object","description":"批量移除角色成员结果","properties":{"result":{"type":"object","description":"调用结果"},"success":{"type":"boolean","description":"是否成功"},"errorCode":{"type":"string","description":"错误码"},"errorMsg":{"type":"string","description":"错误信息"}},"required":["success"],"additionalProperties":true}`),
			},
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "contact", RPCName: "remove_label_members"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "从成员身上批量移除角色（标签）",
				UseWhen:      []string{"需要把一批成员从一个或多个角色中移除时"},
				AvoidWhen:    []string{"删除角色本身应使用 contact label delete；添加成员角色应使用 contact label add-members"},
				Examples:     []string{`dws contact label remove-members --id 12345 --users user1,user2`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "labelIds", Required: boolPtr(true), InterfaceType: "array", Description: "角色 ID 列表，逗号分隔"},
				{Name: "users", Property: "staffIds", Required: boolPtr(true), InterfaceType: "array", Description: "成员 userId 列表，逗号分隔"},
			},
		},
	})

	contactLabelUpdateMemberScopeCmd := newContactLabelUpdateMemberScopeCommand()
	DeclareLeafMetadata(contactLabelUpdateMemberScopeCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "update_label_member_scope",
				CanonicalPath:  "contact.update_label_member_scope",
				CLIPath:        "contact label update-member-scope",
				PrimaryCLIPath: "contact label update-member-scope",
			},
			Description: "修改指定成员在某个角色下的管理范围（部门列表）",
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: json.RawMessage(`{"type":"object","description":"修改角色管理范围结果","properties":{"result":{"type":"object","description":"调用结果"},"success":{"type":"boolean","description":"是否成功"},"errorCode":{"type":"string","description":"错误码"},"errorMsg":{"type":"string","description":"错误信息"}},"required":["success"],"additionalProperties":true}`),
			},
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "contact", RPCName: "update_label_member_scope"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "修改成员在某个角色下的管理范围",
				UseWhen:      []string{"需要设置某成员在角色下可管理的部门范围时"},
				AvoidWhen:    []string{"添加/移除成员角色应使用 contact label add-members / remove-members"},
				Examples:     []string{`dws contact label update-member-scope --user user1 --id 12345 --depts 1,2,3`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "user", Property: "staffId", Required: boolPtr(true), Description: "成员 staffId / userId"},
				{Name: "id", Property: "labelId", Required: boolPtr(true), InterfaceType: "integer", Description: "角色 ID"},
				{Name: "depts", Property: "deptIds", Required: boolPtr(true), InterfaceType: "array", Description: "可管理部门 ID 列表，逗号分隔"},
			},
		},
	})

	contactLabelCmd.AddCommand(
		contactLabelCreateCmd,
		contactLabelUpdateCmd,
		contactLabelDeleteCmd,
		contactLabelAddMembersCmd,
		contactLabelRemoveMembersCmd,
		contactLabelUpdateMemberScopeCmd,
		contactLabelListAllCmd,
		contactLabelGetCmd,
		contactLabelListMembersCmd,
	)

	contactDeptCmd := newGroupCommand(&cobra.Command{Use: "dept", Short: "部门查询", RunE: groupRunE})

	contactDeptSearchCmd := &cobra.Command{
		Use:     "search",
		Short:   "搜索部门",
		Example: `  dws contact dept search --query "技术部"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "query", "keyword", "name"); err != nil {
				return err
			}
			return callMCPTool("search_dept_by_keyword", map[string]any{
				"query": flagOrFallback(cmd, "query", "keyword", "name"),
			})
		},
	}
	DeclareLeafMetadata(contactDeptSearchCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "search_dept_by_keyword",
				CanonicalPath:  "contact.search_dept_by_keyword",
				CLIPath:        "contact dept search",
				PrimaryCLIPath: "contact dept search",
			},
			Description: "按关键词搜索部门",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "contact", RPCName: "search_dept_by_keyword"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "按关键词搜索部门",
				UseWhen:      []string{"只知道部门名称或关键词，需要定位 deptId 时"},
				AvoidWhen:    []string{"已经有准确部门 ID 并需要详情时改用部门详情命令；该命令用于关键词定位部门。"},
				Examples:     []string{"dws contact dept search --query \"技术部\" --format json"},
			},
		},
	})

	contactDeptListChildrenCmd := &cobra.Command{
		Use:     "list-children",
		Short:   "查看子部门",
		Example: `  dws contact dept list-children --dept 12345`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// list-children 主 flag 为 --dept；接受 --id / --ids / --dept-id / --dept-ids 作为别名。
			deptID, err := contactParseInt64WithAliases(cmd, "dept", "id", "ids", "dept-id", "dept-ids", "deptId", "deptIds")
			if err != nil {
				return err
			}
			return callMCPTool("get_sub_depts_by_dept_id", map[string]any{
				"deptId": deptID,
			})
		},
	}
	DeclareLeafMetadata(contactDeptListChildrenCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "get_sub_depts_by_dept_id",
				CanonicalPath:  "contact.get_sub_depts_by_dept_id",
				CLIPath:        "contact dept list-children",
				PrimaryCLIPath: "contact dept list-children",
			},
			Description: "列出指定部门的直属子部门",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "contact", RPCName: "get_sub_depts_by_dept_id"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "列出指定部门的直属子部门",
				UseWhen:      []string{"已知父部门 deptId，需要列出直属子部门时"},
				AvoidWhen:    []string{"需要父部门自身详情或递归组织树时不要使用；该命令只列直属子部门。"},
				Examples:     []string{"dws contact dept list-children --dept 12345 --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "dept", Property: "deptId"},
			},
		},
	})

	contactDeptGetInfoCmd := &cobra.Command{
		Use:     "get-info",
		Short:   "获取部门详情（部门ID、名称、人数）",
		Example: `  dws contact dept get-info --dept 12345  # 查询 deptId: dws contact dept search`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// get-info 主 flag 为 --dept；接受 --id / --ids / --dept-id / --dept-ids 作为别名。
			deptID, err := contactParseInt64WithAliases(cmd, "dept", "id", "ids", "dept-id", "dept-ids", "deptId", "deptIds")
			if err != nil {
				return err
			}
			return callMCPTool("get_dept_info_by_dept_id", map[string]any{
				"deptId": deptID,
			})
		},
	}
	DeclareLeafMetadata(contactDeptGetInfoCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "get_dept_info_by_dept_id",
				CanonicalPath:  "contact.get_dept_info_by_dept_id",
				CLIPath:        "contact dept get-info",
				PrimaryCLIPath: "contact dept get-info",
			},
			Description: "获取指定部门详情",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "contact", RPCName: "get_dept_info_by_dept_id"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取指定部门详情",
				UseWhen:      []string{"已知 deptId，需要部门名称、人数等详情时"},
				AvoidWhen:    []string{"只知道部门名称时应先搜索部门；需要列出直属子部门时改用子部门查询。"},
				Examples:     []string{"dws contact dept get-info --dept 12345 --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "dept", Property: "deptId"},
			},
		},
	})

	contactDeptListMembersCmd := &cobra.Command{
		Use:     "list-members",
		Short:   "查看部门成员（仅本部门，不含下级）",
		Long:    "查看指定部门的成员列表。\n\n范围：仅返回传入 deptId 的**本部门**直接成员，**不递归下级部门**。\n跨层级需求：先 'dws contact dept list-children --dept <父deptId>' 枚举子部门，再对子 deptId 分别或合并调用本命令。",
		Example: `  dws contact dept list-members --depts 12345,67890  # 查询 deptId: dws contact dept search 或 dws contact dept list-children`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// list-members 主 flag 为 --depts；接受 --ids / --id / --dept-id / --dept-ids 作为别名。
			if err := validateRequiredFlagWithAliases(cmd, "depts", "ids", "id", "dept-id", "dept-ids", "deptId", "deptIds"); err != nil {
				return err
			}
			raw := flagOrFallback(cmd, "depts", "ids", "id", "dept-id", "dept-ids", "deptId", "deptIds")
			// 拦截逗号分隔列表中的根部门占位符（self/me/root/0），提示应用 --depts 1。
			// 报错里显示用户实际输入的 flag 名，避免出现 "用户传 --id self 却被报 --depts 不合法" 的错位。
			setName := contactFirstSetFlagName(cmd, "depts", "ids", "id", "dept-id", "dept-ids", "deptId", "deptIds")
			for _, t := range parseCSVValues(raw) {
				if _, ok := contactRootDeptLikeTokens[strings.ToLower(strings.TrimSpace(t))]; ok {
					return fmt.Errorf(
						"flag --%s 包含非法占位符 %q；钉钉根部门 deptId=1，请使用 --%s 1", setName, t, setName)
				}
			}
			return callMCPTool("get_dept_members_by_deptId", map[string]any{
				"deptIds": parseCSVValues(raw),
			})
		},
	}
	DeclareLeafMetadata(contactDeptListMembersCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "get_dept_members_by_deptId",
				CanonicalPath:  "contact.get_dept_members_by_deptId",
				CLIPath:        "contact dept list-members",
				PrimaryCLIPath: "contact dept list-members",
			},
			Description: "查看部门成员（逗号分隔 deptId）",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "contact", RPCName: "get_dept_members_by_deptId"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查看部门成员（逗号分隔 deptId）",
				UseWhen:      []string{"需要按一个或多个 deptId 查看部门成员名单时"},
				AvoidWhen:    []string{"只需部门详情或人数时使用 contact dept get-info；需要直属子部门时使用 contact dept list-children"},
				Examples:     []string{"dws contact dept list-members --depts 12345,67890 --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "depts", Property: "deptIds"},
			},
		},
	})

	// ── user profile 用户档案（花名册） ────────────────────────────────────
	contactUserProfileCmd := newGroupCommand(&cobra.Command{
		Use:   "profile",
		Short: "用户档案（花名册）",
		Long: `用户档案（花名册）：查询花名册字段列表、查询员工花名册字段信息。

花名册字段包含：试用/转正信息、个人/家庭信息、学历信息、银行卡/合同信息、
紧急联系人和其他企业自定义信息。

【与 contact user get 的区别】
  - contact user get: 组织管理信息（部门、主管、管理员权限）
  - contact user profile get: 个人档案信息（学历、家庭、银行卡等）`,
		RunE: groupRunE,
	})

	contactUserProfileFieldsCmd := &cobra.Command{
		Use:   "fields",
		Short: "查询花名册有权限的字段列表",
		Long: `查询花名册有权限的字段列表，根据当前用户查询花名册有权限的字段列表。

花名册字段包含：试用/转正信息、个人/家庭信息、学历信息、银行卡/合同信息、
紧急联系人和其他企业自定义信息。

认证信息（corpId、optUserId）由系统自动注入，无需手动传入。

【典型用法】
  通常作为 contact user profile get 的前置步骤，用于获取可查询的字段 code 列表。`,
		Example: `  dws contact user profile fields`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPToolOnServer("hrmregister", "list_authorized_roster_fields", map[string]any{})
		},
	}
	DeclareLeafMetadata(contactUserProfileFieldsCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "list_authorized_roster_fields",
				CanonicalPath:  "contact.list_authorized_roster_fields",
				CLIPath:        "contact user profile fields",
				PrimaryCLIPath: "contact user profile fields",
			},
			Description: "查询当前用户有权查看的花名册字段",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "hrmregister", RPCName: "list_authorized_roster_fields"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询当前用户有权查看的花名册字段",
				UseWhen:      []string{"查询花名册前，需要先确认当前用户可见的字段 code 时"},
				AvoidWhen:    []string{"需要读取某位员工的具体花名册字段值时不要停在字段目录，应再调用员工花名册查询。"},
				Examples:     []string{"dws contact user profile fields --format json"},
			},
		},
	})

	contactUserProfileGetCmd := &cobra.Command{
		Use:   "get",
		Short: "查询员工花名册字段信息（个人档案）",
		Long: `查询员工花名册字段信息，根据当前用户指定员工和字段列表，查询相应管理范围内员工的字段值信息。

花名册字段包含：试用/转正信息、个人/家庭信息、学历信息、银行卡/合同信息、
紧急联系人和其他企业自定义信息。

返回字段枚举说明：
  - employeeType 员工类型：0 无类型，1 全职，2 兼职，3 实习，4 劳务派遣，5 退休返聘，6 劳务外包
  - employeeStatus 员工状态：-1 无状态，1 待入职，2 试用，3 正式，4 离职，5 待离职，6 试岗，7 已退休

认证信息（corpId、optUserId）由系统自动注入，无需手动传入。
--staff-id 为查询员工 ID，--fields 为指定字段集合（逗号分隔），可通过
contact user profile fields 获取可用字段列表。

【适用场景】
  - 查询某员工的学历、家庭、银行卡、紧急联系人、合同等档案信息

【不适用场景】
  - 查询用户的部门、主管、管理员权限 → 请用 contact user get`,
		Example: `  dws contact user profile get --staff-id STAFF_ID
  dws contact user profile get --staff-id STAFF_ID --fields fieldCode1,fieldCode2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{}
			if v := mustGetFlag(cmd, "staff-id"); v != "" {
				params["staffId"] = v
			}
			if v, _ := cmd.Flags().GetString("fields"); v != "" {
				fieldCodes := parseCSVValues(v)
				if len(fieldCodes) > 0 {
					params["fieldCodeList"] = fieldCodes
				}
			}
			return callMCPToolOnServer("hrmregister", "get_authorized_emp_rosterInfo", params)
		},
	}
	DeclareLeafMetadata(contactUserProfileGetCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "get_authorized_emp_rosterInfo",
				CanonicalPath:  "contact.get_authorized_emp_rosterInfo",
				CLIPath:        "contact user profile get",
				PrimaryCLIPath: "contact user profile get",
			},
			Description: "按字段 code 查询指定员工花名册字段值",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "hrmregister", RPCName: "get_authorized_emp_rosterInfo"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "按字段 code 查询指定员工花名册字段值",
				UseWhen:      []string{"已确认 staffId 与可见字段 code，需要读花名册字段"},
				AvoidWhen: []string{
					"尚不知道可见字段时先用 contact user profile fields",
					"普通通讯录姓名搜索不要走花名册",
				},
				Examples: []string{"dws contact user profile get --staff-id STAFF_ID --fields fieldCode1,fieldCode2"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "fields", Property: "fieldCodeList", Required: boolPtr(false)},
				{Name: "staff-id", Required: boolPtr(false)},
			},
		},
	})
	contactUserProfileGetCmd.Flags().String("staff-id", "", "查询员工 ID（可选）")
	contactUserProfileGetCmd.Flags().String("fields", "", "指定字段集合, 逗号分隔, 可通过 profile fields 获取（可选）")

	contactUserProfileCmd.AddCommand(contactUserProfileFieldsCmd, contactUserProfileGetCmd)

	// ── user dismission 离职员工 ───────────────────────────────────────────
	contactUserDismissionCmd := newGroupCommand(&cobra.Command{
		Use:   "dismission",
		Short: "离职员工查询",
		Long:  `离职员工查询：分页获取离职员工列表，支持按员工姓名、离职时间范围、部门进行过滤。`,
		RunE:  groupRunE,
	})

	contactUserDismissionSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "分页获取离职员工列表",
		Long: `分页获取离职员工列表，支持按员工姓名、离职时间范围、部门进行过滤。

认证信息（corpId、optUserId）由系统自动注入，无需手动传入。
  --name              员工姓名，模糊搜索（可选）
  --start             离职日期查询范围开始，格式 YYYY-MM-DD（可选）
  --end               离职日期查询范围结束，格式 YYYY-MM-DD（可选）
  --depts             部门 ID 列表，逗号分隔（可选）
  --hide-retirement   是否隐藏退休，默认 true（可选）
  --hide-partner      是否隐藏合作伙伴，默认 false（可选）
  --page              页码，从 1 开始（可选，默认 1）
  --limit             页大小，200 以内（可选，默认 20）

注意：--start 和 --end 必须同时设置或同时不设置，不允许只设置其中一个。

【适用场景】
  - 查询公司离职员工名单
  - 按时间范围/部门/姓名筛选离职员工

【不适用场景】
  - 查询在职员工 → 使用 contact user search`,
		Example: `  dws contact user dismission search
  dws contact user dismission search --name "张三"
  dws contact user dismission search --start 2026-01-01 --end 2026-03-31
  dws contact user dismission search --depts 123456,789012 --page 1 --limit 50`,
		RunE: func(cmd *cobra.Command, args []string) error {
			startStr, _ := cmd.Flags().GetString("start")
			endStr, _ := cmd.Flags().GetString("end")
			if (startStr == "") != (endStr == "") {
				return fmt.Errorf("--start 和 --end 必须同时设置或同时不设置")
			}
			searchVO := map[string]any{}
			if v, _ := cmd.Flags().GetString("name"); v != "" {
				searchVO["empName"] = v
			}
			if startStr != "" {
				ts, err := parseDateToTimestamp(startStr, "start")
				if err != nil {
					return err
				}
				searchVO["startDate"] = ts
			}
			if endStr != "" {
				ts, err := parseDateToTimestamp(endStr, "end")
				if err != nil {
					return err
				}
				searchVO["endDate"] = ts
			}
			if v, _ := cmd.Flags().GetString("depts"); v != "" {
				searchVO["depts"] = parseCSVInts(v)
			}
			if cmd.Flags().Changed("hide-retirement") {
				v, _ := cmd.Flags().GetBool("hide-retirement")
				searchVO["hideRetirement"] = v
			}
			if cmd.Flags().Changed("hide-partner") {
				v, _ := cmd.Flags().GetBool("hide-partner")
				searchVO["hidePartner"] = v
			}
			params := map[string]any{
				"searchVO": searchVO,
			}
			if v, _ := cmd.Flags().GetInt("page"); v > 0 {
				params["pageNum"] = v
			}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				params["pageSize"] = v
			}
			return callMCPToolOnServer("hrmregister", "query_dismission_employee_list", params)
		},
	}
	DeclareLeafMetadata(contactUserDismissionSearchCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "query_dismission_employee_list",
				CanonicalPath:  "contact.query_dismission_employee_list",
				CLIPath:        "contact user dismission search",
				PrimaryCLIPath: "contact user dismission search",
			},
			Description: "查询离职员工列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "hrmregister", RPCName: "query_dismission_employee_list"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询离职员工列表",
				UseWhen:      []string{"需要按姓名、离职时间或部门查询离职员工时"},
				AvoidWhen:    []string{"需要查询在职员工或修改离职信息时不要使用；该命令只检索离职员工记录。"},
				Examples: []string{
					"dws contact user dismission search --format json",
					"dws contact user dismission search --name \"张三\" --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "depts", Property: "searchVO.depts"},
				{Name: "end", Property: "searchVO.endDate"},
				{Name: "hide-partner", Property: "searchVO.hidePartner"},
				{Name: "hide-retirement", Property: "searchVO.hideRetirement"},
				{Name: "limit", Property: "pageSize"},
				{Name: "name", Property: "searchVO.empName"},
				{Name: "page", Property: "pageNum"},
				{Name: "start", Property: "searchVO.startDate"},
			},
		},
	})
	contactUserDismissionSearchCmd.Flags().String("name", "", "员工姓名，模糊搜索（可选）")
	contactUserDismissionSearchCmd.Flags().String("start", "", "离职日期查询范围开始，格式 YYYY-MM-DD（可选），与end要么都不填要么都填")
	contactUserDismissionSearchCmd.Flags().String("end", "", "离职日期查询范围结束，格式 YYYY-MM-DD（可选），与start要么都不填要么都填")
	contactUserDismissionSearchCmd.Flags().String("depts", "", "部门 ID 列表，逗号分隔（可选）")
	contactUserDismissionSearchCmd.Flags().Bool("hide-retirement", true, "是否隐藏退休，默认 true（可选）")
	contactUserDismissionSearchCmd.Flags().Bool("hide-partner", false, "是否隐藏合作伙伴，默认 false（可选）")
	contactUserDismissionSearchCmd.Flags().Int("page", 1, "页码，从 1 开始（可选）")
	contactUserDismissionSearchCmd.Flags().Int("limit", 20, "页大小，200 以内（可选）")

	contactUserDismissionCmd.AddCommand(contactUserDismissionSearchCmd)

	contactUserInviteCmd := &cobra.Command{
		Use:   "invite",
		Short: "邀请员工加入企业",
		Long: `通过手机号邀请单个员工加入当前企业。

参数：
  --org-user-name    员工在企业内的名称
  --org-user-mobile  员工手机号
  --depts            员工所属部门列表 JSON 数组，格式: [{"deptId":1}]

认证信息（corpId、optUserId）由系统自动注入，无需手动传入。`,
		Example: `  dws contact user invite --org-user-name "张三" --org-user-mobile "13800138000" --depts '[{"deptId":1}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "org-user-name", "org-user-mobile"); err != nil {
				return err
			}
			deptsJSON := mustGetFlag(cmd, "depts")
			var depts []map[string]any
			if deptsJSON != "" {
				if err := json.Unmarshal([]byte(deptsJSON), &depts); err != nil {
					return fmt.Errorf("--depts JSON 解析失败: %w\n  hint: 正确格式: [{\"deptId\":1}]", err)
				}
			}
			name := strings.TrimSpace(mustGetFlag(cmd, "org-user-name"))
			mobile := strings.TrimSpace(mustGetFlag(cmd, "org-user-mobile"))
			if name == "" || mobile == "" {
				return fmt.Errorf("--org-user-name 和 --org-user-mobile 不能为空")
			}
			return callMCPTool("add_employee", map[string]any{
				"orgUserName":   name,
				"orgUserMobile": mobile,
				"depts":         depts,
			})
		},
	}
	DeclareLeafMetadata(contactUserInviteCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "add_employee",
				CanonicalPath:  "contact.add_employee",
				CLIPath:        "contact user invite",
				PrimaryCLIPath: "contact user invite",
			},
			Description: "按手机号邀请一名员工加入当前企业",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: the executable CLI maps employee identity and decoded department JSON to contact/add_employee, which is absent from the pinned MCP metadata snapshot.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "按手机号邀请一名员工加入当前企业",
				UseWhen:      []string{"用户明确要求邀请或添加员工到当前企业，且已提供企业内姓名和手机号"},
				AvoidWhen:    []string{"需要创建企业专属登录账号时使用 contact account create；需要创建企业组织本身时使用 contact org create"},
				Examples:     []string{"dws contact user invite --org-user-name \"张三\" --org-user-mobile \"13800138000\" --depts '[{\"deptId\":1}]'"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "depts", Property: "depts", Required: boolPtr(false), InterfaceType: "array"},
				{Name: "org-user-mobile", Property: "orgUserMobile", Required: boolPtr(true)},
				{Name: "org-user-name", Property: "orgUserName", Required: boolPtr(true)},
			},
		},
	})
	contactUserInviteCmd.Flags().String("org-user-name", "", "员工在企业内的名称 (必填)")
	contactUserInviteCmd.Flags().String("org-user-mobile", "", "员工手机号 (必填)")
	contactUserInviteCmd.Flags().String("depts", "", "员工所属部门列表 JSON 数组（可选），格式: [{\"deptId\":1}]")
	contactUserUpdateCmd := newContactUserUpdateCommand()
	DeclareLeafMetadata(contactUserUpdateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "employee_update",
				CanonicalPath:  "contact.employee_update",
				CLIPath:        "contact user update",
				PrimaryCLIPath: "contact user update",
			},
			Description: "修改指定员工的企业内姓名、所属部门或直属主管",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: the executable CLI maps employee update flags to contact/employee_update, which is absent from the pinned MCP metadata snapshot.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "修改指定员工的企业内姓名、所属部门或直属主管",
				UseWhen:      []string{"用户明确要求更新已有员工的组织信息，且已确认目标 userId 和至少一个修改项"},
				AvoidWhen:    []string{"修改当前用户自己的昵称或头像应使用 contact user update-self；创建企业专属账号应使用 contact account create"},
				Examples:     []string{"dws contact user update --user-id user001 --org-user-name \"张三三\""},
			},
			Parameters: []contract.ParamDecl{
				{Name: "depts", Property: "depts", Required: boolPtr(false), InterfaceType: "array"},
				{Name: "id", Property: "userId", Required: boolPtr(false)},
				{Name: "master-user-id", Property: "masterUserId", Required: boolPtr(false)},
				{Name: "org-user-name", Property: "orgUserName", Required: boolPtr(false)},
				{Name: "user-id", Property: "userId", Required: boolPtr(true)},
				{Name: "userid", Property: "userId", Required: boolPtr(false)},
			},
		},
	})
	contactUserUpdateSelfCmd := newContactUserUpdateSelfCommand()
	DeclareLeafMetadata(contactUserUpdateSelfCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "self_user_profile_update",
				CanonicalPath:  "contact.self_user_profile_update",
				CLIPath:        "contact user update-self",
				PrimaryCLIPath: "contact user update-self",
			},
			Description: "更新当前登录用户自己的昵称或头像",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: the executable CLI maps self-profile update flags to contact/self_user_profile_update, which is absent from the pinned MCP metadata snapshot.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "更新当前登录用户自己的昵称或头像",
				UseWhen:      []string{"用户明确要求修改自己的 profile 昵称或头像 fileId"},
				AvoidWhen:    []string{"修改其他员工的组织信息应使用 contact user update；修改企业专属账号应使用 contact account update"},
				Examples:     []string{"dws contact user update-self --nick \"新昵称\""},
			},
			Parameters: []contract.ParamDecl{
				{Name: "avatar-file-id", Property: "avatarFileId", Required: boolPtr(false)},
				{Name: "nick", Property: "nick", Required: boolPtr(false)},
			},
		},
	})
	contactUserUpdateOwnnessCmd := newContactUserUpdateOwnnessCommand()
	DeclareLeafMetadata(contactUserUpdateOwnnessCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "user_ownness_update",
				CanonicalPath:  "contact.user_ownness_update",
				CLIPath:        "contact user update-ownness",
				PrimaryCLIPath: "contact user update-ownness",
			},
			Description: "更新指定用户的个人状态文本（展示在个人资料与聊天会话中，如「居家办公中」）",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: the executable CLI maps personal-status update flags to contact/user_ownness_update, which is absent from the pinned MCP metadata snapshot.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "更新指定用户的个人状态文本（如「居家办公中」）",
				UseWhen:      []string{"用户明确要求设置或修改自己/指定用户的个人状态文本，且已确认目标 userId 和状态内容"},
				AvoidWhen:    []string{"修改员工组织信息（姓名 / 部门 / 主管）应使用 contact user update；修改当前用户昵称或头像应使用 contact user update-self"},
				Examples:     []string{"dws contact user update-ownness --user-id user001 --ownness-text \"居家办公中\""},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "userId", Required: boolPtr(false)},
				{Name: "ownness-text", Property: "ownnessText", Required: boolPtr(true)},
				{Name: "user-id", Property: "userId", Required: boolPtr(true)},
				{Name: "userid", Property: "userId", Required: boolPtr(false)},
			},
		},
	})

	// ── flags 注册 ───────────────────────────────────────────────
	contactUserSearchCmd.Flags().String("query", "", "搜索关键词 (必填)")
	contactUserSearchCmd.Flags().String("keyword", "", "--query 的别名")
	contactUserSearchCmd.Flags().String("name", "", "--query 的别名")
	_ = contactUserSearchCmd.Flags().MarkHidden("keyword")
	_ = contactUserSearchCmd.Flags().MarkHidden("name")
	contactUserSearchMobileCmd.Flags().String("mobile", "", "手机号 (必填)")
	contactUserGetCmd.Flags().String("ids", "", "用户 ID 列表 (必填)")
	contactUserGetCmd.Flags().String("user-id", "", "--ids 的别名")
	contactUserGetCmd.Flags().String("user-ids", "", "--ids 的别名")
	contactUserGetCmd.Flags().String("userid", "", "--ids 的别名（全小写）")
	_ = contactUserGetCmd.Flags().MarkHidden("user-id")
	_ = contactUserGetCmd.Flags().MarkHidden("user-ids")
	_ = contactUserGetCmd.Flags().MarkHidden("userid")
	userCmd.AddCommand(
		contactUserGetSelfCmd, contactUserSearchCmd, contactUserSearchMobileCmd, contactUserGetCmd,
		contactUserInviteCmd,        // 邀请员工加入企业
		contactUserUpdateCmd,        // 修改员工信息
		contactUserUpdateSelfCmd,    // 更新当前用户自己的 profile 信息
		contactUserUpdateOwnnessCmd, // 更新用户个人状态
		contactUserProfileCmd,       // 花名册档案
		contactUserDismissionCmd,    // 离职员工
	)

	contactDeptSearchCmd.Flags().String("query", "", "搜索关键词 (必填)")
	contactDeptSearchCmd.Flags().String("keyword", "", "--query 的别名")
	contactDeptSearchCmd.Flags().String("name", "", "--query 的别名")
	_ = contactDeptSearchCmd.Flags().MarkHidden("keyword")
	_ = contactDeptSearchCmd.Flags().MarkHidden("name")
	// 主 flag 与 RunE 读取保持一致：get-info / list-children 用 --dept，list-members 用 --depts。
	// 历史上主 flag 曾误注册为 --id/--ids，导致 RunE 读的 --dept/--depts 未注册、命令行传入报 unknown flag。
	contactDeptGetInfoCmd.Flags().String("dept", "", "部门 ID (必填)")
	contactDeptListChildrenCmd.Flags().String("dept", "", "部门 ID (必填)")
	contactDeptListMembersCmd.Flags().String("depts", "", "部门 ID 列表 (必填)")

	// dept 系列命令统一接受 --id / --ids / --dept-id / --dept-ids 别名（集中注册避免逐命令重复写）。
	// camelCase --deptId / --deptIds 由 RegisterCamelCaseAliases 自动派生，无需手写。
	type deptIDAliasSpec struct {
		cmd     *cobra.Command
		aliases []string
	}
	contactDeptCreateCmd := newContactDeptCreateCommand()
	DeclareLeafMetadata(contactDeptCreateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "department_create",
				CanonicalPath:  "contact.department_create",
				CLIPath:        "contact dept create",
				PrimaryCLIPath: "contact dept create",
			},
			Description: "在当前企业的根部门或指定父部门下创建部门",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: the executable CLI maps department creation flags to contact/department_create, which is absent from the pinned MCP metadata snapshot.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "在当前企业的根部门或指定父部门下创建部门",
				UseWhen:      []string{"用户明确要求新建部门，且已确认部门名称、父部门及是否同步创建部门群"},
				AvoidWhen:    []string{"修改已有部门名称或父级应使用 contact dept update；仅查找部门应使用 contact dept search"},
				Examples:     []string{"dws contact dept create --name \"新产品部\" --create-dept-group=true"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "create-dept-group", Property: "createDeptGroup", Required: boolPtr(true), InterfaceType: "boolean"},
				{Name: "dept-name", Property: "deptName", Required: boolPtr(false)},
				{Name: "name", Property: "deptName", Required: boolPtr(true)},
				{Name: "parent", Property: "superDeptId", Required: boolPtr(false), InterfaceType: "integer"},
				{Name: "super-dept", Property: "superDeptId", Required: boolPtr(false), InterfaceType: "integer"},
				{Name: "super-dept-id", Property: "superDeptId", Required: boolPtr(false), InterfaceType: "integer"},
			},
		},
	})
	contactDeptUpdateCmd := newContactDeptUpdateCommand()
	DeclareLeafMetadata(contactDeptUpdateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "department_update",
				CanonicalPath:  "contact.department_update",
				CLIPath:        "contact dept update",
				PrimaryCLIPath: "contact dept update",
			},
			Description: "更新指定部门的名称，并可调整父部门",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: the executable CLI maps department update flags to contact/department_update, which is absent from the pinned MCP metadata snapshot.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "更新指定部门的名称，并可调整父部门",
				UseWhen:      []string{"用户明确要求修改已有部门名称或迁移父部门，且已确认目标 deptId"},
				AvoidWhen:    []string{"创建新部门应使用 contact dept create；仅查看部门信息应使用 contact dept get-info"},
				Examples:     []string{"dws contact dept update --dept 12345 --name \"研发中心\""},
			},
			Parameters: []contract.ParamDecl{
				{Name: "dept", Property: "deptId", Required: boolPtr(true), InterfaceType: "integer"},
				{Name: "dept-id", Property: "deptId", Required: boolPtr(false), InterfaceType: "integer"},
				{Name: "dept-ids", Property: "deptId", Required: boolPtr(false), InterfaceType: "integer"},
				{Name: "dept-name", Property: "deptName", Required: boolPtr(false)},
				{Name: "id", Property: "deptId", Required: boolPtr(false), InterfaceType: "integer"},
				{Name: "ids", Property: "deptId", Required: boolPtr(false), InterfaceType: "integer"},
				{Name: "name", Property: "deptName", Required: boolPtr(true)},
				{Name: "parent", Property: "superDeptId", Required: boolPtr(false), InterfaceType: "integer"},
				{Name: "super-dept", Property: "superDeptId", Required: boolPtr(false), InterfaceType: "integer"},
				{Name: "super-dept-id", Property: "superDeptId", Required: boolPtr(false), InterfaceType: "integer"},
			},
		},
	})
	for _, s := range []deptIDAliasSpec{
		{contactDeptGetInfoCmd, []string{"id", "dept-id", "ids", "dept-ids"}},
		{contactDeptListChildrenCmd, []string{"id", "ids", "dept-id", "dept-ids"}},
		{contactDeptListMembersCmd, []string{"ids", "id", "dept-id", "dept-ids"}},
		{contactDeptUpdateCmd, []string{"id", "ids", "dept-id", "dept-ids"}},
	} {
		for _, name := range s.aliases {
			s.cmd.Flags().String(name, "", "部门 ID 别名（等价于当前命令的主 flag）")
			_ = s.cmd.Flags().MarkHidden(name)
		}
	}
	contactDeptCmd.AddCommand(
		contactDeptSearchCmd,
		contactDeptGetInfoCmd,
		contactDeptListChildrenCmd,
		contactDeptListMembersCmd,
		contactDeptCreateCmd,
		contactDeptUpdateCmd,
	)

	// ── org 企业管理 ──────────────────────────────────────────────────

	contactOrgCmd := newGroupCommand(&cobra.Command{
		Use:   "org",
		Short: "企业管理",
		Long: `企业管理：创建企业。

【何时用哪个命令】
  - 创建新企业                         → contact org create
  - 创建企业专属账号                   → contact account create
  - 邀请员工加入企业                   → contact user invite`,
		RunE: groupRunE,
	})

	contactOrgCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建企业",
		Long: `创建一个新的钉钉企业。需提供企业名称和当前用户在企业内的名称（作为创建者）。

认证信息（corpId、optUserId）由系统自动注入，无需手动传入。`,
		Example: `  dws contact org create --org-name "我的企业" --creator-username "张三"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "org-name", "creator-username"); err != nil {
				return err
			}
			orgName := strings.TrimSpace(mustGetFlag(cmd, "org-name"))
			creatorUsername := strings.TrimSpace(mustGetFlag(cmd, "creator-username"))
			if orgName == "" || creatorUsername == "" {
				return fmt.Errorf("--org-name 和 --creator-username 不能为空")
			}
			return callMCPTool("org_create", map[string]any{
				"orgName":         orgName,
				"creatorUsername": creatorUsername,
			})
		},
	}
	DeclareLeafMetadata(contactOrgCreateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "org_create",
				CanonicalPath:  "contact.org_create",
				CLIPath:        "contact org create",
				PrimaryCLIPath: "contact org create",
			},
			Description: "创建一个新的钉钉企业组织",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: the executable CLI maps organization and creator names to contact/org_create, which is absent from the pinned MCP metadata snapshot.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建一个新的钉钉企业组织",
				UseWhen:      []string{"用户明确要求创建、新建、开通或初始化企业组织，并已提供企业名称和创建者企业内名称"},
				AvoidWhen:    []string{"请求中包含企业账号、专属账号或登录账号时改用 contact account create；邀请员工时改用 contact user invite"},
				Examples:     []string{"dws contact org create --org-name \"我的企业\" --creator-username \"张三\""},
			},
			Parameters: []contract.ParamDecl{
				{Name: "creator-username", Property: "creatorUsername", Required: boolPtr(true)},
				{Name: "org-name", Property: "orgName", Required: boolPtr(true)},
			},
		},
	})
	contactOrgCreateCmd.Flags().String("org-name", "", "企业名称 (必填)")
	contactOrgCreateCmd.Flags().String("creator-username", "", "创建者在企业内的名称，对应 creatorUsername (必填)")
	contactOrgCmd.AddCommand(contactOrgCreateCmd)

	// ── account 企业账号管理 ──────────────────────────────────────────

	contactAccountCmd := newGroupCommand(&cobra.Command{
		Use:   "account",
		Short: "企业账号管理",
		Long:  "企业账号管理：创建或更新企业专属账号。",
		RunE:  groupRunE,
	})

	contactAccountCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建企业专属账号",
		Long: `为当前企业创建一个专属登录账号。

必填：--org-user-name、--login-id
可选：--org-user-mobile、--email、--dept-ids、--send-pwd-via-sms

注意：
  - 登录号（--login-id）请勿包含手机号等联系方式，否则可能被运营商拦截短信
  - --send-pwd-via-sms 控制是否通过手机短信/邮件发送登录邀请

认证信息（corpId、optUserId）由系统自动注入，无需手动传入。`,
		Example: `  dws contact account create --org-user-name "张三" --login-id "zhangsan001" --org-user-mobile "13800138000" --email "zhangsan@example.com" --dept-ids "1,2,3" --send-pwd-via-sms`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "org-user-name", "login-id"); err != nil {
				return err
			}
			orgUserName := strings.TrimSpace(mustGetFlag(cmd, "org-user-name"))
			loginID := strings.TrimSpace(mustGetFlag(cmd, "login-id"))
			if orgUserName == "" || loginID == "" {
				return fmt.Errorf("--org-user-name 和 --login-id 不能为空")
			}
			toolArgs := map[string]any{
				"orgUserName": orgUserName,
				"loginId":     loginID,
			}
			if cmd.Flags().Changed("send-pwd-via-sms") {
				sendPwdViaSMS, _ := cmd.Flags().GetBool("send-pwd-via-sms")
				toolArgs["sendPwdViaSms"] = sendPwdViaSMS
			}
			if mobile := strings.TrimSpace(mustGetFlag(cmd, "org-user-mobile")); mobile != "" {
				toolArgs["orgUserMobile"] = mobile
			}
			if email := strings.TrimSpace(mustGetFlag(cmd, "email")); email != "" {
				toolArgs["email"] = email
			}
			if cmd.Flags().Changed("dept-ids") {
				ids, err := parseCSVIntsStrict(mustGetFlag(cmd, "dept-ids"))
				if err != nil {
					return fmt.Errorf("--dept-ids 解析失败: %w", err)
				}
				toolArgs["deptIds"] = ids
			}
			return callMCPTool("exclusive_account_create", toolArgs)
		},
	}
	DeclareLeafMetadata(contactAccountCreateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "exclusive_account_create",
				CanonicalPath:  "contact.exclusive_account_create",
				CLIPath:        "contact account create",
				PrimaryCLIPath: "contact account create",
			},
			Description: "为当前企业创建专属登录账号",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: the executable CLI maps enterprise-account flags to contact/exclusive_account_create, which is absent from the pinned MCP metadata snapshot.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "为当前企业创建专属登录账号",
				UseWhen:      []string{"用户明确要求创建企业账号、专属账号或企业登录账号，并已提供员工名称和登录号"},
				AvoidWhen:    []string{"创建企业组织本身应使用 contact org create；仅邀请已有手机号员工入企应使用 contact user invite"},
				Examples:     []string{"dws contact account create --org-user-name \"张三\" --login-id \"zhangsan001\" --org-user-mobile \"13800138000\""},
			},
			Parameters: []contract.ParamDecl{
				{Name: "dept-ids", Property: "deptIds", Required: boolPtr(false), InterfaceType: "array"},
				{Name: "email", Property: "email", Required: boolPtr(false)},
				{Name: "login-id", Property: "loginId", Required: boolPtr(true)},
				{Name: "org-user-mobile", Property: "orgUserMobile", Required: boolPtr(false)},
				{Name: "org-user-name", Property: "orgUserName", Required: boolPtr(true)},
				{Name: "send-pwd-via-sms", Property: "sendPwdViaSms", Required: boolPtr(false), InterfaceType: "boolean"},
			},
		},
	})
	contactAccountCreateCmd.Flags().String("org-user-name", "", "员工在企业内的名称 (必填)")
	contactAccountCreateCmd.Flags().String("login-id", "", "登录号 (必填)，请勿包含手机号")
	contactAccountCreateCmd.Flags().String("org-user-mobile", "", "员工手机号（可选）")
	contactAccountCreateCmd.Flags().String("email", "", "邮箱（可选）")
	contactAccountCreateCmd.Flags().String("dept-ids", "", "要加入的部门 ID 列表，逗号分隔（可选）")
	contactAccountCreateCmd.Flags().Bool("send-pwd-via-sms", false, "是否通过手机短信/邮件发送登录邀请（可选）")
	contactAccountUpdateCmd := newContactAccountUpdateCommand()
	DeclareLeafMetadata(contactAccountUpdateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "exclusive_account_user_update",
				CanonicalPath:  "contact.exclusive_account_user_update",
				CLIPath:        "contact account update",
				PrimaryCLIPath: "contact account update",
			},
			Description: "更新企业专属账号的组织信息或个人资料",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: the executable CLI maps enterprise-account update flags to contact/exclusive_account_user_update, which is absent from the pinned MCP metadata snapshot.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "更新企业专属账号的组织信息或个人资料",
				UseWhen:      []string{"用户明确要求修改已有企业专属账号的姓名、部门、主管、昵称或头像"},
				AvoidWhen:    []string{"创建新企业专属账号应使用 contact account create；修改普通员工组织信息应使用 contact user update"},
				Examples:     []string{"dws contact account update --user-id user001 --nick \"新昵称\""},
			},
			Parameters: []contract.ParamDecl{
				{Name: "avatar-file-id", Property: "avatarFileId", Required: boolPtr(false)},
				{Name: "depts", Property: "depts", Required: boolPtr(false), InterfaceType: "array"},
				{Name: "id", Property: "userId", Required: boolPtr(false)},
				{Name: "master-user-id", Property: "masterUserId", Required: boolPtr(false)},
				{Name: "nick", Property: "nick", Required: boolPtr(false)},
				{Name: "org-user-name", Property: "orgUserName", Required: boolPtr(false)},
				{Name: "user-id", Property: "userId", Required: boolPtr(true)},
				{Name: "userid", Property: "userId", Required: boolPtr(false)},
			},
		},
	})
	contactAccountCmd.AddCommand(contactAccountCreateCmd, contactAccountUpdateCmd)

	relationCmd.AddCommand(contactRelationListMyFollowingsCmd)

	// ── ext-field 自定义成员字段 ──────────────────────────────────────────────────
	contactExtFieldCmd := newGroupCommand(&cobra.Command{
		Use:     "ext-field",
		Aliases: []string{"org-field", "custom-field"},
		Short:   "自定义成员字段管理",
		Long: `自定义成员字段管理：创建、更新属性、删除企业自定义字段。

【何时用哪个命令】
  - contact ext-field create   创建新的自定义成员字段
  - contact ext-field update   更新字段属性（clientDisplay / isSearch 等）
  - contact ext-field delete   删除自定义字段`,
		RunE: groupRunE,
	})

	contactExtFieldCreateCmd := newContactExtFieldCreateCommand()
	DeclareLeafMetadata(contactExtFieldCreateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "add_org_ext_attrs",
				CanonicalPath:  "contact.add_org_ext_attrs",
				CLIPath:        "contact ext-field create",
				PrimaryCLIPath: "contact ext-field create",
			},
			Description: "在通讯录中创建新的自定义成员字段",
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: json.RawMessage(`{"type":"object","description":"创建自定义字段结果","properties":{"result":{"type":"object","description":"调用结果","properties":{"code":{"type":"string","description":"字段编码"},"name":{"type":"string","description":"字段名称"},"clientDisplay":{"type":"boolean","description":"是否在客户端展示"},"isSearch":{"type":"boolean","description":"是否可搜索"},"orgSelfTag":{"type":"integer","description":"字段类型"}},"required":["code","name"]},"success":{"type":"boolean","description":"是否创建成功"},"errorCode":{"type":"string","description":"错误码"},"errorMsg":{"type":"string","description":"错误信息"}},"required":["success"],"additionalProperties":true}`),
			},
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "The executable CLI composes the add_org_ext_attrs MCP tool call manually: the orgEmpAttrModels wrapper array and fixed orgSelfTag/newAdd fields are CLI-side conventions.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建通讯录自定义成员字段",
				UseWhen:      []string{"需要为企业新增自定义成员字段（如职级、员工类型）时"},
				AvoidWhen:    []string{"更新或删除已有字段应使用 contact ext-field update / delete"},
				Examples:     []string{`dws contact ext-field create --name "职级"`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "name", Property: "orgEmpAttrModels[0].name", Required: boolPtr(true), Description: "自定义字段显示名称"},
			},
		},
	})

	contactExtFieldUpdateCmd := newContactExtFieldUpdateCommand()
	DeclareLeafMetadata(contactExtFieldUpdateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "update_org_ext_attrs",
				CanonicalPath:  "contact.update_org_ext_attrs",
				CLIPath:        "contact ext-field update",
				PrimaryCLIPath: "contact ext-field update",
			},
			Description: "更新指定自定义字段的属性设置，如 clientDisplay（是否在 profile 展示）、isSearch（是否支持搜索）等",
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: json.RawMessage(`{"type":"object","description":"更新自定义字段结果","properties":{"result":{"type":"object","description":"调用结果","properties":{"code":{"type":"string","description":"字段编码"},"name":{"type":"string","description":"字段名称"},"clientDisplay":{"type":"boolean","description":"是否在客户端展示"},"isSearch":{"type":"boolean","description":"是否可搜索"},"orgSelfTag":{"type":"integer","description":"字段类型"}},"required":["code"]},"success":{"type":"boolean","description":"是否更新成功"},"errorCode":{"type":"string","description":"错误码"},"errorMsg":{"type":"string","description":"错误信息"}},"required":["success"],"additionalProperties":true}`),
			},
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "The executable CLI composes the update_org_ext_attrs MCP tool call manually: the orgEmpAttrModels wrapper array is a CLI-side convention.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "更新自定义字段属性设置",
				UseWhen:      []string{"需要修改已有自定义字段的展示/搜索等属性时"},
				AvoidWhen:    []string{"创建新字段应使用 contact ext-field create；删除字段应使用 contact ext-field delete"},
				Examples:     []string{`dws contact ext-field update --code "rank" --client-display true --is-search false`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "code", Property: "orgEmpAttrModels[0].code", Required: boolPtr(true), Description: "自定义字段编码"},
				{Name: "org-self-tag", Property: "orgEmpAttrModels[0].orgSelfTag", Required: boolPtr(false), InterfaceType: "integer", Description: "字段类型：1 企业个性化字段，0 默认扩展字段"},
				{Name: "client-display", Property: "orgEmpAttrModels[0].clientDisplay", Required: boolPtr(true), Description: "是否在客户端展示：true / false"},
				{Name: "is-search", Property: "orgEmpAttrModels[0].isSearch", Required: boolPtr(true), Description: "是否支持搜索：true / false"},
			},
		},
	})

	contactExtFieldDeleteCmd := newContactExtFieldDeleteCommand()
	DeclareLeafMetadata(contactExtFieldDeleteCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "high",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "contact",
				Name:           "remove_org_ext_attrs",
				CanonicalPath:  "contact.remove_org_ext_attrs",
				CLIPath:        "contact ext-field delete",
				PrimaryCLIPath: "contact ext-field delete",
			},
			Description: "删除指定的自定义成员字段",
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: json.RawMessage(`{"type":"object","description":"删除自定义字段结果","properties":{"result":{"type":"object","description":"调用结果","properties":{"code":{"type":"string","description":"字段编码"},"name":{"type":"string","description":"字段名称"},"clientDisplay":{"type":"boolean","description":"是否在客户端展示"},"isSearch":{"type":"boolean","description":"是否可搜索"},"orgSelfTag":{"type":"integer","description":"字段类型"}},"required":["code"]},"success":{"type":"boolean","description":"是否删除成功"},"errorCode":{"type":"string","description":"错误码"},"errorMsg":{"type":"string","description":"错误信息"}},"required":["success"],"additionalProperties":true}`),
			},
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "The executable CLI composes the remove_org_ext_attrs MCP tool call manually: the orgEmpAttrModels wrapper array and fixed toDelete field are CLI-side conventions.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "删除通讯录自定义成员字段",
				UseWhen:      []string{"需要删除企业自定义成员字段时"},
				AvoidWhen:    []string{"创建新字段应使用 contact ext-field create；更新属性应使用 contact ext-field update"},
				Examples:     []string{`dws contact ext-field delete --code "rank"`},
			},
			Parameters: []contract.ParamDecl{
				{Name: "code", Property: "orgEmpAttrModels[0].code", Required: boolPtr(true), Description: "自定义字段编码"},
				{Name: "org-self-tag", Property: "orgEmpAttrModels[0].orgSelfTag", Required: boolPtr(false), InterfaceType: "integer", Description: "字段类型：1 企业个性化字段，0 默认扩展字段"},
			},
		},
	})

	contactExtFieldCmd.AddCommand(contactExtFieldCreateCmd, contactExtFieldUpdateCmd, contactExtFieldDeleteCmd)

	root.AddCommand(userCmd, contactDeptCmd, contactLabelCmd, contactExtFieldCmd, relationCmd, contactOrgCmd, contactAccountCmd)

	addQueryFlags := func(cmd *cobra.Command) {
		cmd.Flags().String("query", "", "搜索关键词 (必填)")
		cmd.Flags().String("keyword", "", "--query 的别名")
		cmd.Flags().String("name", "", "--query 的别名")
		_ = cmd.Flags().MarkHidden("keyword")
		_ = cmd.Flags().MarkHidden("name")
	}
	addUserIDFlags := func(cmd *cobra.Command) {
		cmd.Flags().String("ids", "", "用户 ID 列表")
		cmd.Flags().String("user-id", "", "--ids 的别名")
		cmd.Flags().String("user-ids", "", "--ids 的别名")
		cmd.Flags().String("userid", "", "--ids 的别名（全小写）")
		_ = cmd.Flags().MarkHidden("user-id")
		_ = cmd.Flags().MarkHidden("user-ids")
		_ = cmd.Flags().MarkHidden("userid")
	}
	addLabelNameFlags := func(cmd *cobra.Command) {
		cmd.Flags().String("names", "", "角色名称，逗号分隔")
		cmd.Flags().String("name", "", "--names 的别名")
		cmd.Flags().String("query", "", "--names 的别名")
		cmd.Flags().String("keyword", "", "--names 的别名")
		_ = cmd.Flags().MarkHidden("name")
		_ = cmd.Flags().MarkHidden("query")
		_ = cmd.Flags().MarkHidden("keyword")
	}
	addLabelIDFlags := func(cmd *cobra.Command) {
		cmd.Flags().String("id", "", "角色 ID")
		cmd.Flags().String("label-id", "", "--id 的别名")
		cmd.Flags().String("role-id", "", "--id 的别名")
		_ = cmd.Flags().MarkHidden("label-id")
		_ = cmd.Flags().MarkHidden("role-id")
	}
	addDeptIDFlags := func(cmd *cobra.Command) {
		cmd.Flags().String("dept", "", "部门 ID")
		cmd.Flags().String("id", "", "--dept 的别名")
		cmd.Flags().String("dept-id", "", "--dept 的别名")
		cmd.Flags().String("dept-ids", "", "--dept 的别名")
		_ = cmd.Flags().MarkHidden("id")
		_ = cmd.Flags().MarkHidden("dept-id")
		_ = cmd.Flags().MarkHidden("dept-ids")
	}
	runContactUserGet := func(cmd *cobra.Command, args []string) error {
		if err := validateRequiredFlagWithAliases(cmd, contactUserIDFlagKeys[0], contactUserIDFlagKeys[1:]...); err != nil {
			return err
		}
		raw := flagOrFallback(cmd, contactUserIDFlagKeys[0], contactUserIDFlagKeys[1:]...)
		for _, part := range parseCSVValues(raw) {
			switch strings.ToLower(strings.TrimSpace(part)) {
			case "me", "self", "current", "whoami", "i":
				return fmt.Errorf("--ids 需要真实的 userId，不接受 %q 这类占位符\n  hint: 获取当前用户用: dws contact user get-self", part)
			}
		}
		return callMCPTool("get_user_info_by_user_ids", map[string]any{
			"user_id_list": parseCSVValues(raw),
		})
	}
	runContactUserSearch := func(cmd *cobra.Command, args []string) error {
		if err := validateRequiredFlagWithAliases(cmd, "query", "keyword", "name"); err != nil {
			return err
		}
		return callMCPTool("search_contact_by_key_word", map[string]any{
			"keyword": flagOrFallback(cmd, "query", "keyword", "name"),
		})
	}

	contactRootSearchCmd := &cobra.Command{
		Use:    "search",
		Short:  "按关键词搜索用户（兼容入口）",
		Hidden: true,
		RunE:   runContactUserSearch,
	}
	contactRootFindCmd := &cobra.Command{
		Use:    "find",
		Short:  "按关键词搜索用户（兼容入口）",
		Hidden: true,
		RunE:   runContactUserSearch,
	}
	addQueryFlags(contactRootSearchCmd)
	addQueryFlags(contactRootFindCmd)

	contactRootGetCmd := &cobra.Command{
		Use:    "get",
		Short:  "获取用户/部门/角色详情（兼容入口）",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch {
			case contactAnyFlagChanged(cmd, contactUserIDFlagKeys...):
				return runContactUserGet(cmd, args)
			case contactAnyFlagChanged(cmd, "dept", "id", "dept-id", "dept-ids"):
				deptID, err := contactParseInt64WithAliases(cmd, "dept", "id", "dept-id", "dept-ids")
				if err != nil {
					return err
				}
				return callMCPTool("get_dept_info_by_dept_id", map[string]any{"deptId": deptID})
			case contactAnyFlagChanged(cmd, "names", "name", "query", "keyword"):
				return runContactLabelGet(cmd, args)
			case contactAnyFlagChanged(cmd, "label-id", "role-id"):
				return runContactLabelMembers(cmd, args)
			default:
				return fmt.Errorf("contact get 需要指定 --ids <userId>、--dept <deptId>、--names <角色名> 或 --label-id <角色ID>")
			}
		},
	}
	addUserIDFlags(contactRootGetCmd)
	addDeptIDFlags(contactRootGetCmd)
	addLabelNameFlags(contactRootGetCmd)
	contactRootGetCmd.Flags().String("label-id", "", "角色 ID")
	contactRootGetCmd.Flags().String("role-id", "", "--label-id 的别名")
	_ = contactRootGetCmd.Flags().MarkHidden("role-id")

	contactRootListCmd := &cobra.Command{
		Use:    "list",
		Short:  "列出角色/部门成员/用户详情（兼容入口）",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch {
			case contactAnyFlagChanged(cmd, "depts", "id", "dept-id", "dept-ids"):
				if err := validateRequiredFlagWithAliases(cmd, "depts", "id", "dept-id", "dept-ids"); err != nil {
					return err
				}
				raw := flagOrFallback(cmd, "depts", "id", "dept-id", "dept-ids")
				setName := contactFirstSetFlagName(cmd, "depts", "id", "dept-id", "dept-ids")
				for _, t := range parseCSVValues(raw) {
					if _, ok := contactRootDeptLikeTokens[strings.ToLower(strings.TrimSpace(t))]; ok {
						return fmt.Errorf("flag --%s 包含非法占位符 %q；钉钉根部门 deptId=1，请使用 --%s 1", setName, t, setName)
					}
				}
				return callMCPTool("get_dept_members_by_deptId", map[string]any{"deptIds": parseCSVValues(raw)})
			case contactAnyFlagChanged(cmd, contactUserIDFlagKeys...):
				return runContactUserGet(cmd, args)
			case contactAnyFlagChanged(cmd, "names", "name", "query", "keyword"):
				return runContactLabelGet(cmd, args)
			default:
				return runContactLabelList(cmd, args)
			}
		},
	}
	contactRootListCmd.Flags().String("depts", "", "部门 ID 列表")
	contactRootListCmd.Flags().String("id", "", "--depts 的别名")
	contactRootListCmd.Flags().String("dept-id", "", "--depts 的别名")
	contactRootListCmd.Flags().String("dept-ids", "", "--depts 的别名")
	_ = contactRootListCmd.Flags().MarkHidden("id")
	_ = contactRootListCmd.Flags().MarkHidden("dept-id")
	_ = contactRootListCmd.Flags().MarkHidden("dept-ids")
	addUserIDFlags(contactRootListCmd)
	addLabelNameFlags(contactRootListCmd)

	for _, use := range []string{"self", "me", "whoami", "get-self", "user-self", "current-user"} {
		root.AddCommand(&cobra.Command{
			Use:    use,
			Short:  "获取当前用户信息（兼容入口）",
			Hidden: true,
			RunE:   contactUserGetSelfCmd.RunE,
		})
	}
	root.AddCommand(contactRootSearchCmd, contactRootFindCmd, contactRootGetCmd, contactRootListCmd)

	contactHintSubCmd := func(use, suggestion string) *cobra.Command {
		c := hintSubCmd(use, suggestion)
		runHint := c.RunE
		c.DisableFlagParsing = true
		c.RunE = func(cmd *cobra.Command, args []string) error {
			for _, arg := range args {
				if arg == "--help" || arg == "-h" {
					return cmd.Help()
				}
			}
			return runHint(cmd, args)
		}
		return c
	}

	root.AddCommand(contactHintSubCmd("department", "use: dws contact dept [search|list-members|list-children|get-info]"))

	// hint: dws contact user find/list/info/detail → 指向 user search / user get
	userCmd.AddCommand(contactHintSubCmd("find", "use: dws contact user search --query <关键词>"))
	userCmd.AddCommand(contactHintSubCmd("list", "use: dws contact user search --query <关键词>"))
	// 针对 LLM 常混淆的 REST 风格子命名：user info / user detail / user get-info
	userCmd.AddCommand(contactHintSubCmd("info", "use: dws contact user get --ids <用户ID>  or  dws contact user get-self"))
	userCmd.AddCommand(contactHintSubCmd("detail", "use: dws contact user get --ids <用户ID>"))
	userCmd.AddCommand(contactHintSubCmd("get-info", "use: dws contact user get --ids <用户ID>"))
	// 注：me / whoami / current 现已是 get-self 的真别名（Aliases），不再注册 hintSubCmd（会与真别名冲突）。

	// hint: dws contact dept list / dept info / dept detail → 指向 list-members / list-children / get-info
	contactDeptCmd.AddCommand(contactHintSubCmd("list", "use: dws contact dept list-members --depts <部门ID>  or  dws contact dept list-children --dept <父部门ID>"))
	contactDeptCmd.AddCommand(contactHintSubCmd("info", "use: dws contact dept get-info --dept <部门ID>"))
	contactDeptCmd.AddCommand(contactHintSubCmd("detail", "use: dws contact dept get-info --dept <部门ID>"))

	// dws contact label find/search/info/detail/list-all → 真实兼容入口。
	// 注：list 已是真命令（label list），不再注册 hintSubCmd（会与真命令冲突）。
	for _, use := range []string{"find", "search", "info"} {
		cmd := &cobra.Command{Use: use, Hidden: true, RunE: runContactLabelGet}
		addLabelNameFlags(cmd)
		contactLabelCmd.AddCommand(cmd)
	}
	contactLabelDetailCmd := &cobra.Command{Use: "detail", Hidden: true, RunE: runContactLabelMembers}
	addLabelIDFlags(contactLabelDetailCmd)
	contactLabelCmd.AddCommand(contactLabelDetailCmd)
	contactLabelCmd.AddCommand(&cobra.Command{Use: "list-all", Hidden: true, RunE: runContactLabelList})

	// contact 子树统一错误兜底：任何 flag 解析失败均在尾部追加 "See '<CommandPath> --help' for usage."
	// 与 docker / kubectl / gh 的 UX 一致。unknown subcommand 由 cobra 自带 Did-You-Mean 处理。
	var attachContactHelpHint func(c *cobra.Command)
	attachContactHelpHint = func(c *cobra.Command) {
		c.SetFlagErrorFunc(func(cc *cobra.Command, err error) error {
			// 与 root 级 flagErrorWithSuggestions 保持同款尾部 hint 格式（句号结尾为全树 UX 约定）。
			msg := fmt.Sprintf("%s\nSee '%s --help' for usage.", err.Error(), cc.CommandPath())
			return errors.New(msg)
		})
		for _, sub := range c.Commands() {
			attachContactHelpHint(sub)
		}
	}
	attachContactHelpHint(root)

	return root
}

// parseCSVInts 解析逗号分隔的整数字符串为 []int64 切片，
// 去除空白并过滤无法解析的项。
func parseCSVInts(s string) []int64 {
	parts := strings.Split(s, ",")
	result := make([]int64, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			if n, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
				result = append(result, n)
			}
		}
	}
	return result
}

func parseCSVIntsStrict(s string) ([]int64, error) {
	parts := strings.Split(s, ",")
	result := make([]int64, 0, len(parts))
	for i, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			return nil, fmt.Errorf("第 %d 项为空", i+1)
		}
		n, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("第 %d 项 %q 不是整数: %w", i+1, trimmed, err)
		}
		result = append(result, n)
	}
	return result, nil
}
