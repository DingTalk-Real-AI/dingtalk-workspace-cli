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
	"sort"
	"strconv"
	"strings"
	"sync"
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
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/agentproduct"
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
	rootPluginStdioDescriptor       = stdioServerDescriptorFromManifest
	rootRegisterResolvedStdioServer = registerResolvedStdioServer
	rootPluginLoadHooks             = (*plugin.Plugin).LoadHooks
	rootPluginSyncSkills            = plugin.SyncSkills
	rootAuthLoadTokenData           = authpkg.LoadTokenData
	rootNewCommandRunnerWithFlags   = newCommandRunnerWithFlags
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

	if err := validateChatWorkbookRawArgs(os.Args[1:]); err != nil {
		if rawArgsRequestJSON(os.Args[1:]) {
			_ = apperrors.PrintJSON(os.Stderr, err)
		} else {
			_ = apperrors.PrintHumanAt(os.Stderr, err, resolveVerbosity(root))
		}
		return apperrors.ExitCode(err)
	}
	suppressJSONDeprecationPreamble(root, os.Args[1:])

	// Run PreParse handlers on raw argv before Cobra parses flags.
	// This corrects model-generated errors like --userId → --user-id
	// and --limit100 → --limit 100.
	if err := rootRunPreParse(root, engine); err != nil {
		err = newPreParseValidationError(err)
		_ = printExecutionError(root, os.Stdout, os.Stderr, err)
		return apperrors.ExitCode(err)
	}

	executed, err := rootExecuteCommand(root)
	if err != nil {
		if executed == nil {
			executed = root
		}
		err = rewordRequiredFlagError(err)
		err = enrichChatWorkbookError(executed, err)
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

func suppressJSONDeprecationPreamble(root *cobra.Command, args []string) {
	if root == nil || !rawArgsRequestJSON(args) || len(args) < 3 {
		return
	}
	if args[0] != "chat" || args[1] != "media" || args[2] != "upload" {
		return
	}
	if cmd, _, err := root.Find([]string{"chat", "media", "upload"}); err == nil && cmd != nil {
		cmd.Deprecated = ""
	}
}

func validateChatWorkbookRawArgs(args []string) error {
	path := strings.Join(args, " ")
	switch {
	case strings.HasPrefix(path, "chat message send ") && rawArgsFlagValue(args, "msg-type") == "file" &&
		rawArgsContainFlag(args, "media-id"):
		return apperrors.NewValidation(
			"文件消息不能使用 --media-id",
			apperrors.WithReason("PDF、DOCX、XLSX 和本地图片等文件通过 --file-path 上传发送；mediaId 仅用于已有媒体标识的 image 消息"),
			apperrors.WithActions("移除 --media-id", "补充 --file-path 并保留 --msg-type file"),
			apperrors.WithExamples(`dws chat message send --group <openConversationId> --msg-type file --file-path ./report.pdf --format json`),
		)
	case strings.HasPrefix(path, "chat message send ") && rawArgsContainFlag(args, "media-id") &&
		rawArgsFlagValue(args, "msg-type") == "":
		return apperrors.NewValidation(
			"检测到 --media-id，但没有指定媒体消息类型",
			apperrors.WithReason("未指定 --msg-type 时命令会进入文本分支，可能把文件名当成普通文字发送"),
			apperrors.WithActions("已有图片 mediaId 时补充 --msg-type image", "发送 PDF/DOCX/XLSX 时移除 --media-id，改用 --msg-type file --file-path"),
			apperrors.WithExamples(`dws chat message send --group <openConversationId> --msg-type image --media-id <mediaId> --format json`, `dws chat message send --group <openConversationId> --msg-type file --file-path ./thesis.pdf --format json`),
		)
	case strings.HasPrefix(path, "chat message send ") && rawArgsFlagValue(args, "msg-type") == "image" &&
		rawArgsContainFlag(args, "file-path") && !rawArgsContainFlag(args, "media-id"):
		filePath := rawArgsFlagValue(args, "file-path")
		return apperrors.NewValidation(
			"image 消息不能直接使用 --file-path",
			apperrors.WithReason("msg-type=image 只接受已有 mediaId；本地图片路径不能自动转换为 mediaId"),
			apperrors.WithActions("发送本地图片时改用 --msg-type file", "保留原路径并通过 --file-path 发送为文件附件"),
			apperrors.WithExamples(fmt.Sprintf(`dws chat message send --group <openConversationId> --msg-type file --file-path %q --format json`, filePath)),
		)
	case strings.HasPrefix(path, "chat message send ") && rawArgsFlagValue(args, "msg-type") == "image" &&
		!rawArgsContainFlag(args, "media-id"):
		return apperrors.NewValidation(
			"图片消息缺少 --media-id",
			apperrors.WithReason("msg-type=image 只接受上游已经获得的有效 mediaId，不能把本地文件名当作 mediaId"),
			apperrors.WithActions("已有 mediaId 时补充 --media-id", "发送本地图片时改用 --msg-type file --file-path"),
			apperrors.WithExamples(`dws chat message send --group <openConversationId> --msg-type image --media-id <mediaId> --format json`, `dws chat message send --group <openConversationId> --msg-type file --file-path ./image.png --format json`),
		)
	case strings.HasPrefix(path, "chat message send "):
		msgType := rawArgsFlagValue(args, "msg-type")
		switch msgType {
		case "", "text", "markdown", "image", "file", "audio", "video", "location", "profile":
		default:
			return apperrors.NewValidation(
				"不支持指定的 --msg-type："+msgType,
				apperrors.WithReason("当前命令不支持 sticker/card 等消息类型；文本或 Markdown 消息无需传 --msg-type"),
				apperrors.WithActions("文本消息移除 --msg-type 并使用 --text", "媒体消息使用 image、file、audio、video、location 或 profile"),
				apperrors.WithExamples(`dws chat message send --group <openConversationId> --text "hi" --format json`),
			)
		}
	case strings.HasPrefix(path, "chat group get-by-group-id "):
		value := rawArgsFlagValue(args, "group-id")
		if value != "" {
			if _, err := strconv.ParseInt(value, 10, 64); err != nil {
				return apperrors.NewValidation(
					"--group-id 必须是数字群号",
					apperrors.WithReason("cid 开头的值是 openConversationId，不是 get-by-group-id 所需的数字群号"),
					apperrors.WithActions("如果已有 openConversationId，请改用接受 --group 的群查询命令", "只有拿到数字群号时才调用 get-by-group-id"),
					apperrors.WithExamples(`dws chat group get-by-group-id --group-id 12345678 --format json`),
				)
			}
		}
	case strings.HasPrefix(path, "chat group dismiss ") && rawArgsContainFlag(args, "group"):
		value := rawArgsFlagValue(args, "group")
		if _, err := strconv.ParseInt(value, 10, 64); err == nil {
			return apperrors.NewValidation(
				"解散群命令需要 openConversationId，不是数字群号",
				apperrors.WithReason("--group 应传 cid 开头或服务端返回的 openConversationId；数字群号只用于 get-by-group-id"),
				apperrors.WithActions("先通过 chat search 获取 openConversationId", "确认目标群及不可逆影响后再执行解散"),
				apperrors.WithExamples(`dws chat group dismiss --group <openConversationId> --format json`),
			)
		}
	case strings.HasPrefix(path, "chat group members ") && rawArgsContainFlag(args, "group"):
		return apperrors.NewValidation(
			"群成员列表命令路径或群参数不正确",
			apperrors.WithReason("群成员列表的可执行命令是 chat group members，群 ID 参数名为 --id；不存在 members list --group 这一组合"),
			apperrors.WithActions("移除多余的 list 子命令", "将 --group 改为 --id"),
			apperrors.WithExamples(`dws chat group members --id <openConversationId> --format json`),
		)
	case strings.HasPrefix(path, "chat group rename ") && rawArgsContainFlag(args, "group"):
		return apperrors.NewValidation(
			"群重命名命令不支持 --group",
			apperrors.WithReason("chat group rename 使用 --id 接收群 openConversationId，而不是 --group"),
			apperrors.WithActions("将 --group 改为 --id", "群 ID 不确定时先用 chat search 查询"),
			apperrors.WithExamples(`dws chat group rename --id <openConversationId> --name "新群名" --format json`),
		)
	}
	return nil
}

func rawArgsContainFlag(args []string, name string) bool {
	prefix := "--" + name
	for _, arg := range args {
		if arg == prefix || strings.HasPrefix(arg, prefix+"=") {
			return true
		}
	}
	return false
}

func rawArgsFlagValue(args []string, name string) string {
	prefix := "--" + name
	for i, arg := range args {
		if strings.HasPrefix(arg, prefix+"=") {
			return strings.TrimPrefix(arg, prefix+"=")
		}
		if arg == prefix && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func rawArgsRequestJSON(args []string) bool {
	for i, arg := range args {
		if arg == "--format=json" || arg == "-f=json" {
			return true
		}
		if (arg == "--format" || arg == "-f") && i+1 < len(args) && strings.EqualFold(args[i+1], "json") {
			return true
		}
	}
	return false
}

type chatWorkbookGuidance struct {
	message  string
	reason   string
	actions  []string
	examples []string
}

var chatRequiredGuidance = map[string]chatWorkbookGuidance{
	"chat message send-by-webhook": {
		"Webhook 发送参数不完整",
		"Webhook 消息必须同时提供机器人地址中的 access_token、标题和正文；不能降级为普通群消息",
		[]string{"从自定义机器人 Webhook 地址提取 token", "同时补齐 --title 和 --text，并确保包含机器人安全关键词"},
		[]string{`dws chat message send-by-webhook --token <access_token> --title "dws测试通知" --text "dws测试：评测结果已出" --format json`},
	},
	"chat group rename": {
		"群重命名缺少群 ID 或新名称", "--id 必须是群 openConversationId，--name 是新的群名称",
		[]string{"先用 chat search 获取群 openConversationId", "同时提供 --id 和 --name"},
		[]string{`dws chat group rename --id <openConversationId> --name "新群名" --format json`},
	},
	"chat group dismiss": {
		"解散群缺少目标群 ID", "解散群不可逆且需要群主权限，--group 必须是 openConversationId",
		[]string{"先确认目标群和影响范围", "获取 openConversationId 后再执行，并按运行时要求确认"},
		[]string{`dws chat group dismiss --group <openConversationId> --format json`},
	},
	"chat group quit": {
		"退出群缺少目标群 ID", "quit 表示当前用户退出群聊，不会解散整个群；--group 必须是 openConversationId",
		[]string{"确认你要退出而不是解散群", "先获取目标群 openConversationId"},
		[]string{`dws chat group quit --group <openConversationId> --format json`},
	},
	"chat group set-admin": {
		"设置群管理员参数不完整", "需要目标群以及一个或多个成员；默认设为管理员，--off 表示取消管理员",
		[]string{"补充 --group", "通过 --user 或 --users 指定成员，取消管理员时增加 --off"},
		[]string{`dws chat group set-admin --group <openConversationId> --users <userId1>,<userId2> --format json`},
	},
	"chat group transfer-owner": {
		"转让群主参数不完整", "--group 指定群，--new-owner 使用 openDingTalkId，--user 使用 userId",
		[]string{"补充群 openConversationId", "在 --new-owner 和 --user 中选择一个新群主标识"},
		[]string{`dws chat group transfer-owner --group <openConversationId> --new-owner <openDingTalkId> --format json`},
	},
	"chat group update-nick": {
		"修改本人群昵称参数不完整", "update-nick 只修改当前登录用户在指定群里的昵称，需要群 ID 和新昵称",
		[]string{"补充 --group openConversationId", "补充新的昵称参数"},
		[]string{`dws chat group update-nick --group <openConversationId> --nick "新昵称" --format json`},
	},
	"chat group update-icon": {
		"更新群头像参数不完整", "需要群 openConversationId 和上游已经获得的有效图片 mediaId",
		[]string{"补充 --group", "从上游媒体能力获取 mediaId 后传入 --icon-media-id"},
		[]string{`dws chat group update-icon --group <openConversationId> --icon-media-id <mediaId> --format json`},
	},
	"chat group share-invite": {
		"分享群邀请参数不完整", "--source 是被分享群，--target 是接收分享的会话，--receiver 是接收分享的单聊用户",
		[]string{"补充 --source", "在 --target 和 --receiver 中选择一个接收目标"},
		[]string{`dws chat group share-invite --source <源群ID> --target <目标会话ID> --format json`},
	},
	"chat message reply": {
		"引用回复参数不完整", "会话 ID、原消息 ID、原发送者和回复正文必须来自或对应同一条原消息",
		[]string{"先拉取目标消息", "补齐 conversation-id、ref-msg-id、ref-sender 和 text"},
		[]string{`dws chat message reply --conversation-id <openConversationId> --ref-msg-id <openMessageId> --ref-sender <openDingTalkId> --text "收到" --format json`},
	},
	"chat message forward": {
		"转发消息参数不完整", "消息 ID 必须属于源会话，并需要明确源会话和目标会话",
		[]string{"先从源会话拉取真实消息 ID", "确认 src 和 dest 没有写反"},
		[]string{`dws chat message forward --src-conversation-id <源会话ID> --msg-id <openMessageId> --dest-conversation-id <目标会话ID> --format json`},
	},
	"chat message recall": {
		"撤回消息参数不完整", "用户消息撤回需要会话 ID 和本人发送的消息 ID；机器人消息应使用 recall-by-bot",
		[]string{"确认消息由当前用户发送", "补齐 conversation-id 和 msg-id"},
		[]string{`dws chat message recall --conversation-id <openConversationId> --msg-id <openMessageId> --format json`},
	},
	"chat message read-status": {
		"查询消息已读状态参数不完整", "只能查询当前用户发出消息的已读状态，需要会话和消息标识",
		[]string{"补齐会话和消息 ID", "人员筛选时区分 userId 与 openDingTalkId"},
		[]string{`dws chat message read-status --conversation-id <openConversationId> --message-id <openMessageId> --format json`},
	},
	"chat message list-by-ids": {
		"缺少消息 ID 列表", "--msg-ids 使用逗号分隔的真实 openMessageId，单次最多 50 条",
		[]string{"先拉取真实消息 ID", "将不超过 50 条 ID 用逗号连接"},
		[]string{`dws chat message list-by-ids --msg-ids <id1>,<id2> --format json`},
	},
	"chat message download-media": {
		"媒体下载参数不完整", "type、resource-id、message-id、open-conversation-id 和 output 必须完整，且资源与消息来自同一条消息",
		[]string{"先拉取目标媒体消息", "从同一条消息取得资源、消息和会话标识"},
		[]string{`dws chat message download-media --type mediaId --resource-id <mediaId> --message-id <openMessageId> --open-conversation-id <openConversationId> --output ./downloads/ --format json`},
	},
	"chat message add-emoji": {
		"添加表情回应参数不完整", "需要真实会话 ID、消息 ID 和 emoji 名称",
		[]string{"先拉取目标消息", "补齐 conversation-id、msg-id 和 emoji"},
		[]string{`dws chat message add-emoji --conversation-id <openConversationId> --msg-id <openMessageId> --emoji "赞" --format json`},
	},
	"chat message remove-emoji": {
		"移除表情回应参数不完整", "只能移除当前用户已添加的同名回应，需要会话、消息和 emoji 名称完全匹配",
		[]string{"确认当前用户添加过该回应", "补齐 conversation-id、msg-id 和 emoji"},
		[]string{`dws chat message remove-emoji --conversation-id <openConversationId> --msg-id <openMessageId> --emoji "赞" --format json`},
	},
	"chat message list-by-sender": {
		"按发送者查询参数不完整", "必须提供开始时间以及发送者 userId/openDingTalkId 二选一，可选 end 和 cursor",
		[]string{"补充 --start", "在 sender-user-id 和 sender-open-dingtalk-id 中选择一个"},
		[]string{`dws chat message list-by-sender --sender-user-id <userId> --start "2026-07-14T00:00:00+08:00" --format json`},
	},
	"chat message query-send-status": {
		"缺少发送任务 ID", "--open-task-id 来自 message send 返回的 openTaskId，不是消息 ID",
		[]string{"先执行 message send", "从发送结果读取 openTaskId"},
		[]string{`dws chat message query-send-status --open-task-id <openTaskId> --format json`},
	},
}

func enrichChatWorkbookError(cmd *cobra.Command, err error) error {
	if cmd == nil || err == nil {
		return err
	}
	path := cmd.CommandPath()
	if fields := strings.Fields(path); len(fields) > 1 {
		path = strings.Join(fields[1:], " ")
	}
	message := err.Error()
	var guide chatWorkbookGuidance
	switch {
	case path == "chat message send" &&
		(strings.Contains(message, "unknown flag: --at-user-ids") ||
			strings.Contains(message, "unknown flag: --at-users") ||
			strings.Contains(message, "unknown flag: --mention")):
		guide = chatWorkbookGuidance{
			"群消息 @成员参数不正确",
			"当前用户身份发送群消息时使用 --at-open-dingtalk-ids，参数值必须是成员的 openDingTalkId；--at-user-ids、--at-users、--mention 均不是有效参数",
			[]string{"先查询目标成员的 openDingTalkId", "改用 --at-open-dingtalk-ids，并在正文中写入 <@openDingTalkId>"},
			[]string{`dws chat message send --group <openConversationId> --at-open-dingtalk-ids <openDingTalkId> --text "<@openDingTalkId> 请关注" --format json`},
		}
	case path == "chat media upload":
		guide = chatWorkbookGuidance{
			"chat media upload 已下线",
			"当前 CLI 不再通过该命令把本地文件转换为 mediaId，本地图片和文件统一由 message send 的 file 路径上传并发送",
			[]string{"发送本地图片或文件时使用 --msg-type file --file-path", "只有上游已提供 mediaId 时才使用 --msg-type image --media-id"},
			[]string{`dws chat message send --group <openConversationId> --msg-type file --file-path ./image.png --format json`},
		}
	case path == "chat group members" && strings.Contains(message, "unknown flag: --group"):
		guide = chatWorkbookGuidance{
			"群成员列表命令路径或群参数不正确",
			"群成员列表的可执行命令是 chat group members，群 ID 参数名为 --id；不存在 members list --group 这一组合",
			[]string{"移除多余的 list 子命令", "将 --group 改为 --id"},
			[]string{`dws chat group members --id <openConversationId> --format json`},
		}
	case path == "chat group rename" && strings.Contains(message, "unknown flag: --group"):
		guide = chatWorkbookGuidance{
			"群重命名命令不支持 --group",
			"chat group rename 使用 --id 接收群 openConversationId，而不是 --group",
			[]string{"将 --group 改为 --id", "群 ID 不确定时先用 chat search 查询"},
			[]string{`dws chat group rename --id <openConversationId> --name "新群名" --format json`},
		}
	case path == "chat group create" && strings.Contains(message, "unknown flag: --members"):
		guide = chatWorkbookGuidance{
			"建群命令不支持 --members",
			"chat group create 使用 --users 接收逗号分隔的成员 userId；--members 是其他命令的参数名",
			[]string{"将 --members 改为 --users", "成员标识不确定时先查询 userId"},
			[]string{`dws chat group create --name "V2评审小组" --users 489149,550582 --format json`},
		}
	case path == "chat group bots" && strings.Contains(message, "unknown flag: --id"):
		guide = chatWorkbookGuidance{
			"群机器人列表命令不支持 --id",
			"chat group bots 使用 --group 接收群 openConversationId；该参数名与 members、rename 命令不同",
			[]string{"将 --id 改为 --group", "群 ID 不确定时先用 chat search 查询"},
			[]string{`dws chat group bots --group <openConversationId> --format json`},
		}
	case path == "chat message list-mentions" && strings.Contains(message, "required flag"):
		guide = chatWorkbookGuidance{
			"缺少必填参数：--start、--end",
			"查询 @我 消息必须同时提供 ISO-8601 格式的开始和结束时间；只提供分页参数不能确定查询范围",
			[]string{"同时补充 --start 和 --end，不要逐个参数反复试错", "按本地时区设置明确的查询时间窗"},
			[]string{`dws chat message list-mentions --start "2026-07-23T00:00:00+08:00" --end "2026-07-30T23:59:59+08:00" --limit 50 --format json`},
		}
	case path == "chat message send" && strings.Contains(message, "--group, --user or --open-dingtalk-id is required"):
		guide = chatWorkbookGuidance{
			"缺少消息接收目标",
			"发送消息必须在 --group、--user、--open-dingtalk-id 中选择且只选择一个接收目标",
			[]string{"发群消息时先查询并传入群 openConversationId", "发单聊时先查询并传入 userId 或 openDingTalkId"},
			[]string{`dws chat message send --group <openConversationId> --text "评测消息" --format json`, `dws chat message send --open-dingtalk-id <openDingTalkId> --text "评测消息" --format json`},
		}
	case path == "chat search" && strings.Contains(message, "query"):
		guide = chatWorkbookGuidance{
			"缺少群聊搜索关键词：--query",
			"群聊搜索需要关键词才能定位候选群，不能使用空查询",
			[]string{"使用 --query 传入群名称或名称片段", "从结果中读取 openConversationId 供后续群命令使用"},
			[]string{`dws chat search --query "项目群" --format json`},
		}
	case path == "chat message search-advanced":
		guide = chatWorkbookGuidance{
			"高级消息搜索至少需要一个搜索条件",
			"空条件搜索无法限定目标消息，必须提供关键词、人员、@我状态或会话范围中的至少一种",
			[]string{"按内容搜索时传入 --query", "也可通过 --user、--at-me 或 --conversation-ids 缩小范围"},
			[]string{`dws chat message search-advanced --query "评审" --format json`},
		}
	case path == "chat message search" && strings.Contains(message, "required flag"):
		guide = chatWorkbookGuidance{
			"关键词消息搜索缺少完整查询条件",
			"关键词消息搜索需要 --query、--start 和 --end；当前命令没有提供完整的关键词和时间范围",
			[]string{"补充搜索关键词", "同时提供 ISO-8601 格式的开始和结束时间"},
			[]string{`dws chat message search --query "评审" --start "2026-07-23T00:00:00+08:00" --end "2026-07-30T23:59:59+08:00" --format json`},
		}
	case path == "chat message list-all" && strings.Contains(message, "required flag"):
		guide = chatWorkbookGuidance{
			"跨会话消息查询缺少时间范围",
			"拉取全部会话消息必须使用 --start 和 --end 限定范围，避免无边界查询历史消息",
			[]string{"同时补充 --start 和 --end", "结果存在 hasMore 时使用 nextCursor 继续翻页"},
			[]string{`dws chat message list-all --start "2026-07-23T00:00:00+08:00" --end "2026-07-30T23:59:59+08:00" --limit 50 --format json`},
		}
	case path == "chat message list-topic-replies" && strings.Contains(message, "topic-id"):
		guide = chatWorkbookGuidance{
			"缺少话题定位参数：--topic-id",
			"topic-id 不能臆造，必须来自同一群聊消息列表中目标话题消息的 openConvThreadId",
			[]string{"先执行 chat message list 拉取目标群消息", "从目标话题消息读取 openConvThreadId 并作为 --topic-id"},
			[]string{`dws chat message list --group <openConversationId> --time "2026-07-30 23:59:59" --direction older --format json`, `dws chat message list-topic-replies --group <openConversationId> --topic-id <openConvThreadId> --limit 50 --format json`},
		}
	case path == "chat message send-by-bot" && strings.Contains(message, "required flag"):
		guide = chatWorkbookGuidance{
			"机器人发送消息缺少必填参数",
			"机器人发送需要 robotCode、标题、正文以及群聊或单聊目标，当前参数不完整",
			[]string{"补充 --robot-code 和 --title", "通过 --group 或用户参数指定接收目标"},
			[]string{`dws chat message send-by-bot --robot-code <robotCode> --group <openConversationId> --title "通知" --text "hello" --format json`},
		}
	case path == "chat message recall-by-bot" && strings.Contains(message, "required flag"):
		guide = chatWorkbookGuidance{
			"机器人撤回消息缺少 robotCode 或 processQueryKey",
			"--keys 的 processQueryKey 来自机器人发送消息的返回结果，不能凭空构造",
			[]string{"补充发送该消息的 --robot-code", "从发送结果读取 processQueryKey 并传给 --keys"},
			[]string{`dws chat message recall-by-bot --robot-code <robotCode> --group <openConversationId> --keys <processQueryKey> --format json`},
		}
	case path == "chat message list" && strings.Contains(message, "required flag") && strings.Contains(message, "--time"):
		guide = chatWorkbookGuidance{
			"拉取会话消息缺少时间锚点：--time",
			"消息列表按时间向前或向后拉取，必须提供一个明确的时间锚点",
			[]string{"补充格式为 YYYY-MM-DD HH:mm:ss 的 --time", "使用 --direction older 或 newer 明确查询方向"},
			[]string{`dws chat message list --group <openConversationId> --time "2026-07-30 10:00:00" --direction older --format json`},
		}
	case path == "chat message list" && strings.Contains(message, "--group, --user or --open-dingtalk-id is required"):
		guide = chatWorkbookGuidance{
			"拉取消息时缺少会话目标",
			"必须在群聊 openConversationId、单聊 userId、单聊 openDingTalkId 中选择且只选择一个目标",
			[]string{"群聊先用 chat search 获取 openConversationId", "单聊先查询人员标识，再传 --user 或 --open-dingtalk-id"},
			[]string{`dws chat message list --group <openConversationId> --time "2026-07-15 10:00:00" --format json`},
		}
	case path == "chat message send" && strings.Contains(message, "media-id is required"):
		guide = chatWorkbookGuidance{
			"图片消息缺少 --media-id",
			"msg-type=image 只接受上游已经获得的有效 mediaId，不能把本地文件名当作 mediaId",
			[]string{"已有 mediaId 时补充 --media-id", "发送本地图片时改用 --msg-type file --file-path"},
			[]string{`dws chat message send --group <openConversationId> --msg-type image --media-id <mediaId> --format json`, `dws chat message send --group <openConversationId> --msg-type file --file-path ./image.png --format json`},
		}
	case path == "chat message send" && strings.Contains(message, "readable local --file-path is required"):
		guide = chatWorkbookGuidance{
			"文件消息缺少可读的本地文件",
			"file、audio、video 消息需要可读的 --file-path；旧版 dentry 参数则必须成组提供",
			[]string{"优先传入当前机器上可读的 --file-path", "使用旧参数时同时提供 dentry-id、space-id 和 file-name"},
			[]string{`dws chat message send --group <openConversationId> --msg-type file --file-path ./report.pdf --format json`},
		}
	case path == "chat message send" && strings.Contains(message, "--file-path must be a readable local file"):
		guide = chatWorkbookGuidance{
			"--file-path 指向的文件不可读",
			"指定路径不存在、不是普通文件或当前进程没有读取权限，因此无法上传并发送",
			[]string{"检查路径拼写并确认文件存在", "改用当前用户可读取的绝对路径或工作目录相对路径"},
			[]string{`dws chat message send --group <openConversationId> --msg-type file --file-path ./report.pdf --format json`},
		}
	case path == "chat message send" && strings.Contains(message, "unsupported --msg-type"):
		guide = chatWorkbookGuidance{
			"不支持指定的 --msg-type",
			"card 不是当前命令支持的消息类型；文本或 Markdown 消息无需传 --msg-type",
			[]string{"文本消息移除 --msg-type 并使用 --text", "媒体消息仅使用 image、file、audio、video、location 或 profile"},
			[]string{`dws chat message send --group <openConversationId> --text "消息正文" --format json`},
		}
	case path == "chat message send" && strings.Contains(message, "message content required"):
		guide = chatWorkbookGuidance{
			"群消息缺少正文内容",
			"未提供 --text 或位置参数，同时也没有选择需要专用参数的媒体消息类型",
			[]string{"发送文字时补充 --text", "发送文件时使用 --msg-type file --file-path"},
			[]string{`dws chat message send --group <openConversationId> --text "消息正文" --format json`},
		}
	}
	if guide.message == "" {
		if required, ok := chatRequiredGuidance[path]; ok &&
			(strings.Contains(message, "required") || strings.Contains(message, "缺少")) {
			guide = required
		} else if strings.HasPrefix(path, "chat ") &&
			(strings.Contains(message, "required") ||
				strings.Contains(message, "invalid") ||
				strings.Contains(message, "unsupported") ||
				strings.Contains(message, "unknown flag") ||
				strings.Contains(message, "must be")) {
			example := fmt.Sprintf("dws %s --help", path)
			if meta, ok := cli.ResolveMeta(path); ok && len(meta.Selection.Examples) > 0 {
				example = meta.Selection.Examples[0]
				if !strings.Contains(example, "--format") {
					example += " --format json"
				}
			}
			guide = chatWorkbookGuidance{
				"Chat 命令参数校验失败",
				message,
				[]string{"根据错误补齐或修正参数", fmt.Sprintf("运行 dws %s --help 核对当前命令参数", path)},
				[]string{example},
			}
		} else {
			return err
		}
	}
	return apperrors.NewValidation(
		guide.message,
		apperrors.WithReason(guide.reason),
		apperrors.WithHint(guide.actions[0]),
		apperrors.WithActions(guide.actions...),
		apperrors.WithExamples(guide.examples...),
		apperrors.WithCause(err),
	)
}

// newPreParseValidationError keeps pipeline handler identity in internal logs
// while exposing only the underlying parameter-domain error to CLI users.
func newPreParseValidationError(err error) error {
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
	if enriched := enrichChatWorkbookError(cmd, err); enriched != err {
		return enriched
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
	const prefix = "unknown flag: --"
	idx := strings.Index(errMsg, prefix)
	if idx < 0 {
		return "", "", false
	}
	flag := strings.TrimSpace(errMsg[idx+len(prefix):])
	if i := strings.IndexAny(flag, " =\n\t"); i >= 0 {
		flag = flag[:i]
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
	return newRootCommandWithEngine(rootCtx, nil, true)
}

// NewSchemaSourceRootCommand constructs the distribution-owned command tree
// used by Schema generation and command-surface policy. Installed plugins and
// user-defined shortcuts must not change the reviewed embedded Schema.
func NewSchemaSourceRootCommand(ctx ...context.Context) *cobra.Command {
	var rootCtx context.Context
	if len(ctx) > 0 && ctx[0] != nil {
		rootCtx = ctx[0]
	}
	return newRootCommandWithEngine(rootCtx, nil, false)
}

// NewRootCommandWithEngine constructs the root CLI command with an
// optional pipeline engine for input correction. When engine is nil,
// no pipeline processing is applied.
func NewRootCommandWithEngine(rootCtx context.Context, engine *pipeline.Engine) *cobra.Command {
	return newRootCommandWithEngine(rootCtx, engine, true)
}

func newRootCommandWithEngine(rootCtx context.Context, engine *pipeline.Engine, loadRuntimeExtensions bool) *cobra.Command {
	if rootCtx == nil {
		rootCtx = context.Background()
	}
	flags := &GlobalFlags{}
	authpkg.SetRuntimeProfile(preparseProfileFlag(os.Args[1:]))
	loader := cli.EnvironmentLoader{
		LookupEnv: os.LookupEnv,
	}
	runner := rootNewCommandRunnerWithFlags(loader, flags)

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
			// Validate caller-provided identity labels before any edition hook
			// or command network activity can run. Header-only library callers
			// use the best-effort path in resolveIdentityHeaders instead.
			if _, err := parseAgentHost(os.Getenv(envDWSAgentHost)); err != nil {
				return err
			}
			if _, err := parseAgentProduct(os.Getenv(agentproduct.EnvName)); err != nil {
				return err
			}

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
	// The legacy dynamic MCP surface remains disabled, but reviewed static MCP
	// helpers registered below are part of the public CLI and Schema surface.
	mcpCmd.Hidden = false
	mcpCmd.Short = "管理 MCP 服务连接信息"
	mcpCmd.Long = "管理经过审核并纳入 Schema 的 MCP 服务连接辅助能力。"
	// Wrap the caller so every MCP tool call's shape is recorded to the local
	// usage log (privacy-preserving; see internal/shortcut/usage). Powers
	// `dws shortcut stats` and future high-frequency shortcut distillation.
	patCaller := newRecordingToolCaller(newToolCallerAdapter(runner, flags))
	mcpCmd.AddCommand(newMCPURLGroup(patCaller))

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

	root.AddCommand(newLegacyPublicCommands(runner, patCaller, loadRuntimeExtensions)...)
	root.AddCommand(newLegacyHiddenCommands(runner)...)

	// PAT authorization commands (open-source core)
	pat.RegisterCommands(root, patCaller)

	if fn := edition.Get().RegisterExtraCommands; fn != nil {
		caller := newToolCallerAdapter(runner, flags)
		fn(root, caller)
		deduplicateCommands(root)
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
	hideNonDirectRuntimeCommands(root)
	configureRootHelp(root)
	// Set custom flag error handler for better UX
	root.SetFlagErrorFunc(flagErrorWithSuggestions)
	installReviewedFlagProtectionHandlers(root)
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
		"markdown":   true,
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
	ClearPluginAuth(productID)
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
	productID := firstNonEmptyPluginString(srv.CLI.ID, srv.Key)
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
