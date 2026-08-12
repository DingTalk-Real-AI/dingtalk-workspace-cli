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
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

const chatMediaUploadReplacement = "dws chat message send --msg-type file --file-path <本地路径>"

func newChatMediaGroup() *cobra.Command {
	media := &cobra.Command{
		Use:               "media",
		Short:             "已下线：媒体文件上传兼容入口",
		Deprecated:        "本地图片和文件请改用 " + chatMediaUploadReplacement,
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(*cobra.Command, []string) error {
			return chatMediaUploadDownlineError()
		},
	}
	media.AddCommand(newChatMediaUploadCommand())
	return media
}

func newChatMediaUploadCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:        "upload",
		Short:      "已下线：请通过 chat message send 直接发送本地文件",
		Deprecated: "请改用 " + chatMediaUploadReplacement,
		Long: `此命令仅为 1.x 命令行兼容保留，不再读取应用凭证或调用旧版媒体上传接口。

发送本地图片或文件时，请使用 chat message send --msg-type file --file-path。
该路径会把图片作为可下载的 file 消息发送，不会生成 mediaId，也不会渲染成内联 image 消息。
如果上游已经提供 mediaId，仍可使用 chat message send --msg-type image --media-id。`,
		Example: "  dws chat message send --group <openConversationId> --msg-type file --file-path ./screenshot.png\n" +
			"  dws chat message send --open-dingtalk-id <openDingTalkId> --msg-type file --file-path ./report.pdf",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(*cobra.Command, []string) error {
			return chatMediaUploadDownlineError()
		},
	}
	// Keep the historical flags so existing argv remains parseable while the
	// 1.x compatibility command returns an actionable migration error.
	cmd.Flags().String("file", "", "旧版兼容参数；本地文件请改用 chat message send --file-path")
	cmd.Flags().String("type", "image", "旧版兼容参数；不再执行媒体上传")
	return cmd
}

func chatMediaUploadDownlineError() error {
	return apperrors.NewValidation(
		"chat media upload 已下线，当前 CLI 不提供本地文件到 mediaId 的上传能力。" +
			" 本地图片或文件请改用: " + chatMediaUploadReplacement +
			"；已有 mediaId 时可使用 dws chat message send --msg-type image --media-id <mediaId>。",
	)
}
