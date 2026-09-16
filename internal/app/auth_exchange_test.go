// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type externalIdentityCaller struct {
	authCoverageCaller
	token string
	calls int
}

func (c *externalIdentityCaller) CallToolWithToken(_ context.Context, token, product, tool string, _ map[string]any) (*edition.ToolResult, error) {
	c.token = token
	c.calls++
	if product != "contact" || tool != "get_current_user_profile" {
		return nil, fmt.Errorf("wrong identity tool")
	}
	return c.result, c.err
}

func externalExchangeRoot(t *testing.T, caller edition.ToolCaller) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	root := &cobra.Command{Use: "dws", TraverseChildren: true, SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().String("format", "json", "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("client-id", "", "")
	root.PersistentFlags().String("client-secret", "", "")
	auth := &cobra.Command{Use: "auth", TraverseChildren: true}
	auth.AddCommand(newAuthExchangeCommand(caller))
	root.AddCommand(auth)
	root.SetContext(context.Background())
	out := new(bytes.Buffer)
	root.SetOut(out)
	root.SetErr(out)
	return root, out
}

func TestCrossPlatformCoverageExternalExchangeKeepsUpstreamFlagsVisible(t *testing.T) {
	cmd := newAuthExchangeCommand(nil)
	for _, name := range []string{"uid", "authorize-url", "token-url", "refresh-url", "redirect-url", "scopes"} {
		flag := cmd.Flags().Lookup(name)
		if flag == nil || flag.Hidden {
			t.Errorf("upstream auth exchange flag --%s must remain visible", name)
		}
	}
}

func TestCrossPlatformCoverageExternalExchangeCLIApplicationFlagsAndIdentityReadback(t *testing.T) {
	for _, tc := range []struct {
		name      string
		args      []string
		app       string
		discovery int
	}{
		{"default-after", []string{"auth", "exchange", "--client-id", "default"}, "official-app", 1},
		{"default-before", []string{"--client-id", "default", "auth", "exchange"}, "official-app", 1},
		{"custom-after", []string{"auth", "exchange", "--client-id", "employee-app"}, "employee-app", 0},
		{"custom-before", []string{"--client-id", "employee-app", "auth", "exchange"}, "employee-app", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("DWS_CONFIG_DIR", dir)
			t.Setenv(keychain.StorageDirEnv, filepath.Join(dir, "keys"))
			t.Setenv(keychain.DisableKeychainEnv, "1")
			oldID, oldSecret, oldProfile := authpkg.ClientID(), authpkg.ClientSecret(), authpkg.RuntimeProfile()
			authpkg.SetClientID("supervisor-app")
			authpkg.SetClientSecret("supervisor-secret")
			t.Cleanup(func() {
				authpkg.SetClientID(oldID)
				authpkg.SetClientSecret(oldSecret)
				authpkg.SetRuntimeProfile(oldProfile)
			})
			requests, discoveries := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					discoveries++
					io.WriteString(w, `{"success":true,"result":"official-app"}`)
					return
				}
				requests++
				var body map[string]string
				json.NewDecoder(r.Body).Decode(&body)
				if body["clientId"] != tc.app || body["authCode"] != "test-code" || body["clientSecret"] != "" {
					t.Error("CLI parameter did not reach exchange correctly")
				}
				io.WriteString(w, `{"accessToken":"employee-access","refreshToken":"employee-refresh","expiresIn":7200,"corpId":"employee-corp"}`)
			}))
			t.Cleanup(server.Close)
			if err := os.WriteFile(filepath.Join(dir, "mcp_url"), []byte(server.URL), 0600); err != nil {
				t.Fatal(err)
			}
			caller := &externalIdentityCaller{authCoverageCaller: authCoverageCaller{result: &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"result":[{"orgEmployeeModel":{"corpId":"employee-corp","userId":"employee-user","orgUserName":"Employee"}}]}`}}}}}
			root, out := externalExchangeRoot(t, caller)
			root.SetIn(strings.NewReader("test-code\n"))
			root.SetArgs(append(tc.args, "--code-stdin"))
			if err := root.Execute(); err != nil {
				t.Fatalf("exchange: %v", err)
			}
			if caller.token != "employee-access" || caller.calls != 1 || requests != 1 || discoveries != tc.discovery {
				t.Fatal("incorrect token or request count")
			}
			var result struct {
				Success bool `json:"success"`
				Data    struct {
					Profile string `json:"dwsProfile"`
					Current string `json:"currentProfile"`
					Client  string `json:"clientId"`
				} `json:"data"`
			}
			if err := json.Unmarshal(out.Bytes(), &result); err != nil || !result.Success || result.Data.Profile != "employee-corp:employee-user" || result.Data.Current != result.Data.Profile || result.Data.Client != tc.app {
				t.Fatalf("invalid JSON login result: %v", err)
			}
			for _, secret := range []string{"test-code", "employee-access", "employee-refresh", "supervisor-secret"} {
				if strings.Contains(out.String(), secret) {
					t.Fatal("secret in output")
				}
			}
			saved, err := authpkg.LoadTokenDataForProfile(dir, result.Data.Profile)
			if err != nil || saved.UserID != "employee-user" || saved.ClientID != tc.app {
				t.Fatalf("incomplete saved login: %v", err)
			}
			if authpkg.ClientID() != "supervisor-app" {
				t.Fatal("mutated global application")
			}
		})
	}
}

type exchangeUnreadableInput struct{}

func (exchangeUnreadableInput) Read([]byte) (int, error) { panic("dry-run must not read stdin") }

func TestCrossPlatformCoverageExternalExchangeCLIValidationAndDryRun(t *testing.T) {
	testseam.Swap(t, &authExternalExchange, func(context.Context, string, authpkg.ExternalExchangeRequest) (*authpkg.TokenData, error) {
		t.Fatal("validation/dry-run invoked exchange")
		return nil, nil
	})
	for _, tc := range []struct {
		args    []string
		success bool
	}{
		{[]string{"--code-stdin", "--dry-run", "--client-id", "default"}, true},
		{[]string{"--code", "test-code", "--dry-run"}, true},
		{[]string{"--code", "test-code", "--code-stdin"}, false},
		{[]string{"--code-stdin", "--mcp"}, false},
		{[]string{"--code-stdin", "--client-id", ""}, false},
		{[]string{"--code-stdin", "--client-id", "default", "--client-secret", "test-secret"}, false},
		{[]string{"--code-stdin", "--uid", "one", "--expected-user-id", "two"}, false},
		{nil, false},
	} {
		root, out := externalExchangeRoot(t, nil)
		root.SetIn(exchangeUnreadableInput{})
		root.SetArgs(append([]string{"auth", "exchange"}, tc.args...))
		err := root.Execute()
		if (err == nil) != tc.success {
			t.Fatalf("args %v: %v", tc.args, err)
		}
		if strings.Contains(out.String(), "test-code") || strings.Contains(out.String(), "test-secret") {
			t.Fatal("secret in output")
		}
	}
}

func TestCrossPlatformCoverageExternalExchangeRealRootPreRunDoesNotOverrideApplication(t *testing.T) {
	oldID, oldSecret, oldProfile := authpkg.ClientID(), authpkg.ClientSecret(), authpkg.RuntimeProfile()
	t.Cleanup(func() {
		authpkg.SetClientID(oldID)
		authpkg.SetClientSecret(oldSecret)
		authpkg.SetRuntimeProfile(oldProfile)
	})
	for _, args := range [][]string{
		{"--client-id", "default", "auth", "exchange", "--code-stdin", "--dry-run"},
		{"auth", "exchange", "--client-id", "custom-app", "--code-stdin", "--dry-run"},
	} {
		authpkg.SetClientID("original-app")
		authpkg.SetClientSecret("original-secret")
		root := NewRootCommand()
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		root.SetIn(exchangeUnreadableInput{})
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if authpkg.ClientID() != "original-app" || authpkg.ClientSecret() != "original-secret" {
			t.Fatal("root pre-run changed application")
		}
	}
}
