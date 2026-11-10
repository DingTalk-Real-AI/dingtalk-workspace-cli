package helpers

import (
	"fmt"
	"os"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// prepareSuggestionNodeUUIDs checks only JSONML node attributes in the supplied
// SG block input. Document-wide uniqueness remains authoritative on the server.
func prepareSuggestionNodeUUIDs(cmd *cobra.Command, node []any, repair bool) error {
	if !docSuggestionEnabled(cmd) || cmd.Parent() == nil || cmd.Parent().Name() != "block" {
		return nil
	}
	if cmd.Name() != "insert" && cmd.Name() != "update" {
		return nil
	}
	targetUUID := ""
	if cmd.Name() == "update" {
		// The producer assigns this identity to the root. Reserve it before
		// checking descendants, even when the supplied root UUID differs.
		targetUUID, _ = cmd.Flags().GetString("block-id")
	}
	return prepareInputNodeUUIDs(node, repair, targetUUID)
}

func prepareInputNodeUUIDs(node []any, repair bool, targetUUID string) error {
	seen := make(map[string]struct{})
	if targetUUID != "" {
		seen[targetUUID] = struct{}{}
	}

	repaired := 0
	var visit func([]any, bool) error
	visit = func(current []any, root bool) error {
		if len(current) == 0 {
			return nil
		}
		if _, ok := current[0].(string); !ok {
			return nil
		}
		if len(current) > 1 {
			if attrs, ok := current[1].(map[string]any); ok {
				if id, ok := attrs["uuid"].(string); ok && id != "" {
					_, duplicate := seen[id]
					if duplicate && !(root && id == targetUUID) {
						if !repair {
							return apperrors.NewValidation(fmt.Sprintf("JSONML 输入中存在重复的节点 UUID %q；可通过 --fix-jsonml 为后续重复节点重新生成 UUID（UPDATE 目标节点身份保持不变）", id))
						}
						for {
							id = uuid.NewString()
							if _, used := seen[id]; !used {
								break
							}
						}
						attrs["uuid"] = id
						repaired++
					}
					seen[id] = struct{}{}
				}
			}
		}
		// Attribute objects (including metadata containing UUIDs or arrays) are
		// opaque; only actual JSONML child nodes participate in identity checks.
		for _, child := range current[jsonmlChildStart(current):] {
			if childNode, ok := child.([]any); ok {
				if err := visit(childNode, false); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := visit(node, true); err != nil {
		return err
	}
	if repaired > 0 {
		fmt.Fprintf(os.Stderr, "[WARN] 建议模式已为 %d 个重复节点重新生成 UUID，保留首次出现和 UPDATE 目标节点身份；与文档中既有 UUID 的冲突仍由服务端校验。\n", repaired)
	}
	return nil
}
