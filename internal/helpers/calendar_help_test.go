// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"bytes"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageCalendarEventCreateHelpKeepsRoomsStringMetavar(t *testing.T) {
	cmd := newCalendarCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"event", "create", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("calendar event create --help: %v\n%s", err, out.String())
	}

	help := out.String()
	if !strings.Contains(help, "--rooms string") {
		t.Fatalf("calendar event create help missing string metavar for --rooms:\n%s", help)
	}
	if strings.Contains(help, "--rooms room search") {
		t.Fatalf("calendar event create help treated description text as --rooms metavar:\n%s", help)
	}
}
