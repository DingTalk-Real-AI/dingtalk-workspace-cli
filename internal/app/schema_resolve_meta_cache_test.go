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

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

const schemaResolveMetaCacheChildEnv = "DWS_SCHEMA_RESOLVE_META_CACHE_CHILD"

// TestResolveMetaAndLeafHelpReuseAssembledMetaCache proves production
// RegisterSchemaSourceRoot → delivery Once caches CommandMeta: first
// ResolveMeta pays assembly once; subsequent ResolveMeta and leaf --help
// Safety do not increment the Catalog counter.
func TestResolveMetaAndLeafHelpReuseAssembledMetaCache(t *testing.T) {
	if os.Getenv(schemaResolveMetaCacheChildEnv) == "1" {
		registerSchemaRuntimeDelivery()
		// A caller may select a profile after parsing process argv (for example
		// while executing against several profiles). Cold metadata assembly must
		// preserve that selection just as a warm lookup does.
		testseam.Swap(t, &os.Args, []string{"dws", "--profile=argv-profile"})
		previousProfile := authpkg.RuntimeProfile()
		t.Cleanup(func() { authpkg.SetRuntimeProfile(previousProfile) })
		const activeProfile = "active-delivery-profile"
		authpkg.SetRuntimeProfile(activeProfile)

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
		if got := authpkg.RuntimeProfile(); got != activeProfile {
			t.Fatalf("cold ResolveMeta changed profile to %q, want %q", got, activeProfile)
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
		if got := authpkg.RuntimeProfile(); got != activeProfile {
			t.Fatalf("warm ResolveMeta changed profile to %q, want %q", got, activeProfile)
		}

		root := NewRootCommand()
		if got := authpkg.RuntimeProfile(); got != "argv-profile" {
			t.Fatalf("runtime root did not initialize argv profile: %q", got)
		}
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
