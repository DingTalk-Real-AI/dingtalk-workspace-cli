// cmd_root_help_snapshot projects the reviewed root for development-candidate
// sealing. The build driver must compare its references with the finalized core
// before injecting Snapshot into a launcher. This does not authorize releases.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/i18n"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/roothelp"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

func main() {
	var commit, digest string
	flag.StringVar(&commit, "commit", "", "full source commit")
	flag.StringVar(&digest, "core-sha256", "", "finalized native core digest")
	flag.Parse()
	if runtime.Version() != "go1.25.9" || os.Getenv("NO_COLOR") == "" {
		fail(fmt.Errorf("requires pinned Go 1.25.9 and explicit NO_COLOR"))
	}
	hooks := edition.Get()
	if hooks == nil || hooks.Name != "open" || hooks.RegisterExtraCommands != nil {
		fail(fmt.Errorf("requires overlay-free open edition"))
	}
	snapshot := roothelp.Snapshot{Version: 1, Edition: "open", Commit: commit, CoreSHA256: digest}
	references := map[string][]byte{}
	for _, locale := range []string{"en", "zh"} {
		i18n.SetLang(locale)
		root := app.NewSchemaSourceRootCommand()
		root.InitDefaultHelpCmd()
		model := app.RootHelpModel(root)
		if locale == "en" {
			snapshot.English = model
		} else {
			snapshot.Chinese = model
		}
		var out bytes.Buffer
		roothelp.Render(&out, model)
		references[locale] = out.Bytes()
	}
	encoded, err := roothelp.EncodeSnapshot(snapshot)
	if err != nil {
		fail(err)
	}
	for locale, reference := range references {
		model, err := roothelp.DecodeSnapshot(encoded, digest, commit, "open", locale)
		if err != nil {
			fail(err)
		}
		var out bytes.Buffer
		roothelp.Render(&out, model)
		if !bytes.Equal(out.Bytes(), reference) {
			fail(fmt.Errorf("%s help snapshot round trip changed output", locale))
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(struct {
		Snapshot   string
		References map[string][]byte
	}{encoded, references}); err != nil {
		fail(err)
	}
}
func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
