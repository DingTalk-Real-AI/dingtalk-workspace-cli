#!/bin/sh
set -eu

# Validate CHANGELOG.md changes against the release contract.
#
# --fast-path additionally requires the complete PR diff to be exactly one
# in-place CHANGELOG.md modification. --content-only validates CHANGELOG.md
# while allowing other files to change in the same PR.

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"

usage() {
	printf '%s\n' "usage: $0 (--fast-path|--content-only) BASE HEAD" >&2
}

if [ "$#" -ne 3 ]; then
	usage
	exit 2
fi

case "$1" in
--fast-path)
	VALIDATION_MODE="fast-path"
	;;
--content-only)
	VALIDATION_MODE="content-only"
	;;
*)
	usage
	exit 2
	;;
esac
BASE_REF="$2"
HEAD_REF="$3"

cd "$ROOT"

BASE_COMMIT="$(git rev-parse --verify --quiet "${BASE_REF}^{commit}")" || {
	printf 'error: CHANGELOG base ref is not an available commit: %s\n' "$BASE_REF" >&2
	exit 2
}
HEAD_COMMIT="$(git rev-parse --verify --quiet "${HEAD_REF}^{commit}")" || {
	printf 'error: CHANGELOG head ref is not an available commit: %s\n' "$HEAD_REF" >&2
	exit 2
}

TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/dws-changelog-pr.XXXXXX")"
trap 'rm -rf "$TMP_ROOT"' EXIT HUP INT TERM

if ! git merge-base --all "$BASE_COMMIT" "$HEAD_COMMIT" >"$TMP_ROOT/merge-bases"; then
	printf 'error: CHANGELOG base and head commits have no merge base\n' >&2
	exit 2
fi
merge_base_count="$(wc -l <"$TMP_ROOT/merge-bases" | tr -d '[:space:]')"
if [ "$merge_base_count" -ne 1 ]; then
	printf 'error: expected exactly one merge base, found %s\n' "$merge_base_count" >&2
	exit 2
fi
IFS= read -r MERGE_BASE <"$TMP_ROOT/merge-bases"

require_regular_changelog() {
	_rrc_label="$1"
	_rrc_commit="$2"
	if ! git ls-tree "$_rrc_commit" -- CHANGELOG.md |
		awk '
			NR == 1 &&
				$1 == "100644" &&
				$2 == "blob" &&
				$4 == "CHANGELOG.md" {
				valid = 1
			}
			END {
				exit !(NR == 1 && valid)
			}
		'; then
		printf 'error: CHANGELOG.md must be a regular 100644 blob at %s\n' \
			"$_rrc_label" >&2
		exit 1
	fi
}

# The merge base is the effective base of the PR diff. Also validate the
# caller-provided base commit so a stale or malformed target ref fails closed.
require_regular_changelog "base" "$BASE_COMMIT"
if [ "$MERGE_BASE" != "$BASE_COMMIT" ]; then
	require_regular_changelog "merge base" "$MERGE_BASE"
fi
require_regular_changelog "head" "$HEAD_COMMIT"

git diff --no-ext-diff --find-renames --name-status \
	"$MERGE_BASE" "$HEAD_COMMIT" >"$TMP_ROOT/name-status"
case "$VALIDATION_MODE" in
fast-path)
	printf 'M\tCHANGELOG.md\n' >"$TMP_ROOT/expected-name-status"
	if ! cmp -s "$TMP_ROOT/expected-name-status" "$TMP_ROOT/name-status"; then
		printf '%s\n' 'error: fast path requires exactly one in-place modification: CHANGELOG.md' >&2
		if [ -s "$TMP_ROOT/name-status" ]; then
			sed 's/^/  /' "$TMP_ROOT/name-status" >&2
		else
			printf '%s\n' '  (no changed files)' >&2
		fi
		exit 1
	fi
	;;
content-only)
	if ! awk -F '	' '
		$1 == "M" && $2 == "CHANGELOG.md" && NF == 2 {
			count++
		}
		END {
			exit count != 1
		}
	' "$TMP_ROOT/name-status"; then
		printf '%s\n' 'error: content-only validation requires an in-place modification: CHANGELOG.md' >&2
		if [ -s "$TMP_ROOT/name-status" ]; then
			sed 's/^/  /' "$TMP_ROOT/name-status" >&2
		else
			printf '%s\n' '  (no changed files)' >&2
		fi
		exit 1
	fi
	;;
esac

if ! git diff --no-ext-diff --check "$MERGE_BASE" "$HEAD_COMMIT" -- CHANGELOG.md; then
	printf '%s\n' 'error: CHANGELOG diff contains whitespace errors' >&2
	exit 1
fi

"$ROOT/scripts/policy/open-source-audit.sh"

if ! git show "${MERGE_BASE}:CHANGELOG.md" >"$TMP_ROOT/base-changelog" ||
	! git show "${HEAD_COMMIT}:CHANGELOG.md" >"$TMP_ROOT/head-changelog"; then
	printf '%s\n' 'error: could not read CHANGELOG.md from both commits' >&2
	exit 2
fi

git diff --no-ext-diff --unified=0 \
	"$MERGE_BASE" "$HEAD_COMMIT" -- CHANGELOG.md >"$TMP_ROOT/changelog.patch"
if grep -q '^Binary files ' "$TMP_ROOT/changelog.patch"; then
	printf '%s\n' 'error: CHANGELOG.md must remain a text file' >&2
	exit 1
fi
if ! awk '
	/^\+\+\+ / { next }
	/^\+/ {
		line = substr($0, 2)
		if (line ~ /(^|[^[:alnum:]_])(TODO|TBD)([^[:alnum:]_]|$)/) {
			bad = 1
		}
	}
	END { exit bad }
' "$TMP_ROOT/changelog.patch"; then
	printf '%s\n' 'error: changed CHANGELOG additions must not contain TODO/TBD placeholders' >&2
	exit 1
fi

. "$ROOT/scripts/release/release-lib.sh"

extract_labeled_section() {
	_els_file="$1"
	_els_wanted="$2"
	awk -v wanted="$_els_wanted" '
		/^## / {
			active = 0
			if (substr($0, 1, 4) == "## [") {
				rest = substr($0, 5)
				close_pos = index(rest, "]")
				if (close_pos > 1 && substr(rest, 1, close_pos - 1) == wanted) {
					active = 1
				}
			}
		}
		active { print }
	' "$_els_file"
}

extract_unmanaged_content() {
	_euc_file="$1"
	awk '
		BEGIN { active = 1 }
		/^## / {
			active = 1
			if (substr($0, 1, 4) == "## [") {
				rest = substr($0, 5)
				close_pos = index(rest, "]")
				if (close_pos > 1) {
					active = 0
				}
			}
		}
		active { print }
	' "$_euc_file"
}

label_heading_count() {
	_lhc_file="$1"
	_lhc_wanted="$2"
	awk -v wanted="$_lhc_wanted" '
		/^## / && substr($0, 1, 4) == "## [" {
			rest = substr($0, 5)
			close_pos = index(rest, "]")
			if (close_pos > 1 && substr(rest, 1, close_pos - 1) == wanted) count++
		}
		END { print count + 0 }
	' "$_lhc_file"
}

calendar_date_is_valid() {
	_cdiv_date="$1"
	awk -v date="$_cdiv_date" 'BEGIN {
		if (date !~ /^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]$/) exit 1
		split(date, part, "-")
		year = part[1] + 0
		month = part[2] + 0
		day = part[3] + 0
		if (year < 1 || month < 1 || month > 12 || day < 1) exit 1
		days = 31
		if (month == 4 || month == 6 || month == 9 || month == 11) days = 30
		if (month == 2) {
			days = 28
			if ((year % 4 == 0 && year % 100 != 0) || year % 400 == 0) days = 29
		}
		exit day > days
	}'
}

release_heading_date() {
	_rhd_file="$1"
	_rhd_version="$2"
	awk -v wanted="$_rhd_version" '
		BEGIN { prefix = "## [" wanted "] - " }
		index($0, prefix) == 1 {
			print substr($0, length(prefix) + 1)
		}
	' "$_rhd_file"
}

validate_release_notes() {
	_vrn_file="$1"
	set +e
	awk '
		NR == 1 { next }
		{
			if ($0 ~ /[^[:space:]]/) meaningful = 1
			if ($0 ~ /^- /) bullet = 1
			if ($0 ~ /(^|[^[:alnum:]_])(TODO|TBD)([^[:alnum:]_]|$)/) placeholder = 1
		}
		END {
			if (!meaningful || !bullet) exit 43
			if (placeholder) exit 44
		}
	' "$_vrn_file"
	_vrn_status=$?
	set -e
	case "$_vrn_status" in
	0) return 0 ;;
	43) printf '%s\n' 'error: changed release section must contain notes and at least one bullet' >&2 ;;
	44) printf '%s\n' 'error: changed release section still contains TODO/TBD placeholders' >&2 ;;
	*) printf '%s\n' 'error: failed to validate changed release notes' >&2 ;;
	esac
	return "$_vrn_status"
}

extract_unmanaged_content "$TMP_ROOT/base-changelog" >"$TMP_ROOT/base-unmanaged"
extract_unmanaged_content "$TMP_ROOT/head-changelog" >"$TMP_ROOT/head-unmanaged"
if ! cmp -s "$TMP_ROOT/base-unmanaged" "$TMP_ROOT/head-unmanaged"; then
	printf '%s\n' 'error: CHANGELOG validation only permits notes inside Unreleased or versioned release sections' >&2
	exit 1
fi

unreleased_count="$(label_heading_count "$TMP_ROOT/head-changelog" Unreleased)"
unreleased_exact_count="$(awk '$0 == "## [Unreleased]" { count++ } END { print count + 0 }' \
	"$TMP_ROOT/head-changelog")"
if [ "$unreleased_count" -ne 1 ] || [ "$unreleased_exact_count" -ne 1 ]; then
	printf 'error: CHANGELOG must contain exactly one heading: ## [Unreleased]\n' >&2
	exit 1
fi

extract_labeled_section "$TMP_ROOT/base-changelog" Unreleased >"$TMP_ROOT/base-unreleased"
extract_labeled_section "$TMP_ROOT/head-changelog" Unreleased >"$TMP_ROOT/head-unreleased"
unreleased_changed=0
if ! cmp -s "$TMP_ROOT/base-unreleased" "$TMP_ROOT/head-unreleased"; then
	unreleased_changed=1
fi

awk '
	/^## / && substr($0, 1, 4) == "## [" {
		rest = substr($0, 5)
		close_pos = index(rest, "]")
		if (close_pos > 1) {
			label = substr(rest, 1, close_pos - 1)
			if (label != "Unreleased") print label
		}
	}
' "$TMP_ROOT/base-changelog" "$TMP_ROOT/head-changelog" |
	LC_ALL=C sort -u >"$TMP_ROOT/release-labels"

changed_release_count=0
section_index=0
while IFS= read -r version; do
	[ -n "$version" ] || continue
	section_index=$((section_index + 1))
	base_section="$TMP_ROOT/base-release-$section_index"
	head_section="$TMP_ROOT/head-release-$section_index"
	extract_labeled_section "$TMP_ROOT/base-changelog" "$version" >"$base_section"
	extract_labeled_section "$TMP_ROOT/head-changelog" "$version" >"$head_section"
	if cmp -s "$base_section" "$head_section"; then
		continue
	fi

	changed_release_count=$((changed_release_count + 1))
	if ! release_channel_for_version "v$version" >/dev/null 2>&1; then
		printf 'error: changed CHANGELOG release heading has an invalid version: %s\n' "$version" >&2
		exit 1
	fi
	if [ "$(label_heading_count "$TMP_ROOT/head-changelog" "$version")" -ne 1 ]; then
		printf 'error: CHANGELOG must contain exactly one well-formed section for %s\n' \
			"$version" >&2
		exit 1
	fi
	if ! validate_release_notes "$head_section"; then
		exit 1
	fi
	release_date="$(release_heading_date "$TMP_ROOT/head-changelog" "$version")"
	if ! calendar_date_is_valid "$release_date"; then
		printf 'error: CHANGELOG section for %s has an invalid calendar date: %s\n' \
			"$version" "$release_date" >&2
		exit 1
	fi
done <"$TMP_ROOT/release-labels"

if [ "$changed_release_count" -eq 0 ] && [ "$unreleased_changed" -eq 0 ]; then
	printf '%s\n' 'error: CHANGELOG modification did not change an eligible notes section' >&2
	exit 1
fi

printf 'CHANGELOG PR check: ok (mode=%s release_sections=%s unreleased_changed=%s)\n' \
	"$VALIDATION_MODE" "$changed_release_count" "$unreleased_changed"
