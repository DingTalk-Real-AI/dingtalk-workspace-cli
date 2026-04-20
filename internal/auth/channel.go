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

package auth

import (
	"net/http"
	"os"
	"strings"
)

const (
	// DWSChannelEnv carries the upstream channelCode forwarded as x-dws-channel.
	DWSChannelEnv = "DWS_CHANNEL"
	// DINGTALK_AGENT carries the business agent name used for both the
	// x-dingtalk-agent header and the effective claw-type selector.
	DingTalkAgentEnv = "DINGTALK_AGENT"
)

// CurrentChannelCode returns the raw upstream channel code as configured locally.
func CurrentChannelCode() string {
	return os.Getenv(DWSChannelEnv)
}

// CurrentClawType returns the normalized claw-type selector derived from
// DINGTALK_AGENT.
func CurrentClawType() string {
	return normalizeClawType(os.Getenv(DingTalkAgentEnv))
}

// IsHostPATClawType reports whether the effective claw-type represents a
// host-owned PAT integration.
func IsHostPATClawType(raw string) bool {
	normalized := normalizeClawType(raw)
	return normalized != "" && normalized != "default"
}

// CurrentHostPATClawType returns the effective host-owned PAT selector.
func CurrentHostPATClawType() string {
	if clawType := CurrentClawType(); IsHostPATClawType(clawType) {
		return clawType
	}
	return ""
}

// ApplyChannelHeader injects the configured channel code into a request.
func ApplyChannelHeader(req *http.Request) {
	if req == nil {
		return
	}
	if ch := CurrentChannelCode(); ch != "" {
		req.Header.Set("x-dws-channel", ch)
	}
}

func normalizeClawType(raw string) string {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	return normalized
}
