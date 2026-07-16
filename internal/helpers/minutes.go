package helpers

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// dws minutes — 听记
// ──────────────────────────────────────────────────────────

func newMinutesCommand() *cobra.Command {
	minutesListCmd := &cobra.Command{Use: "list", Short: "听记列表", RunE: groupRunE}

	minutesListMineCmd := &cobra.Command{
		Use:   "mine",
		Short: "查询我创建的听记列表",
		Long:  `查询我创建的听记列表，支持分页，支持按关键字和时间范围筛选。`,
		Example: `  dws minutes list mine
  dws minutes list mine --limit 10
  dws minutes list mine --limit 10 --cursor <token>
  dws minutes list mine --query "周会"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callListByKeywordRange(cmd, "created")
		},
	}

	minutesListSharedCmd := &cobra.Command{
		Use:   "shared",
		Short: "查询他人共享给我的听记列表",
		Long:  `查询他人共享给我的听记列表，支持分页，支持按关键字和时间范围筛选。`,
		Example: `  dws minutes list shared
  dws minutes list shared --limit 20
  dws minutes list shared --limit 5 --cursor <token>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callListByKeywordRange(cmd, "shared")
		},
	}

	minutesListAllCmd := &cobra.Command{
		Use:   "all",
		Short: "查询我有权限访问的所有听记列表",
		Long: `查询我有权限访问的所有听记列表（包括我创建的、他人共享给我的等所有有权限的听记），支持按关键字和时间范围筛选。
时间范围和时间关键词为可选参数，不传则返回所有有权限的听记。
--limit 为可选参数，不传时默认返回 10 条。`,
		Example: `  dws minutes list all
  dws minutes list all --limit 20
  dws minutes list all --query "周会" --limit 20
  dws minutes list all --start "2026-03-01T00:00:00+08:00" --end "2026-03-20T23:59:59+08:00"
  dws minutes list all --limit 10 --cursor <token>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callListByKeywordRange(cmd, "noLimit")
		},
	}

	minutesGetCmd := &cobra.Command{Use: "get", Short: "获取听记内容", RunE: groupRunE}

	minutesGetInfoCmd := &cobra.Command{
		Use:   "info",
		Short: "获取听记基础信息",
		Long: `获取指定听记的基础元数据信息。
返回字段：创建人、开始时间、截止时间、听记标题、听记访问链接URL。`,
		Example: `  dws minutes get info --id <taskUuid>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "id", "url", "task-uuid", "uuid"); err != nil {
				return err
			}
			return callMCPTool("get_minutes_basic_info", map[string]any{
				"taskUuid": flagOrFallback(cmd, "id", "url", "task-uuid", "uuid"),
			})
		},
	}

	minutesGetSummaryCmd := &cobra.Command{
		Use:   "summary",
		Short: "获取听记 AI 摘要",
		Long: `获取由 AI 对听记转写原文进行结构化提炼生成的摘要，返回 Markdown 格式。
内容涵盖会议主题、核心结论、关键讨论点等。`,
		Example: `  dws minutes get summary --id <taskUuid>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "id", "url", "task-uuid", "uuid"); err != nil {
				return err
			}
			return callMCPTool("get_minutes_ai_summary", map[string]any{
				"taskUuid": flagOrFallback(cmd, "id", "url", "task-uuid", "uuid"),
			})
		},
	}

	minutesGetKeywordsCmd := &cobra.Command{
		Use:     "keywords",
		Short:   "获取听记关键字列表",
		Example: `  dws minutes get keywords --id <taskUuid>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "id", "url", "task-uuid", "uuid"); err != nil {
				return err
			}
			return callMCPTool("get_minutes_keywords", map[string]any{
				"taskUuid": flagOrFallback(cmd, "id", "url", "task-uuid", "uuid"),
			})
		},
	}

	minutesGetTranscriptionCmd := &cobra.Command{
		Use:   "transcription",
		Short: "获取听记语音转写原文",
		Long: `获取指定听记的语音转写原文。
每条记录包含：发言人信息、转写文本、对应时间戳。

当用户明确要求查看或分析转写原文时，应默认拉取全部原文（自动翻页），
不需要用户手动指定"第一页"。如果用户意图不是专门看原文（如查列表、
看摘要），则不应主动调用此命令。

字符上限保护：循环拉取累积超过 12000 字符时，应暂停并询问用户
是否继续拉取后续分页内容。

--direction:
  0 = 正序（时间递增，默认）
  1 = 倒序（时间递减）`,
		Example: `  dws minutes get transcription --id <taskUuid>
  dws minutes get transcription --id <taskUuid> --direction 1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "id", "url", "task-uuid", "uuid"); err != nil {
				return err
			}

			toolArgs := map[string]any{
				"taskUuid":  flagOrFallback(cmd, "id", "url", "task-uuid", "uuid"),
				"direction": mustGetFlag(cmd, "direction"),
			}

			if v := flagOrFallback(cmd, "cursor", "next-token"); v != "" {
				toolArgs["nextToken"] = v
			}
			return callMCPTool("get_minutes_transcription", toolArgs)
		},
	}

	minutesGetTodosCmd := &cobra.Command{
		Use:   "todos",
		Short: "获取听记中提取的待办事项",
		Long: `查询指定听记中由系统提取的待办事项列表。
每条记录包含：待办内容、待办唯一ID、参与人信息、待办时间。`,
		Example: `  dws minutes get todos --id <taskUuid>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "id", "url", "task-uuid", "uuid"); err != nil {
				return err
			}
			return callMCPTool("list_minutes_todos", map[string]any{
				"taskUuid": flagOrFallback(cmd, "id", "url", "task-uuid", "uuid"),
			})
		},
	}

	// minutesGetAudioCmd — 对应 MCP 工具 query_minutes_audio_url
	// 必填参数：taskUuid(--id 或 --url)
	// 操作人需拥有该听记的"读"权限及以上才会返回音频/视频地址。
	// 支持所有类型的听记（线上闪记、线下闪记、A1 硬件听记、上传文件听记等）。
	// 以下场景不返回地址：听记已被删除、A1 无痕模式听记、临存过期的听记（媒体未准备好或临时存储已过期）。
	// 注意：返回的 OSS 地址通常包含 & 等字符，使用 callMCPToolUnescaped 避免 JSON 转义。
	minutesGetAudioCmd := &cobra.Command{
		Use:   "audio",
		Short: "获取听记音频/视频地址",
		Long: `查询听记的音频/视频文件地址（OSS 链接）。
操作人需拥有该听记的"读"权限及以上才会返回。
支持所有类型的听记（线上闪记、线下闪记、A1 硬件听记、上传文件听记等）。

以下场景不返回地址：
  - 听记已被删除
  - A1 无痕模式听记
  - 临存过期的听记（媒体未准备好或临时存储已过期）`,
		Example: `  dws minutes get audio --id <taskUuid>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "id", "url", "task-uuid", "uuid"); err != nil {
				return err
			}
			return callMCPToolUnescaped("query_minutes_audio_url", map[string]any{
				"taskUuid": flagOrFallback(cmd, "id", "url", "task-uuid", "uuid"),
			})
		},
	}

	minutesGetBatchCmd := &cobra.Command{
		Use:   "batch",
		Short: "批量查询听记详情",
		Long: `根据 taskUuid 列表批量查询听记详情。
返回字段：听记标题、时长、参与人列表、创建时间、taskUuid、听记状态。`,
		Example: `  dws minutes get batch --ids uuid1,uuid2,uuid3`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "ids"); err != nil {
				return err
			}
			return callMCPTool("batch_get_minutes_details", map[string]any{
				"requestBody": map[string]any{
					"taskUuids": parseCSVValues(mustGetFlag(cmd, "ids")),
				},
			})
		},
	}

	minutesUpdateCmd := &cobra.Command{Use: "update", Short: "更新听记信息", RunE: groupRunE}

	minutesUpdateTitleCmd := &cobra.Command{
		Use:     "title",
		Short:   "修改听记标题",
		Example: `  dws minutes update title --id <taskUuid> --title "Q2 复盘会议"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id", "title"); err != nil {
				return err
			}
			return callMCPTool("update_minutes_title", map[string]any{
				"taskUuid": mustGetFlag(cmd, "id"),
				"title":    mustGetFlag(cmd, "title"),
			})
		},
	}

	// list subcommands — mine/shared/all 共享 callListByKeywordRange 链路
	minutesListMineCmd.Flags().Float64("limit", 10, "每页数据条数 (默认 10)")
	minutesListSharedCmd.Flags().Float64("limit", 10, "每页数据条数 (默认 10)")
	minutesListAllCmd.Flags().Float64("limit", 10, "每页数据条数 (默认 10)")

	for _, sub := range []*cobra.Command{minutesListMineCmd, minutesListSharedCmd, minutesListAllCmd} {
		sub.Flags().Float64("max", 10, "--limit 的别名 (兼容旧版)")
		_ = sub.Flags().MarkHidden("max")
		sub.Flags().String("cursor", "", "分页 token (首页留空)")
		sub.Flags().String("next-token", "", "--cursor 的别名 (兼容旧版)")
		_ = sub.Flags().MarkHidden("next-token")
		sub.Flags().String("offset", "", "[已废弃] 分页 offset，请使用 --cursor")
		_ = sub.Flags().MarkHidden("offset")
		sub.Flags().String("query", "", "关键字筛选 (可选)")
		sub.Flags().String("keyword", "", "关键字筛选 (--query 的别名)")
		_ = sub.Flags().MarkHidden("keyword")
		sub.Flags().String("start", "", "开始时间 ISO-8601 (可选)")
		sub.Flags().String("end", "", "结束时间 ISO-8601 (可选)")
	}

	minutesListCmd.AddCommand(minutesListMineCmd, minutesListSharedCmd, minutesListAllCmd)

	for _, sub := range []*cobra.Command{
		minutesGetInfoCmd, minutesGetSummaryCmd,
		minutesGetKeywordsCmd, minutesGetTodosCmd,
		minutesGetAudioCmd,
	} {
		sub.Flags().String("id", "", "听记 taskUuid (必填)")
		sub.Flags().String("url", "", "--id 的别名")
		_ = sub.Flags().MarkHidden("url")
		sub.Flags().String("task-uuid", "", "--id 的别名 (兼容 OpenAPI 字段名)")
		_ = sub.Flags().MarkHidden("task-uuid")
		sub.Flags().String("uuid", "", "--id 的别名")
		_ = sub.Flags().MarkHidden("uuid")
	}

	minutesGetTranscriptionCmd.Flags().String("id", "", "听记 taskUuid (必填)")
	minutesGetTranscriptionCmd.Flags().String("url", "", "--id 的别名")
	_ = minutesGetTranscriptionCmd.Flags().MarkHidden("url")
	minutesGetTranscriptionCmd.Flags().String("task-uuid", "", "--id 的别名 (兼容 OpenAPI 字段名)")
	_ = minutesGetTranscriptionCmd.Flags().MarkHidden("task-uuid")
	minutesGetTranscriptionCmd.Flags().String("uuid", "", "--id 的别名")
	_ = minutesGetTranscriptionCmd.Flags().MarkHidden("uuid")
	minutesGetTranscriptionCmd.Flags().String("direction", "0", "排序方向: 0=正序, 1=倒序 (默认 0)")
	minutesGetTranscriptionCmd.Flags().String("cursor", "", "分页 token (首页留空)")
	minutesGetTranscriptionCmd.Flags().String("next-token", "", "--cursor 的别名 (兼容旧版)")
	_ = minutesGetTranscriptionCmd.Flags().MarkHidden("next-token")

	minutesGetBatchCmd.Flags().String("ids", "", "听记 taskUuid 列表，逗号分隔 (必填)")

	minutesGetCmd.AddCommand(
		minutesGetInfoCmd, minutesGetSummaryCmd, minutesGetKeywordsCmd,
		minutesGetTranscriptionCmd, minutesGetTodosCmd, minutesGetBatchCmd,
		minutesGetAudioCmd,
	)

	minutesUpdateTitleCmd.Flags().String("id", "", "听记 taskUuid (必填)")
	minutesUpdateTitleCmd.Flags().String("title", "", "新标题 (必填)")

	// listeningNoteCmdTool 是听记指令工具在 MCP 网关上的注册名。
	// 网关侧该工具以中文名注册（title: minutes_cmd_start），旧英文名
	// execute_listening_note_command 会返回 "PARAM_ERROR - 未找到指定工具"。
	// 单个工具通过 cmd 参数覆盖 create/pause/resume/end 四种指令。
	const listeningNoteCmdTool = "执行听记指令-发起AI听记录音"

	minutesRecordCmd := &cobra.Command{Use: "record", Short: "控制听记录音", RunE: groupRunE}

	minutesRecordStartCmd := &cobra.Command{
		Use:   "start",
		Short: "发起听记（开始录音）",
		Example: `  dws minutes record start
  dws minutes record start --session-id <sessionId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{"cmd": "create"}
			if v := mustGetFlag(cmd, "session-id"); v != "" {
				toolArgs["sessionId"] = v
			}
			return callMCPTool(listeningNoteCmdTool, toolArgs)
		},
	}

	minutesRecordPauseCmd := &cobra.Command{
		Use:   "pause",
		Short: "暂停听记录音",
		Example: `  dws minutes record pause --id <taskUuid>
  dws minutes record pause --id <taskUuid> --session-id <sessionId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"cmd":  "pause",
				"uuid": mustGetFlag(cmd, "id"),
			}
			if v := mustGetFlag(cmd, "session-id"); v != "" {
				toolArgs["sessionId"] = v
			}
			return callMCPTool(listeningNoteCmdTool, toolArgs)
		},
	}

	minutesRecordResumeCmd := &cobra.Command{
		Use:   "resume",
		Short: "恢复听记录音",
		Example: `  dws minutes record resume --id <taskUuid>
  dws minutes record resume --id <taskUuid> --session-id <sessionId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"cmd":  "resume",
				"uuid": mustGetFlag(cmd, "id"),
			}
			if v := mustGetFlag(cmd, "session-id"); v != "" {
				toolArgs["sessionId"] = v
			}
			return callMCPTool(listeningNoteCmdTool, toolArgs)
		},
	}

	minutesRecordStopCmd := &cobra.Command{
		Use:   "stop",
		Short: "结束听记录音",
		Example: `  dws minutes record stop --id <taskUuid>
  dws minutes record stop --id <taskUuid> --session-id <sessionId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"cmd":  "end",
				"uuid": mustGetFlag(cmd, "id"),
			}
			if v := mustGetFlag(cmd, "session-id"); v != "" {
				toolArgs["sessionId"] = v
			}
			return callMCPTool(listeningNoteCmdTool, toolArgs)
		},
	}

	for _, sub := range []*cobra.Command{
		minutesRecordStartCmd, minutesRecordPauseCmd, minutesRecordResumeCmd, minutesRecordStopCmd,
	} {
		sub.Flags().String("session-id", "", "AI 助理会话 ID (可选)")
	}
	for _, sub := range []*cobra.Command{
		minutesRecordPauseCmd, minutesRecordResumeCmd, minutesRecordStopCmd,
	} {
		sub.Flags().String("id", "", "听记 taskUuid (必填)")
	}
	minutesRecordCmd.AddCommand(minutesRecordStartCmd, minutesRecordPauseCmd, minutesRecordResumeCmd, minutesRecordStopCmd)

	// ── update summary ──────────────────────────────────────────
	minutesUpdateSummaryCmd := &cobra.Command{
		Use:   "summary",
		Short: "更新纪要内容",
		Long: `用传入的摘要文本全量覆盖听记的纪要内容，不触发 AI 重新生成。
适用于用户手动编辑或 AI Agent 修改纪要的场景。

修改纪要的完整流程（读取 -> 修改 -> 校验 -> 写回）：
1. 先调用 get summary 获取当前纪要原文
2. 修改时必须保留原文中所有 Markdown 图片，仅优化文本内容
3. 写回前执行 Markdown 格式校验，确保结构合理、可渲染
4. 调用 update summary 将修改后的完整内容写回听记`,
		Example: `  dws minutes update summary --id <taskUuid> --content "新的纪要内容"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id", "content"); err != nil {
				return err
			}
			return callMCPTool("update_minutes_summary", map[string]any{
				"taskUuid":    mustGetFlag(cmd, "id"),
				"summaryText": mustGetFlag(cmd, "content"),
			})
		},
	}
	minutesUpdateSummaryCmd.Flags().String("id", "", "听记 taskUuid (必填)")
	minutesUpdateSummaryCmd.Flags().String("content", "", "新的纪要内容 (必填)")

	minutesUpdateCmd.AddCommand(minutesUpdateTitleCmd, minutesUpdateSummaryCmd)

	// ── mind-graph 子组 ─────────────────────────────────────────
	mindGraphCmd := &cobra.Command{Use: "mind-graph", Short: "思维导图管理", RunE: groupRunE}

	mindGraphCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建思维导图",
		Long: `触发创建听记思维导图任务。触发成功后，可通过 query_mind_graph_status 轮询任务状态。
状态：0=进行中，1=成功，2=失败。`,
		Example: `  dws minutes mind-graph create --id <taskUuid>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "id", "url", "task-uuid", "uuid"); err != nil {
				return err
			}
			return callMCPTool("create_mind_graph", map[string]any{
				"taskUuid": flagOrFallback(cmd, "id", "url", "task-uuid", "uuid"),
			})
		},
	}
	mindGraphCreateCmd.Flags().String("id", "", "听记 taskUuid (必填)")
	mindGraphCreateCmd.Flags().String("url", "", "--id 的别名")
	_ = mindGraphCreateCmd.Flags().MarkHidden("url")
	mindGraphCreateCmd.Flags().String("task-uuid", "", "--id 的别名 (兼容 OpenAPI 字段名)")
	_ = mindGraphCreateCmd.Flags().MarkHidden("task-uuid")
	mindGraphCreateCmd.Flags().String("uuid", "", "--id 的别名")
	_ = mindGraphCreateCmd.Flags().MarkHidden("uuid")

	mindGraphStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "查询思维导图状态",
		Long: `查询指定听记的思维导图生成状态。
返回任务状态：0=进行中，1=成功，2=失败。如果没有返回任务状态，也视为成功。`,
		Example: `  dws minutes mind-graph status --id <taskUuid>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "id", "url", "task-uuid", "uuid"); err != nil {
				return err
			}
			return callMCPTool("query_mind_graph_status", map[string]any{
				"taskUuid": flagOrFallback(cmd, "id", "url", "task-uuid", "uuid"),
			})
		},
	}
	mindGraphStatusCmd.Flags().String("id", "", "听记 taskUuid (必填)")
	mindGraphStatusCmd.Flags().String("url", "", "--id 的别名")
	_ = mindGraphStatusCmd.Flags().MarkHidden("url")
	mindGraphStatusCmd.Flags().String("task-uuid", "", "--id 的别名 (兼容 OpenAPI 字段名)")
	_ = mindGraphStatusCmd.Flags().MarkHidden("task-uuid")
	mindGraphStatusCmd.Flags().String("uuid", "", "--id 的别名")
	_ = mindGraphStatusCmd.Flags().MarkHidden("uuid")

	mindGraphCmd.AddCommand(mindGraphCreateCmd, mindGraphStatusCmd)

	// ── speaker 子组 ────────────────────────────────────────────
	speakerCmd := &cobra.Command{Use: "speaker", Short: "发言人管理", RunE: groupRunE}

	speakerReplaceCmd := &cobra.Command{
		Use:   "replace",
		Short: "替换发言人",
		Long: `批量替换听记转写中指定发言人，将源发言人（speakerNick）精确匹配的所有段落替换为目标发言人。
支持同时替换 nickName 和 subSpeakerNickname 两种匹配方式，并自动更新纪要、待办中的发言人信息。`,
		Example: `  dws minutes speaker replace --id <taskUuid> --from "张三" --to "李四"
  dws minutes speaker replace --id <taskUuid> --from "张三" --to "李四" --target-uid <uid>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id", "from", "to"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"taskUuid":       mustGetFlag(cmd, "id"),
				"speakerNick":    mustGetFlag(cmd, "from"),
				"targetNickName": mustGetFlag(cmd, "to"),
			}
			if v, _ := cmd.Flags().GetString("target-uid"); v != "" {
				toolArgs["targetUid"] = v
			}
			return callMCPTool("replace_speaker", toolArgs)
		},
	}
	speakerReplaceCmd.Flags().String("id", "", "听记 taskUuid (必填)")
	speakerReplaceCmd.Flags().String("from", "", "源发言人昵称 (必填)")
	speakerReplaceCmd.Flags().String("to", "", "目标发言人昵称 (必填)")
	speakerReplaceCmd.Flags().String("target-uid", "", "目标发言人钉钉 UID (可选)")

	// ── speaker summary 子组 ────────────────────────────────────
	// 对应 MCP 工具 create_speaker_summary / get_speaker_summary
	// 批量按听记维度汇总每位发言人的段落总结

	speakerSummaryCmd := &cobra.Command{Use: "summary", Short: "发言人段落总结", RunE: groupRunE}

	speakerSummaryCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "触发创建发言人段落总结任务",
		Long: `触发创建发言人的段落总结任务，将听记中每位发言人的所有发言内容汇总总结。
触发后需调用 dws minutes speaker summary get 查询总结结果。
--ids 和 --task-uuids 等价，均可使用。`,
		Example: `  dws minutes speaker summary create --ids <uuid1,uuid2>
  dws minutes speaker summary create --task-uuids <uuid1,uuid2>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			v := flagOrFallback(cmd, "ids", "task-uuids")
			if v == "" {
				return fmt.Errorf("flag --ids (or --task-uuids) is required")
			}
			return callMCPTool("create_speaker_summary", map[string]any{
				"uuids": parseCSVValues(v),
			})
		},
	}
	speakerSummaryCreateCmd.Flags().String("ids", "", "听记 taskUuid 列表，逗号分隔 (必填)")
	speakerSummaryCreateCmd.Flags().String("task-uuids", "", "--ids 的别名")
	_ = speakerSummaryCreateCmd.Flags().MarkHidden("task-uuids")

	speakerSummaryGetCmd := &cobra.Command{
		Use:   "get",
		Short: "查询发言人段落总结结果",
		Long: `查询发言人段落总结任务的结果，返回每位发言人的发言汇总。
需先调用 dws minutes speaker summary create 触发任务。
--ids 和 --task-uuids 等价，均可使用。`,
		Example: `  dws minutes speaker summary get --ids <uuid1,uuid2>
  dws minutes speaker summary get --task-uuids <uuid1,uuid2>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			v := flagOrFallback(cmd, "ids", "task-uuids")
			if v == "" {
				return fmt.Errorf("flag --ids (or --task-uuids) is required")
			}
			return callMCPTool("get_speaker_summary", map[string]any{
				"uuids": parseCSVValues(v),
			})
		},
	}
	speakerSummaryGetCmd.Flags().String("ids", "", "听记 taskUuid 列表，逗号分隔 (必填)")
	speakerSummaryGetCmd.Flags().String("task-uuids", "", "--ids 的别名")
	_ = speakerSummaryGetCmd.Flags().MarkHidden("task-uuids")

	speakerSummaryCmd.AddCommand(speakerSummaryCreateCmd, speakerSummaryGetCmd)
	speakerCmd.AddCommand(speakerReplaceCmd, speakerSummaryCmd)

	// ── hot-word 子组 ───────────────────────────────────────────
	hotWordCmd := &cobra.Command{Use: "hot-word", Short: "个人热词管理", RunE: groupRunE}

	hotWordAddCmd := &cobra.Command{
		Use:   "add",
		Short: "添加个人热词",
		Long: `添加听记个人热词，用于优化语音识别中专有名词、人名等的识别准确率。
支持一次添加多个热词（逗号分隔），每个热词长度不超过 10 个汉字或 5 个英文单词。`,
		Example: `  dws minutes hot-word add --words "钉钉"
  dws minutes hot-word add --words "OKR,钉钉,Copilot"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "words"); err != nil {
				return err
			}
			return callMCPTool("add_personal_hot_word", map[string]any{
				"hotWordList": parseCSVValues(mustGetFlag(cmd, "words")),
			})
		},
	}
	hotWordAddCmd.Flags().String("words", "", "要添加的热词，多个用逗号分隔 (必填)")

	hotWordListCmd := &cobra.Command{
		Use:   "list",
		Short: "查询我的热词列表",
		Long: `查询当前用户配置的所有听记热词列表。
无需传入额外参数，系统自动识别当前用户身份。
返回用户已添加的全部热词，适用于查看已有热词、去重检查等场景。`,
		Example: `  dws minutes hot-word list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("list_my_hotwords", map[string]any{})
		},
	}

	hotWordCmd.AddCommand(hotWordAddCmd, hotWordListCmd)

	// ── replace-text 命令 ───────────────────────────────────────
	replaceTextCmd := &cobra.Command{
		Use:   "replace-text",
		Short: "查找替换段落和纪要中匹配的文字",
		Long: `把听记中所有出现的原文字替换为目标文字，包括转写段落和纪要摘要中出现的原文字都会被替换。
区分大小写，精确匹配。`,
		Example: `  dws minutes replace-text --id <taskUuid> --search "旧文字" --replace "新文字"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id", "search", "replace"); err != nil {
				return err
			}
			return callMCPTool("replace_minutes_text", map[string]any{
				"taskUuid":     mustGetFlag(cmd, "id"),
				"originalText": mustGetFlag(cmd, "search"),
				"replacedText": mustGetFlag(cmd, "replace"),
			})
		},
	}
	replaceTextCmd.Flags().String("id", "", "听记 taskUuid (必填)")
	replaceTextCmd.Flags().String("search", "", "要查找的文字 (必填)")
	replaceTextCmd.Flags().String("replace", "", "替换为的新文字 (必填)")

	// ── upload 子组 ─────────────────────────────────────────────
	// 文件上传管理：通过预签名 URL 上传音视频文件并创建听记。
	// 完整流程：create → HTTP PUT → complete，或 create → cancel 取消。
	// 注意：upload 子组的所有命令均使用 callMCPToolUnescaped 输出 JSON，
	// 避免 presignedUrl 中的 & 被 Go 标准库转义为 \u0026。
	uploadCmd := &cobra.Command{Use: "upload", Short: "文件上传管理", RunE: groupRunE}

	// upload create — 对应 MCP 工具 create_upload_session
	// 必填参数：fileName(--file-name), fileSize(--file-size)
	// 可选参数：title(--title), minutesOption 嵌套对象(--template-id, --input-language, --enable-message-card)
	// 返回值包含 sessionId（后续 complete/cancel 使用）和 presignedUrl（HTTP PUT 上传目标）
	uploadCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建文件上传会话",
		Long: `创建文件上传会话，获取预签名上传URL。
调用方拿到 URL 后，直接用 HTTP PUT 将文件上传到该 URL。
必须与 complete 配合使用：
  1. 调用 create 获取预签名上传 URL 和上传 ID
  2. HTTP PUT 预签名上传 URL 上传文件（不带 HEADER）
  3. 调用 complete 传入会话 ID`,
		Example: `  dws minutes upload create --file-name "meeting.mp4" --file-size 102400
  dws minutes upload create --file-name "meeting.mp4" --file-size 102400 --title "周会录音"
  dws minutes upload create --file-name "meeting.mp4" --file-size 102400 --input-language "zh" --enable-message-card`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 校验必填参数 --file-name
			if err := validateRequiredFlags(cmd, "file-name"); err != nil {
				return err
			}
			// 校验必填参数 --file-size，必须为正整数
			fileSize, _ := cmd.Flags().GetInt64("file-size")
			if fileSize <= 0 {
				return fmt.Errorf("flag --file-size is required and must be a positive integer")
			}

			// 构建 MCP 工具调用参数
			toolArgs := map[string]any{
				"fileName": mustGetFlag(cmd, "file-name"),
				"fileSize": fileSize,
			}
			// 可选：听记标题，不传时服务端默认使用文件名去掉后缀
			if v, _ := cmd.Flags().GetString("title"); v != "" {
				toolArgs["title"] = v
			}

			// 构建可选的嵌套对象 minutesOption，仅在有子字段时才传入
			minutesOption := map[string]any{}
			if v, _ := cmd.Flags().GetString("template-id"); v != "" {
				minutesOption["templateId"] = v
			}
			if v, _ := cmd.Flags().GetString("input-language"); v != "" {
				minutesOption["inputLanguage"] = v
			}
			// enable-message-card 是 bool 类型，仅在用户显式传入时才加入参数
			if cmd.Flags().Changed("enable-message-card") {
				enableCard, _ := cmd.Flags().GetBool("enable-message-card")
				minutesOption["enableMessageCard"] = enableCard
			}
			if len(minutesOption) > 0 {
				toolArgs["minutesOption"] = minutesOption
			}

			// 使用 Unescaped 版本输出，保证 presignedUrl 中的 & 不被转义
			return callMCPToolUnescaped("create_upload_session", toolArgs)
		},
	}
	uploadCreateCmd.Flags().String("file-name", "", "文件名（含后缀），如 meeting.mp4 (必填)")
	uploadCreateCmd.Flags().Int64("file-size", 0, "文件大小（字节）(必填)")
	uploadCreateCmd.Flags().String("title", "", "听记标题，不传时默认使用文件名去掉后缀 (可选)")
	uploadCreateCmd.Flags().String("template-id", "", "纪要生成使用的模板 ID (可选)")
	uploadCreateCmd.Flags().String("input-language", "", "ASR 识别的源语言 (可选)")
	uploadCreateCmd.Flags().Bool("enable-message-card", false, "是否推送闪记卡片消息 (可选)")

	// upload complete — 对应 MCP 工具 complete_upload_session
	// 必填参数：sessionId(--session-id)，来自 create 返回值
	// 幂等：同一 sessionId 重复调用直接返回已有任务，不会重复创建
	uploadCompleteCmd := &cobra.Command{
		Use:   "complete",
		Short: "完成文件上传并创建听记",
		Long: `文件上传完成后，创建听记。
必须在 create 之后、预签名 URL 上传完成后调用。
调用流程：
  1. dws minutes upload create ... → 获取 sessionId 和 presignedUrl
  2. curl -X PUT "<presignedUrl>" -T "/path/to/file.mp4"
  3. dws minutes upload complete --session-id <sessionId>

幂等：同一 sessionId 重复调用直接返回已有的任务，不会重复创建。`,
		Example: `  dws minutes upload complete --session-id <sessionId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "session-id"); err != nil {
				return err
			}
			return callMCPToolUnescaped("complete_upload_session", map[string]any{
				"sessionId": mustGetFlag(cmd, "session-id"),
			})
		},
	}
	uploadCompleteCmd.Flags().String("session-id", "", "上传会话 ID，来自 create 返回的 sessionId (必填)")

	// upload cancel — 对应 MCP 工具 cancel_upload_session
	// 必填参数：sessionId(--session-id)，来自 create 返回值
	// 用于在上传前或上传失败后取消会话，释放服务端资源
	uploadCancelCmd := &cobra.Command{
		Use:     "cancel",
		Short:   "取消文件上传会话",
		Long:    `取消 create 创建的上传会话，传入要取消的会话 ID。`,
		Example: `  dws minutes upload cancel --session-id <sessionId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "session-id"); err != nil {
				return err
			}
			return callMCPToolUnescaped("cancel_upload_session", map[string]any{
				"sessionId": mustGetFlag(cmd, "session-id"),
			})
		},
	}
	uploadCancelCmd.Flags().String("session-id", "", "要取消的会话 sessionId (必填)")

	// 注册 upload 子命令：create / complete / cancel
	uploadCmd.AddCommand(uploadCreateCmd, uploadCompleteCmd, uploadCancelCmd)

	// ── permission 子组 ─────────────────────────────────────────
	// 听记成员权限管理：批量添加/移除成员及其权限。
	// 对应 MCP 工具 add_member_permission / remove_member_permission。
	permissionCmd := &cobra.Command{Use: "permission", Short: "听记成员权限管理", RunE: groupRunE}

	// permission add — 对应 MCP 工具 add_member_permission
	// 批量给多个听记增加成员，并设置成员的权限。
	// 权限类型(--policy): 0=管理员, 1=所有者, 2=可编辑, 3=可查看/下载, 4=仅查看
	// 必填参数：--ids（听记 taskUuid 列表）、--member-uids（成员钉钉 UID 列表）、--policy（权限类型）
	// 可选参数：--cover（是否覆盖已有权限）、--sub-resources（权限子模块列表）
	permissionAddCmd := &cobra.Command{
		Use:   "add",
		Short: "批量添加听记成员并设置权限",
		Long: `批量给多个听记增加成员，并设置成员的权限。

权限类型 (--policy):
  0 = 管理员
  1 = 所有者
  2 = 可编辑
  3 = 可查看/下载
  4 = 仅查看

权限子模块 (--sub-resources，可选，逗号分隔):
  OrigContent = 原始内容
  Summary     = 纪要
  Analysis    = 分析
  Note        = 笔记`,
		Example: `  dws minutes permission add --ids <uuid1,uuid2> --member-uids 123456,789012 --policy 3
  dws minutes permission add --ids <uuid> --member-uids 123456 --policy 2 --cover
  dws minutes permission add --ids <uuid> --member-uids 123456 --policy 3 --sub-resources "OrigContent,Summary"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			v := flagOrFallback(cmd, "ids", "uuids", "task-uuids")
			if v == "" {
				return fmt.Errorf("flag --ids (or --uuids / --task-uuids) is required")
			}
			if err := validateRequiredFlags(cmd, "member-uids", "policy"); err != nil {
				return err
			}

			policyID, err := strconv.ParseInt(mustGetFlag(cmd, "policy"), 10, 64)
			if err != nil || policyID < 0 || policyID > 4 {
				return fmt.Errorf("flag --policy must be an integer between 0 and 4 (0=管理员, 1=所有者, 2=可编辑, 3=可查看/下载, 4=仅查看)")
			}

			memberUids := parseCSVValues(mustGetFlag(cmd, "member-uids"))

			toolArgs := map[string]any{
				"uuids":      parseCSVValues(v),
				"policyId":   float64(policyID),
				"memberUids": memberUids,
			}

			if cmd.Flags().Changed("cover") {
				cover, _ := cmd.Flags().GetBool("cover")
				if cover {
					toolArgs["coverPermission"] = "true"
				} else {
					toolArgs["coverPermission"] = "false"
				}
			}

			if sv, _ := cmd.Flags().GetString("sub-resources"); sv != "" {
				toolArgs["roleSubResourceIds"] = parseCSVValues(sv)
			}

			return callMCPTool("add_member_permission", toolArgs)
		},
	}
	permissionAddCmd.Flags().String("ids", "", "听记 taskUuid 列表，逗号分隔 (必填)")
	permissionAddCmd.Flags().String("uuids", "", "--ids 的别名")
	_ = permissionAddCmd.Flags().MarkHidden("uuids")
	permissionAddCmd.Flags().String("task-uuids", "", "--ids 的别名")
	_ = permissionAddCmd.Flags().MarkHidden("task-uuids")
	permissionAddCmd.Flags().String("member-uids", "", "成员钉钉 UID 列表，逗号分隔 (必填)")
	permissionAddCmd.Flags().String("policy", "", "权限类型: 0=管理员, 1=所有者, 2=可编辑, 3=可查看/下载, 4=仅查看 (必填)")
	permissionAddCmd.Flags().Bool("cover", false, "是否覆盖已有权限 (可选，默认 false)")
	permissionAddCmd.Flags().String("sub-resources", "", "权限子模块，逗号分隔: OrigContent/Summary/Analysis/Note (可选)")

	// permission remove — 对应 MCP 工具 remove_member_permission
	// 批量移除多个听记的成员权限。
	// 必填参数：--ids（听记 taskUuid 列表）、--member-uids（成员钉钉 UID 列表）
	permissionRemoveCmd := &cobra.Command{
		Use:   "remove",
		Short: "批量移除听记成员权限",
		Long:  `批量移除多个听记的成员权限。移除后，对应成员将失去对这些听记的访问权限。`,
		Example: `  dws minutes permission remove --ids <uuid1,uuid2> --member-uids 123456,789012
  dws minutes permission remove --ids <uuid> --member-uids 123456`,
		RunE: func(cmd *cobra.Command, args []string) error {
			v := flagOrFallback(cmd, "ids", "uuids", "task-uuids")
			if v == "" {
				return fmt.Errorf("flag --ids (or --uuids / --task-uuids) is required")
			}
			if err := validateRequiredFlags(cmd, "member-uids"); err != nil {
				return err
			}

			memberUids := parseCSVValues(mustGetFlag(cmd, "member-uids"))

			return callMCPTool("remove_member_permission", map[string]any{
				"uuids":      parseCSVValues(v),
				"memberUids": memberUids,
			})
		},
	}
	permissionRemoveCmd.Flags().String("ids", "", "听记 taskUuid 列表，逗号分隔 (必填)")
	permissionRemoveCmd.Flags().String("uuids", "", "--ids 的别名")
	_ = permissionRemoveCmd.Flags().MarkHidden("uuids")
	permissionRemoveCmd.Flags().String("task-uuids", "", "--ids 的别名")
	_ = permissionRemoveCmd.Flags().MarkHidden("task-uuids")
	permissionRemoveCmd.Flags().String("member-uids", "", "成员钉钉 UID 列表，逗号分隔 (必填)")

	permissionCmd.AddCommand(permissionAddCmd, permissionRemoveCmd)

	// ── tag 子组 ────────────────────────────────────────────────
	// 听记标签/分组管理：查询用户标签列表、按标签查询听记。
	// 对应 MCP 工具 query_user_tag_list / query_minutes_by_tag_id。
	// 标签/分组由用户在听记页面手动创建，此处仅提供查询能力。
	tagCmd := &cobra.Command{Use: "tag", Short: "听记标签/分组管理", RunE: groupRunE}

	// tag list — 对应 MCP 工具 query_user_tag_list
	// 无需传入参数，系统自动识别当前用户身份。
	// 返回用户在听记页面创建的所有标签/分组列表，每条记录包含 tagId 和标签名称。
	tagListCmd := &cobra.Command{
		Use:   "list",
		Short: "查询我的听记标签/分组列表",
		Long: `查询当前用户的听记标签或分组列表。
标签/分组在听记页面手动创建，此命令用于查看已有标签。
无需传入额外参数，系统自动识别当前用户身份。
返回所有标签/分组的列表，每条记录包含 tagId 和标签名称。
获取到 tagId 后，可使用 dws minutes tag query --tag-id <tagId> 查询该标签下的听记列表。`,
		Example: `  dws minutes tag list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("query_user_tag_list", map[string]any{})
		},
	}

	// tag query — 对应 MCP 工具 query_minutes_by_tag_id
	// 根据用户的标签/分组 ID 查询该标签下的听记列表。
	// 必填参数：tagId(--tag-id)
	// 可选参数：maxResults(--limit), nextToken(--cursor)
	// tagId 可通过 dws minutes tag list 获取。
	tagQueryCmd := &cobra.Command{
		Use:   "query",
		Short: "根据标签ID查询听记列表",
		Long: `根据用户的标签或分组 ID 查询该标签下的听记列表。
标签/分组在听记页面手动创建，tagId 可通过 dws minutes tag list 获取。
支持分页查询，使用 --limit 控制每页数量，--cursor 传入分页 token。`,
		Example: `  dws minutes tag query --tag-id <tagId>
  dws minutes tag query --tag-id <tagId> --limit 20
  dws minutes tag query --tag-id <tagId> --limit 10 --cursor <nextToken>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "tag-id"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"tagId": mustGetFlag(cmd, "tag-id"),
			}
			if cmd.Flags().Changed("limit") {
				limit, _ := cmd.Flags().GetFloat64("limit")
				toolArgs["maxResults"] = limit
			}
			if v := flagOrFallback(cmd, "cursor", "next-token"); v != "" {
				toolArgs["nextToken"] = v
			}
			return callMCPTool("query_minutes_by_tag_id", toolArgs)
		},
	}
	tagQueryCmd.Flags().String("tag-id", "", "标签/分组 ID，可通过 tag list 获取 (必填)")
	tagQueryCmd.Flags().Float64("limit", 10, "每页数据条数 (默认 10)")
	tagQueryCmd.Flags().String("cursor", "", "分页 token (首页留空)")
	tagQueryCmd.Flags().String("next-token", "", "--cursor 的别名 (兼容旧版)")
	_ = tagQueryCmd.Flags().MarkHidden("next-token")

	tagCmd.AddCommand(tagListCmd, tagQueryCmd)

	minutesCmd := &cobra.Command{
		Use:   "minutes",
		Short: "AI 听记 / 会议纪要",
		Long:  `管理钉钉AI听记：查询列表、获取详情、摘要、转写、待办、关键字、音频地址、思维导图、发言人管理、文件上传、成员权限管理，以及修改标题和纪要内容。`,
		RunE:  groupRunE,
	}
	minutesCmd.AddCommand(minutesListCmd, minutesGetCmd, minutesUpdateCmd, minutesRecordCmd, mindGraphCmd, speakerCmd, hotWordCmd, replaceTextCmd, uploadCmd, permissionCmd, tagCmd)
	return minutesCmd
}

// callListByKeywordRange 调用 list_by_keyword_and_time_range，
// mine/shared/all 统一入口，通过 belongingConditionId 区分（created / shared / noLimit）。
func callListByKeywordRange(cmd *cobra.Command, filterType string) error {
	toolArgs := map[string]any{
		"belongingConditionId": filterType,
	}

	limit, _ := cmd.Flags().GetFloat64("limit")
	if !cmd.Flags().Changed("limit") {
		if maxVal, _ := cmd.Flags().GetFloat64("max"); cmd.Flags().Changed("max") {
			limit = maxVal
		}
	}
	toolArgs["maxResults"] = limit

	startStr, _ := cmd.Flags().GetString("start")
	endStr, _ := cmd.Flags().GetString("end")

	if startStr == "" && endStr == "" {
		// no time range specified — omit time filters
	} else {
		var startMs, endMs int64
		if startStr != "" {
			var err error
			startMs, err = parseISOTimeToMillis("start", startStr)
			if err != nil {
				return err
			}
			toolArgs["createTimeStart"] = float64(startMs)
		}
		if endStr != "" {
			var err error
			endMs, err = parseISOTimeToMillis("end", endStr)
			if err != nil {
				return err
			}
			toolArgs["createTimeEnd"] = float64(endMs)
		}
		if startStr != "" && endStr != "" {
			if err := validateTimeRange(startMs, endMs); err != nil {
				return err
			}
		}
	}

	if v := flagOrFallback(cmd, "query", "keyword"); v != "" {
		toolArgs["keyword"] = v
	}
	if v := flagOrFallback(cmd, "cursor", "next-token", "offset"); v != "" {
		toolArgs["nextToken"] = v
	}
	return callMCPToolOnServer("minutes", "list_by_keyword_and_time_range", toolArgs)
}
