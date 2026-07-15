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

package shortcut

import (
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// Cross-field validation helpers for use inside a Shortcut's Validate hook —
// the systematic cross-field validation (MutuallyExclusive /
// AtLeastOne / ExactlyOne / range checks) so per-command validation reads the
// same everywhere instead of being hand-rolled each time.

// setFlags returns the subset of the given flag names for which the user
// supplied a meaningful value. Empty strings and empty string slices do not
// satisfy cross-field constraints merely because the flag token was present.
func (rt *RuntimeContext) setFlags(flags ...string) []string {
	var set []string
	for _, name := range flags {
		if !rt.Changed(name) {
			continue
		}
		provided := true
		for _, flag := range rt.shortcut.Flags {
			if flag.Name != name {
				continue
			}
			switch flag.Type {
			case FlagStringSlice:
				provided = hasNonEmptyString(rt.StrSlice(name))
			case FlagString, "":
				provided = rt.Str(name) != ""
			}
			break
		}
		if provided {
			set = append(set, name)
		}
	}
	return set
}

func dashed(flags []string) string {
	out := make([]string, len(flags))
	for i, f := range flags {
		out[i] = "--" + f
	}
	return strings.Join(out, "、")
}

// MutuallyExclusive returns a validation error if more than one of the flags is
// set. Zero or one set is allowed.
func (rt *RuntimeContext) MutuallyExclusive(flags ...string) error {
	if set := rt.setFlags(flags...); len(set) > 1 {
		return apperrors.NewValidation(fmt.Sprintf(
			"参数 %s 互斥，只能指定其一（当前指定了 %s）", dashed(flags), dashed(set)))
	}
	return nil
}

// AtLeastOne returns a validation error if none of the flags is set.
func (rt *RuntimeContext) AtLeastOne(flags ...string) error {
	if len(rt.setFlags(flags...)) == 0 {
		return apperrors.NewValidation(fmt.Sprintf(
			"请至少指定 %s 之一", dashed(flags)))
	}
	return nil
}

// ExactlyOne returns a validation error unless exactly one of the flags is set.
func (rt *RuntimeContext) ExactlyOne(flags ...string) error {
	set := rt.setFlags(flags...)
	switch len(set) {
	case 1:
		return nil
	case 0:
		return apperrors.NewValidation(fmt.Sprintf("请指定 %s 之一", dashed(flags)))
	default:
		return apperrors.NewValidation(fmt.Sprintf(
			"参数 %s 只能指定其一（当前指定了 %s）", dashed(flags), dashed(set)))
	}
}

// RangeInt validates that an int flag (when set) is within [min, max].
func (rt *RuntimeContext) RangeInt(flag string, min, max int) error {
	if !rt.Changed(flag) {
		return nil
	}
	v := rt.Int(flag)
	if v < min || v > max {
		return apperrors.NewValidation(fmt.Sprintf(
			"参数 --%s 取值 %d 超出范围，应在 %d–%d 之间", flag, v, min, max))
	}
	return nil
}

// RequireAll returns a validation error if any of the flags is not set. Useful
// for "these flags come as a group" constraints (e.g. --start requires --end).
func (rt *RuntimeContext) RequireAll(flags ...string) error {
	var missing []string
	for _, f := range flags {
		if !rt.Changed(f) {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		return apperrors.NewValidation(fmt.Sprintf("缺少必填参数 %s", dashed(missing)))
	}
	return nil
}
