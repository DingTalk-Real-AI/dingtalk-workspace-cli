// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageExternalExchangeFailureBoundaries(t *testing.T) {
	for _, scenario := range []string{"stdin-read", "stdin-empty", "lookup-unavailable", "lookup-error", "exchange-error", "incomplete-identity", "profile-readback"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("DWS_CONFIG_DIR", dir)
			var caller edition.ToolCaller = &externalIdentityCaller{authCoverageCaller: authCoverageCaller{err: errors.New("private lookup error")}}
			if scenario == "lookup-unavailable" {
				caller = struct{ edition.ToolCaller }{caller}
			}
			root, _ := externalExchangeRoot(t, caller)
			root.SetArgs([]string{"auth", "exchange", "--code", "fixture-code"})
			if strings.HasPrefix(scenario, "stdin-") {
				root.SetArgs([]string{"auth", "exchange", "--code-stdin"})
				if scenario == "stdin-read" {
					root.SetIn(exchangeBoundaryReader{})
				} else {
					root.SetIn(strings.NewReader(" \n"))
				}
			}
			testseam.Swap(t, &authExternalExchange, func(ctx context.Context, _ string, req authpkg.ExternalExchangeRequest) (*authpkg.TokenData, error) {
				switch scenario {
				case "lookup-unavailable", "lookup-error":
					_, err := req.ResolveIdentity(ctx, "fixture-token", "corp")
					return nil, err
				case "exchange-error":
					return nil, errors.New("exchange failed")
				case "incomplete-identity":
					return &authpkg.TokenData{CorpID: "corp"}, nil
				case "profile-readback":
					if err := os.Mkdir(filepath.Join(dir, "profiles.json"), 0700); err != nil {
						t.Fatal(err)
					}
					return &authpkg.TokenData{CorpID: "corp", UserID: "employee"}, nil
				default:
					t.Fatal("invalid stdin reached exchange")
					return nil, nil
				}
			})
			if err := root.Execute(); err == nil {
				t.Fatal("failed login reported success")
			}
		})
	}
}

type exchangeBoundaryReader struct{}

func (exchangeBoundaryReader) Read([]byte) (int, error) { return 0, errors.New("reader failed") }

func TestCrossPlatformCoverageExternalExchangeApplicationConflictAndHumanPreview(t *testing.T) {
	for _, scenario := range []string{"empty-local", "empty-root", "conflict", "human-preview"} {
		t.Run(scenario, func(t *testing.T) {
			root, out := externalExchangeRoot(t, nil)
			cmd, _, err := root.Find([]string{"auth", "exchange"})
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "empty-local":
				_ = cmd.Flags().Set("client-id", " ")
			case "empty-root":
				_ = root.PersistentFlags().Set("client-id", " ")
			case "conflict":
				_ = root.PersistentFlags().Set("client-id", "first")
				_ = cmd.Flags().Set("client-id", "second")
			case "human-preview":
				_ = root.PersistentFlags().Set("format", "text")
				if err := writeAuthExchangeResult(cmd, map[string]any{"status": "planned"}); err != nil || !strings.Contains(out.String(), "未执行") {
					t.Fatalf("out=%s err=%v", out, err)
				}
				return
			}
			if _, err := authExchangeClientIDFlag(cmd); err == nil {
				t.Fatal("ambiguous application accepted")
			}
		})
	}
}

func TestCrossPlatformCoverageTokenAdapterMissingCapabilitiesAndExplicitToken(t *testing.T) {
	if _, err := (&toolCallerAdapter{runner: executor.EchoRunner{}}).CallToolWithToken(context.Background(), "token", "contact", "get", nil); err == nil {
		t.Fatal("non-scoped runner accepted")
	}
	if _, err := (*toolCallerAdapter)(nil).AccessToken(context.Background()); err == nil {
		t.Fatal("nil resolver accepted")
	}
	caller := &toolCallerAdapter{flags: &GlobalFlags{Token: "explicit-fixture-token"}}
	if token, err := caller.AccessToken(context.Background()); err != nil || token != "explicit-fixture-token" {
		t.Fatalf("token=%q err=%v", token, err)
	}
	for _, runner := range []*runtimeRunner{nil, {}, {globalFlags: &GlobalFlags{DryRun: true}}} {
		inv := executor.NewHelperInvocation("overlay.contact.get", "contact", "get", nil)
		_, err := runner.RunWithToken(context.Background(), inv, "token")
		if runner != nil && runner.globalFlags != nil {
			if err != nil {
				t.Fatal(err)
			}
		} else if err == nil {
			t.Fatal("unconfigured runner accepted")
		}
	}
}
