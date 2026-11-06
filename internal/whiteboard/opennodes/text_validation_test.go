package opennodes

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageOpenNodesTextContract(t *testing.T) {
	const runPath = "/source/nodes/0/text/blocks/0/runs/0"
	for _, tc := range []struct{ text, code, path string }{
		{`null`, "invalidType", "/source/nodes/0/text"},
		{`"legacy"`, "invalidType", "/source/nodes/0/text"},
		{`{}`, "invalidValue", "/source/nodes/0/text/blocks"},
		{`{"blocks":[]}`, "invalidValue", "/source/nodes/0/text/blocks"},
		{`{"blocks":[null]}`, "invalidType", "/source/nodes/0/text/blocks/0"},
		{`{"blocks":[{"runs":[{"text":""}]}]}`, "invalidValue", "/source/nodes/0/text/blocks/0/type"},
		{`{"blocks":[{"type":"paragraph","runs":[]}]}`, "invalidValue", "/source/nodes/0/text/blocks/0/runs"},
		{`{"blocks":[{"type":"paragraph","runs":[{"text":""}],"unknown":true}]}`, "unknownField", "/source/nodes/0/text/blocks/0/unknown"},
		{`{"blocks":[{"type":"invalid","runs":[{"text":""}]}]}`, "invalidValue", "/source/nodes/0/text/blocks/0/type"},
		{`{"blocks":[{"type":"paragraph","runs":[{"text":""}]}],"verticalAlign":"middle"}`, "invalidValue", "/source/nodes/0/text/verticalAlign"},
		{`{"blocks":[{"type":"paragraph","runs":[null]}]}`, "invalidType", runPath},
		{`{"blocks":[{"type":"paragraph","runs":[{"text":12}]}]}`, "invalidType", runPath + "/text"},
		{`{"blocks":[{"type":"paragraph","runs":[{"text":"redacted","marks":{"fontColor":"#FFFFFF"}}]}]}`, "unknownField", runPath + "/marks/fontColor"},
		{`{"blocks":[{"type":"paragraph","runs":[{"text":"redacted","fontSize":16}]}]}`, "unknownField", runPath + "/fontSize"},
		{`{"blocks":[{"type":"paragraph","runs":[{"text":"","marks":null}]}]}`, "invalidType", runPath + "/marks"},
		{`{"blocks":[{"type":"paragraph","runs":[{"text":"","marks":{"fontFamily":" "}}]}]}`, "invalidValue", runPath + "/marks/fontFamily"},
		{`{"blocks":[{"type":"paragraph","runs":[{"text":"","marks":{"fontSize":"16"}}]}]}`, "invalidValue", runPath + "/marks/fontSize"},
		{`{"blocks":[{"type":"paragraph","runs":[{"text":"","marks":{"fontSize":0}}]}]}`, "invalidValue", runPath + "/marks/fontSize"},
		{`{"blocks":[{"type":"paragraph","runs":[{"text":"","marks":{"bold":"true"}}]}]}`, "invalidType", runPath + "/marks/bold"},
		{`{"blocks":[{"type":"paragraph","runs":[{"text":"","marks":{"color":null}}]}]}`, "invalidValue", runPath + "/marks/color"},
		{`{"blocks":[{"type":"paragraph","runs":[{"text":"","link":null}]}]}`, "invalidType", runPath + "/link"},
		{`{"blocks":[{"type":"paragraph","runs":[{"text":"","link":{}}]}]}`, "required", runPath + "/link/url"},
		{`{"blocks":[{"type":"paragraph","runs":[{"text":"","link":{"url":"javascript:alert(1)"}}]}]}`, "invalidValue", runPath + "/link/url"},
		{`{"blocks":[{"type":"paragraph","runs":[{"text":"","link":{"url":"123","href":"123"}}]}]}`, "unknownField", runPath + "/link/href"},
		{`{"plainText":"private"}`, "readOnlyField", "/source/nodes/0/text/plainText"},
		{`{"blocks":[{"type":"paragraph","horizontalAlign":"justify","runs":[{"text":""}]}]}`, "invalidValue", "/source/nodes/0/text/blocks/0/horizontalAlign"},
	} {
		t.Run(tc.path+tc.text, func(t *testing.T) {
			nodes := decodeTextTestNodes(t, `[{"text":`+tc.text+`}]`)
			before, _ := json.Marshal(nodes)
			err := ValidateText(nodes)
			var issue *NodeValidationError
			if !errors.As(err, &issue) || issue.Code != tc.code || issue.Path != tc.path {
				t.Fatalf("want %s@%s, got %v", tc.code, tc.path, err)
			}
			after, _ := json.Marshal(nodes)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("validation mutated source")
			}
			// Frame titles use exactly the same rules and issue paths.
			nodes[0] = map[string]any{"title": map[string]any{"text": nodes[0]["text"]}}
			err = ValidateText(nodes)
			if !errors.As(err, &issue) || issue.Path != strings.Replace(tc.path, "/0/text", "/0/title/text", 1) {
				t.Fatalf("title rules diverged: %v", err)
			}
		})
	}
}

func TestCrossPlatformCoverageOpenNodesPaddingAndValidText(t *testing.T) {
	for _, padding := range []string{`0`, `2.5`, `[0,4]`, `-1`, `"4"`, `null`, `[]`, `[1]`, `[1,2,3]`, `[0,-1]`, `[0,"2"]`} {
		nodes := decodeTextTestNodes(t, `[{"text":{"blocks":[{"type":"paragraph","runs":[{"text":"","marks":{"fontSize":16,"bold":false,"color":"#fff","highlight":"rgb(0 0 0 / 50%)","fontFamily":"Arial"},"link":{"url":"123123"}}]}],"padding":`+padding+`,"verticalAlign":"center"}}]`)
		err := ValidateNodes(nodes)
		valid := padding == "0" || padding == "2.5" || padding == "[0,4]"
		if (err == nil) != valid {
			t.Fatalf("padding %s: %v", padding, err)
		}
		if !valid && !strings.Contains(err.Error(), "/text/padding") {
			t.Fatal(err)
		}
	}
	for _, blockType := range []string{"paragraph", "bulletList", "orderedList"} {
		nodes := decodeTextTestNodes(t, `[{"text":{"blocks":[{"type":"`+blockType+`","horizontalAlign":"right","runs":[{"text":""}]}]}}]`)
		if err := ValidateNodes(nodes); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		url   string
		valid bool
	}{
		{"https://example.com", true}, {"HTTP://example.com", true}, {"mailto:a@example.com", true},
		{"tel:123", true}, {"dingtalk://test", true}, {"../relative", true}, {"123123", true},
		{strings.Repeat("a", 2048), true}, {strings.Repeat("a", 2049), false},
		{strings.Repeat("😀", 1024), true}, {strings.Repeat("😀", 1025), false},
		{"data:text/html,test", false}, {"a\n", false}, {"<a>", false}, {"", false}, {"ftp:test", false},
	} {
		if writableTextLink(tc.url) != tc.valid {
			t.Fatalf("link mismatch length=%d", len(tc.url))
		}
	}
}

func decodeTextTestNodes(t *testing.T, input string) []map[string]any {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.UseNumber()
	var nodes []map[string]any
	if err := decoder.Decode(&nodes); err != nil {
		t.Fatal(err)
	}
	return nodes
}
