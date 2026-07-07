package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
)

func TestMain(m *testing.M) {
	// Set an empty catalog fixture so that EnvironmentLoader does not
	// attempt live discovery (which would hang on unreachable MCP endpoints).
	absFixture, _ := filepath.Abs("testdata/empty_catalog.json")
	os.Setenv(cli.CatalogFixtureEnv, absFixture)

	code := m.Run()
	os.Exit(code)
}
