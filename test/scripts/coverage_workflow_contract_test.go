package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverageGatePolicyProfileCanBeExplicitlyOmitted(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}

	binDir := t.TempDir()
	fakeGoPath := filepath.Join(binDir, "go")
	const fakeGo = `#!/bin/sh
set -eu
case "$1" in
  build)
    shift
    output=
    while [ "$#" -gt 0 ]; do
      case "$1" in
        -o)
          output="$2"
          shift 2
          ;;
        *)
          shift
          ;;
      esac
    done
    cat > "$output" <<'EOF'
#!/bin/sh
printf '%s\n' "$@" > "$COVERAGE_ARGS_LOG"
EOF
    chmod +x "$output"
    ;;
  list)
    printf '%s\n' "example.com/coverage-fixture"
    ;;
  *)
    printf 'unexpected fake go command: %s\n' "$1" >&2
    exit 2
    ;;
esac
`
	if err := os.WriteFile(fakeGoPath, []byte(fakeGo), 0o755); err != nil {
		t.Fatalf("WriteFile(fake go) error = %v", err)
	}

	baseEnv := make([]string, 0, len(os.Environ())+2)
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "PATH=") ||
			strings.HasPrefix(value, "COVERAGE_DIFF_PROFILE=") ||
			strings.HasPrefix(value, "COVERAGE_ARGS_LOG=") {
			continue
		}
		baseEnv = append(baseEnv, value)
	}
	baseEnv = append(baseEnv, "PATH="+binDir+":"+os.Getenv("PATH"))

	runGate := func(t *testing.T, diffProfile *string) []string {
		t.Helper()

		argsLog := filepath.Join(t.TempDir(), "args.log")
		cmd := exec.Command(
			"sh",
			"./scripts/policy/check-coverage-gate.sh",
			"--base-ref",
			"HEAD",
		)
		cmd.Dir = root
		cmd.Env = append(append([]string{}, baseEnv...), "COVERAGE_ARGS_LOG="+argsLog)
		if diffProfile != nil {
			cmd.Env = append(cmd.Env, "COVERAGE_DIFF_PROFILE="+*diffProfile)
		}
		output, runErr := cmd.CombinedOutput()
		if runErr != nil {
			t.Fatalf("coverage gate error = %v\noutput:\n%s", runErr, output)
		}
		data, readErr := os.ReadFile(argsLog)
		if readErr != nil {
			t.Fatalf("ReadFile(args log) error = %v", readErr)
		}
		return strings.Fields(string(data))
	}

	assertDiffProfiles := func(t *testing.T, args []string, want ...string) {
		t.Helper()

		var got []string
		for i := 0; i+1 < len(args); i++ {
			if args[i] == "--diff-profile" {
				got = append(got, args[i+1])
				i++
			}
		}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("diff profiles = %q, want %q; args = %q", got, want, args)
		}
	}

	t.Run("unset keeps strict policy profile", func(t *testing.T) {
		assertDiffProfiles(
			t,
			runGate(t, nil),
			"coverage-policy.txt",
			"coverage.txt",
		)
	})

	t.Run("explicit empty omits only policy profile", func(t *testing.T) {
		empty := ""
		assertDiffProfiles(t, runGate(t, &empty), "coverage.txt")
	})
}

// TestCoverageWorkflowShardsAndBaselineCache pins the full-suite coverage
// architecture: the candidate profile is produced by disjoint per-shard
// helper jobs and reassembled before enforcement, and the merge-base profile
// is reused only through an exact-key cache written by a green main push of
// that same commit. Near-miss reuse (restore-keys) would compare the
// candidate against the wrong commit and must never appear.
func TestCoverageWorkflowShardsAndBaselineCache(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("ReadFile(ci.yml) error = %v", err)
	}
	admission := string(data)
	for _, want := range []string{
		"group: ci-${{ github.workflow }}-${{ github.event_name == 'pull_request' && format('pr-{0}', github.event.pull_request.number) || format('push-{0}', github.sha) }}",
		"cancel-in-progress: true",
	} {
		if !strings.Contains(admission, want) {
			t.Errorf("CI workflow missing latest-PR/exact-main producer contract %q", want)
		}
	}
	for _, forbidden := range []string{
		"format('pr-{0}-{1}-{2}'",
		"github.event.pull_request.number || github.ref",
	} {
		if strings.Contains(admission, forbidden) {
			t.Errorf("CI workflow retains a stale concurrency identity %q", forbidden)
		}
	}

	currentStart := strings.Index(admission, "\n  coverage-current:\n")
	fullStart := strings.Index(admission, "\n  coverage-current-full:\n")
	supportingStart := strings.Index(admission, "\n  coverage-supporting:\n")
	baselineStart := strings.Index(admission, "\n  coverage-baseline:\n")
	metadataStart := strings.Index(admission, "\n  coverage-main-metadata:\n")
	gateStart := strings.Index(admission, "\n  coverage:\n")
	policyStart := strings.Index(admission, "\n  policy:\n")
	if currentStart < 0 || fullStart <= currentStart || supportingStart <= fullStart ||
		baselineStart <= supportingStart || metadataStart <= baselineStart || gateStart <= metadataStart || policyStart <= gateStart {
		t.Fatal("CI workflow missing ordered coverage job boundaries")
	}

	currentJob := admission[currentStart:fullStart]
	if !strings.Contains(currentJob, "needs.lint.outputs.full_suite != 'true'") {
		t.Error("coverage-current must be scoped-tier only; the full suite belongs to the shard matrix")
	}
	if strings.Contains(currentJob, "./ ./cmd/... ./internal/... ./skills/...") {
		t.Error("coverage-current must not retain the retired single serial full-suite run")
	}

	fullJob := admission[fullStart:supportingStart]
	for _, want := range []string{
		"needs.lint.outputs.full_suite == 'true'",
		"fail-fast: false",
		"          - app",
		"          - cli",
		"          - generators",
		"          - helpers",
		"          - remaining",
		`./scripts/ci/test-packages.sh list-coverage "$COVERAGE_SHARD"`,
		"go test -count=1 -p 1",
		`-coverprofile="coverage-shard-$COVERAGE_SHARD.txt"`,
		"-covermode=atomic",
		"name: coverage-current-shard-${{ matrix.shard }}",
	} {
		if !strings.Contains(fullJob, want) {
			t.Errorf("coverage-current-full missing shard contract %q", want)
		}
	}

	baselineJob := admission[baselineStart:metadataStart]
	if !strings.Contains(baselineJob, "timeout-minutes: 30") {
		t.Error("coverage-baseline must retain enough headroom for an authoritative cold-cache fallback")
	}
	cachePath := "coverage-cache.txt"
	baselineKey := "dws-coverage-full-v2-${{ env.COVERAGE_BASE_REF }}-go${{ steps.setup-go.outputs.go-version }}"
	for _, want := range []string{
		"uses: actions/cache/restore@v4",
		"uses: actions/cache/save@v4",
		"path: " + cachePath,
		"key: " + baselineKey,
		"if: steps.baseline-cache.outputs.cache-hit != 'true'",
		"cp coverage-cache.txt coverage-base.txt",
		"cp coverage-base.txt coverage-cache.txt",
	} {
		if !strings.Contains(baselineJob, want) {
			t.Errorf("coverage-baseline missing cache contract %q", want)
		}
	}
	if strings.Count(baselineJob, "key: "+baselineKey) != 2 {
		t.Error("coverage-baseline restore and save must use the identical exact cache key")
	}
	if strings.Count(baselineJob, "path: "+cachePath) != 2 {
		t.Error("coverage-baseline restore and save must use the identical cache path/version")
	}
	if strings.Contains(baselineJob, "restore-keys") {
		t.Error("coverage baseline cache must stay exact-key; prefix restore-keys can resurrect a wrong-commit baseline")
	}

	metadataJob := admission[metadataStart:gateStart]
	for _, want := range []string{
		"github.event_name == 'push'",
		"needs.lint.outputs.changelog_only == 'true' || needs.lint.outputs.docs_only == 'true'",
		"timeout-minutes: 30",
		"fetch-depth: 0",
		"PUSH_BEFORE_SHA: ${{ github.event.before }}",
		"PUSH_AFTER_SHA: ${{ github.event.after }}",
		"git merge-base --is-ancestor \"$PUSH_BEFORE_SHA\" \"$PUSH_AFTER_SHA\"",
		"git diff --name-only --no-renames -z",
		`^\.changes/[a-z0-9][a-z0-9._-]*\.md$`,
		`^\.changes/released/[0-9]+\.[0-9]+\.[0-9]+(-beta\.[1-9][0-9]*)?/[a-z0-9][a-z0-9._-]*\.md$`,
		"Refusing coverage-cache promotion for unreviewed change-fragment path",
		"Refusing coverage-cache promotion for executable path",
		"COVERAGE_SOURCE_REF=$PUSH_BEFORE_SHA",
		"id: metadata-current-cache",
		"id: metadata-source-cache",
		"key: dws-coverage-full-v2-${{ env.COVERAGE_SOURCE_REF }}-go${{ steps.setup-go-metadata.outputs.go-version }}",
		"go test -count=1 -p 1",
		"./ ./cmd/... ./internal/... ./skills/...",
		"key: dws-coverage-full-v2-${{ github.sha }}-go${{ steps.setup-go-metadata.outputs.go-version }}",
		"id: metadata-target-cache-verification",
		"lookup-only: true",
		"fail-on-cache-miss: true",
		"EXACT_CACHE_HIT: ${{ steps.metadata-target-cache-verification.outputs.cache-hit }}",
		`run: test "$EXACT_CACHE_HIT" = true`,
	} {
		if !strings.Contains(metadataJob, want) {
			t.Errorf("metadata-only main cache producer missing contract %q", want)
		}
	}
	if strings.Contains(metadataJob, "restore-keys") {
		t.Error("metadata-only main cache promotion must use exact source and target keys")
	}
	if strings.Count(metadataJob, "path: "+cachePath) != 4 {
		t.Error("metadata-only producer must restore target/source, save, and verify through the shared cache version path")
	}

	gateJob := admission[gateStart:policyStart]
	for _, want := range []string{
		"- coverage-main-metadata",
		"MAIN_METADATA_RESULT: ${{ needs.coverage-main-metadata.result }}",
		"main_metadata_expected=success",
		`"main metadata cache:$MAIN_METADATA_RESULT:$main_metadata_expected"`,
		"pattern: coverage-current-*",
		"merge-multiple: true",
		"for shard in app cli generators helpers remaining; do",
		"test ! -f coverage.txt",
		`test "$(head -n 1 "$profile")" = "mode: atomic"`,
		"github.event_name == 'push'",
		"cp coverage.txt coverage-cache.txt",
		"path: " + cachePath,
		"key: dws-coverage-full-v2-${{ github.sha }}-go${{ steps.setup-go.outputs.go-version }}",
		"name: Verify push coverage cache exists",
		"id: push-cache-verification",
		"lookup-only: true",
		"fail-on-cache-miss: true",
		"EXACT_CACHE_HIT: ${{ steps.push-cache-verification.outputs.cache-hit }}",
		`run: test "$EXACT_CACHE_HIT" = true`,
		`"current shards:$CURRENT_FULL_RESULT:$current_full_expected"`,
	} {
		if !strings.Contains(gateJob, want) {
			t.Errorf("coverage gate missing shard assembly contract %q", want)
		}
	}

	if strings.Count(gateJob, "path: "+cachePath) != 2 {
		t.Error("green main push must save and verify the candidate profile through the same cache path/version as baseline restore")
	}
}

func TestFormulaCoverageBaselinePromotionContract(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	promotionData, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "coverage-baseline-promotion.yml"))
	if err != nil {
		t.Fatalf("ReadFile(coverage-baseline-promotion.yml) error = %v", err)
	}
	promotion := string(promotionData)
	for _, want := range []string{
		"repository_dispatch:",
		"types: [coverage-baseline-promote]",
		"checks: write",
		"contents: read",
		"group: coverage-baseline-promotion-${{ github.event.client_payload.target_sha }}",
		"cancel-in-progress: false",
		"queue: max",
		"github.repository == 'DingTalk-Real-AI/dingtalk-workspace-cli'",
		"timeout-minutes: 30",
		"context.payload.client_payload?.target_sha",
		"context.payload.client_payload?.source_run_id",
		"context.payload.client_payload?.check_run_id",
		"targetCommit.parents.length !== 1",
		"targetCommit.author?.login !== 'github-actions[bot]'",
		"targetCommit.committer?.login !== 'github-actions[bot]'",
		"files.length !== 1",
		"Formula/dingtalk-workspace-cli.rb",
		"Formula/dingtalk-workspace-cli-beta.rb",
		"[skip ci",
		"await requireSuccessfulAdmission(parentSha, 'Formula parent')",
		"await requireSuccessfulAdmission(targetSha, 'Formula target')",
		"for (let attempt = 1; attempt <= 6; attempt += 1)",
		"setTimeout(resolve, 5000)",
		"run.app?.slug !== 'github-actions'",
		"run.conclusion !== 'success'",
		"basehead: `${targetSha}...${branch.commit.sha}`",
		"['ahead', 'identical'].includes(containment.status)",
		"promotionCheck.name !== 'Coverage Baseline Cache'",
		"promotionCheck.external_id !== promotionExternalId",
		"promotionCheck.status !== 'queued'",
		"core.setOutput('check_run_id', String(checkRunId))",
		"name: Mark Formula cache promotion in progress",
		"status: 'in_progress'",
		"persist-credentials: false",
		"ref: ${{ steps.validate-target.outputs.target_sha }}",
		"test \"$(git rev-parse HEAD^)\" = \"$PARENT_SHA\"",
		"id: target-cache",
		"id: parent-cache",
		"key: dws-coverage-full-v2-${{ steps.validate-target.outputs.parent_sha }}-go${{ steps.setup-go.outputs.go-version }}",
		"go test -count=1 -p 1",
		"./ ./cmd/... ./internal/... ./skills/...",
		"key: dws-coverage-full-v2-${{ steps.validate-target.outputs.target_sha }}-go${{ steps.setup-go.outputs.go-version }}",
		"lookup-only: true",
		"fail-on-cache-miss: true",
		"id: formula-target-cache-verification",
		"EXACT_CACHE_HIT: ${{ steps.formula-target-cache-verification.outputs.cache-hit }}",
		`run: test "$EXACT_CACHE_HIT" = true`,
		"name: Complete Formula cache promotion acknowledgement",
		"PROMOTION_JOB_STATUS: ${{ job.status }}",
		"conclusion: succeeded ? 'success' : 'failure'",
	} {
		if !strings.Contains(promotion, want) {
			t.Errorf("Formula baseline promotion missing contract %q", want)
		}
	}
	for _, want := range []string{
		"actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683",
		"actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff",
		"actions/cache/restore@0057852bfaa89a56745cba8c7296529d2fc39830",
		"actions/cache/save@0057852bfaa89a56745cba8c7296529d2fc39830",
	} {
		if !strings.Contains(promotion, want) {
			t.Errorf("Formula baseline promotion must pin trusted action %q", want)
		}
	}
	for _, forbidden := range []string{
		"pull_request:",
		"pull_request_target:",
		"push:",
		"workflow_dispatch:",
		"restore-keys",
		"HOMEBREW_PR_TOKEN",
		"RELEASE_GOVERNANCE_TOKEN",
		"contents: write",
	} {
		if strings.Contains(promotion, forbidden) {
			t.Errorf("Formula baseline promotion must not contain %q", forbidden)
		}
	}
	validateIndex := strings.Index(promotion, "name: Validate Formula-only main target")
	checkoutIndex := strings.Index(promotion, "name: Check out validated Formula-only target")
	if validateIndex < 0 || checkoutIndex <= validateIndex {
		t.Error("Formula target must be fully validated before it is checked out or executed")
	}
	ackReadIndex := strings.Index(promotion, "const {data: promotionCheck} = await github.rest.checks.get")
	ackOutputIndex := strings.Index(promotion, "core.setOutput('check_run_id', String(checkRunId))")
	targetReadIndex := strings.Index(promotion, "const {data: targetCommit} = await github.rest.repos.getCommit")
	if ackReadIndex < 0 || ackOutputIndex <= ackReadIndex || targetReadIndex <= ackOutputIndex {
		t.Error("Formula acknowledgement must be identity-checked and bound to the finalizer before target validation")
	}
	if strings.Count(promotion, "path: coverage-cache.txt") != 4 {
		t.Error("Formula promotion must restore target/source, save, and verify through the shared cache version path")
	}

	releaseData, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("ReadFile(release.yml) error = %v", err)
	}
	release := string(releaseData)
	publishStart := strings.Index(release, "  publish-release:\n")
	sealStart := strings.Index(release, "      - name: Seal Formula-only Code Admission contexts\n")
	if publishStart < 0 || sealStart <= publishStart {
		t.Fatal("release workflow missing Formula-only seal start")
	}
	if !strings.Contains(release[publishStart:sealStart], "timeout-minutes: 30") {
		t.Error("publish-release must not be extended by cache acknowledgement polling")
	}
	sealEnd := strings.Index(release[sealStart:], "\n      - name: Reverify exact immutable npm package\n")
	if sealEnd < 0 {
		t.Fatal("release workflow missing Formula-only seal boundaries")
	}
	seal := release[sealStart : sealStart+sealEnd]
	for _, want := range []string{
		`core.setOutput("coverage_baseline_required", "true")`,
		`core.setOutput("coverage_baseline_commit", commit)`,
	} {
		if !strings.Contains(seal, want) {
			t.Errorf("Formula seal missing cache-promotion dispatch contract %q", want)
		}
	}
	for _, forbidden := range []string{
		"for (let attempt = 1; attempt <= 180; attempt += 1)",
		"github.rest.checks.get",
		"promotionComplete",
		"Coverage Baseline Cache",
		"github.rest.repos.createDispatchEvent",
	} {
		if strings.Contains(seal, forbidden) {
			t.Errorf("irreversible publish-release must not wait on cache through %q", forbidden)
		}
	}
	confirmationStart := strings.Index(release, "  coverage-baseline-confirmation:\n")
	deliveryGateStart := strings.Index(release, "  release-delivery-gate:\n")
	if confirmationStart < 0 || deliveryGateStart <= confirmationStart {
		t.Fatal("release workflow missing independent Formula baseline confirmation job")
	}
	confirmation := release[confirmationStart:deliveryGateStart]
	for _, want := range []string{
		"if (!['true', 'false'].includes(rawRequired))",
		"github.rest.checks.create",
		"name: 'Coverage Baseline Cache'",
		"status: 'queued'",
		"external_id: expectedExternalId",
		"github.rest.repos.createDispatchEvent",
		"event_type: 'coverage-baseline-promote'",
		"check_run_id: String(promotionCheck.id)",
		"conclusion: 'failure'",
		"for (let attempt = 1; attempt <= 180; attempt += 1)",
		"github.rest.checks.get",
		"currentCheck.name !== 'Coverage Baseline Cache'",
		"currentCheck.conclusion !== 'success'",
		"setTimeout(resolve, 10000)",
		"Formula baseline promotion timed out",
	} {
		if !strings.Contains(confirmation, want) {
			t.Errorf("independent Formula baseline confirmation missing %q", want)
		}
	}
}

func TestCoverageBaselineRepairContract(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Abs(repo root) error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "coverage-baseline-repair.yml"))
	if err != nil {
		t.Fatalf("ReadFile(coverage-baseline-repair.yml) error = %v", err)
	}
	workflow := string(data)

	for _, want := range []string{
		"pull_request_target:",
		"branches: [main]",
		"types: [closed]",
		"workflow_run:",
		"workflows: [CI]",
		"types: [completed]",
		"repository_dispatch:",
		"types: [coverage-baseline-repair]",
		"schedule:",
		`cron: "23 * * * *"`,
		"workflow_dispatch:",
		"group: coverage-baseline-repair-${{ github.event_name == 'pull_request_target' && github.event.pull_request.merge_commit_sha || github.event_name == 'workflow_run' && github.event.workflow_run.head_sha || github.event_name == 'repository_dispatch' && github.event.client_payload.merge_commit_sha || github.sha }}",
		"cancel-in-progress: false",
		"queue: max",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("coverage baseline repair missing workflow contract %q", want)
		}
	}

	dispatchStart := strings.Index(workflow, "\n  dispatch-merged-pr:\n")
	failedDispatchStart := strings.Index(workflow, "\n  dispatch-failed-ci:\n")
	repairStart := strings.Index(workflow, "\n  repair:\n")
	if dispatchStart < 0 || failedDispatchStart <= dispatchStart || repairStart <= failedDispatchStart {
		t.Fatal("coverage baseline repair is missing ordered merged-PR, failed-CI, and producer jobs")
	}
	dispatcher := workflow[dispatchStart:failedDispatchStart]
	for _, want := range []string{
		"github.event_name == 'pull_request_target'",
		"github.event.pull_request.merged == true",
		"github.repository == 'DingTalk-Real-AI/dingtalk-workspace-cli'",
		"actions: read",
		"contents: write",
		"pull-requests: read",
		"actions/github-script@f28e40c7f34bde8b3046d885e986cb6290c5673b",
		"context.payload.repository?.default_branch !== 'main'",
		"currentPull.state === 'closed'",
		"currentPull.merged === true",
		"typeof currentPull.merged_at === 'string'",
		"const baseRef = eventPull?.base?.ref;",
		"baseRef !== 'main'",
		"currentPull.base?.ref === 'main'",
		"isStableMergedPRIdentity(",
		"currentPull.head?.sha === headSha",
		"currentPull.merge_commit_sha === mergeCommitSha",
		"for (let attempt = 1; attempt <= 6; attempt += 1)",
		"setTimeout(resolve, 5000)",
		"basehead: `${targetSha}...${branch.commit.sha}`",
		"['ahead', 'identical'].includes(comparison.status)",
		"for (let attempt = 1; attempt <= 12; attempt += 1)",
		"github.rest.actions.getWorkflow",
		"workflow_id: '.github/workflows/ci.yml'",
		"ciWorkflow.name !== 'CI'",
		"ciWorkflow.path !== '.github/workflows/ci.yml'",
		"ciWorkflow.state !== 'active'",
		"github.rest.actions.listWorkflowRunsForRepo",
		"branch: 'main'",
		"event: 'push'",
		"run.name === 'CI'",
		"run.workflow_id === ciWorkflow.id",
		"run.path === ciWorkflow.path",
		"run.event === 'push'",
		"run.head_sha === mergeCommitSha",
		"run.head_branch === 'main'",
		"['queued', 'in_progress', 'completed'].includes(run.status)",
		"setTimeout(resolve, 5000)",
		"repair dispatch is unnecessary",
		"github.rest.repos.createDispatchEvent",
		"event_type: 'coverage-baseline-repair'",
		"source: 'merged_pr'",
		"pull_number: String(pullNumber)",
		"head_sha: headSha",
		"merge_commit_sha: mergeCommitSha",
	} {
		if !strings.Contains(dispatcher, want) {
			t.Errorf("merged-PR repair dispatcher missing %q", want)
		}
	}
	ciWorkflowLookup := strings.Index(dispatcher, "github.rest.actions.getWorkflow")
	pushRunLookup := strings.Index(dispatcher, "github.rest.actions.listWorkflowRunsForRepo")
	repairDispatch := strings.Index(dispatcher, "github.rest.repos.createDispatchEvent")
	if ciWorkflowLookup < 0 || pushRunLookup <= ciWorkflowLookup || repairDispatch <= pushRunLookup {
		t.Error("merged-PR dispatcher must bind the fixed CI workflow before exhausting exact push-run lookup and dispatch")
	}
	stableIdentityCheck := strings.Index(dispatcher, "if (!isStableMergedPRIdentity(")
	mainContainmentCheck := strings.Index(dispatcher, "await requireMainContainment(mergeCommitSha)")
	if stableIdentityCheck < 0 || mainContainmentCheck <= stableIdentityCheck || repairDispatch <= mainContainmentCheck {
		t.Error("merged-PR dispatcher must prove stable PR identity and main containment before dispatch")
	}
	for _, forbidden := range []string{
		"actions/checkout@",
		"actions/cache/",
		"actions/setup-go@",
		"go test ",
		"github.event.pull_request.head.ref",
		"currentPull.base.sha",
		"base_sha",
		"secrets.",
	} {
		if strings.Contains(dispatcher, forbidden) {
			t.Errorf("privileged pull_request_target dispatcher must not contain %q", forbidden)
		}
	}

	failedDispatcher := workflow[failedDispatchStart:repairStart]
	for _, want := range []string{
		"github.event_name == 'workflow_run'",
		"github.repository == 'DingTalk-Real-AI/dingtalk-workspace-cli'",
		"github.event.workflow_run.name == 'CI'",
		"github.event.workflow_run.event == 'push'",
		"github.event.workflow_run.head_branch == 'main'",
		"github.event.workflow_run.status == 'completed'",
		"github.event.workflow_run.conclusion != 'success'",
		"actions: read",
		"contents: write",
		"actions/github-script@f28e40c7f34bde8b3046d885e986cb6290c5673b",
		"context.payload.repository?.default_branch !== 'main'",
		"eventRun?.name !== 'CI'",
		"eventRun?.event !== 'push'",
		"eventRun?.head_branch !== 'main'",
		"eventRun?.status !== 'completed'",
		"conclusion === 'success'",
		"github.rest.actions.getWorkflow",
		"workflow_id: '.github/workflows/ci.yml'",
		"github.rest.actions.getWorkflowRun",
		"eventRun.workflow_id !== ciWorkflow.id",
		"currentRun.workflow_id !== ciWorkflow.id",
		"currentRun.name !== 'CI'",
		"currentRun.event !== 'push'",
		"currentRun.head_branch !== 'main'",
		"currentRun.head_sha !== headSha",
		"currentRun.run_attempt !== runAttempt",
		"currentRun.status !== 'completed'",
		"currentRun.conclusion !== conclusion",
		"currentRun.repository?.full_name !== upstream",
		"currentRun.head_repository?.full_name !== upstream",
		"github.rest.repos.createDispatchEvent",
		"event_type: 'coverage-baseline-repair'",
		"source: 'failed_ci'",
		"workflow_run_id: String(runID)",
		"workflow_run_attempt: String(runAttempt)",
		"workflow_conclusion: conclusion",
		"merge_commit_sha: headSha",
	} {
		if !strings.Contains(failedDispatcher, want) {
			t.Errorf("failed-CI repair dispatcher missing %q", want)
		}
	}
	failedRunRead := strings.Index(failedDispatcher, "github.rest.actions.getWorkflowRun")
	failedRepairDispatch := strings.Index(failedDispatcher, "github.rest.repos.createDispatchEvent")
	if failedRunRead < 0 || failedRepairDispatch <= failedRunRead {
		t.Error("workflow_run dispatcher must API-bind the exact failed CI run before repository dispatch")
	}
	for _, forbidden := range []string{
		"actions/checkout@",
		"actions/cache/",
		"actions/setup-go@",
		"go test ",
		"secrets.",
	} {
		if strings.Contains(failedDispatcher, forbidden) {
			t.Errorf("privileged workflow_run dispatcher must not contain %q", forbidden)
		}
	}

	producer := workflow[repairStart:]
	for _, want := range []string{
		"github.event_name == 'repository_dispatch' || github.event_name == 'schedule' || github.event_name == 'workflow_dispatch'",
		"timeout-minutes: 35",
		"actions: read",
		"contents: read",
		"pull-requests: read",
		"id: resolve-target",
		"context.eventName === 'repository_dispatch'",
		"payload.source === 'merged_pr'",
		"payload.pull_number",
		"payload.head_sha",
		"payload.merge_commit_sha",
		"currentPull.state === 'closed'",
		"currentPull.merged === true",
		"typeof currentPull.merged_at === 'string'",
		"currentPull.base?.ref === 'main'",
		"isStableMergedPRIdentity(",
		"currentPull.head?.sha === headSha",
		"currentPull.merge_commit_sha === mergeCommitSha",
		"payload.source === 'failed_ci'",
		"payload.workflow_run_id",
		"payload.workflow_run_attempt",
		"payload.workflow_conclusion",
		"github.rest.actions.getWorkflow",
		"workflow_id: '.github/workflows/ci.yml'",
		"github.rest.actions.getWorkflowRun",
		"ciWorkflow.name !== 'CI'",
		"ciWorkflow.path !== '.github/workflows/ci.yml'",
		"currentRun.id !== workflowRunID",
		"currentRun.workflow_id !== ciWorkflow.id",
		"currentRun.name !== 'CI'",
		"currentRun.event !== 'push'",
		"currentRun.head_branch !== 'main'",
		"currentRun.head_sha !== targetSha",
		"currentRun.run_attempt !== workflowRunAttempt",
		"currentRun.status !== 'completed'",
		"currentRun.conclusion !== workflowConclusion",
		"coverage-baseline-repair payload has an unknown source",
		"context.ref !== 'refs/heads/main'",
		"targetSha = context.sha",
		"for (let attempt = 1; attempt <= 6; attempt += 1)",
		"basehead: `${targetSha}...${branch.commit.sha}`",
		"core.setOutput('target_sha', targetSha)",
		"actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683",
		"fetch-depth: 0",
		"persist-credentials: false",
		"ref: ${{ steps.resolve-target.outputs.target_sha }}",
		`run: test "$(git rev-parse HEAD)" = "$TARGET_SHA"`,
		"actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff",
		"id: target-cache",
		"actions/cache/restore@0057852bfaa89a56745cba8c7296529d2fc39830",
		"key: dws-coverage-full-v2-${{ steps.resolve-target.outputs.target_sha }}-go${{ steps.setup-go.outputs.go-version }}",
		"go test -count=1 -p 1",
		"./ ./cmd/... ./internal/... ./skills/...",
		"actions/cache/save@0057852bfaa89a56745cba8c7296529d2fc39830",
		"id: target-cache-verification",
		"lookup-only: true",
		"fail-on-cache-miss: true",
		"EXACT_CACHE_HIT: ${{ steps.target-cache-verification.outputs.cache-hit }}",
		`run: test "$EXACT_CACHE_HIT" = true`,
	} {
		if !strings.Contains(producer, want) {
			t.Errorf("trusted coverage repair producer missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"restore-keys",
		"github.event.pull_request.head.ref",
		"currentPull.base.sha",
		"base_sha",
		"HOMEBREW_PR_TOKEN",
		"RELEASE_GOVERNANCE_TOKEN",
		"REVIEWER_ROUTER_APP_PRIVATE_KEY",
		"contents: write",
	} {
		if strings.Contains(producer, forbidden) {
			t.Errorf("trusted coverage repair producer must not contain %q", forbidden)
		}
	}
	if strings.Count(producer, "path: coverage-cache.txt") != 3 {
		t.Error("coverage repair must restore, save, and independently verify one exact cache path/version")
	}
	if strings.Count(producer, "key: dws-coverage-full-v2-${{ steps.resolve-target.outputs.target_sha }}-go${{ steps.setup-go.outputs.go-version }}") != 3 {
		t.Error("coverage repair restore, save, and verification must share one exact target key")
	}
	stablePayloadIdentityCheck := strings.Index(producer, "if (!isStableMergedPRIdentity(")
	mainPayloadContainmentCheck := strings.Index(producer, "await requireMainContainment(targetSha)")
	resolvedTargetOutput := strings.Index(producer, "core.setOutput('target_sha', targetSha)")
	if stablePayloadIdentityCheck < 0 ||
		mainPayloadContainmentCheck <= stablePayloadIdentityCheck ||
		resolvedTargetOutput <= mainPayloadContainmentCheck {
		t.Error("merged-PR producer must prove stable payload identity and main containment before resolving checkout target")
	}
	validateIndex := strings.Index(producer, "name: Resolve trusted main repair target")
	checkoutIndex := strings.Index(producer, "name: Check out exact protected-main target")
	if validateIndex < 0 || checkoutIndex <= validateIndex {
		t.Error("repository_dispatch input must be fully bound to its protected-main source before checkout")
	}
}
