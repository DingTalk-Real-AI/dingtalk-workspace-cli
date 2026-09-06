package launcher

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCrossPlatformCoverageLauncherRuntimeDependencies(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "list", "-deps", "./cmd/dws-launcher")
	command.Dir = filepath.Join("..", "..")
	data, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect launcher dependencies: %v\n%s", err, data)
	}
	const module = "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/"
	allowed := map[string]bool{}
	for _, name := range []string{
		"cmd/dws-launcher", "internal/launcher", "internal/buildversion",
		"internal/clisignal", "internal/clitelemetry", "internal/profilemetadata",
		"internal/jsonutil", "internal/errors", "internal/tui", "pkg/config", "pkg/validate",
		"internal/schemacache", "internal/schemareader", "internal/schemafastpath",
		"internal/cli/schemacachepb", "internal/cli/schemaruntime", "internal/corecmd/contract", "internal/skillpaths",
	} {
		allowed[module+name] = true
	}
	for _, name := range strings.Fields(string(data)) {
		if strings.HasPrefix(name, module) && !allowed[name] {
			t.Errorf("launcher acquired an unreviewed runtime dependency: %s", name)
		}
		if name == "github.com/spf13/cobra" || name == "github.com/spf13/pflag" {
			t.Errorf("launcher imports the command parser: %s", name)
		}
	}
}
