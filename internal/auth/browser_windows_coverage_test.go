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

//go:build windows

package auth

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestCrossPlatformCoverageWindowsBrowserCoverageEdges(t *testing.T) {
	cmd := browserWindowsCommand("https://example.test")
	if cmd.Path == "" || !strings.Contains(strings.ToLower(cmd.String()), "rundll32") {
		t.Fatalf("browser command = %q, want rundll32 command", cmd.String())
	}
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags == 0 {
		t.Fatalf("browser command SysProcAttr = %#v", cmd.SysProcAttr)
	}

	oldStart := browserWindowsStartCommand
	t.Cleanup(func() { browserWindowsStartCommand = oldStart })

	var started string
	browserWindowsStartCommand = func(cmd *exec.Cmd) error {
		started = cmd.String()
		return nil
	}
	if err := openBrowser("https://example.test"); err != nil {
		t.Fatalf("openBrowser() = %v", err)
	}
	if !strings.Contains(strings.ToLower(started), "rundll32") {
		t.Fatalf("started command = %q, want rundll32", started)
	}
	if err := openBrowser("://bad"); err == nil {
		t.Fatal("invalid browser URL succeeded")
	}
	if err := openBrowser("file:///tmp/x"); err == nil {
		t.Fatal("unsafe browser URL succeeded")
	}
	browserWindowsStartCommand = func(*exec.Cmd) error { return errors.New("start") }
	if err := openBrowser("https://example.test"); err == nil {
		t.Fatal("browser start error was ignored")
	}

	if err := oldStart(exec.Command(os.Args[0], "-test.run=^TestCrossPlatformCoverageWindowsBrowserHelperProcess$")); err != nil {
		t.Fatalf("default browser start hook = %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := oldStart(exec.Command("dws-browser-command-that-should-not-exist")); err == nil {
		t.Fatal("default browser start hook unexpectedly started a missing command")
	}
}

func TestCrossPlatformCoverageWindowsBrowserHelperProcess(t *testing.T) {}
