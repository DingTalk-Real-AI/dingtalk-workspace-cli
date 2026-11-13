package helpers

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
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
		HelpReferences: contract.HelpReferences{
			RelatedSkills: []string{"dingtalk-minutes"},
			Documentation: []contract.HelpDocumentation{
				contract.SkillDocumentation("AI 听记深度指南", "dingtalk-minutes", "references/minutes.md"),
			},
		},
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "查询和维护钉钉听记的转写、摘要、待办、纪要模板、权限、录音、标签、说话人总结、语音备忘及文件上传会话。",
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
			Effect: "destructive", Risk: "high",
			Confirmation: "user_required", Idempotency: "unknown",
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
	// 必填参数：--ids（听记 taskUuid 列表）、--member-uids/--member-staff-ids（二选一）、--policy（权限类型）
	// 可选参数：--cover（是否覆盖已有权限）、--sub-resources（权限子模块列表）
	permissionAddCmd := NewLeafCommand(LeafSpec{
		Use:   "add",
		Short: "批量添加听记成员并设置权限",
		Long: `批量给多个听记增加成员，并设置成员的权限。

成员标识必须且只能选择一种：--member-uids 传真实钉钉 UID；--member-staff-ids 传组织内 staffId，并保留前导零。

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
		Example: `  dws minutes permission add --ids <uuid1,uuid2> --member-uids 1156610563,5908034181 --policy 3
  dws minutes permission add --ids <uuid> --member-staff-ids "074360" --policy 4 --cover`,
		Flags: []LeafFlag{
			{Name: "ids", Usage: "听记 taskUuid 列表，逗号分隔 (必填)", Bind: "uuids", Aliases: []string{"uuids", "task-uuids"}},
			{Name: "member-uids", Usage: "真实成员钉钉 UID 列表，逗号分隔；与 --member-staff-ids 二选一", Bind: "memberUids"},
			{Name: "member-staff-ids", Usage: "组织内成员 staffId 列表，逗号分隔并保留前导零；与 --member-uids 二选一", Bind: "memberStaffIds"},
			{Name: "policy", Usage: "权限类型: 0=管理员, 1=所有者, 2=可编辑, 3=可查看/下载, 4=仅查看 (必填)", Bind: "policyId"},
			{Name: "cover", Usage: "是否覆盖已有权限 (可选，默认 false)", Kind: LeafBool, Bind: "coverPermission"},
			{Name: "sub-resources", Usage: "权限子模块，逗号分隔: OrigContent/Summary/Analysis/Note (可选)", Bind: "roleSubResourceIds"},
		},
		Constraints: []LeafConstraint{{
			Kind:        LeafExactlyOne,
			Flags:       []string{"member-uids", "member-staff-ids"},
			Description: "--member-uids 与 --member-staff-ids 必须且只能提供一个",
		}},
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			v := flagOrFallback(cmd, "ids", "uuids", "task-uuids")
			policyID, _ := strconv.ParseInt(mustGetFlag(cmd, "policy"), 10, 64)

			toolArgs := map[string]any{
				"uuids":    parseCSVValues(v),
				"policyId": float64(policyID),
			}
			if memberUids := parseCSVValues(mustGetFlag(cmd, "member-uids")); len(memberUids) > 0 {
				toolArgs["memberUids"] = memberUids
			} else {
				toolArgs["memberStaffIds"] = parseCSVValues(mustGetFlag(cmd, "member-staff-ids"))
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
				AgentSummary: "使用明确的钉钉 UID 或组织 staffId 批量添加听记成员并设置权限。",
				UseWhen:      []string{"已知听记 uuid，且已明确成员标识是钉钉 UID 还是组织 staffId，需要批量设置权限（policy 0管理员/1所有者/2可编辑/3可查看下载/4仅查看）时"},
				AvoidWhen: []string{
					"要移除成员权限时改用 dws minutes permission remove",
					"当前用户自己申请访问权限时改用 dws minutes permission apply",
					"成员标识类型、权限策略或听记 id 未确认时不要添加",
				},
				Examples: []string{
					"dws minutes permission add --ids <uuid1,uuid2> --member-uids 1156610563,5908034181 --policy 3",
					"dws minutes permission add --ids <uuid> --member-staff-ids 074360 --policy 4 --cover",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "cover", Property: "coverPermission"},
				{Name: "ids", Property: "uuids", Required: boolPtr(true)},
				{Name: "member-staff-ids", Property: "memberStaffIds", InterfaceType: "array"},
				{Name: "member-uids", Property: "memberUids"},
				{Name: "policy", Property: "policyId", Required: boolPtr(true)},
				{Name: "sub-resources", Property: "roleSubResourceIds"},
			},
		},
		Validate: validateMinutesPermissionAdd,
	})

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
	// 自定义标签可在听记页面或通过tag create创建。
	tagCmd := newGroupCommand(&cobra.Command{Use: "tag", Short: "听记标签/分组管理", RunE: groupRunE})

	// tag list — 对应 MCP 工具 query_user_tag_list
	// 无需传入参数，系统自动识别当前用户身份。
	// 返回用户在听记页面创建的所有标签/分组列表，每条记录包含 tagId 和标签名称。
	tagListCmd := &cobra.Command{
		Use:   "list",
		Short: "查询我的听记标签/分组列表",
		Long: `查询当前用户的听记标签或分组列表。
标签/分组可在听记页面或通过tag create创建，此命令用于查看已有标签。
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
标签/分组可在听记页面或通过tag create创建，tagId 可通过 dws minutes tag list 获取。
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
	addMinutesTagWriteCommands(tagCmd)

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
		Long:  `管理钉钉AI听记：查询列表、获取详情、摘要、转写、待办、关键字、音频地址、思维导图、发言人管理、文件上传、成员权限、纪要模板、标签、语音备忘，以及修改标题和纪要内容。`,
		RunE:  groupRunE,
	})
	minutesCmd.AddCommand(minutesListCmd, minutesGetCmd, minutesUpdateCmd, minutesRecordCmd, mindGraphCmd, speakerCmd, hotWordCmd, replaceTextCmd, audioMemoCmd, uploadCmd, permissionCmd, tagCmd)
	minutesCmd.AddCommand(newMinutesTemplateCommand())
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
	if err := validateRequiredFlags(cmd, "policy"); err != nil {
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

func newMinutesTemplateCommand() *cobra.Command {
	templates := newGroupCommand(&cobra.Command{Use: "template", Short: "纪要模板管理", RunE: groupRunE})
	listSystem := NewLeafCommand(LeafSpec{
		Use: "list-system", Short: "分页查询系统纪要模板",
		Long:    "分页查询系统纪要模板目录，返回 templateId、title、brief。默认20条，最多50条；locale默认zh_CN。空页仍按 meta.pagination 判断续页，续页原样传入 cursor 并保持相同 locale。",
		Example: "  dws minutes template list-system --limit 20 --locale zh_CN --format json",
		Server:  "minutes", Tool: "list_system_templates", OutputRollout: output.RolloutUnifiedActive,
		PostMount:  minutesMCPNoArgs,
		ResultCall: callMinutesMCPResult, Safety: minutesMCPReadSafety(),
		Flags: []LeafFlag{
			{Name: "limit", Kind: LeafInt, Default: "20", Bind: "maxResults", Usage: "每页数量，整数1-50，默认20"},
			{Name: "cursor", Bind: "nextToken", OmitEmpty: true, Usage: "上一页 meta.pagination.next_token，原样传入；首页省略"},
			{Name: "locale", Bind: "locale", Default: "zh_CN", Usage: "语言区域，默认zh_CN；续页保持一致"},
		},
		Constraints: []LeafConstraint{{Kind: "custom", Flags: []string{"limit", "cursor"}, Description: "limit必须在1-50；显式cursor不能空白"}},
		Validate:    validateMinutesSystemTemplateList,
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "minutes", Name: "list_system_templates", CanonicalPath: "minutes.list_system_templates", CLIPath: "minutes template list-system", PrimaryCLIPath: "minutes template list-system"},
			Description: "分页查询系统纪要模板目录，按配置顺序返回模板标识、标题和简介。",
			Interface:   minutesMCPInterface("list_system_templates"), DryRun: minutesMCPDryRun(),
			Result: minutesMCPResultSpec(minutesSystemTemplatePageSchema), Pagination: minutesMCPagination(),
			Selection:  contract.SelectionSpec{AgentSummary: "浏览可选的系统纪要模板目录。", UseWhen: []string{"用户要查询或选择官方系统纪要模板时"}, AvoidWhen: []string{"个人模板用 template list-custom；系统模板目录不返回提示词"}, Examples: []string{"dws minutes template list-system --format json"}},
			Parameters: []contract.ParamDecl{{Name: "limit", Property: "maxResults", InterfaceType: "number"}, {Name: "cursor", Property: "nextToken", InterfaceType: "string"}, {Name: "locale", Property: "locale", InterfaceType: "string"}},
		},
	})
	listCustom := NewLeafCommand(LeafSpec{
		Use: "list-custom", Short: "分页查询个人纪要模板",
		Long:    "分页查询当前用户的个人纪要模板，返回 templateKey、title、完整 prompt 和 version，版本0有效。默认5条，最多10条。items为空也须按 meta.pagination 续页，cursor必须原样回传。",
		Example: "  dws minutes template list-custom --limit 5 --format json",
		Server:  "minutes", Tool: "list_custom_templates", OutputRollout: output.RolloutUnifiedActive,
		PostMount:  minutesMCPNoArgs,
		ResultCall: callMinutesMCPResult, Safety: minutesMCPReadSafety(),
		Flags: []LeafFlag{
			{Name: "limit", Kind: LeafInt, Default: "5", Bind: "maxResults", Usage: "每页数量，整数1-10，默认5"},
			{Name: "cursor", Bind: "nextToken", OmitEmpty: true, Usage: "上一页meta.pagination.next_token；首页省略，不能空白"},
		},
		Constraints: []LeafConstraint{{Kind: "custom", Flags: []string{"limit", "cursor"}, Description: "limit必须在1-10；显式cursor不能空白"}},
		Validate:    validateMinutesCustomTemplateList,
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "minutes", Name: "list_custom_templates", CanonicalPath: "minutes.list_custom_templates", CLIPath: "minutes template list-custom", PrimaryCLIPath: "minutes template list-custom"},
			Description: "分页查询当前用户的个人纪要模板及完整提示词，保留版本0和空页续页。",
			Interface:   minutesMCPInterface("list_custom_templates"), DryRun: minutesMCPDryRun(),
			Result: minutesMCPResultSpec(minutesCustomTemplatePageSchema), Pagination: minutesMCPagination(),
			Selection:  contract.SelectionSpec{AgentSummary: "浏览当前用户的个人纪要模板和提示词。", UseWhen: []string{"用户要查询个人纪要模板、templateKey或其完整提示词时"}, AvoidWhen: []string{"系统模板用 template list-system；空页且仍有游标时不要声称列表已结束"}, Examples: []string{"dws minutes template list-custom --format json"}},
			Parameters: []contract.ParamDecl{{Name: "limit", Property: "maxResults", InterfaceType: "number"}, {Name: "cursor", Property: "nextToken", InterfaceType: "string"}},
		},
	})
	create := NewLeafCommand(LeafSpec{
		Use: "create", Short: "创建个人纪要模板",
		Long:    "创建当前用户的个人纪要模板。先确认标题和完整提示词；title非空白且最多150个UTF-16单元，prompt非空白且原样保留空格换行，支持@文件或-读取stdin；最大提示词长度由服务端校验。简介省略或空串由服务异步生成。不会修改已有听记；结果不确定时先查询，不自动重复创建。",
		Example: "  dws minutes template create --title \"周会\" --prompt \"归纳结论和行动项\"",
		Server:  "minutes", Tool: "create_custom_template", OutputRollout: output.RolloutUnifiedActive,
		PostMount:  minutesMCPNoArgs,
		ResultCall: callMinutesMCPResult, Safety: minutesMCPCreateSafety(),
		Flags: []LeafFlag{
			{Name: "title", Bind: "title", Required: true, Usage: "非空白标题，最多150个UTF-16单元"},
			{Name: "prompt", Bind: "prompt", Required: true, Input: []string{corecmd.InputFile, corecmd.InputStdin}, Usage: "完整提示词，保留格式；支持@文件或-"},
			{Name: "brief-intro", Bind: "briefIntro", OmitEmpty: true, Usage: "模板简介；省略或空串由服务异步生成"},
		},
		Validate: validateMinutesTemplateCreate,
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "minutes", Name: "create_custom_template", CanonicalPath: "minutes.create_custom_template", CLIPath: "minutes template create", PrimaryCLIPath: "minutes template create"},
			Description: "创建个人纪要模板，保留完整提示词格式；结果不确定时先查询确认，不自动重试。",
			Interface:   minutesMCPInterface("create_custom_template"), DryRun: minutesMCPDryRun(), Result: minutesMCPResultSpec(minutesTemplateWriteSchema),
			Selection:  contract.SelectionSpec{AgentSummary: "按已确认标题和提示词创建个人纪要模板。", UseWhen: []string{"用户明确要新建个人纪要模板且已提供标题和完整提示词时"}, AvoidWhen: []string{"修改已有模板用 template update；设置后续默认用 template set-default"}, Examples: []string{"dws minutes template create --title \"周会\" --prompt \"归纳结论和行动项\""}},
			Parameters: []contract.ParamDecl{{Name: "title", Property: "title", InterfaceType: "string"}, {Name: "prompt", Property: "prompt", InterfaceType: "string"}, {Name: "brief-intro", Property: "briefIntro", InterfaceType: "string"}},
		},
	})
	update := NewLeafCommand(LeafSpec{
		Use: "update", Short: "修改个人纪要模板",
		Long:    "按真实templateKey修改个人纪要模板。title、prompt、brief-intro至少显式提供一项；省略保留旧值，显式空简介也保留旧简介。title非空白且最多150个UTF-16单元；prompt非空白、原样保留格式，支持@文件或-。不重新生成已有听记。",
		Example: "  dws minutes template update --template-key <templateKey> --title \"周会纪要\"",
		Server:  "minutes", Tool: "update_custom_template", OutputRollout: output.RolloutUnifiedActive,
		PostMount:  minutesMCPNoArgs,
		ResultCall: callMinutesMCPResult, Safety: minutesMCPWriteSafety(),
		Flags: []LeafFlag{
			{Name: "template-key", Bind: "templateKey", Required: true, Trim: true, Usage: "本人个人模板templateKey，不是系统templateId"},
			{Name: "title", Bind: "title", OmitEmpty: true, Usage: "新标题，非空白，最多150个UTF-16单元"},
			{Name: "prompt", Bind: "prompt", OmitEmpty: true, Input: []string{corecmd.InputFile, corecmd.InputStdin}, Usage: "新提示词，原样保留格式；支持@文件或-"},
			{Name: "brief-intro", Bind: "briefIntro", Usage: "新简介；显式空串保留旧值"},
		},
		Constraints: []LeafConstraint{{Kind: corecmd.AtLeastOne, Flags: []string{"title", "prompt", "brief-intro"}, PresenceOnly: true, Description: "title、prompt、brief-intro至少显式提供一项，空简介也算显式提供"}},
		Validate:    validateMinutesTemplateUpdate,
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "minutes", Name: "update_custom_template", CanonicalPath: "minutes.update_custom_template", CLIPath: "minutes template update", PrimaryCLIPath: "minutes template update"},
			Description: "修改指定个人模板，未提供字段与空简介保持旧值，不改已有听记。",
			Interface:   minutesMCPInterface("update_custom_template"), DryRun: minutesMCPDryRun(), Result: minutesMCPResultSpec(minutesTemplateWriteSchema),
			Selection:  contract.SelectionSpec{AgentSummary: "修改已知个人模板的标题、提示词或简介。", UseWhen: []string{"用户要更新已知templateKey的个人纪要模板时"}, AvoidWhen: []string{"新建模板用 template create；未选定真实templateKey时先查询个人模板"}, Examples: []string{"dws minutes template update --template-key <templateKey> --title \"周会纪要\""}},
			Parameters: []contract.ParamDecl{{Name: "template-key", Property: "templateKey", InterfaceType: "string"}, {Name: "title", Property: "title", InterfaceType: "string"}, {Name: "prompt", Property: "prompt", InterfaceType: "string"}, {Name: "brief-intro", Property: "briefIntro", InterfaceType: "string"}},
		},
	})
	deleteTemplate := NewLeafCommand(LeafSpec{
		Use: "delete", Short: "删除个人纪要模板",
		Long:    "删除当前用户指定templateKey的个人纪要模板，执行前确认目标；不存在或已删除也按幂等完成。不会删除已有听记或清除默认模板设置。wasDefault=null表示原默认状态未知，不能当作false。",
		Example: "  dws minutes template delete --template-key <templateKey>",
		Server:  "minutes", Tool: "delete_custom_template", OutputRollout: output.RolloutUnifiedActive,
		PostMount:  minutesMCPNoArgs,
		ResultCall: callMinutesMCPResult, Safety: minutesMCPDeleteSafety(),
		Flags: []LeafFlag{{Name: "template-key", Bind: "templateKey", Required: true, Trim: true, Usage: "待删除的本人个人模板templateKey"}},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "minutes", Name: "delete_custom_template", CanonicalPath: "minutes.delete_custom_template", CLIPath: "minutes template delete", PrimaryCLIPath: "minutes template delete"},
			Description: "幂等删除个人模板；保留已有听记和默认设置，保留wasDefault未知状态。",
			Interface:   minutesMCPInterface("delete_custom_template"), DryRun: minutesMCPDryRun(), Result: minutesMCPResultSpec(minutesTemplateDeleteSchema),
			Selection:  contract.SelectionSpec{AgentSummary: "删除已确认的个人纪要模板。", UseWhen: []string{"用户明确要删除本人某个个人纪要模板且目标已确认时"}, AvoidWhen: []string{"只改变默认选择用 template set-default；不能删除系统模板"}, Examples: []string{"dws minutes template delete --template-key <templateKey>"}},
			Parameters: []contract.ParamDecl{{Name: "template-key", Property: "templateKey", InterfaceType: "string"}},
		},
	})
	setDefault := NewLeafCommand(LeafSpec{
		Use: "set-default", Short: "设置默认纪要模板",
		Long:    "设置当前用户后续听记的默认模板。系统template-id与个人template-key必须二选一，未选字段省略，显式空串非法；template-id不能填diy。个人模板回执templateId=diy；官方模板templateKey=null。保留现有主题，不修改或重新生成已有听记。",
		Example: "  dws minutes template set-default --template-key <templateKey>",
		Server:  "minutes", Tool: "set_default_template", OutputRollout: output.RolloutUnifiedActive,
		PostMount:  minutesMCPNoArgs,
		ResultCall: callMinutesMCPResult, Safety: minutesMCPWriteSafety(),
		Flags: []LeafFlag{
			{Name: "template-id", Bind: "templateId", OmitEmpty: true, Trim: true, Usage: "官方templateId，与template-key二选一，不能填diy或空串"},
			{Name: "template-key", Bind: "templateKey", OmitEmpty: true, Trim: true, Usage: "本人个人模板templateKey，与template-id二选一"},
			{Name: "locale", Bind: "locale", OmitEmpty: true, Usage: "可选，校验系统模板目录时使用"},
		},
		Constraints: []LeafConstraint{{Kind: LeafExactlyOne, Flags: []string{"template-id", "template-key"}, PresenceOnly: true}},
		Validate:    validateMinutesDefaultTemplate,
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "minutes", Name: "set_default_template", CanonicalPath: "minutes.set_default_template", CLIPath: "minutes template set-default", PrimaryCLIPath: "minutes template set-default"},
			Description: "设置后续听记默认模板，系统与个人模板标识二选一，不修改已有听记。",
			Interface:   minutesMCPInterface("set_default_template"), DryRun: minutesMCPDryRun(), Result: minutesMCPResultSpec(minutesTemplateDefaultSchema),
			Selection:  contract.SelectionSpec{AgentSummary: "选择后续听记默认使用的系统或个人纪要模板。", UseWhen: []string{"用户明确要设置之后创建的听记默认纪要模板时"}, AvoidWhen: []string{"更新个人模板内容用 template update；本命令不会重新生成已有听记"}, Examples: []string{"dws minutes template set-default --template-key <templateKey>"}},
			Parameters: []contract.ParamDecl{{Name: "template-id", Property: "templateId", InterfaceType: "string"}, {Name: "template-key", Property: "templateKey", InterfaceType: "string"}, {Name: "locale", Property: "locale", InterfaceType: "string"}},
		},
	})
	templates.AddCommand(listSystem, listCustom, create, update, deleteTemplate, setDefault)
	return templates
}

func validateMinutesSystemTemplateList(cmd *cobra.Command, _ []string) error {
	return validateMinutesTemplateList(cmd, 50)
}

func validateMinutesCustomTemplateList(cmd *cobra.Command, _ []string) error {
	return validateMinutesTemplateList(cmd, 10)
}

func validateMinutesTemplateList(cmd *cobra.Command, maximum int) error {
	limit, _ := cmd.Flags().GetInt("limit")
	if limit < 1 || limit > maximum {
		return apperrors.NewValidation("--limit超出允许范围")
	}
	if cursor, _ := cmd.Flags().GetString("cursor"); cmd.Flags().Changed("cursor") && strings.TrimSpace(cursor) == "" {
		return apperrors.NewValidation("--cursor不能是空白；首页请省略")
	}
	return nil
}

func validateMinutesTemplateCreate(cmd *cobra.Command, _ []string) error {
	return validateMinutesTemplateContent(cmd, true)
}

func validateMinutesTemplateUpdate(cmd *cobra.Command, _ []string) error {
	return validateMinutesTemplateContent(cmd, false)
}

func validateMinutesTemplateContent(cmd *cobra.Command, create bool) error {
	if title, _ := cmd.Flags().GetString("title"); create || cmd.Flags().Changed("title") {
		if strings.TrimSpace(title) == "" || len(utf16.Encode([]rune(title))) > 150 {
			return apperrors.NewValidation("--title必须非空白且不超过150个UTF-16单元")
		}
	}
	if prompt, _ := cmd.Flags().GetString("prompt"); (create || cmd.Flags().Changed("prompt")) && strings.TrimSpace(prompt) == "" {
		return apperrors.NewValidation("--prompt必须非空白；内容不能截断")
	}
	return nil
}

func validateMinutesDefaultTemplate(cmd *cobra.Command, _ []string) error {
	for _, name := range []string{"template-id", "template-key"} {
		value, _ := cmd.Flags().GetString(name)
		if cmd.Flags().Changed(name) && strings.TrimSpace(value) == "" {
			return apperrors.NewValidation("--" + name + "不能是空白；未选字段请省略")
		}
	}
	if value, _ := cmd.Flags().GetString("template-id"); strings.TrimSpace(value) == "diy" {
		return apperrors.NewValidation("个人模板请使用--template-key，--template-id不能填diy")
	}
	return nil
}

func addMinutesTagWriteCommands(tags *cobra.Command) {
	create := NewLeafCommand(LeafSpec{
		Use: "create", Short: "创建听记自定义标签",
		Long:    "为当前用户创建自定义标签，返回实际保存的tagId和displayName，不自动关联听记。名称非空白且最多50个UTF-16单元。系统或自定义标签不能同名；同名推荐标签会先删除，创建失败不恢复。执行前确认名称，结果不确定时先查询核对。",
		Example: "  dws minutes tag create --display-name \"周会\"",
		Server:  "minutes", Tool: "create_minutes_tag", OutputRollout: output.RolloutUnifiedActive,
		PostMount:  minutesMCPNoArgs,
		ResultCall: callMinutesMCPResult, Safety: minutesMCPCreateSafety(),
		Flags: []LeafFlag{{Name: "display-name", Bind: "displayName", Required: true, Usage: "标签名称，非空白且最多50个UTF-16单元", Transform: minutesTagName}},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "minutes", Name: "create_minutes_tag", CanonicalPath: "minutes.create_minutes_tag", CLIPath: "minutes tag create", PrimaryCLIPath: "minutes tag create"},
			Description: "创建个人自定义标签；同名推荐标签可能先删除，失败不承诺回滚。",
			Interface:   minutesMCPInterface("create_minutes_tag"), DryRun: minutesMCPDryRun(), Result: minutesMCPResultSpec(minutesTagCreateSchema),
			Selection:  contract.SelectionSpec{AgentSummary: "按已确认名称创建听记自定义标签。", UseWhen: []string{"用户明确要创建一个本人听记自定义标签时"}, AvoidWhen: []string{"给一条听记添加已有标签用 tag add；只查看标签用 tag list"}, Examples: []string{"dws minutes tag create --display-name \"周会\""}},
			Parameters: []contract.ParamDecl{{Name: "display-name", Property: "displayName", InterfaceType: "string"}},
		},
	})
	deleteTag := NewLeafCommand(LeafSpec{
		Use: "delete", Short: "删除听记自定义标签定义",
		Long:    "删除当前用户的自定义标签定义，随后异步清理所有听记关联。不存在或已删除按幂等完成；系统和推荐标签不能删除。执行前确认真实ID和名称，无恢复工具。成功回执不证明异步清理完成，听记正文和已同步的知识库文件不撤回。",
		Example: "  dws minutes tag delete --tag-id <tagId>",
		Server:  "minutes", Tool: "delete_minutes_tag", OutputRollout: output.RolloutUnifiedActive,
		PostMount:  minutesMCPNoArgs,
		ResultCall: callMinutesMCPResult, Safety: minutesMCPDeleteSafety(),
		Flags: []LeafFlag{{Name: "tag-id", Bind: "tagId", Required: true, Trim: true, Usage: "已确认的本人自定义标签ID"}},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "minutes", Name: "delete_minutes_tag", CanonicalPath: "minutes.delete_minutes_tag", CLIPath: "minutes tag delete", PrimaryCLIPath: "minutes tag delete"},
			Description: "幂等删除自定义标签定义并异步清理关联，不撤回已同步知识库文件。",
			Interface:   minutesMCPInterface("delete_minutes_tag"), DryRun: minutesMCPDryRun(), Result: minutesMCPResultSpec(minutesTagDeleteSchema),
			Selection:  contract.SelectionSpec{AgentSummary: "删除已确认的听记自定义标签定义及其关联。", UseWhen: []string{"用户明确要删除本人自定义标签本身且已确认目标ID和名称时"}, AvoidWhen: []string{"只从一条听记移除关联用 tag remove；系统或推荐标签不可删除"}, Examples: []string{"dws minutes tag delete --tag-id <tagId>"}},
			Parameters: []contract.ParamDecl{{Name: "tag-id", Property: "tagId", InterfaceType: "string"}},
		},
	})
	add := NewLeafCommand(LeafSpec{
		Use: "add", Short: "为一条听记添加标签",
		Long:    "为有查看权限的一条听记增量添加本人自定义标签，保留已有标签。原始输入1-20项，去首尾ASCII空白后按首次顺序去重；关联并集最多20项。可能触发已配置知识库同步，回执不证明同步完成。首个失败停止且不回滚；FAILED为写入结果未知，NOT_PROCESSED为未执行，须检查逐项结果。",
		Example: "  dws minutes tag add --id <taskUuid> --tag-ids <tagId1,tagId2>",
		Server:  "minutes", Tool: "add_tags_to_minutes", OutputRollout: output.RolloutUnifiedActive,
		PostMount:  minutesMCPNoArgs,
		ResultCall: callMinutesMCPResult, Safety: minutesMCPWriteSafety(),
		Flags: []LeafFlag{
			{Name: "id", Bind: "taskUuid", Required: true, Trim: true, Usage: "真实听记taskUuid，不能使用taskId或分享链接"},
			{Name: "tag-ids", Bind: "tagIds", Required: true, Transform: minutesTagIDs, Usage: "逗号分隔本人自定义标签ID，原始1-20项，允许重复"},
			{Name: "locale", Bind: "locale", OmitEmpty: true, Usage: "可选，知识库同步语言；缺省或空白由服务使用zh"},
		},
		Constraints: []LeafConstraint{{Kind: "custom", Flags: []string{"tag-ids"}, Description: "原始标签ID列表须有1-20项且无空白项，按首次出现顺序去重"}},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "minutes", Name: "add_tags_to_minutes", CanonicalPath: "minutes.add_tags_to_minutes", CLIPath: "minutes tag add", PrimaryCLIPath: "minutes tag add"},
			Description: "为一条听记增量关联自定义标签，逐项串行执行，保留已成功、未知及未执行的结果。",
			Interface:   minutesMCPInterface("add_tags_to_minutes"), DryRun: minutesMCPDryRun(), Result: minutesMCPRelationResultSpec(),
			Selection:  contract.SelectionSpec{AgentSummary: "为已知听记添加已有的本人自定义标签。", UseWhen: []string{"用户明确要把已有自定义标签添加到一条有查看权限的听记时"}, AvoidWhen: []string{"创建标签用 tag create；移除一条听记关联用 tag remove；不代表知识库已同步完成"}, Examples: []string{"dws minutes tag add --id <taskUuid> --tag-ids <tagId1,tagId2>"}},
			Parameters: []contract.ParamDecl{{Name: "id", Property: "taskUuid", InterfaceType: "string"}, {Name: "tag-ids", Property: "tagIds", InterfaceType: "array"}, {Name: "locale", Property: "locale", InterfaceType: "string"}},
		},
	})
	remove := NewLeafCommand(LeafSpec{
		Use: "remove", Short: "从一条听记移除标签关联",
		Long:    "从有查看权限的一条听记移除本人自定义标签关联，不删除标签定义、听记正文或已同步的知识库文件。原始输入1-20项，去首尾ASCII空白后按首次顺序去重。无关联返回UNCHANGED。首个失败停止且不回滚；FAILED为写入结果未知，NOT_PROCESSED为未执行，须检查逐项结果。",
		Example: "  dws minutes tag remove --id <taskUuid> --tag-ids <tagId1,tagId2>",
		Server:  "minutes", Tool: "remove_tags_from_minutes", OutputRollout: output.RolloutUnifiedActive,
		PostMount:  minutesMCPNoArgs,
		ResultCall: callMinutesMCPResult, Safety: minutesMCPWriteSafety(),
		Flags: []LeafFlag{
			{Name: "id", Bind: "taskUuid", Required: true, Trim: true, Usage: "真实听记taskUuid，不能使用taskId或分享链接"},
			{Name: "tag-ids", Bind: "tagIds", Required: true, Transform: minutesTagIDs, Usage: "逗号分隔本人自定义标签ID，原始1-20项，允许重复"},
		},
		Constraints: []LeafConstraint{{Kind: "custom", Flags: []string{"tag-ids"}, Description: "原始标签ID列表须有1-20项且无空白项，按首次出现顺序去重"}},
		Contract: LeafContract{
			Identity:    contract.ToolIdentitySpec{ProductID: "minutes", Name: "remove_tags_from_minutes", CanonicalPath: "minutes.remove_tags_from_minutes", CLIPath: "minutes tag remove", PrimaryCLIPath: "minutes tag remove"},
			Description: "从一条听记移除自定义标签关联，保留标签定义及逐项执行状态。",
			Interface:   minutesMCPInterface("remove_tags_from_minutes"), DryRun: minutesMCPDryRun(), Result: minutesMCPRelationResultSpec(),
			Selection:  contract.SelectionSpec{AgentSummary: "只移除一条听记的自定义标签关联。", UseWhen: []string{"用户明确要从指定听记移除本人自定义标签关联时"}, AvoidWhen: []string{"删除标签定义用 tag delete；添加关联用 tag add；不能撤回已同步知识库文件"}, Examples: []string{"dws minutes tag remove --id <taskUuid> --tag-ids <tagId1,tagId2>"}},
			Parameters: []contract.ParamDecl{{Name: "id", Property: "taskUuid", InterfaceType: "string"}, {Name: "tag-ids", Property: "tagIds", InterfaceType: "array"}},
		},
	})
	tags.AddCommand(create, deleteTag, add, remove)
}

func minutesTagName(raw string) (any, error) {
	if strings.TrimSpace(raw) == "" || len(utf16.Encode([]rune(raw))) > 50 {
		return nil, apperrors.NewValidation("--display-name必须非空白且不超过50个UTF-16单元")
	}
	return raw, nil
}

// Use a declared string Transform rather than KindStringSlice: the framework's
// slice normalization drops blank entries and trims Unicode whitespace before
// a domain transform can enforce the backend's raw-count/Java-trim contract.
func minutesTagIDs(raw string) (any, error) {
	reader := csv.NewReader(strings.NewReader(raw))
	values, err := reader.Read()
	if err != nil {
		return nil, apperrors.NewValidation("--tag-ids须是有效的逗号分隔ID列表")
	}
	if _, err := reader.Read(); err != io.EOF {
		return nil, apperrors.NewValidation("--tag-ids只接受一行逗号分隔ID列表")
	}
	if len(values) < 1 || len(values) > 20 {
		return nil, apperrors.NewValidation("--tag-ids原始数量必须在1-20项之间")
	}
	seen := map[string]bool{}
	unique := make([]string, 0, len(values))
	for index, value := range values {
		if strings.TrimSpace(value) == "" {
			return nil, apperrors.NewValidation("--tag-ids包含空白项")
		}
		value = strings.TrimFunc(value, func(r rune) bool { return r <= 0x20 })
		if value == "" {
			return nil, apperrors.NewValidation(fmt.Sprintf("--tag-ids第%d项为空白", index+1))
		}
		if !seen[value] {
			seen[value] = true
			unique = append(unique, value)
		}
	}
	return unique, nil
}

const minutesMCPWriteRecovery = "写入结果可能已经生效；先查询原模板、标签或听记关联核对，不自动重试整个写请求。"

const minutesSystemTemplatePageSchema = `{"type":"object","properties":{"items":{"type":"array","description":"当前页系统模板，按服务配置排序","items":{"type":"object","properties":{"templateId":{"type":"string","description":"系统模板ID"},"title":{"type":"string","description":"本地化标题"},"brief":{"type":"string","description":"本地化简介，允许空串"}},"required":["templateId","title","brief"],"additionalProperties":true}}},"required":["items"],"additionalProperties":false}`
const minutesCustomTemplatePageSchema = `{"type":"object","properties":{"items":{"type":"array","description":"当前页个人模板；空数组也可能有续页","items":{"type":"object","properties":{"templateKey":{"type":"string","description":"个人模板key"},"title":{"type":"string","description":"模板标题"},"prompt":{"type":"string","description":"完整提示词，原样保留格式"},"version":{"type":"integer","minimum":0,"description":"版本号，0有效"}},"required":["templateKey","title","prompt","version"],"additionalProperties":true}}},"required":["items"],"additionalProperties":false}`
const minutesTemplateWriteSchema = `{"type":"object","properties":{"success":{"type":"boolean","const":true,"description":"模板已保存"},"templateKey":{"type":"string","description":"保存后的个人模板key"},"title":{"type":"string","description":"实际保存标题"},"version":{"type":"integer","minimum":0,"description":"保存版本号，0有效"}},"required":["success","templateKey","title","version"],"additionalProperties":true}`
const minutesTemplateDeleteSchema = `{"type":"object","properties":{"success":{"type":"boolean","const":true,"description":"删除已完成或目标原本不存在"},"templateKey":{"type":"string","description":"请求删除的个人模板key"},"wasDefault":{"type":["boolean","null"],"description":"原默认状态，null为未知；默认设置不清除"}},"required":["success","templateKey","wasDefault"],"additionalProperties":true}`
const minutesTemplateDefaultSchema = `{"type":"object","properties":{"success":{"type":"boolean","const":true,"description":"默认模板设置完成"},"templateId":{"type":"string","description":"系统templateId，个人模板为diy"},"templateKey":{"type":["string","null"],"description":"个人模板key；系统模板为null"}},"required":["success","templateId","templateKey"],"additionalProperties":true}`
const minutesTagCreateSchema = `{"type":"object","properties":{"success":{"type":"boolean","const":true,"description":"标签已创建"},"tagId":{"type":"string","description":"新标签ID"},"displayName":{"type":"string","description":"实际保存名称"}},"required":["success","tagId","displayName"],"additionalProperties":true}`
const minutesTagDeleteSchema = `{"type":"object","properties":{"success":{"type":"boolean","const":true,"description":"主删除完成或标签原本不存在，不证明异步关联清理完成"},"tagId":{"type":"string","description":"请求删除的标签ID"}},"required":["success","tagId"],"additionalProperties":true}`
const minutesTagRelationSchema = `{"type":"object","properties":{"success":{"type":"boolean","const":true,"description":"所有关联项均确认完成"},"taskUuid":{"type":"string","description":"本次听记taskUuid"},"results":{"type":"array","description":"与去重请求同序的逐项成功回执","items":{"type":"object","properties":{"tagId":{"type":"string","description":"标签ID"},"status":{"type":"string","enum":["APPLIED","UNCHANGED"],"description":"实际变更或原本即目标状态"}},"required":["tagId","status"],"additionalProperties":true}}},"required":["success","taskUuid","results"],"additionalProperties":true}`
const minutesTagPartialSchema = `{"type":"object","properties":{"total":{"type":"integer","description":"去重后的请求标签总数"},"succeeded":{"type":"array","minItems":1,"description":"原APPLIED或UNCHANGED条目，加上id与taskUuid","items":{"type":"object","properties":{"id":{"type":"string","description":"标签ID"},"taskUuid":{"type":"string","description":"听记taskUuid"},"tagId":{"type":"string","description":"标签ID"},"status":{"type":"string","enum":["APPLIED","UNCHANGED"],"description":"已确认完成状态"}},"required":["id","taskUuid","tagId","status"],"additionalProperties":true}},"failed":{"type":"array","description":"前序失败而未执行的NOT_PROCESSED项","items":{"type":"object","properties":{"id":{"type":"string","description":"未执行的标签ID"},"error":{"type":"object","description":"subtype=not_processed，rpc_data保留原始条目","additionalProperties":true}},"required":["id","error"]}},"unknown":{"type":"array","description":"FAILED项的写入终态未知，可能已落库","items":{"type":"object","properties":{"id":{"type":"string","description":"终态未知的标签ID"},"reason":{"type":"string","description":"服务端失败诊断；先核对再决定后续操作"}},"required":["id","reason"]}}},"required":["total","succeeded","failed","unknown"],"additionalProperties":false}`
const minutesMCPDryRunSchema = `{"type":"object","properties":{"tool":{"type":"string","description":"预览的MCP工具名"},"arguments":{"type":"object","description":"已验证且将要发送的业务参数","additionalProperties":true},"executed":{"type":"boolean","const":false,"description":"本次未执行远端请求"}},"required":["tool","arguments","executed"],"additionalProperties":false}`

func minutesMCPReadSafety() contract.SafetySpec {
	return contract.SafetySpec{Effect: "read", Risk: "low", Confirmation: "not_required", Idempotency: "idempotent"}
}

func minutesMCPWriteSafety() contract.SafetySpec {
	return contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "idempotent"}
}

func minutesMCPCreateSafety() contract.SafetySpec {
	return contract.SafetySpec{Effect: "write", Risk: "medium", Confirmation: "user_required", Idempotency: "non_idempotent"}
}

func minutesMCPDeleteSafety() contract.SafetySpec {
	return contract.SafetySpec{Effect: "destructive", Risk: "high", Confirmation: "user_required", Idempotency: "idempotent"}
}

func minutesMCPInterface(tool string) *contract.InterfaceSpec {
	return &contract.InterfaceSpec{Mode: "mcp", Availability: "available", Ref: &contract.InterfaceRefSpec{ProductID: "minutes", RPCName: tool}}
}

func minutesMCPDryRun() *contract.DryRunSpec {
	return &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewRequest, RemoteReads: false}
}

func minutesMCPagination() *contract.PaginationSpec {
	return &contract.PaginationSpec{Kind: contract.PaginationKindCursor, CursorParameter: "cursor"}
}

func minutesMCPResultSpec(schema string) *contract.ResultSpec {
	return &contract.ResultSpec{Outcomes: []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}, DataSchema: json.RawMessage(`{"type":"object","oneOf":[` + schema + `,` + minutesMCPDryRunSchema + `]}`)}
}

func minutesMCPRelationResultSpec() *contract.ResultSpec {
	result := minutesMCPResultSpec(minutesTagRelationSchema)
	result.Outcomes = append(result.Outcomes, contract.ResultOutcomePartialFailure)
	result.DataSchema = json.RawMessage(`{"type":"object","oneOf":[` + minutesTagRelationSchema + `,` + minutesTagPartialSchema + `,` + minutesMCPDryRunSchema + `]}`)
	return result
}

func callMinutesMCPResult(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error) {
	// Omitted and explicitly empty briefIntro have different request presence
	// semantics. FlagSpec has no changed-only string mode; keep its declared
	// binding/value intact, omitting only the absent optional update field here.
	if tool == "update_custom_template" && !cmd.Flags().Changed("brief-intro") {
		delete(args, "briefIntro")
	}
	if corecmd.BoolFlag(cmd, "dry-run") || (deps != nil && deps.Caller != nil && deps.Caller.DryRun()) {
		return output.Success(map[string]any{"tool": tool, "arguments": args, "executed": false}, output.WithDryRun()), nil
	}
	text, err := CallMCPToolTextOnServerContext(cmd.Context(), "minutes", tool, args)
	if err != nil {
		// The shared text parser retains an outer ServiceResult error as raw
		// JSON in the legacy error. Reuse the established classifier to keep
		// permission/auth categories and upstream codes; never branch on prose.
		var legacy *CLIError
		if errors.As(err, &legacy) && legacy.Code == CodeMCPToolError {
			if classified := ClassifyMCPResponseText(legacy.Message); classified != nil {
				var projected *CLIError
				if errors.As(classified, &projected) {
					copy := *projected
					copy.Operation = "minutes/" + tool
					copy.Cause = err
					var body map[string]any
					if json.Unmarshal([]byte(legacy.Message), &body) == nil {
						copy.ServerCode = businessErrorCode(body)
						copy.Details = businessErrorDetails(body)
					}
					classified = &copy
				}
				err = classified
			}
		}
		if !strings.HasPrefix(tool, "list_") {
			err = minutesMCPNonRetryableError(err)
		}
		return nil, err
	}
	var envelope map[string]any
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	if err := decoder.Decode(&envelope); err != nil {
		return minutesMCPInvalidResult(tool, "响应不是JSON对象", nil), nil
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return minutesMCPInvalidResult(tool, "响应包含多个JSON值", nil), nil
	}
	if ok, valid := envelope["success"].(bool); !valid || !ok {
		return minutesMCPInvalidResult(tool, "响应缺少成功的ServiceResult", envelope), nil
	}
	data, ok := envelope["result"].(map[string]any)
	if !ok || data == nil {
		return minutesMCPInvalidResult(tool, "ServiceResult缺少result对象", envelope), nil
	}
	if tool == "list_system_templates" || tool == "list_custom_templates" {
		return minutesMCPTemplatePage(tool, data, args)
	}
	if tool == "add_tags_to_minutes" || tool == "remove_tags_from_minutes" {
		return minutesMCPTagRelation(tool, data, args)
	}
	if err := validateMinutesMCPWrite(tool, data, args); err != nil {
		return minutesMCPInvalidResult(tool, err.Error(), data), nil
	}
	// These DTO members are nullable. Some ServiceResult serializers omit
	// nulls; normalize only the two declared nullable fields, preserving their
	// unknown/no-personal-key meaning rather than inventing false or "".
	if tool == "delete_custom_template" {
		if _, exists := data["wasDefault"]; !exists {
			data["wasDefault"] = nil
		}
	}
	if tool == "set_default_template" {
		if _, exists := data["templateKey"]; !exists {
			data["templateKey"] = nil
		}
	}
	return output.Success(data), nil
}

func minutesMCPNonRetryableError(err error) error {
	var typed *apperrors.Error
	if errors.As(err, &typed) {
		copy := *typed
		copy.Retryable, copy.RetryableSet = false, true
		copy.RetryAfterSeconds, copy.NextRetryAt, copy.Actions = nil, nil, nil
		copy.Hint = minutesMCPWriteRecovery
		return &copy
	}
	var legacy *CLIError
	if errors.As(err, &legacy) {
		copy := *legacy
		copy.Suggestion = minutesMCPWriteRecovery
		return &copy
	}
	return err
}

func minutesMCPInvalidResult(tool, message string, receipt map[string]any) output.CommandResult {
	subtype := "malformed_tool_response"
	hint := "响应不符合听记工具契约，请保留诊断信息排查。"
	if !strings.HasPrefix(tool, "list_") {
		subtype, hint = "write_outcome_unknown", minutesMCPWriteRecovery
	}
	return output.Failure(&output.ErrorInfo{Type: "api", Subtype: subtype, Message: message, Operation: "minutes/" + tool, Origin: "mcp", Stage: "response_validation", Hint: hint, Details: map[string]any{"receipt": receipt}})
}

func minutesMCPTemplatePage(tool string, data, args map[string]any) (output.CommandResult, error) {
	items, ok := data["items"].([]any)
	if !ok {
		return minutesMCPInvalidResult(tool, "items必须为数组", data), nil
	}
	hasMore, ok := data["hasMore"].(bool)
	if !ok {
		return minutesMCPInvalidResult(tool, "hasMore必须为布尔值", data), nil
	}
	next := ""
	if raw := data["nextToken"]; raw != nil {
		var valid bool
		next, valid = raw.(string)
		if !valid {
			return minutesMCPInvalidResult(tool, "nextToken必须为字符串或null", data), nil
		}
	}
	if (hasMore && next == args["nextToken"]) || (!hasMore && next != "") {
		return minutesMCPInvalidResult(tool, "分页游标缺失、未前进或与hasMore矛盾", data), nil
	}
	for _, value := range items {
		item, ok := value.(map[string]any)
		if !ok || !minutesMCPString(item, "title") {
			return minutesMCPInvalidResult(tool, "模板条目或标题无效", data), nil
		}
		if tool == "list_system_templates" {
			_, briefOK := item["brief"].(string)
			if !minutesMCPString(item, "templateId") || !briefOK {
				return minutesMCPInvalidResult(tool, "系统模板标识或简介无效", data), nil
			}
		} else if !minutesMCPString(item, "templateKey") || !minutesMCPString(item, "prompt") || !minutesMCPVersion(item["version"]) {
			return minutesMCPInvalidResult(tool, "个人模板标识、提示词或版本无效", data), nil
		}
	}
	pagination, err := output.NewPagination(!hasMore, next)
	if err != nil {
		return minutesMCPInvalidResult(tool, err.Error(), data), nil
	}
	// NewPagination trims a generic token. Minutes continuation tokens are
	// opaque: retain the exact bytes after the nonblank/consistency checks.
	// Pagination.Validate checks usability without rewriting NextToken.
	pagination.NextToken = next
	pagination.Pages, pagination.Items = 1, len(items)
	return output.Success(map[string]any{"items": items}, output.WithMeta(&output.Meta{Count: output.NewCount(len(items)), Pagination: pagination})), nil
}

func minutesMCPString(data map[string]any, key string) bool {
	value, ok := data[key].(string)
	return ok && strings.TrimSpace(value) != ""
}

func minutesMCPVersion(raw any) bool {
	number, ok := raw.(json.Number)
	if !ok {
		return false
	}
	value, err := strconv.ParseInt(string(number), 10, 64)
	return err == nil && value >= 0
}

func validateMinutesMCPWrite(tool string, data, args map[string]any) error {
	if success, ok := data["success"].(bool); !ok || !success {
		return fmt.Errorf("业务回执未确认success=true")
	}
	switch tool {
	case "create_custom_template", "update_custom_template":
		if !minutesMCPString(data, "templateKey") || !minutesMCPString(data, "title") || !minutesMCPVersion(data["version"]) {
			return fmt.Errorf("模板标识、标题或版本无效")
		}
		if tool == "update_custom_template" && data["templateKey"] != args["templateKey"] {
			return fmt.Errorf("返回模板与请求不一致")
		}
	case "delete_custom_template":
		if !minutesMCPString(data, "templateKey") || data["templateKey"] != args["templateKey"] {
			return fmt.Errorf("返回模板与请求不一致")
		}
		if raw := data["wasDefault"]; raw != nil {
			if _, ok := raw.(bool); !ok {
				return fmt.Errorf("wasDefault必须为布尔值或null")
			}
		}
	case "set_default_template":
		if key, personal := args["templateKey"]; personal {
			if data["templateId"] != "diy" || data["templateKey"] != key {
				return fmt.Errorf("个人默认模板与请求不一致")
			}
		} else if data["templateKey"] != nil || data["templateId"] != args["templateId"] {
			return fmt.Errorf("系统默认模板回执无效，templateKey必须为null")
		}
	case "create_minutes_tag":
		if !minutesMCPString(data, "tagId") || !minutesMCPString(data, "displayName") {
			return fmt.Errorf("创建标签回执缺少tagId或实际displayName")
		}
	case "delete_minutes_tag":
		if !minutesMCPString(data, "tagId") || data["tagId"] != args["tagId"] {
			return fmt.Errorf("返回标签与请求不一致")
		}
	default:
		return fmt.Errorf("未声明的听记写工具")
	}
	return nil
}

func minutesMCPTagRelation(tool string, data, args map[string]any) (output.CommandResult, error) {
	items, ok := data["results"].([]any)
	ids, idsOK := args["tagIds"].([]string)
	success, successOK := data["success"].(bool)
	if !ok || !idsOK || !successOK || data["taskUuid"] != args["taskUuid"] || len(items) != len(ids) {
		return minutesMCPInvalidResult(tool, "标签回执缺少逐项结果或目标不一致", data), nil
	}
	succeeded := []any{}
	failed := []output.PartialFailedEntry{}
	unknown := []output.PartialUnknownEntry{}
	seenFailure := false
	for index, value := range items {
		item, ok := value.(map[string]any)
		if !ok || item["tagId"] != ids[index] {
			return minutesMCPInvalidResult(tool, "标签结果必须与去重请求同序且不重不漏", data), nil
		}
		switch item["status"] {
		case "APPLIED", "UNCHANGED":
			if seenFailure {
				return minutesMCPInvalidResult(tool, "标签结果在首个失败之后仍声称执行成功", data), nil
			}
			entry := make(map[string]any, len(item)+2)
			for key, value := range item {
				entry[key] = value
			}
			entry["id"], entry["taskUuid"] = ids[index], data["taskUuid"]
			succeeded = append(succeeded, entry)
		case "FAILED":
			if seenFailure || !minutesMCPString(item, "errorCode") || !minutesMCPString(item, "errorMessage") {
				return minutesMCPInvalidResult(tool, "FAILED条目重复或缺少诊断字段", data), nil
			}
			seenFailure = true
			unknown = append(unknown, output.PartialUnknownEntry{ID: ids[index], Reason: fmt.Sprintf("FAILED (%s): %s；写入结果未知，请先核对。", item["errorCode"], item["errorMessage"])})
		case "NOT_PROCESSED":
			if !seenFailure {
				return minutesMCPInvalidResult(tool, "NOT_PROCESSED之前缺少FAILED项", data), nil
			}
			started := false
			failed = append(failed, output.PartialFailedEntry{ID: ids[index], Error: &output.ErrorInfo{Type: "api", Subtype: "not_processed", Message: "前序项失败，本项未执行", Operation: "minutes/" + tool, ExecutionStarted: &started, RPCData: item, Hint: minutesMCPWriteRecovery}})
		default:
			return minutesMCPInvalidResult(tool, "未知标签结果状态", data), nil
		}
	}
	if success == seenFailure {
		return minutesMCPInvalidResult(tool, "业务success与逐项状态矛盾", data), nil
	}
	if success {
		return output.Success(data), nil
	}
	if len(succeeded) == 0 {
		return minutesMCPInvalidResult(tool, "没有标签项得到完成确认；FAILED可能已落库，后续项未执行", data), nil
	}
	partial, err := output.NewPartialData(len(ids), succeeded, failed, unknown)
	if err != nil {
		return minutesMCPInvalidResult(tool, err.Error(), data), nil
	}
	return output.Partial(partial), nil
}

func minutesMCPNoArgs(cmd *cobra.Command) {
	cmd.Args = cobra.NoArgs
}
