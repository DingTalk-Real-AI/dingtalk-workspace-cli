package chat

import (
	"fmt"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

func groupSearchNeedsScan(rt *shortcut.RuntimeContext) bool {
	return rt.Bool("page-all") || len(rt.StrSlice("member-ids")) > 0 || rt.Bool("is-manager") || len(rt.StrSlice("chat-modes")) > 0
}

func filterVerifiedGroups(rt *shortcut.RuntimeContext, groups []map[string]any) ([]map[string]any, error) {
	ids := uniqueShortcutStrings(rt.StrSlice("member-ids"))
	for _, id := range ids {
		if err := validateExplicitOpenIDs("--member-ids", []string{id}); err != nil {
			return nil, err
		}
	}
	self := ""
	if rt.Bool("is-manager") {
		profile, err := rt.CallMCPData("contact", "get_current_user_profile", nil)
		if err != nil {
			return nil, err
		}
		uid := currentProfileUserID(profile)
		if uid == "" {
			return nil, apperrors.NewAPI("当前用户缺少稳定userId")
		}
		self, err = resolveUserOpenDingTalkID(rt, uid)
		if err != nil {
			return nil, err
		}
	}
	requested := append([]string{}, ids...)
	if self != "" {
		requested = append(requested, self)
	}
	requested = uniqueShortcutStrings(requested)
	if len(requested) > 0 && len(groups) > 100 {
		return nil, apperrors.NewValidation("成员/角色筛选最多核验100个候选群，请缩小关键词范围")
	}
	modes := map[string]bool{}
	for _, m := range rt.StrSlice("chat-modes") {
		if m != "group" && m != "topic" {
			return nil, apperrors.NewValidation("--chat-modes 只支持group/topic")
		}
		modes[m] = true
	}
	out := []map[string]any{}
	for _, group := range groups {
		if len(modes) > 0 {
			channel, known := group["channel"].(bool)
			if !known {
				return nil, apperrors.NewAPI("群模式缺少可验证channel字段")
			}
			mode := "group"
			if channel {
				mode = "topic"
			}
			if !modes[mode] {
				continue
			}
		}
		if len(requested) > 0 {
			cid := shortcutString(group, "openConversationId", "conversationId")
			if cid == "" {
				return nil, apperrors.NewAPI("候选群缺少会话ID")
			}
			data, err := rt.CallMCPData("im", "list_group_member_by_ids", map[string]any{"openConversationId": cid, "memberOpenDingTalkIds": requested})
			if err != nil {
				return nil, err
			}
			members, err := StrictChatCollection(data, "members", "list", "items")
			if err != nil {
				return nil, err
			}
			found := map[string]map[string]any{}
			allowed := map[string]bool{}
			for _, id := range requested {
				allowed[id] = true
			}
			for _, m := range members {
				id := shortcutString(m, "openDingtalkId", "openDingTalkId")
				if id == "" || !allowed[id] {
					return nil, apperrors.NewAPI("成员精确查询返回非请求身份")
				}
				found[id] = m
			}
			match := true
			for _, id := range ids {
				if found[id] == nil {
					match = false
				}
			}
			if self != "" {
				m := found[self]
				if m == nil {
					match = false
				} else {
					role := fmt.Sprint(m["roleType"])
					if role == "<nil>" {
						return nil, apperrors.NewAPI("管理者筛选缺少角色字段")
					}
					if role != "1" && role != "2" {
						match = false
					}
				}
			}
			if !match {
				continue
			}
		}
		out = append(out, group)
	}
	return out, nil
}

// Sorting describes the returned set. It cannot repair upstream exhaustion.
func sortVerifiedGroups(rows []map[string]any, kind string, active bool) error {
	if kind == "" {
		return nil
	}
	key := "createAt"
	ascending := active
	switch kind {
	case "create_time":
	case "member_count":
		key = "memberCount"
		ascending = false
	case "active_time":
		if !active {
			return apperrors.NewValidation("群搜索没有已验证活跃排序字段")
		}
		key = "lastMsgCreateAt"
		ascending = false
	default:
		return apperrors.NewValidation("未知排序类型")
	}
	values := map[string]float64{}
	for _, row := range rows {
		id := shortcutString(row, "openConversationId", "conversationId")
		v, present := row[key]
		if id == "" || !present {
			return apperrors.NewAPI("排序缺少稳定ID或字段: " + key)
		}
		number, err := strconv.ParseFloat(fmt.Sprint(v), 64)
		if err != nil {
			t, e := time.Parse(time.RFC3339, strings.TrimSpace(fmt.Sprint(v)))
			if e != nil {
				t, e = time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(fmt.Sprint(v)), time.FixedZone("DingTalk", 8*60*60))
			}
			if e != nil {
				return apperrors.NewAPI("排序字段无法解析: " + key)
			}
			number = float64(t.UnixMilli())
		}
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return apperrors.NewAPI("排序字段不是有限数值: " + key)
		}
		values[id] = number
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a := shortcutString(rows[i], "openConversationId", "conversationId")
		b := shortcutString(rows[j], "openConversationId", "conversationId")
		if values[a] == values[b] {
			return a < b
		}
		if ascending {
			return values[a] < values[b]
		}
		return values[a] > values[b]
	})
	return nil
}
