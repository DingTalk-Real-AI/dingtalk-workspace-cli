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

//go:build darwin || linux

package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageUnixBrowserCoverageEdges(t *testing.T) {
	oldStart := browserStartCommand
	t.Cleanup(func() { browserStartCommand = oldStart })

	var calls []string
	browserStartCommand = func(name string, args ...string) error {
		calls = append(calls, name+":"+strings.Join(args, ","))
		return nil
	}
	if err := openBrowserForOS("darwin", "https://example.test"); err != nil {
		t.Fatal(err)
	}
	if err := openBrowserForOS("linux", "http://example.test"); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || !strings.HasPrefix(calls[0], "open:") || !strings.HasPrefix(calls[1], "xdg-open:") {
		t.Fatalf("browser calls = %v", calls)
	}
	if err := openBrowserForOS("plan9", "https://example.test"); err == nil {
		t.Fatal("unsupported browser platform succeeded")
	}
	if err := openBrowserForOS("darwin", "://bad"); err == nil {
		t.Fatal("invalid browser URL succeeded")
	}
	if err := openBrowserForOS("darwin", "file:///tmp/x"); err == nil {
		t.Fatal("unsafe browser URL succeeded")
	}
	browserStartCommand = func(string, ...string) error { return errors.New("start") }
	if err := openBrowserForOS("darwin", "https://example.test"); err == nil {
		t.Fatal("browser start error was ignored")
	}

	browserStartCommand = func(string, ...string) error { return nil }
	if err := openBrowser("https://example.test"); err != nil {
		t.Fatal(err)
	}
	if err := oldStart("true"); err != nil {
		t.Fatalf("default browser command hook = %v", err)
	}
}
