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

// PostResponseHandler runs in the PostResponse phase — after the
// transport returns a result and before the output is written to
// stdout. It receives the raw response and can mutate it.
//
// Default behaviour: no-op pass-through. This establishes the
// extension point for:
//   - Output format transformation (e.g. table, CSV, YAML renderers)
//   - Response field filtering or redaction
//   - Pagination metadata injection
//   - Response caching or analytics collection
//
// Logging is handled at the integration point in canonical.go,
// consistent with how other phases log at their call sites.
type PostResponseHandler struct{}

func (PostResponseHandler) Name() string          { return "postresponse" }
func (PostResponseHandler) Phase() pipeline.Phase { return pipeline.PostResponse }

func (PostResponseHandler) Handle(ctx *pipeline.Context) error {
	return nil
}
