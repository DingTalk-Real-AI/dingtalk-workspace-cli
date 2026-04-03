package output

import (
	"bytes"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"testing"
)

func TestUnwrapAndWrite(t *testing.T) {
	// Simulate the Result
	result := executor.Result{
		Invocation: executor.Invocation{
			Implemented: true,
			Kind:        "compat_invocation",
		},
		Response: map[string]any{
			"endpoint": "https://mcp-gw",
			"content":  map[string]any{},
		},
	}

	var buf bytes.Buffer
	Write(&buf, FormatJSON, result)

	t.Logf("Output: %s", buf.String())

	resultNil := executor.Result{
		Invocation: executor.Invocation{
			Implemented: true,
			Kind:        "compat_invocation",
		},
		Response: map[string]any{
			"endpoint": "https://mcp-gw",
			"content":  nil,
		},
	}
	buf.Reset()
	Write(&buf, FormatJSON, resultNil)
	t.Logf("Output nil: %s", buf.String())
}
