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

import "testing"

func TestCloneToolArgsDefensiveCopy(t *testing.T) {
	args := map[string]any{"page": 1, "query": "keep"}
	cloned := cloneToolArgs(args)
	args["page"] = 2
	args["extra"] = true

	if got := cloned["page"]; got != 1 {
		t.Fatalf("cloned page = %#v, want 1", got)
	}
	if _, ok := cloned["extra"]; ok {
		t.Fatal("clone changed after source map mutation")
	}
}

func TestCloneToolArgsEmpty(t *testing.T) {
	if got := cloneToolArgs(nil); got != nil {
		t.Fatalf("nil clone = %#v, want nil", got)
	}
	if got := cloneToolArgs(map[string]any{}); got != nil {
		t.Fatalf("empty clone = %#v, want nil", got)
	}
}
