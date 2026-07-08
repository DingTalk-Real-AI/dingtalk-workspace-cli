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
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/spf13/cobra"
)

func newChatMediaGroup() *cobra.Command {
	media := &cobra.Command{
		Use:               "media",
		Short:             "媒体文件管理",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE:              func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	media.AddCommand(newChatMediaUploadCommand())
	return media
}

func newChatMediaUploadCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload",
		Short: "上传图片获取 mediaId（用于 chat message send --msg-type image）",
		Example: "  dws chat media upload --file ./screenshot.png\n" +
			"  dws chat media upload --file ./photo.jpg --type image",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath, _ := cmd.Flags().GetString("file")
			filePath = strings.TrimSpace(filePath)
			if filePath == "" {
				return apperrors.NewValidation("--file is required")
			}
			fi, err := os.Stat(filePath)
			if err != nil {
				return apperrors.NewValidation("cannot read file: " + err.Error())
			}
			if fi.IsDir() {
				return apperrors.NewValidation(filePath + " is a directory")
			}
			mediaType, _ := cmd.Flags().GetString("type")
			mediaType = strings.TrimSpace(strings.ToLower(mediaType))
			if mediaType == "" {
				mediaType = "image"
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
			defer cancel()

			token, err := mediaResolveAppToken(ctx)
			if err != nil {
				return err
			}

			mediaID, err := mediaUploadFile(ctx, token, filePath, mediaType)
			if err != nil {
				return err
			}

			return writeCommandPayload(cmd, map[string]any{
				"success": true,
				"mediaId": mediaID,
			})
		},
	}
	cmd.Flags().String("file", "", "本地文件路径 (必填)")
	cmd.Flags().String("type", "image", "媒体类型: image/voice/video/file")
	return cmd
}

func mediaResolveAppToken(ctx context.Context) (string, error) {
	appKey := os.Getenv("DWS_CLIENT_ID")
	appSecret := os.Getenv("DWS_CLIENT_SECRET")
	if appKey == "" || appSecret == "" {
		return "", apperrors.NewAuth(
			"缺少应用凭证。chat media upload 需要 DWS_CLIENT_ID / DWS_CLIENT_SECRET 环境变量。\n" +
				"请使用 dws auth login --client-id <APP_KEY> --client-secret <APP_SECRET> 登录。")
	}
	url := "https://oapi.dingtalk.com/gettoken?appkey=" + appKey + "&appsecret=" + appSecret
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "", apperrors.NewAuth("获取访问令牌失败: " + err.Error())
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return "", apperrors.NewAuth(fmt.Sprintf("获取访问令牌 HTTP %d: %s", resp.StatusCode, string(raw)))
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", apperrors.NewAuth("gettoken 响应解析失败: " + string(raw))
	}
	if parsed.ErrCode != 0 || parsed.AccessToken == "" {
		return "", apperrors.NewAuth(fmt.Sprintf("gettoken errcode=%d errmsg=%s", parsed.ErrCode, parsed.ErrMsg))
	}
	return parsed.AccessToken, nil
}

func mediaUploadFile(ctx context.Context, token, filePath, mediaType string) (string, error) {
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		defer pw.Close()
		defer writer.Close()
		part, err := writer.CreateFormFile("media", filepath.Base(filePath))
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		f, err := os.Open(filePath)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		defer f.Close()
		if _, err := io.Copy(part, f); err != nil {
			pw.CloseWithError(err)
		}
	}()

	url := "https://oapi.dingtalk.com/media/upload?access_token=" + token + "&type=" + mediaType
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, pr)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
	if err != nil {
		return "", apperrors.NewAPI("media upload failed: " + err.Error())
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return "", apperrors.NewAPI(fmt.Sprintf("media upload HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
	}

	var parsed struct {
		MediaID string `json:"media_id"`
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", apperrors.NewAPI("media upload 响应解析失败: " + string(body))
	}
	if parsed.ErrCode != 0 || strings.TrimSpace(parsed.MediaID) == "" {
		return "", apperrors.NewAPI(fmt.Sprintf("media upload errcode=%d errmsg=%s body=%s", parsed.ErrCode, parsed.ErrMsg, string(body)))
	}
	return strings.TrimSpace(parsed.MediaID), nil
}
