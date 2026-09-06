package app

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/roothelp"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func TestCrossPlatformCoverageRootHelpDeclarationProjectionMatchesRuntime(t *testing.T) {
	if hooks := edition.Get(); hooks.Name != "open" || hooks.RegisterExtraCommands != nil {
		t.Skip("the launcher help projection is limited to the overlay-free open edition")
	}
	t.Setenv("DWS_CONFIG_DIR", filepath.Join(t.TempDir(), "config"))
	previous := i18n.Lang()
	t.Cleanup(func() { i18n.SetLang(previous) })
	for _, locale := range []string{"en", "zh"} {
		t.Run(locale, func(t *testing.T) {
			i18n.SetLang(locale)
			root := NewRootCommand()
			var actual, stderr bytes.Buffer
			root.SetOut(&actual)
			root.SetErr(&stderr)
			root.SetArgs([]string{"--help"})
			if err := root.Execute(); err != nil || stderr.Len() != 0 {
				t.Fatalf("runtime help: %v, stderr %s", err, stderr.String())
			}

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
			if !bytes.Equal(actual.Bytes(), projected.Bytes()) {
				t.Fatalf("declaration-derived %s help differs from runtime after the DTO round trip", locale)
			}
		})
	}
}
