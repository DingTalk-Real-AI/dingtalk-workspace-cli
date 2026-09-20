// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageAuthStatusReadOnlyOutputParity(t *testing.T) {
	for _, state := range []string{"valid", "expired", "not_logged_in"} {
		for _, format := range []string{"json", "table"} {
			t.Run(state+"/"+format, func(t *testing.T) {
				token := authLogoutTestToken("corp-readonly-parity")
				if state == "expired" {
					token.ExpiresAt = time.Now().Add(-time.Hour)
					token.RefreshExpAt = time.Now().Add(-time.Hour)
				}
				dir := setupAuthLogoutProfiles(t, token)
				if state == "not_logged_in" {
					if err := authpkg.SaveProfiles(dir, &authpkg.ProfilesConfig{Version: 2}); err != nil {
						t.Fatal(err)
					}
				}
				var normal, readonly bytes.Buffer
				for _, readonlyMode := range []bool{false, true} {
					cmd := newAuthStatusCommand()
					cmd.PersistentFlags().String("format", format, "")
					if readonlyMode {
						cmd.SetArgs([]string{"--readonly"})
						cmd.SetOut(&readonly)
					} else {
						cmd.SetArgs([]string{})
						cmd.SetOut(&normal)
					}
					if err := cmd.Execute(); err != nil {
						t.Fatal(err)
					}
				}
				if !bytes.Equal(normal.Bytes(), readonly.Bytes()) {
					t.Fatalf("output differs: normal=%s readonly=%s", normal.String(), readonly.String())
				}
			})
		}
	}
}

func TestCrossPlatformCoverageAuthStatusReadOnlyFlagSemantics(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want bool
	}{
		{"default", []string{"auth", "status"}, false},
		{"enabled", []string{"auth", "status", "--readonly"}, true},
		{"explicit true", []string{"auth", "status", "--readonly=true"}, true},
		{"explicit false", []string{"auth", "status", "--readonly=false"}, false},
		{"last false", []string{"auth", "status", "--readonly", "--readonly=false"}, false},
		{"last true", []string{"auth", "status", "--readonly=false", "--readonly"}, true},
		{"terminator", []string{"auth", "status", "--", "--readonly"}, false},
		{"profile value", []string{"auth", "status", "--profile=--readonly"}, false},
		{"flags before command", []string{"--format", "json", "auth", "status", "--readonly"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := &cobra.Command{Use: "dws"}
			root.PersistentFlags().String("format", "", "")
			root.AddCommand(buildAuthCommand(nil))
			cmd, args, err := root.Find(tc.args)
			if err != nil {
				t.Fatal(err)
			}
			if got := authStatusReadOnlyRequested(cmd, args); got != tc.want {
				t.Fatalf("startup readonly=%v, want %v", got, tc.want)
			}
			if cmd.Flags().Changed("readonly") || root.PersistentFlags().Changed("format") {
				t.Fatal("startup detection changed real flags")
			}
			if err := cmd.ParseFlags(args); err != nil {
				t.Fatal(err)
			}
			if got := isAuthStatusReadOnlyCommand(cmd); got != tc.want {
				t.Fatalf("readonly=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestCrossPlatformCoverageAuthStatusReadOnlyReplacesInspect(t *testing.T) {
	if isAuthStatusReadOnlyCommand(nil) {
		t.Fatal("nil command cannot enable readonly mode")
	}
	for _, cmd := range buildAuthCommand(nil).Commands() {
		if cmd.Name() == "inspect" {
			t.Fatal("retired auth inspect is still mounted")
		}
		if isAuthStatusReadOnlyCommand(cmd) {
			t.Fatal("an ordinary auth command enabled readonly mode")
		}
	}
	cmd := newAuthStatusCommand()
	if flag := cmd.Flags().Lookup("readonly"); flag == nil || flag.DefValue != "false" {
		t.Fatal("readonly must be an opt-in flag")
	}
}
