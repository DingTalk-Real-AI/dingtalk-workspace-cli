package helpers

import (
	"os"
	"strings"
	"testing"
)

func TestDeapAgentMCPRequiresEmployeeForEveryResourceCommand(t *testing.T) {
	for _, operation := range []string{"create", "list", "query"} {
		t.Run(operation, func(t *testing.T) {
			caller, _ := newDeapAgentTestTree(t, false)
			root := deapHandler{}.Command(&captureRunner{})
			args := []string{"capability", "mcp", operation}
			if operation == "create" {
				args = append(args, "--config-file", "./not-read.json")
			}
			if operation == "query" {
				args = append(args, "--mcp-id", "mcp-1")
			}
			root.SetArgs(args)
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), "agent-uuid") {
				t.Fatalf("missing employee must fail before execution: %v", err)
			}
			if len(caller.calls) != 0 {
				t.Fatal("missing employee made a remote call")
			}
		})
	}
}

func TestDeapAgentMCPInvalidConfigNeverCallsRemoteOrLeaksValues(t *testing.T) {
	for name, body := range map[string]string{
		"wrapped HSF shape":    `{"config":{"name":"example","configString":"secret-marker"}}`,
		"missing configString": `{"name":"example"}`,
		"empty configString":   `{"name":"example","configString":"  "}`,
		"object not string":    `{"name":"example","configString":{"token":"secret-marker"}}`,
		"missing name":         `{"configString":"secret-marker"}`,
		"employee override":    `{"name":"example","configString":"secret-marker","agentUuid":"other-agent"}`,
		"identity override":    `{"name":"example","configString":"secret-marker","identity":{"userId":"other-user"}}`,
		"unknown secret key":   `{"name":"example","configString":"secret-marker","secret-marker":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			caller, out := newDeapAgentTestTree(t, false)
			t.Chdir(t.TempDir())
			if err := os.WriteFile("mcp.json", []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			err := deapAgentCallMCPCreateFromFile(nil, deapAgentMCPCreateTool,
				map[string]any{"agentUuid": "agent-1", "configFile": "./mcp.json"})
			if err == nil {
				t.Fatal("invalid configuration accepted")
			}
			if len(caller.calls) != 0 {
				t.Fatal("invalid configuration reached the remote")
			}
			if strings.Contains(err.Error()+out.String(), "secret-marker") {
				t.Fatal("invalid configuration leaked a sensitive value")
			}
		})
	}
}
