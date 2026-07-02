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

func TestIPCEndpointShortWorkDirUsesCanonicalPath(t *testing.T) {
	workDir := "/tmp/dws/events/open/app_stream/aabbccdd00112233"
	got := IPCEndpoint(workDir, "open", SourceKindAppStream, "aabbccdd00112233")
	want := filepath.Join(workDir, "bus.sock")
	if got != want {
		t.Fatalf("IPCEndpoint = %q, want %q", got, want)
	}
}

func TestIPCEndpointLongWorkDirFallsBackUnderTempDir(t *testing.T) {
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
