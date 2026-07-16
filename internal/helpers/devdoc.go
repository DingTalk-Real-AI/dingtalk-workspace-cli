package helpers

import (
	"strconv"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"
)

func newDevdocCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "devdoc",
		Short: "开放平台文档搜索",
		Long:  `搜索钉钉开放平台开发文档。默认以表格格式输出（标题、URL），使用 -f json 获取原始 JSON。`,
		RunE:  groupRunE,
	}

	articleCmd := &cobra.Command{Use: "article", Short: "文档文章", RunE: groupRunE}
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
	cli.AnnotateRuntimePositionals(cmd, cli.RuntimeSchemaPositional{
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
	return cmd
}
