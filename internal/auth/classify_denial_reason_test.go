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

import "testing"

func TestClassifyDenialReason(t *testing.T) {
	cases := []struct {
		name           string
		status         *CLIAuthStatus
		currentChannel string
		want           string
	}{
		{
			name: "error CHANNEL_REQUIRED",
			status: &CLIAuthStatus{
				ErrorCode: "CHANNEL_REQUIRED",
			},
			want: "channel_required",
		},
		{
			name: "error NO_AUTH",
			status: &CLIAuthStatus{
				ErrorCode: "NO_AUTH",
			},
			want: "no_auth",
		},
		{
			name: "success false or nil result → unknown",
			status: &CLIAuthStatus{
				Success: false,
			},
			want: "unknown",
		},
		{
			name: "cliAuthEnabled true → no denial",
			status: &CLIAuthStatus{
				Success: true,
				Result:  &CLIAuthResult{CLIAuthEnabled: true},
			},
			want: "",
		},
		{
			name: "userScope forbidden wins over channel",
			status: &CLIAuthStatus{
				Success: true,
				Result: &CLIAuthResult{
					CLIAuthEnabled:  false,
					UserScope:       "forbidden",
					ChannelScope:    "specified",
					AllowedChannels: []string{"channel-a"},
				},
			},
			currentChannel: "channel-b",
			want:           "user_forbidden",
		},
		{
			// Real-world case reported: user is in allowedUsers but the current
			// DWS_CHANNEL is not in allowedChannels. Reason must be channel,
			// NOT user.
			name: "user allowed but channel not in allowedChannels → channel_not_allowed",
			status: &CLIAuthStatus{
				Success: true,
				Result: &CLIAuthResult{
					CLIAuthEnabled:  false,
					UserScope:       "specified",
					AllowedUsers:    []string{"014566033934857460"},
					ChannelScope:    "specified",
					AllowedChannels: []string{"2a4a658e467998befb7fa333c19ba2b3a3bacfa4"},
				},
			},
			currentChannel: "different-channel",
			want:           "channel_not_allowed",
		},
		{
			name: "channelScope specified but current channel empty → channel_required",
			status: &CLIAuthStatus{
				Success: true,
				Result: &CLIAuthResult{
					CLIAuthEnabled:  false,
					UserScope:       "specified",
					ChannelScope:    "specified",
					AllowedChannels: []string{"channel-a"},
				},
			},
			currentChannel: "",
			want:           "channel_required",
		},
		{
			name: "channel matches allowedChannels → fall back to user denial",
			status: &CLIAuthStatus{
				Success: true,
				Result: &CLIAuthResult{
					CLIAuthEnabled:  false,
					UserScope:       "specified",
					ChannelScope:    "specified",
					AllowedChannels: []string{"channel-a"},
				},
			},
			currentChannel: "channel-a",
			want:           "user_not_allowed",
		},
		{
			name: "only userScope=specified, no channel restriction → user_not_allowed",
			status: &CLIAuthStatus{
				Success: true,
				Result: &CLIAuthResult{
					CLIAuthEnabled: false,
					UserScope:      "specified",
				},
			},
			currentChannel: "",
			want:           "user_not_allowed",
		},
		{
			name: "no user or channel restriction → cli_not_enabled",
			status: &CLIAuthStatus{
				Success: true,
				Result:  &CLIAuthResult{CLIAuthEnabled: false},
			},
			want: "cli_not_enabled",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyDenialReason(tc.status, tc.currentChannel)
			if got != tc.want {
				t.Fatalf("classifyDenialReason() = %q, want %q", got, tc.want)
			}
		})
	}
}
