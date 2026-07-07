#!/usr/bin/env bash
# Copyright 2026 Alibaba Group
# Licensed under the Apache License, Version 2.0
#
# Localize README files for the Gitee mirror:
#   1) Replace raw.githubusercontent.com install URLs with Gitee raw URLs
#      (raw.githubusercontent.com is blocked or unreliable in mainland China).
#   2) Replace the in-repo coverage.svg badge with a shields.io static badge
#      (Gitee renders relative-path SVGs as text/plain, not inline).
#
# This script mutates README.md / README_zh.md in-place. The caller is
# responsible for committing the changes (if any) to a Gitee-only branch.
#
# Required environment:
#   GITEE_REPO  "owner/repo" on Gitee, e.g. DingTalk-Real-AI/dingtalk-workspace-cli

set -eu

: "${GITEE_REPO:?GITEE_REPO must be set}"

# 1) Install command localization: raw.githubusercontent → gitee raw.
for f in README.md README_zh.md; do
  [ -f "$f" ] || continue
  sed -i "s#raw.githubusercontent.com/${GITEE_REPO}/main#gitee.com/${GITEE_REPO}/raw/main#g" "$f"
done

# 2) Coverage badge: in-repo SVG → shields.io static badge.
SVG=".github/badges/coverage.svg"
if [ -f "$SVG" ]; then
  PCT="$(grep -oE '[0-9]+(\.[0-9]+)?%' "$SVG" | head -1)"
  NUM="${PCT%\%}"; INT="${NUM%.*}"
  if [ "${INT:-0}" -ge 80 ]; then C=brightgreen; elif [ "${INT:-0}" -ge 60 ]; then C=yellow; else C=red; fi
  BADGE="https://img.shields.io/badge/coverage-${NUM}%25-${C}"
  for f in README.md README_zh.md; do
    [ -f "$f" ] || continue
    sed -i "s#\.github/badges/coverage\.svg#${BADGE}#g" "$f"
  done
fi
