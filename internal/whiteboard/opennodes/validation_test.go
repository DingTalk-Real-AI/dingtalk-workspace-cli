package opennodes

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageOpenNodesKnownInvalidFieldsAndGraphs(t *testing.T) {
	for _, tc := range []struct{ nodes, code, path string }{
		{`[{"x":"10"}]`, "invalidValue", "/0/x"},
		{`[{"width":0}]`, "invalidValue", "/0/width"},
		{`[{"height":-1}]`, "invalidValue", "/0/height"},
		{`[{"zIndex":-1}]`, "invalidValue", "/0/zIndex"},
		{`[{"zIndex":0.5}]`, "invalidValue", "/0/zIndex"},
		{`[{"type":"frame","presentationOrder":-1}]`, "invalidValue", "/0/presentationOrder"},
		{`[{"type":"frame","angle":30}]`, "invalidValue", "/0/angle"},
		{`[{"hidden":"true"}]`, "invalidType", "/0/hidden"},
		{`[{"layer":"front"}]`, "invalidValue", "/0/layer"},
		{`[{"id":" "}]`, "invalidValue", "/0/id"},
		{`[{"parentId":12}]`, "invalidValue", "/0/parentId"},
		{`[{"id":"a"},{"id":"a"}]`, "duplicateTemporaryId", "/1/id"},
		{`[{"parentId":"missing"}]`, "referenceNotFound", "/0/parentId"},
		{`[{"id":"a","type":"shape"},{"parentId":"a"}]`, "invalidParent", "/1/parentId"},
		{`[{"id":"a","type":"group","parentId":"b"},{"id":"b","type":"group","parentId":"a"}]`, "parentCycle", "/0/parentId"},
		{`[{"id":"a","type":"group"},{"type":"connector","parentId":"a"}]`, "invalidParent", "/1/parentId"},
		{`[{"type":"connector","start":{"type":"node","nodeRef":{"scope":"document","id":"a"}}}]`, "referenceScopeUnsupported", "/0/start/nodeRef/scope"},
		{`[{"type":"connector","start":{"type":"node","nodeRef":{"scope":"request","id":"a"}}}]`, "referenceNotFound", "/0/start/nodeRef/id"},
		{`[{"id":"a","type":"icon"},{"type":"connector","start":{"type":"node","nodeRef":{"scope":"request","id":"a"}}}]`, "connectorTargetUnsupported", "/1/start/nodeRef/id"},
		{`[{"id":"a","type":"shape","hidden":true},{"type":"connector","start":{"type":"node","nodeRef":{"scope":"request","id":"a"}}}]`, "connectorTargetUnsupported", "/1/start/nodeRef/id"},
		{`[{"id":"a","type":"shape"},{"type":"connector","start":{"type":"node","nodeRef":{"scope":"request","id":"a"}},"end":{"type":"node","nodeRef":{"scope":"request","id":"a"}}}]`, "connectorSelfLoop", "/1/end/nodeRef/id"},
	} {
		t.Run(tc.code+tc.path, func(t *testing.T) {
			_, err := Parse([]byte(`{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":` + tc.nodes + `}`))
			var issue *NodeValidationError
			if !errors.As(err, &issue) || issue.Code != tc.code || issue.Path != "/source/nodes"+tc.path {
				t.Fatalf("want %s %s, got %v", tc.code, tc.path, err)
			}
			if !strings.Contains(err.Error(), issue.Path) {
				t.Fatalf("missing actionable path: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageOpenNodesValidationPreservesValidSource(t *testing.T) {
	input := `{"schemaVersion":"1.0","catalogVersion":"dml-v1","nodes":[
		{"id":"frame","type":"frame","x":-10.5,"y":0,"width":100,"height":50,"angle":0,"presentationOrder":0},
		{"id":"a","type":"shape","parentId":"frame","x":0,"y":0,"zIndex":1,"hidden":false,"layer":"normal"},
		{"id":"b","type":"text","parentId":"frame","x":50,"y":0,"layer":"foreground"},
		{"type":"connector","start":{"type":"node","nodeRef":{"scope":"request","id":"a"}},"end":{"type":"node","nodeRef":{"scope":"request","id":"b"}}},
		{"type":"connector","start":{"type":"point","point":{"x":0,"y":0}},"end":{"type":"point","point":{"x":1,"y":1}}}
	]}`
	source, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	before, _ := CanonicalJSON(source)
	if err := ValidateNodes(source.Nodes); err != nil {
		t.Fatal(err)
	}
	after, _ := CanonicalJSON(source)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("validation changed approved source")
	}
	for _, value := range []any{json.Number("1e999"), math.Inf(1), math.NaN(), nil} {
		if err := ValidateNodes([]map[string]any{{"x": value}}); err == nil {
			t.Fatalf("accepted non-finite/non-numeric value %v", value)
		}
	}
	// A deep acyclic graph must not overflow the Go stack or require quadratic traversal.
	nodes := make([]map[string]any, 10000)
	for index := range nodes {
		nodes[index] = map[string]any{"id": fmt.Sprint(index), "type": "group", "x": 0, "y": 0}
		if index > 0 {
			nodes[index]["parentId"] = fmt.Sprint(index - 1)
		}
	}
	if err := ValidateNodes(nodes); err != nil {
		t.Fatal(err)
	}
}
