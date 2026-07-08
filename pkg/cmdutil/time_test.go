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

func TestParseISOTimeToMillisAcceptsUnixTimestamps(t *testing.T) {
	got, err := ParseISOTimeToMillis("start", "1774332000000")
	if err != nil {
		t.Fatalf("milliseconds timestamp: %v", err)
	}
	if got != 1774332000000 {
		t.Fatalf("milliseconds timestamp = %d", got)
	}

	got, err = ParseISOTimeToMillis("start", "1774332000")
	if err != nil {
		t.Fatalf("seconds timestamp: %v", err)
	}
	if got != 1774332000000 {
		t.Fatalf("seconds timestamp = %d", got)
	}
}
