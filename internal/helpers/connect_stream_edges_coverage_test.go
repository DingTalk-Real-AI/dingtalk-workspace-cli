package helpers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCrossPlatformCoverageConnectStreamFinalStatementEdges(t *testing.T) {
	if got := newConnectMediaClient("client", "secret"); got == nil {
		t.Fatal("default media client is nil")
	}

	t.Run("backend error clears an addressable session", func(t *testing.T) {
		sessions := newConvSessions("")
		oldID := sessions.id("conversation")
		bin := writeShellExecutable(t, t.TempDir(), "backend-agent", "printf 'API Error: backend rejected request'\n")
		fwd := &execForwarder{
			name:     "test",
			argv:     []string{bin},
			sessions: sessions,
		}
		if reply, err := fwd.forward(context.Background(), "conversation", "question"); err != nil || reply == "" {
			t.Fatalf("backend reply = %q, %v", reply, err)
		}
		if newID := sessions.id("conversation"); newID == oldID {
			t.Fatal("backend error did not clear the session")
		}
	})

	t.Run("successful command with empty stdout", func(t *testing.T) {
		bin := writeShellExecutable(t, t.TempDir(), "silent-agent", "exit 0\n")
		fwd := &execForwarder{name: "test", argv: []string{bin}}
		if reply, err := fwd.forward(context.Background(), "", "question"); err != nil || reply != "（本地 agent 无文本输出）" {
			t.Fatalf("empty stdout reply = %q, %v", reply, err)
		}
	})

	t.Run("blank Claude settings key is ignored", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", dir)
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"env":{" ":"ignored"}}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := claudeUserSettingsEnv(); len(got) != 0 {
			t.Fatalf("blank settings key produced env: %v", got)
		}
	})
}

func TestCrossPlatformCoverageResolveExecAgentInstallAndForwarderErrors(t *testing.T) {
	clearChannelEnv(t)
	binDir := t.TempDir()
	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath)

	restoreSpec := func(channel string, spec agentSpec) {
		t.Helper()
		original, existed := agentSpecs[channel]
		agentSpecs[channel] = spec
		t.Cleanup(func() {
			if existed {
				agentSpecs[channel] = original
			} else {
				delete(agentSpecs, channel)
			}
		})
	}

	source := writeShellExecutable(t, t.TempDir(), "source-agent", "exit 0\n")
	target := testExecutablePath(binDir, "installed-agent")
	restoreSpec("install-success", agentSpec{
		app:     "Install Success",
		bins:    []string{"installed-agent"},
		install: []string{"cp", source, target},
	})
	argv, _, err := resolveExecAgent("install-success")
	if err != nil || len(argv) != 1 || argv[0] != target {
		t.Fatalf("installed agent = %v, %v", argv, err)
	}

	restoreSpec("install-failure", agentSpec{
		app:     "Install Failure",
		bins:    []string{"never-installed-agent"},
		install: []string{"sh", "-c", "exit 1"},
		hint:    "install it",
	})
	if _, _, err := resolveExecAgent("install-failure"); err == nil {
		t.Fatal("failed installation unexpectedly resolved an agent")
	}
	if _, err := forwarderForChannel("install-failure", "", connectAgentOptions{}); err == nil {
		t.Fatal("forwarder unexpectedly accepted a missing binary")
	}

	writeShellExecutable(t, binDir, "model-less-agent", "exit 0\n")
	restoreSpec("model-less", agentSpec{app: "Model-less", bins: []string{"model-less-agent"}})
	if _, err := forwarderForChannel("model-less", "", connectAgentOptions{Model: "unsupported"}); err == nil {
		t.Fatal("model override unexpectedly succeeded without a model flag")
	}
}
