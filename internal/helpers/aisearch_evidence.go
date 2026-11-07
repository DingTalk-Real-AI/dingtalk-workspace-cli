package helpers

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// runAisearchWithEvidence preserves the MCP payload and adds deterministic
// evidence metadata. The metadata makes the limits of a search result explicit
// without turning a snippet, a backend candidate, or one returned page into a
// stronger business claim.
func runAisearchWithEvidence(cmd *cobra.Command, toolName string, args map[string]any) error {
	data, err := CallMCPToolDataOnServer(cmd.Context(), "aisearch", toolName, args)
	if err != nil {
		return err
	}
	if toolName == "search_enterprise" {
		data = resolveAisearchExactCandidates(cmd.Context(), data, args, time.Now())
	} else if toolName == "search_enterprise_behavior" {
		data = resolveAisearchBehaviorMessages(cmd.Context(), data, args, time.Now())
	}
	return GetFormatter().PrintJSON(enrichAisearchResponse(data, args, time.Now()))
}

// resolveAisearchBehaviorMessages verifies person-directed IM behavior inside
// the same CLI command. AISearch first resolves the person's returned aliases,
// then asks Chat for the original time range and keeps only messages whose
// sender display matches one of those aliases. This replaces the common agent
// loop of loading Chat, guessing flags, widening time and post-filtering JSON.
func resolveAisearchBehaviorMessages(ctx context.Context, data any, args map[string]any, now time.Time) any {
	root, ok := data.(map[string]any)
	if !ok || !containsAisearchString(stringSlice(args["searchTypes"]), "im") {
		return data
	}
	sender := behaviorDirectionSender(aisearchStringValue(args["direction"]))
	if sender == "" {
		return data
	}
	aliases := []string(nil)
	identityMatch := "any_returned_sender"
	if !isGenericAisearchSender(sender) {
		person, err := CallMCPToolDataOnServer(ctx, "aisearch", "enterprise_person_search", map[string]any{
			"keyword": sender, "dimension": []string{"name"},
		})
		if err != nil {
			return data
		}
		var unique bool
		aliases, unique = uniquePersonAliases(person)
		if !unique {
			return data
		}
		identityMatch = "returned_alias"
	}
	start, end, ranged := aisearchNaturalRange(aisearchStringValue(args["timeRange"]), now)
	if !ranged {
		start, end = now.AddDate(-1, 0, 0), now
	}
	queries := stringSlice(args["queries"])
	server, tool := "chat", "search_messages_by_time_range"
	callArgs := map[string]any{"startTime": start.UnixMilli(), "endTime": end.UnixMilli(), "limit": 100, "cursor": "0"}
	if looksLikeWordFileQuery(queries) {
		tool = "search_messages_by_keyword"
		callArgs["keyword"] = ".docx"
	} else if isGenericAisearchSender(sender) && len(queries) > 0 {
		tool = "search_messages_by_keyword"
		callArgs["keyword"] = aisearchMessageKeyword(queries[0])
	}
	messagesRaw, err := CallMCPToolDataOnServer(ctx, server, tool, callArgs)
	if err != nil {
		root["resolvedMessageEvidence"] = map[string]any{"status": "failed", "source": server + "/" + tool, "error": err.Error()}
		return root
	}
	messages := filterAisearchMessagesByTime(collectAisearchMessages(messagesRaw, aliases, queries), start, end)
	if looksLikeWordFileQuery(queries) {
		messages = dedupeAisearchMessagesByText(messages)
	}
	truncated := false
	if len(messages) > 20 {
		messages, truncated = messages[:20], true
	}
	root["resolvedMessageEvidence"] = map[string]any{
		"status": "resolved", "source": server + "/" + tool,
		"identity":  map[string]any{"query": sender, "aliases": aliases, "match": identityMatch},
		"timeRange": map[string]any{"start": start.Format(time.RFC3339), "end": end.Format(time.RFC3339), "fromUserConstraint": ranged},
		"messages":  messages, "messageCount": len(messages), "truncatedTo20": truncated,
		"delivery": map[string]any{"action": "answer_from_resolved_messages", "doNotCallChatAgain": true},
	}
	return root
}

func aisearchMessageKeyword(query string) string {
	query = strings.TrimSpace(query)
	for _, suffix := range []string{"相关文件", "文件", "相关消息", "消息"} {
		query = strings.TrimSpace(strings.TrimSuffix(query, suffix))
	}
	return query
}

func isGenericAisearchSender(sender string) bool {
	sender = strings.TrimSpace(sender)
	return sender == "某人" || sender == "别人" || sender == "其他人" || sender == "谁"
}

func filterAisearchMessagesByTime(messages []map[string]any, start, end time.Time) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		raw := aisearchStringValue(message["time"])
		parsed, err := time.ParseInLocation("2006-01-02 15:04:05", raw, start.Location())
		if err == nil && (parsed.Before(start) || parsed.After(end)) {
			continue
		}
		out = append(out, message)
	}
	return out
}

func dedupeAisearchMessagesByText(messages []map[string]any) []map[string]any {
	seen := map[string]bool{}
	out := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		key := strings.ToLower(aisearchStringValue(message["text"]))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, message)
	}
	return out
}

func behaviorDirectionSender(direction string) string {
	for _, arrow := range []string{"->", "→"} {
		parts := strings.Split(direction, arrow)
		if len(parts) == 2 && strings.TrimSpace(parts[1]) == "我" && strings.TrimSpace(parts[0]) != "我" {
			return strings.TrimSpace(parts[0])
		}
	}
	return ""
}

func uniquePersonAliases(data any) ([]string, bool) {
	root, _ := data.(map[string]any)
	items, _ := root["result"].([]any)
	if len(items) != 1 {
		return nil, false
	}
	item, _ := items[0].(map[string]any)
	aliases := uniqueAisearchStrings(
		firstAisearchValue(item, "meta.name"), firstAisearchValue(item, "meta.nick"),
		firstAisearchValue(item, "title"), firstAisearchValue(item, "author"),
	)
	return aliases, len(aliases) > 0
}

func looksLikeWordFileQuery(queries []string) bool {
	joined := strings.ToLower(strings.Join(queries, " "))
	return strings.Contains(joined, "word") || strings.Contains(joined, "docx") || strings.Contains(joined, "文档文件")
}

func collectAisearchMessages(data any, aliases, queries []string) []map[string]any {
	out := make([]map[string]any, 0)
	seen := map[string]bool{}
	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			sender := firstAisearchValue(typed, "sender", "senderName", "creatorName")
			text := firstAisearchValue(typed, "text", "content", "snippet")
			id := firstAisearchValue(typed, "messageId", "openMessageId", "msgId")
			if sender != "" && (id != "" || text != "") && matchesAisearchAlias(sender, aliases) && messageMatchesAisearchQuery(typed, text, queries) {
				key := id
				if key == "" {
					key = sender + "\x00" + text
				}
				if !seen[key] {
					seen[key] = true
					out = append(out, map[string]any{
						"sender": sender, "senderId": firstAisearchValue(typed, "senderId", "senderUserId"),
						"time": firstAisearchValue(typed, "time", "createTime", "sendTime"), "text": text,
						"conversationId": firstAisearchValue(typed, "conversationId", "openConversationId"), "messageId": id,
						"resourceRefs": typed["resourceRefs"],
					})
				}
			}
			for _, child := range typed {
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(data)
	return out
}

func matchesAisearchAlias(sender string, aliases []string) bool {
	if len(aliases) == 0 {
		return strings.TrimSpace(sender) != ""
	}
	sender = strings.ToLower(sender)
	for _, alias := range aliases {
		if strings.Contains(sender, strings.ToLower(alias)) {
			return true
		}
	}
	return false
}

func messageMatchesAisearchQuery(item map[string]any, text string, queries []string) bool {
	if len(queries) == 0 {
		return true
	}
	haystack := strings.ToLower(text + " " + fmt.Sprint(item["resourceRefs"]))
	if looksLikeWordFileQuery(queries) {
		return strings.Contains(haystack, ".docx") || strings.Contains(haystack, "word")
	}
	for _, query := range queries {
		for _, term := range append([]string{query}, splitAisearchQueryTerms(query)...) {
			if term != "" && strings.Contains(haystack, strings.ToLower(term)) {
				return true
			}
		}
	}
	return false
}

// resolveAisearchExactCandidates folds bounded native reads into one enterprise
// search. It only runs when a requested source has exactly one exact-title
// candidate with a stable ID. This avoids a second agent turn without widening
// the user's query or guessing an object.
func resolveAisearchExactCandidates(ctx context.Context, data any, args map[string]any, now time.Time) any {
	root, ok := data.(map[string]any)
	if !ok {
		return data
	}
	items, _ := root["result"].([]any)
	items = appendExactCalendarCandidate(ctx, items, args, now)
	root["result"] = items

	queries := stringSlice(args["queries"])
	bySource := map[string][]map[string]any{}
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok || !matchesExactAisearchTitle(aisearchStringValue(item["title"]), queries) {
			continue
		}
		source := aisearchSourceType(item)
		bySource[source] = append(bySource[source], item)
	}
	for source, candidates := range bySource {
		if len(candidates) != 1 {
			continue
		}
		item := candidates[0]
		var server, tool string
		var callArgs map[string]any
		switch source {
		case "document":
			nodeID := firstAisearchValue(item, "nodeId", "meta.nodeId", "meta.doc_id")
			if nodeID == "" {
				continue
			}
			docType := strings.ToLower(firstAisearchValue(item, "meta.doc_type"))
			if docType == "adoc" || docType == "doc" || docType == "document" {
				server, tool, callArgs = "doc", "get_document_content", map[string]any{"nodeId": nodeID}
			} else {
				server, tool, callArgs = "drive", "get_file_info", map[string]any{"fileId": nodeID}
			}
		case "todo":
			taskID := firstAisearchValue(item, "taskId", "meta.taskId")
			if taskID == "" {
				taskID = taskIDFromURL(aisearchStringValue(item["url"]))
			}
			if taskID == "" {
				continue
			}
			server, tool, callArgs = "todo", "get_todo_detail", map[string]any{"taskId": taskID}
		case "minute":
			if summary := firstAisearchValue(item, "meta.summary"); summary != "" {
				item["resolvedDetail"] = map[string]any{
					"status": "resolved_from_search_payload",
					"source": "minute.summary",
					"data":   map[string]any{"summary": summary},
				}
			}
			continue
		case "calendar":
			// The calendar fallback below already returns the native event object.
			continue
		default:
			continue
		}
		resolved, err := CallMCPToolDataOnServer(ctx, server, tool, callArgs)
		if err != nil {
			item["resolvedDetail"] = map[string]any{
				"status": "failed", "source": server + "/" + tool, "error": err.Error(),
			}
			continue
		}
		item["resolvedDetail"] = map[string]any{
			"status": "resolved", "source": server + "/" + tool, "data": resolved,
		}
	}
	return root
}

func appendExactCalendarCandidate(ctx context.Context, items []any, args map[string]any, now time.Time) []any {
	requested := stringSlice(args["searchTypes"])
	if !containsAisearchString(requested, "calendar") && !containsAisearchString(requested, "all") {
		return items
	}
	queries := stringSlice(args["queries"])
	for _, raw := range items {
		if item, ok := raw.(map[string]any); ok && aisearchSourceType(item) == "calendar" && matchesExactAisearchTitle(aisearchStringValue(item["title"]), queries) {
			return items
		}
	}
	start, end, ok := aisearchNaturalRange(aisearchStringValue(args["timeRange"]), now)
	if !ok {
		return items
	}
	data, err := CallMCPToolDataOnServer(ctx, "calendar", "list_calendar_events", map[string]any{
		"startTime": start.UnixMilli(), "endTime": end.UnixMilli(), "limit": 100,
	})
	if err != nil {
		return items
	}
	events := aisearchCalendarEvents(data)
	exact := make([]map[string]any, 0)
	for _, event := range events {
		if matchesExactAisearchTitle(firstAisearchValue(event, "summary", "title"), queries) {
			exact = append(exact, event)
		}
	}
	if len(exact) != 1 {
		return items
	}
	event := exact[0]
	items = append(items, map[string]any{
		"sourceType": "calendar",
		"sourceName": "日程",
		"title":      firstAisearchValue(event, "summary", "title"),
		"date":       firstAisearchValue(event, "start.dateTime", "start.date"),
		"snippet":    firstAisearchValue(event, "description", "richTextDescription.text"),
		"meta": map[string]any{
			"eventId": event["id"], "nativeFallback": true,
		},
		"resolvedDetail": map[string]any{
			"status": "resolved", "source": "calendar/list_calendar_events", "data": event,
		},
	})
	return items
}

func aisearchCalendarEvents(data any) []map[string]any {
	root, _ := data.(map[string]any)
	for _, path := range [][]string{{"result", "events"}, {"events"}, {"result", "items"}, {"items"}} {
		value := any(root)
		for _, key := range path {
			current, ok := value.(map[string]any)
			if !ok {
				value = nil
				break
			}
			value = current[key]
		}
		if rows, ok := value.([]any); ok {
			out := make([]map[string]any, 0, len(rows))
			for _, row := range rows {
				if item, ok := row.(map[string]any); ok {
					out = append(out, item)
				}
			}
			return out
		}
	}
	return nil
}

func matchesExactAisearchTitle(title string, queries []string) bool {
	title = normalizeAisearchTitle(title)
	if title == "" {
		return false
	}
	for _, query := range queries {
		if title == normalizeAisearchTitle(query) {
			return true
		}
	}
	return false
}

func normalizeAisearchTitle(value string) string {
	value = strings.TrimSpace(strings.Trim(value, "《》\"'"))
	return strings.ToLower(value)
}

func firstAisearchValue(root map[string]any, paths ...string) string {
	for _, path := range paths {
		var value any = root
		for _, key := range strings.Split(path, ".") {
			current, ok := value.(map[string]any)
			if !ok {
				value = nil
				break
			}
			value = current[key]
		}
		if text := aisearchStringValue(value); text != "" {
			return text
		}
	}
	return ""
}

func containsAisearchString(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}

func runAisearchPersonWithEvidence(cmd *cobra.Command, args map[string]any) error {
	data, err := CallMCPToolDataOnServer(cmd.Context(), "aisearch", "enterprise_person_search", args)
	if err != nil {
		return err
	}
	data = resolveAisearchPersonRelation(cmd.Context(), data, args)
	return GetFormatter().PrintJSON(enrichAisearchPersonResponse(data, args))
}

func resolveAisearchPersonRelation(ctx context.Context, data any, args map[string]any) any {
	dimensions := stringSlice(args["dimension"])
	if !containsAisearchString(dimensions, "supervisor") {
		return data
	}
	query := aisearchStringValue(args["keyword"])
	target := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(query, "的直属上级"), "的上级"))
	if target == "" || target == query {
		return data
	}
	person, err := CallMCPToolDataOnServer(ctx, "aisearch", "enterprise_person_search", map[string]any{
		"keyword": target, "dimension": []string{"name"},
	})
	if err != nil {
		return data
	}
	root, ok := data.(map[string]any)
	if !ok {
		return data
	}
	personRoot, _ := person.(map[string]any)
	items, _ := personRoot["result"].([]any)
	if len(items) != 1 {
		return data
	}
	item, _ := items[0].(map[string]any)
	supervisor := firstAisearchValue(item, "meta.supervisor")
	if supervisor == "" {
		return data
	}
	root["resolvedRelation"] = map[string]any{
		"status": "resolved", "relation": "direct_supervisor", "target": target,
		"supervisor": supervisor, "source": "enterprise_person_search(name).meta.supervisor",
		"delivery": map[string]any{"action": "answer_from_resolved_relation", "doNotSearchAgain": true},
	}
	return root
}

func enrichAisearchPersonResponse(data any, args map[string]any) any {
	root, ok := data.(map[string]any)
	if !ok {
		return map[string]any{"result": data}
	}
	items, _ := root["result"].([]any)
	exactAliasContainsCount := 0
	query := strings.ToLower(strings.TrimSpace(aisearchStringValue(args["keyword"])))
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		aliases := uniqueAisearchStrings(
			aisearchStringValue(nestedValue(item, "meta", "name")),
			aisearchStringValue(nestedValue(item, "meta", "nick")),
			aisearchStringValue(item["title"]),
			aisearchStringValue(item["author"]),
		)
		exactAliasContains := false
		for _, alias := range aliases {
			if query != "" && strings.Contains(strings.ToLower(alias), query) {
				exactAliasContains = true
				break
			}
		}
		if exactAliasContains {
			exactAliasContainsCount++
		}
		item["_searchEvidence"] = map[string]any{
			"candidateKind":           "person_search_candidate",
			"aliases":                 aliases,
			"identityRefs":            aisearchIdentityRefs(item),
			"exactAliasContainsQuery": exactAliasContains,
			"note":                    "aliases are returned identity labels; a candidate is not proof of unique responsibility",
		}
	}
	root["searchEvidence"] = map[string]any{
		"requestConstraints": map[string]any{
			"query":     args["keyword"],
			"dimension": args["dimension"],
		},
		"returnedCandidateCount":  len(items),
		"exactAliasContainsCount": exactAliasContainsCount,
		"interpretation":          "an empty result means this exact person search returned no candidate; a non-empty result is a candidate set, not proof of uniqueness",
	}
	return root
}

func enrichAisearchResponse(data any, args map[string]any, now time.Time) any {
	root, ok := data.(map[string]any)
	if !ok {
		return map[string]any{
			"result":         data,
			"searchEvidence": buildAisearchEvidence(nil, args, now),
		}
	}

	items, _ := root["result"].([]any)
	for _, raw := range items {
		if item, ok := raw.(map[string]any); ok {
			item["_searchEvidence"] = buildAisearchCandidateEvidence(item, args, now)
			compactAisearchSnippet(item, 1600)
		}
	}
	evidence := buildAisearchEvidence(items, args, now)
	if firstAisearchValue(root, "resolvedMessageEvidence.status") == "resolved" {
		root["result"] = []any{}
		evidence["delivery"] = map[string]any{
			"action": "answer_from_resolved_messages", "returnedToAgentCount": 0,
			"omittedSearchCandidateCount": len(items), "doNotRetryOrExpand": true,
			"note": "resolvedMessageEvidence contains the verified per-message evidence; verbose semantic conversation candidates were omitted",
		}
		root["searchEvidence"] = evidence
		return root
	}
	compact, omittedUnverified, omittedOutOfRange := compactAisearchItems(items)
	root["result"] = compact
	evidence["delivery"] = map[string]any{
		"action":                          "answer_from_this_response",
		"returnedToAgentCount":            len(compact),
		"omittedUnverifiedCandidateCount": omittedUnverified,
		"omittedOutOfRangeCount":          omittedOutOfRange,
		"doNotRetryOrExpand":              true,
		"note":                            "list retained candidates only; omitted candidates had no textual evidence or were outside the requested time range",
	}
	root["searchEvidence"] = evidence
	return root
}

func compactAisearchSnippet(item map[string]any, limit int) {
	snippet, ok := item["snippet"].(string)
	if !ok || limit < 1 || len([]rune(snippet)) <= limit {
		return
	}
	runes := []rune(snippet)
	item["snippet"] = string(runes[:limit]) + "…"
	evidence, _ := item["_searchEvidence"].(map[string]any)
	if evidence != nil {
		evidence["snippetTruncated"] = true
		evidence["originalSnippetCharacters"] = len(runes)
	}
}

// compactAisearchItems keeps the response useful while preventing a long tail
// of unverified semantic candidates from consuming agent context. Confirmed or
// partially evidenced candidates are always kept. If a source has no textual
// evidence at all, at most three examples are retained so the caller can still
// see what the backend returned without mistaking the list for verified hits.
func compactAisearchItems(items []any) ([]any, int, int) {
	evidencedBySource := map[string]bool{}
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		match := firstAisearchValue(item, "_searchEvidence.queryMatch")
		if match == "text_evidence_present" || match == "partial_text_evidence" || item["resolvedDetail"] != nil {
			evidencedBySource[aisearchSourceType(item)] = true
		}
	}
	keptUnverified := map[string]int{}
	out := make([]any, 0, len(items))
	omittedUnverified, omittedOutOfRange := 0, 0
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			out = append(out, raw)
			continue
		}
		if within := nestedValue(item, "_searchEvidence", "timeEvidence", "withinRequestedRange"); within == false {
			omittedOutOfRange++
			continue
		}
		match := firstAisearchValue(item, "_searchEvidence.queryMatch")
		if match != "no_text_evidence" || item["resolvedDetail"] != nil {
			out = append(out, raw)
			continue
		}
		source := aisearchSourceType(item)
		limit := 3
		if evidencedBySource[source] {
			limit = 0
		}
		if keptUnverified[source] < limit {
			keptUnverified[source]++
			out = append(out, raw)
		} else {
			omittedUnverified++
		}
	}
	return out, omittedUnverified, omittedOutOfRange
}

func buildAisearchEvidence(items []any, args map[string]any, now time.Time) map[string]any {
	requested := stringSlice(args["searchTypes"])
	counts := map[string]int{}
	for _, raw := range items {
		if item, ok := raw.(map[string]any); ok {
			counts[aisearchSourceType(item)]++
		}
	}
	if len(requested) == 1 && requested[0] == "all" {
		requested = make([]string, 0, len(counts))
		for source := range counts {
			requested = append(requested, source)
		}
		sort.Strings(requested)
	}
	sources := make([]map[string]any, 0, len(requested))
	for _, source := range requested {
		count := counts[source]
		status := "no_returned_items"
		if count > 0 {
			status = "items_returned"
		}
		sources = append(sources, map[string]any{
			"sourceType": source,
			"requested":  true,
			"status":     status,
			"itemCount":  count,
		})
	}

	constraints := map[string]any{
		"queries":     stringSlice(args["queries"]),
		"searchTypes": requested,
	}
	for _, key := range []string{"timeRange", "behaviorType", "direction", "chatScope"} {
		if value, ok := args[key]; ok {
			constraints[key] = value
		}
	}
	return map[string]any{
		"requestConstraints": constraints,
		"returnedItemCount":  len(items),
		"sources":            sources,
		"coverage": map[string]any{
			"complete": false,
			"status":   "unknown",
			"reason":   "backend response has no verified pagination or completeness evidence",
		},
		"interpretation": "no_returned_items means this request returned no candidates; it does not prove the source has no matching object",
	}
}

func buildAisearchCandidateEvidence(item, args map[string]any, now time.Time) map[string]any {
	title := aisearchStringValue(item["title"])
	snippet := aisearchStringValue(item["snippet"])
	queries := stringSlice(args["queries"])
	match := "not_requested"
	matchedTerms := []string{}
	if len(queries) > 0 {
		match = "no_text_evidence"
		haystack := strings.ToLower(title + "\n" + snippet)
		for _, query := range queries {
			if normalized := strings.ToLower(strings.TrimSpace(query)); normalized != "" && strings.Contains(haystack, normalized) {
				match = "text_evidence_present"
				matchedTerms = append(matchedTerms, query)
				break
			}
			for _, term := range splitAisearchQueryTerms(query) {
				if strings.Contains(haystack, strings.ToLower(term)) {
					matchedTerms = append(matchedTerms, term)
				}
			}
		}
		matchedTerms = uniqueAisearchStrings(matchedTerms...)
		if match == "no_text_evidence" && len(matchedTerms) > 0 {
			match = "partial_text_evidence"
		}
	}

	contentKind := "search_snippet"
	originalContentRead := false
	if status := firstAisearchValue(item, "resolvedDetail.status"); status == "resolved" || status == "resolved_from_search_payload" {
		contentKind, originalContentRead = "resolved_original_object", true
	}
	evidence := map[string]any{
		"sourceType":          aisearchSourceType(item),
		"contentKind":         contentKind,
		"originalContentRead": originalContentRead,
		"queryMatch":          match,
		"matchedTerms":        matchedTerms,
		"queryMatchNote":      "partial_text_evidence and no_text_evidence are unverified semantic candidates, not confirmed full-topic matches",
		"stableRefs":          aisearchStableRefs(item),
		"identityRefs":        aisearchIdentityRefs(item),
	}
	if rawDate := aisearchStringValue(item["date"]); rawDate != "" {
		within, status := aisearchTimeRangeStatus(rawDate, aisearchStringValue(args["timeRange"]), now)
		evidence["timeEvidence"] = map[string]any{
			"returnedAt":           rawDate,
			"withinRequestedRange": within,
			"status":               status,
			"note":                 "returnedAt is the search result date; its business meaning still depends on the source object",
		}
	}
	if aisearchSourceType(item) == "im" {
		evidence["messageEvidence"] = map[string]any{
			"perMessageVerified":       false,
			"deliveryRelationVerified": false,
			"note":                     "a conversation snippet may contain multiple senders; conversation visibility does not prove every message was sent by or directly to the requested person",
		}
	}
	return evidence
}

func aisearchSourceType(item map[string]any) string {
	for _, value := range []any{item["sourceType"], nestedValue(item, "meta", "dataType")} {
		if s := strings.ToLower(strings.TrimSpace(aisearchStringValue(value))); s != "" {
			if s == "doc" {
				return "document"
			}
			return s
		}
	}
	return "unknown"
}

func aisearchStableRefs(item map[string]any) []map[string]string {
	fields := []struct{ key, domain string }{
		{"nodeId", "document.nodeId"}, {"taskUuid", "minutes.taskUuid"},
		{"taskId", "todo.taskId"}, {"openConversationId", "chat.openConversationId"},
		{"fileId", "drive.fileId"},
	}
	refs := make([]map[string]string, 0)
	seen := map[string]bool{}
	for _, field := range fields {
		value := aisearchStringValue(item[field.key])
		if value == "" {
			value = aisearchStringValue(nestedValue(item, "meta", field.key))
		}
		if value == "" && field.key == "taskId" {
			value = taskIDFromURL(aisearchStringValue(item["url"]))
		}
		if value == "" || seen[field.domain+"\x00"+value] {
			continue
		}
		seen[field.domain+"\x00"+value] = true
		refs = append(refs, map[string]string{"domain": field.domain, "value": value})
	}
	return refs
}

func aisearchIdentityRefs(item map[string]any) []map[string]string {
	fields := []struct{ key, domain string }{
		{"actor", "aisearch.actor"}, {"author", "aisearch.author"},
		{"creator", "aisearch.creator"}, {"creatorId", "aisearch.creatorId"},
		{"userId", "contact.userId"},
	}
	refs := make([]map[string]string, 0)
	for _, field := range fields {
		value := aisearchStringValue(item[field.key])
		if value == "" {
			value = aisearchStringValue(nestedValue(item, "meta", field.key))
		}
		if value != "" {
			refs = append(refs, map[string]string{"domain": field.domain, "value": value})
		}
	}
	return refs
}

func aisearchTimeRangeStatus(rawDate, rawRange string, now time.Time) (any, string) {
	if strings.TrimSpace(rawRange) == "" {
		return nil, "not_requested"
	}
	value, err := time.ParseInLocation("2006-01-02 15:04:05", rawDate, now.Location())
	if err != nil {
		return nil, "unparseable_result_time"
	}
	start, end, ok := aisearchNaturalRange(rawRange, now)
	if !ok {
		return nil, "range_not_machine_verifiable"
	}
	return !value.Before(start) && !value.After(end), "verified_against_returned_at"
}

var recentDaysPattern = regexp.MustCompile(`^(?:最近|近)([一二三四五六七八九十\d]+)天$`)

func aisearchNaturalRange(raw string, now time.Time) (time.Time, time.Time, bool) {
	raw = strings.TrimSpace(raw)
	dayStart := func(t time.Time) time.Time { y, m, d := t.Date(); return time.Date(y, m, d, 0, 0, 0, 0, t.Location()) }
	end := now
	switch raw {
	case "今天", "今日":
		return dayStart(now), end, true
	case "昨天", "昨日":
		start := dayStart(now).AddDate(0, 0, -1)
		return start, start.Add(24*time.Hour - time.Nanosecond), true
	case "本周":
		weekday := (int(now.Weekday()) + 6) % 7
		return dayStart(now).AddDate(0, 0, -weekday), end, true
	case "本月", "这个月", "当月":
		y, m, _ := now.Date()
		return time.Date(y, m, 1, 0, 0, 0, 0, now.Location()), end, true
	case "最近一周", "近一周", "最近七天", "近七天":
		return dayStart(now).AddDate(0, 0, -6), end, true
	case "未来七天", "未来7天":
		return now, dayStart(now).AddDate(0, 0, 7).Add(24*time.Hour - time.Nanosecond), true
	}
	if match := recentDaysPattern.FindStringSubmatch(raw); len(match) == 2 {
		days, ok := chineseOrArabicNumber(match[1])
		if ok && days > 0 {
			return dayStart(now).AddDate(0, 0, -days+1), end, true
		}
	}
	return time.Time{}, time.Time{}, false
}

func splitAisearchQueryTerms(query string) []string {
	parts := strings.FieldsFunc(strings.TrimSpace(query), func(r rune) bool {
		return r == ' ' || r == '\t' || r == ',' || r == '，' || r == '/' || r == '、'
	})
	if len(parts) > 1 {
		return parts
	}
	// Common compound intent suffixes are useful evidence boundaries without
	// pretending to perform semantic segmentation.
	for _, suffix := range []string{"风险", "负责人", "策略", "资料", "文件", "项目", "培训", "报价", "续约"} {
		if strings.HasSuffix(query, suffix) && len([]rune(query)) > len([]rune(suffix)) {
			prefix := strings.TrimSuffix(query, suffix)
			terms := []string{prefix, suffix}
			prefixRunes := []rune(prefix)
			if len(prefixRunes) >= 2 {
				terms = append(terms, string(prefixRunes[len(prefixRunes)-2:])+suffix)
			}
			return uniqueAisearchStrings(terms...)
		}
	}
	return parts
}

func uniqueAisearchStrings(values ...string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func chineseOrArabicNumber(raw string) (int, bool) {
	if n, err := strconv.Atoi(raw); err == nil {
		return n, true
	}
	values := map[string]int{"一": 1, "二": 2, "三": 3, "四": 4, "五": 5, "六": 6, "七": 7, "八": 8, "九": 9, "十": 10}
	n, ok := values[raw]
	return n, ok
}

func taskIDFromURL(raw string) string {
	if raw == "" {
		return ""
	}
	decoded, err := url.QueryUnescape(raw)
	if err != nil {
		decoded = raw
	}
	parsed, err := url.Parse(decoded)
	if err == nil {
		if value := parsed.Query().Get("taskId"); value != "" {
			return value
		}
	}
	match := regexp.MustCompile(`[?&]taskId=([^&#]+)`).FindStringSubmatch(decoded)
	if len(match) == 2 {
		return match[1]
	}
	return ""
}

func nestedValue(root map[string]any, keys ...string) any {
	var value any = root
	for _, key := range keys {
		current, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = current[key]
	}
	return value
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s := aisearchStringValue(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{typed}
	default:
		return nil
	}
}

func aisearchStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}
