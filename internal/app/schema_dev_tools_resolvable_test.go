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
	"strings"
	"testing"
)

// TestCrossPlatformCoverageSchemaAllDevToolsResolvable 是 B192 的旁路断言：
// schema --all 装配 dump（BuildSchemaCatalogSnapshot）中 dev 产品工具必须可
// 解析到可执行 Cobra 命令（primary_cli_path / aliases）。只读测试，不改装配。
// 所有 dev 可执行叶子均应进入 Schema surface，不再依赖 dev 专属 exclusion。
func TestCrossPlatformCoverageSchemaAllDevToolsResolvable(t *testing.T) {
	root := NewRootCommand()
	snapshot := fullSchemaSnapshotForTest(t)

	if len(snapshot.Tools) == 0 {
		t.Fatal("schema --all assembly dump contains no tools")
	}

	count := 0
	for canonicalPath, definition := range snapshot.Tools {
		if !strings.HasPrefix(canonicalPath, "dev.") {
			continue
		}
		count++
		cliPath := schemaContractString(definition["primary_cli_path"])
		if cliPath == "" {
			cliPath = schemaContractString(definition["cli_path"])
		}
		command := exactCommandForTest(root, cliPath)
		if command == nil {
			for _, alias := range schemaContractStringSlice(definition["aliases"]) {
				if command = exactCommandForTest(root, alias); command != nil {
					break
				}
			}
		}
		if command == nil {
			t.Errorf("dev tool %q (cli path %q) is not resolvable to an executable command", canonicalPath, cliPath)
			continue
		}
		if !command.Runnable() {
			t.Errorf("dev tool %q resolves to non-runnable command %q", canonicalPath, cliPath)
		}
	}

	if count == 0 {
		t.Fatal("schema --all assembly dump contains no dev.* tools; dev domain resolution is unverified")
	}
}

func TestCrossPlatformCoverageSchemaDoesNotExposeInternalOutputRollout(t *testing.T) {
	snapshot := fullSchemaSnapshotForTest(t)
	count := 0
	for canonicalPath, definition := range snapshot.Tools {
		if !strings.HasPrefix(canonicalPath, "dev.") {
			continue
		}
		count++
		if value, present := definition["output_contract"]; present {
			t.Errorf("dev tool %q leaks internal output rollout into the unversioned Schema wire: %#v", canonicalPath, value)
		}
	}
	if count == 0 {
		t.Fatal("schema contains no dev tools")
	}
}
