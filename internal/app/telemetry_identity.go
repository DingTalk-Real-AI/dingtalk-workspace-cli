// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"fmt"
	"strings"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
)

// TelemetryIdentity is the privacy-reviewed subset of the local authentication
// record that may be attached to a CLI execution event.
type TelemetryIdentity struct {
	UserID   string
	UserName string
	CorpID   string
}

var telemetryResolveProfileMetadata = authpkg.ResolveProfileMetadataReadOnly

// ResolveTelemetryIdentity returns a pre-execution snapshot of the identity
// selected by args. Multi-profile executions are attributed to the current
// default profile. Resolution is deliberately best-effort: telemetry must not
// refresh credentials or change command behavior when local auth data is
// missing, invalid, or unreadable.
func ResolveTelemetryIdentity(args []string) (identity TelemetryIdentity) {
	defer func() {
		if recover() != nil {
			identity = TelemetryIdentity{}
		}
	}()

	selector, specified, valid := preparseProfileSelection(args)
	if specified && !valid {
		return TelemetryIdentity{}
	}
	profile, err := resolveTelemetryProfileMetadata(defaultConfigDir(), selector)
	if err != nil || profile == nil {
		return TelemetryIdentity{}
	}
	return TelemetryIdentity{
		UserID:   strings.TrimSpace(profile.UserID),
		UserName: strings.TrimSpace(profile.UserName),
		CorpID:   strings.TrimSpace(profile.CorpID),
	}
}

func resolveTelemetryProfileMetadata(configDir, selector string) (*authpkg.ProfileMetadata, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" || !strings.Contains(selector, ",") {
		return telemetryResolveProfileMetadata(configDir, selector)
	}

	// A local profile name may itself contain a comma. Match the runtime
	// resolver by trying the full selector before interpreting it as CSV.
	if profile, err := telemetryResolveProfileMetadata(configDir, selector); err == nil && profile != nil {
		return profile, nil
	}

	for _, part := range strings.Split(selector, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("--profile contains an empty profile selector: %q", selector)
		}
		profile, err := telemetryResolveProfileMetadata(configDir, part)
		if err != nil {
			return nil, err
		}
		if profile == nil {
			return nil, fmt.Errorf("profile %q not found", part)
		}
	}
	return telemetryResolveProfileMetadata(configDir, "")
}
