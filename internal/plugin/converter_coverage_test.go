// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package plugin

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveCLIOverlayDefaultsAndInlineErrors(t *testing.T) {
	plugin := &Plugin{
		Root: t.TempDir(),
		Manifest: Manifest{MCPServers: map[string]*MCPServer{
			"empty":      {CLI: nil},
			"whitespace": {CLI: json.RawMessage("   \n\t")},
			"fallback":   {CLI: json.RawMessage(`{"id":" ","command":""}`)},
			"malformed":  {CLI: json.RawMessage(`{`)},
			"unknown":    {CLI: json.RawMessage(`{"id":"plugin","unknownField":true}`)},
			"unknownFlag": {CLI: json.RawMessage(`{
				"toolOverrides":{"tool":{"flags":{"value":{"unknownFlagField":true}}}}
			}`)},
			"trailing": {CLI: json.RawMessage(`{} {}`)},
			"legacyTools": {CLI: json.RawMessage(`{
				"tools":[{
					"name":"tool","cliName":"leaf","title":"Title","description":"Description",
					"isSensitive":true,"category":"read","hidden":true,
					"flags":{"value":{"alias":"value-alias","shorthand":"v"}}
				}]
			}`)},
		}},
	}

	for _, serverKey := range []string{"missing", "empty", "whitespace", "fallback"} {
		t.Run(serverKey, func(t *testing.T) {
			overlay, ok := plugin.ResolveCLIOverlay(serverKey)
			if !ok || overlay.ID != serverKey || overlay.Command != serverKey {
				t.Fatalf("ResolveCLIOverlay(%q) = (%#v, %v), want default overlay", serverKey, overlay, ok)
			}
		})
	}
	legacy, ok := plugin.ResolveCLIOverlay("legacyTools")
	if !ok || len(legacy.Tools) != 1 || legacy.Tools[0].CLIName != "leaf" ||
		legacy.Tools[0].Flags["value"].Alias != "value-alias" {
		t.Fatalf("ResolveCLIOverlay(legacyTools) = (%#v, %v), want historical tool metadata", legacy, ok)
	}
	if overlay, ok := plugin.ResolveCLIOverlay("malformed"); ok {
		t.Fatalf("ResolveCLIOverlay(malformed) = (%#v, true), want failure", overlay)
	}
	for _, serverKey := range []string{"unknown", "unknownFlag", "trailing"} {
		if overlay, ok := plugin.ResolveCLIOverlay(serverKey); ok {
			t.Fatalf("ResolveCLIOverlay(%s) = (%#v, true), want strict failure", serverKey, overlay)
		}
	}
}

func TestResolveCLIOverlayRejectsInvalidExternalFiles(t *testing.T) {
	root := t.TempDir()
	plugin := &Plugin{
		Root: root,
		Manifest: Manifest{MCPServers: map[string]*MCPServer{
			"external": {},
		}},
	}
	server := plugin.Manifest.MCPServers["external"]

	t.Run("malformed path JSON", func(t *testing.T) {
		server.CLI = json.RawMessage(`"unterminated`)
		if overlay, ok := plugin.ResolveCLIOverlay("external"); ok {
			t.Fatalf("ResolveCLIOverlay() = (%#v, true), want malformed path failure", overlay)
		}
	})

	t.Run("blank path", func(t *testing.T) {
		server.CLI = json.RawMessage(`"  "`)
		if overlay, ok := plugin.ResolveCLIOverlay("external"); ok {
			t.Fatalf("ResolveCLIOverlay() = (%#v, true), want blank path failure", overlay)
		}
	})

	t.Run("missing root", func(t *testing.T) {
		plugin.Root = filepath.Join(root, "does-not-exist")
		server.CLI = json.RawMessage(`"overlay.json"`)
		if overlay, ok := plugin.ResolveCLIOverlay("external"); ok {
			t.Fatalf("ResolveCLIOverlay() = (%#v, true), want missing root failure", overlay)
		}
		plugin.Root = root
	})

	t.Run("missing file", func(t *testing.T) {
		server.CLI = json.RawMessage(`"missing.json"`)
		if overlay, ok := plugin.ResolveCLIOverlay("external"); ok {
			t.Fatalf("ResolveCLIOverlay() = (%#v, true), want missing file failure", overlay)
		}
	})

	t.Run("read error", func(t *testing.T) {
		if err := os.Mkdir(filepath.Join(root, "overlay-dir"), 0o700); err != nil {
			t.Fatal(err)
		}
		server.CLI = json.RawMessage(`"overlay-dir"`)
		if overlay, ok := plugin.ResolveCLIOverlay("external"); ok {
			t.Fatalf("ResolveCLIOverlay() = (%#v, true), want directory read failure", overlay)
		}
	})

	t.Run("oversized file", func(t *testing.T) {
		path := filepath.Join(root, "oversized.json")
		if err := os.WriteFile(path, bytes.Repeat([]byte(" "), maxPluginCLIOverlayBytes+1), 0o600); err != nil {
			t.Fatal(err)
		}
		server.CLI = json.RawMessage(`"oversized.json"`)
		if overlay, ok := plugin.ResolveCLIOverlay("external"); ok {
			t.Fatalf("ResolveCLIOverlay() = (%#v, true), want oversized file failure", overlay)
		}
	})
}

func TestResolveCLIOverlayExternalFileAppliesFallbackIdentity(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "overlay.json"), []byte(`{
		"id":"",
		"command":" ",
		"aliases":["conference"]
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	plugin := &Plugin{
		Root: root,
		Manifest: Manifest{MCPServers: map[string]*MCPServer{
			"server-key": {CLI: json.RawMessage(`"overlay.json"`)},
		}},
	}

	overlay, ok := plugin.ResolveCLIOverlay("server-key")
	if !ok || overlay.ID != "server-key" || overlay.Command != "server-key" ||
		len(overlay.Aliases) != 1 || overlay.Aliases[0] != "conference" {
		t.Fatalf("ResolveCLIOverlay() = (%#v, %v), want fallback identity with file metadata", overlay, ok)
	}
}
