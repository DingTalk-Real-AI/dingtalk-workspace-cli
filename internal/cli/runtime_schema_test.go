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

package cli

import "testing"

func TestLowerCamelFlagName(t *testing.T) {
	tests := map[string]string{
		"agentCode":      "agentCode",
		"unified-app-id": "unifiedAppId",
		"session_id":     "sessionId",
	}
	for input, want := range tests {
		if got := lowerCamelFlagName(input); got != want {
			t.Errorf("lowerCamelFlagName(%q) = %q, want %q", input, got, want)
		}
	}
}
