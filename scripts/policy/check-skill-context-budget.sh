#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
cd "$ROOT"

python3 scripts/gen_skill_shortcut_sections.py --check

chat_skill="skills/multi/dingtalk-chat/SKILL.md"
event_skill="skills/multi/dingtalk-event/SKILL.md"
mono_skill="skills/mono/SKILL.md"
runtime_contract="skills/multi/dingtalk-shared/references/runtime-contract.md"
chat_target_bytes=10000
chat_max_overage_percent=5
chat_max_bytes=$((chat_target_bytes * (100 + chat_max_overage_percent) / 100))
event_max_bytes=10000
runtime_contract_max_bytes=3000

chat_bytes="$(wc -c < "$chat_skill" | tr -d ' ')"
if [ "$chat_bytes" -gt "$chat_max_bytes" ]; then
	printf '%s\n' \
		"skill context budget exceeded: $chat_skill is ${chat_bytes} bytes (target ${chat_target_bytes}, max ${chat_max_bytes} with ${chat_max_overage_percent}% allowance)" >&2
	exit 1
fi

event_bytes="$(wc -c < "$event_skill" | tr -d ' ')"
if [ "$event_bytes" -gt "$event_max_bytes" ]; then
	printf '%s\n' \
		"skill context budget exceeded: $event_skill is ${event_bytes} bytes (max ${event_max_bytes})" >&2
	exit 1
fi

runtime_contract_bytes="$(wc -c < "$runtime_contract" | tr -d ' ')"
if [ "$runtime_contract_bytes" -gt "$runtime_contract_max_bytes" ]; then
	printf '%s\n' \
		"skill context budget exceeded: $runtime_contract is ${runtime_contract_bytes} bytes (max ${runtime_contract_max_bytes})" >&2
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

for required_heading in \
	"## 最小 DWS 执行契约" \
	"## Golden Route" \
	"## 关键结果语义" \
	"## 按需加载" \
	"## 错误最短路径"
do
	if ! grep -Fq "$required_heading" "$chat_skill"; then
		printf '%s\n' \
			"skill value regression: $chat_skill is missing $required_heading" >&2
		exit 1
	fi
done

for required_route in \
	"+dm" \
	"+send-to-group" \
	"+messages-send" \
	"+chat-messages" \
	"+search-msg" \
	"--download-resources" \
	"+conversation-list-top"
do
	if ! grep -Fq -- "$required_route" "$chat_skill"; then
		printf '%s\n' \
			"skill route regression: $chat_skill is missing $required_route" >&2
		exit 1
	fi
done

for forbidden_route in \
	"## 标准 SOP" \
	"dws aisearch person --keyword" \
	"dws chat message send-by-webhook" \
	"dt_media_upload" \
	"MUST 先用 Read"
do
	if grep -Fq "$forbidden_route" "$chat_skill"; then
		printf '%s\n' \
			"skill route regression: $chat_skill restored legacy route: $forbidden_route" >&2
		exit 1
	fi
done

if grep -Fq "../dingtalk-shared/SKILL.md" "$chat_skill"; then
	printf '%s\n' \
		"skill context regression: $chat_skill requires full dingtalk-shared cold-start loading" >&2
	exit 1
fi

if grep -Fq "充分阅读产品参考文件" "$mono_skill"; then
	printf '%s\n' \
		"skill context budget regression: $mono_skill requires full product-reference loading" >&2
	exit 1
fi

printf '%s\n' \
	"skill context budget: ok (chat_bytes=$chat_bytes target=$chat_target_bytes max=$chat_max_bytes allowance=${chat_max_overage_percent}% event_bytes=$event_bytes event_max=$event_max_bytes runtime_contract_bytes=$runtime_contract_bytes runtime_contract_max=$runtime_contract_max_bytes shortcut_rows=$shortcut_rows)"
