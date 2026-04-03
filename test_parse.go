package main

import (
	"encoding/json"
	"fmt"
)

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

func main() {
	data := []byte(`{"content":[{"type":"text","text":"{\"summary\":\"...\",\"data\":{\"tableId\":\"abc\"},\"status\":\"success\"}"}],"structuredContent":{"summary":"...","data":{"tableId":"abc"},"status":"success"},"isError":false}`)

	type rawResult struct {
		Content           json.RawMessage `json:"content"`
		StructuredContent map[string]any  `json:"structuredContent"`
		IsError           bool            `json:"isError,omitempty"`
	}

	var raw rawResult
	_ = json.Unmarshal(data, &raw)

	fmt.Printf("raw.Content string: %s\n", string(raw.Content))

	var object map[string]any
	errMap := json.Unmarshal(raw.Content, &object)
	fmt.Printf("errMap: %v\n", errMap)

	var blocks []ContentBlock
	errBlocks := json.Unmarshal(raw.Content, &blocks)
	fmt.Printf("errBlocks: %v, len(blocks): %d\n", errBlocks, len(blocks))
}
