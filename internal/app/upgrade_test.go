// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// --- ensureV ---

func TestEnsureV(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"1.0.6", "v1.0.6"},
		{"v1.0.6", "v1.0.6"},
		{"0.0.1", "v0.0.1"},
		{"dev", "dev"},
		{"unknown", "unknown"},
		{"", "v0.0.0"},
		{"v", "v"},
	}
	for _, tt := range tests {
		got := ensureV(tt.in)
		if got != tt.want {
			t.Errorf("ensureV(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// --- parseChangelogEntries ---

func TestParseChangelogEntries(t *testing.T) {
	body := `## Changelog
* abcdef1234567 - fix login bug
* 0123456789abc Merge branch 'main' into main
* fedcba9876543 - add upgrade command
* deadbeef12345 Merge pull request #42
* 1234567890abc - improve error handling
`
	entries := parseChangelogEntries(body, 10)

	if len(entries) != 3 {
		t.Fatalf("len(entries) = %d, want 3 (merge commits should be filtered)", len(entries))
	}
	if entries[0] != "fix login bug" {
		t.Errorf("entries[0] = %q, want %q", entries[0], "fix login bug")
	}
	if entries[1] != "add upgrade command" {
		t.Errorf("entries[1] = %q, want %q", entries[1], "add upgrade command")
	}
	if entries[2] != "improve error handling" {
		t.Errorf("entries[2] = %q, want %q", entries[2], "improve error handling")
	}
}

func TestParseChangelogEntries_MaxLimit(t *testing.T) {
	body := "* abc1234 - msg1\n* def5678 - msg2\n* ghi9012 - msg3\n"
	entries := parseChangelogEntries(body, 2)
	if len(entries) != 2 {
		t.Errorf("len = %d, want 2 (should respect maxEntries)", len(entries))
	}
}

func TestParseChangelogEntries_EmptyBody(t *testing.T) {
	entries := parseChangelogEntries("", 10)
	if len(entries) != 0 {
		t.Errorf("len = %d, want 0 for empty body", len(entries))
	}
}

func TestParseChangelogEntries_OnlyHeaders(t *testing.T) {
	body := "## Changelog\n## Another heading\n"
	entries := parseChangelogEntries(body, 10)
	if len(entries) != 0 {
		t.Errorf("len = %d, want 0 for headers-only body", len(entries))
	}
}

func TestParseChangelogEntries_OnlyMergeCommits(t *testing.T) {
	body := "* abc1234 Merge branch 'main'\n* def5678 Merge pull request #10\n"
	entries := parseChangelogEntries(body, 10)
	if len(entries) != 0 {
		t.Errorf("len = %d, want 0 (all merge commits should be filtered)", len(entries))
	}
}

func TestParseChangelogEntries_DashPrefixedLines(t *testing.T) {
	body := "- fix bug\n- add feature\n"
	entries := parseChangelogEntries(body, 10)
	if len(entries) != 2 {
		t.Fatalf("len = %d, want 2", len(entries))
	}
	if entries[0] != "fix bug" {
		t.Errorf("entries[0] = %q, want %q", entries[0], "fix bug")
	}
}

// --- stripCommitHash ---

func TestStripCommitHash(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"abcdef1234567 - fix bug", "fix bug"},
		{"abcdef1234567 fix bug", "fix bug"},
		{"short", "short"},   // too short to be a hash
		{"abc123", "abc123"}, // less than 7 hex chars
		{"no hash here", "no hash here"},
		{"ABCDEF1234567 - upper case hash", "upper case hash"},
	}
	for _, tt := range tests {
		got := stripCommitHash(tt.in)
		if got != tt.want {
			t.Errorf("stripCommitHash(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// --- isNoiseCommit ---

func TestIsNoiseCommit(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"Merge branch 'main'", true},
		{"merge branch 'develop'", true},
		{"Merge pull request #42", true},
		{"Merge remote-tracking branch 'origin/main'", true},
		{"fix login bug", false},
		{"add new feature", false},
		{"merge conflicts resolved", false}, // doesn't start with "merge branch"
	}
	for _, tt := range tests {
		got := isNoiseCommit(tt.msg)
		if got != tt.want {
			t.Errorf("isNoiseCommit(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

// --- truncateChangelog ---

func TestTruncateChangelog(t *testing.T) {
	body := "## Changelog\n* abc1234 - fix A\n* def5678 - fix B\n* ghi9012 - fix C\n* jkl3456 - fix D\n"
	result := truncateChangelog(body)
	if result == "" {
		t.Error("truncateChangelog returned empty")
	}
	// Should contain max 3 entries separated by "; "
	parts := strings.Split(result, "; ")
	if len(parts) > 3 {
		t.Errorf("truncateChangelog should have at most 3 entries, got %d", len(parts))
	}
}

func TestTruncateChangelog_EmptyBody(t *testing.T) {
	if got := truncateChangelog(""); got != "" {
		t.Errorf("truncateChangelog('') = %q, want empty", got)
	}
}

// --- truncateChangelogForList ---

func TestTruncateChangelogForList(t *testing.T) {
	tests := []struct {
		body   string
		maxLen int
		want   string
	}{
		{"", 40, "-"},
		{"## Changelog\n", 40, "-"},
	}
	for _, tt := range tests {
		got := truncateChangelogForList(tt.body, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateChangelogForList(%q, %d) = %q, want %q", tt.body, tt.maxLen, got, tt.want)
		}
	}
}

func TestTruncateChangelogForList_Truncation(t *testing.T) {
	body := "* abc1234 - a very long commit message that should be truncated eventually\n"
	result := truncateChangelogForList(body, 20)
	if len(result) > 20 {
		t.Errorf("result len = %d, want <= 20", len(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Errorf("truncated result should end with '...' , got %q", result)
	}
}

// --- progressBar ---

func TestProgressBar(t *testing.T) {
	tests := []struct {
		percent float64
		filled  int
	}{
		{0, 0},
		{50, 10},
		{100, 20},
		{150, 20}, // capped
	}
	for _, tt := range tests {
		bar := progressBar(tt.percent)
		if len(bar) != 20*len("█") && len(bar) != 20*len("░") {
			// Since multi-byte chars, just check total rune count
			runes := []rune(bar)
			if len(runes) != 20 {
				t.Errorf("progressBar(%v) rune count = %d, want 20", tt.percent, len(runes))
			}
		}
		filledCount := strings.Count(bar, "█")
		if filledCount != tt.filled {
			t.Errorf("progressBar(%v) filled = %d, want %d", tt.percent, filledCount, tt.filled)
		}
	}
}

// --- shortenHome ---

func TestShortenHome(t *testing.T) {
	// Non-home path should be unchanged
	got := shortenHome("/tmp/somewhere")
	if got != "/tmp/somewhere" {
		t.Errorf("shortenHome(/tmp/somewhere) = %q", got)
	}
}

// --- resolveUpgradeFormat ---

func TestResolveUpgradeFormat_Default(t *testing.T) {
	root := &cobra.Command{}
	root.PersistentFlags().String("format", "json", "output format")
	child := &cobra.Command{}
	root.AddCommand(child)

	// format not changed => should default to "table" for upgrade
	got := resolveUpgradeFormat(child)
	if got != "table" {
		t.Errorf("resolveUpgradeFormat(unchanged) = %q, want %q", got, "table")
	}
}

func TestResolveUpgradeFormat_ExplicitJSON(t *testing.T) {
	root := &cobra.Command{}
	root.PersistentFlags().String("format", "json", "output format")
	child := &cobra.Command{}
	root.AddCommand(child)

	// Simulate user explicitly setting format
	root.PersistentFlags().Set("format", "json")

	got := resolveUpgradeFormat(child)
	if got != "json" {
		t.Errorf("resolveUpgradeFormat(explicit json) = %q, want %q", got, "json")
	}
}

func TestResolveUpgradeFormat_ExplicitTable(t *testing.T) {
	root := &cobra.Command{}
	root.PersistentFlags().String("format", "json", "output format")
	child := &cobra.Command{}
	root.AddCommand(child)

	root.PersistentFlags().Set("format", "table")

	got := resolveUpgradeFormat(child)
	if got != "table" {
		t.Errorf("resolveUpgradeFormat(explicit table) = %q, want %q", got, "table")
	}
}

// --- writeJSON ---

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{
		"version": "v1.0.6",
		"ok":      true,
	}
	if err := writeJSON(&buf, data); err != nil {
		t.Fatalf("writeJSON() error = %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, `"version": "v1.0.6"`) {
		t.Errorf("output missing version: %s", output)
	}
	if !strings.Contains(output, `"ok": true`) {
		t.Errorf("output missing ok: %s", output)
	}
}

// --- strictVerifyFile ---

func TestStrictVerifyFile_MatchesChecksums(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.tar.gz")
	content := []byte("valid binary content")
	os.WriteFile(filePath, content, 0644)

	hash := computeTestSHA256(t, content)
	checksums := hash + "  test.tar.gz\n"

	err := strictVerifyFile("[1/5]", filePath, "test.tar.gz", "", checksums)
	if err != nil {
		t.Errorf("expected success, got %v", err)
	}
}

func TestStrictVerifyFile_ChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.tar.gz")
	os.WriteFile(filePath, []byte("tampered content"), 0644)

	checksums := "0000000000000000000000000000000000000000000000000000000000000000  test.tar.gz\n"

	err := strictVerifyFile("[1/5]", filePath, "test.tar.gz", "", checksums)
	if err == nil {
		t.Fatal("expected error for checksum mismatch")
	}
	if !strings.Contains(err.Error(), "校验失败") {
		t.Errorf("error = %q, want to contain '校验失败'", err.Error())
	}
}

func TestStrictVerifyFile_DigestMismatch(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.tar.gz")
	os.WriteFile(filePath, []byte("tampered"), 0644)

	err := strictVerifyFile("[1/5]", filePath, "test.tar.gz",
		"sha256:0000000000000000000000000000000000000000000000000000000000000000",
		"")
	if err == nil {
		t.Fatal("expected error for digest mismatch")
	}
}

func TestStrictVerifyFile_NoChecksumInfo(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.tar.gz")
	os.WriteFile(filePath, []byte("content"), 0644)

	err := strictVerifyFile("[1/5]", filePath, "test.tar.gz", "", "")
	if err != nil {
		t.Errorf("no checksum info should skip, not error: %v", err)
	}
}

func TestStrictVerifyFile_FileNotInChecksums_FallsToDigest(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "skills.zip")
	content := []byte("skills content")
	os.WriteFile(filePath, content, 0644)

	hash := computeTestSHA256(t, content)
	// checksums.txt has entries but NOT skills.zip
	checksums := "abcdef1234567890  other-file.tar.gz\n"

	err := strictVerifyFile("[1/5]", filePath, "skills.zip", "sha256:"+hash, checksums)
	if err != nil {
		t.Errorf("should fall through to digest and succeed: %v", err)
	}
}

func computeTestSHA256(t *testing.T, data []byte) string {
	t.Helper()
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// --- newUpgradeCommand ---

func TestNewUpgradeCommand_Flags(t *testing.T) {
	cmd := newUpgradeCommand()

	if cmd.Use != "upgrade" {
		t.Errorf("Use = %q, want upgrade", cmd.Use)
	}

	expectedFlags := []string{"check", "list", "version", "beta", "rollback", "force", "skip-skills"}
	for _, name := range expectedFlags {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("missing flag: --%s", name)
		}
	}
}

func TestNewUpgradeCommand_NoArgs(t *testing.T) {
	cmd := newUpgradeCommand()
	// Simulate passing positional args - should error with cobra.NoArgs
	cmd.SetArgs([]string{"rollback"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for positional args (NoArgs)")
	}
}

func TestNewUpgradeCommand_Help(t *testing.T) {
	cmd := newUpgradeCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--help"})
	cmd.Execute()
	help := buf.String()

	if !strings.Contains(help, "upgrade") {
		t.Error("help should contain 'upgrade'")
	}
	if !strings.Contains(help, "--check") {
		t.Error("help should contain --check")
	}
	if !strings.Contains(help, "--rollback") {
		t.Error("help should contain --rollback")
	}
	if !strings.Contains(help, "--beta") {
		t.Error("help should contain --beta")
	}
	// Regression for #364: --dry-run must be discoverable from upgrade help so
	// users know it is supported (and is now actually honored).
	if !strings.Contains(help, "--dry-run") {
		t.Error("help should advertise --dry-run for upgrade")
	}
}

func TestNewUpgradeCommand_BetaAndVersionAreMutuallyExclusive(t *testing.T) {
	cmd := newUpgradeCommand()
	cmd.SetArgs([]string{"--beta", "--version", "v1.0.8-beta.1"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --beta with --version")
	}
	if !strings.Contains(err.Error(), "--beta") || !strings.Contains(err.Error(), "--version") {
		t.Fatalf("error = %q, want to mention --beta and --version", err.Error())
	}
}

func TestUpgradeTrack(t *testing.T) {
	if got := upgradeTrack(false); got != "release" {
		t.Fatalf("upgradeTrack(false) = %q, want release", got)
	}
	if got := upgradeTrack(true); got != "beta" {
		t.Fatalf("upgradeTrack(true) = %q, want beta", got)
	}
	if got := upgradeHintForTrack("beta"); !strings.Contains(got, "--beta") {
		t.Fatalf("upgradeHintForTrack(beta) = %q, want --beta hint", got)
	}
}

// --- writeDryRunPlan (#364) ---
//
// Regression for #364: `dws upgrade --dry-run` previously performed a real
// upgrade because the flag was silently ignored. The dry-run path must now be
// preview-only — it describes the steps without downloading or replacing
// anything. writeDryRunPlan is the side-effect-free renderer for that preview.

func TestWriteDryRunPlan_PreviewOnly(t *testing.T) {
	var buf bytes.Buffer
	writeDryRunPlan(&buf, "v1.0.30", "dws-darwin-arm64.tar.gz", false)
	out := buf.String()

	if !strings.Contains(out, "dry-run") {
		t.Errorf("output should be marked as dry-run, got:\n%s", out)
	}
	if !strings.Contains(out, "不会下载或修改任何文件") {
		t.Errorf("output should state nothing is downloaded or modified, got:\n%s", out)
	}
	if !strings.Contains(out, "dws-darwin-arm64.tar.gz") {
		t.Errorf("output should name the resolved platform asset, got:\n%s", out)
	}
	// All five steps should be previewed, including the (skipped) replace step.
	for _, step := range []string{"[1/5]", "[2/5]", "[3/5]", "[4/5]", "[5/5]"} {
		if !strings.Contains(out, step) {
			t.Errorf("output missing step %s, got:\n%s", step, out)
		}
	}
}

func TestWriteDryRunPlan_WithSkills(t *testing.T) {
	var buf bytes.Buffer
	writeDryRunPlan(&buf, "v1.0.30", "dws-linux-amd64.tar.gz", true)
	out := buf.String()

	if !strings.Contains(out, "dws-skills.zip") {
		t.Errorf("with skills, output should mention dws-skills.zip, got:\n%s", out)
	}
	if !strings.Contains(out, "安装技能包") {
		t.Errorf("with skills, replace step should mention installing skills, got:\n%s", out)
	}

	// Without skills, neither should appear.
	var buf2 bytes.Buffer
	writeDryRunPlan(&buf2, "v1.0.30", "dws-linux-amd64.tar.gz", false)
	if strings.Contains(buf2.String(), "dws-skills.zip") {
		t.Errorf("without skills, output should not mention dws-skills.zip, got:\n%s", buf2.String())
	}
}

// --- isLikelyAMFIKill ---

func TestIsLikelyAMFIKill(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"signal killed (real Go format)", errors.New("signal: killed"), true},
		{"signal kill variant", errors.New("signal: kill"), true},
		{"unrelated error", errors.New("exit status 1"), false},
		{"file not found", errors.New("no such file or directory"), false},
		{"wrapped killed in middle", errors.New("exec: signal: killed: cleanup"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLikelyAMFIKill(tt.err); got != tt.want {
				t.Errorf("isLikelyAMFIKill(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// --- validateNewBinary self-heal (darwin only) ---
//
// On macOS, an unsigned arm64 binary is SIGKILL'd by amfid. This test verifies
// validateNewBinary recovers via repairDarwinBinary (ad-hoc codesign) and
// successfully re-executes the binary. We use go itself as a stand-in for the
// new dws binary — it's a real signed Mach-O we can strip and re-sign.

func TestValidateNewBinary_RecoversFromUnsignedDarwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("amfid SIGKILL only happens on macOS")
	}
	if _, err := exec.LookPath("codesign"); err != nil {
		t.Skip("codesign not available")
	}

	// Build a fresh dws binary into a temp dir.
	tmpDir := t.TempDir()
	bin := filepath.Join(tmpDir, "dws-test")

	// Locate repo root from this test file's location.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Join(wd, "..", "..")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}

	// Strip signature to reproduce the unsigned state from CI cross-compilation.
	if out, err := exec.Command("codesign", "--remove-signature", bin).CombinedOutput(); err != nil {
		t.Fatalf("strip signature: %v\n%s", err, out)
	}

	// Sanity: confirm direct exec is killed.
	if _, err := tryExecVersion(bin); err == nil {
		t.Skip("unsigned binary executed without amfid kill — likely Intel Mac or SIP disabled")
	}

	// validateNewBinary should self-heal and succeed.
	if err := validateNewBinary(bin, "dev"); err != nil {
		t.Fatalf("validateNewBinary did not recover: %v", err)
	}

	// Verify the binary now has an ad-hoc signature.
	out, _ := exec.Command("codesign", "-dv", bin).CombinedOutput()
	if !strings.Contains(string(out), "Signature=adhoc") {
		t.Errorf("expected adhoc signature, got: %s", out)
	}
}
