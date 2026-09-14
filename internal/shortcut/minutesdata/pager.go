// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package minutesdata

import "fmt"

// TranscriptCaller fetches one transcript page for the supplied nextToken.
// The empty token means the first page.
type TranscriptCaller func(nextToken string) (map[string]any, error)

// TranscriptResult is the complete or partial result of transcript pagination.
type TranscriptResult struct {
	Paragraphs     []map[string]any
	Pages          int
	Duplicates     int
	Complete       bool
	NextToken      string
	FailurePage    int
	FailureMessage string
}

// CollectTranscript follows hasNext/nextToken, de-duplicates paragraphs and
// fails closed on cursor drift. When a later call fails it returns the partial
// paragraphs plus the error so callers can publish an honest recovery ledger.
func CollectTranscript(call TranscriptCaller, initialToken string, singlePage bool, maxPages int) (TranscriptResult, error) {
	if call == nil {
		return TranscriptResult{}, fmt.Errorf("minutes transcript caller is nil")
	}
	if maxPages <= 0 {
		maxPages = 1000
	}
	result := TranscriptResult{Complete: false, NextToken: initialToken}
	seenTokens := map[string]bool{}
	seenItems := map[string]bool{}
	token := initialToken
	for pageIndex := 1; pageIndex <= maxPages; pageIndex++ {
		if seenTokens[token] {
			result.NextToken = token
			result.FailurePage = pageIndex
			result.FailureMessage = "cursor stalled or cycled"
			return result, fmt.Errorf("minutes transcript cursor stalled or cycled at page %d", pageIndex)
		}
		seenTokens[token] = true
		data, err := call(token)
		if err != nil {
			result.NextToken = token
			result.FailurePage = pageIndex
			result.FailureMessage = err.Error()
			return result, fmt.Errorf("fetch minutes transcript page %d: %w", pageIndex, err)
		}
		page, err := ParseTranscriptPage(data)
		if err != nil {
			result.NextToken = token
			result.FailurePage = pageIndex
			result.FailureMessage = err.Error()
			return result, fmt.Errorf("validate minutes transcript page %d: %w", pageIndex, err)
		}
		result.Pages++
		for _, item := range page.Items {
			key, err := StableItemKey(item)
			if err != nil {
				result.FailurePage = pageIndex
				result.FailureMessage = err.Error()
				return result, err
			}
			if seenItems[key] {
				result.Duplicates++
				continue
			}
			seenItems[key] = true
			result.Paragraphs = append(result.Paragraphs, item)
		}
		result.NextToken = page.NextToken
		if singlePage {
			result.Complete = !page.HasMore
			return result, nil
		}
		if !page.HasMore {
			result.Complete = true
			result.NextToken = ""
			return result, nil
		}
		token = page.NextToken
	}
	result.FailurePage = maxPages + 1
	result.FailureMessage = "page safety limit exceeded"
	return result, fmt.Errorf("minutes transcript exceeded page safety limit %d", maxPages)
}

// TranscriptPayload converts a pagination result into the stable public view.
func TranscriptPayload(taskUUID, direction string, result TranscriptResult) map[string]any {
	paragraphs := make([]any, 0, len(result.Paragraphs))
	for _, paragraph := range result.Paragraphs {
		paragraphs = append(paragraphs, paragraph)
	}
	payload := map[string]any{
		"taskUuid":       taskUUID,
		"direction":      direction,
		"complete":       result.Complete,
		"pages":          result.Pages,
		"paragraphCount": len(paragraphs),
		"duplicateCount": result.Duplicates,
		"paragraphList":  paragraphs,
	}
	if result.NextToken != "" {
		payload["nextToken"] = result.NextToken
	}
	if result.FailurePage > 0 {
		payload["failurePage"] = result.FailurePage
	}
	if result.FailureMessage != "" {
		payload["failure"] = result.FailureMessage
	}
	return payload
}
