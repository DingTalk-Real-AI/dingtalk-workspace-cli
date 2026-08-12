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

//go:build !windows

package event

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEndpointPlatformVariants(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	if maxUnixSocketPath("linux") != 107 || maxUnixSocketPath("darwin") != 103 {
		t.Fatal("Unix socket limits changed")
	}
	pipe := ipcEndpointForOS("windows", "ignored", "open", "", "hash")
	if pipe != `\\.\pipe\dws-event-open-app_stream-hash` {
		t.Fatalf("Windows pipe = %q", pipe)
	}
	short := ipcEndpointForOS("darwin", "/tmp/events", "open", SourceKindPersonalStream, "hash")
	if short != filepath.Join(os.TempDir(), eventRuntimeDirPrefix+currentUserID(), "dws-evt-"+IdentityHash("/tmp/events")+".sock") {
		t.Fatalf("short Unix endpoint = %q", short)
	}
	long := ipcEndpointForOS("darwin", "/"+strings.Repeat("deep/", 40), "open", SourceKindAppStream, "hash")
	if !strings.Contains(long, "dws-evt-") {
		t.Fatalf("long Unix endpoint = %q", long)
	}
}

func TestIPCEndpointUsesXDGUserRuntimeDir(t *testing.T) {
	tempRoot, err := filepath.EvalSymlinks("/tmp")
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	runtimeDir, err := os.MkdirTemp(tempRoot, "dws-xdg-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDir) })
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	workDir := "/shared/events/open/app_stream/aabbccdd00112233"
	got := IPCEndpoint(workDir, "open", SourceKindAppStream, "aabbccdd00112233")
	want := filepath.Join(runtimeDir, eventRuntimeDirPrefix+currentUserID(), "dws-evt-"+IdentityHash(workDir)+".sock")
	if got != want {
		t.Fatalf("IPCEndpoint = %q, want %q", got, want)
	}
}

func TestIPCEndpointWithoutXDGUsesPerUserLocalTempDir(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	workDir := "/tmp/dws/events/open/app_stream/aabbccdd00112233"
	got := IPCEndpoint(workDir, "open", SourceKindAppStream, "aabbccdd00112233")
	want := filepath.Join(os.TempDir(), eventRuntimeDirPrefix+currentUserID(), "dws-evt-"+IdentityHash(workDir)+".sock")
	if got != want {
		t.Fatalf("IPCEndpoint = %q, want %q", got, want)
	}
	if strings.HasPrefix(got, workDir) {
		t.Fatalf("IPCEndpoint = %q, want endpoint outside workDir", got)
	}
}

func TestIPCEndpointLongWorkDirUsesLocalTempDir(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	// Mirrors the dwssb sandbox layout that produced a 111-byte socket
	// path — over macOS's 103-byte usable sun_path budget.
	workDir := "/Users/zhengyubai/.dwssb/sandboxes/event-subscribe/config/events/open/personal_stream/3928ce0fb4860a52"
	got := IPCEndpoint(workDir, "open", SourceKindPersonalStream, "3928ce0fb4860a52")
	if strings.HasPrefix(got, workDir) {
		t.Fatalf("IPCEndpoint = %q, want fallback outside workDir", got)
	}
	if !strings.HasPrefix(got, os.TempDir()) {
		t.Fatalf("IPCEndpoint = %q, want fallback under os.TempDir %q", got, os.TempDir())
	}
	if len(got) > MaxUnixSocketPath() {
		t.Fatalf("fallback path still too long: %d > %d (%q)", len(got), MaxUnixSocketPath(), got)
	}
}

func TestIPCEndpointFallbackIsDeterministicPerWorkDir(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	long := strings.Repeat("x", 120)
	a := IPCEndpoint("/base/"+long+"/one", "open", SourceKindPersonalStream, "hash")
	b := IPCEndpoint("/base/"+long+"/one", "open", SourceKindPersonalStream, "hash")
	c := IPCEndpoint("/base/"+long+"/two", "open", SourceKindPersonalStream, "hash")
	if a != b {
		t.Fatalf("same workDir produced different endpoints: %q vs %q", a, b)
	}
	if a == c {
		t.Fatalf("different workDirs collided on endpoint %q", a)
	}
}

func TestIPCEndpointLongXDGPathFallsBackToTempDir(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/"+strings.Repeat("runtime/", 30))
	workDir := "/shared/events/open/personal_stream/aabbccdd00112233"
	got := ipcEndpointForOS("linux", workDir, "open", SourceKindPersonalStream, "hash")
	wantPrefix := filepath.Join(os.TempDir(), eventRuntimeDirPrefix+currentUserID()) + string(filepath.Separator)
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("IPCEndpoint = %q, want fallback under %q", got, wantPrefix)
	}
	if len(got) > maxUnixSocketPath("linux") {
		t.Fatalf("fallback path still too long: %d > %d (%q)", len(got), maxUnixSocketPath("linux"), got)
	}
}

func TestIPCEndpointLongTempDirUsesShortSystemFallback(t *testing.T) {
	workDir := "/shared/events/open/personal_stream/aabbccdd00112233"
	longTempDir := "/" + strings.Repeat("long-temp-root/", 20)
	got := unixSocketEndpoint("linux", workDir, "", longTempDir)
	want := filepath.Join("/tmp", eventRuntimeDirPrefix+currentUserID(), "dws-evt-"+IdentityHash(workDir)+".sock")
	if got != want {
		t.Fatalf("IPCEndpoint = %q, want short fallback %q", got, want)
	}
	if len(got) > maxUnixSocketPath("linux") {
		t.Fatalf("short fallback path too long: %d > %d (%q)", len(got), maxUnixSocketPath("linux"), got)
	}
}
