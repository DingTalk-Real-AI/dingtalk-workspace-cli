// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package minutesdata

import (
	"errors"
	"fmt"
	"testing"
)

func TestCrossPlatformCoverageTranscriptCollectsPagesAndDeduplicatesE2E(t *testing.T) {
	calls := []string{}
	result, err := CollectTranscript(func(token string) (map[string]any, error) {
		calls = append(calls, token)
		if token == "" {
			return decode(t, `{"result":{"paragraphList":[{"paragraphId":"p1"},{"paragraphId":"p2"}],"hasNext":true,"nextToken":"n2"}}`), nil
		}
		return decode(t, `{"result":{"paragraphList":[{"paragraphId":"p2"},{"paragraphId":"p3"}],"hasNext":false,"nextToken":""}}`), nil
	}, "", false, 10)
	if err != nil || !result.Complete || result.Pages != 2 || len(result.Paragraphs) != 3 || result.Duplicates != 1 || fmt.Sprint(calls) != "[ n2]" {
		t.Fatalf("collect = %#v calls=%v err=%v", result, calls, err)
	}
}

func TestCrossPlatformCoverageTranscriptPartialReadNeverBecomesSuccessE2E(t *testing.T) {
	result, err := CollectTranscript(func(token string) (map[string]any, error) {
		if token == "" {
			return decode(t, `{"result":{"paragraphList":[{"paragraphId":"p1"}],"hasNext":true,"nextToken":"n2"}}`), nil
		}
		return nil, errors.New("backend failed")
	}, "", false, 10)
	if err == nil || result.Complete || result.Pages != 1 || len(result.Paragraphs) != 1 || result.FailurePage != 2 {
		t.Fatalf("partial read = %#v err=%v", result, err)
	}
}

func TestCrossPlatformCoverageTranscriptCursorCycleAndSinglePageE2E(t *testing.T) {
	cycle, err := CollectTranscript(func(string) (map[string]any, error) {
		return decode(t, `{"result":{"paragraphList":[],"hasNext":true,"nextToken":"same"}}`), nil
	}, "same", false, 10)
	if err == nil || cycle.Complete {
		t.Fatalf("cursor cycle = %#v err=%v", cycle, err)
	}
	one, err := CollectTranscript(func(string) (map[string]any, error) {
		return decode(t, `{"result":{"paragraphList":[{"paragraphId":"p1"}],"hasNext":true,"nextToken":"n2"}}`), nil
	}, "", true, 10)
	if err != nil || one.Complete || one.Pages != 1 || one.NextToken != "n2" {
		t.Fatalf("single page = %#v err=%v", one, err)
	}
}
