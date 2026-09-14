package helpers

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/spf13/cobra"
)

func newLiveCommand() *cobra.Command {
	// Product-level Agent routing Decl (migrated from selection/live.json
	// products.live). Catalog assembly stamps provenance contract_final.
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "live",
		HelpReferences: contract.HelpReferences{
			RelatedSkills: []string{"dingtalk-misc"},
			Documentation: []contract.HelpDocumentation{
				contract.SkillDocumentation("直播深度指南", "dingtalk-misc", "references/live.md"),
			},
		},
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "查询当前用户发起的直播列表",
			UseWhen: []string{
				"用户要查看自己的直播列表或基础统计",
			},
			AvoidWhen: []string{
				"当前公开面不支持创建/控制直播",
			},
		},
	})
	root := newGroupCommand(&cobra.Command{
		Use:   "live",
		Short: "直播列表 / 信息",
		Long:  `查看钉钉直播：列出我的直播记录。`,
		RunE:  groupRunE,
	})

	streamCmd := newGroupCommand(&cobra.Command{Use: "stream", Short: "直播流管理", RunE: groupRunE})

	streamListCmd := &cobra.Command{
		Use:     "list",
		Short:   "查看我的直播列表",
		Example: `  dws live stream list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPTool("get_my_lives", nil)
		},
	}
	DeclareLeafMetadata(streamListCmd, LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "live",
				Name:           "get_my_lives",
				CanonicalPath:  "live.get_my_lives",
				CLIPath:        "live stream list",
				PrimaryCLIPath: "live stream list",
			},
			Description: "查看当前用户发起的直播列表与基础统计",
			Interface: &contract.InterfaceSpec{
				Mode:         "mcp",
				Availability: "available",
				Ref:          &contract.InterfaceRefSpec{ProductID: "live", RPCName: "get_my_lives"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查看当前用户发起的直播列表与基础统计",
				UseWhen:      []string{"用户要看自己发起过的直播、状态或观看量等列表信息"},
				AvoidWhen:    []string{"需要创建/开播/结束直播时不要使用；当前公开面仅列表查询"},
				Examples:     []string{"dws live stream list"},
			},
		},
	})

	streamCmd.AddCommand(streamListCmd)
	root.AddCommand(streamCmd)
	return root
}
