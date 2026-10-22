// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

// OA 审批图片（DDPhotoField）上传命令：NewLeafCommand 完全托管声明（Tier1），
// 执行经 Call 钩子接入业务无关上传原语（image_upload.go 的 runImageUpload）。
//
// bizType 终态命名 approval_image；生产 upload_media 未放行前命令以服务端错误
// 如实失败，不做 bizType 回退（禁止以 chat_emoticon 借用形态发布命令）。
// 压缩策略：>2MB 自动压缩（复用表情压缩链、零改动），>10MB 拒绝。
var oaImageUploadSpec = imageUploadSpec{
	BizType:  "approval_image",
	MaxBytes: 2 * 1024 * 1024,
	Compress: true,
	Identity: contract.ToolIdentitySpec{
		ProductID:      "oa",
		Name:           "image_upload",
		CanonicalPath:  "oa.image_upload",
		CLIPath:        "oa approval image upload",
		PrimaryCLIPath: "oa approval image upload",
	},
	MediaUsage: "组装 DDPhotoField value（数组转义字符串）",
}

func newOAImageCommand() *cobra.Command {
	imageCmd := newGroupCommand(&cobra.Command{
		Use:   "image",
		Short: "审批图片（DDPhotoField）上传",
		RunE:  groupRunE,
	})
	imageCmd.AddCommand(NewLeafCommand(LeafSpec{
		Use:   "upload",
		Short: "上传本地图片供审批图片控件（DDPhotoField）使用",
		Long: `上传本地图片到钉钉文件服务（dingtalk-file/upload_media，bizType=approval_image），
返回 mediaIdV2，供组装审批图片控件（DDPhotoField）value（数组转义字符串）
随 oa approval create-instance 提交。

超过 2MB 的图片先自动压缩（有损，静图转为 jpg，GIF 保留动图帧）再上传，
超过 10MB 拒绝。`,
		Example:       `  dws oa approval image upload --file ./证据图.png --format json`,
		OutputRollout: output.RolloutUnifiedActive,
		Flags: []LeafFlag{
			{Name: "file", Usage: imageUploadFlagUsage(oaImageUploadSpec), MarkRequired: true},
		},
		Safety: contract.SafetySpec{
			Effect: "write", Risk: "low",
			Confirmation: "not_required", Idempotency: "unknown",
		},
		Contract: LeafContract{
			Identity:    oaImageUploadSpec.Identity,
			Description: "上传本地图片到钉钉文件服务（bizType=approval_image），返回 mediaIdV2 供组装审批图片控件（DDPhotoField）value",
			DryRun:      &contract.DryRunSpec{PreviewKind: "plan", RemoteReads: false},
			Result: &contract.ResultSpec{
				Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure},
				DataSchema: imageUploadDataSchema(oaImageUploadSpec),
			},
			Interface: &contract.InterfaceSpec{
				Mode:         "composite",
				Availability: "available",
				Reason:       "Reviewed unpinned remote adapter: 命令组合本地校验、图片压缩与 dingtalk-file/upload_media 调用，无可绑定的单一 interface_ref",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "上传本地图片取 mediaIdV2，供组装审批图片控件（DDPhotoField）value",
				UseWhen:      []string{"用户要把本地图片提交到审批图片控件（DDPhotoField），需要先上传取得 mediaIdV2 时"},
				AvoidWhen: []string{
					"上传审批附件（DDAttachment）改用 oa approval attachment upload",
					"图片已是公网 URL 时无需上传，直接组装 DDPhotoField value 提交",
				},
				Examples: []string{"dws oa approval image upload --file ./证据图.png --format json"},
			},
			Parameters: []contract.ParamDecl{
				{Name: "file", Required: boolPtr(true)},
			},
		},
		Call: func(cmd *cobra.Command, tool string, args map[string]any) error {
			return runImageUpload(oaImageUploadSpec, cmd)
		},
	}))
	return imageCmd
}
