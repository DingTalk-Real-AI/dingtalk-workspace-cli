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

// oauth_errors.go holds the wire-level response envelope for the
// /cli/cliAuthEnabled endpoint and the denial-reason classifier that turns
// its error_code / auxiliary scope fields into a machine-readable reason
// string for the callback server and device flow.
package auth

// CLIAuthStatus represents the response from /cli/cliAuthEnabled API.
type CLIAuthStatus struct {
	Success   bool           `json:"success"`
	ErrorCode string         `json:"errorCode,omitempty"`
	ErrorMsg  string         `json:"errorMsg,omitempty"`
	Result    *CLIAuthResult `json:"result"`
}

// CLIAuthResult holds the business data returned by /cli/cliAuthEnabled.
// The server computes cliAuthEnabled by considering the org switch, userScope,
// and channelScope together; the CLI uses it as-is.
type CLIAuthResult struct {
	CLIAuthEnabled  bool     `json:"cliAuthEnabled"`
	UserScope       string   `json:"userScope,omitempty"`       // "all" | "specified" | "forbidden"
	AllowedUsers    []string `json:"allowedUsers,omitempty"`    // staffId list when userScope="specified"
	ChannelScope    string   `json:"channelScope,omitempty"`    // "all" | "specified"
	AllowedChannels []string `json:"allowedChannels,omitempty"` // channelCode list when channelScope="specified"
}

// classifyDenialReason inspects a CLIAuthStatus response and returns a machine-readable
// denial reason string. Returns "" when access is granted.
func classifyDenialReason(status *CLIAuthStatus, currentChannel string) string {
	if status.ErrorCode == "CHANNEL_REQUIRED" {
		return "channel_required"
	}
	if status.ErrorCode == "NO_AUTH" {
		return "no_auth"
	}
	if status.Result == nil || !status.Success {
		return "unknown"
	}
	if status.Result.CLIAuthEnabled {
		return ""
	}
	// cliAuthEnabled=false — infer reason from auxiliary fields (same priority as server)
	if status.Result.UserScope == "forbidden" {
		return "user_forbidden"
	}
	if status.Result.UserScope == "specified" {
		return "user_not_allowed"
	}
	if status.Result.ChannelScope == "specified" && currentChannel != "" {
		return "channel_not_allowed"
	}
	return "cli_not_enabled"
}
