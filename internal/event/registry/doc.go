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

// Package registry holds two cross-cutting reference data sets used by
// daemon, consume, and the cobra command layer:
//
//   - CatchAll: the default event_type list passed to `consume`'s Hello
//     when --event-types is omitted. v1 (P4) ships an empty list so the
//     consumer falls back to bus-wide catch-all; P7 fills in the curated
//     DingTalk default set after P0 SDK verification confirms the exact
//     event_type string values.
//
//   - CompactProcessors: per-event_type formatters that flatten the SDK
//     RawEvent into an agent-friendly map[string]any. A generic processor
//     handles unknown types by surfacing the top-level header fields plus
//     a parsed payload; specialised processors (im.message.* etc.) extract
//     semantically meaningful fields.
package registry
