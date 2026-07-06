package helpers

import (
	"strings"

	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// dws law — 法律
// MCP endpoint: 钉钉法律MCP服务
// tools/list 同步时间: 2026-06-11
// ──────────────────────────────────────────────────────────

// splitKeywords splits a comma-separated string into a slice of trimmed keywords.
func splitKeywords(raw string) []string {
	parts := strings.Split(raw, ",")
	keywords := make([]string, 0, len(parts))
	for _, p := range parts {
		kw := strings.TrimSpace(p)
		if kw != "" {
			keywords = append(keywords, kw)
		}
	}
	return keywords
}

func newLawCommand() *cobra.Command {
	lawCmd := &cobra.Command{
		Use:   "law",
		Short: "法律咨询与检索",
		Long:  `法律相关操作：法律咨询、法规检索、案例检索。`,
		RunE:  groupRunE,
	}

	// ── legal-consult ───────────────────────────────────────
	// MCP params: query(string,required), deepThink(bool), onlineSearch(bool)
	lawConsultCmd := &cobra.Command{
		Use:   "consult",
		Short: "法律咨询",
		Long: `法律咨询工具，同时咨询法律依据和参考案例时，优先使用该工具。

示例问题：
  请提供股权回购权不属于形成权，应属于请求权的案例，并提供法律依据`,
		Example: `  dws law consult --query "劳动合同解除的法律依据"
  dws law consult --query "股权回购权的法律性质" --deep-think
  dws law consult --query "最新知识产权保护案例" --online-search`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "query", "keyword"); err != nil {
				return err
			}
			body := map[string]any{
				"query": flagOrFallback(cmd, "query", "keyword"),
			}
			if v, _ := cmd.Flags().GetBool("deep-think"); v {
				body["deepThink"] = true
			}
			if v, _ := cmd.Flags().GetBool("online-search"); v {
				body["onlineSearch"] = true
			}
			return callMCPTool("legal-consult", map[string]any{"Body": body})
		},
	}

	// ── law-search ──────────────────────────────────────────
	// MCP params: query(string,required), searchType(string), pageSize(number),
	//
	//	pageNumber(number), queryKeywords([]string), filterCondition(object)
	lawSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "法规/案例统一检索",
		Long: `统一法律检索工具，明确的单一的法规检索或案例检索需求时使用该工具。

搜索类型:
  laws  — 法规检索
  cases — 案例检索`,
		Example: `  dws law search --query "合同法违约责任" --type laws
  dws law search --query "知识产权侵权" --type cases --size 20
  dws law search --query "劳动争议" --type cases --keywords "工资,加班"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "query", "keyword"); err != nil {
				return err
			}
			body := map[string]any{
				"query": flagOrFallback(cmd, "query", "keyword"),
			}
			if v, _ := cmd.Flags().GetString("type"); v != "" {
				body["searchType"] = v
			}
			if v, _ := cmd.Flags().GetInt("page"); v > 0 {
				body["pageNumber"] = v
			}
			if v, _ := cmd.Flags().GetInt("size"); v > 0 {
				body["pageSize"] = v
			}
			if v, _ := cmd.Flags().GetString("keywords"); v != "" {
				body["queryKeywords"] = splitKeywords(v)
			}
			return callMCPTool("law-search", map[string]any{"Body": body})
		},
	}

	// ── case-search ─────────────────────────────────────────
	// MCP params: query(string,required), pageSize(number,required),
	//
	//	pageNumber(number,required), referLevel(string),
	//	queryKeywords([]string), filterCondition(object),
	//	sortKeyAndDirection(object)
	lawCaseCmd := &cobra.Command{
		Use:   "case",
		Short: "案例检索",
		Long: `利用接口搜索相关法律案例，支持分页查询、排序和过滤条件查询。

如有时间范围要求，以 "yyyy-yyyy" 格式添加到 query 中。`,
		Example: `  dws law case --query "合同纠纷 2023-2025"
  dws law case --query "劳动争议" --size 20 --keywords "工资,加班"
  dws law case --query "知识产权" --level "指导性案例"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "query", "keyword"); err != nil {
				return err
			}
			page, _ := cmd.Flags().GetInt("page")
			if page <= 0 {
				page = 1
			}
			size, _ := cmd.Flags().GetInt("size")
			if size <= 0 {
				size = 10
			}
			body := map[string]any{
				"query":      flagOrFallback(cmd, "query", "keyword"),
				"pageNumber": page,
				"pageSize":   size,
			}
			if v, _ := cmd.Flags().GetString("keywords"); v != "" {
				body["queryKeywords"] = splitKeywords(v)
			}
			if v, _ := cmd.Flags().GetString("level"); v != "" {
				body["referLevel"] = v
			}
			return callMCPTool("case-search", map[string]any{"Body": body})
		},
	}

	// ── deli_legal_advice (新增) ────────────────────────────
	// MCP params: query(string,required), model(string,optional)
	deliAdviceCmd := &cobra.Command{
		Use:   "deliadvice",
		Short: "法律咨询 (得力)",
		Long: `法律咨询工具，用于解答各类法律问题。
同时需要法律依据和参考案例的综合咨询时，优先使用该工具。

返回结果可能包含：
  completeResult    — 回答正文
  lawQaRelatedLaws  — 相关法条
  lawQaRelatedCases — 相关案例`,
		Example: `  dws law deliadvice --query "请提供股权回购权不属于形成权，应属于请求权的案例，并提供法律依据"
  dws law deliadvice --query "劳动合同解除的法律依据"
  dws law deliadvice --query "知识产权侵权赔偿标准" --model deli-lite-v2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "query", "keyword"); err != nil {
				return err
			}
			body := map[string]any{
				"query": flagOrFallback(cmd, "query", "keyword"),
			}
			if v, _ := cmd.Flags().GetString("model"); v != "" {
				body["model"] = v
			}
			return callMCPTool("deli_legal_advice", body)
		},
	}

	// ── deli_case_search (新增) ─────────────────────────────
	// MCP params: query(string,required), page_no(int), page_size(int),
	//             sort_field(string), sort_order(string)
	deliCaseSearchCmd := &cobra.Command{
		Use:   "delicasesearch",
		Short: "案例检索 (得力)",
		Long: `检索司法案例（判决书、裁定书等裁判文书），查找与用户法律问题相关的类案判例。
支持按案号精确查询具体案件详情，也支持按法律问题描述模糊检索相关案例。

如有时间范围要求，以 "yyyy-yyyy" 格式添加到 query 中。
本工具仅检索司法案例，不检索法律条文；查找法律条文请使用 delisearch。`,
		Example: `  dws law delicasesearch --query "劳动纠纷案例"
  dws law delicasesearch --query "(2022)粤03民终21879号"
  dws law delicasesearch --query "租赁合同纠纷 2020-2024" --sort-field time --sort-order desc
  dws law delicasesearch --query "知识产权侵权" --page-size 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "query", "keyword"); err != nil {
				return err
			}
			body := map[string]any{
				"query": flagOrFallback(cmd, "query", "keyword"),
			}
			if v, _ := cmd.Flags().GetInt("page-no"); v > 0 {
				body["page_no"] = v
			}
			if v, _ := cmd.Flags().GetInt("page-size"); v > 0 {
				body["page_size"] = v
			}
			sortField, _ := cmd.Flags().GetString("sort-field")
			if sortField == "" {
				sortField = "correlation"
			}
			body["sort_field"] = sortField
			sortOrder, _ := cmd.Flags().GetString("sort-order")
			if sortOrder == "" {
				sortOrder = "desc"
			}
			body["sort_order"] = sortOrder
			return callMCPTool("deli_case_search", body)
		},
	}

	// ── deli_law_search (新增) ──────────────────────────────
	// MCP params: query(string,required), page_no(int), page_size(int),
	//             sort_field(string), sort_order(string)
	deliLawSearchCmd := &cobra.Command{
		Use:   "delisearch",
		Short: "法规检索 (得力)",
		Long: `检索法律法规条文（法律、行政法规、部门规章、司法解释等规范性文件）。
当需要查找具体法条、了解法律规定或查看某部法律的具体内容时使用该工具。

建议使用法律专业术语，去除口语化表达，保留核心法律事实和行为要素。
本工具仅检索法律条文，不检索司法案例；查找案例请使用 delicasesearch。`,
		Example: `  dws law delisearch --query "用人单位未签劳动合同的法律责任"
  dws law delisearch --query "房屋租赁合同提前解除违约金规定"
  dws law delisearch --query "知识产权保护" --sort-field activeDate --sort-order desc`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "query", "keyword"); err != nil {
				return err
			}
			body := map[string]any{
				"query": flagOrFallback(cmd, "query", "keyword"),
			}
			if v, _ := cmd.Flags().GetInt("page-no"); v > 0 {
				body["page_no"] = v
			}
			if v, _ := cmd.Flags().GetInt("page-size"); v > 0 {
				body["page_size"] = v
			}
			sortField, _ := cmd.Flags().GetString("sort-field")
			if sortField == "" {
				sortField = "correlation"
			}
			body["sort_field"] = sortField
			sortOrder, _ := cmd.Flags().GetString("sort-order")
			if sortOrder == "" {
				sortOrder = "desc"
			}
			body["sort_order"] = sortOrder
			return callMCPTool("deli_law_search", body)
		},
	}

	// ── flags: consult (原有) ───────────────────────────────
	lawConsultCmd.Flags().String("query", "", "法律咨询问题或需要咨询的内容 (必填)")
	lawConsultCmd.Flags().String("keyword", "", "--query alias")
	_ = lawConsultCmd.Flags().MarkHidden("keyword")
	lawConsultCmd.Flags().Bool("deep-think", false, "是否进行深度思考")
	lawConsultCmd.Flags().Bool("online-search", false, "是否进行联网检索")

	// ── flags: search (原有) ────────────────────────────────
	lawSearchCmd.Flags().String("query", "", "搜索问题 (必填)")
	lawSearchCmd.Flags().String("keyword", "", "--query alias")
	_ = lawSearchCmd.Flags().MarkHidden("keyword")
	lawSearchCmd.Flags().String("type", "", "搜索类型: laws=法规检索, cases=案例检索")
	lawSearchCmd.Flags().Int("page", 0, "页码 (默认 1)")
	lawSearchCmd.Flags().Int("size", 0, "每页条数 (默认 10)")
	lawSearchCmd.Flags().String("keywords", "", "查询关键词，逗号分隔")

	// ── flags: case (原有) ──────────────────────────────────
	lawCaseCmd.Flags().String("query", "", "搜索问题 (必填)")
	lawCaseCmd.Flags().String("keyword", "", "--query alias")
	_ = lawCaseCmd.Flags().MarkHidden("keyword")
	lawCaseCmd.Flags().Int("page", 0, "页码 (默认 1)")
	lawCaseCmd.Flags().Int("size", 0, "每页条数 (默认 10)")
	lawCaseCmd.Flags().String("keywords", "", "查询关键词，逗号分隔")
	lawCaseCmd.Flags().String("level", "", "参考级别 (如: 指导性案例)")

	// ── flags: deliadvice (新增) ────────────────────────────
	deliAdviceCmd.Flags().String("query", "", "法律咨询问题或需要咨询的内容 (必填)")
	deliAdviceCmd.Flags().String("keyword", "", "--query alias")
	_ = deliAdviceCmd.Flags().MarkHidden("keyword")
	deliAdviceCmd.Flags().String("model", "", "模型名称 (默认 deli-lite-v2)")

	// ── flags: delicasesearch (新增) ────────────────────────
	deliCaseSearchCmd.Flags().String("query", "", "搜索问题或案号 (必填)")
	deliCaseSearchCmd.Flags().String("keyword", "", "--query alias")
	_ = deliCaseSearchCmd.Flags().MarkHidden("keyword")
	deliCaseSearchCmd.Flags().Int("page-no", 0, "页码 (默认 1)")
	deliCaseSearchCmd.Flags().Int("page-size", 0, "每页条数 (默认 10)")
	deliCaseSearchCmd.Flags().String("sort-field", "", "排序字段: correlation(相关性), time(时间), activeDate(实施时间) (默认 correlation)")
	deliCaseSearchCmd.Flags().String("sort-order", "", "排序方向: desc(降序), asc(升序) (默认 desc)")

	// ── flags: delisearch (新增) ────────────────────────────
	deliLawSearchCmd.Flags().String("query", "", "法律检索问题 (必填)")
	deliLawSearchCmd.Flags().String("keyword", "", "--query alias")
	_ = deliLawSearchCmd.Flags().MarkHidden("keyword")
	deliLawSearchCmd.Flags().Int("page-no", 0, "页码 (默认 1)")
	deliLawSearchCmd.Flags().Int("page-size", 0, "每页条数 (默认 10)")
	deliLawSearchCmd.Flags().String("sort-field", "", "排序字段: correlation(相关性), time(时间), activeDate(实施时间) (默认 correlation)")
	deliLawSearchCmd.Flags().String("sort-order", "", "排序方向: desc(降序), asc(升序) (默认 desc)")

	lawCmd.AddCommand(lawConsultCmd, lawSearchCmd, lawCaseCmd, deliAdviceCmd, deliCaseSearchCmd, deliLawSearchCmd)

	return lawCmd
}
