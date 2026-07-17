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
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	dwsroot "github.com/DingTalk-Real-AI/dingtalk-workspace-cli"
)

var (
	embeddedSkillStat = func(name string) (fs.FileInfo, error) {
		return fs.Stat(dwsroot.EmbeddedSkills, name)
	}
	embeddedSkillMkdirTemp = os.MkdirTemp
	embeddedSkillRemoveAll = os.RemoveAll
	embeddedSkillWalkDir   = func(root string, fn fs.WalkDirFunc) error {
		return fs.WalkDir(dwsroot.EmbeddedSkills, root, fn)
	}
	embeddedSkillReadFile  = dwsroot.EmbeddedSkills.ReadFile
	embeddedSkillMkdirAll  = os.MkdirAll
	embeddedSkillWriteFile = os.WriteFile
)

// resolveSkillSetupSourceOrEmbedded resolves the skill source for `skill
// setup`. An explicit --source or DWS_SKILL_SOURCE is honored as a developer
// override (validated as an on-disk dir). Otherwise it falls back to the skill
// bundle embedded in THIS binary, so a plain `dws skill setup` always installs
// the version shipped with the running binary — upgrading the binary therefore
// refreshes the installed skill, instead of silently reusing a stale copy from
// the current working directory.
//
// The returned cleanup func removes any temp dir created for the embedded
// bundle; it is a no-op when an on-disk source is used. Always call it.
func resolveSkillSetupSourceOrEmbedded(explicit, mode string) (string, func(), error) {
	noop := func() {}
	explicit = strings.TrimSpace(explicit)
	env := strings.TrimSpace(os.Getenv("DWS_SKILL_SOURCE"))
	if explicit != "" || env != "" {
		dir, err := resolveSkillSetupSource(explicit, mode)
		return dir, noop, err
	}
	return materializeEmbeddedSkillSource(mode)
}

// materializeEmbeddedSkillSource extracts the embedded skills/<mode> subtree
// into a fresh temp dir and returns its path plus a cleanup func. Reusing a
// real directory lets the existing dir-based install/copy logic stay unchanged.
func materializeEmbeddedSkillSource(mode string) (string, func(), error) {
	noop := func() {}
	sub := "skills/" + mode // embed.FS always uses forward slashes
	if _, err := embeddedSkillStat(sub); err != nil {
		return "", noop, fmt.Errorf("内嵌 skill 不含 %q（二进制可能未随 skills/ 重新构建）: %w", sub, err)
	}

	tmp, err := embeddedSkillMkdirTemp("", "dws-skill-"+mode+"-")
	if err != nil {
		return "", noop, fmt.Errorf("创建临时 skill 目录失败: %w", err)
	}
	cleanup := func() { _ = embeddedSkillRemoveAll(tmp) }

	walkErr := embeddedSkillWalkDir(sub, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, sub), "/")
		dst := filepath.Join(tmp, filepath.FromSlash(rel))
		if d.IsDir() {
			return embeddedSkillMkdirAll(dst, 0o755)
		}
		data, readErr := embeddedSkillReadFile(p)
		if readErr != nil {
			return readErr
		}
		if mkErr := embeddedSkillMkdirAll(filepath.Dir(dst), 0o755); mkErr != nil {
			return mkErr
		}
		return embeddedSkillWriteFile(dst, data, 0o644)
	})
	if walkErr != nil {
		cleanup()
		return "", noop, fmt.Errorf("展开内嵌 skill 到临时目录失败: %w", walkErr)
	}
	return tmp, cleanup, nil
}
