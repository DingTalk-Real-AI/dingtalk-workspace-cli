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
	"regexp"
	"strings"
)

const (
	// DWSChannelEnv is the environment variable carrying the upstream
	// channelCode, optionally followed by local-only tags.
	DWSChannelEnv = "DWS_CHANNEL"
	// DWSChannelTagHostControl moves PAT UI/polling ownership to the host while
	// keeping the upstream x-dws-channel header on the original channelCode.
	DWSChannelTagHostControl = "host-control"
)

var dwsTaggedChannelCodeRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
var dwsLocalTagRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// ChannelConfig represents the parsed DWS_CHANNEL specification.
//
// Grammar:
//
//	<channelCode>[;tag[;tag...]]
//
// Only channelCode is forwarded to upstream headers. Tags are local CLI hints.
// The parser is intentionally conservative: any malformed tagged form falls
// back to the raw input so upstream header semantics remain unchanged.
type ChannelConfig struct {
	Raw  string
	Code string
	Tags map[string]struct{}
}

// ParseChannelConfig parses a DWS_CHANNEL-style specification.
func ParseChannelConfig(raw string) ChannelConfig {
	cfg := ChannelConfig{
		Raw:  raw,
		Code: raw,
		Tags: map[string]struct{}{},
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		cfg.Code = ""
		return cfg
	}

	parts := strings.Split(trimmed, ";")
	if len(parts) == 1 {
		return cfg
	}
	code := strings.TrimSpace(parts[0])
	if code == "" || !dwsTaggedChannelCodeRE.MatchString(code) {
		return cfg
	}

	tags := make(map[string]struct{}, len(parts)-1)
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if !dwsLocalTagRE.MatchString(part) {
			return cfg
		}
		tags[strings.ToLower(part)] = struct{}{}
	}
	if len(tags) == 0 {
		return cfg
	}

	cfg.Code = code
	cfg.Tags = tags
	return cfg
}

// CurrentChannelConfig parses the current DWS_CHANNEL environment variable.
func CurrentChannelConfig() ChannelConfig {
	return ParseChannelConfig(os.Getenv(DWSChannelEnv))
}

// CurrentChannelCode returns only the upstream-safe channel code portion.
func CurrentChannelCode() string {
	return CurrentChannelConfig().Code
}

// HasTag reports whether a parsed local tag is present.
func (c ChannelConfig) HasTag(name string) bool {
	if c.Tags == nil {
		return false
	}
	_, ok := c.Tags[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// HostPATPassthroughEnabled reports whether PAT flow ownership should move to the host.
func (c ChannelConfig) HostPATPassthroughEnabled() bool {
	return c.HasTag(DWSChannelTagHostControl)
}

// ApplyChannelHeader injects the upstream-safe channel code into a request.
func ApplyChannelHeader(req *http.Request) {
	if req == nil {
		return
	}
	if ch := CurrentChannelCode(); ch != "" {
		req.Header.Set("x-dws-channel", ch)
	}
}
