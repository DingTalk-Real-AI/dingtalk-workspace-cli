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

package consume

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

// Route describes one --route rule. The CLI accepts the wire form
// `<regex>=dir:<path>`; ParseRoute compiles regex once at startup so the
// hot path is just a Match.
//
// Pattern matches against event.EventType (NOT the whole event JSON).
// The first matching rule in CLI order wins; unmatched events fall through
// to the default sink (stdout or --output-dir).
type Route struct {
	Pattern *regexp.Regexp
	Dir     string
	// Raw is the original CLI spec; preserved for status / debug output.
	Raw string
}

// ParseRoute parses one `<regex>=dir:<path>` spec. Returns a typed error
// for bad inputs so the CLI can render a clear "did you mean" message.
//
// Wire grammar:
//
//	spec   = regex "=dir:" path
//	regex  = any chars except literal "=" (use \= to escape)  (v1: no escape)
//	path   = any string (no validation here; sink validates at write time)
//
// Examples:
//
//	"^im\\.message=dir:./im/"
//	"^approval\\.=dir:./approval/"
func ParseRoute(spec string) (Route, error) {
	if spec == "" {
		return Route{}, fmt.Errorf("consume: empty route spec")
	}
	// v1 grammar is intentionally rigid: split on the first "=dir:".
	// Earlier proposals supported other sink kinds (=file: / =mcp:), but
	// the cobra layer rejects those — keep parsing tight here too.
	const sep = "=dir:"
	idx := strings.Index(spec, sep)
	if idx <= 0 || idx == len(spec)-len(sep) {
		return Route{}, fmt.Errorf("consume: route spec must be '<regex>=dir:<path>', got %q", spec)
	}
	pattern := spec[:idx]
	path := spec[idx+len(sep):]
	re, err := regexp.Compile(pattern)
	if err != nil {
		return Route{}, fmt.Errorf("consume: route regex %q: %w", pattern, err)
	}
	return Route{Pattern: re, Dir: path, Raw: spec}, nil
}

// ParseRoutes parses many specs in CLI order. On any parse failure returns
// the partial parse so far and the error — the caller decides whether to
// continue. (The cobra layer treats any parse error as fatal validation.)
func ParseRoutes(specs []string) ([]Route, error) {
	out := make([]Route, 0, len(specs))
	for _, s := range specs {
		r, err := ParseRoute(s)
		if err != nil {
			return out, err
		}
		out = append(out, r)
	}
	return out, nil
}

// Router decides which sink an event goes to. Match returns the directory
// of the first matching Route, or empty string when no rule matches (fall
// through to default sink).
type Router struct {
	rules []Route
}

// NewRouter constructs a router from pre-parsed rules.
func NewRouter(rules []Route) *Router { return &Router{rules: rules} }

// Match returns the destination directory for the event's type, or "" if
// no rule matched. Iterates rules in CLI order (first match wins).
func (r *Router) Match(ev transport.Event) string {
	if r == nil {
		return ""
	}
	for _, rule := range r.rules {
		if rule.Pattern.MatchString(ev.EventType) {
			return rule.Dir
		}
	}
	return ""
}

// Rules returns the parsed routes for status / debug output. Caller MUST
// NOT mutate the returned slice.
func (r *Router) Rules() []Route {
	if r == nil {
		return nil
	}
	return r.rules
}
