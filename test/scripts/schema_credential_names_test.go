package scripts_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchemaCredentialGuardDistinguishesDeclaredArgvNames(t *testing.T) {
	root := filepath.Join("..", "..")
	script := readTestFile(t, filepath.Join(root, "scripts", "policy", "check-schema-catalog.sh"))
	var pattern string
	for _, line := range strings.Split(script, "\n") {
		if strings.HasPrefix(line, "if policy_search_paths 'mcp-gw") {
			pattern = strings.Split(line, "'")[1]
		}
	}
	if pattern == "" {
		t.Fatal("credential scan missing")
	}
	cases := []struct {
		json    string
		blocked bool
	}{
		{`{"canonical_path":"dev.connect","parameters":{"robot-client-secret":{"description":"机器人凭证输入","type":"string","property":"robotClientSecret","required":false}}}`, false},
		{`{"example":"--robot-client-secret fixture-value"}`, true},
		{`{"robot-client-secret":"fixture-value"}`, true},
		{`{"default_access_token":"fixture-value"}`, true},
		{`{"client_secret":"fixture"}`, true},
		{`{"access-token":"fixture"}`, true},
		{`{"example":"Authorization: fixture"}`, true},
		{`{"example":"Bearer fixture"}`, true},
		{`{"canonical_path":"dev.connect","parameters":{"robot-credential-parameter":"Bearer fixture","robot-client-secret":{"description":"输入","type":"string","property":"robotClientSecret","required":false}}}`, true},
		{`{"example":"https://mcp.dingtalk.com/server"}`, true},
		{`{"example":"https://mcp-gw.dingtalk.com"}`, true},
		{`{"canonical_path":"dev.connect","parameters":{"robot-client-secret":{"description":"输入","type":"string","property":"robotClientSecret","required":false,"default":"fixture-value"}}}`, true},
		{`{"canonical_path":"another.command","parameters":{"robot-client-secret":{"description":"输入","type":"string","property":"robotClientSecret","required":false}}}`, true},
	}
	for _, tc := range cases {
		filter := exec.Command("jq", "-f", filepath.Join(root, "scripts", "policy", "schema-credential-scan.jq"))
		filter.Stdin = strings.NewReader(tc.json)
		data, err := filter.Output()
		if err != nil {
			t.Fatal(err)
		}
		guard := exec.Command("grep", "-E", pattern)
		guard.Stdin = strings.NewReader(string(data))
		err = guard.Run()
		if err != nil {
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
				t.Fatal(err)
			}
		}
		if (err == nil) != tc.blocked {
			t.Fatalf("guard result for %s: %v", tc.json, err)
		}
	}
	for _, field := range []string{`has("client_secret")`, `has("access_token")`} {
		if !strings.Contains(script, field) {
			t.Fatalf("structured credential prohibition removed: %s", field)
		}
	}
}
