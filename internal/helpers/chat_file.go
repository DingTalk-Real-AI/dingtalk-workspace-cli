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
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

// newChatFileGroup builds `dws chat file upload`. wukong implements it as a
// multi-step upload to the conversation file space: for --file it does
// init_conversation_file_upload (im) -> HTTP PUT -> commit_conversation_file_upload
// (im); for --url it calls upload_conversation_file_by_url (chat). The envelope
// cannot express the local pipeline, so it lives here. Wired into the chat
// handler (see chat.go).
func newChatFileGroup(runner executor.Runner) *cobra.Command {
	file := &cobra.Command{
		Use:               "file",
		Short:             "会话文件上传",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE:              func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	file.AddCommand(newChatFileUploadCommand(runner))
	return file
}

func newChatFileUploadCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload",
		Short: "上传本地文件或 URL 文件到会话文件空间",
		Example: "  dws chat file upload --group <openConversationId> --file ./report.pdf\n" +
			"  dws chat file upload --user <userId> --url https://example.com/a.pdf --file-name a.pdf",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := chatConversationTargetArgs(cmd)
			if err != nil {
				return err
			}
			filePath := strings.TrimSpace(firstNonEmptyFlag(cmd, "file", "file-path"))
			fileURL := strings.TrimSpace(firstNonEmptyFlag(cmd, "url"))
			if filePath == "" && fileURL == "" {
				return apperrors.NewValidation("--file or --url is required")
			}
			if filePath != "" && fileURL != "" {
				return apperrors.NewValidation("--file and --url are mutually exclusive")
			}
			fileName := strings.TrimSpace(firstNonEmptyFlag(cmd, "file-name"))
			md5v := strings.TrimSpace(firstNonEmptyFlag(cmd, "md5"))
			uuid := strings.TrimSpace(firstNonEmptyFlag(cmd, "uuid"))

			// URL path: server pulls the file (chat server).
			if fileURL != "" {
				if fileName == "" {
					fileName = filepath.Base(fileURL)
				}
				params := cloneStringAnyMap(target)
				params["fileUrl"] = fileURL
				params["fileName"] = fileName
				if md5v != "" {
					params["md5"] = md5v
				}
				if uuid != "" {
					params["uuid"] = uuid
				}
				inv := executor.NewHelperInvocation(cobracmd.LegacyCommandPath(cmd), "chat", "upload_conversation_file_by_url", params)
				if commandDryRun(cmd) {
					return writeCommandPayload(cmd, inv)
				}
				result, err := runner.Run(cmd.Context(), inv)
				if err != nil {
					return err
				}
				return writeCommandPayload(cmd, result)
			}

			// Local path: init (im) -> HTTP PUT -> commit (im).
			fi, err := os.Stat(filePath)
			if err != nil {
				return apperrors.NewValidation("cannot read file " + filePath + ": " + err.Error())
			}
			if fi.IsDir() {
				return apperrors.NewValidation(filePath + " is a directory, not a file")
			}
			if fileName == "" {
				fileName = filepath.Base(filePath)
			}
			fileSize := fi.Size()
			if md5v == "" {
				if md5v, err = fileMD5Hex(filePath); err != nil {
					return err
				}
			}
			if commandDryRun(cmd) {
				preview := cloneStringAnyMap(target)
				preview["fileName"] = fileName
				preview["fileSize"] = fileSize
				preview["md5"] = md5v
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "im", "init_conversation_file_upload", preview))
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Minute)
			defer cancel()

			initParams := cloneStringAnyMap(target)
			initParams["fileName"] = fileName
			initParams["fileSize"] = fileSize
			initParams["md5"] = md5v
			initRes, err := runner.Run(ctx, executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "im", "init_conversation_file_upload", initParams))
			if err != nil {
				return err
			}
			resourceURL := findStringDeep(initRes.Response, "resourceUrl", "resourceURL", "url")
			if resourceURL == "" {
				resourceURL = findFirstInStringArrayDeep(initRes.Response, "resourceUrls", "resourceURLs")
			}
			uploadKey := findStringDeep(initRes.Response, "uploadKey", "key")
			if resourceURL == "" || uploadKey == "" {
				return apperrors.NewAPI("incomplete upload credentials: resourceUrl=" + resourceURL + " uploadKey=" + uploadKey)
			}
			headers := findHeadersDeep(initRes.Response, "headers", "ossHeaders")
			if err := httpPutLocalFile(ctx, resourceURL, headers, filePath, fileSize); err != nil {
				return err
			}

			commitParams := cloneStringAnyMap(target)
			commitParams["uploadKey"] = uploadKey
			commitParams["fileName"] = fileName
			commitParams["fileSize"] = fileSize
			commitParams["md5"] = md5v
			if uuid != "" {
				commitParams["uuid"] = uuid
			}
			commitRes, err := runner.Run(ctx, executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "im", "commit_conversation_file_upload", commitParams))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, commitRes)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("group", "", "群聊 openConversationId（群聊时使用）")
	cmd.Flags().String("conversation-id", "", "--group 的别名")
	cmd.Flags().String("id", "", "--group 的别名")
	cmd.Flags().String("user", "", "单聊对方 userId（单聊时使用）")
	cmd.Flags().String("open-dingtalk-id", "", "单聊对方 openDingTalkId（单聊时使用）")
	cmd.Flags().String("file", "", "本地文件路径（与 --url 二选一）")
	cmd.Flags().String("file-path", "", "--file 的别名")
	cmd.Flags().String("url", "", "远程文件 URL（与 --file 二选一，服务端代传）")
	cmd.Flags().String("file-name", "", "文件名（可选）")
	cmd.Flags().String("md5", "", "文件 MD5（可选，本地不传自动计算）")
	cmd.Flags().String("uuid", "", "幂等 UUID（可选）")
	return cmd
}

func chatConversationTargetArgs(cmd *cobra.Command) (map[string]any, error) {
	group := strings.TrimSpace(firstNonEmptyFlag(cmd, "group", "conversation-id", "id"))
	user := strings.TrimSpace(firstNonEmptyFlag(cmd, "user"))
	openDingTalkID := strings.TrimSpace(firstNonEmptyFlag(cmd, "open-dingtalk-id"))
	if group == "" && user == "" && openDingTalkID == "" {
		return nil, apperrors.NewValidation("需指定会话目标：--group（群聊）或 --user / --open-dingtalk-id（单聊）之一")
	}
	m := map[string]any{}
	if group != "" {
		m["openConversationId"] = group
	}
	if user != "" {
		m["userId"] = user
	}
	if openDingTalkID != "" {
		m["openDingTalkId"] = openDingTalkID
	}
	return m, nil
}

func cloneStringAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+4)
	for k, v := range in {
		out[k] = v
	}
	return out
}
