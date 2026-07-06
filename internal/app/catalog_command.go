package app

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"
)

func newCatalogCommand(_ cli.CatalogLoader) *cobra.Command {
	return &cobra.Command{
		Use:               "catalog",
		Short:             "查看服务目录 (静态端点模式)",
		Hidden:            true,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
}
