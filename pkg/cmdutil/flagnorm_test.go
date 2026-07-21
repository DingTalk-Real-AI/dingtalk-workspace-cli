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

package cmdutil

import "testing"

func TestMorphIsStableOnCanonicalNames(t *testing.T) {
	for _, name := range []string{"limit", "page-size", "query", "conversation-id", "start", "id", "unified-app-id"} {
		if got := Morph(name); got != name {
			t.Fatalf("Morph(%q) = %q, want stable %q", name, got, name)
		}
	}
}

func TestMorphNormalizesVariants(t *testing.T) {
	cases := map[string]string{
		"pageSize":       "page-size",
		"page_size":      "page-size",
		"page.size":      "page-size",
		"PageSize":       "page-size",
		"maxResults":     "max-results",
		"startTime":      "start-time",
		"start_time":     "start-time",
		"APIKey":         "api-key",
		"userID":         "user-id",
		"conversationId": "conversation-id",
		"LIMIT":          "limit",
		"--page--size":   "page-size",
	}
	for in, want := range cases {
		if got := Morph(in); got != want {
			t.Fatalf("Morph(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMorphIsIdempotent(t *testing.T) {
	for _, name := range []string{"pageSize", "start_time", "APIKey", "conversationId", "max.results"} {
		once := Morph(name)
		if twice := Morph(once); twice != once {
			t.Fatalf("Morph not idempotent for %q: %q -> %q", name, once, twice)
		}
	}
}

func TestMorphHandlesEmptyAndSeparators(t *testing.T) {
	if got := Morph(""); got != "" {
		t.Fatalf("Morph(\"\") = %q, want empty", got)
	}
	if got := Morph("-x-"); got != "x" {
		t.Fatalf("Morph(%q) = %q, want %q", "-x-", got, "x")
	}
}

// TestMorphCoversLegacyToKebabCase locks the basis-A invariant: Morph is a
// strict superset of the pipeline handlers' former toKebabCase, which is now a
// shim delegating here. These are the exact cases the handler package used to
// assert, so a future change to Morph that regressed any of them (camelCase,
// PascalCase, snake_case, UPPER_CASE, acronym runs, spaces, or leading
// separators) would be caught before it could split the two implementations.
func TestMorphCoversLegacyToKebabCase(t *testing.T) {
	cases := map[string]string{
		"userId":                "user-id",
		"UserName":              "user-name",
		"user_name":             "user-name",
		"USER_ID":               "user-id",
		"pageSize":              "page-size",
		"user-id":               "user-id",
		"limit":                 "limit",
		"ID":                    "id",
		"userID":                "user-id",
		"HTMLParser":            "html-parser",
		"getHTTPResponse":       "get-http-response",
		"a":                     "a",
		"":                      "",
		"ABC":                   "abc",
		"already-kebab":         "already-kebab",
		"with spaces":           "with-spaces",
		"__leading_underscores": "leading-underscores",
		"mixedCamel_and_snake":  "mixed-camel-and-snake",
	}
	for in, want := range cases {
		if got := Morph(in); got != want {
			t.Fatalf("Morph(%q) = %q, want %q (legacy toKebabCase parity)", in, got, want)
		}
	}
}
