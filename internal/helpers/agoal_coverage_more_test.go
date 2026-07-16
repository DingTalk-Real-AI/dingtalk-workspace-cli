package helpers

import (
	"errors"
	"testing"
	"time"
)

func TestCrossPlatformCoverageAgoalUpdateRemainingCoverage(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})
	if err := executeFilterCoverage(t, newAgoalCommand(),
		"strategy", "update", "--profile-id", "profile", "--content", `[]`, "--request-id", "request",
	); err != nil {
		t.Fatalf("strategy update: %v", err)
	}
	if err := executeFilterCoverage(t, newAgoalCommand(),
		"contract", "update", "--contract-id", "contract", "--dimensions", `[]`,
		"--request-id", "request", "--audit-config", `{}`, "--objective-template", `{}`,
	); err != nil {
		t.Fatalf("contract update: %v", err)
	}
	if err := executeFilterCoverage(t, newAgoalCommand(),
		"scorecard", "update", "--dept-id", "dept", "--selected-time", "bad", "--id", "card",
		"--tracking-period-type", "MONTHLY", "--content", `[]`,
	); err == nil {
		t.Fatal("invalid scorecard time returned nil")
	}
	if err := executeFilterCoverage(t, newAgoalCommand(),
		"scorecard", "update", "--dept-id", "dept", "--selected-time", "2026-01-01T00:00:00", "--id", "card",
		"--tracking-period-type", "MONTHLY", "--content", `[]`, "--request-id", "request",
	); err != nil {
		t.Fatalf("scorecard update: %v", err)
	}
}

func TestCrossPlatformCoverageAgoalLocationFallbackCoverage(t *testing.T) {
	original := agoalLoadLocation
	agoalLoadLocation = func(string) (*time.Location, error) { return nil, errors.New("zoneinfo unavailable") }
	t.Cleanup(func() { agoalLoadLocation = original })
	if got := shanghaiLocation(); got == nil || got.String() != "CST" {
		t.Fatalf("fallback location = %v", got)
	}
}
