#!/usr/bin/env python3
"""Run every `dws schema` leaf command in dry-run mode.

The script treats `dws schema --format json` as the command inventory, expands
each leaf schema, synthesizes flag values from parameter metadata, and executes
the runnable CLI path with `--dry-run --yes --format json`.
"""

from __future__ import annotations

import argparse
import concurrent.futures
import json
import os
import re
import shlex
import subprocess
import tempfile
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


@dataclass
class SmokeResult:
    canonical_path: str
    cli_path: str
    command: list[str]
    status: str
    exit_code: int
    stdout: str
    stderr: str
    error: str = ""


def run_json(argv: list[str], cwd: Path, timeout: int) -> Any:
    proc = subprocess.run(
        argv,
        cwd=cwd,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=timeout,
    )
    if proc.returncode != 0:
        raise RuntimeError(
            f"{shlex.join(argv)} exited {proc.returncode}: "
            f"{(proc.stderr or proc.stdout).strip()}"
        )
    return json.loads(proc.stdout)


def kebab_to_words(value: str) -> str:
    return re.sub(r"[-_]+", " ", value.lower())


def scalar_value(flag: str, param: dict[str, Any], canonical_path: str) -> str:
    if "default" in param and param["default"] not in (None, ""):
        return str(param["default"])

    haystack = " ".join(
        [
            kebab_to_words(flag),
            kebab_to_words(str(param.get("property", ""))),
            str(param.get("description", "")).lower(),
        ]
    )

    if flag == "fields":
        return '[{"fieldName":"Name","type":"text"}]'
    if flag in {"records", "record"}:
        return '[{"cells":{"fld1":"value"}}]'
    if flag == "json":
        samples = {
            "aitable.view_update_aggregate": '{"fld1":"COUNT"}',
            "aitable.view_update_card": '{"displayFieldName":true}',
            "aitable.view_update_field_widths": '{"fld1":160}',
            "aitable.view_update_timebar": '{"startField":"fld_start"}',
        }
        if canonical_path in samples:
            return samples[canonical_path]
    if "json" in haystack:
        if "array" in haystack or "数组" in haystack:
            return "[]"
        return "{}"
    if flag == "config" or flag == "layout" or flag.endswith("-config"):
        return "{}"
    if flag == "scope":
        return "all"
    if flag == "format":
        return "excel"
    if "enabled" in haystack or "allow back" in haystack:
        return "true"
    if flag in {"to", "cc", "bcc", "from"}:
        return "user@example.com"
    if "url" in haystack or "callback" in haystack or "homepage" in haystack:
        return "https://example.com"
    if "email" in haystack or "mail" in haystack:
        return "user@example.com"
    if "mobile" in haystack or "phone" in haystack:
        return "13800138000"
    if "date" in haystack and "time" not in haystack:
        return "2026-07-09"
    if "time" in haystack or "deadline" in haystack:
        return "2026-07-09T10:00:00+08:00"
    if "file" in haystack or "path" in haystack or "attachment" in haystack or flag == "output":
        return str(Path(tempfile.gettempdir()) / "dws-schema-smoke-fixture.txt")
    if "confirm name" in haystack:
        return "SchemaSmoke"
    if flag in {"redirect-urls", "sso-urls"}:
        return "https://example.com"
    if flag == "ip-whitelist" or "ip" in haystack:
        return "192.0.2.10"
    if "secret" in haystack:
        return "test-secret"
    if "robot code" in haystack:
        return "robot-code-smoke"
    if flag == "mode" and any(item in haystack for item in ["https", "stream", "aiskill"]):
        return "STREAM"
    if flag == "share-type":
        return "PUBLIC"
    if flag in {"id", "parent-id", "dept-id"} or "dept id" in haystack:
        return "1"
    if "version" in haystack:
        return "1.0.0"
    if "content" in haystack or "message" in haystack or "desc" in haystack:
        return "schema smoke"
    if "name" in haystack or "title" in haystack or "subject" in haystack:
        return "SchemaSmoke"
    if "type" in haystack:
        return "app"
    return f"{flag}-smoke"


def value_for(flag: str, param: dict[str, Any], canonical_path: str) -> str | None:
    typ = str(param.get("type", "string")).lower()
    prop = str(param.get("property", ""))
    haystack = " ".join([kebab_to_words(flag), kebab_to_words(prop)])

    if typ == "boolean":
        return None
    if typ in {"integer", "number"}:
        return "1"
    if typ == "object":
        return "{}"
    if typ == "array":
        if "url" in haystack:
            return "https://example.com"
        if "ip" in haystack:
            return "192.0.2.10"
        if any(token in haystack for token in [" id", " ids", "user", "users", "field", "fields"]):
            return "value1,value2"
        if any(token in haystack for token in ["record", "records"]):
            return '[{"cells":{"fld1":"value"}}]'
        if any(token in haystack for token in ["sort"]):
            return "[]"
        if any(token in haystack for token in ["field", "fields"]):
            return "field1,field2"
        return "value1,value2"
    return scalar_value(flag, param, canonical_path)


def smoke_extra_flags(canonical_path: str) -> set[str]:
    # Avoid environment-dependent auto-detection in dry-run smoke. The command
    # accepts --email as optional, but without it the CLI probes the bound mailbox.
    return {
        "mail.search_mail_users": {"email"},
    }.get(canonical_path, set())


def build_command(binary: str, leaf: dict[str, Any], include_optional: bool) -> list[str]:
    cli_path = str(leaf["cli_path"])
    canonical_path = str(leaf.get("canonical_path", ""))
    extra_flags = smoke_extra_flags(canonical_path)
    argv = [binary, *shlex.split(cli_path)]
    params = leaf.get("parameters") or {}
    for flag in sorted(params):
        param = params[flag] or {}
        if not include_optional and not bool(param.get("required")) and flag not in extra_flags:
            continue
        if str(param.get("type", "")).lower() == "boolean":
            argv.append(f"--{flag}")
            continue
        value = value_for(flag, param, canonical_path)
        argv.extend([f"--{flag}", "" if value is None else value])
    argv.extend(["--dry-run", "--yes", "--format", "json"])
    return argv


def help_flags(binary: str, cwd: Path, cli_path: str, timeout: int) -> set[str]:
    proc = subprocess.run(
        [binary, *shlex.split(cli_path), "--help"],
        cwd=cwd,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=timeout,
    )
    if proc.returncode != 0:
        raise RuntimeError(
            f"{binary} {cli_path} --help exited {proc.returncode}: "
            f"{(proc.stderr or proc.stdout).strip()}"
        )
    return set(re.findall(r"--([a-zA-Z0-9][a-zA-Z0-9-]*)", proc.stdout))


def run_one(binary: str, cwd: Path, path: str, timeout: int, include_optional: bool) -> SmokeResult:
    try:
        leaf = run_json([binary, "schema", path, "--format", "json"], cwd, timeout)
        cli_path = str(leaf.get("cli_path", ""))
        declared = set((leaf.get("parameters") or {}).keys())
        missing = sorted(declared - help_flags(binary, cwd, cli_path, timeout))
        if missing:
            return SmokeResult(
                canonical_path=path,
                cli_path=cli_path,
                command=[binary, *shlex.split(cli_path), "--help"],
                status="schema_flag_mismatch",
                exit_code=3,
                stdout="",
                stderr="",
                error="schema parameters missing from command help: " + ", ".join(missing),
            )
        command = build_command(binary, leaf, include_optional)
        proc = subprocess.run(
            command,
            cwd=cwd,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            timeout=timeout,
        )
        status = "pass" if proc.returncode == 0 else "fail"
        return SmokeResult(
            canonical_path=path,
            cli_path=str(leaf.get("cli_path", "")),
            command=command,
            status=status,
            exit_code=proc.returncode,
            stdout=proc.stdout,
            stderr=proc.stderr,
        )
    except subprocess.TimeoutExpired as exc:
        return SmokeResult(
            canonical_path=path,
            cli_path="",
            command=exc.cmd if isinstance(exc.cmd, list) else [str(exc.cmd)],
            status="timeout",
            exit_code=124,
            stdout=exc.stdout or "",
            stderr=exc.stderr or "",
            error=str(exc),
        )
    except Exception as exc:  # noqa: BLE001 - record all smoke failures.
        return SmokeResult(
            canonical_path=path,
            cli_path="",
            command=[],
            status="error",
            exit_code=1,
            stdout="",
            stderr="",
            error=str(exc),
        )


def result_to_dict(result: SmokeResult) -> dict[str, Any]:
    return {
        "canonical_path": result.canonical_path,
        "cli_path": result.cli_path,
        "command": result.command,
        "status": result.status,
        "exit_code": result.exit_code,
        "stdout": result.stdout[-4000:],
        "stderr": result.stderr[-4000:],
        "error": result.error,
    }


def write_markdown(results: list[SmokeResult], output: Path) -> None:
    counts: dict[str, int] = {}
    for result in results:
        counts[result.status] = counts.get(result.status, 0) + 1

    lines = [
        "# Schema Command Smoke Results",
        "",
        f"- Generated: {datetime.now(timezone.utc).isoformat()}",
        f"- Total: {len(results)}",
    ]
    for status in sorted(counts):
        lines.append(f"- {status}: {counts[status]}")
    lines.extend(["", "## Failures", ""])

    failures = [r for r in results if r.status != "pass"]
    if not failures:
        lines.append("No failures.")
    else:
        lines.append("| canonical_path | cli_path | status | exit | issue |")
        lines.append("| --- | --- | --- | --- | --- |")
        for result in failures:
            issue = result.error or result.stderr.strip() or result.stdout.strip()
            issue = issue.replace("\n", "<br>")
            if len(issue) > 500:
                issue = issue[:500] + "..."
            lines.append(
                "| "
                + " | ".join(
                    [
                        f"`{result.canonical_path}`",
                        f"`{result.cli_path}`",
                        result.status,
                        str(result.exit_code),
                        issue or "-",
                    ]
                )
                + " |"
            )

    output.write_text("\n".join(lines) + "\n", encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", default="./dws")
    parser.add_argument("--output-jsonl", default="tmp/schema-command-smoke.jsonl")
    parser.add_argument("--output-md", default="tmp/schema-command-smoke.md")
    parser.add_argument("--jobs", type=int, default=8)
    parser.add_argument("--timeout", type=int, default=20)
    parser.add_argument("--include-optional", action="store_true")
    parser.add_argument("--path", action="append", help="canonical schema path to run; repeatable")
    args = parser.parse_args()

    cwd = Path.cwd()
    fixture = Path(tempfile.gettempdir()) / "dws-schema-smoke-fixture.txt"
    fixture.write_text("schema smoke fixture\n", encoding="utf-8")

    if args.path:
        paths = args.path
    else:
        listing = run_json([args.binary, "schema", "--format", "json"], cwd, args.timeout)
        paths = [tool["canonical_path"] for product in listing["products"] for tool in product["tools"]]

    with concurrent.futures.ThreadPoolExecutor(max_workers=args.jobs) as executor:
        results = list(
            executor.map(
                lambda p: run_one(args.binary, cwd, p, args.timeout, args.include_optional),
                paths,
            )
        )

    results.sort(key=lambda item: item.canonical_path)

    jsonl_path = Path(args.output_jsonl)
    jsonl_path.parent.mkdir(parents=True, exist_ok=True)
    with jsonl_path.open("w", encoding="utf-8") as fh:
        for result in results:
            fh.write(json.dumps(result_to_dict(result), ensure_ascii=False) + "\n")

    md_path = Path(args.output_md)
    md_path.parent.mkdir(parents=True, exist_ok=True)
    write_markdown(results, md_path)

    failures = [result for result in results if result.status != "pass"]
    print(f"total={len(results)}")
    print(f"failures={len(failures)}")
    print(f"jsonl={jsonl_path}")
    print(f"markdown={md_path}")
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
