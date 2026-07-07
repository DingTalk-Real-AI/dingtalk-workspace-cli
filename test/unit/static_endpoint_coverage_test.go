package unit_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/syncdata"
)

var explicitServerCallPattern = regexp.MustCompile(`(?:callMCPToolReturnTextOnServer|callMCPToolOnServer|CallMCPToolOnServer)\(\s*"([^"]+)"`)

func TestStaticServersCoverExplicitHelperServerCalls(t *testing.T) {
	root := repoRoot(t)
	helpersDir := filepath.Join(root, "internal", "helpers")
	registeredProducts := syncdata.CmdToProduct()

	servers := map[string]bool{}
	for _, server := range syncdata.StaticServers() {
		if id := strings.TrimSpace(server.ID); id != "" {
			servers[id] = true
		}
		for _, prefix := range server.Prefixes {
			if prefix = strings.TrimSpace(prefix); prefix != "" {
				servers[prefix] = true
			}
		}
	}

	missing := map[string]bool{}
	err := filepath.WalkDir(helpersDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		product := strings.TrimSuffix(filepath.Base(path), ".go")
		if _, ok := registeredProducts[product]; !ok {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range explicitServerCallPattern.FindAllSubmatch(data, -1) {
			serverID := string(match[1])
			if !servers[serverID] {
				rel, _ := filepath.Rel(root, path)
				missing[serverID+" ("+rel+")"] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan helpers: %v", err)
	}

	if len(missing) == 0 {
		return
	}
	items := make([]string, 0, len(missing))
	for item := range missing {
		items = append(items, item)
	}
	sort.Strings(items)
	t.Fatalf("static endpoint registry does not cover explicit helper server calls:\n%s", strings.Join(items, "\n"))
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}
