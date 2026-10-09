#!/usr/bin/env python3
"""Exercise a packaged CLI against localhost OAuth/MCP fixtures, without real credentials."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True)
    args = parser.parse_args()
    command = [str(Path(args.binary).resolve())]
    calls = []
    errors = []
    state = {"app": "official-app", "bad_identity": False}

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass

        def respond(self, value):
            data = json.dumps(value).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def do_GET(self):
            calls.append("discover")
            if self.path != "/cli/clientId":
                errors.append("unexpected discovery path")
            self.respond({"success": True, "result": "official-app"})

        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            if self.path in ("/oauth2/getToken", "/oauth2/refreshToken"):
                refresh = self.path.endswith("refreshToken")
                calls.append("refresh" if refresh else "exchange")
                if body.get("clientId") != state["app"] or "clientSecret" in body:
                    errors.append("incorrect application/credential pairing")
                if not refresh and body.get("authCode") != "fixture-code":
                    errors.append("incorrect code input")
                if refresh and body.get("refreshToken") != "fixture-refresh":
                    errors.append("incorrect refresh token")
                self.respond({"accessToken": "fixture-access", "refreshToken": "fixture-refresh",
                              "expiresIn": 7200 if refresh else 1, "corpId": "fixture-corp"})
                return
            method = body.get("method")
            if method == "tools/call":
                calls.append("identity")
                if body.get("params", {}).get("name") != "get_current_user_profile":
                    errors.append("unexpected identity tool")
                if self.headers.get("x-user-access-token") != "fixture-access":
                    errors.append("identity lookup did not use exchanged token")
                identity = {"corpId": "fixture-corp", "userId": "fixture-user"}
                if state["bad_identity"]:
                    identity.pop("userId")
                result = {"content": [{"type": "text", "text": json.dumps({
                    "result": [{"orgEmployeeModel": identity}]})}], "isError": False}
            elif method == "initialize":
                result = {"protocolVersion": "2024-11-05", "capabilities": {"tools": {}},
                          "serverInfo": {"name": "exchange-fixture", "version": "1"}}
            else:
                errors.append("unexpected MCP request")
                result = {}
            self.respond({"jsonrpc": "2.0", "id": body.get("id"), "result": result})

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    url = f"http://127.0.0.1:{server.server_port}"
    checks = []
    try:
        with tempfile.TemporaryDirectory(prefix="dws-exchange-smoke-") as tmp:
            def environment(name):
                directory = Path(tmp) / name
                directory.mkdir()
                (directory / "mcp_url").write_text(url)
                env = {k: v for k, v in os.environ.items()
                       if not k.startswith(("DWS_", "DINGTALK_"))}
                env.update(DWS_CONFIG_DIR=str(directory), DWS_KEYCHAIN_DIR=str(directory / "keys"),
                           DWS_DISABLE_KEYCHAIN="1", DINGTALK_CONTACT_MCP_URL=url + "/contact",
                           DWS_ALLOW_HTTP_ENDPOINTS="1", DWS_TRUSTED_DOMAINS="127.0.0.1")
                return env, directory

            def run(argv, env, stdin="", success=True):
                report = Path(env["DWS_CONFIG_DIR"]) / "perf.json"
                report.unlink(missing_ok=True)
                env = dict(env, DWS_PERF_REPORT=str(report))
                result = subprocess.run(command + argv + ["--format", "json"], env=env,
                                        input=stdin, capture_output=True, text=True, timeout=90)
                if (result.returncode == 0) != success:
                    # Fixtures only; keep even their token strings out of reports.
                    detail = result.stderr
                    for secret in ("fixture-code", "fixture-access", "fixture-refresh"):
                        detail = detail.replace(secret, "[redacted]")
                    raise AssertionError(f"unexpected CLI exit {result.returncode}: {detail[:1200]}")
                surfaces = result.stdout + result.stderr
                if argv[:2] == ["auth", "exchange"] and "--mcp" not in argv:
                    assert report.exists(), "exchange did not write a performance report for verification"
                if report.exists():
                    surfaces += report.read_text()
                    assert "command" in json.loads(report.read_text()), "missing performance command field"
                for secret in ("fixture-code", "fixture-access", "fixture-refresh"):
                    assert secret not in surfaces, "credential in stdout/stderr/performance report"
                return json.loads(result.stdout) if success else None

            for name, selection in (("default", ["--client-id", "default"]),
                                    ("custom", ["--client-id", "custom-app"]), ("omitted", [])):
                env, directory = environment(name)
                state["app"] = "custom-app" if name == "custom" else "official-app"
                before = len(calls)
                login = run(["auth", "exchange", "--code-stdin"] + selection, env, "fixture-code\n")
                assert login["data"]["dwsProfile"] == "fixture-corp:fixture-user"
                assert login["data"]["clientId"] == state["app"]
                assert login["data"]["isCurrentProfile"]
                # Short-lived fixture triggers an actual refresh in another CLI process.
                status = run(["auth", "status"], env)
                assert status.get("user_id") == "fixture-user", "status missing employee userId"
                assert status.get("authenticated") and status.get("token_valid") and status.get("refreshed"), \
                    "status did not confirm a valid refreshed login"
                # A saved profile must work through the normal command runner,
                # not only through exchange's in-memory token override.
                for profile_args in ([], ["--profile", login["data"]["dwsProfile"]]):
                    current = run(["contact", "user", "get-self"] + profile_args, env)
                    identities = current.get("result", [])
                    assert current.get("success") and len(identities) == 1
                    identity = identities[0].get("orgEmployeeModel", {})
                    assert identity.get("corpId") == "fixture-corp" and identity.get("userId") == "fixture-user", \
                        "saved profile could not execute a real DWS command as the employee"
                segment = calls[before:]
                assert segment.count("exchange") == 1 and segment.count("identity") == 3
                assert segment.count("refresh") == 1
                assert segment.count("discover") == (0 if name == "custom" else 1)
                checks.append(name + ": exchange -> online identity -> save -> auth status -> refresh -> get-self (default and exact profile)")

            env, directory = environment("validation")
            before = len(calls)
            run(["auth", "exchange", "--code-stdin", "--client-id", "default", "--dry-run"], env)
            run(["auth", "exchange", "--code-stdin", "--mcp"], env, success=False)
            run(["auth", "exchange", "--code-stdin", "--code", "fixture-code"], env, success=False)
            assert len(calls) == before and not (directory / "profiles.json").exists()
            checks.append("dry-run and invalid arguments: no network or profile writes")

            for outcome in ("success", "failure", "dry-run"):
                for input_mode in ("separate", "equals", "stdin"):
                    env, directory = environment("redaction-" + outcome + "-" + input_mode)
                    state.update(app="official-app", bad_identity=(outcome == "failure"))
                    before = len(calls)
                    code_args = {"separate": ["--code", "fixture-code"],
                                 "equals": ["--code=fixture-code"], "stdin": ["--code-stdin"]}[input_mode]
                    run(["auth", "exchange", "--client-id", "default"] + code_args +
                        (["--dry-run"] if outcome == "dry-run" else []), env,
                        "fixture-code\n" if input_mode == "stdin" else "", success=outcome != "failure")
                    if outcome == "dry-run":
                        assert len(calls) == before
                    if outcome != "success":
                        assert not (directory / "profiles.json").exists()
            checks.append("9 binary redaction cases: success/failure/dry-run x --code VALUE/--code=VALUE/stdin; stdout, stderr, perf report")

            env, directory = environment("missing-identity")
            state.update(app="custom-app", bad_identity=True)
            run(["auth", "exchange", "--code-stdin", "--client-id", "custom-app"], env,
                "fixture-code\n", success=False)
            assert not (directory / "profiles.json").exists()
            checks.append("missing online userId: rejected without saving profile")
            assert not errors, errors
            print(json.dumps({"success": True, "environment": "localhost fixtures only",
                              "checks": checks}, ensure_ascii=False, indent=2))
    finally:
        server.shutdown()
        server.server_close()


if __name__ == "__main__":
    main()
