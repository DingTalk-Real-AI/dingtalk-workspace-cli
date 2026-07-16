package helpers

import (
	"errors"
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
)

type invalidIntFlagValue struct{}

func (invalidIntFlagValue) Set(string) error { return nil }
func (invalidIntFlagValue) String() string   { return "not-an-int" }
func (invalidIntFlagValue) Type() string     { return "int" }

func filterViewHandlerCommand(column bool) *cobra.Command {
	cmd := &cobra.Command{Use: "handler"}
	cmd.Flags().String("node", "node", "")
	cmd.Flags().String("sheet-id", "sheet", "")
	cmd.Flags().String("filter-view-id", "one", "")
	if column {
		cmd.Flags().Int("column", 0, "")
	}
	return cmd
}

func executeFilterCoverage(t *testing.T, root *cobra.Command, args ...string) error {
	t.Helper()
	oldArgs := os.Args
	os.Args = []string{"dws", "sheet"}
	t.Cleanup(func() { os.Args = oldArgs })
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	return root.Execute()
}

func findCoverageSubcommand(t *testing.T, root *cobra.Command, name string) *cobra.Command {
	t.Helper()
	for _, child := range root.Commands() {
		if child.Name() == name {
			return child
		}
	}
	t.Fatalf("subcommand %q not found", name)
	return nil
}

func TestCrossPlatformCoverageFilterViewFetchAndHandlerRemainingCoverage(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"dws", "sheet"}
	t.Cleanup(func() { os.Args = oldArgs })

	for _, step := range []scriptedToolStep{{err: errors.New("fetch")}, {text: ""}, {text: "{"}} {
		installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{step}})
		if _, err := fetchFilterViews("node", "sheet"); err == nil {
			t.Fatalf("fetch step %#v returned nil", step)
		}
	}
	installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: `{}`}}})
	if views, err := fetchFilterViews("node", "sheet"); err != nil || views != nil {
		t.Fatalf("missing views = %#v, %v", views, err)
	}
	payload := `{"result":{"filterViews":[1,{"id":"one","criteria":{"0":{"filterType":"values"}}},{"filterViewId":"two"}]}}`
	installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: payload}}})
	views, err := fetchFilterViews("node", "sheet")
	if err != nil || len(views) != 2 {
		t.Fatalf("views = %#v, %v", views, err)
	}

	installScriptedCaller(t, &scriptedToolCaller{dry: true})
	if err := runFilterViewInfo(filterViewHandlerCommand(false), nil); err != nil {
		t.Fatal(err)
	}
	if err := runFilterViewListCriteria(filterViewHandlerCommand(false), nil); err != nil {
		t.Fatal(err)
	}
	if err := runFilterViewGetCriteria(filterViewHandlerCommand(true), nil); err != nil {
		t.Fatal(err)
	}

	installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: payload}}})
	if err := runFilterViewInfo(filterViewHandlerCommand(false), nil); err != nil {
		t.Fatalf("info: %v", err)
	}
	if err := runFilterViewListCriteria(filterViewHandlerCommand(false), nil); err != nil {
		t.Fatalf("list criteria: %v", err)
	}
	if err := runFilterViewGetCriteria(filterViewHandlerCommand(true), nil); err != nil {
		t.Fatalf("get criteria: %v", err)
	}

	noCriteria := `{"filterViews":[{"id":"one"}]}`
	installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: noCriteria}}})
	if err := runFilterViewListCriteria(filterViewHandlerCommand(false), nil); err != nil {
		t.Fatalf("empty criteria list: %v", err)
	}
	if err := runFilterViewGetCriteria(filterViewHandlerCommand(true), nil); err == nil {
		t.Fatal("empty criteria get returned nil")
	}

	missingColumn := `{"filterViews":[{"id":"one","criteria":{"1":{}}}]}`
	installScriptedCaller(t, &scriptedToolCaller{steps: []scriptedToolStep{{text: missingColumn}}})
	if err := runFilterViewGetCriteria(filterViewHandlerCommand(true), nil); err == nil {
		t.Fatal("missing column criteria returned nil")
	}

	negative := filterViewHandlerCommand(true)
	_ = negative.Flags().Set("column", "-1")
	if err := runFilterViewGetCriteria(negative, nil); err == nil {
		t.Fatal("negative column returned nil")
	}
	invalid := filterViewHandlerCommand(true)
	invalid.Flags().Lookup("column").Value = invalidIntFlagValue{}
	if err := runFilterViewGetCriteria(invalid, nil); err == nil {
		t.Fatal("invalid column flag returned nil")
	}
}

func TestCrossPlatformCoverageFilterCommandOptionalArgumentRemainingCoverage(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})
	common := []string{"--node", "node", "--sheet-id", "sheet"}
	if err := executeFilterCoverage(t, newFilterCmd(), append([]string{"create"}, append(common, "--range", "A1:B2", "--criteria", "[]")...)...); err != nil {
		t.Fatalf("filter create criteria: %v", err)
	}
	if err := executeFilterCoverage(t, newFilterCmd(), append([]string{"update"}, append(common, "--criteria", "[]")...)...); err != nil {
		t.Fatalf("filter update criteria: %v", err)
	}

	viewCommon := []string{"--node", "node", "--sheet-id", "sheet"}
	if err := executeFilterCoverage(t, newFilterViewCmd(), append([]string{"create"}, append(viewCommon, "--name", "view", "--range", "A1:B2", "--criteria", "[]")...)...); err != nil {
		t.Fatalf("view create criteria: %v", err)
	}
	withID := append(viewCommon, "--filter-view-id", "view")
	if err := executeFilterCoverage(t, newFilterViewCmd(), append([]string{"update"}, append(withID, "--criteria", "[]")...)...); err != nil {
		t.Fatalf("view update criteria: %v", err)
	}
	if err := executeFilterCoverage(t, newFilterViewCmd(), append([]string{"update-criteria"}, append(withID, "--column", "1", "--filter-criteria", `{}`)...)...); err != nil {
		t.Fatalf("view set criteria: %v", err)
	}

	for _, name := range []string{"update-criteria", "delete-criteria"} {
		cmd := findCoverageSubcommand(t, newFilterViewCmd(), name)
		cmd.Flags().Lookup("column").Value = invalidIntFlagValue{}
		if err := cmd.RunE(cmd, nil); err == nil {
			t.Fatalf("%s invalid column returned nil", name)
		}
	}
}
