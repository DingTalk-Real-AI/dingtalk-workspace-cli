package helpers

import (
	"github.com/spf13/cobra"
)

func newDocparseCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "docparse",
		Short: "文档解析（PDF/图片转 Markdown）",
		Long:  `使用合合信息文档解析引擎，将 PDF、图片等文件解析为 Markdown 格式。`,
		RunE:  groupRunE,
	}

	convertCmd := &cobra.Command{
		Use:   "convert",
		Short: "解析文件内容为 Markdown",
		Long: `根据文件链接，解析文件内容，转换成 Markdown 格式。

支持 PDF、图片等多种格式。文件必须是可公开访问的 URL。`,
		Example: `  dws docparse convert --file "https://example.com/report.pdf"
  dws docparse convert --file "https://cdn.example.com/image.png"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "file"); err != nil {
				return err
			}
			return callMCPTool("parse_pdf_to_markdown", map[string]any{
				"file": mustGetFlag(cmd, "file"),
			})
		},
	}

	convertCmd.Flags().String("file", "", "文件 URL (必填，需可公开访问)")

	root.AddCommand(convertCmd)
	return root
}
