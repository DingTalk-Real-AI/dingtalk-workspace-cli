package helpers

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// dws hrbrain — 组织大脑（人才池、员工档案、人才搜索）
// ──────────────────────────────────────────────────────────

// callHrbrainMCPTool wraps the generic MCP call and injects the current
// profile's corpId into tool arguments. This works around server-side
// token-claim / clientId mismatches where the gateway would otherwise
// resolve corpId to the application owner's organization instead of the
// user's selected profile.
func callHrbrainMCPTool(toolName string, args map[string]any) error {
	return callMCPTool(toolName, withCurrentCorpID(args))
}

func newHrbrainCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "hrbrain",
		Short: "组织大脑：人才池、员工档案与人才搜索",
		Long: `钉钉组织大脑（hrbrain）能力：人才池管理、员工档案查询、人才搜索与标签管理。

命令结构:
  dws hrbrain talent-pool list              人才池列表
  dws hrbrain talent-pool detail            获取人才池详情
  dws hrbrain talent-pool employees         人才池人员列表
  dws hrbrain profile metadata              查询员工档案元数据结构
  dws hrbrain profile query                 按模块批量查询员工档案数据
  dws hrbrain profile labels                获取员工标签
  dws hrbrain employee career               查询员工公司内职业历程
  dws hrbrain employee performance          查询员工绩效记录
  dws hrbrain employee search               人才搜索
  dws hrbrain employee search-structured    使用高级条件搜索人员
  dws hrbrain employee search-fields        获取高级搜索字段列表`,
		RunE: groupRunE,
	}

	// ── talent-pool: 人才池管理 ────────────────────────────────

	talentPoolCmd := &cobra.Command{Use: "talent-pool", Short: "人才池管理", RunE: groupRunE}

	talentPoolListCmd := &cobra.Command{
		Use:   "list",
		Short: "人才池列表",
		Long:  `查询人才池列表，支持按名称关键词、类型、创建人、标签等条件筛选。`,
		Example: `  dws hrbrain talent-pool list --page 1 --page-size 20
  dws hrbrain talent-pool list --keyword "储备干部" --pool-type TYPE --creator USER_ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			page, _ := cmd.Flags().GetInt("page")
			pageSize, _ := cmd.Flags().GetInt("page-size")
			toolArgs := map[string]any{
				"currentPage": page,
				"pageSize":    pageSize,
			}
			if v, _ := cmd.Flags().GetString("keyword"); v != "" {
				toolArgs["keyword"] = v
			}
			if v, _ := cmd.Flags().GetString("pool-type"); v != "" {
				toolArgs["poolType"] = v
			}
			if v, _ := cmd.Flags().GetString("creator"); v != "" {
				toolArgs["creator"] = v
			}
			if v, _ := cmd.Flags().GetString("labels"); v != "" {
				toolArgs["labels"] = parseCSVValues(v)
			}
			return callHrbrainMCPTool("list_talent_pools", toolArgs)
		},
	}
	talentPoolListCmd.Flags().String("keyword", "", "人才池名称关键词 (可选)")
	talentPoolListCmd.Flags().String("pool-type", "", "人才池类型 (可选)")
	talentPoolListCmd.Flags().String("creator", "", "创建人 (可选)")
	talentPoolListCmd.Flags().String("labels", "", "标签列表，逗号分隔 (可选)")
	talentPoolListCmd.Flags().Int("page", 1, "当前页码 (必填，默认 1)")
	talentPoolListCmd.Flags().Int("page-size", 20, "每页条数 (必填，默认 20)")

	talentPoolDetailCmd := &cobra.Command{
		Use:   "detail",
		Short: "获取人才池详情",
		Long:  `根据人才池编码获取人才池详细信息。`,
		Example: `  dws hrbrain talent-pool detail --pool-code POOL_CODE`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "pool-code"); err != nil {
				return err
			}
			return callHrbrainMCPTool("get_talent_pool_detail", map[string]any{
				"poolCode": mustGetFlag(cmd, "pool-code"),
			})
		},
	}
	talentPoolDetailCmd.Flags().String("pool-code", "", "人才池编码 (必填)")

	talentPoolEmployeesCmd := &cobra.Command{
		Use:   "employees",
		Short: "人才池人员列表",
		Long:  `查询指定人才池内的人员列表。`,
		Example: `  dws hrbrain talent-pool employees --pool-code POOL_CODE --page 1 --page-size 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "pool-code"); err != nil {
				return err
			}
			page, _ := cmd.Flags().GetInt("page")
			pageSize, _ := cmd.Flags().GetInt("page-size")
			return callHrbrainMCPTool("list_pool_employees", map[string]any{
				"poolCode":    mustGetFlag(cmd, "pool-code"),
				"currentPage": page,
				"pageSize":    pageSize,
			})
		},
	}
	talentPoolEmployeesCmd.Flags().String("pool-code", "", "人才池编码 (必填)")
	talentPoolEmployeesCmd.Flags().Int("page", 1, "当前页码 (必填，默认 1)")
	talentPoolEmployeesCmd.Flags().Int("page-size", 20, "每页条数 (必填，默认 20)")

	talentPoolCmd.AddCommand(talentPoolListCmd, talentPoolDetailCmd, talentPoolEmployeesCmd)

	// ── profile: 员工档案管理 ──────────────────────────────────

	profileCmd := &cobra.Command{Use: "profile", Short: "员工档案管理", RunE: groupRunE}

	profileMetadataCmd := &cobra.Command{
		Use:   "metadata",
		Short: "查询员工档案元数据结构",
		Long:  `查询指定员工档案的元数据结构，用于构造 query_profile_data 的 dataQueries 参数。`,
		Example: `  dws hrbrain profile metadata --work-no WORK_NO`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "work-no"); err != nil {
				return err
			}
			return callHrbrainMCPTool("get_profile_metadata", map[string]any{
				"workNo": mustGetFlag(cmd, "work-no"),
			})
		},
	}
	profileMetadataCmd.Flags().String("work-no", "", "员工工号 (必填)")

	profileQueryCmd := &cobra.Command{
		Use:   "query",
		Short: "按模块批量查询员工档案数据",
		Long: `按模块维度批量查询员工档案数据。
--data-queries 为 JSON 数组，每个元素包含:
  modelCode — 档案模块编码
  fields    — 要查询的字段编码列表`,
		Example: `  dws hrbrain profile query --work-no WORK_NO --data-queries '[{"modelCode":"basic","fields":["name","dept"]}]'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "work-no", "data-queries"); err != nil {
				return err
			}
			var queries []any
			if err := json.Unmarshal([]byte(mustGetFlag(cmd, "data-queries")), &queries); err != nil {
				return fmt.Errorf("--data-queries must be a valid JSON array: %w", err)
			}
			return callHrbrainMCPTool("query_profile_data", map[string]any{
				"workNo":      mustGetFlag(cmd, "work-no"),
				"dataQueries": queries,
			})
		},
	}
	profileQueryCmd.Flags().String("work-no", "", "目标员工工号 (必填)")
	profileQueryCmd.Flags().String("data-queries", "", "按模块查询的条件列表 JSON 数组 (必填)")

	profileLabelsCmd := &cobra.Command{
		Use:   "labels",
		Short: "获取员工标签",
		Long:  `根据员工工号列表获取员工标签。`,
		Example: `  dws hrbrain profile labels --staff-ids WORK_NO1,WORK_NO2 --all-label`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "staff-ids"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"staffIds": parseCSVValues(mustGetFlag(cmd, "staff-ids")),
			}
			if cmd.Flags().Changed("all-label") {
				v, _ := cmd.Flags().GetBool("all-label")
				toolArgs["allLabel"] = v
			}
			return callHrbrainMCPTool("get_profile_label", toolArgs)
		},
	}
	profileLabelsCmd.Flags().String("staff-ids", "", "员工工号列表，逗号分隔 (必填)")
	profileLabelsCmd.Flags().Bool("all-label", false, "是否所有标签 (可选)")

	profileCmd.AddCommand(profileMetadataCmd, profileQueryCmd, profileLabelsCmd)

	// ── employee: 员工职业历程 / 绩效 / 搜索 ─────────────────────

	employeeCmd := &cobra.Command{Use: "employee", Short: "员工职业历程、绩效与搜索", RunE: groupRunE}

	employeeCareerCmd := &cobra.Command{
		Use:   "career",
		Short: "查询员工公司内职业历程",
		Example: `  dws hrbrain employee career --work-no WORK_NO`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "work-no"); err != nil {
				return err
			}
			return callHrbrainMCPTool("get_employee_career", map[string]any{
				"workNo": mustGetFlag(cmd, "work-no"),
			})
		},
	}
	employeeCareerCmd.Flags().String("work-no", "", "员工工号 (必填)")

	employeePerformanceCmd := &cobra.Command{
		Use:   "performance",
		Short: "查询员工绩效记录",
		Example: `  dws hrbrain employee performance --work-no WORK_NO`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "work-no"); err != nil {
				return err
			}
			return callHrbrainMCPTool("get_employee_performance", map[string]any{
				"workNo": mustGetFlag(cmd, "work-no"),
			})
		},
	}
	employeePerformanceCmd.Flags().String("work-no", "", "员工工号 (必填)")

	employeeSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "人才搜索",
		Long:  `按关键词、部门、职务、职级、人才池等条件搜索人员。`,
		Example: `  dws hrbrain employee search --keyword "张三" --page 1 --page-size 20
  dws hrbrain employee search --dept-name "技术部" --job-level P7 --pool-code POOL_CODE`,
		RunE: func(cmd *cobra.Command, args []string) error {
			page, _ := cmd.Flags().GetInt("page")
			pageSize, _ := cmd.Flags().GetInt("page-size")
			toolArgs := map[string]any{
				"currentPage": page,
				"pageSize":    pageSize,
			}
			if v, _ := cmd.Flags().GetString("keyword"); v != "" {
				toolArgs["keyword"] = v
			}
			if v, _ := cmd.Flags().GetString("dept-name"); v != "" {
				toolArgs["deptName"] = v
			}
			if v, _ := cmd.Flags().GetString("position-name"); v != "" {
				toolArgs["positionName"] = v
			}
			if v, _ := cmd.Flags().GetString("job-level"); v != "" {
				toolArgs["jobLevel"] = v
			}
			if v, _ := cmd.Flags().GetString("pool-code"); v != "" {
				toolArgs["poolCode"] = v
			}
			return callHrbrainMCPTool("search_employees", toolArgs)
		},
	}
	employeeSearchCmd.Flags().String("keyword", "", "全文搜索关键词（姓名/工号等）(可选)")
	employeeSearchCmd.Flags().String("dept-name", "", "部门名称 (可选)")
	employeeSearchCmd.Flags().String("position-name", "", "职务名称 (可选)")
	employeeSearchCmd.Flags().String("job-level", "", "职级 (可选)")
	employeeSearchCmd.Flags().String("pool-code", "", "限定人才池编码 (可选)")
	employeeSearchCmd.Flags().Int("page", 1, "当前页码 (必填，默认 1)")
	employeeSearchCmd.Flags().Int("page-size", 20, "每页条数 (必填，默认 20)")

	employeeSearchStructuredCmd := &cobra.Command{
		Use:   "search-structured",
		Short: "使用高级条件搜索人员",
		Long: `使用高级条件（originJson 表达式）搜索人员。
建议先调用 "dws hrbrain employee search-fields" 获取有权限的字段与操作符列表。
--origin-json 为 JSON 字符串，例如:
  {"rules":[{"field":"name","operator":"contains","value":"张"}],"combinator":"and"}
--fields 为 JSON 数组，例如:
  [{"label":"姓名","value":"name"}]
--order-by 为逗号分隔的排序字段列表 (可选)`,
		Example: `  dws hrbrain employee search-fields
  dws hrbrain employee search-structured --origin-json '{"rules":[{"field":"name","operator":"contains","value":"张"}],"combinator":"and"}' --fields '[{"label":"姓名","value":"name"}]' --page 1 --page-size 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "origin-json", "fields"); err != nil {
				return err
			}
			page, _ := cmd.Flags().GetInt("page")
			pageSize, _ := cmd.Flags().GetInt("page-size")
			var fields []any
			if err := json.Unmarshal([]byte(mustGetFlag(cmd, "fields")), &fields); err != nil {
				return fmt.Errorf("--fields must be a valid JSON array: %w", err)
			}
			toolArgs := map[string]any{
				"originJson":  mustGetFlag(cmd, "origin-json"),
				"currentPage": page,
				"pageSize":    pageSize,
				"fields":      fields,
			}
			if v, _ := cmd.Flags().GetString("order-by"); v != "" {
				toolArgs["orderByClauses"] = parseCSVValues(v)
			}
			return callHrbrainMCPTool("search_employees_structured", toolArgs)
		},
	}
	employeeSearchStructuredCmd.Flags().String("origin-json", "", "搜索条件 JSON 表达式 (必填)")
	employeeSearchStructuredCmd.Flags().Int("page", 1, "当前页码 (必填，默认 1)")
	employeeSearchStructuredCmd.Flags().Int("page-size", 20, "每页条数 (必填，默认 20)")
	employeeSearchStructuredCmd.Flags().String("order-by", "", "排序字段列表，逗号分隔 (可选)")
	employeeSearchStructuredCmd.Flags().String("fields", "", "返回列定义 JSON 数组 (必填)")

	employeeSearchFieldsCmd := &cobra.Command{
		Use:   "search-fields",
		Short: "获取高级搜索字段列表",
		Long:  `获取当前操作人有权限使用的高级搜索字段列表，用于构造 search-structured 的参数。`,
		Example: `  dws hrbrain employee search-fields`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callHrbrainMCPTool("get_search_fields", nil)
		},
	}

	employeeCmd.AddCommand(employeeCareerCmd, employeePerformanceCmd, employeeSearchCmd, employeeSearchStructuredCmd, employeeSearchFieldsCmd)

	root.AddCommand(talentPoolCmd, profileCmd, employeeCmd)

	return root
}
