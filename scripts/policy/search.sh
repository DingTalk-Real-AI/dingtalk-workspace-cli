#!/bin/sh

# Portable source searches for policy checks. GitHub-hosted runners do not
# guarantee that ripgrep is installed, so policy must depend only on POSIX
# tools that are already required by the repository build.

policy_search_paths() {
	pattern="$1"
	shift
	output="$(
		for path in "$@"; do
			if [ -d "$path" ]; then
				find "$path" -type f -exec grep -HnE "$pattern" {} + 2>/dev/null || true
			elif [ -f "$path" ]; then
				grep -HnE "$pattern" "$path" 2>/dev/null || true
			fi
		done
	)"
	[ -n "$output" ] || return 1
	printf '%s\n' "$output"
}

policy_search_go() {
	pattern="$1"
	shift
	output="$(
		for path in "$@"; do
			if [ -d "$path" ]; then
				find "$path" -type f -name '*.go' -exec grep -HnE "$pattern" {} + 2>/dev/null || true
			elif [ -f "$path" ] && [ "${path%.go}" != "$path" ]; then
				grep -HnE "$pattern" "$path" 2>/dev/null || true
			fi
		done
	)"
	[ -n "$output" ] || return 1
	printf '%s\n' "$output"
}

policy_search_production_go() {
	pattern="$1"
	shift
	output="$(
		for path in "$@"; do
			if [ -d "$path" ]; then
				find "$path" -type f -name '*.go' ! -name '*_test.go' -exec grep -HnE "$pattern" {} + 2>/dev/null || true
			elif [ -f "$path" ] &&
				[ "${path%.go}" != "$path" ] &&
				[ "${path%_test.go}" = "$path" ]; then
				grep -HnE "$pattern" "$path" 2>/dev/null || true
			fi
		done
	)"
	[ -n "$output" ] || return 1
	printf '%s\n' "$output"
}
