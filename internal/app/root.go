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
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/logging"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline/handlers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/plugin"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/usage"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/agentproduct"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type outputFileContextKey struct{}

var (
	rootNormalizeProcessProfileArgs = normalizeProcessProfileArgs
	rootExecuteCommand              = (*cobra.Command).ExecuteC
	rootNewRootCommandWithEngine    = NewRootCommandWithEngine
	rootRunPreParse                 = pipeline.RunPreParse
	rootStopAllStdioClients         = StopAllStdioClients
	rootLoadPlugins                 = loadPlugins
	rootMkdirAll                    = os.MkdirAll
	rootCreateTemp                  = os.CreateTemp
	rootSyncFile                    = (*os.File).Sync
	rootCloseFile                   = (*os.File).Close
	// os.Rename replaces an existing non-directory target on every supported
	// Go host; the Windows implementation uses MOVEFILE_REPLACE_EXISTING. Keep
	// the temporary file beside the target so publication stays on one volume.
	rootRenameFile                  = os.Rename
	rootRemoveFile                  = os.Remove
	rootPluginInjectConfigEnv       = (*plugin.Loader).InjectPluginConfigEnv
	rootPluginLoadUser              = (*plugin.Loader).LoadUser
	rootPluginLoadDev               = (*plugin.Loader).LoadDev
	rootPluginDescriptors           = (*plugin.Plugin).ToServerDescriptors
	rootPluginStdioClients          = (*plugin.Plugin).StdioClients
	rootRegisterPluginHTTPServer    = registerPluginHTTPServer
	rootPluginStdioDescriptor       = stdioServerDescriptorFromManifest
	rootRegisterResolvedStdioServer = registerResolvedStdioServer
	rootPluginLoadHooks             = (*plugin.Plugin).LoadHooks
	rootPluginSyncSkills            = plugin.SyncSkills
	rootAuthLoadTokenData           = authpkg.LoadTokenData
	rootNewCommandRunnerWithFlags   = newCommandRunnerWithFlags
	rootEmitResult                  = output.EmitResult
	rootInstallProcessSignalContext = installProcessSignalContext
)

// Execute runs the root command and returns the process exit code.
func Execute() int {
	exitCode, _, _ := ExecuteWithTelemetry()
	return exitCode
}

// ExecuteWithTelemetry runs the root command and additionally returns a
// privacy-safe command path and error summary for the official CLI entrypoint.
func ExecuteWithTelemetry() (exitCode int, commandPath string, errorMessage string) {
	commandPath = "dws"
	var (
		root        *cobra.Command
		executed    *cobra.Command
		resultStore *output.ResultStore
	)
	defer func() {
		if r := recover(); r != nil {
			errorMessage = "internal panic"
			target := executed
			if target == nil && root != nil {
				if found, _, err := root.Find(os.Args[1:]); err == nil {
					target = found
				}
			}
			if target != nil {
				commandPath = telemetryCommandPath(target)
			}
			if code, attempted, _, _ := output.StoredEmissionState(resultStore); attempted {
				exitCode = code
				if target != nil {
					fmt.Fprintf(target.ErrOrStderr(), "Warning: command panicked after result emission attempt: %v\n", r)
				}
			} else if target != nil && output.UsesUnifiedResult(target) {
				info := &output.ErrorInfo{Type: "internal", ExitCode: 5, Message: fmt.Sprintf("internal panic: %v", r)}
				if code, err := output.EmitResult(target, output.Failure(info)); err == nil {
					exitCode = code
				} else {
					fmt.Fprintf(os.Stderr, "Error: internal panic: %v\n", r)
					exitCode = 5
				}
			} else {
				fmt.Fprintf(os.Stderr, "Error: internal panic: %v\n", r)
				exitCode = 5
			}
			if executed == nil {
				executed = target
			}
		}
		CloseFileLogger()
		if executed != nil {
			if err := closeOutputSink(executed); err != nil {
				errorMessage = telemetryErrorSummary(err)
				if code, handled, emitErr := emitOutputPublicationFailure(executed, err); handled && emitErr == nil {
					exitCode = code
				} else {
					exitCode = apperrors.ExitCode(err)
					fmt.Fprintf(os.Stderr, "Warning: close output sink: %v\n", err)
					if emitErr != nil {
						fmt.Fprintf(os.Stderr, "Warning: emit output publication failure: %v\n", emitErr)
					}
				}
			}
		}
	}()

	restoreArgs := rootNormalizeProcessProfileArgs()
	defer restoreArgs()

	// Validate MCP Agent metadata before constructing the command tree.
	// Construction may invoke edition registration/static-server hooks and load
	// plugin PreParse handlers, so PersistentPreRunE alone is too late for the
	// process entry point. Retain this exact pair for the eventual invocation.
	agentMetadata := readAgentMetadataSnapshot()
	if err := agentMetadata.validationError(); err != nil {
		emitEarlyAgentMetadataValidationError(err, os.Args[1:])
		errorMessage = telemetryErrorSummary(err)
		exitCode = apperrors.ExitCode(err)
		return
	}

	timing := NewTimingCollector()
	defer func() {
		rootStopAllStdioClients() // Ensure child processes are terminated on exit
		CloseAuditSink()          // Drain async audit forwards on all exit paths,
		// including command errors where Cobra skips PersistentPostRunE.
		timing.PrintIfEnabled()
		timing.WriteReportIfEnabled(RawVersion(), SanitizeCommand(os.Args))
	}()

	// Attach timing collector to context for use by child components
	ctx := WithTimingCollector(context.Background(), timing)
	ctx = contextWithAgentMetadataSnapshot(ctx, agentMetadata)
	ctx, resultStore = output.WithResultStore(ctx)
	var signalState *processSignalState
	var stopSignals func()
	ctx, signalState, stopSignals = rootInstallProcessSignalContext(ctx, resultStore)
	defer stopSignals()

	initStart := time.Now()
	engine := newPipelineEngine()
	root = rootNewRootCommandWithEngine(ctx, engine)
	commandPath = telemetryCommandPath(root)
	timing.Record("cmd_init", time.Since(initStart))

	// Run PreParse handlers on raw argv before Cobra parses flags.
	// This corrects model-generated errors like --userId → --user-id
	// and --limit100 → --limit 100.
	if err := rootRunPreParse(root, engine); err != nil {
		err = newPreParseValidationError(err)
		if interrupted, _ := signalState.outcome(); interrupted != nil {
			err = interrupted
		}
		if target, _, findErr := root.Find(os.Args[1:]); findErr == nil && target != nil && output.UsesUnifiedResult(target) {
			result := output.FailureWithExitCode(errorInfoFromExecutionError(err), apperrors.ExitCode(err))
			code, emitErr := output.EmitResult(target, result)
			if emitErr == nil {
				errorMessage = telemetryErrorSummary(err)
				exitCode = code
				return
			}
		}
		_ = printExecutionError(root, os.Stdout, os.Stderr, err)
		errorMessage = telemetryErrorSummary(err)
		exitCode = apperrors.ExitCode(err)
		return
	}
	commandPath = telemetryCommandPathForArgs(root, os.Args[1:])

	var err error
	executed, err = rootExecuteCommand(root)
	if executed != nil {
		commandPath = telemetryCommandPath(executed)
	}
	// PersistentPostRunE normally commits or aborts the transactional output
	// sink. Finalize once more at the process boundary so custom execution
	// seams, embedding callers, or future hook changes cannot leave publication
	// errors to a defer that runs after the process exit code is fixed.
	if executed != nil {
		if err == nil {
			if closeErr := closeOutputSink(executed); closeErr != nil {
				err = closeErr
			}
		} else if abortErr := abortOutputSink(executed); abortErr != nil {
			fmt.Fprintf(executed.ErrOrStderr(), "Warning: abort output sink after command failure: %v\n", abortErr)
		}
	}
	interrupted, primaryCompletedBeforeSignal := signalState.outcome()
	if interrupted != nil && !primaryCompletedBeforeSignal {
		if code, attempted, _, _ := output.StoredEmissionState(resultStore); attempted {
			var publicationErr *outputPublicationError
			if err != nil && stderrors.As(err, &publicationErr) {
				// The successful result was written only to a transaction that did
				// not publish. Let the error path replace it with one observable
				// failure envelope on the restored original stream.
			} else {
				if executed == nil {
					executed = root
				}
				fmt.Fprintf(executed.ErrOrStderr(), "Warning: process interrupted after result emission attempt: %v\n", interrupted)
				// Once publication starts, its stored exit code is authoritative. A
				// signal recorded just before or during publication must not turn a
				// successfully emitted result into a contradictory 130/143 process
				// status; likewise, a failed publication must retain its internal
				// error code instead of being relabelled as cancellation.
				errorMessage = telemetryErrorSummary(interrupted)
				exitCode = code
				return
			}
		}
		var publicationErr *outputPublicationError
		if err == nil || !stderrors.As(err, &publicationErr) {
			err = interrupted.withCancellationDetail(err)
		}
	}
	if err != nil {
		if executed == nil {
			executed = root
			commandPath = telemetryCommandPath(root)
		}
		if code, attempted, _, _ := output.StoredEmissionState(resultStore); attempted {
			var publicationErr *outputPublicationError
			if stderrors.As(err, &publicationErr) {
				errorMessage = telemetryErrorSummary(publicationErr)
				if failureCode, handled, emitErr := emitOutputPublicationFailure(executed, publicationErr); handled {
					if emitErr == nil {
						exitCode = failureCode
						return
					}
					fmt.Fprintf(executed.ErrOrStderr(), "Warning: emit output publication failure: %v\n", emitErr)
				}
				exitCode = apperrors.ExitCode(publicationErr)
				return
			}
			fmt.Fprintf(executed.ErrOrStderr(), "Warning: command hook failed after result emission: %v\n", err)
			errorMessage = telemetryErrorSummary(err)
			exitCode = code
			return
		}
		err = rewordRequiredFlagError(err)
		var raw apperrors.RawStderrError
		if output.UsesUnifiedResult(executed) && !stderrors.As(err, &raw) {
			result := output.FailureWithExitCode(errorInfoFromExecutionError(err), apperrors.ExitCode(err))
			code, emitErr := output.EmitResult(executed, result)
			if emitErr == nil {
				errorMessage = telemetryErrorSummary(err)
				exitCode = code
				return
			}
			err = apperrors.NewInternal("emit failure result: "+emitErr.Error(), apperrors.WithCause(emitErr))
		}
		if isUnknownCommandError(err) {
			executed.SetOut(os.Stderr)
			_ = executed.Help()
			_, _ = fmt.Fprintln(os.Stderr)
		}
		_ = printExecutionError(executed, os.Stdout, os.Stderr, err)
		errorMessage = telemetryErrorSummary(err)
		exitCode = apperrors.ExitCode(err)
		return
	}
	if code, emitted := output.StoredExitCode(resultStore); emitted {
		exitCode = code
		return
	}
	return
}

func telemetryCommandPath(command *cobra.Command) string {
	if command == nil {
		return "dws"
	}
	path := strings.TrimSpace(command.CommandPath())
	root := command.Root()
	rootName := strings.TrimSpace(root.Name())
	if path == rootName {
		return rootName
	}
	if rootName != "" {
		path = strings.TrimSpace(strings.TrimPrefix(path, rootName+" "))
	}
	return path
}

func telemetryCommandPathForArgs(root *cobra.Command, args []string) string {
	if root == nil {
		return "dws"
	}
	command, _, err := root.Find(args)
	if err != nil || command == nil {
		return telemetryCommandPath(root)
	}
	return telemetryCommandPath(command)
}

// emitEarlyAgentMetadataValidationError preserves each built-in command's
// legacy-vs-unified output contract without running extension hooks. The
// presentation-only tree contains reviewed open-source commands and flags but
// deliberately omits edition registration, plugin loading, and visibility
// hooks; callers therefore still fail before any external hook executes.
func emitEarlyAgentMetadataValidationError(err error, args []string) {
	format := processArgsFormat(args)
	presentationRoot := newRootPresentationCommand()
	_ = presentationRoot.PersistentFlags().Set("format", format)
	if target, _, findErr := presentationRoot.Find(args); findErr == nil && target != nil && output.UsesUnifiedResult(target) {
		target.SetOut(os.Stdout)
		target.SetErr(os.Stderr)
		result := output.FailureWithExitCode(errorInfoFromExecutionError(err), apperrors.ExitCode(err))
		if _, emitErr := rootEmitResult(target, result); emitErr == nil {
			return
		}
	}
	if strings.EqualFold(strings.TrimSpace(format), "json") {
		_ = apperrors.PrintJSON(os.Stderr, err)
		return
	}
	_ = apperrors.PrintHumanAt(os.Stderr, err, apperrors.VerbosityNormal)
}

// processArgsRequestJSON preserves the CLI's machine-readable error contract
// for validation that must occur before Cobra and its presentation flags exist.
// The global format defaults to JSON; an explicit non-JSON format switches to
// the human diagnostic path. Last occurrence wins, matching pflag semantics.
func processArgsRequestJSON(args []string) bool {
	return strings.EqualFold(strings.TrimSpace(processArgsFormat(args)), "json")
}

func processArgsFormat(args []string) string {
	format := "json"
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			break
		}
		if value, ok := strings.CutPrefix(arg, "--format="); ok {
			format = value
			continue
		}
		if value, ok := strings.CutPrefix(arg, "-f="); ok {
			format = value
			continue
		}
		if strings.HasPrefix(arg, "-f") && len(arg) > len("-f") {
			format = strings.TrimPrefix(arg, "-f")
			continue
		}
		if arg != "--format" && arg != "-f" {
			continue
		}
		if index+1 >= len(args) {
			format = ""
			break
		}
		index++
		format = args[index]
	}
	return format
}

// errorInfoFromExecutionError projects the repository error model into the unified
// failure body. Exit code and category are derived from the same error value,
// preventing the wire and process status from drifting apart.
func errorInfoFromExecutionError(err error) *output.ErrorInfo {
	exitCode := apperrors.ExitCode(err)
	info := &output.ErrorInfo{
		Type:     errorTypeForExitCode(exitCode),
		ExitCode: exitCode,
		Message:  err.Error(),
	}
	var interrupted *processInterruption
	if stderrors.As(err, &interrupted) && interrupted != nil {
		info.Type = "internal"
		info.Subtype = interrupted.Subtype()
		return info
	}
	if stderrors.Is(err, context.DeadlineExceeded) {
		info.Subtype = "deadline_exceeded"
	}
	var cliErr *helpers.CLIError
	if stderrors.As(err, &cliErr) && cliErr != nil {
		info.UpstreamCode = cliErr.Code
		info.Hint = cliErr.Suggestion
		info.Operation = cliErr.Operation
	}
	var callErr *transport.CallError
	if stderrors.As(err, &callErr) && callErr != nil {
		info.HTTPStatus = callErr.HTTPStatus
		info.RPCCode = callErr.RPCCode
		info.Stage = string(callErr.Stage)
		if callErr.RequestID != "" {
			info.RequestID = callErr.RequestID
		} else if callErr.TraceID != "" {
			info.RequestID = callErr.TraceID
		}
	}
	var typed *apperrors.Error
	if !stderrors.As(err, &typed) || typed == nil {
		return info
	}
	if typed.Category == apperrors.CategoryPartial {
		// An error lacks the item-level data required by partial_failure.
		// Callers must use output.Partial; fail closed consistently otherwise.
		info.Type = string(apperrors.CategoryInternal)
	} else {
		info.Type = string(typed.Category)
	}
	info.Subtype = typed.Reason
	if typed.Hint != "" {
		info.Hint = typed.Hint
	}
	info.Actions = apperrors.RecoveryActions(err)
	info.Retryable = typed.RetryableSet && typed.Retryable
	info.RetryAfterSeconds = typed.RetryAfterSeconds
	if typed.RPCCode != 0 {
		info.RPCCode = typed.RPCCode
	}
	if typed.ServerDiag.TraceID != "" {
		info.TraceID = typed.ServerDiag.TraceID
	}
	if typed.Operation != "" {
		info.Operation = typed.Operation
	}
	info.ServerKey = typed.ServerKey
	info.Origin = typed.Origin
	if typed.FailureStage != "" {
		info.Stage = typed.FailureStage
	}
	info.ExecutionStarted = typed.ExecutionStarted
	if typed.NextRetryAt != nil {
		info.NextRetryAt = typed.NextRetryAt.UTC().Format(time.RFC3339)
	}
	info.AvailableFlags = append([]string(nil), typed.AvailableFlags...)
	info.SnapshotPath = typed.Snapshot
	info.Details = typed.Details
	if len(typed.RPCData) > 0 {
		var rpcData any
		if json.Unmarshal(typed.RPCData, &rpcData) == nil {
			info.RPCData = rpcData
		}
	}
	info.TechnicalDetail = typed.ServerDiag.TechnicalDetail
	info.FriendlyHint, info.ActionURL = apperrors.ServerGuidance(typed.ServerDiag)
	if typed.Cause != nil {
		info.Cause = typed.Cause.Error()
	}
	if typed.ServerDiag.ServerErrorCode != "" {
		info.UpstreamCode = typed.ServerDiag.ServerErrorCode
	}
	return info
}

func errorTypeForExitCode(code int) string {
	switch code {
	case 1:
		return "api"
	case 2:
		return "auth"
	case 3:
		return "validation"
	case 4:
		return "permission"
	case 6:
		return "discovery"
	default:
		return "internal"
	}
}

// newPreParseValidationError keeps pipeline handler identity in internal logs
// while exposing only the underlying parameter-domain error to CLI users.
func newPreParseValidationError(err error) error {
	if structured, ok := err.(*apperrors.Error); ok {
		return structured
	}
	userErr := err
	var handlerErr *pipeline.HandlerError
	if stderrors.As(err, &handlerErr) && handlerErr.Unwrap() != nil {
		userErr = handlerErr.Unwrap()
	}
	return apperrors.NewValidation(
		userErr.Error(),
		apperrors.WithReason("parameter_conflict"),
		apperrors.WithHint("Remove the duplicate alias/canonical spelling and pass the parameter exactly once."),
		apperrors.WithCause(userErr),
	)
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
	if flag, ok := unknownFlagName(errMsg); ok && flag == "from" {
		switch cmd.CommandPath() {
		case "dws chat +search-msg", "dws chat +chat-messages":
			return apperrors.NewValidation(
				msgWithTail,
				apperrors.WithHint("--from 在消息查询中含义不明确：按发送者过滤请使用 --sender <姓名|userId|openDingTalkId>；指定时间起点请使用 --start <RFC3339>"),
				apperrors.WithReason("ambiguous_flag"),
				apperrors.WithCause(err),
				apperrors.WithActions(
					"Use --sender <姓名|userId|openDingTalkId> to filter by sender",
					"Use --start <RFC3339> together with --end <RFC3339> to set a time range",
				),
				apperrors.WithAvailableFlags(cmdutil.VisibleFlagNames(cmd)...),
			)
		}
	}
	if flag, protection, ok := reviewedFlagProtection(cmd, errMsg); ok {
		hint := fmt.Sprintf("Parameter --%s is blocked from automatic normalization on %q; choose an explicit flag from --help.", flag, cmd.CommandPath())
		reason := "blocked_flag"
		if protection == pipeline.FlagProtectionAmbiguous {
			hint = fmt.Sprintf("Parameter --%s is ambiguous on %q and cannot be normalized safely; choose the intended explicit flag from --help.", flag, cmd.CommandPath())
			reason = "ambiguous_flag"
		}
		return apperrors.NewValidation(
			msgWithTail,
			apperrors.WithHint(hint),
			apperrors.WithReason(reason),
			apperrors.WithCause(err),
			apperrors.WithActions(fmt.Sprintf("Run '%s --help' for valid flags", cmd.CommandPath())),
			apperrors.WithAvailableFlags(cmdutil.VisibleFlagNames(cmd)...),
		)
	}

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

func reviewedFlagProtection(cmd *cobra.Command, errMsg string) (string, pipeline.FlagProtection, bool) {
	if cmd == nil {
		return "", "", false
	}
	flag, ok := unknownFlagName(errMsg)
	if !ok {
		return "", "", false
	}
	entry, ok := cli.LookupParamAlias(cmd.CommandPath())
	if !ok {
		return "", "", false
	}
	morphed := cmdutil.Morph(flag)
	if entry.IsBlocked(morphed) {
		return flag, pipeline.FlagProtectionBlocked, true
	}
	if entry.IsAmbiguous(morphed) {
		return flag, pipeline.FlagProtectionAmbiguous, true
	}
	return "", "", false
}

func unknownFlagName(errMsg string) (string, bool) {
	const prefix = "unknown flag: --"
	idx := strings.Index(errMsg, prefix)
	if idx < 0 {
		return "", false
	}
	flag := strings.TrimSpace(errMsg[idx+len(prefix):])
	if i := strings.IndexAny(flag, " =\n\t"); i >= 0 {
		flag = flag[:i]
	}
	return flag, flag != ""
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
	registerSchemaRuntimeDelivery()
	var rootCtx context.Context
	if len(ctx) > 0 && ctx[0] != nil {
		rootCtx = ctx[0]
	}
	rootCtx, _ = output.WithResultStore(rootCtx)
	return newRootCommandWithEngine(rootCtx, nil, true, false)
}

// NewSchemaSourceRootCommand constructs the distribution-owned command tree
// used as the Schema assembly source root (RegisterSchemaSourceRoot →
// ResolveSchemaBuild) and by command-surface policy. Installed plugins and
// user-defined shortcuts must not change the reviewed Schema surface.
// declarationOnly skips injectStaticServers / helpers.InitDeps so Schema
// assembly cannot clobber a live process's ToolCaller or plugin endpoints.
func NewSchemaSourceRootCommand(ctx ...context.Context) *cobra.Command {
	var rootCtx context.Context
	if len(ctx) > 0 && ctx[0] != nil {
		rootCtx = ctx[0]
	}
	return newRootCommandWithEngine(rootCtx, nil, false, true)
}

// NewRootCommandWithEngine constructs the root CLI command with an
// optional pipeline engine for input correction. When engine is nil,
// no pipeline processing is applied.
func NewRootCommandWithEngine(rootCtx context.Context, engine *pipeline.Engine) *cobra.Command {
	registerSchemaRuntimeDelivery()
	rootCtx, _ = output.WithResultStore(rootCtx)
	return newRootCommandWithEngine(rootCtx, engine, true, false)
}

func newRootCommandWithEngine(rootCtx context.Context, engine *pipeline.Engine, loadRuntimeExtensions bool, declarationOnly bool) *cobra.Command {
	return newRootCommandWithMode(rootCtx, engine, loadRuntimeExtensions, declarationOnly, false)
}

func newRootPresentationCommand() *cobra.Command {
	return newRootCommandWithMode(context.Background(), nil, false, true, true)
}

func consumeCredentialInvocationFlags(root *cobra.Command, flags *GlobalFlags, invocationSeen *bool) {
	if root == nil || flags == nil || invocationSeen == nil {
		return
	}

	clientIDFlag := root.PersistentFlags().Lookup("client-id")
	clientSecretFlag := root.PersistentFlags().Lookup("client-secret")
	clientIDSet := clientIDFlag != nil && clientIDFlag.Changed
	clientSecretSet := clientSecretFlag != nil && clientSecretFlag.Changed

	clientID := ""
	if clientIDSet {
		clientID = flags.ClientID
	}
	clientSecret := ""
	if clientSecretSet {
		clientSecret = flags.ClientSecret
	}

	// Keep GlobalFlags scoped to this execution. A flag omitted from the current
	// invocation must not retain the value pflag parsed for a previous one.
	flags.ClientID = clientID
	flags.ClientSecret = clientSecret
	if clientIDSet || clientSecretSet {
		authpkg.SetClientCredentials(clientID, clientSecret)
	} else if *invocationSeen {
		// Preserve a programmatically supplied runtime pair on the first execution,
		// but never carry credentials installed by an earlier execution of this root.
		authpkg.SetClientCredentials("", "")
	}
	*invocationSeen = true

	// Changed is execution state in this reusable command tree, not a lifetime
	// property. The next parse will set it again for flags actually supplied.
	if clientIDFlag != nil {
		clientIDFlag.Changed = false
	}
	if clientSecretFlag != nil {
		clientSecretFlag.Changed = false
	}
}

func discardCredentialInvocationFlags(root *cobra.Command, flags *GlobalFlags, invocationSeen bool) {
	if flags != nil {
		flags.ClientID = ""
		flags.ClientSecret = ""
	}
	if root != nil {
		if flag := root.PersistentFlags().Lookup("client-id"); flag != nil {
			flag.Changed = false
		}
		if flag := root.PersistentFlags().Lookup("client-secret"); flag != nil {
			flag.Changed = false
		}
	}
	if invocationSeen {
		authpkg.SetClientCredentials("", "")
	}
}

func consumeRootVersionInvocationFlag(root *cobra.Command, requested *bool) {
	if root == nil || requested == nil {
		return
	}
	flag := root.Flags().Lookup("version")
	*requested = flag != nil && flag.Changed && *requested
	if flag != nil {
		flag.Changed = false
	}
}

func discardRootVersionInvocationFlag(root *cobra.Command, requested *bool) {
	if requested != nil {
		*requested = false
	}
	if root != nil {
		if flag := root.Flags().Lookup("version"); flag != nil {
			flag.Changed = false
		}
	}
}

func installInvocationExitHandlers(root *cobra.Command, flags *GlobalFlags, credentialInvocationSeen *bool, versionRequested *bool) {
	if root == nil || credentialInvocationSeen == nil {
		return
	}
	cleanup := func() {
		discardCredentialInvocationFlags(root, flags, *credentialInvocationSeen)
		discardRootVersionInvocationFlag(root, versionRequested)
	}

	// Cobra handles --help before PersistentPreRunE. Wrap the inherited help
	// renderer once at the root so both flag-based and help-command paths clean
	// partially parsed invocation state after rendering.
	previousHelp := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		defer cleanup()
		previousHelp(cmd, args)
	})

	// Install leaf handlers before the root handler so inherited handlers are
	// captured without recursively wrapping an already wrapped parent.
	var visit func(*cobra.Command)
	visit = func(cmd *cobra.Command) {
		for _, child := range cmd.Commands() {
			visit(child)
		}

		if previousArgs := cmd.Args; previousArgs != nil {
			cmd.Args = func(current *cobra.Command, args []string) error {
				err := previousArgs(current, args)
				if err != nil {
					cleanup()
				}
				return err
			}
		}

		previousFlagError := cmd.FlagErrorFunc()
		cmd.SetFlagErrorFunc(func(current *cobra.Command, err error) error {
			cleanup()
			return previousFlagError(current, err)
		})
	}
	visit(root)
}

func newRootCommandWithMode(rootCtx context.Context, engine *pipeline.Engine, loadRuntimeExtensions bool, declarationOnly bool, presentationOnly bool) *cobra.Command {
	if rootCtx == nil {
		rootCtx = context.Background()
	}
	flags := &GlobalFlags{}
	authpkg.SetRuntimeProfile(preparseProfileFlag(os.Args[1:]))
	runner := rootNewCommandRunnerWithFlags(flags)
	if snapshot, ok := agentMetadataSnapshotFromContext(rootCtx); ok {
		if runtime, ok := runner.(*runtimeRunner); ok {
			runtime.agentMetadata = &snapshot
		}
	}

	credentialInvocationSeen := false
	rootVersionRequested := false
	rootVersionShortCircuit := false
	root := &cobra.Command{
		Use:               "dws",
		Short:             "DWS CLI",
		Long:              `提示: 如果遇到能力缺失、命令报错、新功能未注册、或无法完成任务, 请先用 'dws upgrade' 升级到最新版本后再试. 钉钉 OpenAPI 和 dws CLI 持续迭代, 新能力和 bugfix 会先在新版本上线.`,
		Args:              cobra.NoArgs,
		SilenceErrors:     true,
		SilenceUsage:      true,
		DisableAutoGenTag: true,
		// Cobra's built-in --version path returns before PersistentPreRunE and
		// cannot participate in reusable-root credential cleanup. Keep the same
		// public flag and output in RunE so it crosses the normal invocation
		// boundary instead.
		Version: "",
		RunE:    runRootHelp,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			rootVersionShortCircuit = false
			consumeRootVersionInvocationFlag(cmd.Root(), &rootVersionRequested)
			if rootVersionRequested && cmd == cmd.Root() {
				// Preserve Cobra's historical --version behavior: no metadata
				// validation, edition hook, output lifecycle, or credential mutation.
				// Only discard credential flags parsed for this non-operational call.
				discardCredentialInvocationFlags(cmd.Root(), flags, false)
				rootVersionShortCircuit = true
				return nil
			}

			// Cobra command trees can be reused by embedding callers. pflag keeps a
			// bound flag's value and Changed bit after ExecuteC returns, so consume
			// credential flags at the execution boundary before any validation or
			// hook can observe state left by a previous invocation.
			consumeCredentialInvocationFlags(cmd.Root(), flags, &credentialInvocationSeen)

			// A public root may be reused by embedding callers through multiple
			// ExecuteC invocations. Begin each invocation with an empty result
			// lifecycle while retaining the store pointer observed by Execute's
			// signal and exit-code handling. Declaration-only command trees do not
			// install a store at construction time, so add one lazily when those
			// trees are executed for compatibility and policy tests.
			executionCtx, _ := output.WithResultStore(cmd.Context())
			cmd.SetContext(executionCtx)
			// WithResultStore above guarantees the reset precondition.
			_ = output.ResetResultStore(executionCtx)
			// Do not run Cobra's ValidateRequiredFlags/ValidateFlagGroups here:
			// Cobra executes them between the leaf's PreRunE and RunE, and leaves
			// rely on that order to normalize alias flags into required canonical
			// flags (for example chat message download-media copies --msg-id into
			// the required --message-id in PreRunE). Running them early fails the
			// alias path before the leaf can normalize it. The transactional
			// --output sink instead opens at Run entry (after Cobra's own
			// validation), so validation failures still cannot strand a
			// temporary file.
			// Validate caller-provided identity and MCP metadata before command
			// execution hooks or network activity. The process entry point additionally
			// validates Agent metadata before command-tree construction; direct Cobra
			// embedding retains this execution-boundary guard.
			if _, err := parseAgentHost(os.Getenv(envDWSAgentHost)); err != nil {
				return err
			}
			if _, err := parseAgentProduct(os.Getenv(agentproduct.EnvName)); err != nil {
				return err
			}
			agentMetadata, cached := agentMetadataSnapshotFromContext(cmd.Context())
			if !cached {
				agentMetadata = readAgentMetadataSnapshot()
			}
			if err := agentMetadata.validationError(); err != nil {
				return err
			}
			if runtime, ok := runner.(*runtimeRunner); ok {
				// Retain the exact validated pair for this command execution so a
				// concurrently mutating embedding environment cannot change what is
				// later applied after edition and credential hooks.
				runtime.agentMetadata = &agentMetadata
			}
			if shouldDetectNestedSkillLayout(cmd) {
				if found, err := detectNestedMultiSkillLayout(); err == nil && found {
					fmt.Fprintln(cmd.ErrOrStderr(), "⚠️  检测到旧升级器留下的嵌套 Skill；请运行 dws skill setup --mode multi 查看迁移计划并确认")
				}
			}

			authpkg.SetRuntimeProfile(flags.Profile)

			// Configure global slog level based on --debug / --verbose flags.
			configureLogLevel(flags)

			installOutputSinkRunBoundary(cmd)
			if fn := edition.Get().AfterPersistentPreRun; fn != nil {
				if err := fn(cmd, args); err != nil {
					return err
				}
			}
			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) (err error) {
			if rootVersionShortCircuit {
				rootVersionShortCircuit = false
				rootVersionRequested = false
				return nil
			}
			defer func() {
				if r := recover(); r != nil {
					warnAbortOutputSink(cmd)
					panic(r)
				}
				if err != nil {
					warnAbortOutputSink(cmd)
				}
			}()
			_, emitted, emitErr := output.EmitStoredResult(cmd)
			StopAllStdioClients()
			CloseAuditSink()
			if emitErr != nil {
				return apperrors.NewInternal("emit command result: "+emitErr.Error(), apperrors.WithCause(emitErr))
			}
			if output.UsesUnifiedResult(cmd) && !emitted {
				return apperrors.NewInternal("framework 2.0 command returned without a CommandResult")
			}
			if closeErr := closeOutputSink(cmd); closeErr != nil {
				return closeErr
			}
			return nil
		},
	}
	root.Flags().BoolVar(&rootVersionRequested, "version", false, "version for dws")
	corecmd.ApplyGroupPolicy(root, corecmd.GroupPolicy{
		Mode:        corecmd.GroupNavigationOnly,
		Positionals: corecmd.PositionalsReject,
		Recovery:    corecmd.RecoverySibling,
	})
	rootArgs := root.Args
	root.Args = func(cmd *cobra.Command, args []string) error {
		// Cobra's built-in --version returns before Args validation. Preserve
		// that behavior only when the current parse explicitly selected the
		// lifecycle-aware replacement; a prior failed writer must not bypass Args.
		versionFlag := cmd.Flags().Lookup("version")
		if versionFlag != nil && versionFlag.Changed && rootVersionRequested {
			return nil
		}
		return rootArgs(cmd, args)
	}
	rootRunE := root.RunE
	root.RunE = func(cmd *cobra.Command, args []string) error {
		if rootVersionRequested {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s version %s\n", cmd.Name(), Version())
			return err
		}
		return rootRunE(cmd, args)
	}

	bindPersistentFlags(root, flags)

	schemaCmd := cli.NewSchemaCommand()
	mcpCmd := cli.NewMCPCommand()
	// Wrap the caller so every MCP tool call's shape is recorded to the local
	// usage log (privacy-preserving; see internal/shortcut/usage). Powers
	// `dws shortcut stats` and future high-frequency shortcut distillation.
	patCaller := newRecordingToolCaller(newToolCallerAdapter(runner, flags))
	mcpCmd.AddCommand(
		newMCPURLGroup(patCaller),
		newMCPPublishedGroup(patCaller, newAuthenticatedMCPPublishedTransportFactory(runner, flags)),
	)

	navigationGroup := func(command *cobra.Command) *cobra.Command {
		corecmd.ApplyGroupPolicy(command, corecmd.GroupPolicy{
			Mode:        corecmd.GroupNavigationOnly,
			Positionals: corecmd.PositionalsReject,
			Recovery:    corecmd.RecoverySibling,
		})
		return command
	}
	hybridGroup := func(command *cobra.Command) *cobra.Command {
		corecmd.ApplyGroupPolicy(command, corecmd.GroupPolicy{
			Mode:        corecmd.GroupHybrid,
			Positionals: corecmd.PositionalsReject,
			Recovery:    corecmd.RecoverySibling,
		})
		return command
	}

	utilityCommands := []*cobra.Command{
		navigationGroup(newAuthCommand(patCaller)),
		navigationGroup(newProfileCommand()),
		newAPICommand(flags),
		navigationGroup(newSkillCommand()),
		hybridGroup(newCacheCommand()),
		newCatalogCommand(),
		navigationGroup(newConfigCommand()),
		newDoctorCommand(),
		hybridGroup(newRecoveryCommand()),
		navigationGroup(newEventCommand(flags)),
		navigationGroup(newAuditCommand()),
		newCompletionCommand(root),
		newUpgradeCommand(),
		newVersionCommand(),
		newPluginCommand(),
		navigationGroup(usage.NewShortcutCommand()),
		schemaCmd,
		navigationGroup(mcpCmd),
	}
	root.AddCommand(utilityCommands...)

	if declarationOnly {
		// Schema / surface assembly: mount the reviewed tree only. Do not
		// injectStaticServers or InitDeps — those mutate process globals and
		// would clobber a live runtime's caller and plugin endpoints.
		root.AddCommand(mountLegacyPublicCommands(runner, loadRuntimeExtensions)...)
	} else {
		root.AddCommand(newLegacyPublicCommands(runner, patCaller, loadRuntimeExtensions)...)
	}

	// PAT authorization commands (open-source core)
	pat.RegisterCommands(root, patCaller)

	if !presentationOnly {
		if fn := edition.Get().RegisterExtraCommands; fn != nil {
			caller := newToolCallerAdapter(runner, flags)
			fn(root, caller)
			deduplicateCommands(root)
		}
	}
	if loadRuntimeExtensions {
		// Resolve plugins only after the complete distribution command tree is
		// present, so endpoint and Cobra conflict checks see PAT and edition
		// commands as well as the open-source base.
		pluginCmds := rootLoadPlugins(root, engine, runner)
		if len(pluginCmds) > 0 {
			addPluginCommandsSafe(root, pluginCmds)
		}
	}
	if !presentationOnly {
		hideNonDirectRuntimeCommands(root)
	}
	configureRootHelp(root)
	// Set custom flag error handler for better UX.
	root.SetFlagErrorFunc(flagErrorWithSuggestions)
	installReviewedFlagProtectionHandlers(root)
	installInvocationExitHandlers(root, flags, &credentialInvocationSeen, &rootVersionRequested)
	root.SetContext(rootCtx)

	return root
}

// installReviewedFlagProtectionHandlers makes reviewed blocked/ambiguous
// parameters authoritative even when an older command subtree has installed a
// local FlagErrorFunc. Commands without a reviewed guard keep their existing
// handler or inherit the root handler as before.
func installReviewedFlagProtectionHandlers(root *cobra.Command) {
	if root == nil {
		return
	}
	var visit func(*cobra.Command)
	visit = func(cmd *cobra.Command) {
		if entry, ok := cli.LookupParamAlias(cmd.CommandPath()); ok && (len(entry.Blocked) > 0 || len(entry.Ambiguous) > 0) {
			previous := cmd.FlagErrorFunc()
			cmd.SetFlagErrorFunc(func(current *cobra.Command, err error) error {
				if _, _, guarded := reviewedFlagProtection(current, err.Error()); guarded {
					return flagErrorWithSuggestions(current, err)
				}
				return previous(current, err)
			})
		}
		for _, child := range cmd.Commands() {
			visit(child)
		}
	}
	visit(root)
}

func preparseProfileFlag(args []string) string {
	profile, _, valid := preparseProfileSelection(args)
	if !valid {
		return ""
	}
	return profile
}

func preparseProfileSelection(args []string) (profile string, specified, valid bool) {
	args, _ = normalizeProfileFlagArgs(args)
	valid = true
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "--profile":
			specified = true
			if i+1 >= len(args) || strings.HasPrefix(strings.TrimSpace(args[i+1]), "-") {
				profile = ""
				valid = false
				continue
			}
			profile = strings.TrimSpace(args[i+1])
			valid = profile != ""
			i++
		case strings.HasPrefix(arg, "--profile="):
			specified = true
			profile = strings.TrimSpace(strings.TrimPrefix(arg, "--profile="))
			valid = profile != ""
		}
	}
	return profile, specified, valid
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

// hideNonDirectRuntimeCommands marks top-level product commands as hidden
// unless they correspond to a static endpoint product or an edition-visible
// compatibility command.
// Public utility commands are always kept visible; explicitly hidden commands
// stay hidden.
func hideNonDirectRuntimeCommands(root *cobra.Command) {
	allowedProducts := resolveVisibleProducts()
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

// builtinCommandNames is the shared base set of built-in command names. Both
// staticCommands (the visibility allow-list used by
// hideNonDirectRuntimeCommands) and reservedCommands (the plugin-override
// blocklist) derive from this single set so they cannot drift apart.
var builtinCommandNames = map[string]bool{
	"auth": true, "api": true, "audit": true, "cache": true, "config": true,
	"doctor": true, "event": true, "completion": true, "skill": true,
	"plugin": true, "profile": true, "recovery": true, "version": true, "help": true,
	"schema": true, "mcp": true, "upgrade": true,
}

// commandNameSet returns a new set containing every name in base plus extras.
func commandNameSet(base map[string]bool, extras ...string) map[string]bool {
	set := make(map[string]bool, len(base)+len(extras))
	for name := range base {
		set[name] = true
	}
	for _, extra := range extras {
		set[extra] = true
	}
	return set
}

// staticCommands is the set of built-in commands that stay visible even when
// they are not backed by a static endpoint product. Asymmetry with
// reservedCommands is intentional: dev/markdown stay visible but are not
// plugin-reserved, while login/logout are plugin-reserved but are not static
// top-level commands.
var staticCommands = commandNameSet(builtinCommandNames, "dev", "markdown")

// reservedCommands is the set of built-in command names that plugins must
// not override. This protects core CLI functionality from being hijacked
// by a malicious or misconfigured plugin.
var reservedCommands = commandNameSet(builtinCommandNames, "login", "logout")

var replaceablePluginFallbacks = map[string]bool{
	"conference": true,
}

// addPluginCommandsSafe registers plugin commands with conflict detection.
//
// Rules:
//   - Plugin vs reserved (auth/plugin/cache/...) → reject, warn
//   - Plugin vs plugin (same name)               → reject later one, warn
//   - Plugin vs hidden compatibility fallback     → allow, plugin wins
//   - Plugin vs visible distribution command      → reject, warn
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

		// An alias must not bypass the same protections applied to primary
		// plugin command names or shadow another root command.
		filteredAliases := make([]string, 0, len(cmd.Aliases))
		for _, rawAlias := range cmd.Aliases {
			alias := strings.TrimSpace(rawAlias)
			if alias == "" || alias == name || reservedCommands[alias] ||
				existing[alias] || pluginSeen[alias] {
				if alias != "" {
					slog.Warn("plugin: command alias conflicts with an existing command, skipping",
						"command", name, "alias", alias)
				}
				continue
			}
			pluginSeen[alias] = true
			filteredAliases = append(filteredAliases, alias)
		}
		cmd.Aliases = filteredAliases

		// Rule 3: an installed plugin may replace a hidden compatibility
		// fallback (for example conference), but never a visible distribution
		// command that participates in the reviewed base interface.
		if existing[name] {
			for _, old := range root.Commands() {
				if old.Name() == name {
					if !old.Hidden || !replaceablePluginFallbacks[name] ||
						cmdutil.IsPluginSourced(old) {
						slog.Warn("plugin: command conflicts with a visible distribution command, skipping",
							"command", name)
						cmd = nil
						break
					}
					root.RemoveCommand(old)
					slog.Debug("plugin: overriding hidden compatibility command",
						"command", name)
					break
				}
			}
		}
		if cmd == nil {
			continue
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

type outputSinkState struct {
	mu       sync.Mutex
	file     *os.File
	original io.Writer
	tempPath string
	target   string
	finished bool
}

type outputPublicationError struct {
	cause error
}

func (e *outputPublicationError) Error() string { return e.cause.Error() }
func (e *outputPublicationError) Unwrap() error { return e.cause }
func (e *outputPublicationError) ExitCode() int { return 5 }

func newOutputPublicationError(message string, cause error) error {
	return &outputPublicationError{cause: fmt.Errorf("%s: %w", message, cause)}
}

// emitOutputPublicationFailure replaces a result that was rendered only into a
// rolled-back transactional file with one observable failure envelope on the
// original output stream. This is not a second public result: closeOutputSink
// has removed the temporary file and restored cmd.OutOrStdout before returning
// the publication error.
func emitOutputPublicationFailure(cmd *cobra.Command, err error) (code int, handled bool, emitErr error) {
	var publicationErr *outputPublicationError
	if cmd == nil || !stderrors.As(err, &publicationErr) || !output.UsesUnifiedResult(cmd) {
		return 0, false, nil
	}
	state := outputSinkForCommand(cmd)
	if state == nil {
		return 0, false, nil
	}
	state.mu.Lock()
	original := state.original
	finished := state.finished
	state.mu.Unlock()
	if original == nil || !finished {
		return 0, false, nil
	}
	cmd.SetOut(original)
	result := output.FailureWithExitCode(errorInfoFromExecutionError(publicationErr), apperrors.ExitCode(publicationErr))
	code, emitErr = output.EmitResult(cmd, result)
	return code, true, emitErr
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
	// A public root may be reused across ExecuteC calls, accumulating one Run
	// wrapper per execution. When the sink for this invocation is already open,
	// an inner wrapper must not replace it with a second temporary file.
	if state := outputSinkForCommand(cmd); state != nil {
		state.mu.Lock()
		finished := state.finished
		state.mu.Unlock()
		if !finished {
			return nil
		}
	}
	if err := validateOptionalPath("--output", outputPath); err != nil {
		return err
	}
	if err := rootMkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return apperrors.NewInternal(fmt.Sprintf("failed to prepare output directory: %v", err))
	}
	tempPattern := "." + filepath.Base(outputPath) + ".tmp-*"
	file, err := rootCreateTemp(filepath.Dir(outputPath), tempPattern)
	if err != nil {
		return apperrors.NewInternal(fmt.Sprintf("failed to create temporary output file: %v", err))
	}
	originalOut := cmd.OutOrStdout()
	cmd.SetOut(file)
	cmd.SetContext(context.WithValue(cmd.Context(), outputFileContextKey{}, &outputSinkState{
		file:     file,
		original: originalOut,
		tempPath: file.Name(),
		target:   outputPath,
	}))
	return nil
}

// installOutputSinkRunBoundary defers opening the transactional --output sink
// to the executed command's Run entry. Cobra runs ValidateRequiredFlags and
// ValidateFlagGroups after the leaf's PreRunE and immediately before RunE, so
// opening the sink there keeps two invariants at once: leaf PreRunE hooks can
// still normalize alias flags into required canonical flags, and a validation
// failure can never strand a temporary output file. Run-only leaves are
// converted to RunE so a sink setup failure remains a returned error. Post-run
// hooks keep the error cleanup wrapping so a post-run failure still aborts the
// transaction; pre-run hooks need no wrapping because the sink cannot exist
// before Run entry.
func installOutputSinkRunBoundary(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	openSinkAndRun := func(run func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, args []string) error {
			if err := configureOutputSink(cmd); err != nil {
				return err
			}
			return runWithOutputSinkErrorCleanup(cmd, func() error { return run(cmd, args) })
		}
	}
	if cmd.RunE != nil {
		cmd.RunE = openSinkAndRun(cmd.RunE)
	} else if cmd.Run != nil {
		original := cmd.Run
		cmd.Run = nil
		cmd.RunE = openSinkAndRun(func(cmd *cobra.Command, args []string) error {
			original(cmd, args)
			return nil
		})
	}
	if cmd.PostRunE != nil {
		original := cmd.PostRunE
		cmd.PostRunE = func(cmd *cobra.Command, args []string) error {
			return runWithOutputSinkErrorCleanup(cmd, func() error { return original(cmd, args) })
		}
	}
	if cmd.PostRun != nil {
		original := cmd.PostRun
		cmd.PostRun = func(cmd *cobra.Command, args []string) {
			_ = runWithOutputSinkErrorCleanup(cmd, func() error {
				original(cmd, args)
				return nil
			})
		}
	}
}

func runWithOutputSinkErrorCleanup(cmd *cobra.Command, run func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			warnAbortOutputSink(cmd)
			panic(r)
		}
		if err != nil {
			warnAbortOutputSink(cmd)
		}
	}()
	return run()
}

func warnAbortOutputSink(cmd *cobra.Command) {
	if closeErr := abortOutputSink(cmd); closeErr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: close output sink: %v\n", closeErr)
	}
}

func closeOutputSink(cmd *cobra.Command) error {
	state := outputSinkForCommand(cmd)
	if state == nil {
		return nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	// A reusable Cobra tree must never retain the transactional file as its
	// stdout after this execution. Restore the caller's writer on every terminal
	// path, including sync/close/rename failures and repeated cleanup calls.
	if state.original != nil {
		cmd.SetOut(state.original)
	}
	if state.finished {
		return nil
	}
	state.finished = true
	if err := rootSyncFile(state.file); err != nil {
		_ = rootCloseFile(state.file)
		_ = rootRemoveFile(state.tempPath)
		return newOutputPublicationError("failed to sync output file", err)
	}
	if err := rootCloseFile(state.file); err != nil {
		_ = rootRemoveFile(state.tempPath)
		return newOutputPublicationError("failed to close output file", err)
	}
	if err := rootRenameFile(state.tempPath, state.target); err != nil {
		_ = rootRemoveFile(state.tempPath)
		return newOutputPublicationError("failed to publish output file", err)
	}
	return nil
}

func abortOutputSink(cmd *cobra.Command) error {
	state := outputSinkForCommand(cmd)
	if state == nil {
		return nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.finished {
		return nil
	}
	state.finished = true
	// A business error still needs the root execution boundary to publish one
	// typed failure envelope. Restore the pre-transaction writer before closing
	// and unlinking the temporary file so that failure emission cannot target a
	// closed descriptor. The final --output target remains untouched.
	if state.original != nil {
		cmd.SetOut(state.original)
	}
	closeErr := rootCloseFile(state.file)
	removeErr := rootRemoveFile(state.tempPath)
	if closeErr != nil {
		return apperrors.NewInternal(fmt.Sprintf("failed to close output file: %v", closeErr))
	}
	if removeErr != nil && !stderrors.Is(removeErr, os.ErrNotExist) {
		return apperrors.NewInternal(fmt.Sprintf("failed to remove temporary output file: %v", removeErr))
	}
	return nil
}

func outputSinkForCommand(cmd *cobra.Command) *outputSinkState {
	if cmd == nil || cmd.Context() == nil {
		return nil
	}
	state, _ := cmd.Context().Value(outputFileContextKey{}).(*outputSinkState)
	if state == nil || state.file == nil {
		return nil
	}
	return state
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
var (
	fileLoggerMu sync.Mutex
	fileLogger   *logging.FileLogger
)

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
	logger := logging.Setup(defaultConfigDir())
	fileHandler := slog.NewJSONHandler(logger.Writer(), &slog.HandlerOptions{Level: slog.LevelDebug})
	defaultLogger := slog.New(logging.NewMultiHandler(stderrHandler, fileHandler))

	fileLoggerMu.Lock()
	defer fileLoggerMu.Unlock()
	previous := fileLogger
	fileLogger = logger
	slog.SetDefault(defaultLogger)
	if previous != nil {
		_ = previous.Close()
	}
}

// FileLoggerInstance returns the package-level file logger, or nil if not initialized.
func FileLoggerInstance() *slog.Logger {
	fileLoggerMu.Lock()
	defer fileLoggerMu.Unlock()
	if fileLogger == nil {
		return nil
	}
	return fileLogger.Logger
}

// CloseFileLogger flushes and closes the file logger.
func CloseFileLogger() {
	fileLoggerMu.Lock()
	defer fileLoggerMu.Unlock()
	if fileLogger != nil {
		_ = fileLogger.Close()
		fileLogger = nil
	}
}

// loadPlugins registers versioned plugin manifests, stdio clients, hooks, and
// skills. It deliberately does not initialize MCP transports or call
// tools/list while constructing the command tree.
type pluginServerCandidate struct {
	owner       *plugin.Plugin
	order       int
	descriptor  mcptypes.ServerDescriptor
	stdioClient *plugin.StdioServerClient
}

type pluginIdentityOwner struct {
	plugin    *plugin.Plugin
	serverKey string
	rootName  string
	shareable bool
}

func loadPlugins(root *cobra.Command, engine *pipeline.Engine, runner executor.Runner) []*cobra.Command {
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
	sortPluginsForRegistration(userPlugins)
	sortPluginsForRegistration(devPlugins)

	allPlugins := append(userPlugins, devPlugins...)
	descriptorsByPlugin := make(map[*plugin.Plugin][]mcptypes.ServerDescriptor, len(allPlugins))

	// 3. Resolve every descriptor once, then choose identity winners before
	// mutating endpoint, auth, or stdio-client registries. This keeps the
	// visible command and its transport owned by the same plugin.
	candidates := collectPluginServerCandidates(allPlugins, userCtx)
	accepted := selectPluginServerCandidates(root, candidates)
	for _, candidate := range accepted {
		if candidate.stdioClient != nil {
			rootRegisterResolvedStdioServer(
				candidate.owner,
				*candidate.stdioClient,
				candidate.descriptor,
			)
		} else {
			rootRegisterPluginHTTPServer(candidate.descriptor)
		}
		descriptorsByPlugin[candidate.owner] = append(
			descriptorsByPlugin[candidate.owner],
			candidate.descriptor,
		)
	}

	// 4. Register plugin hooks into pipeline engine
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

	// 5. Sync plugin skills to agent directories
	rootPluginSyncSkills(allPlugins)

	if len(allPlugins) > 0 {
		slog.Debug("plugins loaded",
			"user", len(userPlugins),
			"dev", len(devPlugins),
		)
	}

	var pluginCommands []*cobra.Command
	for _, p := range allPlugins {
		// Build each plugin independently. addPluginCommandsSafe deliberately
		// resolves cross-plugin root conflicts with first-plugin-wins semantics.
		pluginCommands = append(pluginCommands, buildPluginCommands(descriptorsByPlugin[p], runner, root)...)
	}
	return pluginCommands
}

func sortPluginsForRegistration(plugins []*plugin.Plugin) {
	sort.SliceStable(plugins, func(i, j int) bool {
		left := strings.TrimSpace(plugins[i].Manifest.Name) + "\x00" + strings.TrimSpace(plugins[i].Root)
		right := strings.TrimSpace(plugins[j].Manifest.Name) + "\x00" + strings.TrimSpace(plugins[j].Root)
		return left < right
	})
}

func collectPluginServerCandidates(
	plugins []*plugin.Plugin,
	userCtx *plugin.UserContext,
) []pluginServerCandidate {
	var candidates []pluginServerCandidate
	for order, owner := range plugins {
		for _, descriptor := range rootPluginDescriptors(owner) {
			candidates = append(candidates, pluginServerCandidate{
				owner:      owner,
				order:      order,
				descriptor: descriptor,
			})
		}
		for _, stdioClient := range rootPluginStdioClients(owner, userCtx) {
			descriptor, ok := rootPluginStdioDescriptor(owner, stdioClient)
			if !ok {
				continue
			}
			clientCopy := stdioClient
			candidates = append(candidates, pluginServerCandidate{
				owner:       owner,
				order:       order,
				descriptor:  descriptor,
				stdioClient: &clientCopy,
			})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].order != candidates[j].order {
			return candidates[i].order < candidates[j].order
		}
		left := strings.TrimSpace(candidates[i].descriptor.Key)
		right := strings.TrimSpace(candidates[j].descriptor.Key)
		if left != right {
			return left < right
		}
		return candidates[i].stdioClient == nil && candidates[j].stdioClient != nil
	})
	return candidates
}

func selectPluginServerCandidates(
	root *cobra.Command,
	candidates []pluginServerCandidate,
) []pluginServerCandidate {
	distributionProducts := DirectRuntimeProductIDs()
	owners := make(map[string]pluginIdentityOwner)
	for identity := range distributionProducts {
		if replaceablePluginFallbacks[identity] {
			continue
		}
		owners[identity] = pluginIdentityOwner{serverKey: "distribution"}
	}

	accepted := make([]pluginServerCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		descriptor := candidate.descriptor
		if descriptor.CLI.Skip {
			continue
		}
		if reason := unsupportedPluginDescriptor(root, descriptor); reason != "" {
			slog.Warn("plugin: descriptor CLI semantics are unsupported, skipping",
				"plugin", candidate.owner.Manifest.Name,
				"server", descriptor.Key,
				"field", reason)
			continue
		}
		if pluginDescriptorConflictsWithDistribution(root, descriptor, distributionProducts) {
			continue
		}
		claims := pluginDescriptorIdentityClaims(descriptor)
		conflict := ""
		for identity, shareable := range claims {
			existing, exists := owners[identity]
			if !exists {
				continue
			}
			rootName := pluginDescriptorRootName(descriptor)
			if shareable && existing.shareable &&
				existing.plugin == candidate.owner &&
				existing.rootName == rootName {
				continue
			}
			conflict = identity
			break
		}
		if conflict != "" {
			slog.Warn("plugin: descriptor identity already owned, skipping",
				"plugin", candidate.owner.Manifest.Name,
				"server", descriptor.Key,
				"identity", conflict)
			continue
		}
		rootName := pluginDescriptorRootName(descriptor)
		for identity, shareable := range claims {
			if existing, exists := owners[identity]; exists &&
				shareable && existing.shareable &&
				existing.plugin == candidate.owner &&
				existing.rootName == rootName {
				continue
			}
			owners[identity] = pluginIdentityOwner{
				plugin:    candidate.owner,
				serverKey: descriptor.Key,
				rootName:  rootName,
				shareable: shareable,
			}
		}
		accepted = append(accepted, candidate)
	}
	return accepted
}

func pluginDescriptorIdentityClaims(descriptor mcptypes.ServerDescriptor) map[string]bool {
	claims := make(map[string]bool)
	canonicalID := firstNonEmptyPluginString(descriptor.CLI.ID, descriptor.Key)
	if canonicalID != "" {
		claims[canonicalID] = false
	}
	for _, identity := range append(
		[]string{pluginDescriptorRootName(descriptor)},
		descriptor.CLI.Aliases...,
	) {
		identity = strings.TrimSpace(identity)
		if identity == "" {
			continue
		}
		if _, exists := claims[identity]; !exists {
			claims[identity] = true
		}
	}
	return claims
}

func pluginDescriptorRootName(descriptor mcptypes.ServerDescriptor) string {
	return firstNonEmptyPluginString(
		descriptor.CLI.Command,
		descriptor.CLI.ID,
		descriptor.Key,
	)
}

func pluginDescriptorConflictsWithDistribution(
	root *cobra.Command,
	descriptor mcptypes.ServerDescriptor,
	distributionProducts map[string]bool,
) bool {
	candidates := append(
		[]string{
			firstNonEmptyPluginString(descriptor.CLI.ID, descriptor.Key),
			pluginDescriptorRootName(descriptor),
		},
		descriptor.CLI.Aliases...,
	)
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if !reservedCommands[candidate] && replaceablePluginFallbacks[candidate] {
			// The distribution ships only a hidden compatibility fallback for
			// this name; plugins may claim it and the later command merge in
			// addPluginCommandsSafe still rejects visible non-fallback owners.
			continue
		}
		if reservedCommands[candidate] ||
			distributionProducts[candidate] ||
			distributionRootOwns(root, candidate) {
			slog.Warn("plugin: descriptor conflicts with a distribution command, skipping",
				"plugin", descriptor.DisplayName,
				"server", descriptor.Key,
				"identity", candidate)
			return true
		}
	}
	return false
}

func distributionRootOwns(root *cobra.Command, name string) bool {
	if root == nil {
		return false
	}
	for _, command := range root.Commands() {
		if cmdutil.IsPluginSourced(command) {
			continue
		}
		if command.Name() == name {
			if command.Hidden && replaceablePluginFallbacks[name] {
				return false
			}
			return true
		}
		for _, alias := range command.Aliases {
			if strings.TrimSpace(alias) == name {
				return true
			}
		}
	}
	return false
}

func registerPluginHTTPServer(srv mcptypes.ServerDescriptor) {
	AppendDynamicServer(srv)
	productID := firstNonEmptyPluginString(srv.CLI.ID, srv.Key)
	// Register ownership for every accepted HTTP plugin, including anonymous
	// plugins. Execution must never fall back to the built-in DingTalk OAuth or
	// Agent-metadata path merely because a plugin has no Authorization Header.
	RegisterPluginAuth(productID, pluginAuthFromServerDescriptor(srv))
}

// pluginAuthFromServerDescriptor extracts plugin-owned credentials and custom
// Headers. A non-nil result also acts as the HTTP plugin ownership marker for
// anonymous plugins.
func pluginAuthFromServerDescriptor(srv mcptypes.ServerDescriptor) *PluginAuth {
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
	var trustedDomains []string
	if parsed, err := url.Parse(srv.Endpoint); err == nil {
		host := parsed.Hostname()
		if host != "" {
			trustedDomains = []string{host, "*." + host}
		}
	}
	return &PluginAuth{
		Token:          authToken,
		ExtraHeaders:   extraHeaders,
		TrustedDomains: trustedDomains,
	}
}

// newPipelineEngine creates and configures the pipeline engine with
// handlers for all five pipeline phases. The phases execute in order:
// Register → PreParse → PostParse → PreRequest → PostResponse.
//
// Phases are invoked at their respective integration points:
//   - Register:     during command tree construction (cli.NewMCPCommand)
//   - PreParse:     before Cobra parses raw argv (RunPreParse)
//   - PostParse:    after Cobra parsing, before validation (canonical RunE)
//   - PreRequest:   after validation, before JSON-RPC dispatch (canonical RunE)
//   - PostResponse: after transport returns, before stdout (canonical RunE)
func newPipelineEngine() *pipeline.Engine {
	engine := pipeline.NewEngine()
	engine.SetCommandPathFallbackLookup(func(path string) (pipeline.CommandPathFallback, bool) {
		entry, ok := cli.LookupCommandPathFallback(path)
		if !ok {
			return pipeline.CommandPathFallback{}, false
		}
		return pipeline.CommandPathFallback{
			From:       entry.From,
			Mode:       string(entry.Mode),
			To:         entry.To,
			Candidates: append([]string(nil), entry.Candidates...),
		}, true
	})
	engine.RegisterAll(
		// Register handler runs during command tree building.
		handlers.RegisterHandler{},

		// PreParse handlers run in order: alias → semantic → sticky → paramname
		// → boolvalue.
		// Alias normalises case first (--userId → --user-id), then semantic
		// resolves reviewed synonyms to the real flag (--keyword → --query),
		// then sticky splits glued values (--limit100 → --limit 100), then
		// paramname fixes near-miss typos (--limt → --limit). Boolvalue runs
		// last so detached values for every real boolean flag (for example
		// `--dry-run false`) become explicit `--flag=false` tokens before pflag
		// can interpret the bare flag as true.
		handlers.AliasHandler{},
		handlers.SemanticAliasHandler{
			// Inject the build-time reduced alias table with native types so
			// the handler package stays decoupled from cli.
			Lookup: func(rawCommandPath string) (map[string]string, []string, []string, bool) {
				e, ok := cli.LookupParamAlias(rawCommandPath)
				return e.Aliases, e.Blocked, e.Ambiguous, ok
			},
		},
		handlers.StickyHandler{},
		handlers.ParamNameHandler{},
		handlers.BoolValueHandler{},

		// PostParse handlers normalise structured values.
		handlers.ParamValueHandler{},

		// PreRequest handler inspects the validated payload before dispatch.
		handlers.PreRequestHandler{},

		// PostResponse handler processes the response before output.
		handlers.PostResponseHandler{},
	)
	return engine
}

func runRootHelp(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}
