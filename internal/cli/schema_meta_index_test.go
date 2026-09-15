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
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

const schemaLazyMetaIndexChildEnv = "DWS_SCHEMA_LAZY_META_INDEX_CHILD"

func TestAssembledSchemaMetaIndexMatchesCatalog(t *testing.T) {
	loaded := mustDeliverySchemaCatalogMaps(t)
	index, err := BuildSchemaMetaIndex(loaded.Snapshot)
	if err != nil {
		t.Fatalf("BuildSchemaMetaIndex() error = %v", err)
	}
	if err := ValidateSchemaMetaIndexAgainstCatalog(index, loaded.Registry); err != nil {
		t.Fatalf("ValidateSchemaMetaIndexAgainstCatalog() error = %v", err)
	}
	if index.SourceHash != loaded.Snapshot.SourceHash {
		t.Fatalf("source_hash index=%q catalog=%q", index.SourceHash, loaded.Snapshot.SourceHash)
	}
	if got := len(index.Entries); got != len(loaded.Index.CanonicalPaths()) {
		t.Fatalf("meta index entries = %d, catalog tools = %d", got, len(loaded.Index.CanonicalPaths()))
	}
}

func TestResolveMetaLazilyAssemblesRegisteredSourceRoot(t *testing.T) {
	if os.Getenv(schemaLazyMetaIndexChildEnv) == "1" {
		// Child process starts fresh; TestMain installs assembled delivery.
		// MCP/parameter embeds may load from unrelated package init — only
		// Catalog/MetaIndex must stay at zero until ResolveMeta.
		counts := RuntimeSchemaMetadataLoadCounts()
		if counts.Catalog != 0 || counts.MetaIndex != 0 {
			t.Fatalf("Catalog/MetaIndex loaded during package init: %#v", counts)
		}
		if _, ok := ResolveMeta("dev app delete"); !ok {
			t.Fatal(`ResolveMeta("dev app delete") ok=false`)
		}
		counts = RuntimeSchemaMetadataLoadCounts()
		if counts.MetaIndex != 1 {
			t.Fatalf("MetaIndex load count = %d, want 1", counts.MetaIndex)
		}
		if counts.Catalog != 1 {
			t.Fatalf("Catalog load count = %d, want 1 after ResolveMeta (single-track assembly)", counts.Catalog)
		}
		for range 8 {
			if _, ok := ResolveMeta("dev app delete"); !ok {
				t.Fatal("steady ResolveMeta ok=false")
			}
		}
		counts = RuntimeSchemaMetadataLoadCounts()
		if counts.Catalog != 1 || counts.MetaIndex != 1 {
			t.Fatalf("steady ResolveMeta re-assembled: %#v", counts)
		}
		return
	}

	command := exec.Command(os.Args[0], "-test.run=^TestResolveMetaLazilyAssemblesRegisteredSourceRoot$", "-test.count=1")
	command.Env = append(os.Environ(), schemaLazyMetaIndexChildEnv+"=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("meta index lazy child failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func TestResolveMetaFailsClosedWithoutSourceRoot(t *testing.T) {
	resetMetaByCLIPathStateForTest()
	storeSchemaSourceRootFn(nil)
	assembleDeliverySchemaCatalogFn = assembleSchemaCatalogFromRoot
	t.Cleanup(restorePackageCLISchemaDeliveryForTest)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("ResolveMeta must panic without registered source root")
		}
		msg, _ := r.(string)
		if !strings.Contains(msg, "schema source root factory is not registered") &&
			!strings.Contains(msg, "schema CommandMeta index is unusable") {
			t.Fatalf("panic = %#v, want fail-closed missing factory", r)
		}
	}()
	ResolveMeta("dev app delete")
}

func TestResolveMetaLoadsOnlyOnce(t *testing.T) {
	const childEnv = "DWS_SCHEMA_LAZY_META_INDEX_ONCE_CHILD"
	if os.Getenv(childEnv) == "1" {
		var wait sync.WaitGroup
		for range 32 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				_, _ = ResolveMeta("dev app delete")
			}()
		}
		wait.Wait()
		if got := RuntimeSchemaMetadataLoadCounts().MetaIndex; got != 1 {
			t.Fatalf("MetaIndex lazy load count = %d, want 1", got)
		}
		return
	}

	command := exec.Command(os.Args[0], "-test.run=^TestResolveMetaLoadsOnlyOnce$", "-test.count=1")
	command.Env = append(os.Environ(), childEnv+"=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("meta index once child failed: %v\n%s", err, strings.TrimSpace(string(output)))
	}
}

func TestBuildSchemaMetaIndexDeterministic(t *testing.T) {
	loaded := mustDeliverySchemaCatalogMaps(t)
	first, err := BuildSchemaMetaIndex(loaded.Snapshot)
	if err != nil {
		t.Fatalf("BuildSchemaMetaIndex() error = %v", err)
	}
	second, err := BuildSchemaMetaIndex(loaded.Snapshot)
	if err != nil {
		t.Fatalf("second BuildSchemaMetaIndex() error = %v", err)
	}
	left, err := EncodeSchemaMetaIndex(first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := EncodeSchemaMetaIndex(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(left) != string(right) {
		t.Fatal("BuildSchemaMetaIndex is not deterministic")
	}
	if err := ValidateSchemaMetaIndexAgainstSnapshot(first, loaded.Snapshot); err != nil {
		t.Fatalf("ValidateSchemaMetaIndexAgainstSnapshot() error = %v", err)
	}
}
