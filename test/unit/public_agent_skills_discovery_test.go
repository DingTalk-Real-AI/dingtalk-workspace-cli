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
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// vercel-labs/skills walks known containers (including skills/) up to three
// levels, so skills/multi/<name>/SKILL.md is discoverable. A SKILL.md with
// metadata.internal: true is hidden from default `npx skills add`.
const publicSkillsCLIContainerDepth = 3

type publicSkillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Metadata    struct {
		Internal bool `yaml:"internal"`
	} `yaml:"metadata"`
}

func TestPublicAgentSkillsCLIDiscoveryContract(t *testing.T) {
	root := repoRoot(t)
	skillsRoot := filepath.Join(root, "skills")

	type foundSkill struct {
		rel      string
		name     string
		internal bool
		depth    int
		hasName  bool
		hasDesc  bool
	}

	var found []foundSkill
	err := filepath.WalkDir(skillsRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		relSlash := filepath.ToSlash(rel)
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		fm, parseErr := parsePublicSkillFrontmatter(body)
		if parseErr != nil {
			t.Errorf("%s: %v", relSlash, parseErr)
			return nil
		}
		found = append(found, foundSkill{
			rel:      relSlash,
			name:     fm.Name,
			internal: fm.Metadata.Internal,
			depth:    strings.Count(relSlash, "/"),
			hasName:  strings.TrimSpace(fm.Name) != "",
			hasDesc:  strings.TrimSpace(fm.Description) != "",
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk skills/: %v", err)
	}

	var (
		sawMono     bool
		publicNames []string
	)
	for _, skill := range found {
		if !skill.hasName || !skill.hasDesc {
			t.Errorf("%s: npx skills add requires string name and description", skill.rel)
		}
		if skill.depth > publicSkillsCLIContainerDepth {
			t.Errorf("%s: depth %d exceeds vercel-labs/skills default container walk (%d); flatten or move this skill", skill.rel, skill.depth, publicSkillsCLIContainerDepth)
		}
		switch {
		case skill.rel == "skills/mono/SKILL.md":
			sawMono = true
			if skill.name != "dws" {
				t.Errorf("mono skill name = %q, want dws", skill.name)
			}
			if !skill.internal {
				t.Error("skills/mono/SKILL.md must set metadata.internal: true so npx skills add does not also install the mega-skill")
			}
		case strings.HasPrefix(skill.rel, "skills/multi/") && strings.HasSuffix(skill.rel, "/SKILL.md"):
			parts := strings.Split(skill.rel, "/")
			if len(parts) != 4 {
				t.Errorf("%s: want skills/multi/<name>/SKILL.md", skill.rel)
				continue
			}
			dir := parts[2]
			if !strings.HasPrefix(dir, "dingtalk-") {
				t.Errorf("%s: multi skill directory %q must be dingtalk-*", skill.rel, dir)
			}
			if skill.name != dir {
				t.Errorf("%s: frontmatter name %q != directory %q", skill.rel, skill.name, dir)
			}
			if skill.internal {
				t.Errorf("%s: product/shared skill must stay default-installable (metadata.internal must be false/absent)", skill.rel)
			} else {
				publicNames = append(publicNames, skill.name)
			}
		default:
			t.Errorf("%s: unexpected SKILL.md; public discovery only allows skills/multi/<name>/ and internal skills/mono/", skill.rel)
		}
	}

	if !sawMono {
		t.Error("missing skills/mono/SKILL.md")
	}
	if len(publicNames) == 0 {
		t.Error("no default-installable dingtalk-* skills under skills/multi/")
	}
	for _, name := range publicNames {
		if name == "dws" {
			t.Error("mega-skill name dws must not appear as a default-installable public skill")
		}
	}
}

func parsePublicSkillFrontmatter(body []byte) (publicSkillFrontmatter, error) {
	var out publicSkillFrontmatter
	s := string(body)
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return out, fmt.Errorf("missing YAML frontmatter opener")
	}
	rest := s[3:]
	if strings.HasPrefix(rest, "\r\n") {
		rest = rest[2:]
	} else if strings.HasPrefix(rest, "\n") {
		rest = rest[1:]
	}
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return out, fmt.Errorf("missing YAML frontmatter closer")
	}
	if err := yaml.Unmarshal([]byte(rest[:end]), &out); err != nil {
		return out, err
	}
	return out, nil
}
