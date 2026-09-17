package smart

import (
	"encoding/base64"
	"encoding/json"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"reflect"
	"strings"
	"time"
)

type historyCursor struct {
	Tool      string     `json:"tool"`
	Target    string     `json:"target"`
	Direction string     `json:"direction"`
	Start     *time.Time `json:"start,omitempty"`
	End       *time.Time `json:"end,omitempty"`
	Senders   []string   `json:"senders,omitempty"`
	Time      string     `json:"time"`
}

func historyBinding(rt *shortcut.RuntimeContext, r chatMessagesRequest) historyCursor {
	target, _ := json.Marshal(map[string]any{"group": r.params["openconversation_id"], "user": r.params["userId"], "openID": r.params["openDingTalkId"]})
	senders := append([]string{"mixed"}, rt.StrSlice("sender")...)
	senders = append(senders, "queries")
	senders = append(senders, rt.StrSlice("sender-query")...)
	return historyCursor{Tool: r.tool, Target: string(target), Direction: r.direction, Start: r.timeRange.start, End: r.timeRange.end, Senders: senders}
}
func restoreHistoryCursor(rt *shortcut.RuntimeContext, r *chatMessagesRequest) error {
	token := rt.Str("page-token")
	if token == "" {
		return nil
	}
	if len(token) > 8192 || !strings.HasPrefix(token, "dws-chat-v1.") {
		return apperrors.NewValidation("无效DWS历史page-token，不能使用Lark游标")
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, "dws-chat-v1."))
	if err != nil {
		return apperrors.NewValidation("历史page-token编码无效")
	}
	var saved historyCursor
	if json.Unmarshal(data, &saved) != nil {
		return apperrors.NewValidation("历史page-token结构无效")
	}
	current := historyBinding(rt, *r)
	equalTime := func(a, b *time.Time) bool { return a == nil && b == nil || a != nil && b != nil && a.Equal(*b) }
	if saved.Tool != current.Tool || saved.Target != current.Target || saved.Direction != current.Direction || !reflect.DeepEqual(saved.Senders, current.Senders) || !equalTime(saved.Start, current.Start) {
		return apperrors.NewValidation("历史page-token与会话、方向、起点或发送者条件不匹配")
	}
	if (rt.StrFirst("end", "end-time") != "") && !equalTime(saved.End, current.End) {
		return apperrors.NewValidation("历史page-token与结束时间不匹配")
	}
	if _, err := parseDingTalkMessageTime(saved.Time); err != nil {
		return apperrors.NewValidation("历史page-token缺少有效时间边界")
	}
	r.timeRange.end = saved.End
	r.params["time"] = saved.Time
	return nil
}
func attachHistoryCursor(rt *shortcut.RuntimeContext, r chatMessagesRequest, p map[string]any) {
	if p == nil {
		return
	}
	next, ok := p["nextPage"].(map[string]any)
	if !ok {
		return
	}
	boundary, ok := next["time"].(string)
	if !ok || boundary == "" {
		return
	}
	c := historyBinding(rt, r)
	c.Time = boundary
	data, err := json.Marshal(c)
	if err == nil {
		p["nextPageToken"] = "dws-chat-v1." + base64.RawURLEncoding.EncodeToString(data)
	}
}
