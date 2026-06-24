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
	"testing"
)

// TestMaterializeEmbeddedSkillSourceMono verifies that the mono skill bundle
// baked into the binary can be extracted to a temp dir and is a valid skill
// source root (so `dws skill setup` works with zero local checkout). The
// nested-reference and _common checks guard against the embed dropping nested
// docs or the `all:` prefix being lost (which would silently skip
// dot/underscore dirs).
func TestMaterializeEmbeddedSkillSourceMono(t *testing.T) {
	dir, cleanup, err := materializeEmbeddedSkillSource(skillSetupModeMono)
	if err != nil {
		t.Fatalf("materializeEmbeddedSkillSource: %v", err)
	}
	defer cleanup()

	if !isSkillSourceRoot(dir, skillSetupModeMono) {
		t.Fatalf("extracted dir %s is not a valid mono skill source root", dir)
	}
	for _, rel := range []string{
		"SKILL.md",
		filepath.Join("references", "global-reference.md"),
		filepath.Join("references", "best_practices", "_common"),
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("expected embedded skill to contain %s: %v", rel, err)
		}
	}

	// cleanup must actually remove the temp dir.
	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("cleanup did not remove temp dir %s (err=%v)", dir, err)
	}
}

// TestResolveSkillSetupSourceOrEmbeddedFallsBackToEmbedded verifies that with
// no --source and no DWS_SKILL_SOURCE, resolution uses the embedded bundle
// rather than probing the current working directory (the stale-skill footgun).
func TestResolveSkillSetupSourceOrEmbeddedFallsBackToEmbedded(t *testing.T) {
	t.Setenv("DWS_SKILL_SOURCE", "")
	dir, cleanup, err := resolveSkillSetupSourceOrEmbedded("", skillSetupModeMono)
	if err != nil {
		t.Fatalf("resolveSkillSetupSourceOrEmbedded: %v", err)
	}
	defer cleanup()
	if !isSkillSourceRoot(dir, skillSetupModeMono) {
		t.Fatalf("embedded fallback returned non-source-root dir %s", dir)
	}
}
