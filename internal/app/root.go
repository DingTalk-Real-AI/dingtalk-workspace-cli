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
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/compat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/discovery"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/generator"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/logging"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline/handlers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/plugin"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/recovery"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type outputFileContextKey struct{}

const recoveryEventStderrPrefix = "RECOVERY_EVENT_ID="

// Execute runs the root command and returns the process exit code.
func Execute() (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "Error: internal panic: %v\n", r)
			exitCode = 5
		}
	}()

	timing := NewTimingCollector()
	defer func() {
		StopAllStdioClients() // Ensure child processes are terminated on exit
		timing.PrintIfEnabled()
		timing.WriteReportIfEnabled(RawVersion(), SanitizeCommand(os.Args))
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Attach timing collector to context for use by child components
	ctx = WithTimingCollector(ctx, timing)

	initStart := time.Now()
	recovery.ResetRuntimeState()
	engine := newPipelineEngine()
	root := NewRootCommandWithEngine(ctx, engine)
	timing.Record("cmd_init", time.Since(initStart))

	// Run PreParse handlers on raw argv before Cobra parses flags.
	// This corrects model-generated errors like --userId → --user-id
	// and --limit100 → --limit 100.
	pipeline.RunPreParse(root, engine)

	executed, err := root.ExecuteC()
	if err != nil {
		if executed == nil {
			executed = root
		}
		if isUnknownCommandError(err) {
			executed.SetOut(os.Stderr)
			_ = executed.Help()
			_, _ = fmt.Fprintln(os.Stderr)
		}
		_ = printExecutionError(executed, os.Stdout, os.Stderr, err)
		if last := recovery.LatestCapture(); last != nil && last.EventID != "" {
			_, _ = fmt.Fprintf(os.Stderr, "%s%s\n", recoveryEventStderrPrefix, last.EventID)
		}
		return apperrors.ExitCode(err)
	}
	return 0
}

func isUnknownCommandError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "unknown command")
}

// flagErrorWithSuggestions provides helpful suggestions for common flag mistakes.
func flagErrorWithSuggestions(cmd *cobra.Command, err error) error {
	errMsg := err.Error()

	// Common flag aliases and suggestions
	suggestions := map[string]string{
		"--json":        "提示: 请使用 --format json 或 -f json 来输出 JSON 格式",
		"--method":      "提示: dws auth login 默认使用 OAuth 设备流登录，无需指定 --method",
		"--device-flow": "提示: dws auth login 默认已使用设备流，无需 --device-flow 参数",
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
			return fmt.Errorf("%w\n%s", err, suggestion)
		}
	}

	// Fallback: enhance unknown-flag errors with a usage hint.
	// When the command has an Example, the hint includes the actual
	// correct command line. Otherwise a plain '--help' reference is
	// shown — this covers static commands (auth, schema, cache, etc.)
	// that may not have an Example field.
	// Uses %w to preserve the original error type.
	example := strings.TrimSpace(cmd.Example)
	if example != "" {
		firstLine, _, _ := strings.Cut(example, "\n")
		if cmdLine, _, _ := strings.Cut(firstLine, "  #"); cmdLine != "" {
			firstLine = strings.TrimSpace(cmdLine)
		}
		firstLine = strings.ReplaceAll(firstLine, `\"`, `"`)
		return fmt.Errorf("%w; 正确用法: %s; 详见 '%s --help'",
			err, firstLine, cmd.CommandPath())
	}
	return fmt.Errorf("%w; 详见 '%s --help'", err, cmd.CommandPath())
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
		if flags == nil {
			continue
		}
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
	loader := cli.EnvironmentLoader{
		LookupEnv:              os.LookupEnv,
		CatalogBaseURLOverride: DiscoveryBaseURL(),
		AuthTokenFunc: func(ctx context.Context) string {
			return resolveRuntimeAuthToken(ctx, "")
		},
		LoggerFunc: FileLoggerInstance,
	}
	runner := newCommandRunnerWithFlags(loader, flags)

	root := &cobra.Command{
		Use:               "dws",
		Short:             "DWS CLI",
		Args:              cobra.NoArgs,
		SilenceErrors:     true,
		SilenceUsage:      true,
		DisableAutoGenTag: true,
		Version:           Version(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
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
			CloseFileLogger()
			return closeOutputSink(cmd)
		},
	}

	bindPersistentFlags(root, flags)

	schemaCmd := newSchemaCommand(loader)
	genSkillsCmd := newGenerateSkillsCommand()
	genSkillsCmd.Hidden = true
	mcpCmd := newMCPCommand(rootCtx, loader, runner, engine)
	mcpCmd.Hidden = true

	utilityCommands := []*cobra.Command{
		newAuthCommand(),
		newAPICommand(flags),
		newSkillCommand(),
		newCacheCommand(),
		newConfigCommand(),
		newDoctorCommand(),
		newCompletionCommand(root),
		newRecoveryCommand(rootCtx, loader, flags),
		newUpgradeCommand(),
		newVersionCommand(),
		newPluginCommand(),
		schemaCmd,
		genSkillsCmd,
		mcpCmd,
	}
	root.AddCommand(utilityCommands...)

	root.AddCommand(newLegacyPublicCommands(rootCtx, runner)...)
	root.AddCommand(newLegacyHiddenCommands(runner)...)

	// --- Plugin loading: runs AFTER legacy commands so that
	// AppendDynamicServer adds plugin endpoints on top of Market
	// endpoints (SetDynamicServers is called inside loadDynamicCommands).
	pluginCmds := loadPlugins(engine, runner)
	if len(pluginCmds) > 0 {
		addPluginCommandsSafe(root, pluginCmds)
	}

	// PAT authorization commands (open-source core)
	patCaller := newToolCallerAdapter(runner, flags)
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

func newAuthCommand() *cobra.Command {
	return buildAuthCommand()
}

func newSkillCommand() *cobra.Command {
	return buildSkillCommand()
}

func newCacheCommand() *cobra.Command {
	cacheCmd := newPlaceholderParent("cache", "缓存管理")

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "查看缓存状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, err := cmd.Flags().GetBool("json")
			if err != nil {
				return apperrors.NewInternal("failed to read cache status flags")
			}

			store := cacheStoreFromEnv()
			files, bytes, err := cacheDirectoryStats(store.Root)
			if err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to read cache status: %v", err))
			}

			// Enumerate per-server tools cache entries.
			partition := config.DefaultPartition
			entries, _ := store.ListToolsCacheEntries(partition)

			payload := map[string]any{
				"kind":       "cache_status",
				"cache_root": store.Root,
				"files":      files,
				"bytes":      bytes,
			}
			if len(entries) > 0 {
				toolEntries := make([]map[string]any, 0, len(entries))
				for _, e := range entries {
					toolEntries = append(toolEntries, map[string]any{
						"server_key":    e.ServerKey,
						"freshness":     string(e.Freshness),
						"saved_at":      e.SavedAt.Format(time.RFC3339),
						"tool_count":    e.ToolCount,
						"ttl_remaining": e.TTLRemaining,
					})
				}
				payload["tools"] = toolEntries
			}

			if jsonOut {
				return output.WriteJSON(cmd.OutOrStdout(), payload)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "缓存目录: %s\n文件数:   %d   大小: %d 字节\n", store.Root, files, bytes)
			if len(entries) > 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "\n工具缓存:")
				for _, e := range entries {
					age := ""
					if !e.SavedAt.IsZero() {
						dur := time.Since(e.SavedAt).Truncate(time.Minute)
						age = fmt.Sprintf("，%s 前保存", dur)
					}
					ttl := ""
					if e.TTLRemaining != "" {
						ttl = fmt.Sprintf("，剩余 TTL %s", e.TTLRemaining)
					}
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s (%s%s，%d 个工具%s)\n",
						e.ServerKey, string(e.Freshness), age, e.ToolCount, ttl)
				}
			}
			return nil
		},
	}
	statusCmd.Flags().Bool("json", false, "Emit cache status as JSON")

	refreshCmd := &cobra.Command{
		Use:               "refresh",
		Short:             "强制刷新工具缓存",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			product, err := cmd.Flags().GetString("product")
			if err != nil {
				return apperrors.NewInternal("failed to read cache refresh flags")
			}

			store := cacheStoreFromEnv()
			transportClient := transport.NewClient(nil)
			transportClient.AuthToken = resolveRuntimeAuthToken(cmd.Context(), "")
			// Market client here is only a fallback for Detail API calls inside
			// DiscoverAllRuntime; the primary server-list fetch below goes
			// through fetchRegistryServers so edition DiscoveryURL wins.
			service := discovery.NewService(
				market.NewClient(DiscoveryBaseURL(), nil),
				transportClient,
				store,
			)

			resp, err := fetchRegistryServers(cmd.Context(), ipv4HTTPClient(config.HTTPTimeout))
			if err != nil {
				return apperrors.NewDiscovery(fmt.Sprintf("cache refresh: fetch server list failed: %v", err))
			}
			servers := market.NormalizeServers(resp, "live_market")
			_ = store.SaveRegistry(service.CachePartition(), cache.RegistrySnapshot{Servers: servers})

			selected := selectServersForProduct(servers, product)
			if strings.TrimSpace(product) != "" && len(selected) == 0 {
				return apperrors.NewValidation(fmt.Sprintf("no market server matched product %q", product))
			}
			if len(selected) == 0 {
				selected = servers
			}

			if err := clearRuntimeCacheForServers(store, service.CachePartition(), selected); err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to clear cache before refresh: %v", err))
			}

			refreshable := filterRefreshableServers(selected)
			_, failures := service.DiscoverAllRuntime(cmd.Context(), refreshable)
			_, err = fmt.Fprintf(
				cmd.OutOrStdout(),
				"[OK] 缓存刷新完成：已刷新 %d 个服务，失败 %d 个\n缓存目录: %s\n",
				len(refreshable),
				len(failures),
				store.Root,
			)
			return err
		},
	}
	refreshCmd.Flags().String("product", "", "Refresh only the selected canonical product")
	_ = refreshCmd.Flags().MarkHidden("product")

	cleanCmd := &cobra.Command{
		Use:               "clean",
		Short:             "清理缓存",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			staleOnly, err := cmd.Flags().GetBool("stale")
			if err != nil {
				return apperrors.NewInternal("failed to read cache clean stale flag")
			}
			product, err := cmd.Flags().GetString("product")
			if err != nil {
				return apperrors.NewInternal("failed to read cache clean product flag")
			}

			store := cacheStoreFromEnv()
			removed, err := cleanCacheFiles(store.Root, product, staleOnly)
			if err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to clean cache: %v", err))
			}
			_, err = fmt.Fprintf(
				cmd.OutOrStdout(),
				"[OK] 缓存清理完成：已删除 %d 个文件\n",
				removed,
			)
			return err
		},
	}
	cleanCmd.Flags().Bool("stale", false, "Only remove stale cache entries")
	cleanCmd.Flags().String("product", "", "Clean only the selected canonical product")
	cleanCmd.Hidden = true

	cacheCmd.AddCommand(statusCmd, refreshCmd, cleanCmd)
	return cacheCmd
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

			arch := "MCP Dynamic Aggregation"

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

func newGenerateSkillsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "generate-skills",
		Short:             "Generate agent skills from canonical metadata",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			source, err := cmd.Flags().GetString("source")
			if err != nil {
				return apperrors.NewInternal("failed to read generate-skills source flag")
			}
			outputRoot, err := cmd.Flags().GetString("output-root")
			if err != nil {
				return apperrors.NewInternal("failed to read generate-skills output-root flag")
			}
			withDocs, err := cmd.Flags().GetBool("with-docs")
			if err != nil {
				return apperrors.NewInternal("failed to read generate-skills with-docs flag")
			}
			fixture, err := cmd.Flags().GetString("fixture")
			if err != nil {
				return apperrors.NewInternal("failed to read generate-skills fixture flag")
			}
			snapshot, err := cmd.Flags().GetString("snapshot")
			if err != nil {
				return apperrors.NewInternal("failed to read generate-skills snapshot flag")
			}
			catalogPath := fixture
			if strings.EqualFold(strings.TrimSpace(source), string(generator.CatalogSourceSnapshot)) {
				catalogPath = snapshot
			}
			for flagName, raw := range map[string]string{
				"--output-root": outputRoot,
				"--fixture":     fixture,
				"--snapshot":    snapshot,
			} {
				if err := validateOptionalPath(flagName, raw); err != nil {
					return err
				}
			}

			catalog, err := generator.LoadCatalogWithSource(cmd.Context(), source, catalogPath)
			if err != nil {
				return apperrors.NewDiscovery(fmt.Sprintf("failed to load canonical catalog: %v", err))
			}
			artifacts, err := generator.Generate(catalog)
			if err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to generate skill artifacts: %v", err))
			}

			if withDocs {
				if err := generator.WriteArtifacts(outputRoot, artifacts); err != nil {
					return apperrors.NewInternal(fmt.Sprintf("failed to write generated artifacts: %v", err))
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "generated %d artifact(s) in %s\n", len(artifacts), outputRoot)
				return err
			}

			targets := make([]generator.Artifact, 0)
			for _, artifact := range artifacts {
				if !strings.HasPrefix(artifact.Path, "skills/") {
					continue
				}
				targets = append(targets, artifact)
			}
			if len(targets) == 0 {
				return apperrors.NewInternal("no generated skill artifacts were produced")
			}
			if err := generator.WriteArtifacts(outputRoot, targets); err != nil {
				return apperrors.NewInternal(fmt.Sprintf("failed to write generated skills: %v", err))
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "generated %d skill artifact(s) in %s\n", len(targets), outputRoot)
			return err
		},
	}
	cmd.Flags().String("output-root", ".", "Directory root for generated artifacts")
	cmd.Flags().Bool("with-docs", true, "Write docs/schema artifacts in addition to skills")
	cmd.Flags().String("source", string(generator.CatalogSourceFixture), "Catalog source for skill generation: fixture, env, or snapshot")
	cmd.Flags().String("fixture", "", "Optional path to a catalog fixture; used by --source fixture")
	cmd.Flags().String("snapshot", "", "Optional path to a catalog snapshot; used by --source snapshot")
	return cmd
}

func newMCPCommand(ctx context.Context, loader cli.CatalogLoader, runner executor.Runner, engine *pipeline.Engine) *cobra.Command {
	return cli.NewMCPCommand(ctx, loader, runner, engine)
}

// hideNonDirectRuntimeCommands marks top-level product commands as hidden
// unless they correspond to a product discovered via dynamic server discovery
// or listed in the edition's VisibleProducts hook.
// Public utility commands (auth, cache, completion, version) are always kept
// visible; explicitly hidden commands stay hidden.
func hideNonDirectRuntimeCommands(root *cobra.Command) {
	allowedProducts := resolveVisibleProducts()
	staticCommands := map[string]bool{
		"auth":       true,
		"api":        true,
		"cache":      true,
		"config":     true,
		"doctor":     true,
		"completion": true,
		"skill":      true,
		"plugin":     true,
		"version":    true,
		"help":       true,
		"recovery":   true,
		"schema":     true,
		"mcp":        true,
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
	"auth": true, "api": true, "login": true, "logout": true,
	"plugin": true, "skill": true, "cache": true,
	"config": true, "doctor": true, "completion": true,
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

func cacheStoreFromEnv() *cache.Store {
	cacheDir := strings.TrimSpace(os.Getenv(cli.CacheDirEnv))
	return cache.NewStore(cacheDir)
}

// pluginColdTimeouts holds the cold-path discovery budget for plugin MCP
// servers. Timeouts only apply to the *first* discovery for a given
// plugin/server; subsequent startups take the warm cache path and bypass
// the network entirely.
type pluginColdTimeouts struct {
	httpNoAuth time.Duration
	httpAuth   time.Duration
	stdio      time.Duration
}

// resolvePluginColdTimeouts returns the cold-discovery budget for plugin MCP
// servers, applying the DWS_PLUGIN_COLD_TIMEOUT override when set. Defaults
// are tuned so healthy cross-region HTTP endpoints succeed on a cold start
// and Python/Node-based stdio plugins have headroom for interpreter load,
// while an unreachable host still surrenders in bounded time.
func resolvePluginColdTimeouts() pluginColdTimeouts {
	t := pluginColdTimeouts{
		httpNoAuth: 1 * time.Second,
		httpAuth:   1500 * time.Millisecond,
		stdio:      2 * time.Second,
	}
	raw := strings.TrimSpace(os.Getenv(cli.PluginColdTimeoutEnv))
	if raw == "" {
		return t
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		slog.Warn("plugin: ignoring invalid DWS_PLUGIN_COLD_TIMEOUT",
			"value", raw, "error", err)
		return t
	}
	t.httpNoAuth = d
	t.httpAuth = d
	t.stdio = d
	return t
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
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return apperrors.NewInternal(fmt.Sprintf("failed to prepare output directory: %v", err))
	}
	file, err := os.Create(outputPath)
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
	if err := file.Close(); err != nil {
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

func cacheDirectoryStats(root string) (int, int64, error) {
	if strings.TrimSpace(root) == "" {
		return 0, 0, nil
	}
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	files := 0
	var bytes int64
	err := filepath.WalkDir(root, func(entryPath string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files++
		bytes += info.Size()
		return nil
	})
	return files, bytes, err
}

func selectServersForProduct(servers []market.ServerDescriptor, product string) []market.ServerDescriptor {
	product = strings.TrimSpace(strings.ToLower(product))
	if product == "" {
		return servers
	}

	selected := make([]market.ServerDescriptor, 0)
	for _, server := range servers {
		candidates := []string{
			strings.ToLower(strings.TrimSpace(server.DisplayName)),
			strings.ToLower(strings.TrimSpace(server.Key)),
			strings.ToLower(strings.TrimSpace(path.Base(server.Endpoint))),
		}
		for _, candidate := range candidates {
			if candidate == "" {
				continue
			}
			if candidate == product || strings.Contains(candidate, product) {
				selected = append(selected, server)
				break
			}
		}
	}
	return selected
}

func filterRefreshableServers(servers []market.ServerDescriptor) []market.ServerDescriptor {
	filtered := make([]market.ServerDescriptor, 0, len(servers))
	for _, server := range servers {
		if server.CLI.Skip {
			continue
		}
		filtered = append(filtered, server)
	}
	return filtered
}

func clearRuntimeCacheForServers(store *cache.Store, partition string, servers []market.ServerDescriptor) error {
	for _, server := range servers {
		for _, cacheKey := range cacheKeysForServer(server) {
			if err := store.DeleteTools(partition, cacheKey); err != nil {
				return err
			}
		}
		for _, cacheKey := range detailCacheKeysForServer(server) {
			if err := store.DeleteDetail(partition, cacheKey); err != nil {
				return err
			}
		}
	}
	return nil
}

func cacheKeysForServer(server market.ServerDescriptor) []string {
	seen := make(map[string]struct{}, 2)
	keys := make([]string, 0, 2)
	for _, candidate := range []string{
		strings.TrimSpace(server.Key),
		strings.TrimSpace(server.CLI.ID),
	} {
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		keys = append(keys, candidate)
	}
	return keys
}

func detailCacheKeysForServer(server market.ServerDescriptor) []string {
	key := strings.TrimSpace(server.Key)
	if key != "" {
		return []string{key}
	}
	id := strings.TrimSpace(server.CLI.ID)
	if id != "" {
		return []string{id}
	}
	return nil
}

func cleanCacheFiles(root, product string, staleOnly bool) (int, error) {
	if strings.TrimSpace(root) == "" {
		return 0, nil
	}
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	staleCutoff := time.Now().UTC().Add(-cache.ToolsTTL)
	product = strings.TrimSpace(strings.ToLower(product))
	removed := 0

	err := filepath.WalkDir(root, func(entryPath string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		normalizedPath := strings.ToLower(filepath.ToSlash(entryPath))
		if product != "" && !strings.Contains(normalizedPath, product) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		if staleOnly && info.ModTime().After(staleCutoff) {
			return nil
		}
		if err := os.Remove(entryPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		removed++
		return nil
	})
	if err != nil {
		return 0, err
	}
	return removed, nil
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

// loadPlugins scans plugin directories, injects their MCP servers into
// the dynamic server registry, and registers their pipeline hooks.
// This runs before legacy command construction so that plugin servers
// are available for EnvironmentLoader.Load().
func loadPlugins(engine *pipeline.Engine, runner executor.Runner) []*cobra.Command {
	pluginLoader := plugin.NewLoader(RawVersion())

	// 0a. Inject plugin config values from settings.json as environment
	// variables so that expandPluginVars can resolve ${KEY} references
	// in plugin.json headers, endpoints, etc. User-set env vars take
	// precedence (InjectPluginConfigEnv skips already-set keys).
	pluginLoader.InjectPluginConfigEnv()

	// Load TokenData once; reused for stdio injection below.
	tokenData, _ := authpkg.LoadTokenData(defaultConfigDir())
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
	userPlugins := pluginLoader.LoadUser()

	// 2. Load dev plugins (registered via `dws plugin dev`)
	devPlugins := pluginLoader.LoadDev()

	allPlugins := append(userPlugins, devPlugins...)

	// 3. Discover tools from streamable-http servers and build CLI commands.
	//    Third-party servers with auth headers are discovered in parallel
	//    to avoid sequential 10s timeouts when multiple remote servers exist.
	var pluginCmds []*cobra.Command
	tc := transport.NewClient(nil)

	// Collect all server descriptors and register auth first (fast, no I/O).
	type pluginServer struct {
		plugin *plugin.Plugin
		srv    market.ServerDescriptor
	}
	var httpServers []pluginServer

	for _, p := range allPlugins {
		for _, srv := range p.ToServerDescriptors() {
			AppendDynamicServer(srv)

			if len(srv.AuthHeaders) > 0 {
				registerPluginAuthFromHeaders(srv)
			}

			if srv.HasCLIMeta {
				httpServers = append(httpServers, pluginServer{plugin: p, srv: srv})
			}
		}
	}

	// Collect all stdio clients up front so HTTP + stdio discovery can run
	// concurrently — the slowest plugin (typically an unreachable HTTP
	// endpoint hitting its dial timeout) dominates the parallel wall-clock,
	// not the sum of every plugin's cold timeout.
	type stdioEntry struct {
		plugin *plugin.Plugin
		sc     plugin.StdioServerClient
	}
	var stdioEntries []stdioEntry
	for _, p := range allPlugins {
		for _, sc := range p.StdioClients(userCtx) {
			// Use background context so the subprocess lives for the CLI
			// process lifetime (not killed by a short timeout).
			if err := sc.Client.Start(context.Background()); err != nil {
				slog.Warn("plugin: failed to start stdio server",
					"plugin", p.Manifest.Name, "server", sc.Key, "error", err)
				continue
			}
			stdioEntries = append(stdioEntries, stdioEntry{plugin: p, sc: sc})
		}
	}

	// Share one cache.Store across all discovery goroutines. Each goroutine
	// writes to a distinct serverKey path ("tools/<plugin>_<server>.json") with
	// atomic tmp+rename, so concurrent writes to different keys never collide
	// on the filesystem. Global in-process registries (AppendDynamicServer,
	// RegisterStdioClient) carry their own sync.Mutex; see direct_runtime.go
	// and stdio_registry.go.
	sharedStore := cacheStoreFromEnv()
	coldTimeouts := resolvePluginColdTimeouts()

	// Phase A: stdio overlay-first registration (synchronous, no I/O).
	// Plugins whose overlay.json declares ToolOverrides register their full
	// command tree up-front from manifest metadata alone — no subprocess
	// handshake required. This fixes the "discovery fails → no commands
	// ever appear" lock-out and keeps `dws --help` reliable even when the
	// underlying MCP server is temporarily unavailable.
	var legacyStdioEntries []stdioEntry
	for _, e := range stdioEntries {
		cmds, _, ok := registerStdioServerFromOverlay(e.plugin, e.sc, runner, sharedStore)
		if !ok {
			legacyStdioEntries = append(legacyStdioEntries, e)
			continue
		}
		pluginCmds = append(pluginCmds, cmds...)
	}

	// Phase B: fan out discovery in parallel.
	//   - HTTP plugins: same behaviour as before (discovery-first).
	//   - stdio overlay-first plugins: async cache refresh only; their
	//     commands are already registered. Failures are non-fatal and do
	//     NOT poison the warm-cache with a null-tools snapshot.
	//   - stdio legacy plugins (overlay without toolOverrides): preserve
	//     the old discovery-first path for backwards compatibility.
	httpResults := make([][]*cobra.Command, len(httpServers))
	legacyStdioResults := make([][]*cobra.Command, len(legacyStdioEntries))
	var wg sync.WaitGroup
	for i, ps := range httpServers {
		wg.Add(1)
		go func(idx int, ps pluginServer) {
			defer wg.Done()
			httpResults[idx] = registerHTTPServer(ps.plugin, ps.srv, tc, runner, sharedStore, coldTimeouts)
		}(i, ps)
	}
	// overlay-first stdio: async refresh (no command building here).
	for _, e := range stdioEntries {
		if !hasOverlayToolOverrides(e.plugin, e.sc) {
			continue
		}
		wg.Add(1)
		go func(e stdioEntry) {
			defer wg.Done()
			refreshStdioToolsCache(e.plugin, e.sc, sharedStore, coldTimeouts)
		}(e)
	}
	// legacy stdio: discovery-first (commands depend on tool list).
	for i, e := range legacyStdioEntries {
		wg.Add(1)
		go func(idx int, e stdioEntry) {
			defer wg.Done()
			legacyStdioResults[idx] = registerStdioServer(e.plugin, e.sc, runner, sharedStore, coldTimeouts)
		}(i, e)
	}
	wg.Wait()
	for _, cmds := range httpResults {
		pluginCmds = append(pluginCmds, cmds...)
	}
	for _, cmds := range legacyStdioResults {
		pluginCmds = append(pluginCmds, cmds...)
	}

	// 5. Register plugin hooks into pipeline engine
	if engine != nil {
		for _, p := range allPlugins {
			hooksCfg, err := p.LoadHooks()
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
	plugin.SyncSkills(allPlugins)

	if len(allPlugins) > 0 {
		slog.Debug("plugins loaded",
			"user", len(userPlugins),
			"dev", len(devPlugins),
		)
	}

	return pluginCmds
}

// pluginCacheKey derives the cache key used to persist a plugin MCP server's
// tool list. Prefixed with "plugin:" so entries are namespaced apart from the
// Market-derived cache, and visible distinctly via `dws cache status`.
func pluginCacheKey(pluginName, serverKey string) string {
	return "plugin:" + pluginName + ":" + serverKey
}

// registerHTTPServer discovers tools from a streamable-http MCP server and
// builds CLI commands. Used for plugin-owned HTTP servers that provide CLI metadata.
//
// Startup-latency strategy (issue #119):
//   - Warm cache: build commands from the persisted tools snapshot
//     synchronously — no network I/O. `dws --help` returns in ms even when
//     the plugin endpoint is unreachable.
//   - Cold cache: synchronous discovery (Initialize + ListTools) with a tight
//     timeout. The outcome — success or failure — is persisted so the next
//     invocation hits the warm path. Refresh on demand via `dws cache clean`
//     / `dws cache refresh`; the cache TTL (7d) otherwise expires naturally.
//
// When the server descriptor carries AuthHeaders (from plugin.json "headers"),
// a dedicated transport.Client is created with the plugin's Bearer token and
// trusted domains so that third-party MCP servers requiring independent
// authentication (e.g. Alibaba Cloud Bailian) can be discovered at startup.
func registerHTTPServer(p *plugin.Plugin, srv market.ServerDescriptor, tc *transport.Client, runner executor.Runner, store *cache.Store, timeouts pluginColdTimeouts) []*cobra.Command {
	partition := config.DefaultPartition
	cacheKey := pluginCacheKey(p.Manifest.Name, srv.Key)

	if snapshot, freshness, err := store.LoadTools(partition, cacheKey); err == nil {
		slog.Debug("plugin: http server served from cache",
			"plugin", p.Manifest.Name, "server", srv.Key,
			"tools", len(snapshot.Tools), "freshness", string(freshness))
		return buildHTTPCommandsFromTools(srv, snapshot.Tools, runner)
	}

	// Cold cache: synchronous discovery. Persist the outcome even on failure
	// (empty tools == negative cache) so the next invocation takes the fast
	// path regardless of endpoint health.
	tools := discoverHTTPTools(p, srv, tc, timeouts)
	_ = store.SaveTools(partition, cacheKey, cache.ToolsSnapshot{
		ServerKey: cacheKey,
		Tools:     tools,
	})
	return buildHTTPCommandsFromTools(srv, tools, runner)
}

// discoverHTTPTools performs the blocking Initialize + ListTools handshake
// for an HTTP MCP server and returns the discovered tools. Returns nil on
// any transport/protocol error; errors are logged at Debug level.
func discoverHTTPTools(p *plugin.Plugin, srv market.ServerDescriptor, tc *transport.Client, timeouts pluginColdTimeouts) []transport.ToolDescriptor {
	// Cold-path budget. An unreachable endpoint will burn the full window
	// via the TCP dial timeout; a healthy localhost/third-party endpoint
	// typically responds in <200 ms. Third-party servers with auth get a
	// slightly larger window to accommodate TLS + auth RTT. Operators with
	// cross-region endpoints can relax the window via DWS_PLUGIN_COLD_TIMEOUT.
	// The outcome is persisted as a negative cache so subsequent startups
	// (80 ms warm) are unaffected. See issue #119.
	timeout := timeouts.httpNoAuth
	if len(srv.AuthHeaders) > 0 {
		timeout = timeouts.httpAuth
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	discoveryClient := tc
	if len(srv.AuthHeaders) > 0 {
		discoveryClient = buildPluginAuthClient(tc, srv)
	}

	if _, err := discoveryClient.Initialize(ctx, srv.Endpoint); err != nil {
		slog.Debug("plugin: http server offline, skipping tool discovery",
			"plugin", p.Manifest.Name, "server", srv.Key)
		return nil
	}

	toolsResult, err := discoveryClient.ListTools(ctx, srv.Endpoint)
	if err != nil {
		slog.Debug("plugin: http ListTools failed",
			"plugin", p.Manifest.Name, "server", srv.Key, "error", err)
		return nil
	}
	return toolsResult.Tools
}

// buildHTTPCommandsFromTools converts a tool list into Cobra commands via
// the BuildDynamicCommands path. Returns nil for an empty tool list.
func buildHTTPCommandsFromTools(srv market.ServerDescriptor, tools []transport.ToolDescriptor, runner executor.Runner) []*cobra.Command {
	if len(tools) == 0 {
		return nil
	}

	detailsByID := make(map[string][]market.DetailTool)
	var detailTools []market.DetailTool
	for _, tool := range tools {
		schemaJSON := ""
		if tool.InputSchema != nil {
			if data, marshalErr := json.Marshal(tool.InputSchema); marshalErr == nil {
				schemaJSON = string(data)
			}
		}
		detailTools = append(detailTools, market.DetailTool{
			ToolName:    tool.Name,
			ToolTitle:   tool.Title,
			ToolDesc:    tool.Description,
			IsSensitive: tool.Sensitive,
			ToolRequest: schemaJSON,
		})
	}
	detailsByID[strings.TrimSpace(srv.CLI.ID)] = detailTools

	// If the server has no ToolOverrides (e.g. third-party MCP servers that
	// only declare cli.id and cli.command), auto-generate one override per
	// discovered tool so BuildDynamicCommands can create leaf commands.
	if len(srv.CLI.ToolOverrides) == 0 {
		srv.CLI.ToolOverrides = make(map[string]market.CLIToolOverride, len(tools))
		for _, tool := range tools {
			srv.CLI.ToolOverrides[tool.Name] = market.CLIToolOverride{
				CLIName: deriveToolCLIName(tool.Name),
			}
		}
	}

	return compat.BuildDynamicCommands(
		[]market.ServerDescriptor{srv}, runner, detailsByID)
}

// deriveToolCLIName converts an MCP tool name (e.g. "web_search" or
// "maps.search_poi") into a kebab-case CLI command name ("search" or
// "search-poi"). It strips common prefixes and replaces underscores/dots
// with hyphens.
func deriveToolCLIName(toolName string) string {
	// Use the last segment after "." as the base name.
	if idx := strings.LastIndex(toolName, "."); idx >= 0 {
		toolName = toolName[idx+1:]
	}
	// Replace underscores with hyphens for kebab-case.
	return strings.ReplaceAll(toolName, "_", "-")
}

// buildPluginAuthClient creates a transport.Client copy with the plugin's
// Bearer token and trusted domains injected. This allows third-party MCP
// servers that require independent authentication to be discovered at startup.
func buildPluginAuthClient(base *transport.Client, srv market.ServerDescriptor) *transport.Client {
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
		return base
	}
	client := base.WithAuth(authToken, extraHeaders)
	// Trust the endpoint's hostname so the token is actually sent.
	if parsed, err := url.Parse(srv.Endpoint); err == nil {
		host := parsed.Hostname()
		client.TrustedDomains = []string{host, "*." + host}
	}
	return client
}

// registerPluginAuthFromHeaders extracts authentication credentials from
// a server descriptor's AuthHeaders and registers them in the global
// PluginAuth registry. The runner uses this registry at execution time
// to inject the correct Bearer token for third-party MCP servers.
func registerPluginAuthFromHeaders(srv market.ServerDescriptor) {
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

// registerStdioServer initializes a stdio MCP server, discovers its tools
// via ListTools, builds CLI commands, and registers the StdioClient for
// runtime dispatch. Returns generated cobra commands.
//
// Warm-cache fast path (issue #119): when a tools snapshot is already cached
// for this plugin/server, skip the Initialize + ListTools RPC round-trip and
// rebuild commands directly from the snapshot. Cold cache falls back to
// synchronous discovery with a 4s cap and persists the outcome.
func registerStdioServer(p *plugin.Plugin, sc plugin.StdioServerClient, runner executor.Runner, store *cache.Store, timeouts pluginColdTimeouts) []*cobra.Command {
	partition := config.DefaultPartition
	cacheKey := pluginCacheKey(p.Manifest.Name, sc.Key)

	if snapshot, freshness, err := store.LoadTools(partition, cacheKey); err == nil {
		slog.Debug("plugin: stdio server served from cache",
			"plugin", p.Manifest.Name, "server", sc.Key,
			"tools", len(snapshot.Tools), "freshness", string(freshness))
		return buildStdioCommands(p, sc, snapshot.Tools, runner)
	}

	tools := discoverStdioTools(p, sc, timeouts)
	_ = store.SaveTools(partition, cacheKey, cache.ToolsSnapshot{
		ServerKey: cacheKey,
		Tools:     tools,
	})
	return buildStdioCommands(p, sc, tools, runner)
}

// discoverStdioTools performs the blocking Initialize + ListTools handshake
// on a stdio MCP subprocess. Returns nil on any error (logged at Warn level).
// The default 2s budget comfortably accommodates Python/Node runtimes whose
// interpreter + dependency load dominates the first response. Operators with
// heavier startup chains can relax further via DWS_PLUGIN_COLD_TIMEOUT.
func discoverStdioTools(p *plugin.Plugin, sc plugin.StdioServerClient, timeouts pluginColdTimeouts) []transport.ToolDescriptor {
	ctx, cancel := context.WithTimeout(context.Background(), timeouts.stdio)
	defer cancel()

	if _, err := sc.Client.Initialize(ctx); err != nil {
		slog.Warn("plugin: stdio initialize failed",
			"plugin", p.Manifest.Name, "server", sc.Key, "error", err)
		return nil
	}
	toolsResult, err := sc.Client.ListTools(ctx)
	if err != nil {
		slog.Warn("plugin: stdio ListTools failed",
			"plugin", p.Manifest.Name, "server", sc.Key, "error", err)
		return nil
	}
	return toolsResult.Tools
}

// buildStdioCommands constructs Cobra commands from a tool list and
// registers the runtime dispatch state (StdioClient + dynamic server).
// Returns nil for an empty tool list.
//
// This is the legacy discovery-first path, used only for stdio plugins whose
// overlay.json does NOT carry toolOverrides. Plugins that ship toolOverrides
// register commands up-front via registerStdioServerFromOverlay, bypassing
// this function entirely (see plugin_stdio_overlay.go).
func buildStdioCommands(p *plugin.Plugin, sc plugin.StdioServerClient, tools []transport.ToolDescriptor, runner executor.Runner) []*cobra.Command {
	if len(tools) == 0 {
		slog.Debug("plugin: stdio server has no tools",
			"plugin", p.Manifest.Name, "server", sc.Key)
		return nil
	}

	overlay := resolveStdioOverlay(p, sc)

	// Auto-generate ToolOverrides from discovered tools when not provided
	// by the manifest/overlay (legacy discovery-first path).
	if len(overlay.ToolOverrides) == 0 {
		overlay.ToolOverrides = make(map[string]market.CLIToolOverride)
		if len(overlay.Prefixes) == 0 {
			overlay.Prefixes = []string{overlay.ID}
		}
		for _, tool := range tools {
			overlay.ToolOverrides[tool.Name] = market.CLIToolOverride{
				IsSensitive: tool.Sensitive,
			}
		}
	}

	descriptor := market.ServerDescriptor{
		Key:         sc.Key,
		DisplayName: p.Manifest.Name + "/" + sc.Key,
		Description: p.Manifest.Description,
		Endpoint:    StdioEndpoint(p.Manifest.Name, sc.Key),
		Source:      "plugin",
		CLI:         overlay,
		HasCLIMeta:  true,
	}

	AppendDynamicServer(descriptor)
	RegisterStdioClient(p.Manifest.Name+"/"+sc.Key, sc.Client)

	detailsByID := toolsToDetails(tools, overlay.ID)
	cmds := compat.BuildDynamicCommands(
		[]market.ServerDescriptor{descriptor}, runner, detailsByID)

	slog.Debug("plugin: stdio server registered",
		"plugin", p.Manifest.Name, "server", sc.Key,
		"tools", len(tools), "commands", len(cmds))

	return cmds
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

		// PreParse handlers run in order: dash → alias → sticky → paramname.
		// Dash corrects single-dash to double-dash (-name → --name), then
		// Alias normalises case first (--userId → --user-id), then
		// sticky splits glued values (--limit100 → --limit 100), then
		// paramname fixes near-miss typos (--limt → --limit).
		handlers.DashHandler{},
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
