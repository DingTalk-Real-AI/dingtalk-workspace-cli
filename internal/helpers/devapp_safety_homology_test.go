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

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/spf13/cobra"
)

// Homology §1.1: confirmation facts are Contract-declared SafetySpec values or
// explicitly annotated runtime gates. Write leaves must not rely on inference.
func TestDevAppWriteLeavesDeclareOrAnnotateConfirmation(t *testing.T) {
	root := newDevAppCommand(&captureRunner{})
	want := map[string]struct{}{
		"create":            {},
		"update":            {},
		"delete":            {},
		"enable":            {},
		"disable":           {},
		"event subscribe":   {},
		"event unsubscribe": {},
		"webapp config":     {},
		"permission add":    {},
		"permission remove": {},
		"member add":        {},
		"member remove":     {},
		"security config":   {},
		"robot submit":      {},
		"robot config":      {},
		"robot enable":      {},
		"robot disable":     {},
		"version create":    {},
		"version publish":   {},
	}

	var walk func(cmd *cobra.Command, rel string)
	walk = func(cmd *cobra.Command, rel string) {
		for _, child := range cmd.Commands() {
			path := child.Name()
			if rel != "" {
				path = rel + " " + child.Name()
			}
			if len(child.Commands()) > 0 {
				walk(child, path)
				continue
			}
			// devapp 全树已声明化：确认事实只能来自 Contract SafetySpec，
			// 任何 runtime_gate 标注都是回归（声明 > 标注，旧路径专用）。
			if gate, hasGate := cli.RuntimeContractGate(child); hasGate {
				t.Errorf("%s: unexpected runtime_gate %q; devapp leaves declare Contract SafetySpec", path, gate)
			}
			if _, ok := want[path]; !ok {
				continue
			}
			if !cli.HasDeclaredOrAnnotatedConfirmation(child) {
				t.Errorf("%s: missing Contract SafetySpec or runtime_gate annotation", path)
			}
			delete(want, path)
		}
	}
	walk(root, "")

	if len(want) > 0 {
		missing := make([]string, 0, len(want))
		for path := range want {
			missing = append(missing, path)
		}
		t.Fatalf("write leaves not found under app tree: %s", strings.Join(missing, ", "))
	}
}
