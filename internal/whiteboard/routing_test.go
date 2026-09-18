// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package whiteboard

import (
	"reflect"
	"testing"
)

func TestCrossPlatformCoverageWhiteboardResolveKindPreservesEmbeddedCompatibility(t *testing.T) {
	tests := []struct {
		name    string
		target  Target
		want    Kind
		wantErr bool
	}{
		{name: "omitted part defaults standalone", target: Target{NodeID: "wb"}, want: KindStandalone},
		{name: "explicit part selects embedded", target: Target{NodeID: "doc", PartID: " part ", PartIDChanged: true}, want: KindEmbedded},
		{name: "explicit empty part fails", target: Target{NodeID: "doc", PartIDChanged: true}, wantErr: true},
		{name: "explicit blank part fails", target: Target{NodeID: "doc", PartID: "  ", PartIDChanged: true}, wantErr: true},
		{name: "blank node fails", target: Target{NodeID: "  "}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveKind(tc.target)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Fatalf("ResolveKind() = %q, %v; want %q err=%v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestCrossPlatformCoverageWhiteboardBuildQueryCallRoutesExactlyOnce(t *testing.T) {
	embedded, err := BuildQueryCall(QueryOptions{Target: Target{NodeID: " doc ", PartID: " part ", PartIDChanged: true}})
	if err != nil {
		t.Fatal(err)
	}
	if embedded.Kind != KindEmbedded || embedded.Tool != EmbeddedQueryTool || !reflect.DeepEqual(embedded.Args, map[string]any{"nodeId": "doc", "partId": "part"}) {
		t.Fatalf("embedded = %#v", embedded)
	}
	standalone, err := BuildQueryCall(QueryOptions{Target: Target{NodeID: "wb"}, View: "all", ViewChanged: true})
	if err != nil {
		t.Fatal(err)
	}
	if standalone.Kind != KindStandalone || standalone.Tool != StandaloneQueryTool || !reflect.DeepEqual(standalone.Args, map[string]any{"nodeId": "wb", "view": "all"}) {
		t.Fatalf("standalone = %#v", standalone)
	}
	page, err := BuildQueryCall(QueryOptions{Target: Target{NodeID: "wb"}, View: "page", ViewChanged: true, PageID: "p1", PageIDChanged: true})
	if err != nil || page.Args["pageId"] != "p1" {
		t.Fatalf("page = %#v err=%v", page, err)
	}
	for _, options := range []QueryOptions{
		{Target: Target{NodeID: "  "}},
		{Target: Target{NodeID: "doc", PartID: "part", PartIDChanged: true}, View: "all", ViewChanged: true},
		{Target: Target{NodeID: "wb"}, ViewChanged: true},
		{Target: Target{NodeID: "wb"}, View: "page", ViewChanged: true},
		{Target: Target{NodeID: "wb"}, PageID: "p", PageIDChanged: true},
		{Target: Target{NodeID: "wb"}, View: "bad", ViewChanged: true},
	} {
		if _, err := BuildQueryCall(options); err == nil {
			t.Fatalf("options %#v unexpectedly succeeded", options)
		}
	}
}

func TestCrossPlatformCoverageWhiteboardBuildUpdateCallPreservesOldArgsAndValidatesStandalone(t *testing.T) {
	embedded, err := BuildUpdateCall(UpdateOptions{
		Target: Target{NodeID: "doc", PartID: "part", PartIDChanged: true},
		Mode:   "append", NodesJSON: `[{"id":"n1"}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantEmbedded := map[string]any{"nodeId": "doc", "partId": "part", "mode": "append", "nodes": `[{"id":"n1"}]`}
	if embedded.Tool != EmbeddedUpdateTool || !reflect.DeepEqual(embedded.Args, wantEmbedded) {
		t.Fatalf("embedded = %#v", embedded)
	}

	standalone, err := BuildUpdateCall(UpdateOptions{
		Target: Target{NodeID: "wb"}, Mode: "overwrite", NodesJSON: "[]",
		PageID: "page", PageIDChanged: true,
		ExpectedRevision: 12, ExpectedRevisionChanged: true,
		RequestID: "req-1", RequestIDChanged: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if standalone.Tool != StandaloneUpdateTool || standalone.Args["expectedRevision"] != 12 || standalone.Args["requestId"] != "req-1" || standalone.Args["pageId"] != "page" {
		t.Fatalf("standalone = %#v", standalone)
	}
	for _, options := range []UpdateOptions{
		{Target: Target{NodeID: "  "}, Mode: "append", NodesJSON: "[]"},
		{Target: Target{NodeID: "wb"}, Mode: "invalid", NodesJSON: "[]"},
		{Target: Target{NodeID: "wb"}, Mode: "append"},
		{Target: Target{NodeID: "wb"}, Mode: "append", NodesJSON: "[]", PageIDChanged: true},
		{Target: Target{NodeID: "doc", PartID: "part", PartIDChanged: true}, Mode: "append", NodesJSON: "[]", ExpectedRevisionChanged: true},
		{Target: Target{NodeID: "doc", PartID: "part", PartIDChanged: true}, Mode: "overwrite", NodesJSON: "[]", PageID: "page", PageIDChanged: true},
		{Target: Target{NodeID: "wb"}, Mode: "append", NodesJSON: "[]", RequestID: "r", RequestIDChanged: true},
		{Target: Target{NodeID: "wb"}, Mode: "append", NodesJSON: "[]", ExpectedRevision: -1, ExpectedRevisionChanged: true},
		{Target: Target{NodeID: "wb"}, Mode: "append", NodesJSON: "[]", ExpectedRevisionChanged: true, RequestIDChanged: true},
		{Target: Target{NodeID: "wb"}, Mode: "append", NodesJSON: "[]", ExpectedRevisionChanged: true, RequestID: "bad space", RequestIDChanged: true},
		{Target: Target{NodeID: "wb"}, Mode: "overwrite", NodesJSON: "[]", ExpectedRevisionChanged: true, RequestID: "r", RequestIDChanged: true},
	} {
		if _, err := BuildUpdateCall(options); err == nil {
			t.Fatalf("options %#v unexpectedly succeeded", options)
		}
	}
	for _, requestID := range []string{"request-1", " request_2 "} {
		if err := ValidateCreateRequestID(requestID); err != nil {
			t.Fatalf("ValidateCreateRequestID(%q): %v", requestID, err)
		}
	}
	for _, requestID := range []string{"", "bad request", string(make([]byte, 129))} {
		if err := ValidateCreateRequestID(requestID); err == nil {
			t.Fatalf("ValidateCreateRequestID(%q) unexpectedly succeeded", requestID)
		}
	}
}
