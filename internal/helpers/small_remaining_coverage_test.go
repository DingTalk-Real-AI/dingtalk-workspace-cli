package helpers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageSheetAndMinutesSmallRemainingBranches(t *testing.T) {
	if err := applyStyleSpec(&styleSpec{FontColorsJSON: "{"}, 1, 1, map[string]any{}); err == nil {
		t.Fatal("invalid font colors JSON returned nil")
	}

	root := &cobra.Command{Use: "sheet"}
	group := &cobra.Command{Use: "range", Run: func(*cobra.Command, []string) {}}
	group.AddCommand(&cobra.Command{Use: "read", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(group)
	attachUnknownSubcommandGuard(root)
	if err := root.RunE(root, []string{"read"}); err == nil || !strings.Contains(err.Error(), "range read") {
		t.Fatalf("deep suggestion err=%v", err)
	}

	installScriptedCaller(t, &scriptedToolCaller{dry: true})
	if err := executeFilterCoverage(t, newMinutesCommand(), "permission", "add", "--ids", "id", "--member-uids", "uid", "--policy", "3", "--cover=false"); err != nil {
		t.Fatalf("explicit false cover: %v", err)
	}
	if err := executeFilterCoverage(t, newMinutesCommand(), "list", "all", "--start", "2030-01-01T10:00:00+08:00", "--end", "2030-01-01T09:00:00+08:00"); err == nil {
		t.Fatal("reversed minutes range returned nil")
	}
}

func TestCrossPlatformCoverageGeminiForwardRemainingFailures(t *testing.T) {
	invalid := &geminiAPIForwarder{baseURL: "%", model: "model", timeout: time.Second}
	if _, err := invalid.forward(context.Background(), "", "text"); err == nil {
		t.Fatal("invalid Gemini endpoint returned nil")
	}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport")
	})}
	f := &geminiAPIForwarder{baseURL: "https://example.test", model: "model", apiKey: "key", timeout: time.Second, httpClient: client}
	if _, err := f.forward(context.Background(), "", "text"); err == nil || !strings.Contains(err.Error(), "transport") {
		t.Fatalf("Gemini transport err=%v", err)
	}
}
