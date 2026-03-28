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

package output

import "testing"

func TestSanitizeForTerminalStripsControlCharacters(t *testing.T) {
	t.Parallel()

	// ANSI escape sequences and control characters are fully stripped.
	got := SanitizeForTerminal("hello\x1b[31mworld\x07")
	want := "helloworld"
	if got != want {
		t.Fatalf("SanitizeForTerminal() = %q, want %q", got, want)
	}
}

func TestSanitizeForTerminalStripsDangerousUnicode(t *testing.T) {
	t.Parallel()

	got := SanitizeForTerminal("a\u202Eb\u200Bc")
	want := "abc"
	if got != want {
		t.Fatalf("SanitizeForTerminal() = %q, want %q", got, want)
	}
}

func TestSanitizeForTerminalPreservesReadableWhitespace(t *testing.T) {
	t.Parallel()

	got := SanitizeForTerminal("line1\nline2\tvalue")
	want := "line1\nline2\tvalue"
	if got != want {
		t.Fatalf("SanitizeForTerminal() = %q, want %q", got, want)
	}
}
