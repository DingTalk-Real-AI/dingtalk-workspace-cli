#!/usr/bin/env python3
"""Generate ParamDecl Go snippets from schema_hints/metadata overlays.

For each product with parameter overlays, emits the Go code that should be
inserted into the corresponding DeclareLeafMetadata Schema block. Also reports
whether the overlaid flags are registered before or after the DeclareLeafMetadata
call, so the operator knows which need reordering.

Usage: python3 scripts/dev/gen_param_decls.py
"""
import json
import glob
import os
import re
import sys

HELPER_DIR = "internal/helpers"

# Map product hint filename to helper Go file(s)
PRODUCT_FILES = {
    "aisearch": ["aisearch.go"],
    "aitable": ["aitable.go"],
    "attendance": ["attendance.go"],
    "chat": ["chat.go"],
    "contact": ["contact.go"],
    "devdoc": ["devdoc.go"],
    "doc": ["doc.go"],
    "drive": ["drive.go"],
    "hrbrain": ["hrbrain.go"],
    "mail": ["mail.go"],
    "markdown": ["markdown.go"],
    "minutes": ["minutes.go"],
    "report": ["report.go"],
    "sheet": ["sheet.go"],
    "todo": ["todo.go"],
}


def bool_ptr(v: bool) -> str:
    return "boolPtr(true)" if v else "boolPtr(false)"


def render_param_decl(name: str, overlay: dict) -> str:
    parts = [f'Name: "{name}"']
    if "property" in overlay:
        parts.append(f'Property: "{overlay["property"]}"')
    if "required" in overlay:
        parts.append(f"Required: {bool_ptr(overlay['required'])}")
    if "interface_type" in overlay:
        parts.append(f'InterfaceType: "{overlay["interface_type"]}"')
    if "description" in overlay:
        desc = overlay["description"].replace('"', '\\"')
        parts.append(f'Description: "{desc}"')
    if "required_when" in overlay:
        parts.append(f'RequiredWhen: "{overlay["required_when"]}"')
    if "enum" in overlay and overlay["enum"]:
        vals = ", ".join(f'"{v}"' for v in overlay["enum"])
        parts.append(f"Enum: []string{{{vals}}}")
    return "{" + ", ".join(parts) + "}"


def find_declare_line(src: str, tool_name: str) -> int:
    """Find the line number of DeclareLeafMetadata for a tool whose RPCName matches."""
    # Try to find by RPCName in the Schema block
    pattern = re.compile(
        r'DeclareLeafMetadata\(\w+,\s*LeafSpec\{.*?RPCName:\s*"' + re.escape(tool_name) + '"',
        re.S,
    )
    m = pattern.search(src)
    if m:
        return src[: m.start()].count("\n") + 1
    return -1


def find_flag_reg_line(src: str, flag_name: str) -> int:
    """Find the first line where a flag is registered."""
    pattern = re.compile(r'\.Flags\(\)\.\w+\("' + re.escape(flag_name) + '"')
    m = pattern.search(src)
    if m:
        return src[: m.start()].count("\n") + 1
    return -1


def main():
    for hint_file in sorted(glob.glob("internal/cli/schema_hints/metadata/*.json")):
        product = os.path.basename(hint_file).replace(".json", "")
        d = json.load(open(hint_file, encoding="utf-8"))
        tools = d.get("tools") or {}
        overlays = {t: (v or {}).get("parameters") or {} for t, v in tools.items()}
        overlays = {t: p for t, p in overlays.items() if p}
        if not overlays:
            continue

        helper_files = PRODUCT_FILES.get(product, [])
        src = ""
        for hf in helper_files:
            path = os.path.join(HELPER_DIR, hf)
            if os.path.exists(path):
                src += open(path, encoding="utf-8").read() + "\n"

        print(f"\n// === {product} ({len(overlays)} tools) ===")
        for tool_name, params in sorted(overlays.items()):
            decl_line = find_declare_line(src, tool_name)
            print(f"\n// tool: {tool_name}  (DeclareLeafMetadata at line {decl_line})")
            print(f"Parameters: []corecmd.ParamDecl{{")
            for pname in sorted(params):
                overlay = params[pname]
                reg_line = find_flag_reg_line(src, pname)
                order = ""
                if decl_line > 0 and reg_line > 0:
                    if reg_line > decl_line:
                        order = f"  // ⚠ flag registered AFTER DeclareLeafMetadata (line {reg_line} > {decl_line}) — MOVE BEFORE"
                    else:
                        order = f"  // ✓ flag registered before (line {reg_line})"
                print(f"\t{render_param_decl(pname, overlay)},{order}")
            print("},")


if __name__ == "__main__":
    main()
