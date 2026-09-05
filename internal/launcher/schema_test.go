// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package launcher

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli/schemaruntime"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/jsonutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemareader"
)

func TestCrossPlatformCoverageLauncherSchemaRequestRouting(t *testing.T) {
	for _, args := range [][]string{
		{"dws", "schema"}, {"dws", "schema", "list"}, {"dws", "schema", "calendar", "--compact"},
		{"dws", "schema", "--cli-path=calendar event create", "--compact=false", "--format=json"},
		{"dws", "schema", "-f", "json", "calendar/create_calendar_event"},
	} {
		if _, ok := parseSchemaRequest(args); !ok {
			t.Errorf("supported args declined: %q", args)
		}
	}
	for _, args := range [][]string{
		{"dws", "--format=json", "schema"}, {"dws", "schem", "calendar"}, {"dws", "schema", "--all"},
		{"dws", "schema", "--fields", "title"}, {"dws", "schema", "--jq", "."}, {"dws", "schema", "--output", "file"},
		{"dws", "schema", "--help"}, {"dws", "schema", "--compact", "false"}, {"dws", "schema", "--compact=1"},
		{"dws", "schema", "--compact", "--compact=false"}, {"dws", "schema", "--format", "table"},
		{"dws", "schema", "--format"}, {"dws", "schema", "--cli-path="}, {"dws", "schema", "--cli-path", "list", "list"},
		{"dws", "schema", "calendar", "event"}, {"dws", "schema", "--", "calendar"},
	} {
		if _, ok := parseSchemaRequest(args); ok {
			t.Errorf("ambiguous args handled: %q", args)
		}
	}
}

func TestCrossPlatformCoverageLauncherSchemaHitMatchesWireAndReadsOnlySelectedRange(t *testing.T) {
	deps, options, registry, built, _, counters := schemaFixture(t)
	index, err := registry.Index()
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []struct {
		args    []string
		path    string
		compact bool
	}{
		{[]string{"schema"}, "", false}, {[]string{"schema", "LIST", "--compact"}, "", true},
		{[]string{"schema", "sample"}, "sample", false}, {[]string{"schema", "sample group"}, "sample group", false},
		{[]string{"schema", "sample.run", "-f", "json"}, "sample.run", false},
		{[]string{"schema", "--cli-path", "sample legacy run", "--compact"}, "sample legacy run", true},
	} {
		var want map[string]any
		if route.path == "" {
			want, err = schemaruntime.RenderOverview(registry, schemaruntime.TrustedHashes{
				CatalogHash: "sha256:" + hex.EncodeToString(options.SchemaIdentity.SourceSHA256[:]),
				SurfaceHash: "sha256:" + hex.EncodeToString(options.SchemaIdentity.SurfaceSHA256[:]),
			})
		} else {
			want, err = schemaruntime.RenderQuery(registry, index, route.path)
		}
		if err != nil {
			t.Fatal(err)
		}
		if route.compact {
			want = schemaruntime.Compact(want)
		}
		encoded, err := jsonutil.MarshalIndent(want, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		deps.stdout = &output
		deps.args = append([]string{"dws"}, route.args...)
		deps.executable = func() (string, error) { t.Fatal("cache hit tried to load core"); return "", nil }
		before := counters.Snapshot()
		if err := run(options, deps); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(output.Bytes(), append(encoded, '\n')) {
			t.Fatalf("wire differs for %q", route.args)
		}
		after := counters.Snapshot()
		if after.MetaPayloadReadOps-before.MetaPayloadReadOps != 1 || after.WriteOps != 0 || after.LockAttempts != 0 {
			t.Fatalf("unexpected hit I/O: %#v", after)
		}
		wantBytes := uint64(0)
		if route.path != "" {
			for _, d := range built.Descriptors {
				if d.ProductID == "sample" {
					wantBytes = d.Length
				}
			}
		}
		if after.RegistryReadBytes-before.RegistryReadBytes != wantBytes {
			t.Fatalf("selected range bytes=%d want=%d", after.RegistryReadBytes-before.RegistryReadBytes, wantBytes)
		}
	}
}

func TestCrossPlatformCoverageLauncherSchemaDeclinesUncertainInvocationWithoutCacheIO(t *testing.T) {
	deps, options, _, _, _, _ := schemaFixture(t)
	for _, test := range []struct {
		name string
		env  []string
		args []string
	}{
		{"default telemetry", nil, nil}, {"disabled", []string{"DO_NOT_TRACK=1", "DWS_SCHEMA_CACHE_DISABLE=1"}, nil},
		{"agent metadata", []string{"DO_NOT_TRACK=1", "DWS_AGENT_EXT=bad-json"}, nil},
		{"diagnostics", []string{"DO_NOT_TRACK=1", "DWS_PERF_DEBUG=1"}, nil},
		{"future option", []string{"DO_NOT_TRACK=1", "DWS_FUTURE_OPTION=1"}, nil},
		{"relative config", []string{"DO_NOT_TRACK=1", "DWS_CONFIG_DIR=relative"}, nil},
		{"filter", nil, []string{"dws", "schema", "sample.run", "--jq", "."}},
	} {
		t.Run(test.name, func(t *testing.T) {
			local := deps
			local.args = []string{"dws", "schema", "sample.run"}
			local.environ = test.env
			if test.args != nil {
				local.args = test.args
				local.environ = deps.environ
			}
			local.openSchemaCache = func(string) (*schemacache.Cache, error) {
				t.Fatal("uncertain invocation opened cache")
				return nil, nil
			}
			if handled, err := trySchema(options, local); handled || err != nil {
				t.Fatalf("handled=%v err=%v", handled, err)
			}
		})
	}
	for _, name := range []string{"settings.json", "plugins"} {
		path := filepath.Join(environmentValue(deps.environ, "DWS_CONFIG_DIR"), name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("unread settings"), 0o600); err != nil {
			t.Fatal(err)
		}
		local := deps
		local.args = []string{"dws", "schema"}
		local.openSchemaCache = func(string) (*schemacache.Cache, error) { t.Fatal("extension state opened cache"); return nil, nil }
		if handled, err := trySchema(options, local); handled || err != nil {
			t.Fatalf("%s handled=%v err=%v", name, handled, err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCrossPlatformCoverageLauncherSchemaPreservesNestedInstallWarning(t *testing.T) {
	deps, options, _, _, _, _ := schemaFixture(t)
	nested := filepath.Join(environmentValue(deps.environ, "HOME"), ".agents/skills/dws/multi/doc")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "SKILL.md"), []byte("legacy nested layout"), 0o600); err != nil {
		t.Fatal(err)
	}
	deps.args = []string{"dws", "schema"}
	deps.openSchemaCache = func(string) (*schemacache.Cache, error) {
		t.Fatal("legacy-warning path entered cache")
		return nil, nil
	}
	if handled, err := trySchema(options, deps); handled || err != nil {
		t.Fatalf("nested install handled=%v err=%v", handled, err)
	}
}

func TestCrossPlatformCoverageLauncherSchemaCorruptionFallsBackWithoutPartialOutput(t *testing.T) {
	for _, name := range []string{"meta.cache", "registry.shards.cache"} {
		t.Run(name, func(t *testing.T) {
			deps, options, _, _, directory, _ := schemaFixture(t)
			path := filepath.Join(directory, name)
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			corrupt := append([]byte(nil), original...)
			corrupt[schemacache.HeaderSize] ^= 1
			if err := os.WriteFile(path, corrupt, 0o600); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			deps.stdout = &output
			deps.args = []string{"dws", "schema", "sample.run"}
			delegated := false
			deps.delegate = func(_ string, args, env []string, _ string, _ io.Reader, stdout, stderr io.Writer) (int, error) {
				delegated = true
				if output.Len() != 0 || !reflect.DeepEqual(args, deps.args) {
					t.Fatal("fallback changed argv or emitted partial Schema")
				}
				return 0, nil
			}
			if err := run(options, deps); err != nil {
				t.Fatal(err)
			}
			if !delegated {
				t.Fatal("corruption did not reach authoritative core")
			}
		})
	}
}

func TestCrossPlatformCoverageLauncherSchemaShortWriteDoesNotDelegateAfterPublishing(t *testing.T) {
	deps, options, _, _, _, _ := schemaFixture(t)
	deps.args = []string{"dws", "schema", "sample.run"}
	deps.stdout = schemaShortWriter{}
	deps.executable = func() (string, error) { t.Fatal("delegated after output publication"); return "", nil }
	if err := run(options, deps); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write = %v", err)
	}
}

type schemaShortWriter struct{}

func (schemaShortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func schemaFixture(t *testing.T) (dependencies, Options, schemaruntime.SchemaRegistry, schemaruntime.BuiltSchemaCache, string, *schemacache.Counters) {
	t.Helper()
	if !((runtime.GOOS == "darwin" && runtime.GOARCH == "arm64") || (runtime.GOOS == "linux" && runtime.GOARCH == "amd64")) {
		t.Skip("unsupported cache target")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	home, err = os.MkdirTemp(home, ".dws-thin-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	products := []schemaruntime.ProductSpec{}
	for _, id := range []string{"sample", "z-other"} {
		products = append(products, schemaruntime.ProductSpec{ID: id, Name: id, Tools: []schemaruntime.ToolSpec{{
			Identity:    contract.ToolIdentitySpec{ProductID: id, Name: "run", CanonicalPath: id + ".run", Path: id + ".run", CLIPath: id + " group run", PrimaryCLIPath: id + " group run", Aliases: []string{id + " legacy run"}},
			Description: "URL <https://example.test?a=1&b=2>", Parameters: []schemaruntime.ParameterSpec{{Name: "id", Type: "string", Description: "ID", Required: true}},
		}}})
	}
	registry, err := schemaruntime.SchemaRegistryFromRuntime("runtime-assembled", products)
	if err != nil {
		t.Fatal(err)
	}
	overview, err := schemaruntime.BuildSchemaOverview(registry)
	if err != nil {
		t.Fatal(err)
	}
	locators, err := schemaruntime.BuildSchemaProductLocators(registry)
	if err != nil {
		t.Fatal(err)
	}
	hashes := schemaruntime.CacheHashes{SourceSHA256: sha256.Sum256([]byte("source")), SurfaceSHA256: sha256.Sum256([]byte("surface"))}
	built, err := schemaruntime.BuildSchemaCache(registry, schemaruntime.BuildCommandMetaLookup(registry), overview, locators, hashes)
	if err != nil {
		t.Fatal(err)
	}
	artifact := func(kind schemacache.ArtifactKind, data []byte) schemacache.Artifact {
		return schemacache.Artifact{Payload: data, Expectation: schemacache.ArtifactExpectation{Kind: kind, Serializer: schemacache.SerializerProtobuf, Codec: schemacache.CodecRaw, FormatVersion: schemacache.DTOFormatVersion, EncodedLength: uint64(len(data)), DecodedLength: uint64(len(data)), EncodedSHA256: sha256.Sum256(data)}}
	}
	meta, shards := artifact(schemacache.KindMeta, built.Meta), artifact(schemacache.KindRegistry, built.ProductShards)
	identity := schemareader.Identity{Edition: "open", CatalogSnapshotVersion: schemareader.CatalogSnapshotVersion, SourceSHA256: hashes.SourceSHA256, SurfaceSHA256: hashes.SurfaceSHA256, BuildID: sha256.Sum256([]byte("build")), Meta: meta.Expectation, Registry: shards.Expectation}
	cache, err := schemacache.Open("open")
	if err != nil {
		t.Fatal(err)
	}
	directory := cache.Directory()
	if err := cache.Publish(identity.ExpectedIdentity(), shards, meta); err != nil {
		t.Fatal(err)
	}
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	deps, options, _ := testCoreSetup(t, []byte("trusted core"))
	options.SchemaIdentity = &identity
	deps.environ = []string{"DO_NOT_TRACK=1", "HOME=" + home, "DWS_CONFIG_DIR=" + filepath.Join(home, ".dws")}
	deps.stdin = strings.NewReader("")
	counters := &schemacache.Counters{}
	deps.openSchemaCache = func(edition string) (*schemacache.Cache, error) {
		return schemacache.Open(edition, schemacache.WithCounters(counters))
	}
	return deps, options, registry, built, directory, counters
}
