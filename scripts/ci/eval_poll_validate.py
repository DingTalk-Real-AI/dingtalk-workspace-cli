#!/usr/bin/env python3
"""验证 GitHub Actions 产出的 eval-dispatch artifact 请求。

评论只是索引。真正的授权记录是绑定到精确 workflow run、run attempt 和
dispatch comment ID 的不可变 artifact。任何缺失、冲突或异常都必须拒绝，
不能让单条不可信评论终止轮询进程。校验成功时返回稳定的
idempotency_key；调用方必须在触发评测前持久化、原子占用该 key，重复
占用只能 no-op。本脚本无状态，不以 reaction 代替持久去重。
"""

from __future__ import annotations

import hashlib
import io
import json
import re
import stat
import subprocess
import sys
import zipfile
from typing import Any, Optional, Protocol

from eval_comment_parse import parse as parse_eval_comment


REPOSITORY = "DingTalk-Real-AI/dingtalk-workspace-cli"
REPOSITORY_ID = "1187709537"
WORKFLOW_ID = "331725458"
WORKFLOW_PATH = ".github/workflows/eval-dispatch.yml"
DEFAULT_BRANCH = "main"
TRUSTED_BOT_LOGIN = "github-actions[bot]"
TRUSTED_APP_SLUG = "github-actions"
ARTIFACT_FILENAME = "eval-dispatch-request.json"
SCHEMA_VERSION = 1

MAX_COMMENT_BYTES = 64 * 1024
MAX_API_JSON_BYTES = 1024 * 1024
MAX_ARCHIVE_BYTES = 64 * 1024
MAX_MANIFEST_BYTES = 64 * 1024
MAX_PRODUCTS_BYTES = 512
MAX_CASES_REF_BYTES = 1024

_SHA_RE = re.compile(r"^[0-9a-f]{40}$")
_SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
_DIGEST_RE = re.compile(r"^sha256:[0-9a-f]{64}$")
_PRODUCTS_RE = re.compile(r"^[a-z0-9][a-z0-9-]*(?:,[a-z0-9][a-z0-9-]*)*$")
_REF_CHARSET_RE = re.compile(r"^[A-Za-z0-9._/-]+$")
_LOGIN_RE = re.compile(r"^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$")
_MARKER_RE = re.compile(r"<!-- eval-dispatch: ([^\r\n]*?) -->")

_MARKER_FIELDS = {
    "schema_version",
    "repository_id",
    "workflow_id",
    "workflow_path",
    "run_id",
    "run_attempt",
    "dispatch_comment_id",
    "artifact_id",
    "artifact_digest",
}
_MANIFEST_FIELDS = {
    "schema_version",
    "repository_id",
    "repository",
    "workflow_id",
    "workflow_path",
    "run_id",
    "run_attempt",
    "source_comment_id",
    "dispatch_comment_id",
    "actor_id",
    "actor_login",
    "pr_number",
    "pr_head_sha",
    "products",
    "cases_ref",
    "source_body_sha256",
    "idempotency_key",
}


class GitHubClient(Protocol):
    """consumer 所需的最小 GitHub 只读边界。"""

    def get_run_attempt(self, run_id: str, run_attempt: str) -> Any: ...

    def get_artifact(self, artifact_id: str) -> Any: ...

    def download_artifact(self, artifact_id: str) -> bytes: ...

    def get_comment(self, comment_id: str) -> Any: ...

    def get_pull_request(self, pr_number: str) -> Any: ...


class GitHubCLIClient:
    """通过已认证的 gh CLI 读取 GitHub REST API。"""

    def _json(self, endpoint: str) -> Any:
        result = subprocess.run(
            ["gh", "api", endpoint],
            capture_output=True,
            text=True,
            timeout=20,
        )
        if result.returncode != 0:
            raise RuntimeError("GitHub API 请求失败")
        if len(result.stdout.encode("utf-8")) > MAX_API_JSON_BYTES:
            raise ValueError("GitHub API JSON 响应过大")
        return _loads_no_duplicates(result.stdout)

    def get_run_attempt(self, run_id: str, run_attempt: str) -> Any:
        return self._json(
            f"repos/{REPOSITORY}/actions/runs/{run_id}/attempts/{run_attempt}"
        )

    def get_artifact(self, artifact_id: str) -> Any:
        return self._json(f"repos/{REPOSITORY}/actions/artifacts/{artifact_id}")

    def download_artifact(self, artifact_id: str) -> bytes:
        result = subprocess.run(
            ["gh", "api", f"repos/{REPOSITORY}/actions/artifacts/{artifact_id}/zip"],
            capture_output=True,
            timeout=30,
        )
        if result.returncode != 0:
            raise RuntimeError("下载 GitHub artifact 失败")
        if len(result.stdout) > MAX_ARCHIVE_BYTES:
            raise ValueError("artifact 压缩包过大")
        return result.stdout

    def get_comment(self, comment_id: str) -> Any:
        return self._json(f"repos/{REPOSITORY}/issues/comments/{comment_id}")

    def get_pull_request(self, pr_number: str) -> Any:
        return self._json(f"repos/{REPOSITORY}/pulls/{pr_number}")


def _loads_no_duplicates(raw: str) -> Any:
    def reject_duplicates(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise ValueError(f"JSON 字段重复: {key}")
            result[key] = value
        return result

    return json.loads(raw, object_pairs_hook=reject_duplicates)


def _is_decimal_string(value: Any, *, max_length: int = 20) -> bool:
    return (
        type(value) is str
        and 1 <= len(value) <= max_length
        and value.isascii()
        and value.isdigit()
        and value[0] != "0"
    )


def _as_decimal_string(value: Any) -> Optional[str]:
    if type(value) is int and value > 0:
        return str(value)
    if _is_decimal_string(value):
        return value
    return None


def _is_valid_cases_ref(value: Any) -> bool:
    if type(value) is not str or len(value.encode("utf-8")) > MAX_CASES_REF_BYTES:
        return False
    if value == "":
        return True
    if not _REF_CHARSET_RE.fullmatch(value):
        return False
    if value.startswith(("-", "/")) or value.endswith(("/", ".")):
        return False
    if ".." in value or "//" in value:
        return False
    return all(
        component
        and not component.startswith(".")
        and not component.endswith(".lock")
        for component in value.split("/")
    )


def _is_string(value: Any, *, max_bytes: int, allow_empty: bool = False) -> bool:
    return (
        type(value) is str
        and (allow_empty or bool(value))
        and len(value.encode("utf-8")) <= max_bytes
    )


def validate_comment_author(comment: Any) -> bool:
    """校验 dispatch 评论由 GitHub Actions App 发出。"""
    if type(comment) is not dict:
        return False
    user = comment.get("user")
    app = comment.get("performed_via_github_app")
    return (
        type(user) is dict
        and user.get("login") == TRUSTED_BOT_LOGIN
        and user.get("type") == "Bot"
        and type(app) is dict
        and app.get("slug") == TRUSTED_APP_SLUG
    )


def _extract_marker(body: Any) -> Optional[dict[str, Any]]:
    if not _is_string(body, max_bytes=MAX_COMMENT_BYTES):
        return None
    if body.count("<!-- eval-dispatch:") != 1:
        return None
    matches = _MARKER_RE.findall(body)
    if len(matches) != 1 or len(matches[0].encode("utf-8")) > 8 * 1024:
        return None
    try:
        marker = _loads_no_duplicates(matches[0])
    except (json.JSONDecodeError, TypeError, ValueError):
        return None
    if type(marker) is not dict or set(marker) != _MARKER_FIELDS:
        return None
    if type(marker.get("schema_version")) is not int or marker["schema_version"] != SCHEMA_VERSION:
        return None
    for field, expected in (
        ("repository_id", REPOSITORY_ID),
        ("workflow_id", WORKFLOW_ID),
        ("workflow_path", WORKFLOW_PATH),
    ):
        if marker.get(field) != expected:
            return None
    for field in ("run_id", "run_attempt", "dispatch_comment_id", "artifact_id"):
        if not _is_decimal_string(marker.get(field)):
            return None
    if type(marker.get("artifact_digest")) is not str or not _DIGEST_RE.fullmatch(
        marker["artifact_digest"]
    ):
        return None
    return marker


def _validate_run(run: Any, marker: dict[str, Any]) -> bool:
    if type(run) is not dict:
        return False
    repository = run.get("repository")
    head_repository = run.get("head_repository")
    return (
        _as_decimal_string(run.get("id")) == marker["run_id"]
        and _as_decimal_string(run.get("run_attempt")) == marker["run_attempt"]
        and _as_decimal_string(run.get("workflow_id")) == WORKFLOW_ID
        and run.get("path") == WORKFLOW_PATH
        and run.get("event") == "issue_comment"
        and run.get("head_branch") == DEFAULT_BRANCH
        and run.get("status") == "completed"
        and run.get("conclusion") == "success"
        and type(repository) is dict
        and _as_decimal_string(repository.get("id")) == REPOSITORY_ID
        and repository.get("full_name") == REPOSITORY
        and type(head_repository) is dict
        and _as_decimal_string(head_repository.get("id")) == REPOSITORY_ID
        and head_repository.get("full_name") == REPOSITORY
        and type(run.get("head_sha")) is str
        and bool(_SHA_RE.fullmatch(run["head_sha"]))
    )


def _validate_artifact_metadata(
    artifact: Any, marker: dict[str, Any], run: dict[str, Any]
) -> bool:
    if type(artifact) is not dict:
        return False
    workflow_run = artifact.get("workflow_run")
    expected_name = (
        f"eval-dispatch-request-{marker['run_id']}-{marker['run_attempt']}-"
        f"{marker['dispatch_comment_id']}"
    )
    size = artifact.get("size_in_bytes")
    return (
        _as_decimal_string(artifact.get("id")) == marker["artifact_id"]
        and artifact.get("name") == expected_name
        and artifact.get("expired") is False
        and artifact.get("digest") == marker["artifact_digest"]
        and type(size) is int
        and 0 < size <= MAX_ARCHIVE_BYTES
        and type(workflow_run) is dict
        and _as_decimal_string(workflow_run.get("id")) == marker["run_id"]
        and _as_decimal_string(workflow_run.get("repository_id")) == REPOSITORY_ID
        and _as_decimal_string(workflow_run.get("head_repository_id")) == REPOSITORY_ID
        and workflow_run.get("head_sha") == run.get("head_sha")
    )


def _read_manifest(archive_bytes: Any, expected_digest: str) -> Optional[dict[str, Any]]:
    if type(archive_bytes) is not bytes or not 0 < len(archive_bytes) <= MAX_ARCHIVE_BYTES:
        return None
    actual_digest = "sha256:" + hashlib.sha256(archive_bytes).hexdigest()
    if actual_digest != expected_digest:
        return None
    try:
        with zipfile.ZipFile(io.BytesIO(archive_bytes), "r") as archive:
            infos = archive.infolist()
            if len(infos) != 1:
                return None
            info = infos[0]
            if info.filename != ARTIFACT_FILENAME or info.is_dir():
                return None
            if info.flag_bits & 0x1:
                return None
            if info.compress_type not in (zipfile.ZIP_STORED, zipfile.ZIP_DEFLATED):
                return None
            mode = (info.external_attr >> 16) & 0o177777
            file_type = stat.S_IFMT(mode)
            if file_type not in (0, stat.S_IFREG):
                return None
            if not 0 < info.file_size <= MAX_MANIFEST_BYTES:
                return None
            raw_manifest = archive.read(info)
    except (EOFError, OSError, RuntimeError, ValueError, zipfile.BadZipFile):
        return None
    if not 0 < len(raw_manifest) <= MAX_MANIFEST_BYTES:
        return None
    try:
        manifest = _loads_no_duplicates(raw_manifest.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError, TypeError, ValueError):
        return None
    if type(manifest) is not dict or set(manifest) != _MANIFEST_FIELDS:
        return None
    return manifest


def _validate_manifest_shape(manifest: dict[str, Any]) -> bool:
    if (
        type(manifest.get("schema_version")) is not int
        or manifest["schema_version"] != SCHEMA_VERSION
    ):
        return False
    for field, expected in (
        ("repository_id", REPOSITORY_ID),
        ("repository", REPOSITORY),
        ("workflow_id", WORKFLOW_ID),
        ("workflow_path", WORKFLOW_PATH),
    ):
        if manifest.get(field) != expected:
            return False
    for field in (
        "run_id",
        "run_attempt",
        "source_comment_id",
        "dispatch_comment_id",
        "actor_id",
        "pr_number",
    ):
        if not _is_decimal_string(manifest.get(field)):
            return False
    if type(manifest.get("actor_login")) is not str or not _LOGIN_RE.fullmatch(
        manifest["actor_login"]
    ):
        return False
    if type(manifest.get("pr_head_sha")) is not str or not _SHA_RE.fullmatch(
        manifest["pr_head_sha"]
    ):
        return False
    products = manifest.get("products")
    if (
        type(products) is not str
        or len(products.encode("utf-8")) > MAX_PRODUCTS_BYTES
        or not _PRODUCTS_RE.fullmatch(products)
    ):
        return False
    if not _is_valid_cases_ref(manifest.get("cases_ref")):
        return False
    if type(manifest.get("source_body_sha256")) is not str or not _SHA256_RE.fullmatch(
        manifest["source_body_sha256"]
    ):
        return False
    expected_key = f"{REPOSITORY_ID}:{manifest['source_comment_id']}"
    return manifest.get("idempotency_key") == expected_key


def _expected_issue_url(pr_number: str) -> str:
    return f"https://api.github.com/repos/{REPOSITORY}/issues/{pr_number}"


def _validate_source_comment(source: Any, manifest: dict[str, Any]) -> bool:
    if type(source) is not dict:
        return False
    user = source.get("user")
    body = source.get("body")
    if (
        _as_decimal_string(source.get("id")) != manifest["source_comment_id"]
        or source.get("issue_url") != _expected_issue_url(manifest["pr_number"])
        or type(user) is not dict
        or _as_decimal_string(user.get("id")) != manifest["actor_id"]
        or user.get("login") != manifest["actor_login"]
        or not _is_string(body, max_bytes=MAX_COMMENT_BYTES)
        or hashlib.sha256(body.encode("utf-8")).hexdigest()
        != manifest["source_body_sha256"]
    ):
        return False
    try:
        products, cases_ref, reviewed_sha = parse_eval_comment(body)
    except (TypeError, ValueError):
        return False
    return (
        products == manifest["products"]
        and cases_ref == manifest["cases_ref"]
        and (not reviewed_sha or reviewed_sha == manifest["pr_head_sha"])
    )


def _validate_run_actor(run: dict[str, Any], manifest: dict[str, Any]) -> bool:
    actor = run.get("actor")
    return (
        type(actor) is dict
        and _as_decimal_string(actor.get("id")) == manifest["actor_id"]
        and actor.get("login") == manifest["actor_login"]
    )


def _validate_pull_request(pr: Any, manifest: dict[str, Any]) -> bool:
    if type(pr) is not dict:
        return False
    head = pr.get("head")
    return (
        _as_decimal_string(pr.get("number")) == manifest["pr_number"]
        and pr.get("state") == "open"
        and type(head) is dict
        and head.get("sha") == manifest["pr_head_sha"]
    )


def _validate_comment(comment: Any, client: GitHubClient) -> Optional[dict[str, str]]:
    if not validate_comment_author(comment):
        return None
    dispatch_comment_id = _as_decimal_string(comment.get("id"))
    marker = _extract_marker(comment.get("body"))
    if dispatch_comment_id is None or marker is None:
        return None
    if marker["dispatch_comment_id"] != dispatch_comment_id:
        return None

    run = client.get_run_attempt(marker["run_id"], marker["run_attempt"])
    if not _validate_run(run, marker):
        return None

    artifact = client.get_artifact(marker["artifact_id"])
    if not _validate_artifact_metadata(artifact, marker, run):
        return None
    archive = client.download_artifact(marker["artifact_id"])
    manifest = _read_manifest(archive, marker["artifact_digest"])
    if manifest is None or not _validate_manifest_shape(manifest):
        return None

    for field in (
        "schema_version",
        "repository_id",
        "workflow_id",
        "workflow_path",
        "run_id",
        "run_attempt",
        "dispatch_comment_id",
    ):
        if manifest.get(field) != marker.get(field):
            return None
    if comment.get("issue_url") != _expected_issue_url(manifest["pr_number"]):
        return None
    if not _validate_run_actor(run, manifest):
        return None

    source_comment = client.get_comment(manifest["source_comment_id"])
    if not _validate_source_comment(source_comment, manifest):
        return None

    pr = client.get_pull_request(manifest["pr_number"])
    if not _validate_pull_request(pr, manifest):
        return None

    return {
        "pr_number": manifest["pr_number"],
        "pr_head_sha": manifest["pr_head_sha"],
        "products": manifest["products"],
        "cases_ref": manifest["cases_ref"],
        "idempotency_key": manifest["idempotency_key"],
    }


def validate_comment(
    comment: Any, client: Optional[GitHubClient] = None
) -> Optional[dict[str, str]]:
    """完整校验一条 dispatch 评论；调用方须原子占用返回的幂等键。"""
    try:
        return _validate_comment(comment, client or GitHubCLIClient())
    except Exception:
        return None


def main() -> int:
    try:
        comment = _loads_no_duplicates(sys.stdin.read())
    except (UnicodeDecodeError, json.JSONDecodeError, TypeError, ValueError):
        return 1
    result = validate_comment(comment)
    if result is None:
        return 1
    print(json.dumps(result, ensure_ascii=False, separators=(",", ":"), sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main())
