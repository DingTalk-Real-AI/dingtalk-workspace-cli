#!/usr/bin/env python3
"""
DingTalk Bridge Auto-Reply Daemon
=================================
监控 ~/.dingtalk-bridge/queue/ 目录，有新消息时自动调用 LLM 回复。

依赖: pip install openai watchdog

环境变量:
  OPENAI_API_KEY     LLM API Key (必填)
  OPENAI_BASE_URL    LLM API Base URL (默认 https://api.openai.com/v1)
  LLM_MODEL          LLM 模型名 (默认 gpt-4o-mini)
  BRIDGE_TIMEOUT_SEC 单条消息最长等待秒数 (默认 120)
"""

import json
import os
import sys
import time
import threading
from datetime import datetime
from pathlib import Path

BRIDGE_DIR = Path.home() / ".dingtalk-bridge"
QUEUE_DIR = BRIDGE_DIR / "queue"
RESP_DIR = BRIDGE_DIR / "responses"
LOG_FILE = BRIDGE_DIR / "daemon.log"

os.makedirs(QUEUE_DIR, exist_ok=True)
os.makedirs(RESP_DIR, exist_ok=True)


def log(msg: str):
    ts = datetime.now().strftime("%H:%M:%S")
    line = f"[{ts}] {msg}"
    print(line, flush=True)
    try:
        with open(LOG_FILE, "a") as f:
            f.write(line + "\n")
    except Exception:
        pass


# ---- LLM Client ----
class LLMClient:
    def __init__(self):
        self.api_key = os.environ.get("OPENAI_API_KEY", "")
        self.base_url = os.environ.get("OPENAI_BASE_URL", "https://api.openai.com/v1")
        self.model = os.environ.get("LLM_MODEL", "gpt-4o-mini")
        self.timeout = int(os.environ.get("BRIDGE_TIMEOUT_SEC", "120")) - 10

        if not self.api_key:
            log("❌ OPENAI_API_KEY 未设置，守护进程无法自动回复")
            sys.exit(1)

        # Lazy import
        global OpenAI
        from openai import OpenAI as _OpenAI
        self.client = _OpenAI(api_key=self.api_key, base_url=self.base_url)

    def chat(self, user_text: str) -> str:
        try:
            resp = self.client.chat.completions.create(
                model=self.model,
                messages=[
                    {"role": "system", "content": "你是 WorkBuddy 助手，通过钉钉机器人与用户对话。回答简洁、自然、有帮助。用中文回复。"},
                    {"role": "user", "content": user_text},
                ],
                temperature=0.7,
                max_tokens=1024,
            )
            return resp.choices[0].message.content.strip()
        except Exception as e:
            log(f"LLM 调用失败: {e}")
            return f"抱歉，处理你的消息时出了点问题：{e}"


# ---- Queue Processor ----
def process_message(msg_file: Path, llm: LLMClient):
    try:
        raw = msg_file.read_text(encoding="utf-8")
        data = json.loads(raw)
        msg_id = data.get("id", msg_file.stem)
        text = data.get("text", "").strip()
        ts = data.get("timestamp", 0)

        if not text:
            log(f"⚠️  空消息 {msg_id}，跳过")
            msg_file.unlink(missing_ok=True)
            return

        log(f"📩 RECV → {text[:60]}{'...' if len(text) > 60 else ''}")

        reply = llm.chat(text)

        resp_file = RESP_DIR / f"{msg_id}.json"
        resp_file.write_text(
            json.dumps({"id": msg_id, "text": reply, "timestamp": ts}, ensure_ascii=False),
            encoding="utf-8",
        )

        log(f"📤 SEND ← {reply[:60]}{'...' if len(reply) > 60 else ''}")

        # Clean up queue
        msg_file.unlink(missing_ok=True)

    except json.JSONDecodeError:
        log(f"⚠️  非 JSON 文件 {msg_file.name}，跳过")
    except Exception as e:
        log(f"❌ 处理 {msg_file.name} 出错: {e}")


# ---- File Watcher ----
def watch_queue(llm: LLMClient):
    """Poll queue directory for new messages."""
    known: set[str] = set()

    while True:
        try:
            files = sorted(QUEUE_DIR.glob("*.json"))
            for f in files:
                if f.name not in known:
                    known.add(f.name)
                    # Process in a thread so we don't block on LLM call
                    t = threading.Thread(target=process_message, args=(f, llm), daemon=True)
                    t.start()
        except Exception as e:
            log(f"监控出错: {e}")

        time.sleep(0.5)


def main():
    log("🚀 DingTalk Auto-Reply Daemon 启动")

    # Check deps
    try:
        from openai import OpenAI  # noqa: F401
    except ImportError:
        log("❌ 缺少 openai 库，请运行: pip install openai")
        sys.exit(1)

    llm = LLMClient()
    log(f"🔗 LLM: {llm.model} @ {llm.base_url}")

    # Process any existing messages before starting watch
    existing = sorted(QUEUE_DIR.glob("*.json"))
    if existing:
        log(f"📋 发现 {len(existing)} 条待处理消息")
        for f in existing:
            process_message(f, llm)

    watch_queue(llm)


if __name__ == "__main__":
    main()
