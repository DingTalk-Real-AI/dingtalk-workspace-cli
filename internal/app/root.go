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
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/discovery"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/generator"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/logging"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/market"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline/handlers"
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
func Execute() int {
	totalStart := time.Now()
	timing := NewTimingCollector()
	defer func() {
		timing.PrintIfEnabled()
		if os.Getenv("DWS_PERF_DEBUG") != "" {
			_, _ = fmt.Fprintf(os.Stderr, "[PERF] Execute total: %v\n", time.Since(totalStart))
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Attach timing collector to context for use by child components
	ctx = WithTimingCollector(ctx, timing)

	initStart := time.Now()
	recovery.ResetRuntimeState()
	engine := newPipelineEngine()
	root := NewRootCommandWithEngine(ctx, engine)
	initDuration := time.Since(initStart)
	timing.Record("cmd_init", initDuration)
	if os.Getenv("DWS_PERF_DEBUG") != "" {
		_, _ = fmt.Fprintf(os.Stderr, "[PERF] command init: %v\n", initDuration)
	}

	// Run PreParse handlers on raw argv before Cobra parses flags.
	// This corrects model-generated errors like --userId → --user-id
	// and --limit100 → --limit 100.
	pipeline.RunPreParse(root, engine)

	execStart := time.Now()
	executed, err := root.ExecuteC()
	execDuration := time.Since(execStart)
	timing.Record("cobra_exec", execDuration)
	if os.Getenv("DWS_PERF_DEBUG") != "" {
		_, _ = fmt.Fprintf(os.Stderr, "[PERF] cobra ExecuteC: %v\n", execDuration)
	}
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

	return err
}

func printExecutionError(root *cobra.Command, stdout, stderr io.Writer, err error) error {
	if wantsJSONErrors(root) {
		return apperrors.PrintJSON(stdout, err)
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

			return configureOutputSink(cmd)
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
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
		newSkillCommand(),
		newCacheCommand(),
		newCompletionCommand(root),
		newRecoveryCommand(rootCtx, loader, flags),
		newUpgradeCommand(),
		newVersionCommand(),
		schemaCmd,
		genSkillsCmd,
		mcpCmd,
	}
	root.AddCommand(utilityCommands...)
	root.AddCommand(newLegacyPublicCommands(rootCtx, runner)...)
	root.AddCommand(newLegacyHiddenCommands(runner)...)

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
			baseURL := DiscoveryBaseURL()

			store := cacheStoreFromEnv()
			transportClient := transport.NewClient(nil)
			transportClient.AuthToken = resolveRuntimeAuthToken(cmd.Context(), "")
			service := discovery.NewService(
				market.NewClient(baseURL, nil),
				transportClient,
				store,
			)
			servers, err := service.DiscoverServers(cmd.Context())
			if err != nil {
				return err
			}

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
	var allowedProducts map[string]bool
	if fn := edition.Get().VisibleProducts; fn != nil {
		products := fn()
		allowedProducts = make(map[string]bool, len(products))
		for _, p := range products {
			allowedProducts[p] = true
		}
	} else {
		allowedProducts = DirectRuntimeProductIDs()
	}
	staticCommands := map[string]bool{
		"auth":       true,
		"cache":      true,
		"completion": true,
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

// newPipelineEngine creates and configures the pipeline engine with
// the standard set of handlers for model input correction.
func newPipelineEngine() *pipeline.Engine {
	engine := pipeline.NewEngine()
	engine.RegisterAll(
		// PreParse handlers run in order: alias → sticky → paramname.
		// Alias normalises case first (--userId → --user-id), then
		// sticky splits glued values (--limit100 → --limit 100), then
		// paramname fixes near-miss typos (--limt → --limit).
		handlers.AliasHandler{},
		handlers.StickyHandler{},
		handlers.ParamNameHandler{},

		// PostParse handlers normalise structured values.
		handlers.ParamValueHandler{},
	)
	return engine
}
