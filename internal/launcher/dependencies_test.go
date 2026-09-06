package launcher

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCrossPlatformCoverageLauncherRuntimeDependencies(t *testing.T) {
	// This exact repository package closure is the dependency half of the
	// executable capability allowlist enforced by capabilities.go and PR policy.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "list", "-deps", "-json", "./cmd/dws-launcher")
	command.Dir = filepath.Join("..", "..")
	data, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect launcher dependencies: %v\n%s", err, data)
	}
	const module = "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/"
	allowed := map[string]bool{}
	for _, name := range []string{
		"cmd/dws-launcher", "internal/launcher", "internal/buildversion",
		"internal/roothelp", "internal/localename", "internal/clisignal", "internal/clitelemetry", "internal/profilemetadata",
		"internal/jsonutil", "internal/errors", "internal/tui", "pkg/config", "pkg/validate",
		"internal/schemacache", "internal/schemareader", "internal/schemafastpath",
		"internal/cli/schemacachepb", "internal/cli/schemaruntime", "internal/corecmd/contract", "internal/skillpaths",
	} {
		allowed[module+name] = true
	}
	type dependency struct {
		ImportPath string
		Imports    []string
		Standard   bool
	}
	graph := map[string]dependency{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var item dependency
		if err := decoder.Decode(&item); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		graph[item.ImportPath] = item
	}
	// URL parsing is not a transport. Actual socket/HTTP imports belong only
	// to the existing official SDK; other dependencies cannot add a sender.
	networkImports := map[string]string{
		module + "pkg/config":                                    "net/url",
		module + "internal/errors":                               "net/url",
		module + "pkg/validate":                                  "net/url",
		"gitlab.alibaba-inc.com/aes/aem-go-sdk/internal/encoder": "net/url",
		"gitlab.alibaba-inc.com/aes/aem-go-sdk/internal/sender":  "net/http",
		"gitlab.alibaba-inc.com/aes/aem-go-sdk/aem":              "net",
	}
	for name, item := range graph {
		if strings.HasPrefix(name, module) && !allowed[name] {
			t.Errorf("launcher acquired an unreviewed runtime dependency: %s", name)
		}
		if name == "github.com/spf13/cobra" || name == "github.com/spf13/pflag" {
			t.Errorf("launcher imports the command parser: %s", name)
		}
		for _, imported := range item.Imports {
			if !item.Standard && (imported == "net" || strings.HasPrefix(imported, "net/")) && networkImports[name] != imported {
				t.Errorf("unreviewed launcher network dependency: %s imports %s", name, imported)
			}
			if (name == module+"internal/profilemetadata" || name == module+"internal/clisignal") && !graph[imported].Standard {
				t.Errorf("pure shared package %s imports non-stdlib dependency %s", name, imported)
			}
		}
	}
}
