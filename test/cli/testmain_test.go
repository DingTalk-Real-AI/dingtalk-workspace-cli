package cli_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/surface/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/test/testutil"
)

func TestMain(m *testing.M) {
	// Set an empty catalog fixture so that EnvironmentLoader does not
	// attempt live discovery (which would hang on unreachable MCP endpoints).
	absFixture, _ := filepath.Abs("testdata/empty_catalog.json")
	os.Setenv(cli.CatalogFixtureEnv, absFixture)

	// Serve the local servers.json fixture at /cli/discovery/apis so that
	// the dynamic command generator can build CLI commands without network
	// access. FetchServers calls {baseURL}/cli/discovery/apis.
	mux := http.NewServeMux()
	mux.HandleFunc("/cli/discovery/apis", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "testdata/servers.json")
	})
	srv, err := testutil.NewIPv4Server(mux)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "start ipv4 test server: %v\n", err)
		os.Exit(1)
	}
	app.SetDiscoveryBaseURL(srv.URL)

	code := m.Run()
	srv.Close()
	os.Exit(code)
}
