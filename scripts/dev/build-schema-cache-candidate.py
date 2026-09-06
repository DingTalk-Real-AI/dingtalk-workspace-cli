#!/usr/bin/env python3
"""Build a native, runtime-complete Schema cache development candidate.

macOS artifacts are ad-hoc signed. This is reproducible candidate validation,
not a release enablement or Developer ID/notarization proof.
"""

import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess

contract_spec = importlib.util.spec_from_file_location(
    'schema_package_contract', Path(__file__).resolve().parents[1] / 'build/schema_package_contract.py')
contract = importlib.util.module_from_spec(contract_spec)
contract_spec.loader.exec_module(contract)


def sha256(path):
    value = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            value.update(chunk)
    return value.hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True, help="new output directory; must not exist")
    parser.add_argument("--version", default="v0.0.0-schema-cache-candidate")
    args = parser.parse_args()
    if not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?", args.version):
        parser.error("version must be a v-prefixed semantic version")
    root = Path(__file__).resolve().parents[2]
    env = {**os.environ, "GOTOOLCHAIN": "go1.25.9", "CGO_ENABLED": "0",
           "GOFLAGS": "", "GOWORK": "off", "GOEXPERIMENT": "",
           "GOAMD64": "v1", "GOARM64": "v8.0"}

    def run(command, capture=False):
        return subprocess.run(command, cwd=root, env=env, check=True,
                              stdout=subprocess.PIPE if capture else None,
                              text=True).stdout

    host = json.loads(run(["go", "env", "-json", "GOHOSTOS", "GOHOSTARCH", "GOVERSION"], True))
    goos, goarch = host["GOHOSTOS"], host["GOHOSTARCH"]
    if host["GOVERSION"] != "go1.25.9" or (goos, goarch) not in (("darwin", "arm64"), ("linux", "amd64")):
        parser.error("candidate requires Go 1.25.9 on native darwin/arm64 or linux/amd64")
    env.update(GOOS=goos, GOARCH=goarch)
    # Candidate identity must not silently inherit custom build tags or flags.
    env["GOFLAGS"] = ""
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    proof_path = output / "identity.json"
    run(["go", "run", "./internal/generator/cmd_schema_cache_identity", "-root", str(root), "-output", str(proof_path)])
    proof = json.loads(proof_path.read_text())
    commit = run(["git", "rev-parse", "HEAD"], True).strip()
    build_time = run(["sh", "scripts/build/release-build-time.sh", commit], True).strip()
    package = output / f"dws-{args.version}-{goos}-{goarch}"
    package.mkdir(parents=True)
    binary = package / ("dws.exe" if goos == "windows" else "dws")
    core_ldflags = contract.core_ldflags(proof, args.version, commit, build_time)
    run(["go", "build", "-trimpath", "-buildmode=pie", "-ldflags", core_ldflags, "-o", str(binary), "./cmd"])
    staging = output / "runtime-staging"
    run(["sh", "scripts/build/prepare-runtime-payload.sh", goos, goarch, str(staging)])
    payload = staging / ".dws-runtime/20260825"
    if goos == "darwin":
        manifest_path = payload / "manifest.json"
        manifest = json.loads(manifest_path.read_text())
        library = payload / manifest["library"]
        run(["codesign", "--force", "--sign", "-", str(library)])
        run(["codesign", "--verify", "--strict", str(library)])
        manifest["library_sha256"] = sha256(library)
        manifest_path.write_text(json.dumps(manifest, indent=2) + "\n")
    run(["go", "run", "./scripts/build/runtime-payload", "inject", str(binary), str(payload)])
    if goos == "darwin":
        run(["codesign", "--force", "--sign", "-", str(binary)])
        run(["codesign", "--verify", "--strict", str(binary)])
    binary_digest, binary_size = sha256(binary), binary.stat().st_size
    (output / "candidate-build.json").write_text(json.dumps({
        "scope": "native development candidate; not release enablement proof",
        "source_commit": commit,
        "source_tree": run(["git", "rev-parse", "HEAD^{tree}"], True).strip(),
        "source_dirty": bool(run(["git", "status", "--porcelain"], True).strip()),
        "version": args.version,
        "go_version": host["GOVERSION"], "goos": goos, "goarch": goarch,
        "cgo_enabled": "0", "goexperiment": "", "gowork": "off",
        "build_flags": ["-trimpath", "-buildmode=pie"], "build_tags": [],
        "binary_ldflags": core_ldflags, "identity_sha256": sha256(proof_path),
        "binary_sha256": binary_digest, "binary_size": binary_size,
        "signing": "ad-hoc" if goos == "darwin" else "none",
    }, indent=2) + "\n")
    print(json.dumps({"binary": str(binary), "proof": str(proof_path), "signing": "ad-hoc" if goos == "darwin" else "none"}))


if __name__ == "__main__":
    main()
