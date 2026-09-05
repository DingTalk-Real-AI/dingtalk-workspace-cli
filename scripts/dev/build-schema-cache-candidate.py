#!/usr/bin/env python3
"""Build a native, runtime-complete Schema cache development candidate.

macOS artifacts are ad-hoc signed. This is reproducible candidate validation,
not a release enablement or Developer ID/notarization proof.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess


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
    build_time = run(["git", "show", "-s", "--format=%cI", "HEAD"], True).strip()
    package = output / f"dws-{args.version}-{goos}-{goarch}"
    (package / "bin").mkdir(parents=True)
    (package / "libexec").mkdir()
    core, launcher = package / "libexec/dws-core", package / "bin/dws"
    fields = {
        "schemaCacheEdition": "edition", "schemaCacheSourceSHA256": "source_sha256",
        "schemaCacheSurfaceSHA256": "surface_sha256", "schemaCacheBuildID": "build_id",
        "schemaCacheMetaLength": "meta_length", "schemaCacheMetaSHA256": "meta_sha256",
        "schemaCacheRegistryLength": "registry_length", "schemaCacheRegistrySHA256": "registry_sha256",
    }
    app_package = "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
    flags = ["-s", "-w", "-X", f"{app_package}.version={args.version}",
             "-X", f"{app_package}.gitCommit={commit}", "-X", f"{app_package}.buildTime={build_time}"]
    for field, key in fields.items():
        flags += ["-X", f"{app_package}.{field}={proof[key]}"]
    core_ldflags = " ".join(flags)
    run(["go", "build", "-trimpath", "-buildmode=pie", "-ldflags", core_ldflags, "-o", str(core), "./cmd"])
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
    run(["go", "run", "./scripts/build/runtime-payload", "inject", str(core), str(payload)])
    if goos == "darwin":
        run(["codesign", "--force", "--sign", "-", str(core)])
        run(["codesign", "--verify", "--strict", str(core)])
    core_digest, core_size = sha256(core), core.stat().st_size
    flags = f"-s -w -X main.version={args.version} -X main.commit={commit} -X main.edition=open -X main.coreSHA256={core_digest} -X main.coreSize={core_size}"
    run(["go", "build", "-trimpath", "-buildmode=pie", "-ldflags", flags, "-o", str(launcher), "./cmd/dws-launcher"])
    if goos == "darwin":
        run(["codesign", "--force", "--sign", "-", str(launcher)])
        run(["codesign", "--verify", "--strict", str(launcher)])
    run(["go", "run", "./scripts/build/package-manifest", "--package-root", str(package),
         "--version", args.version, "--commit", commit, "--edition", "open", "--goos", goos, "--goarch", goarch])
    if sha256(core) != core_digest or core.stat().st_size != core_size:
        raise RuntimeError("core changed after launcher identity injection")
    (output / "candidate-build.json").write_text(json.dumps({
        "scope": "native development candidate; not release enablement proof",
        "source_commit": commit,
        "source_tree": run(["git", "rev-parse", "HEAD^{tree}"], True).strip(),
        "source_dirty": bool(run(["git", "status", "--porcelain"], True).strip()),
        "go_version": host["GOVERSION"], "goos": goos, "goarch": goarch,
        "cgo_enabled": "0", "goexperiment": "", "gowork": "off",
        "build_flags": ["-trimpath", "-buildmode=pie"], "build_tags": [],
        "core_ldflags": core_ldflags, "launcher_ldflags": flags,
        "identity_sha256": sha256(proof_path), "core_sha256": core_digest,
        "launcher_sha256": sha256(launcher),
        "package_manifest_sha256": sha256(package / "package-manifest.json"),
        "signing": "ad-hoc" if goos == "darwin" else "none",
    }, indent=2) + "\n")
    print(json.dumps({"binary": str(launcher), "proof": str(proof_path), "signing": "ad-hoc" if goos == "darwin" else "none"}))


if __name__ == "__main__":
    main()
