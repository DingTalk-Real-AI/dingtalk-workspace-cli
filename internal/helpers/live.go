package helpers

import (
	"github.com/spf13/cobra"
)

func newLiveCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "live",
		Short: "直播列表 / 信息",
		Long:  `查看钉钉直播：列出我的直播记录。`,
		RunE:  groupRunE,
	}

	streamCmd := &cobra.Command{Use: "stream", Short: "直播流管理", RunE: groupRunE}

	streamListCmd := &cobra.Command{
		Use:     "list",
		Short:   "查看我的直播列表",
		Example: `  dws live stream list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("get_my_lives", nil)
		},
	}

	streamCmd.AddCommand(streamListCmd)
	root.AddCommand(streamCmd)
	return root
}
