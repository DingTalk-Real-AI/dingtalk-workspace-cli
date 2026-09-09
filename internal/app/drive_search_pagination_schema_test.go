package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageDriveSearchPaginationFinalSchema(t *testing.T) {
	root := NewRootCommand()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"schema", "--cli-path", "drive search", "--format", "json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"仅支持首页聚合", "--target file/space", "各自返回的游标"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("final Schema missing %q", want)
		}
	}
}
