package launcher

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/clisignal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/clitelemetry"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
)

func TestCrossPlatformCoverageVersionUsesSharedTrackerWithoutCoreOrCache(t *testing.T) {
	deps, options, _, _, _, _ := schemaFixture(t)
	deps.args = []string{"dws", "--version"}
	deps.environ = deps.environ[1:] // Keep default tracking enabled.
	var stdout bytes.Buffer
	deps.stdout = &stdout
	deps.executable = func() (string, error) { t.Fatal("version located core"); return "", nil }
	deps.openSchemaCache = func(string) (*schemacache.Cache, error) { t.Fatal("version opened Schema cache"); return nil, nil }
	reads, events := 0, 0
	identity := clitelemetry.Identity{UserID: "user", UserName: "Alice", CorpID: "corp"}
	deps.defaultIdentity = func(directory string) clitelemetry.Identity {
		reads++
		if directory != environmentValue(deps.environ, "DWS_CONFIG_DIR") || stdout.Len() != 0 {
			t.Fatal("identity was not captured before version output")
		}
		return identity
	}
	deps.trackRun = func(cfg clitelemetry.Config, execute func() error, exitCode func(error) int) {
		events++
		if reads != 1 || cfg.PID != "wcCRwZ" || cfg.App != "dws" || cfg.Version != options.Version || cfg.UID != identity.UserID || cfg.Username != identity.UserName {
			t.Fatalf("tracker identity changed: %#v", cfg)
		}
		if !cfg.NoCommandLine || !cfg.NoCwd || !cfg.NoAutomaticDimensions || cfg.CaptureOutput || cfg.FlushTimeout != 0 || cfg.Endpoint != "" || cfg.EventID != "" || cfg.Env != "" {
			t.Fatalf("tracker privacy or SDK defaults changed: %#v", cfg)
		}
		if err := execute(); err != nil || exitCode(err) != 0 {
			t.Fatalf("version execution: %v", err)
		}
		if fields := cfg.ExtraFields(); !reflect.DeepEqual(fields, map[string]string{"c9": "dws", "c10": "corp"}) {
			t.Fatalf("root version event fields: %#v", fields)
		}
	}
	if err := run(options, deps); err != nil || reads != 1 || events != 1 || stdout.String() != "dws version 1.2.3 (abcdef, unknown)\n" {
		t.Fatalf("tracked version: %v, reads %d, events %d, output %q", err, reads, events, stdout.String())
	}
}

func TestCrossPlatformCoverageTrackedVersionPreservesOutputError(t *testing.T) {
	deps, options, _, _, _, _ := schemaFixture(t)
	deps.args, deps.environ = []string{"dws", "--version"}, deps.environ[1:]
	failure := fmt.Errorf("write failed for /private/customer/output --access-token secret-value")
	deps.stdout = trackedVersionFailWriter{failure}
	var stderr, expected bytes.Buffer
	deps.stderr = &stderr
	_ = apperrors.PrintHumanAt(&expected, failure, apperrors.VerbosityNormal)
	deps.executable = func() (string, error) { t.Fatal("failed version delegated after publication"); return "", nil }
	deps.defaultIdentity = func(string) clitelemetry.Identity { return clitelemetry.Identity{} }
	events := 0
	deps.trackRun = func(cfg clitelemetry.Config, execute func() error, exitCode func(error) int) {
		events++
		err := execute()
		if err == nil || err.Error() != "" || exitCode(err) != apperrors.ExitCode(failure) {
			t.Fatalf("tracker error/exit: %v, %d", err, exitCode(err))
		}
		if fields := cfg.ExtraFields(); fields["c5"] != clitelemetry.ErrorSummary(failure) || fields["c9"] != "dws" {
			t.Fatalf("error telemetry: %#v", fields)
		}
	}
	var exit *ExitError
	if err := run(options, deps); !errors.As(err, &exit) || exit.Code != apperrors.ExitCode(failure) || stderr.String() != expected.String() || events != 1 {
		t.Fatalf("output error behavior: %v, stderr %q, events %d", err, stderr.String(), events)
	}
}

type trackedVersionFailWriter struct{ error }

func (w trackedVersionFailWriter) Write([]byte) (int, error) { return 0, w.error }

type trackedVersionWriter func([]byte) (int, error)

func (w trackedVersionWriter) Write(data []byte) (int, error) { return w(data) }

func TestCrossPlatformCoverageTrackedPresentationSignalAndPanicLifecycle(t *testing.T) {
	for _, command := range []string{"--version", "--help"} {
		t.Run(command, func(t *testing.T) {
			for _, failure := range []string{"interrupt", "terminate", "panic"} {
				t.Run(failure, func(t *testing.T) {
					deps, options, _ := helpFixture(t)
					deps.args = []string{"dws", command}
					state, stopped := &clisignal.State{}, false
					deps.presentationSignals = func() (*clisignal.State, func()) {
						return state, func() { stopped = true }
					}
					var stderr bytes.Buffer
					deps.stderr = &stderr
					deps.defaultIdentity = func(string) clitelemetry.Identity { return clitelemetry.Identity{} }
					deps.executable = func() (string, error) { t.Fatal("presentation failure delegated"); return "", nil }
					wantCode, wantSummary := 130, "process interrupted by interrupt"
					if failure == "terminate" {
						wantCode, wantSummary = 143, "process interrupted by terminated"
					} else if failure == "panic" {
						wantCode, wantSummary = 5, "internal panic"
					}
					deps.stdout = trackedVersionWriter(func(data []byte) (int, error) {
						if failure == "panic" {
							panic("private-token-value")
						}
						sig := os.Interrupt
						if failure == "terminate" {
							sig = syscall.SIGTERM
						}
						state.Record(sig, nil)
						return len(data), nil
					})
					events := 0
					deps.trackRun = func(cfg clitelemetry.Config, execute func() error, exitCode func(error) int) {
						events++
						err := execute()
						if err == nil || err.Error() != "" || exitCode(err) != wantCode || !stopped {
							t.Fatalf("tracker completed before cleanup or lost error: %v, code %d, stopped %v", err, exitCode(err), stopped)
						}
						if fields := cfg.ExtraFields(); !reflect.DeepEqual(fields, map[string]string{"c9": "dws", "c5": wantSummary}) {
							t.Fatalf("unsafe failure telemetry: %#v", fields)
						}
					}
					var exit *ExitError
					if err := run(options, deps); !errors.As(err, &exit) || exit.Code != wantCode || events != 1 || stderr.Len() == 0 {
						t.Fatalf("failure lifecycle: %v, events %d, stderr %q", err, events, stderr.String())
					}
				})
			}
		})
	}
}

func TestCrossPlatformCoverageVersionDiagnosticsStillDelegate(t *testing.T) {
	for _, condition := range []string{"agent metadata", "settings", "no proof", "profile flag"} {
		t.Run(condition, func(t *testing.T) {
			deps, options, _, _, _, _ := schemaFixture(t)
			deps.args, deps.environ = []string{"dws", "--version"}, deps.environ[1:]
			switch condition {
			case "agent metadata":
				deps.environ = append(deps.environ, "DWS_AGENT_EXT=invalid")
			case "settings":
				directory := environmentValue(deps.environ, "DWS_CONFIG_DIR")
				if err := os.MkdirAll(directory, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(directory, "settings.json"), []byte("{"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "no proof":
				options.SchemaIdentity = nil
			case "profile flag":
				deps.args = append(deps.args, "--profile", "corp-a")
			}
			deps.defaultIdentity = func(string) clitelemetry.Identity {
				t.Fatal("fallback read identity in launcher")
				return clitelemetry.Identity{}
			}
			deps.trackRun = func(clitelemetry.Config, func() error, func(error) int) {
				t.Fatal("fallback emitted duplicate tracker event")
			}
			delegated := false
			deps.delegate = func(_ string, args, _ []string, _ string, _ io.Reader, _, _ io.Writer) (int, error) {
				delegated = true
				if !reflect.DeepEqual(args, deps.args) {
					t.Fatal("fallback changed argv")
				}
				return 0, nil
			}
			if err := run(options, deps); err != nil || !delegated {
				t.Fatalf("version fallback: %v, delegated %v", err, delegated)
			}
		})
	}
}
