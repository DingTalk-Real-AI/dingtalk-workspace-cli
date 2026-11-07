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

package unit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShouldHaveNoWriteFallbackWhenChatThreadPromoteRetiredGivenIntentGuides(t *testing.T) {
	root := repoRoot(t)
	want := "| \"把群里这条已有消息转成 Thread/升级成群内话题\" | 消息升级为 Thread（已下线） | — | — | 当前无等价替代；仅当用户改为新建话题时才使用 `chat thread send` |"
	for _, path := range []string{
		filepath.Join(root, "skills", "mono", "references", "intent-guide.md"),
		filepath.Join(root, "skills", "multi", "dingtalk-shared", "references", "intent-guide.md"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		content := string(data)
		if !strings.Contains(content, want) {
			t.Fatalf("retired promote route in %s must not publish a write fallback", path)
		}
		if strings.Contains(content, "| — | `chat thread send` | promote 已下线") {
			t.Fatalf("retired promote route in %s still treats send as an equivalent fallback", path)
		}
	}
}
