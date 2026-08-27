package doc

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestNormalizeShortcutBlockIDs(t *testing.T) {
	got, err := normalizeShortcutBlockIDs(" a , b ,a, ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(got, ",") != "a,b" {
		t.Fatalf("got %v, want [a b]", got)
	}
}

func TestNormalizeShortcutBlockIDsRejectsEmpty(t *testing.T) {
	if _, err := normalizeShortcutBlockIDs(" , "); err == nil {
		t.Fatal("blank block-id must be rejected")
	}
}

func TestNormalizeShortcutBlockIDsRejectsTooMany(t *testing.T) {
	ids := make([]string, 0, maxShortcutBlockIDsPerDelete+1)
	for i := 0; i <= maxShortcutBlockIDsPerDelete; i++ {
		ids = append(ids, fmt.Sprintf("block%d", i))
	}
	_, err := normalizeShortcutBlockIDs(strings.Join(ids, ","))
	if err == nil {
		t.Fatalf("%d block ids must be rejected", len(ids))
	}
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Reason != "block_id_too_many" {
		t.Fatalf("must fail with block_id_too_many, got %#v", err)
	}
}

func TestNormalizeShortcutBlockIDsAcceptsExactlyMax(t *testing.T) {
	ids := make([]string, 0, maxShortcutBlockIDsPerDelete)
	for i := 0; i < maxShortcutBlockIDsPerDelete; i++ {
		ids = append(ids, fmt.Sprintf("block%d", i))
	}
	got, err := normalizeShortcutBlockIDs(strings.Join(ids, ","))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != maxShortcutBlockIDsPerDelete {
		t.Fatalf("got %d ids, want %d", len(got), maxShortcutBlockIDsPerDelete)
	}
}

// TestUpdateBlockDeleteRejectsInvalidBlockID drives the Update shortcut through
// executeUpdate's block_delete branch so the guard after
// normalizeShortcutBlockIDs is exercised. A comma-only --block-id clears the
// non-empty preflight check but normalizes to zero ids, so the shortcut must
// return that validation error rather than attempting the delete.
func TestUpdateBlockDeleteRejectsInvalidBlockID(t *testing.T) {
	caller := &docCoverageCaller{responses: map[string][]map[string]any{}}
	err := runDocCoverage(t, Update, caller, "--node", "n", "--command", "block_delete", "--block-id", ",", "--yes")
	if err == nil {
		t.Fatal("comma-only --block-id must be rejected before the delete runs")
	}
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Reason != "block_id_empty" {
		t.Fatalf("must fail with block_id_empty, got %#v", err)
	}
}
