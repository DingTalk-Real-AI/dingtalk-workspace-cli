// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// shellPruneScriptTargets are the standalone installers that embed their own
// copy of prune_skill_backups. Each must keep every backup stamp the running
// process created, mirroring Go's run-root registry and install.js's
// currentRunBackupRoots, so a migration retiring more than the keep limit
// stays reversible.
var shellPruneScriptTargets = []string{
	"install.sh",
	"install-skills.sh",
	"install-event.sh",
	"install-devapp.sh",
}

// The ownership-marker contract shared with the Go core
// (internal/upgrade/paths.go): every stamp root DWS creates carries the
// marker, and prune only counts or removes roots with these exact bytes.
const (
	skillBackupMarkerFile = ".dws-skill-backup"
	skillBackupMarkerBody = "dws skill backup v1\n"
)

// extractShellFunction pulls a top-level `name() { ... }` definition out of a
// script so the shipped logic — not a copy — is what the test exercises.
func extractShellFunction(t *testing.T, scriptPath, name string) string {
	t.Helper()
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %s: %v", scriptPath, err)
	}
	text := string(data)
	start := strings.Index(text, "\n"+name+"() {")
	if start < 0 {
		t.Fatalf("function %s not found in %s", name, scriptPath)
	}
	start++
	end := strings.Index(text[start:], "\n}\n")
	if end < 0 {
		t.Fatalf("function %s has no top-level closing brace in %s", name, scriptPath)
	}
	return text[start : start+end+3]
}

// mkdirStampWithMarker creates a DWS-owned stamp fixture: the stamp directory
// plus the exact ownership marker bytes prune verifies. Without the marker a
// stamp-shaped directory is foreign data that prune must neither count nor
// remove.
func mkdirStampWithMarker(t *testing.T, backupRoot, stamp string) {
	t.Helper()
	dir := filepath.Join(backupRoot, stamp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(dir, skillBackupMarkerFile), []byte(skillBackupMarkerBody), 0o644)
}

// runShellPruneScenario builds a sandbox HOME with the given pre-existing
// (earlier-run) stamps, registers the given stamps as created by the current
// run, optionally also plants stamp-shaped directories without the ownership
// marker, runs the script's own prune_skill_backups, and returns the
// surviving stamp directories.
func runShellPruneScenario(t *testing.T, scriptPath string, foreignStamps, runStamps []string, unmarkedStamps ...string) []string {
	t.Helper()
	home := t.TempDir()
	backupRoot := filepath.Join(home, ".dws", "skill-backups")
	for _, stamp := range foreignStamps {
		mkdirStampWithMarker(t, backupRoot, stamp)
	}
	for _, stamp := range unmarkedStamps {
		if err := os.MkdirAll(filepath.Join(backupRoot, stamp, "payload"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	var driver strings.Builder
	driver.WriteString("#!/bin/sh\n")
	driver.WriteString("say() { :; }\n")
	// The keep limit is a script-level variable outside the extracted
	// functions; set both spellings the four installers use.
	driver.WriteString("DWS_SKILL_BACKUP_KEEP=5\n")
	driver.WriteString("SKILL_BACKUP_KEEP=5\n")
	driver.WriteString(extractShellFunction(t, scriptPath, "is_skill_backup_stamp"))
	driver.WriteString("\n")
	driver.WriteString(extractShellFunction(t, scriptPath, "record_current_run_backup_stamp"))
	driver.WriteString("\n")
	driver.WriteString(extractShellFunction(t, scriptPath, "prune_skill_backups"))
	driver.WriteString("\n")
	for _, stamp := range runStamps {
		mkdirStampWithMarker(t, backupRoot, stamp)
		driver.WriteString("record_current_run_backup_stamp \"$HOME/.dws/skill-backups/" + stamp + "\"\n")
	}
	driver.WriteString("prune_skill_backups\n")

	driverPath := filepath.Join(t.TempDir(), "prune-driver.sh")
	if err := os.WriteFile(driverPath, []byte(driver.String()), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", driverPath)
	cmd.Env = append(os.Environ(), "HOME="+home)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("prune driver failed: %v\n%s", err, out)
	}

	entries, err := os.ReadDir(backupRoot)
	if err != nil {
		t.Fatal(err)
	}
	var surviving []string
	for _, entry := range entries {
		if entry.IsDir() {
			surviving = append(surviving, entry.Name())
		}
	}
	return surviving
}

func assertStampsSurvive(t *testing.T, surviving []string, want ...string) {
	t.Helper()
	left := make(map[string]bool)
	for _, stamp := range surviving {
		left[stamp] = true
	}
	for _, stamp := range want {
		if !left[stamp] {
			t.Fatalf("stamp %s must survive pruning, surviving=%v", stamp, surviving)
		}
		delete(left, stamp)
	}
	for stamp := range left {
		t.Fatalf("stamp %s must have been pruned, surviving=%v", stamp, surviving)
	}
}

// TestInstallShellPruneKeepsCurrentRunBackups pins the reversibility contract
// the changelog states for every install surface: pruning may only remove
// batches from earlier runs. The old shell logic deleted the oldest excess
// stamps regardless of origin, so a single migration retiring more than five
// batches silently destroyed its own rollback material.
func TestInstallShellPruneKeepsCurrentRunBackups(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("shell installer tests do not run on %s", runtime.GOOS)
	}
	for _, name := range shellPruneScriptTargets {
		t.Run(name, func(t *testing.T) {
			scriptPath := filepath.Join("..", "..", "scripts", name)
			if _, err := os.Stat(scriptPath); err != nil {
				t.Fatalf("stat %s: %v", scriptPath, err)
			}

			t.Run("run that exceeds the keep limit loses nothing", func(t *testing.T) {
				var runStamps []string
				for i := 1; i <= 8; i++ {
					runStamps = append(runStamps, stampName(i))
				}
				surviving := runShellPruneScenario(t, scriptPath, nil, runStamps)
				assertStampsSurvive(t, surviving, runStamps...)
			})

			t.Run("only earlier-run batches are pruned", func(t *testing.T) {
				var foreignStamps []string
				for i := 1; i <= 6; i++ {
					foreignStamps = append(foreignStamps, oldStampName(i))
				}
				runStamps := []string{stampName(1), stampName(2)}
				surviving := runShellPruneScenario(t, scriptPath, foreignStamps, runStamps)
				// Total 8, keep 5: three oldest foreign batches go, the
				// current run keeps both of its own.
				assertStampsSurvive(t, surviving,
					oldStampName(4), oldStampName(5), oldStampName(6),
					stampName(1), stampName(2))
			})

			t.Run("foreign-only history still prunes to the keep limit", func(t *testing.T) {
				var foreignStamps []string
				for i := 1; i <= 7; i++ {
					foreignStamps = append(foreignStamps, oldStampName(i))
				}
				surviving := runShellPruneScenario(t, scriptPath, foreignStamps, nil)
				assertStampsSurvive(t, surviving,
					oldStampName(3), oldStampName(4), oldStampName(5),
					oldStampName(6), oldStampName(7))
			})

			// A stamp-shaped directory without the ownership marker is foreign
			// data: it must be preserved untouched, never counted toward the
			// keep limit, and never treated as prunable excess. Oldest on
			// purpose — if the marker were not required, this directory would
			// be the first pruning candidate.
			t.Run("unmarked stamp-shaped directory is preserved and uncounted", func(t *testing.T) {
				var foreignStamps []string
				for i := 1; i <= 7; i++ {
					foreignStamps = append(foreignStamps, oldStampName(i))
				}
				unmarked := "20200101-000000"
				surviving := runShellPruneScenario(t, scriptPath, foreignStamps, nil, unmarked)
				assertStampsSurvive(t, surviving,
					append([]string{unmarked},
						oldStampName(3), oldStampName(4), oldStampName(5),
						oldStampName(6), oldStampName(7))...)
			})
		})
	}
}

func stampName(i int) string {
	return "20260817-0000" + twoDigits(i)
}

func oldStampName(i int) string {
	return "20200101-0000" + twoDigits(i)
}

func twoDigits(i int) string {
	if i < 10 {
		return "0" + string(rune('0'+i))
	}
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}

// TestInstallShellBackupNeverAdoptsUnprovenStampRoots pins the ownership
// contract at creation time: a stamp-shaped directory whose marker does not
// verify must never receive the DWS marker or a payload — the backup moves
// to a fresh suffixed root instead, and the foreign data survives untouched.
func TestInstallShellBackupNeverAdoptsUnprovenStampRoots(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell semantics are unavailable")
	}

	for _, scriptName := range []string{"install.sh", "install-skills.sh", "install-event.sh", "install-devapp.sh"} {
		scriptName := scriptName
		t.Run(scriptName, func(t *testing.T) {
			t.Parallel()
			scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", scriptName))
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(scriptPath)
			if err != nil {
				t.Fatal(err)
			}
			cut := strings.LastIndex(string(data), "\nmain\n")
			if cut < 0 {
				t.Fatalf("%s final main invocation not found", scriptName)
			}
			library := filepath.Join(t.TempDir(), scriptName+"-lib.sh")
			mustWriteFile(t, library, data[:cut], 0o755)

			home := t.TempDir()
			// install.sh and install-skills.sh name the payload by basename;
			// the standalone event/devapp publishers mangle the full path.
			payloadName := "victim"
			if scriptName == "install-event.sh" || scriptName == "install-devapp.sh" {
				// Mirror the scripts' own mangling: full path, slashes and
				// backslashes to dashes, leading dash dropped.
				payloadName = strings.NewReplacer("/", "-", "\\", "-").Replace(filepath.Join(home, "victim"))
				payloadName = strings.TrimPrefix(payloadName, "-")
			}
			harness := `. "$DWS_TEST_LIBRARY"
date() { printf '%s\n' 20260101-120000; }
ROOT="$HOME/.dws/skill-backups"
mkdir -p "$ROOT/20260101-120000" || exit 9
printf 'must-survive\n' > "$ROOT/20260101-120000/user-data.txt" || exit 9
mkdir -p "$HOME/victim" || exit 9
printf 'payload\n' > "$HOME/victim/SKILL.md" || exit 9
if ! backup_fn "$HOME/victim" >/dev/null 2>&1; then
  if [ -e "$HOME/victim/SKILL.md" ]; then
    exit 0 # a refused backup that keeps the source is contract-compliant
  fi
  exit 9
fi
[ -f "$ROOT/20260101-120000-1/` + "`printf '%s' \"" + payloadName + "\"`" + `/SKILL.md" ] || exit 3
[ ! -e "$HOME/victim" ] || exit 4
[ -f "$ROOT/20260101-120000/user-data.txt" ] || exit 5
[ "$(cat "$ROOT/20260101-120000/user-data.txt")" = "must-survive" ] || exit 6
[ ! -e "$ROOT/20260101-120000/.dws-skill-backup" ] || exit 7
[ "$(cat "$ROOT/20260101-120000-1/.dws-skill-backup" 2>/dev/null)" = "dws skill backup v1" ] || exit 8
exit 0
`
			harnessPath := filepath.Join(t.TempDir(), "backup-adoption.sh")
			mustWriteFile(t, harnessPath, []byte(harness), 0o755)
			cmd := exec.Command("sh", "-c", "backup_fn() { "+backupFnName(scriptName)+" \"$1\"; }\n. \"$0\"", harnessPath)
			cmd.Env = append(os.Environ(), "HOME="+home, "DWS_TEST_LIBRARY="+library)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s must not adopt an unproven stamp root: %v\n%s", scriptName, err, string(output))
			}
			if backup := findSkillBackupContent(home, "payload\n"); !strings.Contains(backup, "20260101-120000-1") {
				t.Fatalf("%s payload must land in the suffixed root, got %s", scriptName, backup)
			}
		})
	}
}

func backupFnName(scriptName string) string {
	if scriptName == "install-event.sh" || scriptName == "install-devapp.sh" {
		return "backup_skill_dir"
	}
	return "backup_and_remove_skill_dir"
}
