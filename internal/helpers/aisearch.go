package helpers

import (
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

// boolPtr returns a pointer to v, for ParamDecl.Required declarations.
func boolPtr(v bool) *bool { return &v }

// 这些是原有的宽松兜底名，不属于本次 Primary 迁移；保持原生 hidden
// 注册，不给它们新增 alias_of/origin 元数据。
var aisearchPersonGuessFlags = []string{"name", "q", "text"}

// 历史解析优先级必须独立于新旧 Primary：双传时旧 --keyword 始终获胜。
var aisearchQueryResolutionOrder = []string{"keyword", "name", "q", "query", "text"}

// flagValue 安全地读取 flag 值：先查 local，再查自身 PersistentFlags，
// 再查 parents 的 PersistentFlags。比 cmd.Flags().GetString 更鲁棒，
// 因为后者只在 cobra Execute 完成 mergePersistentFlags 后才包含继承的 flag。
func flagValue(cmd *cobra.Command, name string) string {
	if f := cmd.Flag(name); f != nil {
		return f.Value.String()
	}
	return ""
}

func changedFlagValue(cmd *cobra.Command, name string) (string, bool) {
	if f := cmd.Flag(name); f != nil && f.Changed {
		return f.Value.String(), true
	}
	return "", false
}

func aisearchFlagOrFallback(cmd *cobra.Command, primary string, aliases ...string) string {
	if v := flagValue(cmd, primary); v != "" {
		return v
	}
	for _, alias := range aliases {
		if v := flagValue(cmd, alias); v != "" {
			return v
		}
	}
	return ""
}

func aisearchFlagOrDefault(cmd *cobra.Command, primary, def string, aliases ...string) string {
	names := append([]string{primary}, aliases...)
	for _, name := range names {
		if v, ok := changedFlagValue(cmd, name); ok && v != "" {
			return v
		}
	}
	if v := flagValue(cmd, primary); v != "" {
		return v
	}
	return def
}

// resolveAisearchKeyword 按历史顺序解析人员搜索词。虽然公开 Primary 已改为
// --query，但兼容期内显式旧 --keyword 的值仍优先。
func resolveAisearchKeyword(cmd *cobra.Command) string {
	for _, name := range aisearchQueryResolutionOrder {
		if v := flagValue(cmd, name); v != "" {
			return v
		}
	}
	return ""
}

func addAisearchPersonFlags(cmd *cobra.Command) {
	// Alias evidence must be emitted by corecmd.FlagSpec.Aliases. Build the pair
	// on a temporary declaration command so the legacy alias can retain its
	// historical -w shorthand without extending the global alias mechanism or
	// assigning that shorthand to the new --query Primary.
	declaration := &cobra.Command{Use: "aisearch-query-flags"}
	corecmd.RegisterFlags(declaration, []corecmd.FlagSpec{{
		Name:    "query",
		Usage:   "搜索关键词 (必填，如人名、技能关键词等)",
		Aliases: []string{"keyword"},
	}})
	queryFlag := declaration.Flags().Lookup("query")
	keywordFlag := declaration.Flags().Lookup("keyword")
	keywordFlag.Shorthand = "w"
	cmd.Flags().AddFlag(queryFlag)
	cmd.Flags().AddFlag(keywordFlag)
	cmd.Flags().StringP("dimension", "d", "all", "查询维度: all/name/department/position/duty/supervisor/subordinate/phone/jobNumber，多个用逗号分隔")
	for _, alias := range aisearchPersonGuessFlags {
		cmd.Flags().String(alias, "", "")
		_ = cmd.Flags().MarkHidden(alias)
	}
	cmd.Flags().String("type", "", "兼容选择器；person/search 路径仅接受 person/user/people")
	_ = cmd.Flags().MarkHidden("type")
}

func addAisearchKeywordCompatibilityFlag(cmd *cobra.Command) {
	cmd.Flags().String("keyword", "", "--queries 的兼容别名")
	_ = cmd.Flags().MarkHidden("keyword")
	cmd.Flags().String("query", "", "--queries 的兼容别名")
	_ = cmd.Flags().MarkHidden("query")
}

// runAisearchPerson 是 aisearch person 的实际执行体，被 personCmd 和 root
// 的智能 RunE（裸调兜底）共享调用。
func runAisearchPerson(cmd *cobra.Command, _ []string) error {
	if selector := strings.ToLower(strings.TrimSpace(flagValue(cmd, "type"))); selector != "" && selector != "person" && selector != "user" && selector != "people" {
		return apperrors.NewValidation("aisearch person/search 的 --type 仅接受 person、user 或 people")
	}
	keyword := resolveAisearchKeyword(cmd)
	if keyword == "" {
		return validateRequiredFlags(cmd, "query")
	}
	dimensions := parseDimensions(flagValue(cmd, "dimension"))
	return callMCPTool("enterprise_person_search", map[string]any{
		"keyword":   keyword,
		"dimension": dimensions,
	})
}

// runAisearchEnterprise 调用企业内部知识搜索工具。它关注内容本身，
// 参数只包含内容关键词、内容类型和显式时间范围。
func runAisearchEnterprise(cmd *cobra.Command, _ []string) error {
	queries := parseCSVValues(aisearchFlagOrFallback(cmd, "queries", "query", "keyword"))
	searchTypes := normalizeAisearchSearchTypes(parseCSVValues(aisearchFlagOrDefault(cmd, "types", "all", "search-types", "searchTypes")))
	if len(searchTypes) == 0 {
		searchTypes = []string{"all"}
	}

	toolArgs := map[string]any{
		"queries":     queries,
		"searchTypes": searchTypes,
	}
	if v := aisearchFlagOrFallback(cmd, "time-range", "timeRange"); v != "" {
		toolArgs["timeRange"] = v
	}
	return callMCPTool("search_enterprise", toolArgs)
}

// runAisearchBehavior 调用企业内部行为记录搜索工具。该能力和 person 同属
// aisearch server，但参数空间不同，因此独立成 behavior 子命令，避免复用
// search/query 这类已经被 person 兜底占用的路径。
func runAisearchBehavior(cmd *cobra.Command, _ []string) error {
	queries := parseCSVValues(aisearchFlagOrFallback(cmd, "queries", "query", "keyword"))
	searchTypes := normalizeAisearchSearchTypes(parseCSVValues(aisearchFlagOrDefault(cmd, "types", "all", "search-types", "searchTypes")))
	if len(searchTypes) == 0 {
		searchTypes = []string{"all"}
	}

	toolArgs := map[string]any{
		"queries":     queries,
		"searchTypes": searchTypes,
	}
	if v := aisearchFlagOrFallback(cmd, "chat-scope", "chatScope"); v != "" {
		toolArgs["chatScope"] = v
	}
	if v := aisearchFlagOrDefault(cmd, "behavior-type", "all", "behaviorType"); v != "" {
		toolArgs["behaviorType"] = v
	}
	if v := aisearchFlagOrFallback(cmd, "time-range", "timeRange"); v != "" {
		toolArgs["timeRange"] = v
	}
	if v := flagValue(cmd, "direction"); v != "" {
		toolArgs["direction"] = v
	}
	return callMCPTool("search_enterprise_behavior", toolArgs)
}

func normalizeAisearchSearchTypes(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, v := range values {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "doc", "document":
			normalized = append(normalized, "document")
		default:
			normalized = append(normalized, v)
		}
	}
	return normalized
}

func newAisearchCommand() *cobra.Command {
	// Product-level Agent routing Decl (migrated from selection/aisearch.json
	// products.aisearch). Catalog assembly stamps provenance contract_final.
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "aisearch",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "企业内智能搜人、搜知识内容与搜行为记录",
			UseWhen: []string{
				"语义找人、按主题搜企业知识，或追溯发送/创建/分享等行为",
			},
			AvoidWhen: []string{
				"已有明确资源 ID 要读写时改用对应产品；普通 OAuth 登录不用 aisearch",
			},
		},
	})
	root := &cobra.Command{
		Use:   "aisearch",
		Short: "AI 搜问",
		Long:  `AI 搜问：搜索企业人员信息、企业内部知识内容与企业内部行为记录。`,
		// 智能 root：模型常漏 person 子命令直接 dws aisearch --query xxx，
		// 检测到 query 就自动等价于 person；否则退回 group help。
		RunE: func(cmd *cobra.Command, args []string) error {
			if resolveAisearchKeyword(cmd) != "" {
				return runAisearchPerson(cmd, args)
			}
			return groupRunE(cmd, args)
		},
	}

	// root 和 person 各自定义同一组本地 flag，这样：
	//   - dws aisearch --query xxx           ← root 自己能解析
	//   - dws aisearch person --query xxx    ← person 本地 flag
	//   - dws aisearch search --query xxx    ← search 是 person 的 alias
	// 不能放在 product PersistentFlags：否则 enterprise/behavior 会公开接受
	// 它们不拥有的 person-only dimension，破坏 Help ↔ Schema 完整性。
	addAisearchPersonFlags(root)

	personCmd := &cobra.Command{
		Use: "person",
		// alias 列表覆盖真实瞎猜模式（按图里调用频次降序）：
		//   A 类同义瞎猜：search(196) / search-person(74) / user-search(2) /
		//                user(隐含) / query(5) / people(4) / ask(1) / search-user(2)
		//   B 类跨模块混淆：contact(2 + 路径变体共 5)
		// 所有 alias 透明等价于 person，对外文档/help 仍只展示 person。
		Aliases: []string{
			"search", "search-person", "search-user",
			"user", "user-search",
			"query", "people", "ask", "find", "lookup",
			"contact",
		},
		// 显式声明允许任意位置参数：模型可能写 dws aisearch person search --query xxx，
		// 此时 "search" 会作为 positional arg 被忽略，不报错。
		Args:  cobra.ArbitraryArgs,
		Short: "搜索企业人员",
		Long: `通过关键词搜索企业内人员信息，支持按维度筛选。

可选维度 (--dimension):
  all          全部维度 (默认)
  name         姓名
  department   部门
  position     职位
  duty         职责/技能
  supervisor   上级
  subordinate  下级
  phone        手机号
  jobNumber    工号

多个维度用逗号分隔。`,
		Example: `  dws aisearch person --query "张三" --dimension department
  dws aisearch person --query "产品部" --dimension department
  dws aisearch person --query "五道" --dimension supervisor
  dws aisearch person --query "AI搜问" --dimension duty
  dws aisearch person --query "李四" --dimension name,department
  dws aisearch person --query "13800138000" --dimension phone
  dws aisearch person --query "W12345" --dimension jobNumber`,
		RunE: runAisearchPerson,
	}
	DeclareLeafMetadata(personCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "aisearch",
				Name:           "enterprise_person_search",
				CanonicalPath:  "aisearch.enterprise_person_search",
				CLIPath:        "aisearch person",
				PrimaryCLIPath: "aisearch person",
			},
			Description: "企业内找人：按姓名/部门/职位/职责/上下级/手机号/工号筛选",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "aisearch", RPCName: "enterprise_person_search"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "企业内找人：按姓名/部门/职位/职责/上下级/手机号/工号筛选",
				UseWhen: []string{
					"找人、谁负责某事、查上级/下级、按手机号或工号定位人员",
					"需要把维度词映射到 --dimension，关键词只保留目标实体",
				},
				AvoidWhen: []string{
					"已有 userId 只需详情时用 contact user get",
					"精确通讯录关键词搜同事/好友且不涉及职责语义时可用 contact user search",
					"搜企业知识内容时用 aisearch enterprise；搜行为记录时用 aisearch behavior",
				},
				Examples: []string{
					"dws aisearch person --query \"张三\" --dimension name --format json",
					"dws aisearch person --query \"五道\" --dimension supervisor --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "query", Property: "keyword", Required: boolPtr(true)},
			},
		},
	})
	addAisearchPersonFlags(personCmd)

	enterpriseCmd := &cobra.Command{
		Use:     "enterprise",
		Aliases: []string{"knowledge", "content", "search-enterprise", "search_enterprise"},
		Short:   "搜索企业内部知识内容和相关消息",
		Long: `检索企业内部知识内容，如文档、消息、日程、待办、听记、日志、图片、链接、AI 表格、企业百科、邮件等。

普通“XX 相关消息/文档/邮件/日程/待办/纪要有哪些”属于企业内容搜索，使用本命令；queries 只放内容关键词，时间放到 --time-range，所有类型词放到 --types。汇总类场景可不传 queries，使用 --types all。

不要把“最近搜索问题相关消息”截断成 --query "搜索问题"，也不要把“最近 OKR 相关邮件”写成 --query "OKR 邮件"；这会丢失时间和类型槽位。应显式写成 --queries + --types + --time-range。`,
		Example: `  dws aisearch enterprise --queries "智能化方案" --types document
  dws aisearch enterprise --queries "搜索问题" --types im --time-range "最近"
  dws aisearch enterprise --queries "OKR" --types mail --time-range "最近"
  dws aisearch enterprise --queries "AI搜问" --types calendar --time-range "本周"
  dws aisearch enterprise --queries "项目" --types todo,minute --time-range "最近"
  dws aisearch enterprise --queries "发版" --types im --time-range "本周"
  dws aisearch enterprise --types all --time-range "本周"
  dws aisearch enterprise --queries "OKR" --types document,im,mail`,
		RunE: runAisearchEnterprise,
	}
	// Register flags that carry ParamDecl before DeclareLeafMetadata so
	// AttachContract can emit their dws.schema.* annotations.
	enterpriseCmd.Flags().String("queries", "", "内容关键词列表，多个用逗号分隔；汇总类场景可留空")
	enterpriseCmd.Flags().String("time-range", "", "时间范围，仅当用户显式给出时间词时填写，如 今天/本周/9月/过去一周")
	DeclareLeafMetadata(enterpriseCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "aisearch",
				Name:           "search_enterprise",
				CanonicalPath:  "aisearch.search_enterprise",
				CLIPath:        "aisearch enterprise",
				PrimaryCLIPath: "aisearch enterprise",
			},
			Description: "搜索企业内部知识与相关内容（文档/IM/日历/待办/纪要/日志/邮件等）",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "aisearch", RPCName: "search_enterprise"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "搜索企业内部知识与相关内容（文档/IM/日历/待办/纪要/日志/邮件等）",
				UseWhen: []string{
					"按主题找资料、方案、文档、消息、邮件等内容，且关注“有什么内容”",
					"用户显式给出时间词或类型词时分别映射到 --time-range / --types",
				},
				AvoidWhen: []string{
					"问“我发给谁/谁发给我/我创建过”等行为追溯时用 aisearch behavior",
					"企业找人时用 aisearch person",
					"已知具体资源 ID 要读写时改用对应产品命令",
				},
				Examples: []string{
					"dws aisearch enterprise --queries \"智能化方案\" --types document --format json",
					"dws aisearch enterprise --queries \"OKR\" --types mail --time-range \"最近\" --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "queries", Required: boolPtr(false)},
				{Name: "time-range", Required: boolPtr(false)},
				{Name: "types", Property: "searchTypes"},
			},
		},
	})
	enterpriseCmd.Flags().String("types", "all", "搜索类型: all/document/im/calendar/todo/minute/report/image/link/notable/baike/mail，多个用逗号分隔")
	enterpriseCmd.Flags().String("search-types", "", "--types 的别名")
	_ = enterpriseCmd.Flags().MarkHidden("search-types")
	enterpriseCmd.Flags().String("searchTypes", "", "--types 的别名")
	_ = enterpriseCmd.Flags().MarkHidden("searchTypes")
	enterpriseCmd.Flags().String("timeRange", "", "--time-range 的别名")
	_ = enterpriseCmd.Flags().MarkHidden("timeRange")
	addAisearchKeywordCompatibilityFlag(enterpriseCmd)

	behaviorCmd := &cobra.Command{
		Use:   "behavior",
		Short: "搜索明确的发送/创建/接收等行为记录",
		Long: `仅当用户明确询问“我/某人发过、发给、收到、创建、分享、编辑过什么”等行为动作时，检索企业内部行为记录。

普通“XX 相关消息/文档/邮件有哪些”不是行为记录，应使用 aisearch enterprise。behavior 的 queries 只放内容关键词；时间放到 --time-range，所有类型词放到 --types，行为动作放到 --behavior-type，人与人之间的流向放到 --direction。`,
		Example: `  dws aisearch behavior --types mail --behavior-type send --direction "我->汐峰"
  dws aisearch behavior --types im,mail --behavior-type send --direction "我->汐峰"
  dws aisearch behavior --types document --behavior-type receive --direction "汐峰->我"
  dws aisearch behavior --types all --behavior-type create --time-range "本周"
  dws aisearch behavior --types im --chat-scope "scrum群" --behavior-type send --time-range "今天"`,
		RunE: runAisearchBehavior,
	}
	DeclareLeafMetadata(behaviorCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "aisearch",
				Name:           "search_enterprise_behavior",
				CanonicalPath:  "aisearch.search_enterprise_behavior",
				CLIPath:        "aisearch behavior",
				PrimaryCLIPath: "aisearch behavior",
			},
			Description: "搜索发送/创建/分享/编辑/接收等明确行为记录",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "aisearch", RPCName: "search_enterprise_behavior"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "搜索发送/创建/分享/编辑/接收等明确行为记录",
				UseWhen:      []string{"用户明确问我/某人发过、发给、收到、创建、分享、编辑过什么"},
				AvoidWhen: []string{
					"只按主题找内容本身时用 aisearch enterprise",
					"没有行为动作词时不要选用本工具",
				},
				Examples: []string{
					"dws aisearch behavior --types mail --behavior-type send --direction \"我->汐峰\" --format json",
					"dws aisearch behavior --queries \"智能化方案\" --types document --behavior-type create --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "types", Property: "searchTypes"},
			},
		},
	})
	behaviorCmd.Flags().String("queries", "", "内容关键词列表，多个用逗号分隔；汇总类场景可留空")
	behaviorCmd.Flags().String("types", "all", "搜索类型: all/document/im/calendar/todo/minute/report/image/link/notable/baike/mail，多个用逗号分隔")
	behaviorCmd.Flags().String("search-types", "", "--types 的别名")
	_ = behaviorCmd.Flags().MarkHidden("search-types")
	behaviorCmd.Flags().String("searchTypes", "", "--types 的别名")
	_ = behaviorCmd.Flags().MarkHidden("searchTypes")
	behaviorCmd.Flags().String("chat-scope", "", "消息所在会话/群范围，仅 IM 类型且用户明确指定群名时填写")
	behaviorCmd.Flags().String("chatScope", "", "--chat-scope 的别名")
	_ = behaviorCmd.Flags().MarkHidden("chatScope")
	behaviorCmd.Flags().String("behavior-type", "all", "行为类型: all/send/create/share/edit/receive")
	behaviorCmd.Flags().String("behaviorType", "", "--behavior-type 的别名")
	_ = behaviorCmd.Flags().MarkHidden("behaviorType")
	behaviorCmd.Flags().String("time-range", "", "时间范围，仅当用户显式给出时间词时填写，如 今天/本周/9月/过去一周")
	behaviorCmd.Flags().String("timeRange", "", "--time-range 的别名")
	_ = behaviorCmd.Flags().MarkHidden("timeRange")
	behaviorCmd.Flags().String("direction", "", `交互方向，如 "我->汐峰"、"汐峰->我"、"我<->汐峰"`)
	addAisearchKeywordCompatibilityFlag(behaviorCmd)

	root.AddCommand(personCmd, enterpriseCmd, behaviorCmd)
	return root
}

// parseDimensions 将逗号分隔的维度字符串解析为 []string。
func parseDimensions(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{"all"}
	}
	parts := strings.Split(s, ",")
	dims := make([]string, 0, len(parts))
	for _, p := range parts {
		if d := strings.TrimSpace(p); d != "" {
			dims = append(dims, d)
		}
	}
	if len(dims) == 0 {
		return []string{"all"}
	}
	return dims
}
