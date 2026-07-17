package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"
)

const (
	reportDingtalkOpenLinkText        = "在钉钉中查看日志"
	reportDingtalkOpenLinkDescription = "点击后打开钉钉客户端的日志详情页，可查看或修改刚创建的日志。"
	reportContentsMaxBytes            = 10 * 1024 * 1024
	reportDispatchTemplateSuccessHint = "dws report template get --name <模板名> --format json"
	reportDispatchTemplateDetailHint  = "dws report entry submit --template-id <templateId> --contents-file <tmp.json> --format json"
	reportDispatchCreateHint          = "dws report template list --format json\n  dws report template get --name <模板名> --format json\n  dws report entry submit --template-id <templateId> --contents-file <tmp.json> --format json"
	reportDispatchDetailHint          = "dws report outbox list --cursor 0 --size 20 --format json\n  dws report entry get --report-id <reportId> --format json"
	reportDispatchStatsHint           = "dws report outbox list --cursor 0 --size 20 --format json\n  dws report entry stats --report-id <reportId> --format json"
	reportDispatchListHint            = "dws report inbox list --start \"YYYY-MM-DDT00:00:00+08:00\" --end \"YYYY-MM-DDT23:59:59+08:00\" --cursor 0 --size 20 --format json"
	reportDispatchOutboxListHint      = "dws report outbox list --cursor 0 --size 20 --format json"
	reportDispatchTemplateListHint    = "dws report template get --name <模板名> --format json"
	reportDispatchAuthFailureHint     = "dws auth login"
)

var (
	reportOpenFile     = os.Open
	reportAbsPath      = filepath.Abs
	reportGetwd        = os.Getwd
	reportEvalSymlinks = filepath.EvalSymlinks
	reportStat         = os.Stat
	reportRelPath      = filepath.Rel
)

// ──────────────────────────────────────────────────────────
// dws report — 钉钉日志
// MCP tools: get_available_report_templates, get_template_details_by_name,
// create_report, get_report_entry_details, get_received_report_list,
// get_report_statistics_by_id, get_send_report_list
// ──────────────────────────────────────────────────────────

func newReportCommand() *cobra.Command {
	root := &cobra.Command{
		Use:     "report",
		Aliases: []string{"log"},
		Short:   "钉钉日志（OA 周报应用 / 日志模版填报）",
		Long: `钉钉日志（OA 周报应用 / 日志模版填报）— 模版 / 提交 / 收件箱 / 发件箱 / 单条详情 / 统计。

载体辨义：本命令族管理「钉钉日志」OA 应用（按模版填报、收件箱列表、发件箱列表、统计），不是钉钉在线文档。
若你想要的是「钉钉在线文档」请使用 dws doc；想要「待办事项」请使用 dws todo。

资源.动词命令树（与 mail 对齐）：
  template list             列出可用模版
  template get              读取单个模版的字段定义
  inbox list                列出我收到的日报
  outbox list               列出我发出的日报
  entry get                 读取单份日报正文
  entry stats               读取单份日报的已读统计
  entry submit              按模版提交一份新日报

别名：dws log 等价 dws report（注意：此处 log 特指 OA 周报应用，不是通用日志/记录）。`,
		RunE: groupRunE,
	}

	// === template subtree（template list 不变；新增 template get；template detail 转 deprecated alias）===
	templateCmd := &cobra.Command{Use: "template", Short: "日志模版", RunE: groupRunE}

	templateListCmd := &cobra.Command{
		Use:     "list",
		Short:   "获取当前用户可用的日志模版列表",
		Example: `  dws report template list`,
		RunE:    runReportTemplateList,
	}

	templateGetCmd := &cobra.Command{
		Use:     "get",
		Short:   "读取单个日志模版的字段定义",
		Example: `  dws report template get --name <templateName>`,
		RunE:    runReportTemplateDetail,
	}
	addReportTemplateDetailFlags(templateGetCmd)

	templateDetailCmd := &cobra.Command{
		Use:     "detail",
		Short:   "[deprecated] 已废弃，请改用 `dws report template get`",
		Example: `  dws report template detail --name <templateName>`,
		RunE:    withReportDeprecationWarning("template detail", "template get", runReportTemplateDetail),
	}
	addReportTemplateDetailFlags(templateDetailCmd)

	templateCmd.AddCommand(templateListCmd, templateGetCmd, templateDetailCmd)

	// === entry subtree（单条日报操作 — get / stats / submit）===
	entryCmd := &cobra.Command{Use: "entry", Short: "日志条目（单条日报操作 — get / stats / submit）", RunE: groupRunE}

	entryGetCmd := &cobra.Command{
		Use:   "get",
		Short: "读取单份日报正文（含字段明细 + 钉钉跳转链接）",
		Example: `  # 查询 reportId: dws report inbox list / outbox list
  dws report entry get --report-id <reportId>`,
		RunE: runReportDetail,
	}
	addReportDetailFlags(entryGetCmd)

	entryStatsCmd := &cobra.Command{
		Use:   "stats",
		Short: "读取单份日报的已读统计",
		Example: `  # 查询 reportId: dws report inbox list / outbox list
  dws report entry stats --report-id <reportId>`,
		RunE: runReportStats,
	}
	addReportStatsFlags(entryStatsCmd)

	entrySubmitCmd := &cobra.Command{
		Use:   "submit",
		Short: "提交一份新日报（按模版）",
		Long: `按模版提交一份日报。--contents 为 JSON 数组，每项需含 key、sort、content、contentType、type，
与远程 create_report 一致；可先通过 report template list / template get 取得 templateId 与控件定义。

长内容（含中文换行 / Markdown）建议走 --contents-file 避免 shell 引号问题；
也可用 --contents - 从 stdin 读取。
提交成功后会自动反查详情，并在返回中追加 dingtalkOpenUrl / dingtalkOpenMarkdownLink 跳转链接字段。`,
		Example: `  dws report entry submit --template-id TPL_ID --contents '[{"content":"完成开发","sort":"0","key":"今日完成","contentType":"markdown","type":"1"}]'
  # 推荐：长内容走文件
  dws report entry submit --template-id TPL_ID --contents-file ./report.json
  # 或 stdin
  cat report.json | dws report entry submit --template-id TPL_ID --contents -
  dws report entry submit --template-id TPL_ID --contents '[...]' --to-chat --to-user-ids userId1,userId2`,
		RunE: runReportCreate,
	}
	addReportCreateFlags(entrySubmitCmd)

	entryCmd.AddCommand(entryGetCmd, entryStatsCmd, entrySubmitCmd)

	// === inbox subtree（我收到的日报）===
	inboxCmd := &cobra.Command{
		Use:     "inbox",
		Short:   "收件箱（我收到的日报）",
		Example: `  dws report inbox --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" --cursor 0 --size 20`,
		RunE:    withReportDeprecationWarning("inbox", "inbox list", runReportList),
	}
	addReportListFlags(inboxCmd)

	inboxListCmd := &cobra.Command{
		Use:   "list",
		Short: "列出我收到的日报",
		Example: `  dws report inbox list --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" --cursor 0 --size 20
  # 时间 flag 只能是 --start / --end；禁止 --start-date / --end-date / --date
  # CLI 只返回 JSON，Agent 从 result[] 拼 Markdown 表展示给用户，reportId 仅用于 entry get/stats`,
		RunE: runReportList,
	}
	addReportListFlags(inboxListCmd)

	inboxCmd.AddCommand(inboxListCmd)

	// === outbox subtree（我发出的日报）===
	outboxCmd := &cobra.Command{Use: "outbox", Short: "发件箱（我发出的日报）", RunE: groupRunE}

	outboxListCmd := &cobra.Command{
		Use:   "list",
		Short: "列出我发出的日报",
		Example: `  dws report outbox list --cursor 0 --size 20
  dws report outbox list --cursor 0 --size 20 --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00"
  dws report outbox list --cursor 0 --size 20 --template-name "日报"`,
		RunE: runReportSent,
	}
	addReportSentFlags(outboxListCmd)

	outboxCmd.AddCommand(outboxListCmd)

	// === 旧命令（保留作 deprecation alias，调用时打 stderr 警告，行为与新命令一致）===
	createCmd := &cobra.Command{
		Use:     "create",
		Short:   "[deprecated] 已废弃，请改用 `dws report entry submit`",
		Example: `  dws report create --template-id TPL_ID --contents-file ./report.json`,
		RunE:    withReportDeprecationWarning("create", "entry submit", runReportCreate),
	}
	addReportCreateFlags(createCmd)

	detailCmd := &cobra.Command{
		Use:     "detail",
		Short:   "[deprecated] 已废弃，请改用 `dws report entry get`",
		Example: `  dws report detail --report-id <reportId>`,
		RunE:    withReportDeprecationWarning("detail", "entry get", runReportDetail),
	}
	addReportDetailFlags(detailCmd)

	listCmd := &cobra.Command{
		Use:     "list",
		Short:   "[deprecated] 已废弃，请改用 `dws report inbox list`",
		Example: `  dws report list --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" --cursor 0 --size 20`,
		RunE:    withReportDeprecationWarning("list", "inbox list", runReportList),
	}
	addReportListFlags(listCmd)

	statsCmd := &cobra.Command{
		Use:     "stats",
		Short:   "[deprecated] 已废弃，请改用 `dws report entry stats`",
		Example: `  dws report stats --report-id <reportId>`,
		RunE:    withReportDeprecationWarning("stats", "entry stats", runReportStats),
	}
	addReportStatsFlags(statsCmd)

	sendListCmd := &cobra.Command{
		Use:     "sent",
		Short:   "[deprecated] 已废弃，请改用 `dws report outbox list`",
		Example: `  dws report sent --cursor 0 --size 20`,
		RunE:    withReportDeprecationWarning("sent", "outbox list", runReportSent),
	}
	addReportSentFlags(sendListCmd)

	createdListCmd := &cobra.Command{
		Use:     "created",
		Short:   "[deprecated] 已废弃，请改用 `dws report outbox list`",
		Example: `  dws report created --cursor 0 --size 20`,
		RunE:    withReportDeprecationWarning("created", "outbox list", runReportSent),
	}
	addReportSentFlags(createdListCmd)

	// These deprecated leaves wrap the same business handlers with a stderr
	// warning. Keep the implementation-side equivalence review next to the
	// command construction; Registry alias review alone cannot prove handlers
	// do not inject a different request preset.
	cli.AnnotateRuntimeCompatibilityEquivalence(templateGetCmd, templateDetailCmd, cli.RuntimeCompatibilityEquivalence{
		ID: "report.template-get-detail-v1", Reason: "The deprecated detail leaf only adds a deprecation warning before the exact template get business handler.", Reviewed: true,
	})
	cli.AnnotateRuntimeCompatibilityEquivalence(entrySubmitCmd, createCmd, cli.RuntimeCompatibilityEquivalence{
		ID: "report.entry-submit-create-v1", Reason: "The deprecated create leaf only adds a deprecation warning before the exact entry submit business handler.", Reviewed: true,
	})
	cli.AnnotateRuntimeCompatibilityEquivalence(entryGetCmd, detailCmd, cli.RuntimeCompatibilityEquivalence{
		ID: "report.entry-get-detail-v1", Reason: "The deprecated detail leaf only adds a deprecation warning before the exact entry get business handler.", Reviewed: true,
	})
	cli.AnnotateRuntimeCompatibilityEquivalence(inboxListCmd, listCmd, cli.RuntimeCompatibilityEquivalence{
		ID: "report.inbox-list-legacy-list-v1", Reason: "The deprecated list leaf only adds a deprecation warning before the exact inbox list business handler.", Reviewed: true,
	})
	cli.AnnotateRuntimeCompatibilityEquivalence(entryStatsCmd, statsCmd, cli.RuntimeCompatibilityEquivalence{
		ID: "report.entry-stats-legacy-stats-v1", Reason: "The deprecated stats leaf only adds a deprecation warning before the exact entry stats business handler.", Reviewed: true,
	})
	cli.AnnotateRuntimeCompatibilityEquivalence(outboxListCmd, sendListCmd, cli.RuntimeCompatibilityEquivalence{
		ID: "report.outbox-list-legacy-spellings-v1", Reason: "The deprecated sent and created leaves only add a deprecation warning before the exact outbox list business handler.", Reviewed: true,
	})
	cli.AnnotateRuntimeCompatibilityEquivalence(outboxListCmd, createdListCmd, cli.RuntimeCompatibilityEquivalence{
		ID: "report.outbox-list-legacy-spellings-v1", Reason: "The deprecated sent and created leaves only add a deprecation warning before the exact outbox list business handler.", Reviewed: true,
	})

	root.AddCommand(
		// 新命令（资源.动词二段式）
		templateCmd,
		entryCmd,
		inboxCmd,
		outboxCmd,
		// 旧命令（deprecation aliases，下个版本逐步下线）
		createCmd, detailCmd, listCmd, statsCmd, sendListCmd, createdListCmd,
	)
	return root
}

// --- 共享 handler / flag setter / deprecation 包装 ---
// 抽取自原 newReportCommand 内联的 RunE / Flags 调用，使新命令（inbox/outbox/entry/template get）
// 与旧命令（list/sent/detail/stats/create/template detail）共用同一份业务实现，保证逐字节等价。

func runReportTemplateList(cmd *cobra.Command, args []string) error {
	return withReportDispatchHint("template-list", callMCPTool("get_available_report_templates", nil))
}

func runReportTemplateDetail(cmd *cobra.Command, args []string) error {
	if err := validateRequiredFlags(cmd, "name"); err != nil {
		return err
	}
	return withReportDispatchHint("template-detail", callMCPTool("get_template_details_by_name", map[string]any{
		"report_template_name": mustGetFlag(cmd, "name"),
	}))
}

func runReportCreate(cmd *cobra.Command, args []string) error {
	tplID := mustGetFlag(cmd, "template-id")
	if tplID == "" {
		return &CLIError{
			Code:       CodeMissingParam,
			Message:    "flag --template-id is required",
			Suggestion: "运行 `dws report template list --format json` 获取合法 templateId",
			Operation:  "report.create",
		}
	}
	contentsJSON, err := resolveReportContentsFromFlags(cmd)
	if err != nil {
		return err
	}
	if strings.TrimSpace(contentsJSON) == "" {
		return &CLIError{
			Code:       CodeMissingParam,
			Message:    "contents is required",
			Suggestion: "通过 --contents '<json>' / --contents - (stdin) / --contents-file <path> 之一提供 contents JSON 数组",
			Operation:  "report.create",
		}
	}
	var contents []map[string]any
	if err := json.Unmarshal([]byte(contentsJSON), &contents); err != nil {
		return &CLIError{
			Code:       CodeInvalidJSON,
			Message:    fmt.Sprintf("contents JSON parse failed: %v", err),
			Suggestion: "contents 必须是 JSON 数组，每项含 key/sort/content/contentType/type；先看 template get 取字段名与类型",
			Operation:  "report.create",
		}
	}
	if err := validateAndNormalizeReportContents(contents); err != nil {
		return err
	}
	ddFrom := mustGetFlag(cmd, "dd-from")
	if ddFrom == "" {
		ddFrom = "dws"
	}
	toChat, _ := cmd.Flags().GetBool("to-chat")
	toolArgs := map[string]any{
		"templateId": tplID,
		"contents":   contents,
		"ddFrom":     ddFrom,
		"toChat":     toChat,
	}
	if v, _ := cmd.Flags().GetString("to-user-ids"); v != "" {
		toolArgs["toUserIds"] = parseReportUserIDs(v)
	}
	return callReportCreateWithDetailURL(toolArgs)
}

func runReportDetail(cmd *cobra.Command, args []string) error {
	if err := validateRequiredFlags(cmd, "report-id"); err != nil {
		return err
	}
	return withReportDispatchHint("detail", callMCPTool("get_report_entry_details", map[string]any{
		"report_id": mustGetFlag(cmd, "report-id"),
	}))
}

func runReportList(cmd *cobra.Command, args []string) error {
	if err := validateRequiredFlags(cmd, "start", "end"); err != nil {
		return err
	}
	startStr := mustGetFlag(cmd, "start")
	endStr := mustGetFlag(cmd, "end")
	startMs, err := parseISOTimeToMillis("start", startStr)
	if err != nil {
		return err
	}
	endMs, err := parseISOTimeToMillis("end", endStr)
	if err != nil {
		return err
	}
	if err := validateTimeRange(startMs, endMs); err != nil {
		return err
	}
	cursor, _ := cmd.Flags().GetInt("cursor")
	size, _ := cmd.Flags().GetInt("size")
	if v, _ := cmd.Flags().GetInt("limit"); v > 0 && !cmd.Flags().Changed("size") {
		size = v
	}
	toolArgs := map[string]any{
		"startTime": float64(startMs),
		"endTime":   float64(endMs),
		"cursor":    float64(cursor),
		"size":      float64(size),
	}
	// 发送人过滤（来自 develop 分支 feature/select_report_staff）
	if senderIDs, _ := cmd.Flags().GetStringSlice("sender-user-ids"); len(senderIDs) > 0 {
		toolArgs["senderUserIds"] = senderIDs
	}
	return callReportListReadable("get_received_report_list", "list", toolArgs)
}

func runReportStats(cmd *cobra.Command, args []string) error {
	if err := validateRequiredFlags(cmd, "report-id"); err != nil {
		return err
	}
	return withReportDispatchHint("stats", callMCPTool("get_report_statistics_by_id", map[string]any{
		"report_id": mustGetFlag(cmd, "report-id"),
	}))
}

func runReportSent(cmd *cobra.Command, args []string) error {
	cursor, _ := cmd.Flags().GetInt("cursor")
	size, _ := cmd.Flags().GetInt("size")
	if v, _ := cmd.Flags().GetInt("limit"); v > 0 && !cmd.Flags().Changed("size") {
		size = v
	}
	toolArgs := map[string]any{
		"cursor": float64(cursor),
		"size":   float64(size),
	}
	now := time.Now()
	// 服务端 get_send_report_list 单次查询最大跨度为 20 天；
	// 默认窗口与上限对齐，避免触发服务端范围错误。
	startDefault := now.AddDate(0, 0, -20).Truncate(24 * time.Hour).Format(time.RFC3339)
	endDefault := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location()).Format(time.RFC3339)
	startStr, _ := cmd.Flags().GetString("start")
	endStr, _ := cmd.Flags().GetString("end")
	startMissing := startStr == ""
	endMissing := endStr == ""
	if startMissing {
		startStr = startDefault
	}
	if endMissing {
		endStr = endDefault
	}
	if startMissing || endMissing {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"# info: --start / --end not provided, defaulting to last 20 days (%s ~ %s); server caps single-query span at 20 days, pass explicit --start to shift the window\n",
			startStr, endStr)
	}
	startMs, err := parseISOTimeToMillis("start", startStr)
	if err != nil {
		return err
	}
	toolArgs["startTime"] = float64(startMs)
	endMs, err := parseISOTimeToMillis("end", endStr)
	if err != nil {
		return err
	}
	toolArgs["endTime"] = float64(endMs)
	if err := validateTimeRange(startMs, endMs); err != nil {
		return err
	}
	if v, _ := cmd.Flags().GetString("modified-start"); v != "" {
		ms, err := parseISOTimeToMillis("modified-start", v)
		if err != nil {
			return err
		}
		toolArgs["modifiedStartTime"] = float64(ms)
	}
	if v, _ := cmd.Flags().GetString("modified-end"); v != "" {
		ms, err := parseISOTimeToMillis("modified-end", v)
		if err != nil {
			return err
		}
		toolArgs["modifiedEndTime"] = float64(ms)
	}
	if v, _ := cmd.Flags().GetString("template-name"); v != "" {
		toolArgs["report_template_name"] = v
	}
	return callReportListReadable("get_send_report_list", "sent", toolArgs)
}

func addReportTemplateDetailFlags(cmd *cobra.Command) {
	cmd.Flags().String("name", "", "模版名称 (必填)")
}

func addReportDetailFlags(cmd *cobra.Command) {
	cmd.Flags().String("report-id", "", "日志 ID (必填)")
}

func addReportStatsFlags(cmd *cobra.Command) {
	cmd.Flags().String("report-id", "", "日志 ID (必填)")
}

func addReportListFlags(cmd *cobra.Command) {
	cmd.Flags().String("start", "", "开始时间 ISO-8601 (如 2026-03-10T00:00:00+08:00) (必填)")
	cmd.Flags().String("end", "", "结束时间 ISO-8601 (如 2026-03-10T23:59:59+08:00) (必填)")
	cmd.Flags().Int("cursor", 0, "分页游标（默认 0，翻页传返回的 cursor）")
	cmd.Flags().Int("size", 20, "每页条数（默认 20，最大 20）")
	cmd.Flags().Int("limit", 0, "--size 的别名")
	_ = cmd.Flags().MarkHidden("limit")
	// 发送人过滤（来自 develop 分支 feature/select_report_staff）
	cmd.Flags().StringSlice("sender-user-ids", nil, "发送人 staffId 列表，逗号分隔，用于过滤指定发送人的日志")
}

func addReportSentFlags(cmd *cobra.Command) {
	cmd.Flags().Int("cursor", 0, "分页游标，首次传 0 (默认 0)")
	cmd.Flags().Int("size", 20, "每页条数，最大 20 (默认 20)")
	cmd.Flags().Int("limit", 0, "--size 的别名")
	_ = cmd.Flags().MarkHidden("limit")
	cmd.Flags().String("start", "", "创建开始时间 ISO-8601 (默认最近 20 天；服务端单次查询跨度上限 20 天)")
	cmd.Flags().String("end", "", "创建结束时间 ISO-8601 (默认最近 20 天；服务端单次查询跨度上限 20 天)")
	cmd.Flags().String("modified-start", "", "修改开始时间 ISO-8601 (可选)")
	cmd.Flags().String("modified-end", "", "修改结束时间 ISO-8601 (可选)")
	cmd.Flags().String("template-name", "", "日志模板名称 (可选，不传查全部)")
}

func addReportCreateFlags(cmd *cobra.Command) {
	cmd.Flags().String("template-id", "", "日志模版 ID (必填)")
	cmd.Flags().String("contents", "", "日志内容 JSON 数组 (必填，或用 --contents-file)，每项含 key/sort/content/contentType/type；传 - 表示从 stdin 读取")
	cmd.Flags().String("contents-file", "", "从文件读取 contents JSON（推荐用于含中文/换行/Markdown 的长内容，避免 shell 引号转义；优先级：--contents-file > --contents - (stdin) > --contents '<json>'）")
	cmd.Flags().String("dd-from", "dws", "创建来源标识")
	cmd.Flags().Bool("to-chat", false, "是否发送到日志接收人单聊")
	cmd.Flags().String("to-user-ids", "", "接收人 userId，逗号分隔 (可选)")
}

// withReportDeprecationWarning 包装旧命令的 RunE：调用时往 stderr 打废弃提醒，
// 然后透传到新命令的 handler，stdout 与 exit code 与新命令保持一致。
// 文案不带具体下线日期，避免 Phase 3 延后时需要再发一版改文案。
func withReportDeprecationWarning(oldName, newName string, run func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"[deprecated] `dws report %s` 已废弃，将在后续版本中移除。请改用 `dws report %s`。\n",
			oldName, newName)
		return run(cmd, args)
	}
}

// parseReportUserIDs 将 "id1,id2" 转为 []string，供 create_report 的 toUserIds 使用。
func parseReportUserIDs(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func callReportListReadable(toolName, operation string, toolArgs map[string]any) error {
	if deps.Caller.DryRun() {
		return withReportDispatchHint(operation, callMCPTool(toolName, toolArgs))
	}

	ctx := context.Background()
	text, err := callMCPToolReturnTextOnServer(ctx, "report", toolName, toolArgs)
	if err != nil {
		return withReportDispatchHint(operation, err)
	}
	var parsed any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		deps.Out.PrintRaw(text)
		return nil
	}
	enriched := enrichReportListReadable(ctx, operation, parsed)
	return deps.Out.PrintJSONUnescaped(enriched)
}

func enrichReportListReadable(ctx context.Context, operation string, parsed any) any {
	body, ok := parsed.(map[string]any)
	if !ok {
		return parsed
	}
	items, found := findReportListItems(body)
	if !found {
		return body
	}
	includeContent := reportListIncludesContent(operation)
	entries := makeReportReadableEntries(items)
	sortReportReadableEntries(entries)
	rows := make([]map[string]string, 0, len(items))
	detailCommands := make([]map[string]any, 0, len(items))
	for _, entry := range entries {
		row := entry.row
		if entry.reportID != "" {
			detail := reportDetailForListRow(ctx, entry.reportID, includeContent)
			mergeReportListDetail(row, detail, includeContent)
			detailCommands = append(detailCommands, map[string]any{
				"index":   len(detailCommands) + 1,
				"title":   row["标题"],
				"command": "dws report entry get --report-id " + entry.reportID + " --format json",
			})
		}
		if !includeContent {
			delete(row, "日志内容")
		}
		rows = append(rows, row)
	}
	title := reportReadableListTitle(operation)
	columns := reportListDisplayColumns(operation)
	markdownHeader := reportListMarkdownHeader(columns)
	markdownTable := reportListMarkdownTable(columns, rows)
	display := map[string]any{
		"success":                      reportListSuccess(body),
		"result":                       rows,
		"count":                        len(rows),
		"agentDisplayTitle":            title,
		"agentDisplayColumns":          columns,
		"agentDisplayRequired":         true,
		"agentDisplayRequiredColumns":  columns,
		"agentDisplayLinkColumn":       "钉钉链接",
		"agentDisplayMarkdownHeader":   markdownHeader,
		"agentDisplayMarkdown":         markdownTable,
		"agentDisplayMarkdownRequired": true,
		"agentDisplayContentIncluded":  includeContent,
		"agentDisplay": map[string]any{
			"type":             "table",
			"title":            title,
			"columns":          columns,
			"required":         true,
			"requiredColumns":  columns,
			"linkColumn":       "钉钉链接",
			"render":           "markdown",
			"markdown":         markdownTable,
			"markdownRequired": true,
			"contentIncluded":  includeContent,
		},
		"agentDisplayInstruction": reportListAgentDisplayInstruction(includeContent, markdownHeader),
		"_internalDetailCommands": detailCommands,
	}
	copyReportListPageField(display, body, "hasMore")
	copyReportListPageField(display, body, "nextCursor")
	copyReportListPageField(display, body, "cursor")
	return display
}

func reportListIncludesContent(operation string) bool {
	return operation == "sent"
}

func reportListAgentDisplayInstruction(includeContent bool, markdownHeader string) string {
	if !includeContent {
		return "CLI 只返回 JSON 数据；已对 result[] 中所有带 reportId 的日志调用 dws report entry get --report-id <reportId> --format json 只补齐日期、标题、发送人、状态和钉钉链接。inbox list 接口侧禁止返回日志正文、完整内容或日志内容摘要；Agent 面向用户时必须直接原样输出 agentDisplayMarkdown 作为 Markdown 表，不要自行重组列、不要改表头、不要把它渲染成 4 列摘要表。表头必须逐字为：" + markdownHeader + "。钉钉链接列是强制列，必须使用 agentDisplayMarkdown/result[].钉钉链接 中的 markdown link；禁止省略、合并、改名。凡用户说列出/找到/查询/搜索/看看日志，默认都要在 final reply 渲染 Markdown 表；只有用户明确表示不关心列表内容、只要原始 JSON、只要数量或只要 ID 时，才可以不渲染表。不要展示 reportId、毫秒时间戳、_internalDetailCommands 或日志正文。"
	}
	return "CLI 只返回 JSON 数据；已对 result[] 中所有带 reportId 的日志调用 dws report entry get --report-id <reportId> --format json 补齐日期、标题、发送人、状态、日志内容和钉钉链接。Agent 面向用户时必须直接原样输出 agentDisplayMarkdown 作为 Markdown 表，不要自行重组列、不要改表头、不要把它渲染成 4 列摘要表。表头必须逐字为：" + markdownHeader + "。钉钉链接列是强制列，必须使用 agentDisplayMarkdown/result[].钉钉链接 中的 markdown link；禁止省略、合并、改名。凡用户说列出/找到/查询/搜索/看看日志，默认都要在 final reply 渲染 Markdown 表；只有用户明确表示不关心日志内容、只要原始 JSON、只要数量或只要 ID 时，才可以不渲染表。不要展示 reportId、毫秒时间戳或 _internalDetailCommands。"
}

func reportListDisplayColumns(operation string) []string {
	if reportListIncludesContent(operation) {
		return []string{"日期", "标题", "发送人", "状态", "日志内容", "钉钉链接"}
	}
	return []string{"日期", "标题", "发送人", "状态", "钉钉链接"}
}

func reportListMarkdownHeader(columns []string) string {
	return "| " + strings.Join(columns, " | ") + " |"
}

func reportListMarkdownTable(columns []string, rows []map[string]string) string {
	var b strings.Builder
	writeMarkdownRow := func(values []string) {
		b.WriteString(reportListMarkdownHeader(values))
		b.WriteString("\n")
	}
	writeMarkdownRow(columns)
	delimiters := make([]string, len(columns))
	for i := range delimiters {
		delimiters[i] = "---"
	}
	writeMarkdownRow(delimiters)
	for _, row := range rows {
		values := make([]string, 0, len(columns))
		for _, column := range columns {
			values = append(values, reportMarkdownCell(row[column]))
		}
		writeMarkdownRow(values)
	}
	return strings.TrimRight(b.String(), "\n")
}

func reportMarkdownCell(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.Join(strings.Fields(s), " ")
	return strings.ReplaceAll(s, "|", "｜")
}

type reportReadableEntry struct {
	row             map[string]string
	reportID        string
	createdAtMillis int64
}

func makeReportReadableEntries(items []map[string]any) []reportReadableEntry {
	entries := make([]reportReadableEntry, 0, len(items))
	for _, item := range items {
		entries = append(entries, reportReadableEntry{
			row:             reportReadableRow(item),
			reportID:        reportIDFromMap(item),
			createdAtMillis: reportReadableCreateTimeMillis(item),
		})
	}
	return entries
}

func sortReportReadableEntries(entries []reportReadableEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		left := entries[i]
		right := entries[j]
		if left.createdAtMillis != right.createdAtMillis {
			if left.createdAtMillis == 0 {
				return false
			}
			if right.createdAtMillis == 0 {
				return true
			}
			return left.createdAtMillis > right.createdAtMillis
		}
		return left.row["日期"] > right.row["日期"]
	})
}

func reportReadableListTitle(operation string) string {
	switch operation {
	case "sent":
		return "我创建的日志列表"
	default:
		return "收到的日志列表"
	}
}

func reportListSuccess(body map[string]any) any {
	if v, ok := body["success"]; ok {
		return v
	}
	return true
}

func copyReportListPageField(dst, src map[string]any, key string) {
	if v, ok := src[key]; ok {
		dst[key] = v
		return
	}
	if result, ok := src["result"].(map[string]any); ok {
		if v, ok := result[key]; ok {
			dst[key] = v
		}
	}
}

type reportListDetail struct {
	date         string
	title        string
	sender       string
	status       string
	content      string
	markdownLink string
}

func mergeReportListDetail(row map[string]string, detail reportListDetail, includeContent bool) {
	if detail.date != "" {
		row["日期"] = detail.date
	}
	if detail.title != "" {
		row["标题"] = detail.title
	}
	if detail.sender != "" {
		row["发送人"] = detail.sender
	}
	if detail.status != "" {
		row["状态"] = detail.status
	}
	if includeContent {
		row["日志内容"] = detail.content
	}
	row["钉钉链接"] = detail.markdownLink
}

func reportDetailForListRow(ctx context.Context, reportID string, includeContent bool) reportListDetail {
	detailText, err := callMCPToolReturnTextOnServer(ctx, "report", "get_report_entry_details", map[string]any{
		"report_id": reportID,
	})
	if err != nil {
		return reportListDetail{markdownLink: "查看详情"}
	}
	var detail any
	if err := json.Unmarshal([]byte(detailText), &detail); err != nil {
		return reportListDetail{markdownLink: "查看详情"}
	}
	markdownLink := reportMarkdownLinkFromResponse(detail)
	if markdownLink == "" {
		markdownLink = "查看详情"
	}
	result := reportListDetail{
		date:         reportReadableDetailDate(detail),
		title:        reportReadableDetailTitle(detail),
		sender:       reportReadableDetailSender(detail),
		status:       reportReadableDetailStatus(detail),
		markdownLink: markdownLink,
	}
	if includeContent {
		result.content = reportReadableDetailContent(detail)
	}
	return result
}

func reportReadableDetailDate(detail any) string {
	for _, m := range reportResponseMaps(detail) {
		if date := reportReadableCreateTime(m); date != "" {
			return date
		}
	}
	return ""
}

func reportReadableDetailTitle(detail any) string {
	for _, m := range reportResponseMaps(detail) {
		if title := firstReportString(m, "report_name", "reportName", "title", "summary", "report_template_name", "templateName"); title != "" {
			return reportCompactDisplayText(title, 120)
		}
	}
	return ""
}

func reportReadableDetailSender(detail any) string {
	for _, m := range reportResponseMaps(detail) {
		if sender := firstReportString(m, "creatorName", "senderName", "userName", "creator", "sender"); sender != "" {
			return reportCompactDisplayText(sender, 80)
		}
	}
	return ""
}

func reportReadableDetailStatus(detail any) string {
	for _, m := range reportResponseMaps(detail) {
		if status := reportReadableStatus(m); status != "" {
			return status
		}
	}
	return ""
}

func reportMarkdownLinkFromResponse(detail any) string {
	if link := reportStringFromResponse(detail, "dingtalkOpenMarkdownLink"); link != "" {
		return link
	}
	if url := reportURLFromResponse(detail); url != "" {
		return reportDingtalkMarkdownLink(url)
	}
	return ""
}

func reportStringFromResponse(detail any, key string) string {
	for _, m := range reportResponseMaps(detail) {
		if s, ok := m[key].(string); ok {
			if s = strings.TrimSpace(s); s != "" {
				return s
			}
		}
	}
	return ""
}

func reportReadableDetailContent(detail any) string {
	for _, m := range reportResponseMaps(detail) {
		for _, key := range []string{"report_content", "reportContent", "report_contents", "reportContents", "contents", "contentList"} {
			if content := reportReadableContentFromValue(m[key]); content != "" {
				return reportCompactDisplayText(content, 600)
			}
		}
	}
	for _, m := range reportResponseMaps(detail) {
		if content := firstReportString(m, "content", "summary", "text", "plainText", "reportContent"); content != "" {
			return reportCompactDisplayText(content, 600)
		}
	}
	return ""
}

func reportResponseMaps(detail any) []map[string]any {
	body, ok := detail.(map[string]any)
	if !ok {
		return nil
	}
	maps := []map[string]any{body}
	if result, ok := body["result"].(map[string]any); ok {
		maps = append([]map[string]any{result}, maps...)
	}
	return maps
}

func reportReadableContentFromValue(v any) string {
	switch x := v.(type) {
	case []any:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			if s := reportReadableContentFromValue(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "；")
	case map[string]any:
		label := firstReportString(x, "key", "field_name", "fieldName", "name", "title")
		value := reportFirstDisplayValue(x, "value", "content", "text", "plainText", "markdown", "richTextValue")
		if value == "" {
			return ""
		}
		if label != "" {
			return label + "：" + value
		}
		return value
	case string:
		return strings.TrimSpace(x)
	default:
		return reportDisplayStringFromValue(x)
	}
}

func reportFirstDisplayValue(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if s := reportDisplayStringFromValue(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func reportDisplayStringFromValue(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case []any:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			if s := reportDisplayStringFromValue(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	case map[string]any:
		return reportFirstDisplayValue(x, "value", "text", "content", "plainText", "name", "title")
	default:
		return ""
	}
}

func reportCompactDisplayText(s string, maxRunes int) string {
	s = strings.ReplaceAll(s, "|", "｜")
	s = strings.Join(strings.Fields(s), " ")
	if maxRunes <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

func findReportListItems(root map[string]any) ([]map[string]any, bool) {
	for _, key := range []string{"list", "items", "data", "reports", "records", "report_list", "reportList", "result"} {
		value, ok := root[key]
		if !ok {
			continue
		}
		if items, found := reportItemsFromValue(value); found {
			return items, true
		}
	}
	return nil, false
}

func reportItemsFromValue(v any) ([]map[string]any, bool) {
	switch x := v.(type) {
	case []any:
		items := make([]map[string]any, 0, len(x))
		for _, raw := range x {
			item, ok := raw.(map[string]any)
			if !ok || reportIDFromMap(item) == "" {
				continue
			}
			items = append(items, item)
		}
		return items, true
	case map[string]any:
		for _, key := range []string{"list", "items", "data", "reports", "records", "report_list", "reportList"} {
			value, ok := x[key]
			if !ok {
				continue
			}
			if items, found := reportItemsFromValue(value); found {
				return items, true
			}
		}
	}
	return nil, false
}

func reportReadableRow(item map[string]any) map[string]string {
	sender := firstReportString(item, "creatorName", "senderName", "userName", "creator", "sender")
	title := firstReportString(item, "report_name", "reportName", "title", "summary", "report_template_name", "templateName")
	if title == "" && sender != "" {
		title = sender + "的日志"
	}
	if title == "" {
		title = "日志"
	}
	createdAt := reportReadableCreateTime(item)
	return map[string]string{
		"日期":  createdAt,
		"标题":  title,
		"发送人": sender,
		"状态":  reportReadableStatus(item),
	}
}

func firstReportString(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if s, ok := item[key].(string); ok {
			if s = strings.TrimSpace(s); s != "" {
				return s
			}
		}
	}
	return ""
}

func reportReadableCreateTime(item map[string]any) string {
	if ms := reportReadableCreateTimeMillis(item); ms > 0 {
		return reportMillisToLocalString(ms)
	}
	for _, key := range []string{"createTime", "gmtCreate", "sendTime", "time", "modifiedTime"} {
		if s := reportTimeValueToString(item[key]); s != "" {
			return s
		}
	}
	return ""
}

func reportReadableCreateTimeMillis(item map[string]any) int64 {
	for _, key := range []string{"createTime", "gmtCreate", "sendTime", "time", "modifiedTime"} {
		if ms := reportTimeValueToMillis(item[key]); ms > 0 {
			return ms
		}
	}
	return 0
}

func reportTimeValueToString(v any) string {
	if ms := reportTimeValueToMillis(v); ms > 0 {
		return reportMillisToLocalString(ms)
	}
	switch x := v.(type) {
	case string:
		x = strings.TrimSpace(x)
		if x == "" {
			return ""
		}
		if t, err := time.Parse(time.RFC3339, x); err == nil {
			return t.Local().Format("2006-01-02 15:04")
		}
		return x
	default:
		return ""
	}
}

func reportTimeValueToMillis(v any) int64 {
	switch x := v.(type) {
	case float64:
		return normalizeReportTimestamp(int64(x))
	case int64:
		return normalizeReportTimestamp(x)
	case int:
		return normalizeReportTimestamp(int64(x))
	case json.Number:
		n, err := x.Int64()
		if err != nil {
			return 0
		}
		return normalizeReportTimestamp(n)
	case string:
		x = strings.TrimSpace(x)
		if x == "" {
			return 0
		}
		if n, err := strconv.ParseInt(x, 10, 64); err == nil {
			return normalizeReportTimestamp(n)
		}
		if t, err := time.Parse(time.RFC3339, x); err == nil {
			return t.Local().UnixMilli()
		}
		if t, err := time.ParseInLocation("2006-01-02 15:04", x, time.Local); err == nil {
			return t.UnixMilli()
		}
	}
	return 0
}

func normalizeReportTimestamp(ts int64) int64 {
	if ts <= 0 {
		return 0
	}
	if ts < 100000000000 {
		ts *= 1000
	}
	return ts
}

func reportMillisToLocalString(ms int64) string {
	ms = normalizeReportTimestamp(ms)
	if ms == 0 {
		return ""
	}
	return time.UnixMilli(ms).Local().Format("2006-01-02 15:04")
}

func reportReadableStatus(item map[string]any) string {
	for _, key := range []string{"readStatus", "isRead", "hasRead", "read"} {
		if status := reportStatusValueToString(item[key]); status != "" {
			return status
		}
	}
	return ""
}

func reportStatusValueToString(v any) string {
	switch x := v.(type) {
	case bool:
		if x {
			return "已读"
		}
		return "未读"
	case string:
		s := strings.TrimSpace(strings.ToLower(x))
		switch s {
		case "true", "read", "已读", "1":
			return "已读"
		case "false", "unread", "未读", "0":
			return "未读"
		default:
			return strings.TrimSpace(x)
		}
	case float64:
		if x == 1 {
			return "已读"
		}
		if x == 0 {
			return "未读"
		}
	}
	return ""
}

func withReportDispatchHint(operation string, err error) error {
	if err == nil {
		if deps.Caller.Format() == "json" {
			return nil
		}
		if hint := reportDispatchHint(operation, ""); hint != "" {
			deps.Out.PrintDim(hint)
		}
		return nil
	}
	cliErr, ok := err.(*CLIError)
	if !ok {
		return err
	}
	extraHint := reportDispatchHint(operation, cliErr.Message)
	if extraHint == "" {
		return err
	}
	if cliErr.Suggestion == "" {
		cliErr.Suggestion = extraHint
		return cliErr
	}
	cliErr.Suggestion = cliErr.Suggestion + "\n  " + extraHint
	return cliErr
}

func reportDispatchHint(operation, message string) string {
	normalizedOp := strings.TrimSpace(operation)
	lowerMsg := strings.ToLower(strings.TrimSpace(message))
	success := lowerMsg == ""

	switch normalizedOp {
	case "template-list":
		if success {
			return "可复用指令：\n  " + reportDispatchTemplateSuccessHint
		}
	case "template-detail":
		if success {
			return "可复用指令：\n  " + reportDispatchTemplateDetailHint
		}
	case "create":
		if success {
			return ""
		}
	case "detail":
		if success {
			return "可复用指令：\n  " + reportDispatchDetailHint
		}
	case "stats":
		if success {
			return "可复用指令：\n  " + reportDispatchStatsHint
		}
	case "list":
		if success {
			return "可复用指令：\n  " + reportDispatchListHint
		}
	case "sent":
		if success {
			return "可复用指令：\n  " + reportDispatchOutboxListHint
		}
	}
	switch normalizedOp {
	case "create":
		if strings.Contains(lowerMsg, "参数错误") || strings.Contains(lowerMsg, "param error") || strings.Contains(lowerMsg, "param_error") {
			return "可复用指令：\n  " + reportDispatchCreateHint
		}
	}
	switch normalizedOp {
	case "detail":
		return "可复用指令：\n  " + reportDispatchDetailHint
	case "stats":
		return "可复用指令：\n  " + reportDispatchStatsHint
	case "list":
		return "可复用指令：\n  " + reportDispatchListHint
	case "sent":
		return "可复用指令：\n  " + reportDispatchOutboxListHint
	case "template-list":
		return "可复用指令：\n  " + reportDispatchAuthFailureHint
	case "template-detail":
		return "可复用指令：\n  " + reportDispatchTemplateListHint
	default:
		return ""
	}
}

func callReportCreateWithDetailURL(toolArgs map[string]any) error {
	if deps.Caller.DryRun() {
		return withReportDispatchHint("create", callMCPTool("create_report", toolArgs))
	}

	ctx := context.Background()
	createText, err := callMCPToolReturnTextOnServer(ctx, "report", "create_report", toolArgs)
	if err != nil {
		return withReportDispatchHint("create", err)
	}
	return printReportCreateWithDetailURL(ctx, createText)
}

func printReportCreateWithDetailURL(ctx context.Context, createText string) error {
	var parsed any
	if err := json.Unmarshal([]byte(createText), &parsed); err != nil {
		deps.Out.PrintRaw(createText)
		return nil
	}
	enriched := enrichReportCreateWithDetailURL(ctx, parsed)
	return deps.Out.PrintJSONUnescaped(enriched)
}

func enrichReportCreateWithDetailURL(ctx context.Context, parsed any) any {
	body, ok := parsed.(map[string]any)
	if !ok {
		return parsed
	}
	if url := reportURLFromResponse(body); url != "" {
		attachReportURL(body, url)
		return body
	}
	reportID := reportIDFromCreateResponse(body)
	if reportID == "" {
		body["urlLookupError"] = "create response did not include reportId"
		return body
	}

	detailText, err := callMCPToolReturnTextOnServer(ctx, "report", "get_report_entry_details", map[string]any{
		"report_id": reportID,
	})
	if err != nil {
		body["urlLookupError"] = err.Error()
		return body
	}
	var detail any
	if err := json.Unmarshal([]byte(detailText), &detail); err != nil {
		body["urlLookupError"] = fmt.Sprintf("detail response JSON parse failed: %v", err)
		return body
	}
	url := reportURLFromResponse(detail)
	if url == "" {
		body["urlLookupError"] = "detail response did not include result.url"
		return body
	}
	attachReportURL(body, url)
	return body
}

func reportIDFromCreateResponse(body map[string]any) string {
	if id := reportIDFromMap(body); id != "" {
		return id
	}
	if result, ok := body["result"].(map[string]any); ok {
		if id := reportIDFromMap(result); id != "" {
			return id
		}
	}
	if result, ok := body["result"].(string); ok {
		return strings.TrimSpace(result)
	}
	return ""
}

func reportIDFromMap(m map[string]any) string {
	for _, key := range []string{"reportId", "reportID", "report_id", "report_Id", "report-id"} {
		if id, ok := m[key].(string); ok {
			if id = strings.TrimSpace(id); id != "" {
				return id
			}
		}
	}
	return ""
}

func reportURLFromResponse(detail any) string {
	body, ok := detail.(map[string]any)
	if !ok {
		return ""
	}
	if result, ok := body["result"].(map[string]any); ok {
		for _, key := range []string{"url", "dingtalkOpenUrl"} {
			if url, ok := result[key].(string); ok {
				if url = strings.TrimSpace(url); url != "" {
					return url
				}
			}
		}
		if link, ok := result["dingtalkOpenLink"].(map[string]any); ok {
			if url, ok := link["url"].(string); ok {
				if url = strings.TrimSpace(url); url != "" {
					return url
				}
			}
		}
	}
	for _, key := range []string{"url", "dingtalkOpenUrl"} {
		if url, ok := body[key].(string); ok {
			if url = strings.TrimSpace(url); url != "" {
				return url
			}
		}
	}
	if link, ok := body["dingtalkOpenLink"].(map[string]any); ok {
		if url, ok := link["url"].(string); ok {
			return strings.TrimSpace(url)
		}
	}
	return ""
}

func attachReportURL(body map[string]any, url string) {
	body["url"] = url
	body["dingtalkOpenUrl"] = url
	body["dingtalkOpenLinkText"] = reportDingtalkOpenLinkText
	body["dingtalkOpenMarkdownLink"] = reportDingtalkMarkdownLink(url)
	body["dingtalkOpenLink"] = reportDingtalkOpenLink(url)
	if result, ok := body["result"].(map[string]any); ok {
		result["url"] = url
		result["dingtalkOpenUrl"] = url
		result["dingtalkOpenLinkText"] = reportDingtalkOpenLinkText
		result["dingtalkOpenMarkdownLink"] = reportDingtalkMarkdownLink(url)
		result["dingtalkOpenLink"] = reportDingtalkOpenLink(url)
	}
}

func reportDingtalkMarkdownLink(url string) string {
	return fmt.Sprintf("[%s](%s)", reportDingtalkOpenLinkText, url)
}

func reportDingtalkOpenLink(url string) map[string]any {
	return map[string]any{
		"title":       reportDingtalkOpenLinkText,
		"url":         url,
		"description": reportDingtalkOpenLinkDescription,
	}
}

// resolveReportContentsFromFlags reads contents JSON from one of three sources,
// in priority order:
//
//	--contents-file <path>     read from file (UTF-8)
//	--contents -               read from stdin
//	--contents "<json>"        literal value
//
// Returns INPUT_FILE_NOT_FOUND when --contents-file path does not exist; the
// caller is responsible for parsing the returned string as JSON.
func resolveReportContentsFromFlags(cmd *cobra.Command) (string, error) {
	filePath, _ := cmd.Flags().GetString("contents-file")
	if filePath != "" {
		safePath, err := resolveSafeReportContentsFilePath(filePath)
		if err != nil {
			return "", err
		}
		return readReportContentsFile(safePath)
	}
	raw, _ := cmd.Flags().GetString("contents")
	if raw == "-" {
		return readReportContentsStdin(cmd.InOrStdin())
	}
	return raw, nil
}

func readReportContentsFile(filePath string) (string, error) {
	file, err := reportOpenFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", &CLIError{
				Code:       CodeFileNotFound,
				Message:    fmt.Sprintf("--contents-file: file not found: %s", filePath),
				Suggestion: "确认路径存在，且路径位于当前工作目录内",
				Operation:  "report.create",
				Cause:      err,
			}
		}
		return "", &CLIError{
			Code:      CodeFileNotFound,
			Message:   fmt.Sprintf("--contents-file: cannot read %s: %v", filePath, err),
			Operation: "report.create",
			Cause:     err,
		}
	}
	defer file.Close()
	return readReportContentsLimited(file, fmt.Sprintf("--contents-file %s", filePath))
}

func readReportContentsStdin(reader io.Reader) (string, error) {
	contents, err := readReportContentsLimited(reader, "--contents -")
	if err != nil {
		return "", err
	}
	return contents, nil
}

func readReportContentsLimited(reader io.Reader, source string) (string, error) {
	data, err := io.ReadAll(io.LimitReader(reader, int64(reportContentsMaxBytes)+1))
	if err != nil {
		return "", &CLIError{
			Code:      CodeInvalidJSON,
			Message:   fmt.Sprintf("%s: read failed: %v", source, err),
			Operation: "report.create",
			Cause:     err,
		}
	}
	if len(data) > reportContentsMaxBytes {
		return "", reportContentsTooLargeError(source)
	}
	return string(data), nil
}

func reportContentsTooLargeError(source string) error {
	return &CLIError{
		Code:       CodeInputTooLarge,
		Message:    fmt.Sprintf("%s: contents exceed maximum size of 10MB", source),
		Suggestion: "contents JSON 超过 10MB 限制，不支持分批次提交。请精简内容或拆分为多个独立的日志提交",
		Operation:  "report.create",
	}
}

func resolveSafeReportContentsFilePath(filePath string) (string, error) {
	cleanPath := filepath.Clean(filePath)
	if cleanPath == "." || cleanPath == "-" {
		return "", reportContentsFilePathError(filePath, "must be a file path under the current working directory")
	}
	if pathEscapesUpward(cleanPath) {
		return "", reportContentsFilePathError(filePath, "parent-directory traversal is not allowed")
	}

	absPath, err := reportAbsPath(cleanPath)
	if err != nil {
		return "", reportContentsFilePathError(filePath, err.Error())
	}
	cwd, err := reportGetwd()
	if err != nil {
		return "", &CLIError{
			Code:      CodeFileNotFound,
			Message:   fmt.Sprintf("--contents-file: cannot determine current working directory: %v", err),
			Operation: "report.create",
			Cause:     err,
		}
	}
	cwdAbs, err := reportAbsPath(cwd)
	if err != nil {
		return "", reportContentsFilePathError(filePath, err.Error())
	}
	if !pathWithinRoot(cwdAbs, absPath) {
		return "", reportContentsFilePathError(filePath, "path must stay under the current working directory")
	}
	rootPath, err := reportEvalSymlinks(cwdAbs)
	if err != nil {
		return "", &CLIError{
			Code:      CodeFileNotFound,
			Message:   fmt.Sprintf("--contents-file: cannot resolve current working directory: %v", err),
			Operation: "report.create",
			Cause:     err,
		}
	}
	info, err := reportStat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", &CLIError{
				Code:       CodeFileNotFound,
				Message:    fmt.Sprintf("--contents-file: file not found: %s", absPath),
				Suggestion: "确认路径存在，且路径位于当前工作目录内",
				Operation:  "report.create",
				Cause:      err,
			}
		}
		return "", &CLIError{
			Code:      CodeFileNotFound,
			Message:   fmt.Sprintf("--contents-file: cannot read %s: %v", absPath, err),
			Operation: "report.create",
			Cause:     err,
		}
	}
	if info.IsDir() {
		return "", reportContentsFilePathError(filePath, "must point to a file, not a directory")
	}
	realPath, err := reportEvalSymlinks(absPath)
	if err != nil {
		return "", &CLIError{
			Code:      CodeFileNotFound,
			Message:   fmt.Sprintf("--contents-file: cannot resolve %s: %v", absPath, err),
			Operation: "report.create",
			Cause:     err,
		}
	}
	if !pathWithinRoot(rootPath, realPath) {
		return "", reportContentsFilePathError(filePath, "resolved path must stay under the current working directory")
	}
	return realPath, nil
}

func pathEscapesUpward(cleanPath string) bool {
	return cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(os.PathSeparator))
}

func pathWithinRoot(rootPath, targetPath string) bool {
	rel, err := reportRelPath(rootPath, targetPath)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel))
}

func reportContentsFilePathError(filePath, reason string) error {
	return &CLIError{
		Code:       CodeInvalidPath,
		Message:    fmt.Sprintf("--contents-file: invalid path %q: %s", filePath, reason),
		Suggestion: "使用当前工作目录下的 JSON 文件；不要使用 ../ 路径或指向目录外的符号链接",
		Operation:  "report.create",
	}
}

func validateAndNormalizeReportContents(contents []map[string]any) error {
	if len(contents) == 0 {
		return reportContentsValidationError("contents must contain at least one item")
	}
	for i, item := range contents {
		if item == nil {
			return reportContentsValidationError(fmt.Sprintf("contents[%d] must be an object", i))
		}
		key, err := reportRequiredString(item, i, "key")
		if err != nil {
			return err
		}
		sort, err := normalizeReportSort(item, i)
		if err != nil {
			return err
		}
		content, err := reportRequiredString(item, i, "content")
		if err != nil {
			return err
		}
		if _, ok := item["type"]; !ok {
			return reportContentsValidationError(fmt.Sprintf("contents[%d].type is required", i))
		}
		fieldType, err := normalizeReportFieldType(item["type"], i)
		if err != nil {
			return err
		}
		if _, ok := item["contentType"]; !ok {
			return reportContentsValidationError(fmt.Sprintf("contents[%d].contentType is required", i))
		}
		contentType, err := normalizeReportContentType(item["contentType"], fieldType, i)
		if err != nil {
			return err
		}

		item["key"] = key
		item["sort"] = sort
		item["content"] = content
		item["type"] = fieldType
		item["contentType"] = contentType
	}
	return nil
}

func reportRequiredString(item map[string]any, index int, field string) (string, error) {
	v, ok := item[field]
	if !ok {
		return "", reportContentsValidationError(fmt.Sprintf("contents[%d].%s is required", index, field))
	}
	s, ok := v.(string)
	if !ok {
		return "", reportContentsValidationError(fmt.Sprintf("contents[%d].%s must be a string", index, field))
	}
	if field == "content" {
		return s, nil
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", reportContentsValidationError(fmt.Sprintf("contents[%d].%s cannot be empty", index, field))
	}
	return s, nil
}

func normalizeReportSort(item map[string]any, index int) (string, error) {
	v, ok := item["sort"]
	if !ok {
		return "", reportContentsValidationError(fmt.Sprintf("contents[%d].sort is required", index))
	}
	raw, ok := reportScalarToString(v)
	if !ok {
		return "", reportContentsValidationError(fmt.Sprintf("contents[%d].sort must be an integer or integer string", index))
	}
	sort := strings.TrimSpace(raw)
	if sort == "" {
		return "", reportContentsValidationError(fmt.Sprintf("contents[%d].sort cannot be empty", index))
	}
	if _, err := strconv.Atoi(sort); err != nil {
		return "", reportContentsValidationError(fmt.Sprintf("contents[%d].sort=%q is invalid; use template get field_sort", index, raw))
	}
	return sort, nil
}

func normalizeReportFieldType(v any, index int) (string, error) {
	raw, ok := reportScalarToString(v)
	if !ok {
		return "", reportContentsValidationError(fmt.Sprintf("contents[%d].type must be one of 1/2/3/5/7/8/9", index))
	}
	t := strings.ToLower(strings.TrimSpace(raw))
	aliases := map[string]string{
		"1": "1", "text": "1", "markdown": "1", "文本": "1",
		"2": "2", "number": "2", "num": "2", "数字": "2",
		"3": "3", "single": "3", "single_select": "3", "radio": "3", "单选": "3",
		"5": "5", "date": "5", "日期": "5",
		"7": "7", "multi": "7", "multi_select": "7", "checkbox": "7", "多选": "7",
		"8": "8", "image": "8", "picture": "8", "图片": "8",
		"9": "9", "attachment": "9", "file": "9", "附件": "9",
	}
	if normalized, ok := aliases[t]; ok {
		return normalized, nil
	}
	return "", reportContentsValidationError(fmt.Sprintf("contents[%d].type=%q is invalid; use template get field_type, such as \"1\" for text", index, raw))
}

func normalizeReportContentType(v any, fieldType string, index int) (string, error) {
	raw, ok := reportScalarToString(v)
	if !ok {
		return "", reportContentsValidationError(fmt.Sprintf("contents[%d].contentType must be a string", index))
	}
	ct := strings.ToLower(strings.TrimSpace(raw))
	if fieldType == "1" {
		if ct == "markdown" || ct == "text" {
			return "markdown", nil
		}
		return "", reportContentsValidationError(fmt.Sprintf("contents[%d].contentType=%q is invalid for type=1; use \"markdown\"", index, raw))
	}
	if ct == "origin" || ct == "raw" {
		return "origin", nil
	}
	return "", reportContentsValidationError(fmt.Sprintf("contents[%d].contentType=%q is invalid for type=%s; use \"origin\"", index, raw, fieldType))
}

func reportScalarToString(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case float64:
		if x != float64(int64(x)) {
			return "", false
		}
		return strconv.FormatInt(int64(x), 10), true
	case float32:
		if x != float32(int64(x)) {
			return "", false
		}
		return strconv.FormatInt(int64(x), 10), true
	case int:
		return strconv.Itoa(x), true
	case int64:
		return strconv.FormatInt(x, 10), true
	case int32:
		return strconv.FormatInt(int64(x), 10), true
	case json.Number:
		if _, err := strconv.Atoi(x.String()); err != nil {
			return "", false
		}
		return x.String(), true
	default:
		return "", false
	}
}

func reportContentsValidationError(message string) error {
	return &CLIError{
		Code:       CodeInvalidJSON,
		Message:    message,
		Suggestion: "contents 每项必须含 key/sort/content/contentType/type；key/sort/type 来自 report template get，type=1 时 contentType=markdown，其余类型用 origin",
		Operation:  "report.create",
	}
}
