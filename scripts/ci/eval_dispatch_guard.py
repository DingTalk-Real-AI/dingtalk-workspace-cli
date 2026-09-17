#!/usr/bin/env python3
"""校验 `/eval` 派发的仓库权限与已审核 PR head。"""

import json
import os
import re
import sys


TRUSTED_PERMISSIONS = frozenset({"write", "maintain", "admin"})
FULL_SHA_RE = re.compile(r"^[0-9a-fA-F]{40}$")


def _read_api_response():
    try:
        value = json.load(sys.stdin)
    except (json.JSONDecodeError, OSError) as exc:
        raise ValueError(f"GitHub API response is not valid JSON: {exc}") from exc
    if not isinstance(value, dict):
        raise ValueError("GitHub API response must be a JSON object")
    return value


def load_allowlist(path: str) -> frozenset:
    """读取自助触发允许名单（默认分支文件；# 注释与空行忽略；大小写不敏感）。"""
    if not path:
        return frozenset()
    try:
        with open(path, encoding="utf-8") as f:
            lines = f.read().splitlines()
    except FileNotFoundError:
        return frozenset()
    entries = set()
    for line in lines:
        entry = line.split("#", 1)[0].strip()
        if entry:
            entries.add(entry.lower())
    return frozenset(entries)


def authorize_dispatch(response, commenter: str, pr_author: str, allowlist: frozenset):
    """两级授权：仓库写权限可派发任意 PR；名单内用户仅可派发自己创建的 PR。"""
    permission = response.get("permission")
    if permission in TRUSTED_PERMISSIONS:
        return f"maintainer:{permission}"
    commenter_key = (commenter or "").strip().lower()
    author_key = (pr_author or "").strip().lower()
    if commenter_key and commenter_key in allowlist:
        if commenter_key == author_key:
            return "allowlisted-author"
        raise PermissionError(
            f"{commenter or 'commenter'} is allowlisted for self-service only "
            "and may not dispatch other authors' PRs"
        )
    raise PermissionError(
        f"{commenter or 'commenter'} does not have write, maintain, or admin permission "
        "and is not an allowlisted PR author"
    )


def require_reviewed_current_head(response, expected_pr_number: str, reviewed_sha: str, commenter: str):
    try:
        expected_number = int(expected_pr_number)
    except (TypeError, ValueError) as exc:
        raise ValueError("expected PR number is invalid") from exc
    if response.get("number") != expected_number:
        raise ValueError("GitHub API response does not match the requested PR")
    if response.get("state") != "open":
        raise ValueError("PR is not open")

    head = response.get("head")
    current_sha = head.get("sha") if isinstance(head, dict) else ""
    if not FULL_SHA_RE.fullmatch(current_sha or ""):
        raise ValueError("GitHub API response does not contain a valid PR head SHA")

    if reviewed_sha:
        if not FULL_SHA_RE.fullmatch(reviewed_sha):
            raise ValueError("reviewed SHA must be a full 40-character commit SHA")
        if current_sha.lower() != reviewed_sha.lower():
            raise ValueError(
                f"PR head changed after review: reviewed {reviewed_sha.lower()}, "
                f"current {current_sha.lower()}"
            )
        return reviewed_sha.lower()

    # sha 省略：仅限评论者本人创建的 PR（自助模式）——作者对自己分支的
    # tip 背书不存在第三方偷换窗口；钉住派发时刻的当前 head，内网侧
    # FETCH_HEAD 校验继续兜底派发→执行窗口。评测他人 PR 必须显式带 sha=。
    author = ((response.get("user") or {}).get("login") or "").strip().lower()
    commenter_key = (commenter or "").strip().lower()
    if not author or commenter_key != author:
        raise ValueError(
            "dispatching another author's PR requires an explicit reviewed sha=<full-head-sha>"
        )
    return current_sha.lower()


def main() -> int:
    mode = sys.argv[1] if len(sys.argv) == 2 else ""
    try:
        response = _read_api_response()
        if mode == "permission":
            role = authorize_dispatch(
                response,
                os.environ.get("COMMENTER", ""),
                os.environ.get("PR_AUTHOR", ""),
                load_allowlist(os.environ.get("EVAL_ALLOWLIST_PATH", "")),
            )
            print(f"authorized={role}")
            return 0
        if mode == "head":
            head_sha = require_reviewed_current_head(
                response,
                os.environ.get("EXPECTED_PR_NUMBER", ""),
                os.environ.get("REVIEWED_SHA", ""),
                os.environ.get("COMMENTER", ""),
            )
            print(f"head_sha={head_sha}")
            return 0
        raise ValueError(f"unknown guard mode: {mode}")
    except (PermissionError, ValueError) as exc:
        print(str(exc), file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
