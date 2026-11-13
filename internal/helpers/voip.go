package helpers

import (
	"encoding/json"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

// ──────────────────────────────────────────────────────────
// dws voip — VoIP 个人通话（1v1 音视频通话）
//
// 长尾产品：根命令 Hidden（不占 dws --help），叶子声明、直接调用与
// Schema 查询保持可用。当前接入工具：
//   set_voip_minutes_auto_start（通话纪要自动开启开关，会议云端 MCP 服务）
// ──────────────────────────────────────────────────────────

// voipServerID 是承载 VoIP 工具的钉钉视频会议云端 MCP 服务短名称，与
// internal/syncdata/endpoints.go 中的 ID 一致。
const voipServerID = "conference"

func newVoipCommand() *cobra.Command {
	contract.RegisterProductDecl(contract.ProductDecl{
		ID: "voip",
		HelpReferences: contract.HelpReferences{
			RelatedSkills: []string{"dingtalk-misc", "dingtalk-event"},
			Documentation: []contract.HelpDocumentation{
				contract.SkillDocumentation("VoIP 通话命令参考", "dingtalk-misc", "references/voip.md"),
				contract.SkillDocumentation("VoIP 通话邀请事件参考", "dingtalk-event", "references/event-voip.md"),
			},
		},
		Selection: contract.ProductSelectionDecl{
			AgentSummary: "钉钉 VoIP 个人通话（1v1 音视频通话）能力：通话纪要自动开启设置",
			UseWhen: []string{
				"需要开启或关闭当前用户的 VoIP 通话纪要自动开启",
			},
			AvoidWhen: []string{
				"查询或导出听记内容用 dws minutes，不要用本产品",
				"监听 VoIP 通话邀请事件用 dws event，不要用本产品",
				"视频会议预约/邀请/会中控制不要用本产品",
			},
		},
	})

	root := newGroupCommand(&cobra.Command{
		Use:    "voip",
		Short:  "VoIP 通话 / 个人语音视频通话",
		Hidden: true,
		Long:   "钉钉 VoIP 个人通话（1v1 音视频通话）能力。当前提供通话纪要自动开启设置。",
		RunE:   groupRunE,
	})

	minutesCmd := newGroupCommand(&cobra.Command{
		Use:   "minutes",
		Short: "通话纪要设置",
		Long:  "VoIP 通话（1v1 音视频通话）纪要相关设置。",
		RunE:  groupRunE,
	})
	minutesCmd.AddCommand(newVoipMinutesAutoStartCommand())
	root.AddCommand(minutesCmd)

	return root
}

var voipMinutesAutoStartResult = &contract.ResultSpec{
	Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
	DataSchema: json.RawMessage(`{"type":"object","description":"VoIP 通话纪要自动开启设置结果","properties":{"success":{"type":"boolean","description":"设置是否成功"},"errorCode":{"type":"string","description":"错误码，成功时为空"},"errorMsg":{"type":"string","description":"错误信息，成功时为空"}}}`),
}

func newVoipMinutesAutoStartCommand() *cobra.Command {
	return NewLeafCommand(LeafSpec{
		Use:   "auto-start",
		Short: "开启/关闭个人 VoIP 通话纪要自动开启",
		Long: `开启或关闭当前用户的 VoIP 通话（1v1 音视频通话）纪要自动开启开关。
开启后该用户发起的通话接通时将自动开启通话纪要。目标用户由服务端按当前登录身份确定，无需传入 uid。
--enable 必填：true 开启，false 关闭（关闭需显式传 --enable=false）。`,
		Example:       "  dws voip minutes auto-start --enable\n  dws voip minutes auto-start --enable=false",
		OutputRollout: output.RolloutUnifiedActive,
		Server:        voipServerID,
		Tool:          "set_voip_minutes_auto_start",
		ResultCall:    voipMinutesAutoStartCall,
		Flags: []corecmd.FlagSpec{
			{Name: "enable", Kind: corecmd.KindBool, MarkRequired: true, Bind: "autoStart", Usage: "true 开启、false 关闭（必填；关闭需显式传 --enable=false）"},
		},
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "medium",
			Confirmation: "user_required", Idempotency: "idempotent",
		},
		Contract: corecmd.ContractDecl{
			DryRun: &contract.DryRunSpec{PreviewKind: contract.DryRunPreviewInvocation, RemoteReads: false},
			Identity: contract.ToolIdentitySpec{
				ProductID: "voip", Name: "set_voip_minutes_auto_start", Group: "minutes",
				CanonicalPath:  "voip.set_voip_minutes_auto_start",
				CLIPath:        "voip minutes auto-start",
				PrimaryCLIPath: "voip minutes auto-start",
			},
			Description: "开启或关闭当前用户 VoIP 通话纪要自动开启",
			Result:      voipMinutesAutoStartResult,
			Interface: &contract.InterfaceSpec{
				Mode: contract.InterfaceModeMCP, Availability: contract.InterfaceAvailable,
				Ref: &contract.InterfaceRefSpec{ProductID: "conference", RPCName: "set_voip_minutes_auto_start"},
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "设置当前用户 VoIP 通话纪要自动开启的开关",
				UseWhen:      []string{"用户要开启或关闭个人 VoIP 通话的通话纪要自动开启时"},
				AvoidWhen: []string{
					"查询或导出听记内容用 dws minutes",
					"监听 VoIP 通话邀请事件用 dws event consume user_voip_call_receive_invite",
				},
				Examples: []string{
					"dws voip minutes auto-start --enable",
					"dws voip minutes auto-start --enable=false",
				},
			},
			Parameters: []contract.ParamDecl{
				{Name: "enable", Property: "autoStart", Required: boolPtr(true)},
			},
		},
	})
}

// voipMinutesAutoStartCall 是统一结果执行体：经 helpers 统一 MCP 调用通道
// 发起调用并保留服务端原始返回形状。dry-run 只在本地返回请求预览，
// 不调用 ToolCaller。
func voipMinutesAutoStartCall(cmd *cobra.Command, tool string, args map[string]any) (output.CommandResult, error) {
	caller := GetCaller()
	if caller == nil {
		return nil, apperrors.NewInternal("MCP caller is not initialized")
	}
	if corecmd.BoolFlag(cmd, "dry-run") || caller.DryRun() {
		return output.Success(map[string]any{
			"tool": tool, "arguments": args, "executed": false,
		}, output.WithDryRun()), nil
	}
	data, err := CallMCPToolDataOnServer(cmd.Context(), voipServerID, tool, args)
	if err != nil {
		return nil, err
	}
	return output.Success(data), nil
}
