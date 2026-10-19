// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageOIDCCommandsRejectMCPURLFlag(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	for _, args := range [][]string{
		{"provider-login", "ant"},
		{"exchange-prepare", "--provider", "ant"},
		{"token-exchange", "--provider", "ant", "--code", "code", "--state", splitOIDCState},
	} {
		t.Run(args[0], func(t *testing.T) {
			root := NewRootCommand(t.Context())
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			argv := append([]string{"auth"}, args...)
			root.SetArgs(append(argv, "--dry-run", "--mcp-url", "https://other.example.invalid"))
			err := root.Execute()
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || typed.ExitCode() != 3 || !strings.Contains(err.Error(), "unknown flag: --mcp-url") {
				t.Fatalf("expected unknown --mcp-url validation error, got %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageProviderLoginPreservesSystemEndpoint(t *testing.T) {
	for _, configured := range []bool{false, true} {
		name, want := "default", config.DefaultMCPBaseURL
		if configured {
			name, want = "configured", authpkg.InternationalMCPBaseURL
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("DWS_CONFIG_DIR", dir)
			paths := []string{"mcp_url", config.ManagedMCPURLRegionFileName}
			if configured {
				for _, path := range paths {
					if err := os.WriteFile(filepath.Join(dir, path), []byte(want+"\n"), config.FilePerm); err != nil {
						t.Fatal(err)
					}
				}
			}
			calls := 0
			testseam.Swap(t, &authProviderLogin, func(_ context.Context, gotDir, _, base string, _ edition.ToolCaller, _ *authpkg.OIDCExchangeCredentials) (*authpkg.TokenData, error) {
				calls++
				if gotDir != dir || base != want {
					t.Fatalf("provider did not use system endpoint: dir=%q base=%q", gotDir, base)
				}
				return &authpkg.TokenData{AccessToken: "test-token", CorpID: "corp", UserID: "user"}, nil
			})
			root, _, _ := providerLoginRoot()
			root.SetArgs([]string{"auth", "provider-login", "ant"})
			if err := corecmd.ExecuteForTest(root); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("provider calls = %d, want 1", calls)
			}
			for _, path := range paths {
				got, err := os.ReadFile(filepath.Join(dir, path))
				if configured {
					if err != nil || string(got) != want+"\n" {
						t.Fatalf("provider login changed system config %s: %q, %v", path, got, err)
					}
				} else if !os.IsNotExist(err) {
					t.Fatalf("provider login created system config %s: %v", path, err)
				}
			}
		})
	}
}
