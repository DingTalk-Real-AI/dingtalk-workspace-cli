#!/usr/bin/env python3
# Copyright 2026 Alibaba Group
# Licensed under the Apache License, Version 2.0 (the "License").
#
# check-phantom-overrides.py — publish-time guard against "phantom" CLI
# commands: toolOverrides in the discovery envelope whose backing MCP tool is
# not actually deployed on the server. Such overrides render in `dws <svc>
# --help` but fail at invocation ("tool not found"). This is the discovery-side
# (Plan A) complement to the runtime guard in internal/compat/dynamic_commands.go.
#
# TRUTH SOURCE: archived live tools/list snapshots from the legacy discovery CLI.
# into <cache-dir>/<partition>/tools/*.json, mapped to servers via
# <cache-dir>/<partition>/market/servers.json (cli.id slug -> server key).
#
# An override is a phantom ONLY if its tool name is absent from the resolved
# server's live tool set AND it is not otherwise explained:
#   - serverOverride: resolve against the TARGET server's tool set;
#   - pipeline:       orchestrates multiple tools, no single backing tool;
#   - redirectTo/target: a redirect stub, not a real tool invocation;
#   - hidden:true:    explicitly acknowledged (e.g. lead metadata for a tool
#                     not yet deployed) — reported as INFO, never fails CI.
#
# Exit code: 1 if any UN-acknowledged phantom override is found, else 0.
#
# Usage:
#   use a cache directory captured by a pre-static-discovery CLI build
#   python3 scripts/dev/check-phantom-overrides.py
#   python3 scripts/dev/check-phantom-overrides.py --envelope envelope/discovery.pre.json

import argparse
import glob
import json
import os
import sys

CLI_META_KEY = "com.dingtalk.mcp.registry/cli"


def load_real_tools(cache_dir, partition):
    """Return (name2tools, slug2tools): display-name and cli.id slug -> set(tool names)."""
    base = os.path.join(cache_dir, partition)
    real_by_key = {}
    for f in glob.glob(os.path.join(base, "tools", "*.json")):
        try:
            d = json.load(open(f))
        except (OSError, ValueError):
            continue
        names = {(t.get("name") or "").strip() for t in (d.get("tools") or [])}
        names.discard("")
        real_by_key[d.get("server_key")] = names

    servers_path = os.path.join(base, "market", "servers.json")
    if not os.path.exists(servers_path):
        sys.exit(
            f"error: {servers_path} not found — provide --cache-dir from an archived/pre-static-discovery run"
        )
    servers = json.load(open(servers_path)).get("servers", [])

    name2tools, slug2tools = {}, {}
    for s in servers:
        ts = real_by_key.get(s.get("key"), set())
        if s.get("display_name"):
            name2tools[s["display_name"].strip()] = ts
        slug = ((s.get("cli") or {}).get("id") or "").strip()
        if slug:
            slug2tools[slug] = ts
    return name2tools, slug2tools, real_by_key


def cli_block(server):
    """toolOverrides live under _meta['com.dingtalk.mcp.registry/cli'] (flat slash key)."""
    return (server.get("_meta", {}) or {}).get(CLI_META_KEY, {}) or {}


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--envelope", default="envelope/discovery.pre.json")
    ap.add_argument("--cache-dir", default=os.path.expanduser("~/.dws/cache"))
    ap.add_argument("--partition", default="default_default")
    args = ap.parse_args()

    if not os.path.exists(args.envelope):
        sys.exit(f"error: envelope not found: {args.envelope}")

    name2tools, slug2tools, real_by_key = load_real_tools(args.cache_dir, args.partition)
    if not real_by_key:
        sys.exit(
            "error: no tools snapshots in cache — provide --cache-dir from an archived/pre-static-discovery run"
        )

    env = json.load(open(args.envelope))
    phantom, acknowledged = [], []

    for s in env.get("servers", []):
        cli = cli_block(s)
        name = (s.get("server", {}).get("name") or "").strip()
        own = name2tools.get(name, set())
        ov = cli.get("toolOverrides") or {}
        for tool, o in ov.items():
            so = (o.get("serverOverride") or "").strip()
            if o.get("pipeline") or o.get("redirectTo") or o.get("target"):
                continue
            target = slug2tools.get(so, set()) if so else own
            if tool in target:
                continue
            entry = (name, tool, o.get("cliName", ""), o.get("group", ""), so or "self")
            (acknowledged if o.get("hidden") else phantom).append(entry)

    if acknowledged:
        print(f"INFO: {len(acknowledged)} acknowledged (hidden:true) phantom overrides — OK:")
        for name, tool, cn, g, via in acknowledged:
            print(f"  [hidden] {name}: {tool} (cli={cn!r} group={g!r} via={via})")

    if phantom:
        print(f"\nFAIL: {len(phantom)} un-acknowledged phantom override(s) "
              f"(tool not deployed; will fail at invocation):")
        for name, tool, cn, g, via in phantom:
            print(f"  {name}: {tool} -> cli={cn!r} group={g!r} via={via}")
        print("\nFix: remove the override, route via serverOverride/pipeline, "
              "or mark hidden:true if it is intentional lead metadata.")
        return 1

    print(f"\nOK: no un-acknowledged phantom overrides in {args.envelope}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
