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

// Package editiontest provides contract-test helpers that any edition.Hooks
// implementation (open-source or overlay) must satisfy. It lives in a
// non-_test package so external modules can import and run these checks.
package editiontest

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// RunContractTests validates that the supplied Hooks implementation
// satisfies the behavioural contract required by the open-source core.
// Call this from both the open-source test suite and any overlay's tests.
func RunContractTests(t *testing.T, h *edition.Hooks) {
	t.Helper()

	if h == nil {
		t.Fatal("Hooks must not be nil")
	}

	t.Run("Name", func(t *testing.T) {
		if h.Name == "" {
			t.Error("Hooks.Name must be non-empty")
		}
	})

	t.Run("ConfigDir", func(t *testing.T) {
		if h.ConfigDir != nil {
			dir := h.ConfigDir()
			if dir == "" {
				t.Error("ConfigDir() must return a non-empty path when set")
			}
		}
	})

	t.Run("MergeHeaders_preserves_base", func(t *testing.T) {
		if h.MergeHeaders == nil {
			return
		}
		base := map[string]string{"x-test": "value"}
		merged := h.MergeHeaders(base)
		if merged == nil {
			t.Fatal("MergeHeaders must return a non-nil map")
		}
		if merged["x-test"] != "value" {
			t.Error("MergeHeaders must preserve base headers")
		}
	})

	t.Run("StaticServers_non_empty_when_set", func(t *testing.T) {
		if h.StaticServers == nil {
			return
		}
		servers := h.StaticServers()
		if len(servers) == 0 {
			t.Error("StaticServers() must return at least one server when set")
		}
		for i, s := range servers {
			if s.ID == "" || s.Endpoint == "" {
				t.Errorf("StaticServers()[%d]: ID and Endpoint must be non-empty", i)
			}
		}
	})

	t.Run("VisibleProducts_non_empty_when_set", func(t *testing.T) {
		if h.VisibleProducts == nil {
			return
		}
		products := h.VisibleProducts()
		if len(products) == 0 {
			t.Error("VisibleProducts() must return at least one product when set")
		}
	})
}
