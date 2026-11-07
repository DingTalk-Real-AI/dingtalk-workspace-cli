package smart

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	chatshortcut "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/chatmsg"
)

func normalizeSearchConversationType(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "group", "group_chat":
		return "group_chat", nil
	case "p2p", "direct", "single_chat":
		return "single_chat", nil
	default:
		return "", apperrors.NewValidation("会话类型只支持group/p2p/group_chat/single_chat")
	}
}

func verifiedSearchItems(data map[string]any) ([]map[string]any, error) {
	grouped := data
	for i := 0; i < 8 && grouped != nil; i++ {
		if _, present := grouped["conversationMessagesList"]; present {
			groups, err := chatshortcut.StrictChatCollection(grouped, "conversationMessagesList")
			if err != nil {
				return nil, err
			}
			out := []map[string]any{}
			for _, group := range groups {
				rows, err := chatshortcut.StrictChatCollection(group, "messages")
				if err != nil {
					return nil, err
				}
				for _, row := range rows {
					if id := chatmsg.MessageID(row); id == nil || fmt.Sprint(id) == "" {
						return nil, apperrors.NewAPI("搜索项缺少消息身份")
					}
					clone := map[string]any{}
					for k, v := range row {
						clone[k] = v
					}
					if v, ok := group["singleChat"]; ok {
						clone["singleChat"] = v
					}
					// Thread search containers differ from owning message conversations.
					if id, ok := group["openConversationId"]; ok {
						clone["searchContainerId"] = id
						if own := chatmsg.ConversationID(row); own == nil || fmt.Sprint(own) == "" {
							clone["openConversationId"] = id
						}
					}
					if name, ok := group["title"]; ok {
						clone["conversationName"] = name
					}
					out = append(out, clone)
				}
			}
			return out, nil
		}
		child, ok := grouped["result"].(map[string]any)
		if !ok {
			break
		}
		grouped = child
	}
	rows, err := chatshortcut.StrictChatCollection(data, "messages", "items", "list")
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if id := chatmsg.MessageID(row); id == nil || fmt.Sprint(id) == "" {
			return nil, apperrors.NewAPI("搜索项缺少消息身份")
		}
	}
	return rows, nil
}

func verifySearchResultFilters(rt *shortcut.RuntimeContext, params map[string]any, messages []map[string]any) ([]map[string]any, error) {
	kind, _ := params["searchConvType"].(string)
	fileFilter := params["messageType"] == "file"
	if kind == "" && !fileFilter {
		return messages, nil
	}
	out := []map[string]any{}
	for _, m := range messages {
		if fileFilter {
			found := false
			for _, r := range chatmsg.Resources(m) {
				if r["contentType"] == "file" || r["type"] == "fileId" {
					found = true
				}
			}
			if !found {
				return nil, apperrors.NewAPI("文件筛选返回项缺少可验证文件资源", apperrors.WithReason("search_file_unverified"))
			}
		}
		if kind == "" {
			out = append(out, m)
			continue
		}
		single, known := m["singleChat"].(bool)
		if !known {
			return nil, apperrors.NewAPI("下游缺少singleChat，无法验证会话类型筛选", apperrors.WithReason("search_type_unverified"))
		}
		if single == (kind == "single_chat") {
			out = append(out, m)
		}
	}
	return out, nil
}

func verifiedGroupCreationTime(rt *shortcut.RuntimeContext, groupID string) (time.Time, error) {
	data, err := rt.CallMCPData("chat", "get_conversation_info", map[string]any{"openConversationId": groupID})
	if err != nil {
		return time.Time{}, err
	}
	for i := 0; i < 8; i++ {
		if id := chatmsg.ConversationID(data); id != nil && fmt.Sprint(id) == groupID {
			raw := fmt.Sprint(data["createAt"])
			if key, _, e := chatMessagesNextCursorBoundary(data["createAt"]); e == nil {
				ms, _ := strconv.ParseInt(key, 10, 64)
				return time.UnixMilli(ms), nil
			}
			if t, e := parseDingTalkMessageTime(raw); e == nil {
				return t, nil
			}
		}
		child, ok := data["result"].(map[string]any)
		if !ok {
			child, ok = data["conversationInfo"].(map[string]any)
		}
		if !ok {
			break
		}
		data = child
	}
	return time.Time{}, apperrors.NewAPI("无法精确验证群创建时间，请提供--start")
}
