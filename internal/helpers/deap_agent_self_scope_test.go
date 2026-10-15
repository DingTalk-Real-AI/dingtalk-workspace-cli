// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

type employeeSelfScopeCaptureAdapter struct {
	config digitalEmployeeAdapterConfig
}

func TestCrossPlatformCoverageEmployeeWorkerRepairsLegacySelfIdentityScope(t *testing.T) {
	cfg := employeeBoundaryConfig(t)
	cfg.Binding.RuntimeBindingID = "server-binding"
	cfg.SelfOpenDingTalkID = "supervisor-scope-self"
	cfg.SelfIdentityProfile = ""
	if err := saveDigitalEmployeeBinding(deapConnectConfigDir(), cfg.Binding); err != nil {
		t.Fatal(err)
	}
	adapterPath := filepath.Join(digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile), "adapter.json")
	if err := writeEmployeeJSON(adapterPath, cfg); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(adapterPath)
	if err != nil {
		t.Fatal(err)
	}
	testseam.Swap(t, &employeeSelfIdentityToken, func(_ context.Context, profile string) (*auth.TokenData, error) {
		if profile != cfg.Binding.DWSProfile {
			t.Fatal("旧配置修复使用了主管 Profile")
		}
		return &auth.TokenData{CorpID: "corp", UserID: "employee", AccessToken: "employee-token"}, nil
	})
	caller := &digitalEmployeeProtocolCaller{responses: map[string][]string{"contact/search_contact_by_key_word": {`{"result":[{"userId":"employee","openDingTalkId":"self-employee-scope"}]}`}}}
	InitDepsForTest(t, caller)
	called := false
	testseam.Swap(t, &digitalEmployeeNewForwarder, func(_ context.Context, actual digitalEmployeeAdapterConfig) (forwarder, error) {
		called = true
		if actual.SelfOpenDingTalkID != "self-employee-scope" || actual.SelfIdentityProfile != cfg.Binding.DWSProfile {
			t.Fatal("旧配置的发布详情 ID 仍进入运行时")
		}
		return nil, errors.New("controlled stop after self identity verification")
	})
	if err := runEmployeeWorker(context.Background(), cfg, 0); !called || err == nil || err.Error() != "agent_unavailable" {
		t.Fatalf("旧配置自身身份修复失败: called=%v err=%v", called, err)
	}
	after, err := os.ReadFile(adapterPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("启动改写了旧配置")
	}
	if len(caller.tokenCalls) != 1 || caller.tokens[0] != "employee-token" || caller.tokenCalls[0].args["keyword"] != "employee" {
		t.Fatal("旧配置修复未保持员工自身单次查询")
	}
}

func (a *employeeSelfScopeCaptureAdapter) Connect(_ *cobra.Command, cfg digitalEmployeeAdapterConfig) error {
	a.config = cfg
	return nil
}

func TestCrossPlatformCoverageEmployeeConnectSelfIdentityUsesEmployeeScope(t *testing.T) {
	setupSuccessfulConnectSeams(t)
	caller := newSuccessfulConnectCaller(successfulAuthResponse(), `{"result":[{"userId":"supervisor-user","openDingTalkId":"operator-employee-scope"}]}`)
	caller.responses["contact/search_contact_by_key_word"][1] = `{"result":[{"userId":"employee-user","openDingTalkId":"self-employee-scope"}]}`
	InitDepsForTest(t, caller)
	adapter := &employeeSelfScopeCaptureAdapter{}
	testseam.Swap(t, &digitalEmployeeResolveAdapter, func(string) (digitalEmployeeAdapter, error) { return adapter, nil })
	cmd := newConnectTestCommandWithMode(t, false, "custom")
	cmd.SetOut(io.Discard)
	if err := cmd.Flags().Set("agent-cmd", "fixture"); err != nil {
		t.Fatal(err)
	}
	if err := runDeapConnect(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if adapter.config.SelfOpenDingTalkID != "self-employee-scope" {
		t.Fatalf("自身身份使用了发布详情的主管上下文: got=%q", adapter.config.SelfOpenDingTalkID)
	}
	if adapter.config.SelfIdentityProfile != adapter.config.Binding.DWSProfile {
		t.Fatal("自身身份未记录员工 Profile 作用域")
	}
	last := len(caller.tokenCalls) - 2
	if last < 0 || caller.tokenCalls[last].args["keyword"] != "employee-user" || caller.tokens[last] != "managed-access-secret" {
		t.Fatal("未在员工身份下精确解析自身 userId")
	}
}
