package app

import (
	"context"
	"errors"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/source"
)

// TestCrossPlatformCoverageNewEventSourceWiresForceRefreshRejectedToken asserts the portal ticket
// source receives a ForceRefreshToken callback that forwards the actual
// rejected token into the app-level compare-and-refresh chain.
func TestCrossPlatformCoverageNewEventSourceWiresForceRefreshRejectedToken(t *testing.T) {
	oldNew, oldRefresh := eventNewDingtalkSource, eventForceRefreshRejected
	t.Cleanup(func() { eventNewDingtalkSource, eventForceRefreshRejected = oldNew, oldRefresh })

	var captured source.Config
	eventNewDingtalkSource = func(cfg source.Config, _ ...source.SourceOption) (*source.DingtalkSource, error) {
		captured = cfg
		return &source.DingtalkSource{}, nil
	}
	var gotDir, gotRejected string
	eventForceRefreshRejected = func(_ context.Context, configDir, rejectedToken string) (string, error) {
		gotDir, gotRejected = configDir, rejectedToken
		return "fresh", nil
	}
	if _, err := newEventSource(context.Background(), "config-dir", "client", "secret", eventStreamTicketOptions{Mode: "custom"}); err != nil {
		t.Fatal(err)
	}
	if captured.PortalTicket == nil || captured.PortalTicket.ForceRefreshToken == nil {
		t.Fatal("ForceRefreshToken not wired into portal ticket config")
	}
	tok, err := captured.PortalTicket.ForceRefreshToken(context.Background(), "rejected-token")
	if err != nil || tok != "fresh" {
		t.Fatalf("force refresh = %q, %v", tok, err)
	}
	if gotDir != "config-dir" || gotRejected != "rejected-token" {
		t.Fatalf("wiring passed dir %q rejected %q", gotDir, gotRejected)
	}

	fail := errors.New("refresh failed")
	eventForceRefreshRejected = func(context.Context, string, string) (string, error) { return "", fail }
	if _, err := captured.PortalTicket.ForceRefreshToken(context.Background(), "x"); !errors.Is(err, fail) {
		t.Fatalf("refresh error = %v", err)
	}
}
