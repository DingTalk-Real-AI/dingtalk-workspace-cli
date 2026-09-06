package launcher

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/clitelemetry"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/roothelp"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
)

func helpFixture(t *testing.T) (dependencies, Options, roothelp.Snapshot) {
	t.Helper()
	deps, options, _, _, _, _ := schemaFixture(t)
	options.Commit = strings.Repeat("a", 40)
	model := roothelp.Model{Services: []roothelp.Command{{Name: "calendar", Short: "calendar"}}, Utilities: []roothelp.Command{{Name: "help", Short: "help"}}, Flags: []roothelp.Flag{{Label: "--format string", Usage: "format"}}, Long: "guidance"}
	snapshot := roothelp.Snapshot{Version: 1, Edition: options.Edition, Commit: options.Commit, CoreSHA256: options.CoreSHA256, English: model, Chinese: model}
	snapshot.Chinese.Long = "中文提示"
	var err error
	options.HelpSnapshot, err = roothelp.EncodeSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	deps.args = []string{"dws", "--help"}
	deps.environ = deps.environ[1:]
	return deps, options, snapshot
}
func TestCrossPlatformCoverageTrackedHelpUsesBoundProjection(t *testing.T) {
	for _, locale := range []string{"en_US.UTF-8", "zh_CN.UTF-8"} {
		for _, optOut := range []bool{false, true} {
			t.Run(locale+"/"+map[bool]string{true: "opt-out", false: "default"}[optOut], func(t *testing.T) {
				deps, options, snapshot := helpFixture(t)
				deps.environ = append(deps.environ, "LANG="+locale)
				if optOut {
					deps.environ = append(deps.environ, "DO_NOT_TRACK=1")
				}
				var stdout, want bytes.Buffer
				deps.stdout = &stdout
				model := snapshot.English
				if strings.HasPrefix(locale, "zh") {
					model = snapshot.Chinese
				}
				roothelp.Render(&want, model)
				deps.executable = func() (string, error) { t.Fatal("help located core"); return "", nil }
				deps.openSchemaCache = func(string) (*schemacache.Cache, error) { t.Fatal("help opened cache"); return nil, nil }
				reads, events := 0, 0
				deps.defaultIdentity = func(string) clitelemetry.Identity {
					reads++
					return clitelemetry.Identity{UserID: "user", CorpID: "corp"}
				}
				deps.trackRun = func(cfg clitelemetry.Config, execute func() error, exitCode func(error) int) {
					events++
					if optOut && cfg.PID != "" || !optOut && cfg.PID != "wcCRwZ" {
						t.Fatal("incorrect opt-out behavior")
					}
					if err := execute(); err != nil || exitCode(err) != 0 {
						t.Fatalf("help execution: %v", err)
					}
					fields := map[string]string{"c9": "dws"}
					if !optOut {
						fields["c10"] = "corp"
					}
					if !reflect.DeepEqual(fields, cfg.ExtraFields()) {
						t.Fatalf("help event: %#v", cfg.ExtraFields())
					}
				}
				if err := run(options, deps); err != nil || stdout.String() != want.String() || events != 1 {
					t.Fatalf("help result: %v, events %d", err, events)
				}
				expectedReads := 1
				if optOut {
					expectedReads = 0
				}
				if reads != expectedReads {
					t.Fatalf("identity reads %d", reads)
				}
			})
		}
	}
}
func TestCrossPlatformCoverageHelpUncertainInputsDelegate(t *testing.T) {
	for _, condition := range []string{"missing", "unknown flag", "settings", "diagnostics"} {
		t.Run(condition, func(t *testing.T) {
			deps, options, _ := helpFixture(t)
			switch condition {
			case "missing":
				options.HelpSnapshot = ""
			case "unknown flag":
				deps.args = append(deps.args, "--unknown")
			case "settings":
				dir := environmentValue(deps.environ, "DWS_CONFIG_DIR")
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			case "diagnostics":
				deps.environ = append(deps.environ, "DWS_AGENT_EXT=invalid")
			}
			deps.defaultIdentity = func(string) clitelemetry.Identity { t.Fatal("fallback read identity"); return clitelemetry.Identity{} }
			deps.trackRun = func(clitelemetry.Config, func() error, func(error) int) { t.Fatal("fallback emitted an event") }
			delegated := false
			deps.delegate = func(_ string, args, _ []string, _ string, _ io.Reader, _, _ io.Writer) (int, error) {
				delegated = true
				if !reflect.DeepEqual(args, deps.args) {
					t.Fatal("changed fallback argv")
				}
				return 0, nil
			}
			if err := run(options, deps); err != nil || !delegated {
				t.Fatalf("fallback: %v %v", err, delegated)
			}
		})
	}
}

func TestCrossPlatformCoverageInvalidSealedHelpFailsClosed(t *testing.T) {
	for _, condition := range []string{"malformed", "stale"} {
		t.Run(condition, func(t *testing.T) {
			deps, options, _ := helpFixture(t)
			var stdout bytes.Buffer
			deps.stdout = &stdout
			if condition == "malformed" {
				options.HelpSnapshot = "not-base64"
			} else {
				options.Commit = strings.Repeat("c", 40)
			}
			deps.delegate = func(string, []string, []string, string, io.Reader, io.Writer, io.Writer) (int, error) {
				t.Fatal("invalid sealed help delegated")
				return 0, nil
			}
			deps.defaultIdentity = func(string) clitelemetry.Identity {
				t.Fatal("invalid sealed help read tracker identity")
				return clitelemetry.Identity{}
			}
			deps.trackRun = func(clitelemetry.Config, func() error, func(error) int) {
				t.Fatal("invalid sealed help emitted tracker event")
			}
			err := run(options, deps)
			var launcherErr *Error
			if !errors.As(err, &launcherErr) || launcherErr.Kind != ErrorArtifact || launcherErr.Op != "validate sealed help" {
				t.Fatalf("invalid help error = %#v", err)
			}
			expected := errInvalidHelpSnapshot
			if condition == "stale" {
				expected = errHelpSnapshotMismatch
			}
			if !errors.Is(err, expected) || err.Error() != "artifact validate sealed help: "+expected.Error() {
				t.Fatalf("invalid help classification = %q", err)
			}
			if stdout.Len() != 0 {
				t.Fatalf("invalid help emitted %q", stdout.String())
			}
		})
	}
}
func TestCrossPlatformCoverageHelpPreservesLegacyWriterFailure(t *testing.T) {
	deps, options, _ := helpFixture(t)
	deps.stdout = trackedVersionFailWriter{errors.New("write failed")}
	deps.defaultIdentity = func(string) clitelemetry.Identity { return clitelemetry.Identity{} }
	deps.executable = func() (string, error) { t.Fatal("help retried after publication"); return "", nil }
	deps.trackRun = func(cfg clitelemetry.Config, execute func() error, exitCode func(error) int) {
		if err := execute(); err != nil || exitCode(err) != 0 || cfg.ExtraFields()["c5"] != "" {
			t.Fatalf("changed legacy HelpFunc write-error behavior: %v", err)
		}
	}
	if err := run(options, deps); err != nil {
		t.Fatal(err)
	}
}
