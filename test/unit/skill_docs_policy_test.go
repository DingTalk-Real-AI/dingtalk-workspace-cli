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
		filepath.Join(root, "skills", "multi", "dingtalk-misc", "references", "event.md"),
		filepath.Join(root, "skills", "multi", "dingtalk-misc", "references", "event-im.md"),
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
		filepath.Join(root, "skills", "multi", "dingtalk-misc", "references", "event.md"),
		filepath.Join(root, "skills", "multi", "dingtalk-misc", "references", "event-im.md"),
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
		filepath.Join(root, "skills", "multi", "dingtalk-misc", "references", "event.md"),
		filepath.Join(root, "skills", "multi", "dingtalk-misc", "references", "event-im.md"),
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
		filepath.Join(root, "skills", "multi", "dingtalk-misc", "SKILL.md"),
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

func TestCrossPlatformCoverageDocSkillPinsExactVersionRoutesAndHelpBudget(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	paths := []string{
		filepath.Join(root, "skills", "multi", "dingtalk-doc", "SKILL.md"),
		filepath.Join(root, "skills", "multi", "dingtalk-doc", "references", "doc.md"),
		filepath.Join(root, "skills", "mono", "references", "products", "doc.md"),
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(content)
		for _, required := range []string{
			"dws doc +version-save --node",
			"dws doc +version-list --node",
			"dws doc +version-revert --node",
		} {
			if !strings.Contains(text, required) {
				t.Errorf("%s missing exact version route %q", path, required)
			}
		}
		if strings.Contains(text, "dws doc +history-") {
			t.Errorf("%s recommends a history compatibility route", path)
		}
	}

	rootSkill, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"Help 不参与选路", "unknown flag", "只查一次 shortcut 清单", "禁止试探后缀"} {
		if !strings.Contains(string(rootSkill), required) {
			t.Errorf("Doc root Skill missing Help budget rule %q", required)
		}
	}
}

func TestCrossPlatformCoverageDocSkillPinsTemplateSearchRequiredQuery(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	path := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", "skills", "multi", "dingtalk-doc", "SKILL.md"))
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, route := range []string{
		"dws doc +template-list [--source MY\\|PUBLIC]",
		"dws doc +template-search --query <名称或关键词>",
		"dws doc +create-from-template --template-id <唯一ID>",
	} {
		if !strings.Contains(text, route) {
			t.Errorf("Doc Golden Route does not publish mutually exclusive template route %q", route)
		}
	}
	if strings.Contains(text, "`dws doc +template-search` →") {
		t.Fatal("Doc Golden Route still recommends template-search without --query")
	}
	for _, required := range []string{
		"准备 Help 时，本轮仅查一次",
		"--fields use_when,avoid_when,parameters,constraints,confirmation",
		"禁用产品级/`--all`",
		"Help 不参与选路",
		"禁止靠失败探测门禁",
		"已有或临时文件先暂存到 cwd",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("Doc root Skill missing bounded Schema/input protocol %q", required)
		}
	}
}

func TestCrossPlatformCoverageDocSkillReferenceLoadingBudget(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	paths := []string{
		filepath.Join(root, "skills", "multi", "dingtalk-doc", "SKILL.md"),
		filepath.Join(root, "skills", "mono", "references", "products", "doc.md"),
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(content)
		for _, required := range []string{
			"禁止读取 reference",
			"最多读取一个",
			"复杂 JSONML",
			"append/overwrite",
		} {
			if !strings.Contains(text, required) {
				t.Errorf("%s missing bounded Reference rule %q", path, required)
			}
		}
		if strings.Contains(text, "只在任务命中时读取一个精确 reference") {
			t.Errorf("%s restores task-match-driven Reference loading", path)
		}
	}
}

func TestCrossPlatformCoverageDocumentRoutingContractsStayAligned(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	read := func(parts ...string) string {
		t.Helper()
		content, err := os.ReadFile(filepath.Join(append([]string{root}, parts...)...))
		if err != nil {
			t.Fatal(err)
		}
		return string(content)
	}

	docSkill := read("skills", "multi", "dingtalk-doc", "SKILL.md")
	docImport := read("skills", "multi", "dingtalk-doc", "references", "doc", "doc-import.md")
	docCreate := read("skills", "multi", "dingtalk-doc", "references", "doc", "doc-create.md")
	docUpdate := read("skills", "multi", "dingtalk-doc", "references", "doc", "doc-update.md")
	createWorkflow := read("skills", "multi", "dingtalk-doc", "references", "doc", "style", "doc-create-workflow.md")
	updateWorkflow := read("skills", "multi", "dingtalk-doc", "references", "doc", "style", "doc-update-workflow.md")
	driveSkill := read("skills", "multi", "dingtalk-drive", "SKILL.md")
	wikiSkill := read("skills", "multi", "dingtalk-wiki", "SKILL.md")
	wikiReference := read("skills", "multi", "dingtalk-wiki", "references", "wiki.md")
	routing := read("skills", "multi", "dingtalk-shared", "references", "routing.md")
	intentGuide := read("skills", "multi", "dingtalk-shared", "references", "intent-guide.md")

	for _, route := range []string{"dws drive +copy", "dws drive +move", "dws drive rename", "dws drive delete"} {
		if !strings.Contains(docSkill, route) {
			t.Errorf("Doc Skill does not publish storage ownership route %q", route)
		}
	}
	for _, required := range []string{
		"在线文档节点的复制、移动和重命名属于 `dingtalk-drive`",
		"文档正文的创建、读取、追加、覆盖和 block 编辑属于 `dingtalk-doc`",
	} {
		if !strings.Contains(docSkill, required) {
			t.Errorf("Doc Skill missing direct node/body routing rule %q", required)
		}
	}
	for _, command := range []string{"dws doc +cover-set", "+cover-download", "+cover-clear"} {
		if !strings.Contains(docSkill, command) {
			t.Errorf("Doc Skill missing explicit cover mapping %q", command)
		}
	}
	for name, text := range map[string]string{"Doc Skill": docSkill, "Doc create Reference": docCreate, "Create workflow": createWorkflow} {
		if !strings.Contains(text, "空列表必须使用 JSONML") && name != "Create workflow" {
			t.Errorf("%s missing direct empty-list JSONML rule", name)
		}
		for _, token := range []string{"listId", "isOrdered", `"data-type":"leaf"`, `""`} {
			if !strings.Contains(text, token) {
				t.Errorf("%s missing minimal empty-list JSONML token %q", name, token)
			}
		}
	}
	for name, text := range map[string]string{"Doc Skill": docSkill, "Doc import Reference": docImport} {
		for _, required := range []string{"--folder", "--workspace", "最多提供一个", "默认个人文档根目录"} {
			if !strings.Contains(text, required) {
				t.Errorf("%s missing import target invariant %q", name, required)
			}
		}
		if strings.Contains(text, "dws wiki space search --type myWikiSpace") {
			t.Errorf("%s still resolves a default import target through wiki", name)
		}
	}
	for name, text := range map[string]string{"Doc Skill": docSkill, "Drive Skill": driveSkill, "Wiki Skill": wikiSkill, "Shared routing": routing} {
		for _, required := range []string{"文档空间", "知识库"} {
			if !strings.Contains(text, required) {
				t.Errorf("%s missing document-space/knowledge-base boundary %q", name, required)
			}
		}
	}
	for _, stale := range []string{"裸“文档空间”默认个人", "裸“文档空间”、我的文档或个人空间执行 `dws wiki", "用户说裸“文档空间”"} {
		if strings.Contains(docSkill, stale) || strings.Contains(wikiSkill, stale) || strings.Contains(wikiReference, stale) || strings.Contains(routing, stale) {
			t.Errorf("generic document-space routing still forces wiki: %q", stale)
		}
	}
	if !strings.Contains(wikiReference, "不进入 wiki") || !strings.Contains(wikiReference, "明确说“个人知识库”") {
		t.Error("Wiki reference does not guard myWikiSpace behind explicit knowledge-base intent")
	}
	for _, required := range []string{"dws drive mkdir", "dws doc +create", "dws drive move", "dws drive info"} {
		if !strings.Contains(intentGuide, required) {
			t.Errorf("composite document-space route missing %q", required)
		}
	}
	if !strings.Contains(driveSkill, "dws drive mkdir") || !strings.Contains(driveSkill, "dws drive move --folder") || !strings.Contains(driveSkill, "dws drive info --node") {
		t.Error("Drive Skill does not publish the folder -> move -> info storage flow")
	}
	for _, required := range []string{"生成文档", "本地", ".md", "在线"} {
		if !strings.Contains(docSkill, required) || !strings.Contains(routing, required) {
			t.Errorf("online-document/local-Markdown boundary missing %q", required)
		}
	}
	for name, text := range map[string]string{
		"Doc Skill": docSkill, "Doc create Reference": docCreate, "Create workflow": createWorkflow,
	} {
		for _, required := range []string{"普通", "Markdown", "富结构"} {
			if !strings.Contains(text, required) {
				t.Errorf("%s missing lightweight authoring rule %q", name, required)
			}
		}
	}
	for _, required := range []string{"Block ID 生命周期", "不复用", "定点 refetch"} {
		if !strings.Contains(docSkill, required) || !strings.Contains(docUpdate, required) || !strings.Contains(updateWorkflow, required) {
			t.Errorf("Doc update lifecycle contract missing %q", required)
		}
	}
	for _, required := range []string{"--expected-revision", "--command overwrite --doc-format jsonml", "CommandMeta"} {
		if !strings.Contains(updateWorkflow, required) {
			t.Errorf("Doc update workflow missing canonical contract %q", required)
		}
	}
	for _, stale := range []string{"--revision <", "≥3 次", "60-30-10", "RFC —"} {
		if strings.Contains(createWorkflow, stale) || strings.Contains(updateWorkflow, stale) {
			t.Errorf("Doc workflow still contains heavyweight or stale instruction %q", stale)
		}
	}
	if len(createWorkflow) > 12_000 || len(updateWorkflow) > 12_000 {
		t.Errorf("Doc workflows exceed the progressive-disclosure budget: create=%d update=%d", len(createWorkflow), len(updateWorkflow))
	}
	for _, required := range []string{"--source MY\\|PUBLIC", "顺序或序号", "用户在当前请求中已明确", "printf"} {
		if !strings.Contains(docSkill, required) {
			t.Errorf("Doc Skill missing deterministic execution rule %q", required)
		}
	}
	frontmatter := strings.SplitN(docSkill, "---", 3)
	if len(frontmatter) != 3 {
		t.Fatal("Doc Skill missing YAML frontmatter")
	}
	for _, required := range []string{"Use when", "创建", "生成", "撰写", "未明确本地文件", "在线 adoc"} {
		if !strings.Contains(frontmatter[1], required) {
			t.Errorf("Doc Skill discovery contract missing %q", required)
		}
	}
}

func TestCrossPlatformCoverageDocGoldenRoutePinsP0CommandsAndDeduplicatesLifecycle(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	path := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", "skills", "multi", "dingtalk-doc", "SKILL.md"))
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, "| 用户意图 | 唯一推荐入口 | 关键边界 |") {
		t.Fatal("Doc Golden Route must use the intent / unique route / boundary table")
	}
	if !strings.Contains(text, "| 用户意图 | 入口 |") {
		t.Fatal("Doc low-frequency exact routes must use the intent / route table")
	}
	golden := strings.SplitN(strings.SplitN(text, "## Golden Route", 2)[1], "### 低频精确入口", 2)[0]
	rows := 0
	for _, line := range strings.Split(golden, "\n") {
		if strings.HasPrefix(line, "| ") && !strings.HasPrefix(line, "| 用户意图 ") {
			rows++
		}
	}
	if rows > 12 {
		t.Errorf("Doc Golden Route has %d routes; want at most 12", rows)
	}
	for _, required := range []string{
		"dws doc +fetch --node <ID> --detail with-ids",
		"--include-style",
		"--include-permissions",
		"--include-history",
		"--include-media",
		"--include-comments",
		"dws doc +create --name <标题> --content @./content.md",
		"dws doc +create --name <标题> --content -",
		"--command append --content @./content.md",
		"--command block_insert --ref-block <BLOCK_ID> --where before\\|after",
		"--command block_replace --block-id <BLOCK_ID>",
		"dws doc +access-grant --node <ID> --to <姓名[,姓名]>",
		"dws doc +access-revoke --node <ID> --to <姓名[,姓名]>",
		"dws doc +share --to <姓名[,姓名]> --url <URL>",
		"dws chat +dm",
		"禁止 `@绝对路径`、`@../路径`、`@-`",
		"不存在 `+access-list`",
		"接收人参数不是 `--user`",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("Doc Golden Route missing P0 command contract %q", required)
		}
	}
	if got := strings.Count(text, "Block ID 生命周期："); got != 1 {
		t.Errorf("Doc Skill should publish one Block ID lifecycle rule, got %d", got)
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
