// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package helpers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageEmployeeCIBindingReadBoundaries(t *testing.T) {
	for _, scenario := range []string{"mismatch", "server-operation", "optional-self", "token-provider"} {
		t.Run(scenario, func(t *testing.T) {
			_, b := lifecycleFixture(t)
			previous := auth.RuntimeProfile()
			auth.SetRuntimeProfile(b.DWSProfile)
			t.Cleanup(func() { auth.SetRuntimeProfile(previous) })
			if scenario == "optional-self" {
				id, err := employeeBindingSelfIdentity(context.Background(), b, false)
				if err != nil || id != "" {
					t.Fatalf("非受管旧配置: %q %v", id, err)
				}
				return
			}
			if scenario == "token-provider" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				if _, err := employeeSelfIdentityToken(ctx, b.DWSProfile); err == nil {
					t.Fatal("缺失员工 Token 被接受")
				}
				return
			}
			calls := 0
			testseam.Swap(t, &deapChannelLoadBinding, func(string, string) (digitalEmployeeBinding, error) {
				calls++
				got := b
				if calls == 3 {
					got.Channel = "custom"
				}
				if scenario == "server-operation" {
					got = b
					if calls == 4 {
						if err := writeEmployeeJSON(employeeServerOperationPath(b.DWSProfile), employeeServerOperation{Phase: "requested"}); err != nil {
							t.Fatal(err)
						}
					}
				}
				return got, nil
			})
			testseam.Swap(t, &employeeSelfIdentityToken, func(context.Context, string) (*auth.TokenData, error) {
				return &auth.TokenData{CorpID: "corp", UserID: "employee", AccessToken: "fixture"}, nil
			})
			InitDepsForTest(t, &digitalEmployeeProtocolCaller{responses: map[string][]string{"contact/search_contact_by_key_word": {`{"result":[{"userId":"employee","openDingTalkId":"self"}]}`}}})
			cmd := newEmployeeBindingCommand()
			cmd.SetContext(context.Background())
			_ = cmd.Flags().Set("channel", "dsh")
			_ = cmd.Flags().Set("stdin", "true")
			cmd.SetIn(strings.NewReader(fmt.Sprintf(`{"agentUuid":%q,"bindingRevision":7}`, b.AgentUUID)))
			if err := cmd.RunE(cmd, nil); err == nil {
				t.Fatal("绑定重读变化仍返回成功")
			} else {
				want := "revision is no longer authorized"
				if scenario == "server-operation" {
					want = "server_binding_unknown"
				}
				if !strings.Contains(err.Error(), want) || calls < 3 {
					t.Fatalf("未触达绑定重读失败: calls=%d err=%v", calls, err)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageEmployeeCILifecycleFailures(t *testing.T) {
	t.Run("lock-directory", func(t *testing.T) {
		_, b := lifecycleFixture(t)
		b.DWSProfile = "corp:blocked-binding"
		if err := os.MkdirAll(filepath.Dir(employeeBindingLockPath(b.DWSProfile)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(employeeBindingLockPath(b.DWSProfile), []byte("blocked"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := updateEmployeeBinding(b); err == nil {
			t.Fatal("锁目录失败被忽略")
		}
	})
	t.Run("invalid-binding", func(t *testing.T) {
		dir, b := lifecycleFixture(t)
		if err := os.WriteFile(digitalEmployeeBindingPath(dir, b.DWSProfile), []byte("{"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := updateEmployeeBinding(b); err == nil {
			t.Fatal("损坏绑定被覆盖")
		}
	})
	t.Run("worker-activation", func(t *testing.T) {
		cfg := employeeBoundaryConfig(t)
		lock, err := auth.AcquireDualLock(context.Background(), filepath.Join(digitalEmployeeRuntimeDir(cfg.Binding.DWSProfile), "activation"))
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Release()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		if err := runEmployeeWorker(ctx, cfg, 0); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("activation 未保留取消: %v", err)
		}
	})
	t.Run("worker-server-operation", func(t *testing.T) {
		cfg := employeeBoundaryConfig(t)
		if err := writeEmployeeJSON(employeeServerOperationPath(cfg.Binding.DWSProfile), employeeServerOperation{Phase: "requested"}); err != nil {
			t.Fatal(err)
		}
		if err := runEmployeeWorker(context.Background(), cfg, 0); err == nil || !strings.Contains(err.Error(), "server_binding_unknown") {
			t.Fatalf("未完成服务端操作仍启动: %v", err)
		}
	})
	t.Run("worker-self", func(t *testing.T) {
		cfg := employeeBoundaryConfig(t)
		cfg.Binding.RuntimeBindingID = "server-binding"
		cfg.SelfIdentityProfile = ""
		if err := saveDigitalEmployeeBinding(deapConnectConfigDir(), cfg.Binding); err != nil {
			t.Fatal(err)
		}
		testseam.Swap(t, &employeeSelfIdentityToken, func(context.Context, string) (*auth.TokenData, error) { return nil, errors.New("fixture unavailable") })
		if err := runEmployeeWorker(context.Background(), cfg, 0); err == nil || err.Error() != "self_identity_unavailable" {
			t.Fatalf("自身身份失败未阻断: %v", err)
		}
	})
	t.Run("lease-result", func(t *testing.T) {
		_, b := lifecycleFixture(t)
		previous := auth.RuntimeProfile()
		auth.SetRuntimeProfile(b.DWSProfile)
		t.Cleanup(func() { auth.SetRuntimeProfile(previous) })
		cmd := newDeapConnectCommand()
		cmd.SetContext(context.Background())
		for k, v := range map[string]string{"agent-uuid": b.AgentUUID, "channel": "dsh", "binding-revision": "7", "runtime-instance-id": "00000000-0000-4000-8000-000000000001", "local-lease": "true"} {
			if err := cmd.Flags().Set(k, v); err != nil {
				t.Fatal(err)
			}
		}
		cmd.SetIn(strings.NewReader("released\n"))
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&bytes.Buffer{})
		if err := runDeapConnect(cmd, nil); err != nil || !strings.Contains(out.String(), `"released"`) {
			t.Fatalf("lease 成功回执: %s %v", out.String(), err)
		}
	})
}

func TestCrossPlatformCoverageEmployeeCIReceiptBoundaries(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(fmt.Sprint(deadline), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				InitDepsForTest(t, &digitalEmployeeProtocolCaller{responses: map[string][]string{"im/query_message_send_status": {`{}`}}})
				marker := errors.New("fixture wait failure")
				testseam.Swap(t, &deapChannelReceiptWait, func(ctx context.Context, _ time.Duration) error {
					if deadline {
						time.Sleep(2 * time.Second)
						<-ctx.Done()
						return nil
					}
					return marker
				})
				_, err := ResolveChatDelivery(context.Background(), map[string]any{"openTaskId": "task"}, "", "key")
				if deadline {
					var typed *apperrors.Error
					if !errors.As(err, &typed) || typed.Reason != "delivery_unknown" {
						t.Fatalf("deadline: %v", err)
					}
				} else if !errors.Is(err, marker) {
					t.Fatalf("wait: %v", err)
				}
			})
		})
	}
	cmd := newDeapChannelReplyCommand()
	ctx, _ := output.WithResultStore(context.Background())
	cmd.SetContext(ctx)
	if err := WriteChatDelivery(cmd, map[string]any{"openMessageId": "sent"}, "conversation", "key"); err != nil {
		t.Fatal(err)
	}
	if got := digitalEmployeeDeliveryResult(map[string]any{"openMessageId": "sent"}, "conversation", "key"); got["openMessageId"] != "sent" {
		t.Fatal(got)
	}
}
