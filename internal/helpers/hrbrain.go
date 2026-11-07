package helpers

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

// ──────────────────────────────────────────────────────────
// dws hrbrain — 组织大脑（人才池、员工档案、人才搜索）
// ──────────────────────────────────────────────────────────

func newHrbrainCommand() *cobra.Command {
	// Product-level Agent routing Decl (migrated from selection/hrbrain.json
	// products.hrbrain). Catalog assembly stamps provenance contract_final.
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "hrbrain",
		HelpReferences: contract.HelpReferences{
			RelatedSkills: []string{"dingtalk-misc"},
			Documentation: []contract.HelpDocumentation{
				contract.SkillDocumentation("组织大脑深度指南", "dingtalk-misc", "references/hrbrain.md"),
			},
		},
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "钉钉组织大脑：人才池管理、员工档案查询与人才搜索",
			UseWhen: []string{
				"需要查询人才池、员工档案（元数据/标签/职业历程/绩效）或搜索员工时",
			},
			AvoidWhen: []string{
				"要操作通讯录/组织架构基础信息时改用 contact 产品",
				"要提交/查询战略解码、经营合约、目标等 OKR 能力时改用 agoal（CLI-only，未接入 Agent Schema）",
			},
		},
	})
	root := newGroupCommand(&cobra.Command{
		Use:   "hrbrain",
		Short: "组织大脑：人才池、员工档案与人才搜索",
		Long: `钉钉组织大脑（hrbrain）能力：人才池管理、员工档案查询、人才搜索与标签管理。

命令结构:
  dws hrbrain talent-pool list              人才池列表
  dws hrbrain talent-pool detail            获取人才池详情
  dws hrbrain talent-pool employees         人才池人员列表
  dws hrbrain talent-pool save              创建或更新人才池
  dws hrbrain talent-pool move-members      人才池人员出入池
  dws hrbrain profile metadata              查询员工档案元数据结构
  dws hrbrain profile query                 按模块批量查询员工档案数据
  dws hrbrain profile labels                获取员工标签
  dws hrbrain profile career                查询员工公司内职业历程
  dws hrbrain profile performance           查询员工绩效记录
  dws hrbrain search employees              人才搜索
  dws hrbrain search employees-structured   使用高级条件搜索人员
  dws hrbrain search fields                 获取高级搜索字段列表`,
		RunE: groupRunE,
	})

	// ── talent-pool: 人才池管理 ────────────────────────────────

	talentPoolCmd := newGroupCommand(&cobra.Command{Use: "talent-pool", Short: "人才池管理", RunE: groupRunE})

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
			return callMCPTool("list_talent_pools", toolArgs)
		},
	}
	DeclareLeafMetadata(talentPoolListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "hrbrain",
				Name:           "list_talent_pools",
				CanonicalPath:  "hrbrain.list_talent_pools",
				CLIPath:        "hrbrain talent-pool list",
				PrimaryCLIPath: "hrbrain talent-pool list",
			},
			Description: "查询人才池列表，支持按名称关键词、类型、创建人、标签筛选",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls the hrbrain MCP tool list_talent_pools, which is absent from the pinned MCP metadata snapshot; no single pinned interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询人才池列表，支持按名称关键词、类型、创建人、标签筛选",
				UseWhen:      []string{"需要列出组织中的人才池，或按名称/类型/创建人/标签筛选人才池时"},
				AvoidWhen: []string{
					"已知 poolCode 只需要单个人才池详情时改用 dws hrbrain talent-pool detail",
					"要查看人才池内人员名单时改用 dws hrbrain talent-pool employees",
				},
				Examples: []string{
					"dws hrbrain talent-pool list --page 1 --page-size 20",
					"dws hrbrain talent-pool list --keyword \"储备干部\"",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "creator", Property: "creator", Required: boolPtr(false)},
				{Name: "keyword", Property: "keyword", Required: boolPtr(false)},
				{Name: "labels", Property: "labels", Required: boolPtr(false)},
				{Name: "page", Property: "currentPage", Required: boolPtr(false)},
				{Name: "page-size", Property: "pageSize", Required: boolPtr(false)},
				{Name: "pool-type", Property: "poolType", Required: boolPtr(false)},
			},
		},
	})
	talentPoolListCmd.Flags().String("keyword", "", "人才池名称关键词 (可选)")
	talentPoolListCmd.Flags().String("pool-type", "", "人才池类型 (可选)")
	talentPoolListCmd.Flags().String("creator", "", "创建人 (可选)")
	talentPoolListCmd.Flags().String("labels", "", "标签列表，逗号分隔 (可选)")
	talentPoolListCmd.Flags().Int("page", 1, "当前页码 (默认 1)")
	talentPoolListCmd.Flags().Int("page-size", 20, "每页条数 (默认 20)")

	talentPoolDetailCmd := &cobra.Command{
		Use:     "detail",
		Short:   "获取人才池详情",
		Long:    `根据人才池编码获取人才池详细信息。`,
		Example: `  dws hrbrain talent-pool detail --pool-code POOL_CODE`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "pool-code"); err != nil {
				return err
			}
			return callMCPTool("get_talent_pool_detail", map[string]any{
				"poolCode": mustGetFlag(cmd, "pool-code"),
			})
		},
	}
	DeclareLeafMetadata(talentPoolDetailCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "hrbrain",
				Name:           "get_talent_pool_detail",
				CanonicalPath:  "hrbrain.get_talent_pool_detail",
				CLIPath:        "hrbrain talent-pool detail",
				PrimaryCLIPath: "hrbrain talent-pool detail",
			},
			Description: "根据人才池编码获取人才池详细信息",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls the hrbrain MCP tool get_talent_pool_detail, which is absent from the pinned MCP metadata snapshot; no single pinned interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "根据人才池编码获取人才池详细信息",
				UseWhen:      []string{"已知 poolCode，需要查看某个人才池的详细信息时"},
				AvoidWhen:    []string{"尚未取得 poolCode 时先用 dws hrbrain talent-pool list 查找"},
				Examples:     []string{"dws hrbrain talent-pool detail --pool-code POOL_CODE"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "pool-code", Property: "poolCode", Required: boolPtr(true)},
			},
		},
	})
	talentPoolDetailCmd.Flags().String("pool-code", "", "人才池编码 (必填)")

	talentPoolEmployeesCmd := &cobra.Command{
		Use:     "employees",
		Short:   "人才池人员列表",
		Long:    `查询指定人才池内的人员列表。`,
		Example: `  dws hrbrain talent-pool employees --pool-code POOL_CODE --page 1 --page-size 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "pool-code"); err != nil {
				return err
			}
			page, _ := cmd.Flags().GetInt("page")
			pageSize, _ := cmd.Flags().GetInt("page-size")
			return callMCPTool("list_pool_employees", map[string]any{
				"poolCode":    mustGetFlag(cmd, "pool-code"),
				"currentPage": page,
				"pageSize":    pageSize,
			})
		},
	}
	DeclareLeafMetadata(talentPoolEmployeesCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "hrbrain",
				Name:           "list_pool_employees",
				CanonicalPath:  "hrbrain.list_pool_employees",
				CLIPath:        "hrbrain talent-pool employees",
				PrimaryCLIPath: "hrbrain talent-pool employees",
			},
			Description: "查询指定人才池内的人员列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls the hrbrain MCP tool list_pool_employees, which is absent from the pinned MCP metadata snapshot; no single pinned interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询指定人才池内的人员列表",
				UseWhen:      []string{"已知 poolCode，需要查看该人才池内的人员名单时"},
				AvoidWhen: []string{
					"尚未取得 poolCode 时先用 dws hrbrain talent-pool list 查找",
					"不限定人才池的人员搜索请改用 dws hrbrain search employees",
				},
				Examples: []string{"dws hrbrain talent-pool employees --pool-code POOL_CODE --page 1 --page-size 20"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "page", Property: "currentPage", Required: boolPtr(false)},
				{Name: "page-size", Property: "pageSize", Required: boolPtr(false)},
				{Name: "pool-code", Property: "poolCode", Required: boolPtr(true)},
			},
		},
	})
	talentPoolEmployeesCmd.Flags().String("pool-code", "", "人才池编码 (必填)")
	talentPoolEmployeesCmd.Flags().Int("page", 1, "当前页码 (默认 1)")
	talentPoolEmployeesCmd.Flags().Int("page-size", 20, "每页条数 (默认 20)")

	talentPoolSaveCmd := &cobra.Command{
		Use:   "save",
		Short: "创建或更新人才池",
		Long: `创建或更新人才池。不传 --pool-code 为新建，传 --pool-code 为更新指定人才池。
--rule-json 为自动出入池规则 JSON 对象字符串 (可选)。
--pool-tags 为人才池标识 JSON 数组 (可选)，每项含 label 与 setting{color,backgroundColor}。
该写操作执行前需要确认，自动化场景在用户明确授权后传 --yes。`,
		Example: `  dws hrbrain talent-pool save --pool-name "储备干部池"
  dws hrbrain talent-pool save --pool-code POOL_CODE --pool-name "储备干部池" --pool-desc "描述"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "pool-name"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"poolName": mustGetFlag(cmd, "pool-name"),
			}
			if v, _ := cmd.Flags().GetString("pool-code"); v != "" {
				toolArgs["poolCode"] = v
			}
			if v, _ := cmd.Flags().GetString("pool-desc"); v != "" {
				toolArgs["poolDesc"] = v
			}
			if v, _ := cmd.Flags().GetString("rule-json"); v != "" {
				var rule map[string]any
				if err := json.Unmarshal([]byte(v), &rule); err != nil {
					return fmt.Errorf("--rule-json must be a valid JSON object: %w", err)
				}
				toolArgs["ruleJson"] = v
			}
			if v, _ := cmd.Flags().GetString("pool-tags"); v != "" {
				var tags []any
				if err := json.Unmarshal([]byte(v), &tags); err != nil {
					return fmt.Errorf("--pool-tags must be a valid JSON array: %w", err)
				}
				if len(tags) == 0 {
					return fmt.Errorf("--pool-tags must be a non-empty JSON array")
				}
				toolArgs["poolTags"] = tags
			}
			return callMCPTool("create_or_update_pool", toolArgs)
		},
	}
	DeclareLeafMetadata(talentPoolSaveCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "hrbrain",
				Name:           "create_or_update_pool",
				CanonicalPath:  "hrbrain.create_or_update_pool",
				CLIPath:        "hrbrain talent-pool save",
				PrimaryCLIPath: "hrbrain talent-pool save",
			},
			Description: "创建或更新人才池：不传 --pool-code 为新建，传 --pool-code 为更新",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls the hrbrain MCP tool create_or_update_pool, which is absent from the pinned MCP metadata snapshot; no single pinned interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建新人才池或更新已有人才池的名称、描述、自动出入池规则与标识",
				UseWhen:      []string{"用户明确要新建人才池（不传 --pool-code），或修改已有人才池（传 --pool-code）的名称/描述/规则/标识时"},
				AvoidWhen: []string{
					"只是查看人才池列表或详情时改用 dws hrbrain talent-pool list / talent-pool detail",
					"只是把人员移入/移出人才池时改用 dws hrbrain talent-pool move-members",
				},
				Examples: []string{
					"dws hrbrain talent-pool save --pool-name \"储备干部池\"",
					"dws hrbrain talent-pool save --pool-code POOL_CODE --pool-name \"储备干部池\" --pool-desc \"描述\"",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "pool-code", Property: "poolCode", Required: boolPtr(false), Description: "人才池编码；新建时留空，更新时必传"},
				{Name: "pool-desc", Property: "poolDesc", Required: boolPtr(false)},
				{Name: "pool-name", Property: "poolName", Required: boolPtr(true)},
				{Name: "pool-tags", Property: "poolTags", Required: boolPtr(false), InterfaceType: "array"},
				{Name: "rule-json", Property: "ruleJson", Required: boolPtr(false)},
			},
		},
	})
	talentPoolSaveCmd.Flags().String("pool-name", "", "人才池名称 (必填)")
	talentPoolSaveCmd.Flags().String("pool-code", "", "人才池编码；更新时传入，新建时留空 (可选)")
	talentPoolSaveCmd.Flags().String("pool-desc", "", "人才池描述 (可选)")
	talentPoolSaveCmd.Flags().String("rule-json", "", "自动出入池规则 JSON 对象字符串 (可选)")
	talentPoolSaveCmd.Flags().String("pool-tags", "", "人才池标识 JSON 数组 (可选)")

	talentPoolMoveMembersCmd := &cobra.Command{
		Use:   "move-members",
		Short: "人才池人员出入池",
		Long: `将人员批量移入或移出指定人才池。
--opt-type ENTERING 入池，LEAVING 出池。
--staff-ids 为逗号分隔的人员工号列表。
该写操作执行前需要确认，自动化场景在用户明确授权后传 --yes。`,
		Example: `  dws hrbrain talent-pool move-members --pool-code POOL_CODE --opt-type ENTERING --staff-ids WORK_NO1,WORK_NO2
  dws hrbrain talent-pool move-members --pool-code POOL_CODE --opt-type LEAVING --staff-ids WORK_NO1 --remark "转岗"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "pool-code", "opt-type", "staff-ids"); err != nil {
				return err
			}
			optType := mustGetFlag(cmd, "opt-type")
			switch optType {
			case "ENTERING", "LEAVING":
			default:
				return fmt.Errorf("--opt-type 仅支持 ENTERING（入池）或 LEAVING（出池），当前值 %q", optType)
			}
			staffIDs := parseCSVValues(mustGetFlag(cmd, "staff-ids"))
			if len(staffIDs) == 0 {
				return fmt.Errorf("--staff-ids 至少需要一个人员工号")
			}
			toolArgs := map[string]any{
				"poolCode": mustGetFlag(cmd, "pool-code"),
				"optType":  optType,
				"staffIds": staffIDs,
			}
			if v, _ := cmd.Flags().GetString("remark"); v != "" {
				toolArgs["remark"] = v
			}
			return callMCPTool("entering_or_leaving_pool", toolArgs)
		},
	}
	DeclareLeafMetadata(talentPoolMoveMembersCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "non_idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "hrbrain",
				Name:           "entering_or_leaving_pool",
				CanonicalPath:  "hrbrain.entering_or_leaving_pool",
				CLIPath:        "hrbrain talent-pool move-members",
				PrimaryCLIPath: "hrbrain talent-pool move-members",
			},
			Description: "将人员批量移入（ENTERING）或移出（LEAVING）指定人才池",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls the hrbrain MCP tool entering_or_leaving_pool, which is absent from the pinned MCP metadata snapshot; no single pinned interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "将一批人员批量移入或移出指定人才池",
				UseWhen:      []string{"已知 poolCode 与人员工号，需要把他们批量入池（ENTERING）或出池（LEAVING）时"},
				AvoidWhen: []string{
					"尚未取得 poolCode 时先用 dws hrbrain talent-pool list 查找",
					"只是查看人才池内人员名单时改用 dws hrbrain talent-pool employees",
				},
				Examples: []string{
					"dws hrbrain talent-pool move-members --pool-code POOL_CODE --opt-type ENTERING --staff-ids WORK_NO1,WORK_NO2",
					"dws hrbrain talent-pool move-members --pool-code POOL_CODE --opt-type LEAVING --staff-ids WORK_NO1",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "opt-type", Property: "optType", Required: boolPtr(true), Enum: []string{"ENTERING", "LEAVING"}, Description: "操作类型：ENTERING 入池，LEAVING 出池"},
				{Name: "pool-code", Property: "poolCode", Required: boolPtr(true)},
				{Name: "remark", Property: "remark", Required: boolPtr(false)},
				{Name: "staff-ids", Property: "staffIds", Required: boolPtr(true), InterfaceType: "array"},
			},
		},
	})
	talentPoolMoveMembersCmd.Flags().String("pool-code", "", "人才池编码 (必填)")
	talentPoolMoveMembersCmd.Flags().String("opt-type", "", "操作类型：ENTERING 入池 / LEAVING 出池 (必填)")
	talentPoolMoveMembersCmd.Flags().String("staff-ids", "", "出入池人员工号列表，逗号分隔 (必填)")
	talentPoolMoveMembersCmd.Flags().String("remark", "", "操作备注 (可选)")

	talentPoolCmd.AddCommand(talentPoolListCmd, talentPoolDetailCmd, talentPoolEmployeesCmd, talentPoolSaveCmd, talentPoolMoveMembersCmd)

	// ── profile: 员工档案管理 ──────────────────────────────────

	profileCmd := newGroupCommand(&cobra.Command{Use: "profile", Short: "员工档案管理", RunE: groupRunE})

	profileMetadataCmd := &cobra.Command{
		Use:     "metadata",
		Short:   "查询员工档案元数据结构",
		Long:    `查询指定员工档案的元数据结构，用于构造 query_profile_data 的 dataQueries 参数。`,
		Example: `  dws hrbrain profile metadata --work-no WORK_NO`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "work-no"); err != nil {
				return err
			}
			return callMCPTool("get_profile_metadata", map[string]any{
				"workNo": mustGetFlag(cmd, "work-no"),
			})
		},
	}
	DeclareLeafMetadata(profileMetadataCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "hrbrain",
				Name:           "get_profile_metadata",
				CanonicalPath:  "hrbrain.get_profile_metadata",
				CLIPath:        "hrbrain profile metadata",
				PrimaryCLIPath: "hrbrain profile metadata",
			},
			Description: "查询员工档案元数据结构，用于构造档案数据查询参数",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls the hrbrain MCP tool get_profile_metadata, which is absent from the pinned MCP metadata snapshot; no single pinned interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询员工档案元数据结构，用于构造档案数据查询参数",
				UseWhen:      []string{"需要先了解某员工档案有哪些模块/字段，以构造 hrbrain profile query 的 --data-queries 参数时"},
				AvoidWhen:    []string{"已知模块与字段编码，直接查询档案数据时改用 dws hrbrain profile query"},
				Examples:     []string{"dws hrbrain profile metadata --work-no WORK_NO"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "work-no", Property: "workNo", Required: boolPtr(true)},
			},
		},
	})
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
			if len(queries) == 0 {
				return fmt.Errorf("--data-queries must be a non-empty JSON array")
			}
			return callMCPTool("query_profile_data", map[string]any{
				"workNo":      mustGetFlag(cmd, "work-no"),
				"dataQueries": queries,
			})
		},
	}
	DeclareLeafMetadata(profileQueryCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "hrbrain",
				Name:           "query_profile_data",
				CanonicalPath:  "hrbrain.query_profile_data",
				CLIPath:        "hrbrain profile query",
				PrimaryCLIPath: "hrbrain profile query",
			},
			Description: "按模块批量查询员工档案数据",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls the hrbrain MCP tool query_profile_data, which is absent from the pinned MCP metadata snapshot; no single pinned interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "按模块批量查询员工档案数据",
				UseWhen:      []string{"已知模块与字段编码（通常先用 hrbrain profile metadata 获取），需要批量查询员工档案数据时"},
				AvoidWhen: []string{
					"尚不清楚可查询的模块/字段时先用 dws hrbrain profile metadata",
					"只需要职业历程或绩效记录时改用 dws hrbrain profile career / profile performance",
				},
				Examples: []string{"dws hrbrain profile query --work-no WORK_NO --data-queries '[{\"modelCode\":\"basic\",\"fields\":[\"name\",\"dept\"]}]'"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "data-queries", Property: "dataQueries", Required: boolPtr(true)},
				{Name: "work-no", Property: "workNo", Required: boolPtr(true)},
			},
		},
	})
	profileQueryCmd.Flags().String("work-no", "", "目标员工工号 (必填)")
	profileQueryCmd.Flags().String("data-queries", "", "按模块查询的条件列表 JSON 数组 (必填)")

	profileLabelsCmd := &cobra.Command{
		Use:     "labels",
		Short:   "获取员工标签",
		Long:    `根据员工工号列表获取员工标签。`,
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
			return callMCPTool("get_profile_label", toolArgs)
		},
	}
	DeclareLeafMetadata(profileLabelsCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "hrbrain",
				Name:           "get_profile_label",
				CanonicalPath:  "hrbrain.get_profile_label",
				CLIPath:        "hrbrain profile labels",
				PrimaryCLIPath: "hrbrain profile labels",
			},
			Description: "根据员工工号列表获取员工标签",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls the hrbrain MCP tool get_profile_label, which is absent from the pinned MCP metadata snapshot; no single pinned interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "根据员工工号列表获取员工标签",
				UseWhen:      []string{"需要按一个或多个工号批量查看员工标签时"},
				AvoidWhen:    []string{"要查看职业历程或绩效记录时改用 dws hrbrain profile career / profile performance"},
				Examples:     []string{"dws hrbrain profile labels --staff-ids WORK_NO1,WORK_NO2"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "all-label", Property: "allLabel", Required: boolPtr(false)},
				{Name: "staff-ids", Property: "staffIds", Required: boolPtr(true)},
			},
		},
	})
	profileLabelsCmd.Flags().String("staff-ids", "", "员工工号列表，逗号分隔 (必填)")
	profileLabelsCmd.Flags().Bool("all-label", false, "是否所有标签 (可选)")

	profileCareerCmd := &cobra.Command{
		Use:     "career",
		Short:   "查询员工公司内职业历程",
		Example: `  dws hrbrain profile career --work-no WORK_NO`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "work-no"); err != nil {
				return err
			}
			return callMCPTool("get_employee_career", map[string]any{
				"workNo": mustGetFlag(cmd, "work-no"),
			})
		},
	}
	DeclareLeafMetadata(profileCareerCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "hrbrain",
				Name:           "get_employee_career",
				CanonicalPath:  "hrbrain.get_employee_career",
				CLIPath:        "hrbrain profile career",
				PrimaryCLIPath: "hrbrain profile career",
			},
			Description: "查询员工在公司内的职业历程",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls the hrbrain MCP tool get_employee_career, which is absent from the pinned MCP metadata snapshot; no single pinned interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询员工在公司内的职业历程",
				UseWhen:      []string{"需要查看某员工在公司内的岗位/职级变动历史时"},
				AvoidWhen:    []string{"要查看绩效记录时改用 dws hrbrain profile performance"},
				Examples:     []string{"dws hrbrain profile career --work-no WORK_NO"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "work-no", Property: "workNo", Required: boolPtr(true)},
			},
		},
	})
	profileCareerCmd.Flags().String("work-no", "", "员工工号 (必填)")

	profilePerformanceCmd := &cobra.Command{
		Use:     "performance",
		Short:   "查询员工绩效记录",
		Example: `  dws hrbrain profile performance --work-no WORK_NO`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "work-no"); err != nil {
				return err
			}
			return callMCPTool("get_employee_performance", map[string]any{
				"workNo": mustGetFlag(cmd, "work-no"),
			})
		},
	}
	DeclareLeafMetadata(profilePerformanceCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "hrbrain",
				Name:           "get_employee_performance",
				CanonicalPath:  "hrbrain.get_employee_performance",
				CLIPath:        "hrbrain profile performance",
				PrimaryCLIPath: "hrbrain profile performance",
			},
			Description: "查询员工绩效记录",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls the hrbrain MCP tool get_employee_performance, which is absent from the pinned MCP metadata snapshot; no single pinned interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询员工绩效记录",
				UseWhen:      []string{"需要查看某员工的历史绩效评级/记录时"},
				AvoidWhen:    []string{"要查看职业历程时改用 dws hrbrain profile career"},
				Examples:     []string{"dws hrbrain profile performance --work-no WORK_NO"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "work-no", Property: "workNo", Required: boolPtr(true)},
			},
		},
	})
	profilePerformanceCmd.Flags().String("work-no", "", "员工工号 (必填)")

	profileCmd.AddCommand(profileMetadataCmd, profileQueryCmd, profileLabelsCmd, profileCareerCmd, profilePerformanceCmd)

	// ── search: 人才搜索 ─────────────────────────────────────

	searchCmd := newGroupCommand(&cobra.Command{Use: "search", Short: "人才搜索", RunE: groupRunE})

	employeeSearchCmd := &cobra.Command{
		Use:   "employees",
		Short: "人才搜索",
		Long:  `按关键词、部门、职务、职级、人才池等条件搜索人员。`,
		Example: `  dws hrbrain search employees --keyword "张三" --page 1 --page-size 20
  dws hrbrain search employees --dept-name "技术部" --job-level P7 --pool-code POOL_CODE`,
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
			return callMCPTool("search_employees", toolArgs)
		},
	}
	DeclareLeafMetadata(employeeSearchCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "hrbrain",
				Name:           "search_employees",
				CanonicalPath:  "hrbrain.search_employees",
				CLIPath:        "hrbrain search employees",
				PrimaryCLIPath: "hrbrain search employees",
			},
			Description: "按关键词、部门、职务、职级、人才池等条件搜索员工",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls the hrbrain MCP tool search_employees, which is absent from the pinned MCP metadata snapshot; no single pinned interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "按关键词、部门、职务、职级、人才池等条件搜索员工",
				UseWhen:      []string{"需要按姓名/工号/部门/职务/职级等基础条件模糊找人时"},
				AvoidWhen: []string{
					"需要复杂组合条件（表达式）搜索时改用 dws hrbrain search employees-structured",
					"已限定某个人才池要看全部人员时改用 dws hrbrain talent-pool employees",
				},
				Examples: []string{
					"dws hrbrain search employees --keyword \"张三\" --page 1 --page-size 20",
					"dws hrbrain search employees --dept-name \"技术部\" --job-level P7",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "dept-name", Property: "deptName", Required: boolPtr(false)},
				{Name: "job-level", Property: "jobLevel", Required: boolPtr(false)},
				{Name: "keyword", Property: "keyword", Required: boolPtr(false)},
				{Name: "page", Property: "currentPage", Required: boolPtr(false)},
				{Name: "page-size", Property: "pageSize", Required: boolPtr(false)},
				{Name: "pool-code", Property: "poolCode", Required: boolPtr(false)},
				{Name: "position-name", Property: "positionName", Required: boolPtr(false)},
			},
		},
	})
	employeeSearchCmd.Flags().String("keyword", "", "全文搜索关键词（姓名/工号等）(可选)")
	employeeSearchCmd.Flags().String("dept-name", "", "部门名称 (可选)")
	employeeSearchCmd.Flags().String("position-name", "", "职务名称 (可选)")
	employeeSearchCmd.Flags().String("job-level", "", "职级 (可选)")
	employeeSearchCmd.Flags().String("pool-code", "", "限定人才池编码 (可选)")
	employeeSearchCmd.Flags().Int("page", 1, "当前页码 (默认 1)")
	employeeSearchCmd.Flags().Int("page-size", 20, "每页条数 (默认 20)")

	employeeSearchStructuredCmd := &cobra.Command{
		Use:   "employees-structured",
		Short: "使用高级条件搜索人员",
		Long: `使用高级条件（originJson 表达式）搜索人员。
建议先调用 "dws hrbrain search fields" 获取有权限的字段与操作符列表。
--origin-json 为 JSON 字符串，例如:
  {"rules":[{"field":"name","operator":"contains","value":"张"}],"combinator":"and"}
--fields 为 JSON 数组，例如:
  [{"label":"姓名","value":"name"}]
--order-by 为逗号分隔的排序字段列表 (可选)`,
		Example: `  dws hrbrain search employees-structured --origin-json '{"rules":[{"field":"name","operator":"contains","value":"张"}],"combinator":"and"}' --fields '[{"label":"姓名","value":"name"}]' --page 1 --page-size 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "origin-json", "fields"); err != nil {
				return err
			}
			page, _ := cmd.Flags().GetInt("page")
			pageSize, _ := cmd.Flags().GetInt("page-size")
			originJSON := mustGetFlag(cmd, "origin-json")
			var originObj map[string]any
			if err := json.Unmarshal([]byte(originJSON), &originObj); err != nil {
				return fmt.Errorf("--origin-json must be a valid JSON object: %w", err)
			}
			var fields []any
			if err := json.Unmarshal([]byte(mustGetFlag(cmd, "fields")), &fields); err != nil {
				return fmt.Errorf("--fields must be a valid JSON array: %w", err)
			}
			if len(fields) == 0 {
				return fmt.Errorf("--fields must be a non-empty JSON array")
			}
			toolArgs := map[string]any{
				"originJson":  originJSON,
				"currentPage": page,
				"pageSize":    pageSize,
				"fields":      fields,
			}
			if v, _ := cmd.Flags().GetString("order-by"); v != "" {
				toolArgs["orderByClauses"] = parseCSVValues(v)
			}
			return callMCPTool("search_employees_structured", toolArgs)
		},
	}
	DeclareLeafMetadata(employeeSearchStructuredCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "hrbrain",
				Name:           "search_employees_structured",
				CanonicalPath:  "hrbrain.search_employees_structured",
				CLIPath:        "hrbrain search employees-structured",
				PrimaryCLIPath: "hrbrain search employees-structured",
			},
			Description: "使用高级条件表达式（originJson）搜索员工",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls the hrbrain MCP tool search_employees_structured, which is absent from the pinned MCP metadata snapshot; no single pinned interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "使用高级条件表达式（originJson）搜索员工",
				UseWhen:      []string{"需要使用组合过滤条件表达式搜索员工，且已通过 dws hrbrain search fields 获取有权限的字段/操作符时"},
				AvoidWhen: []string{
					"只是简单关键词/部门/职级搜索时改用 dws hrbrain search employees",
					"尚未获取可用字段与操作符时先用 dws hrbrain search fields",
				},
				Examples: []string{"dws hrbrain search employees-structured --origin-json '{\"rules\":[{\"field\":\"name\",\"operator\":\"contains\",\"value\":\"张\"}],\"combinator\":\"and\"}' --fields '[{\"label\":\"姓名\",\"value\":\"name\"}]'"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "fields", Property: "fields", Required: boolPtr(true)},
				{Name: "order-by", Property: "orderByClauses", Required: boolPtr(false)},
				{Name: "origin-json", Property: "originJson", Required: boolPtr(true)},
				{Name: "page", Property: "currentPage", Required: boolPtr(false)},
				{Name: "page-size", Property: "pageSize", Required: boolPtr(false)},
			},
		},
	})
	employeeSearchStructuredCmd.Flags().String("origin-json", "", "搜索条件 JSON 表达式 (必填)")
	employeeSearchStructuredCmd.Flags().Int("page", 1, "当前页码 (默认 1)")
	employeeSearchStructuredCmd.Flags().Int("page-size", 20, "每页条数 (默认 20)")
	employeeSearchStructuredCmd.Flags().String("order-by", "", "排序字段列表，逗号分隔 (可选)")
	employeeSearchStructuredCmd.Flags().String("fields", "", "返回列定义 JSON 数组 (必填)")

	searchFieldsCmd := &cobra.Command{
		Use:     "fields",
		Short:   "获取高级搜索字段列表",
		Long:    `获取当前操作人有权限使用的高级搜索字段列表，用于构造 employees-structured 的参数。`,
		Example: `  dws hrbrain search fields`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("get_search_fields", nil)
		},
	}
	DeclareLeafMetadata(searchFieldsCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "hrbrain",
				Name:           "get_search_fields",
				CanonicalPath:  "hrbrain.get_search_fields",
				CLIPath:        "hrbrain search fields",
				PrimaryCLIPath: "hrbrain search fields",
			},
			Description: "获取当前操作人有权限使用的高级搜索字段与操作符列表",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls the hrbrain MCP tool get_search_fields, which is absent from the pinned MCP metadata snapshot; no single pinned interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取当前操作人有权限使用的高级搜索字段与操作符列表",
				UseWhen:      []string{"在调用 dws hrbrain search employees-structured 之前，需要确认可用字段与操作符时"},
				AvoidWhen:    []string{"只是简单关键词搜索时改用 dws hrbrain search employees"},
				Examples:     []string{"dws hrbrain search fields"},
			},
		},
	})

	searchCmd.AddCommand(employeeSearchCmd, employeeSearchStructuredCmd, searchFieldsCmd)

	root.AddCommand(talentPoolCmd, profileCmd, searchCmd)

	return root
}
