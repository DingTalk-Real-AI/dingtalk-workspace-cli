#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"

python3 scripts/gen_skill_shortcut_sections.py --check

chat_skill="skills/multi/dingtalk-chat/SKILL.md"
mono_skill="skills/mono/SKILL.md"
chat_max_bytes=14000

chat_bytes="$(wc -c < "$chat_skill" | tr -d ' ')"
if [ "$chat_bytes" -gt "$chat_max_bytes" ]; then
	printf '%s\n' \
		"skill context budget exceeded: $chat_skill is ${chat_bytes} bytes (max ${chat_max_bytes})" >&2
	exit 1
fi

shortcut_rows="$(
	awk '
		/<!-- VISIBLE_SHORTCUTS_START -->/ { in_block = 1; next }
		/<!-- VISIBLE_SHORTCUTS_END -->/ { in_block = 0 }
		in_block && /^\| `dws chat \+/ { count++ }
		END { print count + 0 }
	' "$chat_skill"
)"
if [ "$shortcut_rows" -ne 0 ]; then
	printf '%s\n' \
		"skill context budget exceeded: $chat_skill re-expanded $shortcut_rows shortcut rows" >&2
	exit 1
fi

if grep -Fq "充分阅读产品参考文件" "$mono_skill"; then
	printf '%s\n' \
		"skill context budget regression: $mono_skill requires full product-reference loading" >&2
	exit 1
fi

printf '%s\n' \
	"skill context budget: ok (chat_bytes=$chat_bytes max=$chat_max_bytes shortcut_rows=$shortcut_rows)"
