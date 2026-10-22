package chatmsg

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestCrossPlatformCoverageResourcesExtractV2Images(t *testing.T) {
	for name, body := range map[string]string{
		"markdown":       "前文 ![图]($iwElAqNqcGcDAQTR) 后文",
		"list and quote": "1. ![图](mediaid:$iwElAqNqcGcDAQTR)\n\n> ![图]($iwElAqNqcGcDAQTR)",
		"rich text":      `{"contents":[{"type":"paragraph","content":[{"type":"image","data":{"url":"$iwElAqNqcGcDAQTR"}}]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			resources := Resources(map[string]any{"openMessageId": "msg", "openConversationId": "cid", "content": body})
			if len(resources) != 1 || resources[0]["resourceId"] != "$iwElAqNqcGcDAQTR" || resources[0]["type"] != "mediaId" {
				t.Fatalf("resources=%#v", resources)
			}
			download := resources[0]["download"].(map[string]any)
			arguments := download["arguments"].(map[string]any)
			if download["ready"] != true || arguments["message-id"] != "msg" || arguments["open-conversation-id"] != "cid" {
				t.Fatalf("download=%#v", download)
			}
		})
	}
}

func TestCrossPlatformCoverageResourcesIgnoreNonImageV2Text(t *testing.T) {
	for _, body := range []string{
		"编号 $iwElAqNqcGcDAQTR", "[链接]($iwElAqNqcGcDAQTR)",
		"`![图]($iwElAqNqcGcDAQTR)`", "```\n![图]($iwElAqNqcGcDAQTR)\n```",
		"![图](./$local.png)", "![图](https://example.com/$image)",
		`{"type":"link","data":{"url":"$iwElAqNqcGcDAQTR"}}`,
		"{\"text\":\"```\\n![图]($iwElAqNqcGcDAQTR)\"}",
	} {
		t.Run(body, func(t *testing.T) {
			if resources := Resources(map[string]any{"content": body}); len(resources) != 0 {
				t.Fatalf("resources=%#v", resources)
			}
		})
	}
}

func TestCrossPlatformCoverageV2ImagesKeepQuotedMessageOwnership(t *testing.T) {
	message := map[string]any{
		"openMessageId": "parent", "openConversationId": "parent-cid", "content": "plain",
		"quotedMessage": map[string]any{
			"openMessageId": "child", "openConversationId": "child-cid", "content": "![图]($iwElAqNqcGcDAQTR)",
		},
	}
	if resources := Resources(message); len(resources) != 0 {
		t.Fatalf("parent resources=%#v", resources)
	}
	encoded, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	if resources := Resources(map[string]any{"content": string(encoded)}); len(resources) != 0 {
		t.Fatalf("serialized parent resources=%#v", resources)
	}
	resources := ResourcesDeep(message)
	if len(resources) != 1 {
		t.Fatalf("resources=%#v", resources)
	}
	arguments := resources[0]["download"].(map[string]any)["arguments"].(map[string]any)
	if fmt.Sprint(arguments["message-id"]) != "child" || fmt.Sprint(arguments["open-conversation-id"]) != "child-cid" {
		t.Fatalf("arguments=%#v", arguments)
	}
}
