package cli_test

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	configDir, err := os.MkdirTemp("", "dws-cli-test-config-")
	if err != nil {
		panic(err)
	}
	os.Setenv("DWS_CONFIG_DIR", configDir)

	// Tests that construct app root commands must remain serial because root
	// construction initializes process-wide helper dependencies.

	code := m.Run()
	_ = os.RemoveAll(configDir)
	os.Exit(code)
}
