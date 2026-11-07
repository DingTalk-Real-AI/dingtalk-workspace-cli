// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package scripts_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAIBehaviorPolicyProtectsEnforcementInputs(t *testing.T) {
	t.Parallel()

	path, err := filepath.Abs(filepath.Join("..", "..", ".github", "workflows", "ai-behavior-check.yml"))
	if err != nil {
		t.Fatalf("Abs(ai-behavior-check.yml) error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}

	workflow := string(data)
	for _, want := range []string{
		"filename.startsWith('scripts/ci/')",
		"filename.startsWith('scripts/policy/')",
		"filename === 'test/fixtures/cli-interface-baseline.txt'",
		"previous_filename",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("AI behavior policy does not protect %q", want)
		}
	}
}

func TestAIBehaviorReviewedPublicPatchExecution(t *testing.T) {
	testAIBehaviorReviewedPatchExecution(t, "aitable")
}

func TestAIBehaviorReviewedChatThreadPromotePatchExecution(t *testing.T) {
	testAIBehaviorReviewedPatchExecution(t, "chat_thread_promote")
}

func TestAIBehaviorReviewedPublicSyncPatchExecution(t *testing.T) {
	testAIBehaviorReviewedPatchExecution(t, "public_sync_1529")
}

func testAIBehaviorReviewedPatchExecution(t *testing.T, patch string) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("执行 AI Behavior 工作流回归测试需要 Node.js")
	}
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ai-behavior-check.yml"))
	if err != nil {
		t.Fatalf("读取 AI Behavior 工作流: %v", err)
	}
	const marker = "          script: |\n"
	_, body, found := strings.Cut(string(data), marker)
	if !found {
		t.Fatal("未找到工作流内嵌的 github-script")
	}
	var lines []string
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == "" {
			lines = append(lines, "")
			continue
		}
		if !strings.HasPrefix(line, "            ") {
			break
		}
		lines = append(lines, strings.TrimPrefix(line, "            "))
	}
	script := strings.Join(lines, "\n")
	if strings.TrimSpace(script) == "" {
		t.Fatal("提取的 github-script 为空")
	}

	cases := []struct {
		name string
		pass bool
	}{
		{"reviewed_original", true},
		{"reviewed_rebased", true},
		{"ordinary_small", true},
		{"ordinary_at_limit", true},
		{"ordinary_over_limit", false},
		{"ordinary_protected", false},
		{"ordinary_protected_rename", false},
		{"new_blob_same_path", false},
		{"changed_preimage", false},
		{"extra_file", false},
		{"extra_protected_file", false},
		{"unreported_extra_tree_change", false},
		{"approved_protected_path_substitution", false},
		{"changed_mode", false},
		{"symlink", false},
		{"submodule", false},
		{"rename", false},
		{"deletion", false},
		{"truncated_current_tree", false},
		{"truncated_current_base_tree", false},
		{"truncated_reviewed_base_tree", false},
		{"truncated_reviewed_tree", false},
		{"base_not_ancestor", false},
		{"head_drift_before_files", false},
		{"head_drift_after_files", false},
		{"head_drift_after_trees", false},
		{"base_drift_after_trees", false},
		{"draft_drift_after_trees", false},
		{"author_drift_after_trees", false},
		{"ref_drift_after_trees", false},
		{"closed_drift_after_trees", false},
		{"base_repo_drift_after_trees", false},
		{"head_repo_drift_after_trees", false},
		{"wrong_repository", false},
		{"wrong_pr", false},
		{"wrong_author", false},
		{"wrong_ref", false},
		{"wrong_base_ref", false},
		{"wrong_base_repository", false},
		{"closed", false},
		{"governance_pr_self_authorization", false},
		{"fork_head", false},
		{"draft", false},
		{"head_policy_injection", false},
		{"body_exception_injection", false},
		{"files_api_sha_mismatch", false},
		{"files_api_status_mismatch", false},
		{"files_api_omission", false},
		{"files_api_duplicate", false},
		{"duplicate_tree_path", false},
		{"tree_api_error", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input, err := json.Marshal(map[string]string{"script": script, "scenario": tc.name, "patch": patch})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, node, "-e", aiBehaviorExecutionHarness)
			command.Stdin = strings.NewReader(string(input))
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("Node 工作流执行失败: %v\n%s", err, output)
			}
			var result struct {
				Statuses []struct {
					State string `json:"state"`
					SHA   string `json:"sha"`
				} `json:"statuses"`
				Failures    []string `json:"failures"`
				Thrown      string   `json:"thrown"`
				Head        string   `json:"head"`
				CommitReads []string `json:"commitReads"`
				PullReads   int      `json:"pullReads"`
			}
			if err := json.Unmarshal(output, &result); err != nil {
				t.Fatalf("解码 Node 执行结果: %v\n%s", err, output)
			}
			if len(result.Statuses) == 0 {
				t.Fatalf("工作流未发布状态: %s", output)
			}
			final := result.Statuses[len(result.Statuses)-1]
			if final.SHA != result.Head {
				t.Fatalf("状态未绑定事件中的精确 head: %s", output)
			}
			if tc.pass {
				if final.State != "success" || len(result.Failures) != 0 || result.Thrown != "" {
					t.Fatalf("获准补丁被拒绝: %s", output)
				}
				if strings.HasPrefix(tc.name, "reviewed_") {
					anchors := []string{
						"3f0fc94111b1f9cd34e47468980b1f1a723ca69a",
						"53de9d92c9b310798d7f42a385e162273330442b",
					}
					if patch == "chat_thread_promote" {
						anchors = []string{
							"59d1da576c6983582d5be11401e7efe07aaaa238",
							"c70ede29e0860a88e309bb540f27d888e9d8a8ab",
						}
					}
					if patch == "public_sync_1529" {
						anchors = []string{
							"a391c42c7de6c7e86fc19f92aa1f18e630ed402e",
							"4fdf11c739e22d348d3e2878c20a168540b92b1d",
						}
					}
					for _, sha := range anchors {
						if !strings.Contains(strings.Join(result.CommitReads, "\n"), sha) {
							t.Fatalf("未读取固定评审锚点 %s: %s", sha, output)
						}
					}
					if result.PullReads < 3 {
						t.Fatalf("完整补丁读取后未重新核对 PR 身份: %s", output)
					}
				}
			} else if final.State != "failure" && final.State != "error" {
				t.Fatalf("未经批准的候选未失败: %s", output)
			} else if len(result.Failures) == 0 && result.Thrown == "" {
				t.Fatalf("状态失败但工作流自身未失败: %s", output)
			}
		})
	}
}

// 直接执行工作流脚本，避免只检查 YAML 字符串而漏掉实际放行逻辑。
const aiBehaviorExecutionHarness = `
const input = JSON.parse(require('fs').readFileSync(0, 'utf8'));
const scenario = input.scenario;
const clone = value => JSON.parse(JSON.stringify(value));
// 固定业务审批输入独立于工作流中的授权列表，避免测试跟着错误授权一起改变。
const chatThreadPromote = input.patch === 'chat_thread_promote';
const publicSync = input.patch === 'public_sync_1529';
const reviewedNumber = publicSync ? 1529 : chatThreadPromote ? 1502 : 1463;
const reviewedBase = publicSync ? 'a391c42c7de6c7e86fc19f92aa1f18e630ed402e' : chatThreadPromote
  ? '59d1da576c6983582d5be11401e7efe07aaaa238'
  : '3f0fc94111b1f9cd34e47468980b1f1a723ca69a';
const reviewedHead = publicSync ? '4fdf11c739e22d348d3e2878c20a168540b92b1d' : chatThreadPromote
  ? 'c70ede29e0860a88e309bb540f27d888e9d8a8ab'
  : '53de9d92c9b310798d7f42a385e162273330442b';
const reviewedRef = publicSync ? 'codex/sync-qwenwork-1.0.65' : chatThreadPromote
  ? 'codex/fix-chat-thread-promote-retirement'
  : 'codex/sync-aitable-public-fixes';
const rebased = scenario === 'reviewed_rebased';
const base = rebased || scenario === 'changed_preimage' ? 'a'.repeat(40) : reviewedBase;
const head = rebased ? 'b'.repeat(40) : reviewedHead;
const blob = value => value.toString(16).padStart(40, '0');
const protectedPaths = chatThreadPromote || publicSync ? [
  'scripts/policy/interface-migrations/approved-command-migrations-v1.json',
] : [
  'scripts/policy/skill-command-check/main.go',
  'scripts/policy/skill-command-check/main_test.go',
];
// #1529 路径与增改状态固定取自 M→C1，不从被测工作流反读审批内容。
const paths = publicSync ? [
  '.changes/1421-skill-setup-fail-closed-staging.md',
  '.changes/1425-windows-skill-setup-junction.md',
  '.changes/aitable-supabase-api-key.md',
  '.changes/digital-employee-a2ui-replies.md',
  '.changes/dingtalk-tag-draft-editor-schema.md',
  '.changes/public-product-access-and-uploads.md',
  'internal/app/schema_deap_agent_contract_test.go',
  'internal/app/schema_product_upload_contact_test.go',
  'internal/app/schema_sql_sheet_key_test.go',
  'internal/app/skill_setup.go',
  'internal/app/skill_setup_canonical_test.go',
  'internal/app/skill_setup_cleanup_coverage_test.go',
  'internal/app/skill_setup_entry_unix.go',
  'internal/app/skill_setup_entry_windows.go',
  'internal/app/skill_setup_link_unix.go',
  'internal/app/skill_setup_link_windows.go',
  'internal/app/skill_setup_link_windows_test.go',
  'internal/auth/cli_auth_check_test.go',
  'internal/auth/oauth_helpers.go',
  'internal/cli/schema_parameter_mapping_ledger.go',
  'internal/helpers/aitable.go',
  'internal/helpers/aitable_api_key.go',
  'internal/helpers/aitable_api_key_test.go',
  'internal/helpers/chat.go',
  'internal/helpers/connect_codex_appserver.go',
  'internal/helpers/connect_codex_progress.go',
  'internal/helpers/connect_codex_progress_test.go',
  'internal/helpers/connect_progress.go',
  'internal/helpers/contact.go',
  'internal/helpers/contact_enterprise_test.go',
  'internal/helpers/contact_open_id_result_test.go',
  'internal/helpers/deap_agent.go',
  'internal/helpers/deap_agent_adapter.go',
  'internal/helpers/deap_agent_card.go',
  'internal/helpers/deap_agent_card_integration_test.go',
  'internal/helpers/deap_agent_card_payload.go',
  'internal/helpers/deap_agent_card_publish_test.go',
  'internal/helpers/deap_agent_card_regression_test.go',
  'internal/helpers/deap_agent_card_review_test.go',
  'internal/helpers/deap_agent_card_test.go',
  'internal/helpers/deap_agent_channel.go',
  'internal/helpers/deap_agent_draft.go',
  'internal/helpers/deap_agent_draft_contract_test.go',
  'internal/helpers/deap_agent_feedback.go',
  'internal/helpers/deap_agent_runtime.go',
  'internal/helpers/deap_agent_skill_mcp.go',
  'internal/helpers/deap_agent_test.go',
  'internal/helpers/image_upload.go',
  'internal/helpers/image_upload_test.go',
  'internal/helpers/oa.go',
  'internal/helpers/oa_image_upload.go',
  'internal/helpers/oa_image_upload_fifo_test.go',
  'internal/helpers/oa_image_upload_test.go',
  'internal/helpers/wukong_sync_commands_test.go',
  'internal/junction/junction_stub.go',
  'internal/junction/junction_windows.go',
  'internal/junction/junction_windows_test.go',
  'internal/shortcut/chat/chat_message.go',
  'internal/upgrade/canonical_skills_test.go',
  'internal/upgrade/paths.go',
  'internal/upgrade/paths_junction_windows_test.go',
  'internal/upgrade/skill_path_move.go',
  'internal/upgrade/skill_path_move_errors_test.go',
  'internal/upgrade/skill_path_move_test.go',
  'internal/upgrade/skill_path_move_unix.go',
  'internal/upgrade/skill_path_move_unix_test.go',
  'internal/upgrade/skill_path_move_windows.go',
  'internal/upgrade/skill_path_move_windows_test.go',
  'internal/upgrade/skill_publication.go',
  'internal/upgrade/skill_publication_identity_coverage_test.go',
  'internal/upgrade/skill_publication_test.go',
  'scripts/policy/interface-migrations/approved-command-migrations-v1.json',
  'skills/mono/references/products/aitable.md',
  'skills/mono/references/products/aitable/aitable-api-key.md',
  'skills/mono/references/products/attendance.md',
  'skills/mono/references/products/chat.md',
  'skills/mono/references/products/dingtalk-tag/manage-and-connect.md',
  'skills/mono/references/products/dingtalk-tag/manage.md',
  'skills/mono/references/products/oa.md',
  'skills/mono/references/products/oa/oa-form-components.md',
  'skills/mono/references/products/oa/oa-supply.md',
  'skills/multi/dingtalk-aitable/SKILL.md',
  'skills/multi/dingtalk-aitable/references/aitable/aitable-api-key.md',
  'skills/multi/dingtalk-chat/SKILL.md',
  'skills/multi/dingtalk-chat/references/card/a2ui.md',
  'skills/multi/dingtalk-chat/references/card/schema.md',
  'skills/multi/dingtalk-chat/references/chat/group-admin.md',
  'skills/multi/dingtalk-contact/SKILL.md',
  'skills/multi/dingtalk-contact/references/contact.md',
  'skills/multi/dingtalk-event/SKILL.md',
  'skills/multi/dingtalk-event/references/event-im-output.md',
  'skills/multi/dingtalk-misc/references/oa-create.md',
  'skills/multi/dingtalk-misc/references/oa.md',
  'skills/multi/dingtalk-misc/references/oa/oa-form-components.md',
  'skills/multi/dingtalk-misc/references/oa/oa-supply.md',
  'skills/multi/dingtalk-misc/scripts/oa_create_preflight.py',
  'skills/multi/dingtalk-tag/SKILL.md',
  'skills/multi/dingtalk-tag/references/manage-and-connect.md',
  'skills/multi/dingtalk-tag/references/manage.md',
  'test/scripts/oa_skill_scripts_test.py',
] : chatThreadPromote ? [
  '.changes/retire-chat-thread-promote.md',
  'internal/app/root_help_test.go',
  'internal/app/schema_agent_selection_contract_test.go',
  'internal/cli/schema_agent_examples.go',
  'internal/cli/schema_catalog_structure.go',
  'internal/cli/schema_catalog_structure_test.go',
  'internal/cli/schema_cross_platform_coverage_remaining_test.go',
  'internal/helpers/chat_thread.go',
  'internal/helpers/chat_thread_test.go',
  'scripts/policy/interface-migrations/approved-command-migrations-v1.json',
  'skills/mono/references/intent-guide.md',
  'skills/mono/references/products/chat.md',
  'skills/multi/dingtalk-chat/references/chat/thread.md',
  'skills/multi/dingtalk-shared/references/intent-guide.md',
  'test/unit/chat_thread_skill_routing_test.go',
] : ['.changes/aitable-public-reliability.md'];
if (!chatThreadPromote && !publicSync) {
  for (let i = 0; i < 41; i++) paths.push('internal/aitableprotocol/reviewed-fixture-' + i + '.go');
  paths.push(...protectedPaths);
}
const addedPaths = new Set(publicSync ? [
  '.changes/1421-skill-setup-fail-closed-staging.md',
  '.changes/1425-windows-skill-setup-junction.md',
  '.changes/aitable-supabase-api-key.md',
  '.changes/digital-employee-a2ui-replies.md',
  '.changes/dingtalk-tag-draft-editor-schema.md',
  '.changes/public-product-access-and-uploads.md',
  'internal/app/schema_product_upload_contact_test.go',
  'internal/app/skill_setup_entry_unix.go',
  'internal/app/skill_setup_entry_windows.go',
  'internal/app/skill_setup_link_unix.go',
  'internal/app/skill_setup_link_windows.go',
  'internal/app/skill_setup_link_windows_test.go',
  'internal/helpers/connect_codex_progress.go',
  'internal/helpers/connect_codex_progress_test.go',
  'internal/helpers/connect_progress.go',
  'internal/helpers/contact_open_id_result_test.go',
  'internal/helpers/deap_agent_card.go',
  'internal/helpers/deap_agent_card_integration_test.go',
  'internal/helpers/deap_agent_card_payload.go',
  'internal/helpers/deap_agent_card_publish_test.go',
  'internal/helpers/deap_agent_card_regression_test.go',
  'internal/helpers/deap_agent_card_review_test.go',
  'internal/helpers/deap_agent_card_test.go',
  'internal/helpers/image_upload.go',
  'internal/helpers/image_upload_test.go',
  'internal/helpers/oa_image_upload.go',
  'internal/helpers/oa_image_upload_fifo_test.go',
  'internal/helpers/oa_image_upload_test.go',
  'internal/junction/junction_stub.go',
  'internal/junction/junction_windows.go',
  'internal/junction/junction_windows_test.go',
  'internal/upgrade/paths_junction_windows_test.go',
  'internal/upgrade/skill_publication_identity_coverage_test.go',
] : [paths[0]]);
if (chatThreadPromote) addedPaths.add('test/unit/chat_thread_skill_routing_test.go');
// 变异既有文件，确保删除、重命名与 Files API 状态错配场景在三组中语义一致。
const changedIndex = paths.findIndex(path => !addedPaths.has(path));
const entry = (path, sha) => ({ path, sha, mode: '100644', type: 'blob' });
const reviewedBaseTree = paths.filter(path => !addedPaths.has(path)).map((path, i) => entry(path, blob(1000 + i)));
const reviewedHeadTree = paths.map((path, i) => entry(path, blob(2000 + i)));
reviewedBaseTree.push(entry('unchanged.txt', blob(3000)));
reviewedHeadTree.push(entry('unchanged.txt', blob(3000)));
const currentBaseTree = clone(reviewedBaseTree);
const currentHeadTree = clone(reviewedHeadTree);
if (rebased) {
  const governance = entry('.github/workflows/ai-behavior-check.yml', blob(4000));
  currentBaseTree.push(clone(governance));
  currentHeadTree.push(clone(governance));
}
let files = paths.map((filename, i) => ({
  filename, status: addedPaths.has(filename) ? 'added' : 'modified', sha: blob(2000 + i),
}));
const pull = {
  number: reviewedNumber, state: 'open', draft: false,
  user: { login: 'haofeng0705' },
  base: { sha: base, ref: 'main', repo: { full_name: 'DingTalk-Real-AI/dingtalk-workspace-cli' } },
  head: {
    sha: head, ref: reviewedRef,
    repo: { full_name: 'DingTalk-Real-AI/dingtalk-workspace-cli' },
  },
  labels: [{ name: 'ai-generated' }], changed_files: paths.length,
};
const context = {
  eventName: 'pull_request_target', sha: base,
  repo: { owner: 'DingTalk-Real-AI', repo: 'dingtalk-workspace-cli' },
  issue: { number: reviewedNumber }, payload: {},
};
switch (scenario) {
  case 'ordinary_small':
    pull.number = context.issue.number = 1464;
    files = files.filter(file => !protectedPaths.includes(file.filename)).slice(0, 2);
    break;
  case 'ordinary_at_limit':
  case 'ordinary_over_limit':
    pull.number = context.issue.number = 1464;
    files = Array.from({ length: scenario === 'ordinary_at_limit' ? 30 : 31 }, (_, i) => ({ filename: 'internal/cli/ordinary-' + i + '.go', status: 'added', sha: blob(6000 + i) }));
    break;
  case 'ordinary_protected':
    pull.number = context.issue.number = 1464;
    files = files.filter(file => protectedPaths.includes(file.filename));
    break;
  case 'ordinary_protected_rename':
    pull.number = context.issue.number = 1464;
    files = [{ filename: 'internal/cli/renamed-policy.go', previous_filename: protectedPaths[0], status: 'renamed', sha: blob(5000) }];
    break;
  case 'new_blob_same_path':
  case 'body_exception_injection':
    currentHeadTree[changedIndex].sha = files[changedIndex].sha = blob(5000);
    pull.body = JSON.stringify({ reviewed: { base, head, count: paths.length, protectedPaths } });
    break;
  case 'extra_file':
    currentHeadTree.push(entry('internal/aitableprotocol/unreviewed.go', blob(5000)));
    files.push({ filename: 'internal/aitableprotocol/unreviewed.go', status: 'added', sha: blob(5000) });
    break;
  case 'extra_protected_file':
    currentHeadTree.push(entry('scripts/policy/unreviewed.go', blob(5000)));
    files.push({ filename: 'scripts/policy/unreviewed.go', status: 'added', sha: blob(5000) });
    break;
  case 'unreported_extra_tree_change':
    currentHeadTree.push(entry('internal/cli/unreported.go', blob(5000)));
    break;
  case 'approved_protected_path_substitution':
    for (const tree of [reviewedBaseTree, reviewedHeadTree, currentBaseTree, currentHeadTree]) {
      tree.find(item => item.path === protectedPaths[0]).path = 'scripts/policy/unapproved-path.go';
    }
    files.find(file => file.filename === protectedPaths[0]).filename = 'scripts/policy/unapproved-path.go';
    break;
  case 'changed_preimage': currentBaseTree[0].sha = blob(5000); break;
  case 'changed_mode': currentHeadTree[changedIndex].mode = '100755'; break;
  case 'symlink': currentHeadTree[changedIndex].mode = '120000'; break;
  case 'submodule': currentHeadTree[changedIndex].mode = '160000'; currentHeadTree[changedIndex].type = 'commit'; break;
  case 'rename':
    currentHeadTree[changedIndex].path = files[changedIndex].filename = 'internal/aitableprotocol/renamed.go';
    files[changedIndex].previous_filename = paths[changedIndex]; files[changedIndex].status = 'renamed';
    break;
  case 'deletion': currentHeadTree.splice(changedIndex, 1); files[changedIndex].status = 'removed'; break;
  case 'wrong_repository': context.repo.repo = 'other-repository'; break;
  case 'wrong_pr': pull.number = context.issue.number = 1464; break;
  case 'wrong_author': pull.user.login = 'other-author'; break;
  case 'wrong_ref': pull.head.ref = 'other-head-ref'; break;
  case 'wrong_base_ref': pull.base.ref = 'other-base-ref'; break;
  case 'wrong_base_repository': pull.base.repo.full_name = 'other-owner/dingtalk-workspace-cli'; break;
  case 'closed': pull.state = 'closed'; break;
  case 'governance_pr_self_authorization':
    pull.number = context.issue.number = 1503;
    pull.head.ref = publicSync ? 'codex/authorize-public-sync-1529' : 'codex/authorize-chat-thread-promote-1502';
    files = [{ filename: '.github/workflows/ai-behavior-check.yml', status: 'modified', sha: blob(5000) }];
    break;
  case 'fork_head': pull.head.repo.full_name = 'other-owner/dingtalk-workspace-cli'; break;
  case 'draft': pull.draft = true; break;
  case 'head_policy_injection':
    currentHeadTree[0].path = files[0].filename = '.github/workflows/injected-exception.json';
    break;
  case 'files_api_sha_mismatch': files[changedIndex].sha = blob(5000); break;
  case 'files_api_status_mismatch': files[changedIndex].status = 'added'; break;
  case 'files_api_omission': files.pop(); break;
  case 'files_api_duplicate': files[changedIndex] = clone(files[0]); break;
  case 'duplicate_tree_path': currentHeadTree.push(clone(currentHeadTree[changedIndex])); break;
}
pull.changed_files = files.length;
context.payload.pull_request = clone(pull);
const snapshots = new Map([
  [reviewedBase, reviewedBaseTree], [reviewedHead, reviewedHeadTree],
]);
// 同一 SHA 的响应不得被候选更改污染；变体使用独立当前 head。
if (!rebased && scenario !== 'reviewed_original') {
  pull.head.sha = 'c'.repeat(40);
  context.payload.pull_request.head.sha = pull.head.sha;
}
snapshots.set(base, currentBaseTree);
snapshots.set(pull.head.sha, currentHeadTree);
const statuses = [], failures = [], commitReads = [];
let pullReads = 0;
const github = {
  paginate: async (method, params) => {
    if (method !== github.rest.pulls.listFiles || params.pull_number !== context.issue.number) {
      throw new Error('Unexpected Files API request');
    }
    return clone(files);
  },
  rest: {
    pulls: {
      listFiles: async () => { throw new Error('Files API must be paginated'); },
      get: async () => {
        pullReads++;
        const current = clone(pull);
        if ((scenario === 'head_drift_before_files' && pullReads >= 1) ||
            (scenario === 'head_drift_after_files' && pullReads >= 2) ||
            (scenario === 'head_drift_after_trees' && pullReads >= 3)) {
          current.head.sha = 'd'.repeat(40);
        }
        if (scenario === 'base_drift_after_trees' && pullReads >= 3) current.base.sha = 'd'.repeat(40);
        if (scenario === 'draft_drift_after_trees' && pullReads >= 3) current.draft = true;
        if (scenario === 'author_drift_after_trees' && pullReads >= 3) current.user.login = 'other-author';
        if (scenario === 'ref_drift_after_trees' && pullReads >= 3) current.head.ref = 'other-head-ref';
        if (scenario === 'closed_drift_after_trees' && pullReads >= 3) current.state = 'closed';
        if (scenario === 'base_repo_drift_after_trees' && pullReads >= 3) current.base.repo.full_name = 'other-owner/dingtalk-workspace-cli';
        if (scenario === 'head_repo_drift_after_trees' && pullReads >= 3) current.head.repo.full_name = 'other-owner/dingtalk-workspace-cli';
        return { data: current };
      },
    },
    repos: {
      createCommitStatus: async status => { statuses.push(clone(status)); return { data: status }; },
      compareCommits: async params => {
        if (params.base !== base || params.head !== pull.head.sha) throw new Error('Wrong ancestry anchors');
        return { data: {
          merge_base_commit: { sha: scenario === 'base_not_ancestor' ? 'd'.repeat(40) : base },
          behind_by: scenario === 'base_not_ancestor' ? 1 : 0,
        } };
      },
    },
    git: {
      getCommit: async ({ commit_sha }) => {
        commitReads.push(commit_sha);
        if (!snapshots.has(commit_sha)) throw new Error('Unapproved commit lookup');
        return { data: { tree: { sha: commit_sha } } };
      },
      getTree: async ({ tree_sha, recursive }) => {
        if (recursive !== '1') throw new Error('Tree read must be complete and recursive');
        if (scenario === 'tree_api_error') throw new Error('Tree API unavailable');
        if (!snapshots.has(tree_sha)) throw new Error('Unknown tree lookup');
        const truncated =
          (scenario === 'truncated_current_tree' && tree_sha === pull.head.sha) ||
          (scenario === 'truncated_current_base_tree' && tree_sha === base) ||
          (scenario === 'truncated_reviewed_base_tree' && tree_sha === reviewedBase) ||
          (scenario === 'truncated_reviewed_tree' && tree_sha === reviewedHead);
        return { data: { truncated, tree: clone(snapshots.get(tree_sha)) } };
      },
    },
  },
};
const core = {
  setFailed: message => failures.push(String(message)),
  notice: () => {}, warning: () => {}, info: () => {},
};
const AsyncFunction = Object.getPrototypeOf(async function() {}).constructor;
(async () => {
  let thrown = '';
  try { await new AsyncFunction('github', 'context', 'core', input.script)(github, context, core); }
  catch (error) { thrown = String(error.message || error); }
  process.stdout.write(JSON.stringify({
    statuses, failures, thrown, head: context.payload.pull_request.head.sha, commitReads, pullReads,
  }));
})().catch(error => { process.stderr.write(String(error)); process.exitCode = 1; });
`
