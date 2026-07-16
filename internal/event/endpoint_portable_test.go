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
	if got := ipcEndpointForOS("darwin", "short", "open", SourceKindPersonalStream, "hash"); got != filepath.Join("short", "bus.sock") {
		t.Fatalf("short Unix endpoint = %q", got)
	}
	if got := ipcEndpointForOS("darwin", strings.Repeat("x", 200), "open", SourceKindAppStream, "hash"); !strings.Contains(got, "dws-evt-") {
		t.Fatalf("long Unix endpoint = %q", got)
	}
}
