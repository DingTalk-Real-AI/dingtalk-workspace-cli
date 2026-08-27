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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageOpenAPIExplorerSkillDiscoveryAndSafetyContract(t *testing.T) {
	paths := []string{
		filepath.Join("..", "..", "skills", "multi", "dingtalk-misc", "references", "openapi-explorer.md"),
		filepath.Join("..", "..", "skills", "mono", "references", "products", "openapi-explorer.md"),
	}
	markers := []string{
		"https://open.dingtalk.com/llms.txt",
		"llms-*.txt",
		"推荐接口",
		"企业内部应用 + App Token",
		"User Token",
		"dws api",
		"--dry-run",
		"dws devdoc article search",
		"不得猜",
		"确认",
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		content := string(data)
		for _, marker := range markers {
			if !strings.Contains(content, marker) {
				t.Errorf("%s missing contract marker %q", path, marker)
			}
		}
	}
}
