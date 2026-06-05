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

const reportCreateOAPIPath = "https://oapi.dingtalk.com/topapi/report/create"

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
	req := apiclient.RawAPIRequest{
		Method: http.MethodPost,
		Path:   reportCreateOAPIPath,
		Data:   body,
	}
	if reportCommandBoolFlag(cmd, "dry-run") {
		return output.WriteCommandPayload(cmd, map[string]any{
			"dry_run": true,
			"route":   "dingtalk_oapi",
			"request": map[string]any{
				"method": req.Method,
				"url":    req.Path,
				"body":   req.Data,
			},
		}, output.FormatJSON)
	}

	if s == nil || s.resolveToken == nil || s.newClient == nil {
		return apperrors.NewInternal("report sender OAPI submitter is not configured")
	}
	token, err := s.resolveToken(ctx)
	if err != nil {
		return err
	}
	client := s.newClient(token)
	if client == nil {
		return apperrors.NewInternal("report sender OAPI client is not configured")
	}
	if concrete, ok := client.(*apiclient.APIClient); ok {
		if timeout := reportCommandIntFlag(cmd, "timeout"); timeout > 0 {
			concrete.HTTPClient.Timeout = time.Duration(timeout) * time.Second
		}
	}
	resp, err := client.Do(ctx, req)
	if err != nil {
		return apperrors.NewAPI(fmt.Sprintf("代提交日志 OAPI 请求失败: %v", err))
	}
	return apiclient.HandleResponse(resp, apiclient.ResponseOptions{
		Format: output.ResolveFormat(cmd, output.FormatJSON),
		JqExpr: reportCommandStringFlag(cmd, "jq"),
		Fields: reportCommandStringFlag(cmd, "fields"),
		Out:    cmd.OutOrStdout(),
		ErrOut: cmd.ErrOrStderr(),
	})
}

func resolveReportSenderToken(ctx context.Context) (string, error) {
	appKey := strings.TrimSpace(authpkg.ClientID())
	appSecret := strings.TrimSpace(authpkg.ClientSecret())
	if appKey == "" || appSecret == "" || strings.HasPrefix(appKey, "<") || strings.HasPrefix(appSecret, "<") {
		return "", apperrors.NewAuth(
			"--sender-user-id 代提交日志需要自有应用的 AppKey/AppSecret。\n\n" +
				"请通过 --client-id/--client-secret、DWS_CLIENT_ID/DWS_CLIENT_SECRET，或 dws auth login 配置应用凭证；" +
				"应用还需要“管理员工日志数据”权限。",
		)
	}
	provider := &authpkg.AppTokenProvider{AppKey: appKey, AppSecret: appSecret}
	token, err := provider.GetToken(ctx)
	if err != nil {
		return "", apperrors.NewAuth(fmt.Sprintf("获取代提交日志所需的应用级 access token 失败: %v", err))
	}
	return strings.TrimSpace(token), nil
}

func reportCommandStringFlag(cmd *cobra.Command, name string) string {
	for _, flags := range reportCommandFlagSets(cmd) {
		if flags.Lookup(name) == nil {
			continue
		}
		value, err := flags.GetString(name)
		if err == nil {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
