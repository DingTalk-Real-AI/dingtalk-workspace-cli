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
)

// TestAuthScopesAndCheckRegistered guards that the skeleton subcommands stay
// wired into `dws auth`. Once #253 is implemented, replace the not-implemented
// assertions below with real behaviour coverage.
func TestAuthScopesAndCheckRegistered(t *testing.T) {
	auth := buildAuthCommand()
	for _, name := range []string{"scopes", "check"} {
		sub, _, err := auth.Find([]string{name})
		if err != nil || sub == nil || sub.Name() != name {
			t.Fatalf("expected `dws auth %s` to be registered; err=%v sub=%v", name, err, sub)
		}
	}
}

// TODO(#253): replace with real coverage once `dws auth scopes` / `dws auth
// check` are implemented (granted-scope listing, exit-code behaviour, --for).
func TestAuthScopesNotImplementedYet(t *testing.T) {
	cmd := newAuthScopesCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(nil)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected `dws auth scopes` to return a not-implemented error; got nil — implementing #253? update this test")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("error = %q, want it to mention 'not implemented'", err)
	}
}

func TestAuthCheckNotImplementedYet(t *testing.T) {
	cmd := newAuthCheckCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"Calendar.Events.Write"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected `dws auth check` to return a not-implemented error; got nil — implementing #253? update this test")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("error = %q, want it to mention 'not implemented'", err)
	}
}
