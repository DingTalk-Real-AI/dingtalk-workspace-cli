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

package dws

import (
	"errors"
	"io/fs"
	"testing"
)

func TestEmbeddedSkillsContainsOnlyInstallableTrees(t *testing.T) {
	for _, path := range []string{
		"skills/mono/SKILL.md",
		"skills/multi/dws-shared/SKILL.md",
	} {
		if _, err := fs.Stat(EmbeddedSkills, path); err != nil {
			t.Fatalf("expected installable skill path %q in embedded FS: %v", path, err)
		}
	}

	if _, err := fs.Stat(EmbeddedSkills, "internal/cli/schema_hints"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("build-only internal/cli/schema_hints must not be present in embedded FS: %v", err)
	}
}
