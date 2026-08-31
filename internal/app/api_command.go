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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/apiclient"
	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/spf13/cobra"
)

// apiFlags holds the flags specific to the `dws api` command.
type apiFlags struct {
	params    string
	data      string
	file      string
	pageAll   bool
	pageLimit int
	pageDelay int
	baseURL   string
}

type appTokenGetter interface {
	GetToken(context.Context) (string, error)
}

var newAppTokenProvider = func(configDir, appKey, appSecret string) appTokenGetter {
	return &authpkg.AppTokenProvider{ConfigDir: configDir, AppKey: appKey, AppSecret: appSecret}
}

var newRawAPIClient = apiclient.NewClient

type rawAPICredentials = authpkg.AppCredentialPair

var resolveRawAPICredentials = resolveRawAPICredentialsFromSources

// newAPICommand creates the `dws api` subcommand for raw DingTalk OpenAPI calls.
func newAPICommand(flags *GlobalFlags) *cobra.Command {
	af := &apiFlags{}

	cmd := &cobra.Command{
		Use:   "api <METHOD> <PATH> [flags]",
		Short: "调用钉钉 OpenAPI (Raw HTTP)",
		Long: `直接调用钉钉 OpenAPI，支持 api.dingtalk.com 和 oapi.dingtalk.com 两个域名。

api.dingtalk.com:
  Token 通过 HTTP Header (x-acs-dingtalk-access-token) 传递。
  路径格式: /v1.0/xxx 或 /v2.0/xxx

oapi.dingtalk.com:
  Token 通过 URL 查询参数 (access_token) 传递。
  路径格式: /topapi/v2/xxx 或完整 URL https://oapi.dingtalk.com/topapi/...

仅限使用自有应用的完整 Client ID/Client Secret pair。凭证优先级为:
  完整 --client-id/--client-secret > 完整 DWS_CLIENT_ID/DWS_CLIENT_SECRET > 完整 app config。
同一来源只提供一项会明确失败，绝不会与其他来源拼接。
单次 flags/env 不持久化 Client Secret；获取到的 App Token 会按 Client ID 独立缓存。
成功的 dws auth login 会保存其实际使用的完整凭证对；历史 client-secret 槽位会迁移到 appsecret:<clientID>。
新旧 Client Secret 槽位值冲突时拒绝调用并要求重新登录，不猜测正确值。
隐藏 --token 仅临时使用调用方提供的 App Token，不持久化、不自动刷新。
通过 MCP 默认凭证登录获取的加密 token 不支持 raw API 调用。

示例:
  # === api.dingtalk.com ===

  # 获取企业所有应用列表
  dws api GET /v1.0/microApp/allApps

  # 搜索用户 (POST + JSON body)
  dws api POST /v1.0/contact/users/search \
    --data '{"queryWord":"张三","offset":0,"size":10}'

  # === oapi.dingtalk.com ===

  # 获取用户详情 (使用 --base-url)
  dws api POST /topapi/v2/user/get \
    --base-url https://oapi.dingtalk.com \
    --data '{"userid":"manager123"}'

  # 也可以直接使用完整 URL
  dws api POST https://oapi.dingtalk.com/topapi/v2/user/get \
    --data '{"userid":"manager123"}'

  # === 通用功能 ===

  # Dry-run 预览请求
  dws api GET /v1.0/microApp/allApps --dry-run

  # 上传媒体文件（旧 OAPI multipart；先 dry-run 核对）
  dws api POST https://oapi.dingtalk.com/media/upload \
    --data '{"type":"image"}' --file media=./demo.png --dry-run

  # 使用 jq 过滤输出
  dws api GET /v1.0/microApp/allApps --jq '.appList | length'`,
		Args:              cobra.ExactArgs(2),
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAPI(cmd, args, flags, af)
		},
	}

	cmd.Flags().StringVar(&af.params, "params", "", "查询参数 JSON (支持 @file 或 - 从 stdin 读取)")
	cmd.Flags().StringVar(&af.data, "data", "", "请求体 JSON (支持 @file 或 - 从 stdin 读取)")
	cmd.Flags().StringVar(&af.file, "file", "", "multipart 文件 [field=]path 或 [field=]-")
	cmd.Flags().BoolVar(&af.pageAll, "page-all", false, "自动遍历所有分页")
	cmd.Flags().IntVar(&af.pageLimit, "page-limit", apiclient.DefaultPageLimit, "最大翻页数 (0=不限, 默认10, 硬上限500)")
	cmd.Flags().IntVar(&af.pageDelay, "page-delay", apiclient.DefaultPageDelay, "分页间隔毫秒")
	cmd.Flags().StringVar(&af.baseURL, "base-url", "", "覆盖 API 基础 URL (默认 https://api.dingtalk.com)")

	return cmd
}

// runAPI is the main execution logic for `dws api`.
func runAPI(cmd *cobra.Command, args []string, gf *GlobalFlags, af *apiFlags) error {
	ctx := cmd.Context()
	method := args[0]
	path := args[1]

	// 0. Reject path with inline query string — must use --params instead.
	if idx := strings.IndexByte(path, '?'); idx >= 0 {
		cleanPath := path[:idx]
		// Parse query string to generate the exact --params JSON for the user.
		paramsJSON := parseQueryStringToJSON(path[idx+1:])
		return apperrors.NewValidation(
			"API 路径中不允许直接拼接查询参数（?key=value），该写法会导致参数在解析时被静默丢弃。\n\n"+
				"命令格式可参考：\n\n"+
				" dws api "+method+" "+cleanPath+" --params '"+paramsJSON+"'",
			apperrors.WithHint("查询参数必须通过 --params 传递，形如 --params '{\"key\":\"value\"}'"),
		)
	}

	// 1. Validate HTTP method.
	method, err := apiclient.ValidateMethod(method)
	if err != nil {
		return apperrors.NewValidation(err.Error())
	}

	// 2. Validate API path.
	if err := apiclient.ValidatePath(path); err != nil {
		return apperrors.NewValidation(err.Error())
	}

	// 3. Validate input safety for params and data.
	if err := apiclient.ValidateUserInput(af.params, "--params"); err != nil {
		return apperrors.NewValidation(err.Error())
	}
	if err := apiclient.ValidateUserInput(af.data, "--data"); err != nil {
		return apperrors.NewValidation(err.Error())
	}
	if err := apiclient.ValidateUserInput(af.file, "--file"); err != nil {
		return apperrors.NewValidation(err.Error())
	}
	fileUpload, err := apiclient.ParseFileSpec(af.file)
	if err != nil {
		return apperrors.NewValidation(err.Error())
	}

	// 4. Validate mutual exclusion.
	if err := apiclient.ValidateInputStdinExclusion(af.params, af.data, fileUpload); err != nil {
		return apperrors.NewValidation(err.Error())
	}
	if err := apiclient.ValidateFlagExclusion(gf.Output, af.pageAll); err != nil {
		return apperrors.NewValidation(err.Error())
	}
	if fileUpload != nil && method == "GET" {
		return apperrors.NewValidation("GET 请求不允许使用 --file；允许的方法为 POST、PUT、PATCH、DELETE")
	}
	if fileUpload != nil && strings.TrimSpace(gf.Output) != "" {
		return apperrors.NewValidation("--file 和 --output 不能同时使用")
	}
	if fileUpload != nil && af.pageAll {
		return apperrors.NewValidation("--file 和 --page-all 不能同时使用")
	}
	if strings.Contains(path, "#") {
		return apperrors.NewValidation("API 路径中不允许 fragment (#...)")
	}

	// 5. Normalise and validate the target before touching credentials or files.
	fullURL := apiclient.NormalisePath(path, af.baseURL)
	if err := apiclient.ValidateTargetHost(fullURL); err != nil {
		return apperrors.NewValidation(err.Error())
	}

	// 6. Dry-run never reads stdin/@file/upload bytes, Keychain, or the network.
	if gf.DryRun {
		var params map[string]any
		var body any
		if !apiclient.IsDeferredInput(af.params) {
			params, err = apiclient.ParseJSONMap(af.params, "--params", nil)
			if err != nil {
				return apperrors.NewValidation(err.Error())
			}
		}
		if !apiclient.IsDeferredInput(af.data) {
			body, err = apiclient.ParseOptionalBody(method, af.data, nil)
			if err != nil {
				return apperrors.NewValidation(err.Error())
			}
			if fileUpload != nil && body != nil {
				if _, ok := body.(map[string]any); !ok {
					return apperrors.NewValidation("使用 --file 时 --data 必须是 JSON object")
				}
			}
		}
		req := apiclient.RawAPIRequest{Method: method, Path: path, Params: params, Data: body, File: fileUpload}
		if apiclient.IsDeferredInput(af.params) {
			req.ParamsSource = af.params
		}
		if apiclient.IsDeferredInput(af.data) {
			req.DataSource = af.data
		}
		return apiclient.PrintDryRun(cmd.OutOrStdout(), req, af.baseURL, "")
	}

	// 7. Parse --params.
	params, err := apiclient.ParseJSONMap(af.params, "--params", os.Stdin)
	if err != nil {
		return apperrors.NewValidation(err.Error())
	}

	// 8. Parse --data.
	body, err := apiclient.ParseOptionalBody(method, af.data, os.Stdin)
	if err != nil {
		return apperrors.NewValidation(err.Error())
	}

	if fileUpload != nil && body != nil {
		if _, ok := body.(map[string]any); !ok {
			return apperrors.NewValidation("使用 --file 时 --data 必须是 JSON object")
		}
	}
	if fileUpload != nil && fileUpload.Path == "-" {
		fileUpload.Reader = os.Stdin
	}

	// 9. Resolve app-level token (with timeout).
	tokenCtx, tokenCancel := context.WithTimeout(ctx, 15*time.Second)
	defer tokenCancel()
	token, err := resolveRawAPIToken(tokenCtx, gf.Token, gf.ClientID, gf.ClientSecret)
	if err != nil {
		return err
	}

	// 10. Build request.
	req := apiclient.RawAPIRequest{
		Method: method,
		Path:   path,
		Params: params,
		Data:   body,
		File:   fileUpload,
	}

	baseURL := af.baseURL

	// 11. Create client with timeout.
	client := newRawAPIClient(token, baseURL)
	if gf.Timeout > 0 {
		client.HTTPClient.Timeout = time.Duration(gf.Timeout) * time.Second
	}

	// 12. Execute request (with or without pagination).
	format := output.Format(gf.Format)
	respOpts := apiclient.ResponseOptions{
		OutputPath: gf.Output,
		Format:     format,
		JqExpr:     gf.JQ,
		Fields:     gf.Fields,
		Out:        cmd.OutOrStdout(),
		ErrOut:     cmd.ErrOrStderr(),
	}

	if af.pageAll {
		return runPaginated(ctx, client, req, af, respOpts)
	}

	resp, err := client.Do(ctx, req)
	if err != nil {
		return apperrors.NewAPI(fmt.Sprintf("API 请求失败: %v", err))
	}
	if err := apiclient.HandleResponse(resp, respOpts); err != nil {
		var responseErr *apiclient.ResponseError
		if errors.As(err, &responseErr) {
			return apperrors.NewAPI(err.Error())
		}
		return err
	}
	return nil
}

// runPaginated executes a paginated API request and outputs all results.
func runPaginated(ctx context.Context, client *apiclient.APIClient, req apiclient.RawAPIRequest, af *apiFlags, opts apiclient.ResponseOptions) error {
	pages, err := client.PaginateAll(ctx, req, apiclient.PaginationOptions{
		PageLimit: af.pageLimit,
		PageDelay: af.pageDelay,
		LogWriter: opts.ErrOut,
	})
	if err != nil {
		return apperrors.NewAPI(fmt.Sprintf("分页请求失败: %v", err))
	}

	// Output all pages as a JSON array.
	return output.WriteFiltered(opts.Out, opts.Format, pages, opts.Fields, opts.JqExpr)
}

// parseQueryStringToJSON parses a raw URL query string into a JSON object string.
// Uses simple & and = splitting (no URL decoding) to preserve values as-is.
func parseQueryStringToJSON(rawQuery string) string {
	rawQuery = strings.TrimSpace(rawQuery)
	if rawQuery == "" {
		return "{}"
	}

	paramsMap := make(map[string]any)
	for _, pair := range strings.Split(rawQuery, "&") {
		kv := strings.SplitN(pair, "=", 2)
		key := strings.TrimSpace(kv[0])
		if key == "" {
			continue
		}
		var val string
		if len(kv) == 2 {
			val = strings.TrimSpace(kv[1])
		}
		if val == "" {
			continue // skip empty values like nextToken=
		}
		paramsMap[key] = val
	}

	if len(paramsMap) == 0 {
		return "{}"
	}

	data, _ := json.Marshal(paramsMap)
	return string(data)
}

// resolveRawAPIToken resolves an app-level access token for raw API calls.
// It uses AppTokenProvider to fetch from the unified POST /v1.0/oauth2/accessToken
// endpoint. The same token works for both api.dingtalk.com and oapi.dingtalk.com.
// Tokens are cached in keychain and auto-refreshed when expired.
func resolveRawAPIToken(ctx context.Context, explicitToken, flagClientID, flagClientSecret string) (string, error) {
	// Hidden compatibility flag: the caller supplies a temporary App Token.
	// It is never persisted and must not be interpreted as an OAuth User Token.
	if t := strings.TrimSpace(explicitToken); t != "" {
		return t, nil
	}

	configDir := defaultConfigDir()
	credentials, err := resolveRawAPICredentials(flagClientID, flagClientSecret, configDir)
	if err != nil {
		return "", err
	}

	// Use AppTokenProvider for automatic caching and refresh.
	provider := newAppTokenProvider(configDir, credentials.ClientID, credentials.ClientSecret)
	token, err := provider.GetToken(ctx)
	if err != nil {
		return "", apperrors.NewAuth(fmt.Sprintf("获取应用级访问令牌失败 (凭证来源: %s): %v", credentials.Source, err))
	}
	if strings.TrimSpace(token) == "" {
		return "", apperrors.NewAuth("应用级访问令牌为空，请检查应用凭证是否正确")
	}

	return strings.TrimSpace(token), nil
}

func resolveRawAPICredentialsFromSources(flagClientID, flagClientSecret, configDir string) (rawAPICredentials, error) {
	credentials, err := authpkg.ResolveAppCredentialPair(configDir, flagClientID, flagClientSecret)
	if err != nil {
		return rawAPICredentials{}, classifyRawAPIAppConfigError(err)
	}
	return credentials, nil
}

func classifyRawAPIAppConfigError(err error) error {
	switch {
	case errors.Is(err, authpkg.ErrFlagCredentialPairIncomplete):
		return apperrors.NewAuth("--client-id 和 --client-secret 必须同时提供；不会与环境变量或 app config 混用")
	case errors.Is(err, authpkg.ErrEnvCredentialPairIncomplete):
		return apperrors.NewAuth("DWS_CLIENT_ID 和 DWS_CLIENT_SECRET 必须同时设置；不会与 app config 混用")
	case errors.Is(err, authpkg.ErrAppConfigMissing):
		return apperrors.NewAuth(
			"缺少应用凭证。dws api 需要完整的 Client ID/Client Secret pair。\n\n" +
				"可选择以下任一方式:\n" +
				"  1. 同时传入 --client-id 和 --client-secret\n" +
				"  2. 同时设置 DWS_CLIENT_ID 和 DWS_CLIENT_SECRET\n" +
				"  3. 使用自有应用凭证执行 dws auth login\n\n" +
				"说明: 通过 MCP 默认凭证登录的加密 token 无法用于 raw API 调用。",
		)
	case errors.Is(err, authpkg.ErrClientIDEmpty), errors.Is(err, authpkg.ErrClientSecretEmpty):
		return apperrors.NewAuth("本地应用配置不完整，缺少 Client ID 或 Client Secret；请完整设置环境变量 pair、CLI pair，或重新配置自有应用凭证")
	case errors.Is(err, authpkg.ErrSecretResolve):
		return apperrors.NewAuth("无法从 Keychain 解析 Client Secret；请检查 Keychain 状态，或同时设置 DWS_CLIENT_ID 和 DWS_CLIENT_SECRET")
	case errors.Is(err, authpkg.ErrClientSecretConflict):
		return apperrors.NewAuth("检测到新旧 Client Secret 存储值冲突；为避免使用错误凭证已拒绝调用。请使用完整 --client-id/--client-secret 重新执行 dws auth login，或执行 dws auth reset 后重新登录")
	case errors.Is(err, authpkg.ErrClientSecretRefMismatch):
		return apperrors.NewAuth("本地应用配置中的 Client Secret 引用与 Client ID 不匹配；为避免跨应用混用已拒绝调用，请重新执行 dws auth login")
	case errors.Is(err, authpkg.ErrCredentialPlaceholders):
		return apperrors.NewAuth("应用凭证不完整或仍为占位符，Client ID 和 Client Secret 必须同时提供有效值")
	default:
		return apperrors.NewAuth(fmt.Sprintf("解析本地应用凭证失败: %v", err))
	}
}
