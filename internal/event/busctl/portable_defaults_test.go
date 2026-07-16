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

package busctl

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestCrossPlatformCoveragePortableDefaultFunctionCoverage(t *testing.T) {
	originalDiscoverDial := discoverDial
	client, server := net.Pipe()
	t.Cleanup(func() {
		discoverDial = originalDiscoverDial
		_ = client.Close()
		_ = server.Close()
	})
	discoverDial = func(string) (net.Conn, error) { return client, nil }
	conn, err := Discover(DiscoverConfig{
		WorkDir:     "unused",
		IPCEndpoint: "endpoint",
		ClientID:    "client",
	})
	if err != nil {
		t.Fatalf("Discover() initial dial = %v", err)
	}
	if conn != client {
		t.Fatalf("Discover() connection = %T, want injected pipe", conn)
	}

	if conn, err := statusDial(filepath.Join(t.TempDir(), "missing")); err == nil {
		_ = conn.Close()
		t.Fatal("default status dial unexpectedly succeeded")
	}

	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess() = %v", err)
	}
	if err := proc.Release(); err != nil {
		t.Fatalf("Release() = %v", err)
	}
	if err := stopSignalProcess(proc, os.Interrupt); err == nil {
		t.Fatal("signaling a released process unexpectedly succeeded")
	}
}
