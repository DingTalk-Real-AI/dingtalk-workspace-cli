#!/bin/sh
set -eu

printf '%s\n' \
  'Direct Gitee release builds are disabled.' \
  'Use make release-pre/release-stable; release.yml mirrors the exact immutable GitHub assets.' >&2
exit 2
