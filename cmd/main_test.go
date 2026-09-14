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

package main

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/telemetry"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageMainPreservesExecution(t *testing.T) {
	for _, code := range []int{0, 1, 3, 5} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			t.Setenv("DO_NOT_TRACK", "")
			testseam.Swap(t, &os.Args, []string{"dws", "sheet", "read", "--access-token", "must-not-leak"})
			testseam.Swap(t, &resolveTelemetryIdentity, func([]string) app.TelemetryIdentity { return app.TelemetryIdentity{} })
			calls := 0
			message := ""
			if code != 0 {
				message = "sanitized failure"
			}
			testseam.Swap(t, &appExecute, func() (int, string, string) { calls++; return code, "sheet read", message })
			var got []telemetry.Event
			testseam.Swap(t, &submitTelemetry, func(event telemetry.Event) { got = append(got, event) })
			before := time.Now().UnixMilli()
			if actual := run(); actual != code {
				t.Fatalf("exit=%d want %d", actual, code)
			}
			if calls != 1 || len(got) != 1 {
				t.Fatalf("execute=%d reports=%d", calls, len(got))
			}
			event := got[0]
			if event.Command != "dws" || event.Path != "sheet read" || event.ExitCode != code || event.ErrorSummary != message || event.Version != app.RawVersion() {
				t.Fatalf("event=%+v", event)
			}
			if event.DurationMillis < 0 || event.CompletedAtMillis < before || event.CompletedAtMillis > time.Now().UnixMilli() {
				t.Fatalf("incorrect original timing: %+v", event)
			}
			if strings.Contains(fmt.Sprint(event), "must-not-leak") {
				t.Fatal("arguments entered event")
			}
		})
	}
}

func TestCrossPlatformCoverageMainOptOutSkipsAllTelemetry(t *testing.T) {
	for _, value := range []string{"1", "0", " true "} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("DO_NOT_TRACK", value)
			testseam.Swap(t, &os.Args, []string{"dws", "version"})
			testseam.Swap(t, &resolveTelemetryIdentity, func([]string) app.TelemetryIdentity { panic("must not read identity") })
			testseam.Swap(t, &submitTelemetry, func(telemetry.Event) { t.Fatal("must not report") })
			calls := 0
			testseam.Swap(t, &appExecute, func() (int, string, string) { calls++; return 0, "version", "" })
			main()
			if calls != 1 {
				t.Fatalf("execute=%d", calls)
			}
		})
	}
}

func TestCrossPlatformCoverageMainDoesNotWaitForIdentity(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "")
	testseam.Swap(t, &os.Args, []string{"dws", "version"})
	started, release, returned := make(chan struct{}), make(chan struct{}), make(chan struct{})
	testseam.Swap(t, &resolveTelemetryIdentity, func([]string) app.TelemetryIdentity {
		close(started)
		defer close(returned)
		<-release
		return app.TelemetryIdentity{UserID: "late-user"}
	})
	defer func() { close(release); <-returned }()
	testseam.Swap(t, &appExecute, func() (int, string, string) { <-started; return 0, "version", "" })
	testseam.Swap(t, &submitTelemetry, func(event telemetry.Event) {
		if event.Identity != (telemetry.Identity{}) {
			t.Fatalf("late identity=%+v", event.Identity)
		}
	})
	done := make(chan int, 1)
	go func() { done <- run() }()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatal(code)
		}
	case <-time.After(time.Second):
		t.Fatal("command waited for identity")
	}
}

func TestCrossPlatformCoverageIdentitySnapshotOwnsArgumentsAndRecovers(t *testing.T) {
	for _, panics := range []bool{false, true} {
		t.Run(fmt.Sprint(panics), func(t *testing.T) {
			args := []string{"version", "--profile=corp-a"}
			read := make(chan struct{})
			expected := app.TelemetryIdentity{UserID: "user-1", UserName: "Alice", CorpID: "corp-a"}
			testseam.Swap(t, &resolveTelemetryIdentity, func(got []string) app.TelemetryIdentity {
				<-read
				if !reflect.DeepEqual(got, []string{"version", "--profile=corp-a"}) {
					panic("arguments mutated")
				}
				if panics {
					panic("private error")
				}
				return expected
			})
			result := startTelemetryIdentity(args)
			args[0] = "mutated"
			close(read)
			select {
			case got := <-result:
				if panics {
					expected = app.TelemetryIdentity{}
				}
				if got != expected {
					t.Fatalf("identity=%+v want %+v", got, expected)
				}
			case <-time.After(time.Second):
				t.Fatal("identity did not finish")
			}
		})
	}
}

func TestCrossPlatformCoveragePrivateWorkerBypassesApp(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	testseam.Swap(t, &os.Args, []string{"dws", "--_dws-telemetry-worker=1"})
	testseam.Swap(t, &appExecute, func() (int, string, string) { t.Fatal("worker entered app"); return 0, "", "" })
	if code := run(); code != 0 {
		t.Fatal(code)
	}
}

func TestCrossPlatformCoverageReadyIdentityAndProcessExit(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "")
	testseam.Swap(t, &os.Args, []string{"dws", "version"})
	testseam.Swap(t, &snapshotTelemetryIdentity, func([]string) <-chan app.TelemetryIdentity {
		ready := make(chan app.TelemetryIdentity, 1)
		ready <- app.TelemetryIdentity{UserID: "user-1", UserName: "Alice", CorpID: "corp-1"}
		return ready
	})
	testseam.Swap(t, &appExecute, func() (int, string, string) { return 3, "version", "sanitized failure" })
	testseam.Swap(t, &submitTelemetry, func(event telemetry.Event) {
		if event.Identity != (telemetry.Identity{UserID: "user-1", UserName: "Alice", CorpID: "corp-1"}) {
			t.Fatalf("identity=%+v", event.Identity)
		}
	})
	exit := -1
	testseam.Swap(t, &exitProcess, func(code int) { exit = code })
	main()
	if exit != 3 {
		t.Fatalf("process exit=%d", exit)
	}
}
