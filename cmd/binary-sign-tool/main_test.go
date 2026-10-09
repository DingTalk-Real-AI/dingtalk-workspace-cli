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

package main

import (
	"errors"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageMainReportsFailureAndExits(t *testing.T) {
	code := 0
	testseam.Swap(t, &runCLI, func([]string) error { return errors.New("boom") })
	testseam.Swap(t, &exitProcess, func(c int) { code = c })
	main()
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}

func TestCrossPlatformCoverageMainSucceedsWithoutExiting(t *testing.T) {
	code := 0
	testseam.Swap(t, &runCLI, func([]string) error { return nil })
	testseam.Swap(t, &exitProcess, func(c int) { code = c })
	main()
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
}

func TestCrossPlatformCoverageRunRequiresSelfSignMode(t *testing.T) {
	cases := [][]string{
		nil,
		{"verify"},
		{"sign", "-inFile", "input", "-outFile", "output"},
		{"sign", "-inFile", "input", "-outFile", "output", "-selfSign", "0"},
		{"sign", "-inFile", "input", "-inFile", "other", "-outFile", "output", "-selfSign", "1"},
		{"sign", "-inFile", "input", "-outFile", "output", "-outFile", "other", "-selfSign", "1"},
		{"sign", "-inFile", "input", "-outFile", "output", "-selfSign", "1", "-selfSign", "1"},
		{"sign", "-inFile"},
		{"sign", "-outFile"},
		{"sign", "-selfSign"},
		{"sign", "-inFile", "", "-outFile", "output", "-selfSign", "1"},
		{"sign", "-inFile", "input", "-outFile", "", "-selfSign", "1"},
		{"sign", "-inFile", "input", "-outFile", "output", "-selfSign", ""},
	}
	for _, args := range cases {
		if err := run(args); err == nil {
			t.Fatalf("run(%q) unexpectedly succeeded", args)
		}
	}
}

func TestCrossPlatformCoverageRunRejectsUnknownArgument(t *testing.T) {
	if err := run([]string{"sign", "-unknown"}); err == nil {
		t.Fatal("unknown argument was accepted")
	}
}
