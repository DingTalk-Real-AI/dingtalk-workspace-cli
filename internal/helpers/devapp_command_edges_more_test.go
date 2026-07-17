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

package helpers

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

func executeDevAppEdge(runner executor.Runner, args ...string) (string, error) {
	root := newDevAppTestRoot(runner)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"dev", "app"}, args...))
	err := root.Execute()
	return out.String(), err
}

func TestCrossPlatformCoverageDevAppParentHelpCoverage(t *testing.T) {
	for _, path := range [][]string{
		{}, {"webapp"}, {"permission"}, {"credentials"}, {"member"},
		{"security"}, {"robot"}, {"version"}, {"event"},
	} {
		if _, err := executeDevAppEdge(&captureRunner{}, path...); err != nil {
			t.Fatalf("help %v: %v", path, err)
		}
	}
}

func TestCrossPlatformCoverageDevAppValidationBranchesCoverage(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"event list id", []string{"event", "list"}},
		{"event subscribe guard", []string{"event", "subscribe"}},
		{"event subscribe id", []string{"event", "subscribe", "--dry-run"}},
		{"event subscribe codes", []string{"event", "subscribe", "--dry-run", "--unified-app-id", "app"}},
		{"event unsubscribe guard", []string{"event", "unsubscribe"}},
		{"event unsubscribe id", []string{"event", "unsubscribe", "--dry-run"}},
		{"event unsubscribe codes", []string{"event", "unsubscribe", "--dry-run", "--unified-app-id", "app"}},
		{"get id", []string{"get"}},
		{"create name", []string{"create", "--dry-run"}},
		{"update guard", []string{"update"}},
		{"update id", []string{"update", "--dry-run"}},
		{"update fields", []string{"update", "--dry-run", "--unified-app-id", "app"}},
		{"credentials id", []string{"credentials", "get"}},
		{"disable guard", []string{"disable"}},
		{"enable id", []string{"enable", "--dry-run"}},
		{"delete guard", []string{"delete"}},
		{"delete id", []string{"delete", "--dry-run"}},
		{"delete confirm", []string{"delete", "--yes", "--unified-app-id", "app"}},
		{"webapp get id", []string{"webapp", "get"}},
		{"webapp config guard", []string{"webapp", "config"}},
		{"webapp config id", []string{"webapp", "config", "--dry-run"}},
		{"webapp config fields", []string{"webapp", "config", "--dry-run", "--unified-app-id", "app"}},
		{"permission list id", []string{"permission", "list"}},
		{"permission add guard", []string{"permission", "add"}},
		{"permission add id", []string{"permission", "add", "--dry-run"}},
		{"permission add scopes", []string{"permission", "add", "--dry-run", "--unified-app-id", "app"}},
		{"permission remove guard", []string{"permission", "remove"}},
		{"permission remove id", []string{"permission", "remove", "--dry-run"}},
		{"permission remove scopes", []string{"permission", "remove", "--dry-run", "--unified-app-id", "app"}},
		{"member list id", []string{"member", "list"}},
		{"member add guard", []string{"member", "add"}},
		{"member add id", []string{"member", "add", "--dry-run"}},
		{"security guard", []string{"security", "config"}},
		{"security id", []string{"security", "config", "--dry-run"}},
		{"security fields", []string{"security", "config", "--dry-run", "--unified-app-id", "app"}},
		{"robot submit guard", []string{"robot", "submit"}},
		{"robot submit params", []string{"robot", "submit", "--dry-run"}},
		{"robot result task", []string{"robot", "result"}},
		{"robot get id", []string{"robot", "get"}},
		{"robot config guard", []string{"robot", "config"}},
		{"robot config id", []string{"robot", "config", "--dry-run"}},
		{"robot config fields", []string{"robot", "config", "--dry-run", "--unified-app-id", "app"}},
		{"robot enable guard", []string{"robot", "enable"}},
		{"robot enable id", []string{"robot", "enable", "--dry-run"}},
		{"robot disable guard", []string{"robot", "disable"}},
		{"robot disable id", []string{"robot", "disable", "--dry-run"}},
		{"version create id", []string{"version", "create", "--dry-run"}},
		{"version list id", []string{"version", "list"}},
		{"version get app", []string{"version", "get"}},
		{"version get version", []string{"version", "get", "--unified-app-id", "app"}},
		{"version approval app", []string{"version", "check-approval"}},
		{"version publish guard", []string{"version", "publish"}},
		{"version publish app", []string{"version", "publish", "--dry-run"}},
		{"version status app", []string{"version", "status"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := executeDevAppEdge(&captureRunner{}, tc.args...); err == nil {
				t.Fatalf("Execute(%v) returned nil", tc.args)
			}
		})
	}
}

func TestCrossPlatformCoverageDevAppOptionalMutationBranchesCoverage(t *testing.T) {
	success := [][]string{
		{"update", "--dry-run", "--unified-app-id", "app", "--icon-media-id", "media"},
		{"delete", "--dry-run", "--unified-app-id", "app"},
		{"webapp", "config", "--dry-run", "--unified-app-id", "app", "--h5-page-type", "HOME", "--omp-url", "https://omp.invalid"},
		{"robot", "config", "--dry-run", "--unified-app-id", "app", "--disable-ssl-verify", "--i18n-name", `{"zh_CN":"助手"}`},
	}
	for _, args := range success {
		if _, err := executeDevAppEdge(&captureRunner{}, args...); err != nil {
			t.Fatalf("Execute(%v): %v", args, err)
		}
	}
	if _, err := executeDevAppEdge(&captureRunner{}, "robot", "config", "--dry-run", "--unified-app-id", "app", "--mode", "bad"); err == nil {
		t.Fatal("invalid robot mode returned nil")
	}
	if _, err := executeDevAppEdge(&captureRunner{}, "robot", "config", "--dry-run", "--unified-app-id", "app", "--i18n-brief", "{"); err == nil {
		t.Fatal("invalid i18n JSON returned nil")
	}
}

func TestCrossPlatformCoverageDevAppRobotCreateRequiredFieldsCoverage(t *testing.T) {
	cmd := newDevAppRobotSubmitCommand(&captureRunner{})
	if _, err := devAppRobotCreateParams(cmd); err == nil {
		t.Fatal("missing name returned nil")
	}
	_ = cmd.Flags().Set("name", "app")
	if _, err := devAppRobotCreateParams(cmd); err == nil {
		t.Fatal("missing robot name returned nil")
	}
	_ = cmd.Flags().Set("robot-name", "robot")
	if _, err := devAppRobotCreateParams(cmd); err == nil {
		t.Fatal("missing desc returned nil")
	}
}

func TestCrossPlatformCoverageDevAppDeleteFetchErrorCoverage(t *testing.T) {
	want := errors.New("fetch app failed")
	_, err := executeDevAppEdge(devAppErrorRunner{err: want}, "delete", "--yes", "--unified-app-id", "app", "--confirm-name", "name")
	if !errors.Is(err, want) {
		t.Fatalf("delete error = %v", err)
	}
}

func TestCrossPlatformCoverageDevAppPrettyAndAnnotationEdges(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	annotateDevAppTool(cmd, "tool")
	if cmd.Annotations["mcp-tool"] != "tool" {
		t.Fatalf("annotations = %#v", cmd.Annotations)
	}

	// Pretty output takes a separate annotation path after normalization.
	runner := &devAppResponseRunner{response: map[string]any{
		"content": map[string]any{"success": true, "result": map[string]any{"id": "app"}},
	}}
	out, err := executeDevAppEdge(runner, "list", "--format", "pretty")
	if err != nil || !strings.Contains(out, "app") {
		t.Fatalf("pretty list err=%v out=%q", err, out)
	}
}
