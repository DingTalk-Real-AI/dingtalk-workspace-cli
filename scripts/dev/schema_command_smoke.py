#!/usr/bin/env python3
"""Run every executable `dws schema --all` leaf in dry-run mode.

The script treats `dws schema --all --format json` as the command inventory, expands
each leaf schema, synthesizes flag values from parameter metadata, and executes
the runnable CLI path with `--dry-run --yes --format json`. Every available
combination of `require_one_of` branches is exercised independently.
"""

from __future__ import annotations

import argparse
import concurrent.futures
import itertools
import json
import os
import re
import shlex
import subprocess
import tempfile
import time
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


@dataclass
class SmokeResult:
    canonical_path: str
    case_name: str
    cli_path: str
    command: list[str]
    status: str
    exit_code: int
    stdout: str
    stderr: str
    error: str = ""


def should_retry_json_failure(output: str) -> bool:
    return any(
        token in output
        for token in [
            "TOKEN_VERIFIED_FAILED",
            "tools/list failed",
            "returned HTTP 400",
        ]
    )


def run_json(argv: list[str], cwd: Path, timeout: int, attempts: int = 3) -> Any:
    last_output = ""
    last_code = 1
    for attempt in range(attempts):
        proc = subprocess.run(
            argv,
            cwd=cwd,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            timeout=timeout,
        )
        if proc.returncode == 0:
            return json.loads(proc.stdout)
        last_code = proc.returncode
        last_output = (proc.stderr or proc.stdout).strip()
        if attempt + 1 < attempts and should_retry_json_failure(last_output):
            time.sleep(1 + attempt)
            continue
        break
    raise RuntimeError(f"{shlex.join(argv)} exited {last_code}: {last_output}")


def kebab_to_words(value: str) -> str:
    return re.sub(r"[-_]+", " ", value.lower())


def scalar_value(flag: str, param: dict[str, Any], canonical_path: str) -> str:
    if param.get("example") not in (None, ""):
        return str(param["example"])

    enum = param.get("enum") or []
    if enum:
        return str(enum[0])

    value_format = str(param.get("format", "")).lower()
    if value_format in {"numeric-id", "positive-integer"}:
        return "1"
    if value_format == "a1-range":
        return "A1:B2"
    if value_format == "file-path":
        return str(Path(tempfile.gettempdir()) / "dws-schema-smoke-fixture.txt")

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
    if canonical_path == "attendance.generateTurnSchedule" and flag == "scheduleVOS":
        return '[{"userId":"user-smoke","workDate":"2026-07-09 09:00:00","classId":1,"isRest":"N"}]'
    if canonical_path == "attendance.create_group_setting" and flag == "type":
        return "NONE"
    if canonical_path == "attendance.query_at_approve_template" and flag == "type":
        return "leave"
    if canonical_path == "attendance.save_self_setting" and flag == "setting-scene":
        return "checkResultNotify"
    if canonical_path == "attendance.get_attendance_summary" and flag == "date":
        return "2026-07-09 10:00:00"
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
    if canonical_path == "pat.batch_grant" and flag == "scope":
        return "aitable.record:read"
    if canonical_path == "sheet.range_batch_set_style" and flag == "batch":
        return str(Path(tempfile.gettempdir()) / "dws-schema-smoke-style.json")

    if param.get("example") not in (None, ""):
        return str(param["example"])

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


def constraint_groups(leaf: dict[str, Any], name: str) -> list[list[str]]:
    constraints = leaf.get("constraints") or {}
    groups = constraints.get(name) or []
    return [[str(item) for item in group] for group in groups if group]


def available_one_of_groups(leaf: dict[str, Any]) -> list[list[str]]:
    params = leaf.get("parameters") or {}
    positional_names = {
        str(item.get("name"))
        for item in (leaf.get("positionals") or [])
        if item.get("name")
    }
    available = set(params) | positional_names
    return [
        [name for name in group if name in available]
        for group in constraint_groups(leaf, "require_one_of")
        if any(name in available for name in group)
    ]


def one_of_cases(leaf: dict[str, Any]) -> list[tuple[str, ...]]:
    groups = available_one_of_groups(leaf)
    if not groups:
        return [()]
    cases: list[tuple[str, ...]] = []
    seen: set[tuple[str, ...]] = set()
    mutually_exclusive = constraint_groups(leaf, "mutually_exclusive")
    for choices in itertools.product(*groups):
        selected = set(choices)
        if any(sum(name in selected for name in group) > 1 for group in mutually_exclusive):
            continue
        if choices not in seen:
            seen.add(choices)
            cases.append(choices)
    return cases or [()]


def required_when_matches(
    expression: str,
    params: dict[str, Any],
    selected_params: set[str],
    canonical_path: str,
) -> bool:
    expression = expression.strip()
    value_match = re.fullmatch(r"([a-zA-Z0-9_.-]+) is (.+)", expression)
    if value_match:
        controller, expected_raw = value_match.groups()
        controller_param = params.get(controller) or {}
        if controller in selected_params:
            actual = value_for(controller, controller_param, canonical_path)
            actual_value = "true" if actual is None else str(actual)
        elif controller_param.get("default") not in (None, ""):
            actual_value = str(controller_param["default"])
        else:
            return False
        expected = [item.strip() for item in re.split(r"\s+or\s+", expected_raw)]
        return actual_value in expected

    any_flag_match = re.fullmatch(
        r"any ([a-zA-Z0-9_.-]+)\* flag is provided", expression
    )
    if any_flag_match:
        prefix = any_flag_match.group(1)
        return any(name.startswith(prefix) for name in selected_params)
    return False


def selected_inputs(
    leaf: dict[str, Any],
    include_optional: bool,
    one_of_choices: tuple[str, ...] = (),
) -> tuple[set[str], set[str]]:
    params = leaf.get("parameters") or {}
    canonical_path = str(leaf.get("canonical_path", ""))
    positionals = {
        str(item.get("name")): item
        for item in (leaf.get("positionals") or [])
        if item.get("name")
    }
    selected_params = {
        name for name, param in params.items()
        if include_optional or bool((param or {}).get("required"))
    }
    selected_positionals = {
        name for name, positional in positionals.items()
        if bool((positional or {}).get("required"))
    }

    def is_selected(name: str) -> bool:
        return name in selected_params or name in selected_positionals

    def select(name: str) -> bool:
        if name in params:
            selected_params.add(name)
            return True
        if name in positionals:
            selected_positionals.add(name)
            return True
        return False

    one_of_groups = available_one_of_groups(leaf)
    choices = one_of_choices or tuple(group[0] for group in one_of_groups)
    preferred = set(choices)
    for group, choice in zip(one_of_groups, choices):
        for name in group:
            selected_params.discard(name)
            selected_positionals.discard(name)
        select(choice)

    changed = True
    while changed:
        changed = False
        for group in constraint_groups(leaf, "require_together"):
            if not any(is_selected(name) for name in group):
                continue
            for name in group:
                if not is_selected(name) and select(name):
                    changed = True
        for name, param in params.items():
            if name in selected_params:
                continue
            required_when = str((param or {}).get("required_when", ""))
            if required_when_matches(
                required_when, params, selected_params, canonical_path
            ):
                selected_params.add(name)
                changed = True

    for group in constraint_groups(leaf, "mutually_exclusive"):
        selected = [name for name in group if is_selected(name)]
        keep = next((name for name in selected if name in preferred), selected[0] if selected else "")
        for name in selected:
            if name == keep:
                continue
            selected_params.discard(name)
            selected_positionals.discard(name)

    return selected_params, selected_positionals


def build_command(
    binary: str,
    leaf: dict[str, Any],
    include_optional: bool,
    one_of_choices: tuple[str, ...] = (),
) -> list[str]:
    cli_path = str(leaf["cli_path"])
    canonical_path = str(leaf.get("canonical_path", ""))
    extra_flags = smoke_extra_flags(canonical_path)
    argv = [binary, *shlex.split(cli_path)]
    params = leaf.get("parameters") or {}
    selected_params, selected_positionals = selected_inputs(
        leaf, include_optional, one_of_choices
    )

    positionals = sorted(
        (leaf.get("positionals") or []),
        key=lambda item: int(item.get("index", 0)),
    )
    for positional in positionals:
        name = str(positional.get("name", ""))
        if name not in selected_positionals:
            continue
        value = value_for(name, positional, canonical_path)
        argv.append("" if value is None else value)

    for flag in sorted(params):
        param = params[flag] or {}
        if flag not in selected_params and flag not in extra_flags:
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
    return set(re.findall(r"--([a-zA-Z0-9][a-zA-Z0-9_.-]*)", proc.stdout))


def run_smoke_case(
    binary: str,
    cwd: Path,
    path: str,
    leaf: dict[str, Any],
    timeout: int,
    include_optional: bool,
    choices: tuple[str, ...],
) -> SmokeResult:
    case_name = "default" if not choices else "one_of:" + "+".join(choices)
    try:
        command = build_command(binary, leaf, include_optional, choices)
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
            case_name=case_name,
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
            case_name=case_name,
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
            case_name=case_name,
            cli_path="",
            command=[],
            status="error",
            exit_code=1,
            stdout="",
            stderr="",
            error=str(exc),
        )


def run_one(
    binary: str,
    cwd: Path,
    path: str,
    timeout: int,
    include_optional: bool,
) -> list[SmokeResult]:
    try:
        leaf = run_json([binary, "schema", path, "--format", "json"], cwd, timeout)
        cli_path = str(leaf.get("cli_path", ""))
        declared = set((leaf.get("parameters") or {}).keys())
        missing = sorted(declared - help_flags(binary, cwd, cli_path, timeout))
        if missing:
            return [
                SmokeResult(
                    canonical_path=path,
                    case_name="schema",
                    cli_path=cli_path,
                    command=[binary, *shlex.split(cli_path), "--help"],
                    status="schema_flag_mismatch",
                    exit_code=3,
                    stdout="",
                    stderr="",
                    error="schema parameters missing from command help: "
                    + ", ".join(missing),
                )
            ]
        return [
            run_smoke_case(
                binary,
                cwd,
                path,
                leaf,
                timeout,
                include_optional,
                choices,
            )
            for choices in one_of_cases(leaf)
        ]
    except subprocess.TimeoutExpired as exc:
        return [
            SmokeResult(
                canonical_path=path,
                case_name="schema",
                cli_path="",
                command=exc.cmd if isinstance(exc.cmd, list) else [str(exc.cmd)],
                status="timeout",
                exit_code=124,
                stdout=exc.stdout or "",
                stderr=exc.stderr or "",
                error=str(exc),
            )
        ]
    except Exception as exc:  # noqa: BLE001 - record all smoke failures.
        return [
            SmokeResult(
                canonical_path=path,
                case_name="schema",
                cli_path="",
                command=[],
                status="error",
                exit_code=1,
                stdout="",
                stderr="",
                error=str(exc),
            )
        ]


def result_to_dict(result: SmokeResult) -> dict[str, Any]:
    return {
        "canonical_path": result.canonical_path,
        "case": result.case_name,
        "cli_path": result.cli_path,
        "command": result.command,
        "status": result.status,
        "exit_code": result.exit_code,
        "stdout": result.stdout[-4000:],
        "stderr": result.stderr[-4000:],
        "error": result.error,
    }


def write_markdown(results: list[SmokeResult], output: Path, tool_count: int) -> None:
    counts: dict[str, int] = {}
    for result in results:
        counts[result.status] = counts.get(result.status, 0) + 1

    lines = [
        "# Schema Command Smoke Results",
        "",
        f"- Generated: {datetime.now(timezone.utc).isoformat()}",
        f"- Tools: {tool_count}",
        f"- Cases: {len(results)}",
    ]
    for status in sorted(counts):
        lines.append(f"- {status}: {counts[status]}")
    lines.extend(["", "## Failures", ""])

    failures = [r for r in results if r.status != "pass"]
    if not failures:
        lines.append("No failures.")
    else:
        lines.append("| canonical_path | case | cli_path | status | exit | issue |")
        lines.append("| --- | --- | --- | --- | --- | --- |")
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
                        f"`{result.case_name}`",
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
    style_fixture = Path(tempfile.gettempdir()) / "dws-schema-smoke-style.json"
    style_fixture.write_text(
        '[{"sheetId":"sheet-smoke","range":"A1","fontSize":12}]\n',
        encoding="utf-8",
    )

    if args.path:
        paths = args.path
    else:
        listing = run_json([args.binary, "schema", "--all", "--format", "json"], cwd, args.timeout)
        paths = [tool["canonical_path"] for product in listing["products"] for tool in product["tools"]]

    with concurrent.futures.ThreadPoolExecutor(max_workers=args.jobs) as executor:
        result_groups = list(
            executor.map(
                lambda p: run_one(args.binary, cwd, p, args.timeout, args.include_optional),
                paths,
            )
        )
    results = [result for group in result_groups for result in group]

    results.sort(key=lambda item: (item.canonical_path, item.case_name))

    jsonl_path = Path(args.output_jsonl)
    jsonl_path.parent.mkdir(parents=True, exist_ok=True)
    with jsonl_path.open("w", encoding="utf-8") as fh:
        for result in results:
            fh.write(json.dumps(result_to_dict(result), ensure_ascii=False) + "\n")

    md_path = Path(args.output_md)
    md_path.parent.mkdir(parents=True, exist_ok=True)
    write_markdown(results, md_path, len(paths))

    failures = [result for result in results if result.status != "pass"]
    print(f"tools={len(paths)}")
    print(f"total={len(results)}")
    print(f"failures={len(failures)}")
    print(f"jsonl={jsonl_path}")
    print(f"markdown={md_path}")
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
