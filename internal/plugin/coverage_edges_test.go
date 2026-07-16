// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package plugin

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
)

func writePluginManifest(t *testing.T, dir string, m Manifest) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

const fakeGitProgram = `package main

import (
	"os"
	"path/filepath"
)

func main() {
	mode := os.Getenv("DWS_PLUGIN_TEST_GIT_MODE")
	if mode == "clone-fail" {
		os.Exit(1)
	}
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	dest := os.Args[len(os.Args)-1]
	if err := os.MkdirAll(filepath.Join(dest, ".git"), 0o755); err != nil {
		os.Exit(3)
	}

	var manifest string
	switch mode {
	case "missing":
		return
	case "malformed":
		manifest = "{"
	case "invalid":
		manifest = "{\"name\":\"x\",\"version\":\"1.0.0\"}"
	case "build":
		manifest = "{\"name\":\"git-build\",\"version\":\"1.0.0\",\"build\":{\"command\":\"true\"}}"
	case "valid":
		manifest = "{\"name\":\"git-valid\",\"version\":\"1.0.0\"}"
	default:
		manifest = "{\"name\":\"git-plugin\",\"version\":\"1.0.0\",\"type\":\"user\"}"
	}
	if err := os.WriteFile(filepath.Join(dest, "plugin.json"), []byte(manifest), 0o644); err != nil {
		os.Exit(4)
	}
	if err := os.WriteFile(filepath.Join(dest, "content.txt"), []byte("cloned"), 0o644); err != nil {
		os.Exit(5)
	}
}
`

func buildFakeGit(t *testing.T) string {
	t.Helper()
	source := filepath.Join(t.TempDir(), "fake_git.go")
	if err := os.WriteFile(source, []byte(fakeGitProgram), 0o600); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	name := "git"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", filepath.Join(binDir, name), source)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake git: %v\n%s", err, output)
	}
	return binDir
}

func TestCrossPlatformCoveragePluginConverterEdges(t *testing.T) {
	root := filepath.Join(t.TempDir(), "user", "sample-plugin")
	t.Setenv("PLUGIN_TOKEN", "secret")
	p := &Plugin{
		Manifest: Manifest{
			Name:        "sample-plugin",
			Description: "sample",
			MCPServers: map[string]*MCPServer{
				"missing": {Type: "stdio"},
				"stdio": {
					Type:    "stdio",
					Command: "$" + "{DWS_PLUGIN_ROOT}/bin/server",
					Args:    []string{"--data", "$" + "{DWS_PLUGIN_DATA}", "$" + "{PLUGIN_TOKEN}"},
					Env:     map[string]string{"ROOT": "$" + "{DWS_PLUGIN_ROOT}"},
				},
				"http-default": {
					Type:     "streamable-http",
					Endpoint: "https://example.test",
					CLI:      json.RawMessage("{"),
				},
				"http-overlay": {
					Type:     "streamable-http",
					Endpoint: "https://other.test",
					CLI:      json.RawMessage("{\"id\":\"custom\",\"command\":\"run\"}"),
					Headers:  map[string]string{"Authorization": "Bearer $" + "{PLUGIN_TOKEN}"},
				},
			},
		},
		Root: root,
	}

	if got := p.StdioClients(nil); len(got) != 1 || got[0].Key != "stdio" || got[0].Client == nil {
		t.Fatalf("StdioClients(nil) = %#v", got)
	}
	if got := p.StdioClients(&UserContext{UserID: "u1", CorpID: "c1"}); len(got) != 1 {
		t.Fatalf("StdioClients(user) length = %d", len(got))
	}
	descriptors := p.ToServerDescriptors()
	if len(descriptors) != 2 {
		t.Fatalf("descriptors length = %d", len(descriptors))
	}
	seenDefault := false
	seenOverlay := false
	for _, d := range descriptors {
		switch d.Key {
		case "http-default":
			seenDefault = d.CLI.ID == "http-default" && d.CLI.Command == "http-default" && d.HasCLIMeta
		case "http-overlay":
			seenOverlay = d.CLI.ID == "custom" && d.CLI.Command == "run" &&
				d.AuthHeaders["Authorization"] == "Bearer secret"
		}
	}
	if !seenDefault || !seenOverlay {
		t.Fatalf("descriptor defaults/overlay not covered: %#v", descriptors)
	}
	dataDir := filepath.Join(filepath.Dir(filepath.Dir(root)), "data")
	if got := expandPluginVars("$"+"{DWS_PLUGIN_ROOT}|$"+"{DWS_PLUGIN_DATA}|$"+"{PLUGIN_TOKEN}", root); got != root+"|"+dataDir+"|secret" {
		t.Fatalf("expandPluginVars = %q", got)
	}
}

func TestCrossPlatformCoverageHookAdapterEdges(t *testing.T) {
	phases := map[string]pipeline.Phase{
		" PRE-PARSE ":   pipeline.PreParse,
		"post-parse":    pipeline.PostParse,
		"pre-request":   pipeline.PreRequest,
		"post-response": pipeline.PostResponse,
		"unknown":       pipeline.PreRequest,
	}
	for raw, want := range phases {
		h := NewHookAdapter("plug", HookEntry{Phase: raw, Command: "true", Timeout: 1})
		if h.Phase() != want {
			t.Errorf("phase %q = %v, want %v", raw, h.Phase(), want)
		}
		if !strings.HasPrefix(h.Name(), "plugin-hook:plug/") {
			t.Errorf("Name = %q", h.Name())
		}
	}

	ctx := &pipeline.Context{Command: "chat.send", Params: map[string]any{"x": 1}, Args: []string{"a"}}
	tests := []struct {
		name    string
		entry   HookEntry
		context *pipeline.Context
		wantErr bool
	}{
		{"match success", HookEntry{Phase: "pre-request", Matcher: "chat.*", Command: "read input; test -n \"$input\""}, ctx, false},
		{"no match", HookEntry{Matcher: "mail.*", Command: "exit 2"}, ctx, false},
		{"invalid matcher", HookEntry{Matcher: "[", Command: "exit 2"}, ctx, false},
		{"ordinary failure", HookEntry{Command: "echo failed; exit 1"}, ctx, false},
		{"abort", HookEntry{Command: "echo aborted; exit 2"}, ctx, true},
		{"marshal failure", HookEntry{Command: "exit 2"}, &pipeline.Context{Params: map[string]any{"bad": func() {}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewHookAdapter("plug", tt.entry).Handle(tt.context)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Handle error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	h := NewHookAdapter("plug", HookEntry{Command: "sleep 1", Timeout: 0})
	h.timeout = time.Millisecond
	if err := h.Handle(ctx); err != nil {
		t.Fatalf("timeout is non-fatal: %v", err)
	}
}

func TestCrossPlatformCoverageManifestEdges(t *testing.T) {
	dir := t.TempDir()
	if _, err := ParseManifest(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("expected missing manifest error")
	}
	badPath := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badPath, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseManifest(badPath); err == nil {
		t.Fatal("expected malformed manifest error")
	}

	valid := Manifest{Name: "valid-plugin", Version: "v1.2.3-beta"}
	absoluteCommand := filepath.Join(t.TempDir(), "bin", "server")
	for name, mutate := range map[string]func(*Manifest){
		"absolute stdio command": func(m *Manifest) {
			m.MCPServers = map[string]*MCPServer{"x": {Type: "stdio", Command: absoluteCommand}}
		},
		"unsupported server": func(m *Manifest) {
			m.MCPServers = map[string]*MCPServer{"x": {Type: "other"}}
		},
		"unsafe hooks": func(m *Manifest) { m.Hooks = "../hooks.json" },
	} {
		t.Run(name, func(t *testing.T) {
			m := valid
			mutate(&m)
			if err := m.Validate("dev"); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	if err := valid.Validate("dev"); err != nil {
		t.Fatalf("valid dev manifest: %v", err)
	}
	if err := validateSafePath("skills"); err != nil {
		t.Fatal(err)
	}
	if isValidSemver("1.2") || isValidSemver("1.x.3") || !isValidSemver("v1.2.3-beta") {
		t.Fatal("semver validation edge failed")
	}
	if got := compareSemver("2.0.0", "1.9.9"); got != 1 {
		t.Fatalf("major comparison = %d", got)
	}
	if got := compareSemver("1.2.0", "1.1.9"); got != 1 {
		t.Fatalf("minor comparison = %d", got)
	}
	if got := compareSemver("1.2.3", "1.2.4"); got != -1 {
		t.Fatalf("patch comparison = %d", got)
	}
	if got := compareSemver("1.2.3", "1.2.3"); got != 0 {
		t.Fatalf("equal comparison = %d", got)
	}
	if a, b, c := parseSemver("bad"); a != 0 || b != 0 || c != 0 {
		t.Fatalf("parse invalid = %d.%d.%d", a, b, c)
	}

	p := &Plugin{Manifest: Manifest{Name: "valid-plugin"}, Root: dir}
	if hooks, err := p.LoadHooks(); err != nil || hooks != nil {
		t.Fatalf("empty hooks = %#v, %v", hooks, err)
	}
	p.Manifest.Hooks = "hooks.json"
	if hooks, err := p.LoadHooks(); err != nil || hooks != nil {
		t.Fatalf("missing hooks = %#v, %v", hooks, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hooks.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := p.LoadHooks(); err == nil {
		t.Fatal("expected malformed hooks error")
	}
	if err := os.WriteFile(filepath.Join(dir, "hooks.json"), []byte("{\"hooks\":[{\"phase\":\"pre-request\",\"command\":\"true\"}]}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if hooks, err := p.LoadHooks(); err != nil || len(hooks.Hooks) != 1 {
		t.Fatalf("valid hooks = %#v, %v", hooks, err)
	}
	if err := os.Remove(filepath.Join(dir, "hooks.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "hooks.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := p.LoadHooks(); err == nil {
		t.Fatal("expected hooks read error")
	}
	if got := (&Plugin{Root: dir}).SkillsDir(); got != filepath.Join(dir, "skills") {
		t.Fatalf("default SkillsDir = %q", got)
	}
	p.Manifest.Skills = "custom"
	if got := p.SkillsDir(); got != filepath.Join(dir, "custom") {
		t.Fatalf("custom SkillsDir = %q", got)
	}
}

func TestCrossPlatformCoverageLoaderDiscoveryLifecycle(t *testing.T) {
	oldHome := pluginUserHomeDir
	t.Cleanup(func() { pluginUserHomeDir = oldHome })
	home := t.TempDir()
	pluginUserHomeDir = func() (string, error) { return home, nil }
	defaultLoader := NewLoader("1.2.3")
	if defaultLoader.PluginsDir != filepath.Join(home, ".dws", "plugins") {
		t.Fatalf("NewLoader path = %q", defaultLoader.PluginsDir)
	}
	pluginUserHomeDir = func() (string, error) { return "", errors.New("home") }
	if got := NewLoader("dev").PluginsDir; got != filepath.Join(".dws", "plugins") {
		t.Fatalf("NewLoader error path = %q", got)
	}

	root := t.TempDir()
	l := &Loader{PluginsDir: root, CLIVersion: "1.0.0"}
	userDir := filepath.Join(root, "user")
	direct := filepath.Join(userDir, "direct-plugin")
	workspace := filepath.Join(userDir, "acme")
	nested := filepath.Join(workspace, "nested-plugin")
	invalid := filepath.Join(userDir, "invalid-plugin")
	incompatible := filepath.Join(userDir, "future-plugin")
	writePluginManifest(t, direct, Manifest{Name: "direct-plugin", Version: "1.0.0", Description: "direct"})
	writePluginManifest(t, nested, Manifest{Name: "nested-plugin", Version: "1.0.0", Description: "nested"})
	writePluginManifest(t, invalid, Manifest{Name: "x", Version: "1.0.0"})
	writePluginManifest(t, incompatible, Manifest{Name: "future-plugin", Version: "1.0.0", MinCLIVersion: "9.0.0"})
	if err := os.WriteFile(filepath.Join(userDir, "not-a-dir"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "not-a-dir"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	brokenNested := filepath.Join(workspace, "broken-plugin")
	if err := os.MkdirAll(brokenNested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(brokenNested, "plugin.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	l.saveSettings(&Settings{EnabledPlugins: map[string]bool{
		"direct-plugin":      false,
		"acme/nested-plugin": true,
	}})
	got := l.LoadUser()
	if len(got) != 1 || got[0].Manifest.Name != "nested-plugin" {
		t.Fatalf("LoadUser = %#v", got)
	}

	infos := l.ListInstalled()
	if len(infos) != 4 {
		t.Fatalf("ListInstalled user count = %d: %#v", len(infos), infos)
	}
	foundDirect := false
	foundNested := false
	for _, info := range infos {
		if info.Name == "direct-plugin" {
			foundDirect = !info.Enabled && info.Type == "user" && info.Description == "direct"
		}
		if info.Name == "acme/nested-plugin" {
			foundNested = info.Enabled && info.Type == "user" && info.Description == "nested"
		}
	}
	if !foundDirect || !foundNested {
		t.Fatalf("installed info missing: %#v", infos)
	}

	if err := l.SetEnabled("missing-plugin", true); err == nil {
		t.Fatal("expected missing SetEnabled error")
	}
	if err := l.SetEnabled("direct-plugin", true); err != nil {
		t.Fatal(err)
	}
	if got := l.LoadUser(); len(got) != 2 {
		t.Fatalf("enabled LoadUser = %#v", got)
	}
	if err := l.SetEnabled("acme/nested-plugin", false); err != nil {
		t.Fatal(err)
	}
	if !l.loadSettings().EnabledPlugins["direct-plugin"] {
		t.Fatal("direct plugin was not enabled")
	}
	if err := l.RemovePlugin("missing-plugin", false); err == nil {
		t.Fatal("expected missing RemovePlugin error")
	}
	dataDir := filepath.Join(root, configPluginDataDirForTest(), "direct-plugin")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := l.RemovePlugin("direct-plugin", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dataDir); err != nil {
		t.Fatalf("keepData removed data: %v", err)
	}
	if err := l.RemovePlugin("acme/nested-plugin", false); err != nil {
		t.Fatal(err)
	}

	plain := &Loader{PluginsDir: filepath.Join(t.TempDir(), "plugins")}
	if plain.settingsPath() != filepath.Join(filepath.Dir(plain.PluginsDir), "settings.json") {
		t.Fatalf("production settingsPath = %q", plain.settingsPath())
	}
	invalidSettings := &Loader{PluginsDir: t.TempDir()}
	if err := os.WriteFile(invalidSettings.settingsPath(), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if settings := invalidSettings.loadSettings(); settings == nil || settings.EnabledPlugins != nil {
		t.Fatalf("invalid settings = %#v", settings)
	}
}

func configPluginDataDirForTest() string {
	return "data"
}

func TestCrossPlatformCoverageLoaderDevAndConfigEdges(t *testing.T) {
	root := t.TempDir()
	l := &Loader{PluginsDir: root, CLIVersion: "1.0.0"}
	if got := l.LoadDev(); got != nil {
		t.Fatalf("empty LoadDev = %#v", got)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	bad := filepath.Join(t.TempDir(), "bad")
	valid := filepath.Join(t.TempDir(), "valid")
	writePluginManifest(t, bad, Manifest{Name: "x", Version: "1.0.0"})
	writePluginManifest(t, valid, Manifest{Name: "valid-plugin", Version: "1.0.0", Description: "dev"})
	l.saveSettings(&Settings{DevPlugins: map[string]string{
		"missing-plugin": missing,
		"bad-plugin":     bad,
		"valid-plugin":   valid,
	}})
	if got := l.LoadDev(); len(got) != 1 || got[0].Root != valid {
		t.Fatalf("LoadDev = %#v", got)
	}
	infos := l.ListInstalled()
	if len(infos) != 2 {
		t.Fatalf("ListInstalled dev entries = %#v", infos)
	}
	if err := l.UnregisterDevPlugin("valid-plugin"); err != nil {
		t.Fatal(err)
	}

	if l.UnsetPluginConfig("missing", "key") {
		t.Fatal("unset missing plugin succeeded")
	}
	l.saveSettings(&Settings{PluginConfigs: map[string]map[string]any{
		"plug": {"number": 1, "text": "value", "empty": "", "PATH": "/bad"},
	}})
	if _, ok := l.GetPluginConfig("plug", "number"); ok {
		t.Fatal("non-string config returned")
	}
	if got := l.ListPluginConfig("plug"); len(got) != 3 || got["text"] != "value" {
		t.Fatalf("ListPluginConfig mixed = %#v", got)
	}
	if got := l.ListPluginConfig("missing"); len(got) != 0 {
		t.Fatalf("missing configs = %#v", got)
	}
	key := "DWS_PLUGIN_EDGE_CONFIG"
	t.Setenv(key, "existing")
	settings := l.loadSettings()
	settings.PluginConfigs["plug"][key] = "replacement"
	settings.PluginConfigs["plug"]["DWS_PLUGIN_EDGE_NEW"] = "new"
	settings.PluginConfigs["plug"]["not-string"] = 7
	l.saveSettings(settings)
	t.Cleanup(func() { _ = os.Unsetenv("DWS_PLUGIN_EDGE_NEW") })
	l.InjectPluginConfigEnv()
	if os.Getenv(key) != "existing" || os.Getenv("DWS_PLUGIN_EDGE_NEW") != "new" {
		t.Fatalf("config env injection failed: %q %q", os.Getenv(key), os.Getenv("DWS_PLUGIN_EDGE_NEW"))
	}

	empty := &Loader{PluginsDir: t.TempDir()}
	empty.InjectPluginConfigEnv()
	empty.saveSettings(nil)
}

func TestCrossPlatformCoverageLoaderInstallBuildAndGit(t *testing.T) {
	root := t.TempDir()
	l := &Loader{PluginsDir: root, CLIVersion: "1.0.0"}
	if _, err := l.InstallFromDir(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected missing install error")
	}
	invalid := t.TempDir()
	writePluginManifest(t, invalid, Manifest{Name: "x", Version: "1.0.0"})
	if _, err := l.InstallFromDir(invalid); err == nil {
		t.Fatal("expected invalid install error")
	}

	src := t.TempDir()
	builtOutput := filepath.Join("bin", "server")
	buildCommand := "mkdir -p bin && printf ok > bin/server"
	if runtime.GOOS == "windows" {
		builtOutput += ".exe"
		buildCommand = "if not exist bin mkdir bin && echo ok>bin\\server.exe"
	}
	writePluginManifest(t, src, Manifest{
		Name:    "built-plugin",
		Version: "1.0.0",
		Build: &BuildConfig{
			Command: buildCommand,
			Output:  builtOutput,
		},
	})
	if err := os.WriteFile(filepath.Join(src, "payload.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(root, "user", "built-plugin", "stale.txt")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	installed, err := l.InstallFromDir(src)
	if err != nil {
		t.Fatalf("InstallFromDir: %v", err)
	}
	if installed.Manifest.Name != "built-plugin" {
		t.Fatalf("installed = %#v", installed)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale file remains: %v", err)
	}
	info, err := os.Stat(filepath.Join(installed.Root, builtOutput))
	if err != nil {
		t.Fatalf("built output is missing: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		t.Fatalf("built output is not executable: %v %#o", err, info.Mode())
	}

	buildFailure := t.TempDir()
	writePluginManifest(t, buildFailure, Manifest{
		Name: "failed-plugin", Version: "1.0.0",
		Build: &BuildConfig{Command: "exit 1"},
	})
	if _, err := l.InstallFromDir(buildFailure); err == nil {
		t.Fatal("expected build failure")
	}
	if _, err := os.Stat(filepath.Join(root, "user", "failed-plugin")); !os.IsNotExist(err) {
		t.Fatalf("failed build destination remains: %v", err)
	}

	noBuild := t.TempDir()
	writePluginManifest(t, noBuild, Manifest{Name: "plain-plugin", Version: "1.0.0"})
	if err := BuildPlugin(noBuild); err != nil {
		t.Fatal(err)
	}
	if err := BuildPlugin(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected BuildPlugin parse error")
	}
	successfulBuildCommand := "true"
	outputBuildCommand := "printf ok > output"
	if runtime.GOOS == "windows" {
		successfulBuildCommand = "exit /b 0"
		outputBuildCommand = "echo ok>output"
	}
	writePluginManifest(t, noBuild, Manifest{
		Name: "plain-plugin", Version: "1.0.0",
		Build: &BuildConfig{Command: outputBuildCommand, Output: "output"},
	})
	if err := BuildPlugin(noBuild); err != nil {
		t.Fatal(err)
	}

	absoluteOutput := filepath.Join(t.TempDir(), "x")
	for name, build := range map[string]*BuildConfig{
		"empty command":   {},
		"absolute output": {Command: successfulBuildCommand, Output: absoluteOutput},
		"escaping output": {Command: successfulBuildCommand, Output: "../x"},
		"missing output":  {Command: successfulBuildCommand, Output: "missing"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := runBuild(t.TempDir(), build); err == nil {
				t.Fatal("expected build error")
			}
		})
	}
	if got := buildCommandFor("windows", "echo ok"); len(got.Args) < 2 || got.Args[0] != "cmd" {
		t.Fatalf("windows build command = %#v", got.Args)
	}
	if got := buildCommandFor("linux", "true"); len(got.Args) < 2 || got.Args[0] != "sh" {
		t.Fatalf("unix build command = %#v", got.Args)
	}
	bin := buildFakeGit(t)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DWS_PLUGIN_TEST_GIT_MODE", "")
	gitPlugin, err := l.InstallFromGit("https://example.test/acme/repository.git")
	if err != nil {
		t.Fatalf("InstallFromGit: %v", err)
	}
	if gitPlugin.Root != filepath.Join(root, "user", "acme", "git-plugin") {
		t.Fatalf("git plugin root = %q", gitPlugin.Root)
	}
	if _, err := os.Stat(filepath.Join(gitPlugin.Root, ".git")); !os.IsNotExist(err) {
		t.Fatalf(".git was copied: %v", err)
	}
	if _, err := l.InstallFromGit("file:///tmp/plugin"); err == nil {
		t.Fatal("expected local git URL rejection")
	}

	t.Setenv("DWS_PLUGIN_TEST_GIT_MODE", "clone-fail")
	if _, err := l.InstallFromGit("https://example.test/acme/fail.git"); err == nil {
		t.Fatal("expected git clone failure")
	}
}

func TestCrossPlatformCoverageLoaderSystemCallEdges(t *testing.T) {
	oldReadDir := pluginReadDir
	oldMkdirTemp := pluginMkdirTemp
	oldRemoveAll := pluginRemoveAll
	oldMkdirAll := pluginMkdirAll
	oldReadFile := pluginReadFile
	oldWriteFile := pluginWriteFile
	oldRemove := pluginRemove
	oldWalk := pluginWalk
	oldRel := pluginRel
	oldCopyDir := pluginCopyDir
	oldRunBuild := pluginRunBuild
	t.Cleanup(func() {
		pluginReadDir = oldReadDir
		pluginMkdirTemp = oldMkdirTemp
		pluginRemoveAll = oldRemoveAll
		pluginMkdirAll = oldMkdirAll
		pluginReadFile = oldReadFile
		pluginWriteFile = oldWriteFile
		pluginRemove = oldRemove
		pluginWalk = oldWalk
		pluginRel = oldRel
		pluginCopyDir = oldCopyDir
		pluginRunBuild = oldRunBuild
	})

	root := t.TempDir()
	l := &Loader{PluginsDir: root, CLIVersion: "1.0.0"}
	pluginReadDir = func(string) ([]os.DirEntry, error) { return nil, errors.New("read dir") }
	if got := l.LoadUser(); got != nil {
		t.Fatalf("LoadUser read error = %#v", got)
	}
	pluginReadDir = oldReadDir

	workspace := filepath.Join(root, "user", "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	pluginReadDir = func(path string) ([]os.DirEntry, error) {
		if path == workspace {
			return nil, errors.New("workspace read")
		}
		return oldReadDir(path)
	}
	if got := l.LoadUser(); len(got) != 0 {
		t.Fatalf("LoadUser workspace read error = %#v", got)
	}
	var infos []PluginInfo
	l.collectUserPluginInfos(workspace, "workspace", &Settings{}, &infos)
	pluginReadDir = oldReadDir

	src := t.TempDir()
	writePluginManifest(t, src, Manifest{Name: "copy-plugin", Version: "1.0.0"})
	pluginCopyDir = func(string, string) error { return errors.New("copy") }
	if _, err := l.InstallFromDir(src); err == nil {
		t.Fatal("expected install copy error")
	}
	pluginCopyDir = oldCopyDir

	writePluginManifest(t, filepath.Join(root, "user", "remove-plugin"), Manifest{Name: "remove-plugin", Version: "1.0.0"})
	pluginRemoveAll = func(string) error { return errors.New("remove") }
	if err := l.RemovePlugin("remove-plugin", false); err == nil {
		t.Fatal("expected remove error")
	}
	pluginRemoveAll = oldRemoveAll
	l.purgePluginFromSettings("not-present")

	l.saveSettings(&Settings{PluginConfigs: map[string]map[string]any{"bad": {"value": func() {}}}})
	l.saveSettings(&Settings{PluginConfigs: map[string]map[string]any{"plug": {"present": "x"}}})
	if l.UnsetPluginConfig("missing", "present") {
		t.Fatal("unset missing plugin succeeded")
	}
	if l.UnsetPluginConfig("plug", "missing") {
		t.Fatal("unset missing key succeeded")
	}

	skillRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(skillRoot, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldHome := pluginUserHomeDir
	t.Cleanup(func() { pluginUserHomeDir = oldHome })
	home := t.TempDir()
	pluginUserHomeDir = func() (string, error) { return home, nil }
	pluginReadDir = func(string) ([]os.DirEntry, error) { return nil, errors.New("skills read") }
	SyncSkills([]*Plugin{{Manifest: Manifest{Name: "skill-plugin"}, Root: skillRoot}})
	pluginReadDir = oldReadDir

	pluginMkdirTemp = func(string, string) (string, error) { return "", errors.New("temp") }
	if _, err := l.InstallFromGit("https://host/acme/plugin.git"); err == nil {
		t.Fatal("expected temp dir error")
	}
	pluginMkdirTemp = oldMkdirTemp
}

func TestCrossPlatformCoverageInstallFromGitValidationAndBuildEdges(t *testing.T) {
	oldCopyDir := pluginCopyDir
	oldRunBuild := pluginRunBuild
	t.Cleanup(func() {
		pluginCopyDir = oldCopyDir
		pluginRunBuild = oldRunBuild
	})
	bin := buildFakeGit(t)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	l := &Loader{PluginsDir: root, CLIVersion: "1.0.0"}
	for _, mode := range []string{"missing", "malformed", "invalid"} {
		t.Setenv("DWS_PLUGIN_TEST_GIT_MODE", mode)
		if _, err := l.InstallFromGit("https://host/acme/repo.git"); err == nil {
			t.Errorf("mode %q unexpectedly succeeded", mode)
		}
	}
	t.Setenv("DWS_PLUGIN_TEST_GIT_MODE", "valid")
	pluginCopyDir = func(string, string) error { return errors.New("copy") }
	if _, err := l.InstallFromGit("https://host/acme/repo.git"); err == nil {
		t.Fatal("expected git install copy error")
	}
	pluginCopyDir = oldCopyDir

	t.Setenv("DWS_PLUGIN_TEST_GIT_MODE", "build")
	pluginRunBuild = func(string, *BuildConfig) error { return errors.New("build") }
	if _, err := l.InstallFromGit("https://host/acme/repo.git"); err == nil {
		t.Fatal("expected git install build error")
	}
}

func TestCrossPlatformCoverageParseGitURLEdges(t *testing.T) {
	for _, raw := range []string{"/tmp/x", "./x", "git@host", "git@host:single", "ftp://host/org/repo", "://bad"} {
		if _, _, err := parseGitURL(raw); err == nil {
			t.Errorf("parseGitURL(%q) unexpectedly succeeded", raw)
		}
	}
	if ws, repo, err := parseGitURL(" http://host/org/repo/ "); err != nil || ws != "org" || repo != "repo" {
		t.Fatalf("trimmed HTTP URL = %q/%q, %v", ws, repo, err)
	}
}

func TestCrossPlatformCoverageSyncSkillsEdges(t *testing.T) {
	oldHome := pluginUserHomeDir
	t.Cleanup(func() { pluginUserHomeDir = oldHome })
	SyncSkills(nil)
	pluginUserHomeDir = func() (string, error) { return "", errors.New("no home") }
	SyncSkills([]*Plugin{{Manifest: Manifest{Name: "plugin-one"}, Root: t.TempDir()}})

	home := t.TempDir()
	pluginUserHomeDir = func() (string, error) { return home, nil }
	if err := os.Mkdir(filepath.Join(home, ".agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	skills := filepath.Join(root, "skills")
	nested := filepath.Join(skills, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skills, "SKILL.md"), []byte("root"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "child.md"), []byte("child"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := &Plugin{Manifest: Manifest{Name: "missing-plugin"}, Root: t.TempDir()}
	p := &Plugin{Manifest: Manifest{Name: "plugin-one"}, Root: root}
	SyncSkills([]*Plugin{missing, p})
	base := filepath.Join(home, ".agents", "skills", "dws", "plugins", "plugin-one")
	for path, want := range map[string]string{
		filepath.Join(base, "SKILL.md"):           "root",
		filepath.Join(base, "nested", "child.md"): "child",
	} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatalf("synced %q = %q, %v", path, data, err)
		}
	}
}

func TestCrossPlatformCoverageCopyAndStaleEdges(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "nested"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "same"), []byte("same"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "same"), []byte("same"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "nested", "new"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(src, "nested", "new"), filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}
	if err := copyDir(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dst, "link")); !os.IsNotExist(err) {
		t.Fatalf("symlink copied: %v", err)
	}
	staleFile := filepath.Join(dst, "stale")
	staleDir := filepath.Join(dst, "stale-dir")
	if err := os.WriteFile(staleFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	removeStaleFiles(src, dst)
	if _, err := os.Stat(staleFile); !os.IsNotExist(err) {
		t.Fatalf("stale file remains: %v", err)
	}
	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Fatalf("stale dir remains: %v", err)
	}
	if err := copyDir(filepath.Join(src, "missing"), filepath.Join(t.TempDir(), "dest")); err == nil {
		t.Fatal("expected missing source error")
	}
	fileDst := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(fileDst, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyDir(src, fileDst); err == nil {
		t.Fatal("expected destination mkdir error")
	}
	removeStaleFiles(filepath.Join(src, "missing"), filepath.Join(dst, "missing"))
}

func TestCrossPlatformCoverageCopyAndStaleSystemCallEdges(t *testing.T) {
	oldMkdirAll := pluginMkdirAll
	oldReadFile := pluginReadFile
	oldWriteFile := pluginWriteFile
	oldRemove := pluginRemove
	oldWalk := pluginWalk
	oldRel := pluginRel
	t.Cleanup(func() {
		pluginMkdirAll = oldMkdirAll
		pluginReadFile = oldReadFile
		pluginWriteFile = oldWriteFile
		pluginRemove = oldRemove
		pluginWalk = oldWalk
		pluginRel = oldRel
	})

	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")
	file := filepath.Join(src, "file")
	if err := os.WriteFile(file, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}

	pluginWalk = func(string, filepath.WalkFunc) error { return errors.New("walk") }
	if err := copyDir(src, dst); err == nil {
		t.Fatal("expected walk error")
	}
	pluginWalk = func(_ string, fn filepath.WalkFunc) error {
		return fn(file, info, errors.New("visit"))
	}
	if err := copyDir(src, dst); err == nil {
		t.Fatal("expected visit error")
	}
	pluginWalk = func(_ string, fn filepath.WalkFunc) error { return fn(file, info, nil) }
	pluginRel = func(string, string) (string, error) { return "", errors.New("rel") }
	if err := copyDir(src, dst); err == nil {
		t.Fatal("expected rel error")
	}
	pluginRel = func(string, string) (string, error) { return "../escape", nil }
	if err := copyDir(src, dst); err == nil {
		t.Fatal("expected traversal error")
	}
	pluginRel = oldRel
	pluginReadFile = func(string) ([]byte, error) { return nil, errors.New("read") }
	if err := copyDir(src, dst); err == nil {
		t.Fatal("expected source read error")
	}
	pluginReadFile = oldReadFile
	pluginWriteFile = func(string, []byte, os.FileMode) error { return errors.New("write") }
	if err := copyDir(src, dst); err == nil {
		t.Fatal("expected destination write error")
	}
	pluginWriteFile = oldWriteFile
	pluginWalk = oldWalk

	pluginMkdirAll = func(string, os.FileMode) error { return errors.New("mkdir") }
	if err := copyDir(src, dst); err == nil {
		t.Fatal("expected initial mkdir error")
	}
	pluginMkdirAll = oldMkdirAll

	removeStaleFiles(src, t.TempDir())
	pluginWalk = func(root string, fn filepath.WalkFunc) error {
		return fn(root, nil, errors.New("walk item"))
	}
	removeStaleFiles(src, dst)

	call := 0
	pluginWalk = func(root string, fn filepath.WalkFunc) error {
		call++
		rootInfo, statErr := os.Stat(root)
		if statErr != nil {
			return statErr
		}
		return fn(root, rootInfo, nil)
	}
	pluginRel = func(string, string) (string, error) { return "", errors.New("rel") }
	removeStaleFiles(src, dst)
	if call != 2 {
		t.Fatalf("walk calls = %d", call)
	}

	pluginWalk = oldWalk
	pluginRel = oldRel
	staleDst := t.TempDir()
	stale := filepath.Join(staleDst, "stale")
	if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	pluginRemove = func(string) error { return errors.New("remove") }
	removeStaleFiles(src, staleDst)
}

func TestCrossPlatformCoverageCopyAndStaleSystemCallEdgesDuplicate(t *testing.T) {
	oldMkdirAll := pluginMkdirAll
	oldReadFile := pluginReadFile
	oldWriteFile := pluginWriteFile
	oldRemove := pluginRemove
	oldWalk := pluginWalk
	oldRel := pluginRel
	t.Cleanup(func() {
		pluginMkdirAll = oldMkdirAll
		pluginReadFile = oldReadFile
		pluginWriteFile = oldWriteFile
		pluginRemove = oldRemove
		pluginWalk = oldWalk
		pluginRel = oldRel
	})

	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")
	file := filepath.Join(src, "file")
	if err := os.WriteFile(file, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}

	pluginWalk = func(string, filepath.WalkFunc) error { return errors.New("walk") }
	if err := copyDir(src, dst); err == nil {
		t.Fatal("expected walk error")
	}
	pluginWalk = func(_ string, fn filepath.WalkFunc) error {
		return fn(file, info, errors.New("visit"))
	}
	if err := copyDir(src, dst); err == nil {
		t.Fatal("expected visit error")
	}
	pluginWalk = func(_ string, fn filepath.WalkFunc) error { return fn(file, info, nil) }
	pluginRel = func(string, string) (string, error) { return "", errors.New("rel") }
	if err := copyDir(src, dst); err == nil {
		t.Fatal("expected rel error")
	}
	pluginRel = func(string, string) (string, error) { return "../escape", nil }
	if err := copyDir(src, dst); err == nil {
		t.Fatal("expected traversal error")
	}
	pluginRel = oldRel
	pluginReadFile = func(string) ([]byte, error) { return nil, errors.New("read") }
	if err := copyDir(src, dst); err == nil {
		t.Fatal("expected source read error")
	}
	pluginReadFile = oldReadFile
	pluginWriteFile = func(string, []byte, os.FileMode) error { return errors.New("write") }
	if err := copyDir(src, dst); err == nil {
		t.Fatal("expected destination write error")
	}
	pluginWriteFile = oldWriteFile
	pluginWalk = oldWalk

	pluginMkdirAll = func(string, os.FileMode) error { return errors.New("mkdir") }
	if err := copyDir(src, dst); err == nil {
		t.Fatal("expected initial mkdir error")
	}
	pluginMkdirAll = oldMkdirAll

	removeStaleFiles(src, t.TempDir())
	pluginWalk = func(root string, fn filepath.WalkFunc) error {
		return fn(root, nil, errors.New("walk item"))
	}
	removeStaleFiles(src, dst)

	call := 0
	pluginWalk = func(root string, fn filepath.WalkFunc) error {
		call++
		rootInfo, statErr := os.Stat(root)
		if statErr != nil {
			return statErr
		}
		return fn(root, rootInfo, nil)
	}
	pluginRel = func(string, string) (string, error) { return "", errors.New("rel") }
	removeStaleFiles(src, dst)
	if call != 2 {
		t.Fatalf("walk calls = %d", call)
	}

	pluginWalk = oldWalk
	pluginRel = oldRel
	staleDst := t.TempDir()
	stale := filepath.Join(staleDst, "stale")
	if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	pluginRemove = func(string) error { return errors.New("remove") }
	removeStaleFiles(src, staleDst)
}
