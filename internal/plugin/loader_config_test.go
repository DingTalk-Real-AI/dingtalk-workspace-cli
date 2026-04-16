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

package plugin

import (
	"os"
	"testing"
)

func TestSetAndGetPluginConfig(t *testing.T) {
	dir := t.TempDir()
	loader := &Loader{PluginsDir: dir, CLIVersion: "1.0.0"}

	// Initially empty.
	val, ok := loader.GetPluginConfig("demo-devtool", "DASHSCOPE_API_KEY")
	if ok {
		t.Errorf("expected not found, got %q", val)
	}

	// Set a value.
	loader.SetPluginConfig("demo-devtool", "DASHSCOPE_API_KEY", "sk-test-12345")

	// Read it back.
	val, ok = loader.GetPluginConfig("demo-devtool", "DASHSCOPE_API_KEY")
	if !ok {
		t.Fatal("expected to find config after set")
	}
	if val != "sk-test-12345" {
		t.Errorf("got %q, want sk-test-12345", val)
	}
}

func TestSetPluginConfigMultipleKeys(t *testing.T) {
	dir := t.TempDir()
	loader := &Loader{PluginsDir: dir, CLIVersion: "1.0.0"}

	loader.SetPluginConfig("my-plugin", "API_KEY", "key-1")
	loader.SetPluginConfig("my-plugin", "API_ENDPOINT", "https://example.com")
	loader.SetPluginConfig("other-plugin", "TOKEN", "tok-abc")

	val, ok := loader.GetPluginConfig("my-plugin", "API_KEY")
	if !ok || val != "key-1" {
		t.Errorf("API_KEY = %q (ok=%v), want key-1", val, ok)
	}

	val, ok = loader.GetPluginConfig("my-plugin", "API_ENDPOINT")
	if !ok || val != "https://example.com" {
		t.Errorf("API_ENDPOINT = %q (ok=%v), want https://example.com", val, ok)
	}

	val, ok = loader.GetPluginConfig("other-plugin", "TOKEN")
	if !ok || val != "tok-abc" {
		t.Errorf("TOKEN = %q (ok=%v), want tok-abc", val, ok)
	}
}

func TestUnsetPluginConfig(t *testing.T) {
	dir := t.TempDir()
	loader := &Loader{PluginsDir: dir, CLIVersion: "1.0.0"}

	// Unset on empty returns false.
	if loader.UnsetPluginConfig("demo-devtool", "DASHSCOPE_API_KEY") {
		t.Error("expected false for unset on empty config")
	}

	// Set then unset.
	loader.SetPluginConfig("demo-devtool", "DASHSCOPE_API_KEY", "sk-test-12345")
	if !loader.UnsetPluginConfig("demo-devtool", "DASHSCOPE_API_KEY") {
		t.Error("expected true for unset of existing key")
	}

	// Verify it's gone.
	_, ok := loader.GetPluginConfig("demo-devtool", "DASHSCOPE_API_KEY")
	if ok {
		t.Error("expected not found after unset")
	}
}

func TestUnsetPluginConfigCleansEmptyMap(t *testing.T) {
	dir := t.TempDir()
	loader := &Loader{PluginsDir: dir, CLIVersion: "1.0.0"}

	loader.SetPluginConfig("demo-devtool", "KEY1", "val1")
	loader.UnsetPluginConfig("demo-devtool", "KEY1")

	// After removing the last key, the plugin entry should be cleaned up.
	configs := loader.ListPluginConfig("demo-devtool")
	if len(configs) != 0 {
		t.Errorf("expected empty config map after removing last key, got %v", configs)
	}
}

func TestListPluginConfig(t *testing.T) {
	dir := t.TempDir()
	loader := &Loader{PluginsDir: dir, CLIVersion: "1.0.0"}

	// Empty list.
	configs := loader.ListPluginConfig("demo-devtool")
	if len(configs) != 0 {
		t.Errorf("expected empty, got %v", configs)
	}

	// Set some values.
	loader.SetPluginConfig("demo-devtool", "KEY_A", "val-a")
	loader.SetPluginConfig("demo-devtool", "KEY_B", "val-b")

	configs = loader.ListPluginConfig("demo-devtool")
	if len(configs) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(configs))
	}
	if configs["KEY_A"] != "val-a" {
		t.Errorf("KEY_A = %q, want val-a", configs["KEY_A"])
	}
	if configs["KEY_B"] != "val-b" {
		t.Errorf("KEY_B = %q, want val-b", configs["KEY_B"])
	}
}

func TestInjectPluginConfigEnv(t *testing.T) {
	dir := t.TempDir()
	loader := &Loader{PluginsDir: dir, CLIVersion: "1.0.0"}

	// Use a unique env var name to avoid test pollution.
	envKey := "DWS_TEST_INJECT_CONFIG_" + t.Name()
	t.Cleanup(func() { os.Unsetenv(envKey) })

	loader.SetPluginConfig("demo-devtool", envKey, "injected-value")

	// Ensure it's not already set.
	os.Unsetenv(envKey)

	loader.InjectPluginConfigEnv()

	got := os.Getenv(envKey)
	if got != "injected-value" {
		t.Errorf("env %s = %q, want injected-value", envKey, got)
	}
}

func TestInjectPluginConfigEnvDoesNotOverride(t *testing.T) {
	dir := t.TempDir()
	loader := &Loader{PluginsDir: dir, CLIVersion: "1.0.0"}

	envKey := "DWS_TEST_INJECT_NOOVERRIDE_" + t.Name()
	t.Cleanup(func() { os.Unsetenv(envKey) })

	// Pre-set the env var.
	os.Setenv(envKey, "user-value")

	loader.SetPluginConfig("demo-devtool", envKey, "config-value")
	loader.InjectPluginConfigEnv()

	got := os.Getenv(envKey)
	if got != "user-value" {
		t.Errorf("env %s = %q, want user-value (should not be overridden)", envKey, got)
	}
}

func TestSetPluginConfigOverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	loader := &Loader{PluginsDir: dir, CLIVersion: "1.0.0"}

	loader.SetPluginConfig("demo-devtool", "KEY", "old-value")
	loader.SetPluginConfig("demo-devtool", "KEY", "new-value")

	val, ok := loader.GetPluginConfig("demo-devtool", "KEY")
	if !ok || val != "new-value" {
		t.Errorf("got %q (ok=%v), want new-value", val, ok)
	}
}

func TestGetPluginConfigWrongPlugin(t *testing.T) {
	dir := t.TempDir()
	loader := &Loader{PluginsDir: dir, CLIVersion: "1.0.0"}

	loader.SetPluginConfig("plugin-a", "KEY", "value")

	_, ok := loader.GetPluginConfig("plugin-b", "KEY")
	if ok {
		t.Error("expected not found for different plugin name")
	}
}
