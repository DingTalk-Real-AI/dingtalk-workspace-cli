package app

// `dws dev event` 九条命令的实机 QC：经 NewRootCommand 走真实生产派发路径
// （leaf → runtimeRunner → detectBusinessError → helpers 信封出口）。
// 仅在 DWS_EVENT_LIVE_SMOKE=1 时运行，否则整文件 skip，绝不 t.Parallel。
//
// 读命令（list/get/validate/debug）只读；写链 create → update → publish →
// offline 只有在 DWS_EVENT_LIVE_ALLOW_WRITES=1 且显式注入测试对象时才运行，
// 失败即停在失败点，不再继续后续写操作。

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
)

const (
	deveventQCAppIDEnv       = "DWS_LIVE_APP"
	deveventQCReadKeyEnv     = "DWS_LIVE_EVENT_KEY"
	deveventQCWriteKeyEnv    = "DWS_LIVE_WRITE_EVENT_KEY"
	deveventQCAllowWritesEnv = "DWS_EVENT_LIVE_ALLOW_WRITES"
)

func deveventQCLiveConfigDir(t *testing.T) string {
	t.Helper()
	if os.Getenv("DWS_EVENT_LIVE_SMOKE") == "" {
		t.Skip("live QC only: set DWS_EVENT_LIVE_SMOKE=1")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("resolve home: %v", err)
	}
	configDir := filepath.Join(home, ".dws")
	t.Setenv("DWS_CONFIG_DIR", configDir)
	t.Setenv(keychain.StorageDirEnv, "")
	os.Unsetenv(keychain.StorageDirEnv)
	return configDir
}

func deveventQCClip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "..."
	}
	if s == "" {
		return "<empty stdout>"
	}
	return s
}

// deveventQCWriteDefinitionFiles 落盘 create/update 用的定义文件。受控键
// （scope/status/revision 等）按 devevent.go 预检规则一律不写。
func deveventQCWriteDefinitionFiles(t *testing.T, dir, eventKey string) (string, string) {
	t.Helper()
	schema := map[string]any{"type": "object", "properties": map[string]any{"orderId": map[string]any{"type": "string"}}, "required": []string{"orderId"}}
	sample := map[string]any{"orderId": "c1"}
	base := map[string]any{"eventKey": eventKey, "name": "CLI 事件链路复测", "description": "dev event 命令组 QC", "payloadSchema": schema, "samplePayload": sample}
	v2 := map[string]any{"eventKey": eventKey, "name": "CLI 事件链路复测 V2", "description": "dev event 命令组 QC", "payloadSchema": schema, "samplePayload": sample}
	paths := []string{filepath.Join(dir, "definition.json"), filepath.Join(dir, "definition-v2.json")}
	for path, definition := range map[string]map[string]any{paths[0]: base, paths[1]: v2} {
		raw, err := json.Marshal(definition)
		if err != nil {
			t.Fatalf("marshal definition %s: %v", path, err)
		}
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return paths[0], paths[1]
}

// deveventQCFindRevision 在命令 stdout 的 JSON 信封里递归找 configRevision。
func deveventQCFindRevision(stdout string) (float64, bool) {
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		return 0, false
	}
	var walk func(node any) (float64, bool)
	walk = func(node any) (float64, bool) {
		switch cast := node.(type) {
		case map[string]any:
			if revision, ok := cast["configRevision"].(float64); ok {
				return revision, true
			}
			for _, key := range []string{"data", "content", "result", "draft", "published"} {
				if child, ok := cast[key]; ok {
					if revision, found := walk(child); found {
						return revision, found
					}
				}
			}
		}
		return 0, false
	}
	return walk(envelope)
}

type deveventQCRecord struct {
	Step      string
	Argv      string
	ErrText   string
	Classface string
	Stdout    string
}

// TestDevEventLiveCommandQC 依次实跑九条命令并打印脱敏证据与汇总表。
func TestDevEventLiveCommandQC(t *testing.T) {
	deveventQCLiveConfigDir(t)

	appID := strings.TrimSpace(os.Getenv(deveventQCAppIDEnv))
	readKey := strings.TrimSpace(os.Getenv(deveventQCReadKeyEnv))
	if appID == "" || readKey == "" {
		t.Skipf("live QC requires %s and %s", deveventQCAppIDEnv, deveventQCReadKeyEnv)
	}

	var records []deveventQCRecord
	run := func(step string, args ...string) (string, error) {
		root := NewRootCommand()
		var out, errB bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errB)
		root.SetIn(strings.NewReader(""))
		root.SetArgs(args)
		execErr := root.Execute()

		record := deveventQCRecord{
			Step:    step,
			Argv:    strings.Join(args, " "),
			Stdout:  deveventQCClip(out.String(), 1000),
			ErrText: "<none>",
		}
		t.Logf("=== [%s] dws %s", step, record.Argv)
		if execErr != nil {
			record.ErrText = execErr.Error()
			t.Logf("    err = %v", execErr)
			var typed *apperrors.Error
			if errors.As(execErr, &typed) {
				record.Classface = fmt.Sprintf("category=%s reason=%s operation=%q stage=%q exit=%d retryable=%v",
					typed.Category, typed.Reason, typed.Operation, typed.FailureStage, typed.ExitCode(), typed.Retryable)
				t.Logf("    classified: %s", record.Classface)
				t.Logf("    serverDiag: code=%q hint=%q", typed.ServerDiag.ServerErrorCode, typed.Hint)
			} else {
				t.Logf("    err(+%%v) = %+v", execErr)
			}
		}
		t.Logf("    stdout = %s", record.Stdout)
		if errB.Len() > 0 {
			t.Logf("    stderr = %s", deveventQCClip(errB.String(), 800))
		}
		records = append(records, record)
		return out.String(), execErr
	}

	// ---- 读路径四条 ----
	run("list", "--debug", "dev", "event", "list", "--unified-app-id", appID, "-f", "json")
	getStdout, getErr := run("get", "--debug", "dev", "event", "get", "--unified-app-id", appID, "--event-key", readKey, "-f", "json")
	run("validate", "--debug", "dev", "event", "validate", "--unified-app-id", appID, "--event-key", readKey, "-f", "json")

	// debug 是 ConfirmFirst 写命令：确认门先于参数校验。先按任务原样跑（无
	// --yes，预期 confirmation_required），再加 --yes 跑到必填 --expected-revision
	// 校验，最后用真实或占位修订号触发远端 debug 行为。
	run("debug(bare)", "--debug", "dev", "event", "debug", "--unified-app-id", appID, "--event-key", readKey, "--timeout", "5s", "-f", "json")
	run("debug(yes)", "--debug", "dev", "event", "debug", "--unified-app-id", appID, "--event-key", readKey, "--timeout", "5s", "--yes", "-f", "json")
	revisionLabel := "fallback-1"
	revision := 1
	if parsed, ok := deveventQCFindRevision(getStdout); ok {
		revision = int(parsed)
		revisionLabel = fmt.Sprintf("from-get-%d", revision)
	} else {
		t.Logf("configRevision not parsed from get stdout (getErr=%v); falling back to 1", getErr)
	}
	run("debug(rev:"+revisionLabel+")", "--debug", "dev", "event", "debug", "--unified-app-id", appID, "--event-key", readKey,
		"--expected-revision", strconv.Itoa(revision), "--timeout", "5s", "--yes", "-f", "json")

	if os.Getenv(deveventQCAllowWritesEnv) != "1" {
		t.Logf("write QC skipped; set %s=1 with an isolated test app and event key to enable it", deveventQCAllowWritesEnv)
		return
	}
	writeKey := strings.TrimSpace(os.Getenv(deveventQCWriteKeyEnv))
	if writeKey == "" {
		t.Fatal("write QC requires DWS_LIVE_WRITE_EVENT_KEY when writes are enabled")
	}
	definitionDir := t.TempDir()
	definitionFile, definitionFileV2 := deveventQCWriteDefinitionFiles(t, definitionDir, writeKey)

	// ---- 写链（create → update → publish → offline，失败即停）----
	// 先按任务指定的 eventKey 文件 create（记录本地命名预检结果）；再用合规
	// 变体键触发 create 的远端行为；只有远端 create 成功才继续 update/publish/
	// offline 乐观锁链（1 → 2 → 3）。
	_, createErr := run("create", "--debug", "dev", "event", "create", "--unified-app-id", appID, "--file", definitionFile, "--yes", "-f", "json")
	if createErr != nil {
		t.Logf("write chain stopped at create: %v", createErr)
	} else if _, updateErr := run("update", "--debug", "dev", "event", "update", "--unified-app-id", appID, "--event-key", writeKey, "--file", definitionFileV2, "--expected-revision", "1", "--yes", "-f", "json"); updateErr != nil {
		t.Logf("write chain stopped at update: %v", updateErr)
	} else if _, publishErr := run("publish", "--debug", "dev", "event", "publish", "--unified-app-id", appID, "--event-key", writeKey, "--expected-revision", "2", "--yes", "-f", "json"); publishErr != nil {
		t.Logf("write chain stopped at publish: %v", publishErr)
	} else {
		run("offline", "--debug", "dev", "event", "offline", "--unified-app-id", appID, "--event-key", writeKey, "--expected-revision", "3", "--yes", "-f", "json")
	}

	t.Log("==== dev event live QC summary ====")
	for _, record := range records {
		t.Logf("%-14s | err=%s | %s | stdout=%s",
			record.Step, record.ErrText, record.Classface, record.Stdout)
	}
}
