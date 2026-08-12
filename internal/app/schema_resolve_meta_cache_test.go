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

package app

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
)

const schemaResolveMetaCacheChildEnv = "DWS_SCHEMA_RESOLVE_META_CACHE_CHILD"

// TestResolveMetaAndLeafHelpReuseAssembledMetaCache proves production
// RegisterSchemaSourceRoot → delivery Once caches CommandMeta: first
// ResolveMeta pays assembly once; subsequent ResolveMeta and leaf --help
// Safety do not increment the Catalog counter.
func TestResolveMetaAndLeafHelpReuseAssembledMetaCache(t *testing.T) {
	if os.Getenv(schemaResolveMetaCacheChildEnv) == "1" {
		registerSchemaRuntimeDelivery()

		counts := cli.RuntimeSchemaMetadataLoadCounts()
		if counts.Catalog != 0 || counts.MetaIndex != 0 {
			t.Fatalf("precondition Catalog/MetaIndex = %#v", counts)
		}

		meta, ok := cli.ResolveMeta("dev app delete")
		if !ok || meta.Identity.Canonical == "" {
			t.Fatalf("ResolveMeta(dev app delete) = %#v ok=%v", meta, ok)
		}
		counts = cli.RuntimeSchemaMetadataLoadCounts()
		if counts.Catalog != 1 || counts.MetaIndex != 1 {
			t.Fatalf("after first ResolveMeta counts = %#v, want Catalog=1 MetaIndex=1", counts)
		}

		for range 4 {
			if _, ok := cli.ResolveMeta("dev app delete"); !ok {
				t.Fatal("steady ResolveMeta ok=false")
			}
		}
		counts = cli.RuntimeSchemaMetadataLoadCounts()
		if counts.Catalog != 1 || counts.MetaIndex != 1 {
			t.Fatalf("after steady ResolveMeta counts = %#v", counts)
		}

		root := NewRootCommand()
		var helpOut bytes.Buffer
		root.SetOut(&helpOut)
		root.SetErr(io.Discard)
		root.SetArgs([]string{"dev", "app", "delete", "--help"})
		if err := root.Execute(); err != nil {
			t.Fatalf("dev app delete --help: %v", err)
		}
		if meta.Safety.ShouldRender() && !strings.Contains(helpOut.String(), "Safety:") {
			t.Fatalf("leaf --help missing Safety annotation; output=%q", helpOut.String())
		}
		counts = cli.RuntimeSchemaMetadataLoadCounts()
		if counts.Catalog != 1 || counts.MetaIndex != 1 {
			t.Fatalf("leaf --help re-assembled Schema: %#v", counts)
		}
		return
	}

	command := exec.Command(os.Args[0], "-test.run=^TestResolveMetaAndLeafHelpReuseAssembledMetaCache$", "-test.count=1")
	command.Env = append(os.Environ(), schemaResolveMetaCacheChildEnv+"=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("ResolveMeta cache child failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}
