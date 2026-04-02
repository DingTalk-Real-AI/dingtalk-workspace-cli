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

func TestOpenSourceTreeOmitsEmbeddedHostMarkers(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}

	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
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
		"WriteToken" + "Marker",
		"Token" + "Marker",
		"tokenJSON" + "File",
		"REWIND_" + "REQUEST_ID",
		"REWIND_" + "SESSION_ID",
		"REWIND_" + "MESSAGE_ID",
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
			case ".git", ".worktrees", "node_modules", "dist", "plans":
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
