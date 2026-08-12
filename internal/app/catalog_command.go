package app

import (
	"github.com/spf13/cobra"
)

func newCatalogCommand() *cobra.Command {
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
