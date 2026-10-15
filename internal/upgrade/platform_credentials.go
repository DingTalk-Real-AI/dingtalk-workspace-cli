// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package upgrade

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

const githubCredentialTimeout = 2 * time.Second
const githubCredentialMaxBytes = 4096

var (
	platformGitHubToken     = localGitHubToken
	githubCredentialCommand = exec.CommandContext
)

// localGitHubToken 只读取已有凭据，不发起登录；显式环境变量优先于 gh 登录态。
// 不可用或读取超时返回空值，沿用匿名请求。凭据和子进程诊断均不进入日志。
func localGitHubToken(ctx context.Context) string {
	if token := githubToken(); token != "" {
		return normalizeGitHubToken(token)
	}
	ctx, cancel := context.WithTimeout(ctx, githubCredentialTimeout)
	defer cancel()
	cmd := githubCredentialCommand(ctx, "gh", "auth", "token", "--hostname", "github.com")
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1")
	cmd.Stdin = nil
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 100 * time.Millisecond
	var output githubCredentialBuffer
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		return ""
	}
	return normalizeGitHubToken(string(output.data))
}

// 限制本机凭据工具的输出，避免异常输出无限进入内存。
type githubCredentialBuffer struct{ data []byte }

func (b *githubCredentialBuffer) Write(p []byte) (int, error) {
	if len(b.data)+len(p) > githubCredentialMaxBytes {
		return 0, io.ErrShortBuffer
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func normalizeGitHubToken(token string) string {
	token = strings.TrimSpace(token)
	if len(token) > githubCredentialMaxBytes || strings.IndexFunc(token, func(r rune) bool { return r <= ' ' || r >= 127 }) >= 0 {
		return ""
	}
	return token
}

// 凭据只附加到官方 API 的 HTTPS 请求；每次重定向重新判断，禁止随请求转发到其他站点。
// 该 transport 仅属于平台元数据客户端，不修改全局 transport 或资产下载器。
type githubMetadataTransport struct {
	base  http.RoundTripper
	token string
}

func (t githubMetadataTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Del("Authorization")
	if t.token != "" && clone.URL.Scheme == "https" && clone.URL.Host == "api.github.com" && clone.URL.User == nil {
		clone.Header.Set("Authorization", "Bearer "+t.token)
	}
	return t.base.RoundTrip(clone)
}
