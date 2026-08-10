#!/bin/sh
# Mono↔multi skill content QA (G1–G5). See docs/skill-mono-multi-qa.md
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"

exec go test -count=1 ./test/unit -run 'TestMonoMultiSkillContent' 
