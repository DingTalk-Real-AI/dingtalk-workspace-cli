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

// Package convert provides shared type conversion helpers used across
// multiple internal packages (cli, compat).
package convert

// StringsToAny converts a string slice to []any.
func StringsToAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, v := range values {
		out = append(out, v)
	}
	return out
}

// IntsToAny converts an int slice to []any.
func IntsToAny(values []int) []any {
	out := make([]any, 0, len(values))
	for _, v := range values {
		out = append(out, v)
	}
	return out
}

// FloatsToAny converts a float64 slice to []any.
func FloatsToAny(values []float64) []any {
	out := make([]any, 0, len(values))
	for _, v := range values {
		out = append(out, v)
	}
	return out
}

// BoolsToAny converts a bool slice to []any.
func BoolsToAny(values []bool) []any {
	out := make([]any, 0, len(values))
	for _, v := range values {
		out = append(out, v)
	}
	return out
}

// ParseStringList parses each element of values using parse, returning
// the converted slice or the first parse error encountered.
func ParseStringList[T any](values []string, parse func(string) (T, error)) ([]T, error) {
	out := make([]T, 0, len(values))
	for _, v := range values {
		parsed, err := parse(v)
		if err != nil {
			return nil, err
		}
		out = append(out, parsed)
	}
	return out, nil
}
