package personal

import (
	"encoding/json"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

func TestCrossPlatformCoverageMessageThreadContext(t *testing.T) {
	for _, eventKey := range []string{EventInChat, EventAllGroupChat, EventMention} {
		for _, present := range []bool{false, true} {
			name := eventKey + "/absent"
			if present {
				name = eventKey + "/present"
			}
			t.Run(name, func(t *testing.T) {
				var data map[string]any
				if err := json.Unmarshal([]byte(personalMessageData(eventKey)), &data); err != nil {
					t.Fatal(err)
				}
				body := data["payload"].(map[string]any)["body"].(map[string]any)
				if present {
					body["openConvThreadId"] = "thread-id"
					body["parentConversationId"] = "parent-id"
					body["rootMessageId"] = "root-id"
				}
				raw, _ := json.Marshal(data)
				projected, err := ProjectOutput(transport.Event{EventType: eventKey, Data: string(raw)})
				if err != nil {
					t.Fatal(err)
				}
				encoded, err := json.Marshal(projected)
				if err != nil {
					t.Fatal(err)
				}
				var got map[string]any
				if err := json.Unmarshal(encoded, &got); err != nil {
					t.Fatal(err)
				}
				for key, want := range map[string]string{"thread_id": "thread-id", "parent_conversation_id": "parent-id", "root_message_id": "root-id"} {
					value, exists := got[key]
					if present && value != want {
						t.Errorf("%s = %v, want %s", key, value, want)
					}
					if !present && exists {
						t.Errorf("absent upstream field %s was invented: %v", key, value)
					}
				}
				if got["conversation_id"] != "cid-1" {
					t.Errorf("conversation_id was overwritten: %v", got)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageThreadContextSchema(t *testing.T) {
	schema := outputSchema(EventAllGroupChat)
	properties := schema["properties"].(map[string]any)
	for _, key := range []string{"thread_id", "parent_conversation_id", "root_message_id"} {
		property, ok := properties[key].(map[string]any)
		if !ok || property["type"] != "string" || property["description"] == "" {
			t.Errorf("missing typed/documented %s: %#v", key, properties[key])
		}
	}
}
