package helpers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
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

func newContactCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "contact",
		Short: "通讯录 / 用户 / 部门 / 人员关系",
		Long: `查询钉钉通讯录：用户搜索、手机号查找、部门搜索、子部门 / 成员列表、人员关系；用户花名册档案信息（学历、家庭、银行卡、合同等）与离职员工信息。

通讯录功能：
  - contact user get-self/search/search-mobile/get: 通讯录用户查询
  - contact dept search/get-info/list-children/list-members: 部门查询
  - contact relation list-my-followings: 特别关注人查询

基础人事功能（HR 花名册）：
  - contact user profile fields/get: 员工花名册档案查询（学历、家庭、银行卡等）
  - contact user dismission search: 离职员工列表查询`,
		RunE: groupRunE,
	}

	userCmd := &cobra.Command{
		Use:   "user",
		Short: "人员查询",
		Long: `人员查询：通讯录用户查询、用户档案（花名册）查询、离职员工查询。

【何时用哪个命令】
  - 查询用户的部门、主管、管理员权限       → contact user get
  - 查询用户的学历、家庭、银行卡、合同等档案 → contact user profile get
  - 查询离职员工列表                       → contact user dismission search`,
		RunE: groupRunE,
	}

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

	relationCmd := &cobra.Command{Use: "relation",
		Short: "人员关系查询",
		Long:  `查询钉钉人员关系：特别关注人。`,
		RunE:  groupRunE}

	contactRelationListMyFollowingsCmd := &cobra.Command{
		Use:     "list-my-followings",
		Short:   "获取当前用户的特别关注列表",
		Example: `  dws contact relation list-my-followings`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("list_my_followings", nil)
		},
	}

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

	// ── label 角色 ──────────────────────────────────────────────────

	contactLabelCmd := &cobra.Command{
		Use:    "label",
		Short:  "角色查询",
		Hidden: true,
		Long: `角色查询后端工具已下线，保留该命令仅用于兼容旧指令。

请改用 aisearch person 按职责维度找人，例如：
  dws aisearch person --keyword "管理员" --dimension duty`,
		RunE: groupRunE,
	}
	contactLabelUnavailable := func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("contact label 角色查询能力当前不可用：后端工具已下线\n  hint: 请改用 dws aisearch person --keyword <角色或职责> --dimension duty")
	}

	contactLabelGetCmd := &cobra.Command{
		Use:    "get",
		Short:  "根据角色名称查询角色",
		Hidden: true,
		Long:   `contact label 后端工具已下线。请改用 aisearch person --dimension duty 按职责维度找人。`,
		Example: `  dws contact label get --names "管理员"
  dws contact label get --names "管理员,财务"`,
		RunE: contactLabelUnavailable,
	}

	contactLabelListMembersCmd := &cobra.Command{
		Use:     "list-members",
		Short:   "查询角色下的成员",
		Hidden:  true,
		Long:    `contact label 后端工具已下线。请改用 aisearch person --dimension duty 按职责维度找人。`,
		Example: `  dws contact label list-members --id 12345  # 查询 labelId: dws contact label get --names "角色名"`,
		RunE:    contactLabelUnavailable,
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
		Use:     "list",
		Short:   "获取企业所有角色列表",
		Hidden:  true,
		Long:    `contact label 后端工具已下线。请改用 aisearch person --dimension duty 按职责维度找人。`,
		Example: `  dws contact label list`,
		RunE:    contactLabelUnavailable,
	}

	contactLabelCmd.AddCommand(contactLabelListAllCmd, contactLabelGetCmd, contactLabelListMembersCmd)

	contactDeptCmd := &cobra.Command{Use: "dept", Short: "部门查询", RunE: groupRunE}

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

	// ── user profile 用户档案（花名册） ────────────────────────────────────
	contactUserProfileCmd := &cobra.Command{
		Use:   "profile",
		Short: "用户档案（花名册）",
		Long: `用户档案（花名册）：查询花名册字段列表、查询员工花名册字段信息。

花名册字段包含：试用/转正信息、个人/家庭信息、学历信息、银行卡/合同信息、
紧急联系人和其他企业自定义信息。

【与 contact user get 的区别】
  - contact user get: 组织管理信息（部门、主管、管理员权限）
  - contact user profile get: 个人档案信息（学历、家庭、银行卡等）`,
		RunE: groupRunE,
	}

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
	contactUserProfileGetCmd.Flags().String("staff-id", "", "查询员工 ID（可选）")
	contactUserProfileGetCmd.Flags().String("fields", "", "指定字段集合, 逗号分隔, 可通过 profile fields 获取（可选）")

	contactUserProfileCmd.AddCommand(contactUserProfileFieldsCmd, contactUserProfileGetCmd)

	// ── user dismission 离职员工 ───────────────────────────────────────────
	contactUserDismissionCmd := &cobra.Command{
		Use:   "dismission",
		Short: "离职员工查询",
		Long:  `离职员工查询：分页获取离职员工列表，支持按员工姓名、离职时间范围、部门进行过滤。`,
		RunE:  groupRunE,
	}

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
	contactUserDismissionSearchCmd.Flags().String("name", "", "员工姓名，模糊搜索（可选）")
	contactUserDismissionSearchCmd.Flags().String("start", "", "离职日期查询范围开始，格式 YYYY-MM-DD（可选），与end要么都不填要么都填")
	contactUserDismissionSearchCmd.Flags().String("end", "", "离职日期查询范围结束，格式 YYYY-MM-DD（可选），与start要么都不填要么都填")
	contactUserDismissionSearchCmd.Flags().String("depts", "", "部门 ID 列表，逗号分隔（可选）")
	contactUserDismissionSearchCmd.Flags().Bool("hide-retirement", true, "是否隐藏退休，默认 true（可选）")
	contactUserDismissionSearchCmd.Flags().Bool("hide-partner", false, "是否隐藏合作伙伴，默认 false（可选）")
	contactUserDismissionSearchCmd.Flags().Int("page", 1, "页码，从 1 开始（可选）")
	contactUserDismissionSearchCmd.Flags().Int("limit", 20, "页大小，200 以内（可选）")

	contactUserDismissionCmd.AddCommand(contactUserDismissionSearchCmd)

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
		contactUserProfileCmd,    // 花名册档案
		contactUserDismissionCmd, // 离职员工
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
	for _, s := range []deptIDAliasSpec{
		{contactDeptGetInfoCmd, []string{"id", "dept-id", "ids", "dept-ids"}},
		{contactDeptListChildrenCmd, []string{"id", "ids", "dept-id", "dept-ids"}},
		{contactDeptListMembersCmd, []string{"ids", "id", "dept-id", "dept-ids"}},
	} {
		for _, name := range s.aliases {
			if s.cmd.Flags().Lookup(name) != nil {
				continue // 已是主 flag，跳过
			}
			s.cmd.Flags().String(name, "", "部门 ID 别名（等价于当前命令的主 flag）")
			_ = s.cmd.Flags().MarkHidden(name)
		}
	}
	contactDeptCmd.AddCommand(contactDeptSearchCmd, contactDeptGetInfoCmd, contactDeptListChildrenCmd, contactDeptListMembersCmd)

	relationCmd.AddCommand(contactRelationListMyFollowingsCmd)
	root.AddCommand(userCmd, contactDeptCmd, contactLabelCmd, relationCmd)

	// hint: dws contact search → dws contact user search / dept search
	//
	// 使用本地 wrapper contactHintSubCmd，在 cmdutil.HintSubCmd 基础上开启 DisableFlagParsing：
	// 这样 `dws contact get --user-id X` 这样带未知 flag 的误输入也会进入 RunE，
	// 输出 `hint: use: dws contact user get --ids <用户ID>` 等正向引导，
	// 而不是被 cobra 在 flag 解析阶段拦截在 `unknown flag: --user-id`。
	// 类似做法见 calendar.go 的 calendarInfoHintSubCmd。
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

	root.AddCommand(contactHintSubCmd("search", "use: dws contact user search\n  also available: dws contact dept search / dws aisearch person --dimension duty"))

	// hint: dws contact find/list/get → 指向正确子命令
	root.AddCommand(contactHintSubCmd("find", "use: dws contact user search --query <关键词>  or  dws contact dept search --query <关键词>  or  dws aisearch person --keyword <角色或职责> --dimension duty"))
	root.AddCommand(contactHintSubCmd("list", "use: dws contact dept list-members --depts <部门ID>  or  dws contact user get --ids <用户ID>"))
	root.AddCommand(contactHintSubCmd("get", "use: dws contact user get --ids <用户ID>  or  dws contact dept get-info --dept <部门ID>"))

	// hint: dws contact self/me/whoami → user get-self（当前用户语义）
	root.AddCommand(contactHintSubCmd("self", "use: dws contact user get-self"))
	root.AddCommand(contactHintSubCmd("me", "use: dws contact user get-self"))
	root.AddCommand(contactHintSubCmd("whoami", "use: dws contact user get-self"))

	// 顶层高频误写：get-self / user-self / current-user / department
	root.AddCommand(contactHintSubCmd("get-self", "use: dws contact user get-self"))
	root.AddCommand(contactHintSubCmd("user-self", "use: dws contact user get-self"))
	root.AddCommand(contactHintSubCmd("current-user", "use: dws contact user get-self"))
	root.AddCommand(contactHintSubCmd("department", "use: dws contact dept [search|list-members|list-children|get-info]"))
	root.AddCommand(contactHintSubCmd("role", "contact label 已下线; use: dws aisearch person --keyword <角色或职责> --dimension duty"))

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

	// hint: dws contact label find/search/info/detail → 指向 aisearch 替代路径。
	// 注：list 已是真命令（label list），不再注册 hintSubCmd（会与真命令冲突）。
	contactLabelCmd.AddCommand(contactHintSubCmd("find", "contact label 已下线; use: dws aisearch person --keyword <角色或职责> --dimension duty"))
	contactLabelCmd.AddCommand(contactHintSubCmd("search", "contact label 已下线; use: dws aisearch person --keyword <角色或职责> --dimension duty"))
	contactLabelCmd.AddCommand(contactHintSubCmd("info", "contact label 已下线; use: dws aisearch person --keyword <角色或职责> --dimension duty"))
	contactLabelCmd.AddCommand(contactHintSubCmd("detail", "contact label 已下线; use: dws aisearch person --keyword <角色或职责> --dimension duty"))
	contactLabelCmd.AddCommand(contactHintSubCmd("list-all", "contact label 已下线; use: dws aisearch person --keyword <角色或职责> --dimension duty"))

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
