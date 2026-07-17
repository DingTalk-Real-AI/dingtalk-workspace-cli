package app

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	upgradepkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/upgrade"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type fakeUpgradeClient struct {
	latest      *upgradepkg.ReleaseInfo
	latestErr   error
	tagged      *upgradepkg.ReleaseInfo
	taggedErr   error
	versions    []upgradepkg.VersionEntry
	versionsErr error
}

func (c *fakeUpgradeClient) FetchLatestReleaseForTrack(upgradepkg.ReleaseTrack) (*upgradepkg.ReleaseInfo, error) {
	return c.latest, c.latestErr
}

func (c *fakeUpgradeClient) FetchReleaseByTag(string) (*upgradepkg.ReleaseInfo, error) {
	return c.tagged, c.taggedErr
}

func (c *fakeUpgradeClient) FetchReleaseVersions(upgradepkg.ReleaseTrack) ([]upgradepkg.VersionEntry, error) {
	return c.versions, c.versionsErr
}

type fakeUpgradeRollback struct {
	backups     []upgradepkg.BackupInfo
	listErr     error
	backupErr   error
	rollbackErr error
	cleaned     bool
}

func (r *fakeUpgradeRollback) ListBackups() ([]upgradepkg.BackupInfo, error) {
	return r.backups, r.listErr
}
func (r *fakeUpgradeRollback) RollbackTo(upgradepkg.BackupInfo) error { return r.rollbackErr }
func (r *fakeUpgradeRollback) Backup(string) (string, error)          { return "backup", r.backupErr }
func (r *fakeUpgradeRollback) Cleanup(int) error {
	r.cleaned = true
	return nil
}

type upgradeFileInfo struct{ size int64 }

func (i upgradeFileInfo) Name() string       { return "dws" }
func (i upgradeFileInfo) Size() int64        { return i.size }
func (i upgradeFileInfo) Mode() os.FileMode  { return 0o755 }
func (i upgradeFileInfo) ModTime() time.Time { return time.Time{} }
func (i upgradeFileInfo) IsDir() bool        { return false }
func (i upgradeFileInfo) Sys() any           { return nil }

func TestCrossPlatformCoverageUpgradeRollbackAndCommandBranchesCoverage(t *testing.T) {
	oldClient, oldRollback := newUpgradeReleaseClient, newUpgradeRollback
	oldEdition := edition.Get()
	oldStdin := os.Stdin
	oldVersion := version
	t.Cleanup(func() {
		newUpgradeReleaseClient, newUpgradeRollback = oldClient, oldRollback
		edition.Override(oldEdition)
		os.Stdin = oldStdin
		version = oldVersion
	})
	fail := errors.New("failure")
	rb := &fakeUpgradeRollback{listErr: fail}
	newUpgradeRollback = func() upgradeRollbackManager { return rb }
	if err := runUpgradeRollback(true); !errors.Is(err, fail) {
		t.Fatalf("rollback list error = %v", err)
	}
	rb.listErr = nil
	if err := runUpgradeRollback(true); err == nil {
		t.Fatal("rollback without backups succeeded")
	}
	rb.backups = []upgradepkg.BackupInfo{{Version: "1.0.0", CreatedAt: time.Now()}}
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(write, "n\n")
	_ = write.Close()
	os.Stdin = read
	if err := runUpgradeRollback(false); err != nil {
		t.Fatalf("cancel rollback = %v", err)
	}
	_ = read.Close()
	rb.rollbackErr = fail
	if err := runUpgradeRollback(true); !errors.Is(err, fail) {
		t.Fatalf("rollback apply error = %v", err)
	}
	rb.rollbackErr = nil
	if err := runUpgradeRollback(true); err != nil {
		t.Fatalf("rollback success = %v", err)
	}

	client := &fakeUpgradeClient{
		latest:   &upgradepkg.ReleaseInfo{Version: "9.9.9"},
		tagged:   &upgradepkg.ReleaseInfo{Version: "9.9.9"},
		versions: []upgradepkg.VersionEntry{{Version: "9.9.9"}, {Version: "9.9.8"}},
	}
	newUpgradeReleaseClient = func() upgradeReleaseClient { return client }
	client.latestErr = fail
	cmd := &cobra.Command{Use: "upgrade"}
	cmd.SetOut(io.Discard)
	if err := runUpgradeCheck(cmd, "json", upgradepkg.ReleaseTrackRelease); !errors.Is(err, fail) {
		t.Fatalf("upgrade check error = %v", err)
	}
	client.latestErr = nil
	client.versionsErr = fail
	if err := runUpgradeList(cmd, "json", 1, upgradepkg.ReleaseTrackRelease); !errors.Is(err, fail) {
		t.Fatalf("upgrade list error = %v", err)
	}
	client.versionsErr = nil
	version = "9.9.9"
	if err := runUpgradeList(cmd, "json", 1, upgradepkg.ReleaseTrackRelease); err != nil {
		t.Fatal(err)
	}
	if err := runUpgradeList(cmd, "table", 1, upgradepkg.ReleaseTrackRelease); err != nil {
		t.Fatal(err)
	}

	edition.Override(&edition.Hooks{IsEmbedded: true})
	embedded := newUpgradeCommand()
	if err := embedded.Execute(); err == nil || !strings.Contains(err.Error(), "embedded") {
		t.Fatalf("unnamed embedded upgrade error = %v", err)
	}
	edition.Override(&edition.Hooks{})
	for _, args := range [][]string{{"--list", "--all"}, {"--rollback", "--yes"}, {"--check"}} {
		command := newUpgradeCommand()
		if args[0] == "--rollback" {
			command.Flags().Bool("yes", false, "")
		}
		command.SetOut(io.Discard)
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Fatalf("upgrade command %v = %v", args, err)
		}
	}
}

func TestCrossPlatformCoverageRunUpgradeAllStagesCoverage(t *testing.T) {
	oldClient, oldRollback := newUpgradeReleaseClient, newUpgradeRollback
	oldEnsure, oldCleanup := ensureUpgradeDirs, cleanupUpgradeStale
	oldNeeds, oldBinary, oldSkills, oldChecksums := upgradeNeedsUpgrade, findUpgradeBinary, findUpgradeSkills, findUpgradeChecksums
	oldDownload, oldProgress := downloadUpgradeFile, downloadUpgradeProgress
	oldExtract, oldFind, oldLocate := extractUpgradeZip, findExtractedBinary, locateUpgradeSkill
	oldReplace, oldInstall := replaceUpgradeSelf, installUpgradeSkills
	oldTemp, oldRemove, oldRead, oldMkdir := upgradeMkdirTemp, upgradeRemoveAll, upgradeReadFile, upgradeMkdirAll
	oldVerify, oldTar, oldValidate := verifyUpgradeFile, extractUpgradeTarGz, validateUpgradeBinary
	oldStdin := os.Stdin
	t.Cleanup(func() {
		newUpgradeReleaseClient, newUpgradeRollback = oldClient, oldRollback
		ensureUpgradeDirs, cleanupUpgradeStale = oldEnsure, oldCleanup
		upgradeNeedsUpgrade, findUpgradeBinary, findUpgradeSkills, findUpgradeChecksums = oldNeeds, oldBinary, oldSkills, oldChecksums
		downloadUpgradeFile, downloadUpgradeProgress = oldDownload, oldProgress
		extractUpgradeZip, findExtractedBinary, locateUpgradeSkill = oldExtract, oldFind, oldLocate
		replaceUpgradeSelf, installUpgradeSkills = oldReplace, oldInstall
		upgradeMkdirTemp, upgradeRemoveAll, upgradeReadFile, upgradeMkdirAll = oldTemp, oldRemove, oldRead, oldMkdir
		verifyUpgradeFile, extractUpgradeTarGz, validateUpgradeBinary = oldVerify, oldTar, oldValidate
		os.Stdin = oldStdin
	})
	fail := errors.New("stage failure")
	binary := upgradepkg.GitHubAsset{Name: "dws.zip", BrowserDownloadURL: "binary"}
	skills := upgradepkg.GitHubAsset{Name: "dws-skills.zip", BrowserDownloadURL: "skills"}
	checksums := upgradepkg.GitHubAsset{Name: "checksums.txt", BrowserDownloadURL: "checksums"}
	release := &upgradepkg.ReleaseInfo{Version: "9.9.9", Date: "2026-01-01", Prerelease: true, Assets: []upgradepkg.GitHubAsset{binary, skills, checksums}}
	client := &fakeUpgradeClient{latest: release, tagged: release}
	rb := &fakeUpgradeRollback{}

	configure := func(stage string) {
		client.latestErr, client.taggedErr = nil, nil
		newUpgradeReleaseClient = func() upgradeReleaseClient { return client }
		newUpgradeRollback = func() upgradeRollbackManager { return rb }
		rb.backupErr, rb.cleaned = nil, false
		ensureUpgradeDirs = func() error {
			if stage == "ensure" {
				return fail
			}
			return nil
		}
		cleanupUpgradeStale = func() {}
		upgradeNeedsUpgrade = func(string, string) bool { return stage != "not-needed" }
		findUpgradeBinary = func([]upgradepkg.GitHubAsset) (*upgradepkg.GitHubAsset, error) {
			if stage == "find-binary" {
				return nil, fail
			}
			asset := binary
			if stage == "extract-tar" {
				asset.Name = "dws.tar.gz"
			}
			return &asset, nil
		}
		findUpgradeSkills = func([]upgradepkg.GitHubAsset) *upgradepkg.GitHubAsset { asset := skills; return &asset }
		findUpgradeChecksums = func([]upgradepkg.GitHubAsset) *upgradepkg.GitHubAsset { asset := checksums; return &asset }
		tempCalls := 0
		upgradeMkdirTemp = func(string, string) (string, error) {
			tempCalls++
			if stage == "temp-both" || (stage == "temp-fallback" && tempCalls == 1) {
				return "", fail
			}
			return "/tmp/dws-upgrade-coverage", nil
		}
		upgradeRemoveAll = func(string) error { return nil }
		if stage == "backup" {
			rb.backupErr = fail
		}
		downloadUpgradeFile = func(url, _ string) (int64, error) {
			if stage == "checksum-download" && url == "checksums" {
				return 0, fail
			}
			if stage == "skills-download" && url == "skills" {
				return 0, fail
			}
			return 4, nil
		}
		upgradeReadFile = func(string) ([]byte, error) {
			if stage == "checksum-read" {
				return nil, fail
			}
			return []byte("checksum"), nil
		}
		downloadUpgradeProgress = func(_ context.Context, _, _ string, progress func(float64, int64, int64)) (int64, error) {
			progress(150, 10, 10)
			if stage == "binary-download" || stage == "temp-fallback" {
				return 0, fail
			}
			return 1024, nil
		}
		verifyCalls := 0
		verifyUpgradeFile = func(string, string, string, string, string) error {
			verifyCalls++
			if stage == "verify-binary" && verifyCalls == 1 || stage == "verify-skills" && verifyCalls == 2 {
				return fail
			}
			return nil
		}
		extractCalls := 0
		extractUpgradeZip = func(string, string) error {
			extractCalls++
			if stage == "extract-binary" && extractCalls == 1 || stage == "extract-skills" && extractCalls == 2 {
				return fail
			}
			return nil
		}
		extractUpgradeTarGz = func(string, string) error {
			if stage == "extract-tar" {
				return fail
			}
			return nil
		}
		findExtractedBinary = func(string) string {
			if stage == "binary-missing" {
				return ""
			}
			return "/tmp/new-dws"
		}
		validateUpgradeBinary = func(string, string) error {
			if stage == "validate" {
				return fail
			}
			return nil
		}
		upgradeMkdirAll = func(string, os.FileMode) error { return nil }
		locateUpgradeSkill = func(string) string {
			if stage == "skill-missing" {
				return ""
			}
			return "/tmp/SKILL.md"
		}
		replaceUpgradeSelf = func(string) error {
			if stage == "replace" {
				return fail
			}
			return nil
		}
		installUpgradeSkills = func(string) (*upgradepkg.SkillUpgradeResult, error) {
			if stage == "install" {
				return nil, fail
			}
			if stage == "install-failed-dir" {
				return &upgradepkg.SkillUpgradeResult{Results: []upgradepkg.SkillDirResult{{Dir: "/failed", Status: upgradepkg.SkillDirFailed, Err: fail}}}, nil
			}
			return &upgradepkg.SkillUpgradeResult{Results: []upgradepkg.SkillDirResult{{Dir: "/ok", Status: upgradepkg.SkillDirOK}}}, nil
		}
	}

	for _, stage := range []string{
		"ensure", "tag-error", "latest-error", "not-needed", "cancel", "find-binary", "temp-fallback", "temp-both",
		"backup", "checksum-download", "checksum-read", "binary-download", "skills-download", "verify-binary", "verify-skills",
		"extract-binary", "extract-tar", "binary-missing", "validate", "extract-skills", "skill-missing", "replace", "install", "install-failed-dir",
		"success", "success-no-skills",
	} {
		t.Run(stage, func(t *testing.T) {
			configure(stage)
			opts := upgradeOptions{force: true, yes: true}
			if stage == "tag-error" {
				opts.targetVersion = "v9.9.9"
				client.taggedErr = fail
			}
			if stage == "latest-error" {
				client.latestErr = fail
			}
			if stage == "not-needed" {
				opts.force = false
			}
			if stage == "cancel" {
				opts.yes = false
				read, write, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				_, _ = io.WriteString(write, "n\n")
				_ = write.Close()
				os.Stdin = read
				defer read.Close()
			}
			if stage == "success-no-skills" {
				opts.skipSkills = true
			}
			err := runUpgrade(context.Background(), opts)
			wantError := stage != "not-needed" && stage != "cancel" && stage != "backup" && stage != "checksum-download" && stage != "checksum-read" && stage != "success" && stage != "success-no-skills"
			if wantError && err == nil {
				t.Fatalf("stage %s succeeded", stage)
			}
			if !wantError && err != nil {
				t.Fatalf("stage %s failed: %v", stage, err)
			}
			if (stage == "success" || stage == "success-no-skills") && !rb.cleaned {
				t.Fatal("successful upgrade did not clean backups")
			}
			if stage == "success" {
				command := newUpgradeCommand()
				command.Flags().Bool("yes", false, "")
				command.Flags().Bool("dry-run", false, "")
				_ = command.Flags().Set("yes", "true")
				_ = command.Flags().Set("force", "true")
				_ = command.Flags().Set("skip-skills", "true")
				if err := command.RunE(command, nil); err != nil {
					t.Fatalf("default upgrade command = %v", err)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageUpgradeBinaryHelpersFailureCoverage(t *testing.T) {
	oldStat, oldChmod := upgradeStat, upgradeChmod
	oldTry, oldRepair := upgradeTryExecVersion, upgradeRepairDarwin
	oldGOOS := upgradeRuntimeGOOS
	oldLook, oldCommand, oldMkdir, oldHome := upgradeLookPath, upgradeCommandOutput, upgradeMkdirAll, upgradeUserHomeDir
	t.Cleanup(func() {
		upgradeStat, upgradeChmod = oldStat, oldChmod
		upgradeTryExecVersion, upgradeRepairDarwin = oldTry, oldRepair
		upgradeRuntimeGOOS = oldGOOS
		upgradeLookPath, upgradeCommandOutput, upgradeMkdirAll, upgradeUserHomeDir = oldLook, oldCommand, oldMkdir, oldHome
	})
	fail := errors.New("failure")
	if _, err := oldCommand(filepath.Join(t.TempDir(), "missing-command")); err == nil {
		t.Fatal("default command runner executed a missing command")
	}
	upgradeStat = func(string) (os.FileInfo, error) { return nil, fail }
	if err := validateNewBinary("binary", "1.0"); !errors.Is(err, fail) {
		t.Fatalf("binary stat error = %v", err)
	}
	upgradeStat = func(string) (os.FileInfo, error) { return upgradeFileInfo{}, nil }
	if err := validateNewBinary("binary", "1.0"); err == nil || !strings.Contains(err.Error(), "为空") {
		t.Fatalf("empty binary error = %v", err)
	}
	upgradeStat = func(string) (os.FileInfo, error) { return upgradeFileInfo{size: 1}, nil }
	upgradeChmod = func(string, os.FileMode) error { return fail }
	if err := validateNewBinary("binary", "1.0"); !errors.Is(err, fail) {
		t.Fatalf("binary chmod error = %v", err)
	}
	upgradeChmod = func(string, os.FileMode) error { return nil }
	upgradeTryExecVersion = func(string) ([]byte, error) { return nil, fail }
	upgradeRepairDarwin = func(string) error { return fail }
	if err := validateNewBinary("binary", "1.0"); err == nil {
		t.Fatal("unexecutable binary succeeded")
	}
	upgradeRuntimeGOOS = "darwin"
	calls := 0
	upgradeTryExecVersion = func(string) ([]byte, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("signal: killed")
		}
		return []byte("version 1.0"), nil
	}
	upgradeRepairDarwin = func(string) error { return nil }
	if err := validateNewBinary("binary", "1.0"); err != nil {
		t.Fatalf("repaired binary = %v", err)
	}
	upgradeRuntimeGOOS = oldGOOS
	upgradeTryExecVersion = func(string) ([]byte, error) { return []byte("different"), nil }
	if err := validateNewBinary("binary", "1.0"); err != nil {
		t.Fatalf("version mismatch warning = %v", err)
	}

	upgradeLookPath = func(string) (string, error) { return "", fail }
	upgradeCommandOutput = func(string, ...string) ([]byte, error) { return nil, nil }
	if err := repairDarwinBinary("binary"); !errors.Is(err, fail) {
		t.Fatalf("codesign lookup error = %v", err)
	}
	upgradeLookPath = func(string) (string, error) { return "/codesign", nil }
	upgradeCommandOutput = func(name string, _ ...string) ([]byte, error) {
		if name == "codesign" {
			return []byte("denied"), fail
		}
		return nil, nil
	}
	if err := repairDarwinBinary("binary"); err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("codesign execution error = %v", err)
	}
	upgradeCommandOutput = func(string, ...string) ([]byte, error) { return nil, nil }
	if err := repairDarwinBinary("binary"); err != nil {
		t.Fatalf("codesign success = %v", err)
	}
	upgradeMkdirAll = func(string, os.FileMode) error { return nil }
	upgradeCommandOutput = func(string, ...string) ([]byte, error) { return []byte("tar failed"), fail }
	if err := extractTarGz("archive", "dest"); err == nil || !strings.Contains(err.Error(), "tar failed") {
		t.Fatalf("tar error = %v", err)
	}
	upgradeUserHomeDir = func() (string, error) { return "", fail }
	if shortenHome("/path") != "/path" {
		t.Fatal("home lookup failure shortened path")
	}
	if got := parseChangelogEntries("abcdef12", 1); len(got) != 0 {
		t.Fatalf("empty changelog entry = %#v", got)
	}
}
