package helpers

import (
	"github.com/spf13/cobra"
)

func newRecruitCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "recruit",
		Short: "招聘 / 面试安排",
		Long:  `查询钉钉招聘面试安排：按候选人姓名搜索面试信息，支持分页。`,
		RunE:  groupRunE,
	}

	interviewCmd := &cobra.Command{Use: "interview", Short: "面试管理", RunE: groupRunE}

	interviewSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "按候选人姓名搜索面试",
		Example: `  dws recruit interview search --name "张三" --size 10
  dws recruit interview search --name "李四" --size 5 --cursor <cursor>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "name"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"name": mustGetFlag(cmd, "name"),
				"size": mustGetFlag(cmd, "size"),
			}
			if v, _ := cmd.Flags().GetString("cursor"); v != "" {
				toolArgs["cursor"] = v
			}
			return callMCPTool("search_interviews_by_candidate_name", toolArgs)
		},
	}

	interviewSearchCmd.Flags().String("name", "", "候选人姓名 (必填)")
	interviewSearchCmd.Flags().String("size", "10", "返回数量 (必填)")
	interviewSearchCmd.Flags().String("cursor", "", "分页游标")
	interviewCmd.AddCommand(interviewSearchCmd)
	root.AddCommand(interviewCmd)
	return root
}
