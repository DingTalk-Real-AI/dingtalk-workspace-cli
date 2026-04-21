package unit_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestOpenSourceTreeOmitsEmbeddedHostMarkers scans source files for proprietary
// markers that must not leak into the public (open-source) tree.
//
// The scanner intentionally skips the docs/ directory in addition to the other
// excluded folders. The docs/pat/ subtree records the public wire contract
// (docs/pat/contract.md) and docs/_research/ holds design evidence; both
// legitimately enumerate reserved environment-variable names, historical
// symbols, and host-integration mechanics as part of the externally visible
// compatibility surface. The policy's job is to catch internal coupling in
// source code (.go/.sh/.yml/.yaml/.ps1/.tmpl/Makefile). Any reference to
// these markers inside other directories' markdown is still scanned.
func TestOpenSourceTreeOmitsEmbeddedHostMarkers(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}

	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	// NOTE: REWIND_SESSION_ID / REWIND_REQUEST_ID / REWIND_MESSAGE_ID are
	// intentionally NOT on this list. They are accepted by the CLI as
	// optional backward-compatibility aliases for the primary DWS_* trace
	// env names (see docs/pat/contract.md §3.1 and §9). Because they are a
	// documented compatibility surface rather than an internal coupling to
	// a specific host implementation, referring to these literals from
	// source code and docs is allowed. Product names like "RewindDesktop"
	// and other host-implementation specific symbols remain forbidden.
	forbidden := []string{
		"DWS_" + "BUILD_MODE",
		"com.dingtalk.scenario." + "wukong",
		"WUKONG_" + "SKILLS_DIR",
		"Embedded" + "Mode",
		"CleanTokenOn" + "Expiry",
		"HideAuth" + "LoginCommand",
		"EnablePrivate" + "UtilityCommands",
		"UseExecutable" + "ConfigDir",
		"DeleteExeRelative" + "TokenOnAuthErr",
		"MergeWukong" + "MCPHeaders",
		"buildMode ==" + " \"real\"",
	}

	var matches []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".worktrees", "node_modules", "dist", "plans", "docs":
				return filepath.SkipDir
			}
			return nil
		}
		if !isScannableSourceFile(path) {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		for _, needle := range forbidden {
			if strings.Contains(string(content), needle) {
				rel, _ := filepath.Rel(root, path)
				matches = append(matches, rel+": "+needle)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir() error = %v", err)
	}

	if len(matches) > 0 {
		t.Fatalf("found forbidden proprietary markers in OSS tree:\n%s", strings.Join(matches, "\n"))
	}
}

func isScannableSourceFile(path string) bool {
	switch filepath.Ext(path) {
	case ".go", ".md", ".sh", ".ps1", ".yml", ".yaml", ".tmpl":
		return true
	default:
		return filepath.Base(path) == "Makefile"
	}
}
