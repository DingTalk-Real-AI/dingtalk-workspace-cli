package app

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
)

func TestCrossPlatformCoverageCLIErrorCauseProjection(t *testing.T) {
	err := &helpers.CLIError{Code: "MCP_TOOL_ERROR", Message: "invalid result", Cause: errors.New("dry-run response executed must be present and false"), Details: map[string]any{"commitState": "committed", "verified": false}}
	info := errorInfoFromExecutionError(err)
	raw, marshalErr := json.Marshal(info)
	if marshalErr != nil || !strings.Contains(string(raw), `"cause":"dry-run response executed must be present and false"`) {
		t.Fatalf("cause lost: %s, %v", raw, marshalErr)
	}
	if !reflect.DeepEqual(info.Details, err.Details) {
		t.Fatalf("details lost alongside cause: %#v", info.Details)
	}
	err.Cause = nil
	if errorInfoFromExecutionError(err).Cause != "" {
		t.Fatal("invented cause")
	}
}
