// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

// Package minutesdata owns response validation and pagination facts shared by
// Minutes shortcuts. A missing or malformed collection is never treated as an
// empty successful result.
package minutesdata

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Page is one validated Minutes collection page.
type Page struct {
	Items        []map[string]any
	HasMore      bool
	HasMoreKnown bool
	NextToken    string
}

var listKeys = []string{"itemList", "minutesList", "items", "list", "records", "dataList"}

// ParseListPage validates list_by_keyword_and_time_range and returns its
// explicit collection. An explicit [] is success; an absent/null/wrongly typed
// collection is a contract error.
func ParseListPage(data map[string]any) (Page, error) {
	if err := validateEnvelope(data); err != nil {
		return Page{}, err
	}
	containers := collectionContainers(data)
	for _, container := range containers {
		for _, key := range listKeys {
			value, exists := container[key]
			if !exists {
				continue
			}
			items, err := mapItems(value, key)
			if err != nil {
				return Page{}, err
			}
			hasMore, known := boolField(container, "hasMore", "hasNext")
			if !known {
				hasMore, known = boolField(data, "hasMore", "hasNext")
			}
			next := stringField(container, "nextToken", "nextCursor", "pageToken")
			if next == "" {
				next = stringField(data, "nextToken", "nextCursor", "pageToken")
			}
			if hasMore && next == "" {
				return Page{}, fmt.Errorf("minutes list declares another page but omits nextToken")
			}
			return Page{Items: items, HasMore: hasMore, HasMoreKnown: known, NextToken: next}, nil
		}
	}
	return Page{}, fmt.Errorf("minutes list response has no recognized collection (want result.itemList)")
}

// ParseTranscriptPage validates get_minutes_transcription. The real response
// collection is result.paragraphList and pagination uses hasNext/nextToken.
func ParseTranscriptPage(data map[string]any) (Page, error) {
	if err := validateEnvelope(data); err != nil {
		return Page{}, err
	}
	containers := collectionContainers(data)
	for _, container := range containers {
		value, exists := container["paragraphList"]
		if !exists {
			continue
		}
		items, err := mapItems(value, "paragraphList")
		if err != nil {
			return Page{}, err
		}
		hasMore, known := boolField(container, "hasNext", "hasMore")
		if !known {
			hasMore, known = boolField(data, "hasNext", "hasMore")
		}
		next := stringField(container, "nextToken", "nextCursor")
		if next == "" {
			next = stringField(data, "nextToken", "nextCursor")
		}
		if hasMore && next == "" {
			return Page{}, fmt.Errorf("minutes transcript declares another page but omits nextToken")
		}
		return Page{Items: items, HasMore: hasMore, HasMoreKnown: known, NextToken: next}, nil
	}
	return Page{}, fmt.Errorf("minutes transcript response has no result.paragraphList collection")
}

// ValidateArtifact rejects successful-looking but structurally empty artifact
// responses. This is intentionally artifact-specific: a generic {} cannot
// prove that the requested Minutes business data was returned.
func ValidateArtifact(name, taskUUID string, data map[string]any) error {
	if err := validateEnvelope(data); err != nil {
		return err
	}
	result, ok := data["result"].(map[string]any)
	if !ok {
		return fmt.Errorf("minutes %s response has no result object", name)
	}
	switch name {
	case "basic":
		actual := TaskUUID(result)
		if actual == "" {
			return fmt.Errorf("minutes basic response has no task UUID")
		}
		if taskUUID != "" && actual != taskUUID {
			return fmt.Errorf("minutes basic response task UUID mismatch")
		}
	case "summary":
		if _, exists := result["fullSummary"]; !exists {
			return fmt.Errorf("minutes summary response has no fullSummary field")
		}
	case "keywords":
		if _, err := mapSliceField(result, "keywords"); err != nil {
			return fmt.Errorf("minutes keywords response: %w", err)
		}
	case "todos":
		if _, actionsErr := mapSliceField(result, "actions"); actionsErr != nil {
			if _, todosErr := mapSliceField(result, "dingtalkTodoList"); todosErr != nil {
				return fmt.Errorf("minutes todos response has neither actions nor dingtalkTodoList array")
			}
		}
	default:
		if len(result) == 0 {
			return fmt.Errorf("minutes %s response result is empty", name)
		}
	}
	return nil
}

// ValidateEnvelope exposes the common backend failure check for composite
// write/read steps that apply their own operation-specific shape validation.
func ValidateEnvelope(data map[string]any) error {
	return validateEnvelope(data)
}

// Basic returns the validated basic-info result.
func Basic(taskUUID string, data map[string]any) (map[string]any, error) {
	if err := ValidateArtifact("basic", taskUUID, data); err != nil {
		return nil, err
	}
	return data["result"].(map[string]any), nil
}

// SummaryText returns the full summary, including an explicitly empty string.
func SummaryText(data map[string]any) (string, error) {
	if err := ValidateArtifact("summary", "", data); err != nil {
		return "", err
	}
	value, ok := data["result"].(map[string]any)["fullSummary"].(string)
	if !ok {
		return "", fmt.Errorf("minutes summary fullSummary has type %T, want string", data["result"].(map[string]any)["fullSummary"])
	}
	return value, nil
}

// MediaURL returns the actual audio/video download URL. An empty result is not
// accepted as "no media" because it cannot distinguish processing, expiry and
// permission failures.
func MediaURL(data map[string]any) (string, error) {
	if err := validateEnvelope(data); err != nil {
		return "", err
	}
	result, ok := data["result"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("minutes media response has no result object")
	}
	value := stringField(result, "videoUrl", "audioUrl", "downloadUrl", "url")
	if value == "" {
		return "", fmt.Errorf("minutes media response has no audio/video URL")
	}
	return value, nil
}

// UploadSession extracts the two facts required to continue a local upload.
func UploadSession(data map[string]any) (sessionID, presignedURL string, err error) {
	if err := validateEnvelope(data); err != nil {
		return "", "", err
	}
	result, ok := data["result"].(map[string]any)
	if !ok {
		return "", "", fmt.Errorf("minutes upload create response has no result object")
	}
	sessionID = stringField(result, "sessionId", "uploadId")
	presignedURL = stringField(result, "presignedUrl", "uploadUrl")
	if sessionID == "" || presignedURL == "" {
		return "", "", fmt.Errorf("minutes upload create response is missing sessionId or presignedUrl")
	}
	return sessionID, presignedURL, nil
}

// CompletedTaskUUID extracts the created Minutes task UUID from upload complete.
func CompletedTaskUUID(data map[string]any) (string, error) {
	if err := validateEnvelope(data); err != nil {
		return "", err
	}
	containers := collectionContainers(data)
	for _, container := range containers {
		if uuid := TaskUUID(container); uuid != "" {
			return uuid, nil
		}
	}
	return "", fmt.Errorf("minutes upload complete response has no task UUID")
}

// ProjectList converts validated list items into the stable Shortcut view.
// Every row must carry a real task UUID; malformed rows fail the entire page
// instead of disappearing and masquerading as an empty result.
func ProjectList(page Page) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(page.Items))
	for index, item := range page.Items {
		uuid := TaskUUID(item)
		if uuid == "" {
			return nil, fmt.Errorf("minutes list item %d has no task UUID", index)
		}
		row := map[string]any{"taskUuid": uuid}
		copyFirst(row, "title", item, "title", "name")
		copyFirst(row, "creator", item, "creator", "creatorName", "createUserName", "creatorNick")
		copyFirst(row, "startTime", item, "startTime", "gmtStart", "beginTime", "createTime")
		copyFirst(row, "endTime", item, "endTime", "gmtEnd", "deadline")
		copyFirst(row, "url", item, "url", "shareUrl", "link")
		copyFirst(row, "status", item, "status", "taskStatus", "state")
		out = append(out, row)
	}
	return out, nil
}

// TaskUUID returns only spellings known to identify a Minutes task. A generic
// id/minutesId is deliberately not accepted because record-control consumes
// this value directly as the task UUID.
func TaskUUID(item map[string]any) string {
	return stringField(item, "taskUuid", "taskUUID", "task_uuid", "uuid")
}

// LatestTaskUUID selects the newest item by an explicit comparable timestamp.
// It never silently assumes that an undocumented response order is newest-first.
func LatestTaskUUID(page Page) (string, error) {
	if len(page.Items) == 0 {
		return "", nil
	}
	bestUUID := ""
	var bestTime float64
	haveTime := false
	for index, item := range page.Items {
		uuid := TaskUUID(item)
		if uuid == "" {
			return "", fmt.Errorf("minutes list item %d has no task UUID", index)
		}
		value, ok := timestamp(item, "createTime", "gmtCreate", "startTime", "createTimeStart")
		if !ok {
			continue
		}
		if !haveTime || value > bestTime {
			haveTime = true
			bestTime = value
			bestUUID = uuid
		}
	}
	if !haveTime {
		return "", fmt.Errorf("minutes list is non-empty but no item has a comparable creation/start time")
	}
	return bestUUID, nil
}

// StableItemKey returns a deterministic de-duplication key for a transcript
// paragraph. Stable IDs win; otherwise the canonical JSON object is used.
func StableItemKey(item map[string]any) (string, error) {
	if id := stringField(item, "paragraphId", "sentenceId", "segmentId", "id"); id != "" {
		return "id:" + id, nil
	}
	raw, err := json.Marshal(item)
	if err != nil {
		return "", fmt.Errorf("encode transcript paragraph key: %w", err)
	}
	return "json:" + string(raw), nil
}

func validateEnvelope(data map[string]any) error {
	if len(data) == 0 {
		return fmt.Errorf("minutes response is empty")
	}
	if success, ok := data["success"].(bool); ok && !success {
		return fmt.Errorf("minutes backend reported failure: %s", envelopeMessage(data))
	}
	for _, key := range []string{"errorCode", "dingOpenErrcode"} {
		value, exists := data[key]
		if !exists || isZeroCode(value) {
			continue
		}
		return fmt.Errorf("minutes backend reported %s=%v: %s", key, value, envelopeMessage(data))
	}
	return nil
}

func collectionContainers(data map[string]any) []map[string]any {
	containers := []map[string]any{data}
	for _, key := range []string{"result", "data"} {
		if inner, ok := data[key].(map[string]any); ok {
			containers = append(containers, inner)
			for _, nestedKey := range []string{"result", "data"} {
				if nested, ok := inner[nestedKey].(map[string]any); ok {
					containers = append(containers, nested)
				}
			}
		}
	}
	return containers
}

func mapItems(value any, field string) ([]map[string]any, error) {
	raw, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("minutes response field %s has type %T, want array", field, value)
	}
	items := make([]map[string]any, 0, len(raw))
	for index, value := range raw {
		item, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("minutes response field %s item %d has type %T, want object", field, index, value)
		}
		items = append(items, item)
	}
	return items, nil
}

func mapSliceField(data map[string]any, field string) ([]any, error) {
	value, exists := data[field]
	if !exists {
		return nil, fmt.Errorf("missing %s", field)
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("field %s has type %T, want array", field, value)
	}
	return items, nil
}

func copyFirst(dst map[string]any, dstKey string, src map[string]any, keys ...string) {
	for _, key := range keys {
		if value, ok := src[key]; ok && value != nil {
			dst[dstKey] = value
			return
		}
	}
}

func boolField(data map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		if value, ok := data[key].(bool); ok {
			return value, true
		}
	}
	return false, false
}

func stringField(data map[string]any, keys ...string) string {
	for _, key := range keys {
		value, exists := data[key]
		if !exists || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case json.Number:
			return typed.String()
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64)
		case int:
			return strconv.Itoa(typed)
		case int64:
			return strconv.FormatInt(typed, 10)
		}
	}
	return ""
}

func timestamp(data map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		value, exists := data[key]
		if !exists || value == nil {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return typed, true
		case int:
			return float64(typed), true
		case int64:
			return float64(typed), true
		case json.Number:
			if parsed, err := typed.Float64(); err == nil {
				return parsed, true
			}
		case string:
			if parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64); err == nil {
				return parsed, true
			}
			if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(typed)); err == nil {
				return float64(parsed.UnixNano()), true
			}
		}
	}
	return 0, false
}

func isZeroCode(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		trimmed := strings.TrimSpace(typed)
		return trimmed == "" || trimmed == "0"
	case float64:
		return typed == 0
	case int:
		return typed == 0
	case int64:
		return typed == 0
	case json.Number:
		return typed.String() == "0"
	default:
		return false
	}
}

func envelopeMessage(data map[string]any) string {
	for _, key := range []string{"errorMsg", "message", "msg"} {
		if value := stringField(data, key); value != "" {
			return value
		}
	}
	return "no backend error message"
}
