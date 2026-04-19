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

// RegisterHandler runs during the Register phase — the first stage
// in the pipeline, executed while the Cobra command tree is being
// built. It validates that the registration context carries a
// non-empty command identifier.
//
// The handler is intentionally lightweight and side-effect free.
// This provides the structural hook for future extensions (e.g.
// dynamic command injection, feature gating, or Raw API fallback
// command registration) without adding any runtime overhead to
// the default path. Logging is handled at the call site in
// canonical.go, consistent with how PreParse logging is done
// in cobra.go.
type RegisterHandler struct{}

func (RegisterHandler) Name() string          { return "register" }
func (RegisterHandler) Phase() pipeline.Phase { return pipeline.Register }

func (RegisterHandler) Handle(ctx *pipeline.Context) error {
	return nil
}
