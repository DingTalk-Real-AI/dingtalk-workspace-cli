package helpers

import (
	"strings"

	"github.com/spf13/cobra"
)

// aisearchKeywordAliases 是 --keyword flag 的同义瞎猜兜底列表。
// 模型可能写 --name / --q / --query / --text，这些都被识别为 keyword。
var aisearchKeywordAliases = []string{"name", "q", "query", "text"}

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

// resolveAisearchKeyword 从命令的 flag 中解析 keyword：优先 --keyword，
// 否则 fallback 到 aisearchKeywordAliases 中的任一同义 flag。
func resolveAisearchKeyword(cmd *cobra.Command) string {
	if v := flagValue(cmd, "keyword"); v != "" {
		return v
	}
	for _, alias := range aisearchKeywordAliases {
		if v := flagValue(cmd, alias); v != "" {
			return v
		}
	}
	return ""
}

func addAisearchPersonFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("keyword", "w", "", "搜索关键词 (必填，如人名、技能关键词等)")
	cmd.Flags().StringP("dimension", "d", "all", "查询维度: all/name/department/position/duty/supervisor/subordinate/phone/jobNumber，多个用逗号分隔")
	for _, alias := range aisearchKeywordAliases {
		cmd.Flags().String(alias, "", "")
		_ = cmd.Flags().MarkHidden(alias)
	}
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
	keyword := resolveAisearchKeyword(cmd)
	if keyword == "" {
		// 复用原有报错文案（"keyword is required"）
		return validateRequiredFlags(cmd, "keyword")
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
	root := &cobra.Command{
		Use:   "aisearch",
		Short: "AI 搜问",
		Long:  `AI 搜问：搜索企业人员信息、企业内部知识内容与企业内部行为记录。`,
		// 智能 root：模型常漏 person 子命令直接 dws aisearch --keyword xxx，
		// 检测到 keyword 就自动等价于 person；否则退回 group help。
		RunE: func(cmd *cobra.Command, args []string) error {
			if resolveAisearchKeyword(cmd) != "" {
				return runAisearchPerson(cmd, args)
			}
			return groupRunE(cmd, args)
		},
	}

	// root 和 person 各自定义同一组本地 flag，这样：
	//   - dws aisearch --keyword xxx           ← root 自己能解析
	//   - dws aisearch person --keyword xxx    ← person 本地 flag
	//   - dws aisearch search --keyword xxx    ← search 是 person 的 alias
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
		// 显式声明允许任意位置参数：模型可能写 dws aisearch person search --keyword xxx，
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
		Example: `  dws aisearch person --keyword "张三" --dimension department
  dws aisearch person --keyword "产品部" --dimension department
  dws aisearch person --keyword "五道" --dimension supervisor
  dws aisearch person --keyword "AI搜问" --dimension duty
  dws aisearch person --keyword "李四" --dimension name,department
  dws aisearch person --keyword "13800138000" --dimension phone
  dws aisearch person --keyword "W12345" --dimension jobNumber`,
		RunE: runAisearchPerson,
	}
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
	enterpriseCmd.Flags().String("queries", "", "内容关键词列表，多个用逗号分隔；汇总类场景可留空")
	enterpriseCmd.Flags().String("types", "all", "搜索类型: all/document/im/calendar/todo/minute/report/image/link/notable/baike/mail，多个用逗号分隔")
	enterpriseCmd.Flags().String("search-types", "", "--types 的别名")
	_ = enterpriseCmd.Flags().MarkHidden("search-types")
	enterpriseCmd.Flags().String("searchTypes", "", "--types 的别名")
	_ = enterpriseCmd.Flags().MarkHidden("searchTypes")
	enterpriseCmd.Flags().String("time-range", "", "时间范围，仅当用户显式给出时间词时填写，如 今天/本周/9月/过去一周")
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
