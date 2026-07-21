package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
)

func TestMain(m *testing.M) {
	configDir, err := os.MkdirTemp("", "dws-cli-test-config-")
	if err != nil {
		panic(err)
	}
	os.Setenv("DWS_CONFIG_DIR", configDir)

	// Set an empty catalog fixture so that EnvironmentLoader does not
	// attempt live discovery (which would hang on unreachable MCP endpoints).
	// Tests that construct app root commands must remain serial because root
	// construction initializes process-wide helper dependencies.
	absFixture, _ := filepath.Abs("testdata/empty_catalog.json")
	os.Setenv(cli.CatalogFixtureEnv, absFixture)

	code := m.Run()
	_ = os.RemoveAll(configDir)
	os.Exit(code)
}
