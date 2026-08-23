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

// This file holds deliberate no-op stubs that act as edition-sync anchors for
// the private wukong edition overlay. The overlay replaces these function
// bodies with real host-compatibility configuration while the open-source
// build keeps them empty; both editions therefore share identical call sites
// (access_token_resolve.go, auth_command.go, doctor_command.go,
// force_refresh.go). These stubs are sync seams for the edition overlay, not
// dead code: do not delete them or their call sites during cleanup.
package app

import (
	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
)

func configureOAuthProviderCompatibility(*authpkg.OAuthProvider, string) {}

func configureLegacyAuthManagerCompatibility(*authpkg.Manager) {}
