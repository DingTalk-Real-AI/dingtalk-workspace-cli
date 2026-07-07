package helpers

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func resolveMessageForward(cmd *cobra.Command, defaultForward bool) (bool, error) {
	forwardStr, _ := cmd.Flags().GetString("forward")
	forward := forwardStr != "false"
	if !cmd.Flags().Changed("direction") {
		if !cmd.Flags().Changed("forward") {
			return defaultForward, nil
		}
		return forward, nil
	}

	direction, _ := cmd.Flags().GetString("direction")
	switch strings.TrimSpace(strings.ToLower(direction)) {
	case "newer":
		if cmd.Flags().Changed("forward") && !forward {
			return false, fmt.Errorf("--direction newer conflicts with --forward=false")
		}
		return true, nil
	case "older":
		if cmd.Flags().Changed("forward") && forward {
			return false, fmt.Errorf("--direction older conflicts with --forward=true")
		}
		return false, nil
	case "":
		return defaultForward, nil
	default:
		return false, fmt.Errorf("--direction must be newer or older")
	}
}

func chatIntFlagOrFallback(cmd *cobra.Command, primary string, aliases ...string) int {
	for _, alias := range aliases {
		if f := cmd.Flags().Lookup(alias); f != nil && f.Changed {
			v, _ := cmd.Flags().GetInt(alias)
			return v
		}
	}
	v, _ := cmd.Flags().GetInt(primary)
	return v
}

func runChatSearchCommon(cmd *cobra.Command, _ []string) error {
	if err := validateRequiredFlags(cmd, "nicks"); err != nil {
		return err
	}
	nicks := parseCSVValues(mustGetFlag(cmd, "nicks"))
	limit := chatIntFlagOrFallback(cmd, "limit", "size")
	cursor, _ := cmd.Flags().GetString("cursor")
	matchMode, _ := cmd.Flags().GetString("match-mode")
	toolArgs := map[string]any{
		"nicks":     nicks,
		"matchMode": matchMode,
		"limit":     limit,
		"cursor":    cursor,
	}
	if v, _ := cmd.Flags().GetBool("exclude-muted"); v {
		toolArgs["excludeMuted"] = true
	}
	return callMCPTool("search_common_groups", toolArgs)
}

// sanitizeTitleFromText derives a safe title from message text.
// When the user doesn't provide --title, the text content (which may contain
// URLs with percent-encoded characters like %3D, %26) is used as title.
// The title field has stricter validation on the server side (128 bytes max),
// so we strip or truncate content that is likely to be rejected.
func sanitizeTitleFromText(text string) (title string) {
	const maxTitleBytes = 100
	const maxTitleRunes = 30 // conservative: 30 CJK chars = 90 bytes, leaving room for "..."
	const fallbackTitle = "消息"

	defer func() {
		if r := recover(); r != nil {
			title = fallbackTitle
		}
	}()

	if strings.TrimSpace(text) == "" {
		return fallbackTitle
	}

	// If text contains a URL, use only the portion before the URL as title.
	for _, prefix := range []string{"https://", "http://"} {
		if idx := strings.Index(text, prefix); idx > 0 {
			candidate := strings.TrimSpace(text[:idx])
			if candidate != "" {
				return truncateTitleToBytes(candidate, maxTitleBytes, maxTitleRunes)
			}
		}
	}

	// If the entire text is a URL, use a generic title.
	if strings.HasPrefix(text, "https://") || strings.HasPrefix(text, "http://") {
		return fallbackTitle
	}

	// Final fallback: if the result contains percent-encoded sequences
	// that the server may reject, use a generic title.
	if strings.Contains(text, "%") {
		return fallbackTitle
	}

	// No URL — truncate to safe length.
	return truncateTitleToBytes(text, maxTitleBytes, maxTitleRunes)
}

// truncateTitleToBytes truncates a title string ensuring it doesn't exceed
// maxBytes in UTF-8 encoding. It also limits by rune count for readability.
func truncateTitleToBytes(s string, maxBytes, maxRunes int) string {
	runes := []rune(s)
	if len(runes) > maxRunes {
		runes = runes[:maxRunes]
	}
	result := string(runes)
	// Ensure we don't exceed byte limit
	for len(result) > maxBytes-3 { // reserve 3 bytes for "..."
		runes = runes[:len(runes)-1]
		result = string(runes)
	}
	if len([]rune(s)) > len(runes) {
		return result + "..."
	}
	return result
}

// marshalJSONRaw serializes v to JSON without escaping <, >, &.
func marshalJSONRaw(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	// Encode appends a trailing newline; trim it.
	b := buf.Bytes()
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	return b, nil
}

// parseCSVInt64 parses a comma-separated string of integers into []int64.
func parseCSVInt64(raw string) ([]int64, error) {
	parts := strings.Split(raw, ",")
	result := make([]int64, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid integer %q", v)
		}
		result = append(result, n)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("at least one ID is required")
	}
	return result, nil
}

func isNumericUserID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// dingTalkMediaIDPattern 匹配钉钉媒体 ID（上传接口返回），固定以 @ 开头，
// 后跟 base64 风格字符（如 @lADPfake、@lQDPM4DCjE1Mk_nNBInNBImw...）。
var dingTalkMediaIDPattern = regexp.MustCompile(`^@[A-Za-z0-9_\-+/=.]{4,}$`)

func isOpenDingTalkID(value string) bool {
	value = strings.TrimSpace(value)
	return len(value) > 0 && (value[0] == 'D' || value[0] == 'd')
}

func splitChatIDValues(values []string) (userIDs []string, openDingTalkIDs []string) {
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if isOpenDingTalkID(value) {
			openDingTalkIDs = append(openDingTalkIDs, value)
		} else {
			userIDs = append(userIDs, value)
		}
	}
	return userIDs, openDingTalkIDs
}

func appendStringSliceArg(args map[string]any, key string, values []string) {
	if len(values) == 0 {
		return
	}
	if existing, ok := args[key].([]string); ok {
		args[key] = append(existing, values...)
		return
	}
	args[key] = values
}

func appendChatIDArgs(args map[string]any, values []string, userIDKey, openDingTalkIDKey string) bool {
	userIDs, openDingTalkIDs := splitChatIDValues(values)
	appendStringSliceArg(args, userIDKey, userIDs)
	appendStringSliceArg(args, openDingTalkIDKey, openDingTalkIDs)
	return len(userIDs) > 0
}

// normalizeAtPlaceholders 统一文本中针对 ids 的 @ 占位符格式，消化 send 与 send-by-bot 之间的差异。
// wrapAngle=true：用户身份发消息（send），裸 @id 自动包成 <@id>；已有 <@id> 保持不变。
// wrapAngle=false：机器人发消息（send-by-bot），<@id> 自动剥离为 @id。
// 仅替换 ids 中实际声明的标识，避免误伤 markdown 中其他 <...> 内容。
func normalizeAtPlaceholders(text string, ids []string, wrapAngle bool) string {
	const sentinelPrefix = "\x00DWS_AT_WRAPPED_"
	const sentinelSuffix = "\x00"
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		wrapped := "<@" + id + ">"
		bare := "@" + id
		if wrapAngle {
			sentinel := sentinelPrefix + id + sentinelSuffix
			text = strings.ReplaceAll(text, wrapped, sentinel)
			text = strings.ReplaceAll(text, bare, wrapped)
			text = strings.ReplaceAll(text, sentinel, wrapped)
		} else {
			text = strings.ReplaceAll(text, wrapped, bare)
		}
	}
	return text
}

func resolveOpenDingTalkID(ctx context.Context, value string) (string, error) {
	ids, err := resolveOpenDingTalkIDs(ctx, []string{value})
	if err != nil {
		return "", err
	}
	if len(ids) == 0 || ids[0] == "" {
		return "", fmt.Errorf("empty user identifier")
	}
	return ids[0], nil
}

func resolveOpenDingTalkIDs(ctx context.Context, values []string) ([]string, error) {
	resolved := make([]string, len(values))
	userIDs := make([]string, 0, len(values))
	userIDIndexes := make([]int, 0, len(values))
	seenUserIDs := make(map[string]bool)

	for i, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if isOpenDingTalkID(value) {
			resolved[i] = value
		} else {
			userIDIndexes = append(userIDIndexes, i)
			if !seenUserIDs[value] {
				seenUserIDs[value] = true
				userIDs = append(userIDs, value)
			}
		}
	}

	if len(userIDs) == 0 {
		return resolved, nil
	}

	openByUserID, err := lookupOpenDingTalkIDsByUserID(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	for _, index := range userIDIndexes {
		userID := strings.TrimSpace(values[index])
		openDingTalkID := openByUserID[userID]
		if openDingTalkID == "" {
			return nil, fmt.Errorf("cannot resolve userId %q to openDingTalkId; hint: run dws contact user search --keyword \"姓名\" --format json and pass the returned openDingTalkId", userID)
		}
		resolved[index] = openDingTalkID
	}
	return resolved, nil
}

// groupOwnerOpenDingTalkID 分页拉取群成员，返回群主（memberRoleType==1）的 openDingtalkId。
// 找不到群主或群主字段为空时返回 ""（由调用方决定是否放行）。
func groupOwnerOpenDingTalkID(ctx context.Context, openConversationID string) (string, error) {
	cursor := "0"
	for page := 0; page < 50; page++ { // 防御性上限，避免异常分页导致死循环
		raw, err := callMCPToolReturnText(ctx, "get_group_members", map[string]any{
			"openconversation_id": openConversationID,
			"cursor":              cursor,
		})
		if err != nil {
			return "", err
		}
		var body struct {
			Result struct {
				HasMore    bool   `json:"hasMore"`
				NextCursor string `json:"nextCursor"`
				Cursor     string `json:"cursor"`
				List       []struct {
					MemberRoleType int    `json:"memberRoleType"`
					OpenDingtalkID string `json:"openDingtalkId"`
				} `json:"list"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(raw), &body); err != nil {
			return "", err
		}
		for _, m := range body.Result.List {
			if m.MemberRoleType == 1 {
				return m.OpenDingtalkID, nil
			}
		}
		next := body.Result.NextCursor
		if next == "" {
			next = body.Result.Cursor
		}
		if !body.Result.HasMore || next == "" || next == cursor {
			break
		}
		cursor = next
	}
	return "", nil
}

// guardGroupOwnerRemoval 拦截把群主移出群的操作：一个群只有一个群主，移出群主会产生
// 无群主的“孤儿群”。防护为尽力而为：群主信息查询失败或 userId→openDingTalkId 解析
// 失败时不阻塞正常移除路径（由服务端兜底）。
func guardGroupOwnerRemoval(ctx context.Context, openConversationID string, removeValues []string) error {
	ownerOpenID, err := groupOwnerOpenDingTalkID(ctx, openConversationID)
	if err != nil || ownerOpenID == "" {
		return nil
	}
	ownerErr := fmt.Errorf(
		"refusing to remove the group owner: 被移除列表包含群主，移出群主将导致群无群主（孤儿群）\n  hint: 先执行 dws chat group transfer-owner --group %s --user <newOwnerUserId> 转让群主后再移除",
		openConversationID,
	)
	userIDs, openDingTalkIDs := splitChatIDValues(removeValues)
	for _, id := range openDingTalkIDs {
		if id == ownerOpenID {
			return ownerErr
		}
	}
	if len(userIDs) > 0 {
		if resolved, resolveErr := resolveOpenDingTalkIDs(ctx, userIDs); resolveErr == nil {
			for _, id := range resolved {
				if id == ownerOpenID {
					return ownerErr
				}
			}
		}
	}
	return nil
}

func lookupOpenDingTalkIDsByUserID(ctx context.Context, userIDs []string) (map[string]string, error) {
	mapping := map[string]string{}
	namesByUserID := map[string]string{}

	// Step 1: Try contact service to get openDingTalkId and username by userId.
	raw, err := callMCPToolReturnTextOnServer(ctx, "contact", "get_user_info_by_user_ids", map[string]any{
		"user_id_list": userIDs,
	})
	if err == nil {
		var body any
		dec := json.NewDecoder(strings.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&body); err != nil {
			return nil, fmt.Errorf("failed to parse contact user get response: %w", err)
		}
		collectContactUserMappings(body, mapping, namesByUserID)
	}
	if allUserIDsMapped(userIDs, mapping) {
		return mapping, nil
	}

	// Step 2: For unmapped userIds, use aisearch with username (not userId) as keyword.
	for _, userID := range userIDs {
		if userID == "" || mapping[userID] != "" {
			continue
		}
		keyword := namesByUserID[userID]
		if keyword == "" {
			// No username resolved from contact; fall through to keyword search below.
			continue
		}
		_ = lookupOpenDingTalkIDsByAisearchPerson(ctx, keyword, mapping, namesByUserID)
	}
	if allUserIDsMapped(userIDs, mapping) {
		return mapping, nil
	}

	// Step 3: Fallback - search contact by username or userId keyword.
	for _, userID := range userIDs {
		if mapping[userID] != "" {
			continue
		}
		keywords := []string{}
		if name := namesByUserID[userID]; name != "" {
			keywords = append(keywords, name)
		}
		keywords = append(keywords, userID)

		searched := map[string]bool{}
		for _, keyword := range keywords {
			if keyword == "" || searched[keyword] {
				continue
			}
			searched[keyword] = true
			searchRaw, err := callMCPToolReturnTextOnServer(ctx, "contact", "search_contact_by_key_word", map[string]any{
				"keyword": keyword,
			})
			if err != nil {
				return nil, err
			}
			var searchBody any
			searchDec := json.NewDecoder(strings.NewReader(searchRaw))
			searchDec.UseNumber()
			if err := searchDec.Decode(&searchBody); err != nil {
				return nil, fmt.Errorf("failed to parse contact user search response: %w", err)
			}
			collectContactUserMappings(searchBody, mapping, namesByUserID)
			if mapping[userID] != "" {
				break
			}
		}
	}
	return mapping, nil
}

func allUserIDsMapped(userIDs []string, mapping map[string]string) bool {
	for _, userID := range userIDs {
		if strings.TrimSpace(userID) != "" && mapping[userID] == "" {
			return false
		}
	}
	return true
}

func lookupOpenDingTalkIDsByAisearchPerson(ctx context.Context, keyword string, openByUserID map[string]string, nameByUserID map[string]string) error {
	raw, err := callMCPToolReturnTextOnServer(ctx, "aisearch", "enterprise_person_search", map[string]any{
		"keyword":   keyword,
		"dimension": []string{"all"},
	})
	if err != nil {
		return err
	}
	var body any
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&body); err != nil {
		return fmt.Errorf("failed to parse aisearch person response: %w", err)
	}
	collectContactUserMappings(body, openByUserID, nameByUserID)
	return nil
}

var (
	chatUserIDJSONKeys = map[string]bool{
		"userid":      true,
		"user_id":     true,
		"orguserid":   true,
		"org_user_id": true,
		"uid":         true,
		"staffid":     true,
		"staff_id":    true,
	}
	chatOpenDingTalkIDJSONKeys = map[string]bool{
		"opendingtalkid":   true,
		"opendingtalk_id":  true,
		"open_dingtalk_id": true,
		"opendingid":       true,
	}
	chatUserNameJSONKeys = map[string]bool{
		"name":          true,
		"username":      true,
		"user_name":     true,
		"orgusername":   true,
		"org_user_name": true,
		"displayname":   true,
		"display_name":  true,
		"nick":          true,
	}
	chatContactNestedUserKeys = []string{"orgEmployeeModel", "employee", "user", "profile", "staff"}
)

func collectContactUserMappings(value any, openByUserID map[string]string, nameByUserID map[string]string) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			collectContactUserMappings(item, openByUserID, nameByUserID)
		}
	case map[string]any:
		userID := stringForJSONKeys(typed, chatUserIDJSONKeys)
		openDingTalkID := stringForJSONKeys(typed, chatOpenDingTalkIDJSONKeys)
		userName := stringForJSONKeys(typed, chatUserNameJSONKeys)
		if userID == "" {
			userID = stringForNestedJSONKeys(typed, chatContactNestedUserKeys, chatUserIDJSONKeys)
		}
		if openDingTalkID == "" {
			openDingTalkID = stringForNestedJSONKeys(typed, chatContactNestedUserKeys, chatOpenDingTalkIDJSONKeys)
		}
		if userName == "" {
			userName = stringForNestedJSONKeys(typed, chatContactNestedUserKeys, chatUserNameJSONKeys)
		}
		if userID != "" && openDingTalkID != "" {
			openByUserID[userID] = openDingTalkID
		}
		if userID != "" && userName != "" {
			nameByUserID[userID] = userName
		}
		for _, item := range typed {
			collectContactUserMappings(item, openByUserID, nameByUserID)
		}
	}
}

func stringForNestedJSONKeys(value map[string]any, nestedKeys []string, keys map[string]bool) string {
	for _, nestedKey := range nestedKeys {
		if nested, ok := value[nestedKey].(map[string]any); ok {
			if found := stringForJSONKeys(nested, keys); found != "" {
				return found
			}
		}
	}
	return ""
}

func stringForJSONKeys(value map[string]any, keys map[string]bool) string {
	for key, raw := range value {
		if !keys[strings.ToLower(key)] {
			continue
		}
		if str := stringFromJSONScalar(raw); str != "" {
			return str
		}
	}
	return ""
}

func stringFromJSONScalar(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	default:
		return ""
	}
}

func buildConversationTargetArgs(cmd *cobra.Command) (map[string]any, error) {
	groupID := flagOrFallback(cmd, "group", "conversation-id", "id", "chat")
	rawOpenDingTalkID, _ := cmd.Flags().GetString("open-dingtalk-id")
	rawUserID := flagOrFallback(cmd, "user", "userId")

	// All three params are passed through to the server; the server decides which to use.
	userID := rawUserID
	openDingTalkID := rawOpenDingTalkID
	if openDingTalkID != "" && isNumericUserID(openDingTalkID) {
		userID = openDingTalkID
		openDingTalkID = ""
	}
	if userID != "" && !isNumericUserID(userID) {
		openDingTalkID = userID
		userID = ""
	}

	toolArgs := map[string]any{}
	if groupID != "" {
		toolArgs["openConversationId"] = groupID
	}
	if userID != "" {
		toolArgs["userId"] = userID
	}
	if openDingTalkID != "" {
		toolArgs["openDingTalkId"] = openDingTalkID
	}
	return toolArgs, nil
}

var chatValidGrantTypes = map[string]bool{
	"once":      true,
	"session":   true,
	"timed":     true,
	"permanent": true,
}

func validateChatScope(scope string) error {
	if !strings.HasPrefix(scope, "chat.") {
		return fmt.Errorf("invalid scope %q, dws chat chmod only accepts chat.* scope", scope)
	}
	return nil
}

func buildChatGrantBaseArgs(cmd *cobra.Command, scope string) (map[string]any, error) {
	grantType, _ := cmd.Flags().GetString("grant-type")
	if !chatValidGrantTypes[grantType] {
		return nil, fmt.Errorf("invalid --grant-type %q, must be one of: once, session, timed, permanent", grantType)
	}
	ttl, _ := cmd.Flags().GetString("ttl")
	sessionID, _ := cmd.Flags().GetString("session-id")
	if grantType == "timed" && ttl == "" {
		return nil, fmt.Errorf("--ttl is required when --grant-type is timed")
	}
	if grantType == "session" && sessionID == "" {
		return nil, fmt.Errorf("--session-id is required when --grant-type is session")
	}
	toolArgs := map[string]any{
		"agentCode": mustGetFlag(cmd, "agentCode"),
		"scope":     scope,
		"grantType": grantType,
	}
	if grantType == "timed" {
		toolArgs["ttl"] = ttl
	}
	if sessionID != "" {
		toolArgs["sessionId"] = sessionID
	}
	return toolArgs, nil
}

func buildChatChmodArgs(cmd *cobra.Command, scope string) (map[string]any, error) {
	toolArgs, err := buildChatGrantBaseArgs(cmd, scope)
	if err != nil {
		return nil, err
	}
	if err := appendChatChmodParams(cmd, toolArgs); err != nil {
		return nil, err
	}
	return toolArgs, nil
}

func buildChatCrossOrgDataAuthArgs(cmd *cobra.Command) (map[string]any, error) {
	targetOrgID := strings.TrimSpace(mustGetFlag(cmd, "target-org-id"))
	all, err := cmd.Flags().GetBool("all")
	if err != nil {
		return nil, err
	}
	if targetOrgID == "" && !all {
		return nil, fmt.Errorf("--target-org-id or --all is required")
	}
	if targetOrgID != "" && all {
		return nil, fmt.Errorf("--target-org-id and --all cannot be used together")
	}
	if all {
		targetOrgID = "*"
	}
	toolArgs, err := buildChatGrantBaseArgs(cmd, "chat.data:cross-org")
	if err != nil {
		return nil, err
	}
	toolArgs["grantCategory"] = "data"
	paramsJSON, err := marshalJSONRaw(map[string]string{"targetOrgId": targetOrgID})
	if err != nil {
		return nil, err
	}
	toolArgs["grantParams"] = string(paramsJSON)
	return toolArgs, nil
}

func appendChatChmodParams(cmd *cobra.Command, toolArgs map[string]any) error {
	conversationID, _ := cmd.Flags().GetString("conversation-id")
	openDingTalkID, _ := cmd.Flags().GetString("open-dingtalk-id")
	userID, _ := cmd.Flags().GetString("user")
	rawParams, _ := cmd.Flags().GetStringArray("permParam")
	grantParams, err := parseChatChmodParams(rawParams)
	if err != nil {
		return err
	}
	specified := 0
	for _, value := range []string{conversationID, openDingTalkID, userID} {
		if strings.TrimSpace(value) != "" {
			specified++
		}
	}
	if specified > 1 {
		return fmt.Errorf("--conversation-id, --open-dingtalk-id and --user are mutually exclusive")
	}
	if conversationID != "" {
		putChatChmodParam(grantParams, "conversationId", conversationID)
		putChatChmodParam(grantParams, "openConversationId", conversationID)
		putChatChmodParam(grantParams, "openCid", conversationID)
	} else if openDingTalkID != "" {
		putChatChmodParam(grantParams, "openDingTalkId", openDingTalkID)
	} else {
		if userID != "" {
			putChatChmodParam(grantParams, "targetUid", userID)
			putChatChmodParam(grantParams, "receiverUid", userID)
		}
	}
	if specified == 0 && len(grantParams) == 0 {
		return fmt.Errorf("--conversation-id, --open-dingtalk-id, --user or --permParam is required")
	}
	paramsJSON, err := marshalJSONRaw(grantParams)
	if err != nil {
		return err
	}
	toolArgs["grantParams"] = string(paramsJSON)
	return nil
}

func parseChatChmodParams(values []string) (map[string]string, error) {
	params := map[string]string{}
	for _, raw := range values {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("--permParam must be key=value, got %q", raw)
		}
		params[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return params, nil
}

func putChatChmodParam(params map[string]string, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	if _, exists := params[key]; !exists {
		params[key] = strings.TrimSpace(value)
	}
}

func fileMD5Hex(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file for md5: %w", err)
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to calculate md5: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func parseConversationFileUploadInfo(text string) (resourceURL, uploadKey string, headers map[string]string, err error) {
	var data map[string]any
	if err = unmarshalJSONUseNumber(text, &data); err != nil {
		return "", "", nil, fmt.Errorf("failed to parse upload credentials JSON: %w", err)
	}
	if result, ok := data["result"].(map[string]any); ok {
		data = result
	}

	resourceURL = firstStringField(data, "resourceUrl", "resourceURL", "url")
	if resourceURL == "" {
		if values, ok := data["resourceUrls"].([]any); ok && len(values) > 0 {
			resourceURL = stringFromJSONScalar(values[0])
		}
	}
	uploadKey = firstStringField(data, "uploadKey", "key")
	if resourceURL == "" || uploadKey == "" {
		return "", "", nil, fmt.Errorf("incomplete upload credentials: resourceUrl=%q, uploadKey=%q", resourceURL, uploadKey)
	}

	headers = map[string]string{}
	for _, key := range []string{"headers", "ossHeaders"} {
		if h, ok := data[key].(map[string]any); ok {
			for name, value := range h {
				if s := stringFromJSONScalar(value); s != "" {
					headers[name] = s
				}
			}
		}
	}
	return resourceURL, uploadKey, headers, nil
}

type conversationLocalFileMeta struct {
	LocalPath   string
	FileName    string
	FileType    string
	ContentPath string
	FileSize    int64
	MD5         string
}

func buildConversationLocalFileMeta(filePath, fileName, md5Value string) (conversationLocalFileMeta, error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return conversationLocalFileMeta{}, fmt.Errorf("cannot read file %s: %w", filePath, err)
	}
	if fi.IsDir() {
		return conversationLocalFileMeta{}, fmt.Errorf("%s is a directory, not a file", filePath)
	}
	if fileName == "" {
		fileName = filepath.Base(filePath)
	}
	fileType := strings.TrimPrefix(filepath.Ext(fileName), ".")
	if md5Value == "" {
		md5Value, err = fileMD5Hex(filePath)
		if err != nil {
			return conversationLocalFileMeta{}, err
		}
	}
	return conversationLocalFileMeta{
		LocalPath:   filePath,
		FileName:    fileName,
		FileType:    fileType,
		ContentPath: "/" + fileName,
		FileSize:    fi.Size(),
		MD5:         md5Value,
	}, nil
}

func uploadConversationLocalFile(ctx context.Context, targetArgs map[string]any, meta conversationLocalFileMeta, uuid string) (string, error) {
	initArgs := cloneStringAnyMap(targetArgs)
	initArgs["fileName"] = meta.FileName
	initArgs["fileSize"] = meta.FileSize
	initArgs["md5"] = meta.MD5
	if uuid != "" {
		initArgs["uuid"] = uuid
	}

	initText, err := callMCPToolReturnTextOnServer(ctx, "im", "init_conversation_file_upload", initArgs)
	if err != nil {
		return "", err
	}
	resourceURL, uploadKey, headers, err := parseConversationFileUploadInfo(initText)
	if err != nil {
		return "", err
	}
	if err := httpPutFile(ctx, resourceURL, headers, meta.LocalPath, meta.FileSize); err != nil {
		return "", err
	}

	commitArgs := cloneStringAnyMap(targetArgs)
	commitArgs["uploadKey"] = uploadKey
	commitArgs["fileName"] = meta.FileName
	commitArgs["fileSize"] = meta.FileSize
	commitArgs["md5"] = meta.MD5
	if uuid != "" {
		commitArgs["uuid"] = uuid
	}
	return callMCPToolReturnTextOnServer(ctx, "im", "commit_conversation_file_upload", commitArgs)
}

func buildConversationFileContent(dentryID, spaceID int64, meta conversationLocalFileMeta) (string, error) {
	content := struct {
		DentryID int64  `json:"dentryId"`
		SpaceID  int64  `json:"spaceId"`
		FileName string `json:"fileName"`
		FileType string `json:"fileType"`
		FilePath string `json:"filePath"`
		FileSize int64  `json:"fileSize"`
	}{
		DentryID: dentryID,
		SpaceID:  spaceID,
		FileName: meta.FileName,
		FileType: meta.FileType,
		FilePath: meta.ContentPath,
		FileSize: meta.FileSize,
	}
	body, err := marshalJSONRaw(content)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func parseConversationFileSendIDs(text string) (int64, int64, error) {
	var data any
	if err := unmarshalJSONUseNumber(text, &data); err != nil {
		return 0, 0, fmt.Errorf("failed to parse uploaded file response JSON: %w", err)
	}
	dentryID, _ := findInt64Field(data, "dentryId", "dentryID")
	spaceID, _ := findInt64Field(data, "spaceId", "spaceID")
	if dentryID == 0 || spaceID == 0 {
		return 0, 0, fmt.Errorf("uploaded file response missing dentryId or spaceId")
	}
	return dentryID, spaceID, nil
}

func findInt64Field(value any, keys ...string) (int64, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range keys {
			if raw, ok := typed[key]; ok {
				if v, ok := int64FromJSONScalar(raw); ok {
					return v, true
				}
			}
		}
		for _, raw := range typed {
			if v, ok := findInt64Field(raw, keys...); ok {
				return v, true
			}
		}
	case []any:
		for _, raw := range typed {
			if v, ok := findInt64Field(raw, keys...); ok {
				return v, true
			}
		}
	case string:
		trimmed := strings.TrimSpace(typed)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			var nested any
			if unmarshalJSONUseNumber(trimmed, &nested) == nil {
				return findInt64Field(nested, keys...)
			}
		}
	}
	return 0, false
}

func int64FromJSONScalar(value any) (int64, bool) {
	switch typed := value.(type) {
	case json.Number:
		v, err := typed.Int64()
		return v, err == nil
	case float64:
		return int64(typed), typed > 0
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	case string:
		v, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return v, err == nil
	default:
		return 0, false
	}
}

func cloneStringAnyMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func unmarshalJSONUseNumber(text string, v any) error {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	return dec.Decode(v)
}

func firstStringField(data map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key]; ok {
			if s := stringFromJSONScalar(value); s != "" {
				return s
			}
		}
	}
	return ""
}

func printMCPText(text string) error {
	if deps.Caller.Format() == "json" {
		var parsed any
		if err := json.Unmarshal([]byte(text), &parsed); err == nil {
			return deps.Out.PrintJSONUnescaped(parsed)
		}
	}
	deps.Out.PrintRaw(text)
	return nil
}

// ──────────────────────────────────────────────────────────
// dws chat — 会话 / 群聊 / 消息
// ──────────────────────────────────────────────────────────

func newChatCommand() *cobra.Command {
	root := &cobra.Command{
		Use:     "chat",
		Aliases: []string{"im"},
		Short:   "群聊 / 消息 / 机器人",
		Long:    `管理钉钉会话与群聊：创建群、搜索群、查看群成员、添加机器人到群、修改群名称、拉取会话消息、发送群消息、机器人消息与 Webhook。`,
		RunE:    groupRunE,
	}

	chatChmodCmd := &cobra.Command{
		Use:   "chmod <scope>",
		Short: "授予 chat 高风险操作权限",
		Long: `授予指定命令参数维度的 chat 操作权限。

该命令用于触发悟空宿主应用的授权确认弹窗。chat.* scope 每次执行都需要用户在宿主 UI 中确认，模型无法静默绕过。

授权维度：
  --permParam        授权原始业务参数，可重复传入 key=value，例如 --permParam openCid=xxx --permParam msgType=text

兼容目标选择（三选一）：
  --conversation-id   群聊 openConversationId
  --open-dingtalk-id  单聊目标 openDingTalkId
  --user              单聊目标 userId，由服务端按 Diamond 映射解析为授权维度`,
		Example: `  dws chat chmod chat.message:send --agentCode agt-wukong-xxxx --grant-type timed --ttl 24h --permParam openCid=cidXXXXXXXXXX
  dws chat chmod chat.message:send --agentCode agt-wukong-xxxx --grant-type timed --ttl 24h --permParam receiverUid=123456
  dws chat chmod chat.group:destroy --agentCode agt-wukong-xxxx --grant-type once --permParam openCid=cidXXXXXXXXXX
  dws chat chmod chat.message:send --agentCode agt-wukong-xxxx --grant-type timed --ttl 24h --conversation-id cidXXXXXXXXXX`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateChatScope(args[0]); err != nil {
				return err
			}
			toolArgs, err := buildChatChmodArgs(cmd, args[0])
			if err != nil {
				return err
			}
			return callMCPToolOnServer("im", "chat_permission_grant", toolArgs)
		},
	}
	chatChmodCmd.Flags().String("agentCode", "wukong", "Agent 标识，默认 wukong")
	chatChmodCmd.Flags().String("grant-type", "timed", "授权策略: once|session|timed|permanent")
	chatChmodCmd.Flags().String("ttl", "24h", "timed 授权有效期，如 1h/4h/24h/7d")
	chatChmodCmd.Flags().StringArray("permParam", nil, "授权原始业务参数，格式 key=value，可重复传入")
	chatChmodCmd.Flags().String("conversation-id", "", "群聊 openConversationId")
	chatChmodCmd.Flags().String("open-dingtalk-id", "", "单聊目标 openDingTalkId")
	chatChmodCmd.Flags().String("user", "", "单聊目标 userId（与 --open-dingtalk-id 二选一）")
	chatChmodCmd.Flags().String("session-id", "", "session 授权的会话标识")

	chatDataAuthCmd := &cobra.Command{
		Use:   "data-auth",
		Short: "授予 chat 数据读取权限",
		Long:  `授予 chat 数据读取权限。该命令用于跨组织消息拉取等数据访问场景，不用于发送、撤回、群管理等命令操作。`,
		RunE:  groupRunE,
	}
	chatDataAuthCrossOrgCmd := &cobra.Command{
		Use:   "cross-org",
		Short: "授予跨组织 chat 数据访问权限",
		Long: `授予跨组织 chat 数据访问权限。

该命令调用与 dws chat chmod 相同的授权工具，但固定使用数据授权类别：
	  scope: chat.data:cross-org
	  grantCategory: data
	  grantParams: {"targetOrgId":"<目标组织ID>"} 或 {"targetOrgId":"*"}`,
		Example: `  dws chat data-auth cross-org --target-org-id 439446171
  dws chat data-auth cross-org --target-org-id 439446171 --agentCode wukong --grant-type timed --ttl 24h
  dws chat data-auth cross-org --all --grant-type timed --ttl 24h`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs, err := buildChatCrossOrgDataAuthArgs(cmd)
			if err != nil {
				return err
			}
			return callMCPToolOnServer("im", "chat_permission_grant", toolArgs)
		},
	}
	chatDataAuthCrossOrgCmd.Flags().String("target-org-id", "", "目标组织 ID（与 --all 二选一）")
	chatDataAuthCrossOrgCmd.Flags().Bool("all", false, "授权所有目标组织")
	chatDataAuthCrossOrgCmd.Flags().String("agentCode", "wukong", "Agent 标识，默认 wukong")
	chatDataAuthCrossOrgCmd.Flags().String("grant-type", "timed", "授权策略: once|session|timed|permanent")
	chatDataAuthCrossOrgCmd.Flags().String("ttl", "24h", "timed 授权有效期，如 1h/4h/24h/7d")
	chatDataAuthCrossOrgCmd.Flags().String("session-id", "", "session 授权的会话标识")
	chatDataAuthCmd.AddCommand(chatDataAuthCrossOrgCmd)

	// ── group 子命令 ──────────────────────────────────────────

	chatGroupCmd := &cobra.Command{Use: "group", Short: "群组管理", RunE: groupRunE}

	chatGroupCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建群（支持内部群/外部群/普通群/话题圈）",
		Long: `创建一个群聊，支持指定群名称、初始成员列表等参数。
可选择内部群/外部群/普通群/内部话题圈/外部话题圈/普通话题圈等多种类型群。
默认创建内部群。当选择内部群时如果所选成员非组织内成员会创建失败。`,
		Example: `  dws chat group create --name "Q1 项目冲刺群" --users userId1,userId2,userId3
  dws chat group create --name "外部合作群" --users userId1,userId2 --type EXTERNAL
  dws chat group create --name "话题圈" --users userId1,userId2 --thread
  # 查询 userId: dws contact user search --query "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "name", "users"); err != nil {
				return err
			}
			ctx := cmd.Context()

			// 校验 --type 取值
			groupType, _ := cmd.Flags().GetString("type")
			groupType = strings.ToUpper(groupType)
			switch groupType {
			case "INTERNAL", "EXTERNAL", "NORMAL":
				// valid
			default:
				return fmt.Errorf("invalid --type %q, supported: INTERNAL, EXTERNAL, NORMAL", groupType)
			}

			memberUserIds := parseCSVValues(mustGetFlag(cmd, "users"))

			// 钉钉要求：群主(owner)必须是 useridlist 的成员之一。当前登录用户作为群主，须加入成员列表。
			currentUserID, err := getCurrentUserID(ctx)
			if err != nil {
				return err
			}
			// 将当前用户置于首位（作为群主），避免重复添加
			seen := map[string]bool{currentUserID: true}
			allMembers := []string{currentUserID}
			for _, uid := range memberUserIds {
				if !seen[uid] {
					seen[uid] = true
					allMembers = append(allMembers, uid)
				}
			}

			toolArgs := map[string]any{
				"groupName":    mustGetFlag(cmd, "name"),
				"groupMembers": allMembers,
				"groupType":    groupType,
			}
			// 话题模式
			thread, _ := cmd.Flags().GetBool("thread")
			if thread {
				toolArgs["convThreadEnabled"] = true
			}

			raw, err := callMCPToolReturnTextOnServer(ctx, "im", "create_group_conversation", toolArgs)
			if err != nil {
				return err
			}
			var resp map[string]any
			if json.Unmarshal([]byte(raw), &resp) == nil {
				if result, ok := resp["result"].(map[string]any); ok {
					if v, exists := result["openCid"]; exists {
						result["openConversationId"] = v
						delete(result, "openCid")
					}
					delete(result, "cid")
				}
				return deps.Out.PrintJSON(resp)
			}
			deps.Out.PrintRaw(raw)
			return nil
		},
	}

	chatSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "根据关键词搜索群聊",
		Long: `根据关键词搜索群聊列表。分页参数 --limit（默认 20）和 --cursor（默认 "0"）始终传递；hasMore=true 时用返回的 nextCursor 作为下次 --cursor 继续翻页。

注意：
1. query 不要拆分得太细，应使用群名称中连续的核心词作为关键词（如群名"项目冲刺群"应搜"项目冲刺"而非拆成"项目"+"冲刺"分别搜索）。
2. 当搜索结果返回多个群聊时，应列出候选群让用户确认目标群聊，不要自行假定并直接进行后续操作。`,
		Example: `  dws chat search --query "项目冲刺"
  dws chat search --query "项目冲刺" --limit 20 --cursor 0`,
		RunE: func(cmd *cobra.Command, args []string) error {
			keyword := flagOrFallback(cmd, "query", "keyword")
			if keyword == "" {
				return fmt.Errorf("flag --query is required\n  hint: dws chat search --query \"test\"")
			}
			limit := chatIntFlagOrFallback(cmd, "limit", "size")
			cursor, _ := cmd.Flags().GetString("cursor")
			toolArgs := map[string]any{
				"keyword": keyword,
				"limit":   limit,
				"cursor":  cursor,
			}
			if v, _ := cmd.Flags().GetBool("exclude-muted"); v {
				toolArgs["excludeMuted"] = true
			}
			return callMCPToolOnServer("im", "search_groups", toolArgs)
		},
	}

	chatGroupMembersCmd := &cobra.Command{
		Use:   "members",
		Short: "群成员管理",
		Long:  `查看群成员列表，分页查询指定群聊的成员。`,
		Example: `  dws chat group members --id <openconversation_id>
  # 查询群 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"openconversation_id": mustGetFlag(cmd, "id"),
			}
			if v, _ := cmd.Flags().GetString("cursor"); v != "" {
				toolArgs["cursor"] = v
			}
			return callMCPTool("get_group_members", toolArgs)
		},
	}

	chatGroupMembersAddBotCmd := &cobra.Command{
		Use:   "add-bot",
		Short: "将机器人添加到群中",
		Long:  `将自定义机器人添加到当前用户有管理权限的群聊中。如果没有权限则会报错。`,
		Example: `  dws chat group members add-bot --robot-code <robot-code> --id <openconversation_id>
  # 查询群 ID: dws chat search --query "群名"
  # robot-code: $DINGTALK_CHAT_ROBOT_CODE`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "robot-code", "id"); err != nil {
				return err
			}
			return callMCPToolOnServer("bot", "add_robot_to_group", map[string]any{
				"robotCode":          mustGetFlag(cmd, "robot-code"),
				"openConversationId": mustGetFlag(cmd, "id"),
			})
		},
	}

	chatGroupRenameCmd := &cobra.Command{
		Use:   "rename",
		Short: "更新群名称",
		Example: `  dws chat group rename --id <openconversation_id> --name "新群名"
  # 查询群 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id", "name"); err != nil {
				return err
			}
			return callMCPTool("update_group_name", map[string]any{
				"openconversation_id": mustGetFlag(cmd, "id"),
				"group_name":          mustGetFlag(cmd, "name"),
			})
		},
	}

	chatGroupMemberAddCmd := &cobra.Command{
		Use:   "add",
		Short: "添加群成员",
		Long:  `向指定群聊添加成员，需传入群 ID 与用户标识列表。支持 userId 和 openDingTalkId 混传。`,
		Example: `  dws chat group members add --id <openconversation_id> --users userId1,userId2
  dws chat group members add --id <openconversation_id> --users openDingTalkId1,openDingTalkId2
  dws chat group members add --id <openconversation_id> --users userId1,openDingTalkId1
  # 查询群 ID: dws chat search --query "群名"
  # 查询 userId / openDingTalkId: dws contact user search --query "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id", "users"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"openconversation_id": mustGetFlag(cmd, "id"),
			}
			appendChatIDArgs(toolArgs, parseCSVValues(mustGetFlag(cmd, "users")), "userId", "openDingtalkIds")
			return callMCPTool("add_group_member", toolArgs)
		},
	}

	chatGroupMemberRemoveCmd := &cobra.Command{
		Use:   "remove",
		Short: "移除群成员",
		Long:  `从指定群聊中移除成员，需传入群 ID 与待移除的用户 ID 列表。`,
		Example: `  dws chat group members remove --id <openconversation_id> --users userId1,userId2
  # 查询群 ID: dws chat search --query "群名"
  # 查询 userId: dws contact user search --query "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id", "users"); err != nil {
				return err
			}
			groupID := mustGetFlag(cmd, "id")
			removeValues := parseCSVValues(mustGetFlag(cmd, "users"))
			// 群主防护：移出群主会产生无群主的孤儿群，先在客户端拦截。
			if err := guardGroupOwnerRemoval(cmd.Context(), groupID, removeValues); err != nil {
				return err
			}
			return callMCPTool("remove_group_member", map[string]any{
				"openConversationId": groupID,
				"userIdList":         removeValues,
			})
		},
	}

	// ── message 子命令 ────────────────────────────────────────

	chatMessageCmd := &cobra.Command{Use: "message", Short: "会话消息管理", RunE: groupRunE}

	chatMessageListCmd := &cobra.Command{
		Use:   "list",
		Short: "拉取会话消息内容",
		Long:  `拉取指定群聊或单聊的会话消息内容。--group 指定群聊，--user 指定单聊用户（userId），--open-dingtalk-id 指定单聊用户（openDingTalkId），三者互斥。推荐使用 --direction newer/older 控制时间方向：newer 表示从给定时间往现在拉，older 表示从给定时间往以前拉。hasMore=true 时用结果中的边界 createTime 作为下次 --time 翻页。如果返回的会话消息中包含 openConvThreadId 字段，说明是话题消息，可以调用 dws chat message list-topic-replies 拉取话题回复消息列表，openConvThreadId 作为 topic-id 参数。`,
		Example: `  dws chat message list --group <openconversation_id> --time "2025-03-01 00:00:00"
  dws chat message list --user <userId> --time "2025-03-01 00:00:00" --limit 50
  dws chat message list --open-dingtalk-id <openDingTalkId> --time "2025-03-01 00:00:00" --limit 50
  dws chat message list --group <openconversation_id> --time "2025-03-01 00:00:00" --direction older
  # 查询群 ID: dws chat search --query "群名"
  # 查询 userId: dws contact user search --query "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "time"); err != nil {
				return err
			}
			groupID := flagOrFallback(cmd, "group", "conversation-id", "id", "chat")
			userID, _ := cmd.Flags().GetString("user")
			openDingTalkID, _ := cmd.Flags().GetString("open-dingtalk-id")
			specified := 0
			if groupID != "" {
				specified++
			}
			if userID != "" {
				specified++
			}
			if openDingTalkID != "" {
				specified++
			}
			if specified > 1 {
				return fmt.Errorf("--group, --user and --open-dingtalk-id are mutually exclusive, specify exactly one")
			}
			if specified == 0 {
				return fmt.Errorf("--group, --user or --open-dingtalk-id is required")
			}
			if userID != "" && isOpenDingTalkID(userID) {
				openDingTalkID = userID
				userID = ""
			}
			forward, err := resolveMessageForward(cmd, true)
			if err != nil {
				return err
			}
			timeVal := mustGetFlag(cmd, "time")
			if groupID != "" {
				toolArgs := map[string]any{
					"openconversation_id": groupID,
					"time":                timeVal,
					"forward":             forward,
				}
				if v := chatIntFlagOrFallback(cmd, "limit", "size"); v > 0 {
					toolArgs["limit"] = v
				}
				return callMCPTool("list_conversation_message_v2", toolArgs)
			}
			toolArgs := map[string]any{
				"time":    timeVal,
				"forward": forward,
			}
			if userID != "" {
				toolArgs["userId"] = userID
			} else {
				toolArgs["openDingTalkId"] = openDingTalkID
			}
			if v := chatIntFlagOrFallback(cmd, "limit", "size"); v > 0 {
				toolArgs["limit"] = v
			}
			return callMCPTool("list_individual_chat_message", toolArgs)
		},
	}

	chatMessageListDirectCmd := &cobra.Command{
		Use:    "list-direct",
		Short:  "拉取单聊会话消息",
		Hidden: true,
		Long:   `按对方 userId 或 openDingTalkId 拉取单聊会话消息。`,
		Example: `  dws chat message list-direct --user <对方userId> --time "2026-04-01 00:00:00" --forward true --limit 50
  dws chat message list-direct --open-dingtalk-id <openDingTalkId> --time "2026-04-01 00:00:00" --forward false --limit 20
  # 查询 userId / openDingTalkId: dws contact user search --query "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			userID, _ := cmd.Flags().GetString("user")
			openDingTalkID, _ := cmd.Flags().GetString("open-dingtalk-id")
			if userID != "" && openDingTalkID != "" {
				return fmt.Errorf("--user and --open-dingtalk-id are mutually exclusive")
			}
			if userID == "" && openDingTalkID == "" {
				return fmt.Errorf("--user or --open-dingtalk-id is required")
			}
			if userID != "" && isOpenDingTalkID(userID) {
				openDingTalkID = userID
				userID = ""
			}
			timeVal, _ := cmd.Flags().GetString("time")
			defaultForward := true
			if strings.TrimSpace(timeVal) == "" {
				timeVal = time.Now().Format("2006-01-02 15:04:05")
				defaultForward = false
			}
			forward, err := resolveMessageForward(cmd, defaultForward)
			if err != nil {
				return err
			}
			toolArgs := map[string]any{
				"time":    timeVal,
				"forward": forward,
			}
			if userID != "" {
				toolArgs["userId"] = userID
			} else {
				toolArgs["openDingTalkId"] = openDingTalkID
			}
			if v, err := cmd.Flags().GetInt("limit"); err == nil && v > 0 {
				toolArgs["limit"] = v
			}
			return callMCPTool("list_individual_chat_message", toolArgs)
		},
	}

	chatMessageSendCmd := &cobra.Command{
		Use:   "send",
		Short: "以当前用户身份发送消息（--group 群聊 / --user 或 --open-dingtalk-id 单聊）",
		Long: `以当前用户身份发送消息。

⚠️ 重要：该接口会真实发送消息到目标会话，不可用于测试或试探性调用。调用前必须确认消息内容和接收对象无误。

目标选择（三选一，必填）：
  --group              群聊 openconversation_id
  --user               单聊接收人 userId
  --open-dingtalk-id   单聊接收人 openDingTalkId

纯文本 / Markdown 消息（默认）：
  无需指定 --msg-type，直接传消息内容即可。推荐使用 --text flag 传递内容（尤其当内容含换行、引号等特殊字符时），也支持位置参数。可选 --title 作为消息标题。

富媒体消息（通过 --msg-type 指定类型）：
  image — 发送图片：--msg-type image --media-id（通过 dt_media_upload 上传获得）
  file/audio/video — 发送文件、音频、视频：传本地 --file-path，CLI 会上传后按 file 消息发送`,
		Example: `  dws chat message send --group <openconversation_id> "hello"
  dws chat message send --user <userId> "请查收"
  dws chat message send --open-dingtalk-id <openDingTalkId> "请查收"
  dws chat message send --group <openconversation_id> --title "周报提醒" "请大家本周五前提交周报"
  # 发送图片
  dws chat message send --group <openconversation_id> --msg-type image --media-id <mediaId>
  # 发送本地文件/音频/视频（audio/video 是 file 的语义别名）
  dws chat message send --group <openconversation_id> --msg-type file --file-path ./report.pdf
  dws chat message send --group <openconversation_id> --msg-type audio --file-path ./recording.mp3
  dws chat message send --group <openconversation_id> --msg-type video --file-path ./demo.mp4
# 查询群 ID: dws chat search --query "群名"
# 查询用户 ID: dws contact user search --query "姓名"`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			groupID := flagOrFallback(cmd, "group", "conversation-id", "id", "chat")
			userID, _ := cmd.Flags().GetString("user")
			openDingTalkID, _ := cmd.Flags().GetString("open-dingtalk-id")
			msgUuid, _ := cmd.Flags().GetString("uuid")
			specified := 0
			if groupID != "" {
				specified++
			}
			if userID != "" {
				specified++
			}
			if openDingTalkID != "" {
				specified++
			}
			if specified > 1 {
				return fmt.Errorf("--group, --user and --open-dingtalk-id are mutually exclusive, specify exactly one")
			}
			if specified == 0 {
				return fmt.Errorf("--group, --user or --open-dingtalk-id is required")
			}
			if userID != "" && isOpenDingTalkID(userID) {
				openDingTalkID = userID
				userID = ""
			}
			// 数字 userId 尝试 lookup 转换为 openDingTalkId，让所有消息类型（文本/媒体）都走 openDingTalkId 路径
			// lookup 失败时降级保留 userID，由下游用 receiverUserId 发送
			if userID != "" {
				resolved, err := resolveOpenDingTalkID(cmd.Context(), userID)
				if err != nil {
					if commandBoolFlag(cmd, "debug") || commandBoolFlag(cmd, "verbose") {
						fmt.Fprintf(os.Stderr, "[debug] resolveOpenDingTalkID(%q) failed: %v, falling back to receiverUserId\n", userID, err)
					}
				} else {
					if commandBoolFlag(cmd, "debug") || commandBoolFlag(cmd, "verbose") {
						fmt.Fprintf(os.Stderr, "[debug] resolved userID=%q to openDingTalkId=%q\n", userID, resolved)
					}
					openDingTalkID = resolved
					userID = ""
				}
			}
			if commandBoolFlag(cmd, "debug") || commandBoolFlag(cmd, "verbose") {
				fmt.Fprintf(os.Stderr, "[debug] message send after normalization: groupID=%q userID=%q openDingTalkID=%q\n", groupID, userID, openDingTalkID)
			}

			mediaId, _ := cmd.Flags().GetString("media-id")
			msgType, _ := cmd.Flags().GetString("msg-type")
			clawType := ""
			aiTag, _ := cmd.Flags().GetBool("ai-tag")
			if aiTag {
				clawType = edition.ClawType()
			}

			// ── 富媒体消息（image/audio/video/file） ──
			// text/markdown 透传到下方的文本消息分支，避免模型填 --msg-type text 报 unsupported
			if msgType == "text" || msgType == "markdown" {
				msgType = ""
			}
			if msgType != "" {
				var contentJSON string
				serviceMsgType := msgType
				switch msgType {
				case "image":
					if mediaId == "" {
						return fmt.Errorf("--media-id is required for msgType=image")
					}
					contentJSON = fmt.Sprintf(`{"mediaId":"%s"}`, mediaId)
				case "file", "audio", "video":
					serviceMsgType = "file"
					filePath, _ := cmd.Flags().GetString("file-path")
					dentryId, _ := cmd.Flags().GetInt64("dentry-id")
					spaceId, _ := cmd.Flags().GetInt64("space-id")
					if (dentryId == 0) != (spaceId == 0) {
						return fmt.Errorf("--dentry-id and --space-id must be specified together")
					}
					if filePath != "" {
						meta, err := buildConversationLocalFileMeta(filePath, "", "")
						if err == nil && dentryId != 0 && spaceId != 0 {
							contentJSON, err = buildConversationFileContent(dentryId, spaceId, meta)
							if err != nil {
								return err
							}
						} else if err == nil {
							targetArgs, err := buildConversationTargetArgs(cmd)
							if err != nil {
								return err
							}
							if deps.Caller.DryRun() {
								deps.Out.PrintKeyValue("操作", "上传本地文件并发送 file 消息")
								deps.Out.PrintKeyValue("文件", meta.LocalPath)
								deps.Out.PrintKeyValue("名称", meta.FileName)
								deps.Out.PrintKeyValue("大小", fmt.Sprintf("%d bytes", meta.FileSize))
								return nil
							}
							ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Minute)
							defer cancel()
							commitText, err := uploadConversationLocalFile(ctx, targetArgs, meta, "")
							if err != nil {
								return err
							}
							dentryId, spaceId, err = parseConversationFileSendIDs(commitText)
							if err != nil {
								return err
							}
							contentJSON, err = buildConversationFileContent(dentryId, spaceId, meta)
							if err != nil {
								return err
							}
						} else if dentryId == 0 || spaceId == 0 {
							return fmt.Errorf("--file-path must be a readable local file, or pass legacy --dentry-id and --space-id: %w", err)
						}
					}
					if contentJSON == "" {
						fileName, _ := cmd.Flags().GetString("file-name")
						fileType, _ := cmd.Flags().GetString("file-type")
						fileSize, _ := cmd.Flags().GetInt64("file-size")
						if dentryId == 0 || spaceId == 0 || fileName == "" {
							return fmt.Errorf("readable local --file-path is required for msgType=file; legacy flags --dentry-id, --space-id, --file-name are still supported")
						}
						contentJSON = fmt.Sprintf(`{"dentryId":%d,"spaceId":%d,"fileName":"%s","fileType":"%s","filePath":"%s","fileSize":%d}`,
							dentryId, spaceId, fileName, fileType, filePath, fileSize)
					}
				default:
					return fmt.Errorf("unsupported --msg-type: %s (supported: image, file, audio, video)", msgType)
				}

				params := map[string]any{
					"msgType":  serviceMsgType,
					"content":  contentJSON,
					"clawType": clawType,
				}
				if groupID != "" {
					params["openConversationId"] = groupID
				} else if openDingTalkID != "" {
					params["receiverOpenDingTalkId"] = openDingTalkID
				} else if userID != "" {
					params["receiverUserId"] = userID
				} else {
					return fmt.Errorf("--group, --user or --open-dingtalk-id is required for media messages")
				}
				if msgUuid != "" {
					params["uuid"] = msgUuid
				}
				return callMCPTool("send_personal_message", params)
			}

			// ── 文本/Markdown 消息 ──
			text := flagOrFallback(cmd, "text", "content", "body", "message", "markdown")
			if text == "" && len(args) > 0 {
				text = args[0]
			}
			if text == "" {
				return fmt.Errorf("message content required (use --text or positional arg, or --media-id for image)")
			}
			title, _ := cmd.Flags().GetString("title")
			if title == "" {
				title = sanitizeTitleFromText(text)
			}
			if groupID != "" {
				atAll, _ := cmd.Flags().GetBool("at-all")
				atOpenIdsStr, _ := cmd.Flags().GetString("at-open-dingtalk-ids")
				var atOpenIds []string
				if atOpenIdsStr != "" {
					atOpenIds = strings.Split(atOpenIdsStr, ",")
				}
				if atAll && !strings.Contains(text, "<@all>") {
					text = "<@all> " + text
				}
				// 用户身份发消息要求 @ 占位符为 <@openDingTalkId>；模型若写成裸 @id 自动补全，已有 <@id> 不变
				text = normalizeAtPlaceholders(text, atOpenIds, true)
				// 群聊统一走 openDingTalkId @ 人接口。
				contentJSON, _ := marshalJSONRaw(map[string]string{"title": title, "text": text})
				newParams := map[string]any{
					"openConversationId": groupID,
					"msgType":            "markdown",
					"content":            string(contentJSON),
					"clawType":           clawType,
				}
				if atAll {
					newParams["atAll"] = true
				}
				if len(atOpenIds) > 0 {
					newParams["atOpenDingTalkIds"] = atOpenIds
				}
				if msgUuid != "" {
					newParams["uuid"] = msgUuid
				}
				return callMCPTool("send_personal_message", newParams)
			}
			if userID != "" {
				directContentJSON, _ := marshalJSONRaw(map[string]string{"title": title, "text": text})
				directMsgParams := map[string]any{
					"receiverUserId": userID,
					"msgType":        "markdown",
					"content":        string(directContentJSON),
					"clawType":       clawType,
				}
				if msgUuid != "" {
					directMsgParams["uuid"] = msgUuid
				}
				return callMCPTool("send_personal_message", directMsgParams)
			}
			// 单聊：统一走 openDingTalkId
			directContentJSON, _ := marshalJSONRaw(map[string]string{"title": title, "text": text})
			newDirectParams := map[string]any{
				"receiverOpenDingTalkId": openDingTalkID,
				"msgType":                "markdown",
				"content":                string(directContentJSON),
				"clawType":               clawType,
			}
			if msgUuid != "" {
				newDirectParams["uuid"] = msgUuid
			}
			return callMCPTool("send_personal_message", newDirectParams)
		},
	}

	// send-by-bot: 群聊传 --group，单聊传 --users/--open-dingtalk-ids；--text 支持 Markdown
	chatMessageSendByBotCmd := &cobra.Command{
		Use:   "send-by-bot",
		Short: "机器人发送消息（--group 群聊 / --users 单聊）",
		Long: `群聊：传 --group 指定群；单聊：传 --users 或 --open-dingtalk-ids 指定用户列表，与 --group 只能选其一，不能同时指定。--text 支持 Markdown。

⚠️ 重要：该接口会真实发送消息到目标会话，不可用于测试或试探性调用。调用前必须确认消息内容和接收对象无误。`,
		Example: `  dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> --title "日报" --text "## 今日完成..."
  dws chat message send-by-bot --robot-code <robot-code> --users userId1,userId2 --title "提醒" --text "请提交周报"
  dws chat message send-by-bot --robot-code <robot-code> --open-dingtalk-ids openDingtalkId1,openDingtalkId2 --title "提醒" --text "请提交周报"
  dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> --at-user-ids userId1,userId2 --title "提醒" --text "@userId1 @userId2 请查收本周报告"
  dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> --at-open-dingtalk-ids openDingtalkId1,openDingtalkId2 --title "提醒" --text "@openDingtalkId1 @openDingtalkId2 请查收本周报告"
  dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> --at-all --title "通知" --text "请所有人注意"
  # 查询群 ID: dws chat search --query "群名"
  # 查询 userId: dws contact user search --query "姓名"
  # robot-code: $DINGTALK_CHAT_ROBOT_CODE`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "robot-code", "title", "text"); err != nil {
				return err
			}
			chatID := flagOrFallback(cmd, "group", "conversation-id", "id", "chat")
			usersStr, _ := cmd.Flags().GetString("users")
			openDingtalkIdsStr, _ := cmd.Flags().GetString("open-dingtalk-ids")
			hasDirectTarget := usersStr != "" || openDingtalkIdsStr != ""
			if chatID != "" && hasDirectTarget {
				return fmt.Errorf("--group and --users/--open-dingtalk-ids are mutually exclusive")
			}
			if chatID == "" && !hasDirectTarget {
				return fmt.Errorf("--group or --users/--open-dingtalk-ids is required")
			}
			if chatID != "" {
				var atUserIds []string
				if atUserIdsStr, _ := cmd.Flags().GetString("at-user-ids"); atUserIdsStr != "" {
					for _, id := range strings.Split(atUserIdsStr, ",") {
						if s := strings.TrimSpace(id); s != "" {
							atUserIds = append(atUserIds, s)
						}
					}
				}
				var atOpenDingtalkIds []string
				if atOpenDingtalkIdsStr, _ := cmd.Flags().GetString("at-open-dingtalk-ids"); atOpenDingtalkIdsStr != "" {
					for _, id := range strings.Split(atOpenDingtalkIdsStr, ",") {
						if s := strings.TrimSpace(id); s != "" {
							atOpenDingtalkIds = append(atOpenDingtalkIds, s)
						}
					}
				}
				// 机器人发消息要求 @ 占位符为裸 @id；模型若写成 <@id> 会导致 @ 不生效，主动剥离尖括号
				markdown := mustGetFlag(cmd, "text")
				markdown = normalizeAtPlaceholders(markdown, atUserIds, false)
				markdown = normalizeAtPlaceholders(markdown, atOpenDingtalkIds, false)
				markdown = strings.ReplaceAll(markdown, "<@all>", "@all")
				toolArgs := map[string]any{
					"robotCode":          mustGetFlag(cmd, "robot-code"),
					"openConversationId": chatID,
					"title":              mustGetFlag(cmd, "title"),
					"markdown":           markdown,
				}
				if len(atUserIds) > 0 {
					toolArgs["atUserIds"] = atUserIds
				}
				if len(atOpenDingtalkIds) > 0 {
					toolArgs["atOpendingtalkIds"] = atOpenDingtalkIds
				}
				if isAtAll, _ := cmd.Flags().GetBool("at-all"); isAtAll {
					toolArgs["isAtAll"] = "true"
				}
				return callMCPToolOnServer("bot", "send_robot_group_message", toolArgs)
			}
			toolArgs := map[string]any{
				"robotCode": mustGetFlag(cmd, "robot-code"),
				"title":     mustGetFlag(cmd, "title"),
				"markdown":  mustGetFlag(cmd, "text"),
			}
			if usersStr != "" {
				var userIds []string
				for _, u := range strings.Split(usersStr, ",") {
					if s := strings.TrimSpace(u); s != "" {
						userIds = append(userIds, s)
					}
				}
				toolArgs["userIds"] = userIds
			}
			if openDingtalkIdsStr != "" {
				var openDingtalkIds []string
				for _, id := range strings.Split(openDingtalkIdsStr, ",") {
					if s := strings.TrimSpace(id); s != "" {
						openDingtalkIds = append(openDingtalkIds, s)
					}
				}
				toolArgs["openDingtalkIds"] = openDingtalkIds
			}
			if isAtAll, _ := cmd.Flags().GetBool("at-all"); isAtAll {
				toolArgs["isAtAll"] = "true"
			}
			return callMCPToolOnServer("bot", "batch_send_robot_msg_to_users", toolArgs)
		},
	}

	// recall-by-bot: 传 --group 为群聊撤回，不传为单聊撤回
	chatMessageRecallByBotCmd := &cobra.Command{
		Use:   "recall-by-bot",
		Short: "机器人撤回消息（--group 群聊 / 不传为单聊）",
		Long:  `群聊：传 --group 与 --keys；单聊：仅传 --keys。--keys 为发送时返回的 processQueryKey 列表，逗号分隔。`,
		Example: `  dws chat message recall-by-bot --robot-code <robot-code> --group <openconversation_id> --keys <process-query-key>
  dws chat message recall-by-bot --robot-code <robot-code> --keys key1,key2
  # 查询群 ID: dws chat search --query "群名"
  # robot-code: $DINGTALK_CHAT_ROBOT_CODE`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "robot-code", "keys"); err != nil {
				return err
			}
			keysStr := mustGetFlag(cmd, "keys")
			var processQueryKeys []string
			for _, k := range strings.Split(keysStr, ",") {
				if s := strings.TrimSpace(k); s != "" {
					processQueryKeys = append(processQueryKeys, s)
				}
			}
			chatID := flagOrFallback(cmd, "group", "conversation-id", "id", "chat")
			if chatID != "" {
				return callMCPToolOnServer("bot", "recall_robot_group_message", map[string]any{
					"robotCode":          mustGetFlag(cmd, "robot-code"),
					"openConversationId": chatID,
					"processQueryKeys":   processQueryKeys,
				})
			}
			return callMCPToolOnServer("bot", "batch_recall_robot_users_msg", map[string]any{
				"robotCode":        mustGetFlag(cmd, "robot-code"),
				"processQueryKeys": processQueryKeys,
			})
		},
	}

	chatMessageSendByWebhookCmd := &cobra.Command{
		Use:   "send-by-webhook",
		Short: "自定义机器人 Webhook 发送群消息",
		Long: `通过自定义机器人 Webhook 发送群消息。@ 人时需在 --text 中包含 @userId 或 @手机号，否则 @ 不生效。

⚠️ 重要：该接口会真实发送消息到目标群聊，不可用于测试或试探性调用。调用前必须确认消息内容无误。`,
		Example: `  dws chat message send-by-webhook --token <webhook-token> --title "告警" --text "CPU 超 90%" --at-all
  dws chat message send-by-webhook --token <webhook-token> --title "test" --text "hi @118785" --at-users 118785`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "token", "title", "text"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"robotToken": mustGetFlag(cmd, "token"),
				"title":      mustGetFlag(cmd, "title"),
				"text":       mustGetFlag(cmd, "text"),
			}
			if v, _ := cmd.Flags().GetBool("at-all"); v {
				toolArgs["isAtAll"] = true
			}
			if v, _ := cmd.Flags().GetString("at-mobiles"); v != "" {
				var mobiles []string
				for _, m := range strings.Split(v, ",") {
					if s := strings.TrimSpace(m); s != "" {
						mobiles = append(mobiles, s)
					}
				}
				toolArgs["atMobiles"] = mobiles
			}
			if v, _ := cmd.Flags().GetString("at-users"); v != "" {
				var atUserIds []string
				for _, u := range strings.Split(v, ",") {
					if s := strings.TrimSpace(u); s != "" {
						atUserIds = append(atUserIds, s)
					}
				}
				toolArgs["atUserIds"] = atUserIds
			}
			return callMCPToolOnServer("bot", "send_message_by_custom_robot", toolArgs)
		},
	}

	chatMessageListTopicRepliesCmd := &cobra.Command{
		Use:   "list-topic-replies",
		Short: "拉取群话题回复消息列表",
		Long:  `查询指定群聊中某条话题消息的全部回复。--group 指定群会话 ID，--topic-id 指定话题 ID（由 dws chat message list 返回）。`,
		Example: `  dws chat message list-topic-replies --group <openconversation_id> --topic-id <topicId>
  dws chat message list-topic-replies --group <openconversation_id> --topic-id <topicId> --time "2025-03-01 00:00:00" --limit 20
  # 查询群 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "group", "conversation-id", "id", "chat"); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "topic-id"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"openconversationId": flagOrFallback(cmd, "group", "conversation-id", "id", "chat"),
				"topicId":            mustGetFlag(cmd, "topic-id"),
			}
			if v, _ := cmd.Flags().GetString("time"); v != "" {
				toolArgs["startTime"] = v
			}
			if v, err := cmd.Flags().GetInt("limit"); err == nil && v > 0 {
				toolArgs["pageSize"] = v
			}
			forward, err := resolveMessageForward(cmd, false)
			if err != nil {
				return err
			}
			toolArgs["forward"] = forward
			return callMCPTool("list_topic_replies", toolArgs)
		},
	}

	chatMessageListAllCmd := &cobra.Command{
		Use:   "list-all",
		Short: "拉取指定时间范围内当前用户的所有会话消息",
		Long:  `分页拉取当前登录用户在指定时间范围内的所有会话消息。--start 和 --end 限定时间范围，--limit 指定每页数量，--cursor 传分页游标（首页传 0）。服务端按 cursor 分页返回，hasMore=true 时用返回的 nextCursor 值继续翻页。`,
		Example: `  dws chat message list-all --start "2025-03-01 00:00:00" --end "2025-03-31 23:59:59" --limit 50
  dws chat message list-all --start "2025-03-01 00:00:00" --end "2025-03-31 23:59:59" --limit 50 --cursor "abc123token"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "start", "end"); err != nil {
				return err
			}
			limit := chatIntFlagOrFallback(cmd, "limit", "size")
			cursor, _ := cmd.Flags().GetString("cursor")
			toolArgs := map[string]any{
				"startTime": mustGetFlag(cmd, "start"),
				"endTime":   mustGetFlag(cmd, "end"),
				"limit":     limit,
				"cursor":    cursor,
			}
			return callMCPTool("search_messages_by_time_range", toolArgs)
		},
	}

	chatMessageListBySenderCmd := &cobra.Command{
		Use:   "list-by-sender",
		Short: "拉取指定发送者的消息（包含单聊和群聊）",
		Long:  `搜索特定人发送给我的消息，返回结果包含单聊和群聊标识。--sender-user-id 指定发送者 userId，--sender-open-dingtalk-id 指定发送者 openDingTalkId，二者互斥。分页参数 --limit（默认 50）和 --cursor（默认 "0"）始终传递；hasMore=true 时用返回的 nextCursor 作为下次 --cursor 继续翻页。`,
		Example: `  dws chat message list-by-sender --sender-user-id <userId> --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T00:00:00+08:00" --limit 50 --cursor 0
  dws chat message list-by-sender --sender-open-dingtalk-id <openDingTalkId> --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T00:00:00+08:00" --limit 50 --cursor 0
  dws chat message list-by-sender --sender-user-id <userId> --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T23:59:59+08:00" --limit 20 --cursor 0
  dws chat message list-by-sender --sender-open-dingtalk-id <openDingTalkId> --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T00:00:00+08:00" --limit 50 --cursor <nextCursor>
  # 查询 userId: dws contact user search --query "姓名"
  # 查询 openDingTalkId: dws contact user search --query "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "start"); err != nil {
				return err
			}
			senderUserID := flagOrFallback(cmd, "sender-user-id", "sender")
			senderOpenDingTalkID, _ := cmd.Flags().GetString("sender-open-dingtalk-id")
			if senderUserID != "" && senderOpenDingTalkID != "" {
				return fmt.Errorf("--sender-user-id and --sender-open-dingtalk-id are mutually exclusive, specify exactly one")
			}
			if senderUserID == "" && senderOpenDingTalkID == "" {
				return fmt.Errorf("--sender-user-id or --sender-open-dingtalk-id is required")
			}
			startMs, err := parseISOTimeToMillis("start", mustGetFlag(cmd, "start"))
			if err != nil {
				return err
			}
			endRaw, _ := cmd.Flags().GetString("end")
			if strings.TrimSpace(endRaw) == "" {
				endRaw = time.Now().Format(time.RFC3339)
			}
			endMs, err := parseISOTimeToMillis("end", endRaw)
			if err != nil {
				return err
			}
			if err := validateTimeRange(startMs, endMs); err != nil {
				return err
			}
			limit := chatIntFlagOrFallback(cmd, "limit", "size")
			cursor, _ := cmd.Flags().GetString("cursor")
			toolArgs := map[string]any{
				"startTime": startMs,
				"endTime":   endMs,
				"limit":     limit,
				"cursor":    cursor,
			}
			if senderUserID != "" {
				toolArgs["senderUserId"] = senderUserID
			} else {
				toolArgs["senderOpenDingTalkId"] = senderOpenDingTalkID
			}
			return callMCPTool("search_messages_by_sender", toolArgs)
		},
	}

	chatMessageListMentionsCmd := &cobra.Command{
		Use:   "list-mentions",
		Short: "拉取 @我 的消息",
		Long:  `搜索时间范围内 @我 的消息，可选指定群聊。返回结果包含单聊和群聊标识。分页参数 --limit（默认 50）和 --cursor（默认 "0"）始终传递；hasMore=true 时用返回的 nextCursor 作为下次 --cursor 继续翻页。`,
		Example: `  dws chat message list-mentions --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T00:00:00+08:00" --limit 50 --cursor 0
  dws chat message list-mentions --start "2026-04-01T00:00:00+08:00" --end "2026-04-14T00:00:00+08:00" --limit 20 --cursor 0
  dws chat message list-mentions --group <openconversation_id> --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T00:00:00+08:00" --limit 50 --cursor 0
  dws chat message list-mentions --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T00:00:00+08:00" --limit 50 --cursor <nextCursor>
  # 查询群 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "start", "end"); err != nil {
				return err
			}
			startMs, err := parseISOTimeToMillis("start", mustGetFlag(cmd, "start"))
			if err != nil {
				return err
			}
			endMs, err := parseISOTimeToMillis("end", mustGetFlag(cmd, "end"))
			if err != nil {
				return err
			}
			if err := validateTimeRange(startMs, endMs); err != nil {
				return err
			}
			limit := chatIntFlagOrFallback(cmd, "limit", "size")
			cursor, _ := cmd.Flags().GetString("cursor")
			toolArgs := map[string]any{
				"startTime": startMs,
				"endTime":   endMs,
				"limit":     limit,
				"cursor":    cursor,
			}
			groupID := flagOrFallback(cmd, "group", "conversation-id", "id", "chat")
			if groupID != "" {
				toolArgs["openConversationId"] = groupID
			}
			return callMCPTool("search_at_me_message", toolArgs)
		},
	}

	chatMessageListFocusedCmd := &cobra.Command{
		Use:   "list-focused",
		Short: "拉取特别关注人的消息",
		Long:  `拉取当前用户特别关注人的消息。分页参数 --limit 指定每页数量，--cursor 传分页游标（首次不传或传 0）。返回结果中 hasMore=true 时用 nextCursor 作为下次 --cursor 继续翻页。`,
		Example: `  dws chat message list-focused --limit 50
  dws chat message list-focused --limit 20 --cursor <nextCursor>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v, err := cmd.Flags().GetInt("limit"); err == nil && v > 0 {
				toolArgs["limit"] = v
			}
			if v, _ := cmd.Flags().GetInt64("cursor"); v > 0 {
				toolArgs["cursor"] = v
			}
			return callMCPTool("list_special_focus_messages", toolArgs)
		},
	}

	chatMessageListTopConversationsCmd := &cobra.Command{
		Use:   "list-top-conversations",
		Short: "拉取置顶会话列表",
		Long:  `拉取当前用户的置顶会话列表。分页参数 --limit 指定每页数量，--cursor 传分页游标（首次不传或传 0）。返回结果中 hasMore=true 时用 nextCursor 作为下次 --cursor 继续翻页。`,
		Example: `  dws chat list-top-conversations --limit 1000
  dws chat list-top-conversations --limit 1000 --cursor <nextCursor>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v, err := cmd.Flags().GetInt("limit"); err == nil && v > 0 {
				toolArgs["limit"] = v
			}
			if v, _ := cmd.Flags().GetInt64("cursor"); v > 0 {
				toolArgs["cursor"] = v
			}
			if v, _ := cmd.Flags().GetBool("exclude-muted"); v {
				toolArgs["excludeMuted"] = true
			}
			return callMCPTool("list_top_conversations", toolArgs)
		},
	}

	chatMessageListUnreadConversationsCmd := &cobra.Command{
		Use:   "list-unread-conversations",
		Short: "获取未读会话列表",
		Long:  `获取当前用户有未读消息的会话列表，可选通过 --count 控制返回的会话条数。`,
		Example: `  dws chat message list-unread-conversations
  dws chat message list-unread-conversations --count 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if count, err := cmd.Flags().GetInt("count"); err == nil && count > 0 {
				toolArgs["count"] = count
			}
			if v, _ := cmd.Flags().GetBool("exclude-muted"); v {
				toolArgs["excludeMuted"] = true
			}
			return callMCPTool("unread_message_conversation_list", toolArgs)
		},
	}

	chatMessageSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "按关键词搜索消息",
		Long:  `在当前用户的会话中按关键词搜索消息。--query 指定搜索关键词（必填）。可选 --group 限定搜索某个会话，不传则搜索所有会话。时间参数 --start/--end（ISO-8601）限定搜索时间范围。分页参数 --limit（默认 100）和 --cursor（默认 "0"）始终传递；hasMore=true 时用返回的 nextCursor 作为下次 --cursor 继续翻页。`,
		Example: `  dws chat message search --query "changefree" --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00" --limit 50 --cursor 0
  dws chat message search --query "codereview" --group <openconversation_id> --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00" --limit 100 --cursor 0
  dws chat message search --query "链接" --start "2026-04-15T00:00:00+08:00" --end "2026-04-16T00:00:00+08:00" --limit 100 --cursor <nextCursor>
  # 查询群 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "query", "keyword"); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "start", "end"); err != nil {
				return err
			}
			startMs, err := parseISOTimeToMillis("start", mustGetFlag(cmd, "start"))
			if err != nil {
				return err
			}
			endMs, err := parseISOTimeToMillis("end", mustGetFlag(cmd, "end"))
			if err != nil {
				return err
			}
			if err := validateTimeRange(startMs, endMs); err != nil {
				return err
			}
			limit := chatIntFlagOrFallback(cmd, "limit", "size")
			cursor, _ := cmd.Flags().GetString("cursor")
			toolArgs := map[string]any{
				"keyword":   flagOrFallback(cmd, "query", "keyword"),
				"startTime": startMs,
				"endTime":   endMs,
				"limit":     limit,
				"cursor":    cursor,
			}
			groupID := flagOrFallback(cmd, "group", "conversation-id", "id", "chat")
			if groupID != "" {
				toolArgs["openConversationId"] = groupID
			}
			return callMCPTool("search_messages_by_keyword", toolArgs)
		},
	}

	// ── search-advanced：多维度搜索消息 ──────

	chatMessageSearchAdvancedCmd := &cobra.Command{
		Use:   "search-advanced",
		Short: "多维度搜索消息",
		Long:  `支持按关键词、发送者、@我、@指定人、指定会话、时间范围等多维度搜索消息。发送者 userId 使用 --user/--users；发送者或 @ 人的 openDingTalkId 使用 --sender-ids/--at-ids。所有参数均为可选，至少指定一个搜索条件。`,
		Example: `  dws chat message search-advanced --query "周报" --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00"
  dws chat message search-advanced --user <userId> --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00"
  dws chat message search-advanced --users <userId1>,<userId2> --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00"
  dws chat message search-advanced --sender-ids <openDingTalkId1>,<openDingTalkId2> --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00"
  dws chat message search-advanced --at-me --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00"
  dws chat message search-advanced --at-ids <openDingTalkId1>,<openDingTalkId2> --conversation-ids <openConversationId1>,<openConversationId2> --limit 50 --cursor 0
  dws chat message search-advanced --conversation-ids <单聊openConversationId> --query "合同" --start "2026-04-01T00:00:00+08:00" --end "2026-04-15T00:00:00+08:00"
  # 查询群 ID: dws chat search --query "群名"
  # 查询单聊会话 ID: dws chat conversation-info --user <userId>
  # 查询人员: dws contact user search --keyword "姓名" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}

			// The CLI primary is --query; the IM MCP field is still named "keyword".
			if v := flagOrFallback(cmd, "query", "keyword"); v != "" {
				toolArgs["keyword"] = v
			}

			// --user/--userId are the preferred userId inputs for sender filtering.
			if v := flagOrFallback(cmd, "users", "user", "userId"); v != "" {
				appendChatIDArgs(toolArgs, parseCSVValues(v), "senderUserIds", "senderOpenDingTakIds")
			}

			// sender-ids -> senderOpenDingTakIds / senderUserIds（注意：MCP 入参字段名缺少字母 l）
			if v := flagOrFallback(cmd, "sender-ids", "senders", "sender"); v != "" {
				ids := parseCSVValues(v)
				if len(ids) > 0 {
					appendChatIDArgs(toolArgs, ids, "senderUserIds", "senderOpenDingTakIds")
				}
			}

			// at-me
			if v, _ := cmd.Flags().GetBool("at-me"); v {
				toolArgs["atMe"] = true
			}

			// at-ids -> atOpenDingTakIds / atUserIds（注意：MCP 入参字段名缺少字母 l）
			if v, _ := cmd.Flags().GetString("at-ids"); v != "" {
				ids := parseCSVValues(v)
				if len(ids) > 0 {
					appendChatIDArgs(toolArgs, ids, "atUserIds", "atOpenDingTakIds")
				}
			}

			// conversation-ids / groups / group -> openConversationIds
			convIds := ""
			if v, _ := cmd.Flags().GetString("conversation-ids"); v != "" {
				convIds = v
			} else if v, _ := cmd.Flags().GetString("groups"); v != "" {
				convIds = v
			} else if v, _ := cmd.Flags().GetString("group"); v != "" {
				convIds = v
			}
			if convIds != "" {
				var ids []string
				for _, s := range strings.Split(convIds, ",") {
					if t := strings.TrimSpace(s); t != "" {
						ids = append(ids, t)
					}
				}
				if len(ids) > 0 {
					toolArgs["openConversationIds"] = ids
				}
			}

			// start -> startTime (ISO-8601 to milliseconds)
			if v, _ := cmd.Flags().GetString("start"); v != "" {
				ms, err := parseISOTimeToMillis("start", v)
				if err != nil {
					return err
				}
				toolArgs["startTime"] = ms
			}

			// end -> endTime (ISO-8601 to milliseconds)
			if v, _ := cmd.Flags().GetString("end"); v != "" {
				ms, err := parseISOTimeToMillis("end", v)
				if err != nil {
					return err
				}
				toolArgs["endTime"] = ms
			}

			// cursor
			if v, _ := cmd.Flags().GetString("cursor"); v != "" {
				toolArgs["cursor"] = v
			}

			// limit
			if v := chatIntFlagOrFallback(cmd, "limit", "size"); v > 0 {
				toolArgs["limit"] = v
			}

			return callMCPToolOnServer("im", "search_messages", toolArgs)
		},
	}

	// ── query-send-status：查询消息发送状态（走 IM MCP）──────

	chatMessageQuerySendStatusCmd := &cobra.Command{
		Use:   "query-send-status",
		Short: "查询消息发送状态",
		Long:  `查询以当前用户身份发送的消息的发送状态。需要传入发送消息时返回的 openTaskId。`,
		Example: `  dws chat message query-send-status --open-task-id <openTaskId>
  # openTaskId 由 dws chat message send 返回`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "open-task-id"); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "query_message_send_status", map[string]any{
				"openTaskId": mustGetFlag(cmd, "open-task-id"),
			})
		},
	}

	// ── recall：撤回用户发送的消息（走 IM MCP）──────

	chatMessageRecallCmd := &cobra.Command{
		Use:   "recall",
		Short: "撤回用户发送的消息",
		Long:  `撤回当前用户发送的消息。需要指定会话 ID 和消息 ID。`,
		Example: `  dws chat message recall --conversation-id <openConversationId> --msg-id <openMessageId>
  # 查询会话 ID: dws chat search --query "群名"
  # 消息 ID 可通过 dws chat message list 获取`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "conversation-id", "group", "id", "chat"); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "msg-id"); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "recall_message", map[string]any{
				"openConversationId": flagOrFallback(cmd, "conversation-id", "group", "id", "chat"),
				"openMessageId":      mustGetFlag(cmd, "msg-id"),
			})
		},
	}

	chatMessageReadStatusCmd := &cobra.Command{
		Use:   "read-status",
		Short: "查询消息的已读/未读状态",
		Long:  `查询指定会话中消息的已读/未读状态（仅消息发送者可查询自己发出的消息）。--conversation-id 指定会话 openConversationId（群聊或单聊均可），--message-id 指定消息 ID（由 dws chat message list 返回的 openMessageId，必须是当前用户发送的消息）。目标用户 userId 使用 --user/--users；目标用户 openDingTalkId 使用 --target-open-dingtalk-ids；不传目标用户则返回所有接收者的状态。`,
		Example: `  dws chat message read-status --conversation-id <openConversationId> --message-id <openMessageId>
  dws chat message read-status --conversation-id <openConversationId> --message-id <openMessageId> --user userId1,userId2
  dws chat message read-status --conversation-id <openConversationId> --message-id <openMessageId> --users userId1,userId2
  dws chat message read-status --conversation-id <openConversationId> --message-id <openMessageId> --target-open-dingtalk-ids openDingTalkId1,openDingTalkId2
  # 查询会话 ID: dws chat search --query "群名"
  # 查询 openMessageId: dws chat message list --group <openConversationId> --time "2025-03-01 00:00:00"
  # 查询人员: dws contact user search --keyword "姓名" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "conversation-id", "group", "id", "chat"); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "message-id"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"openConversationId": flagOrFallback(cmd, "conversation-id", "group", "id", "chat"),
				"openMessageId":      mustGetFlag(cmd, "message-id"),
			}
			if usersStr := flagOrFallback(cmd, "users", "user", "userId"); usersStr != "" {
				appendChatIDArgs(toolArgs, parseCSVValues(usersStr), "targetUserIds", "targetOpenDingTalkIds")
			}
			if usersStr, _ := cmd.Flags().GetString("target-open-dingtalk-ids"); usersStr != "" {
				appendChatIDArgs(toolArgs, parseCSVValues(usersStr), "targetUserIds", "targetOpenDingTalkIds")
			}
			return callMCPToolOnServer("im", "query_msg_read_status", toolArgs)
		},
	}

	chatSearchCommonCmd := &cobra.Command{
		Use:   "search-common",
		Short: "搜索共同群（查询指定人共同所在的群聊）",
		Long:  `根据昵称列表搜索共同群聊。--nicks 指定要搜索的人员昵称（逗号分隔，必填）。--match-mode 控制匹配模式：AND 表示所有人都在群里，OR 表示任一人在群里（默认 AND）。分页参数 --limit（默认 20）和 --cursor（默认 "0"）始终传递；hasMore=true 时用返回的 nextCursor 作为下次 --cursor 继续翻页。`,
		Example: `  dws chat search-common --nicks "风雷,山乔" --limit 20 --cursor 0
  dws chat search-common --nicks "天鸡,乐函" --match-mode OR --limit 20 --cursor 0
  dws chat search-common --nicks "风雷,山乔,天鸡" --limit 10 --cursor <nextCursor>`,
		RunE: runChatSearchCommon,
	}

	chatMessageSearchCommonCmd := &cobra.Command{
		Use:    "search-common",
		Short:  "搜索共同群",
		Hidden: true,
		RunE:   runChatSearchCommon,
	}

	// ── bot 子命令 ────────────────────────────────────────────

	chatBotCmd := &cobra.Command{Use: "bot", Short: "机器人管理", RunE: groupRunE}

	chatBotSearchCmd := &cobra.Command{
		Use:   "search",
		Short: "搜索【我自己创建】的机器人（仅本人创建的，不含他人/官方机器人）",
		Long: `搜索【当前登录用户自己创建】的机器人，按 robotName 模糊匹配 + 页码分页。

如何在 search 与 find 之间选择：
  - search：用户说"我创建的""我的""我自己的""我做的"机器人 → 用 search
  - find  ：用户说"搜索机器人""找一个机器人""所有可用机器人""帮我找 XXX 机器人"
            （不限范围，包含他人/官方）→ 用 find（dws chat bot find）

注意：search 没有 openDingTalkId，如果需要给机器人发单聊消息请用 find。`,
		Example: `  # "搜一下我创建的机器人" / "我自己的机器人有哪些"
  dws chat bot search --page 1
  dws chat bot search --page 1 --size 10 --name "日报"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			page, _ := cmd.Flags().GetInt("page")
			toolArgs := map[string]any{
				"currentPage": page,
			}
			size, _ := cmd.Flags().GetInt("size")
			if size == 0 {
				size, _ = cmd.Flags().GetInt("limit")
			}
			if size > 0 {
				toolArgs["pageSize"] = size
			}
			if v, _ := cmd.Flags().GetString("name"); v != "" {
				toolArgs["robotName"] = v
			}
			return callMCPToolOnServer("bot", "search_my_robots", toolArgs)
		},
	}

	// group 子命令 flags
	chatGroupCreateCmd.Flags().String("name", "", "群名称 (必填)")
	_ = chatGroupCreateCmd.MarkFlagRequired("name")
	chatGroupCreateCmd.Flags().String("users", "", "成员 userId 或 openDingTalkId（可混传），逗号分隔 (必填)")
	_ = chatGroupCreateCmd.MarkFlagRequired("users")
	chatGroupCreateCmd.Flags().String("type", "INTERNAL", "群类型: INTERNAL(内部群,默认)/EXTERNAL(外部群)/NORMAL(普通群)")
	chatGroupCreateCmd.Flags().Bool("thread", false, "开启话题模式，将创建话题圈")

	chatSearchCmd.Flags().String("query", "", "搜索关键词 (必填)")
	chatSearchCmd.Flags().String("keyword", "", "--query 的别名")
	_ = chatSearchCmd.Flags().MarkHidden("keyword")
	chatSearchCmd.Flags().Int("limit", 20, "每页返回数量（默认 20）")
	chatSearchCmd.Flags().Int("size", 0, "--limit 的旧版别名")
	_ = chatSearchCmd.Flags().MarkHidden("size")
	chatSearchCmd.Flags().String("cursor", "0", "分页游标（默认 \"0\"，翻页传 nextCursor）")
	chatSearchCmd.Flags().Bool("exclude-muted", false, "是否排除已设置免打扰的群聊（默认 false）")

	chatGroupMembersCmd.Flags().String("id", "", "群 ID / openconversation_id (必填)")
	_ = chatGroupMembersCmd.MarkFlagRequired("id")
	chatGroupMembersCmd.Flags().String("cursor", "", "分页游标，首次从 0 开始")

	chatGroupMembersAddBotCmd.Flags().String("robot-code", "", "机器人 Code (必填)")
	_ = chatGroupMembersAddBotCmd.MarkFlagRequired("robot-code")
	chatGroupMembersAddBotCmd.Flags().String("id", "", "群聊 openConversationId (必填)")
	_ = chatGroupMembersAddBotCmd.MarkFlagRequired("id")

	chatGroupRenameCmd.Flags().String("id", "", "群 ID / openconversation_id (必填)")
	_ = chatGroupRenameCmd.MarkFlagRequired("id")
	chatGroupRenameCmd.Flags().String("name", "", "修改后的群名称 (必填)")
	_ = chatGroupRenameCmd.MarkFlagRequired("name")

	chatGroupMemberAddCmd.Flags().String("id", "", "群 ID / openconversation_id (必填)")
	_ = chatGroupMemberAddCmd.MarkFlagRequired("id")
	chatGroupMemberAddCmd.Flags().String("users", "", "要添加的用户 userId 或 openDingTalkId（可混传），逗号分隔 (必填)")
	_ = chatGroupMemberAddCmd.MarkFlagRequired("users")

	chatGroupMemberRemoveCmd.Flags().String("id", "", "群 ID / openconversation_id (必填)")
	_ = chatGroupMemberRemoveCmd.MarkFlagRequired("id")
	chatGroupMemberRemoveCmd.Flags().String("users", "", "要移除的用户 userId 列表，逗号分隔 (必填)")
	_ = chatGroupMemberRemoveCmd.MarkFlagRequired("users")

	chatGroupCmd.AddCommand(chatGroupCreateCmd, chatGroupMembersCmd, chatGroupRenameCmd)
	chatGroupCmd.AddCommand(hintSubCmd("search", "use: dws chat search --query <关键词>"))
	chatGroupMembersCmd.AddCommand(chatGroupMemberAddCmd, chatGroupMemberRemoveCmd, chatGroupMembersAddBotCmd)

	// message 子命令 flags
	chatMessageListCmd.Flags().String("group", "", "群聊 openconversation_id（群聊时必填）")
	chatMessageListCmd.Flags().String("user", "", "单聊用户 userId（单聊时与 --open-dingtalk-id 二选一）")
	chatMessageListCmd.Flags().String("open-dingtalk-id", "", "单聊用户 openDingTalkId（单聊时与 --user 二选一，适用于无法获取 userId 的场景）")
	chatMessageListCmd.Flags().String("time", "", "开始时间，格式: yyyy-MM-dd HH:mm:ss (必填)")
	chatMessageListCmd.Flags().String("direction", "", "时间方向: newer=从给定时间往现在拉，older=从给定时间往以前拉（推荐）")
	chatMessageListCmd.Flags().String("forward", "true", "true 等价 --direction newer，false 等价 --direction older")
	_ = chatMessageListCmd.Flags().MarkHidden("forward")
	chatMessageListCmd.Flags().Int("limit", 0, "返回数量，不传则不限制")
	chatMessageListCmd.Flags().Int("size", 0, "--limit 的旧版别名")
	_ = chatMessageListCmd.Flags().MarkHidden("size")
	chatMessageListDirectCmd.Flags().String("user", "", "对方 userId（同组织内同事，与 --open-dingtalk-id 二选一）")
	chatMessageListDirectCmd.Flags().String("open-dingtalk-id", "", "对方 openDingTalkId（非同组织普通好友场景，与 --user 二选一）")
	chatMessageListDirectCmd.Flags().String("time", "", "开始时间，格式 yyyy-MM-dd HH:mm:ss (必填)")
	chatMessageListDirectCmd.Flags().String("direction", "", "时间方向: newer=从给定时间往现在拉，older=从给定时间往以前拉")
	chatMessageListDirectCmd.Flags().String("forward", "true", "true 等价 --direction newer，false 等价 --direction older")
	_ = chatMessageListDirectCmd.Flags().MarkHidden("forward")
	chatMessageListDirectCmd.Flags().Int("limit", 50, "每页返回数量（默认 50）")
	chatMessageListDirectCmd.Flags().Int("size", 0, "--limit 的旧版别名")
	_ = chatMessageListDirectCmd.Flags().MarkHidden("size")

	chatMessageSendCmd.Flags().String("group", "", "群聊 openconversation_id（群聊时必填）")
	chatMessageSendCmd.Flags().String("user", "", "单聊接收人 userId（单聊时与 --open-dingtalk-id 二选一）")
	chatMessageSendCmd.Flags().String("open-dingtalk-id", "", "单聊接收人 openDingTalkId（单聊时与 --user 二选一）")
	chatMessageSendCmd.Flags().String("title", "", "消息标题，显示在消息列表（可选，未指定时使用消息内容）")
	// 别名注册: --text/--content/--body/--message/--markdown → 位置参数
	chatMessageSendCmd.Flags().String("text", "", "消息内容（推荐方式，也可用位置参数传递。内容含换行/特殊字符时必须使用此 flag）")
	chatMessageSendCmd.Flags().String("content", "", "--text 的别名")
	chatMessageSendCmd.Flags().String("body", "", "--text 的别名")
	chatMessageSendCmd.Flags().String("message", "", "--text 的别名")
	chatMessageSendCmd.Flags().String("markdown", "", "--text 的别名")
	// --text 不再隐藏，作为首选传参方式展示在 help 中
	_ = chatMessageSendCmd.Flags().MarkHidden("content")
	_ = chatMessageSendCmd.Flags().MarkHidden("body")
	_ = chatMessageSendCmd.Flags().MarkHidden("message")
	_ = chatMessageSendCmd.Flags().MarkHidden("markdown")
	chatMessageSendCmd.Flags().Bool("at-all", false, "@所有人（仅群聊时生效，可选）,设置时，消息内容中一定要包含对应的占位符<@all>")
	chatMessageSendCmd.Flags().String("at-open-dingtalk-ids", "", "@指定成员的 openDingTalkId 列表，逗号分隔（仅群聊时生效，可选）,设置--at-open-dingtalk-ids openDingTalkId1,openDingTalkId2时，消息内容中一定要包含对应格式的占位符<@openDingTalkId1> <@openDingTalkId2>")
	chatMessageSendCmd.Flags().String("media-id", "", "图片 mediaId（通过 dt_media_upload 上传后用 extract_media_id.py 提取，仅 msgType=image）")
	chatMessageSendCmd.Flags().String("msg-type", "", "富媒体消息类型: image/file/audio/video（audio/video 是 file 别名；纯文本/Markdown 无需指定，直接传内容即可）")
	chatMessageSendCmd.Flags().Int64("dentry-id", 0, "文件 dentryId（与 --space-id 成对传入时跳过自动上传）")
	chatMessageSendCmd.Flags().Int64("space-id", 0, "空间 ID（与 --dentry-id 成对传入时跳过自动上传）")
	chatMessageSendCmd.Flags().String("file-name", "", "文件名")
	chatMessageSendCmd.Flags().String("file-type", "", "文件类型/扩展名")
	chatMessageSendCmd.Flags().String("file-path", "", "本地文件路径（msgType=file 时可直接上传发送）")
	chatMessageSendCmd.Flags().Int64("file-size", 0, "文件大小，单位字节")
	_ = chatMessageSendCmd.Flags().MarkHidden("dentry-id")
	_ = chatMessageSendCmd.Flags().MarkHidden("space-id")
	_ = chatMessageSendCmd.Flags().MarkHidden("file-name")
	_ = chatMessageSendCmd.Flags().MarkHidden("file-type")
	_ = chatMessageSendCmd.Flags().MarkHidden("file-size")
	chatMessageSendCmd.Flags().Bool("ai-tag", true, "消息是否带 AI 发送角标（默认 true）")
	chatMessageSendCmd.Flags().String("uuid", "", "幂等 UUID，相同 uuid 在 24h 内不会重复发送（可选）")

	chatMessageSendByBotCmd.Flags().String("robot-code", "", "机器人 Code (必填)")
	_ = chatMessageSendByBotCmd.MarkFlagRequired("robot-code")
	chatMessageSendByBotCmd.Flags().String("group", "", "群聊 openConversationId（群聊时必填）")
	chatMessageSendByBotCmd.Flags().String("users", "", "用户 userId 列表，逗号分隔，最多20个（单聊时必填）")
	chatMessageSendByBotCmd.Flags().String("title", "", "消息标题 (必填)")
	_ = chatMessageSendByBotCmd.MarkFlagRequired("title")
	chatMessageSendByBotCmd.Flags().String("text", "", "消息内容 Markdown (必填)")
	_ = chatMessageSendByBotCmd.MarkFlagRequired("text")
	chatMessageSendByBotCmd.Flags().String("at-user-ids", "", "@指定成员的 userId 列表，逗号分隔（仅群聊时生效，可选），--text 中需包含 @userId 对应文本")
	chatMessageSendByBotCmd.Flags().String("open-dingtalk-ids", "", "用户 openDingtalkId 列表，逗号分隔（单聊时可替代 --users，可选）")
	chatMessageSendByBotCmd.Flags().String("at-open-dingtalk-ids", "", "@指定成员的 openDingtalkId 列表，逗号分隔（仅群聊时生效，可选）")
	chatMessageSendByBotCmd.Flags().Bool("at-all", false, "@所有人（可选），服务端接收字符串 true/false")

	chatMessageRecallByBotCmd.Flags().String("robot-code", "", "机器人 Code (必填)")
	_ = chatMessageRecallByBotCmd.MarkFlagRequired("robot-code")
	chatMessageRecallByBotCmd.Flags().String("group", "", "群聊 openConversationId（群聊撤回时必填）")
	chatMessageRecallByBotCmd.Flags().String("keys", "", "消息 processQueryKey 列表，逗号分隔 (必填)")
	_ = chatMessageRecallByBotCmd.MarkFlagRequired("keys")

	chatMessageSendByWebhookCmd.Flags().String("token", "", "Webhook Token (必填)")
	_ = chatMessageSendByWebhookCmd.MarkFlagRequired("token")
	chatMessageSendByWebhookCmd.Flags().String("title", "", "消息标题 (必填)")
	_ = chatMessageSendByWebhookCmd.MarkFlagRequired("title")
	chatMessageSendByWebhookCmd.Flags().String("text", "", "消息内容 (必填)")
	_ = chatMessageSendByWebhookCmd.MarkFlagRequired("text")
	chatMessageSendByWebhookCmd.Flags().Bool("at-all", false, "@ 所有人")
	chatMessageSendByWebhookCmd.Flags().String("at-mobiles", "", "@ 指定手机号，逗号分隔")
	chatMessageSendByWebhookCmd.Flags().String("at-users", "", "@ 指定用户，逗号分隔")

	chatBotSearchCmd.Flags().Int("page", 1, "页码，从1开始")
	chatBotSearchCmd.Flags().Int("size", 0, "每页条数 (默认50)")
	chatBotSearchCmd.Flags().Int("limit", 0, "--size 的别名")
	_ = chatBotSearchCmd.Flags().MarkHidden("limit")
	chatBotSearchCmd.Flags().String("name", "", "按名称搜索")

	chatMessageListTopicRepliesCmd.Flags().String("group", "", "群会话 openconversationId (必填)")
	_ = chatMessageListTopicRepliesCmd.MarkFlagRequired("group")
	chatMessageListTopicRepliesCmd.Flags().String("topic-id", "", "话题 ID，由 dws chat message list 返回 (必填)")
	_ = chatMessageListTopicRepliesCmd.MarkFlagRequired("topic-id")
	chatMessageListTopicRepliesCmd.Flags().String("time", "", "开始时间，格式: yyyy-MM-dd HH:mm:ss（可选）")
	chatMessageListTopicRepliesCmd.Flags().Int("limit", 50, "返回数量（默认 50）")
	chatMessageListTopicRepliesCmd.Flags().String("direction", "", "时间方向: newer=从给定时间往现在拉，older=从给定时间往以前拉（推荐，默认 older）")
	chatMessageListTopicRepliesCmd.Flags().String("forward", "false", "true 等价 --direction newer，false 等价 --direction older（默认 false）")
	_ = chatMessageListTopicRepliesCmd.Flags().MarkHidden("forward")

	chatMessageListAllCmd.Flags().String("start", "", "起始时间，格式: yyyy-MM-dd HH:mm:ss (必填)")
	_ = chatMessageListAllCmd.MarkFlagRequired("start")
	chatMessageListAllCmd.Flags().String("end", "", "结束时间，格式: yyyy-MM-dd HH:mm:ss (必填)")
	_ = chatMessageListAllCmd.MarkFlagRequired("end")
	chatMessageListAllCmd.Flags().Int("limit", 50, "每页返回数量 (必填，默认 50)")
	chatMessageListAllCmd.Flags().Int("size", 0, "--limit 的旧版别名")
	_ = chatMessageListAllCmd.Flags().MarkHidden("size")
	chatMessageListAllCmd.Flags().String("cursor", "0", "分页游标（首页传 \"0\"，后续从响应中获取）")

	// list-by-sender flags
	chatMessageListBySenderCmd.Flags().String("sender-user-id", "", "发送者 userId（与 --sender-open-dingtalk-id 二选一）")
	chatMessageListBySenderCmd.Flags().String("sender", "", "--sender-user-id 的旧版别名")
	_ = chatMessageListBySenderCmd.Flags().MarkHidden("sender")
	chatMessageListBySenderCmd.Flags().String("sender-open-dingtalk-id", "", "发送者 openDingTalkId（与 --sender-user-id 二选一，适用于无法获取 userId 的场景）")
	chatMessageListBySenderCmd.Flags().String("user", "", "")
	_ = chatMessageListBySenderCmd.Flags().MarkHidden("user")
	chatMessageListBySenderCmd.Flags().String("start", "", "开始时间，ISO-8601 格式 (必填)")
	_ = chatMessageListBySenderCmd.MarkFlagRequired("start")
	chatMessageListBySenderCmd.Flags().String("end", "", "结束时间，ISO-8601 格式 (必填)")
	chatMessageListBySenderCmd.Flags().Int("limit", 50, "每页返回数量（默认 50）")
	chatMessageListBySenderCmd.Flags().Int("size", 0, "--limit 的旧版别名")
	_ = chatMessageListBySenderCmd.Flags().MarkHidden("size")
	chatMessageListBySenderCmd.Flags().String("cursor", "0", "分页游标（默认 \"0\"，翻页传 nextCursor）")

	// list-mentions flags
	chatMessageListMentionsCmd.Flags().String("group", "", "群聊 openconversation_id（可选，不传则查全部）")
	chatMessageListMentionsCmd.Flags().String("start", "", "开始时间，ISO-8601 格式 (必填)")
	_ = chatMessageListMentionsCmd.MarkFlagRequired("start")
	chatMessageListMentionsCmd.Flags().String("end", "", "结束时间，ISO-8601 格式 (必填)")
	_ = chatMessageListMentionsCmd.MarkFlagRequired("end")
	chatMessageListMentionsCmd.Flags().Int("limit", 50, "每页返回数量（默认 50）")
	chatMessageListMentionsCmd.Flags().Int("size", 0, "--limit 的旧版别名")
	_ = chatMessageListMentionsCmd.Flags().MarkHidden("size")
	chatMessageListMentionsCmd.Flags().String("cursor", "0", "分页游标（默认 \"0\"，翻页传 nextCursor）")

	// list-focused flags
	chatMessageListFocusedCmd.Flags().Int("limit", 50, "每页返回数量（默认 50）")
	chatMessageListFocusedCmd.Flags().Int64("cursor", 0, "分页游标（首次不传或传 0，翻页传 nextCursor）")

	// list-top-conversations flags
	chatMessageListTopConversationsCmd.Flags().Int("limit", 1000, "每页返回数量（默认 1000）")
	chatMessageListTopConversationsCmd.Flags().Int64("cursor", 0, "分页游标（首次不传或传 0，翻页传 nextCursor）")
	chatMessageListTopConversationsCmd.Flags().Bool("exclude-muted", false, "是否排除已设置免打扰的会话（默认 false）")

	chatMessageListUnreadConversationsCmd.Flags().Int("count", 0, "返回未读会话条数（可选，不传则使用服务端默认值）")
	chatMessageListUnreadConversationsCmd.Flags().Bool("exclude-muted", false, "是否排除已设置免打扰的会话（默认 false）")

	// message search flags
	chatMessageSearchCmd.Flags().String("query", "", "搜索关键词 (必填)")
	chatMessageSearchCmd.Flags().String("keyword", "", "--query 的别名")
	_ = chatMessageSearchCmd.Flags().MarkHidden("keyword")
	chatMessageSearchCmd.Flags().String("group", "", "群聊 openconversation_id（可选，不传则搜索所有会话）")
	chatMessageSearchCmd.Flags().String("start", "", "开始时间，ISO-8601 格式 (必填)")
	_ = chatMessageSearchCmd.MarkFlagRequired("start")
	chatMessageSearchCmd.Flags().String("end", "", "结束时间，ISO-8601 格式 (必填)")
	_ = chatMessageSearchCmd.MarkFlagRequired("end")
	chatMessageSearchCmd.Flags().Int("limit", 100, "每页返回数量（默认 100）")
	chatMessageSearchCmd.Flags().Int("size", 0, "--limit 的旧版别名")
	_ = chatMessageSearchCmd.Flags().MarkHidden("size")
	chatMessageSearchCmd.Flags().String("cursor", "0", "分页游标（默认 \"0\"，翻页传 nextCursor）")

	// read-status flags (主 flag 为 --conversation-id，因为支持群聊和单聊)
	chatMessageReadStatusCmd.Flags().String("conversation-id", "", "会话 openConversationId (必填，群聊或单聊均可)")
	_ = chatMessageReadStatusCmd.MarkFlagRequired("conversation-id")
	chatMessageReadStatusCmd.Flags().String("group", "", "--conversation-id 的别名")
	_ = chatMessageReadStatusCmd.Flags().MarkHidden("group")
	chatMessageReadStatusCmd.Flags().String("id", "", "--conversation-id 的别名")
	_ = chatMessageReadStatusCmd.Flags().MarkHidden("id")
	chatMessageReadStatusCmd.Flags().String("chat", "", "--conversation-id 的别名")
	_ = chatMessageReadStatusCmd.Flags().MarkHidden("chat")
	chatMessageReadStatusCmd.Flags().String("message-id", "", "消息 openMessageId，由 chat message list 返回 (必填)")
	_ = chatMessageReadStatusCmd.MarkFlagRequired("message-id")
	chatMessageReadStatusCmd.Flags().String("user", "", "目标用户 userId，支持逗号分隔（可选，不传则查所有接收者）")
	chatMessageReadStatusCmd.Flags().String("users", "", "目标用户 userId 列表，逗号分隔（可选，不传则查所有接收者）")
	chatMessageReadStatusCmd.Flags().String("userId", "", "--user 的别名")
	_ = chatMessageReadStatusCmd.Flags().MarkHidden("userId")
	chatMessageReadStatusCmd.Flags().String("target-open-dingtalk-ids", "", "目标用户 openDingTalkId 列表，逗号分隔（可选，不传则查所有接收者）")

	// search-common flags
	chatSearchCommonCmd.Flags().String("nicks", "", "要搜索的昵称列表，逗号分隔 (必填)")
	_ = chatSearchCommonCmd.MarkFlagRequired("nicks")
	chatSearchCommonCmd.Flags().String("match-mode", "AND", "匹配模式：AND=所有人都在群里，OR=任一人在群里（默认 AND）")
	chatSearchCommonCmd.Flags().Int("limit", 20, "每页返回数量（默认 20）")
	chatSearchCommonCmd.Flags().Int("size", 0, "--limit 的旧版别名")
	_ = chatSearchCommonCmd.Flags().MarkHidden("size")
	chatSearchCommonCmd.Flags().String("cursor", "0", "分页游标（默认 \"0\"，翻页传 nextCursor）")
	chatSearchCommonCmd.Flags().Bool("exclude-muted", false, "是否排除已设置免打扰的群聊（默认 false）")
	chatMessageSearchCommonCmd.Flags().String("nicks", "", "要搜索的昵称列表，逗号分隔 (必填)")
	_ = chatMessageSearchCommonCmd.MarkFlagRequired("nicks")
	chatMessageSearchCommonCmd.Flags().String("match-mode", "AND", "匹配模式：AND=所有人都在群里，OR=任一人在群里（默认 AND）")
	chatMessageSearchCommonCmd.Flags().Int("limit", 20, "每页返回数量（默认 20）")
	chatMessageSearchCommonCmd.Flags().Int("size", 0, "")
	_ = chatMessageSearchCommonCmd.Flags().MarkHidden("size")
	chatMessageSearchCommonCmd.Flags().String("cursor", "0", "分页游标（默认 \"0\"，翻页传 nextCursor）")
	chatMessageSearchCommonCmd.Flags().Bool("exclude-muted", false, "是否排除已设置免打扰的群聊（默认 false）")
	chatMessageSearchCommonCmd.Flags().String("group", "", "")
	_ = chatMessageSearchCommonCmd.Flags().MarkHidden("group")

	// search-advanced flags
	chatMessageSearchAdvancedCmd.Flags().String("query", "", "搜索关键词（可选）")
	chatMessageSearchAdvancedCmd.Flags().String("keyword", "", "--query 的别名")
	_ = chatMessageSearchAdvancedCmd.Flags().MarkHidden("keyword")
	chatMessageSearchAdvancedCmd.Flags().String("user", "", "发送者 userId，支持逗号分隔（可选）")
	chatMessageSearchAdvancedCmd.Flags().String("users", "", "发送者 userId 列表，逗号分隔（可选）")
	chatMessageSearchAdvancedCmd.Flags().String("userId", "", "--user 的别名")
	_ = chatMessageSearchAdvancedCmd.Flags().MarkHidden("userId")
	chatMessageSearchAdvancedCmd.Flags().String("sender-ids", "", "发送者 openDingTalkId 列表，逗号分隔（可选）")
	chatMessageSearchAdvancedCmd.Flags().String("senders", "", "--sender-ids 的旧版别名")
	_ = chatMessageSearchAdvancedCmd.Flags().MarkHidden("senders")
	chatMessageSearchAdvancedCmd.Flags().String("sender", "", "--sender-ids 的旧版别名")
	_ = chatMessageSearchAdvancedCmd.Flags().MarkHidden("sender")
	chatMessageSearchAdvancedCmd.Flags().Bool("at-me", false, "只搜索 @我 的消息（可选，默认 false）")
	chatMessageSearchAdvancedCmd.Flags().String("at-ids", "", "@指定人的 openDingTalkId 列表，逗号分隔（可选）")
	chatMessageSearchAdvancedCmd.Flags().String("conversation-ids", "", "会话 openConversationId 列表，逗号分隔（可选，群聊或单聊均可，不传则搜索所有会话）")
	chatMessageSearchAdvancedCmd.Flags().String("groups", "", "--conversation-ids 的别名")
	_ = chatMessageSearchAdvancedCmd.Flags().MarkHidden("groups")
	chatMessageSearchAdvancedCmd.Flags().String("group", "", "")
	_ = chatMessageSearchAdvancedCmd.Flags().MarkHidden("group")
	chatMessageSearchAdvancedCmd.Flags().String("start", "", "开始时间，ISO-8601 格式（可选）")
	chatMessageSearchAdvancedCmd.Flags().String("end", "", "结束时间，ISO-8601 格式（可选）")
	chatMessageSearchAdvancedCmd.Flags().String("cursor", "0", "分页游标（默认 \"0\"）")
	chatMessageSearchAdvancedCmd.Flags().Int("limit", 100, "每页返回数量（默认 100）")
	chatMessageSearchAdvancedCmd.Flags().Int("size", 0, "--limit 的旧版别名")
	_ = chatMessageSearchAdvancedCmd.Flags().MarkHidden("size")

	// query-send-status flags
	chatMessageQuerySendStatusCmd.Flags().String("open-task-id", "", "消息发送任务 ID (必填)")
	_ = chatMessageQuerySendStatusCmd.MarkFlagRequired("open-task-id")

	// recall flags
	chatMessageRecallCmd.Flags().String("conversation-id", "", "会话 openConversationId (必填)")
	chatMessageRecallCmd.Flags().String("group", "", "--conversation-id 的别名")
	_ = chatMessageRecallCmd.Flags().MarkHidden("group")
	chatMessageRecallCmd.Flags().String("id", "", "--conversation-id 的别名")
	_ = chatMessageRecallCmd.Flags().MarkHidden("id")
	chatMessageRecallCmd.Flags().String("chat", "", "--conversation-id 的别名")
	_ = chatMessageRecallCmd.Flags().MarkHidden("chat")
	chatMessageRecallCmd.Flags().String("msg-id", "", "消息 openMessageId (必填)")
	_ = chatMessageRecallCmd.MarkFlagRequired("msg-id")

	// 别名注册: --conversation-id/--id/--chat → --group (chat message 子命令)
	groupAliasCmds := []*cobra.Command{
		chatMessageListCmd, chatMessageSendCmd, chatMessageSendByBotCmd,
		chatMessageRecallByBotCmd, chatMessageListTopicRepliesCmd, chatMessageListMentionsCmd,
		chatMessageSearchCmd,
	}
	for _, c := range groupAliasCmds {
		c.Flags().String("conversation-id", "", "--group 的别名")
		_ = c.Flags().MarkHidden("conversation-id")
		if c.Flags().Lookup("id") == nil {
			c.Flags().String("id", "", "--group 的别名")
			_ = c.Flags().MarkHidden("id")
		}
		c.Flags().String("chat", "", "--group 的别名")
		_ = c.Flags().MarkHidden("chat")
	}

	// conversation-info: 获取会话基础信息
	chatConversationInfoCmd := &cobra.Command{
		Use:   "conversation-info",
		Short: "获取会话基础信息",
		Long: `获取指定会话的基础信息。
发送本地文件消息请优先使用 dws chat message send --msg-type file --file-path <本地文件>，CLI 不再要求调用方获取或传递 spaceId。`,
		Example: `  dws chat conversation-info --group <openConversationId> --format json
  dws chat conversation-info --user <userId> --format json
  dws chat conversation-info --open-dingtalk-id <openDingTalkId> --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			groupID := flagOrFallback(cmd, "group", "conversation-id", "id", "chat")
			rawOpenDingTalkID, _ := cmd.Flags().GetString("open-dingtalk-id")
			rawUserID := flagOrFallback(cmd, "user", "userId")
			specified := 0
			for _, value := range []string{groupID, rawUserID, rawOpenDingTalkID} {
				if value != "" {
					specified++
				}
			}
			if specified > 1 {
				return fmt.Errorf("--group, --user and --open-dingtalk-id are mutually exclusive, specify exactly one")
			}
			if specified == 0 {
				return fmt.Errorf("--group, --user or --open-dingtalk-id is required")
			}

			userID := rawUserID
			openDingTalkID := rawOpenDingTalkID
			if openDingTalkID != "" && !isOpenDingTalkID(openDingTalkID) {
				userID = openDingTalkID
				openDingTalkID = ""
			}
			if userID != "" && isOpenDingTalkID(userID) {
				openDingTalkID = userID
				userID = ""
			}
			toolArgs := map[string]any{}
			if groupID != "" {
				toolArgs["openConversationId"] = groupID
			}
			if userID != "" {
				toolArgs["userId"] = userID
			}
			if openDingTalkID != "" {
				toolArgs["openDingTalkId"] = openDingTalkID
			}
			return callMCPToolOnServer("chat", "get_conversation_info", toolArgs)
		},
	}
	chatConversationInfoCmd.Flags().String("group", "", "群聊 openConversationId（群聊时使用）")
	chatConversationInfoCmd.Flags().String("user", "", "单聊对方 userId（单聊时使用）")
	chatConversationInfoCmd.Flags().String("userId", "", "--user 的别名")
	_ = chatConversationInfoCmd.Flags().MarkHidden("userId")
	chatConversationInfoCmd.Flags().String("open-dingtalk-id", "", "单聊对方 openDingTalkId（单聊时使用）")

	// ── file 子命令（会话文件上传，不暴露 spaceId）───────────────

	chatFileCmd := &cobra.Command{
		Use:    "file",
		Short:  "会话文件上传（已下线）",
		Hidden: true,
		RunE:   groupRunE,
	}

	chatFileUploadCmd := &cobra.Command{
		Use:    "upload",
		Short:  "上传本地文件或 URL 文件到会话文件空间（已下线）",
		Hidden: true,
		Long: `chat file upload 已下线，不再调用 chat/upload_conversation_file_by_url。

发送本地文件消息请改用 chat message send --msg-type file --file-path；该路径仍然可用，CLI 内部会完成本地文件上传和消息发送。`,
		Example: `  dws chat message send --group <openConversationId> --msg-type file --file-path ./report.pdf --format json
  dws chat message send --open-dingtalk-id <openDingTalkId> --msg-type file --file-path ./report.pdf --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("chat file upload 已下线；chat/upload_conversation_file_by_url 当前不可用。发送本地文件请改用: dws chat message send --msg-type file --file-path <本地路径>")
		},
	}
	chatFileUploadCmd.Flags().String("group", "", "群聊 openConversationId（群聊时使用）")
	chatFileUploadCmd.Flags().String("conversation-id", "", "--group 的别名")
	_ = chatFileUploadCmd.Flags().MarkHidden("conversation-id")
	chatFileUploadCmd.Flags().String("id", "", "--group 的别名")
	_ = chatFileUploadCmd.Flags().MarkHidden("id")
	chatFileUploadCmd.Flags().String("chat", "", "--group 的别名")
	_ = chatFileUploadCmd.Flags().MarkHidden("chat")
	chatFileUploadCmd.Flags().String("user", "", "单聊对方 userId（单聊时使用）")
	chatFileUploadCmd.Flags().String("userId", "", "--user 的别名")
	_ = chatFileUploadCmd.Flags().MarkHidden("userId")
	chatFileUploadCmd.Flags().String("open-dingtalk-id", "", "单聊对方 openDingTalkId（单聊时使用）")
	chatFileUploadCmd.Flags().String("file", "", "本地文件路径（与 --url 二选一）")
	chatFileUploadCmd.Flags().String("url", "", "远程文件 URL（与 --file 二选一，服务端代传）")
	chatFileUploadCmd.Flags().String("file-name", "", "文件名（可选，本地文件默认取文件名，URL 默认从 URL 推断）")
	chatFileUploadCmd.Flags().String("md5", "", "文件 MD5（可选，本地文件不传时自动计算）")
	chatFileUploadCmd.Flags().String("uuid", "", "幂等 UUID（可选）")
	chatFileCmd.AddCommand(chatFileUploadCmd)

	// ── category 子命令（会话分组，走 IM MCP）───────────────────

	chatCategoryCmd := &cobra.Command{Use: "category", Short: "会话分组管理", RunE: groupRunE}

	chatCategoryListCmd := &cobra.Command{
		Use:   "list",
		Short: "获取用户自定义会话分组",
		Example: `  dws chat category list
  # 返回当前用户的所有自定义会话分组`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPToolOnServer("im", "list_user_define_conv_categories", map[string]any{})
		},
	}

	chatCategoryConvsCmd := &cobra.Command{
		Use:   "list-conversations",
		Short: "拉取指定自定义会话分组下的会话",
		Example: `  dws chat category list-conversations --category-id <分组ID>
  # 分组ID 可通过 dws chat category list 获取`,
		RunE: func(cmd *cobra.Command, args []string) error {
			categoryId, _ := cmd.Flags().GetInt("category-id")
			toolArgs := map[string]any{
				"categoryId": categoryId,
			}
			if v, _ := cmd.Flags().GetBool("exclude-muted"); v {
				toolArgs["excludeMuted"] = true
			}
			return callMCPToolOnServer("im", "list_conversations_by_category", toolArgs)
		},
	}

	chatCategoryCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "创建用户自定义会话分组",
		Example: `  dws chat category create --title "工作群"
  dws chat category create --title "项目组"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "title"); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "create_conv_category", map[string]any{
				"title": mustGetFlag(cmd, "title"),
			})
		},
	}

	chatCategoryDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "删除用户自定义会话分组",
		Example: `  dws chat category delete --category-id <分组ID>
  # 分组ID 可通过 dws chat category list 获取`,
		RunE: func(cmd *cobra.Command, args []string) error {
			categoryId, _ := cmd.Flags().GetInt64("category-id")
			if categoryId == 0 {
				return fmt.Errorf("flag --category-id is required")
			}
			return callMCPToolOnServer("im", "delete_conv_category", map[string]any{
				"categoryId": categoryId,
			})
		},
	}

	chatCategoryRenameCmd := &cobra.Command{
		Use:   "rename",
		Short: "更新用户自定义会话分组的名称",
		Example: `  dws chat category rename --category-id <分组ID> --title "新名称"
  # 分组ID 可通过 dws chat category list 获取`,
		RunE: func(cmd *cobra.Command, args []string) error {
			categoryId, _ := cmd.Flags().GetInt64("category-id")
			if categoryId == 0 {
				return fmt.Errorf("flag --category-id is required")
			}
			if err := validateRequiredFlags(cmd, "title"); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "rename_conv_category", map[string]any{
				"categoryId": categoryId,
				"title":      mustGetFlag(cmd, "title"),
			})
		},
	}

	chatCategoryAddConvCmd := &cobra.Command{
		Use:   "add-conv",
		Short: "将会话移动到指定的自定义分组中",
		Long:  `将某个会话添加到一批用户自定义会话分组中。需指定会话 openConversationId 和目标分组 ID 列表。`,
		Example: `  dws chat category add-conv --group <openConversationId> --category-ids 123,456
  # 分组ID 可通过 dws chat category list 获取
  # 查询群 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			groupID := flagOrFallback(cmd, "group", "conversation-id", "id")
			if groupID == "" {
				return fmt.Errorf("flag --group is required")
			}
			if err := validateRequiredFlags(cmd, "category-ids"); err != nil {
				return err
			}
			categoryIds, err := parseCSVInt64(mustGetFlag(cmd, "category-ids"))
			if err != nil {
				return fmt.Errorf("--category-ids: %w", err)
			}
			return callMCPToolOnServer("im", "add_conv_to_categories", map[string]any{
				"openConversationId": groupID,
				"categoryIds":        categoryIds,
			})
		},
	}

	chatCategoryRemoveConvCmd := &cobra.Command{
		Use:   "remove-conv",
		Short: "将会话从指定的自定义分组中移出",
		Long:  `将某个会话从一批用户自定义会话分组中移出。需指定会话 openConversationId 和目标分组 ID 列表。`,
		Example: `  dws chat category remove-conv --group <openConversationId> --category-ids 123,456
  # 分组ID 可通过 dws chat category list 获取
  # 查询群 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			groupID := flagOrFallback(cmd, "group", "conversation-id", "id")
			if groupID == "" {
				return fmt.Errorf("flag --group is required")
			}
			if err := validateRequiredFlags(cmd, "category-ids"); err != nil {
				return err
			}
			categoryIds, err := parseCSVInt64(mustGetFlag(cmd, "category-ids"))
			if err != nil {
				return fmt.Errorf("--category-ids: %w", err)
			}
			return callMCPToolOnServer("im", "remove_conv_from_categories", map[string]any{
				"openConversationId": groupID,
				"categoryIds":        categoryIds,
			})
		},
	}

	// ── group get-by-group-id（走 IM MCP）─────────────────────────

	chatGroupInfoByIdCmd := &cobra.Command{
		Use:   "get-by-group-id",
		Short: "根据群号获取群聊信息",
		Example: `  dws chat group get-by-group-id --group-id 12345678
  # 群号为数字类型的群ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			groupId, _ := cmd.Flags().GetInt64("group-id")
			return callMCPToolOnServer("im", "get_conv_info_by_group_id", map[string]any{
				"groupId": groupId,
			})
		},
	}

	// ── message 新增命令（批量查消息、emoji 表情、文字表情）─────

	chatMessageListByIdsCmd := &cobra.Command{
		Use:   "list-by-ids",
		Short: "根据消息 ID 批量查询消息",
		Example: `  dws chat message list-by-ids --msg-ids msgId1,msgId2,msgId3
  # 最多传 50 条消息 ID`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "msg-ids"); err != nil {
				return err
			}
			msgIds := parseCSVValues(mustGetFlag(cmd, "msg-ids"))
			if len(msgIds) > 50 {
				return fmt.Errorf("--msg-ids 最多支持 50 条，当前 %d 条", len(msgIds))
			}
			return callMCPToolOnServer("im", "list_messages_by_ids", map[string]any{
				"openMsgIds": msgIds,
			})
		},
	}
	chatMessageListByIdsCmd.Flags().String("msg-ids", "", "消息 ID 列表，逗号分隔，最多 50 条 (必填)")
	_ = chatMessageListByIdsCmd.MarkFlagRequired("msg-ids")

	chatMessageAddEmojiCmd := &cobra.Command{
		Use:   "add-emoji",
		Short: "对消息添加 emoji 表情回应",
		Example: `  dws chat message add-emoji --conversation-id <openConversationId> --msg-id <openMsgId> --emoji "赞"
  # 查询会话 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "conversation-id", "group", "id", "chat"); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "msg-id", "emoji"); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "add_emoji_reaction", map[string]any{
				"openConversationId": flagOrFallback(cmd, "conversation-id", "group", "id", "chat"),
				"openMsgId":          mustGetFlag(cmd, "msg-id"),
				"emojiName":          mustGetFlag(cmd, "emoji"),
			})
		},
	}
	chatMessageAddEmojiCmd.Flags().String("conversation-id", "", "会话 openConversationId (必填，支持单聊/群聊)")
	chatMessageAddEmojiCmd.Flags().String("group", "", "--conversation-id 的别名")
	chatMessageAddEmojiCmd.Flags().String("id", "", "--conversation-id 的别名")
	chatMessageAddEmojiCmd.Flags().String("chat", "", "--conversation-id 的别名")
	chatMessageAddEmojiCmd.Flags().String("msg-id", "", "消息 openMsgId (必填)")
	chatMessageAddEmojiCmd.Flags().String("emoji", "", "emoji 表情名称 (必填)")

	chatMessageRemoveEmojiCmd := &cobra.Command{
		Use:   "remove-emoji",
		Short: "移除消息的 emoji 表情回应",
		Example: `  dws chat message remove-emoji --conversation-id <openConversationId> --msg-id <openMsgId> --emoji "赞"
  # 查询会话 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "conversation-id", "group", "id", "chat"); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "msg-id", "emoji"); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "remove_emoji_reaction", map[string]any{
				"openConversationId": flagOrFallback(cmd, "conversation-id", "group", "id", "chat"),
				"openMsgId":          mustGetFlag(cmd, "msg-id"),
				"emojiName":          mustGetFlag(cmd, "emoji"),
			})
		},
	}
	chatMessageRemoveEmojiCmd.Flags().String("conversation-id", "", "会话 openConversationId (必填，支持单聊/群聊)")
	chatMessageRemoveEmojiCmd.Flags().String("group", "", "--conversation-id 的别名")
	chatMessageRemoveEmojiCmd.Flags().String("id", "", "--conversation-id 的别名")
	chatMessageRemoveEmojiCmd.Flags().String("chat", "", "--conversation-id 的别名")
	chatMessageRemoveEmojiCmd.Flags().String("msg-id", "", "消息 openMsgId (必填)")
	chatMessageRemoveEmojiCmd.Flags().String("emoji", "", "emoji 表情名称 (必填)")

	chatMessageAddTextEmotionCmd := &cobra.Command{
		Use:     "add-text-emotion",
		Short:   "对消息添加文字表情回应",
		Example: `  dws chat message add-text-emotion --conversation-id <openConversationId> --msg-id <openMsgId> --emotion-id <emotionId> --emotion-name "赞" --text "nice" --background-id im_bg_5`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "conversation-id", "group", "id", "chat"); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "msg-id", "emotion-id", "emotion-name", "text", "background-id"); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "add_text_emotion", map[string]any{
				"openConversationId": flagOrFallback(cmd, "conversation-id", "group", "id", "chat"),
				"openMsgId":          mustGetFlag(cmd, "msg-id"),
				"emotionId":          mustGetFlag(cmd, "emotion-id"),
				"emotionName":        mustGetFlag(cmd, "emotion-name"),
				"text":               mustGetFlag(cmd, "text"),
				"backgroundId":       mustGetFlag(cmd, "background-id"),
			})
		},
	}
	chatMessageAddTextEmotionCmd.Flags().String("conversation-id", "", "会话 openConversationId (必填，支持单聊/群聊)")
	chatMessageAddTextEmotionCmd.Flags().String("group", "", "--conversation-id 的别名")
	chatMessageAddTextEmotionCmd.Flags().String("id", "", "--conversation-id 的别名")
	chatMessageAddTextEmotionCmd.Flags().String("chat", "", "--conversation-id 的别名")
	chatMessageAddTextEmotionCmd.Flags().String("msg-id", "", "消息 openMsgId (必填)")
	chatMessageAddTextEmotionCmd.Flags().String("emotion-id", "", "表情 ID (必填，通过 create-text-emotion 获取)")
	chatMessageAddTextEmotionCmd.Flags().String("emotion-name", "", "表情名称 (必填)")
	chatMessageAddTextEmotionCmd.Flags().String("text", "", "文字内容 (必填)")
	chatMessageAddTextEmotionCmd.Flags().String("background-id", "", "背景 ID (必填)")

	chatMessageRemoveTextEmotionCmd := &cobra.Command{
		Use:     "remove-text-emotion",
		Short:   "移除消息的文字表情回应",
		Example: `  dws chat message remove-text-emotion --conversation-id <openConversationId> --msg-id <openMsgId> --emotion-id <emotionId> --emotion-name "赞" --text "nice" --background-id <backgroundId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "conversation-id", "group", "id", "chat"); err != nil {
				return err
			}
			if err := validateRequiredFlags(cmd, "msg-id", "emotion-id", "emotion-name", "text", "background-id"); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "remove_text_emotion", map[string]any{
				"openConversationId": flagOrFallback(cmd, "conversation-id", "group", "id", "chat"),
				"openMsgId":          mustGetFlag(cmd, "msg-id"),
				"emotionId":          mustGetFlag(cmd, "emotion-id"),
				"emotionName":        mustGetFlag(cmd, "emotion-name"),
				"text":               mustGetFlag(cmd, "text"),
				"backgroundId":       mustGetFlag(cmd, "background-id"),
			})
		},
	}
	chatMessageRemoveTextEmotionCmd.Flags().String("conversation-id", "", "会话 openConversationId (必填，支持单聊/群聊)")
	chatMessageRemoveTextEmotionCmd.Flags().String("group", "", "--conversation-id 的别名")
	chatMessageRemoveTextEmotionCmd.Flags().String("id", "", "--conversation-id 的别名")
	chatMessageRemoveTextEmotionCmd.Flags().String("chat", "", "--conversation-id 的别名")
	chatMessageRemoveTextEmotionCmd.Flags().String("msg-id", "", "消息 openMsgId (必填)")
	chatMessageRemoveTextEmotionCmd.Flags().String("emotion-id", "", "表情 ID (必填)")
	chatMessageRemoveTextEmotionCmd.Flags().String("emotion-name", "", "表情名称 (必填)")
	chatMessageRemoveTextEmotionCmd.Flags().String("text", "", "文字内容 (必填)")
	chatMessageRemoveTextEmotionCmd.Flags().String("background-id", "", "背景 ID (必填)")

	// ── 创建文字表情（获取 emotionId）──────────────────────

	chatMessageCreateTextEmotionCmd := &cobra.Command{
		Use:   "create-text-emotion",
		Short: "创建文字表情（获取 emotionId）",
		Long:  `创建一个新的文字表情模板。当 list-emotions 中没有所需表情时，使用此命令创建并获取 emotionId，随后可用于 add-text-emotion。`,
		Example: `  dws chat message create-text-emotion --emotion-name "赞" --text "nice"
  dws chat message create-text-emotion --emotion-name "感谢" --text "感谢" --background-id im_bg_5`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "emotion-name", "text"); err != nil {
				return err
			}
			params := map[string]any{
				"emotionName": mustGetFlag(cmd, "emotion-name"),
				"text":        mustGetFlag(cmd, "text"),
			}
			if v, _ := cmd.Flags().GetString("background-id"); v != "" {
				params["backgroundId"] = v
			}
			return callMCPToolOnServer("im", "create_text_emotion", params)
		},
	}
	chatMessageCreateTextEmotionCmd.Flags().String("emotion-name", "", "表情名称 (必填)")
	_ = chatMessageCreateTextEmotionCmd.MarkFlagRequired("emotion-name")
	chatMessageCreateTextEmotionCmd.Flags().String("text", "", "文字内容 (必填)")
	_ = chatMessageCreateTextEmotionCmd.MarkFlagRequired("text")
	chatMessageCreateTextEmotionCmd.Flags().String("background-id", "", "背景 ID（可选，不传则由服务端默认分配）")

	// ── 流式卡片命令 ──────────────────────────────────────────

	chatMessageSendCardCmd := &cobra.Command{
		Use:   "send-card",
		Short: "创建并推送流式卡片",
		Long: `向群聊或单聊创建并推送流式卡片。群聊传 --group，单聊传 --receiver，二者互斥。
创建时无需传入卡片内容，后续通过 update-card 更新内容。

注意：send-card 必须和 update-card 搭配使用。发送卡片后，使用返回的 bizId 调用 update-card 更新内容，
最后一次更新必须将 --flow-status 设为 3（finish），否则卡片会一直处于"生成中"的加载状态。
flow-status 取值：1=处理中(PROCESSING)，2=输入中(INPUTTING)，3=完成(FINISH)，4=执行中(EXECUTING)，5=错误(ERROR)。`,
		Example: `  dws chat message send-card --group <openConversationId>
  dws chat message send-card --receiver <openDingTalkId>
  # 查询群 ID: dws chat search --query "群名"
  # 查询人员: dws contact user search --keyword "姓名" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			groupID := flagOrFallback(cmd, "group", "conversation-id", "id", "chat")
			receiver, _ := cmd.Flags().GetString("receiver")
			if groupID == "" && receiver == "" {
				return fmt.Errorf("--group or --receiver is required")
			}
			if groupID != "" && receiver != "" {
				return fmt.Errorf("--group and --receiver are mutually exclusive")
			}
			toolArgs := map[string]any{}
			if groupID != "" {
				toolArgs["openConversationId"] = groupID
			}
			if receiver != "" {
				resolved, err := resolveOpenDingTalkID(cmd.Context(), receiver)
				if err != nil {
					return err
				}
				toolArgs["receiverOpenDingTalkId"] = resolved
			}
			return callMCPToolOnServer("im", "create_and_send_card", toolArgs)
		},
	}
	chatMessageSendCardCmd.Flags().String("group", "", "群聊 openConversationId（群聊时必填，与 --receiver 互斥）")
	chatMessageSendCardCmd.Flags().String("receiver", "", "单聊接收者 openDingTalkId（单聊时必填，与 --group 互斥）")

	chatMessageUpdateCardCmd := &cobra.Command{
		Use:   "update-card",
		Short: "流式更新卡片内容",
		Long: `更新已发送的流式卡片内容。--biz-id 为 send-card 返回的业务 ID，--flow-status 控制流式状态。

flow-status 取值：1=处理中(PROCESSING)，2=输入中(INPUTTING)，3=完成(FINISH)，4=执行中(EXECUTING)，5=错误(ERROR)。
最后一次更新必须将 --flow-status 设为 3（finish），否则卡片会一直处于"生成中"的加载状态。`,
		Example: `  dws chat message update-card --biz-id <bizId> --content "更新的卡片内容" --flow-status 2
  dws chat message update-card --biz-id <bizId> --content "最终内容" --flow-status 3`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "biz-id", "content"); err != nil {
				return err
			}
			if !cmd.Flags().Changed("flow-status") {
				return fmt.Errorf("flag --flow-status is required")
			}
			flowStatus, _ := cmd.Flags().GetInt("flow-status")
			return callMCPToolOnServer("im", "update_streaming_card", map[string]any{
				"bizId":      mustGetFlag(cmd, "biz-id"),
				"msgContent": mustGetFlag(cmd, "content"),
				"flowStatus": flowStatus,
			})
		},
	}
	chatMessageUpdateCardCmd.Flags().String("biz-id", "", "卡片业务 ID (必填)")
	_ = chatMessageUpdateCardCmd.MarkFlagRequired("biz-id")
	chatMessageUpdateCardCmd.Flags().String("content", "", "卡片消息内容 (必填)")
	_ = chatMessageUpdateCardCmd.MarkFlagRequired("content")
	chatMessageUpdateCardCmd.Flags().Int("flow-status", 0, "流式状态 (必填)")

	// ── download-media：下载消息中的媒体资源（走 IM MCP）──────
	chatMessageDownloadMediaCmd := &cobra.Command{
		Use:   "download-media",
		Short: "下载消息中的资源（图片/视频/语音等）到本地",
		Long: `下载聊天消息中的图片、视频、语音等资源到本地文件。

流程:
  1. 根据 resourceType + resource-id + message-id + open-conversation-id 向服务端获取下载 URL
  2. HTTP GET 下载文件到本地

--output 指定本地保存路径，可以是文件路径或目录。
如果指定目录，文件名从下载 URL 中自动推断。默认保存到当前目录。`,
		Example: `  dws chat message download-media --type mediaId --resource-id <mediaId> --message-id <openMessageId> --open-conversation-id <openConversationId>
  dws chat message download-media --type mediaId --resource-id <mediaId> --message-id <openMessageId> --open-conversation-id <openConversationId> --output ./downloads/
  dws chat message download-media --type mediaId --resource-id <mediaId> --message-id <openMessageId> --open-conversation-id <openConversationId> --output ./photo.jpg
  # resource-id: 从 dws chat message list 返回的消息内容中获取 mediaId
  # message-id: 从 dws chat message list 返回的 openMessageId
  # open-conversation-id: 从 dws chat search 获取 openConversationId`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "type", "resource-id", "message-id", "open-conversation-id", "output"); err != nil {
				return err
			}

			resourceType := mustGetFlag(cmd, "type")
			resourceID := mustGetFlag(cmd, "resource-id")
			conversationID := mustGetFlag(cmd, "open-conversation-id")
			messageID := mustGetFlag(cmd, "message-id")
			outputPath := mustGetFlag(cmd, "output")

			switch resourceType {
			case "mediaId":
				// current supported type
			default:
				return fmt.Errorf("unsupported resource type: %s (supported: mediaId)", resourceType)
			}

			if deps.Caller.DryRun() {
				deps.Out.PrintKeyValue("操作", "下载消息资源")
				deps.Out.PrintKeyValue("类型", resourceType)
				deps.Out.PrintKeyValue("资源ID", resourceID)
				deps.Out.PrintKeyValue("消息ID", messageID)
				deps.Out.PrintKeyValue("会话ID", conversationID)
				deps.Out.PrintKeyValue("输出", outputPath)
				return nil
			}

			ctx := context.Background()

			// Step 1: 获取下载 URL
			deps.Out.PrintInfo("[1/2] 获取资源下载链接...")
			text, err := callMCPToolReturnTextOnServer(ctx, "im", "get_resource_download_url", map[string]any{
				"resourceType":       resourceType,
				"resourceId":         resourceID,
				"openMessageId":      messageID,
				"openConversationId": conversationID,
			})
			if err != nil {
				return err
			}

			resourceURL, dlHeaders, err := parseDownloadInfo(text)
			if err != nil {
				return err
			}

			// 解析输出路径：如果是目录，从 URL 推断文件名
			fi, statErr := os.Stat(outputPath)
			if statErr == nil && fi.IsDir() {
				filename := inferFilename(resourceURL)
				outputPath = filepath.Join(outputPath, filename)
			}

			// Step 2: HTTP GET 下载文件
			deps.Out.PrintInfo(fmt.Sprintf("[2/2] 下载资源到 %s ...", outputPath))
			if err := httpGetFile(ctx, resourceURL, dlHeaders, outputPath); err != nil {
				return err
			}

			deps.Out.PrintInfo(fmt.Sprintf("下载完成: %s", outputPath))
			return nil
		},
	}

	// download-media flags
	chatMessageDownloadMediaCmd.Flags().String("type", "", "资源类型: mediaId (必填)")
	_ = chatMessageDownloadMediaCmd.MarkFlagRequired("type")
	chatMessageDownloadMediaCmd.Flags().String("resource-id", "", "资源 ID，mediaId 类型时为消息中的 mediaId 值 (必填)")
	_ = chatMessageDownloadMediaCmd.MarkFlagRequired("resource-id")
	chatMessageDownloadMediaCmd.Flags().String("open-conversation-id", "", "会话 openConversationId (必填)")
	_ = chatMessageDownloadMediaCmd.MarkFlagRequired("open-conversation-id")
	chatMessageDownloadMediaCmd.Flags().String("message-id", "", "消息 openMessageId (必填)")
	_ = chatMessageDownloadMediaCmd.MarkFlagRequired("message-id")
	chatMessageDownloadMediaCmd.Flags().String("output", "", "本地保存路径，文件或目录 (必填)")
	_ = chatMessageDownloadMediaCmd.MarkFlagRequired("output")

	chatMessageCmd.AddCommand(chatMessageListCmd, chatMessageSendCmd, chatMessageSendByBotCmd, chatMessageRecallByBotCmd, chatMessageSendByWebhookCmd, chatMessageListTopicRepliesCmd, chatMessageListAllCmd, chatMessageListBySenderCmd, chatMessageListMentionsCmd, chatMessageListFocusedCmd, chatMessageListUnreadConversationsCmd, chatMessageSearchCmd, chatMessageListByIdsCmd, chatMessageAddEmojiCmd, chatMessageRemoveEmojiCmd, chatMessageAddTextEmotionCmd, chatMessageRemoveTextEmotionCmd, chatMessageCreateTextEmotionCmd, chatMessageSearchAdvancedCmd, chatMessageQuerySendStatusCmd, chatMessageRecallCmd, chatMessageReadStatusCmd, chatMessageSendCardCmd, chatMessageUpdateCardCmd, chatMessageDownloadMediaCmd)
	chatBotCmd.AddCommand(chatBotSearchCmd)
	chatCategoryCmd.AddCommand(chatCategoryListCmd, chatCategoryConvsCmd, chatCategoryCreateCmd, chatCategoryDeleteCmd, chatCategoryRenameCmd, chatCategoryAddConvCmd, chatCategoryRemoveConvCmd)
	chatGroupCmd.AddCommand(chatGroupInfoByIdCmd)

	// ── group 新增命令（群主转让、邀请链接、免打扰）──────────

	chatGroupTransferOwnerCmd := &cobra.Command{
		Use:   "transfer-owner",
		Short: "转让群主",
		Example: `  dws chat group transfer-owner --group <openConversationId> --new-owner <openDingTalkId>
  dws chat group transfer-owner --group <openConversationId> --user <userId>
  # 查询群 ID: dws chat search --query "群名"
  # 查询 openDingTalkId: dws contact user search --query "姓名"
  # 查询 userId: dws contact user search --query "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "group"); err != nil {
				return err
			}
			newOwnerOpenDingTalkID, _ := cmd.Flags().GetString("new-owner")
			newOwnerUserID := flagOrFallback(cmd, "user", "userId")
			if newOwnerOpenDingTalkID != "" && newOwnerUserID != "" {
				return fmt.Errorf("--new-owner and --user are mutually exclusive, specify exactly one")
			}
			newOwner := newOwnerOpenDingTalkID
			if newOwner == "" {
				newOwner = newOwnerUserID
			}
			if newOwner == "" {
				return fmt.Errorf("flag --new-owner or --user is required")
			}
			if !isOpenDingTalkID(newOwner) {
				return callMCPToolOnServer("im", "transfer_group_owner", map[string]any{
					"openConversationId": mustGetFlag(cmd, "group"),
					"newOwnerUid":        newOwner,
				})
			}
			return callMCPToolOnServer("im", "transfer_group_owner", map[string]any{
				"openConversationId":     mustGetFlag(cmd, "group"),
				"newOwnerOpenDingTalkId": newOwner,
			})
		},
	}
	chatGroupTransferOwnerCmd.Flags().String("group", "", "群聊 openConversationId (必填)")
	_ = chatGroupTransferOwnerCmd.MarkFlagRequired("group")
	chatGroupTransferOwnerCmd.Flags().String("new-owner", "", "新群主 openDingTalkId")
	chatGroupTransferOwnerCmd.Flags().String("user", "", "新群主 userId")
	chatGroupTransferOwnerCmd.Flags().String("userId", "", "--user 的别名")
	_ = chatGroupTransferOwnerCmd.Flags().MarkHidden("userId")

	chatGroupInviteUrlCmd := &cobra.Command{
		Use:   "invite-url",
		Short: "获取群邀请链接",
		Long:  `获取群聊邀请链接。可选 --expires-seconds 指定链接有效期（秒），0 表示永久有效，不传则使用服务端默认值。`,
		Example: `  dws chat group invite-url --group <openConversationId>
  dws chat group invite-url --group <openConversationId> --expires-seconds 86400
  dws chat group invite-url --group <openConversationId> --expires-seconds 0
  # 查询群 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "group"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"openConversationId": mustGetFlag(cmd, "group"),
			}
			if v, _ := cmd.Flags().GetInt64("expires-seconds"); v >= 0 && cmd.Flags().Changed("expires-seconds") {
				toolArgs["expiresSeconds"] = v
			}
			return callMCPToolOnServer("im", "get_group_invite_url", toolArgs)
		},
	}
	chatGroupInviteUrlCmd.Flags().String("group", "", "群聊 openConversationId (必填)")
	_ = chatGroupInviteUrlCmd.MarkFlagRequired("group")
	chatGroupInviteUrlCmd.Flags().Int64("expires-seconds", 0, "链接有效期（秒），0 表示永久有效，不传使用服务端默认值")

	chatMuteCmd := &cobra.Command{
		Use:   "mute",
		Short: "会话消息免打扰",
		Long:  `开启或关闭会话消息免打扰（支持单聊和群聊）。默认开启免打扰，传 --off 则关闭免打扰。`,
		Example: `  dws chat mute --conversation-id <openConversationId>
  dws chat mute --conversation-id <openConversationId> --off
  # 查询群 ID: dws chat search --query "群名"
  # 查询单聊会话 ID: dws chat conversation-info --user <userId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			convID := flagOrFallback(cmd, "conversation-id", "id", "chat")
			if convID == "" {
				return fmt.Errorf("flag --conversation-id is required\n  hint: dws chat mute --conversation-id <openConversationId>")
			}
			off, _ := cmd.Flags().GetBool("off")
			return callMCPToolOnServer("im", "update_notification_off", map[string]any{
				"openConversationId": convID,
				"mute":               !off,
			})
		},
	}
	chatMuteCmd.Flags().String("conversation-id", "", "会话 openConversationId (必填，支持单聊/群聊)")
	chatMuteCmd.Flags().String("id", "", "--conversation-id 的别名")
	chatMuteCmd.Flags().String("chat", "", "--conversation-id 的别名")
	chatMuteCmd.Flags().Bool("off", false, "关闭免打扰（不传则开启免打扰）")

	// ── group 新增命令（退群、更新群头像、更新群设置）──────────

	chatGroupQuitCmd := &cobra.Command{
		Use:   "quit",
		Short: "退出群聊",
		Long:  `当前用户退出指定群聊。退出后将无法查看群消息。`,
		Example: `  dws chat group quit --group <openConversationId>
  # 查询群 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "group"); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "quit_group", map[string]any{
				"openConversationId": mustGetFlag(cmd, "group"),
			})
		},
	}
	chatGroupQuitCmd.Flags().String("group", "", "群聊 openConversationId (必填)")
	_ = chatGroupQuitCmd.MarkFlagRequired("group")

	chatGroupUpdateIconCmd := &cobra.Command{
		Use:   "update-icon",
		Short: "更新群头像",
		Long:  `更新指定群聊的群头像。需传入群 ID 和头像 mediaId。`,
		Example: `  dws chat group update-icon --group <openConversationId> --icon-media-id <mediaId>
  # 查询群 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "group", "icon-media-id"); err != nil {
				return err
			}
			iconMediaID := strings.TrimSpace(mustGetFlag(cmd, "icon-media-id"))
			// 客户端预校验：钉钉媒体 ID 由上传接口（如 dt_media_upload）返回，
			// 固定以 @ 开头（形如 @lADP.../@lQDP...）。明显非法的值本地直接报错。
			if !dingTalkMediaIDPattern.MatchString(iconMediaID) {
				return fmt.Errorf("invalid --icon-media-id %q: 钉钉媒体 ID 应以 @ 开头（如 @lADP...）\n  hint: 先通过媒体上传命令（dt_media_upload）上传图片，使用返回的 mediaId", iconMediaID)
			}
			return callMCPToolOnServer("im", "update_group_icon", map[string]any{
				"openConversationId": mustGetFlag(cmd, "group"),
				"iconMediaId":        iconMediaID,
			})
		},
	}
	chatGroupUpdateIconCmd.Flags().String("group", "", "群聊 openConversationId (必填)")
	_ = chatGroupUpdateIconCmd.MarkFlagRequired("group")
	chatGroupUpdateIconCmd.Flags().String("icon-media-id", "", "群头像 mediaId (必填)")
	_ = chatGroupUpdateIconCmd.MarkFlagRequired("icon-media-id")

	chatGroupUpdateSettingsCmd := &cobra.Command{
		Use:   "update-settings",
		Short: "更新群设置",
		Long: `更新指定群聊的设置项。--setting-key 指定设置项，--status 指定值（0=关闭，1=开启）。

支持的 settingKey:
  authority               仅群主和管理员可管理
  joinValidation          入群许可
  onlyAdminCanAtAll       仅群主和管理员可@所有人
  searchable              群可被搜索
  addFriendForbidden      禁止群内私聊
  toolbarStatus           群快捷栏状态
  pluginCustomizeVerify   仅群主和管理员可管理快捷栏
  onlyAdminCanDING        谁可以在群内发DING
  allMembersCanCreateMcsConf  谁可以在群里发起视频和语音会议
  onlyAdminCanSetMsgTop   谁可以把群消息置顶
  onlyAdminCanPinMsg      谁可以把群消息钉住
  onlyAdminCanSendFile    谁可以上传文件、文件夹和钉盘文件
  allMembersCanCreateCalendar  群成员日历可见性
  groupEmailDisabled      群邮件组
  groupRedEnvelopeSwitch  发红包
  groupLiveAuthority      谁可以发起直播
  groupBillAuthority      群收款开关`,
		Example: `  dws chat group update-settings --group <openConversationId> --setting-key searchable --status 1
  dws chat group update-settings --group <openConversationId> --setting-key onlyAdminCanAtAll --status 0
  # 查询群 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "group", "setting-key"); err != nil {
				return err
			}
			if !cmd.Flags().Changed("status") {
				return fmt.Errorf("flag --status is required (0=关闭, 1=开启)")
			}
			status, _ := cmd.Flags().GetInt("status")
			return callMCPToolOnServer("im", "update_group_settings", map[string]any{
				"openConversationId": mustGetFlag(cmd, "group"),
				"settingKey":         mustGetFlag(cmd, "setting-key"),
				"status":             status,
			})
		},
	}
	chatGroupUpdateSettingsCmd.Flags().String("group", "", "群聊 openConversationId (必填)")
	_ = chatGroupUpdateSettingsCmd.MarkFlagRequired("group")
	chatGroupUpdateSettingsCmd.Flags().String("setting-key", "", "群设置项 key (必填)")
	_ = chatGroupUpdateSettingsCmd.MarkFlagRequired("setting-key")
	chatGroupUpdateSettingsCmd.Flags().Int("status", 0, "设置值: 0=关闭, 1=开启 (必填)")

	chatGroupCmd.AddCommand(chatGroupTransferOwnerCmd, chatGroupInviteUrlCmd, chatGroupQuitCmd, chatGroupUpdateIconCmd, chatGroupUpdateSettingsCmd)

	// ── message reply: 引用回复消息 ──────────────────────────

	chatMessageReplyCmd := &cobra.Command{
		Use:   "reply",
		Short: "引用回复消息（支持单聊/群聊）",
		Long: `以当前用户身份引用某条消息并回复。需要指定会话 ID、被引用消息 ID、原消息发送者 openDingTalkId，以及回复内容。

如何获取 openConversationId（如果上层已有则直接使用，不必再查）：
  - 群聊：dws chat search --query "群名"
  - 单聊：dws chat conversation-info --open-dingtalk-id <openDingTalkId>
          （人员信息可通过 dws contact user search --keyword "姓名" --format json 获取）`,
		Example: `  dws chat message reply --conversation-id <openConversationId> --ref-msg-id <openMessageId> --ref-sender <openDingTalkId> --text "收到，马上处理"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "conversation-id", "ref-msg-id", "ref-sender", "text"); err != nil {
				return err
			}
			refSender := mustGetFlag(cmd, "ref-sender")
			if !isOpenDingTalkID(refSender) {
				resolved, err := resolveOpenDingTalkID(cmd.Context(), refSender)
				if err != nil {
					return err
				}
				refSender = resolved
			}
			replyContent := map[string]string{
				"referenceOpenMessageId":   mustGetFlag(cmd, "ref-msg-id"),
				"srcMsgSendOpenDingTalkId": refSender,
				"replyMsgType":             "text",
				"content":                  mustGetFlag(cmd, "text"),
			}
			contentJSON, err := marshalJSONRaw(replyContent)
			if err != nil {
				return fmt.Errorf("failed to marshal reply content: %w", err)
			}
			clawType := ""
			aiTag, _ := cmd.Flags().GetBool("ai-tag")
			if aiTag {
				clawType = edition.ClawType()
			}
			toolArgs := map[string]any{
				"openConversationId": mustGetFlag(cmd, "conversation-id"),
				"msgType":            "reply",
				"content":            string(contentJSON),
				"clawType":           clawType,
			}
			if v, _ := cmd.Flags().GetString("uuid"); v != "" {
				toolArgs["uuid"] = v
			}
			return callMCPTool("send_personal_message", toolArgs)
		},
	}
	chatMessageReplyCmd.Flags().String("conversation-id", "", "会话 openConversationId (必填，支持单聊/群聊)")
	_ = chatMessageReplyCmd.MarkFlagRequired("conversation-id")
	chatMessageReplyCmd.Flags().String("ref-msg-id", "", "被引用的消息 openMessageId (必填)")
	_ = chatMessageReplyCmd.MarkFlagRequired("ref-msg-id")
	chatMessageReplyCmd.Flags().String("ref-sender", "", "被引用消息的发送者 openDingTalkId (必填)")
	_ = chatMessageReplyCmd.MarkFlagRequired("ref-sender")
	chatMessageReplyCmd.Flags().String("text", "", "回复内容 (必填)")
	_ = chatMessageReplyCmd.MarkFlagRequired("text")
	chatMessageReplyCmd.Flags().String("uuid", "", "幂等键（可选）")
	chatMessageReplyCmd.Flags().Bool("ai-tag", true, "消息是否带 AI 发送角标（默认 true）")

	// ── message forward: 转发单条消息 ────────────────────────

	chatMessageForwardCmd := &cobra.Command{
		Use:   "forward",
		Short: "转发单条消息（源/目标会话均支持单聊/群聊）",
		Long: `将一条消息从源会话转发到目标会话。需要指定源会话 ID、源消息 ID、目标会话 ID。

如何获取 openConversationId（如果上层已有则直接使用，不必再查）：
  - 群聊：dws chat search --query "群名"
  - 单聊：dws chat conversation-info --open-dingtalk-id <openDingTalkId>
          （openDingTalkId 可通过 dws contact user search --query "姓名" 获取）`,
		Example: `  dws chat message forward --src-conversation-id <srcOpenConversationId> --msg-id <srcOpenMessageId> --dest-conversation-id <destOpenConversationId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "src-conversation-id", "msg-id", "dest-conversation-id"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"srcOpenCid":       mustGetFlag(cmd, "src-conversation-id"),
				"srcOpenMessageId": mustGetFlag(cmd, "msg-id"),
				"destOpenCid":      mustGetFlag(cmd, "dest-conversation-id"),
			}
			if v, _ := cmd.Flags().GetString("uuid"); v != "" {
				toolArgs["uuid"] = v
			}
			return callMCPToolOnServer("im", "forward_message", toolArgs)
		},
	}
	chatMessageForwardCmd.Flags().String("src-conversation-id", "", "源会话 openConversationId (必填，支持单聊/群聊)")
	_ = chatMessageForwardCmd.MarkFlagRequired("src-conversation-id")
	chatMessageForwardCmd.Flags().String("msg-id", "", "源消息 openMessageId (必填)")
	_ = chatMessageForwardCmd.MarkFlagRequired("msg-id")
	chatMessageForwardCmd.Flags().String("dest-conversation-id", "", "目标会话 openConversationId (必填，支持单聊/群聊)")
	_ = chatMessageForwardCmd.MarkFlagRequired("dest-conversation-id")
	chatMessageForwardCmd.Flags().String("uuid", "", "幂等键（可选）")

	// ── set-top: 会话置顶 ──────────────────────────────────

	chatSetTopCmd := &cobra.Command{
		Use:   "set-top",
		Short: "会话置顶 / 取消置顶（支持单聊/群聊）",
		Long: `设置或取消会话置顶。默认设置置顶，传 --off 则取消置顶。

如何获取 openConversationId（如果上层已有则直接使用，不必再查）：
  - 群聊：dws chat search --query "群名"
  - 单聊：dws chat conversation-info --open-dingtalk-id <openDingTalkId>
          （openDingTalkId 可通过 dws contact user search --query "姓名" 获取）`,
		Example: `  dws chat set-top --conversation-id <openConversationId>
  dws chat set-top --conversation-id <openConversationId> --off`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "conversation-id"); err != nil {
				return err
			}
			off, _ := cmd.Flags().GetBool("off")
			return callMCPToolOnServer("im", "set_top_conversation", map[string]any{
				"openConversationId": mustGetFlag(cmd, "conversation-id"),
				"top":                !off,
			})
		},
	}
	chatSetTopCmd.Flags().String("conversation-id", "", "会话 openConversationId (必填，支持单聊/群聊)")
	_ = chatSetTopCmd.MarkFlagRequired("conversation-id")
	chatSetTopCmd.Flags().Bool("off", false, "取消置顶（不传则设置置顶）")

	// ── group-mute: 全员禁言 ───────────────────────────────

	chatGroupMuteCmd := &cobra.Command{
		Use:   "group-mute",
		Short: "全员禁言 / 取消全员禁言",
		Long:  `设置或取消群全员禁言。默认开启全员禁言，传 --off 则取消。`,
		Example: `  dws chat group-mute --group <openConversationId>
  dws chat group-mute --group <openConversationId> --off
  # 查询群 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			groupID := flagOrFallback(cmd, "group", "conversation-id", "id", "chat")
			if groupID == "" {
				return fmt.Errorf("flag --group is required\n  hint: dws chat group-mute --group <openConversationId>")
			}
			off, _ := cmd.Flags().GetBool("off")
			return callMCPToolOnServer("im", "set_group_mute", map[string]any{
				"openConversationId": groupID,
				"mute":               !off,
			})
		},
	}
	chatGroupMuteCmd.Flags().String("group", "", "群聊 openConversationId (必填)")
	chatGroupMuteCmd.Flags().String("conversation-id", "", "--group 的别名")
	_ = chatGroupMuteCmd.Flags().MarkHidden("conversation-id")
	chatGroupMuteCmd.Flags().String("id", "", "--group 的别名")
	_ = chatGroupMuteCmd.Flags().MarkHidden("id")
	chatGroupMuteCmd.Flags().String("chat", "", "--group 的别名")
	_ = chatGroupMuteCmd.Flags().MarkHidden("chat")
	chatGroupMuteCmd.Flags().Bool("off", false, "取消全员禁言（不传则开启禁言）")

	// ── group-mute-member: 指定群成员禁言 ──────────────────

	chatGroupMuteMemberCmd := &cobra.Command{
		Use:   "group-mute-member",
		Short: "指定群成员禁言 / 取消禁言",
		Long: `将指定群成员加入或移出禁言名单。
--mute-time 禁言时长（毫秒），仅支持 5min(300000) / 1h(3600000) / 1d(86400000) / 7d(604800000) / 30d(2592000000)。
默认加入禁言名单，传 --off 则移除。`,
		Example: `  dws chat group-mute-member --group <openConversationId> --users userId1,userId2 --mute-time 3600000
  dws chat group-mute-member --group <openConversationId> --user userId1 --mute-time 3600000
  dws chat group-mute-member --group <openConversationId> --user userId1,userId2 --off
  # 查询群 ID: dws chat search --query "群名"
  # 查询人员: dws contact user search --keyword "姓名" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			groupID := flagOrFallback(cmd, "group", "conversation-id", "id", "chat")
			if groupID == "" {
				return fmt.Errorf("flag --group is required\n  hint: dws chat group-mute-member --group <openConversationId> --user <userIds> --mute-time <ms>")
			}
			usersRaw := flagOrFallback(cmd, "users", "user", "userId")
			if usersRaw == "" {
				return fmt.Errorf("flag --users or --user is required")
			}
			userIDs, openDingTalkIDs := splitChatIDValues(parseCSVValues(usersRaw))
			// 服务端 set_group_member_mute_list 的 uids（staffId）入参存在缺陷：
			// 即使传了 uids 仍返回 "uids is required"，而 openDingTalkIds 路径正常。
			// 与 message send 一致：先把 userId 解析为 openDingTalkId；解析失败再降级透传 uids。
			if len(userIDs) > 0 {
				if resolved, err := resolveOpenDingTalkIDs(cmd.Context(), userIDs); err == nil {
					openDingTalkIDs = append(openDingTalkIDs, resolved...)
					userIDs = nil
				}
			}
			off, _ := cmd.Flags().GetBool("off")
			toolArgs := map[string]any{
				"openConversationId": groupID,
				"mute":               !off,
			}
			if len(userIDs) > 0 {
				toolArgs["uids"] = userIDs
			}
			if len(openDingTalkIDs) > 0 {
				toolArgs["openDingTalkIds"] = openDingTalkIDs
			}
			if !off {
				muteTime, _ := cmd.Flags().GetInt64("mute-time")
				if muteTime <= 0 {
					return fmt.Errorf("--mute-time is required when muting (supported: 300000/3600000/86400000/604800000/2592000000)")
				}
				toolArgs["muteTime"] = muteTime
			}
			return callMCPToolOnServer("im", "set_group_member_mute_list", toolArgs)
		},
	}
	chatGroupMuteMemberCmd.Flags().String("group", "", "群聊 openConversationId (必填)")
	chatGroupMuteMemberCmd.Flags().String("conversation-id", "", "--group 的别名")
	_ = chatGroupMuteMemberCmd.Flags().MarkHidden("conversation-id")
	chatGroupMuteMemberCmd.Flags().String("id", "", "--group 的别名")
	_ = chatGroupMuteMemberCmd.Flags().MarkHidden("id")
	chatGroupMuteMemberCmd.Flags().String("chat", "", "--group 的别名")
	_ = chatGroupMuteMemberCmd.Flags().MarkHidden("chat")
	chatGroupMuteMemberCmd.Flags().String("users", "", "群成员 userId 列表，逗号分隔（批量）")
	chatGroupMuteMemberCmd.Flags().String("user", "", "群成员 userId，支持逗号分隔")
	chatGroupMuteMemberCmd.Flags().String("userId", "", "--user 的别名")
	_ = chatGroupMuteMemberCmd.Flags().MarkHidden("userId")
	chatGroupMuteMemberCmd.Flags().Int64("mute-time", 0, "禁言时长（毫秒），支持 300000/3600000/86400000/604800000/2592000000")
	chatGroupMuteMemberCmd.Flags().Bool("off", false, "移出禁言名单（不传则加入禁言名单）")

	// ── group set-admin: 设置群成员为管理员 ─────────────────

	chatGroupSetAdminCmd := &cobra.Command{
		Use:   "set-admin",
		Short: "设置 / 取消群管理员",
		Long:  `将指定群成员设置为管理员或取消管理员身份。默认设为管理员，传 --off 则取消。`,
		Example: `  dws chat group set-admin --group <openConversationId> --users userId1,userId2
  dws chat group set-admin --group <openConversationId> --user userId1
  dws chat group set-admin --group <openConversationId> --user userId1 --off
  # 查询群 ID: dws chat search --query "群名"
  # 查询人员: dws contact user search --keyword "姓名" --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "group"); err != nil {
				return err
			}
			usersRaw := flagOrFallback(cmd, "users", "user", "userId")
			if usersRaw == "" {
				return fmt.Errorf("flag --users or --user is required")
			}
			userIDs, openDingTalkIDs := splitChatIDValues(parseCSVValues(usersRaw))
			off, _ := cmd.Flags().GetBool("off")
			toolArgs := map[string]any{
				"openConversationId": mustGetFlag(cmd, "group"),
				"admin":              !off,
			}
			if len(userIDs) > 0 {
				toolArgs["uids"] = userIDs
			}
			if len(openDingTalkIDs) > 0 {
				toolArgs["openDingTalkIds"] = openDingTalkIDs
			}
			return callMCPToolOnServer("im", "update_conv_member_roles", toolArgs)
		},
	}
	chatGroupSetAdminCmd.Flags().String("group", "", "群聊 openConversationId (必填)")
	_ = chatGroupSetAdminCmd.MarkFlagRequired("group")
	chatGroupSetAdminCmd.Flags().String("users", "", "成员 userId 列表，逗号分隔（批量）")
	chatGroupSetAdminCmd.Flags().String("user", "", "成员 userId，支持逗号分隔")
	chatGroupSetAdminCmd.Flags().String("userId", "", "--user 的别名")
	_ = chatGroupSetAdminCmd.Flags().MarkHidden("userId")
	chatGroupSetAdminCmd.Flags().Bool("off", false, "取消管理员（不传则设为管理员）")

	chatGroupCmd.AddCommand(chatGroupSetAdminCmd)

	chatMessageCmd.AddCommand(chatMessageReplyCmd, chatMessageForwardCmd)

	// info-by-id flags
	chatGroupInfoByIdCmd.Flags().Int64("group-id", 0, "群号 (必填，数字类型)")
	_ = chatGroupInfoByIdCmd.MarkFlagRequired("group-id")

	// category conversations flags
	chatCategoryConvsCmd.Flags().Int("category-id", 0, "会话分组 ID (必填)")
	chatCategoryConvsCmd.Flags().Bool("exclude-muted", false, "是否排除已设置免打扰的会话（默认 false）")
	_ = chatCategoryConvsCmd.MarkFlagRequired("category-id")

	// category create flags
	chatCategoryCreateCmd.Flags().String("title", "", "分组名称 (必填)")
	_ = chatCategoryCreateCmd.MarkFlagRequired("title")

	// category delete flags
	chatCategoryDeleteCmd.Flags().Int64("category-id", 0, "会话分组 ID (必填)")

	// category rename flags
	chatCategoryRenameCmd.Flags().Int64("category-id", 0, "会话分组 ID (必填)")
	chatCategoryRenameCmd.Flags().String("title", "", "新的分组名称 (必填)")
	_ = chatCategoryRenameCmd.MarkFlagRequired("title")

	// category add-conv flags
	chatCategoryAddConvCmd.Flags().String("group", "", "会话 openConversationId (必填)")
	chatCategoryAddConvCmd.Flags().String("conversation-id", "", "--group 的别名")
	_ = chatCategoryAddConvCmd.Flags().MarkHidden("conversation-id")
	chatCategoryAddConvCmd.Flags().String("id", "", "--group 的别名")
	_ = chatCategoryAddConvCmd.Flags().MarkHidden("id")
	chatCategoryAddConvCmd.Flags().String("category-ids", "", "目标分组 ID 列表，逗号分隔 (必填)")
	_ = chatCategoryAddConvCmd.MarkFlagRequired("category-ids")

	// category remove-conv flags
	chatCategoryRemoveConvCmd.Flags().String("group", "", "会话 openConversationId (必填)")
	chatCategoryRemoveConvCmd.Flags().String("conversation-id", "", "--group 的别名")
	_ = chatCategoryRemoveConvCmd.Flags().MarkHidden("conversation-id")
	chatCategoryRemoveConvCmd.Flags().String("id", "", "--group 的别名")
	_ = chatCategoryRemoveConvCmd.Flags().MarkHidden("id")
	chatCategoryRemoveConvCmd.Flags().String("category-ids", "", "目标分组 ID 列表，逗号分隔 (必填)")
	_ = chatCategoryRemoveConvCmd.MarkFlagRequired("category-ids")

	// ── group-role 子命令（群身份管理）────────────────────────

	chatGroupRoleCmd := &cobra.Command{Use: "group-role", Short: "群身份管理", RunE: groupRunE}

	chatGroupRoleListCmd := &cobra.Command{
		Use:   "list",
		Short: "拉取会话的群身份列表",
		Example: `  dws chat group-role list --group <openConversationId>
  # 查询群 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			groupID := flagOrFallback(cmd, "group", "conversation-id", "id")
			if groupID == "" {
				return fmt.Errorf("flag --group is required\n  hint: dws chat group-role list --group <openConversationId>")
			}
			return callMCPToolOnServer("im", "list_custom_group_roles", map[string]any{
				"openConversationId": groupID,
			})
		},
	}
	chatGroupRoleListCmd.Flags().String("group", "", "群聊 openConversationId (必填)")
	_ = chatGroupRoleListCmd.MarkFlagRequired("group")

	chatGroupRoleAddCmd := &cobra.Command{
		Use:     "add",
		Short:   "添加群身份",
		Example: `  dws chat group-role add --group <openConversationId> --name "管理员"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "group", "name"); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "add_custom_group_role", map[string]any{
				"openConversationId": mustGetFlag(cmd, "group"),
				"name":               mustGetFlag(cmd, "name"),
			})
		},
	}
	chatGroupRoleAddCmd.Flags().String("group", "", "群聊 openConversationId (必填)")
	_ = chatGroupRoleAddCmd.MarkFlagRequired("group")
	chatGroupRoleAddCmd.Flags().String("name", "", "群身份名称 (必填)")
	_ = chatGroupRoleAddCmd.MarkFlagRequired("name")

	chatGroupRoleUpdateCmd := &cobra.Command{
		Use:     "update",
		Short:   "更新群身份名称",
		Example: `  dws chat group-role update --group <openConversationId> --role-id <openRoleId> --name "新名称"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "group", "role-id", "name"); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "update_custom_group_role", map[string]any{
				"openConversationId": mustGetFlag(cmd, "group"),
				"openRoleId":         mustGetFlag(cmd, "role-id"),
				"name":               mustGetFlag(cmd, "name"),
			})
		},
	}
	chatGroupRoleUpdateCmd.Flags().String("group", "", "群聊 openConversationId (必填)")
	_ = chatGroupRoleUpdateCmd.MarkFlagRequired("group")
	chatGroupRoleUpdateCmd.Flags().String("role-id", "", "群身份 openRoleId，由 group-role list 返回 (必填)")
	_ = chatGroupRoleUpdateCmd.MarkFlagRequired("role-id")
	chatGroupRoleUpdateCmd.Flags().String("name", "", "群身份新名称 (必填)")
	_ = chatGroupRoleUpdateCmd.MarkFlagRequired("name")

	chatGroupRoleRemoveCmd := &cobra.Command{
		Use:     "remove",
		Short:   "删除群身份",
		Example: `  dws chat group-role remove --group <openConversationId> --role-id <openRoleId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "group", "role-id"); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "remove_custom_group_role", map[string]any{
				"openConversationId": mustGetFlag(cmd, "group"),
				"openRoleId":         mustGetFlag(cmd, "role-id"),
			})
		},
	}
	chatGroupRoleRemoveCmd.Flags().String("group", "", "群聊 openConversationId (必填)")
	_ = chatGroupRoleRemoveCmd.MarkFlagRequired("group")
	chatGroupRoleRemoveCmd.Flags().String("role-id", "", "群身份 openRoleId，由 group-role list 返回 (必填)")
	_ = chatGroupRoleRemoveCmd.MarkFlagRequired("role-id")

	chatGroupRoleSetUserCmd := &cobra.Command{
		Use:   "set-user",
		Short: "设置用户的群身份（覆盖该用户的全部群身份）",
		Example: `  dws chat group-role set-user --group <openConversationId> --user <userId> --role-ids roleId1,roleId2
  # 查询人员: dws contact user search --keyword "姓名" --format json
  # 查询 role-id: dws chat group-role list --group <openConversationId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "group", "role-ids"); err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "user", "userId"); err != nil {
				return err
			}
			roleIDs := parseCSVValues(mustGetFlag(cmd, "role-ids"))
			user := flagOrFallback(cmd, "user", "userId")
			toolArgs := map[string]any{
				"openConversationId": mustGetFlag(cmd, "group"),
				"openRoleIds":        roleIDs,
			}
			if isOpenDingTalkID(user) {
				toolArgs["openDingTalkId"] = user
			} else {
				toolArgs["userId"] = user
			}
			return callMCPToolOnServer("im", "set_custom_user_roles", toolArgs)
		},
	}
	chatGroupRoleSetUserCmd.Flags().String("group", "", "群聊 openConversationId (必填)")
	_ = chatGroupRoleSetUserCmd.MarkFlagRequired("group")
	chatGroupRoleSetUserCmd.Flags().String("user", "", "用户 userId（必填）")
	chatGroupRoleSetUserCmd.Flags().String("userId", "", "--user 的别名")
	_ = chatGroupRoleSetUserCmd.Flags().MarkHidden("userId")
	chatGroupRoleSetUserCmd.Flags().String("role-ids", "", "群身份 openRoleId 列表，逗号分隔 (必填)，传空字符串则清除该用户所有群身份")
	_ = chatGroupRoleSetUserCmd.MarkFlagRequired("role-ids")

	chatGroupRoleRemoveUserCmd := &cobra.Command{
		Use:     "remove-user",
		Short:   "移除用户的指定群身份",
		Example: `  dws chat group-role remove-user --group <openConversationId> --user <userId> --role-ids roleId1,roleId2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "group", "role-ids"); err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "user", "userId"); err != nil {
				return err
			}
			roleIDs := parseCSVValues(mustGetFlag(cmd, "role-ids"))
			user := flagOrFallback(cmd, "user", "userId")
			toolArgs := map[string]any{
				"openConversationId": mustGetFlag(cmd, "group"),
				"openRoleIds":        roleIDs,
			}
			if isOpenDingTalkID(user) {
				toolArgs["openDingTalkId"] = user
			} else {
				toolArgs["userId"] = user
			}
			return callMCPToolOnServer("im", "remove_custom_user_roles", toolArgs)
		},
	}
	chatGroupRoleRemoveUserCmd.Flags().String("group", "", "群聊 openConversationId (必填)")
	_ = chatGroupRoleRemoveUserCmd.MarkFlagRequired("group")
	chatGroupRoleRemoveUserCmd.Flags().String("user", "", "用户 userId（必填）")
	chatGroupRoleRemoveUserCmd.Flags().String("userId", "", "--user 的别名")
	_ = chatGroupRoleRemoveUserCmd.Flags().MarkHidden("userId")
	chatGroupRoleRemoveUserCmd.Flags().String("role-ids", "", "要移除的群身份 openRoleId 列表，逗号分隔 (必填)")
	_ = chatGroupRoleRemoveUserCmd.MarkFlagRequired("role-ids")

	chatGroupRoleQueryUserCmd := &cobra.Command{
		Use:     "query-user",
		Short:   "查询群成员的群身份",
		Example: `  dws chat group-role query-user --group <openConversationId> --user <userId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "group"); err != nil {
				return err
			}
			if err := validateRequiredFlagWithAliases(cmd, "user", "userId"); err != nil {
				return err
			}
			user := flagOrFallback(cmd, "user", "userId")
			toolArgs := map[string]any{
				"openConversationId": mustGetFlag(cmd, "group"),
			}
			if isOpenDingTalkID(user) {
				toolArgs["openDingTalkId"] = user
			} else {
				toolArgs["userId"] = user
			}
			return callMCPToolOnServer("im", "query_custom_user_roles", toolArgs)
		},
	}
	chatGroupRoleQueryUserCmd.Flags().String("group", "", "群聊 openConversationId (必填)")
	_ = chatGroupRoleQueryUserCmd.MarkFlagRequired("group")
	chatGroupRoleQueryUserCmd.Flags().String("user", "", "用户 userId（必填）")
	chatGroupRoleQueryUserCmd.Flags().String("userId", "", "--user 的别名")
	_ = chatGroupRoleQueryUserCmd.Flags().MarkHidden("userId")

	chatGroupRoleCmd.AddCommand(
		chatGroupRoleListCmd,
		chatGroupRoleAddCmd,
		chatGroupRoleUpdateCmd,
		chatGroupRoleRemoveCmd,
		chatGroupRoleSetUserCmd,
		chatGroupRoleRemoveUserCmd,
		chatGroupRoleQueryUserCmd,
	)

	// ── 群机器人 / 群解散 / 历史消息 / 合并转发（5.18 + 5.14 表）──

	chatGroupBotsCmd := &cobra.Command{
		Use:   "bots",
		Short: "查看群内所有机器人",
		Long:  `获取指定群聊中的所有机器人列表。`,
		Example: `  dws chat group bots --group <openConversationId>
  # 查询群 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "group"); err != nil {
				return err
			}
			return callMCPToolOnServer("bot", "list_group_bots", map[string]any{
				"openConversationId": mustGetFlag(cmd, "group"),
			})
		},
	}
	chatGroupBotsCmd.Flags().String("group", "", "群聊 openConversationId (必填)")
	_ = chatGroupBotsCmd.MarkFlagRequired("group")

	chatGroupMembersRemoveBotCmd := &cobra.Command{
		Use:   "remove-bot",
		Short: "从群内移除机器人",
		Long:  `将指定机器人从群聊中移除。需要群管理员或群主权限。`,
		Example: `  dws chat group members remove-bot --id <openConversationId> --bot-id <openBotId>
  # 查询群 ID: dws chat search --query "群名"
  # 查询群内机器人: dws chat group bots --group <openConversationId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id", "bot-id"); err != nil {
				return err
			}
			return callMCPToolOnServer("bot", "remove_robot_in_group", map[string]any{
				"openConversationId": mustGetFlag(cmd, "id"),
				"openBotId":          mustGetFlag(cmd, "bot-id"),
			})
		},
	}
	chatGroupMembersRemoveBotCmd.Flags().String("id", "", "群聊 openConversationId (必填)")
	_ = chatGroupMembersRemoveBotCmd.MarkFlagRequired("id")
	chatGroupMembersRemoveBotCmd.Flags().String("bot-id", "", "机器人 openBotId (必填)")
	_ = chatGroupMembersRemoveBotCmd.MarkFlagRequired("bot-id")

	chatBotFindCmd := &cobra.Command{
		Use:   "find",
		Short: "搜索【全部可用】机器人（含他人/官方，额外返回 openDingTalkId 可发单聊）",
		Long: `按关键词搜索当前用户可用的【全部】机器人（含他人创建、官方机器人），支持游标分页。

如何在 find 与 search 之间选择：
  - find ：用户说"搜索机器人""找一个机器人""所有可用机器人""帮我找 XXX 机器人"
           （不限范围，包含他人/官方）→ 用 find
  - search：用户说"我创建的""我的""我自己的""我做的"机器人 → 用 search（dws chat bot search）

返回字段差异（核心区分点）：
  - find  额外返回 openDingTalkId（可用于给机器人发单聊消息）
  - search 没有 openDingTalkId

如果后续需要给机器人发单聊消息，必须用 find 拿 openDingTalkId。`,
		Example: `  dws chat bot find --query "日报"
  dws chat bot find --query "日报" --limit 20
  # 拿到 openDingTalkId 后可用于给机器人发单聊消息`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlagWithAliases(cmd, "query", "keyword"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"keyword": flagOrFallback(cmd, "query", "keyword"),
			}
			if v, err := cmd.Flags().GetInt("limit"); err == nil && v > 0 {
				toolArgs["limit"] = v
			}
			if v, _ := cmd.Flags().GetString("cursor"); v != "" {
				toolArgs["cursor"] = v
			}
			return callMCPToolOnServer("bot", "search_bots", toolArgs)
		},
	}
	chatBotFindCmd.Flags().String("query", "", "搜索关键词 (必填)")
	chatBotFindCmd.Flags().String("keyword", "", "--query 的别名")
	_ = chatBotFindCmd.Flags().MarkHidden("keyword")
	chatBotFindCmd.Flags().Int("limit", 20, "每页返回数量（默认 20）")
	chatBotFindCmd.Flags().String("cursor", "", "分页游标（首次调用不传，翻页时传上次返回的 nextCursor）")

	chatGroupDismissCmd := &cobra.Command{
		Use:   "dismiss",
		Short: "解散群聊",
		Long:  `解散指定群聊。该操作不可逆，需要群主权限。`,
		Example: `  dws chat group dismiss --group <openConversationId>
  # 查询群 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "group"); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "dismiss_group", map[string]any{
				"openConversationId": mustGetFlag(cmd, "group"),
			})
		},
	}
	chatGroupDismissCmd.Flags().String("group", "", "群聊 openConversationId (必填)")
	_ = chatGroupDismissCmd.MarkFlagRequired("group")

	chatGroupSetHistoryCmd := &cobra.Command{
		Use:   "set-history",
		Short: "设置新成员入群可查看历史消息选项",
		Long: `设置新成员入群后可查看的历史消息范围。--option 取值:
  FORBIDDEN    禁止查看历史消息
  RECENT_100   可查看最近 100 条消息
  ALL          可查看全部历史消息`,
		Example: `  dws chat group set-history --group <openConversationId> --option RECENT_100
  dws chat group set-history --group <openConversationId> --option FORBIDDEN
  # 查询群 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "group", "option"); err != nil {
				return err
			}
			option := mustGetFlag(cmd, "option")
			switch option {
			case "FORBIDDEN", "RECENT_100", "ALL":
			default:
				return fmt.Errorf("--option must be one of FORBIDDEN | RECENT_100 | ALL, got %q", option)
			}
			return callMCPToolOnServer("im", "update_show_history_msg_option", map[string]any{
				"openConversationId": mustGetFlag(cmd, "group"),
				"option":             option,
			})
		},
	}
	chatGroupSetHistoryCmd.Flags().String("group", "", "群聊 openConversationId (必填)")
	_ = chatGroupSetHistoryCmd.MarkFlagRequired("group")
	chatGroupSetHistoryCmd.Flags().String("option", "", "可见范围: FORBIDDEN | RECENT_100 | ALL (必填)")
	_ = chatGroupSetHistoryCmd.MarkFlagRequired("option")

	chatMessageCombineForwardCmd := &cobra.Command{
		Use:   "combine-forward",
		Short: "合并转发多条消息",
		Long: `将多条消息合并后转发到目标会话。需要指定源会话 ID、源消息 ID 列表（逗号分隔）、目标会话 ID。

如何获取 openConversationId（如果上层已有则直接使用，不必再查）：
  - 群聊：dws chat search --query "群名"
  - 单聊：dws chat conversation-info --open-dingtalk-id <openDingTalkId>
          （openDingTalkId 可通过 dws contact user search --query "姓名" 获取）`,
		Example: `  dws chat message combine-forward --src-conversation-id <srcOpenCid> --msg-ids <id1>,<id2>,<id3> --dest-conversation-id <destOpenCid>
  dws chat message combine-forward --src-conversation-id <srcOpenCid> --msg-ids <id1>,<id2> --dest-conversation-id <destOpenCid> --uuid <idempotencyKey>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "src-conversation-id", "msg-ids", "dest-conversation-id"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"srcOpenCid":        mustGetFlag(cmd, "src-conversation-id"),
				"srcOpenMessageIds": parseCSVValues(mustGetFlag(cmd, "msg-ids")),
				"destOpenCid":       mustGetFlag(cmd, "dest-conversation-id"),
			}
			if v, _ := cmd.Flags().GetString("uuid"); v != "" {
				toolArgs["uuid"] = v
			}
			return callMCPToolOnServer("im", "combine_forward_messages", toolArgs)
		},
	}
	chatMessageCombineForwardCmd.Flags().String("src-conversation-id", "", "源会话 openConversationId (必填，支持单聊/群聊)")
	_ = chatMessageCombineForwardCmd.MarkFlagRequired("src-conversation-id")
	chatMessageCombineForwardCmd.Flags().String("msg-ids", "", "源消息 openMessageId 列表，逗号分隔 (必填)")
	_ = chatMessageCombineForwardCmd.MarkFlagRequired("msg-ids")
	chatMessageCombineForwardCmd.Flags().String("dest-conversation-id", "", "目标会话 openConversationId (必填，支持单聊/群聊)")
	_ = chatMessageCombineForwardCmd.MarkFlagRequired("dest-conversation-id")
	chatMessageCombineForwardCmd.Flags().String("uuid", "", "幂等键（可选）")

	// ── message forward-topic: 转发话题消息 ─────────────────────

	chatMessageForwardTopicCmd := &cobra.Command{
		Use:   "forward-topic",
		Short: "转发话题消息到目标会话",
		Long: `将一条话题消息从源会话转发到目标会话。需要指定源消息 ID、源会话 ID、话题 ID、目标会话 ID。

如何获取 openConversationId（如果上层已有则直接使用，不必再查）：
  - 群聊：dws chat search --query "群名"
  - 单聊：dws chat conversation-info --open-dingtalk-id <openDingTalkId>
          （openDingTalkId 可通过 dws contact user search --query "姓名" 获取）

话题 ID（srcOpenConvThreadId）格式为 "convThread" + 加密后的 convThreadId，
可通过 dws chat message list 返回的话题信息获取。`,
		Example: `  dws chat message forward-topic --src-msg-id <srcOpenMessageId> --src-conversation-id <srcOpenConversationId> --src-thread-id <srcOpenConvThreadId> --dest-conversation-id <destOpenConversationId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "src-msg-id", "src-conversation-id", "src-thread-id", "dest-conversation-id"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"srcOpenMessageId":       mustGetFlag(cmd, "src-msg-id"),
				"srcOpenConversationId":  mustGetFlag(cmd, "src-conversation-id"),
				"srcOpenConvThreadId":    mustGetFlag(cmd, "src-thread-id"),
				"destOpenConversationId": mustGetFlag(cmd, "dest-conversation-id"),
			}
			return callMCPToolOnServer("im", "forward_topic", toolArgs)
		},
	}
	chatMessageForwardTopicCmd.Flags().String("src-msg-id", "", "源消息 openMessageId (必填，要转发的消息)")
	_ = chatMessageForwardTopicCmd.MarkFlagRequired("src-msg-id")
	chatMessageForwardTopicCmd.Flags().String("src-conversation-id", "", "源会话 openConversationId (必填，消息所在的会话)")
	_ = chatMessageForwardTopicCmd.MarkFlagRequired("src-conversation-id")
	chatMessageForwardTopicCmd.Flags().String("src-thread-id", "", "话题 ID (必填，格式: convThread + 加密后的convThreadId)")
	_ = chatMessageForwardTopicCmd.MarkFlagRequired("src-thread-id")
	chatMessageForwardTopicCmd.Flags().String("dest-conversation-id", "", "目标会话 openConversationId (必填，转发到的会话)")
	_ = chatMessageForwardTopicCmd.MarkFlagRequired("dest-conversation-id")

	// ── message pin: 钉住/取消钉住/拉取钉住消息 ──────────────

	chatMessageSetPinCmd := &cobra.Command{
		Use:   "set-pin-msg",
		Short: "钉住消息（Pin）",
		Long: `将指定消息设置为钉住状态。需要指定会话 ID 和消息 ID。

如何获取 openConversationId（如果上层已有则直接使用，不必再查）：
  - 群聊：dws chat search --query "群名"
  - 单聊：dws chat conversation-info --open-dingtalk-id <openDingTalkId>`,
		Example: `  dws chat message set-pin-msg --open-conversation-id <openConversationId> --msg-id <openMessageId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "open-conversation-id", "msg-id"); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "set_pin_message", map[string]any{
				"openConversationId": mustGetFlag(cmd, "open-conversation-id"),
				"openMessageId":      mustGetFlag(cmd, "msg-id"),
			})
		},
	}
	chatMessageSetPinCmd.Flags().String("open-conversation-id", "", "会话 openConversationId (必填，支持群聊/单聊)")
	_ = chatMessageSetPinCmd.MarkFlagRequired("open-conversation-id")
	chatMessageSetPinCmd.Flags().String("msg-id", "", "消息 openMessageId (必填)")
	_ = chatMessageSetPinCmd.MarkFlagRequired("msg-id")

	chatMessageUnsetPinCmd := &cobra.Command{
		Use:   "unset-pin-msg",
		Short: "取消钉住消息（Unpin）",
		Long: `取消指定消息的钉住状态。需要指定会话 ID 和消息 ID。

如何获取 openConversationId（如果上层已有则直接使用，不必再查）：
  - 群聊：dws chat search --query "群名"
  - 单聊：dws chat conversation-info --open-dingtalk-id <openDingTalkId>`,
		Example: `  dws chat message unset-pin-msg --open-conversation-id <openConversationId> --msg-id <openMessageId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "open-conversation-id", "msg-id"); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "unset_pin_message", map[string]any{
				"openConversationId": mustGetFlag(cmd, "open-conversation-id"),
				"openMessageId":      mustGetFlag(cmd, "msg-id"),
			})
		},
	}
	chatMessageUnsetPinCmd.Flags().String("open-conversation-id", "", "会话 openConversationId (必填，支持群聊/单聊)")
	_ = chatMessageUnsetPinCmd.MarkFlagRequired("open-conversation-id")
	chatMessageUnsetPinCmd.Flags().String("msg-id", "", "消息 openMessageId (必填)")
	_ = chatMessageUnsetPinCmd.MarkFlagRequired("msg-id")

	chatMessageListPinCmd := &cobra.Command{
		Use:   "list-pin-msg",
		Short: "拉取会话中钉住的消息列表",
		Long: `拉取指定会话中被钉住的消息列表，支持游标分页。

如何获取 openConversationId（如果上层已有则直接使用，不必再查）：
  - 群聊：dws chat search --query "群名"
  - 单聊：dws chat conversation-info --open-dingtalk-id <openDingTalkId>`,
		Example: `  dws chat message list-pin-msg --open-conversation-id <openConversationId>
  dws chat message list-pin-msg --open-conversation-id <openConversationId> --size 50
  dws chat message list-pin-msg --open-conversation-id <openConversationId> --cursor <nextCursor> --size 20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "open-conversation-id"); err != nil {
				return err
			}
			toolArgs := map[string]any{
				"openConversationId": mustGetFlag(cmd, "open-conversation-id"),
			}
			if v, _ := cmd.Flags().GetString("cursor"); v != "" {
				toolArgs["cursor"] = v
			}
			if v, _ := cmd.Flags().GetInt("size"); v > 0 {
				toolArgs["count"] = v
			}
			return callMCPToolOnServer("im", "list_pin_messages", toolArgs)
		},
	}
	chatMessageListPinCmd.Flags().String("open-conversation-id", "", "会话 openConversationId (必填，支持群聊/单聊)")
	_ = chatMessageListPinCmd.MarkFlagRequired("open-conversation-id")
	chatMessageListPinCmd.Flags().String("cursor", "", "分页游标（首次不传，翻页时传上次返回的 nextCursor）")
	chatMessageListPinCmd.Flags().Int("size", 0, "一次拉取的消息数量（默认 20，最大 100）")

	// ── group list-my-groups: 拉取我创建/管理的群 ──────────────

	chatGroupListMyGroupsCmd := &cobra.Command{
		Use:   "list-my-groups",
		Short: "拉取我创建/管理的群",
		Long: `拉取当前用户作为群主或管理员的群列表。
可通过 --role 过滤角色：OWNER 仅群主、ADMIN 仅管理员，不传则返回全部。
可通过 --limit 限制返回数量，不传则返回所有符合条件的群。`,
		Example: `  dws chat group list-my-groups
  dws chat group list-my-groups --role OWNER
  dws chat group list-my-groups --role ADMIN --limit 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v, _ := cmd.Flags().GetString("role"); v != "" {
				toolArgs["roleFilter"] = v
			}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["limit"] = v
			}
			if v, _ := cmd.Flags().GetBool("exclude-muted"); v {
				toolArgs["excludeMuted"] = true
			}
			return callMCPToolOnServer("im", "list_owned_or_admin_groups", toolArgs)
		},
	}
	chatGroupListMyGroupsCmd.Flags().String("role", "", "角色过滤: OWNER(仅群主) / ADMIN(仅管理员)，不传返回全部")
	chatGroupListMyGroupsCmd.Flags().Int("limit", 0, "最多返回群数量，不传返回全部")
	chatGroupListMyGroupsCmd.Flags().Bool("exclude-muted", false, "是否排除已设置免打扰的群聊（默认 false）")

	// ── group list-all: 分页拉取我所有群列表 ───────────────────────

	chatGroupListAllCmd := &cobra.Command{
		Use:   "list-all",
		Short: "分页拉取我所有群列表",
		Long: `分页获取当前用户加入的所有群聊列表。
支持分页，每次最多返回 200 个群。`,
		Example: `  dws chat group list-all
  dws chat group list-all --limit 50
  dws chat group list-all --limit 100 --cursor <nextCursor>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["limit"] = v
			}
			if v, _ := cmd.Flags().GetString("cursor"); v != "" && v != "0" {
				toolArgs["cursor"] = v
			}
			return callMCPToolOnServer("im", "list_my_groups_pagination", toolArgs)
		},
	}
	chatGroupListAllCmd.Flags().Int("limit", 100, "每页返回数量（默认 100，最大 200）")
	chatGroupListAllCmd.Flags().String("cursor", "", "分页游标（首次不传，翻页传返回的 nextCursor）")

	// ── group list-join-validations: 分页拉取入群验证记录 ─────────

	chatGroupListJoinValidationsCmd := &cobra.Command{
		Use:   "list-join-validations",
		Short: "分页拉取入群验证记录",
		Long: `分页拉取当前用户的所有入群验证记录，包括自己被拒绝的记录以及作为审批者的记录。
支持分页，每页最多返回 50 条。`,
		Example: `  dws chat group list-join-validations
  dws chat group list-join-validations --limit 30
  dws chat group list-join-validations --limit 20 --cursor <nextCursor>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["limit"] = v
			}
			if v, _ := cmd.Flags().GetString("cursor"); v != "" {
				toolArgs["cursor"] = v
			}
			return callMCPToolOnServer("im", "list_apply_join_group_records", toolArgs)
		},
	}
	chatGroupListJoinValidationsCmd.Flags().Int("limit", 20, "单页数量（默认 20，最大 50）")
	chatGroupListJoinValidationsCmd.Flags().String("cursor", "", "分页游标（首次不传，翻页传返回的 nextCursor）")

	// ── group audit-join-validation: 审批入群验证 ─────────────────────────

	chatGroupAuditJoinValidationCmd := &cobra.Command{
		Use:   "audit-join-validation",
		Short: "审批入群验证（通过、拒绝、删除）",
		Long: `审批入群验证。支持通过、拒绝、删除、忽略、拒绝并拉黑等操作。

status 可选值:
  AuditApprove — 通过
  AuditDelete  — 删除
  AuditIgnore  — 忽略
  AuditRefuse  — 拒绝
  AuditBlock   — 拒绝且不再接受该用户的申请`,
		Example: `  dws chat group audit-join-validation --group <openConversationId> --record-id 123456 --applicant <openDingTalkId> --inviter <openDingTalkId> --status AuditApprove
  dws chat group audit-join-validation --group <openConversationId> --record-id 123456 --applicant <openDingTalkId> --inviter <openDingTalkId> --status AuditRefuse --description "不符合入群条件"
  # 查询入群验证记录: dws chat group list-join-validations`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "group", "record-id", "applicant", "inviter", "status"); err != nil {
				return err
			}
			recordID, err := strconv.ParseInt(mustGetFlag(cmd, "record-id"), 10, 64)
			if err != nil {
				return fmt.Errorf("--record-id must be a valid integer: %w", err)
			}
			toolArgs := map[string]any{
				"openConversationId":      mustGetFlag(cmd, "group"),
				"applyRecordId":           recordID,
				"applicantOpenDingTalkId": mustGetFlag(cmd, "applicant"),
				"inviterOpenDingTalkId":   mustGetFlag(cmd, "inviter"),
				"status":                  mustGetFlag(cmd, "status"),
			}
			if v, _ := cmd.Flags().GetString("description"); v != "" {
				toolArgs["auditDescription"] = v
			}
			return callMCPToolOnServer("im", "audit_join_group", toolArgs)
		},
	}
	chatGroupAuditJoinValidationCmd.Flags().String("group", "", "群 openConversationId (必填)")
	_ = chatGroupAuditJoinValidationCmd.MarkFlagRequired("group")
	chatGroupAuditJoinValidationCmd.Flags().String("record-id", "", "申请记录 ID (必填)")
	_ = chatGroupAuditJoinValidationCmd.MarkFlagRequired("record-id")
	chatGroupAuditJoinValidationCmd.Flags().String("status", "", "审批动作: AuditApprove/AuditDelete/AuditIgnore/AuditRefuse/AuditBlock (必填)")
	_ = chatGroupAuditJoinValidationCmd.MarkFlagRequired("status")
	chatGroupAuditJoinValidationCmd.Flags().String("applicant", "", "申请人 openDingTalkId (必填)")
	_ = chatGroupAuditJoinValidationCmd.MarkFlagRequired("applicant")
	chatGroupAuditJoinValidationCmd.Flags().String("inviter", "", "邀请人 openDingTalkId (必填)")
	_ = chatGroupAuditJoinValidationCmd.MarkFlagRequired("inviter")
	chatGroupAuditJoinValidationCmd.Flags().String("description", "", "审批说明（可选）")

	// ── mark-unread: 标记会话为未读 ───────────────────────────

	chatMarkUnreadCmd := &cobra.Command{
		Use:   "mark-unread",
		Short: "标记会话为未读",
		Long: `将指定会话标记为未读状态。支持群聊和单聊。

如何获取 openConversationId（如果上层已有则直接使用，不必再查）：
  - 群聊：dws chat search --query "群名"
  - 单聊：dws chat conversation-info --open-dingtalk-id <openDingTalkId>`,
		Example: `  dws chat mark-unread --conversation-id <openConversationId>
  dws chat mark-unread --id <openConversationId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			convID := flagOrFallback(cmd, "conversation-id", "id", "chat")
			if convID == "" {
				return fmt.Errorf("flag --conversation-id is required\n  hint: dws chat mark-unread --conversation-id <openConversationId>")
			}
			return callMCPToolOnServer("im", "mark_conversation_unread", map[string]any{
				"openConversationId": convID,
			})
		},
	}
	chatMarkUnreadCmd.Flags().String("conversation-id", "", "会话 openConversationId (必填，支持群聊/单聊)")
	chatMarkUnreadCmd.Flags().String("id", "", "--conversation-id 的别名")
	_ = chatMarkUnreadCmd.Flags().MarkHidden("id")
	chatMarkUnreadCmd.Flags().String("chat", "", "--conversation-id 的别名")
	_ = chatMarkUnreadCmd.Flags().MarkHidden("chat")

	// ── clear-red-point: 清除会话红点 ──────────────────────────

	chatClearRedPointCmd := &cobra.Command{
		Use:   "clear-red-point",
		Short: "清除会话红点",
		Long: `清除指定会话的未读红点。支持群聊和单聊。

如何获取 openConversationId（如果上层已有则直接使用，不必再查）：
  - 群聊：dws chat search --query "群名"
  - 单聊：dws chat conversation-info --open-dingtalk-id <openDingTalkId>`,
		Example: `  dws chat clear-red-point --conversation-id <openConversationId>
  dws chat clear-red-point --id <openConversationId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			convID := flagOrFallback(cmd, "conversation-id", "id", "chat")
			if convID == "" {
				return fmt.Errorf("flag --conversation-id is required\n  hint: dws chat clear-red-point --conversation-id <openConversationId>")
			}
			return callMCPToolOnServer("im", "clear_conversation_red_point", map[string]any{
				"openConversationId": convID,
			})
		},
	}
	chatClearRedPointCmd.Flags().String("conversation-id", "", "会话 openConversationId (必填，支持群聊/单聊)")
	chatClearRedPointCmd.Flags().String("id", "", "--conversation-id 的别名")
	_ = chatClearRedPointCmd.Flags().MarkHidden("id")
	chatClearRedPointCmd.Flags().String("chat", "", "--conversation-id 的别名")
	_ = chatClearRedPointCmd.Flags().MarkHidden("chat")

	// ── clear-all-red-point: 红点清零（全部已读） ─────────────────

	chatClearAllRedPointCmd := &cobra.Command{
		Use:     "clear-all-red-point",
		Short:   "清除所有会话红点（全部已读）",
		Long:    `一键清除当前用户所有会话的未读红点，等效于“全部已读”。`,
		Example: `  dws chat clear-all-red-point`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return callMCPToolOnServer("im", "clear_all_red_point", map[string]any{})
		},
	}

	// ── list-all-conversations: 分页获取全部会话 ────────────────────

	chatListAllConversationsCmd := &cobra.Command{
		Use:   "list-all-conversations",
		Short: "分页获取当前用户的全部会话列表",
		Long: `分页获取当前用户的全部会话列表（包含单聊和群聊）。--limit 指定每页数量（最大 100），--cursor 传分页游标（首次不传或传 0）。
返回 hasMore=true 时用 nextCursor 作为下次 --cursor 继续翻页。`,
		Example: `  dws chat list-all-conversations
  dws chat list-all-conversations --limit 50
  dws chat list-all-conversations --limit 100 --cursor <nextCursor>
  dws chat list-all-conversations --exclude-muted`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if v, err := cmd.Flags().GetInt("limit"); err == nil && v > 0 {
				toolArgs["limit"] = v
			}
			if v, _ := cmd.Flags().GetInt64("cursor"); v > 0 {
				toolArgs["cursor"] = v
			}
			if v, _ := cmd.Flags().GetBool("exclude-muted"); v {
				toolArgs["excludeMuted"] = true
			}
			return callMCPToolOnServer("im", "list_all_conversations", toolArgs)
		},
	}
	chatListAllConversationsCmd.Flags().Int("limit", 1000, "每页数量（默认 1000）")
	chatListAllConversationsCmd.Flags().Int64("cursor", 0, "分页游标（首次不传或传 0，翻页传 nextCursor）")
	chatListAllConversationsCmd.Flags().Bool("exclude-muted", false, "是否排除已免打扰会话（默认 false）")

	// ── clear-messages: 清空会话聊天记录 ────────────────────────────

	chatClearMessagesCmd := &cobra.Command{
		Use:   "clear-messages",
		Short: "清空当前用户指定会话的聊天记录",
		Long: `清空当前用户在指定会话中的聊天记录。仅清空当前用户视角的消息，不影响其他成员。

如何获取 openConversationId（如果上层已有则直接使用，不必再查）：
  - 群聊：dws chat search --query "群名"
  - 单聊：dws chat conversation-info --open-dingtalk-id <openDingTalkId>`,
		Example: `  dws chat clear-messages --conversation-id <openConversationId>
  dws chat clear-messages --id <openConversationId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			convID := flagOrFallback(cmd, "conversation-id", "id", "chat")
			if convID == "" {
				return fmt.Errorf("flag --conversation-id is required\n  hint: dws chat clear-messages --conversation-id <openConversationId>")
			}
			return callMCPToolOnServer("im", "clear_conversation_messages", map[string]any{
				"openConversationId": convID,
			})
		},
	}
	chatClearMessagesCmd.Flags().String("conversation-id", "", "会话 openConversationId (必填，支持群聊/单聊)")
	chatClearMessagesCmd.Flags().String("id", "", "--conversation-id 的别名")
	_ = chatClearMessagesCmd.Flags().MarkHidden("id")
	chatClearMessagesCmd.Flags().String("chat", "", "--conversation-id 的别名")
	_ = chatClearMessagesCmd.Flags().MarkHidden("chat")

	// ── mark-read: 标记消息已读 ────────────────────────────────────

	chatMarkReadCmd := &cobra.Command{
		Use:   "mark-read",
		Short: "标记消息已读",
		Long: `标记指定会话中某条消息为已读。该消息及之前的所有消息都会被标记为已读。

如何获取 openConversationId（如果上层已有则直接使用，不必再查）：
  - 群聊：dws chat search --query "群名"
  - 单聊：dws chat conversation-info --open-dingtalk-id <openDingTalkId>
如何获取 openMessageId：
  - dws chat message list --group <openConversationId> --time "2025-03-01 00:00:00"`,
		Example: `  dws chat mark-read --conversation-id <openConversationId> --message-id <openMessageId>
  dws chat mark-read --id <openConversationId> --message-id <openMessageId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			convID := flagOrFallback(cmd, "conversation-id", "id", "chat")
			if convID == "" {
				return fmt.Errorf("flag --conversation-id is required\n  hint: dws chat mark-read --conversation-id <openConversationId> --message-id <openMessageId>")
			}
			if err := validateRequiredFlags(cmd, "message-id"); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "mark_message_read", map[string]any{
				"openConversationId": convID,
				"openMessageId":      mustGetFlag(cmd, "message-id"),
			})
		},
	}
	chatMarkReadCmd.Flags().String("conversation-id", "", "会话 openConversationId (必填，支持群聊/单聊)")
	chatMarkReadCmd.Flags().String("id", "", "--conversation-id 的别名")
	_ = chatMarkReadCmd.Flags().MarkHidden("id")
	chatMarkReadCmd.Flags().String("chat", "", "--conversation-id 的别名")
	_ = chatMarkReadCmd.Flags().MarkHidden("chat")
	chatMarkReadCmd.Flags().String("message-id", "", "消息 openMessageId (必填)")
	_ = chatMarkReadCmd.MarkFlagRequired("message-id")

	// ── set-top-msg: 置顶某条消息 ────────────────────────────────

	chatMessageSetTopMsgCmd := &cobra.Command{
		Use:   "set-top-msg",
		Short: "置顶消息",
		Long: `将指定消息设置为置顶状态。需要指定会话 ID 和消息 ID。

如何获取 openConversationId（如果上层已有则直接使用，不必再查）：
  - 群聊：dws chat search --query "群名"
  - 单聊：dws chat conversation-info --open-dingtalk-id <openDingTalkId>`,
		Example: `  dws chat message set-top-msg --open-conversation-id <openConversationId> --msg-id <openMessageId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "open-conversation-id", "msg-id"); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "set_top_message", map[string]any{
				"openConversationId": mustGetFlag(cmd, "open-conversation-id"),
				"openMessageId":      mustGetFlag(cmd, "msg-id"),
			})
		},
	}
	chatMessageSetTopMsgCmd.Flags().String("open-conversation-id", "", "会话 openConversationId (必填，支持群聊/单聊)")
	_ = chatMessageSetTopMsgCmd.MarkFlagRequired("open-conversation-id")
	chatMessageSetTopMsgCmd.Flags().String("msg-id", "", "消息 openMessageId (必填)")
	_ = chatMessageSetTopMsgCmd.MarkFlagRequired("msg-id")

	// ── unset-top-msg: 取消置顶某条消息 ─────────────────────────

	chatMessageUnsetTopMsgCmd := &cobra.Command{
		Use:   "unset-top-msg",
		Short: "取消置顶消息",
		Long: `取消指定消息的置顶状态。需要指定会话 ID 和消息 ID。

如何获取 openConversationId（如果上层已有则直接使用，不必再查）：
  - 群聊：dws chat search --query "群名"
  - 单聊：dws chat conversation-info --open-dingtalk-id <openDingTalkId>`,
		Example: `  dws chat message unset-top-msg --open-conversation-id <openConversationId> --msg-id <openMessageId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "open-conversation-id", "msg-id"); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "unset_top_message", map[string]any{
				"openConversationId": mustGetFlag(cmd, "open-conversation-id"),
				"openMessageId":      mustGetFlag(cmd, "msg-id"),
			})
		},
	}
	chatMessageUnsetTopMsgCmd.Flags().String("open-conversation-id", "", "会话 openConversationId (必填，支持群聊/单聊)")
	_ = chatMessageUnsetTopMsgCmd.MarkFlagRequired("open-conversation-id")
	chatMessageUnsetTopMsgCmd.Flags().String("msg-id", "", "消息 openMessageId (必填)")
	_ = chatMessageUnsetTopMsgCmd.MarkFlagRequired("msg-id")

	chatGroupCmd.AddCommand(chatGroupBotsCmd, chatGroupDismissCmd, chatGroupSetHistoryCmd, chatGroupListMyGroupsCmd)
	chatGroupMembersCmd.AddCommand(chatGroupMembersRemoveBotCmd)
	// ── group update-nick: 设置用户在群内的群昵称 ──────────────

	chatGroupUpdateNickCmd := &cobra.Command{
		Use:   "update-nick",
		Short: "设置用户在群内的群昵称",
		Long:  `设置当前用户在指定群聊内的个人群昵称。`,
		Example: `  dws chat group update-nick --group <openConversationId> --nick "我的群昵称"
  # 查询群 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "group", "nick"); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "update_group_nick", map[string]any{
				"openConversationId": mustGetFlag(cmd, "group"),
				"nick":               mustGetFlag(cmd, "nick"),
			})
		},
	}
	chatGroupUpdateNickCmd.Flags().String("group", "", "群聊 openConversationId (必填)")
	_ = chatGroupUpdateNickCmd.MarkFlagRequired("group")
	chatGroupUpdateNickCmd.Flags().String("nick", "", "个人群昵称 (必填)")
	_ = chatGroupUpdateNickCmd.MarkFlagRequired("nick")

	// ── group update-alias: 设置群备注 ──────────────────────────

	chatGroupUpdateAliasCmd := &cobra.Command{
		Use:   "update-alias",
		Short: "设置群备注",
		Long:  `设置当前用户对指定群聊的备注名称（仅自己可见）。`,
		Example: `  dws chat group update-alias --group <openConversationId> --alias-title "项目A群"
  # 查询群 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "group", "alias-title"); err != nil {
				return err
			}
			return callMCPToolOnServer("im", "update_user_group_alias", map[string]any{
				"openConversationId": mustGetFlag(cmd, "group"),
				"aliasTitle":         mustGetFlag(cmd, "alias-title"),
			})
		},
	}
	chatGroupUpdateAliasCmd.Flags().String("group", "", "群聊 openConversationId (必填)")
	_ = chatGroupUpdateAliasCmd.MarkFlagRequired("group")
	chatGroupUpdateAliasCmd.Flags().String("alias-title", "", "群备注标题 (必填)")
	_ = chatGroupUpdateAliasCmd.MarkFlagRequired("alias-title")

	// ── hide: 会话列表中隐藏会话 ────────────────────────────────

	chatHideCmd := &cobra.Command{
		Use:   "hide",
		Short: "会话列表中隐藏会话",
		Long:  `在会话列表中隐藏指定会话（支持单聊/群聊）。隐藏后会话不再显示在列表中，收到新消息时会重新出现。`,
		Example: `  dws chat hide --conversation-id <openConversationId>
  dws chat hide --id <openConversationId>
  # 查询群 ID: dws chat search --query "群名"
  # 查询单聊会话 ID: dws chat conversation-info --user <userId>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			convID := flagOrFallback(cmd, "conversation-id", "id", "chat")
			if convID == "" {
				return fmt.Errorf("flag --conversation-id is required\n  hint: dws chat hide --conversation-id <openConversationId>")
			}
			return callMCPToolOnServer("im", "hide_conversation", map[string]any{
				"openConversationId": convID,
			})
		},
	}
	chatHideCmd.Flags().String("conversation-id", "", "会话 openConversationId (必填，支持单聊/群聊)")
	chatHideCmd.Flags().String("id", "", "--conversation-id 的别名")
	_ = chatHideCmd.Flags().MarkHidden("id")
	chatHideCmd.Flags().String("chat", "", "--conversation-id 的别名")
	_ = chatHideCmd.Flags().MarkHidden("chat")

	// ── mute-at-all: 关闭/开启 @所有人通知 ─────────────────────

	chatMuteAtAllCmd := &cobra.Command{
		Use:   "mute-at-all",
		Short: "关闭/开启 @所有人消息提醒",
		Long:  `关闭或开启会话中 @所有人的消息通知。默认关闭通知，传 --off 则恢复接收通知。`,
		Example: `  dws chat mute-at-all --conversation-id <openConversationId>
  dws chat mute-at-all --conversation-id <openConversationId> --off
  # 查询群 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			convID := flagOrFallback(cmd, "conversation-id", "id", "chat")
			if convID == "" {
				return fmt.Errorf("flag --conversation-id is required\n  hint: dws chat mute-at-all --conversation-id <openConversationId>")
			}
			off, _ := cmd.Flags().GetBool("off")
			return callMCPToolOnServer("im", "update_at_all_notification_off", map[string]any{
				"openConversationId": convID,
				"mute":               !off,
			})
		},
	}
	chatMuteAtAllCmd.Flags().String("conversation-id", "", "会话 openConversationId (必填，支持单聊/群聊)")
	chatMuteAtAllCmd.Flags().String("id", "", "--conversation-id 的别名")
	_ = chatMuteAtAllCmd.Flags().MarkHidden("id")
	chatMuteAtAllCmd.Flags().String("chat", "", "--conversation-id 的别名")
	_ = chatMuteAtAllCmd.Flags().MarkHidden("chat")
	chatMuteAtAllCmd.Flags().Bool("off", false, "恢复接收 @所有人通知（不传则关闭通知）")

	// ── mute-red-envelope: 关闭/开启红包通知 ────────────────────

	chatMuteRedEnvelopeCmd := &cobra.Command{
		Use:   "mute-red-envelope",
		Short: "关闭/开启红包消息提醒",
		Long:  `关闭或开启会话中的红包消息通知。默认关闭通知，传 --off 则恢复接收通知。`,
		Example: `  dws chat mute-red-envelope --conversation-id <openConversationId>
  dws chat mute-red-envelope --conversation-id <openConversationId> --off
  # 查询群 ID: dws chat search --query "群名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			convID := flagOrFallback(cmd, "conversation-id", "id", "chat")
			if convID == "" {
				return fmt.Errorf("flag --conversation-id is required\n  hint: dws chat mute-red-envelope --conversation-id <openConversationId>")
			}
			off, _ := cmd.Flags().GetBool("off")
			return callMCPToolOnServer("im", "update_red_env_notification_off", map[string]any{
				"openConversationId": convID,
				"mute":               !off,
			})
		},
	}
	chatMuteRedEnvelopeCmd.Flags().String("conversation-id", "", "会话 openConversationId (必填，支持单聊/群聊)")
	chatMuteRedEnvelopeCmd.Flags().String("id", "", "--conversation-id 的别名")
	_ = chatMuteRedEnvelopeCmd.Flags().MarkHidden("id")
	chatMuteRedEnvelopeCmd.Flags().String("chat", "", "--conversation-id 的别名")
	_ = chatMuteRedEnvelopeCmd.Flags().MarkHidden("chat")
	chatMuteRedEnvelopeCmd.Flags().Bool("off", false, "恢复接收红包通知（不传则关闭通知）")

	// ── group members list-by-ids: 批量查看群成员详情 ───────────

	chatGroupMembersListByIdsCmd := &cobra.Command{
		Use:   "list-by-ids",
		Short: "根据成员 ID 批量查询群成员详情",
		Long:  `根据群 openConversationId 和成员 openDingTalkId 列表，批量查询群成员详情信息。`,
		Example: `  dws chat group members list-by-ids --id <openConversationId> --users openDingTalkId1,openDingTalkId2
  # 查询群 ID: dws chat search --query "群名"
  # 查询 openDingTalkId: dws contact user search --query "姓名"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateRequiredFlags(cmd, "id", "users"); err != nil {
				return err
			}
			users := parseCSVValues(mustGetFlag(cmd, "users"))
			return callMCPToolOnServer("im", "list_group_member_by_ids", map[string]any{
				"openConversationId":    mustGetFlag(cmd, "id"),
				"memberOpenDingTalkIds": users,
			})
		},
	}
	chatGroupMembersListByIdsCmd.Flags().String("id", "", "群 ID / openConversationId (必填)")
	_ = chatGroupMembersListByIdsCmd.MarkFlagRequired("id")
	chatGroupMembersListByIdsCmd.Flags().String("users", "", "成员 openDingTalkId 列表，逗号分隔 (必填)")
	_ = chatGroupMembersListByIdsCmd.MarkFlagRequired("users")

	chatGroupCmd.AddCommand(chatGroupBotsCmd, chatGroupDismissCmd, chatGroupSetHistoryCmd, chatGroupListMyGroupsCmd, chatGroupUpdateNickCmd, chatGroupUpdateAliasCmd, chatGroupListAllCmd, chatGroupListJoinValidationsCmd, chatGroupAuditJoinValidationCmd)
	chatGroupMembersCmd.AddCommand(chatGroupMembersRemoveBotCmd, chatGroupMembersListByIdsCmd)
	chatBotCmd.AddCommand(chatBotFindCmd)
	chatMessageCmd.AddCommand(chatMessageListDirectCmd, chatMessageSearchCommonCmd, chatMessageCombineForwardCmd, chatMessageForwardTopicCmd, chatMessageSetPinCmd, chatMessageUnsetPinCmd, chatMessageListPinCmd, chatMessageSetTopMsgCmd, chatMessageUnsetTopMsgCmd)

	root.AddCommand(chatChmodCmd, chatDataAuthCmd, chatGroupCmd, chatSearchCmd, chatSearchCommonCmd, chatMessageCmd, chatFileCmd, newChatMediaGroup(), chatBotCmd, chatMessageListTopConversationsCmd, chatConversationInfoCmd, chatCategoryCmd, chatGroupRoleCmd, chatMuteCmd, chatSetTopCmd, chatGroupMuteCmd, chatGroupMuteMemberCmd, chatHideCmd, chatMuteAtAllCmd, chatMuteRedEnvelopeCmd, chatMarkUnreadCmd, chatClearRedPointCmd, chatClearAllRedPointCmd, chatListAllConversationsCmd, chatClearMessagesCmd, chatMarkReadCmd)

	// hint: dws chat send → dws chat message send
	root.AddCommand(hintSubCmd("send", "use: dws chat message send"))
	// hint: dws chat history → dws chat message list
	root.AddCommand(hintSubCmd("history", "use: dws chat message list --group <GROUP_OPEN_CONVERSATION_ID>"))

	return root
}
