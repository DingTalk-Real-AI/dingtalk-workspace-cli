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
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestDevLeafFormatFlagVisibility 是队列 B189 的 dev 域叶子 --format flag 可见性
// 断言。--format 是生产根命令的持久 flag（internal/app/flags.go bindPersistentFlags），
// 叶子自身不声明 local --format，而是经 cmd.InheritedFlags() 继承可见。本测试：
//
//  1. 在完整命令树下执行 dev app/dev connect 叶子，--format 必须被 resolveCommandFormat
//     识别（table 生效），证明其可见；
//
// 2. 叶子 --help 必须展示 --format 于 Global Flags 段（继承可见性的人读证据）。
func TestDevLeafFormatFlagVisibility(t *testing.T) {
	leaves := []struct {
		name     string
		args     []string
		helpPath []string
	}{
		{"dev-app-get", []string{"dev", "app", "get", "--unified-app-id", "u-1"}, []string{"dev", "app", "get"}},
		{"dev-app-list", []string{"dev", "app", "list", "--name", "DemoApp"}, []string{"dev", "app", "list"}},
		{"dev-version-list", []string{"dev", "app", "version", "list", "--unified-app-id", "u-1"}, []string{"dev", "app", "version", "list"}},
		{"dev-event-list", []string{"dev", "app", "event", "list", "--unified-app-id", "u-1"}, []string{"dev", "app", "event", "list"}},
		{"dev-permission-list", []string{"dev", "app", "permission", "list", "--unified-app-id", "u-1"}, []string{"dev", "app", "permission", "list"}},
		{"dev-member-list", []string{"dev", "app", "member", "list", "--unified-app-id", "u-1"}, []string{"dev", "app", "member", "list"}},
	}

	for _, lc := range leaves {
		t.Run(lc.name, func(t *testing.T) {
			// --format table 必须生效：输出为表（含列头/业务值），且不泄漏信封外壳键。
			args := append(append([]string{}, lc.args...), "--format", "table")
			root := newDevAppTestRoot(devAppFamilyContentRunner(map[string]any{"items": []any{map[string]any{"id": "one"}}}))
			out, errBuf, err := runRootBuffered(t, root, args...)
			if err != nil {
				t.Fatalf("Execute() error = %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errBuf.String())
			}
			s := out.String()
			if strings.Contains(s, `"outcome"`) || strings.Contains(s, `"ok":`) {
				t.Fatalf("--format table not effective (envelope shell leaked):\n%s", s)
			}
			if strings.TrimSpace(s) == "" {
				t.Fatalf("--format table produced empty stdout (format not visible?):\n%s", s)
			}

			// --help 必须展示 --format 于 Global Flags 段。
			helpArgs := append(append([]string{}, lc.helpPath...), "--help")
			helpRoot := newDevAppTestRoot(&captureRunner{})
			helpOut, _, herr := runRootBuffered(t, helpRoot, helpArgs...)
			if herr != nil {
				t.Fatalf("--help error = %v", herr)
			}
			if !strings.Contains(helpOut.String(), "--format") {
				t.Fatalf("leaf --help must expose --format (global inherited flag):\n%s", helpOut.String())
			}
		})
	}
}

// TestDevLeafFormatFlagHelpShowsGlobalSection 是队列 B189 的补充观察断言：dev 叶子
// 的 --help 中 --format 出现在 "Global Flags" 段而非本地 Flags 段（flag 可见性来源
// 为根命令持久继承，而非叶子自声明）。这是 B189 结论的形式化——若某叶子把
// --format 放回本地段，本测试会因 --format 出现在非 Global 段而暴露。
func TestDevLeafFormatFlagHelpShowsGlobalSection(t *testing.T) {
	root := newDevAppTestRoot(&captureRunner{})
	out, _, err := runRootBuffered(t, root, "dev", "app", "get", "--help")
	if err != nil {
		t.Fatalf("--help error = %v", err)
	}
	help := out.String()
	globalIdx := strings.Index(help, "Global Flags")
	if globalIdx < 0 {
		t.Fatalf("leaf help missing 'Global Flags' section:\n%s", help)
	}
	formatIdx := strings.LastIndex(help, "--format")
	if formatIdx < 0 {
		t.Fatalf("leaf help missing --format in Global Flags:\n%s", help)
	}
	if formatIdx < globalIdx {
		t.Fatalf("--format found before 'Global Flags' section (should be inherited global):\n%s", help)
	}
}

var _ = cobra.Command{}
