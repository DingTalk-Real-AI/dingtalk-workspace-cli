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

package handlers

import (
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
)

// PreRequestHandler runs in the PreRequest phase — after parameter
// validation succeeds and just before the JSON-RPC call is dispatched.
// It receives the final payload and can inspect or mutate it.
//
// Default behaviour: no-op pass-through. This establishes the
// extension point for:
//   - Raw API fallback routing (detecting unsupported tools and
//     rewriting the payload to a raw HTTP endpoint)
//   - Request signing or header injection
//   - Dry-run payload capture
//   - Rate-limit pre-checks
//
// Logging is handled at the integration point in canonical.go,
// consistent with how other phases log at their call sites.
type PreRequestHandler struct{}

func (PreRequestHandler) Name() string          { return "prerequest" }
func (PreRequestHandler) Phase() pipeline.Phase { return pipeline.PreRequest }

func (PreRequestHandler) Handle(ctx *pipeline.Context) error {
	return nil
}
