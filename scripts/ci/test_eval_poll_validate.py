"""eval-dispatch 消费端安全协议的无网络行为测试。"""

from __future__ import annotations

import copy
import hashlib
import io
import json
import os
import stat
import sys
import unittest
import warnings
import zipfile
from unittest import mock


sys.path.insert(0, os.path.dirname(__file__))

from eval_poll_validate import GitHubCLIClient, main as validate_main, validate_comment


REPOSITORY = "DingTalk-Real-AI/dingtalk-workspace-cli"
REPOSITORY_ID = "1187709537"
WORKFLOW_ID = "331725458"
WORKFLOW_PATH = ".github/workflows/eval-dispatch.yml"
RUN_ID = "31490000000"
RUN_ATTEMPT = "1"
SOURCE_COMMENT_ID = "5250000001"
DISPATCH_COMMENT_ID = "5250000002"
ARTIFACT_ID = "9100000001"
ACTOR_ID = "30925823"
ACTOR_LOGIN = "trusted-reviewer"
PR_NUMBER = "952"
PR_HEAD_SHA = "a" * 40
DEFAULT_BRANCH_SHA = "b" * 40
PRODUCTS = "drive,doc"
CASES_REF = "fixtures/v1"
SOURCE_BODY = f"/eval {PRODUCTS} sha={PR_HEAD_SHA} cases={CASES_REF}"
ARTIFACT_FILENAME = "eval-dispatch-request.json"


def _compact_json(value: dict) -> str:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"), sort_keys=True)


def _zip_single_file(name: str, content: bytes) -> bytes:
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_DEFLATED) as archive:
        archive.writestr(name, content)
    return output.getvalue()


def _zip_files(
    entries: tuple[tuple[str, bytes], ...], *, compression=zipfile.ZIP_DEFLATED
) -> bytes:
    output = io.BytesIO()
    with warnings.catch_warnings():
        warnings.simplefilter("ignore", UserWarning)
        with zipfile.ZipFile(output, "w", compression=compression) as archive:
            for name, content in entries:
                archive.writestr(name, content)
    return output.getvalue()


def _zip_symlink(name: str, target: str) -> bytes:
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w") as archive:
        info = zipfile.ZipInfo(name)
        info.create_system = 3
        info.external_attr = (stat.S_IFLNK | 0o777) << 16
        archive.writestr(info, target.encode("utf-8"))
    return output.getvalue()


def _set_marker(comment: dict, marker: dict) -> None:
    comment["body"] = f"<!-- eval-dispatch: {_compact_json(marker)} -->\n已受理。"


def _install_archive(comment: dict, client, marker: dict, archive: bytes) -> None:
    digest = "sha256:" + hashlib.sha256(archive).hexdigest()
    client.archive = archive
    client.artifact["size_in_bytes"] = len(archive)
    client.artifact["digest"] = digest
    marker["artifact_digest"] = digest
    _set_marker(comment, marker)


def _install_manifest(
    comment: dict, client, manifest: dict, marker: dict
) -> None:
    archive = _zip_single_file(
        ARTIFACT_FILENAME,
        (_compact_json(manifest) + "\n").encode("utf-8"),
    )
    _install_archive(comment, client, marker, archive)


def _set_nested(value: dict, path: tuple[str, ...], replacement) -> None:
    current = value
    for component in path[:-1]:
        current = current[component]
    current[path[-1]] = replacement


class FakeGitHubClient:
    """只模拟 consumer 的 GitHub 外部边界，不模拟内部校验步骤。"""

    def __init__(self, *, run, artifact, archive, source_comment, pr):
        self.run = run
        self.artifact = artifact
        self.archive = archive
        self.source_comment = source_comment
        self.pr = pr
        self.calls = []

    def get_run_attempt(self, run_id: str, run_attempt: str):
        self.calls.append(("get_run_attempt", run_id, run_attempt))
        return copy.deepcopy(self.run)

    def get_artifact(self, artifact_id: str):
        self.calls.append(("get_artifact", artifact_id))
        return copy.deepcopy(self.artifact)

    def download_artifact(self, artifact_id: str):
        self.calls.append(("download_artifact", artifact_id))
        return self.archive

    def get_comment(self, comment_id: str):
        self.calls.append(("get_comment", comment_id))
        return copy.deepcopy(self.source_comment)

    def get_pull_request(self, pr_number: str):
        self.calls.append(("get_pull_request", pr_number))
        return copy.deepcopy(self.pr)


def valid_fixture():
    manifest = {
        "schema_version": 1,
        "repository_id": REPOSITORY_ID,
        "repository": REPOSITORY,
        "workflow_id": WORKFLOW_ID,
        "workflow_path": WORKFLOW_PATH,
        "run_id": RUN_ID,
        "run_attempt": RUN_ATTEMPT,
        "source_comment_id": SOURCE_COMMENT_ID,
        "dispatch_comment_id": DISPATCH_COMMENT_ID,
        "actor_id": ACTOR_ID,
        "actor_login": ACTOR_LOGIN,
        "pr_number": PR_NUMBER,
        "pr_head_sha": PR_HEAD_SHA,
        "products": PRODUCTS,
        "cases_ref": CASES_REF,
        "source_body_sha256": hashlib.sha256(SOURCE_BODY.encode("utf-8")).hexdigest(),
        "idempotency_key": f"{REPOSITORY_ID}:{SOURCE_COMMENT_ID}",
    }
    archive = _zip_single_file(
        ARTIFACT_FILENAME,
        (_compact_json(manifest) + "\n").encode("utf-8"),
    )
    artifact_digest = "sha256:" + hashlib.sha256(archive).hexdigest()
    marker = {
        "schema_version": 1,
        "repository_id": REPOSITORY_ID,
        "workflow_id": WORKFLOW_ID,
        "workflow_path": WORKFLOW_PATH,
        "run_id": RUN_ID,
        "run_attempt": RUN_ATTEMPT,
        "dispatch_comment_id": DISPATCH_COMMENT_ID,
        "artifact_id": ARTIFACT_ID,
        "artifact_digest": artifact_digest,
    }
    issue_url = f"https://api.github.com/repos/{REPOSITORY}/issues/{PR_NUMBER}"
    comment = {
        "id": int(DISPATCH_COMMENT_ID),
        "issue_url": issue_url,
        "body": f"<!-- eval-dispatch: {_compact_json(marker)} -->\n已受理。",
        "user": {"login": "github-actions[bot]", "type": "Bot"},
        "performed_via_github_app": {"slug": "github-actions"},
    }
    run = {
        "id": int(RUN_ID),
        "run_attempt": int(RUN_ATTEMPT),
        "workflow_id": int(WORKFLOW_ID),
        "path": WORKFLOW_PATH,
        "event": "issue_comment",
        "head_branch": "main",
        "head_sha": DEFAULT_BRANCH_SHA,
        "status": "completed",
        "conclusion": "success",
        "repository": {"id": int(REPOSITORY_ID), "full_name": REPOSITORY},
        "head_repository": {"id": int(REPOSITORY_ID), "full_name": REPOSITORY},
        "actor": {"id": int(ACTOR_ID), "login": ACTOR_LOGIN},
    }
    artifact = {
        "id": int(ARTIFACT_ID),
        "name": f"eval-dispatch-request-{RUN_ID}-{RUN_ATTEMPT}-{DISPATCH_COMMENT_ID}",
        "size_in_bytes": len(archive),
        "expired": False,
        "digest": artifact_digest,
        "workflow_run": {
            "id": int(RUN_ID),
            "repository_id": int(REPOSITORY_ID),
            "head_repository_id": int(REPOSITORY_ID),
            "head_sha": DEFAULT_BRANCH_SHA,
        },
    }
    source_comment = {
        "id": int(SOURCE_COMMENT_ID),
        "issue_url": issue_url,
        "body": SOURCE_BODY,
        "user": {"id": int(ACTOR_ID), "login": ACTOR_LOGIN},
    }
    pr = {"number": int(PR_NUMBER), "state": "open", "head": {"sha": PR_HEAD_SHA}}
    client = FakeGitHubClient(
        run=run,
        artifact=artifact,
        archive=archive,
        source_comment=source_comment,
        pr=pr,
    )
    return comment, client, manifest, marker


class EvalPollValidateTests(unittest.TestCase):
    def assert_rejected(self, comment, client):
        self.assertIsNone(validate_comment(comment, client=client))

    def test_accepts_fully_bound_dispatch(self):
        comment, client, _, _ = valid_fixture()

        self.assertEqual(
            validate_comment(comment, client=client),
            {
                "pr_number": PR_NUMBER,
                "pr_head_sha": PR_HEAD_SHA,
                "products": PRODUCTS,
                "cases_ref": CASES_REF,
                "idempotency_key": f"{REPOSITORY_ID}:{SOURCE_COMMENT_ID}",
            },
        )
        self.assertEqual(
            client.calls,
            [
                ("get_run_attempt", RUN_ID, RUN_ATTEMPT),
                ("get_artifact", ARTIFACT_ID),
                ("download_artifact", ARTIFACT_ID),
                ("get_comment", SOURCE_COMMENT_ID),
                ("get_pull_request", PR_NUMBER),
            ],
        )

    def test_rejects_untrusted_or_malformed_dispatch_comment(self):
        mutations = (
            ("非字典", lambda comment: None),
            ("缺少用户", lambda comment: comment.pop("user")),
            ("伪造机器人", lambda comment: comment["user"].update(login="attacker")),
            ("用户类型错误", lambda comment: comment["user"].update(type="User")),
            ("缺少 GitHub App", lambda comment: comment.pop("performed_via_github_app")),
            (
                "GitHub App 错误",
                lambda comment: comment["performed_via_github_app"].update(
                    slug="untrusted-app"
                ),
            ),
            ("body 非字符串", lambda comment: comment.update(body=None)),
            ("comment id 非十进制", lambda comment: comment.update(id="01")),
            (
                "重复 marker",
                lambda comment: comment.update(body=comment["body"] + "\n" + comment["body"]),
            ),
        )
        for label, mutate in mutations:
            with self.subTest(label=label):
                comment, client, _, _ = valid_fixture()
                replacement = mutate(comment)
                if label == "非字典":
                    comment = replacement
                self.assert_rejected(comment, client)

    def test_rejects_marker_schema_type_and_binding_drift(self):
        mutations = (
            ("schema 版本", "schema_version", 2),
            ("schema 类型", "schema_version", "1"),
            ("仓库 ID", "repository_id", "999"),
            ("仓库 ID 类型", "repository_id", int(REPOSITORY_ID)),
            ("workflow ID", "workflow_id", "999"),
            ("workflow path", "workflow_path", ".github/workflows/other.yml"),
            ("run ID 类型", "run_id", int(RUN_ID)),
            ("attempt 前导零", "run_attempt", "01"),
            ("comment ID", "dispatch_comment_id", "999"),
            ("artifact ID 类型", "artifact_id", int(ARTIFACT_ID)),
            ("digest 无前缀", "artifact_digest", "0" * 64),
            ("digest 大写", "artifact_digest", "sha256:" + "A" * 64),
        )
        for label, field, value in mutations:
            with self.subTest(label=label):
                comment, client, _, marker = valid_fixture()
                marker[field] = value
                _set_marker(comment, marker)
                self.assert_rejected(comment, client)

        for label, mutate in (
            ("marker 缺字段", lambda marker: marker.pop("artifact_id")),
            ("marker 多字段", lambda marker: marker.update(payload={"products": "all"})),
        ):
            with self.subTest(label=label):
                comment, client, _, marker = valid_fixture()
                mutate(marker)
                _set_marker(comment, marker)
                self.assert_rejected(comment, client)

    def test_rejects_marker_copied_to_different_comment(self):
        comment, client, _, _ = valid_fixture()
        comment["id"] = int(DISPATCH_COMMENT_ID) + 1

        self.assert_rejected(comment, client)

    def test_emits_stable_idempotency_key_across_run_attempts(self):
        first_comment, first_client, _, _ = valid_fixture()
        first_result = validate_comment(first_comment, client=first_client)

        second_comment, second_client, manifest, marker = valid_fixture()
        second_attempt = "2"
        second_dispatch_comment_id = str(int(DISPATCH_COMMENT_ID) + 1)
        second_artifact_id = str(int(ARTIFACT_ID) + 1)
        second_comment["id"] = int(second_dispatch_comment_id)
        marker.update(
            run_attempt=second_attempt,
            dispatch_comment_id=second_dispatch_comment_id,
            artifact_id=second_artifact_id,
        )
        manifest.update(
            run_attempt=second_attempt,
            dispatch_comment_id=second_dispatch_comment_id,
        )
        second_client.run["run_attempt"] = int(second_attempt)
        second_client.artifact.update(
            id=int(second_artifact_id),
            name=(
                f"eval-dispatch-request-{RUN_ID}-{second_attempt}-"
                f"{second_dispatch_comment_id}"
            ),
        )
        _install_manifest(second_comment, second_client, manifest, marker)

        second_result = validate_comment(second_comment, client=second_client)

        self.assertIsNotNone(first_result)
        self.assertIsNotNone(second_result)
        self.assertEqual(
            first_result["idempotency_key"],
            second_result["idempotency_key"],
        )

    def test_rejects_cross_workflow_and_run_attempt_drift(self):
        mutations = (
            ("run id", ("id",), int(RUN_ID) + 1),
            ("run attempt", ("run_attempt",), int(RUN_ATTEMPT) + 1),
            ("workflow id", ("workflow_id",), int(WORKFLOW_ID) + 1),
            ("workflow path", ("path",), ".github/workflows/other.yml"),
            ("event", ("event",), "workflow_dispatch"),
            ("branch", ("head_branch",), "feature/eval"),
            ("status", ("status",), "in_progress"),
            ("conclusion", ("conclusion",), "failure"),
            ("repository id", ("repository", "id"), int(REPOSITORY_ID) + 1),
            ("repository name", ("repository", "full_name"), "attacker/fork"),
            (
                "head repository id",
                ("head_repository", "id"),
                int(REPOSITORY_ID) + 1,
            ),
            (
                "head repository name",
                ("head_repository", "full_name"),
                "attacker/fork",
            ),
            ("head sha 类型", ("head_sha",), None),
        )
        for label, path, value in mutations:
            with self.subTest(label=label):
                comment, client, _, _ = valid_fixture()
                _set_nested(client.run, path, value)
                self.assert_rejected(comment, client)

    def test_rejects_artifact_metadata_drift(self):
        mutations = (
            ("artifact id", ("id",), int(ARTIFACT_ID) + 1),
            ("artifact name", ("name",), "eval-dispatch-request-forged"),
            ("expired", ("expired",), True),
            ("expired 类型", ("expired",), 0),
            ("digest", ("digest",), "sha256:" + "0" * 64),
            ("size 零", ("size_in_bytes",), 0),
            ("size 类型", ("size_in_bytes",), "123"),
            (
                "workflow run id",
                ("workflow_run", "id"),
                int(RUN_ID) + 1,
            ),
            (
                "workflow repository id",
                ("workflow_run", "repository_id"),
                int(REPOSITORY_ID) + 1,
            ),
            (
                "workflow head repository id",
                ("workflow_run", "head_repository_id"),
                int(REPOSITORY_ID) + 1,
            ),
            ("workflow head sha", ("workflow_run", "head_sha"), "c" * 40),
        )
        for label, path, value in mutations:
            with self.subTest(label=label):
                comment, client, _, _ = valid_fixture()
                _set_nested(client.artifact, path, value)
                self.assert_rejected(comment, client)

        comment, client, _, _ = valid_fixture()
        client.artifact["workflow_run"] = None
        self.assert_rejected(comment, client)

    def test_rejects_download_digest_mismatch(self):
        comment, client, _, _ = valid_fixture()
        client.archive += b"tampered-after-digest"

        self.assert_rejected(comment, client)

    def test_rejects_tampered_manifest_payload(self):
        for field, value in (
            ("products", "sheet"),
            ("cases_ref", "fixtures/other"),
            ("pr_head_sha", "c" * 40),
        ):
            with self.subTest(field=field):
                comment, client, manifest, marker = valid_fixture()
                manifest[field] = value
                _install_manifest(comment, client, manifest, marker)
                self.assert_rejected(comment, client)

    def test_rejects_cross_workflow_historical_run_reuse(self):
        comment, client, manifest, marker = valid_fixture()
        historical_run_id = str(int(RUN_ID) - 100)
        marker["run_id"] = historical_run_id
        manifest["run_id"] = historical_run_id
        client.run["id"] = int(historical_run_id)
        client.run["workflow_id"] = int(WORKFLOW_ID) + 1
        client.artifact["name"] = (
            f"eval-dispatch-request-{historical_run_id}-{RUN_ATTEMPT}-"
            f"{DISPATCH_COMMENT_ID}"
        )
        client.artifact["workflow_run"]["id"] = int(historical_run_id)
        _install_manifest(comment, client, manifest, marker)

        self.assert_rejected(comment, client)

    def test_rejects_unsafe_or_ambiguous_zip(self):
        comment, client, manifest, marker = valid_fixture()
        manifest_bytes = (_compact_json(manifest) + "\n").encode("utf-8")
        unsafe_archives = (
            ("非 ZIP", b"not-a-zip"),
            (
                "路径穿越",
                _zip_single_file("../eval-dispatch-request.json", manifest_bytes),
            ),
            (
                "绝对路径",
                _zip_single_file("/eval-dispatch-request.json", manifest_bytes),
            ),
            (
                "多文件",
                _zip_files(
                    (
                        (ARTIFACT_FILENAME, manifest_bytes),
                        ("second.json", b"{}"),
                    )
                ),
            ),
            (
                "重复文件名",
                _zip_files(
                    (
                        (ARTIFACT_FILENAME, manifest_bytes),
                        (ARTIFACT_FILENAME, manifest_bytes),
                    )
                ),
            ),
            ("符号链接", _zip_symlink(ARTIFACT_FILENAME, "target.json")),
            (
                "不允许的压缩算法",
                _zip_files(
                    ((ARTIFACT_FILENAME, manifest_bytes),),
                    compression=zipfile.ZIP_BZIP2,
                ),
            ),
            (
                "解压后 manifest 超限",
                _zip_single_file(ARTIFACT_FILENAME, b"x" * (64 * 1024 + 1)),
            ),
            (
                "压缩包字节超限",
                _zip_files(
                    ((ARTIFACT_FILENAME, b"x" * (64 * 1024)),),
                    compression=zipfile.ZIP_STORED,
                ),
            ),
        )
        for label, archive in unsafe_archives:
            with self.subTest(label=label):
                test_comment, test_client, _, test_marker = valid_fixture()
                _install_archive(test_comment, test_client, test_marker, archive)
                self.assert_rejected(test_comment, test_client)

    def test_rejects_duplicate_manifest_json_keys(self):
        comment, client, manifest, marker = valid_fixture()
        raw_manifest = (
            _compact_json(manifest)[:-1] + ',"products":"sheet"}\n'
        ).encode("utf-8")
        archive = _zip_single_file(ARTIFACT_FILENAME, raw_manifest)
        _install_archive(comment, client, marker, archive)

        self.assert_rejected(comment, client)

    def test_rejects_duplicate_marker_json_keys(self):
        comment, client, _, marker = valid_fixture()
        raw_marker = _compact_json(marker)[:-1] + ',"run_id":"1"}'
        comment["body"] = f"<!-- eval-dispatch: {raw_marker} -->\n已受理。"

        self.assert_rejected(comment, client)

    def test_rejects_manifest_exact_field_and_type_violations(self):
        mutations = (
            ("schema 版本", lambda manifest: manifest.update(schema_version=2)),
            ("schema bool", lambda manifest: manifest.update(schema_version=True)),
            (
                "repository ID 类型",
                lambda manifest: manifest.update(repository_id=int(REPOSITORY_ID)),
            ),
            (
                "repository name",
                lambda manifest: manifest.update(repository="attacker/fork"),
            ),
            (
                "workflow ID 类型",
                lambda manifest: manifest.update(workflow_id=int(WORKFLOW_ID)),
            ),
            (
                "workflow path",
                lambda manifest: manifest.update(
                    workflow_path=".github/workflows/other.yml"
                ),
            ),
            ("run ID 类型", lambda manifest: manifest.update(run_id=int(RUN_ID))),
            ("attempt 前导零", lambda manifest: manifest.update(run_attempt="01")),
            ("source comment 零", lambda manifest: manifest.update(source_comment_id="0")),
            (
                "dispatch comment 超长",
                lambda manifest: manifest.update(dispatch_comment_id="1" * 21),
            ),
            ("actor ID 类型", lambda manifest: manifest.update(actor_id=int(ACTOR_ID))),
            ("actor login", lambda manifest: manifest.update(actor_login="bad_login")),
            ("actor login 超长", lambda manifest: manifest.update(actor_login="a" * 40)),
            ("PR number 类型", lambda manifest: manifest.update(pr_number=int(PR_NUMBER))),
            ("PR SHA 大写", lambda manifest: manifest.update(pr_head_sha="A" * 40)),
            ("products 空", lambda manifest: manifest.update(products="")),
            ("products 非法", lambda manifest: manifest.update(products="drive,Doc")),
            ("products 超长", lambda manifest: manifest.update(products="a" * 513)),
            (
                "source hash 带前缀",
                lambda manifest: manifest.update(
                    source_body_sha256="sha256:" + "0" * 64
                ),
            ),
            (
                "idempotency 前缀错误",
                lambda manifest: manifest.update(
                    idempotency_key=(
                        f"eval-dispatch:{REPOSITORY_ID}:{SOURCE_COMMENT_ID}"
                    )
                ),
            ),
            ("缺字段", lambda manifest: manifest.pop("products")),
            ("多字段", lambda manifest: manifest.update(payload={})),
        )
        for label, mutate in mutations:
            with self.subTest(label=label):
                comment, client, manifest, marker = valid_fixture()
                mutate(manifest)
                _install_manifest(comment, client, manifest, marker)
                self.assert_rejected(comment, client)

    def test_rejects_invalid_cases_refs(self):
        invalid_refs = (
            "-option",
            "/absolute",
            "trailing/",
            "trailing.",
            "a..b",
            "a//b",
            ".hidden",
            "a/.hidden",
            "a.lock",
            "a/b.lock",
            "contains space",
            "中文",
            "a" * 1025,
        )
        for cases_ref in invalid_refs:
            with self.subTest(cases_ref=cases_ref[:40]):
                comment, client, manifest, marker = valid_fixture()
                manifest["cases_ref"] = cases_ref
                _install_manifest(comment, client, manifest, marker)
                self.assert_rejected(comment, client)

    def test_accepts_empty_cases_ref(self):
        comment, client, manifest, marker = valid_fixture()
        source_body = f"/eval {PRODUCTS} sha={PR_HEAD_SHA}"
        manifest["cases_ref"] = ""
        manifest["source_body_sha256"] = hashlib.sha256(
            source_body.encode("utf-8")
        ).hexdigest()
        client.source_comment["body"] = source_body
        _install_manifest(comment, client, manifest, marker)

        result = validate_comment(comment, client=client)

        self.assertIsNotNone(result)
        self.assertEqual(result["cases_ref"], "")

    def test_rejects_source_comment_and_actor_drift(self):
        mutations = (
            ("source id", ("source_comment", "id"), int(SOURCE_COMMENT_ID) + 1),
            (
                "source issue URL",
                ("source_comment", "issue_url"),
                f"https://api.github.com/repos/{REPOSITORY}/issues/951",
            ),
            ("source user", ("source_comment", "user"), None),
            (
                "source actor id",
                ("source_comment", "user", "id"),
                int(ACTOR_ID) + 1,
            ),
            (
                "source actor login",
                ("source_comment", "user", "login"),
                "attacker",
            ),
            ("source body 类型", ("source_comment", "body"), None),
            ("source body hash", ("source_comment", "body"), SOURCE_BODY + "\nchanged"),
            ("run actor", ("run", "actor"), None),
            ("run actor id", ("run", "actor", "id"), int(ACTOR_ID) + 1),
            ("run actor login", ("run", "actor", "login"), "attacker"),
        )
        for label, path, value in mutations:
            with self.subTest(label=label):
                comment, client, _, _ = valid_fixture()
                target = client.source_comment if path[0] == "source_comment" else client.run
                _set_nested(target, path[1:], value)
                self.assert_rejected(comment, client)

    def test_rejects_source_command_payload_mismatch_even_with_matching_hash(self):
        comment, client, manifest, marker = valid_fixture()
        changed_body = f"/eval sheet sha={PR_HEAD_SHA} cases={CASES_REF}"
        client.source_comment["body"] = changed_body
        manifest["source_body_sha256"] = hashlib.sha256(
            changed_body.encode("utf-8")
        ).hexdigest()
        _install_manifest(comment, client, manifest, marker)

        self.assert_rejected(comment, client)

    def test_rejects_dispatch_issue_url_drift(self):
        comment, client, _, _ = valid_fixture()
        comment["issue_url"] = f"https://api.github.com/repos/{REPOSITORY}/issues/951"

        self.assert_rejected(comment, client)

    def test_rejects_pull_request_drift(self):
        mutations = (
            ("PR number", ("number",), int(PR_NUMBER) + 1),
            ("PR closed", ("state",), "closed"),
            ("PR head", ("head", "sha"), "c" * 40),
            ("PR head 缺失", ("head",), None),
        )
        for label, path, value in mutations:
            with self.subTest(label=label):
                comment, client, _, _ = valid_fixture()
                _set_nested(client.pr, path, value)
                self.assert_rejected(comment, client)

    def test_api_errors_and_invalid_response_shapes_fail_closed(self):
        def explode(*_args, **_kwargs):
            raise RuntimeError("simulated API failure")

        for method_name in (
            "get_run_attempt",
            "get_artifact",
            "download_artifact",
            "get_comment",
            "get_pull_request",
        ):
            with self.subTest(method=method_name):
                comment, client, _, _ = valid_fixture()
                setattr(client, method_name, explode)
                self.assert_rejected(comment, client)

        for label, attribute, value in (
            ("run list", "run", []),
            ("artifact null", "artifact", None),
            ("archive text", "archive", "not bytes"),
            ("source list", "source_comment", []),
            ("PR list", "pr", []),
        ):
            with self.subTest(label=label):
                comment, client, _, _ = valid_fixture()
                setattr(client, attribute, value)
                self.assert_rejected(comment, client)

    def test_cli_client_uses_attempt_and_artifact_id_endpoints(self):
        client = GitHubCLIClient()
        with mock.patch("eval_poll_validate.subprocess.run") as run:
            run.return_value = mock.Mock(returncode=0, stdout="{}")
            client.get_run_attempt(RUN_ID, RUN_ATTEMPT)
            run.assert_called_once_with(
                [
                    "gh",
                    "api",
                    (
                        f"repos/{REPOSITORY}/actions/runs/{RUN_ID}/"
                        f"attempts/{RUN_ATTEMPT}"
                    ),
                ],
                capture_output=True,
                text=True,
                timeout=20,
            )

        with mock.patch("eval_poll_validate.subprocess.run") as run:
            run.return_value = mock.Mock(returncode=0, stdout=b"zip")
            self.assertEqual(client.download_artifact(ARTIFACT_ID), b"zip")
            run.assert_called_once_with(
                [
                    "gh",
                    "api",
                    f"repos/{REPOSITORY}/actions/artifacts/{ARTIFACT_ID}/zip",
                ],
                capture_output=True,
                timeout=30,
            )

    def test_main_rejects_duplicate_stdin_json_keys_without_network(self):
        with mock.patch(
            "eval_poll_validate.sys.stdin",
            io.StringIO('{"body":{},"body":{}}'),
        ), mock.patch("eval_poll_validate.subprocess.run") as run:
            self.assertEqual(validate_main(), 1)
            run.assert_not_called()


if __name__ == "__main__":
    unittest.main(verbosity=2)
