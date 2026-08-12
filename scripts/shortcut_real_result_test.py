#!/usr/bin/env python3
"""Self-checking tests for scripts/shortcut_real_result.py.

Run: python3 scripts/shortcut_real_result_test.py

Focus: the upper/lower layer comparison rules added after the
contact +list-roles / oa +list-forms projection-data-loss case, plus a
regression guard that pre-existing exit-code / error-envelope behaviour is
unchanged.
"""

from __future__ import annotations

import json

from shortcut_real_result import (
    backend_record_count,
    classify_failure,
    classify_real_status,
    compare_layers,
    count_projection_items,
    payload_indicates_error,
    projection_audit,
)

# Faithful backend shapes from the real case.
GROUPED_ROLES = {
    "result": [
        {"groupName": "默认", "labels": [{"labelId": 1, "name": "主管理员"}, {"labelId": 2, "name": "负责人"}]},
        {"groupName": "职务", "labels": [{"labelId": 3, "name": "财务"}]},
    ]
}
PROCESS_CODE_LIST = {
    "result": {
        "processCodeList": [
            {"processCode": "PROC-1", "processName": "请假"},
            {"processCode": "PROC-2", "processName": "加班"},
        ],
        "totalCount": -1,
    },
    "success": True,
}

EMPTY_ROLES_PROJECTION = json.dumps({"count": 0, "roles": []})
FULL_ROLES_PROJECTION = json.dumps({"count": 2, "roles": [{"labelId": 1}, {"labelId": 2}]})
DETAIL_PROJECTION = json.dumps({"userId": "u1", "name": "张三"})


def check(name, cond):
    if not cond:
        raise AssertionError(f"FAIL: {name}")
    print(f"  ok: {name}")


def test_count_projection_items():
    check("empty roles -> 0", count_projection_items(EMPTY_ROLES_PROJECTION) == 0)
    check("full roles -> 2", count_projection_items(FULL_ROLES_PROJECTION) == 2)
    check("detail (no list) -> None", count_projection_items(DETAIL_PROJECTION) is None)
    check("bare array -> len", count_projection_items("[1,2,3]") == 3)
    check("empty string -> None", count_projection_items("") is None)


def test_backend_record_count():
    # backend_record_count is a "has data" proxy = size of the largest single
    # object array found at any depth (not the flattened total).
    check("grouped roles has data (>0)", backend_record_count(GROUPED_ROLES) == 2)
    check("processCodeList count", backend_record_count(PROCESS_CODE_LIST) == 2)
    check("empty backend -> 0", backend_record_count({"result": []}) == 0)
    check("error envelope -> 0", backend_record_count({"errorCode": 1, "errorMessage": "x"}) == 0)


def test_compare_layers():
    lost, upper, lower = compare_layers(EMPTY_ROLES_PROJECTION, GROUPED_ROLES)
    check("roles: data loss detected", lost and upper == 0 and lower == 2)
    lost2, _, _ = compare_layers(FULL_ROLES_PROJECTION, GROUPED_ROLES)
    check("roles: no loss when upper populated", not lost2)
    lost3, _, lower3 = compare_layers(EMPTY_ROLES_PROJECTION, {"result": []})
    check("roles: no loss when backend also empty", not lost3 and lower3 == 0)


def test_classify_real_status():
    # Backward-compat: without backend_raw, empty projection is still real-ok.
    check(
        "legacy empty projection stays real-ok",
        classify_real_status(0, EMPTY_ROLES_PROJECTION) == "real-ok",
    )
    # With lower layer that has data, empty projection becomes real-error.
    check(
        "empty projection + backend data -> real-error",
        classify_real_status(0, EMPTY_ROLES_PROJECTION, backend_raw=GROUPED_ROLES) == "real-error",
    )
    # Populated projection stays ok even with backend.
    check(
        "full projection + backend -> real-ok",
        classify_real_status(0, FULL_ROLES_PROJECTION, backend_raw=GROUPED_ROLES) == "real-ok",
    )
    # Regression: non-zero exit and error envelope untouched.
    check("nonzero exit -> real-error", classify_real_status(5, "") == "real-error")
    check(
        "error envelope -> real-error",
        classify_real_status(0, json.dumps({"success": False, "error": "boom"})) == "real-error",
    )
    check("timeout preserved", classify_real_status(0, "", "timeout") == "timeout")


def test_classify_failure_projection_loss():
    result = {
        "status": "real-ok",  # process exited 0, looked fine
        "exit_code": 0,
        "service": "contact",
        "command": "+list-roles",
        "stdout": EMPTY_ROLES_PROJECTION,
        "backend_stdout": json.dumps(GROUPED_ROLES),
    }
    category, fixability, _ = classify_failure(result)
    check("projection-data-loss category", category == "projection-data-loss")
    check("cli fixable", fixability == "cli-projection-fix-needed")

    # No loss when projection is populated.
    ok_result = dict(result, stdout=FULL_ROLES_PROJECTION)
    check("populated -> passed", classify_failure(ok_result)[0] == "passed")

    # Regression: a normal real-ok with no backend capture still passes.
    check(
        "plain real-ok passes",
        classify_failure({"status": "real-ok", "exit_code": 0, "stdout": FULL_ROLES_PROJECTION})[0] == "passed",
    )


def test_projection_audit():
    # Empty projection, no backend -> must-verify warning.
    warn = projection_audit({"stdout": EMPTY_ROLES_PROJECTION})
    check("unverified empty flagged", warn is not None and warn["kind"] == "empty-projection-unverified")

    # Empty projection + backend data -> data loss.
    warn2 = projection_audit({"stdout": EMPTY_ROLES_PROJECTION, "backend": GROUPED_ROLES})
    check("data loss flagged", warn2 is not None and warn2["kind"] == "projection-data-loss")

    # Populated projection -> no warning.
    check("populated -> no warning", projection_audit({"stdout": FULL_ROLES_PROJECTION}) is None)

    # Detail (non-list) projection -> no warning.
    check("detail -> no warning", projection_audit({"stdout": DETAIL_PROJECTION}) is None)


def test_payload_indicates_error_regression():
    check("clean payload not error", not payload_indicates_error(FULL_ROLES_PROJECTION))
    check("errorCode!=0 is error", payload_indicates_error(json.dumps({"errorCode": 1})))
    check("errorCode==0 not error", not payload_indicates_error(json.dumps({"errorCode": 0})))


def test_record_run_wires_lower_layer():
    """End-to-end: record_real_shortcut_run.py must capture the lower layer and
    flag an exit-0 empty projection as projection-data-loss (not real-ok)."""
    import os
    import subprocess
    import tempfile

    here = os.path.dirname(os.path.abspath(__file__))
    recorder = os.path.join(here, "record_real_shortcut_run.py")
    with tempfile.TemporaryDirectory() as d:
        backend = os.path.join(d, "backend.json")
        out = os.path.join(d, "out.json")
        # Lower layer has 3 roles nested under result[].labels[].
        with open(backend, "w") as f:
            json.dump(GROUPED_ROLES, f)
        env = dict(os.environ, SHORTCUT_REAL_RESULTS_PATH=out)
        # Upper command: an empty projection (exit 0, no error envelope).
        argv = ["python3", "-c", "print('{\"count\": 0, \"roles\": []}')"]
        p = subprocess.run(
            ["python3", recorder, "--service", "contact", "--command", "+list-roles",
             "--risk", "read", "--backend-raw", backend, "--"] + argv,
            capture_output=True, text=True, env=env,
        )
        check("recorder exits non-zero on data loss", p.returncode == 2)
        rec = json.loads(p.stdout)
        check("status flipped to real-error", rec["status"] == "real-error")
        check("category is projection-data-loss", rec.get("failure_category") == "projection-data-loss")
        check("projection_audit attached", rec.get("projection_audit", {}).get("kind") == "projection-data-loss")
        # Raw backend payload must NOT be persisted.
        persisted = json.load(open(out))
        blob = json.dumps(persisted, ensure_ascii=False)
        check("raw lower-layer not persisted", "主管理员" not in blob and "labels" not in blob)


def main():
    tests = [
        test_count_projection_items,
        test_backend_record_count,
        test_compare_layers,
        test_classify_real_status,
        test_classify_failure_projection_loss,
        test_projection_audit,
        test_payload_indicates_error_regression,
        test_record_run_wires_lower_layer,
    ]
    for t in tests:
        print(f"{t.__name__}:")
        t()
    print(f"\nAll {len(tests)} test groups passed.")


if __name__ == "__main__":
    main()
