package cli_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
)

func TestMain(m *testing.M) {
	// Set an empty catalog fixture so that EnvironmentLoader does not
	// attempt live discovery (which would hang on unreachable MCP endpoints).
	absFixture, _ := filepath.Abs("testdata/empty_catalog.json")
	os.Setenv(cli.CatalogFixtureEnv, absFixture)

	// Serve the local servers.json fixture at /cli/discovery/apis/bamboo so that
	// the dynamic command generator can build CLI commands without network
	// access. FetchServers calls {baseURL}/cli/discovery/apis/bamboo.
	mux := http.NewServeMux()
	mux.HandleFunc("/cli/discovery/apis/bamboo", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "testdata/servers.json")
	})
	srv := httptest.NewServer(mux)
	app.SetDiscoveryBaseURL(srv.URL)

	code := m.Run()
	srv.Close()
	os.Exit(code)
}
