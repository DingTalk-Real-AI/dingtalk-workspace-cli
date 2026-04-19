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

package app

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func TestStdioEndpoint(t *testing.T) {
	endpoint := StdioEndpoint("hello-plugin", "hello")
	want := "stdio://hello-plugin/hello"
	if endpoint != want {
		t.Errorf("StdioEndpoint() = %q, want %q", endpoint, want)
	}
}

func TestIsStdioEndpoint(t *testing.T) {
	tests := []struct {
		endpoint string
		want     bool
	}{
		{"stdio://hello-plugin/hello", true},
		{"stdio://conference/local", true},
		{"https://mcp.dingtalk.com", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsStdioEndpoint(tt.endpoint); got != tt.want {
			t.Errorf("IsStdioEndpoint(%q) = %v, want %v", tt.endpoint, got, tt.want)
		}
	}
}

func TestStdioClientRegistry(t *testing.T) {
	// Clean up after test
	defer func() {
		stdioMu.Lock()
		delete(stdioClients, "test-product")
		stdioMu.Unlock()
	}()

	// Initially not found
	if _, ok := LookupStdioClient("test-product"); ok {
		t.Error("expected LookupStdioClient to return false for unregistered product")
	}

	// Register a client
	client := transport.NewStdioClient("echo", nil, nil)
	RegisterStdioClient("test-product", client)

	// Now should be found
	got, ok := LookupStdioClient("test-product")
	if !ok {
		t.Fatal("expected LookupStdioClient to return true after registration")
	}
	if got != client {
		t.Error("LookupStdioClient returned different client instance")
	}
}
