#!/usr/bin/env python3
"""Verify an exact native candidate's cache, repair, wire parity and process cost.

This is an offline, telemetry-opted-out candidate/base measurement. It does not
claim network sandboxing, notarization, or competitive public-entry acceptance.
Run against the final packaged executable, never a rebuilt substitute.
"""

import argparse
import hashlib
import json
import math
import os
from pathlib import Path
import platform
import random
import shutil
import statistics
import subprocess
import sys
import tempfile
import time


def digest(path):
    value = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            value.update(chunk)
    return value.hexdigest()


def invoke(binary, args, env, cwd):
    # Keep retained full-export JSON in this coordinator out of child RSS.
    # Report only the sampler's child, never the sampler process's own usage.
    with tempfile.TemporaryDirectory(prefix="measurement-", dir=cwd) as directory:
        stdout, stderr = Path(directory) / "stdout", Path(directory) / "stderr"
        request = {"argv": [str(binary), *args], "env": env, "cwd": str(cwd),
                   "stdout": str(stdout), "stderr": str(stderr)}
        sampler = subprocess.run(
            [sys.executable, str(Path(__file__).with_name("schema-cache-process-measure.py"))],
            input=json.dumps(request), text=True, capture_output=True, check=True)
        result = json.loads(sampler.stdout)
        output, error = stdout.read_bytes(), stderr.read_bytes()
        if result["returncode"]:
            raise RuntimeError(f"{args}: exit {result['returncode']}: {error.decode(errors='replace')}")
        if error:
            raise RuntimeError(f"{args}: unexpected stderr: {error.decode(errors='replace')}")
        return output, result["measurement"]


def invoke_concurrently(binary, routes, expected, env, cwd, timeout=180):
    """Start all independent CLI processes before waiting; bound and reap the batch.

    This is a correctness probe, not a process benchmark. Temporary files avoid
    pipe backpressure from a full Schema export while other children repair.
    """
    started = time.monotonic()
    children = []
    with tempfile.TemporaryDirectory(prefix="concurrent-", dir=cwd) as output_dir:
        try:
            for index, route in enumerate(routes):
                output = Path(output_dir) / f"{index}.stdout"
                error = Path(output_dir) / f"{index}.stderr"
                with output.open("wb") as stdout, error.open("wb") as stderr:
                    child = subprocess.Popen([str(binary), *route], env=env, cwd=cwd,
                                             stdin=subprocess.DEVNULL, stdout=stdout, stderr=stderr)
                children.append((child, route, output, error))
            for index, (child, route, output, error) in enumerate(children):
                child.wait(timeout=max(.001, timeout - (time.monotonic() - started)))
                stderr = error.read_text(errors="replace")
                if child.returncode or stderr:
                    raise RuntimeError(f"concurrent {route}: exit {child.returncode}: {stderr}")
                if json.loads(output.read_bytes()) != expected[index]:
                    raise RuntimeError(f"concurrent output differs from authoritative assembly: {route}")
        finally:
            for child, *_ in children:
                if child.poll() is None:
                    child.kill()
            for child, *_ in children:
                child.wait()
    return {"processes": len(routes), "routes": routes,
            "elapsed_ms": (time.monotonic() - started) * 1000, "wire_parity": True}


def summarize(samples):
    def percentile(key, fraction):
        values = sorted(sample[key] for sample in samples)
        return values[max(0, math.ceil(len(values) * fraction) - 1)]
    return {key: {"p50": statistics.median(s[key] for s in samples),
                  "p95": percentile(key, .95)} for key in samples[0]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--proof", type=Path, required=True, help="native generator identity JSON")
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--samples", type=int, default=30)
    parser.add_argument("--seed", type=int, default=20260906)
    parser.add_argument("--require-schema-fast-path", action="store_true", help="prove hits execute in a byte-identical launcher copy with no core available")
    args = parser.parse_args()
    if not hasattr(os, "wait4") or sys.platform not in ("darwin", "linux"):
        parser.error("native process accounting requires macOS or Linux")
    if args.samples < 30:
        parser.error("at least 30 samples per mode are required")
    proof = json.loads(args.proof.read_text())
    if proof["go_runtime_version"] != "go1.25.9":
        parser.error("release proof must use Go 1.25.9")
    binary = args.binary.resolve(strict=True)
    binary_sha = digest(binary)
    build_info = subprocess.check_output(["go", "version", "-m", str(binary)], text=True)
    if "go1.25.9" not in build_info.splitlines()[0]:
        raise RuntimeError("candidate toolchain differs from the native proof")
    package_manifest = binary.parent.parent / "package-manifest.json"
    core = None
    if package_manifest.is_file():
        manifest = json.loads(package_manifest.read_text())
        core = package_manifest.parent / manifest["core"]["path"]
        if digest(core) != manifest["core"]["sha256"] or binary_sha != manifest["launcher"]["sha256"]:
            raise RuntimeError("candidate package does not match its final manifest")
    core_sha = digest(core) if core else None
    leaf = ["schema", "calendar.create_calendar_event", "--compact", "-f", "json"]
    report = {
        "binary": str(binary), "binary_sha256": binary_sha, "core_sha256": core_sha,
        "build_id": proof["build_id"], "proof_sha256": digest(args.proof),
        "platform": platform.platform(), "samples_per_mode": args.samples, "seed": args.seed,
        "measurement": "wait4 in a fresh sampler; sampler startup excluded from candidate wall time",
        "telemetry": "DO_NOT_TRACK=1 in both modes", "network_sandboxed": False,
        "scope": "native candidate cache versus its authoritative assembly; not competitive acceptance",
    }
    # A user-owned parent also satisfies the cache's secure ancestry policy;
    # /tmp is deliberately not used as the synthetic HOME.
    with tempfile.TemporaryDirectory(prefix=".dws-cache-proof-", dir=Path.home()) as directory:
        home = Path(directory)
        cache_base = home / ("Library/Caches" if sys.platform == "darwin" else ".cache")
        cache_base.mkdir(parents=True, mode=0o700)
        environment = {
            "PATH": os.environ.get("PATH", "/usr/bin:/bin"), "HOME": str(home),
            "XDG_CACHE_HOME": str(cache_base), "XDG_CONFIG_HOME": str(home / ".config"),
            "DWS_CONFIG_DIR": str(home / ".dws"), "DO_NOT_TRACK": "1", "LANG": "C", "LC_ALL": "C",
        }
        disabled = {**environment, "DWS_SCHEMA_CACHE_DISABLE": "1"}
        cache = cache_base / "dws/schema" / hashlib.sha256(proof["edition"].encode()).hexdigest() / "v1"

        if core is not None:
            core_version, _ = invoke(core, ["--version"], environment, home)
            expected_prefix = f"dws version {manifest['release']['version']} ({manifest['release']['commit']}, "
            if not core_version.decode().startswith(expected_prefix) or not core_version.endswith(b")\n"):
                raise RuntimeError("core runtime version/commit differs from the package manifest")
            public_version, _ = invoke(binary, ["--version"], environment, home)
            if public_version != f"dws version {manifest['release']['version']}\n".encode():
                raise RuntimeError("launcher runtime version differs from the package manifest")

        def verify_artifacts():
            for name, prefix in (("meta.cache", "meta"), ("registry.shards.cache", "registry")):
                data = (cache / name).read_bytes()
                if len(data) != 208 + proof[prefix + "_length"]:
                    raise RuntimeError(f"{name}: binary did not publish the expected artifact length")
                if hashlib.sha256(data[208:]).hexdigest() != proof[prefix + "_sha256"]:
                    raise RuntimeError(f"{name}: binary and native generator artifact digests differ")

        # Root help/version must not populate persistent Schema state.
        invoke(binary, ["--version"], environment, home)
        invoke(binary, ["--help"], environment, home)
        if cache.exists():
            raise RuntimeError("root help/version touched persistent Schema state")
        cold, report["cold_leaf"] = invoke(binary, leaf, environment, home)
        verify_artifacts()
        canonical_leaf = json.loads(cold)
        for route in (["schema"], ["schema", "calendar"], ["schema", "calendar event"],
                      ["schema", "--all"], ["schema", "--cli-path", "calendar event create", "--compact"]):
            cached, _ = invoke(binary, [*route, "-f", "json"], environment, home)
            live, _ = invoke(binary, [*route, "-f", "json"], disabled, home)
            if json.loads(cached) != json.loads(live):
                raise RuntimeError(f"cached/live wire differs: {route}")
        if args.require_schema_fast_path:
            if core is None:
                raise RuntimeError("Schema fast-path proof requires a canonical launcher/core package")
            isolated_launcher = home / "core-free-probe/bin/dws"
            isolated_launcher.parent.mkdir(parents=True)
            shutil.copyfile(binary, isolated_launcher)
            isolated_launcher.chmod(binary.stat().st_mode & 0o777)
            if digest(isolated_launcher) != binary_sha:
                raise RuntimeError("core-free probe does not contain the exact launcher bytes")
            # This copy cannot delegate: no libexec/core exists beside it.
            # The measured candidate itself is neither moved nor modified.
            for route in (["schema"], ["schema", "list"], ["schema", "calendar"],
                          ["schema", "calendar event"], leaf,
                          ["schema", "--cli-path", "calendar event create", "--compact"]):
                actual, _ = invoke(isolated_launcher, route, environment, home)
                expected, _ = invoke(core, route, disabled, home)
                if actual != expected:
                    raise RuntimeError(f"core-free launcher bytes differ from authoritative core output: {route}")
            report["schema_fast_path"] = {"core_free_copy_sha256": binary_sha, "exact_wire_parity": True}
            # User shortcut loading owns startup diagnostics even though those
            # shortcuts are not part of the declaration-only Schema surface.
            shortcut_directory = Path(environment["DWS_CONFIG_DIR"]) / "shortcuts"
            shortcut_directory.mkdir(parents=True)
            broken_shortcut = shortcut_directory / "broken.yaml"
            broken_shortcut.write_text("[invalid YAML")
            try:
                outputs = []
                for executable in (binary, core):
                    result = subprocess.run([str(executable), "schema", "--compact"],
                                            env=environment, cwd=home, stdin=subprocess.DEVNULL,
                                            capture_output=True, timeout=180, check=True)
                    if b"shortcut: failed to load user-defined shortcuts" not in result.stderr:
                        raise RuntimeError("Schema entry swallowed user-shortcut startup diagnostics")
                    outputs.append(result.stdout)
                if outputs[0] != outputs[1]:
                    raise RuntimeError("user-shortcut fallback changed Schema output")
                report["schema_fast_path"]["user_shortcut_diagnostics_preserved"] = True
            finally:
                broken_shortcut.unlink()
                shortcut_directory.rmdir()
        # Corruption must synchronously repair from declarations, preserving output.
        with (cache / "meta.cache").open("r+b") as target:
            target.seek(208)
            first = target.read(1)
            target.seek(208)
            target.write(bytes([first[0] ^ 1]))
        repaired, report["repair_leaf"] = invoke(binary, leaf, environment, home)
        if json.loads(repaired) != canonical_leaf:
            raise RuntimeError("repair changed leaf output")
        verify_artifacts()
        # Independent processes share the same HOME/cache. Exercise directory
        # bootstrap as well as both publication phases, without assuming a
        # single assembler across processes (the lock has a bounded wait).
        routes = [leaf, ["schema", "-f", "json"], ["schema", "calendar", "-f", "json"],
                  ["schema", "--all", "-f", "json"]]
        expected = [json.loads(invoke(binary, route, disabled, home)[0]) for route in routes]
        for path in cache.iterdir():
            if path.name not in {"meta.cache", "registry.shards.cache", "rebuild.lock"}:
                raise RuntimeError(f"unexpected cache artifact before cold-start probe: {path.name}")
            path.unlink()
        for directory in (cache, *list(cache.parents)[:3]):
            directory.rmdir()
        report["concurrent_cold"] = invoke_concurrently(binary, routes, expected, environment, home)
        verify_artifacts()
        for artifact, field in (("meta.cache", "concurrent_meta_repair"),
                                ("registry.shards.cache", "concurrent_registry_repair")):
            with (cache / artifact).open("r+b") as target:
                target.seek(208)
                first = target.read(1)
                target.seek(208)
                target.write(bytes([first[0] ^ 1]))
            report[field] = invoke_concurrently(binary, routes, expected, environment, home)
            verify_artifacts()
        samples = {"cache": [], "live": []}
        order = ["cache", "live"] * args.samples
        random.Random(args.seed).shuffle(order)
        for index, mode in enumerate(order):
            output, measurement = invoke(binary, leaf, environment if mode == "cache" else disabled, home)
            if json.loads(output) != canonical_leaf:
                raise RuntimeError(f"benchmark output changed in {mode} sample {index}")
            samples[mode].append(measurement)
            if (index + 1) % 10 == 0:
                print(f"measured {index + 1}/{len(order)} interleaved processes", file=sys.stderr, flush=True)
        report["raw_samples"] = samples
        report["summary"] = {mode: summarize(values) for mode, values in samples.items()}
        cache_summary, live_summary = report["summary"]["cache"], report["summary"]["live"]
        report["gates"] = {
            "leaf_user_cpu_reduction_at_least_80_percent": cache_summary["user_ms"]["p50"] <= .2 * live_summary["user_ms"]["p50"],
            "leaf_peak_rss_at_most_100_mib": max(s["max_rss_bytes"] for s in samples["cache"]) <= 100 * 1024 * 1024,
        }
        verify_artifacts()
    if digest(binary) != binary_sha or (core and digest(core) != core_sha):
        raise RuntimeError("candidate bytes changed during verification")
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps({"report": str(args.output), "gates": report["gates"]}))
    return 0 if all(report["gates"].values()) else 1


if __name__ == "__main__":
    raise SystemExit(main())
