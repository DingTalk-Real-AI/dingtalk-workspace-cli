package app

import (
	"context"
	"errors"
	"io"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageProfileRemainingCoverage(t *testing.T) {
	oldMigrate := profileEnsureProfilesMigration
	oldLoad := profileLoadProfiles
	oldPrevious := profileUsePrevious
	oldCurrent := profileSetCurrent
	oldInteractive := profileSwitchInteractiveTerminal
	oldTUI := profileSwitchTUIRunner
	oldProgram := profileRunTeaProgram
	t.Cleanup(func() {
		profileEnsureProfilesMigration = oldMigrate
		profileLoadProfiles = oldLoad
		profileUsePrevious = oldPrevious
		profileSetCurrent = oldCurrent
		profileSwitchInteractiveTerminal = oldInteractive
		profileSwitchTUIRunner = oldTUI
		profileRunTeaProgram = oldProgram
	})
	fail := errors.New("failure")

	list := newProfileListCommand()
	_, _, _ = authCoverageRoot(list, "table", false)
	profileEnsureProfilesMigration = func(string) error { return fail }
	if err := list.RunE(list, nil); err == nil {
		t.Fatal("profile-list migration should fail")
	}
	profileEnsureProfilesMigration = func(string) error { return nil }
	profileLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) { return nil, fail }
	if err := list.RunE(list, nil); err == nil {
		t.Fatal("profile-list load should fail")
	}

	selectorCmd := &cobra.Command{}
	addProfileSwitchSelectorFlags(selectorCmd)
	_ = selectorCmd.Flags().Set("name", " ")
	if _, err := profileSwitchSelectorFromCommand(selectorCmd, nil); err == nil {
		t.Fatal("blank profile selector should fail")
	}

	profileSetCurrent = func(string, string) (*authpkg.Profile, error) {
		return &authpkg.Profile{CorpID: "ding", CorpName: "Corp"}, nil
	}
	profileUsePrevious = func(string) (*authpkg.Profile, error) { return nil, fail }
	cmd := &cobra.Command{}
	_, _, _ = authCoverageRoot(cmd, "table", false)
	if err := switchProfileAndWrite(cmd, "cfg", "-", false); err == nil {
		t.Fatal("previous-profile error should fail")
	}
	profileUsePrevious = func(string) (*authpkg.Profile, error) { return &authpkg.Profile{CorpID: "previous"}, nil }
	if err := switchProfileAndWrite(cmd, "cfg", "-", false); err != nil {
		t.Fatal(err)
	}
	jsonCmd := &cobra.Command{}
	_, _, _ = authCoverageRoot(jsonCmd, "json", false)
	profileLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) { return nil, fail }
	if err := switchProfileAndWrite(jsonCmd, "cfg", "ding", false); err == nil {
		t.Fatal("JSON profile load should fail")
	}
	profileLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) {
		return &authpkg.ProfilesConfig{CurrentProfile: "ding"}, nil
	}
	jsonCmd.SetOut(&appFailWriter{err: fail})
	if err := switchProfileAndWrite(jsonCmd, "cfg", "ding", false); err == nil {
		t.Fatal("JSON profile write should fail")
	}

	profileSwitchInteractiveTerminal = func() bool { return true }
	profileEnsureProfilesMigration = func(string) error { return fail }
	if _, err := selectProfileSwitchProfile(cmd, "cfg"); err == nil {
		t.Fatal("selector migration should fail")
	}
	profileEnsureProfilesMigration = func(string) error { return nil }
	profileLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) { return nil, fail }
	if _, err := selectProfileSwitchProfile(cmd, "cfg"); err == nil {
		t.Fatal("selector load should fail")
	}
	profileLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) { return nil, nil }
	if _, err := selectProfileSwitchProfile(cmd, "cfg"); err == nil {
		t.Fatal("empty selector profiles should fail")
	}
	choices := []string{}
	profileSwitchTUIRunner = func(_ *cobra.Command, _ *authpkg.ProfilesConfig, choice string) (string, error) {
		choices = append(choices, choice)
		return choice, nil
	}
	for _, cfg := range []*authpkg.ProfilesConfig{
		{CurrentProfile: "current", PrimaryProfile: "primary", Profiles: []authpkg.Profile{{CorpID: "first"}}},
		{PrimaryProfile: "primary", Profiles: []authpkg.Profile{{CorpID: "first"}}},
		{Profiles: []authpkg.Profile{{CorpID: "first"}}},
	} {
		profileLoadProfiles = func(string) (*authpkg.ProfilesConfig, error) { return cfg, nil }
		if _, err := selectProfileSwitchProfile(cmd, "cfg"); err != nil {
			t.Fatal(err)
		}
	}
	if len(choices) != 3 || choices[0] != "current" || choices[1] != "first" || choices[2] != "first" {
		t.Fatalf("profile choices = %#v", choices)
	}

	tuiCmd := &cobra.Command{}
	tuiCmd.SetContext(context.Background())
	tuiCmd.SetIn(io.NopCloser(&emptyReader{}))
	tuiCmd.SetErr(io.Discard)
	cfg := &authpkg.ProfilesConfig{Profiles: []authpkg.Profile{{CorpID: "ding"}}}
	profileRunTeaProgram = func(*tea.Program) (tea.Model, error) { return nil, tea.ErrInterrupted }
	if _, err := runProfileSwitchTUI(tuiCmd, cfg, "ding"); err == nil {
		t.Fatal("interrupted TUI should fail")
	}
	profileRunTeaProgram = func(*tea.Program) (tea.Model, error) { return nil, fail }
	if _, err := runProfileSwitchTUI(tuiCmd, cfg, "ding"); err == nil {
		t.Fatal("failed TUI should fail")
	}
	profileRunTeaProgram = func(*tea.Program) (tea.Model, error) { return structModel{}, nil }
	if _, err := runProfileSwitchTUI(tuiCmd, cfg, "ding"); err == nil {
		t.Fatal("wrong TUI model should fail")
	}
	for _, model := range []profileSwitchTUIModel{{aborted: true}, {submitted: false}} {
		model := model
		profileRunTeaProgram = func(*tea.Program) (tea.Model, error) { return model, nil }
		if _, err := runProfileSwitchTUI(tuiCmd, cfg, "ding"); err == nil {
			t.Fatal("aborted TUI should fail")
		}
	}
	final := newProfileSwitchTUIModel(cfg, "ding")
	final.submitted = true
	profileRunTeaProgram = func(*tea.Program) (tea.Model, error) { return final, nil }
	if got, err := runProfileSwitchTUI(tuiCmd, cfg, "ding"); err != nil || got != "ding" {
		t.Fatalf("submitted TUI = %q, %v", got, err)
	}

	profiles := make([]authpkg.Profile, 8)
	model := profileSwitchTUIModel{profiles: profiles, selected: 7, offset: 99}
	model.ensureSelectedVisible()
	if model.offset != 3 {
		t.Fatalf("clamped offset = %d", model.offset)
	}
	model.offset = -2
	model.selected = 0
	model.ensureSelectedVisible()
	if model.offset != 0 {
		t.Fatalf("negative offset = %d", model.offset)
	}
}

type emptyReader struct{}

func (*emptyReader) Read([]byte) (int, error) { return 0, io.EOF }

type structModel struct{}

func (structModel) Init() tea.Cmd                       { return nil }
func (structModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return structModel{}, nil }
func (structModel) View() string                        { return "" }
