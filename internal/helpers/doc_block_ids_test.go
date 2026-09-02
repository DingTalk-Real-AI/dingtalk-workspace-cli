package helpers

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestNormalizeBlockIDs(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"single", "a", []string{"a"}},
		{"multiple", "a,b,c", []string{"a", "b", "c"}},
		{"trim", " a , b ,c ", []string{"a", "b", "c"}},
		{"drop empty", "a,,b,", []string{"a", "b"}},
		{"dedupe preserving order", "b,a,b,c,a", []string{"b", "a", "c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeBlockIDs(tc.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNormalizeBlockIDsRejectsEmpty(t *testing.T) {
	for _, raw := range []string{"", "   ", ",,", " , , "} {
		got, err := NormalizeBlockIDs(raw)
		if err == nil {
			t.Fatalf("raw %q must be rejected, got %v", raw, got)
		}
		var appErr *apperrors.Error
		if !errors.As(err, &appErr) || appErr.Reason != "block_id_empty" {
			t.Fatalf("raw %q must fail with block_id_empty, got %#v", raw, err)
		}
	}
}

func TestNormalizeBlockIDsRejectsTooMany(t *testing.T) {
	ids := make([]string, 0, MaxBlockIDsPerDelete+1)
	for i := 0; i <= MaxBlockIDsPerDelete; i++ {
		ids = append(ids, fmt.Sprintf("block%d", i))
	}
	_, err := NormalizeBlockIDs(strings.Join(ids, ","))
	if err == nil {
		t.Fatalf("%d block ids must be rejected", len(ids))
	}
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Reason != "block_id_too_many" {
		t.Fatalf("must fail with block_id_too_many, got %#v", err)
	}
}

func TestNormalizeBlockIDsAcceptsExactlyMax(t *testing.T) {
	ids := make([]string, 0, MaxBlockIDsPerDelete)
	for i := 0; i < MaxBlockIDsPerDelete; i++ {
		ids = append(ids, fmt.Sprintf("block%d", i))
	}
	got, err := NormalizeBlockIDs(strings.Join(ids, ","))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != MaxBlockIDsPerDelete {
		t.Fatalf("got %d ids, want %d", len(got), MaxBlockIDsPerDelete)
	}
}

// TestDocBlockDeleteCommandRejectsInvalidBlockID drives the `doc block delete`
// RunE so the guard after NormalizeBlockIDs is exercised end to end. A
// comma-only value is non-empty, so it clears validateRequiredFlags, but
// normalizes to zero ids — the command must return that validation error
// instead of calling delete_document_block.
func TestDocBlockDeleteCommandRejectsInvalidBlockID(t *testing.T) {
	err := runDocCoverageCommand(t, &scriptedToolCaller{}, "block", "delete", "--node=node", "--block-id=,", "--yes")
	if err == nil {
		t.Fatal("comma-only --block-id must be rejected before calling the MCP tool")
	}
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Reason != "block_id_empty" {
		t.Fatalf("must fail with block_id_empty, got %#v", err)
	}
}
