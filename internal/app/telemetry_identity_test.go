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
	"errors"
	"reflect"
	"strings"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageResolveTelemetryIdentityProfileSelection(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", "/telemetry-config")
	profiles := map[string]*authpkg.ProfileMetadata{
		"":           {UserID: " default-user ", UserName: " Default User ", CorpID: " default-corp "},
		"corp-a":     {UserID: "user-a", UserName: "Alice", CorpID: "corp-a"},
		"corp-b":     {UserID: "user-b", UserName: "Bob", CorpID: "corp-b"},
		"alpha,beta": {UserID: "comma-user", UserName: "Comma User", CorpID: "comma-corp"},
	}

	for _, tc := range []struct {
		name      string
		args      []string
		profiles  map[string]*authpkg.ProfileMetadata
		wantCalls []string
		want      TelemetryIdentity
	}{
		{name: "default profile", args: []string{"version"}, profiles: profiles, wantCalls: []string{""}, want: TelemetryIdentity{UserID: "default-user", UserName: "Default User", CorpID: "default-corp"}},
		{name: "single profile", args: []string{"--profile", "corp-a", "version"}, profiles: profiles, wantCalls: []string{"corp-a"}, want: TelemetryIdentity{UserID: "user-a", UserName: "Alice", CorpID: "corp-a"}},
		{name: "equals form after command", args: []string{"version", "--profile=corp-b"}, profiles: profiles, wantCalls: []string{"corp-b"}, want: TelemetryIdentity{UserID: "user-b", UserName: "Bob", CorpID: "corp-b"}},
		{name: "last repeated profile", args: []string{"--profile", "corp-a", "version", "--profile=corp-b"}, profiles: profiles, wantCalls: []string{"corp-b"}, want: TelemetryIdentity{UserID: "user-b", UserName: "Bob", CorpID: "corp-b"}},
		{name: "comma profile name", args: []string{"--profile", "alpha,beta", "version"}, profiles: profiles, wantCalls: []string{"alpha,beta"}, want: TelemetryIdentity{UserID: "comma-user", UserName: "Comma User", CorpID: "comma-corp"}},
		{name: "multi profile uses default", args: []string{"--profile", "corp-a,corp-b", "version"}, profiles: profiles, wantCalls: []string{"corp-a,corp-b", "corp-a", "corp-b", ""}, want: TelemetryIdentity{UserID: "default-user", UserName: "Default User", CorpID: "default-corp"}},
		{name: "unquoted multi profile", args: []string{"--profile", "corp-a,", "corp-b", "version"}, profiles: profiles, wantCalls: []string{"corp-a,corp-b", "corp-a", "corp-b", ""}, want: TelemetryIdentity{UserID: "default-user", UserName: "Default User", CorpID: "default-corp"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			testseam.Swap(t, &telemetryResolveProfileMetadata, func(configDir, selector string) (*authpkg.ProfileMetadata, error) {
				if configDir != "/telemetry-config" {
					t.Fatalf("resolver config dir = %q", configDir)
				}
				calls = append(calls, selector)
				profile := tc.profiles[selector]
				if profile == nil {
					return nil, errors.New("profile not found")
				}
				clone := *profile
				return &clone, nil
			})

			if got := ResolveTelemetryIdentity(tc.args); got != tc.want {
				t.Fatalf("ResolveTelemetryIdentity() = %#v, want %#v", got, tc.want)
			}
			if !reflect.DeepEqual(calls, tc.wantCalls) {
				t.Fatalf("metadata selectors = %#v, want %#v", calls, tc.wantCalls)
			}
		})
	}
}

func TestCrossPlatformCoverageResolveTelemetryIdentityRejectsMissingProfileValue(t *testing.T) {
	for _, args := range [][]string{
		{"version", "--profile"},
		{"--profile=corp-a", "version", "--profile="},
		{"--profile", "--debug", "version"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			testseam.Swap(t, &telemetryResolveProfileMetadata, func(string, string) (*authpkg.ProfileMetadata, error) {
				t.Fatal("invalid profile syntax attempted metadata resolution")
				return nil, nil
			})
			if got := ResolveTelemetryIdentity(args); got != (TelemetryIdentity{}) {
				t.Fatalf("invalid profile identity = %#v", got)
			}
		})
	}
}

func TestCrossPlatformCoverageResolveTelemetryIdentityFailsClosed(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", "/telemetry-config")
	fail := errors.New("metadata unavailable")

	for _, tc := range []struct {
		name    string
		resolve func(string, string) (*authpkg.ProfileMetadata, error)
	}{
		{name: "read error", resolve: func(string, string) (*authpkg.ProfileMetadata, error) { return nil, fail }},
		{name: "missing profile", resolve: func(string, string) (*authpkg.ProfileMetadata, error) { return nil, nil }},
		{name: "empty fields", resolve: func(string, string) (*authpkg.ProfileMetadata, error) { return &authpkg.ProfileMetadata{}, nil }},
		{name: "resolver panic", resolve: func(string, string) (*authpkg.ProfileMetadata, error) { panic("metadata failure") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testseam.Swap(t, &telemetryResolveProfileMetadata, tc.resolve)
			if got := ResolveTelemetryIdentity(nil); got != (TelemetryIdentity{}) {
				t.Fatalf("failed-closed identity = %#v", got)
			}
		})
	}
}

func TestCrossPlatformCoverageResolveTelemetryIdentityRejectsInvalidMultiProfile(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", "/telemetry-config")
	testseam.Swap(t, &telemetryResolveProfileMetadata, func(_ string, selector string) (*authpkg.ProfileMetadata, error) {
		if selector == "corp-a" {
			return &authpkg.ProfileMetadata{UserID: "user-a", CorpID: "corp-a"}, nil
		}
		return nil, errors.New("profile not found")
	})
	if got := ResolveTelemetryIdentity([]string{"--profile", "corp-a,missing", "version"}); got != (TelemetryIdentity{}) {
		t.Fatalf("invalid multi-profile identity = %#v", got)
	}
}

func TestCrossPlatformCoverageResolveTelemetryProfileMetadataRejectsMalformedMulti(t *testing.T) {
	testseam.Swap(t, &telemetryResolveProfileMetadata, func(_ string, selector string) (*authpkg.ProfileMetadata, error) {
		switch selector {
		case "corp-a,,corp-b":
			return nil, errors.New("not a literal profile")
		case "corp-a":
			return &authpkg.ProfileMetadata{UserID: "user-a"}, nil
		case "missing":
			return nil, nil
		default:
			return nil, errors.New("unexpected selector")
		}
	})
	if _, err := resolveTelemetryProfileMetadata("/config", "corp-a,,corp-b"); err == nil || !strings.Contains(err.Error(), "empty profile selector") {
		t.Fatalf("empty multi-profile selector error = %v", err)
	}
	if _, err := resolveTelemetryProfileMetadata("/config", "corp-a,missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing multi-profile selector error = %v", err)
	}
}
