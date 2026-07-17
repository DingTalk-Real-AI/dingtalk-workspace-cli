package helpers

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageLastPureAndCommandBranches(t *testing.T) {
	if commandBoolFlag(nil, "dry-run") {
		t.Fatal("nil command flag is true")
	}
	SetCmdClassOverride(" ", CmdClassWrite)
	formatCmd := &cobra.Command{Use: "format"}
	formatCmd.Flags().String("content-format", "json", "")
	if err := validateDocFormat(formatCmd, []string{"", "markdown"}, "doc create", "dws doc create"); err == nil {
		t.Fatal("json content format returned nil")
	}
	if sniffJsonMLLike("") {
		t.Fatal("empty content looks like JSONML")
	}
	if err := validateSegment("name", "ab-", 2, 50); err == nil {
		t.Fatal("trailing hyphen name returned nil")
	}
	if got := NormalizeSkillName("Skill --- "); got != "skill" {
		t.Fatalf("normalized skill=%q", got)
	}

	installScriptedCaller(t, &scriptedToolCaller{dry: true})
	aisearch := newAisearchCommand()
	if err := aisearch.RunE(aisearch, nil); err != nil {
		t.Fatalf("aisearch group help: %v", err)
	}

	gate := newConnectGate(nil, nil, 1)
	for i := 0; i < 4097; i++ {
		gate.hits[string(rune(0x1000+i))] = nil
	}
	if ok, reason := gate.allow("sender", "1", "conv"); !ok || reason != "" || len(gate.hits) != 1 {
		t.Fatalf("bounded gate ok=%v reason=%q size=%d", ok, reason, len(gate.hits))
	}

	q := newConvQueue()
	if process := q.processFor("missing"); process != nil {
		t.Fatal("missing queue process is non-nil")
	}

	role := &RoleConfig{ConfirmPolicy: ConfirmManual}
	merged := applyRoleConfig(connectAgentOptions{}, role)
	if merged.ConfirmPolicy != string(ConfirmManual) {
		t.Fatalf("confirm policy=%q", merged.ConfirmPolicy)
	}
	roleDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(roleDir, "bad.yaml"), []byte(":\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRoleConfigs(roleDir); err == nil {
		t.Fatal("invalid role directory returned nil")
	}

	dev := (devHandler{}).Command(executor.EchoRunner{})
	if err := dev.RunE(dev, nil); err != nil {
		t.Fatalf("dev help: %v", err)
	}
	doc := findCoverageSubcommand(t, dev, "doc")
	if err := doc.RunE(doc, nil); err != nil {
		t.Fatalf("dev doc help: %v", err)
	}
}

func TestCrossPlatformCoverageLastIOAndConfirmationBranches(t *testing.T) {
	origOpen, origRead, origRemove := connectLockOpenFile, connectLockReadFile, connectLockRemove
	connectLockOpenFile = func(string, int, os.FileMode) (*os.File, error) { return nil, errors.New("open") }
	if _, err := acquireConnectLock("client"); err == nil || err.Error() != "open" {
		t.Fatalf("non-exist lock err=%v", err)
	}
	connectLockOpenFile = func(string, int, os.FileMode) (*os.File, error) { return nil, os.ErrExist }
	connectLockReadFile = func(string) ([]byte, error) { return nil, errors.New("read") }
	connectLockRemove = func(string) error { return errors.New("remove") }
	if _, err := acquireConnectLock("client"); err == nil || !strings.Contains(err.Error(), "无法获取") {
		t.Fatalf("terminal lock err=%v", err)
	}
	connectLockOpenFile, connectLockReadFile, connectLockRemove = origOpen, origRead, origRemove
	t.Cleanup(func() { connectLockOpenFile, connectLockReadFile, connectLockRemove = origOpen, origRead, origRemove })

	origReadAll, origReadFile := sheetCSVReadAll, sheetCSVReadFile
	sheetCSVReadAll = func(io.Reader) ([]byte, error) { return nil, errors.New("stdin") }
	installScriptedCaller(t, &scriptedToolCaller{dry: true})
	if err := executeFilterCoverage(t, newDataCommandRoot(), "csv-put", "--node", "node", "--sheet-id", "sheet", "--start-cell", "A1", "--csv", "-"); err == nil {
		t.Fatal("CSV stdin failure returned nil")
	}
	sheetCSVReadAll = origReadAll
	csvFile := filepath.Join(t.TempDir(), "data.csv")
	if err := os.WriteFile(csvFile, []byte("a,b"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := executeFilterCoverage(t, newDataCommandRoot(), "csv-put", "--node", "node", "--sheet-id", "sheet", "--start-cell", "A1", "--csv", "@"+csvFile); err != nil {
		t.Fatalf("CSV file input: %v", err)
	}
	sheetCSVReadAll, sheetCSVReadFile = origReadAll, origReadFile
	t.Cleanup(func() { sheetCSVReadAll, sheetCSVReadFile = origReadAll, origReadFile })

	if err := executeFilterCoverage(t, newDataCommandRoot(), "append", "--node", "node", "--sheet-id", "sheet", "--values", "{"); err == nil {
		t.Fatal("invalid append JSON returned nil")
	}
	floatRoot := &cobra.Command{Use: "sheet"}
	floatRoot.AddCommand(newFloatImageCmds()...)
	createFloat := findCoverageSubcommand(t, floatRoot, "create-float-image")
	for _, name := range []string{"node", "sheet-id", "src", "range"} {
		_ = createFloat.Flags().Set(name, "value")
	}
	createFloat.Flags().Lookup("width").Value = invalidIntFlagValue{}
	if err := createFloat.RunE(createFloat, nil); err == nil {
		t.Fatal("invalid float image width returned nil")
	}
	floatRoot = &cobra.Command{Use: "sheet"}
	floatRoot.AddCommand(newFloatImageCmds()...)
	createFloat = findCoverageSubcommand(t, floatRoot, "create-float-image")
	for _, name := range []string{"node", "sheet-id", "src", "range"} {
		_ = createFloat.Flags().Set(name, "value")
	}
	_ = createFloat.Flags().Set("width", "1")
	createFloat.Flags().Lookup("height").Value = invalidIntFlagValue{}
	if err := createFloat.RunE(createFloat, nil); err == nil {
		t.Fatal("invalid float image height returned nil")
	}

	root := newOaCommand()
	approval := findCoverageSubcommand(t, root, "approval")
	revoke := findCoverageSubcommand(t, approval, "revoke")
	_ = revoke.Flags().Set("instance-id", "instance")
	oldArgs, oldStdin := os.Args, os.Stdin
	os.Args = []string{"dws"}
	noPath := filepath.Join(t.TempDir(), "no")
	if err := os.WriteFile(noPath, []byte("no\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdin, err := os.Open(noPath)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = stdin
	if err := revoke.RunE(revoke, nil); err != nil {
		t.Fatalf("cancel revoke: %v", err)
	}
	os.Args, os.Stdin = oldArgs, oldStdin
	_ = stdin.Close()

}

func newDataCommandRoot() *cobra.Command {
	root := &cobra.Command{Use: "sheet"}
	root.AddCommand(newDataCmds()...)
	return root
}
