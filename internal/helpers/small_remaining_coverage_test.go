package helpers

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageSheetAndMinutesSmallRemainingBranches(t *testing.T) {
	if _, err := buildStyleCells(&styleSpec{FontColorsJSON: "{"}, 1, 1); err == nil {
		t.Fatal("invalid font colors JSON returned nil")
	}

	root := &cobra.Command{Use: "sheet"}
	group := &cobra.Command{Use: "range", Run: func(*cobra.Command, []string) {}}
	group.AddCommand(&cobra.Command{Use: "read", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(group)
	err := cmdutil.NewCommandResolution(
		root,
		"read",
		cmdutil.ResolutionUnknownSubcommand,
		cmdutil.SuggestDescendantSubcommands(root, "read"),
		"",
	).Err()
	var structured *apperrors.Error
	if !errors.As(err, &structured) || structured.Reason != "unknown_subcommand" || !strings.Contains(structured.Hint, "range read") {
		t.Fatalf("deep suggestion err=%#v", err)
	}

	bounded := &cobra.Command{Use: "sheet"}
	for _, name := range []string{"alpha", "beta", "delta", "gamma"} {
		group := &cobra.Command{Use: name}
		group.AddCommand(&cobra.Command{Use: "read", Run: func(*cobra.Command, []string) {}})
		bounded.AddCommand(group)
	}
	err = cmdutil.NewCommandResolution(
		bounded,
		"read",
		cmdutil.ResolutionUnknownSubcommand,
		cmdutil.SuggestDescendantSubcommands(bounded, "read"),
		"",
	).Err()
	if !errors.As(err, &structured) {
		t.Fatalf("bounded deep suggestion error = %T %v", err, err)
	}
	suggestions, ok := structured.Details["suggestions"].([]string)
	if !ok || !slices.Equal(suggestions, []string{"alpha read", "beta read", "delta read"}) {
		t.Fatalf("bounded deep suggestions = %#v", structured.Details["suggestions"])
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
