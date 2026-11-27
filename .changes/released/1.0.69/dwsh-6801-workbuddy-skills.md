---
category: Added
---

- Add WorkBuddy as a DWS Skill installation target: `dws skill setup --target workbuddy` and `dws skill install <skillId> workbuddy`. Installers and upgrades detect `~/.workbuddy` (or `WORKBUDDY_CONFIG_DIR`) and use the existing shared Skill links with a copy fallback. WorkBuddy connector plugin caches remain independently managed.
