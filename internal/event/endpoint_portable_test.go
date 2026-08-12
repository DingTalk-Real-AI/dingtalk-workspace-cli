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

package event

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageEndpointPortableCoverageEdges(t *testing.T) {
	if got := MaxUnixSocketPath(); got != 103 && got != 107 {
		t.Fatalf("MaxUnixSocketPath() = %d", got)
	}
	if maxUnixSocketPath("linux") != 107 || maxUnixSocketPath("darwin") != 103 {
		t.Fatal("Unix socket limits changed")
	}

	if got := IPCEndpoint("short", "open", SourceKindAppStream, "hash"); got == "" {
		t.Fatal("IPCEndpoint() returned an empty endpoint")
	}
	if got := ipcEndpointForOS("windows", "ignored", "open", "", "hash"); got != `\\.\pipe\dws-event-open-app_stream-hash` {
		t.Fatalf("Windows endpoint = %q", got)
	}

	runtimeRoot := filepath.VolumeName(os.TempDir()) + string(filepath.Separator)
	runtimeDir := filepath.Join(runtimeRoot, "dws-xdg-runtime")
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	workDir := "portable-xdg-workdir"
	wantXDG := filepath.Join(runtimeDir, eventRuntimeDirPrefix+currentUserID(), "dws-evt-"+IdentityHash(workDir)+".sock")
	if got := ipcEndpointForOS("linux", workDir, "open", SourceKindPersonalStream, "hash"); got != wantXDG {
		t.Fatalf("XDG Unix endpoint = %q, want %q", got, wantXDG)
	}

	t.Setenv("XDG_RUNTIME_DIR", "")
	if got := ipcEndpointForOS("darwin", "short", "open", SourceKindPersonalStream, "hash"); got != filepath.Join(os.TempDir(), eventRuntimeDirPrefix+currentUserID(), "dws-evt-"+IdentityHash("short")+".sock") {
		t.Fatalf("short Unix endpoint = %q", got)
	}
	if got := ipcEndpointForOS("darwin", strings.Repeat("x", 200), "open", SourceKindAppStream, "hash"); !strings.Contains(got, "dws-evt-") {
		t.Fatalf("long Unix endpoint = %q", got)
	}

	longTempDir := filepath.Join(string(filepath.Separator), strings.Repeat("long-temp-root", 20))
	wantShortFallback := filepath.Join("/tmp", eventRuntimeDirPrefix+currentUserID(), "dws-evt-"+IdentityHash(workDir)+".sock")
	if got := unixSocketEndpoint("darwin", workDir, "", longTempDir); got != wantShortFallback {
		t.Fatalf("overlong temp endpoint = %q, want %q", got, wantShortFallback)
	}
}
