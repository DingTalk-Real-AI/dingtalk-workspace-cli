// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package state

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReduceAppendAndDiff(t *testing.T) {
	create := []map[string]any{
		{"version": "v1.0", "createSurface": map[string]any{"surfaceId": "s", "catalogId": "catalog"}},
		{"version": "v1.0", "updateDataModel": map[string]any{"surfaceId": "s", "path": "/", "value": map[string]any{"stream": map[string]any{"body": "hello"}}}},
		{"version": "v1.0", "updateComponents": map[string]any{"surfaceId": "s", "components": []any{map[string]any{"id": "root", "component": "Text", "text": map[string]any{"path": "/stream/body"}}}}},
	}
	base, err := Reduce(Surface{}, create)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := Reduce(base, []map[string]any{{"version": "v0.9.1", "appendDataModel": map[string]any{"surfaceId": "s", "path": "/stream/body", "value": " world"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := getPointer(updated.Data, "/stream/body"); got != "hello world" {
		t.Fatalf("got %#v", got)
	}
	messages, err := Diff(base, updated)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("diff messages = %d", len(messages))
	}
	replayed, err := Reduce(base, messages)
	if err != nil {
		t.Fatal(err)
	}
	if !equalJSON(replayed, updated) {
		t.Fatalf("replay mismatch: %#v != %#v", replayed, updated)
	}
}

func TestReduceRejectsCrossSurfaceAndNonStringAppend(t *testing.T) {
	base := Surface{SurfaceID: "a", Data: map[string]any{"value": 1.0}}
	if _, err := Reduce(base, []map[string]any{{"version": "v1.0", "updateDataModel": map[string]any{"surfaceId": "b", "path": "/", "value": map[string]any{}}}}); err == nil {
		t.Fatal("expected surface mismatch")
	}
	if _, err := Reduce(base, []map[string]any{{"version": "v0.9.1", "appendDataModel": map[string]any{"surfaceId": "a", "path": "/value", "value": "x"}}}); err == nil {
		t.Fatal("expected append type error")
	}
}

func TestDiffPreservesInteractiveBindingsAndUsesLeafUpdates(t *testing.T) {
	components := map[string]map[string]any{
		"root":    {"id": "root", "component": "Column", "children": []any{"body", "comment"}},
		"body":    {"id": "body", "component": "Text", "text": map[string]any{"path": "/content/body"}},
		"comment": {"id": "comment", "component": "TextField", "value": map[string]any{"path": "/form/comment"}},
	}
	previous := Surface{SurfaceID: "s", Components: components, Data: map[string]any{"content": map[string]any{"body": "old"}, "form": map[string]any{"comment": "user typed"}}}
	desired := Surface{SurfaceID: "s", Components: components, Data: map[string]any{"content": map[string]any{"body": "new"}, "form": map[string]any{"comment": "snapshot default"}}}
	messages, err := Diff(previous, desired)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages=%#v", messages)
	}
	body := messages[0]["updateDataModel"].(map[string]any)
	if body["path"] != "/content/body" || body["value"] != "new" {
		t.Fatalf("update=%#v", body)
	}
	replayed, err := Reduce(previous, messages)
	if err != nil {
		t.Fatal(err)
	}
	if got := getPointer(replayed.Data, "/form/comment"); got != "user typed" {
		t.Fatalf("interactive value overwritten: %#v", got)
	}
}

func TestDiffPreservesEverySupportedInteractiveBinding(t *testing.T) {
	for _, componentName := range []string{"CheckBox", "CheckableImageList", "CheckboxListMulti", "ChoicePicker", "ConversationPicker", "DateTimeInput", "ImageUpload", "InputList", "NumberInput", "Rating", "Slider", "Switch", "TextField", "UserPicker"} {
		t.Run(componentName, func(t *testing.T) {
			components := map[string]map[string]any{
				"root":  {"id": "root", "component": "Column", "children": []any{"input"}},
				"input": {"id": "input", "component": componentName, "value": map[string]any{"path": "/form/value"}},
			}
			previous := Surface{SurfaceID: "s", Components: components, Data: map[string]any{"form": map[string]any{"value": "user value"}, "content": map[string]any{"status": "old"}}}
			desired := Surface{SurfaceID: "s", Components: components, Data: map[string]any{"form": map[string]any{"value": "default"}, "content": map[string]any{"status": "new"}}}
			messages, err := Diff(previous, desired)
			if err != nil {
				t.Fatal(err)
			}
			replayed, err := Reduce(previous, messages)
			if err != nil {
				t.Fatal(err)
			}
			if got := getPointer(replayed.Data, "/form/value"); got != "user value" {
				t.Fatalf("interactive value overwritten: %#v", got)
			}
			if got := getPointer(replayed.Data, "/content/status"); got != "new" {
				t.Fatalf("managed value not updated: %#v", got)
			}
		})
	}
}

func TestDiffRejectsUnsupportedRemovals(t *testing.T) {
	previous := Surface{
		SurfaceID:  "s",
		Components: map[string]map[string]any{"root": {"id": "root", "component": "Text", "text": "hello"}, "extra": {"id": "extra", "component": "Text", "text": "remove"}},
		Data:       map[string]any{"content": map[string]any{"body": "hello", "obsolete": true}},
	}
	desired := cloneSurface(previous)
	delete(desired.Components, "extra")
	if _, err := Diff(previous, desired); err == nil || !strings.Contains(err.Error(), "component") {
		t.Fatalf("component removal error=%v", err)
	}
	desired = cloneSurface(previous)
	delete(desired.Data.(map[string]any)["content"].(map[string]any), "obsolete")
	messages, err := Diff(previous, desired)
	if err != nil {
		t.Fatalf("missing snapshot data must preserve the existing path: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("missing snapshot data emitted an update: %#v", messages)
	}
}

func TestReduceUpdatesDataInsideArrays(t *testing.T) {
	base := Surface{SurfaceID: "s", Data: map[string]any{"items": []any{map[string]any{"status": "old"}}}}
	updated, err := Reduce(base, []map[string]any{{"version": "v1.0", "updateDataModel": map[string]any{"surfaceId": "s", "path": "/items/0/status", "value": "new"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := getPointer(updated.Data, "/items/0/status"); got != "new" {
		t.Fatalf("got %#v", got)
	}
	if _, err := Reduce(base, []map[string]any{{"version": "v1.0", "updateDataModel": map[string]any{"surfaceId": "s", "path": "/items/2/status", "value": "new"}}}); err == nil {
		t.Fatal("expected out-of-bounds array update rejection")
	}
	if _, err := Reduce(Surface{SurfaceID: "s", Data: map[string]any{}}, []map[string]any{{"version": "v1.0", "updateDataModel": map[string]any{"surfaceId": "s", "path": "/items/0/status", "value": "new"}}}); err == nil {
		t.Fatal("expected missing array creation rejection")
	}
}

func TestParseSnapshotContract(t *testing.T) {
	raw := `{"version":"v1.0","surfaceId":"s","components":{"root":{"id":"root","component":"Text","text":"ok"}},"dataModel":{"content":{"body":"ok"}}}`
	snapshot, err := ParseSnapshot(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.SurfaceID != "s" || len(snapshot.Components) != 1 {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	for name, invalid := range map[string]string{
		"message batch": `[{"version":"v1.0"}]`,
		"wrong version": `{"version":"v0.9.1","surfaceId":"s","components":{"root":{"id":"root"}},"dataModel":{}}`,
		"unknown field": `{"version":"v1.0","surfaceId":"s","components":{"root":{"id":"root"}},"dataModel":{},"extra":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseSnapshot(strings.NewReader(invalid)); err == nil {
				t.Fatal("expected snapshot parse failure")
			}
		})
	}
}

func TestReducedSurfaceRoundTripsAsSnapshot(t *testing.T) {
	surface, err := Reduce(Surface{}, []map[string]any{
		{"version": "v1.0", "createSurface": map[string]any{"surfaceId": "s", "catalogId": "catalog"}},
		{"version": "v1.0", "updateDataModel": map[string]any{"surfaceId": "s", "path": "/", "value": map[string]any{"content": map[string]any{"status": "ready"}}}},
		{"version": "v1.0", "updateComponents": map[string]any{"surfaceId": "s", "components": []any{map[string]any{"id": "root", "component": "Text", "text": "ready"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(surface)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := ParseSnapshot(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("card show surface must be reusable as snapshot: %v\n%s", err, raw)
	}
	if roundTrip.Version != "v1.0" || !equalJSON(roundTrip, surface) {
		t.Fatalf("round trip mismatch: %#v != %#v", roundTrip, surface)
	}
}
