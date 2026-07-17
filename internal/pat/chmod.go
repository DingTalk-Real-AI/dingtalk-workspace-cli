// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pat

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type LoginRecommendOptions struct {
	// ProductCodes limits the recommend plan to service-owned product domains.
	ProductCodes []string
	// Confirmed grants the server-selected recommend scopes directly. When
	// false, recommend authorization starts the batch confirmation page.
	Confirmed bool
	// ScopeMode controls whether the plan uses the safe recommended set or
	// all scopes under the selected product domains. Empty means recommended.
	ScopeMode LoginRecommendScopeMode
	// ProductSelector lets interactive callers present the service-owned domain
	// list before the final plan. It receives products extracted from the
	// initial recommend plan and must return the product codes to grant.
	ProductSelector func([]LoginRecommendProduct) ([]string, error)
	// InitialPlan lets callers preflight the default recommend plan before any
	// interactive selector and then reuse the same dry-run result.
	InitialPlan *LoginRecommendPlan
}

type LoginRecommendScopeMode string

const (
	LoginRecommendScopeRecommended LoginRecommendScopeMode = "recommend"
	LoginRecommendScopeAll         LoginRecommendScopeMode = "all"
)

type LoginRecommendProduct struct {
	ProductCode        string
	ProductName        string
	Summary            string
	ScopeCount         int
	SelectedScopeCount int
}

type LoginRecommendPlan struct {
	Result     *edition.ToolResult
	Scopes     []string
	Products   []LoginRecommendProduct
	AllGranted bool
}

const (
	sessionIDEnvDingtalk = "DINGTALK_SESSION_ID"
	sessionIDEnvDWS      = "DWS_SESSION_ID"
	sessionIDEnvRewind   = "REWIND_SESSION_ID"
)

// resolveSessionIDFromEnv returns the effective session id from environment
// variables. Resolution order matches the MCP header resolver:
//  1. DINGTALK_SESSION_ID (primary gateway header env).
//  2. DWS_SESSION_ID (stable CLI/session-grant env name).
//  3. REWIND_SESSION_ID (compatibility alias; kept only so hosts that
//     already inject the legacy trace triple keep working without code
//     churn).
//
// When multiple env vars are set to different non-empty values, the first
// one above wins silently. We deliberately do NOT log either raw session id
// value or any derived fingerprint: this resolver is invoked by `dws pat
// chmod` session grants, and any stderr / ~/.dws/logs capture of those
// identifiers can land verbatim in attached troubleshooting bundles. Hosts
// that need to detect a mismatch must do so before invoking the CLI.
func resolveSessionIDFromEnv() string {
	for _, key := range []string{sessionIDEnvDingtalk, sessionIDEnvDWS, sessionIDEnvRewind} {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

// agentCodeEnv is the canonical environment variable name used as a
// per-shell fallback for the --agentCode flag on `dws pat *` commands.
//
// Why: agent hosts may set their business agent code once when spawning
// a long-lived shell / sub-process. Exposing DINGTALK_DWS_AGENTCODE lets
// the host export the code once and let the CLI resolve it on every pat
// subcommand. The flag always wins when both are set so scripted one-offs
// remain deterministic. When neither flag nor env is set, `pat chmod` omits
// agentCode and lets the PAT server apply its open-source default. Batch PAT
// tools receive the resolved agentCode in arguments when present, while the CLI
// also keeps exporting it through env for older gateway paths.
//
// Namespace note: keep this as a single-spelled public contract. The reversed
// draft name DWS_DINGTALK_AGENTCODE and legacy names such as DWS_AGENTCODE /
// DINGTALK_AGENTCODE / REWIND_AGENTCODE are explicitly NOT consumed.
const (
	agentCodeEnv       = authpkg.AgentCodeEnv
	agentCodeEnvCompat = authpkg.AgentCodeEnvCompat
)

// agentCodePattern is the validation regex for any --agentCode value
// resolved from either the flag or the agent-code env var. It matches
// documented agent-code generation schemes (e.g. md5 digests, uuid-like
// ids, short host-assigned slugs) while rejecting shell metacharacters
// and whitespace that would otherwise flow unescaped into an MCP tool
// argument.
var agentCodePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// resolveAgentCodeFromEnv returns the fallback agent code from the canonical
// DINGTALK_DWS_AGENTCODE env var. The second return value reports the env name
// that was consumed (for error attribution); it is "" when the env var is unset
// or blank.
func resolveAgentCodeFromEnv() (string, string) {
	return authpkg.AgentCodeFromEnv()
}

// validateAgentCode rejects agent codes that would be ambiguous or unsafe
// once spliced into a shell / MCP argv. Allowed character set is
// [A-Za-z0-9_-], length 1..64 — see agentCodePattern above.
func validateAgentCode(code string) error {
	if code == "" {
		return fmt.Errorf("--agentCode must not be empty")
	}
	if !agentCodePattern.MatchString(code) {
		return fmt.Errorf(
			"invalid agentCode %q: must match %s (A-Z, a-z, 0-9, _, -; 1..64 chars)",
			code, agentCodePattern.String())
	}
	return nil
}

// resolveAgentCode implements the canonical optional lookup for --agentCode:
//
//  1. explicit --agentCode flag value (highest priority; wins over env)
//  2. DINGTALK_DWS_AGENTCODE env var (per-shell primary fallback)
//  3. empty ("") so PAT-core can apply its open-source default.
//
// Any non-empty resolved value is validated via validateAgentCode, so
// callers never have to re-validate.
func resolveAgentCode(flagVal string) (string, error) {
	code := strings.TrimSpace(flagVal)
	envSource := ""
	if code == "" {
		code, envSource = resolveAgentCodeFromEnv()
	}
	if code == "" {
		return "", nil
	}
	if err := validateAgentCode(code); err != nil {
		if envSource != "" {
			return "", apperrors.NewValidation(
				fmt.Sprintf("%s env: %v", envSource, err),
				apperrors.WithReason("invalid_agent_code"),
			)
		}
		return "", apperrors.NewValidation(
			err.Error(),
			apperrors.WithReason("invalid_agent_code"),
		)
	}
	return code, nil
}

const (
	// patGrantToolName is the English-first wire name for the PAT grant tool.
	patGrantToolName = "pat.grant"

	// patBatchGrantToolName is the English-first wire name for PAT batch grant.
	patBatchGrantToolName = "pat.batch_grant"

	// patBatchPlanToolName is the English-first wire name for PAT batch plan.
	patBatchPlanToolName = "pat.batch_plan"

	// patGrantToolNameLegacyAlias is retained for server builds that still
	// expose only the legacy Chinese display name.
	patGrantToolNameLegacyAlias = "个人授权"

	patBatchUnsupportedCode      = "PAT_BATCH_AUTH_UNSUPPORTED"
	patBatchUnsupportedCodeLower = "pat_batch_auth_unsupported"
	patForgedIdentityCode        = "PAT_FORGED_IDENTITY_FIELD"
	patForgedIdentityCodeLower   = "pat_forged_identity_field"
	grantTypePermanent           = "permanent"
	patCallerAuthLoginRecommend  = "auth_login_recommend"
)

var patBatchMetadataContractCodes = map[string]bool{
	"pat_batch_auth_metadata_required": true,
	"pat_batch_scope_not_declared":     true,
	"pat_batch_product_not_declared":   true,
}

var patBatchIdentityArgumentKeys = map[string]bool{
	"agentCode": true,
	"sessionId": true,
	"orgId":     true,
	"uid":       true,
	"source":    true,
	"caller":    true,
}

var validGrantTypes = map[string]bool{
	"once":      true,
	"session":   true,
	"permanent": true,
}

// newChmodCommand builds a fresh `dws pat chmod` cobra.Command wired to
// the supplied ToolCaller. A factory is used (instead of a package-level
// var) so multiple RegisterCommands invocations never share mutable flag /
// RunE state across concurrent tests.
func newChmodCommand(c edition.ToolCaller) *cobra.Command {
	var recommend bool
	var productFlags []string
	var productsFlag []string
	var domainFlags []string
	var domainsFlag []string

	chmodCmd := &cobra.Command{
		Use:   "chmod <scope>...",
		Short: "授予指定权限",
		Long: `授予指定 scope 的操作权限。

scope 格式: <product>.<entity>:<permission>
  例: aitable.record:query  chat.message:list  calendar.event:get

grantType 规则:
  once       一次性，执行一次后自动失效
  session    当前会话有效，需要 --session-id
  permanent  永久有效（默认）

批量授权:
  dws pat chmod 支持一次传多个 scope 直接批量授予。
  也支持 --products / --product 按产品编码批量展开 scope 模板，
  --domains / --domain 按产品域批量展开 scope 模板，
  --recommend 使用服务端推荐 scope 集合。
  使用产品 / 域 / 推荐集合时，CLI 会先生成 batch plan，确认
  selected / skipped / pending，再对 selected scopes 执行 batch grant；
  --dry-run 只返回授权计划，不写入授权。真正执行批量授权必须显式
  添加 --yes；未加 --yes 时 CLI 会阻断并提示 agent 先确认。

agentCode 配置:
  可通过 --agentCode 或 DINGTALK_DWS_AGENTCODE
  指定；未传 agentCode 时，CLI 会省略该字段并由服务端默认兜底。`,
		Args: func(cmd *cobra.Command, args []string) error {
			productCodes := collectChmodProductCodes(productFlags, productsFlag, domainFlags, domainsFlag)
			if len(args) > 0 || recommend || len(productCodes) > 0 {
				return nil
			}
			return cobra.MinimumNArgs(1)(cmd, args)
		},
		Example: `  dws pat chmod aitable.record:query --grant-type session --session-id session-xxx
  dws pat chmod chat.message:list --grant-type once
  dws pat chmod aitable.record:query aitable.record:create --grant-type permanent --yes
  dws pat chmod --product calendar --product aitable --grant-type once --dry-run --format json
  dws pat chmod --products calendar,aitable --grant-type permanent --yes
  dws pat chmod --domain calendar --domain chat --grant-type once --yes
  dws pat chmod --recommend --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			flagVal, _ := cmd.Flags().GetString("agentCode")
			agentCode, err := resolveAgentCode(flagVal)
			if err != nil {
				return err
			}
			scopes := args
			productCodes := collectChmodProductCodes(productFlags, productsFlag, domainFlags, domainsFlag)
			usesPlan := recommend || len(productCodes) > 0
			grantType, _ := cmd.Flags().GetString("grant-type")
			sessionID, _ := cmd.Flags().GetString("session-id")
			if sessionID == "" {
				sessionID = resolveSessionIDFromEnv()
			}

			if !validGrantTypes[grantType] {
				return fmt.Errorf("invalid --grant-type %q, must be one of: once, session, permanent", grantType)
			}

			if grantType == "session" && sessionID == "" {
				return fmt.Errorf("--session-id is required when --grant-type is session\n  hint: dws pat chmod <scope> --grant-type session --session-id <id>")
			}

			if c != nil && c.DryRun() {
				if usesPlan {
					planArgs := buildBatchPlanArgs(scopes, productCodes, recommend, grantType, agentCode, sessionID, true)
					result, err := callPATBatchPlan(cmd.Context(), c, agentCode, sessionID, planArgs)
					if err != nil {
						return fmt.Errorf("pat chmod plan failed: %w", err)
					}
					if err := ensurePATResultAgentCode(result, agentCode); err != nil {
						return err
					}
					return handleToolResult(cmd, c, result)
				}
				bold := color.New(color.FgYellow, color.Bold)
				bold.Println("[DRY-RUN] Preview only, not executed:")
				fmt.Printf("%-16s%s\n", "Tool:", patBatchGrantToolName)
				if agentCode != "" {
					fmt.Printf("%-16s%s\n", "AgentCode:", agentCode)
				}
				fmt.Printf("%-16s%v\n", "Scope:", scopes)
				fmt.Printf("%-16s%s\n", "GrantType:", grantType)
				if sessionID != "" {
					fmt.Printf("%-16s%s\n", "SessionID:", sessionID)
				}
				return nil
			}

			if c == nil {
				return fmt.Errorf("internal error: tool runtime not initialized")
			}

			if usesPlan {
				planArgs := buildBatchPlanArgs(scopes, productCodes, recommend, grantType, agentCode, sessionID, true)
				planResult, err := callPATBatchPlan(cmd.Context(), c, agentCode, sessionID, planArgs)
				if err != nil {
					return fmt.Errorf("pat chmod plan failed: %w", err)
				}
				if err := ensurePATResultAgentCode(planResult, agentCode); err != nil {
					return err
				}
				scopes, err = extractSelectedScopes(planResult)
				if err != nil {
					return err
				}
				if len(scopes) == 0 {
					return handleToolResult(cmd, c, planResult)
				}
				if err := requireBatchGrantConfirmation(cmd, true, scopes); err != nil {
					return err
				}
			} else if err := requireBatchGrantConfirmation(cmd, false, scopes); err != nil {
				return err
			}
			batchArgs := map[string]any{
				"scopes":    scopes,
				"grantType": grantType,
			}
			toolArgs := map[string]any{
				"scopes":    scopes,
				"grantType": grantType,
			}
			if agentCode != "" {
				batchArgs["agentCode"] = agentCode
				toolArgs["agentCode"] = agentCode
			}
			if sessionID != "" {
				batchArgs["sessionId"] = sessionID
				toolArgs["sessionId"] = sessionID
			}
			// Legacy server schema accepted singular "scope"; clone the
			// canonical argv and rename the key so the two payloads stay
			// in lock-step on every other field.
			legacyToolArgs := make(map[string]any, len(toolArgs))
			for k, v := range toolArgs {
				if k == "scopes" {
					legacyToolArgs["scope"] = v
					continue
				}
				legacyToolArgs[k] = v
			}

			ctx := context.Background()
			result, err := callPATBatchGrantWithLegacyFallback(
				ctx,
				c,
				agentCode,
				sessionID,
				batchArgs,
				toolArgs,
				legacyToolArgs,
			)
			if err != nil {
				return fmt.Errorf("pat chmod failed: %w", err)
			}
			if err := ensurePATResultAgentCode(result, agentCode); err != nil {
				return err
			}

			return handleToolResult(cmd, c, result)
		},
	}

	chmodCmd.Flags().String("agentCode", "",
		"Agent 唯一标识（可选；也可通过 env DINGTALK_DWS_AGENTCODE 注入，flag 优先；未传则由服务端默认兜底）")
	chmodCmd.Flags().String("grant-type", grantTypePermanent, "授权策略: once|session|permanent")
	chmodCmd.Flags().String("session-id", "", "会话标识（session 模式下必填）")
	chmodCmd.Flags().StringArrayVar(&productFlags, "product", nil, "产品编码，可重复；与 --products 等价；执行批量授权需 --yes")
	chmodCmd.Flags().StringSliceVar(&productsFlag, "products", nil, "产品编码列表，逗号分隔；执行批量授权需 --yes")
	chmodCmd.Flags().StringArrayVar(&domainFlags, "domain", nil, "产品域/产品编码，可重复；按产品 scope 模板批量授权；执行授权需 --yes")
	chmodCmd.Flags().StringSliceVar(&domainsFlag, "domains", nil, "产品域/产品编码列表，逗号分隔；执行批量授权需 --yes")
	chmodCmd.Flags().BoolVar(&recommend, "recommend", false, "使用推荐 scope 集合批量授权；执行授权需 --yes")
	cli.AttachRuntimeSchema(chmodCmd, "pat", "batch_grant", "hardcoded:pat")
	cli.AnnotateRuntimeFlagEnum(chmodCmd, "grant-type", "once", "session", "permanent")
	cli.AnnotateRuntimeConstraints(chmodCmd, cli.RuntimeSchemaConstraints{
		RequireOneOf: [][]string{{"scope", "product", "products", "domain", "domains", "recommend"}},
	})
	cli.AnnotateRuntimePositionals(chmodCmd, cli.RuntimeSchemaPositional{
		Name:        "scope",
		Type:        "array",
		Description: "权限 scope，格式为 <product>.<entity>:<permission>；可重复",
		Required:    false,
		Variadic:    true,
		Index:       0,
	})

	return chmodCmd
}

func requireBatchGrantConfirmation(cmd *cobra.Command, usesPlan bool, scopes []string) error {
	if !usesPlan && len(scopes) <= 1 {
		return nil
	}
	if commandBoolFlag(cmd, "yes") {
		return nil
	}
	return apperrors.NewValidation(
		"batch PAT authorization blocked: explicit user confirmation is required; rerun with --yes only after the user approves the batch grant",
		apperrors.WithReason("pat_batch_requires_yes"),
		apperrors.WithHint("先执行 dws pat chmod ... --dry-run --format json 查看 selected/skipped/pending；用户明确确认后再追加 --yes 执行批量授权。"),
		apperrors.WithActions(
			"dws pat chmod <scope1> <scope2> ... --grant-type once --yes",
			"dws pat chmod --products <product1,product2> --grant-type once --yes",
			"dws pat chmod --recommend --grant-type once --yes",
		),
	)
}

func collectChmodProductCodes(groups ...[]string) []string {
	seen := map[string]bool{}
	result := make([]string, 0)
	for _, group := range groups {
		for _, raw := range group {
			for _, part := range strings.Split(raw, ",") {
				code := strings.TrimSpace(part)
				if code == "" || seen[code] {
					continue
				}
				seen[code] = true
				result = append(result, code)
			}
		}
	}
	return result
}

func withPATContextEnv(agentCode, sessionID string, fn func() (*edition.ToolResult, error)) (*edition.ToolResult, error) {
	restore := map[string]*string{}
	setEnv := func(key, value string) {
		if value == "" {
			return
		}
		if _, seen := restore[key]; !seen {
			if old, ok := os.LookupEnv(key); ok {
				oldCopy := old
				restore[key] = &oldCopy
			} else {
				restore[key] = nil
			}
		}
		_ = os.Setenv(key, value)
	}
	setEnv(agentCodeEnv, agentCode)
	setEnv(sessionIDEnvDingtalk, sessionID)
	setEnv(sessionIDEnvDWS, sessionID)
	defer func() {
		for key, old := range restore {
			if old == nil {
				_ = os.Unsetenv(key)
				continue
			}
			_ = os.Setenv(key, *old)
		}
	}()
	return fn()
}

func callPATBatchGrantWithLegacyFallback(
	ctx context.Context,
	c edition.ToolCaller,
	agentCode string,
	sessionID string,
	batchArgs map[string]any,
	canonicalGrantArgs map[string]any,
	legacyGrantArgs map[string]any,
) (*edition.ToolResult, error) {
	if c == nil {
		return nil, fmt.Errorf("internal error: tool runtime not initialized")
	}
	result, err := callPATBatchToolWithIdentityFallback(ctx, c, agentCode, sessionID, patBatchGrantToolName, batchArgs)
	if err == nil && !isPATBatchFallbackResult(result) {
		return result, nil
	}
	if err != nil && !isPATBatchFallbackError(err) && !isToolNotRegisteredError(err) {
		return nil, err
	}
	return withPATContextEnv(agentCode, sessionID, func() (*edition.ToolResult, error) {
		return callPATToolWithLegacyFallback(
			ctx,
			c,
			"pat",
			patGrantToolName,
			patGrantToolNameLegacyAlias,
			canonicalGrantArgs,
			legacyGrantArgs,
		)
	})
}

func callPATBatchPlan(ctx context.Context, c edition.ToolCaller, agentCode, sessionID string, args map[string]any) (*edition.ToolResult, error) {
	if c == nil {
		return nil, fmt.Errorf("internal error: tool runtime not initialized")
	}
	return callPATBatchToolWithIdentityFallback(ctx, c, agentCode, sessionID, patBatchPlanToolName, args)
}

func callPATBatchToolWithIdentityFallback(ctx context.Context, c edition.ToolCaller, agentCode, sessionID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	result, err := withPATContextEnv(agentCode, sessionID, func() (*edition.ToolResult, error) {
		return c.CallTool(ctx, "pat", toolName, args)
	})
	if !shouldRetryPATBatchWithoutIdentityArgs(result, err, args) {
		return result, err
	}
	compatArgs := cloneWithoutPATIdentityArgs(args)
	result, err = withPATContextEnv(agentCode, sessionID, func() (*edition.ToolResult, error) {
		return c.CallTool(ctx, "pat", toolName, compatArgs)
	})
	if err != nil {
		return result, err
	}
	if err := ensurePATIdentityFallbackAgentCode(result, agentCode); err != nil {
		return result, err
	}
	return result, nil
}

func buildBatchPlanArgs(scopes []string, productCodes []string, recommend bool, grantType string, agentCode string, sessionID string, dryRun bool) map[string]any {
	args := map[string]any{
		"scopes":       scopes,
		"productCodes": productCodes,
		"recommend":    recommend,
		"grantType":    grantType,
		"dryRun":       dryRun,
	}
	if agentCode != "" {
		args["agentCode"] = agentCode
	}
	if sessionID != "" {
		args["sessionId"] = sessionID
	}
	return args
}

// RunLoginRecommendAuthorization plans the server-owned recommended scope set
// after a successful dws auth login, then starts a batch Device Code Flow for
// the server-selected grantable scopes. The actual browser open, polling, and
// retry are handled by the shared PAT runner once PAT_BATCH_AUTH_PENDING is
// classified as a PAT auth check.
func RunLoginRecommendAuthorization(ctx context.Context, c edition.ToolCaller, output io.Writer) error {
	return RunLoginRecommendAuthorizationWithOptions(ctx, c, output, LoginRecommendOptions{})
}

func PlanLoginRecommendAuthorization(ctx context.Context, c edition.ToolCaller) (*LoginRecommendPlan, error) {
	if c == nil {
		return nil, fmt.Errorf("internal error: tool runtime not initialized")
	}
	planResult, scopes, allGranted, err := planLoginRecommend(ctx, c, nil, true)
	if err != nil {
		return nil, err
	}
	products, _ := extractLoginRecommendProducts(planResult)
	return &LoginRecommendPlan{
		Result:     planResult,
		Scopes:     scopes,
		Products:   products,
		AllGranted: allGranted,
	}, nil
}

func RunLoginRecommendAuthorizationWithOptions(ctx context.Context, c edition.ToolCaller, output io.Writer, opts LoginRecommendOptions) error {
	if c == nil {
		return fmt.Errorf("internal error: tool runtime not initialized")
	}
	recommendPlan := opts.ScopeMode != LoginRecommendScopeAll
	productCodes := normalizeProductCodes(opts.ProductCodes)
	initialRecommendPlan := recommendPlan
	if opts.ProductSelector != nil && len(productCodes) == 0 {
		// The first plan only discovers the service-owned product-domain list
		// for the TUI. The server contract rejects an empty all-scope request
		// (recommend=false without productCodes), so all-scope mode is applied
		// only after the user selects concrete productCodes.
		initialRecommendPlan = true
	}
	var planResult *edition.ToolResult
	var scopes []string
	var allGranted bool
	if opts.InitialPlan != nil && initialRecommendPlan && len(productCodes) == 0 {
		planResult = opts.InitialPlan.Result
		scopes = append([]string(nil), opts.InitialPlan.Scopes...)
		allGranted = opts.InitialPlan.AllGranted
	} else {
		var err error
		planResult, scopes, allGranted, err = planLoginRecommend(ctx, c, productCodes, initialRecommendPlan)
		if err != nil {
			return err
		}
	}
	if recommendPlan && (allGranted || len(scopes) == 0) {
		if output != nil {
			_, _ = fmt.Fprintln(output, "推荐权限已全部授权或没有可授权项")
		}
		return nil
	}
	if opts.ProductSelector != nil {
		products, err := extractLoginRecommendProducts(planResult)
		if err != nil {
			return err
		}
		if len(products) > 0 {
			selectedProducts, err := opts.ProductSelector(products)
			if err != nil {
				return err
			}
			productCodes = normalizeProductCodes(selectedProducts)
			if len(productCodes) == 0 {
				return fmt.Errorf("至少选择一个授权业务域")
			}
			_, scopes, allGranted, err = planLoginRecommend(ctx, c, productCodes, recommendPlan)
			if err != nil {
				return err
			}
		}
		if len(products) == 0 && !recommendPlan {
			return fmt.Errorf("全部授权需要服务端返回可选授权业务域")
		}
	}
	if allGranted || len(scopes) == 0 {
		if output != nil {
			_, _ = fmt.Fprintln(output, "推荐权限已全部授权或没有可授权项")
		}
		return nil
	}
	grantArgs := map[string]any{
		"scopes":          scopes,
		"grantType":       grantTypePermanent,
		"caller":          patCallerAuthLoginRecommend,
		"clientRequestId": newBatchClientRequestID("auth-login-recommend"),
	}
	if !opts.Confirmed {
		grantArgs["startFlow"] = true
		grantArgs["noWait"] = true
	}
	result, err := callPATBatchToolWithIdentityFallback(ctx, c, "", "", patBatchGrantToolName, grantArgs)
	if err != nil {
		return fmt.Errorf("pat login recommend grant failed: %w", err)
	}
	return handleToolResultForWriter(output, result, c)
}

func planLoginRecommend(ctx context.Context, c edition.ToolCaller, productCodes []string, recommend bool) (*edition.ToolResult, []string, bool, error) {
	productCodes = normalizeProductCodes(productCodes)
	if !recommend && len(productCodes) == 0 {
		return nil, nil, false, fmt.Errorf("全部授权需要至少一个授权业务域")
	}
	planArgs := buildBatchPlanArgs(nil, productCodes, recommend, grantTypePermanent, "", "", true)
	planArgs["caller"] = patCallerAuthLoginRecommend
	planResult, err := callPATBatchPlan(ctx, c, "", "", planArgs)
	if err != nil {
		return nil, nil, false, fmt.Errorf("pat login recommend plan failed: %w", err)
	}
	if err := classifyToolResultText(planResult); err != nil {
		return planResult, nil, false, err
	}
	scopes, err := extractSelectedScopesAllowEmpty(planResult)
	if err != nil {
		return planResult, nil, false, err
	}
	allGranted, _ := extractBatchPlanAllGranted(planResult)
	return planResult, scopes, allGranted, nil
}

func newBatchClientRequestID(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "batch"
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func extractLoginRecommendProducts(result *edition.ToolResult) ([]LoginRecommendProduct, error) {
	text := firstToolResultText(result)
	if text == "" {
		return nil, fmt.Errorf("empty PAT batch plan result")
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		return nil, fmt.Errorf("parsing PAT batch plan result: %w", err)
	}
	data, _ := body["data"].(map[string]any)
	if data == nil {
		return nil, fmt.Errorf("PAT batch plan result missing data.items")
	}
	selectedSet := map[string]bool{}
	if rawSelected, _ := data["selectedScopes"].([]any); len(rawSelected) > 0 {
		for _, raw := range rawSelected {
			if scope, ok := raw.(string); ok && strings.TrimSpace(scope) != "" {
				selectedSet[strings.TrimSpace(scope)] = true
			}
		}
	}

	rawItems, _ := data["items"].([]any)
	type productGroup struct {
		item         LoginRecommendProduct
		summarySeen  map[string]bool
		summaryParts []string
	}
	groups := map[string]*productGroup{}
	order := make([]string, 0)
	for _, raw := range rawItems {
		item, _ := raw.(map[string]any)
		if item == nil {
			continue
		}
		scope := stringField(item, "scope")
		code := stringField(item, "productCode")
		if code == "" {
			code = productCodeFromScope(scope)
		}
		if code == "" {
			continue
		}
		group := groups[code]
		if group == nil {
			group = &productGroup{
				item: LoginRecommendProduct{
					ProductCode: code,
					ProductName: stringField(item, "productName"),
				},
				summarySeen: map[string]bool{},
			}
			if group.item.ProductName == "" {
				group.item.ProductName = code
			}
			groups[code] = group
			order = append(order, code)
		}
		group.item.ScopeCount++
		if selectedSet[scope] {
			group.item.SelectedScopeCount++
		}
		summary := stringField(item, "operationSummary")
		if summary == "" {
			summary = stringField(item, "displayName")
		}
		if summary != "" && !group.summarySeen[summary] && len(group.summaryParts) < 3 {
			group.summarySeen[summary] = true
			group.summaryParts = append(group.summaryParts, summary)
		}
	}

	if len(order) == 0 {
		scopes, _ := extractSelectedScopesAllowEmpty(result)
		for _, scope := range scopes {
			code := productCodeFromScope(scope)
			if groups[code] == nil {
				groups[code] = &productGroup{
					item: LoginRecommendProduct{
						ProductCode:        code,
						ProductName:        code,
						ScopeCount:         0,
						SelectedScopeCount: 0,
					},
				}
				order = append(order, code)
			}
			groups[code].item.ScopeCount++
			groups[code].item.SelectedScopeCount++
		}
	}

	products := make([]LoginRecommendProduct, 0, len(order))
	for _, code := range order {
		group := groups[code]
		group.item.Summary = strings.Join(group.summaryParts, "、")
		products = append(products, group.item)
	}
	return products, nil
}

func normalizeProductCodes(codes []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(codes))
	for _, raw := range codes {
		for _, part := range strings.Split(raw, ",") {
			code := strings.TrimSpace(part)
			if code == "" || seen[code] {
				continue
			}
			seen[code] = true
			out = append(out, code)
		}
	}
	return out
}

func productCodeFromScope(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return ""
	}
	end := len(scope)
	if idx := strings.Index(scope, "."); idx >= 0 && idx < end {
		end = idx
	}
	if idx := strings.Index(scope, ":"); idx >= 0 && idx < end {
		end = idx
	}
	return strings.TrimSpace(scope[:end])
}

func extractBatchPlanAllGranted(result *edition.ToolResult) (bool, error) {
	text := firstToolResultText(result)
	if text == "" {
		return false, fmt.Errorf("empty PAT batch plan result")
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		return false, fmt.Errorf("parsing PAT batch plan result: %w", err)
	}
	data, _ := body["data"].(map[string]any)
	if data == nil {
		return false, fmt.Errorf("PAT batch plan result missing data")
	}
	allGranted, _ := data["allGranted"].(bool)
	return allGranted, nil
}

func extractSelectedScopes(result *edition.ToolResult) ([]string, error) {
	text := firstToolResultText(result)
	if text == "" {
		return nil, fmt.Errorf("empty PAT batch plan result")
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		return nil, fmt.Errorf("parsing PAT batch plan result: %w", err)
	}
	data, _ := body["data"].(map[string]any)
	if data == nil {
		return nil, fmt.Errorf("PAT batch plan result missing data.selectedScopes")
	}
	rawScopes, _ := data["selectedScopes"].([]any)
	if len(rawScopes) == 0 {
		if allGranted, _ := data["allGranted"].(bool); allGranted {
			return []string{}, nil
		}
		return nil, fmt.Errorf("PAT batch plan selectedScopes is empty")
	}
	scopes := make([]string, 0, len(rawScopes))
	for _, raw := range rawScopes {
		scope, ok := raw.(string)
		if ok && strings.TrimSpace(scope) != "" {
			scopes = append(scopes, scope)
		}
	}
	if len(scopes) == 0 {
		if allGranted, _ := data["allGranted"].(bool); allGranted {
			return []string{}, nil
		}
		return nil, fmt.Errorf("PAT batch plan selectedScopes is empty")
	}
	return scopes, nil
}

func extractSelectedScopesAllowEmpty(result *edition.ToolResult) ([]string, error) {
	text := firstToolResultText(result)
	if text == "" {
		return nil, fmt.Errorf("empty PAT batch plan result")
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		return nil, fmt.Errorf("parsing PAT batch plan result: %w", err)
	}
	data, _ := body["data"].(map[string]any)
	if data == nil {
		return nil, fmt.Errorf("PAT batch plan result missing data.selectedScopes")
	}
	rawScopes, _ := data["selectedScopes"].([]any)
	scopes := make([]string, 0, len(rawScopes))
	for _, raw := range rawScopes {
		scope, ok := raw.(string)
		if ok && strings.TrimSpace(scope) != "" {
			scopes = append(scopes, scope)
		}
	}
	return scopes, nil
}

func firstToolResultText(result *edition.ToolResult) string {
	if result == nil {
		return ""
	}
	for _, c := range result.Content {
		if c.Type == "text" && strings.TrimSpace(c.Text) != "" {
			return strings.TrimSpace(c.Text)
		}
	}
	return ""
}

func ensurePATResultAgentCode(result *edition.ToolResult, expectedAgentCode string) error {
	expectedAgentCode = strings.TrimSpace(expectedAgentCode)
	if expectedAgentCode == "" {
		return nil
	}
	actualAgentCode := patResultAgentCode(result)
	if actualAgentCode == "" || actualAgentCode == expectedAgentCode {
		return nil
	}
	return fmt.Errorf(
		"pat chmod returned agentCode %q, want %q from --agentCode/%s; authorization was not applied to the requested agent",
		actualAgentCode,
		expectedAgentCode,
		agentCodeEnv,
	)
}

func ensurePATIdentityFallbackAgentCode(result *edition.ToolResult, expectedAgentCode string) error {
	expectedAgentCode = strings.TrimSpace(expectedAgentCode)
	if expectedAgentCode == "" {
		return nil
	}
	actualAgentCode := patResultAgentCode(result)
	if actualAgentCode == expectedAgentCode {
		return nil
	}
	if actualAgentCode == "" {
		return fmt.Errorf(
			"pat chmod identity fallback did not return agentCode %q; authorization target cannot be verified",
			expectedAgentCode,
		)
	}
	return fmt.Errorf(
		"pat chmod identity fallback returned agentCode %q, want %q from --agentCode/%s; authorization was not applied to the requested agent",
		actualAgentCode,
		expectedAgentCode,
		agentCodeEnv,
	)
}

func patResultAgentCode(result *edition.ToolResult) string {
	text := firstToolResultText(result)
	if text == "" {
		return ""
	}
	var body map[string]any
	if json.Unmarshal([]byte(text), &body) != nil {
		return ""
	}
	if code := stringField(body, "agentCode"); code != "" {
		return code
	}
	data, _ := body["data"].(map[string]any)
	if data != nil {
		if code := stringField(data, "agentCode"); code != "" {
			return code
		}
	}
	resultBody, _ := body["result"].(map[string]any)
	if resultBody != nil {
		if code := stringField(resultBody, "agentCode"); code != "" {
			return code
		}
		resultData, _ := resultBody["data"].(map[string]any)
		if resultData != nil {
			return stringField(resultData, "agentCode")
		}
	}
	return ""
}

func isPATBatchUnsupportedResult(result *edition.ToolResult) bool {
	return patBatchResultHasCode(result, func(code string) bool {
		return strings.EqualFold(code, patBatchUnsupportedCode)
	})
}

func isPATBatchFallbackResult(result *edition.ToolResult) bool {
	return patBatchResultHasCode(result, func(code string) bool {
		normalized := strings.ToLower(strings.TrimSpace(code))
		return strings.EqualFold(code, patBatchUnsupportedCode) || patBatchMetadataContractCodes[normalized]
	})
}

func isPATForgedIdentityResult(result *edition.ToolResult) bool {
	return patBatchResultHasCode(result, func(code string) bool {
		return strings.EqualFold(code, patForgedIdentityCode)
	})
}

func patBatchResultHasCode(result *edition.ToolResult, matches func(string) bool) bool {
	text := firstToolResultText(result)
	if text == "" {
		return false
	}
	var body map[string]any
	if json.Unmarshal([]byte(text), &body) != nil {
		return false
	}
	for _, key := range []string{"code", "errorCode", "error_code"} {
		if code, ok := body[key].(string); ok && matches(strings.TrimSpace(code)) {
			return true
		}
	}
	return false
}

func isPATBatchUnsupportedError(err error) bool {
	return err != nil && strings.Contains(normalizedPATErrorText(err), patBatchUnsupportedCodeLower)
}

func isPATBatchFallbackError(err error) bool {
	if isPATBatchUnsupportedError(err) {
		return true
	}
	text := normalizedPATErrorText(err)
	for code := range patBatchMetadataContractCodes {
		if strings.Contains(text, code) {
			return true
		}
	}
	return false
}

func isPATForgedIdentityError(err error) bool {
	return err != nil && strings.Contains(normalizedPATErrorText(err), patForgedIdentityCodeLower)
}

func shouldRetryPATBatchWithoutIdentityArgs(result *edition.ToolResult, err error, args map[string]any) bool {
	if !hasPATIdentityArgs(args) {
		return false
	}
	if err != nil {
		return isPATForgedIdentityError(err)
	}
	return isPATForgedIdentityResult(result)
}

func hasPATIdentityArgs(args map[string]any) bool {
	for key := range args {
		if patBatchIdentityArgumentKeys[key] {
			return true
		}
	}
	return false
}

func cloneWithoutPATIdentityArgs(args map[string]any) map[string]any {
	out := make(map[string]any, len(args))
	for key, value := range args {
		if patBatchIdentityArgumentKeys[key] {
			continue
		}
		out[key] = value
	}
	return out
}

// callPATToolWithLegacyFallback invokes the canonical PAT grant tool first,
// then silently retries the legacy Chinese alias when the server has not
// registered the canonical tool yet. The retry intentionally emits no stderr
// banner because host-owned PAT callers parse stderr as machine JSON.
func callPATToolWithLegacyFallback(ctx context.Context, c edition.ToolCaller, productID, toolName, legacyAlias string, toolArgs, legacyArgs map[string]any) (*edition.ToolResult, error) {
	if c == nil {
		return nil, fmt.Errorf("internal error: tool runtime not initialized")
	}
	result, err := c.CallTool(ctx, productID, toolName, toolArgs)
	if err == nil {
		return result, nil
	}
	if legacyAlias == "" {
		return nil, err
	}
	if !isToolNotRegisteredError(err) && !isLegacyGrantSchemaMismatchError(err, toolArgs, legacyArgs) {
		return nil, err
	}
	return c.CallTool(ctx, productID, legacyAlias, legacyArgs)
}

func isEmptyToolResult(result *edition.ToolResult) bool {
	if result == nil || len(result.Content) == 0 {
		return true
	}
	for _, block := range result.Content {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			return false
		}
	}
	return true
}

// isToolNotRegisteredError reports whether err looks like a server-side
// tool-not-registered / tool-not-found classification. We match on a few
// conservative substrings rather than a structured error type because the
// upstream runner surfaces the server message as plain text.
func isToolNotRegisteredError(err error) bool {
	if err == nil {
		return false
	}
	msg := normalizedPATErrorText(err)
	needles := []string{
		"tool_not_found",
		"mcp_tool_not_found",
		"tool not found",
		"tool not registered",
		"tool not exist",
		"tool does not exist",
		"unknown tool",
		"no such tool",
		"未找到指定工具",
		"未找到工具",
		"工具不存在",
	}
	for _, needle := range needles {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func isLegacyGrantSchemaMismatchError(err error, toolArgs, legacyArgs map[string]any) bool {
	if err == nil || !hasScopeKeyShapeMismatch(toolArgs, legacyArgs) {
		return false
	}
	if apperrors.IsPATError(err) {
		return false
	}
	msg := normalizedPATErrorText(err)
	if containsAny(msg,
		"pat_no_permission",
		"pat_low_risk_no_permission",
		"pat_medium_risk_no_permission",
		"pat_high_risk_no_permission",
		"pat_scope_auth_required",
		"agent_code_not_exists",
		"requiredscopes",
		"missingscope",
		"missing_scope",
		"insufficient_scope",
	) {
		return false
	}
	if !containsAny(msg, "scope", "scopes") {
		return false
	}
	if !containsAny(msg,
		"param_error",
		"参数错误",
		"parameter",
		"validation",
		"required",
		"missing",
		"unknown",
		"unexpected",
		"invalid",
		"unmarshal",
	) {
		return false
	}
	if containsAny(msg,
		"permission denied",
		"no permission",
		"forbidden",
		"unauthorized",
		"auth required",
		"无权限",
		"未授权",
		"pat_medium_risk_no_permission",
	) {
		return false
	}
	return true
}

func hasScopeKeyShapeMismatch(toolArgs, legacyArgs map[string]any) bool {
	if toolArgs == nil || legacyArgs == nil {
		return false
	}
	_, hasCanonicalPlural := toolArgs["scopes"]
	_, hasCanonicalSingular := toolArgs["scope"]
	_, hasLegacyPlural := legacyArgs["scopes"]
	_, hasLegacySingular := legacyArgs["scope"]
	return hasCanonicalPlural && !hasCanonicalSingular && hasLegacySingular && !hasLegacyPlural
}

func normalizedPATErrorText(err error) string {
	if err == nil {
		return ""
	}
	parts := []string{strings.ToLower(err.Error())}
	var typed *apperrors.Error
	if stderrors.As(err, &typed) && typed != nil {
		parts = append(parts,
			strings.ToLower(typed.Reason),
			strings.ToLower(typed.ServerDiag.ServerErrorCode),
			strings.ToLower(typed.ServerDiag.TechnicalDetail),
			strings.ToLower(typed.Hint),
		)
	}
	return strings.Join(parts, " ")
}

func containsAny(msg string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// handleToolResult processes a ToolResult and writes output to stdout.
func handleToolResult(cmd *cobra.Command, caller edition.ToolCaller, result *edition.ToolResult) error {
	if result == nil {
		return fmt.Errorf("empty tool result")
	}
	for _, c := range result.Content {
		if c.Type != "text" || c.Text == "" {
			continue
		}
		if respErr := apperrors.ClassifyMCPResponseText(c.Text); respErr != nil {
			return respErr
		}
		if !shouldEmitRawPATResult(cmd) {
			if summary := formatPATAuthorizationSummary(c.Text, caller); summary != "" {
				fmt.Print(summary)
				return nil
			}
		}
		fmt.Println(c.Text)
		return nil
	}
	data, _ := json.Marshal(result)
	return fmt.Errorf("empty PAT authorization result: %s", string(data))
}

func handleToolResultForWriter(w io.Writer, result *edition.ToolResult, caller edition.ToolCaller) error {
	if result == nil {
		return fmt.Errorf("empty tool result")
	}
	for _, c := range result.Content {
		if c.Type != "text" || c.Text == "" {
			continue
		}
		if respErr := apperrors.ClassifyMCPResponseText(c.Text); respErr != nil {
			return respErr
		}
		if w == nil {
			return nil
		}
		if summary := formatPATAuthorizationSummary(c.Text, caller); summary != "" {
			_, _ = fmt.Fprint(w, summary)
			return nil
		}
		_, _ = fmt.Fprintln(w, c.Text)
		return nil
	}
	data, _ := json.Marshal(result)
	return fmt.Errorf("empty PAT authorization result: %s", string(data))
}

func classifyToolResultText(result *edition.ToolResult) error {
	if result == nil {
		return fmt.Errorf("empty tool result")
	}
	for _, c := range result.Content {
		if c.Type != "text" || c.Text == "" {
			continue
		}
		return apperrors.ClassifyMCPResponseText(c.Text)
	}
	return nil
}

func shouldEmitRawPATResult(cmd *cobra.Command) bool {
	if commandBoolFlag(cmd, "verbose") {
		return true
	}
	if !commandFlagChanged(cmd, "format") {
		return false
	}
	format := strings.ToLower(strings.TrimSpace(commandStringFlag(cmd, "format")))
	return format == "json" || format == "raw"
}

func commandBoolFlag(cmd *cobra.Command, name string) bool {
	if cmd == nil {
		return false
	}
	if flag := cmd.Flags().Lookup(name); flag != nil {
		value, err := cmd.Flags().GetBool(name)
		return err == nil && value
	}
	root := cmd.Root()
	value, err := root.PersistentFlags().GetBool(name)
	return err == nil && value
}

func commandFlagChanged(cmd *cobra.Command, name string) bool {
	if cmd == nil {
		return false
	}
	if flag := cmd.Flags().Lookup(name); flag != nil && flag.Changed {
		return true
	}
	root := cmd.Root()
	flag := root.PersistentFlags().Lookup(name)
	return flag != nil && flag.Changed
}

func commandStringFlag(cmd *cobra.Command, name string) string {
	if cmd == nil {
		return ""
	}
	if flag := cmd.Flags().Lookup(name); flag != nil {
		value, _ := cmd.Flags().GetString(name)
		return value
	}
	root := cmd.Root()
	value, _ := root.PersistentFlags().GetString(name)
	return value
}

func formatPATAuthorizationSummary(text string, caller edition.ToolCaller) string {
	var body map[string]any
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		return ""
	}
	data, _ := body["data"].(map[string]any)
	if data == nil {
		return ""
	}

	lines := []string{"PAT authorization"}
	if code := stringField(body, "code"); code != "" {
		lines = append(lines, "status: "+code)
	} else if success, ok := body["success"].(bool); ok {
		lines = append(lines, fmt.Sprintf("success: %v", success))
	}
	if agentCode := stringField(data, "agentCode"); agentCode != "" {
		lines = append(lines, "agentCode: "+agentCode)
	}
	if grantType := stringField(data, "grantType"); grantType != "" {
		lines = append(lines, "grantType: "+grantType)
	}
	if allGranted, ok := data["allGranted"].(bool); ok {
		lines = append(lines, fmt.Sprintf("allGranted: %v", allGranted))
	}

	appendCountLine := func(label, key string) {
		if count, ok := countField(data, key); ok {
			lines = append(lines, fmt.Sprintf("%s: %d", label, count))
		}
	}
	appendCountLine("items", "items")
	appendCountLine("selected", "selectedScopes")
	appendCountLine("granted", "grantedScopes")
	appendCountLine("alreadyGranted", "alreadyGrantedScopes")
	appendCountLine("skipped", "skippedScopes")
	appendCountLine("pending", "pendingScopes")

	lines = append(lines, "suggestion: "+patAuthorizationSuggestion(data, caller))
	lines = append(lines, "hint: use --format json or --verbose for full scope details")
	return strings.Join(lines, "\n") + "\n"
}

func patAuthorizationSuggestion(data map[string]any, caller edition.ToolCaller) string {
	if allGranted, ok := data["allGranted"].(bool); ok && allGranted {
		return "no action needed"
	}
	if count, ok := countField(data, "selectedScopes"); ok && count > 0 {
		if caller != nil && caller.DryRun() {
			return "rerun this command without --dry-run to grant selected scopes"
		}
		return "selected scopes are ready to grant"
	}
	if count, ok := countField(data, "pendingScopes"); ok && count > 0 {
		return "complete authorization, then retry the command"
	}
	return "check auth status or rerun with --format json for details"
}

func countField(data map[string]any, key string) (int, bool) {
	raw, ok := data[key]
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case []any:
		return len(v), true
	case []string:
		return len(v), true
	}
	return 0, false
}

func stringField(data map[string]any, key string) string {
	if value, ok := data[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}
