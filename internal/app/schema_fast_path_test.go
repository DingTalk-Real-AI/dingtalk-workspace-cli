// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/jsonutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageCoreSchemaFastPathPreservesWireAndTelemetryResult(t *testing.T) {
	const child = "DWS_CORE_SCHEMA_FAST_PATH_TEST_CHILD"
	if os.Getenv(child) != "1" {
		command := exec.Command(os.Args[0], "-test.run=^TestCrossPlatformCoverageCoreSchemaFastPathPreservesWireAndTelemetryResult$", "-test.count=1")
		command.Env = append(os.Environ(), child+"=1")
		data, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("core fast path child: %v\n%s", err, data)
		}
		return
	}
	home, artifacts := coreSchemaFastPathFixture(t)
	if err := cli.AuditSchemaAssembly(func() error {
		_, _ = cli.SchemaCacheFastPathIdentity()
		return nil
	}); !errors.Is(err, cli.ErrSchemaAssemblyConsumedDelivery) {
		t.Fatalf("early delivery escaped the generator audit: %v", err)
	}
	for _, args := range [][]string{
		{"dws", "schema", "calendar event create", "--format=json"},
		{"dws", "schema", "calendar event create"},
	} {
		t.Run(strings.Join(args[2:], "_"), func(t *testing.T) {
			testseam.Swap(t, &os.Args, args)
			testseam.Swap(t, &rootNewRootCommandWithEngine, func(context.Context, *pipeline.Engine) *cobra.Command {
				t.Fatal("authenticated hit constructed the command tree")
				return nil
			})
			out, err := os.CreateTemp(home, "stdout-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = out.Close() })
			testseam.Swap(t, &os.Stdout, out)
			code, path, message := ExecuteWithTelemetry()
			if code != 0 || path != "schema" || message != "" {
				t.Fatalf("telemetry result = (%d, %q, %q)", code, path, message)
			}
			payload, err := artifacts.RenderQuery("calendar event create")
			if err != nil {
				t.Fatal(err)
			}
			want, err := jsonutil.MarshalIndent(payload, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(out.Name())
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, append(want, '\n')) {
				t.Fatal("core fast path changed authoritative wire")
			}
		})
	}

	t.Run("write failure remains a tracked error", func(t *testing.T) {
		testseam.Swap(t, &os.Args, []string{"dws", "schema", "calendar event create", "-f", "json"})
		testseam.Swap(t, &rootNewRootCommandWithEngine, func(context.Context, *pipeline.Engine) *cobra.Command {
			t.Fatal("output failure attempted fallback")
			return nil
		})
		out, err := os.CreateTemp(home, "closed-output-")
		if err != nil {
			t.Fatal(err)
		}
		if err := out.Close(); err != nil {
			t.Fatal(err)
		}
		stderr, err := os.CreateTemp(home, "stderr-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = stderr.Close() })
		testseam.Swap(t, &os.Stdout, out)
		testseam.Swap(t, &os.Stderr, stderr)
		code, path, message := ExecuteWithTelemetry()
		if code != 5 || path != "schema" || message == "" {
			t.Fatalf("write failure result = (%d, %q, %q)", code, path, message)
		}
		data, err := os.ReadFile(stderr.Name())
		if err != nil {
			t.Fatal(err)
		}
		var wire struct {
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(data, &wire); err != nil {
			t.Fatalf("invalid JSON error: %v", err)
		}
		if wire.Error.Code != 5 || !strings.Contains(wire.Error.Message, "closed") {
			t.Fatalf("wrong error: %s", data)
		}
	})

	t.Run("signal lifecycle is retained", func(t *testing.T) {
		testseam.Swap(t, &os.Args, []string{"dws", "schema", "-f", "json"})
		stopped := false
		testseam.Swap(t, &rootInstallProcessSignalContext, func(ctx context.Context, store *output.ResultStore) (context.Context, *processSignalState, func()) {
			state := &processSignalState{}
			state.Record(syscall.SIGTERM, processResultCompleted(store))
			return ctx, state, func() { stopped = true }
		})
		for _, stream := range []**os.File{&os.Stdout, &os.Stderr} {
			file, err := os.CreateTemp(home, "signal-output-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = file.Close() })
			testseam.Swap(t, stream, file)
		}
		code, path, message := ExecuteWithTelemetry()
		if code != 143 || path != "schema" || message == "" || !stopped {
			t.Fatalf("signal result = (%d, %q, %q), stopped=%v", code, path, message, stopped)
		}
	})

	t.Run("uncertain invocation falls back", func(t *testing.T) {
		for _, entry := range []string{"DWS_SCHEMA_CACHE_DISABLE=1", "DWS_INTERNAL_FUTURE_MARKER=1", "DWS_PERF_DEBUG=1"} {
			environment := append(os.Environ(), entry)
			if _, ok := prepareSchemaFastPath([]string{"dws", "schema"}, environment); ok {
				t.Fatalf("accepted %s", entry)
			}
		}
		for _, name := range []string{"settings.json", "plugins", "shortcuts"} {
			path := filepath.Join(home, ".dws", name)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("invalid"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, ok := prepareSchemaFastPath([]string{"dws", "schema"}, os.Environ()); ok {
				t.Fatalf("suppressed %s diagnostics", name)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("replacement authority cannot reuse binary cache", func(t *testing.T) {
		previous := edition.Get()
		for _, mutate := range []func(*edition.Hooks){
			func(h *edition.Hooks) { h.IsEmbedded = true },
			func(h *edition.Hooks) {
				h.ConfigDir = func() string { t.Fatal("fast path read host configuration"); return "" }
			},
			func(h *edition.Hooks) { h.AfterPersistentPreRun = func(*cobra.Command, []string) error { return nil } },
		} {
			hooks := *previous
			mutate(&hooks)
			edition.Override(&hooks)
			_, accepted := prepareSchemaFastPath([]string{"dws", "schema"}, os.Environ())
			edition.Override(previous)
			if accepted {
				t.Fatal("fast path bypassed host execution/configuration hooks")
			}
		}
		cli.MarkSchemaCacheRuntimeUncertain()
		if _, ok := prepareSchemaFastPath([]string{"dws", "schema"}, os.Environ()); ok {
			t.Fatal("runtime uncertainty did not disable the fast path")
		}
		options, ok := productionSchemaCacheOptions()
		if !ok {
			t.Fatal("lost test identity")
		}
		if err := cli.RegisterSchemaCacheOptions(options); err != nil {
			t.Fatal(err)
		}
		cli.RegisterSchemaSourceRoot(func() *cobra.Command {
			t.Fatal("fast path attempted to assemble a replacement authority")
			return nil
		})
		if _, ok := prepareSchemaFastPath([]string{"dws", "schema"}, os.Environ()); ok {
			t.Fatal("replacement source inherited the compiled identity")
		}
	})
}

func coreSchemaFastPathFixture(t *testing.T) (string, cli.SchemaCacheArtifacts) {
	t.Helper()
	if !((runtime.GOOS == "darwin" && runtime.GOARCH == "arm64") || (runtime.GOOS == "linux" && runtime.GOARCH == "amd64")) {
		t.Skip("unsupported cache target")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	home, err = os.MkdirTemp(home, ".dws-core-schema-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "DWS_") {
			t.Setenv(key, "")
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("DO_NOT_TRACK", "") // ExecuteWithTelemetry itself never starts a tracker.
	t.Setenv("DWS_CONFIG_DIR", filepath.Join(home, ".dws"))
	t.Setenv("DWS_INTERNAL_LAUNCHER_PATH", "/synthetic/bin/dws")
	t.Setenv("DWS_INTERNAL_CORE_SHA256", strings.Repeat("a", 64))
	t.Setenv("DWS_INTERNAL_CORE_VERSION", "v0.0.0-test")
	resolved, err := cli.ResolveSchemaBuild(NewSchemaSourceRootCommand())
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := cli.BuildSchemaCacheArtifacts(resolved)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(saveSchemaCacheBuildVars())
	schemaCacheEdition = "open"
	schemaCacheGOOS, schemaCacheGOARCH = runtime.GOOS, runtime.GOARCH
	schemaCacheSourceSHA256 = strings.TrimPrefix(artifacts.SourceHash, "sha256:")
	schemaCacheSurfaceSHA256 = strings.TrimPrefix(artifacts.SurfaceHash, "sha256:")
	buildID := sha256.Sum256([]byte("core-fast-path-test"))
	schemaCacheBuildID = hex.EncodeToString(buildID[:])
	schemaCacheMetaSHA256 = hex.EncodeToString(artifacts.MetaSHA256[:])
	schemaCacheRegistrySHA256 = hex.EncodeToString(artifacts.RegistrySHA256[:])
	schemaCacheMetaLength = strconv.Itoa(len(artifacts.Meta))
	schemaCacheRegistryLength = strconv.Itoa(len(artifacts.Registry))
	options, ok := productionSchemaCacheOptions()
	if !ok {
		t.Fatal("invalid test identity")
	}
	// TestMain registers the source before these synthetic ldflag values are
	// installed. Preserve that source and install its fixture identity explicitly.
	if err := cli.RegisterSchemaCacheOptions(options); err != nil {
		t.Fatal(err)
	}
	cache, err := schemacache.Open("open")
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	if err := cache.Publish(options.Identity.ExpectedIdentity(), artifacts.RegistryArtifact(), artifacts.MetaArtifact()); err != nil {
		t.Fatal(err)
	}
	return home, artifacts
}
