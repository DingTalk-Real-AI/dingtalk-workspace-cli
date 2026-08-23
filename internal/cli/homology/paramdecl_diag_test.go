package homology

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
)

func TestParamDeclAnnotationCompare(t *testing.T) {
	root := app.NewSchemaSourceRootCommand()
	for _, path := range [][]string{
		{"aisearch", "enterprise"},
		{"hrbrain", "search", "employees"},
		{"chat", "add-emoji"},
	} {
		cmd, _, err := root.Find(path)
		if err != nil || cmd == nil {
			t.Logf("%v: NOT FOUND", path)
			continue
		}
		ann := cmd.Annotations["dws.schema.param_decls"]
		if ann == "" {
			t.Logf("%v: NO param_decls annotation", path)
		} else {
			t.Logf("%v: HAS param_decls (%d bytes)", path, len(ann))
		}
	}
}
