package helpers

import (
	"strconv"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

func newDevdocCommand() *cobra.Command {
	// Product-level Agent routing Decl (migrated from selection/devdoc.json
	// products.devdoc). Catalog assembly stamps provenance contract_final.
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "devdoc",
		HelpReferences: contract.HelpReferences{
			RelatedSkills: []string{"dingtalk-misc"},
			Documentation: []contract.HelpDocumentation{
				contract.SkillDocumentation("开放平台文档搜索指南", "dingtalk-misc", "references/devdoc.md"),
			},
		},
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "搜索钉钉开放平台开发文档与错误排查资料",
			UseWhen: []string{
				"查询 OpenAPI、字段、错误码、SDK、鉴权或接入指南",
			},
			AvoidWhen: []string{
				"搜索用户业务文档或读写业务数据时不要用 devdoc",
			},
		},
	})
	root := newGroupCommand(&cobra.Command{
		Use:   "devdoc",
		Short: "开放平台文档搜索",
		Long:  `搜索钉钉开放平台开发文档。默认以表格格式输出（标题、URL），使用 -f json 获取原始 JSON。`,
		RunE:  groupRunE,
	})

	articleCmd := newGroupCommand(&cobra.Command{Use: "article", Short: "文档文章", RunE: groupRunE})
	articleCmd.AddCommand(newDevdocArticleSearchCommand())
	root.AddCommand(articleCmd)
	root.AddCommand(hintSubCmd("search", "use: dws devdoc article search --query <关键词>"))
	return root
}

func newDevdocArticleSearchCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search [keyword]",
		Short: "搜索开放平台文档",
		Long:  `按关键词搜索 open.dingtalk.com 文档，支持分页。默认表格输出，可用 -f json 获取完整响应。`,
		Example: `  dws devdoc article search "MCP"
  dws devdoc article search --query "MCP" --page 1 --size 10
  dws devdoc article search --query "openConversationId" --page 2 --size 5`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && flagOrFallback(cmd, "query", "keyword") == "" {
				_ = cmd.Flags().Set("query", args[0])
			}
			if err := validateRequiredFlagWithAliases(cmd, "query", "keyword"); err != nil {
				return err
			}
			page, _ := strconv.Atoi(mustGetFlag(cmd, "page"))
			if page < 1 {
				page = 1
			}
			size, _ := strconv.Atoi(mustGetFlag(cmd, "size"))
			if size < 1 {
				size = 10
			}
			return callMCPTool("search_open_platform_docs", map[string]any{
				"keyword": flagOrFallback(cmd, "query", "keyword"),
				"page":    page,
				"size":    size,
			})
		},
	}
	cmd.Flags().String("query", "", "搜索关键词 (必填)")
	cmd.Flags().String("keyword", "", "搜索关键词 (--query 的别名)")
	_ = cmd.Flags().MarkHidden("keyword")
	cmd.Flags().String("page", "1", "页码，默认 1")
	cmd.Flags().String("size", "10", "每页数量，默认 10")
	cli.AnnotateRuntimePositionals(cmd, contract.RuntimeSchemaPositional{
		// Keep the positional identity aligned with Cobra's authoritative
		// `search [keyword]` contract. The public --query flag is the other
		// member of the Schema require-one-of group; the hidden --keyword
		// compatibility flag is deliberately not published as a parameter.
		Name:        "keyword",
		Type:        "string",
		Description: "搜索关键词；也可通过 --query 传入",
		Required:    false,
		Index:       0,
	})
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "devdoc",
				Name:           "search_open_platform_docs_rag",
				CanonicalPath:  "devdoc.search_open_platform_docs_rag",
				CLIPath:        "devdoc article search",
				PrimaryCLIPath: "devdoc article search",
			},
			Description: "搜索钉钉开放平台开发文档，返回资料与链接（不生成分析答案）",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "devdoc", RPCName: "search_open_platform_docs"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "搜索钉钉开放平台开发文档，返回资料与链接（不生成分析答案）",
				UseWhen:      []string{"查 OpenAPI、字段、错误码、OAuth2、接入指南等开放平台开发问题"},
				AvoidWhen: []string{
					"搜索用户业务文档用 drive/wiki/doc，不要用 devdoc",
					"要执行开放平台应用配置变更时用 dev",
				},
				Examples: []string{
					"dws devdoc article search --query \"OAuth2 接入\" --format json",
					"dws devdoc article search --query \"errcode 40078\" --format json",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "query", Property: "keyword", Required: boolPtr(false)},
				{Name: "page", Property: "page", InterfaceType: "number"},
				{Name: "size", Property: "size", InterfaceType: "number"},
			},
		},
	})
	return cmd
}

// newDevDocSearchCommand is the `dws dev doc search` surface — same execution
// body as devdoc article search, but ContractFinal examples must use the
// reviewed primary path for canonical dev.search_open_platform_docs_rag.
func newDevDocSearchCommand(_ ...executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search [keyword]",
		Short: "搜索开放平台文档",
		Long:  `按关键词搜索 open.dingtalk.com 文档，支持分页。默认表格输出，可用 -f json 获取完整响应。`,
		Example: `  dws dev doc search "MCP"
  dws dev doc search --query "MCP" --page 1 --size 10`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && flagOrFallback(cmd, "query", "keyword") == "" {
				_ = cmd.Flags().Set("query", args[0])
			}
			if err := validateRequiredFlagWithAliases(cmd, "query", "keyword"); err != nil {
				return err
			}
			page, _ := strconv.Atoi(mustGetFlag(cmd, "page"))
			if page < 1 {
				page = 1
			}
			size, _ := strconv.Atoi(mustGetFlag(cmd, "size"))
			if size < 1 {
				size = 10
			}
			return callMCPTool("search_open_platform_docs", map[string]any{
				"keyword": flagOrFallback(cmd, "query", "keyword"),
				"page":    page,
				"size":    size,
			})
		},
	}
	cmd.Flags().String("query", "", "搜索关键词 (必填)")
	cmd.Flags().String("keyword", "", "搜索关键词 (--query 的别名)")
	_ = cmd.Flags().MarkHidden("keyword")
	cmd.Flags().String("page", "1", "页码，默认 1")
	cmd.Flags().String("size", "10", "每页数量，默认 10")
	cli.AnnotateRuntimePositionals(cmd, contract.RuntimeSchemaPositional{
		Name:        "keyword",
		Type:        "string",
		Description: "搜索关键词；也可通过 --query 传入",
		Required:    false,
		Index:       0,
	})
	DeclareLeafMetadata(cmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "dev",
				Name:           "search_open_platform_docs_rag",
				CanonicalPath:  "dev.search_open_platform_docs_rag",
				CLIPath:        "dev doc search",
				PrimaryCLIPath: "dev doc search",
			},
			Description: "通过 dev 兼容入口搜索开放平台文档",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "devdoc", RPCName: "search_open_platform_docs"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "搜索钉钉开放平台官方文档",
				UseWhen:      []string{"需要查询开放平台 API、参数、权限点或错误码时"},
				AvoidWhen:    []string{"已有确定义务命令可直接完成任务时"},
				Examples:     []string{"dws dev doc search --query \"MCP\" --size 10"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "query", Property: "keyword"},
				{Name: "page", Property: "page", InterfaceType: "number"},
				{Name: "size", Property: "size", InterfaceType: "number"},
			},
		},
	})
	return cmd
}
