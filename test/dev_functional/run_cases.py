#!/usr/bin/env python3
"""dev 命令树功能测试执行器：并发跑用例，输出 JSONL 结果。

只负责机械执行与取数（跑命令、收输出、比对预期子串）；
失败归因与报告判断由调用方（agent）完成。
"""
import concurrent.futures
import json
import subprocess
import sys
import time

DWS = sys.argv[1] if len(sys.argv) > 1 else "/tmp/dws-dev"
PARALLEL = 20
TIMEOUT = 30


# 用例格式: (id, args, expect_exit, [output 必须包含的子串])
# expect_exit: 0=成功, 1=报错（CLI 校验/未知命令等）
U = "--unified-app-id"
CASES = [
    # ---- 根与旧名 ----
    ("root-dev-help", ["dev", "--help"], 0, ["app", "connect", "doc"]),
    ("root-dev-app-help", ["dev", "app", "--help"], 0, ["credentials", "permission", "robot", "version"]),
    ("root-old-devapp-removed", ["devapp", "list"], 1, []),
    ("root-old-app-alias-removed", ["app", "list"], 1, []),

    # ---- app list ----
    ("list-plain", ["dev", "app", "list", "--dry-run", "--format", "json"], 0, ["list_dev_app"]),
    ("list-name", ["dev", "app", "list", "--name", "Demo", "--dry-run", "--format", "json"], 0, ['"appName": "Demo"']),
    ("list-app-key", ["dev", "app", "list", "--app-key", "dingxxx", "--dry-run", "--format", "json"], 0, ['"appKey": "dingxxx"']),
    ("list-agent-id-removed", ["dev", "app", "list", "--agent-id", "123", "--dry-run", "--format", "json"], 1, ["unknown flag"]),
    ("list-sort", ["dev", "app", "list", "--creator", "张三", "--sort", "gmt_modified", "--order", "desc", "--dry-run", "--format", "json"], 0, ['"sortType"', '"sortOrder"']),

    # ---- cursor 分页（纯透传：--cursor/--page-size 原样进 params，无合成/换算）----
    ("cursor-list-first", ["dev", "app", "list", "--page-size", "20", "--dry-run", "--format", "json"], 0, ['"pageSize": 20']),
    ("cursor-list-passthrough", ["dev", "app", "list", "--cursor", "TOKEN-ABC", "--page-size", "30", "--dry-run", "--format", "json"], 0, ['"cursor": "TOKEN-ABC"', '"pageSize": 30']),
    # --page 不是独立 flag，但 cobra 把它当 --page-size 的前缀缩写（唯一前缀），故 --page 2 == --page-size 2
    ("cursor-list-page-abbrev", ["dev", "app", "list", "--page", "2", "--dry-run", "--format", "json"], 0, ['"pageSize": 2']),
    ("cursor-perm-passthrough", ["dev", "app", "permission", "list", "--unified-app-id", "u-1", "--cursor", "TOK-P", "--page-size", "50", "--dry-run", "--format", "json"], 0, ['"cursor": "TOK-P"', '"pageSize": 50']),
    ("cursor-perm-limit-removed", ["dev", "app", "permission", "list", "--unified-app-id", "u-1", "--limit", "10", "--dry-run", "--format", "json"], 1, ["unknown flag"]),
    ("cursor-version-passthrough", ["dev", "app", "version", "list", "--unified-app-id", "u-1", "--cursor", "TOK-V", "--page-size", "20", "--dry-run", "--format", "json"], 0, ['"cursor": "TOK-V"', '"pageSize": 20']),
    ("cursor-doc-passthrough", ["dev", "doc", "search", "--query", "x", "--cursor", "TOK-D", "--page-size", "5", "--dry-run", "--format", "json"], 0, ['"cursor": "TOK-D"', '"pageSize": 5']),
    ("cursor-doc-size-removed", ["dev", "doc", "search", "--query", "x", "--size", "5", "--dry-run", "--format", "json"], 1, ["unknown flag"]),

    # ---- app get ----
    ("get-unified", ["dev", "app", "get", U, "u-1", "--dry-run", "--format", "json"], 0, ["get_dev_app", '"unifiedAppId": "u-1"']),
    ("get-agent-id-removed", ["dev", "app", "get", "--agent-id", "123", "--dry-run", "--format", "json"], 1, ["unknown flag"]),
    ("get-no-locator", ["dev", "app", "get", "--dry-run", "--format", "json"], 1, []),

    # ---- app create ----
    ("create-full", ["dev", "app", "create", "--name", "Demo", "--desc", "d", "--icon", "m1", "--dry-run", "--format", "json"], 0, ["create_dev_app", '"appName": "Demo"', '"desc"', '"icon"']),
    ("create-no-name", ["dev", "app", "create", "--dry-run", "--format", "json"], 1, ["--name"]),
    ("create-type-removed", ["dev", "app", "create", "--name", "X", "--type", "internal", "--dry-run"], 1, ["unknown flag"]),
    ("create-write-guard", ["dev", "app", "create", "--name", "Demo", "--format", "json"], 1, []),

    # ---- app update ----
    ("update-name", ["dev", "app", "update", U, "u-1", "--name", "N2", "--dry-run", "--format", "json"], 0, ["update_dev_app", '"appName": "N2"']),
    ("update-no-field", ["dev", "app", "update", U, "u-1", "--dry-run", "--format", "json"], 1, ["至少提供一项"]),
    ("update-no-locator", ["dev", "app", "update", "--name", "N2", "--dry-run", "--format", "json"], 1, []),

    # ---- app lifecycle ----
    ("delete-dry", ["dev", "app", "delete", U, "u-1", "--dry-run", "--format", "json"], 0, ["delete_dev_app"]),
    ("disable-dry", ["dev", "app", "disable", U, "u-1", "--dry-run", "--format", "json"], 0, ["disable_dev_app"]),
    ("enable-dry", ["dev", "app", "enable", U, "u-1", "--dry-run", "--format", "json"], 0, ["enable_dev_app"]),
    ("delete-write-guard", ["dev", "app", "delete", U, "u-1", "--format", "json"], 1, []),
    ("delete-no-confirm-name", ["dev", "app", "delete", U, "u-1", "--yes", "--format", "json"], 1, ["二次确认"]),
    ("disable-no-locator", ["dev", "app", "disable", "--dry-run", "--format", "json"], 1, []),

    # ---- credentials ----
    ("cred-unified", ["dev", "app", "credentials", "get", U, "u-1", "--dry-run", "--format", "json"], 0, ["get_dev_app_credentials"]),
    ("cred-agent", ["dev", "app", "credentials", "get", "--unified-app-id", "u-1", "--dry-run", "--format", "json"], 0, ['"unifiedAppId": "u-1"']),
    ("cred-no-locator", ["dev", "app", "credentials", "get", "--dry-run", "--format", "json"], 1, []),

    # ---- webapp ----
    ("webapp-get", ["dev", "app", "webapp", "get", "--unified-app-id", "u-1", "--dry-run", "--format", "json"], 0, ["get_extension_webapp_config"]),
    ("webapp-config", ["dev", "app", "webapp", "config", "--unified-app-id", "u-1", "--homepage-url", "https://x.invalid/m", "--dry-run", "--format", "json"], 0, ["set_extension_webapp_config", '"homepageUrl"']),
    ("webapp-config-pc", ["dev", "app", "webapp", "config", U, "u-1", "--pc-homepage-url", "https://x.invalid/pc", "--omp-url", "https://x.invalid/o", "--dry-run", "--format", "json"], 0, ['"pcHomepageUrl"', '"ompUrl"']),
    ("webapp-config-no-field", ["dev", "app", "webapp", "config", "--unified-app-id", "u-1", "--dry-run", "--format", "json"], 1, []),
    ("webapp-config-guard", ["dev", "app", "webapp", "config", "--unified-app-id", "u-1", "--homepage-url", "https://x.invalid/m", "--format", "json"], 1, []),

    # ---- permission list ----
    ("perm-list", ["dev", "app", "permission", "list", U, "u-1", "--dry-run", "--format", "json"], 0, ["list_dev_app_permissions", '"pageSize": 20', '"authStatus": "ALL"']),
    ("perm-list-page-size", ["dev", "app", "permission", "list", U, "u-1", "--page-size", "50", "--dry-run", "--format", "json"], 0, ['"pageSize": 50']),
    ("perm-list-keyword", ["dev", "app", "permission", "list", U, "u-1", "--keyword", "机器人", "--status", "unauthed", "--scope-type", "sns", "--dry-run", "--format", "json"], 0, ['"keyword"', '"authStatus": "UNAUTHED"', '"scopeType": "SNS"']),
    ("perm-list-scope", ["dev", "app", "permission", "list", U, "u-1", "--scope", "qyapi_robot_sendmsg", "--dry-run", "--format", "json"], 0, ['"scopeValue"']),
    ("perm-list-scope-alias", ["dev", "app", "permission", "list", U, "u-1", "--permission", "qyapi_robot_sendmsg", "--dry-run", "--format", "json"], 0, ['"scopeValue"']),
    ("perm-list-search-alias", ["dev", "app", "permission", "search", U, "u-1", "--dry-run", "--format", "json"], 0, ["list_dev_app_permissions"]),
    ("perm-list-no-locator", ["dev", "app", "permission", "list", "--dry-run", "--format", "json"], 1, []),

    # ---- permission add ----
    ("perm-add-multi", ["dev", "app", "permission", "add", U, "u-1", "--permissions", "A,B", "--dry-run", "--format", "json"], 0, ["apply_dev_app_permissions", '"A"', '"B"']),
    ("perm-add-scope-alias", ["dev", "app", "permission", "add", U, "u-1", "--scope", "A", "--dry-run", "--format", "json"], 0, ['"scopeValues"']),
    ("perm-add-perm-alias", ["dev", "app", "permission", "add", U, "u-1", "--permission", "A", "--dry-run", "--format", "json"], 0, ['"scopeValues"']),
    ("perm-add-missing", ["dev", "app", "permission", "add", U, "u-1", "--dry-run", "--format", "json"], 1, ["--permissions"]),
    ("perm-add-guard", ["dev", "app", "permission", "add", U, "u-1", "--permissions", "A", "--format", "json"], 1, []),

    # ---- permission remove ----
    ("perm-rm-single", ["dev", "app", "permission", "remove", U, "u-1", "--permissions", "A", "--dry-run", "--format", "json"], 0, ["remove_dev_app_permissions", '"scopeValue": "A"']),
    ("perm-rm-batch", ["dev", "app", "permission", "remove", U, "u-1", "--permissions", "A,B", "--dry-run", "--format", "json"], 0, ['"results"', '"scopeValue": "A"', '"scopeValue": "B"']),
    ("perm-rm-old-alias", ["dev", "app", "permission", "remove", U, "u-1", "--permission", "A", "--dry-run", "--format", "json"], 0, ['"scopeValue": "A"']),
    ("perm-rm-scope-alias", ["dev", "app", "permission", "remove", U, "u-1", "--scope", "A", "--dry-run", "--format", "json"], 0, ['"scopeValue": "A"']),
    ("perm-rm-missing", ["dev", "app", "permission", "remove", U, "u-1", "--dry-run", "--format", "json"], 1, ["--permissions"]),
    ("perm-rm-guard", ["dev", "app", "permission", "remove", U, "u-1", "--permissions", "A", "--format", "json"], 1, []),

    # ---- member ----
    ("member-list", ["dev", "app", "member", "list", U, "u-1", "--dry-run", "--format", "json"], 0, ["list_dev_app_members"]),
    ("member-list-appid-removed", ["dev", "app", "member", "list", "--app-id", "u-1", "--dry-run", "--format", "json"], 1, ["unknown flag"]),
    ("member-list-missing", ["dev", "app", "member", "list", "--dry-run", "--format", "json"], 1, ["--unified-app-id"]),
    ("member-add", ["dev", "app", "member", "add", U, "u-1", "--users", "a,b", "--member-type", "DEVELOPER", "--dry-run", "--format", "json"], 0, ["add_dev_app_members", '"memberUserIds"']),
    ("member-add-trim", ["dev", "app", "member", "add", U, "u-1", "--users", " a , b ", "--member-type", "DEVELOPER", "--dry-run", "--format", "json"], 0, ['"a"', '"b"']),
    ("member-add-no-users", ["dev", "app", "member", "add", U, "u-1", "--member-type", "DEVELOPER", "--dry-run", "--format", "json"], 1, ["--users"]),
    ("member-remove", ["dev", "app", "member", "remove", U, "u-1", "--users", "a", "--member-type", "DEVELOPER", "--dry-run", "--format", "json"], 0, ["remove_dev_app_members"]),
    ("member-add-guard", ["dev", "app", "member", "add", U, "u-1", "--users", "a", "--member-type", "DEVELOPER", "--format", "json"], 1, []),

    # ---- security ----
    ("sec-ip", ["dev", "app", "security", "config", U, "u-1", "--ip-whitelist", "192.0.2.1,192.0.2.2", "--dry-run", "--format", "json"], 0, ["update_dev_app_security_config", '"ipWhitelist"']),
    ("sec-redirect", ["dev", "app", "security", "config", U, "u-1", "--redirect-url", "https://x.invalid/cb", "--dry-run", "--format", "json"], 0, ['"redirectUrls"']),
    ("sec-sso", ["dev", "app", "security", "config", U, "u-1", "--sso-url", "https://x.invalid/sso", "--dry-run", "--format", "json"], 0, ['"ssoUrls"']),
    ("sec-combined", ["dev", "app", "security", "config", "--unified-app-id", "u-1", "--ip-whitelist", "192.0.2.1", "--redirect-url", "https://x.invalid/cb", "--dry-run", "--format", "json"], 0, ['"ipWhitelist"', '"redirectUrls"']),
    ("sec-no-field", ["dev", "app", "security", "config", U, "u-1", "--dry-run", "--format", "json"], 1, ["至少提供一项安全配置"]),
    ("sec-guard", ["dev", "app", "security", "config", U, "u-1", "--ip-whitelist", "192.0.2.1", "--format", "json"], 1, []),

    # ---- robot ----
    ("robot-submit", ["dev", "app", "robot", "submit", "--app-name", "智能体", "--robot-name", "小助手", "--desc", "审批", "--task-id", "t-1", "--dry-run", "--format", "json"], 0, ["submit_robot_create_task", '"taskId": "t-1"']),
    ("robot-result", ["dev", "app", "robot", "result", "--task-id", "t-1", "--format", "json", "--dry-run"], 0, ["query_robot_create_result"]),
    ("robot-result-missing", ["dev", "app", "robot", "result", "--format", "json", "--dry-run"], 1, []),
    ("robot-get", ["dev", "app", "robot", "get", U, "u-1", "--dry-run", "--format", "json"], 0, ["get_extension_robot_config"]),
    ("robot-config-upsert", ["dev", "app", "robot", "config", U, "u-1", "--name", "小助手", "--mode", "2", "--skills", "qa,approval", "--dry-run", "--format", "json"], 0, ["set_extension_robot_config", '"skillList"']),
    # update 子命令已并入 config（upsert）：robot update 落到 robot 组、报错重定向到 robot --help
    ("robot-update-removed", ["dev", "app", "robot", "update", U, "u-1", "--brief", "新简介", "--dry-run", "--format", "json"], 1, ["robot --help"]),
    ("robot-enable-pure", ["dev", "app", "robot", "enable", U, "u-1", "--dry-run", "--format", "json"], 0, ["enable_dev_app_robot"]),
    ("robot-config-no-field", ["dev", "app", "robot", "config", U, "u-1", "--dry-run", "--format", "json"], 1, []),
    ("robot-disable", ["dev", "app", "robot", "disable", U, "u-1", "--dry-run", "--format", "json"], 0, ["disable_dev_app_robot"]),
    ("robot-connect-removed", ["dev", "app", "robot", "connect", "--dry-run"], 1, []),

    # ---- version ----
    ("ver-create", ["dev", "app", "version", "create", U, "u-1", "--version", "1.0.1", "--desc", "d", "--dry-run", "--format", "json"], 0, ["create_dev_app_version", '"version": "1.0.1"']),
    ("ver-list", ["dev", "app", "version", "list", U, "u-1", "--page-size", "20", "--dry-run", "--format", "json"], 0, ["list_dev_app_versions", '"pageSize": 20']),
    ("ver-get", ["dev", "app", "version", "get", U, "u-1", "--version-id", "v-1", "--dry-run", "--format", "json"], 0, ["get_dev_app_version_detail"]),
    ("ver-check", ["dev", "app", "version", "check-approval", U, "u-1", "--version-id", "v-1", "--dry-run", "--format", "json"], 0, ["publish_dev_app_version"]),
    ("ver-publish", ["dev", "app", "version", "publish", U, "u-1", "--version-id", "v-1", "--approver", "user1", "--confirm-sensitive", "--dry-run", "--format", "json"], 0, ["publish_dev_app_version", '"approverUserId"', '"confirmedSensitive"']),
    ("ver-publish-guard", ["dev", "app", "version", "publish", U, "u-1", "--version-id", "v-1", "--format", "json"], 1, []),
    ("ver-status", ["dev", "app", "version", "status", U, "u-1", "--version-id", "v-1", "--dry-run", "--format", "json"], 0, ["get_dev_app_version_status"]),
    ("event-list", ["dev", "app", "event", "list", U, "u-1", "--page-size", "10", "--dry-run", "--format", "json"], 0, ["list_dev_app_events", '"pageSize": 10']),
    ("event-sub", ["dev", "app", "event", "subscribe", U, "u-1", "--event-types", "t1,t2", "--callback-url", "https://x.invalid", "--dry-run", "--format", "json"], 0, ["subscribe_dev_app_event", '"eventTypes"']),
    ("event-unsub", ["dev", "app", "event", "unsubscribe", U, "u-1", "--event-types", "t1", "--dry-run", "--format", "json"], 0, ["unsubscribe_dev_app_event"]),
    ("event-list-missing", ["dev", "app", "event", "list", "--dry-run", "--format", "json"], 1, []),
    ("ver-create-missing", ["dev", "app", "version", "create", U, "u-1", "--dry-run", "--format", "json"], 1, []),

    # ---- dev connect ----
    ("connect-dry-flags", ["dev", "connect", "--channel", "claudecode", "--robot-client-id", "id1", "--robot-client-secret", "s1", "--dry-run", "--format", "json"], 0, ['"channel": "claudecode"', '"cli"', '"connect"']),
    ("connect-dry-unified", ["dev", "connect", "--channel", "qoderwork", U, "ua-1", "--dry-run", "--format", "json"], 0, ['"credentialSource"', '"unifiedAppId": "ua-1"']),
    ("connect-bad-channel", ["dev", "connect", "--channel", "nope", "--robot-client-id", "i", "--robot-client-secret", "s", "--dry-run"], 1, ["未知渠道"]),
    ("connect-no-cred", ["dev", "connect", "--channel", "claudecode", "--dry-run", "--format", "json"], 1, ["--robot-client-id"]),
    ("connect-agent-opts", ["dev", "connect", "--channel", "codex", "--robot-client-id", "i", "--robot-client-secret", "s", "--agent-model", "m1", "--agent-workdir", "/tmp", "--agent-memory=false", "--reply-card=false", "--dry-run", "--format", "json"], 0, ['"channel": "codex"']),

    # ---- dev doc ----
    ("doc-search", ["dev", "doc", "search", "--query", "errcode 40035", "--dry-run", "--format", "json"], 0, ["search_open_platform_docs", '"keyword": "errcode 40035"']),
    ("doc-search-positional", ["dev", "doc", "search", "回调失败", "--dry-run", "--format", "json"], 0, ['"keyword": "回调失败"']),
    ("doc-search-paging", ["dev", "doc", "search", "--query", "x", "--page-size", "5", "--dry-run", "--format", "json"], 0, ['"pageSize": 5']),
    ("doc-search-missing", ["dev", "doc", "search", "--dry-run", "--format", "json"], 1, ["--query"]),

    # ---- pretty 出参 ----
    ("pretty-no-crash", ["dev", "app", "get", U, "u-1", "--dry-run", "--format", "pretty"], 0, []),
]


def run_case(case):
    cid, args, want_exit, want_subs = case
    started = time.time()
    try:
        proc = subprocess.run(
            [DWS, *args], capture_output=True, text=True, timeout=TIMEOUT
        )
        out = (proc.stdout or "") + (proc.stderr or "")
        exit_ok = (proc.returncode == 0) == (want_exit == 0)
        missing = [s for s in want_subs if s not in out]
        status = "PASS" if exit_ok and not missing else "FAIL"
        return {
            "id": cid, "status": status, "exit": proc.returncode,
            "want_exit": want_exit, "missing": missing,
            "ms": int((time.time() - started) * 1000),
            "cmd": "dws " + " ".join(args),
            "output_head": out[:600],
        }
    except subprocess.TimeoutExpired:
        return {"id": cid, "status": "TIMEOUT", "ms": TIMEOUT * 1000,
                "cmd": "dws " + " ".join(args), "output_head": ""}


def main():
    results = []
    with concurrent.futures.ThreadPoolExecutor(max_workers=PARALLEL) as pool:
        for res in pool.map(run_case, CASES):
            results.append(res)
            print(f"[{res['status']}] {res['id']}", file=sys.stderr)
    with open(sys.argv[2] if len(sys.argv) > 2 else "results.jsonl", "w") as f:
        for r in results:
            f.write(json.dumps(r, ensure_ascii=False) + "\n")
    passed = sum(1 for r in results if r["status"] == "PASS")
    print(f"\n{passed}/{len(results)} PASS", file=sys.stderr)


if __name__ == "__main__":
    main()
