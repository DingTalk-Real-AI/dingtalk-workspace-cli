package clitrack

import (
	"testing"
	"time"
)

func TestBuildFieldsKeepsOrganizationDimension(t *testing.T) {
	tracker := &Tracker{
		noCommandLine: true,
		noCwd:         true,
		extraFields: func() map[string]string {
			return map[string]string{"c9": "version", "c10": "corp-1"}
		},
	}

	fields := tracker.buildFields(0, time.Millisecond, "", "")
	if fields["c9"] != "version" || fields["c10"] != "corp-1" {
		t.Fatalf("custom telemetry fields = %#v", fields)
	}
}
