package app

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/roothelp"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageRootHelpDeclarationProjectionMatchesRuntime(t *testing.T) {
	if hooks := edition.Get(); hooks.Name != "open" || hooks.RegisterExtraCommands != nil {
		t.Skip("the root help projection is limited to the overlay-free open edition")
	}
	// Each locale must start without runtime-injected endpoints. Building a
	// runtime root first used to mask missing supplement-backed commands in
	// the declaration projection (contract and whiteboard in the open edition).
	locale := os.Getenv("DWS_HELP_PROJECTION_CHILD")
	if locale == "" {
		for _, lang := range []string{"en", "zh"} {
			t.Run(lang, func(t *testing.T) {
				child := exec.Command(os.Args[0], "-test.run=^TestCrossPlatformCoverageRootHelpDeclarationProjectionMatchesRuntime$")
				child.Env = append(os.Environ(), "DWS_HELP_PROJECTION_CHILD="+lang)
				if out, err := child.CombinedOutput(); err != nil {
					t.Fatalf("fresh-process %s projection: %v\n%s", lang, err, out)
				}
			})
		}
		return
	}
	t.Setenv("DWS_CONFIG_DIR", filepath.Join(t.TempDir(), "config"))
	previous := i18n.Lang()
	t.Cleanup(func() { i18n.SetLang(previous) })
	for _, locale := range []string{locale} {
		t.Run(locale, func(t *testing.T) {
			i18n.SetLang(locale)

			// The declaration constructor deliberately omits runtime injections.
			// Its projection must still describe exactly the plain public root.
			source := NewSchemaSourceRootCommand()
			source.InitDefaultHelpCmd()
			data, err := json.Marshal(RootHelpModel(source))
			if err != nil {
				t.Fatal(err)
			}
			var model roothelp.Model
			if err := json.Unmarshal(data, &model); err != nil {
				t.Fatal(err)
			}
			var projected bytes.Buffer
			roothelp.Render(&projected, model)
			root := NewRootCommand()
			var actual, stderr bytes.Buffer
			root.SetOut(&actual)
			root.SetErr(&stderr)
			root.SetArgs([]string{"--help"})
			if err := root.Execute(); err != nil || stderr.Len() != 0 {
				t.Fatalf("runtime help: %v, stderr %s", err, stderr.String())
			}

			if !bytes.Equal(actual.Bytes(), projected.Bytes()) {
				t.Fatalf("declaration-derived %s help differs from runtime: projected=%s ACTUAL=%s", locale, projected.String(), actual.String())
			}
		})
	}
}
