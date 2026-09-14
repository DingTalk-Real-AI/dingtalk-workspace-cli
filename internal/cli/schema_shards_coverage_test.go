// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestMergeSchemaCatalogDumpMergesShards(t *testing.T) {
	dir := t.TempDir()
	envelope := []byte(`{"version":1,"surface_hash":"sha256:s","source_hash":"sha256:c","catalog":{"kind":"schema"}}`)
	writeFile := func(name, data string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	writeFile("doc.json", `{"product":"doc","tools":{"doc.copy":{"title":"复制"}}}`)
	writeFile("sheet.json", `{"product":"sheet","tools":{"sheet.get":{"title":"读取"}}}`)
	writeFile("README.md", "not a shard")

	snapshot, err := mergeSchemaCatalogDump(envelope, dir)
	if err != nil {
		t.Fatalf("mergeSchemaCatalogDump() error = %v", err)
	}
	if snapshot.Version != 1 || snapshot.SurfaceHash != "sha256:s" || snapshot.SourceHash != "sha256:c" {
		t.Fatalf("envelope fields lost: %+v", snapshot)
	}
	if len(snapshot.Tools) != 2 || snapshot.Tools["doc.copy"] == nil || snapshot.Tools["sheet.get"] == nil {
		t.Fatalf("tools = %#v, want merged doc.copy + sheet.get", snapshot.Tools)
	}
}

func TestMergeSchemaCatalogDumpFailureModes(t *testing.T) {
	valid := []byte(`{"version":1,"source_hash":"sha256:c","catalog":{}}`)
	if _, err := mergeSchemaCatalogDump([]byte("{bad"), t.TempDir()); err == nil || !strings.Contains(err.Error(), "decode schema catalog.json") {
		t.Fatalf("bad envelope err = %v", err)
	}
	if _, err := mergeSchemaCatalogDump(valid, filepath.Join(t.TempDir(), "missing-dir")); err == nil || !strings.Contains(err.Error(), "tools directory") {
		t.Fatalf("missing dir err = %v", err)
	}
	badShard := t.TempDir()
	if err := os.WriteFile(filepath.Join(badShard, "doc.json"), []byte("{bad"), 0o600); err != nil {
		t.Fatalf("write shard: %v", err)
	}
	if _, err := mergeSchemaCatalogDump(valid, badShard); err == nil || !strings.Contains(err.Error(), "decode schema catalog shard") {
		t.Fatalf("bad shard err = %v", err)
	}
}

func TestBuildMetaByCLIPathGuards(t *testing.T) {
	// nil Tools：损坏快照直接返回空表。
	if got := buildMetaByCLIPath(loadedSchemaCatalog{}); len(got) != 0 {
		t.Fatalf("nil tools lookup = %#v, want empty", got)
	}
	loaded := loadedSchemaCatalog{Snapshot: SchemaCatalogSnapshot{Tools: map[string]map[string]any{
		"doc.no_cli_path": {"canonical_path": "doc.no_cli_path"},
		"doc.primary": {
			"cli_path":       "doc primary",
			"canonical_path": "doc.primary",
			"aliases":        []any{"doc alias", "doc primary", "  ", float64(1)},
		},
		"doc.other": {
			"cli_path":       "doc alias",
			"canonical_path": "doc.other",
		},
	}}}
	lookup := buildMetaByCLIPath(loaded)
	if _, ok := lookup[""]; ok {
		t.Fatal("entry without cli_path must be skipped")
	}
	// 别名与其他命令的主路径冲突时，主路径必须胜出。
	if got := lookup["doc alias"].Identity.Canonical; got != "doc.other" {
		t.Fatalf("doc alias resolved to %q, want primary owner doc.other", got)
	}
	if got := lookup["doc primary"].Identity.Canonical; got != "doc.primary" {
		t.Fatalf("doc primary resolved to %q", got)
	}
	// Identity.Aliases 保留 Catalog 中的字符串成员（非字符串被丢弃）；
	// 空白/自指别名仅在查找表注册时跳过。
	if aliases := lookup["doc primary"].Identity.Aliases; len(aliases) != 3 {
		t.Fatalf("aliases = %v, want the 3 string members (float dropped)", aliases)
	}
}

func TestSchemaStringSliceNonSlice(t *testing.T) {
	if got := schemaStringSlice("not-a-slice"); got != nil {
		t.Fatalf("schemaStringSlice() = %v, want nil", got)
	}
}

func TestRenderSafetyAnnotation(t *testing.T) {
	// dev app delete: destructive/high/user_required —— 必须渲染完整注解。
	root := &cobra.Command{Use: "dws"}
	dev := &cobra.Command{Use: "dev"}
	app := &cobra.Command{Use: "app"}
	deleteCmd := &cobra.Command{Use: "delete"}
	root.AddCommand(dev)
	dev.AddCommand(app)
	app.AddCommand(deleteCmd)
	var out bytes.Buffer
	deleteCmd.SetOut(&out)
	RenderSafetyAnnotation(deleteCmd)
	rendered := out.String()
	if !strings.Contains(rendered, "Safety: effect=destructive") || !strings.Contains(rendered, "Do not use --yes until the user explicitly confirms") {
		t.Fatalf("rendered = %q, want destructive annotation with confirmation hint", rendered)
	}
	if !strings.Contains(rendered, "idempotency=") {
		t.Fatalf("rendered = %q, want idempotency field", rendered)
	}

	// 未收录命令不输出任何内容。
	unknown := &cobra.Command{Use: "unknown-cmd"}
	root.AddCommand(unknown)
	var silent bytes.Buffer
	unknown.SetOut(&silent)
	RenderSafetyAnnotation(unknown)
	if silent.Len() != 0 {
		t.Fatalf("unknown command rendered %q, want empty", silent.String())
	}
}

// TestBuildMetaByCLIPathAliasCollisionDeterministic（CR C3）：alias-vs-alias
// 冲突必须跨进程稳定——归属字典序最小的主 cli_path，而非 map 遍历顺序。
func TestBuildMetaByCLIPathAliasCollisionDeterministic(t *testing.T) {
	loaded := loadedSchemaCatalog{Snapshot: SchemaCatalogSnapshot{Tools: map[string]map[string]any{
		"doc.zeta": {
			"cli_path":       "doc zeta",
			"canonical_path": "doc.zeta",
			"aliases":        []any{"doc shared"},
		},
		"doc.alpha": {
			"cli_path":       "doc alpha",
			"canonical_path": "doc.alpha",
			"aliases":        []any{"doc shared"},
		},
	}}}
	// 多次构建，归属必须始终是字典序更小的 "doc alpha"。
	for i := 0; i < 20; i++ {
		lookup := buildMetaByCLIPath(loaded)
		if got := lookup["doc shared"].Identity.Canonical; got != "doc.alpha" {
			t.Fatalf("run %d: doc shared owned by %q, want deterministic doc.alpha", i, got)
		}
	}
}
