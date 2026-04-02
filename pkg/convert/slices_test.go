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

package convert

import (
	"fmt"
	"strconv"
	"testing"
)

func TestStringsToAny(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  []string
		wantN  int
		wantAt map[int]any
	}{
		{
			name:  "empty slice",
			input: []string{},
			wantN: 0,
		},
		{
			name:   "single element",
			input:  []string{"hello"},
			wantN:  1,
			wantAt: map[int]any{0: "hello"},
		},
		{
			name:   "multiple elements",
			input:  []string{"a", "b", "c"},
			wantN:  3,
			wantAt: map[int]any{0: "a", 1: "b", 2: "c"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := StringsToAny(tc.input)
			if len(got) != tc.wantN {
				t.Fatalf("len = %d, want %d", len(got), tc.wantN)
			}
			for idx, want := range tc.wantAt {
				if got[idx] != want {
					t.Fatalf("got[%d] = %v, want %v", idx, got[idx], want)
				}
			}
		})
	}
}

func TestIntsToAny(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  []int
		wantN  int
		wantAt map[int]any
	}{
		{
			name:  "empty slice",
			input: []int{},
			wantN: 0,
		},
		{
			name:   "single element",
			input:  []int{42},
			wantN:  1,
			wantAt: map[int]any{0: 42},
		},
		{
			name:   "multiple elements",
			input:  []int{1, -2, 3},
			wantN:  3,
			wantAt: map[int]any{0: 1, 1: -2, 2: 3},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := IntsToAny(tc.input)
			if len(got) != tc.wantN {
				t.Fatalf("len = %d, want %d", len(got), tc.wantN)
			}
			for idx, want := range tc.wantAt {
				if got[idx] != want {
					t.Fatalf("got[%d] = %v, want %v", idx, got[idx], want)
				}
			}
		})
	}
}

func TestFloatsToAny(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  []float64
		wantN  int
		wantAt map[int]any
	}{
		{
			name:  "empty slice",
			input: []float64{},
			wantN: 0,
		},
		{
			name:   "single element",
			input:  []float64{3.14},
			wantN:  1,
			wantAt: map[int]any{0: 3.14},
		},
		{
			name:   "multiple elements",
			input:  []float64{1.1, 2.2, 3.3},
			wantN:  3,
			wantAt: map[int]any{0: 1.1, 1: 2.2, 2: 3.3},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := FloatsToAny(tc.input)
			if len(got) != tc.wantN {
				t.Fatalf("len = %d, want %d", len(got), tc.wantN)
			}
			for idx, want := range tc.wantAt {
				if got[idx] != want {
					t.Fatalf("got[%d] = %v, want %v", idx, got[idx], want)
				}
			}
		})
	}
}

func TestBoolsToAny(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  []bool
		wantN  int
		wantAt map[int]any
	}{
		{
			name:  "empty slice",
			input: []bool{},
			wantN: 0,
		},
		{
			name:   "single element",
			input:  []bool{true},
			wantN:  1,
			wantAt: map[int]any{0: true},
		},
		{
			name:   "multiple elements",
			input:  []bool{true, false, true},
			wantN:  3,
			wantAt: map[int]any{0: true, 1: false, 2: true},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := BoolsToAny(tc.input)
			if len(got) != tc.wantN {
				t.Fatalf("len = %d, want %d", len(got), tc.wantN)
			}
			for idx, want := range tc.wantAt {
				if got[idx] != want {
					t.Fatalf("got[%d] = %v, want %v", idx, got[idx], want)
				}
			}
		})
	}
}

func TestParseStringList(t *testing.T) {
	t.Parallel()

	t.Run("empty slice", func(t *testing.T) {
		t.Parallel()
		got, err := ParseStringList([]string{}, strconv.Atoi)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("len = %d, want 0", len(got))
		}
	})

	t.Run("successful parse", func(t *testing.T) {
		t.Parallel()
		got, err := ParseStringList([]string{"1", "2", "3"}, strconv.Atoi)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("len = %d, want 3", len(got))
		}
		want := []int{1, 2, 3}
		for i, v := range want {
			if got[i] != v {
				t.Fatalf("got[%d] = %d, want %d", i, got[i], v)
			}
		}
	})

	t.Run("parse error on first element", func(t *testing.T) {
		t.Parallel()
		_, err := ParseStringList([]string{"abc", "2", "3"}, strconv.Atoi)
		if err == nil {
			t.Fatal("expected error for non-numeric first element")
		}
	})

	t.Run("parse error on middle element", func(t *testing.T) {
		t.Parallel()
		_, err := ParseStringList([]string{"1", "not-a-number", "3"}, strconv.Atoi)
		if err == nil {
			t.Fatal("expected error for non-numeric middle element")
		}
	})

	t.Run("returns nil on error", func(t *testing.T) {
		t.Parallel()
		got, err := ParseStringList([]string{"bad"}, strconv.Atoi)
		if err == nil {
			t.Fatal("expected error")
		}
		if got != nil {
			t.Fatalf("expected nil result on error, got %v", got)
		}
	})

	t.Run("works with custom parser", func(t *testing.T) {
		t.Parallel()
		parser := func(s string) (float64, error) {
			v, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return 0, fmt.Errorf("parse %q: %w", s, err)
			}
			return v, nil
		}
		got, err := ParseStringList([]string{"1.5", "2.7"}, parser)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 || got[0] != 1.5 || got[1] != 2.7 {
			t.Fatalf("got %v, want [1.5, 2.7]", got)
		}
	})
}
