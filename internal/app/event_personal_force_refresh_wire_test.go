package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
)

// TestCrossPlatformCoverageNewPersonalStreamSourceWiresForceRefreshRejectedToken asserts the
// personal stream source receives a ForceRefreshToken callback that forwards
// the rejected token into the app-level compare-and-refresh chain.
func TestCrossPlatformCoverageNewPersonalStreamSourceWiresForceRefreshRejectedToken(t *testing.T) {
	oldAux := personalResolveAuxiliaryAccessToken
	oldRefresh := personalForceRefreshRejectedToken
	t.Cleanup(func() {
		personalResolveAuxiliaryAccessToken = oldAux
		personalForceRefreshRejectedToken = oldRefresh
	})
	personalResolveAuxiliaryAccessToken = func(context.Context, string, string) (string, error) {
		return "old-token", nil
	}
	refreshErr := errors.New("refresh rejected")
	var gotDir, gotRejected string
	personalForceRefreshRejectedToken = func(_ context.Context, configDir, rejectedToken string) (string, error) {
		gotDir, gotRejected = configDir, rejectedToken
		return "", refreshErr
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	src, err := newPersonalStreamSource(context.Background(), personalStreamSourceOptions{
		ConfigDir: "config-dir",
		Identity:  personal.Identity{ClientID: "client", SourceID: "source"},
		TicketURL: srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The 401 ticket response routes the rejected token through the wired
	// ForceRefreshToken; the unknown refresh failure stays fatal.
	if err := src.Start(context.Background(), func(*dwsevent.RawEvent) {}); !errors.Is(err, refreshErr) {
		t.Fatalf("Start() error = %v, want wrapped refresh error", err)
	}
	if gotDir != "config-dir" || gotRejected != "old-token" {
		t.Fatalf("refresh wiring got dir %q rejected %q", gotDir, gotRejected)
	}
}
