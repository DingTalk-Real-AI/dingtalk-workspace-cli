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
	"net/url"
	"strings"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/requestmeta"
	"github.com/spf13/cobra"
)

// 每个参数保留第一次值及冲突状态，不能通过重复参数覆盖身份。
// Set 不返回输入错误：按协议，无效的委托输入整组省略，业务仍执行普通调用。
type delegatorFlagValue struct {
	target  *string
	seen    bool
	invalid bool
}

func (v *delegatorFlagValue) String() string { return *v.target }
func (*delegatorFlagValue) Type() string     { return "string" }
func (v *delegatorFlagValue) Set(raw string) error {
	if v.seen && *v.target != raw {
		v.invalid = true
	}
	if !validDelegatorID(raw) {
		v.invalid = true
	}
	if !v.seen {
		*v.target = raw
	}
	v.seen = true
	return nil
}

func validDelegatorID(raw string) bool {
	if raw == "" || !utf8.ValidString(raw) || strings.TrimSpace(raw) != raw {
		return false
	}
	for _, r := range raw {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func bindDelegatorFlags(cmd *cobra.Command, flags *GlobalFlags) {
	for _, entry := range []struct {
		name   string
		target *string
	}{
		{requestmeta.DelegatorUserIDHeader, &flags.DelegatorUserID},
		{requestmeta.DelegatorCorpIDHeader, &flags.DelegatorCorpID},
		{requestmeta.DelegatorOpenDingtalkIDHeader, &flags.DelegatorOpenDingtalkID},
	} {
		cmd.PersistentFlags().Var(&delegatorFlagValue{target: entry.target}, entry.name, "宿主注入的本次调用委托身份")
		_ = cmd.PersistentFlags().MarkHidden(entry.name)
	}
}

type delegatorSnapshot struct{ userID, corpID, openDingtalkID string }
type delegatorContextKey struct{}

// delegatorInvocation 属于一棵 root，而非进程全局变量。执行入口发布不可变快照，
// 退出时清理；同次执行的并发辅助请求可以安全读取，但 Cobra 本身仍须串行执行。
type delegatorInvocation struct {
	snapshot atomic.Pointer[delegatorSnapshot]
}

func (s *delegatorInvocation) clear() {
	if s != nil {
		s.snapshot.Store(nil)
	}
}

// withDelegatorContext 在公共 runner 补齐旧 helper 丢弃的身份。已有快照（含空快照）
// 保持不变，派生 context 保留取消和截止时间，重试也始终沿用首次捕获的身份。
func (r *runtimeRunner) withDelegatorContext(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Value(delegatorContextKey{}).(delegatorSnapshot); ok {
		return ctx
	}
	if r == nil || r.globalFlags == nil || r.globalFlags.delegatorInvocation == nil {
		return ctx
	}
	var snapshot delegatorSnapshot
	if current := r.globalFlags.delegatorInvocation.snapshot.Load(); current != nil {
		snapshot = *current
	}
	return context.WithValue(ctx, delegatorContextKey{}, snapshot)
}

// consumeDelegatorFlags 在执行边界消费并清理解析状态。后续请求只读取不可变快照。
func consumeDelegatorFlags(root *cobra.Command) (delegatorSnapshot, bool) {
	var snapshot delegatorSnapshot
	invalid, supplied := false, false
	for _, entry := range []struct {
		name   string
		target *string
	}{
		{requestmeta.DelegatorUserIDHeader, &snapshot.userID},
		{requestmeta.DelegatorCorpIDHeader, &snapshot.corpID},
		{requestmeta.DelegatorOpenDingtalkIDHeader, &snapshot.openDingtalkID},
	} {
		flag := root.PersistentFlags().Lookup(entry.name)
		if flag == nil {
			continue
		}
		value, ok := flag.Value.(*delegatorFlagValue)
		if !ok {
			continue
		}
		if value.seen {
			supplied = true
			*entry.target = *value.target
			invalid = invalid || value.invalid
		}
		*value.target = ""
		value.seen, value.invalid, flag.Changed = false, false, false
	}
	if invalid || (snapshot.userID == "") != (snapshot.corpID == "") {
		snapshot = delegatorSnapshot{}
	}
	return snapshot, supplied
}

func beginDelegatorInvocation(cmd *cobra.Command) error {
	snapshot, supplied := consumeDelegatorFlags(cmd.Root())
	// 即使输入无效也覆盖上次 context，避免 Cobra 复用命令时继承旧身份。
	cmd.SetContext(context.WithValue(cmd.Context(), delegatorContextKey{}, snapshot))
	// 旧入口没有组织上下文，暂不猜测等价关系或静默选择其中一套协议。
	principal := cmd.Flags().Lookup("principal-user-id")
	if principal != nil {
		if !principal.Changed {
			_ = principal.Value.Set("")
		}
		principal.Changed = false
	}
	if supplied && principal != nil && strings.TrimSpace(principal.Value.String()) != "" {
		return apperrors.NewValidation("--principal-user-id 不能与 --delegator-* 同时使用；请选择一种委托协议", apperrors.WithReason("conflicting_delegation_protocols"))
	}
	return nil
}

// Cobra 的业务 hook 出错或 panic 时不会执行 PostRun，需在各阶段统一清理。
// 成功路径保留身份到最后一个 PersistentPostRun，以覆盖业务 PostRun 的辅助请求。
func wrapDelegatorInvocationHook(scope *delegatorInvocation, hook func(*cobra.Command, []string) error, final bool) func(*cobra.Command, []string) error {
	if hook == nil {
		return nil
	}
	return func(cmd *cobra.Command, args []string) (err error) {
		completed := false
		defer func() {
			if !completed || err != nil || final {
				scope.clear()
			}
		}()
		err = hook(cmd, args)
		completed = true
		return err
	}
}

func installDelegatorInvocationHooks(cmd *cobra.Command, scope *delegatorInvocation) {
	if scope == nil {
		return
	}
	for _, pair := range []struct {
		withError *func(*cobra.Command, []string) error
		plain     *func(*cobra.Command, []string)
		final     bool
	}{
		{&cmd.PersistentPreRunE, &cmd.PersistentPreRun, false},
		{&cmd.PreRunE, &cmd.PreRun, false},
		{&cmd.RunE, &cmd.Run, false},
		{&cmd.PostRunE, &cmd.PostRun, false},
		{&cmd.PersistentPostRunE, &cmd.PersistentPostRun, true},
	} {
		if *pair.withError == nil && *pair.plain != nil {
			plain := *pair.plain
			*pair.withError = func(cmd *cobra.Command, args []string) error {
				plain(cmd, args)
				return nil
			}
			*pair.plain = nil
		}
		*pair.withError = wrapDelegatorInvocationHook(scope, *pair.withError, pair.final)
	}
}

// 委托信息只交给已知的 HTTPS 网关。自定义端点、换票、发现及插件请求不继承。
func applyDelegatorHeaders(ctx context.Context, headers map[string]string, endpoint string, invocation executor.Invocation, excluded bool) map[string]string {
	requestmeta.RemoveDelegatorHeaders(headers)
	target, err := url.Parse(endpoint)
	if excluded || err != nil || target.Scheme != "https" || target.User != nil ||
		(target.Port() != "" && target.Port() != "443") || !isDingTalkMCPGatewayHost(target.Hostname()) ||
		strings.EqualFold(strings.TrimSpace(invocation.CanonicalProduct), mcpMetaServerID) {
		return headers
	}
	if ctx == nil {
		return headers
	}
	snapshot, _ := ctx.Value(delegatorContextKey{}).(delegatorSnapshot)
	for key, value := range map[string]string{
		requestmeta.DelegatorUserIDHeader:         snapshot.userID,
		requestmeta.DelegatorCorpIDHeader:         snapshot.corpID,
		requestmeta.DelegatorOpenDingtalkIDHeader: snapshot.openDingtalkID,
	} {
		if value != "" {
			if headers == nil {
				headers = make(map[string]string)
			}
			headers[key] = value
		}
	}
	return headers
}
