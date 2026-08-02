package unit_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSkillDocsDoNotRecommendRetiredCommands(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}

	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	skillsDir := filepath.Join(root, "skills")
	retiredCommands := []string{
		"chat file upload",
		"conference start",
		"conference get-id",
		"conference member invite",
		"conference share",
		"dingtalk-conference",
	}
	allowedContext := []string{
		"已下线",
		"下线",
		"不支持",
		"不要",
		"无需",
		"当前 CLI 不支持",
		"兼容提示",
		"不可用",
		"钉钉客户端",
	}

	var violations []string
	err := filepath.WalkDir(skillsDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		rel, _ := filepath.Rel(root, path)
		for i, line := range strings.Split(string(content), "\n") {
			for _, retired := range retiredCommands {
				if !strings.Contains(line, retired) {
					continue
				}
				if hasAny(line, allowedContext) {
					continue
				}
				violations = append(violations, fmt.Sprintf("%s:%d recommends retired command %q: %s", rel, i+1, retired, strings.TrimSpace(line)))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir() error = %v", err)
	}
	if len(violations) > 0 {
		t.Fatalf("skill docs recommend retired commands:\n%s", strings.Join(violations, "\n"))
	}
}

func TestEventSkillUsesFlatOutputContract(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	paths := []string{
		filepath.Join(root, "skills", "multi", "dingtalk-event", "SKILL.md"),
		filepath.Join(root, "skills", "multi", "dingtalk-event", "references", "event-im.md"),
		filepath.Join(root, "skills", "mono", "references", "products", "event.md"),
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(content)
		for _, required := range []string{
			"[event] ready",
			"--flatten",
			"conversation_id",
			"sender_open_dingtalk_id",
			"reader_open_dingtalk_id",
			"recaller_open_dingtalk_id",
			"reaction_name",
			"operation_type",
			"dws chat message download-media",
			"--open-dingtalk-id",
			"user_im_message_receive_o2o_all",
			"user_im_message_receive_group_all",
			"user_im_group_updated",
			"user_im_group_member_added",
			"user_im_group_member_exited",
			"user_im_group_disbanded",
			"operator_open_dingtalk_id",
			"members",
			"open_dingtalk_id",
		} {
			if !strings.Contains(text, required) {
				t.Errorf("%s missing event contract %q", path, required)
			}
		}
		for _, retired := range []string{
			"payload.body.",
			"尚无稳定业务样本",
			"暂无稳定 payload schema",
		} {
			if strings.Contains(text, retired) {
				t.Errorf("%s still documents retired event path %q", path, retired)
			}
		}
	}
}

func TestCrossPlatformCoverageEventSkillPinsSubscriptionRetryOrchestrationContract(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	paths := []string{
		filepath.Join(root, "skills", "multi", "dingtalk-event", "SKILL.md"),
		filepath.Join(root, "skills", "multi", "dingtalk-event", "references", "event-im.md"),
		filepath.Join(root, "skills", "mono", "references", "products", "event.md"),
		filepath.Join(root, "docs", "event-subprocess-contract.md"),
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(content)
		normalizedText := strings.Join(strings.Fields(text), " ")
		for _, required := range []string{
			"16",
			"--profile",
			"Agent/host",
			"0/2/1",
			"retryable=false",
			"max_additional_attempts=0",
			"retryable=true",
			"max_additional_attempts=2",
			"retryable=unknown",
			"max_additional_attempts=1",
			"retry_after_seconds",
			"next_retry_at",
			"in_flight",
			"cooldown",
			"terminal_hold",
			"subscribe_id",
			"trace_id",
		} {
			if !strings.Contains(text, required) {
				t.Errorf("%s missing subscription retry orchestration contract %q", path, required)
			}
		}
		if path == filepath.Join(root, "docs", "event-subprocess-contract.md") {
			for _, required := range []string{
				"not a CLI-enforced persisted total-attempt cap",
				"performs no in-process automatic retry",
				"does not persist or enforce the Agent/host attempt count",
			} {
				if !strings.Contains(normalizedText, required) {
					t.Errorf("%s overstates CLI retry enforcement; missing %q", path, required)
				}
			}
			continue
		}
		for _, required := range []string{
			"不是 CLI 持久化硬总次数上限",
			"进程内不会自动重试",
			"不持久化或计算跨调用的 Agent/host 尝试次数",
		} {
			if !strings.Contains(normalizedText, required) {
				t.Errorf("%s overstates CLI retry enforcement; missing %q", path, required)
			}
		}
	}
}

func TestCrossPlatformCoverageEventSkillDocumentsSubscriptionGuardOperations(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	paths := []string{
		filepath.Join(root, "skills", "multi", "dingtalk-event", "SKILL.md"),
		filepath.Join(root, "skills", "multi", "dingtalk-event", "references", "event-im.md"),
		filepath.Join(root, "skills", "mono", "references", "products", "event.md"),
		filepath.Join(root, "docs", "event-subprocess-contract.md"),
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(content)
		normalizedText := strings.Join(strings.Fields(text), " ")
		for _, required := range []string{
			"~/.dws/events/open/personal_stream/<identity_hash>/personal_subscription_attempts.json",
			"DWS_CONFIG_DIR",
			"personal_subscription_attempts.json",
			"personal_subscription_attempts.lock",
			"0700",
			"0600",
			"24h",
			"1h",
			"terminal_hold",
			"next_retry_at",
		} {
			if !strings.Contains(text, required) {
				t.Errorf("%s missing subscription guard operations %q", path, required)
			}
		}
		if path == filepath.Join(root, "docs", "event-subprocess-contract.md") {
			for _, required := range []string{
				"Delete only",
				"never the lock file",
				"every protection record for that identity",
			} {
				if !strings.Contains(normalizedText, required) {
					t.Errorf("%s missing emergency guard-clear warning %q", path, required)
				}
			}
			continue
		}
		for _, required := range []string{
			"只删除 `personal_subscription_attempts.json`",
			"不要删除 lock 文件",
			"该 identity 的全部保护记录",
		} {
			if !strings.Contains(normalizedText, required) {
				t.Errorf("%s missing emergency guard-clear warning %q", path, required)
			}
		}
	}
}

func TestEventSkillFrontmatterAdvertisesGroupMemberLifecycle(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	paths := []string{
		filepath.Join(root, "skills", "mono", "SKILL.md"),
		filepath.Join(root, "skills", "multi", "dingtalk-event", "SKILL.md"),
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		parts := strings.SplitN(string(content), "---", 3)
		if len(parts) != 3 {
			t.Fatalf("%s missing YAML frontmatter", path)
		}
		frontmatter := parts[1]
		for _, required := range []string{"个人 IM 事件", "群成员加入", "群成员退出"} {
			if !strings.Contains(frontmatter, required) {
				t.Errorf("%s frontmatter missing event discovery trigger %q", path, required)
			}
		}
	}
}

func hasAny(s string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
