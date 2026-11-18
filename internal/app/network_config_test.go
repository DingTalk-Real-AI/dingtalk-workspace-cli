// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/netpolicy"
	"github.com/spf13/cobra"
)

func networkCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	flags := &GlobalFlags{}
	root := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	corecmd.ApplyGroupPolicy(root, corecmd.GroupPolicy{Mode: corecmd.GroupNavigationOnly, Positionals: corecmd.PositionalsReject, Recovery: corecmd.RecoverySibling})
	bindPersistentFlags(root, flags)
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error { return configureNetworkInvocation(cmd, flags) }
	config := newConfigCommand()
	corecmd.ApplyGroupPolicy(config, corecmd.GroupPolicy{Mode: corecmd.GroupNavigationOnly, Positionals: corecmd.PositionalsReject, Recovery: corecmd.RecoverySibling})
	root.AddCommand(config)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := corecmd.ExecuteForTest(root)
	return out.String(), err
}

func clearNetworkEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"DWS_PROXY", "DWS_NO_PROXY", "DWS_CA_BUNDLE", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy", "ALL_PROXY", "all_proxy", "REQUEST_METHOD"} {
		old, exists := os.LookupEnv(k)
		os.Unsetenv(k)
		t.Cleanup(func() {
			if exists {
				os.Setenv(k, old)
			} else {
				os.Unsetenv(k)
			}
		})
	}
}

func TestNetworkConfigurationCLI(t *testing.T) {
	clearNetworkEnv(t)
	dir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", dir)
	call := func(args ...string) string {
		t.Helper()
		out, err := networkCLI(t, args...)
		if err != nil {
			t.Fatal(err, out)
		}
		return out
	}
	call("config", "network", "set", "--proxy", "http://proxy.example:7890", "--no-proxy", "corp.example")
	out := call("config", "network", "show", "--effective")
	if !strings.Contains(out, "proxy.example:7890") || !strings.Contains(out, `"source": "profile"`) {
		t.Fatal(out)
	}
	out = call("config", "network", "explain", "--url", "https://corp.example/private?token=secret")
	if !strings.Contains(out, `"route": "direct"`) || strings.Contains(out, "secret") || strings.Contains(out, "private") {
		t.Fatal(out)
	}
	t.Setenv("DWS_PROXY", "direct")
	out = call("--proxy", "http://override:7890", "--no-proxy", "", "config", "network", "explain", "--url", "https://corp.example")
	if !strings.Contains(out, `"route": "proxy"`) || !strings.Contains(out, "override:7890") {
		t.Fatal(out)
	}
	if out, err := networkCLI(t, "config", "network", "set", "--proxy", "http://user:secret@proxy:80"); err == nil || strings.Contains(out+err.Error(), "secret") {
		t.Fatal("credential persistence/redaction", out, err)
	}
	call("config", "network", "reset")
	f, err := netpolicy.Load(dir)
	if err != nil || f.Default.Proxy != nil {
		t.Fatal(f, err)
	}
}

func TestNetworkProfileIsolationAndDryRun(t *testing.T) {
	clearNetworkEnv(t)
	dir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", dir)
	metadata := `{"version":3,"currentProfile":"corp:user1","profiles":[{"name":"one","corpId":"corp","userId":"user1"},{"name":"two","corpId":"corp","userId":"user2"}]}`
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte(metadata), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := networkCLI(t, "--profile", "corp:user1", "config", "network", "set", "--proxy", "http://one:80"); err != nil {
		t.Fatal(out, err)
	}
	if out, err := networkCLI(t, "--profile", "corp:user2", "config", "network", "set", "--proxy", "direct"); err != nil {
		t.Fatal(out, err)
	}
	for _, tc := range []struct{ selector, want string }{{"one", "http://one:80"}, {"corp:user2", "direct"}} {
		out, err := networkCLI(t, "--profile", tc.selector, "config", "network", "show", "--effective")
		if err != nil || !strings.Contains(out, tc.want) {
			t.Fatal(out, err)
		}
	}
	before, _ := os.ReadFile(filepath.Join(dir, "network.json"))
	out, err := networkCLI(t, "config", "network", "reset", "--dry-run")
	if err != nil {
		t.Fatal(out, err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "network.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("dry run wrote configuration")
	}
	if out, err := networkCLI(t, "--profile", "missing", "config", "network", "set", "--proxy", "direct"); err == nil {
		t.Fatal(out, "missing profile accepted")
	}
}

func TestNetworkRootAndRunnerSnapshot(t *testing.T) {
	clearNetworkEnv(t)
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	root := NewRootCommand()
	root.SetArgs([]string{"--proxy", "http://user:secret@proxy.example:80", "config", "network", "explain", "--url", "https://files.example/private?key=secret"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatal(err, out.String())
	}
	var result map[string]any
	if json.Unmarshal(out.Bytes(), &result) != nil || result["route"] != "proxy" || strings.Contains(out.String(), "secret") {
		t.Fatal(out.String())
	}
	// Legacy MCP helpers can pass Background. The runner must restore the
	// owning command's captured policy instead of consulting changed env.
	flags := &GlobalFlags{Proxy: "http://captured:80"}
	cmd := &cobra.Command{Use: "leaf"}
	bindPersistentFlags(cmd, flags)
	cmd.Flags().AddFlagSet(cmd.PersistentFlags())
	cmd.Flags().Set("proxy", "http://captured:80")
	cmd.SetContext(context.Background())
	if err := configureNetworkInvocation(cmd, flags); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DWS_PROXY", "direct")
	r := &runtimeRunner{globalFlags: flags}
	ctx, err := r.withNetworkContext(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse("https://any.example")
	e, err := netpolicy.FromContext(ctx).Explain(u)
	if err != nil || e.Proxy != "http://captured:80" {
		t.Fatal(e, err)
	}
}
