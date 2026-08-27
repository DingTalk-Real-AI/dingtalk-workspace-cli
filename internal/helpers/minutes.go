package helpers

import (
	"fmt"
	"strconv"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// dws minutes — 听记
// ──────────────────────────────────────────────────────────

func newMinutesCommand() *cobra.Command {
	// Product-level Agent routing Decl (migrated from selection/minutes.json
	// products.minutes). Catalog assembly stamps provenance contract_final.
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "minutes",
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "查询和维护钉钉听记的转写、摘要、待办、权限、录音、标签、说话人总结、语音备忘及文件上传会话。",
			UseWhen: []string{
				"用户要查找、读取、编辑或管理钉钉听记及其录音、转写、摘要和衍生内容。",
			},
			AvoidWhen: []string{
				"用户要处理普通文档正文、群聊消息或日历会议安排，而不是钉钉听记。",
			},
		},
	})
	minutesListCmd := newGroupCommand(&cobra.Command{Use: "list", Short: "听记列表", RunE: groupRunE})

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
	DeclareLeafMetadata(minutesListMineCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "list_by_keyword_and_time_range",
				CanonicalPath:  "minutes.list_by_keyword_and_time_range",
				CLIPath:        "minutes list mine",
				PrimaryCLIPath: "minutes list mine",
			},
			Description: "查询当前用户自己创建的听记列表，支持分页、关键字和时间范围筛选。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "list_by_keyword_and_time_range"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询当前用户自己创建的听记列表，支持分页、关键字和时间范围筛选。",
				UseWhen:      []string{"需要按关键词/时间范围查询我自己创建的听记并提取 taskUuid 时"},
				AvoidWhen:    []string{"只要共享听记时改用 list shared；要覆盖全部可访问时改用 list all"},
				Examples:     []string{"dws minutes list mine --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "cursor", Property: "nextToken"},
				{Name: "end", Property: "createTimeEnd", InterfaceType: "number"},
				{Name: "limit", Property: "maxResults"},
				{Name: "query", Property: "keyword"},
				{Name: "start", Property: "createTimeStart", InterfaceType: "number"},
			},
		},
	})

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
	DeclareLeafMetadata(minutesListSharedCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "list_shared_minutes",
				CanonicalPath:  "minutes.list_shared_minutes",
				CLIPath:        "minutes list shared",
				PrimaryCLIPath: "minutes list shared",
			},
			Description: "查询他人共享给当前用户的听记列表，支持分页、关键字和时间范围筛选。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "list_by_keyword_and_time_range"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询他人共享给当前用户的听记列表，支持分页、关键字和时间范围筛选。",
				UseWhen:      []string{"需要查看别人共享给我的听记，或在共享范围内按关键词/时间搜索时"},
				AvoidWhen:    []string{"只要自己创建时改用 list mine；要全部可访问时改用 list all"},
				Examples:     []string{"dws minutes list shared --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "cursor", Property: "nextToken"},
				{Name: "end", Property: "createTimeEnd", InterfaceType: "number"},
				{Name: "limit", Property: "maxResults"},
				{Name: "query", Property: "keyword"},
				{Name: "start", Property: "createTimeStart", InterfaceType: "number"},
			},
		},
	})

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
	DeclareLeafMetadata(minutesListAllCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "list_accessible_minutes",
				CanonicalPath:  "minutes.list_accessible_minutes",
				CLIPath:        "minutes list all",
				PrimaryCLIPath: "minutes list all",
			},
			Description: "查询当前用户有权限访问的全部听记，包括自己创建和他人共享的听记，并支持分页、关键字和时间范围筛选。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "list_by_keyword_and_time_range"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询当前用户有权限访问的全部听记，包括自己创建和他人共享的听记，并支持分页、关键字和时间范围筛选。",
				UseWhen:      []string{"需要按关键词/时间范围覆盖全部可访问听记（含他人共享）时"},
				AvoidWhen:    []string{"明确只要自己创建的听记时改用 list mine；只要共享给我的时改用 list shared"},
				Examples:     []string{"dws minutes list all --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "cursor", Property: "nextToken"},
				{Name: "end", Property: "createTimeEnd", InterfaceType: "number"},
				{Name: "limit", Property: "maxResults"},
				{Name: "query", Property: "keyword"},
				{Name: "start", Property: "createTimeStart", InterfaceType: "number"},
			},
		},
	})

	minutesGetCmd := newGroupCommand(&cobra.Command{Use: "get", Short: "获取听记内容", RunE: groupRunE})

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
	DeclareLeafMetadata(minutesGetInfoCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "get_minutes_basic_info",
				CanonicalPath:  "minutes.get_minutes_basic_info",
				CLIPath:        "minutes get info",
				PrimaryCLIPath: "minutes get info",
			},
			Description: "获取指定听记的基础元数据信息。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "get_minutes_basic_info"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取指定听记的基础元数据信息。",
				UseWhen:      []string{"已知 taskUuid，需要获取创建人/起止时间/标题/访问链接等基础信息时"},
				AvoidWhen:    []string{"要摘要或转写时改用 get summary/transcription"},
				Examples:     []string{"dws minutes get info --id <taskUuid>"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "taskUuid"},
			},
		},
	})

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
	DeclareLeafMetadata(minutesGetSummaryCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "get_minutes_ai_summary",
				CanonicalPath:  "minutes.get_minutes_ai_summary",
				CLIPath:        "minutes get summary",
				PrimaryCLIPath: "minutes get summary",
			},
			Description: "获取由 AI 对听记转写原文进行结构化提炼生成的摘要，返回 Markdown 格式。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "get_minutes_ai_summary"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取由 AI 对听记转写原文进行结构化提炼生成的摘要，返回 Markdown 格式。",
				UseWhen:      []string{"已知 taskUuid，需要获取 AI 生成的 Markdown 听记摘要时"},
				AvoidWhen: []string{
					"要完整转写原文时改用 get transcription",
					"要基础元数据时改用 get info",
				},
				Examples: []string{
					"dws minutes get summary --id <taskUuid>",
					"dws minutes get summary --id <taskUuid> --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "taskUuid", Required: boolPtr(true)},
			},
		},
	})

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
	DeclareLeafMetadata(minutesGetKeywordsCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "get_minutes_keywords",
				CanonicalPath:  "minutes.get_minutes_keywords",
				CLIPath:        "minutes get keywords",
				PrimaryCLIPath: "minutes get keywords",
			},
			Description: "获取指定听记的关键字列表。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "get_minutes_keywords"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取指定听记的关键字列表。",
				UseWhen:      []string{"已知 taskUuid，需要获取听记关键字列表时"},
				AvoidWhen:    []string{"要摘要/转写/待办时改用对应 get 命令"},
				Examples:     []string{"dws minutes get keywords --id <taskUuid>"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "taskUuid"},
			},
		},
	})

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
	DeclareLeafMetadata(minutesGetTranscriptionCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "get_minutes_transcription",
				CanonicalPath:  "minutes.get_minutes_transcription",
				CLIPath:        "minutes get transcription",
				PrimaryCLIPath: "minutes get transcription",
			},
			Description: "获取指定听记的语音转写原文。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "get_minutes_transcription"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "获取指定听记的语音转写原文。",
				UseWhen:      []string{"已知 taskUuid，需要拉取完整语音转写原文（发言人/文本/时间戳）时"},
				AvoidWhen:    []string{"只要结构化摘要时改用 get summary"},
				Examples: []string{
					"dws minutes get transcription --id <taskUuid>",
					"dws minutes get transcription --id <taskUuid> --direction 1",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "cursor", Property: "nextToken"},
				{Name: "id", Property: "taskUuid"},
			},
		},
	})

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
	DeclareLeafMetadata(minutesGetTodosCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "list_minutes_todos",
				CanonicalPath:  "minutes.list_minutes_todos",
				CLIPath:        "minutes get todos",
				PrimaryCLIPath: "minutes get todos",
			},
			Description: "查询指定听记中由系统提取的待办事项列表。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "list_minutes_todos"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询指定听记中由系统提取的待办事项列表。",
				UseWhen:      []string{"已知 taskUuid，需要提取听记中的待办事项列表时"},
				AvoidWhen:    []string{"要管理钉钉个人待办时改用 todo 产品命令"},
				Examples: []string{
					"dws minutes get todos --id <taskUuid>",
					"dws minutes get todos --id <taskUuid> --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "taskUuid"},
			},
		},
	})

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
	DeclareLeafMetadata(minutesGetAudioCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "query_minutes_audio_url",
				CanonicalPath:  "minutes.query_minutes_audio_url",
				CLIPath:        "minutes get audio",
				PrimaryCLIPath: "minutes get audio",
			},
			Description: "查询听记的音频/视频文件地址（OSS 链接）。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "query_minutes_audio_url"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询听记的音频/视频文件地址（OSS 链接）。",
				UseWhen:      []string{"已知 taskUuid 且有读权限，需要获取听记音视频地址以下载或播放时"},
				AvoidWhen:    []string{"听记已删除、无痕模式或媒体未就绪时可能无地址；不要用本命令改内容"},
				Examples:     []string{"dws minutes get audio --id <taskUuid> --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "taskUuid"},
			},
		},
	})

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
	DeclareLeafMetadata(minutesGetBatchCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "batch_get_minutes_details",
				CanonicalPath:  "minutes.batch_get_minutes_details",
				CLIPath:        "minutes get batch",
				PrimaryCLIPath: "minutes get batch",
			},
			Description: "根据 taskUuid 列表批量查询听记详情。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "batch_get_minutes_details"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "根据 taskUuid 列表批量查询听记详情。",
				UseWhen:      []string{"已知多个 taskUuid，需要批量查询听记标题/时长/参与人/状态等详情时"},
				AvoidWhen: []string{
					"只要 AI 摘要/转写/关键字时改用 get summary/transcription/keywords",
					"未知 uuid 时先用 list mine/shared/all",
				},
				Examples: []string{"dws minutes get batch --ids uuid1,uuid2,uuid3"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "ids", Property: "requestBody.taskUuids"},
			},
		},
	})

	minutesUpdateCmd := newGroupCommand(&cobra.Command{Use: "update", Short: "更新听记信息", RunE: groupRunE})

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
	DeclareLeafMetadata(minutesUpdateTitleCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "update_minutes_title",
				CanonicalPath:  "minutes.update_minutes_title",
				CLIPath:        "minutes update title",
				PrimaryCLIPath: "minutes update title",
			},
			Description: "修改指定听记的标题。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "update_minutes_title"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "修改指定听记的标题。",
				UseWhen:      []string{"已知 taskUuid，需要重命名听记标题时"},
				AvoidWhen:    []string{"要改纪要正文时改用 update summary"},
				Examples: []string{
					"dws minutes update title --id <taskUuid> --title \"Q2 复盘会议\"",
					"dws minutes update title --id <taskUuid> --title \"新标题\" --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "taskUuid"},
			},
		},
	})

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

	minutesRecordCmd := newGroupCommand(&cobra.Command{Use: "record", Short: "控制听记录音", RunE: groupRunE})

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
	DeclareLeafMetadata(minutesRecordStartCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "record_start",
				CanonicalPath:  "minutes.record_start",
				CLIPath:        "minutes record start",
				PrimaryCLIPath: "minutes record start",
			},
			Description: "发起听记并开始录音；网关可能只确认受理而不返回 taskUuid。",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "发起听记并开始录音；只有回执明确返回 taskUuid 时才可继续控制。",
				UseWhen:      []string{"需要发起听记并开始录音时；若回执未返回 taskUuid，停止并报告未绑定，不得通过最新听记猜测"},
				AvoidWhen:    []string{"已有进行中的录音只需 pause/resume/stop 时不要重复 start"},
				Examples: []string{
					"dws minutes record start",
					"dws minutes record start --session-id <sessionId>",
				},
			},
		},
	})

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
	DeclareLeafMetadata(minutesRecordPauseCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "record_pause",
				CanonicalPath:  "minutes.record_pause",
				CLIPath:        "minutes record pause",
				PrimaryCLIPath: "minutes record pause",
			},
			Description: "暂停正在进行的听记录音。",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "暂停正在进行的听记录音。",
				UseWhen:      []string{"已知进行中的听记 taskUuid，需要暂停听记录音时"},
				AvoidWhen:    []string{"要恢复时改用 record resume；要结束时改用 record stop；要开始新听记时改用 record start"},
				Examples: []string{
					"dws minutes record pause --id <taskUuid>",
					"dws minutes record pause --id <taskUuid> --session-id <sessionId>",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "uuid"},
			},
		},
	})

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
	DeclareLeafMetadata(minutesRecordResumeCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "record_resume",
				CanonicalPath:  "minutes.record_resume",
				CLIPath:        "minutes record resume",
				PrimaryCLIPath: "minutes record resume",
			},
			Description: "恢复已暂停的听记录音。",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "恢复已暂停的听记录音。",
				UseWhen:      []string{"已知已暂停的听记 taskUuid，需要恢复听记录音时"},
				AvoidWhen:    []string{"要暂停/结束/新开始时改用 pause/stop/start"},
				Examples: []string{
					"dws minutes record resume --id <taskUuid>",
					"dws minutes record resume --id <taskUuid> --session-id <sessionId>",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "uuid"},
			},
		},
	})

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
	DeclareLeafMetadata(minutesRecordStopCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "record_stop",
				CanonicalPath:  "minutes.record_stop",
				CLIPath:        "minutes record stop",
				PrimaryCLIPath: "minutes record stop",
			},
			Description: "结束正在进行的听记录音。",
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: this executable CLI wrapper calls a remote helper that is absent from the pinned MCP metadata snapshot; no single pinned semantically equivalent interface_ref can represent the command.",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "结束正在进行的听记录音。",
				UseWhen:      []string{"已知 taskUuid，需要结束听记录音时"},
				AvoidWhen:    []string{"只需暂停时可改用 pause；尚未 start 时不要 stop"},
				Examples: []string{
					"dws minutes record stop --id <taskUuid>",
					"dws minutes record stop --id <taskUuid> --session-id <sessionId>",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "uuid"},
			},
		},
	})

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
	DeclareLeafMetadata(minutesUpdateSummaryCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "update_minutes_summary",
				CanonicalPath:  "minutes.update_minutes_summary",
				CLIPath:        "minutes update summary",
				PrimaryCLIPath: "minutes update summary",
			},
			Description: "用传入的摘要文本全量覆盖听记的纪要内容，不触发 AI 重新生成。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "update_minutes_summary"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "用传入的摘要文本全量覆盖听记的纪要内容，不触发 AI 重新生成。",
				UseWhen:      []string{"已知 taskUuid，需要用新文本全量覆盖纪要且不触发 AI 重算时"},
				AvoidWhen: []string{
					"只要改标题时改用 update title",
					"内容未确认时不要覆盖",
				},
				Examples: []string{
					"dws minutes update summary --id <taskUuid> --content \"新的纪要内容\"",
					"dws minutes update summary --id <taskUuid> --content \"新的纪要内容\" --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "content", Property: "summaryText"},
				{Name: "id", Property: "taskUuid"},
			},
		},
	})
	minutesUpdateSummaryCmd.Flags().String("id", "", "听记 taskUuid (必填)")
	minutesUpdateSummaryCmd.Flags().String("content", "", "新的纪要内容 (必填)")

	minutesUpdateCmd.AddCommand(minutesUpdateTitleCmd, minutesUpdateSummaryCmd)

	// ── mind-graph 子组 ─────────────────────────────────────────
	mindGraphCmd := newGroupCommand(&cobra.Command{Use: "mind-graph", Short: "思维导图管理", RunE: groupRunE})

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
	DeclareLeafMetadata(mindGraphCreateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "create_mind_graph",
				CanonicalPath:  "minutes.create_mind_graph",
				CLIPath:        "minutes mind-graph create",
				PrimaryCLIPath: "minutes mind-graph create",
			},
			Description: "触发创建听记思维导图任务。触发成功后，可通过 query_mind_graph_status 轮询任务状态。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "create_mind_graph"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "触发创建听记思维导图任务。触发成功后，可通过 query_mind_graph_status 轮询任务状态。",
				UseWhen:      []string{"已知 taskUuid，需要触发听记思维导图生成任务时"},
				AvoidWhen:    []string{"要查询任务状态时改用 mind-graph status 并轮询"},
				Examples: []string{
					"dws minutes mind-graph create --id <taskUuid>",
					"dws minutes mind-graph create --id <taskUuid> --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "taskUuid"},
			},
		},
	})
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
	DeclareLeafMetadata(mindGraphStatusCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "query_mind_graph_status",
				CanonicalPath:  "minutes.query_mind_graph_status",
				CLIPath:        "minutes mind-graph status",
				PrimaryCLIPath: "minutes mind-graph status",
			},
			Description: "查询指定听记的思维导图生成状态。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "query_mind_graph_status"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询指定听记的思维导图生成状态。",
				UseWhen:      []string{"已 create 思维导图后，需要查询任务状态（0进行中/1成功/2失败）时"},
				AvoidWhen:    []string{"尚未触发创建时先用 mind-graph create"},
				Examples: []string{
					"dws minutes mind-graph status --id <taskUuid>",
					"dws minutes mind-graph status --id <taskUuid> --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "taskUuid"},
			},
		},
	})
	mindGraphStatusCmd.Flags().String("id", "", "听记 taskUuid (必填)")
	mindGraphStatusCmd.Flags().String("url", "", "--id 的别名")
	_ = mindGraphStatusCmd.Flags().MarkHidden("url")
	mindGraphStatusCmd.Flags().String("task-uuid", "", "--id 的别名 (兼容 OpenAPI 字段名)")
	_ = mindGraphStatusCmd.Flags().MarkHidden("task-uuid")
	mindGraphStatusCmd.Flags().String("uuid", "", "--id 的别名")
	_ = mindGraphStatusCmd.Flags().MarkHidden("uuid")

	mindGraphCmd.AddCommand(mindGraphCreateCmd, mindGraphStatusCmd)

	// ── speaker 子组 ────────────────────────────────────────────
	speakerCmd := newGroupCommand(&cobra.Command{Use: "speaker", Short: "发言人管理", RunE: groupRunE})

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
	DeclareLeafMetadata(speakerReplaceCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "replace_speaker",
				CanonicalPath:  "minutes.replace_speaker",
				CLIPath:        "minutes speaker replace",
				PrimaryCLIPath: "minutes speaker replace",
			},
			Description: "批量替换听记转写中指定发言人，将源发言人（speakerNick）精确匹配的所有段落替换为目标发言人。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "replace_speaker"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "批量替换听记转写中指定发言人，将源发言人（speakerNick）精确匹配的所有段落替换为目标发言人。",
				UseWhen:      []string{"用户明确要求把转写中指定发言人昵称批量替换为目标发言人时"},
				AvoidWhen:    []string{"源/目标发言人或 taskUuid 未确认时不要替换"},
				Examples: []string{
					"dws minutes speaker replace --id <taskUuid> --from \"张三\" --to \"李四\"",
					"dws minutes speaker replace --id <taskUuid> --from \"张三\" --to \"李四\" --target-uid <uid>",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "from", Property: "speakerNick"},
				{Name: "id", Property: "taskUuid"},
				{Name: "to", Property: "targetNickName"},
			},
		},
	})
	speakerReplaceCmd.Flags().String("id", "", "听记 taskUuid (必填)")
	speakerReplaceCmd.Flags().String("from", "", "源发言人昵称 (必填)")
	speakerReplaceCmd.Flags().String("to", "", "目标发言人昵称 (必填)")
	speakerReplaceCmd.Flags().String("target-uid", "", "目标发言人钉钉 UID (可选)")

	// ── speaker summary 子组 ────────────────────────────────────
	// 对应 MCP 工具 create_speaker_summary / get_speaker_summary
	// 批量按听记维度汇总每位发言人的段落总结

	speakerSummaryCmd := newGroupCommand(&cobra.Command{Use: "summary", Short: "发言人段落总结", RunE: groupRunE})

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
	DeclareLeafMetadata(speakerSummaryCreateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "create_speaker_summary",
				CanonicalPath:  "minutes.create_speaker_summary",
				CLIPath:        "minutes speaker summary create",
				PrimaryCLIPath: "minutes speaker summary create",
			},
			Description: "触发创建发言人的段落总结任务，将听记中每位发言人的所有发言内容汇总总结。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "create_speaker_summary"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "触发创建发言人的段落总结任务，将听记中每位发言人的所有发言内容汇总总结。",
				UseWhen:      []string{"已知听记 uuid，需要触发发言人段落总结异步任务时"},
				AvoidWhen:    []string{"要取结果时改用 speaker summary get（需等待后轮询）"},
				Examples: []string{
					"dws minutes speaker summary create --ids <uuid1,uuid2>",
					"dws minutes speaker summary create --task-uuids <uuid1,uuid2>",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "ids", Property: "uuids"},
			},
		},
	})
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
	DeclareLeafMetadata(speakerSummaryGetCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "get_speaker_summary",
				CanonicalPath:  "minutes.get_speaker_summary",
				CLIPath:        "minutes speaker summary get",
				PrimaryCLIPath: "minutes speaker summary get",
			},
			Description: "查询发言人段落总结任务的结果，返回每位发言人的发言汇总。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "get_speaker_summary"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询发言人段落总结任务的结果，返回每位发言人的发言汇总。",
				UseWhen:      []string{"已触发 create_speaker_summary 后，需要查询发言人段落总结结果时"},
				AvoidWhen:    []string{"尚未 create 任务时先 create；不要把本命令当触发器"},
				Examples: []string{
					"dws minutes speaker summary get --ids <uuid1,uuid2>",
					"dws minutes speaker summary get --task-uuids <uuid1,uuid2>",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "ids", Property: "uuids"},
			},
		},
	})
	speakerSummaryGetCmd.Flags().String("ids", "", "听记 taskUuid 列表，逗号分隔 (必填)")
	speakerSummaryGetCmd.Flags().String("task-uuids", "", "--ids 的别名")
	_ = speakerSummaryGetCmd.Flags().MarkHidden("task-uuids")

	speakerSummaryCmd.AddCommand(speakerSummaryCreateCmd, speakerSummaryGetCmd)
	speakerCmd.AddCommand(speakerReplaceCmd, speakerSummaryCmd)

	// ── hot-word 子组 ───────────────────────────────────────────
	hotWordCmd := newGroupCommand(&cobra.Command{Use: "hot-word", Short: "个人热词管理", RunE: groupRunE})

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
	DeclareLeafMetadata(hotWordAddCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "add_personal_hot_word",
				CanonicalPath:  "minutes.add_personal_hot_word",
				CLIPath:        "minutes hot-word add",
				PrimaryCLIPath: "minutes hot-word add",
			},
			Description: "添加听记个人热词，用于优化语音识别中专有名词、人名等的识别准确率。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "add_personal_hot_word"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "添加听记个人热词，用于优化语音识别中专有名词、人名等的识别准确率。",
				UseWhen:      []string{"需要添加听记个人热词以优化专有名词/人名识别时（单词不超过约10汉字）"},
				AvoidWhen: []string{
					"要查看已有热词时改用 dws minutes hot-word list",
					"要删除热词时改用 dws minutes hot-word delete",
				},
				Examples: []string{
					"dws minutes hot-word add --words \"钉钉\"",
					"dws minutes hot-word add --words \"OKR,钉钉,Copilot\"",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "words", Property: "hotWordList"},
			},
		},
	})
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
	DeclareLeafMetadata(hotWordListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "list_my_hotwords",
				CanonicalPath:  "minutes.list_my_hotwords",
				CLIPath:        "minutes hot-word list",
				PrimaryCLIPath: "minutes hot-word list",
			},
			Description: "查询当前用户配置的所有听记热词列表。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "list_my_hotwords"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询当前用户配置的所有听记热词列表。",
				UseWhen:      []string{"需要查看当前用户已配置的听记个人热词列表时"},
				AvoidWhen: []string{
					"要添加热词时改用 hot-word add",
					"要删除热词时改用 hot-word delete",
				},
				Examples: []string{"dws minutes hot-word list"},
			},
		},
	})

	hotWordDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "批量删除个人热词",
		Long: `批量删除听记个人热词。
支持一次删除多个热词（逗号分隔）。删除后对应热词不再参与后续语音识别优化。`,
		Example: `  dws minutes hot-word delete --words "天气"
  dws minutes hot-word delete --words "OKR,钉钉,Copilot"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "words"); err != nil {
				return err
			}
			return callMCPTool("delete_personal_hotword", map[string]any{
				"hotWordList": parseCSVValues(mustGetFlag(cmd, "words")),
			})
		},
	}
	DeclareLeafMetadata(hotWordDeleteCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "delete_personal_hotword",
				CanonicalPath:  "minutes.delete_personal_hotword",
				CLIPath:        "minutes hot-word delete",
				PrimaryCLIPath: "minutes hot-word delete",
			},
			Description: "批量删除听记个人热词。删除后对应热词不再参与后续语音识别优化。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "delete_personal_hotword"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "批量删除听记个人热词，清理误加或过时热词。",
				UseWhen:      []string{"用户要删除/移除已配置的听记个人热词时"},
				AvoidWhen: []string{
					"要添加热词时改用 hot-word add",
					"不确定现有热词时先用 hot-word list",
				},
				Examples: []string{
					"dws minutes hot-word delete --words \"天气\"",
					"dws minutes hot-word delete --words \"OKR,钉钉,Copilot\"",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "words", Property: "hotWordList"},
			},
		},
	})
	hotWordDeleteCmd.Flags().String("words", "", "要删除的热词，多个用逗号分隔 (必填)")

	hotWordCmd.AddCommand(hotWordAddCmd, hotWordListCmd, hotWordDeleteCmd)

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
	DeclareLeafMetadata(replaceTextCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "replace_minutes_text",
				CanonicalPath:  "minutes.replace_minutes_text",
				CLIPath:        "minutes replace-text",
				PrimaryCLIPath: "minutes replace-text",
			},
			Description: "把听记中所有出现的原文字替换为目标文字，包括转写段落和纪要摘要中出现的原文字都会被替换。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "replace_minutes_text"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "把听记中所有出现的原文字替换为目标文字，包括转写段落和纪要摘要中出现的原文字都会被替换。",
				UseWhen:      []string{"用户明确要求在转写段落与纪要中把原文精确替换为目标文字时"},
				AvoidWhen: []string{
					"搜索词/替换词或 taskUuid 未确认时不要执行",
					"只要改标题时改用 update title",
				},
				Examples: []string{
					"dws minutes replace-text --id <taskUuid> --search \"旧文字\" --replace \"新文字\"",
					"dws minutes replace-text --id <taskUuid> --search \"发言人1\" --replace \"张三\" --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "taskUuid"},
				{Name: "replace", Property: "replacedText"},
				{Name: "search", Property: "originalText"},
			},
		},
	})
	replaceTextCmd.Flags().String("id", "", "听记 taskUuid (必填)")
	replaceTextCmd.Flags().String("search", "", "要查找的文字 (必填)")
	replaceTextCmd.Flags().String("replace", "", "替换为的新文字 (必填)")

	// ── upload 子组 ─────────────────────────────────────────────
	// 文件上传管理：通过预签名 URL 上传音视频文件并创建听记。
	// 完整流程：create → HTTP PUT → complete，或 create → cancel 取消。
	// 注意：upload 子组的所有命令均使用 callMCPToolUnescaped 输出 JSON，
	// 避免 presignedUrl 中的 & 被 Go 标准库转义为 \u0026。
	uploadCmd := newGroupCommand(&cobra.Command{Use: "upload", Short: "文件上传管理", RunE: groupRunE})

	// upload create — 对应 MCP 工具 create_upload_session
	// 必填参数：fileName(--file-name), fileSize(--file-size)
	// 可选参数：title(--title), minutesOption 嵌套对象(--template-id, --input-language)。
	// 消息卡片副作用拆到 create-and-notify，旧 flag 仅保留隐藏迁移错误。
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
  dws minutes upload create --file-name "meeting.mp4" --file-size 102400 --title "周会录音"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMinutesUploadCreate(cmd, false)
		},
	}
	DeclareLeafMetadata(uploadCreateCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "create_upload_session",
				CanonicalPath:  "minutes.create_upload_session",
				CLIPath:        "minutes upload create",
				PrimaryCLIPath: "minutes upload create",
			},
			Description: "创建文件上传会话，获取预签名上传URL。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "create_upload_session"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建文件上传会话，获取预签名上传URL。",
				UseWhen:      []string{"需要把本地音视频上传转成听记：先创建上传会话取得预签名 URL 与 sessionId 时"},
				AvoidWhen:    []string{"会话已存在只需 complete/cancel 时不要重复 create"},
				Examples: []string{
					"dws minutes upload create --file-name \"meeting.mp4\" --file-size 102400",
					"dws minutes upload create --file-name \"meeting.mp4\" --file-size 102400 --title \"周会录音\"",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "enable-message-card", Property: "minutesOption.enableMessageCard"},
				{Name: "input-language", Property: "minutesOption.inputLanguage"},
				{Name: "template-id", Property: "minutesOption.templateId"},
			},
		},
	})
	uploadCreateCmd.Flags().String("file-name", "", "文件名（含后缀），如 meeting.mp4 (必填)")
	uploadCreateCmd.Flags().Int64("file-size", 0, "文件大小（字节）(必填)")
	uploadCreateCmd.Flags().String("title", "", "听记标题，不传时默认使用文件名去掉后缀 (可选)")
	uploadCreateCmd.Flags().String("template-id", "", "纪要生成使用的模板 ID (可选)")
	uploadCreateCmd.Flags().String("input-language", "", "ASR 识别的源语言 (可选)")
	uploadCreateCmd.Flags().Bool("enable-message-card", false, "[兼容提示] 已迁移，请使用 upload create-and-notify")

	uploadCreateAndNotifyCmd := &cobra.Command{
		Use:   "create-and-notify",
		Short: "创建上传会话并在生成后推送闪记卡片",
		Example: `  dws minutes upload create-and-notify --file-name "meeting.mp4" --file-size 102400
  dws minutes upload create-and-notify --file-name "meeting.mp4" --file-size 102400 --title "周会录音"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMinutesUploadCreate(cmd, true)
		},
	}
	DeclareLeafMetadata(uploadCreateAndNotifyCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "create_upload_session_and_notify",
				CanonicalPath:  "minutes.create_upload_session_and_notify",
				CLIPath:        "minutes upload create-and-notify",
				PrimaryCLIPath: "minutes upload create-and-notify",
			},
			Description: "创建文件上传会话，并在听记生成后向用户推送闪记卡片消息。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "create_upload_session"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "创建文件上传会话，并在听记生成后推送闪记卡片消息。",
				UseWhen:      []string{"用户明确要求上传音视频创建听记，并希望额外收到闪记卡片通知时"},
				AvoidWhen:    []string{"只需要创建上传会话、不需要通知时使用 upload create"},
				Examples: []string{
					"dws minutes upload create-and-notify --file-name \"meeting.mp4\" --file-size 102400",
					"dws minutes upload create-and-notify --file-name \"meeting.mp4\" --file-size 102400 --title \"周会录音\"",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "input-language", Property: "minutesOption.inputLanguage"},
				{Name: "template-id", Property: "minutesOption.templateId"},
			},
		},
	})
	uploadCreateAndNotifyCmd.Flags().String("file-name", "", "文件名（含后缀），如 meeting.mp4 (必填)")
	uploadCreateAndNotifyCmd.Flags().Int64("file-size", 0, "文件大小（字节）(必填)")
	uploadCreateAndNotifyCmd.Flags().String("title", "", "听记标题，不传时默认使用文件名去掉后缀 (可选)")
	uploadCreateAndNotifyCmd.Flags().String("template-id", "", "纪要生成使用的模板 ID (可选)")
	uploadCreateAndNotifyCmd.Flags().String("input-language", "", "ASR 识别的源语言 (可选)")

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
	DeclareLeafMetadata(uploadCompleteCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "complete_upload_session",
				CanonicalPath:  "minutes.complete_upload_session",
				CLIPath:        "minutes upload complete",
				PrimaryCLIPath: "minutes upload complete",
			},
			Description: "文件上传完成后，创建听记。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "complete_upload_session"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "文件上传完成后，创建听记。",
				UseWhen:      []string{"预签名 PUT 上传完成后，需要通知服务端完成会话并创建听记时（同 sessionId 幂等）"},
				AvoidWhen: []string{
					"尚未 PUT 上传文件时不要 complete",
					"要取消会话时改用 upload cancel",
				},
				Examples: []string{
					"dws minutes upload complete --session-id <sessionId>",
					"dws minutes upload complete --session-id <sessionId> --format json",
				},
			},
		},
	})
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
	DeclareLeafMetadata(uploadCancelCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "cancel_upload_session",
				CanonicalPath:  "minutes.cancel_upload_session",
				CLIPath:        "minutes upload cancel",
				PrimaryCLIPath: "minutes upload cancel",
			},
			Description: "取消 create 创建的上传会话，传入要取消的会话 ID。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "cancel_upload_session"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "取消 create 创建的上传会话，传入要取消的会话 ID。",
				UseWhen:      []string{"需要取消已创建的文件上传会话并释放资源时"},
				AvoidWhen: []string{
					"要完成上传创建听记时改用 upload complete",
					"要新建上传会话时改用 upload create",
				},
				Examples: []string{
					"dws minutes upload cancel --session-id <sessionId>",
					"dws minutes upload cancel --session-id <sessionId> --format json",
				},
			},
		},
	})
	uploadCancelCmd.Flags().String("session-id", "", "要取消的会话 sessionId (必填)")

	// 注册 upload 子命令：纯创建与带通知创建使用不同的确认策略。
	uploadCmd.AddCommand(uploadCreateCmd, uploadCreateAndNotifyCmd, uploadCompleteCmd, uploadCancelCmd)

	// ── permission 子组 ─────────────────────────────────────────
	// 听记成员权限管理：批量添加/移除成员及其权限、为当前用户申请权限。
	// 对应 MCP 工具 add_member_permission / remove_member_permission / apply_minutes_permission。
	permissionCmd := newGroupCommand(&cobra.Command{Use: "permission", Short: "听记成员权限管理", RunE: groupRunE})

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
			policyID, _ := strconv.ParseInt(mustGetFlag(cmd, "policy"), 10, 64)

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
	DeclareLeafMetadata(permissionAddCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "add_member_permission",
				CanonicalPath:  "minutes.add_member_permission",
				CLIPath:        "minutes permission add",
				PrimaryCLIPath: "minutes permission add",
			},
			Description: "批量给多个听记增加成员，并设置成员的权限。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "add_member_permission"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "批量给多个听记增加成员，并设置成员的权限。",
				UseWhen:      []string{"已知听记 uuid，需要批量给听记增加成员并设置权限（policy 0管理员/1所有者/2可编辑/3可查看下载/4仅查看）时"},
				AvoidWhen: []string{
					"要移除成员权限时改用 dws minutes permission remove",
					"当前用户自己申请访问权限时改用 dws minutes permission apply",
					"成员、权限策略或听记 id 未确认时不要添加",
				},
				Examples: []string{
					"dws minutes permission add --ids <uuid1,uuid2> --member-uids 123456,789012 --policy 3",
					"dws minutes permission add --ids <uuid> --member-uids 123456 --policy 2 --cover",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "cover", Property: "coverPermission"},
				{Name: "ids", Property: "uuids"},
				{Name: "policy", Property: "policyId"},
				{Name: "sub-resources", Property: "roleSubResourceIds"},
			},
		},
		Validate: validateMinutesPermissionAdd,
	})
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
			memberUids := parseCSVValues(mustGetFlag(cmd, "member-uids"))

			return callMCPTool("remove_member_permission", map[string]any{
				"uuids":      parseCSVValues(v),
				"memberUids": memberUids,
			})
		},
	}
	DeclareLeafMetadata(permissionRemoveCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "remove_member_permission",
				CanonicalPath:  "minutes.remove_member_permission",
				CLIPath:        "minutes permission remove",
				PrimaryCLIPath: "minutes permission remove",
			},
			Description: "批量移除多个听记的成员权限。移除后，对应成员将失去对这些听记的访问权限。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "remove_member_permission"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "批量移除多个听记的成员权限。移除后，对应成员将失去对这些听记的访问权限。",
				UseWhen:      []string{"用户明确要求批量移除听记成员权限，使其失去访问时"},
				AvoidWhen: []string{
					"要添加权限时改用 permission add",
					"当前用户自己申请访问权限时改用 permission apply",
					"成员或听记 id 未确认时不要移除",
				},
				Examples: []string{
					"dws minutes permission remove --ids <uuid1,uuid2> --member-uids 123456,789012",
					"dws minutes permission remove --ids <uuid> --member-uids 123456",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "ids", Property: "uuids"},
			},
		},
		Validate: validateMinutesPermissionRemove,
	})
	permissionRemoveCmd.Flags().String("ids", "", "听记 taskUuid 列表，逗号分隔 (必填)")
	permissionRemoveCmd.Flags().String("uuids", "", "--ids 的别名")
	_ = permissionRemoveCmd.Flags().MarkHidden("uuids")
	permissionRemoveCmd.Flags().String("task-uuids", "", "--ids 的别名")
	_ = permissionRemoveCmd.Flags().MarkHidden("task-uuids")
	permissionRemoveCmd.Flags().String("member-uids", "", "成员钉钉 UID 列表，逗号分隔 (必填)")

	// permission apply — 对应 MCP 工具 apply_minutes_permission
	permissionApplyCmd := &cobra.Command{
		Use:   "apply",
		Short: "为当前用户申请听记权限",
		Long: `为当前登录用户申请指定听记的权限。
适用于用户无权限访问某听记（如打开分享链接提示无权限）时，主动向听记所有者发起权限申请。

权限类型 (--policy):
  2 = 可编辑
  3 = 可查看/下载
  4 = 仅查看`,
		Example: `  dws minutes permission apply --id <taskUuid> --policy 4
  dws minutes permission apply --id <taskUuid> --policy 2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			policyID, _ := cmd.Flags().GetInt("policy")

			return callMCPTool("apply_minutes_permission", map[string]any{
				"taskUuid": flagOrFallback(cmd, "id", "url", "task-uuid", "uuid"),
				"policyId": float64(policyID),
			})
		},
	}
	DeclareLeafMetadata(permissionApplyCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "apply_minutes_permission",
				CanonicalPath:  "minutes.apply_minutes_permission",
				CLIPath:        "minutes permission apply",
				PrimaryCLIPath: "minutes permission apply",
			},
			Description: "为当前登录用户申请指定听记的权限。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "apply_minutes_permission"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "为当前登录用户申请指定听记的访问权限（可编辑/可查看下载/仅查看）。",
				UseWhen:      []string{"当前用户对某听记无权限，需要向所有者申请访问（policy 2/3/4）时"},
				AvoidWhen: []string{
					"所有者批量给他人加权限时改用 permission add",
					"要移除他人权限时改用 permission remove",
				},
				Examples: []string{
					"dws minutes permission apply --id <taskUuid> --policy 4",
					"dws minutes permission apply --id <taskUuid> --policy 2",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "id", Property: "taskUuid"},
				{Name: "policy", Property: "policyId"},
			},
		},
		Validate: validateMinutesPermissionApply,
	})
	permissionApplyCmd.Flags().String("id", "", "听记 taskUuid (必填)")
	permissionApplyCmd.Flags().String("url", "", "--id 的别名")
	_ = permissionApplyCmd.Flags().MarkHidden("url")
	permissionApplyCmd.Flags().String("task-uuid", "", "--id 的别名 (兼容 OpenAPI 字段名)")
	_ = permissionApplyCmd.Flags().MarkHidden("task-uuid")
	permissionApplyCmd.Flags().String("uuid", "", "--id 的别名")
	_ = permissionApplyCmd.Flags().MarkHidden("uuid")
	permissionApplyCmd.Flags().Int("policy", 0, "权限类型: 2=可编辑, 3=可查看/下载, 4=仅查看 (必填)")

	permissionCmd.AddCommand(permissionAddCmd, permissionRemoveCmd, permissionApplyCmd)

	// ── tag 子组 ────────────────────────────────────────────────
	// 听记标签/分组管理：查询用户标签列表、按标签查询听记。
	// 对应 MCP 工具 query_user_tag_list / query_minutes_by_tag_id。
	// 标签/分组由用户在听记页面手动创建，此处仅提供查询能力。
	tagCmd := newGroupCommand(&cobra.Command{Use: "tag", Short: "听记标签/分组管理", RunE: groupRunE})

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
	DeclareLeafMetadata(tagListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "query_user_tag_list",
				CanonicalPath:  "minutes.query_user_tag_list",
				CLIPath:        "minutes tag list",
				PrimaryCLIPath: "minutes tag list",
			},
			Description: "查询当前用户的听记标签或分组列表。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "query_user_tag_list"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询当前用户的听记标签或分组列表。",
				UseWhen:      []string{"需要列出我的听记标签或分组时"},
				AvoidWhen:    []string{"已知 tagId 要查听记时改用 tag query"},
				Examples:     []string{"dws minutes tag list"},
			},
		},
	})

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
	DeclareLeafMetadata(tagQueryCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "query_minutes_by_tag_id",
				CanonicalPath:  "minutes.query_minutes_by_tag_id",
				CLIPath:        "minutes tag query",
				PrimaryCLIPath: "minutes tag query",
			},
			Description: "根据用户的标签或分组 ID 查询该标签下的听记列表。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "query_minutes_by_tag_id"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "根据用户的标签或分组 ID 查询该标签下的听记列表。",
				UseWhen:      []string{"已知 tagId，需要查询该标签/分组下的听记列表时"},
				AvoidWhen:    []string{"不知道标签时先用 tag list"},
				Examples: []string{
					"dws minutes tag query --tag-id <tagId>",
					"dws minutes tag query --tag-id <tagId> --limit 20",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "cursor", Property: "nextToken"},
				{Name: "limit", Property: "maxResults"},
			},
		},
	})
	tagQueryCmd.Flags().String("tag-id", "", "标签/分组 ID，可通过 tag list 获取 (必填)")
	tagQueryCmd.Flags().Float64("limit", 10, "每页数据条数 (默认 10)")
	tagQueryCmd.Flags().String("cursor", "", "分页 token (首页留空)")
	tagQueryCmd.Flags().String("next-token", "", "--cursor 的别名 (兼容旧版)")
	_ = tagQueryCmd.Flags().MarkHidden("next-token")

	tagCmd.AddCommand(tagListCmd, tagQueryCmd)

	// ── audio-memo 子组 ────────────────────────────
	// 语音备忘查询：对应 MCP 工具 list_audio_memos。
	// 用户身份由网关按登录态注入 uid，agent/CLI 无需传入。
	// 返回值 items[].audioUrl 为带签名的音频 URL（含 &），因此使用
	// callMCPToolUnescaped 输出，避免 & 被转义为 \u0026（与 upload 一致）。
	audioMemoCmd := newGroupCommand(&cobra.Command{Use: "audio-memo", Short: "语音备忘查询", RunE: groupRunE})

	audioMemoListCmd := &cobra.Command{
		Use:   "list",
		Short: "查询语音备忘列表",
		Long: `查询当前用户的语音备忘列表，支持分页和时间范围筛选。
分页：首页 --cursor 留空（或 0），后续把上一页返回的 nextCursor 回填到 --cursor。
时间范围：--start/--end 为 ISO-8601（可选），不传默认查询近一年。`,
		Example: `  dws minutes audio-memo list
  dws minutes audio-memo list --max 500
  dws minutes audio-memo list --start "2026-01-01T00:00:00+08:00" --end "2026-07-21T23:59:59+08:00"
  dws minutes audio-memo list --cursor 1740000000000`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}

			max, _ := cmd.Flags().GetFloat64("max")
			if max <= 0 || max > 1000 {
				return fmt.Errorf("flag --max must be between 1 and 1000")
			}
			toolArgs["pageSize"] = max

			if cmd.Flags().Changed("cursor") {
				cursor, _ := cmd.Flags().GetInt64("cursor")
				if cursor < 0 {
					return fmt.Errorf("flag --cursor must be >= 0")
				}
				toolArgs["cursor"] = float64(cursor)
			}

			startStr, _ := cmd.Flags().GetString("start")
			endStr, _ := cmd.Flags().GetString("end")
			// China Standard Time has no DST; FixedZone avoids zoneinfo nil-fallback branches.
			loc := time.FixedZone("Asia/Shanghai", 8*3600)
			var startMs, endMs int64
			if startStr != "" {
				var err error
				startMs, err = parseISOTimeToMillis("start", startStr)
				if err != nil {
					return err
				}
				toolArgs["startTime"] = time.UnixMilli(startMs).In(loc).Format(time.RFC3339)
			}
			if endStr != "" {
				var err error
				endMs, err = parseISOTimeToMillis("end", endStr)
				if err != nil {
					return err
				}
				toolArgs["endTime"] = time.UnixMilli(endMs).In(loc).Format(time.RFC3339)
			}
			if startStr != "" && endStr != "" {
				if err := validateTimeRange(startMs, endMs); err != nil {
					return err
				}
			}

			return callMCPToolUnescaped("list_audio_memos", toolArgs)
		},
	}
	DeclareLeafMetadata(audioMemoListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "minutes",
				Name:           "list_audio_memos",
				CanonicalPath:  "minutes.list_audio_memos",
				CLIPath:        "minutes audio-memo list",
				PrimaryCLIPath: "minutes audio-memo list",
			},
			Description: "查询当前用户的语音备忘列表，支持分页和时间范围筛选。",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: "list_audio_memos"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询当前用户的语音备忘列表（独立于听记列表与 get audio）。",
				UseWhen:      []string{"用户要查看语音备忘/录音备忘列表时（可带时间范围或翻页）"},
				AvoidWhen: []string{
					"要查听记列表改用 minutes list",
					"只要某篇听记的音频地址改用 minutes get audio",
				},
				Examples: []string{
					"dws minutes audio-memo list",
					"dws minutes audio-memo list --start \"2026-01-01T00:00:00+08:00\" --end \"2026-07-21T23:59:59+08:00\"",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "max", Property: "pageSize"},
				{Name: "cursor", Property: "cursor"},
				{Name: "start", Property: "startTime"},
				{Name: "end", Property: "endTime"},
			},
		},
	})
	audioMemoListCmd.Flags().Float64("max", 200, "每页数据条数 (默认 200，上限 1000)")
	audioMemoListCmd.Flags().Int64("cursor", 0, "翻页游标，回填上一页返回的 nextCursor (首页留空)")
	audioMemoListCmd.Flags().String("start", "", "开始时间 ISO-8601 (可选，默认近一年)")
	audioMemoListCmd.Flags().String("end", "", "结束时间 ISO-8601 (可选)")
	audioMemoCmd.AddCommand(audioMemoListCmd)

	minutesCmd := newGroupCommand(&cobra.Command{
		Use:   "minutes",
		Short: "AI 听记 / 会议纪要",
		Long:  `管理钉钉AI听记：查询列表、获取详情、摘要、转写、待办、关键字、音频地址、思维导图、发言人管理、文件上传、成员权限管理、语音备忘查询，以及修改标题和纪要内容。`,
		RunE:  groupRunE,
	})
	minutesCmd.AddCommand(minutesListCmd, minutesGetCmd, minutesUpdateCmd, minutesRecordCmd, mindGraphCmd, speakerCmd, hotWordCmd, replaceTextCmd, audioMemoCmd, uploadCmd, permissionCmd, tagCmd)
	return minutesCmd
}

func runMinutesUploadCreate(cmd *cobra.Command, enableMessageCard bool) error {
	if !enableMessageCard && cmd.Flags().Changed("enable-message-card") {
		return fmt.Errorf("--enable-message-card 已迁移：需要通知时请使用 dws minutes upload create-and-notify")
	}
	if err := validateRequiredFlags(cmd, "file-name"); err != nil {
		return err
	}
	fileSize, _ := cmd.Flags().GetInt64("file-size")
	if fileSize <= 0 {
		return fmt.Errorf("flag --file-size is required and must be a positive integer")
	}
	toolArgs := map[string]any{
		"fileName": mustGetFlag(cmd, "file-name"),
		"fileSize": fileSize,
	}
	if value, _ := cmd.Flags().GetString("title"); value != "" {
		toolArgs["title"] = value
	}
	minutesOption := map[string]any{}
	if value, _ := cmd.Flags().GetString("template-id"); value != "" {
		minutesOption["templateId"] = value
	}
	if value, _ := cmd.Flags().GetString("input-language"); value != "" {
		minutesOption["inputLanguage"] = value
	}
	if enableMessageCard {
		minutesOption["enableMessageCard"] = true
	}
	if len(minutesOption) > 0 {
		toolArgs["minutesOption"] = minutesOption
	}
	return callMCPToolUnescaped("create_upload_session", toolArgs)
}

func validateMinutesPermissionAdd(cmd *cobra.Command, _ []string) error {
	if flagOrFallback(cmd, "ids", "uuids", "task-uuids") == "" {
		return fmt.Errorf("flag --ids (or --uuids / --task-uuids) is required")
	}
	if err := validateRequiredFlags(cmd, "member-uids", "policy"); err != nil {
		return err
	}
	policyID, err := strconv.ParseInt(mustGetFlag(cmd, "policy"), 10, 64)
	if err != nil || policyID < 0 || policyID > 4 {
		return fmt.Errorf("flag --policy must be an integer between 0 and 4 (0=管理员, 1=所有者, 2=可编辑, 3=可查看/下载, 4=仅查看)")
	}
	return nil
}

func validateMinutesPermissionRemove(cmd *cobra.Command, _ []string) error {
	if flagOrFallback(cmd, "ids", "uuids", "task-uuids") == "" {
		return fmt.Errorf("flag --ids (or --uuids / --task-uuids) is required")
	}
	return validateRequiredFlags(cmd, "member-uids")
}

func validateMinutesPermissionApply(cmd *cobra.Command, _ []string) error {
	if err := validateRequiredFlagWithAliases(cmd, "id", "url", "task-uuid", "uuid"); err != nil {
		return err
	}
	if !cmd.Flags().Changed("policy") {
		return fmt.Errorf("missing required flag --policy")
	}
	policyID, err := cmd.Flags().GetInt("policy")
	if err != nil || policyID < 2 || policyID > 4 {
		return fmt.Errorf("flag --policy must be an integer between 2 and 4 (2=可编辑, 3=可查看/下载, 4=仅查看)")
	}
	return nil
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
