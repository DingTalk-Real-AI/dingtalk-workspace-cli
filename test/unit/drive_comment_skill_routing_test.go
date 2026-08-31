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

func TestDriveCommentSkillIntentRoutesUseV2(t *testing.T) {
	root := repoRoot(t)
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "mono",
			path: filepath.Join(root, "skills", "mono", "references", "products", "drive.md"),
			want: "→ `comment list-v2/create-v2/reply/update/delete/batch-query/list-replies/resolve/restore/react-reply`",
		},
		{
			name: "multi",
			path: filepath.Join(root, "skills", "multi", "dingtalk-drive", "references", "intent-guide.md"),
			want: "Drive `comment list-v2/create-v2/reply/update/delete/batch-query/list-replies/resolve/restore/react-reply`",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), tc.want) {
				t.Fatalf("Drive comment intent route in %s does not use v2: want %q", tc.path, tc.want)
			}
		})
	}
}
