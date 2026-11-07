---
category: Fixed
---

- **Windows Skill installation** (#1177) — stops the PowerShell installer from
  rejecting a correct multi/mono Skill publication when the staged copy and the
  destination carry different inherited Windows ACLs, and makes the transaction
  record its published paths before verifying them so a failed publication is
  rolled back instead of leaving the original Skill stranded in
  `~/.dws/skill-backups`.
