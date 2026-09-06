package roothelp

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestCrossPlatformCoverageRootHelpDependencyClosure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "list", "-deps", ".")
	data, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect help renderer dependencies: %v\n%s", err, data)
	}
	const module = "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/"
	for _, path := range strings.Fields(string(data)) {
		if strings.HasPrefix(path, module) && path != module+"internal/roothelp" && path != module+"internal/tui" {
			t.Errorf("help renderer reaches command/configuration code: %s", path)
		}
		if path == "net" || strings.HasPrefix(path, "net/") || strings.HasPrefix(path, "github.com/spf13/") {
			t.Errorf("help renderer reaches a network or command framework dependency: %s", path)
		}
	}
}
