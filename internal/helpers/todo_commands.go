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
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/spf13/cobra"
)

// newTodoAddAttachmentCommand builds the hardcoded `todo task add-attachment`
// leaf. The MCP backend exposes attachment upload as a multi-step orchestration
// that the envelope/pipeline layer cannot express (no upload step, local file
// IO): init upload credentials -> HTTP PUT the local file -> commit -> add.
// wukong implements it in code; this is the open-edition equivalent. It is
// wired into the existing todo handler's task group (see todo.go).
func newTodoAddAttachmentCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add-attachment",
		Short:   i18n.T("上传待办附件"),
		Long:    i18n.T("上传本地文件作为待办附件（init → 上传 → commit → add 四步）。会真实上传文件，请确认待办存在。"),
		Example: "  dws todo task add-attachment --task-id <taskId> --file-path <filePath>",
		Args:    cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := strings.TrimSpace(firstNonEmptyFlag(cmd, "task-id", "id"))
			filePath := strings.TrimSpace(firstNonEmptyFlag(cmd, "file-path", "file"))
			if taskID == "" {
				return apperrors.NewValidation("missing required flag(s): --task-id")
			}
			if filePath == "" {
				return apperrors.NewValidation("missing required flag(s): --file-path")
			}
			fi, err := os.Stat(filePath)
			if err != nil {
				return apperrors.NewValidation(fmt.Sprintf("cannot read file %s: %v", filePath, err))
			}
			if fi.IsDir() {
				return apperrors.NewValidation(fmt.Sprintf("%s is a directory, not a file", filePath))
			}
			fileName := filepath.Base(filePath)
			fileType := strings.TrimPrefix(filepath.Ext(fileName), ".")
			fileSize := fi.Size()
			md5Hex, err := fileMD5Hex(filePath)
			if err != nil {
				return err
			}

			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, executor.NewHelperInvocation(
					cobracmd.LegacyCommandPath(cmd), "todo", "add_todo_attachment", map[string]any{
						"todoAttachmentAddRequest": map[string]any{
							"taskId":   taskID,
							"fileName": fileName, "fileSize": fileSize, "md5": md5Hex,
						},
					}))
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Minute)
			defer cancel()

			// 1) init upload credentials
			initRes, err := runner.Run(ctx, executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "todo", "init_todo_file_upload", map[string]any{
					"todoAttachmentInitUploadInfoRequest": map[string]any{
						"fileName": fileName, "fileSize": fileSize, "md5": md5Hex,
					},
				}))
			if err != nil {
				return err
			}
			resourceURL := findStringDeep(initRes.Response, "resourceUrl", "resourceURL", "url")
			if resourceURL == "" {
				resourceURL = findFirstInStringArrayDeep(initRes.Response, "resourceUrls", "resourceURLs")
			}
			uploadKey := findStringDeep(initRes.Response, "uploadKey", "key")
			if resourceURL == "" || uploadKey == "" {
				return apperrors.NewAPI(fmt.Sprintf("incomplete upload credentials: resourceUrl=%q uploadKey=%q", resourceURL, uploadKey))
			}
			headers := findHeadersDeep(initRes.Response, "headers", "ossHeaders")

			// 2) PUT the local file to the resource URL
			if err := httpPutLocalFile(ctx, resourceURL, headers, filePath, fileSize); err != nil {
				return err
			}

			// 3) commit upload
			commitRes, err := runner.Run(ctx, executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "todo", "commit_todo_file_upload", map[string]any{
					"todoAttachmentCommitUploadInfoRequest": map[string]any{
						"uploadKey": uploadKey, "fileName": fileName, "fileSize": fileSize, "md5": md5Hex,
					},
				}))
			if err != nil {
				return err
			}
			dentryID := findInt64Deep(commitRes.Response, "dentryId", "dentryID")
			spaceID := findInt64Deep(commitRes.Response, "spaceId", "spaceID")
			if dentryID == 0 || spaceID == 0 {
				return apperrors.NewAPI("uploaded file response missing dentryId or spaceId")
			}

			// 4) add attachment to the todo
			addRes, err := runner.Run(ctx, executor.NewHelperInvocation(
				cobracmd.LegacyCommandPath(cmd), "todo", "add_todo_attachment", map[string]any{
					"todoAttachmentAddRequest": map[string]any{
						"taskId": taskID,
						"attachmentList": []any{map[string]any{
							"fileId":   strconv.FormatInt(dentryID, 10),
							"fileName": fileName,
							"fileSize": fileSize,
							"spaceId":  strconv.FormatInt(spaceID, 10),
							"fileType": fileType,
						}},
					},
				}))
			if err != nil {
				return err
			}
			return writeCommandPayload(cmd, addRes)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("task-id", "", i18n.T("待办任务 ID (必填)"))
	cmd.Flags().String("id", "", i18n.T("--task-id 的别名"))
	cmd.Flags().String("file-path", "", i18n.T("本地文件路径 (必填)"))
	cmd.Flags().String("file", "", i18n.T("--file-path 的别名"))
	return cmd
}

func fileMD5Hex(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", apperrors.NewValidation(fmt.Sprintf("cannot open file %s: %v", path, err))
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", apperrors.NewInternal(fmt.Sprintf("md5 of %s: %v", path, err))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func httpPutLocalFile(ctx context.Context, url string, headers map[string]string, path string, size int64) error {
	f, err := os.Open(path)
	if err != nil {
		return apperrors.NewValidation(fmt.Sprintf("cannot open file %s: %v", path, err))
	}
	defer f.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, f)
	if err != nil {
		return apperrors.NewInternal(fmt.Sprintf("build PUT request: %v", err))
	}
	req.ContentLength = size
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return apperrors.NewAPI(fmt.Sprintf("upload PUT failed: %v", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return apperrors.NewAPI(fmt.Sprintf("upload PUT returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
	}
	return nil
}

// findStringDeep recursively searches a decoded JSON value for the first
// non-empty string value under any of the given keys.
func findStringDeep(v any, keys ...string) string {
	switch t := v.(type) {
	case map[string]any:
		for _, k := range keys {
			if s, ok := scalarString(t[k]); ok && s != "" {
				return s
			}
		}
		for _, child := range t {
			if s := findStringDeep(child, keys...); s != "" {
				return s
			}
		}
	case []any:
		for _, child := range t {
			if s := findStringDeep(child, keys...); s != "" {
				return s
			}
		}
	}
	return ""
}

// findFirstInStringArrayDeep recursively finds the first non-empty string in an
// array stored under any of the given keys (e.g. "resourceUrls").
func findFirstInStringArrayDeep(v any, keys ...string) string {
	switch t := v.(type) {
	case map[string]any:
		for _, k := range keys {
			if arr, ok := t[k].([]any); ok {
				for _, e := range arr {
					if s, ok := scalarString(e); ok && s != "" {
						return s
					}
				}
			}
		}
		for _, child := range t {
			if s := findFirstInStringArrayDeep(child, keys...); s != "" {
				return s
			}
		}
	case []any:
		for _, child := range t {
			if s := findFirstInStringArrayDeep(child, keys...); s != "" {
				return s
			}
		}
	}
	return ""
}

func findInt64Deep(v any, keys ...string) int64 {
	switch t := v.(type) {
	case map[string]any:
		for _, k := range keys {
			if n, ok := scalarInt64(t[k]); ok && n != 0 {
				return n
			}
		}
		for _, child := range t {
			if n := findInt64Deep(child, keys...); n != 0 {
				return n
			}
		}
	case []any:
		for _, child := range t {
			if n := findInt64Deep(child, keys...); n != 0 {
				return n
			}
		}
	}
	return 0
}

func findHeadersDeep(v any, keys ...string) map[string]string {
	out := map[string]string{}
	switch t := v.(type) {
	case map[string]any:
		for _, k := range keys {
			if h, ok := t[k].(map[string]any); ok {
				for name, val := range h {
					if s, ok := scalarString(val); ok && s != "" {
						out[name] = s
					}
				}
			}
		}
		if len(out) == 0 {
			for _, child := range t {
				if h := findHeadersDeep(child, keys...); len(h) > 0 {
					return h
				}
			}
		}
	}
	return out
}

func scalarString(v any) (string, bool) {
	switch s := v.(type) {
	case string:
		return s, true
	case json.Number:
		return s.String(), true
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64), true
	}
	return "", false
}

func scalarInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return i, true
		}
		if f, err := n.Float64(); err == nil {
			return int64(f), true
		}
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case string:
		if i, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64); err == nil {
			return i, true
		}
	}
	return 0, false
}
