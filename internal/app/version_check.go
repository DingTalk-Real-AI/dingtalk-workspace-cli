// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/upgrade"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/configmeta"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var checkCurrentVersion = upgrade.CheckVersion

type versionCheckArgsKey struct{}

func init() {
	configmeta.Register(configmeta.ConfigItem{
		Name: "DWS_NO_UPDATE_CHECK", Category: configmeta.CategoryNetwork,
		Description: "非空时关闭关键指令和未知命令的版本检查", Example: "1",
	})
}

// 只在版本查询、登录态查询和未知命令恢复时检查，不增加业务命令的网络请求。
func commandVersionCheck(cmd *cobra.Command, force bool) upgrade.CheckResult {
	current := RawVersion()
	if os.Getenv("DWS_NO_UPDATE_CHECK") != "" || edition.Get().IsEmbedded {
		return upgrade.CheckResult{Current: current, Status: upgrade.CheckStatusSkipped}
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	return checkCurrentVersion(ctx, current, upgrade.CheckOptions{
		CacheDir: filepath.Join(defaultConfigDir(), "cache"),
		Edition:  edition.Get().Name, ReadOnly: versionCheckReadOnly(cmd, ctx), Force: force,
	})
}

// 未知 flag 可能让 Cobra 在 --readonly 之前停止；只读意图不能依赖部分解析结果。
func versionCheckReadOnly(cmd *cobra.Command, ctx context.Context) bool {
	readonly, _ := versionCheckIntent(cmd, ctx)
	return readonly
}

func versionCheckIntent(cmd *cobra.Command, ctx context.Context) (bool, bool) {
	readonly, _ := cmd.Flags().GetBool("readonly")
	help, _ := cmd.Flags().GetBool("help")
	args, ok := ctx.Value(versionCheckArgsKey{}).([]string)
	if !ok {
		return readonly, help
	}
	target, _, _ := cmd.Root().Find(args)
	flags := pflag.NewFlagSet("version-check-readonly", pflag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.ParseErrorsWhitelist.UnknownFlags = true
	flags.Bool("readonly", false, "")
	flags.BoolP("help", "h", false, "")
	copyFlag := func(flag *pflag.Flag) {
		if flags.Lookup(flag.Name) == nil {
			flags.StringP(flag.Name, flag.Shorthand, "", "")
			flags.Lookup(flag.Name).NoOptDefVal = flag.NoOptDefVal
		}
	}
	target.Flags().VisitAll(copyFlag)
	target.InheritedFlags().VisitAll(copyFlag)
	if flags.Parse(args) != nil {
		// 参数格式损坏时不再发起额外的诊断网络请求。
		return true, true
	}
	readonly, _ = flags.GetBool("readonly")
	help, _ = flags.GetBool("help")
	return readonly && target.Flags().Lookup("readonly") != nil, help
}

type versionUpdateNotice struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
	Command string `json:"command"`
	Message string `json:"message"`
}

type versionNotices struct {
	Update versionUpdateNotice `json:"update"`
}

func noticeForVersion(check upgrade.CheckResult, unknownInvocation bool) *versionNotices {
	if check.Status != upgrade.CheckStatusUpdateAvailable {
		return nil
	}
	message := fmt.Sprintf("发现新版本（当前 %s，最新 %s），运行 %s 升级。", check.Current, check.Latest, check.UpgradeCommand())
	if unknownInvocation {
		message += " 当前版本可能不支持此命令或参数；请核对拼写，或升级后重试。"
	}
	return &versionNotices{Update: versionUpdateNotice{
		Current: check.Current, Latest: check.Latest, Command: check.UpgradeCommand(), Message: message,
	}}
}

func writeVersionCheck(w io.Writer, check upgrade.CheckResult) error {
	var message string
	switch check.Status {
	case upgrade.CheckStatusUpdateAvailable:
		message = noticeForVersion(check, false).Update.Message
	case upgrade.CheckStatusUpToDate:
		message = fmt.Sprintf("当前 %s，无可用更新（发布渠道最新 %s）。", check.Current, check.Latest)
	case upgrade.CheckStatusSkipped:
		message = fmt.Sprintf("当前 %s，未执行版本检查（已关闭检查、开发版本或宿主托管）。", check.Current)
	default:
		message = fmt.Sprintf("当前 %s，暂时无法确认是否最新；可运行 dws upgrade --check 重试。", check.Current)
	}
	if check.Cached && strings.TrimSpace(check.CheckedAt) != "" {
		message += " 检查时间：" + check.CheckedAt + "（缓存）。"
	}
	_, err := fmt.Fprintln(w, "版本检查: "+message)
	return err
}

func unknownInvocationNotice(cmd *cobra.Command, err error) *versionNotices {
	// 纠错候选或人工处理方案由解析生产方标记；不能因通用 --help hint 非空而跳过检查。
	if !isUnknownInvocationError(err) || apperrors.HasLocalRecovery(err) {
		return nil
	}
	target := cmd
	if ctx := cmd.Context(); ctx != nil {
		if args, ok := ctx.Value(versionCheckArgsKey{}).([]string); ok {
			if found, _, _ := cmd.Root().Find(args); found != nil {
				target = found
			}
		}
	}
	for current := target; current != nil; current = current.Parent() {
		switch current.Name() {
		case "completion", "__complete", "__completeNoDesc":
			return nil
		case "help":
			if current.Parent() == cmd.Root() {
				return nil
			}
		}
	}
	if ctx := cmd.Context(); ctx != nil {
		if _, help := versionCheckIntent(cmd, ctx); help {
			return nil
		}
	}
	// 未知命令不沿用“已是最新”的日缓存，避免错过当天刚发布的新能力。
	return noticeForVersion(commandVersionCheck(cmd, true), true)
}

func emitFailureWithVersionNotice(cmd *cobra.Command, err error, notice *versionNotices) (int, error) {
	var opts []output.ResultOption
	if notice != nil {
		opts = append(opts, output.WithNotice(notice))
	}
	result := output.FailureWithExitCode(errorInfoFromExecutionError(err), apperrors.ExitCode(err), opts...)
	code, emitErr := output.EmitResult(cmd, result)
	format := output.ResolveFormat(cmd, output.FormatJSON)
	if emitErr == nil && notice != nil && format != output.FormatJSON && format != output.FormatNDJSON {
		// 提醒是附加诊断；写入失败不能覆盖已经发出的业务结果或退出码。
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), notice.Update.Message)
	}
	return code, emitErr
}
