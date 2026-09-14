package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"gitlab.alibaba-inc.com/aes/aem-go-sdk/clitrack"
)

func TestCrossPlatformCoverageMainRunsThroughCLITracker(t *testing.T) {
	for _, wantCode := range []int{0, 1, 3, 5} {
		t.Run(fmt.Sprintf("exit_%d", wantCode), func(t *testing.T) {
			t.Setenv("DO_NOT_TRACK", "")
			wantError := ""
			if wantCode != 0 {
				wantError = "synthetic failure"
			}
			testseam.Swap(t, &os.Args, []string{"dws", "sheet", "read", "--profile", "corp-a"})
			testseam.Swap(t, &resolveTelemetryIdentity, func(args []string) app.TelemetryIdentity {
				if strings.Join(args, " ") != "sheet read --profile corp-a" {
					t.Fatalf("telemetry identity args = %#v", args)
				}
				return app.TelemetryIdentity{UserID: "user-1", UserName: "Alice", CorpID: "corp-1"}
			})
			testseam.Swap(t, &appExecute, func() (int, string, string) { return wantCode, "sheet read", wantError })
			called := false
			testseam.Swap(t, &trackRun, func(cfg clitrack.Config, execute func() error, exitCode func(error) int) {
				called = true
				if cfg.PID != "wcCRwZ" || cfg.App != "dws" {
					t.Fatalf("tracker identity = PID %q App %q", cfg.PID, cfg.App)
				}
				if cfg.Version != app.RawVersion() {
					t.Fatalf("tracker Version = %q, want %q", cfg.Version, app.RawVersion())
				}
				if !cfg.NoCommandLine || !cfg.NoCwd || !cfg.NoAutomaticDimensions || cfg.CaptureOutput {
					t.Fatalf("tracker privacy config = NoCommandLine %v NoCwd %v NoAutomaticDimensions %v CaptureOutput %v", cfg.NoCommandLine, cfg.NoCwd, cfg.NoAutomaticDimensions, cfg.CaptureOutput)
				}
				if cfg.Env != "" || cfg.EventID != "" || cfg.Endpoint != "" || cfg.FlushTimeout != 0 || cfg.OutputMaxLen != 0 {
					t.Fatalf("tracker SDK defaults were overridden: %#v", cfg)
				}
				if cfg.UID != "user-1" || cfg.Username != "Alice" || cfg.UserType != "" {
					t.Fatalf("tracker user identity = UID %q Username %q UserType %q", cfg.UID, cfg.Username, cfg.UserType)
				}

				err := execute()
				if wantCode == 0 && err != nil {
					t.Fatalf("successful tracked execute error = %v", err)
				}
				if wantCode != 0 && (err == nil || err.Error() != "") {
					t.Fatalf("failed tracked execute error = %#v, want empty sentinel", err)
				}
				if gotCode := exitCode(err); gotCode != wantCode {
					t.Fatalf("tracked exit code = %d, want %d", gotCode, wantCode)
				}
				fields := cfg.ExtraFields()
				if fields["c9"] != "sheet read" || fields["c10"] != "corp-1" || fields["c5"] != wantError {
					t.Fatalf("tracker extra fields = %#v, want command path, corp ID, and error %q", fields, wantError)
				}
				if (wantError == "" && len(fields) != 2) || (wantError != "" && len(fields) != 3) {
					t.Fatalf("tracker extra field count = %d for error %q", len(fields), wantError)
				}
			})

			main()
			if !called {
				t.Fatalf("trackRun was not called for exit code %d", wantCode)
			}
		})
	}
}

func TestCrossPlatformCoverageTrackerConfigOmitsEmptyOrganization(t *testing.T) {
	commandPath := "version"
	errorMessage := ""
	cfg := trackerConfig(app.TelemetryIdentity{}, &commandPath, &errorMessage)
	if cfg.UID != "" {
		t.Fatalf("empty identity UID = %q", cfg.UID)
	}
	if cfg.Username != "" {
		t.Fatalf("empty identity Username = %q", cfg.Username)
	}
	if fields := cfg.ExtraFields(); len(fields) != 1 || fields["c9"] != "version" {
		t.Fatalf("empty organization fields = %#v", fields)
	}
}

func TestCrossPlatformCoverageDefaultTrackRunNoopTracker(t *testing.T) {
	called := false
	trackRun(clitrack.Config{}, func() error {
		called = true
		return nil
	}, nil)
	if !called {
		t.Fatal("default tracker did not execute callback")
	}
}

func TestCrossPlatformCoverageMainRespectsDoNotTrack(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	testseam.Swap(t, &os.Args, []string{"dws", "version"})
	testseam.Swap(t, &resolveTelemetryIdentity, func([]string) app.TelemetryIdentity {
		t.Fatal("DO_NOT_TRACK must skip telemetry identity reads")
		return app.TelemetryIdentity{}
	})
	testseam.Swap(t, &appExecute, func() (int, string, string) { return 0, "version", "" })
	testseam.Swap(t, &trackRun, func(cfg clitrack.Config, execute func() error, exitCode func(error) int) {
		if cfg.PID != "" || cfg.UID != "" || cfg.Username != "" {
			t.Fatalf("opted-out tracker config = %#v", cfg)
		}
		if err := execute(); err != nil {
			t.Fatalf("opted-out execution failed: %v", err)
		}
		if code := exitCode(nil); code != 0 {
			t.Fatalf("opted-out exit code = %d, want 0", code)
		}
	})

	main()
}

func TestCrossPlatformCoverageTrackerPayloadUsesReviewedFieldWhitelist(t *testing.T) {
	testseam.Protect(t, &os.Args)
	os.Args = []string{"dws", "sheet", "read", "--access-token", "must-not-leak"}
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("TERM_SESSION_ID", "stable-session")
	t.Setenv("TMUX_PANE", "%42")
	t.Setenv("LANG", "zh_CN.UTF-8")
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	t.Chdir(t.TempDir())

	requestBody := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		requestBody <- body
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	commandPath := "sheet read"
	errorMessage := ""
	cfg := trackerConfig(app.TelemetryIdentity{UserID: "user-1", UserName: "Alice", CorpID: "corp-1"}, &commandPath, &errorMessage)
	cfg.Endpoint = server.URL
	cfg.FlushTimeout = time.Second
	clitrack.New(cfg).Run(func() error { return nil }, nil)

	var body []byte
	select {
	case body = <-requestBody:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for telemetry request")
	}
	var envelope map[string]string
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode telemetry request %q: %v", body, err)
	}
	decoded, err := url.QueryUnescape(envelope["gokey"])
	if err != nil {
		t.Fatalf("decode gokey: %v", err)
	}
	globalFields, err := url.ParseQuery(decoded)
	if err != nil {
		t.Fatalf("parse global telemetry fields: %v", err)
	}
	eventFields, err := url.ParseQuery(globalFields.Get("msg"))
	if err != nil {
		t.Fatalf("parse event telemetry fields: %v", err)
	}

	assertTelemetryKeys(t, globalFields, []string{"app_name", "app_version", "env", "msg", "pid", "platform", "uid", "username", "version"})
	assertTelemetryKeys(t, eventFields, []string{"c1", "c10", "c3", "c4", "c9", "p1", "p4", "ts", "type"})
	for key, want := range map[string]string{
		"app_name": "dws", "app_version": app.RawVersion(), "env": "prod", "pid": "wcCRwZ",
		"platform": "cli", "uid": "user-1", "username": "Alice", "version": app.RawVersion(),
	} {
		if got := globalFields.Get(key); got != want {
			t.Fatalf("global telemetry field %s = %q, want %q", key, got, want)
		}
	}
	for key, want := range map[string]string{
		"type": "event", "p1": "cli.exec", "p4": "SYS", "c1": "dws", "c3": "0", "c9": "sheet read", "c10": "corp-1",
	} {
		if got := eventFields.Get(key); got != want {
			t.Fatalf("event telemetry field %s = %q, want %q", key, got, want)
		}
	}
	for _, key := range []string{"device_id", "ext", "os", "os_version", "pv_id", "sdk_version", "sid", "timezone_offset"} {
		if globalFields.Has(key) {
			t.Fatalf("global telemetry leaked %s: %q", key, decoded)
		}
	}
	for _, key := range []string{"c2", "c5", "c6", "c7", "c8"} {
		if eventFields.Has(key) {
			t.Fatalf("event telemetry leaked %s: %q", key, globalFields.Get("msg"))
		}
	}
}

func assertTelemetryKeys(t *testing.T, fields url.Values, want []string) {
	t.Helper()
	got := make([]string, 0, len(fields))
	for key := range fields {
		got = append(got, key)
	}
	sort.Strings(got)
	if !slices.Equal(got, want) {
		t.Fatalf("telemetry keys = %v, want %v", got, want)
	}
}
