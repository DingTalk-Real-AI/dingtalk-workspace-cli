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
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageCacheDeprecatedCompatShim(t *testing.T) {
	root := NewRootCommand()
	group := mustFindCommand(t, root, "cache")
	if group.Hidden || group.Deprecated == "" || !group.Runnable() {
		t.Fatalf("cache group contract: hidden=%v deprecated=%q runnable=%v", group.Hidden, group.Deprecated, group.Runnable())
	}
	if group.IsAvailableCommand() {
		t.Fatal("deprecated cache group must not be IsAvailableCommand")
	}

	for _, leaf := range []string{"refresh", "status", "clean"} {
		cmd := mustFindCommand(t, root, "cache", leaf)
		if cmd.Hidden || cmd.Deprecated == "" || !cmd.Runnable() {
			t.Fatalf("cache %s contract: hidden=%v deprecated=%q runnable=%v", leaf, cmd.Hidden, cmd.Deprecated, cmd.Runnable())
		}
		if cmd.IsAvailableCommand() {
			t.Fatalf("deprecated cache %s must not be IsAvailableCommand", leaf)
		}
	}

	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"cache", "refresh", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cache refresh compatibility stub: %v\n%s", err, out.String())
	}
	got := out.String()
	for _, want := range []string{`"status":"deprecated"`, `"command":"dws cache refresh"`, "不再支持", "服务发现已下线"} {
		if !strings.Contains(got, want) {
			t.Fatalf("cache refresh output missing %q:\n%s", want, got)
		}
	}

	for _, format := range []string{"", "json", "pretty", "table"} {
		var buf bytes.Buffer
		parent := &cobra.Command{Use: "dws"}
		parent.PersistentFlags().String("format", format, "")
		parent.SetOut(&buf)
		sub := &cobra.Command{Use: "cache"}
		parent.AddCommand(sub)
		if err := printCacheCompatNotice(sub, "dws cache status"); err != nil {
			t.Fatalf("format=%q: %v", format, err)
		}
		text := buf.String()
		if !strings.Contains(text, "不再支持") && !strings.Contains(text, "服务发现已下线") {
			t.Fatalf("format=%q missing notice:\n%s", format, text)
		}
		if format == "" || format == "json" || format == "pretty" {
			if !strings.Contains(text, `"status":"deprecated"`) && !strings.Contains(text, `"status": "deprecated"`) {
				t.Fatalf("format=%q missing deprecated JSON status:\n%s", format, text)
			}
		}
	}

	for _, format := range []string{"pretty", "table"} {
		parent := &cobra.Command{Use: "dws"}
		parent.PersistentFlags().String("format", format, "")
		parent.SetOut(failWriter{})
		sub := &cobra.Command{Use: "cache"}
		parent.AddCommand(sub)
		if err := printCacheCompatNotice(sub, "dws cache clean"); err == nil || !strings.Contains(err.Error(), "write failed") {
			t.Fatalf("format=%q write failure = %v, want write failed", format, err)
		}
	}

	parent := newCacheCommand()
	var parentOut bytes.Buffer
	rootWrap := &cobra.Command{Use: "dws"}
	rootWrap.PersistentFlags().String("format", "json", "")
	rootWrap.SetOut(&parentOut)
	rootWrap.AddCommand(parent)
	parent.SetOut(&parentOut)
	if err := parent.RunE(parent, nil); err != nil {
		t.Fatalf("cache parent RunE = %v, want nil success", err)
	}
	if !strings.Contains(parentOut.String(), `"command":"dws cache"`) {
		t.Fatalf("cache parent notice missing command:\n%s", parentOut.String())
	}

	cache := newCacheCommand()
	cache.SetOut(&bytes.Buffer{})
	cache.SetArgs([]string{"status"})
	if err := cache.Execute(); err != nil {
		t.Fatal(err)
	}
}
