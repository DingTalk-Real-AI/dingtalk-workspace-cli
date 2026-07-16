package app

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/plugin"
	"github.com/spf13/cobra"
)

func pluginCoverageRun(cmd *cobra.Command, args ...string) (string, error) {
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(io.Discard)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestCrossPlatformCoveragePluginCommandRemainingCoverage(t *testing.T) {
	oldGit := pluginInstallFromGit
	oldStat := pluginStat
	oldMkdir := pluginMkdirAll
	oldWrite := pluginWriteFile
	oldAbs := pluginAbs
	oldRegister := pluginRegisterDev
	oldBuild := pluginBuild
	oldList, oldParse := pluginListInstalled, pluginParseManifest
	t.Cleanup(func() {
		pluginInstallFromGit = oldGit
		pluginStat = oldStat
		pluginMkdirAll = oldMkdir
		pluginWriteFile = oldWrite
		pluginAbs = oldAbs
		pluginRegisterDev = oldRegister
		pluginBuild = oldBuild
		pluginListInstalled, pluginParseManifest = oldList, oldParse
	})
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	fail := errors.New("failure")
	pluginListInstalled = func(*plugin.Loader) []plugin.PluginInfo {
		return []plugin.PluginInfo{{Name: "broken", Path: "missing"}}
	}
	pluginParseManifest = func(string) (*plugin.Manifest, error) { return nil, fail }
	if got := loadDeclaredUserConfig(plugin.NewLoader(RawVersion()), "broken"); got != nil {
		t.Fatalf("broken declared config = %#v", got)
	}
	pluginListInstalled, pluginParseManifest = oldList, oldParse

	pluginInstallFromGit = func(*plugin.Loader, string) (*plugin.Plugin, error) { return nil, fail }
	if _, err := pluginCoverageRun(newPluginInstallCommand(), "--git", "https://example.test/org/plugin.git"); err == nil {
		t.Fatal("git install failure should propagate")
	}
	pluginInstallFromGit = func(*plugin.Loader, string) (*plugin.Plugin, error) {
		return &plugin.Plugin{Manifest: plugin.Manifest{Name: "git-plugin", Version: "1.0.0"}}, nil
	}
	if out, err := pluginCoverageRun(newPluginInstallCommand(), "--git", "https://example.test/org/plugin.git"); err != nil || !strings.Contains(out, "git-plugin") {
		t.Fatalf("git install = %q, %v", out, err)
	}
	if _, err := pluginCoverageRun(newPluginDisableCommand(), "missing"); err == nil {
		t.Fatal("disable missing plugin should fail")
	}

	invalidDir := filepath.Join(work, "invalid")
	if err := os.MkdirAll(invalidDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(invalidDir, "plugin.json"), []byte(`{"name":"ok-name","version":"1.0.0","type":"invalid"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := pluginCoverageRun(newPluginValidateCommand(), invalidDir); err == nil {
		t.Fatal("invalid manifest validation should fail")
	}

	pluginStat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	pluginMkdirAll = func(string, os.FileMode) error { return fail }
	if _, err := pluginCoverageRun(newPluginCreateCommand(), "mkdir-plugin"); err == nil {
		t.Fatal("scaffold mkdir failure should propagate")
	}
	pluginMkdirAll = oldMkdir
	for _, target := range []string{"plugin.json", "SKILL.md", "hooks.json"} {
		name := "write-" + strings.ToLower(strings.TrimSuffix(target, filepath.Ext(target)))
		pluginWriteFile = func(path string, data []byte, mode os.FileMode) error {
			if strings.HasSuffix(path, target) {
				return fail
			}
			return oldWrite(path, data, mode)
		}
		if _, err := pluginCoverageRun(newPluginCreateCommand(), name); err == nil {
			t.Fatalf("scaffold %s failure should propagate", target)
		}
	}
	pluginWriteFile = oldWrite

	pluginAbs = func(string) (string, error) { return "", fail }
	if _, err := pluginCoverageRun(newPluginDevCommand(), "dir"); err == nil {
		t.Fatal("dev absolute-path failure should propagate")
	}
	pluginAbs = oldAbs
	if _, err := pluginCoverageRun(newPluginDevCommand(), invalidDir); err == nil {
		t.Fatal("dev manifest validation should fail")
	}
	validDir := filepath.Join(work, "valid")
	if err := os.MkdirAll(validDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "name":"valid-plugin","version":"1.0.0","type":"user",
  "userConfig":{
    "API_KEY":{"description":"secret key","sensitive":true},
    "PLAIN":{"description":"plain value","default":"default"},
    "UNSET":{"description":"required value"}
  },
  "build":{"command":"true","output":"bin/server"}
}`
	if err := os.WriteFile(filepath.Join(validDir, "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	pluginRegisterDev = func(*plugin.Loader, string, string) error { return fail }
	if _, err := pluginCoverageRun(newPluginDevCommand(), validDir); err == nil {
		t.Fatal("dev registration failure should propagate")
	}

	loader := plugin.NewLoader(RawVersion())
	installed, err := loader.InstallFromDir(validDir)
	if err != nil {
		// The declared build output is intentionally absent; install a no-build copy.
		noBuild := strings.Replace(manifest, ",\n  \"build\":{\"command\":\"true\",\"output\":\"bin/server\"}", "", 1)
		if writeErr := os.WriteFile(filepath.Join(validDir, "plugin.json"), []byte(noBuild), 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
		installed, err = loader.InstallFromDir(validDir)
	}
	if err != nil {
		t.Fatal(err)
	}
	loader.SetPluginConfig("valid-plugin", "API_KEY", "abcdefghijk")
	loader.SetPluginConfig("valid-plugin", "PLAIN", "value")
	for _, args := range [][]string{{"valid-plugin"}, {"valid-plugin", "--json"}} {
		out, runErr := pluginCoverageRun(newPluginConfigListCommand(), args...)
		if runErr != nil || !strings.Contains(out, "UNSET") || !strings.Contains(out, "abcd") {
			t.Fatalf("config list %#v = %q, %v", args, out, runErr)
		}
	}
	if err := os.WriteFile(filepath.Join(installed.Root, "plugin.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := loadDeclaredUserConfig(loader, "valid-plugin"); got != nil {
		t.Fatalf("corrupt declared config = %#v", got)
	}

	pluginAbs = func(string) (string, error) { return "", fail }
	if _, err := pluginCoverageRun(newPluginBuildCommand(), validDir); err == nil {
		t.Fatal("build absolute-path failure should propagate")
	}
	pluginAbs = oldAbs
	if err := os.WriteFile(filepath.Join(validDir, "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	pluginBuild = func(string) error { return fail }
	if _, err := pluginCoverageRun(newPluginBuildCommand(), validDir); err == nil {
		t.Fatal("plugin build failure should propagate")
	}
}
