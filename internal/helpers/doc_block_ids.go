package helpers

import (
	"fmt"
	"strings"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

// MaxBlockIDsPerDelete caps one batch delete. The backend validator enforces the
// same limit; this front check exists so an oversized request fails locally with
// an actionable message instead of after a round trip.
const MaxBlockIDsPerDelete = 50

// NormalizeBlockIDs parses a --block-id value that accepts either a single
// block ID or several ASCII-comma-separated IDs ("a,b,c").
//
// It splits, trims, drops empty entries, and dedupes while preserving first-seen
// order. Deletion is keyed by block ID, so order carries no semantics — dedupe
// simply avoids asking the server to delete the same block twice.
func NormalizeBlockIDs(raw string) ([]string, error) {
	seen := make(map[string]struct{})
	ids := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		id := strings.TrimSpace(part)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return nil, apperrors.NewValidation(
			"--block-id 未提供有效的块 ID",
			apperrors.WithReason("block_id_empty"),
			apperrors.WithHint("传单个 ID，或用逗号分隔多个 ID，如 --block-id a,b,c"),
			apperrors.WithActions("执行 dws doc block list --node <nodeId> --format json 获取 blockId"),
		)
	}
	if len(ids) > MaxBlockIDsPerDelete {
		return nil, apperrors.NewValidation(
			fmt.Sprintf("--block-id 传入 %d 个块 ID，超过单次上限 %d", len(ids), MaxBlockIDsPerDelete),
			apperrors.WithReason("block_id_too_many"),
			apperrors.WithHint(fmt.Sprintf("拆成多次调用，每次不超过 %d 个", MaxBlockIDsPerDelete)),
		)
	}
	return ids, nil
}
