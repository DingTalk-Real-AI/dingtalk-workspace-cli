#!/usr/bin/env python3
"""Run destructive, real-data Wiki Shortcut verification on a disposable space.

Requires an authenticated `dws` session. Set DWS_WIKI_E2E_MEMBER_ID to a real
internal user ID that may be granted temporary access to the empty fixture.
The script prints capability labels only; business IDs, names, URLs, and raw
responses remain in memory. The disposable Wiki space is deleted in `finally`.
"""

from __future__ import annotations

import json
import os
import subprocess
import sys
import time
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
DWS = ROOT / "dws"
MEMBER_ID = os.environ.get("DWS_WIKI_E2E_MEMBER_ID", "").strip()


class E2EFailure(RuntimeError):
    pass


def invoke(args: list[str]) -> dict:
    process = subprocess.run(
        [str(DWS), "wiki", *args, "--format", "json"],
        cwd=ROOT,
        capture_output=True,
        text=True,
        check=False,
    )
    try:
        envelope = json.loads(process.stdout)
    except json.JSONDecodeError as exc:
        raise E2EFailure(f"non-JSON response (exit {process.returncode})") from exc
    if process.returncode != 0 or envelope.get("ok") is not True:
        error = envelope.get("error") or {}
        reason = error.get("reason") or error.get("category") or "command_failed"
        raise E2EFailure(f"{reason} (exit {process.returncode})")
    data = envelope.get("data")
    if not isinstance(data, dict):
        raise E2EFailure("success envelope lacks object data")
    return data


def check(label: str, condition: bool) -> None:
    if not condition:
        raise E2EFailure(f"{label}: business assertion failed")
    print(f"PASS {label}")


def main() -> int:
    if not DWS.exists():
        raise E2EFailure("build ./dws first with make build")
    if not MEMBER_ID:
        raise E2EFailure("set DWS_WIKI_E2E_MEMBER_ID to a temporary internal test member")

    stamp = time.strftime("%m%d%H%M%S")
    space_name = f"DWS Wiki E2E {stamp}"  # <= 32 characters
    workspace = ""
    disposable_nodes: list[str] = []
    member_added = False
    try:
        created = invoke(["+space-create", "--name", space_name, "--desc", "Disposable E2E fixture"])
        workspace = str(created.get("workspaceId") or "")
        check("space-create-readback", bool(workspace) and created.get("space", {}).get("workspaceId") == workspace)

        page = invoke(["+space-list", "--limit", "1"])
        check("space-list-cursor", page.get("count") == 1 and isinstance(page.get("hasMore"), bool))
        all_spaces = invoke(["+space-list", "--limit", "1", "--page-all", "--max-items", "2"])
        check("space-list-auto-page", all_spaces.get("count") == 2 and len(all_spaces.get("spaces", [])) == 2)
        search_ok = False
        for _ in range(8):
            searched = invoke(["+space-search", "--query", space_name])
            if any(row.get("workspaceId") == workspace for row in searched.get("spaces", [])):
                search_ok = True
                break
            time.sleep(2)
        check("space-search", search_ok)
        detail = invoke(["+space-get", "--workspace", workspace])
        check("space-get", detail.get("workspaceId") == workspace)
        resolved = invoke(["+resolve-space", "--name", space_name])
        check("resolve-space", resolved.get("resolved") is True and resolved.get("spaceId") == workspace)

        empty_nodes = invoke(["+node-list", "--workspace", workspace])
        check("node-list-explicit-empty", empty_nodes.get("count") == 0 and empty_nodes.get("nodes") == [])

        folder = invoke(["+node-create", "--workspace", workspace, "--name", "E2E Folder", "--type", "folder"])
        folder_id = str(folder.get("nodeId") or "")
        disposable_nodes.append(folder_id)
        check("node-create-folder-readback", bool(folder_id) and folder.get("node", {}).get("nodeId") == folder_id)
        document = invoke(["+node-create", "--workspace", workspace, "--folder", folder_id, "--name", "E2E Document"])
        node_id = str(document.get("nodeId") or "")
        disposable_nodes.append(node_id)
        check("node-create-document-readback", bool(node_id) and document.get("node", {}).get("nodeId") == node_id)

        listed = invoke(["+node-list", "--workspace", workspace, "--limit", "1"])
        check("node-list-cursor", listed.get("count") == 1 and listed.get("hasMore") is True and bool(listed.get("nextCursor")))
        all_nodes = invoke(["+node-list", "--workspace", workspace, "--limit", "1", "--page-all"])
        check(
            "node-list-auto-page",
            all_nodes.get("count", 0) >= 1
            and (all_nodes.get("autoPageComplete") is True or all_nodes.get("hasMore") is False),
        )
        info = invoke(["+node-get", "--node", node_id])
        check("node-get", info.get("nodeId") == node_id)

        # Search indexing can lag after a create; retry without accepting a
        # malformed/missing collection as an empty success.
        search_ok = False
        for _ in range(6):
            found = invoke(["+node-search", "--workspace", workspace, "--query", "E2E Document"])
            if any(row.get("nodeId") == node_id for row in found.get("nodes", [])):
                search_ok = True
                break
            time.sleep(2)
        check("node-search", search_ok)

        copied = invoke(["+node-copy", "--workspace", workspace, "--folder", folder_id, "--node", node_id, "--yes"])
        copy_id = str(copied.get("nodeId") or "")
        disposable_nodes.append(copy_id)
        check("node-copy-readback", bool(copy_id) and bool(copied.get("copy")))

        moved = invoke(["+move", "--workspace", workspace, "--folder", folder_id, "--node", node_id, "--yes"])
        check("move-readback", moved.get("node", {}).get("workspaceId") == workspace and moved.get("node", {}).get("folderId") == folder_id)
        moved_out = invoke(["+move-to-drive", "--node", node_id, "--yes"])
        check("move-to-drive-readback", moved_out.get("node", {}).get("workspaceId") != workspace)
        moved_back = invoke(["+node-move", "--workspace", workspace, "--folder", folder_id, "--node", node_id, "--yes"])
        check("node-move-alias-readback", moved_back.get("node", {}).get("workspaceId") == workspace)

        named = invoke(["+wiki-new-doc", "--space", space_name, "--title", "E2E Name Resolved"])
        named_id = str(named.get("nodeId") or "")
        disposable_nodes.append(named_id)
        check("wiki-new-doc-readback", bool(named_id) and named.get("document", {}).get("nodeId") == named_id)

        members = invoke(["+member-list", "--workspace", workspace, "--limit", "50"])
        check("member-list", members.get("count", 0) >= 1 and isinstance(members.get("members"), list))
        added = invoke(["+member-add", "--workspace", workspace, "--users", MEMBER_ID, "--role", "READER"])
        member_added = True
        check("member-add-readback", added.get("success") is True and added.get("verifiedBy") == "member_list_readback")
        updated = invoke(["+member-update", "--workspace", workspace, "--users", MEMBER_ID, "--role", "EDITOR"])
        check("member-update-readback", updated.get("success") is True and updated.get("verifiedBy") == "member_list_readback")
        removed = invoke(["+member-remove", "--workspace", workspace, "--users", MEMBER_ID])
        member_added = False
        check("member-remove-readback", removed.get("success") is True and removed.get("verifiedBy") == "member_list_readback")

        feeds = invoke(["+feed-list", "--workspace", workspace, "--limit", "10"])
        check("feed-list", isinstance(feeds.get("feeds"), list))

        # Exercise high-risk delete before final whole-space cleanup.
        delete_target = disposable_nodes.pop()
        deleted = invoke(["+node-delete", "--workspace", workspace, "--node", delete_target, "--yes"])
        check("node-delete-terminal", deleted.get("success") is True and deleted.get("deleted") is True)
        return 0
    finally:
        if workspace:
            if member_added:
                try:
                    invoke(["+member-remove", "--workspace", workspace, "--users", MEMBER_ID])
                except Exception:
                    pass
            try:
                deleted = invoke(["+space-delete", "--workspace", workspace, "--yes"])
                check("space-delete-alias-cleanup", deleted.get("success") is True and deleted.get("deleted") is True)
            except Exception as exc:
                print(f"CLEANUP FAILED: {exc}", file=sys.stderr)


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except E2EFailure as exc:
        print(f"FAIL {exc}", file=sys.stderr)
        raise SystemExit(1)
