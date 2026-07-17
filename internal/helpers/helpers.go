package helpers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// Re-export cmdutil functions as package-level aliases so that existing product
// files continue to compile with their current (unexported) call sites.
// This avoids a mass-rename in 22 product files while still consolidating the
// implementations in pkg/cmdutil.
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
	if err != nil {
		if patErr := reclassifyPATFromError(err); patErr != nil {
			return "", patErr
		}
		return "", WrapError(err)
	}
	for _, c := range result.Content {
		if c.Type == "text" && c.Text != "" {
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
						Code:       CodeMCPToolError,
						Message:    c.Text,
						Suggestion: suggestForBusinessError(errBody),
					}
				}
			}
			return c.Text, nil
		}
	}
	return "", nil
}

// CallMCPToolTextOnServer invokes an MCP tool and returns its raw text response
// WITHOUT printing anything, applying the same error classification as the
// print path. Exported for the shortcut layer's multi-step ("smart") shortcuts,
// which chain several tool calls and need each intermediate result as data.
func CallMCPToolTextOnServer(serverID, toolName string, args map[string]any) (string, error) {
	return callMCPToolReturnTextOnServer(context.Background(), serverID, toolName, args)
}

// callMCPTool 是通用的 MCP 工具调用入口：自动路由 → 调用 → 格式化输出。
// 通过 resolveProductID() 自动确定目标 MCP Server，JSON 输出使用默认的 HTML 转义。
func callMCPTool(toolName string, args map[string]any) error {
	return callMCPToolInternalOpts("", toolName, args, false)
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
	return callMCPToolInternalOpts(serverID, toolName, args, false)
}

// CallMCPToolOnServer is the exported version of callMCPToolOnServer for use
// by extension packages that live in separate Go packages.
func CallMCPToolOnServer(serverID, toolName string, args map[string]any) error {
	return callMCPToolOnServer(serverID, toolName, args)
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
	ctx := context.Background()

	// DryRun 模式：仅预览工具名和参数，不实际调用 MCP Server
	if deps.Caller.DryRun() {
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

	flagFormat := deps.Caller.Format()
	for _, c := range result.Content {
		if c.Type == "text" {
			// 尝试将返回文本解析为 JSON，进行错误分类
			var errBody map[string]any
			if json.Unmarshal([]byte(c.Text), &errBody) == nil {
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
					return &CLIError{Code: CodeMCPToolError, Message: c.Text, Suggestion: suggestForBusinessError(errBody)}
				}
			}

			// JSON 格式输出：解析后使用选定的 printJSON 函数输出
			if flagFormat == "json" {
				var parsed any
				if err := json.Unmarshal([]byte(c.Text), &parsed); err == nil {
					return printJSON(parsed)
				}
			}
			// 特殊处理：开放平台文档搜索结果的表格格式输出
			if toolName == "search_open_platform_docs" && flagFormat == "table" {
				if formatted := formatDevdocSearchTable(c.Text); formatted {
					return nil
				}
			}
			// 默认：原样输出文本内容。
			// 当 unescapeHTML=true 时，c.Text 是一段 JSON 字符串，其中 & 已被服务端
			// 的 JSON 编码器转义为 \u0026。此处先 Unmarshal 还原为 Go 对象，再用
			// PrintJSONUnescaped 输出，保证 & 不被二次转义。
			if unescapeHTML {
				var parsed any
				if err := json.Unmarshal([]byte(c.Text), &parsed); err == nil {
					return printJSON(parsed)
				}
			}
			deps.Out.PrintRaw(c.Text)
			return nil
		}
	}
	// 无 text 类型内容时，将整个 result 对象序列化为 JSON 输出
	return printJSON(result)
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
	for _, key := range []string{"errorCode", "error_code", "code"} {
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
	msg := ""
	if v, ok := body["errorMsg"].(string); ok {
		msg = v
	} else if v, ok := body["message"].(string); ok {
		msg = v
	} else if v, ok := body["error"].(string); ok {
		msg = v
	}
	return suggestForBusinessErrorText(msg)
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
