#!/bin/sh
set -eu

printf '%s\n' \
  'Direct Gitee artifact publication is disabled.' \
  'Use the guarded Release workflow so Gitee receives the exact immutable GitHub assets.' >&2
exit 2
