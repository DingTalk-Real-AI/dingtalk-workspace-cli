import json
import os
import re
import shlex
import subprocess
from pathlib import Path


def repo_root(start_file: str) -> Path:
    current = Path(start_file).resolve()
    for parent in [current, *current.parents]:
        if (parent / "go.mod").exists():
            return parent
    raise RuntimeError(f"cannot locate repo root from {start_file}")


def resolve_dws_cmd(start_file: str) -> list[str]:
    root = repo_root(start_file)
    if env_bin := os.environ.get("DWS_BIN"):
        return shlex.split(env_bin)

    for rel in ("dws", "build/dws", "bin/dws", "dingtalk-workspace-cli"):
        candidate = root / rel
        if candidate.exists() and os.access(candidate, os.X_OK):
            return [str(candidate)]

    return ["go", "run", "./cmd"]


def combined_output(result: subprocess.CompletedProcess) -> str:
    return (result.stdout or "") + (result.stderr or "")


def dry_run_args(output: str) -> dict:
    match = re.search(r"Arguments:\s*(\{.*\})", output, re.S)
    assert match, f"dry-run output does not contain Arguments JSON: {output}"
    return json.loads(match.group(1))


class DWSRunner:
    def __init__(self):
        self.root = repo_root(__file__)
        self.cmd = resolve_dws_cmd(__file__)

    def run_raw(self, *args: str, timeout: int = 45) -> subprocess.CompletedProcess:
        return subprocess.run(
            [*self.cmd, *args],
            cwd=self.root,
            text=True,
            capture_output=True,
            timeout=timeout,
        )

    def run(self, *args: str, timeout: int = 45):
        result = self.run_raw(*args, timeout=timeout)
        output = combined_output(result)
        assert result.returncode == 0, output
        return json.loads(result.stdout)
