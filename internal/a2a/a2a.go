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

// Package a2a is the single entry point for third-party host integration
// with dws (the A2A / agent-to-agent surface). It aggregates four capabilities
// that were previously scattered across pkg/runtimetoken, internal/app and
// internal/auth:
//
//   - access token resolution (ResolveAccessToken)
//   - identity / trace HTTP headers (IdentityHeaders)
//   - upstream channel forwarding (CurrentChannelCode, ApplyChannelHeader)
//   - plugin-scoped auth registry (Register / Lookup)
//
// a2a 是第三方宿主对接 dws 的 A2A 能力聚合包：提供 access token 解析、
// 身份 header、上游 channelCode 转发与 plugin auth registry 四类能力。
//
// Host-facing callers SHOULD depend on this package rather than reaching
// into the underlying internal packages. The legacy call sites remain as
// thin shims marked Deprecated for backwards compatibility and will be
// removed in a future minor release.
package a2a
