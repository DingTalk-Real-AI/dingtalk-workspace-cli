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

package app

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/apiclient"
	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const reportCreateOAPIURL = "https://oapi.dingtalk.com/topapi/report/create"

type reportSenderAPICaller interface {
	Do(context.Context, apiclient.RawAPIRequest) (*apiclient.RawAPIResponse, error)
}

type reportSenderOAPISubmitter struct {
	resolveToken func(context.Context) (string, error)
	newClient    func(string) reportSenderAPICaller
}

func newReportSenderOAPISubmitter() *reportSenderOAPISubmitter {
	return &reportSenderOAPISubmitter{
		resolveToken: resolveReportSenderToken,
		newClient: func(token string) reportSenderAPICaller {
			return apiclient.NewClient(token, apiclient.LegacyBaseURL)
		},
	}
}

func (s *reportSenderOAPISubmitter) Submit(ctx context.Context, cmd *cobra.Command, submission helpers.ReportSenderSubmission) error {
	body, err := helpers.BuildReportCreateOAPIRequest(submission)
	if err != nil {
		return err
	}
	request := apiclient.RawAPIRequest{
		Method: http.MethodPost,
		Path:   reportCreateOAPIURL,
		Data:   body,
	}

	// Dry-run intentionally happens before token resolution. The preview never
	// fetches, caches, or prints an application access token.
	if submission.DryRun || reportCommandBoolFlag(cmd, "dry-run") {
		return output.WriteCommandPayload(cmd, map[string]any{
			"dry_run": true,
			"route":   "dingtalk_oapi",
			"request": map[string]any{
				"method": request.Method,
				"url":    request.Path,
				"body":   request.Data,
			},
		}, output.FormatJSON)
	}

	if s == nil || s.resolveToken == nil || s.newClient == nil {
		return apperrors.NewInternal(
			"report sender OAPI submitter is not configured",
			apperrors.WithOperation("report.create.delegated"),
		)
	}
	token, err := s.resolveToken(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(token) == "" {
		return apperrors.NewAuth(
			"获取到的应用级 access token 为空",
			apperrors.WithOperation("report.create.delegated"),
			apperrors.WithReason("empty_app_access_token"),
			apperrors.WithHint("检查自有应用 AppKey/AppSecret 后重试"),
		)
	}
	client := s.newClient(token)
	if client == nil {
		return apperrors.NewInternal(
			"report sender OAPI client is not configured",
			apperrors.WithOperation("report.create.delegated"),
		)
	}
	if concrete, ok := client.(*apiclient.APIClient); ok {
		if timeout := reportCommandIntFlag(cmd, "timeout"); timeout > 0 {
			concrete.HTTPClient.Timeout = time.Duration(timeout) * time.Second
		}
	}
	response, err := client.Do(ctx, request)
	if err != nil {
		return apperrors.NewAPI(
			fmt.Sprintf("代提交日志 OAPI 请求失败: %v", err),
			apperrors.WithOperation("report.create.delegated"),
			apperrors.WithReason("delegated_report_request_failed"),
			apperrors.WithHint("检查网络、自有应用凭证和“管理员工日志数据”权限；该路径不会回退 MCP"),
			apperrors.WithCause(err),
		)
	}
	if response == nil {
		return apperrors.NewAPI(
			"代提交日志 OAPI 返回空响应",
			apperrors.WithOperation("report.create.delegated"),
			apperrors.WithReason("empty_api_response"),
			apperrors.WithHint("稍后重试；该路径不会回退 MCP"),
		)
	}
	if err := apiclient.HandleResponse(response, apiclient.ResponseOptions{
		Format: output.ResolveFormat(cmd, output.FormatJSON),
		JqExpr: output.ResolveJQ(cmd),
		Fields: output.ResolveFields(cmd),
		Out:    cmd.OutOrStdout(),
		ErrOut: cmd.ErrOrStderr(),
	}); err != nil {
		return apperrors.NewAPI(
			fmt.Sprintf("代提交日志 OAPI 返回错误: %v", err),
			apperrors.WithOperation("report.create.delegated"),
			apperrors.WithReason("delegated_report_api_error"),
			apperrors.WithHint("确认 sender userId 属于当前企业且应用已开通“管理员工日志数据”权限；不要改走 MCP 重试"),
			apperrors.WithCause(err),
		)
	}
	return nil
}

func resolveReportSenderToken(ctx context.Context) (string, error) {
	appKey := strings.TrimSpace(authpkg.ClientID())
	appSecret := strings.TrimSpace(authpkg.ClientSecret())
	if appKey == "" || appSecret == "" || strings.HasPrefix(appKey, "<") || strings.HasPrefix(appSecret, "<") {
		return "", apperrors.NewAuth(
			"--sender-user-id 代提交日志需要自有应用的 AppKey/AppSecret",
			apperrors.WithOperation("report.create.delegated"),
			apperrors.WithReason("app_credentials_required"),
			apperrors.WithHint("通过 --client-id/--client-secret、DWS_CLIENT_ID/DWS_CLIENT_SECRET 或 dws auth login 配置；应用还需“管理员工日志数据”权限"),
		)
	}
	provider := &authpkg.AppTokenProvider{AppKey: appKey, AppSecret: appSecret}
	token, err := provider.GetToken(ctx)
	if err != nil {
		return "", apperrors.NewAuth(
			fmt.Sprintf("获取代提交日志所需的应用级 access token 失败: %v", err),
			apperrors.WithOperation("report.create.delegated"),
			apperrors.WithReason("app_token_failed"),
			apperrors.WithHint("检查自有应用凭证及网络后重试"),
			apperrors.WithCause(err),
		)
	}
	return strings.TrimSpace(token), nil
}

func reportCommandBoolFlag(cmd *cobra.Command, name string) bool {
	for _, flags := range reportCommandFlagSets(cmd) {
		if flags.Lookup(name) == nil {
			continue
		}
		value, err := flags.GetBool(name)
		if err == nil {
			return value
		}
	}
	return false
}

func reportCommandIntFlag(cmd *cobra.Command, name string) int {
	for _, flags := range reportCommandFlagSets(cmd) {
		if flags.Lookup(name) == nil {
			continue
		}
		value, err := flags.GetInt(name)
		if err == nil {
			return value
		}
	}
	return 0
}

func reportCommandFlagSets(cmd *cobra.Command) []*pflag.FlagSet {
	if cmd == nil {
		return nil
	}
	sets := []*pflag.FlagSet{cmd.Flags(), cmd.InheritedFlags()}
	if root := cmd.Root(); root != nil {
		sets = append(sets, root.PersistentFlags())
	}
	return sets
}
