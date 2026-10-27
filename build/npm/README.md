# DingTalk Workspace CLI npm Package

This package installs the `dws` executable and bundles the `DingTalk Workspace`
agent skills for local installation.

By default, npm installs the CLI and bundled Skills. Binary-only consumers can
skip the complete Skill path while keeping platform-archive validation and CLI
extraction:

```bash
DWS_NO_SKILLS=1 npm install -g dingtalk-workspace-cli
```

```powershell
$env:DWS_NO_SKILLS = "1"
npm install -g dingtalk-workspace-cli
```

With the variable set to exactly `1`, the postinstall script does not validate
or extract `dws-skills.zip`, write Agent Skill directories or state, or refresh
`~/.dws/skills`. Without it, Skill installation keeps its existing failure
contract. A failure limited to the optional user-cache refresh is reported as a
warning after the CLI and Agent Skills are installed; run `dws skill setup` to
refresh Skills later.
