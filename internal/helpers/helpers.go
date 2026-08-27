package helpers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// Re-export shared command helpers as package-level aliases so existing product
// files continue to compile with their current (unexported) call sites. This
// avoids a mass-rename while keeping reusable command-resolution and flag
// utilities in cmdutil.
var (
	groupRunE                       = cmdutil.GroupRunE
	hintSubCmd                      = cmdutil.HintSubCmd
	mustGetFlag                     = cmdutil.MustGetFlag
	flagOrFallback                  = cmdutil.FlagOrFallback
	mustFlagOrFallback              = cmdutil.MustFlagOrFallback
	validateRequiredFlags           = cmdutil.ValidateRequiredFlags
	validateRequiredFlagWithAliases = cmdutil.ValidateRequiredFlagWithAliases
	parseISOTimeToMillis            = cmdutil.ParseISOTimeToMillis
	validateTimeRange               = cmdutil.ValidateTimeRange
	helperSleep                     = time.Sleep
	helperAfter                     = time.After
)

// newGroupCommand declares the ordinary navigation policy used by helper
// command containers. The unified framework compiles this declaration into
// Cobra behavior and command-resolution metadata.
func newGroupCommand(command *cobra.Command) *cobra.Command {
	corecmd.ApplyGroupPolicy(command, corecmd.GroupPolicy{
		Mode:        corecmd.GroupNavigationOnly,
		Positionals: corecmd.PositionalsReject,
		Recovery:    corecmd.RecoverySibling,
	})
	return command
}

// newDeepGroupCommand declares a navigation container whose typo recovery may
// teach exact descendant paths (for example sheet read -> sheet range read).
func newDeepGroupCommand(command *cobra.Command) *cobra.Command {
	corecmd.ApplyGroupPolicy(command, corecmd.GroupPolicy{
		Mode:        corecmd.GroupNavigationOnly,
		Positionals: corecmd.PositionalsReject,
		Recovery:    corecmd.RecoveryDeep,
	})
	return command
}

// newHybridGroupCommand declares a business command that also owns children.
// Its existing RunE remains the command's default action.
func newHybridGroupCommand(command *cobra.Command) *cobra.Command {
	corecmd.ApplyGroupPolicy(command, corecmd.GroupPolicy{
		Mode:        corecmd.GroupHybrid,
		Positionals: corecmd.PositionalsReject,
		Recovery:    corecmd.RecoverySibling,
	})
	return command
}

// Deps holds shared dependencies injected from the host application.
type Deps struct {
	Caller edition.ToolCaller
	Out    *Formatter
}

// deps is the package-level dependency holder, set during registration.
var deps *Deps

// InitDeps initializes shared dependencies for all product commands.
// Must be called before any product command's RunE executes (typically
// during command tree construction in newLegacyPublicCommands).
func InitDeps(caller edition.ToolCaller) {
	deps = &Deps{
		Caller: caller,
		Out:    NewFormatter(),
	}
}

// GetFormatter returns the shared output formatter for use by sibling packages.
func GetFormatter() *Formatter {
	if deps == nil {
		return NewFormatter()
	}
	return deps.Out
}

// copyFlags copies specified flags from source command to target command.
// This is useful when creating alias commands that reuse another command's RunE.
func copyFlags(src, dst *cobra.Command, flagNames ...string) {
	for _, name := range flagNames {
		if f := src.Flags().Lookup(name); f != nil {
			dst.Flags().AddFlag(f)
		}
	}
}

// GetCaller returns the shared ToolCaller for use by sibling packages.
func GetCaller() edition.ToolCaller {
	if deps == nil {
		return nil
	}
	return deps.Caller
}

// cmdToProduct maps CLI command names to MCP server IDs for direct routing.
var cmdToProduct = map[string]string{
	"aitable": "aitable", "calendar": "calendar", "contact": "contact",
	"todo": "todo", "doc": "doc", "chat": "chat",
	"oa": "oa", "mail": "mail", "ding": "ding",
	"devdoc":     "devdoc",
	"attendance": "attendance",
	"live":       "live", "aiapp": "aiapp",
	"minutes":    "minutes",
	"finance":    "finance",
	"report":     "report",
	"drive":      "drive",
	"blackboard": "blackboard",
	"credit":     "credit-ep",
	"docparse":   "docparse",
	"aidesign":   "aidesign",
	"sheet":      "sheet",
	"wiki":       "wiki",
	"aisearch":   "aisearch",
	"hrbrain":    "hrbrain",
	"yida":       "yida",
	// vendor extension command routing (kept here for resolveProductID)
	"unified-toolkit": "unified-toolkit",
	"outbound-call":   "outbound-call",
	"discovery":       "discovery",
	"ai-sincere-hire": "ai-sincere-hire",
	"contract":        "contract",
	"oa-plus":         "oa",
	"pat":             "pat",
	"edu-contact":     "edu-contact",
	"edu-group":       "edu-group",
	"edu-app":         "edu-app",
	"agoal":           "agoal",
}

// resolveProductID determines the MCP server ID from the CLI args.
// It scans os.Args for the first known product command name.
func resolveProductID() string {
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if pid, ok := cmdToProduct[arg]; ok {
			return pid
		}
	}
	return ""
}

// callMCPToolReturnText calls an MCP tool and returns the first text content,
// used when the caller needs to parse the result (e.g. extracting an eventId).
func callMCPToolReturnText(ctx context.Context, toolName string, args map[string]any) (string, error) {
	serverID := resolveProductID()
	if serverID == "" {
		return "", &CLIError{
			Code:    CodeMCPToolError,
			Message: fmt.Sprintf("cannot resolve product for tool %q", toolName),
		}
	}
	return callMCPToolReturnTextOnServer(ctx, serverID, toolName, args)
}

func callMCPToolReturnTextOnServer(ctx context.Context, serverID, toolName string, args map[string]any) (string, error) {
	result, err := deps.Caller.CallTool(ctx, serverID, toolName, args)
	return parseMCPToolTextResult(serverID, toolName, result, err)
}

// CallMCPReadToolTextOnServer performs a read-only lookup needed to construct a
// semantic Shortcut dry-run plan. Under ordinary execution it is identical to
// CallMCPToolTextOnServer. Under --dry-run it requires the host's optional
// ReadToolCaller capability; if unavailable, it fails closed instead of
// returning a synthetic dry-run envelope that looks like business data.
func CallMCPReadToolTextOnServer(serverID, toolName string, args map[string]any) (string, error) {
	return CallMCPReadToolTextOnServerContext(context.Background(), serverID, toolName, args)
}

// CallMCPReadToolTextOnServerContext is the cancellable form used by Cobra
// commands and composite shortcuts. Keeping the caller context attached to the
// transport lets SIGTERM/parent deadlines stop an in-flight MCP read and return
// a structured error instead of leaving the CLI silent until the host kills it.
func CallMCPReadToolTextOnServerContext(ctx context.Context, serverID, toolName string, args map[string]any) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return callMCPReadToolReturnTextOnServer(ctx, serverID, toolName, args)
}

func callMCPReadToolReturnTextOnServer(ctx context.Context, serverID, toolName string, args map[string]any) (string, error) {
	if !IsReadToolName(toolName) {
		return "", &CLIError{
			Code:    CodeMCPToolError,
			Message: fmt.Sprintf("tool %q is not allowed on the dry-run read channel", toolName),
		}
	}
	if deps == nil || deps.Caller == nil {
		return "", &CLIError{
			Code:    CodeMCPToolError,
			Message: "MCP caller is not initialized",
		}
	}
	if !deps.Caller.DryRun() {
		return callMCPToolReturnTextOnServer(ctx, serverID, toolName, args)
	}
	readCaller, ok := deps.Caller.(edition.ReadToolCaller)
	if !ok {
		return "", &CLIError{
			Code:    CodeMCPToolError,
			Message: "当前运行时不支持在 --dry-run 下执行只读解析",
		}
	}
	result, err := readCaller.CallReadTool(ctx, serverID, toolName, args)
	return parseMCPToolTextResult(serverID, toolName, result, err)
}

// IsReadToolName is the fail-closed naming contract for the dry-run read
// channel. Both the Shortcut runtime and the helper boundary enforce it so a
// future direct helper caller cannot accidentally route a write tool through
// ReadToolCaller.
func IsReadToolName(toolName string) bool {
	toolName = strings.TrimSpace(strings.ToLower(toolName))
	if toolName == "enterprise_person_search" {
		return true
	}
	for _, prefix := range []string{
		"get_", "list_", "query_", "search_", "unread_",
	} {
		if strings.HasPrefix(toolName, prefix) {
			return true
		}
	}
	return false
}

func parseMCPToolTextResult(serverID, toolName string, result *edition.ToolResult, err error) (string, error) {
	if err != nil {
		if patErr := reclassifyPATFromError(err); patErr != nil {
			return "", patErr
		}
		return "", WrapError(err)
	}
	if result == nil {
		return "", &CLIError{
			Code:       CodeMCPToolError,
			Message:    "MCP 工具返回 nil result，无法判断操作结果",
			Suggestion: "不要把空回复当作成功；请重试并携带 operation/trace 信息排查服务端",
			Operation:  serverID + "/" + toolName,
		}
	}
	for _, c := range result.Content {
		if c.Type == "text" && strings.TrimSpace(c.Text) != "" {
			dumpRawToolResponse(serverID, toolName, c.Text)
			var errBody map[string]any
			if json.Unmarshal([]byte(c.Text), &errBody) == nil {
				if _, ok := getDWSGatewayErrorCode(errBody); ok {
					return "", &CLIError{
						Code:       CodeAuthTokenExpired,
						Message:    c.Text,
						Suggestion: authExpiredSuggestion(),
					}
				}
				if isNotLoggedInError(errBody) {
					return "", &CLIError{
						Code:       CodeAuthNotConfigured,
						Message:    "当前未登录",
						Suggestion: notLoggedInSuggestion(),
					}
				}
				if patErr := classifyPATError(errBody); patErr != nil {
					return "", patErr
				}
				if isBusinessError(errBody) {
					return "", &CLIError{
						Code: CodeMCPToolError,
						// 注意：这里必须保留原始响应 JSON。此路径是数据编排层
						// （如 drive list --depth 的限流重试）的输入，下游会从
						// Message 反解析 errorCode；人话提取（含 code/logId 附加）
						// 只用于 callMCPToolInternalOptsContext 的终端展示路径。
						Message:    c.Text,
						Suggestion: suggestForBusinessError(errBody),
					}
				}
			}
			return c.Text, nil
		}
	}
	// Some legacy tools intentionally use an empty text response as an
	// acknowledgement. This low-level parser cannot know the business contract,
	// so preserve that representation. Data-returning callers must validate the
	// operation-specific shape (for example records/tables/valid) before they
	// report success. Orchestrators whose contract requires a business result use
	// CallMCPWriteDataStrict and may prove an unknown effect by independent read-back.
	return "", nil
}

// CallMCPToolTextOnServer invokes an MCP tool and returns its raw text response
// WITHOUT printing anything, applying the same error classification as the
// print path. Exported for the shortcut layer's multi-step ("smart") shortcuts,
// which chain several tool calls and need each intermediate result as data.
func CallMCPToolTextOnServer(serverID, toolName string, args map[string]any) (string, error) {
	return CallMCPToolTextOnServerContext(context.Background(), serverID, toolName, args)
}

// CallMCPToolTextOnServerContext invokes one MCP tool while preserving the
// command's cancellation and deadline. Legacy callers may continue using
// CallMCPToolTextOnServer, which intentionally retains background context.
func CallMCPToolTextOnServerContext(ctx context.Context, serverID, toolName string, args map[string]any) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return callMCPToolReturnTextOnServer(ctx, serverID, toolName, args)
}

// CallMCPToolDataOnServer invokes one tool without printing and decodes its
// JSON text payload. Framework renderers use this seam so the business request
// is executed exactly once and presentation remains a separate step.
func CallMCPToolDataOnServer(ctx context.Context, serverID, toolName string, args map[string]any) (any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	text, err := callMCPToolReturnTextOnServer(ctx, serverID, toolName, args)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) == "" {
		return map[string]any{}, nil
	}
	var data any
	decoder := json.NewDecoder(strings.NewReader(text))
	if err := decoder.Decode(&data); err != nil {
		return nil, apperrors.NewInternal(fmt.Sprintf("解析 %s 返回失败: %v", toolName, err))
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("存在多个 JSON 值")
		}
		return nil, apperrors.NewInternal(fmt.Sprintf("解析 %s 返回失败: %v", toolName, err))
	}
	return data, nil
}

// callMCPTool 是通用的 MCP 工具调用入口：自动路由 → 调用 → 格式化输出。
// 通过 resolveProductID() 自动确定目标 MCP Server，JSON 输出使用默认的 HTML 转义。
func callMCPTool(toolName string, args map[string]any) error {
	return callMCPToolContext(context.Background(), toolName, args)
}

func callMCPToolContext(ctx context.Context, toolName string, args map[string]any) error {
	return callMCPToolInternalOptsContext(ctx, "", toolName, args, false)
}

// callMCPToolUnescaped 与 callMCPTool 功能相同，但 JSON 输出禁用 HTML 转义。
// 适用于返回值中包含 URL（如 presignedUrl）的接口，避免 & 被转义为 \u0026。
// 当前仅 minutes upload 的三个命令使用此函数。
func callMCPToolUnescaped(toolName string, args map[string]any) error {
	return callMCPToolInternalOpts("", toolName, args, true)
}

// callMCPToolOnServer 在指定的 MCP Server 上调用工具，跳过 resolveProductID() 的自动路由。
// 用于需要显式指定 serverID 的场景（如 credit 等多 server 产品）。
func callMCPToolOnServer(serverID, toolName string, args map[string]any) error {
	return callMCPToolInternalOptsContext(context.Background(), serverID, toolName, args, false)
}

// CallMCPToolOnServer is the exported version of callMCPToolOnServer for use
// by extension packages that live in separate Go packages.
func CallMCPToolOnServer(serverID, toolName string, args map[string]any) error {
	return callMCPToolOnServer(serverID, toolName, args)
}

// CallMCPToolOnServerContext is the cancellable print-path variant for
// Shortcut execution. It preserves the same output and error projection as the
// legacy wrapper while allowing the root signal context to abort transport.
func CallMCPToolOnServerContext(ctx context.Context, serverID, toolName string, args map[string]any) error {
	return callMCPToolInternalOptsContext(ctx, serverID, toolName, args, false)
}

// GroupRunE is the exported version of groupRunE for use by extension packages.
func GroupRunE(cmd *cobra.Command, args []string) error {
	return groupRunE(cmd, args)
}

// MustGetStringFlag retrieves a string flag, falling back to inherited flags.
func MustGetStringFlag(cmd *cobra.Command, name string) string {
	val, _ := cmd.Flags().GetString(name)
	if val == "" {
		val, _ = cmd.InheritedFlags().GetString(name)
	}
	return val
}

// callMCPToolInternalOpts 是所有 MCP 工具调用的核心实现。
//
// 参数说明：
//   - explicitServerID: 显式指定的 MCP Server ID，为空时自动路由
//   - toolName:         MCP 工具名称（如 "create_upload_session"）
//   - args:             工具调用参数，会被序列化为 JSON 传给 MCP Server
//   - unescapeHTML:     是否禁用 JSON 输出的 HTML 转义（仅影响最终输出格式）
//
// 执行流程：
//  1. DryRun 模式：仅打印工具名和参数，不实际调用
//  2. 调用 MCP Server 获取结果
//  3. 错误分类：网关错误 → 未登录 → PAT 错误 → 业务错误
//  4. 根据 --format 标志选择输出格式（json / table / raw）
func callMCPToolInternalOpts(explicitServerID, toolName string, args map[string]any, unescapeHTML bool) error {
	return callMCPToolInternalOptsContext(context.Background(), explicitServerID, toolName, args, unescapeHTML)
}

func callMCPToolInternalOptsContext(ctx context.Context, explicitServerID, toolName string, args map[string]any, unescapeHTML bool) error {
	if ctx == nil {
		ctx = context.Background()
	}

	// DryRun 模式：仅预览工具名和参数，不实际调用 MCP Server
	if deps.Caller.DryRun() {
		// Pre-check: let decoration layers (e.g. delegation auth) validate
		// before rendering the preview. Only fires when the caller implements
		// the package-internal dryRunValidator interface (i.e. the delegation
		// auth decorator is installed); undecorated callers skip this entirely.
		if v, ok := deps.Caller.(dryRunValidator); ok {
			preCheckServerID := explicitServerID
			if preCheckServerID == "" {
				preCheckServerID = resolveProductID()
			}
			if err := v.ensureDelegationAuth(ctx, preCheckServerID, toolName, args); err != nil {
				return err
			}
		}

		if deps.Caller.Format() == "json" {
			return deps.Out.PrintJSON(map[string]any{
				"dry_run":   true,
				"executed":  false,
				"tool":      toolName,
				"arguments": args,
			})
		}
		bold := color.New(color.FgYellow, color.Bold)
		bold.Println("[DRY-RUN] Preview only, not executed:")
		deps.Out.PrintKeyValue("Tool", toolName)
		if args != nil {
			argsJSON, _ := json.MarshalIndent(args, "  ", "  ")
			deps.Out.PrintKeyValue("Arguments", "\n  "+string(argsJSON))
		}
		return nil
	}

	// 确定目标 MCP Server：优先使用显式指定的 serverID，否则自动路由
	serverID := explicitServerID
	if serverID == "" {
		serverID = resolveProductID()
	}

	// 调用 MCP Server
	result, err := deps.Caller.CallTool(ctx, serverID, toolName, args)
	if err != nil {
		if patErr := reclassifyPATFromError(err); patErr != nil {
			return patErr
		}
		return WrapErrorWithOperation(err, serverID+"/"+toolName)
	}

	// 根据 unescapeHTML 选择 JSON 输出函数：
	// - false（默认）：使用 PrintJSON，& 会被转义为 \u0026
	// - true：使用 PrintJSONUnescaped，保留原始字符（适用于含 URL 的返回值）
	printJSON := deps.Out.PrintJSON
	if unescapeHTML {
		printJSON = deps.Out.PrintJSONUnescaped
	}

	for _, c := range result.Content {
		if c.Type == "text" {
			dumpRawToolResponse(serverID, toolName, c.Text)
			// 尝试将返回文本解析为 JSON，进行错误分类
			var errBody map[string]any
			if json.Unmarshal([]byte(c.Text), &errBody) == nil {
				// errBody == nil 表示文本为 "null"，服务端返回了空响应。
				// 仅对已确认“空响应=写成功”契约的 permission/member update/remove
				// 工具适配为空对象 {}，避免消费方解析 null 时出错；其它工具的
				// 合法 null 保持原样输出，不改变未版本化的机器输出契约。
				if errBody == nil {
					if nullOnSuccessTools[toolName] {
						if deps.Caller.Format() == "json" {
							return printJSON(map[string]any{})
						}
						deps.Out.PrintRaw("{}")
						return nil
					}
					return renderLegacyMCPText(toolName, c.Text, unescapeHTML)
				}
				// 网关层错误（如 token 过期）
				if _, ok := getDWSGatewayErrorCode(errBody); ok {
					return &CLIError{Code: CodeAuthTokenExpired, Message: c.Text, Suggestion: authExpiredSuggestion()}
				}
				// 未登录错误
				if isNotLoggedInError(errBody) {
					return &CLIError{Code: CodeAuthNotConfigured, Message: "当前未登录", Suggestion: notLoggedInSuggestion()}
				}
				// PAT（个人访问令牌）相关错误
				if patErr := classifyPATError(errBody); patErr != nil {
					return patErr
				}
				// 业务逻辑错误
				if isBusinessError(errBody) {
					return &CLIError{Code: CodeMCPToolError, Message: businessErrorDisplayMessage(errBody, c.Text), Suggestion: suggestForBusinessError(errBody)}
				}
			}

			return renderLegacyMCPText(toolName, c.Text, unescapeHTML)
		}
	}
	// 无 text 类型内容时，将整个 result 对象序列化为 JSON 输出
	return printJSON(result)
}

// nullOnSuccessTools lists MCP tools whose server contract is confirmed to
// return a literal JSON null for successful no-payload writes (permission /
// member update/remove). Only these get the null→{} adaptation; every other
// tool keeps its raw null output so the shared machine-output contract stays
// unchanged.
var nullOnSuccessTools = map[string]bool{
	"update_permission": true,
	"remove_permission": true,
	"update_member":     true,
	"remove_member":     true,
}

// RenderLegacyMCPText renders an already-fetched MCP text response through the
// exact legacy formatter. It lets dual validation execute the business request
// once, validate a shadow unified result, and still preserve legacy bytes.
func RenderLegacyMCPText(toolName, text string) error {
	return renderLegacyMCPText(toolName, text, false)
}

func renderLegacyMCPText(toolName, text string, unescapeHTML bool) error {
	printJSON := deps.Out.PrintJSON
	if unescapeHTML {
		printJSON = deps.Out.PrintJSONUnescaped
	}
	flagFormat := deps.Caller.Format()
	if flagFormat == "json" {
		var parsed any
		if err := json.Unmarshal([]byte(text), &parsed); err == nil {
			return printJSON(parsed)
		}
	}
	if toolName == "search_open_platform_docs" && flagFormat == "table" {
		if formatted := formatDevdocSearchTable(text); formatted {
			return nil
		}
	}
	if unescapeHTML {
		var parsed any
		if err := json.Unmarshal([]byte(text), &parsed); err == nil {
			return printJSON(parsed)
		}
	}
	deps.Out.PrintRaw(text)
	return nil
}

// dumpRawToolResponse emits one opt-in lower-layer record for live projection
// audits. Responses may contain business data, so normal CLI runs never emit
// this line; an auditor must explicitly set DWS_DUMP_RAW.
func dumpRawToolResponse(serverID, toolName, text string) {
	if strings.TrimSpace(os.Getenv("DWS_DUMP_RAW")) == "" {
		return
	}
	fmt.Fprintln(os.Stderr, formatRawDumpLine(serverID, toolName, text))
}

func formatRawDumpLine(serverID, toolName, text string) string {
	raw := json.RawMessage(strings.TrimSpace(text))
	if json.Valid(raw) {
		var compact bytes.Buffer
		_ = json.Compact(&compact, raw)
		raw = compact.Bytes()
	} else {
		encoded, _ := json.Marshal(text)
		raw = encoded
	}
	return "DWSRAW\t" + serverID + "\t" + toolName + "\t" + string(raw)
}

// formatDevdocSearchTable formats devdoc search JSON results as a table.
// Returns true on success, false if the JSON cannot be parsed.
func formatDevdocSearchTable(raw string) bool {
	var resp struct {
		Result struct {
			Items       []struct{ Title, URL string }
			CurrentPage int  `json:"currentPage"`
			TotalCount  int  `json:"totalCount"`
			HasMore     bool `json:"hasMore"`
		}
	}
	if json.Unmarshal([]byte(raw), &resp) != nil {
		return false
	}
	items := resp.Result.Items
	if len(items) == 0 {
		deps.Out.PrintInfo("no matching documents")
		return true
	}
	headers := []string{"标题", "URL"}
	rows := make([][]string, len(items))
	for i, it := range items {
		title := stripHTMLEm(it.Title)
		rows[i] = []string{title, it.URL}
	}
	deps.Out.PrintTable(headers, rows)
	pageInfo := fmt.Sprintf("page %d, total %d", resp.Result.CurrentPage, resp.Result.TotalCount)
	if resp.Result.HasMore {
		pageInfo += ", use --page " + fmt.Sprintf("%d", resp.Result.CurrentPage+1) + " for more"
	}
	deps.Out.PrintDim(pageInfo)
	return true
}

// stripHTMLEm removes <em></em> tags, keeping inner text.
func stripHTMLEm(s string) string {
	s = strings.ReplaceAll(s, "<em>", "")
	s = strings.ReplaceAll(s, "</em>", "")
	return s
}

// getCurrentUserID fetches the current user's userId via the contact MCP server.
func getCurrentUserID(ctx context.Context) (string, error) {
	result, err := deps.Caller.CallTool(ctx, "contact", "get_current_user_profile", nil)
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %w", err)
	}
	for _, c := range result.Content {
		if c.Type != "text" || c.Text == "" {
			continue
		}
		var data struct {
			Result []struct {
				OrgEmployeeModel struct {
					UserID string `json:"userId"`
				} `json:"orgEmployeeModel"`
			} `json:"result"`
		}
		if json.Unmarshal([]byte(c.Text), &data) == nil && len(data.Result) > 0 && data.Result[0].OrgEmployeeModel.UserID != "" {
			return data.Result[0].OrgEmployeeModel.UserID, nil
		}
		var data2 struct {
			Result struct {
				UserID string `json:"userId"`
			} `json:"result"`
		}
		if json.Unmarshal([]byte(c.Text), &data2) == nil && data2.Result.UserID != "" {
			return data2.Result.UserID, nil
		}
		var flat map[string]any
		if json.Unmarshal([]byte(c.Text), &flat) == nil {
			if arr, ok := flat["result"].([]any); ok && len(arr) > 0 {
				if m, ok := arr[0].(map[string]any); ok {
					if oem, ok := m["orgEmployeeModel"].(map[string]any); ok {
						if uid, ok := oem["userId"].(string); ok && uid != "" {
							return uid, nil
						}
					}
				}
			}
		}
	}
	return "", fmt.Errorf("cannot parse userId from get_current_user_profile response")
}

// classifyPATError checks if a parsed JSON body contains a PAT permission error code.
// Returns a *PATError if matched, nil otherwise.
func classifyPATError(body map[string]any) error {
	for _, key := range []string{"code", "errorCode"} {
		if code, ok := body[key].(string); ok && patNoPermissionCodes[code] {
			return &PATError{RawJSON: cleanPATJSON(body, code)}
		}
	}
	return nil
}

// reclassifyPATFromError inspects an error returned by the framework (which may
// have classified a PAT response as a generic business error) and converts it
// to a *PATError if the error message contains a known PAT permission code.
func reclassifyPATFromError(err error) error {
	if _, ok := err.(*PATError); ok {
		return err
	}
	msg := err.Error()
	for code := range patNoPermissionCodes {
		if strings.Contains(msg, code) {
			return &PATError{RawJSON: buildMinimalPATJSON(code)}
		}
	}
	return nil
}

func buildMinimalPATJSON(code string) string {
	out := map[string]any{"success": false, "code": code}
	b, _ := json.MarshalIndent(out, "", "  ")
	return string(b)
}

// isBusinessError checks if a parsed JSON body represents a business-level error.
func isBusinessError(body map[string]any) bool {
	if v, ok := body["error"]; ok {
		switch t := v.(type) {
		case string:
			if strings.TrimSpace(t) != "" {
				return true
			}
		case map[string]any:
			if len(t) > 0 {
				return true
			}
		case []any:
			if len(t) > 0 {
				return true
			}
		default:
			if t != nil {
				return true
			}
		}
	}
	if v, ok := body["status"].(string); ok && strings.EqualFold(strings.TrimSpace(v), "error") {
		return true
	}
	for _, key := range []string{"errorCode", "error_code", "errcode", "err_code", "code"} {
		if isErrorCodeValue(body[key]) {
			return true
		}
	}
	if v, ok := body["success"].(bool); ok && !v {
		return true
	}
	if v, ok := body["success"].(string); ok && strings.EqualFold(v, "false") {
		return true
	}
	return false
}

func isErrorCodeValue(v any) bool {
	switch t := v.(type) {
	case string:
		code := strings.TrimSpace(t)
		if code == "" {
			return false
		}
		switch strings.ToLower(code) {
		case "0", "ok", "success", "succeed":
			return false
		default:
			return true
		}
	case float64:
		return t != 0
	case int:
		return t != 0
	case int64:
		return t != 0
	case json.Number:
		return strings.TrimSpace(t.String()) != "" && t.String() != "0"
	default:
		return false
	}
}

// isNotLoggedInError checks if the error body indicates missing authentication.
func isNotLoggedInError(body map[string]any) bool {
	if errMsg, ok := body["error"].(string); ok {
		if strings.Contains(errMsg, "Missing service_id or access_key") {
			return true
		}
	}
	return false
}

// notLoggedInSuggestion returns a mode-aware hint for not-logged-in errors.
func notLoggedInSuggestion() string {
	if edition.Get().IsEmbedded {
		return "请先登录"
	}
	return "请先登录：dws auth login"
}

// authExpiredSuggestion returns a mode-aware hint for auth errors.
func authExpiredSuggestion() string {
	if edition.Get().IsEmbedded {
		return "Auth expired, re-run previous command (max 2 retries)"
	}
	return "Re-authenticate: dws auth login"
}

// dwsGatewayErrors is the set of DWS gateway-level auth error codes that
// must be surfaced as CodeAuthTokenExpired so the runner / overlay knows the
// current bearer token has been rejected.
//
// TOKEN_VERIFIED_FAILED / USER_TOKEN_ILLEGAL come from upstream DingTalk
// services that pass through the gateway; they are server-side rejections
// of an otherwise locally-valid token.
var dwsGatewayErrors = map[string]bool{
	"DWS_SERVICE_UNAUTHORIZED": true,
	"DWS_AUTH_SERVICE_FAILED":  true,
	"TOKEN_VERIFIED_FAILED":    true,
	"USER_TOKEN_ILLEGAL":       true,
}

// getDWSGatewayErrorCode extracts a DWS gateway error code from errBody.
//
// Field-name coverage: the gateway and upstream services are inconsistent
// about which key carries the code — we have observed all of "errorCode",
// "error_code" and "code" in the wild — so check all three. The transport
// layer's ExtractServerDiagnosticsFromMap normalises into ServerErrorCode,
// but ClassifyToolResultContent is fed the raw content map and must do its
// own lookup.
func getDWSGatewayErrorCode(errBody map[string]any) (string, bool) {
	for _, key := range []string{"errorCode", "error_code", "code"} {
		if code, ok := errBody[key].(string); ok && dwsGatewayErrors[code] {
			return code, true
		}
	}
	return "", false
}

// suggestForBusinessError returns a user-facing suggestion for known business
// error patterns in a parsed JSON body, or "" if no specific suggestion applies.
func suggestForBusinessError(body map[string]any) string {
	return suggestForBusinessErrorText(businessErrorMessage(body))
}

// businessErrorMessage extracts the human-readable message from a parsed error
// body, checking errorMsg > errorMessage > message > error. Returns "" if none present.
func businessErrorMessage(body map[string]any) string {
	for _, k := range []string{"errorMsg", "errorMessage", "message", "error"} {
		if v, ok := body[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// businessErrorDisplayMessage extracts the human-readable message from a parsed
// error body, appends the backend error code and logId if present (for
// traceability), and falls back to rawText if no message field is found.
func businessErrorDisplayMessage(body map[string]any, rawText string) string {
	msg := businessErrorMessage(body)
	if msg == "" {
		return rawText
	}
	var extras []string
	for _, k := range []string{"errorCode", "error_code", "code"} {
		if code, ok := body[k].(string); ok && code != "" && !strings.Contains(msg, code) {
			extras = append(extras, "code: "+code)
			break
		}
	}
	if logId, ok := body["logId"].(string); ok && logId != "" && !strings.Contains(msg, logId) {
		extras = append(extras, "logId: "+logId)
	}
	if len(extras) > 0 {
		msg = msg + " (" + strings.Join(extras, ", ") + ")"
	}
	return msg
}

// isNoPermissionError reports whether a parsed error body represents a
// permission-denied error, by known server codes or message text. Used to
// surface apply-permission guidance before the framework's generic rendering.
func isNoPermissionError(body map[string]any) bool {
	for _, key := range []string{"code", "errorCode", "server_error_code"} {
		if code, ok := body[key].(string); ok && noPermissionServerCodes[code] {
			return true
		}
	}
	msg := businessErrorMessage(body)
	if msg == "" {
		return false
	}
	lower := strings.ToLower(msg)
	return strings.Contains(msg, "无权限访问") || strings.Contains(msg, "没有访问权限") ||
		strings.Contains(msg, "没有权限") || strings.Contains(msg, "权限不足") ||
		strings.Contains(lower, "no permission") || strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "forbidden.no.auth") ||
		strings.Contains(lower, "forbidden.accessdenied") ||
		strings.Contains(msg, "需要您具备") || strings.Contains(msg, "及以上角色")
}

// confirmDelete is a convenience wrapper around cmdutil.ConfirmDelete that
// checks os.Args for --yes/-y (for callers that don't pass *cobra.Command).
func confirmDelete(resourceType, resourceName string) bool {
	for _, arg := range os.Args[1:] {
		if arg == "--yes" || arg == "-y" {
			return true
		}
	}

	warning := color.New(color.FgRed, color.Bold)
	warning.Printf("About to delete %s: %s\n", resourceType, resourceName)
	fmt.Print("Confirm deletion? (yes/no): ")

	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer == "yes" || answer == "y" {
		return true
	}

	deps.Out.PrintInfo("Operation cancelled")
	return false
}

// confirmDangerousAction confirms a high-impact action whose semantics are
// not deletion. Keeping this separate from confirmDelete prevents enable,
// disable, or publication operations from being described as deletes.
func confirmDangerousAction(cmd *cobra.Command, action, resourceName string) bool {
	if cmd == nil {
		return false
	}
	if yes, err := cmd.Flags().GetBool("yes"); err == nil && yes {
		return true
	}

	output := cmd.ErrOrStderr()
	fmt.Fprintf(output, "About to %s: %s\n", action, resourceName)
	fmt.Fprint(output, "Confirm action? (yes/no): ")

	reader := bufio.NewReader(cmd.InOrStdin())
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer == "yes" || answer == "y" {
		return true
	}

	fmt.Fprintln(output, "Operation cancelled")
	return false
}

// toCamelCase converts a kebab-case string to camelCase.
// Examples: "base-id" -> "baseId", "open-dingtalk-id" -> "openDingtalkId"
func toCamelCase(kebab string) string {
	parts := strings.Split(kebab, "-")
	if len(parts) <= 1 {
		return kebab
	}
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// RegisterCamelCaseAliases recursively walks the command tree and registers
// hidden camelCase aliases for every kebab-case flag. This prevents flag-value
// prefix matching from misinterpreting AI-generated camelCase flags
// (e.g. --baseId) as a short flag + glued value (--base + "Id").
func RegisterCamelCaseAliases(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if !strings.Contains(f.Name, "-") {
			return
		}
		camel := toCamelCase(f.Name)
		if camel == f.Name || cmd.Flags().Lookup(camel) != nil {
			return
		}
		switch f.Value.Type() {
		case "int", "int64":
			cmd.Flags().Int(camel, 0, "")
		case "float64":
			cmd.Flags().Float64(camel, 0, "")
		case "bool":
			cmd.Flags().Bool(camel, false, "")
		case "stringSlice":
			cmd.Flags().StringSlice(camel, nil, "")
		default:
			cmd.Flags().String(camel, "", "")
		}
		_ = cmd.Flags().MarkHidden(camel)
	})
	for _, child := range cmd.Commands() {
		RegisterCamelCaseAliases(child)
	}
}
