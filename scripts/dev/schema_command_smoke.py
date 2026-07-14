#!/usr/bin/env python3
"""Validate every full Schema leaf against Cobra Help and smoke runtime wiring.

The script loads ``dws schema --all --format json`` exactly once and uses those
full leaf payloads directly. Every leaf receives a bidirectional Schema/Help
flag contract check that excludes root execution controls and reviewed generic
payload escape hatches, but retains effective product/group flags. Runtime
execution is opt-in: only a leaf that publishes a valid ``dry_run`` object is
invoked with ``--dry-run``. Merely inheriting the global flag is never treated
as capability evidence, and ``--yes`` is never injected.

The subprocess result is only a final-binary exit-health check. It deliberately
does not infer preview evidence from human-readable text. The Go Agent-example
gate is the sole proof of preview kind, absence of real ToolCaller invocations,
and non-interactive safety.

Run the fast, binary-free self-tests with::

    python3 -m unittest scripts/dev/schema_command_smoke_test.py
"""

from __future__ import annotations

import argparse
import concurrent.futures
import itertools
import json
import re
import shlex
import subprocess
import tempfile
import time
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


GLOBAL_EXECUTION_CONTROL_FLAGS = frozenset(
    {
        "client-id",
        "client-secret",
        "debug",
        "dry-run",
        "fields",
        "format",
        "jq",
        "mock",
        "output",
        "profile",
        "timeout",
        "token",
        "verbose",
        "yes",
    }
)
COBRA_FRAMEWORK_FLAGS = frozenset({"help"})
DRY_RUN_PREVIEW_KINDS = frozenset({"invocation", "request", "plan", "diff"})
GENERIC_PAYLOAD_FLAG_USAGES = {
    "json": "Base JSON object payload for this tool invocation",
    "params": "Additional JSON object payload merged after --json",
}
HELP_FLAG_PATTERN = re.compile(
    r"^\s*(?:-[a-zA-Z0-9],\s*)?--([a-zA-Z0-9][a-zA-Z0-9_.-]*)(?:[ =]|$)"
)


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

    is_end = flag == "end" or flag.startswith("end-") or flag.endswith("-end")
    if value_format in {"numeric-id", "positive-integer"}:
        return "1"
    if value_format == "a1-range":
        return "A1:B2"
    if value_format == "file-path":
        return str(Path(tempfile.gettempdir()) / "dws-schema-smoke-fixture.txt")
    if value_format == "date-time":
        return "2027-01-15T11:00:00+08:00" if is_end else "2027-01-15T10:00:00+08:00"
    if value_format == "date":
        return "2027-01-16" if is_end else "2027-01-15"

    if "default" in param and param["default"] not in (None, ""):
        return str(param["default"])

    haystack = " ".join(
        [
            kebab_to_words(flag),
            kebab_to_words(str(param.get("property", ""))),
            str(param.get("description", "")).lower(),
        ]
    )

    if flag in {"start", "end", "start-time", "end-time"}:
        return "2027-01-15T11:00:00+08:00" if is_end else "2027-01-15T10:00:00+08:00"

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
    # Reviewed examples and declared enums are contract data. They must win
    # before type/name heuristics (notably integer enums such as mute-time).
    if param.get("example") not in (None, ""):
        return str(param["example"])
    enum = param.get("enum") or []
    if enum:
        return str(enum[0])

    if canonical_path == "sheet.range_batch_set_style" and flag == "batch":
        return str(Path(tempfile.gettempdir()) / "dws-schema-smoke-style.json")
    if canonical_path == "report.create_report" and flag == "contents-file":
        return "tmp/dws-schema-smoke-report.json"

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
        elif str(controller_param.get("type", "")).lower() == "boolean":
            actual_value = "false"
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
    if not declared_dry_run(leaf):
        raise ValueError("runtime smoke requires an explicit leaf dry_run capability")
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
        if flag == "yes":
            # A preview must prove that confirmation cannot be bypassed.
            continue
        if flag not in selected_params and flag not in extra_flags:
            continue
        if str(param.get("type", "")).lower() == "boolean":
            argv.append(f"--{flag}")
            continue
        value = value_for(flag, param, canonical_path)
        argv.extend([f"--{flag}", "" if value is None else value])
    # A positive leaf dry_run declaration is the only caller-side admission
    # condition. Confirmation must never be bypassed by injecting --yes, and
    # no output flag is appended because commands may own a same-named flag.
    argv.append("--dry-run")
    return argv


def effective_schema_flags_from_help(output: str) -> set[str]:
    """Return executable non-root flags from Cobra's rendered Help.

    Cobra renders a leaf's own flags under ``Flags`` and all persistent
    ancestors under ``Global Flags``. Schema intentionally excludes only the
    root execution controls; product/group persistent flags remain part of the
    leaf contract. Keeping section origin prevents a local business ``--format``
    flag from being confused with the root output-format control.
    """
    local_flags: set[str] = set()
    inherited_flags: set[str] = set()
    section = ""
    saw_flags_section = False
    for line in output.splitlines():
        heading = line.strip()
        if heading in {"Flags:", "Local Flags:"}:
            section = "local"
            saw_flags_section = True
            continue
        if heading in {"Global Flags:", "Inherited Flags:"}:
            section = "inherited"
            saw_flags_section = True
            continue
        if re.fullmatch(r"[A-Za-z][A-Za-z ]*:", heading):
            section = ""
            continue
        if not section:
            continue
        match = HELP_FLAG_PATTERN.match(line)
        if not match:
            continue
        name = match.group(1)
        generic_usage = GENERIC_PAYLOAD_FLAG_USAGES.get(name)
        if generic_usage and generic_usage in line:
            # These two generic transport escape hatches are intentionally not
            # projected as typed Schema parameters. Match their reviewed usage
            # text rather than excluding every business flag named json/params.
            continue
        if section == "local":
            local_flags.add(name)
        else:
            inherited_flags.add(name)

    if not saw_flags_section:
        raise RuntimeError("command help has no recognizable Flags section")
    return (
        local_flags
        | (inherited_flags - GLOBAL_EXECUTION_CONTROL_FLAGS)
    ) - COBRA_FRAMEWORK_FLAGS


def help_schema_flags(binary: str, cwd: Path, cli_path: str, timeout: int) -> set[str]:
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
    return effective_schema_flags_from_help(proc.stdout)


def run_smoke_case(
    binary: str,
    cwd: Path,
    path: str,
    leaf: dict[str, Any],
    timeout: int,
    include_optional: bool,
    choices: tuple[str, ...],
) -> SmokeResult:
    case_name = (
        "dry_run/default"
        if not choices
        else "dry_run/one_of:" + "+".join(choices)
    )
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
        # Exit zero proves only that the built command accepted and completed
        # the reviewed dry-run invocation. Preview/safety evidence belongs to
        # the fail-closed Go Agent-example gate, not text scraping here.
        status = (
            "runtime_exit_pass"
            if proc.returncode == 0
            else "runtime_exit_fail"
        )
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


def dry_run_capability_error(leaf: dict[str, Any]) -> str:
    """Return a closed-shape error for a leaf-local dry_run declaration."""
    if "dry_run" not in leaf:
        return ""
    capability = leaf["dry_run"]
    if not isinstance(capability, dict):
        return "dry_run must be an object"
    unknown_fields = sorted(set(capability) - {"preview_kind", "remote_reads"})
    if unknown_fields:
        return "dry_run has unsupported fields: " + ", ".join(unknown_fields)
    preview_kind = capability.get("preview_kind")
    if not isinstance(preview_kind, str) or preview_kind not in DRY_RUN_PREVIEW_KINDS:
        allowed = ", ".join(sorted(DRY_RUN_PREVIEW_KINDS))
        return f"dry_run.preview_kind must be one of: {allowed}"
    if "remote_reads" in capability and not isinstance(capability["remote_reads"], bool):
        return "dry_run.remote_reads must be a boolean"
    return ""


def declared_dry_run(leaf: dict[str, Any]) -> bool:
    """Return true only for a valid, positive leaf-local declaration."""
    problem = dry_run_capability_error(leaf)
    if problem:
        raise ValueError(problem)
    return "dry_run" in leaf


def run_one(
    binary: str,
    cwd: Path,
    leaf: dict[str, Any],
    timeout: int,
    include_optional: bool,
) -> list[SmokeResult]:
    path = str(leaf.get("canonical_path", ""))
    cli_path = str(leaf.get("cli_path", ""))
    help_command = [binary, *shlex.split(cli_path), "--help"]
    try:
        if not path or not cli_path:
            raise ValueError("full Schema leaf requires canonical_path and cli_path")
        parameters = leaf.get("parameters")
        if not isinstance(parameters, dict):
            raise ValueError("full Schema leaf parameters must be an object")
        declared = set(parameters)
        executable = help_schema_flags(binary, cwd, cli_path, timeout)
        schema_only = sorted(declared - executable)
        help_only = sorted(executable - declared)
        if schema_only or help_only:
            problems = []
            if schema_only:
                problems.append(
                    "Schema-only/non-effective flags: " + ", ".join(schema_only)
                )
            if help_only:
                problems.append(
                    "effective non-global Help flags missing from Schema: "
                    + ", ".join(help_only)
                )
            return [
                SmokeResult(
                    canonical_path=path,
                    case_name="contract",
                    cli_path=cli_path,
                    command=help_command,
                    status="schema_flag_mismatch",
                    exit_code=3,
                    stdout="",
                    stderr="",
                    error="; ".join(problems),
                )
            ]

        contract = SmokeResult(
            canonical_path=path,
            case_name="contract",
            cli_path=cli_path,
            command=help_command,
            status="contract_pass",
            exit_code=0,
            stdout="",
            stderr="",
        )
        capability_problem = dry_run_capability_error(leaf)
        if capability_problem:
            contract.status = "invalid_dry_run_capability"
            contract.exit_code = 3
            contract.error = capability_problem
            return [contract]
        if not declared_dry_run(leaf):
            return [contract]

        return [
            contract,
            *[
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
            ],
        ]
    except subprocess.TimeoutExpired as exc:
        return [
            SmokeResult(
                canonical_path=path,
                case_name="contract",
                cli_path=cli_path,
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
                case_name="contract",
                cli_path=cli_path,
                command=help_command,
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
        "phase": (
            "contract"
            if result.case_name == "contract"
            else "runtime_exit_health"
        ),
        "cli_path": result.cli_path,
        "command": result.command,
        "status": result.status,
        "exit_code": result.exit_code,
        "stdout": result.stdout[-4000:],
        "stderr": result.stderr[-4000:],
        "error": result.error,
    }


def load_schema_leaves(
    binary: str,
    cwd: Path,
    timeout: int,
    requested_paths: list[str] | None = None,
) -> list[dict[str, Any]]:
    """Load the full inventory once and optionally select canonical paths."""
    listing = run_json(
        [binary, "schema", "--all", "--format", "json"],
        cwd,
        timeout,
        attempts=1,
    )
    if not isinstance(listing, dict):
        raise RuntimeError("schema --all payload must be an object")
    products = listing.get("products")
    if not isinstance(products, list):
        raise RuntimeError("schema --all products must be an array")
    by_path: dict[str, dict[str, Any]] = {}
    for product_index, product in enumerate(products):
        if not isinstance(product, dict):
            raise RuntimeError(
                f"schema --all product {product_index} must be an object"
            )
        tools = product.get("tools")
        if not isinstance(tools, list):
            raise RuntimeError(
                f"schema --all product {product_index} tools must be an array"
            )
        for raw_leaf in tools:
            if not isinstance(raw_leaf, dict):
                raise RuntimeError("schema --all returned a non-object tool")
            raw_path = raw_leaf.get("canonical_path")
            if not isinstance(raw_path, str) or not raw_path.strip():
                raise RuntimeError("schema --all returned a tool without canonical_path")
            path = raw_path.strip()
            if path in by_path:
                raise RuntimeError(f"schema --all returned duplicate tool {path}")
            cli_path = raw_leaf.get("cli_path")
            if not isinstance(cli_path, str) or not cli_path.strip():
                raise RuntimeError(
                    f"schema --all tool {path} has no non-empty cli_path"
                )
            if "parameters" not in raw_leaf:
                raise RuntimeError(
                    f"schema --all tool {path} is a summary, not a full leaf"
                )
            if not isinstance(raw_leaf["parameters"], dict):
                raise RuntimeError(
                    f"schema --all full tool {path} parameters must be an object"
                )
            for parameter_name, parameter in raw_leaf["parameters"].items():
                if not isinstance(parameter, dict):
                    raise RuntimeError(
                        "schema --all full tool "
                        f"{path} parameter {parameter_name!r} must be an object"
                    )
            capability_problem = dry_run_capability_error(raw_leaf)
            if capability_problem:
                raise RuntimeError(
                    f"schema --all tool {path} has invalid dry_run: "
                    f"{capability_problem}"
                )
            by_path[path] = raw_leaf

    if not by_path:
        raise RuntimeError("schema --all returned no full tools")
    if not requested_paths:
        return [by_path[path] for path in sorted(by_path)]

    selected_paths = list(dict.fromkeys(requested_paths))
    missing = sorted(set(selected_paths) - set(by_path))
    if missing:
        raise RuntimeError(
            "requested canonical path missing from schema --all: " + ", ".join(missing)
        )
    return [by_path[path] for path in selected_paths]


def write_markdown(
    results: list[SmokeResult],
    output: Path,
    tool_count: int,
    runtime_tool_count: int,
) -> None:
    counts: dict[str, int] = {}
    for result in results:
        counts[result.status] = counts.get(result.status, 0) + 1

    lines = [
        "# Schema Command Smoke Results",
        "",
        f"- Generated: {datetime.now(timezone.utc).isoformat()}",
        f"- Tools: {tool_count}",
        f"- Contract checks: {sum(r.case_name == 'contract' for r in results)}",
        f"- Runtime exit-health tools: {runtime_tool_count}",
        f"- Runtime exit-health cases: {sum(r.case_name.startswith('dry_run/') for r in results)}",
        f"- Result rows: {len(results)}",
        "- Safety proof: Go Agent-example dry-run gate (not inferred here)",
    ]
    for status in sorted(counts):
        lines.append(f"- {status}: {counts[status]}")
    lines.extend(["", "## Non-passing cases", ""])

    failures = [
        r
        for r in results
        if r.status not in {"contract_pass", "runtime_exit_pass"}
    ]
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
    parser = argparse.ArgumentParser(
        description=(
            "Load full schema --all once, check every selected leaf against "
            "Cobra Help, and exit-health-check only explicit dry_run capabilities."
        )
    )
    parser.add_argument("--binary", default="./dws", help="dws binary to inspect")
    parser.add_argument("--output-jsonl", default="tmp/schema-command-smoke.jsonl")
    parser.add_argument("--output-md", default="tmp/schema-command-smoke.md")
    parser.add_argument(
        "--jobs", type=int, default=8, help="parallel Help/runtime-health workers"
    )
    parser.add_argument("--timeout", type=int, default=20, help="seconds per subprocess")
    parser.add_argument(
        "--include-optional",
        action="store_true",
        help="include optional parameters in explicit runtime exit-health cases",
    )
    parser.add_argument(
        "--path",
        action="append",
        help=(
            "canonical path selected from the single schema --all payload; "
            "repeatable"
        ),
    )
    args = parser.parse_args()

    cwd = Path.cwd()
    leaves = load_schema_leaves(
        args.binary, cwd, args.timeout, requested_paths=args.path
    )
    runtime_tool_count = sum(declared_dry_run(leaf) for leaf in leaves)
    if runtime_tool_count:
        fixture = Path(tempfile.gettempdir()) / "dws-schema-smoke-fixture.txt"
        fixture.write_text("schema smoke fixture\n", encoding="utf-8")
        style_fixture = Path(tempfile.gettempdir()) / "dws-schema-smoke-style.json"
        style_fixture.write_text(
            '[{"sheetId":"sheet-smoke","range":"A1","fontSize":12}]\n',
            encoding="utf-8",
        )
        report_fixture = cwd / "tmp" / "dws-schema-smoke-report.json"
        report_fixture.parent.mkdir(parents=True, exist_ok=True)
        report_fixture.write_text(
            '[{"content":"schema smoke","sort":"0","key":"work",'
            '"contentType":"markdown","type":"1"}]\n',
            encoding="utf-8",
        )

    with concurrent.futures.ThreadPoolExecutor(max_workers=args.jobs) as executor:
        result_groups = list(
            executor.map(
                lambda leaf: run_one(
                    args.binary, cwd, leaf, args.timeout, args.include_optional
                ),
                leaves,
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
    write_markdown(results, md_path, len(leaves), runtime_tool_count)

    accepted = {"contract_pass", "runtime_exit_pass"}
    failures = [result for result in results if result.status not in accepted]
    contract_results = [result for result in results if result.case_name == "contract"]
    runtime_results = [
        result for result in results if result.case_name.startswith("dry_run/")
    ]
    print(f"tools={len(leaves)}")
    print(
        "contract_pass="
        f"{sum(result.status == 'contract_pass' for result in contract_results)}"
    )
    print(
        "contract_failures="
        f"{sum(result.status != 'contract_pass' for result in contract_results)}"
    )
    print(f"runtime_exit_tools={runtime_tool_count}")
    print(f"runtime_exit_cases={len(runtime_results)}")
    print(
        "runtime_exit_failures="
        f"{sum(result.status != 'runtime_exit_pass' for result in runtime_results)}"
    )
    print(f"total={len(results)}")
    print(f"failures={len(failures)}")
    print(f"jsonl={jsonl_path}")
    print(f"markdown={md_path}")
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
