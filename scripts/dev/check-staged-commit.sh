#!/bin/sh

set -eu

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

printf '%s\n' 'Running staged pre-commit checks...'

# This reads only the index and reports whitespace errors without changing files.
git diff --cached --check

work_dir=$(mktemp -d "${TMPDIR:-/tmp}/dws-pre-commit.XXXXXX")
trap 'rm -rf "$work_dir"' EXIT HUP INT TERM

staged_go_files="$work_dir/staged-go-files"
git diff --cached --name-only --diff-filter=ACMR -z -- '*.go' >"$staged_go_files"

if [ ! -s "$staged_go_files" ]; then
	printf '%s\n' 'Staged pre-commit checks passed.'
	exit 0
fi

if ! command -v gofmt >/dev/null 2>&1; then
	printf '%s\n' 'gofmt is required to check staged Go files.' >&2
	exit 1
fi

if ! xargs -0 sh -c '
	work_dir=$1
	shift
	failed=0
	for path do
		original="$work_dir/original"
		formatted="$work_dir/formatted"
		if ! git show ":$path" >"$original"; then
			printf "Unable to read staged Go file: %s\n" "$path" >&2
			failed=1
			continue
		fi
		if ! gofmt <"$original" >"$formatted"; then
			printf "Unable to format staged Go file: %s\n" "$path" >&2
			failed=1
			continue
		fi
		if ! cmp -s "$original" "$formatted"; then
			printf "%s\n" "$path" >&2
			failed=1
		fi
	done
	exit "$failed"
' sh "$work_dir" <"$staged_go_files"; then
	printf '%s\n' 'Staged Go files are not formatted. Run gofmt, then stage them again.' >&2
	exit 1
fi

printf '%s\n' 'Staged pre-commit checks passed.'
