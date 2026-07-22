#!/bin/sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
. "$SCRIPT_DIR/release-lib.sh"
ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"

CHANNEL="${1:-}"
VERSION="${2:-}"
REMOTE=""
BRANCH="main"
FROM_BETA=""
PUBLISH=0
YES=0
OFFICIAL_TAGS_URL="${DWS_RELEASE_OFFICIAL_TAGS_URL:-https://github.com/DingTalk-Real-AI/dingtalk-workspace-cli.git}"

usage() {
  cat >&2 <<'EOF'
usage: release.sh <prerelease|stable> <version> [options]

Runs the full test, command-compatibility, package, and install preflight.
The default is validation only; add --publish to create and push the tag.

Options:
  --remote <name>       Required tag destination
  --from-beta <tag>     Required for stable releases
  --publish             Create and push the annotated release tag
  --yes                 Skip the interactive version confirmation
EOF
}

[ -n "$CHANNEL" ] && [ -n "$VERSION" ] || { usage; exit 2; }
shift 2
while [ "$#" -gt 0 ]; do
  case "$1" in
    --remote) [ "$#" -ge 2 ] || { usage; exit 2; }; REMOTE="$2"; shift 2 ;;
    --from-beta) [ "$#" -ge 2 ] || { usage; exit 2; }; FROM_BETA="$2"; shift 2 ;;
    --publish) PUBLISH=1; shift ;;
    --yes) YES=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) printf 'unknown argument: %s\n' "$1" >&2; usage; exit 2 ;;
  esac
done

release_validate_version_channel "$CHANNEL" "$VERSION"
[ -n "$REMOTE" ] || { printf '%s\n' '--remote is required' >&2; exit 2; }
cd "$ROOT"
fetch_url="$(git remote get-url "$REMOTE" 2>/dev/null)" || { printf 'unknown release remote: %s\n' "$REMOTE" >&2; exit 1; }
push_urls="$(git remote get-url --push --all "$REMOTE" 2>/dev/null)" || {
  printf 'could not resolve push URL for release remote: %s\n' "$REMOTE" >&2
  exit 1
}
[ "$(printf '%s\n' "$push_urls" | sed '/^$/d' | wc -l | tr -d '[:space:]')" -eq 1 ] || {
  printf 'release remote must have exactly one push URL: %s\n' "$REMOTE" >&2
  exit 1
}
push_url="$(printf '%s\n' "$push_urls" | sed -n '1p')"

repository_identity() {
  identity_url="$1"
  case "$identity_url" in
    https://github.com/*) identity_path="${identity_url#https://github.com/}" ;;
    http://github.com/*) identity_path="${identity_url#http://github.com/}" ;;
    git://github.com/*) identity_path="${identity_url#git://github.com/}" ;;
    git@github.com:*) identity_path="${identity_url#git@github.com:}" ;;
    ssh://git@github.com/*) identity_path="${identity_url#ssh://git@github.com/}" ;;
    *) printf '%s\n' "$identity_url"; return 0 ;;
  esac
  identity_path="${identity_path%/}"
  identity_path="${identity_path%.git}"
  printf 'github.com/%s\n' "$identity_path"
}

fetch_identity="$(repository_identity "$fetch_url")"
push_identity="$(repository_identity "$push_url")"
[ "$fetch_identity" = "$push_identity" ] || {
  printf 'release remote fetch and push URLs target different repositories:\n  fetch: %s\n  push:  %s\n' "$fetch_url" "$push_url" >&2
  exit 1
}
printf 'Release target: %s\n  fetch: %s\n  push:  %s\n' "$REMOTE" "$fetch_url" "$push_url"

github_repository_from_url() {
  github_identity="$(repository_identity "$1")"
  case "$github_identity" in
    github.com/*) printf '%s\n' "${github_identity#github.com/}" ;;
    *) return 1 ;;
  esac
}

require_delivered_previous_stable() {
  github_repository="$(github_repository_from_url "$push_url" 2>/dev/null || true)"
  if [ -z "$github_repository" ]; then
    [ "${DWS_RELEASE_ALLOW_NON_GITHUB_REMOTE:-0}" = "1" ] || {
      printf 'release validation requires a github.com remote to prove the stable baseline was delivered\n' >&2
      return 1
    }
    return 0
  fi
  [ -n "$previous_stable" ] || {
    printf 'a delivered previous stable baseline is required\n' >&2
    return 1
  }
  stable_commit="$(git rev-parse "$previous_stable^{commit}")"
  DWS_RELEASE_OFFICIAL_REPOSITORY="${DWS_RELEASE_OFFICIAL_REPOSITORY:-DingTalk-Real-AI/dingtalk-workspace-cli}" \
    "$SCRIPT_DIR/verify-delivered-stable.sh" "$previous_stable" "$stable_commit"
}

require_github_publication_authority() {
  github_repository="$(github_repository_from_url "$push_url" 2>/dev/null || true)"
  if [ -z "$github_repository" ]; then
    [ "${DWS_RELEASE_ALLOW_NON_GITHUB_REMOTE:-0}" = "1" ] || {
      printf 'publishing requires a github.com remote so CI and delivery authority can be verified\n' >&2
      return 1
    }
    printf 'warning: GitHub publication checks explicitly disabled for non-GitHub test remote\n' >&2
    return 0
  fi
  command -v gh >/dev/null 2>&1 || {
    printf 'gh is required to verify CI and release authority before publishing\n' >&2
    return 1
  }

  active_runs="$(
    gh api -H 'Accept: application/vnd.github+json' \
      "repos/$github_repository/actions/workflows/release.yml/runs?per_page=100" \
      --jq '.workflow_runs[] | select(.status != "completed") | [.id, .event, .status, .head_branch] | @tsv'
  )" || {
    printf 'could not query active Release workflow runs for %s\n' "$github_repository" >&2
    return 1
  }
  [ -z "$active_runs" ] || {
    printf 'another Release workflow is still active; wait for it before allocating a new tag:\n%s\n' "$active_runs" >&2
    return 1
  }

  sealed_commit="$(git rev-parse HEAD)"
  admission_check_runs="$(
    gh api -H 'Accept: application/vnd.github+json' \
      "repos/$github_repository/commits/$sealed_commit/check-runs?filter=latest&per_page=100" \
      --jq '.check_runs | group_by(.name) | map(max_by(.id)) | .[] | [.name, (.conclusion // .status // "unknown")] | @tsv'
  )" || {
    printf 'could not query Code Admission contexts for %s\n' "$sealed_commit" >&2
    return 1
  }

  missing_contexts=""
  non_success_contexts=""
  while IFS= read -r required_context; do
    context_state="$(
      printf '%s\n' "$admission_check_runs" |
        awk -F '\t' -v required="$required_context" '$1 == required { state = $2 } END { print state }'
    )"
    if [ -z "$context_state" ]; then
      missing_contexts="${missing_contexts}${missing_contexts:+, }$required_context"
    elif [ "$context_state" != "success" ]; then
      non_success_contexts="${non_success_contexts}${non_success_contexts:+, }$required_context=$context_state"
    fi
  done <<'ADMISSION_CONTEXTS'
Lint
Test
Coverage
Policy
Edition
Interface Integrity
AI Behavior
CLI Smoke
Mock MCP
ADMISSION_CONTEXTS
  [ -z "$missing_contexts" ] && [ -z "$non_success_contexts" ] || {
    printf 'Code Admission contexts are not all successful for sealed commit %s in %s; missing: %s; non-success: %s\n' \
      "$sealed_commit" \
      "$github_repository" \
      "${missing_contexts:-none}" \
      "${non_success_contexts:-none}" >&2
    return 1
  }

  if [ "$CHANNEL" = "stable" ]; then
    beta_state="$(
      gh api -H 'Accept: application/vnd.github+json' \
        "repos/$github_repository/releases/tags/$FROM_BETA" \
        --jq '[.tag_name, .draft, .prerelease, .immutable] | @tsv'
    )" || {
      printf 'could not query beta release %s in %s\n' "$FROM_BETA" "$github_repository" >&2
      return 1
    }
    [ "$beta_state" = "$(printf '%s\tfalse\ttrue\ttrue' "$FROM_BETA")" ] || {
      printf '%s must be a public immutable prerelease before stable tag allocation (got: %s)\n' "$FROM_BETA" "$beta_state" >&2
      return 1
    }
    beta_assets="$(
      gh api -H 'Accept: application/vnd.github+json' \
        "repos/$github_repository/releases/tags/$FROM_BETA" \
        --jq '.assets[].name'
    )" || return 1
    [ "$(printf '%s\n' "$beta_assets" | sed '/^$/d' | wc -l | tr -d '[:space:]')" -eq 8 ] || {
      printf 'beta release %s must contain exactly eight supported assets\n' "$FROM_BETA" >&2
      return 1
    }
    for beta_asset in \
      dws-darwin-amd64.tar.gz dws-darwin-arm64.tar.gz \
      dws-linux-amd64.tar.gz dws-linux-arm64.tar.gz \
      dws-windows-amd64.zip dws-windows-arm64.zip \
      dws-skills.zip checksums.txt; do
      [ "$(printf '%s\n' "$beta_assets" | grep -Fxc "$beta_asset")" -eq 1 ] || {
        printf 'beta release %s must contain %s exactly once\n' "$FROM_BETA" "$beta_asset" >&2
        return 1
      }
    done
    beta_commit="$(git rev-parse "$FROM_BETA^{commit}")"
    DWS_RELEASE_OFFICIAL_REPOSITORY="$github_repository" \
      "$SCRIPT_DIR/verify-release-workflow-delivery.sh" "$FROM_BETA" "$beta_commit" || return 1
  fi
}

require_release_governance_ci() {
  governance_repository="$(github_repository_from_url "$push_url" 2>/dev/null || true)"
  if [ -z "$governance_repository" ]; then
    [ "${DWS_RELEASE_ALLOW_NON_GITHUB_REMOTE:-0}" = "1" ] || {
      printf 'release governance preflight requires a github.com publication remote\n' >&2
      return 1
    }
    printf 'warning: Release governance CI preflight explicitly disabled for non-GitHub test remote\n' >&2
    return 0
  fi
  "$SCRIPT_DIR/verify-release-governance-ci.sh" \
    "$governance_repository" \
    "$(git rev-parse HEAD)"
}

preflight_proof_path() {
  proof_dir="$(git rev-parse --git-path dws-release-preflight)"
  printf '%s/%s.proof\n' "$proof_dir" "$VERSION"
}

preflight_proof_is_reusable() {
  proof_file="$1"
  [ -f "$proof_file" ] || return 1
  [ "$(wc -l < "$proof_file" | tr -d '[:space:]')" -eq 9 ] || return 1
  [ "$(sed -n 's/^format=//p' "$proof_file")" = "1" ] || return 1
  [ "$(sed -n 's/^channel=//p' "$proof_file")" = "$CHANNEL" ] || return 1
  [ "$(sed -n 's/^version=//p' "$proof_file")" = "$VERSION" ] || return 1
  [ "$(sed -n 's/^commit=//p' "$proof_file")" = "$(git rev-parse HEAD)" ] || return 1
  [ "$(sed -n 's/^repository=//p' "$proof_file")" = "$push_identity" ] || return 1
  [ "$(sed -n 's/^branch=//p' "$proof_file")" = "$BRANCH" ] || return 1
  [ "$(sed -n 's/^from_beta=//p' "$proof_file")" = "$FROM_BETA" ] || return 1
  [ "$(sed -n 's/^previous_stable=//p' "$proof_file")" = "$previous_stable" ] || return 1

  proof_created_at="$(sed -n 's/^created_at=//p' "$proof_file")"
  printf '%s\n' "$proof_created_at" | grep -Eq '^[0-9]+$' || return 1
  proof_now="$(date +%s)"
  [ "$proof_created_at" -le "$proof_now" ] || return 1
  [ $((proof_now - proof_created_at)) -le 21600 ] || return 1
}

write_preflight_proof() {
  proof_file="$1"
  proof_dir="$(dirname "$proof_file")"
  umask 077
  mkdir -p "$proof_dir"
  proof_tmp="${proof_file}.tmp.$$"
  {
    printf 'format=1\n'
    printf 'channel=%s\n' "$CHANNEL"
    printf 'version=%s\n' "$VERSION"
    printf 'commit=%s\n' "$(git rev-parse HEAD)"
    printf 'repository=%s\n' "$push_identity"
    printf 'branch=%s\n' "$BRANCH"
    printf 'from_beta=%s\n' "$FROM_BETA"
    printf 'previous_stable=%s\n' "$previous_stable"
    printf 'created_at=%s\n' "$(date +%s)"
  } > "$proof_tmp"
  mv "$proof_tmp" "$proof_file"
}

build_policy_binary() {
  policy_tmp="$ROOT/tmp/go-build"
  mkdir -p "$policy_tmp"
  GOTMPDIR="$policy_tmp" make build
  if [ "$(uname -s)" = "Darwin" ] && [ "${DWS_RELEASE_ALLOW_NON_GITHUB_REMOTE:-0}" != "1" ]; then
    command -v codesign >/dev/null 2>&1 || {
      printf '%s\n' 'codesign is required to prepare the macOS policy binary' >&2
      return 1
    }
    codesign --force --sign - "$ROOT/dws"
    codesign --verify --strict "$ROOT/dws"
  fi
}

fetch_release_tags() {
  git fetch --force "$REMOTE" \
    '+refs/tags/v*:refs/tags/v*' \
    '+refs/tags/withdrawn/v*:refs/tags/withdrawn/v*'
  git fetch --force --no-tags "$OFFICIAL_TAGS_URL" \
    '+refs/tags/v*:refs/tags/v*' \
    '+refs/tags/withdrawn/v*:refs/tags/withdrawn/v*'
}

printf '==> Refreshing %s/%s and release tags\n' "$REMOTE" "$BRANCH"
git fetch --force "$REMOTE" "+refs/heads/$BRANCH:refs/remotes/$REMOTE/$BRANCH"
fetch_release_tags

metadata="$(mktemp "${TMPDIR:-/tmp}/dws-release-metadata.XXXXXX")"
final_metadata="$(mktemp "${TMPDIR:-/tmp}/dws-release-final-metadata.XXXXXX")"
cleanup() { rm -f "$metadata" "$final_metadata"; }
trap cleanup EXIT HUP INT TERM

run_contract() {
  if [ -n "$FROM_BETA" ]; then
    "$SCRIPT_DIR/release-contract.sh" \
      --channel "$CHANNEL" \
      --version "$VERSION" \
      --context local \
      --remote "$REMOTE" \
      --branch "$BRANCH" \
      --from-beta "$FROM_BETA" \
      "$@"
  else
    "$SCRIPT_DIR/release-contract.sh" \
      --channel "$CHANNEL" \
      --version "$VERSION" \
      --context local \
      --remote "$REMOTE" \
      --branch "$BRANCH" \
      "$@"
  fi
}

run_contract --metadata-output "$metadata"
previous_stable="$(sed -n 's/^previous_stable=//p' "$metadata" | tail -1)"

printf '==> Verifying delivered stable baseline %s\n' "${previous_stable:-none}"
require_delivered_previous_stable

printf '==> Verifying GitHub publication authority before local gates\n'
require_github_publication_authority

proof_path="$(preflight_proof_path)"
ran_expensive_gates=0
if [ "$PUBLISH" -eq 1 ] && preflight_proof_is_reusable "$proof_path"; then
  printf '==> Reusing exact preflight proof for %s at %s\n' "$VERSION" "$(git rev-parse HEAD)"
else
  rm -f "$proof_path"
  printf '==> Running repository test, build, and policy gates\n'
  make test
  # Installer tests intentionally exercise source builds. Rebuild the release
  # binary before policy so policy never depends on a pre-existing ./dws.
  build_policy_binary
  make policy

  if [ -n "$previous_stable" ]; then
    printf '==> Comparing command tree with %s\n' "$previous_stable"
    "$ROOT/scripts/policy/check-command-compatibility.sh" \
      --base-ref HEAD \
      --stable-ref "$previous_stable"
  fi

  printf '==> Building local release artifacts for %s\n' "$VERSION"
  make package VERSION="$VERSION"

  printf '==> Verifying release artifact set and checksums\n'
  "$SCRIPT_DIR/verify-release-artifacts.sh" "$VERSION"

  printf '==> Verifying npm package installation\n'
  "$SCRIPT_DIR/verify-package-managers.sh" --npm-only --expected-version "$VERSION"
  if command -v brew >/dev/null 2>&1; then
    printf '==> Verifying Homebrew package installation\n'
    "$SCRIPT_DIR/verify-package-managers.sh" --brew-only --expected-version "$VERSION"
  fi
  ran_expensive_gates=1
fi

# Collect human confirmation before the final remote validations.
if [ "$PUBLISH" -eq 1 ] && [ "$YES" -ne 1 ]; then
  if [ ! -t 0 ]; then
    printf 'interactive confirmation is unavailable; pass --yes after reviewing the preflight\n' >&2
    exit 1
  fi
  printf 'Type %s to create and push the release tag: ' "$VERSION"
  IFS= read -r confirmation
  [ "$confirmation" = "$VERSION" ] || { printf 'release cancelled\n' >&2; exit 1; }
fi

printf '==> Verifying the Release workflow publication identity\n'
require_release_governance_ci

# Refresh after every local gate and the remote governance check. dist/ is
# ignored and therefore does not make the tree dirty.
printf '==> Refreshing %s/%s before sealing the tag\n' "$REMOTE" "$BRANCH"
git fetch --force "$REMOTE" "+refs/heads/$BRANCH:refs/remotes/$REMOTE/$BRANCH"
fetch_release_tags
run_contract --metadata-output "$final_metadata"
final_previous_stable="$(sed -n 's/^previous_stable=//p' "$final_metadata" | tail -1)"
[ -n "$final_previous_stable" ] || {
  printf 'latest stable authority unexpectedly became empty\n' >&2
  exit 1
}
previous_stable_before_refresh="$previous_stable"
previous_stable="$final_previous_stable"

printf '==> Revalidating delivered stable baseline %s\n' "$previous_stable"
require_delivered_previous_stable

if [ "$previous_stable" != "$previous_stable_before_refresh" ]; then
  printf '==> Stable authority advanced from %s to %s; rechecking command compatibility\n' \
    "${previous_stable_before_refresh:-none}" "$previous_stable"
  "$ROOT/scripts/policy/check-command-compatibility.sh" \
    --base-ref HEAD \
    --stable-ref "$previous_stable"
fi

if [ "$PUBLISH" -eq 1 ]; then
  printf '==> Reconfirming GitHub publication authority on the settled baseline\n'
  require_github_publication_authority
fi

# Delivery, compatibility, and publication checks above may take long enough
# for main or stable authority to move. This last refresh must be followed only
# by local proof/tag creation. Only the tag is pushed: the sealed commit is
# already contained in protected main history, so a later main advance never
# invalidates the release.
printf '==> Settling final %s/%s and stable authority\n' "$REMOTE" "$BRANCH"
git fetch --force "$REMOTE" "+refs/heads/$BRANCH:refs/remotes/$REMOTE/$BRANCH"
fetch_release_tags
run_contract --metadata-output "$final_metadata"
settled_previous_stable="$(sed -n 's/^previous_stable=//p' "$final_metadata" | tail -1)"
[ "$settled_previous_stable" = "$previous_stable" ] || {
  printf 'stable authority moved from %s to %s during final validation; rerun the release command\n' \
    "$previous_stable" "$settled_previous_stable" >&2
  exit 1
}

if [ "$ran_expensive_gates" -eq 1 ]; then
  write_preflight_proof "$proof_path"
fi

if [ "$PUBLISH" -ne 1 ]; then
  printf '\nPreflight passed. No tag was created.\n'
  printf 'Publish within six hours with the same command plus --publish to reuse this exact proof.\n'
  exit 0
fi

printf '==> Creating annotated tag %s\n' "$VERSION"
if [ "$CHANNEL" = "stable" ]; then
  git tag -a "$VERSION" -m "Release $VERSION" -m 'Channel: stable' -m "From-Beta: $FROM_BETA"
else
  git tag -a "$VERSION" -m "Release $VERSION" -m 'Channel: prerelease'
fi

if ! git push "$push_url" "refs/tags/$VERSION"; then
  set +e
  remote_refs="$(git ls-remote --tags "$push_url" "refs/tags/$VERSION" "refs/tags/$VERSION^{}")"
  query_status=$?
  set -e
  if [ "$query_status" -ne 0 ]; then
    printf 'tag push reported failure and remote state could not be verified; keeping local tag %s for investigation\n' "$VERSION" >&2
    exit 1
  fi
  remote_object="$(printf '%s\n' "$remote_refs" | awk '$2 !~ /\^\{\}$/ { print $1; exit }')"
  local_object="$(git rev-parse "refs/tags/$VERSION")"
  if [ -z "$remote_object" ]; then
    git tag -d "$VERSION" >/dev/null 2>&1 || true
    printf 'tag push failed; remote has no tag and the new local tag was removed: %s\n' "$VERSION" >&2
    exit 1
  fi
  if [ "$remote_object" != "$local_object" ]; then
    printf 'tag push failed and remote %s points elsewhere; keeping local tag for investigation\n' "$VERSION" >&2
    exit 1
  fi
  printf 'warning: push reported failure, but push target %s has the exact sealed tag; treating it as published\n' "$push_url" >&2
fi

printf 'Release tag pushed: %s -> %s. CI/CD now owns artifact publication.\n' "$VERSION" "$push_url"
rm -f "$proof_path"
