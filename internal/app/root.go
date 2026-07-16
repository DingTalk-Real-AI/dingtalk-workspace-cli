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

package app

import (
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/logging"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline/handlers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/plugin"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/recovery"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/usage"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type outputFileContextKey struct{}

const recoveryEventStderrPrefix = "RECOVERY_EVENT_ID="

var (
	rootNormalizeProcessProfileArgs = normalizeProcessProfileArgs
	rootExecuteCommand              = (*cobra.Command).ExecuteC
	rootNewRootCommandWithEngine    = NewRootCommandWithEngine
	rootRunPreParse                 = pipeline.RunPreParse
	rootLatestRecoveryCapture       = recovery.LatestCapture
	rootResetRecoveryState          = recovery.ResetRuntimeState
	rootStopAllStdioClients         = StopAllStdioClients
	rootLoadPlugins                 = loadPlugins
	rootMkdirAll                    = os.MkdirAll
	rootCreateFile                  = os.Create
	rootCloseFile                   = (*os.File).Close
	rootPluginInjectConfigEnv       = (*plugin.Loader).InjectPluginConfigEnv
	rootPluginLoadUser              = (*plugin.Loader).LoadUser
	rootPluginLoadDev               = (*plugin.Loader).LoadDev
	rootPluginDescriptors           = (*plugin.Plugin).ToServerDescriptors
	rootPluginStdioClients          = (*plugin.Plugin).StdioClients
	rootRegisterPluginHTTPServer    = registerPluginHTTPServer
	rootRegisterStdioManifest       = registerStdioServerFromManifest
	rootPluginLoadHooks             = (*plugin.Plugin).LoadHooks
	rootPluginSyncSkills            = plugin.SyncSkills
	rootAuthLoadTokenData           = authpkg.LoadTokenData
)

// Execute runs the root command and returns the process exit code.
func Execute() (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "Error: internal panic: %v\n", r)
			exitCode = 5
		}
	}()

	restoreArgs := rootNormalizeProcessProfileArgs()
	defer restoreArgs()

	timing := NewTimingCollector()
	defer func() {
		rootStopAllStdioClients() // Ensure child processes are terminated on exit
		CloseAuditSink()          // Drain async audit forwards on all exit paths,
		// including command errors where Cobra skips PersistentPostRunE.
		timing.PrintIfEnabled()
		timing.WriteReportIfEnabled(RawVersion(), SanitizeCommand(os.Args))
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Attach timing collector to context for use by child components
	ctx = WithTimingCollector(ctx, timing)

	initStart := time.Now()
	rootResetRecoveryState()
	engine := newPipelineEngine()
	root := rootNewRootCommandWithEngine(ctx, engine)
	timing.Record("cmd_init", time.Since(initStart))

	// Run PreParse handlers on raw argv before Cobra parses flags.
	// This corrects model-generated errors like --userId → --user-id
	// and --limit100 → --limit 100.
	rootRunPreParse(root, engine)

	executed, err := rootExecuteCommand(root)
	if err != nil {
		if executed == nil {
			executed = root
		}
		err = rewordRequiredFlagError(err)
		if isUnknownCommandError(err) {
			executed.SetOut(os.Stderr)
			_ = executed.Help()
			_, _ = fmt.Fprintln(os.Stderr)
		}
		_ = printExecutionError(executed, os.Stdout, os.Stderr, err)
		if last := rootLatestRecoveryCapture(); last != nil && last.EventID != "" {
			_, _ = fmt.Fprintf(os.Stderr, "%s%s\n", recoveryEventStderrPrefix, last.EventID)
		}
		return apperrors.ExitCode(err)
	}
	return 0
}

func isUnknownCommandError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "unknown command")
}

// rewordRequiredFlagError rewrites cobra's default missing-required-flag message
// (`required flag(s) "email" not set`) into the wukong-aligned form
// (`missing required flag(s): --email`). cobra's ValidateRequiredFlags returns
// this error directly (it does not pass through FlagErrorFunc), so it is
// normalised here. The substring "required flag" is preserved for compatibility
// with existing assertions; flag names gain the "--" prefix and quotes are
// dropped so error output matches hardcoded cmdutil.ValidateRequiredFlags.
func rewordRequiredFlagError(err error) error {
	if err == nil {
		return err
	}
	const pfx = "required flag(s) "
	const sfx = " not set"
	msg := err.Error()
	if !strings.HasPrefix(msg, pfx) || !strings.HasSuffix(msg, sfx) {
		return err
	}
	mid := strings.TrimSuffix(strings.TrimPrefix(msg, pfx), sfx)
	var flags []string
	for _, part := range strings.Split(mid, ", ") {
		if name := strings.Trim(strings.TrimSpace(part), "\""); name != "" {
			flags = append(flags, "--"+name)
		}
	}
	if len(flags) == 0 {
		return err
	}
	return apperrors.NewValidation(fmt.Sprintf("missing required flag(s): %s", strings.Join(flags, ", ")))
}

// flagErrorWithSuggestions provides helpful suggestions for common flag mistakes.
//
// 所有 flag 解析错误都会在 message 末尾追加 "See '<CommandPath> --help' for usage."，
// 与 docker / kubectl / gh / wukong CLI 的 UX 一致，方便用户/agent 复制完整命令查 help。
// 装在 root 的 FlagErrorFunc 通过 cobra 的 parent fallback 机制覆盖全命令树
// （cobra.Command.FlagErrorFunc 沿 c.parent 递归向上查找）。
func flagErrorWithSuggestions(cmd *cobra.Command, err error) error {
	errMsg := err.Error()
	// 尾部 hint：换行 + See '...' for usage.
	// JSON 输出时 \n 会被序列化为字面 \n，文本输出时换行；
	// 无论哪种格式，子串 "--help' for usage." 都可被检索到。
	tail := fmt.Sprintf("\nSee '%s --help' for usage.", cmd.CommandPath())
	msgWithTail := errMsg + tail

	// Common flag aliases and suggestions
	suggestions := map[string]string{
		"--json":        "提示: 请使用 --format json 或 -f json 来输出 JSON 格式",
		"--method":      "提示: dws auth login 默认使用 OAuth loopback 流；SSH/无头环境请加 --device 走设备流",
		"--device-flow": "提示: 设备流的标志名是 --device（不是 --device-flow），SSH/无头环境登录请用 dws auth login --device",
		"--email":       "提示: dws 不支持邮箱/密码登录，请使用 dws auth login 进行扫码登录",
		"--code":        "提示: dws 不支持验证码登录，请使用 dws auth login 进行扫码登录",
		"--corp-id":     "提示: corp-id 会在登录时自动获取，无需手动指定",
		"--password":    "提示: dws 不支持密码登录，请使用 dws auth login 进行扫码登录",
		"--phone":       "提示: dws 不支持手机号登录，请使用 dws auth login 进行扫码登录",
		"--app-key":     "提示: 请使用环境变量 DWS_CLIENT_ID 或 --client-id 设置 AppKey",
		"--app-secret":  "提示: 请使用环境变量 DWS_CLIENT_SECRET 或 --client-secret 设置 AppSecret",
	}

	for flag, suggestion := range suggestions {
		if strings.Contains(errMsg, "unknown flag: "+flag) {
			return apperrors.NewValidation(
				msgWithTail,
				apperrors.WithHint(suggestion),
				apperrors.WithReason("unknown_flag"),
				apperrors.WithCause(err),
				apperrors.WithActions(fmt.Sprintf("Run '%s --help' for valid flags", cmd.CommandPath())),
				apperrors.WithAvailableFlags(cmdutil.VisibleFlagNames(cmd)...),
			)
		}
	}

	if strings.Contains(errMsg, "unknown flag:") {
		fix := cmdutil.SuggestFlagFix(cmd, err)
		if fix.Suggestion != "" {
			return apperrors.NewValidation(
				msgWithTail,
				apperrors.WithHint(fix.Suggestion),
				apperrors.WithReason("unknown_flag"),
				apperrors.WithCause(err),
				apperrors.WithActions(fmt.Sprintf("Run '%s --help' for valid flags", cmd.CommandPath())),
				apperrors.WithAvailableFlags(cmdutil.VisibleFlagNames(cmd)...),
			)
		}
	}

	// Fallback：未命中已知别名 / SuggestFlagFix 未给建议的 flag 解析错误
	// （missing required / ambiguous / unknown shorthand 等），仍包尾部 hint，
	// 行为对齐 wukong / docker / kubectl。
	return fmt.Errorf("%s%s", errMsg, tail)
}

func printExecutionError(root *cobra.Command, stdout, stderr io.Writer, err error) error {
	var raw apperrors.RawStderrError
	if stderrors.As(err, &raw) {
		_, writeErr := fmt.Fprintln(stderr, raw.RawStderr())
		return writeErr
	}
	if wantsJSONErrors(root) {
		return apperrors.PrintJSON(stderr, err)
	}
	return apperrors.PrintHumanAt(stderr, err, resolveVerbosity(root))
}

// resolveVerbosity derives the error verbosity level from the root command's flags.
func resolveVerbosity(cmd *cobra.Command) apperrors.Verbosity {
	if cmd == nil {
		return apperrors.VerbosityNormal
	}
	if debug, err := cmd.Flags().GetBool("debug"); err == nil && debug {
		return apperrors.VerbosityDebug
	}
	if verbose, err := cmd.Flags().GetBool("verbose"); err == nil && verbose {
		return apperrors.VerbosityVerbose
	}
	return apperrors.VerbosityNormal
}

func wantsJSONErrors(root *cobra.Command) bool {
	if root == nil {
		return false
	}
	if commandRequestsJSONErrors(root) {
		return true
	}
	if rootCmd := root.Root(); rootCmd != nil && rootCmd != root {
		return commandRequestsJSONErrors(rootCmd)
	}
	return false
}

func commandRequestsJSONErrors(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	for _, flags := range []interface {
		Lookup(string) *pflag.Flag
		GetString(string) (string, error)
		GetBool(string) (bool, error)
	}{
		cmd.Flags(),
		cmd.InheritedFlags(),
		cmd.PersistentFlags(),
	} {
		if flag := flags.Lookup("format"); flag != nil {
			if value, err := flags.GetString("format"); err == nil && strings.EqualFold(strings.TrimSpace(value), "json") {
				return true
			}
		}
		if flag := flags.Lookup("json"); flag != nil && flag.Changed {
			if value, err := flags.GetBool("json"); err == nil {
				if value {
					return true
				}
				continue
			}
			return true
		}
	}
	return false
}

// NewRootCommand constructs the root CLI command. The provided context
// is propagated to background goroutines and the Cobra command tree so
// that SIGINT/SIGTERM can cancel in-flight work.
func NewRootCommand(ctx ...context.Context) *cobra.Command {
	var rootCtx context.Context
	if len(ctx) > 0 && ctx[0] != nil {
		rootCtx = ctx[0]
	}
	return NewRootCommandWithEngine(rootCtx, nil)
}

// NewRootCommandWithEngine constructs the root CLI command with an
// optional pipeline engine for input correction. When engine is nil,
// no pipeline processing is applied.
func NewRootCommandWithEngine(rootCtx context.Context, engine *pipeline.Engine) *cobra.Command {
	if rootCtx == nil {
		rootCtx = context.Background()
	}
	flags := &GlobalFlags{}
	authpkg.SetRuntimeProfile(preparseProfileFlag(os.Args[1:]))
	loader := cli.EnvironmentLoader{
		LookupEnv: os.LookupEnv,
	}
	runner := newCommandRunnerWithFlags(loader, flags)

	root := &cobra.Command{
		Use:               "dws",
		Short:             "DWS CLI",
		Long:              `提示: 如果遇到能力缺失、命令报错、新功能未注册、或无法完成任务, 请先用 'dws upgrade' 升级到最新版本后再试. 钉钉 OpenAPI 和 dws CLI 持续迭代, 新能力和 bugfix 会先在新版本上线.`,
		Args:              cobra.NoArgs,
		SilenceErrors:     true,
		SilenceUsage:      true,
		DisableAutoGenTag: true,
		Version:           Version(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			authpkg.SetRuntimeProfile(flags.Profile)
			// Apply OAuth credential overrides from CLI flags (highest priority).
			if flags.ClientID != "" {
				authpkg.SetClientID(flags.ClientID)
			}
			if flags.ClientSecret != "" {
				authpkg.SetClientSecret(flags.ClientSecret)
			}

			// Configure global slog level based on --debug / --verbose flags.
			configureLogLevel(flags)

			if err := configureOutputSink(cmd); err != nil {
				return err
			}
			if fn := edition.Get().AfterPersistentPreRun; fn != nil {
				return fn(cmd, args)
			}
			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			StopAllStdioClients()
			CloseAuditSink()
			CloseFileLogger()
			return closeOutputSink(cmd)
		},
	}

	bindPersistentFlags(root, flags)

	schemaCmd := newSchemaCommand(loader)
	mcpCmd := newMCPCommand(rootCtx, loader, runner, engine)
	mcpCmd.Hidden = true
	// Wrap the caller so every MCP tool call's shape is recorded to the local
	// usage log (privacy-preserving; see internal/shortcut/usage). Powers
	// `dws shortcut stats` and future high-frequency shortcut distillation.
	patCaller := newRecordingToolCaller(newToolCallerAdapter(runner, flags))

	utilityCommands := []*cobra.Command{
		newAuthCommand(patCaller),
		newProfileCommand(),
		newAPICommand(flags),
		newSkillCommand(),
		newCacheCommand(),
		newCatalogCommand(loader),
		newConfigCommand(),
		newDoctorCommand(),
		newEventCommand(),
		newAuditCommand(),
		newCompletionCommand(root),
		newRecoveryCommand(rootCtx, loader, flags),
		newUpgradeCommand(),
		newVersionCommand(),
		newPluginCommand(),
		usage.NewShortcutCommand(),
		schemaCmd,
		mcpCmd,
	}
	root.AddCommand(utilityCommands...)

	root.AddCommand(newLegacyPublicCommands(runner, patCaller)...)
	root.AddCommand(newLegacyHiddenCommands(runner)...)

	// --- Plugin loading: runs AFTER legacy commands so plugin endpoints can
	// be appended on top of the static endpoint registry.
	pluginCmds := rootLoadPlugins(engine, runner)
	if len(pluginCmds) > 0 {
		addPluginCommandsSafe(root, pluginCmds)
	}

	// PAT authorization commands (open-source core)
	pat.RegisterCommands(root, patCaller)

	if fn := edition.Get().RegisterExtraCommands; fn != nil {
		caller := newToolCallerAdapter(runner, flags)
		fn(root, caller)
		deduplicateCommands(root)
	}
	hideNonDirectRuntimeCommands(root)
	configureRootHelp(root)
	// Set custom flag error handler for better UX
	root.SetFlagErrorFunc(flagErrorWithSuggestions)
	root.SetContext(rootCtx)

	return root
}

func preparseProfileFlag(args []string) string {
	args, _ = normalizeProfileFlagArgs(args)
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "--profile" && i+1 < len(args):
			return strings.TrimSpace(args[i+1])
		case strings.HasPrefix(arg, "--profile="):
			return strings.TrimSpace(strings.TrimPrefix(arg, "--profile="))
		}
	}
	return ""
}

func normalizeProcessProfileArgs() func() {
	original := append([]string(nil), os.Args...)
	if len(os.Args) > 1 {
		if normalized, changed := normalizeProfileFlagArgs(os.Args[1:]); changed {
			os.Args = append([]string{os.Args[0]}, normalized...)
		}
	}
	return func() {
		os.Args = original
	}
}

func normalizeProfileFlagArgs(args []string) ([]string, bool) {
	if len(args) == 0 {
		return args, false
	}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		trimmed := strings.TrimSpace(arg)
		switch {
		case trimmed == "--profile":
			out = append(out, arg)
			if i+1 >= len(args) {
				continue
			}
			value, next := collectProfileFlagValue(args[i+1], args, i+2)
			out = append(out, value)
			i = next - 1
		case strings.HasPrefix(trimmed, "--profile="):
			value, next := collectProfileFlagValue(strings.TrimPrefix(trimmed, "--profile="), args, i+1)
			out = append(out, "--profile="+value)
			i = next - 1
		default:
			out = append(out, arg)
		}
	}
	return out, argsChanged(args, out)
}

func collectProfileFlagValue(first string, args []string, next int) (string, int) {
	parts := []string{strings.TrimSpace(first)}
	for len(parts) > 0 && strings.HasSuffix(strings.TrimSpace(parts[len(parts)-1]), ",") && next < len(args) {
		candidate := strings.TrimSpace(args[next])
		if candidate == "" || strings.HasPrefix(candidate, "-") {
			break
		}
		parts = append(parts, candidate)
		next++
	}
	return strings.Join(parts, ""), next
}

func argsChanged(before, after []string) bool {
	if len(before) != len(after) {
		return true
	}
	for i := range before {
		if before[i] != after[i] {
			return true
		}
	}
	return false
}

func newAuthCommand(patCaller edition.ToolCaller) *cobra.Command {
	return buildAuthCommand(patCaller)
}

func newSkillCommand() *cobra.Command {
	return buildSkillCommand()
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "version",
		Short:             "显示版本信息",
		Example:           "  dws version\n  dws version --format json",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			wantJSON := cmd.Flags().Changed("format")
			if wantJSON {
				format, _ := cmd.Flags().GetString("format")
				wantJSON = (format == "json")
			}

			editionName := edition.Get().Name
			if editionName == "" {
				editionName = "open"
			}
			ver := RawVersion()
			bt := BuildTime()
			gc := GitCommit()
			goVer := "1.24+"

			arch := "MCP Static Endpoint Mode"

			if wantJSON {
				payload := map[string]any{
					"version":      ver,
					"edition":      editionName,
					"architecture": arch,
					"go":           goVer,
				}
				if bt != "unknown" {
					payload["build"] = bt
				}
				if gc != "unknown" {
					payload["commit"] = gc
				}
				return output.WriteJSON(cmd.OutOrStdout(), payload)
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%-16s%s\n", "Version:", ver)
			fmt.Fprintf(w, "%-16s%s\n", "Edition:", editionName)
			if bt != "unknown" {
				fmt.Fprintf(w, "%-16s%s\n", "Build:", bt)
			}
			if gc != "unknown" {
				fmt.Fprintf(w, "%-16s%s\n", "Commit:", gc)
			}
			fmt.Fprintf(w, "%-16s%s\n", "Architecture:", arch)
			fmt.Fprintf(w, "%-16s%s\n", "Go:", goVer)
			return nil
		},
	}
}

func newSchemaCommand(loader cli.CatalogLoader) *cobra.Command {
	return cli.NewSchemaCommand(loader)
}

// buildMCPCommandFn is a test seam for newMCPCommand.
var buildMCPCommandFn = cli.NewMCPCommand

// newMCPCommand builds the `dws mcp` command tree.
func newMCPCommand(ctx context.Context, loader cli.CatalogLoader, runner executor.Runner, engine *pipeline.Engine) *cobra.Command {
	return buildMCPCommandFn(ctx, loader, runner, engine)
}

// hideNonDirectRuntimeCommands marks top-level product commands as hidden
// unless they correspond to a static endpoint product or an edition-visible
// compatibility command.
// Public utility commands are always kept visible; explicitly hidden commands
// stay hidden.
func hideNonDirectRuntimeCommands(root *cobra.Command) {
	allowedProducts := resolveVisibleProducts()
	staticCommands := map[string]bool{
		"auth":       true,
		"api":        true,
		"audit":      true,
		"cache":      true,
		"config":     true,
		"dev":        true,
		"doctor":     true,
		"event":      true,
		"completion": true,
		"skill":      true,
		"plugin":     true,
		"profile":    true,
		"version":    true,
		"help":       true,
		"recovery":   true,
		"schema":     true,
		"mcp":        true,
		"upgrade":    true,
	}
	for _, cmd := range root.Commands() {
		name := cmd.Name()
		if cmd.Hidden {
			continue
		}
		if staticCommands[name] {
			continue
		}
		if allowedProducts[name] {
			continue
		}
		cmd.Hidden = true
	}
}

// reservedCommands is the set of built-in command names that plugins must
// not override. This protects core CLI functionality from being hijacked
// by a malicious or misconfigured plugin.
var reservedCommands = map[string]bool{
	"auth": true, "api": true, "audit": true, "login": true, "logout": true,
	"plugin": true, "profile": true, "skill": true, "cache": true,
	"config": true, "doctor": true, "event": true, "completion": true,
	"recovery": true, "upgrade": true, "version": true,
	"schema": true, "mcp": true, "help": true,
}

// addPluginCommandsSafe registers plugin commands with conflict detection.
//
// Rules:
//   - Plugin vs reserved (auth/plugin/cache/...) → reject, warn
//   - Plugin vs plugin (same name)               → reject later one, warn
//   - Plugin vs Market dynamic command            → allow, plugin wins
func addPluginCommandsSafe(root *cobra.Command, pluginCmds []*cobra.Command) {
	// Build index of existing commands before plugin registration.
	existing := make(map[string]bool)
	for _, cmd := range root.Commands() {
		existing[cmd.Name()] = true
	}

	pluginSeen := make(map[string]bool)

	for _, cmd := range pluginCmds {
		name := cmd.Name()

		// Rule 1: never override reserved built-in commands.
		if reservedCommands[name] {
			slog.Warn("plugin: command name conflicts with built-in command, skipping",
				"command", name)
			continue
		}

		// Rule 2: plugin vs plugin — first plugin wins.
		if pluginSeen[name] {
			slog.Warn("plugin: duplicate command from another plugin, skipping",
				"command", name)
			continue
		}
		pluginSeen[name] = true

		// Rule 3: plugin vs Market — plugin wins, remove the old one.
		if existing[name] {
			for _, old := range root.Commands() {
				if old.Name() == name {
					root.RemoveCommand(old)
					slog.Debug("plugin: overriding Market command",
						"command", name)
					break
				}
			}
		}

		root.AddCommand(cmd)
	}
}

// deduplicateCommands removes duplicate top-level commands, keeping the last
// registered one. This ensures overlay commands take precedence over
// open-source defaults when both register the same product name.
func deduplicateCommands(root *cobra.Command) {
	seen := make(map[string]*cobra.Command)
	var dups []*cobra.Command
	for _, cmd := range root.Commands() {
		name := cmd.Name()
		if prev, ok := seen[name]; ok {
			dups = append(dups, prev)
		}
		seen[name] = cmd
	}
	for _, dup := range dups {
		root.RemoveCommand(dup)
	}
}

func configureOutputSink(cmd *cobra.Command) error {
	if local := cmd.LocalFlags().Lookup("output"); local != nil {
		return nil
	}
	outputPath, err := cmd.Flags().GetString("output")
	if err != nil {
		return apperrors.NewInternal("failed to read output flag")
	}
	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		return nil
	}
	if err := validateOptionalPath("--output", outputPath); err != nil {
		return err
	}
	if err := rootMkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return apperrors.NewInternal(fmt.Sprintf("failed to prepare output directory: %v", err))
	}
	file, err := rootCreateFile(outputPath)
	if err != nil {
		return apperrors.NewInternal(fmt.Sprintf("failed to create output file: %v", err))
	}
	cmd.SetOut(file)
	cmd.SetContext(context.WithValue(cmd.Context(), outputFileContextKey{}, file))
	return nil
}

func closeOutputSink(cmd *cobra.Command) error {
	file, ok := cmd.Context().Value(outputFileContextKey{}).(*os.File)
	if !ok || file == nil {
		return nil
	}
	if err := rootCloseFile(file); err != nil {
		return apperrors.NewInternal(fmt.Sprintf("failed to close output file: %v", err))
	}
	return nil
}

func validateOptionalPath(flagName, path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if err := apperrors.SafePath(path); err != nil {
		return apperrors.NewValidation(fmt.Sprintf("%s contains an unsafe path: %v", flagName, err))
	}
	return nil
}

// fileLogger holds the package-level file logger for diagnostics.
// It is initialized by configureLogLevel and closed by CloseFileLogger.
var fileLogger *logging.FileLogger

// configureLogLevel sets the global slog level based on --debug and --verbose flags
// and initializes the file logger for diagnostics.
// --debug → slog.LevelDebug; --verbose → slog.LevelInfo; default → slog.LevelWarn.
func configureLogLevel(flags *GlobalFlags) {
	if flags == nil {
		return
	}
	var level slog.Level
	switch {
	case flags.Debug:
		level = slog.LevelDebug
	case flags.Verbose:
		level = slog.LevelInfo
	default:
		level = slog.LevelWarn
	}
	stderrHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})

	// Initialize file logger — writes to ~/.dws/logs/dws.log at DEBUG level
	// regardless of stderr level. All slog calls are captured for diagnostics.
	// A previously closed rotating writer may have been reopened by slog calls
	// made while constructing another root command, so close it before replacing
	// the package pointer during reentrant executions.
	CloseFileLogger()
	fileLogger = logging.Setup(defaultConfigDir())
	fileHandler := slog.NewJSONHandler(fileLogger.Writer(), &slog.HandlerOptions{Level: slog.LevelDebug})

	slog.SetDefault(slog.New(logging.NewMultiHandler(stderrHandler, fileHandler)))
}

// FileLoggerInstance returns the package-level file logger, or nil if not initialized.
func FileLoggerInstance() *slog.Logger {
	if fileLogger == nil {
		return nil
	}
	return fileLogger.Logger
}

// CloseFileLogger flushes and closes the file logger.
func CloseFileLogger() {
	if fileLogger != nil {
		fileLogger.Close()
	}
}

// loadPlugins registers versioned plugin manifests, stdio clients, hooks, and
// skills. It deliberately does not initialize MCP transports or call
// tools/list while constructing the command tree.
func loadPlugins(engine *pipeline.Engine, _ executor.Runner) []*cobra.Command {
	pluginLoader := plugin.NewLoader(RawVersion())

	// 0a. Inject plugin config values from settings.json as environment
	// variables so that expandPluginVars can resolve ${KEY} references
	// in plugin.json headers, endpoints, etc. User-set env vars take
	// precedence (InjectPluginConfigEnv skips already-set keys).
	rootPluginInjectConfigEnv(pluginLoader)

	// Load TokenData once; reused for stdio injection below.
	tokenData, _ := rootAuthLoadTokenData(defaultConfigDir())
	var userCtx *plugin.UserContext
	if tokenData != nil {
		// Inject user context if either UserID or CorpID is present.
		if tokenData.UserID != "" || tokenData.CorpID != "" {
			userCtx = &plugin.UserContext{
				UserID: tokenData.UserID,
				CorpID: tokenData.CorpID,
			}
		}
	}

	// 1. Load user plugins (per settings.json)
	userPlugins := rootPluginLoadUser(pluginLoader)

	// 2. Load dev plugins (registered via `dws plugin dev`)
	devPlugins := rootPluginLoadDev(pluginLoader)

	allPlugins := append(userPlugins, devPlugins...)

	// 3. Register HTTP descriptors and authentication from the manifest.
	for _, p := range allPlugins {
		for _, srv := range rootPluginDescriptors(p) {
			rootRegisterPluginHTTPServer(srv)
		}
	}

	// 4. Register stdio descriptors and unstarted clients. The subprocess is
	// started and initialized only when a command is actually executed.
	for _, p := range allPlugins {
		for _, sc := range rootPluginStdioClients(p, userCtx) {
			rootRegisterStdioManifest(p, sc)
		}
	}

	// 5. Register plugin hooks into pipeline engine
	if engine != nil {
		for _, p := range allPlugins {
			hooksCfg, err := rootPluginLoadHooks(p)
			if err != nil {
				slog.Warn("plugin: failed to load hooks",
					"plugin", p.Manifest.Name, "error", err)
				continue
			}
			if hooksCfg == nil {
				continue
			}
			for _, entry := range hooksCfg.Hooks {
				engine.Register(plugin.NewHookAdapter(p.Manifest.Name, entry))
			}
		}
	}

	// 7. Sync plugin skills to agent directories
	rootPluginSyncSkills(allPlugins)

	if len(allPlugins) > 0 {
		slog.Debug("plugins loaded",
			"user", len(userPlugins),
			"dev", len(devPlugins),
		)
	}

	return nil
}

func registerPluginHTTPServer(srv mcptypes.ServerDescriptor) {
	AppendDynamicServer(srv)
	if len(srv.AuthHeaders) > 0 {
		registerPluginAuthFromHeaders(srv)
	}
}

// registerPluginAuthFromHeaders extracts authentication credentials from
// a server descriptor's AuthHeaders and registers them in the global
// PluginAuth registry. The runner uses this registry at execution time
// to inject the correct Bearer token for third-party MCP servers.
func registerPluginAuthFromHeaders(srv mcptypes.ServerDescriptor) {
	authToken := ""
	extraHeaders := make(map[string]string)
	for key, value := range srv.AuthHeaders {
		if strings.EqualFold(key, "Authorization") {
			authToken = strings.TrimPrefix(value, "Bearer ")
			authToken = strings.TrimSpace(authToken)
		} else {
			extraHeaders[key] = value
		}
	}
	if authToken == "" {
		return
	}
	var trustedDomains []string
	if parsed, err := url.Parse(srv.Endpoint); err == nil {
		host := parsed.Hostname()
		trustedDomains = []string{host, "*." + host}
	}
	productID := strings.TrimSpace(srv.CLI.ID)
	if productID == "" {
		productID = srv.Key
	}
	RegisterPluginAuth(productID, &PluginAuth{
		Token:          authToken,
		ExtraHeaders:   extraHeaders,
		TrustedDomains: trustedDomains,
	})
}

// newPipelineEngine creates and configures the pipeline engine with
// handlers for all five pipeline phases. The phases execute in order:
// Register → PreParse → PostParse → PreRequest → PostResponse.
//
// Phases are invoked at their respective integration points:
//   - Register:     during command tree construction (newMCPCommand)
//   - PreParse:     before Cobra parses raw argv (RunPreParse)
//   - PostParse:    after Cobra parsing, before validation (canonical RunE)
//   - PreRequest:   after validation, before JSON-RPC dispatch (canonical RunE)
//   - PostResponse: after transport returns, before stdout (canonical RunE)
func newPipelineEngine() *pipeline.Engine {
	engine := pipeline.NewEngine()
	engine.RegisterAll(
		// Register handler runs during command tree building.
		handlers.RegisterHandler{},

		// PreParse handlers run in order: alias → sticky → paramname.
		// Alias normalises case first (--userId → --user-id), then
		// sticky splits glued values (--limit100 → --limit 100), then
		// paramname fixes near-miss typos (--limt → --limit).
		handlers.AliasHandler{},
		handlers.StickyHandler{},
		handlers.ParamNameHandler{},

		// PostParse handlers normalise structured values.
		handlers.ParamValueHandler{},

		// PreRequest handler inspects the validated payload before dispatch.
		handlers.PreRequestHandler{},

		// PostResponse handler processes the response before output.
		handlers.PostResponseHandler{},
	)
	return engine
}
