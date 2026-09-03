# Copyright 2026 Alibaba Group
# Licensed under the Apache License, Version 2.0
#
# One-command installer for dws dev on native Windows (PowerShell).
# Downloads the dev binary (dws.exe) + dingtalk-misc skill (hosts open-platform app docs) from the DingTalk-Real-AI GitHub Releases.
#
# Usage:
#   irm https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install-devapp.ps1 | iex
#
# Env (all optional):
#   DEVAPP_REPO      repo holding dev releases (default: DingTalk-Real-AI/dingtalk-workspace-cli)
#   DEVAPP_VERSION   pin a release tag (default: latest release)
#   DWS_ARCH         architecture override (amd64 or arm64)
#   DWS_INSTALL_DIR  binary dir (default: ~/.local/bin)
#   DWS_NO_SKILLS    set 1 to skip the dev skill

$ErrorActionPreference = "Stop"

$Repo       = if ($env:DEVAPP_REPO) { $env:DEVAPP_REPO } else { "DingTalk-Real-AI/dingtalk-workspace-cli" }
$Version    = $env:DEVAPP_VERSION
$InstallDir = if ($env:DWS_INSTALL_DIR) { $env:DWS_INSTALL_DIR } else { Join-Path $HOME ".local\bin" }
$NoSkills   = $env:DWS_NO_SKILLS -eq "1"
$SkillName  = "dingtalk-misc"

# vercel-labs/skills agents.ts (c6f69c6). Each row is
# id|universal|effective global directory; '-' means no global directory.
$AgentRegistryRows = @(
    "aider-desk|0|.aider-desk\skills", "amp|1|.config\agents\skills",
    "antigravity|1|.gemini\antigravity\skills", "antigravity-cli|1|.gemini\antigravity-cli\skills",
    "astrbot|0|.astrbot\data\skills", "autohand-code|0|.autohand\skills", "augment|0|.augment\skills",
    "bob|0|.bob\skills", "claude-code|0|.claude\skills", "openclaw|0|.openclaw\skills",
    "cline|1|.agents\skills", "codearts-agent|0|.codeartsdoer\skills", "codebuddy|0|.codebuddy\skills",
    "codemaker|0|.codemaker\skills", "codestudio|0|.codestudio\skills", "codex|1|.codex\skills",
    "command-code|0|.commandcode\skills", "continue|0|.continue\skills", "cortex|0|.snowflake\cortex\skills",
    "crush|0|.config\crush\skills", "cursor|1|.cursor\skills", "deepagents|1|.deepagents\agent\skills",
    "devin|0|.config\devin\skills", "dexto|1|.agents\skills", "droid|0|.factory\skills", "eve|0|-",
    "firebender|1|.firebender\skills", "forgecode|0|.forge\skills", "gemini-cli|1|.gemini\skills",
    "github-copilot|1|.copilot\skills", "goose|0|.config\goose\skills", "grok|0|.grok\skills",
    "hermes-agent|0|.hermes\skills", "inference-sh|0|.inferencesh\skills", "jazz|0|.jazz\skills",
    "junie|0|.junie\skills", "iflow-cli|0|.iflow\skills", "kilo|0|.kilocode\skills",
    "kimchi|0|.config\kimchi\harness\skills", "kimi-code-cli|1|.agents\skills", "kiro-cli|0|.kiro\skills",
    "kode|0|.kode\skills", "lingma|0|.lingma\skills", "loaf|1|.agents\skills", "mcpjam|0|.mcpjam\skills",
    "minimax-code|0|.minimax\skills", "mistral-vibe|0|.vibe\skills", "moxby|0|.moxby\skills",
    "mux|0|.mux\skills", "opencode|1|.config\opencode\skills", "openhands|0|.openhands\skills",
    "ona|0|.ona\skills", "pi|0|.pi\agent\skills", "qoder|0|.qoder\skills", "qoder-cn|0|.qoder-cn\skills",
    "qwen-code|0|.qwen\skills", "replit|1|.config\agents\skills", "reasonix|0|.reasonix\skills",
    "rovodev|0|.rovodev\skills", "roo|0|.roo\skills", "tabnine-cli|0|.tabnine\agent\skills",
    "terramind|0|.terramind\skills", "tinycloud|0|.tinycloud\skills", "trae|0|.trae\skills",
    "trae-cn|0|.trae-cn\skills", "warp|1|.agents\skills", "windsurf|0|.codeium\windsurf\skills",
    "zed|1|.agents\skills", "zcode|0|.zcode\skills", "zencoder|0|.zencoder\skills",
    "zenflow|0|.zencoder\skills", "neovate|0|.neovate\skills", "pochi|0|.pochi\skills",
    "promptscript|1|-", "adal|0|.adal\skills", "universal|1|.config\agents\skills"
)
$LegacyAgentCleanupRows = @(
    "dws-qoderwork|0|.qoderwork\skills", "dws-legacy-github|1|.github\skills",
    "dws-legacy-amp|1|.amp\skills", "dws-legacy-cline|1|.cline\skills",
    "dws-legacy-windsurf|1|.windsurf\skills"
)

function Say($m) { Write-Host "  $m" }
function Die($m) { Write-Host "  X $m" -ForegroundColor Red; exit 1 }

function Resolve-AgentBase([string]$row) {
    $parts = $row.Split('|')
    $id = $parts[0]
    if ($parts[2] -eq "-") { return $null }
    switch ($id) {
        "autohand-code" { if ($env:AUTOHAND_HOME) { return Join-Path $env:AUTOHAND_HOME "skills" } }
        "claude-code" { if ($env:CLAUDE_CONFIG_DIR) { return Join-Path $env:CLAUDE_CONFIG_DIR "skills" } }
        "codex" { if ($env:CODEX_HOME) { return Join-Path $env:CODEX_HOME "skills" } }
        "grok" { if ($env:GROK_HOME) { return Join-Path $env:GROK_HOME "skills" } }
        "hermes-agent" { if ($env:HERMES_HOME) { return Join-Path $env:HERMES_HOME "skills" } }
        "mistral-vibe" { if ($env:VIBE_HOME) { return Join-Path $env:VIBE_HOME "skills" } }
        "openclaw" {
            foreach ($legacy in @(".openclaw", ".clawdbot", ".moltbot")) {
                $candidate = Join-Path $HOME $legacy
                if (Test-Path $candidate -PathType Container) { return Join-Path $candidate "skills" }
            }
        }
        { $_ -in @("amp", "replit", "universal") } {
            $xdg = if ($env:XDG_CONFIG_HOME) { $env:XDG_CONFIG_HOME } else { Join-Path $HOME ".config" }
            return Join-Path $xdg "agents\skills"
        }
        { $_ -in @("crush", "devin", "goose", "kimchi", "opencode") } {
            $xdg = if ($env:XDG_CONFIG_HOME) { $env:XDG_CONFIG_HOME } else { Join-Path $HOME ".config" }
            $child = switch ($id) { "crush" { "crush\skills" }; "devin" { "devin\skills" }; "goose" { "goose\skills" }; "kimchi" { "kimchi\harness\skills" }; default { "opencode\skills" } }
            return Join-Path $xdg $child
        }
    }
    return Join-Path $HOME $parts[2]
}

function Test-AgentBaseDetected([string]$id, [string]$base) {
    $parent = Split-Path $base -Parent
    switch ($id) {
        "kimchi" { return Test-Path (Split-Path $parent -Parent) -PathType Container }
        "tabnine-cli" { return Test-Path (Split-Path $parent -Parent) -PathType Container }
        "zcode" { return (Test-Path $parent -PathType Container) -or (Test-Path "/Applications/ZCode.app" -PathType Container) }
        "minimax-code" { return (Test-Path $parent -PathType Container) -or (Test-Path "/Applications/MiniMax Code.app" -PathType Container) }
        default { return Test-Path $parent -PathType Container }
    }
}

function Test-PathLexically([string]$path) {
    try { Get-Item -LiteralPath $path -Force -ErrorAction Stop | Out-Null; return $true } catch {}
    $parent = Split-Path $path -Parent; $leaf = Split-Path $path -Leaf
    if (!(Test-Path $parent -PathType Container)) { return $false }
    return $null -ne (Get-ChildItem -LiteralPath $parent -Force -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -eq $leaf } | Select-Object -First 1)
}

function Move-DevSkillPath([string]$Source, [string]$Destination) {
    if (Test-PathLexically $Destination) { throw "Skill move destination already exists: $Destination" }
    $sourceItem = Get-Item -LiteralPath $Source -Force -ErrorAction Stop
    if ($sourceItem.PSIsContainer) {
        [System.IO.Directory]::Move($Source, $Destination)
    } else {
        [System.IO.File]::Copy($Source, $Destination, $false)
        try {
            Remove-DevSkillPathLexically $Source
        } catch {
            $removeErr = $_
            try {
                Remove-DevSkillPathLexically $Destination
            } catch {
                throw "Skill move state uncertain; source $Source and dest $Destination retained: $removeErr; retract failed: $_"
            }
            throw $removeErr
        }
    }
}

function Test-DevCrossDeviceMoveError([System.Management.Automation.ErrorRecord]$Record) {
    $exception = $Record.Exception
    while ($null -ne $exception) {
        if (($exception.HResult -band 0xffff) -eq 17) { return $true }
        $exception = $exception.InnerException
    }
    return $false
}

function Copy-DevSkillMetadata($SourceItem, [string]$Destination) {
    (Get-Item -LiteralPath $Destination -Force -ErrorAction Stop).Attributes = $SourceItem.Attributes
    $nativeWindows = $env:OS -eq "Windows_NT" -or $PSVersionTable.PSEdition -eq "Desktop"
    if ($nativeWindows) {
        Set-Acl -LiteralPath $Destination -AclObject (Get-Acl -LiteralPath $SourceItem.FullName -ErrorAction Stop) -ErrorAction Stop
    } else {
        $mode = [System.IO.File]::GetUnixFileMode($SourceItem.FullName)
        [System.IO.File]::SetUnixFileMode($Destination, $mode)
    }
}

function Get-DevSkillPermissionFingerprint([string]$Path) {
    $nativeWindows = $env:OS -eq "Windows_NT" -or $PSVersionTable.PSEdition -eq "Desktop"
    if ($nativeWindows) { return (Get-Acl -LiteralPath $Path -ErrorAction Stop).Sddl }
    return [string][System.IO.File]::GetUnixFileMode($Path)
}

function Copy-DevSkillPathLexically([string]$Source, [string]$Destination) {
    $item = Get-Item -LiteralPath $Source -Force -ErrorAction Stop
    if ($item.LinkType) {
        $itemType = if ($item.LinkType -eq "Junction") { "Junction" } else { "SymbolicLink" }
        New-Item -ItemType $itemType -Path $Destination -Target $item.Target -ErrorAction Stop | Out-Null
        return
    }
    if ($item.PSIsContainer) {
        New-Item -ItemType Directory -Path $Destination -ErrorAction Stop | Out-Null
        foreach ($child in @(Get-ChildItem -LiteralPath $Source -Force -ErrorAction Stop)) {
            Copy-DevSkillPathLexically $child.FullName (Join-Path $Destination $child.Name)
        }
        Copy-DevSkillMetadata $item $Destination
        return
    }
    if ($item -isnot [System.IO.FileInfo]) { throw "unsupported special Skill path $Source" }
    [System.IO.File]::Copy($Source, $Destination, $false)
    Copy-DevSkillMetadata $item $Destination
}

function Assert-DevSkillPathCopy([string]$Source, [string]$Destination) {
    $sourceItem = Get-Item -LiteralPath $Source -Force -ErrorAction Stop
    $destinationItem = Get-Item -LiteralPath $Destination -Force -ErrorAction Stop
    if ([bool]$sourceItem.LinkType -ne [bool]$destinationItem.LinkType -or
        $sourceItem.PSIsContainer -ne $destinationItem.PSIsContainer) {
        throw "Skill path type mismatch: $Source != $Destination"
    }
    if ($sourceItem.LinkType) {
        if ($sourceItem.LinkType -ne $destinationItem.LinkType -or
            ($sourceItem.Target -join "`0") -ne ($destinationItem.Target -join "`0")) {
            throw "Skill link target mismatch: $Source != $Destination"
        }
        return
    }
    if ((Get-DevSkillPermissionFingerprint $Source) -ne (Get-DevSkillPermissionFingerprint $Destination)) {
        throw "Skill path permissions mismatch: $Source != $Destination"
    }
    if ($sourceItem.PSIsContainer) {
        $sourceChildren = @(Get-ChildItem -LiteralPath $Source -Force -ErrorAction Stop | Sort-Object -Property Name)
        $destinationChildren = @(Get-ChildItem -LiteralPath $Destination -Force -ErrorAction Stop | Sort-Object -Property Name)
        if ($sourceChildren.Count -ne $destinationChildren.Count) { throw "Skill directory entries mismatch: $Source != $Destination" }
        for ($i = 0; $i -lt $sourceChildren.Count; $i++) {
            if ($sourceChildren[$i].Name -ne $destinationChildren[$i].Name) { throw "Skill directory entries mismatch: $Source != $Destination" }
            Assert-DevSkillPathCopy $sourceChildren[$i].FullName $destinationChildren[$i].FullName
        }
        return
    }
    if ($sourceItem.Length -ne $destinationItem.Length) { throw "Skill file size mismatch: $Source != $Destination" }
    if ((Get-FileHash -LiteralPath $Source -Algorithm SHA256 -ErrorAction Stop).Hash -ne
        (Get-FileHash -LiteralPath $Destination -Algorithm SHA256 -ErrorAction Stop).Hash) {
        throw "Skill file digest mismatch: $Source != $Destination"
    }
}

function Remove-DevSkillPathLexically([string]$Path) {
    $item = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
    if ($item.PSIsContainer -and !$item.LinkType) {
        Remove-Item -LiteralPath $Path -Recurse -Force -ErrorAction Stop
    } else {
        Remove-Item -LiteralPath $Path -Force -ErrorAction Stop
    }
}

# Removes a staging directory that may hold junction/symlink children. Link
# children must be removed non-recursively first: Windows PowerShell 5.1 can
# follow reparse points during Remove-Item -Recurse and would delete the
# canonical store's contents through the staged link.
function Remove-DevLinkStageRoot([string]$StageRoot) {
    if (!(Test-Path -LiteralPath $StageRoot)) { return $true }
    $ok = $true
    foreach ($child in @(Get-ChildItem -LiteralPath $StageRoot -Force -ErrorAction SilentlyContinue)) {
        try { Remove-DevSkillPathLexically $child.FullName } catch { $ok = $false }
    }
    try { Remove-Item -LiteralPath $StageRoot -Force -ErrorAction Stop } catch { $ok = $false }
    return $ok
}

function Move-DevSkillPathRecoverably([string]$Source, [string]$Destination) {
    if (Test-PathLexically $Destination) { throw "move destination already exists: $Destination" }
    $destinationParent = Split-Path $Destination -Parent
    New-Item -ItemType Directory -Path $destinationParent -Force -ErrorAction Stop | Out-Null
    try {
        Move-DevSkillPath $Source $Destination
        return
    } catch {
        if (!(Test-DevCrossDeviceMoveError $_)) { throw }
    }
    $stageRoot = Join-Path $destinationParent ("." + (Split-Path $Destination -Leaf) + ".cross-device-" + [guid]::NewGuid().ToString("N"))
    $stage = Join-Path $stageRoot "payload"
    New-Item -ItemType Directory -Path $stageRoot -ErrorAction Stop | Out-Null
    $published = $false
    try {
        Copy-DevSkillPathLexically $Source $stage
        Assert-DevSkillPathCopy $Source $stage
        Move-DevSkillPath $stage $Destination
        $published = $true
        try {
            Assert-DevSkillPathCopy $Source $Destination
            if (!(Remove-DevLinkStageRoot $stageRoot)) {
                throw "Skill staging cleanup failed: $stageRoot"
            }
        } catch {
            $postErr = $_
            try {
                # Retract only after proving the destination is still this
                # transaction's copy (mirrors install.ps1's
                # Remove-PublishedSkillPathSafely retract): a path-blind
                # lexical removal would delete a concurrent replacement.
                if (Test-PathLexically $Destination) {
                    Remove-VerifiedDevSkillPublication $Destination $Source ""
                }
            } catch {
                throw "Skill move state uncertain: $postErr; failed to retract $Destination`: $_; source $Source and dest $Destination retained"
            }
            throw "Skill move failed, dest retracted, source retained ${Source}: $postErr"
        }
        try { Remove-DevSkillPathLexically $Source } catch {
            throw "Skill target published but source removal failed; both retained ($Source, $Destination): $_"
        }
        if (Test-PathLexically $Source) { throw "Skill source still exists; both retained ($Source, $Destination)" }
    } catch {
        $failure = $_
        if (Test-Path -LiteralPath $stageRoot) {
            if (!(Remove-DevLinkStageRoot $stageRoot)) {
                if (-not $published) {
                    throw "$failure; cross-device Skill staging cleanup failed $stageRoot (backup and original retained)"
                }
            }
        }
        throw $failure
    }
}

# Get-DevSkillBackupName encodes the HOME-relative path of a backed-up Skill
# directory ('.codex\skills\dingtalk-misc' → '.codex-skills-dingtalk-misc') so
# copies retired from different Agent roots stay distinguishable. Mirrors
# build/npm/install.js and internal/upgrade/paths.go; paths outside HOME fall
# back to the bare leaf.
function Get-DevSkillBackupName([string]$Dir) {
    $leaf = Split-Path $Dir -Leaf
    try {
        $full = [System.IO.Path]::GetFullPath($Dir)
        $root = [System.IO.Path]::GetFullPath($HOME).TrimEnd([char[]]@('\', '/'))
    } catch { return $leaf }
    if ([string]::IsNullOrWhiteSpace($root)) { return $leaf }
    foreach ($sep in @('\', '/')) {
        $prefix = $root + $sep
        if ($full.Length -gt $prefix.Length -and
            $full.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)) {
            $rel = $full.Substring($prefix.Length).Trim([char[]]@('\', '/'))
            if (![string]::IsNullOrWhiteSpace($rel)) { return ($rel -replace '[\\/]+', '-') }
        }
    }
    return $leaf
}

# Keep $HOME\.dws\skill-backups bounded to the newest stamped roots, matching
# skillBackupKeep / pruneSkillBackups in internal/upgrade/paths.go. Roots this
# run created are never pruned: a rollback still needs them.
$DevSkillBackupKeep = 5
$script:DevSkillBackupRootsThisRun = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)

# Ownership marker: every stamp root DWS creates carries .dws-skill-backup
# with exactly "dws skill backup v1" + LF — the same bytes internal/upgrade/
# paths.go, install.sh, and build/npm/install.js write. A stamp-shaped name
# alone is not ownership proof, so pruning only deletes directories whose
# marker content verifies.
$DevSkillBackupMarkerFile = ".dws-skill-backup"
$DevSkillBackupMarkerBody = "dws skill backup v1"

# Write-DevSkillBackupMarker stamps a freshly created stamp root as DWS-owned.
# [IO.File]::WriteAllText pins the exact LF-terminated bytes (Set-Content
# would append a platform newline on Windows PowerShell 5.1).
function Write-DevSkillBackupMarker([string]$Root) {
    [System.IO.File]::WriteAllText(
        (Join-Path $Root $DevSkillBackupMarkerFile),
        "$DevSkillBackupMarkerBody`n",
        [System.Text.UTF8Encoding]::new($false))
}

# Test-DevSkillBackupMarker reports whether a stamp root carries the ownership
# marker. The check normalizes CRLF→LF and drops trailing newlines before
# comparing, so this surface accepts every surface's exact-LF bytes (writer
# and checker agree); any other content, or a missing/unreadable marker,
# means foreign data.
function Test-DevSkillBackupMarker([string]$Dir) {
    try {
        $marker = Join-Path $Dir $DevSkillBackupMarkerFile
        if (![System.IO.File]::Exists($marker)) { return $false }
        $body = [System.IO.File]::ReadAllText($marker).Replace("`r`n", "`n").TrimEnd("`r", "`n")
        return ($body -eq $DevSkillBackupMarkerBody)
    } catch {
        return $false
    }
}

# Removes a whole stamp root child-first without ever following a reparse
# point: link children are deleted non-recursively, real directories are
# recursed the same way, and only an emptied directory is removed. Backup
# trees can contain junctions/symlinks (victims are collected before the
# physical-equality filter), and Windows PowerShell 5.1 can follow reparse
# points during Remove-Item -Recurse — the invariant Remove-DevLinkStageRoot
# enforces for staging roots, applied at every depth here.
function Remove-DevSkillBackupTreeLexically([string]$Path) {
    $item = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
    if ($item.PSIsContainer -and !$item.LinkType) {
        foreach ($child in @(Get-ChildItem -LiteralPath $Path -Force -ErrorAction Stop)) {
            Remove-DevSkillBackupTreeLexically $child.FullName
        }
    }
    Remove-Item -LiteralPath $Path -Force -ErrorAction Stop
}

function Remove-OldDevSkillBackups {
    $root = Join-Path $HOME ".dws\skill-backups"
    if (!(Test-Path -LiteralPath $root -PathType Container)) { return }
    # Only directories whose names match the DWS backup stamp format (UTC
    # yyyyMMdd-HHmmss, optional -N collision suffix) AND whose ownership
    # marker verifies are candidates; anything else is foreign data —
    # preserved and never counted against $DevSkillBackupKeep.
    $dirs = @(Get-ChildItem -LiteralPath $root -Directory -Force -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -match '^[0-9]{8}-[0-9]{6}(-[0-9]+)?$' -and (Test-DevSkillBackupMarker $_.FullName) } |
        Sort-Object -Property Name)
    $excess = $dirs.Count - $DevSkillBackupKeep
    foreach ($dir in $dirs) {
        if ($excess -le 0) { break }
        if ($script:DevSkillBackupRootsThisRun.Contains($dir.FullName)) { continue }
        try { Remove-DevSkillBackupTreeLexically $dir.FullName } catch { }
        $excess--
    }
}

function Backup-DevSkill([string]$path, [ref]$BackupPath) {
    if ($null -ne $BackupPath) { $BackupPath.Value = "" }
    if (!(Test-PathLexically $path)) { return }
    $backupBase = Join-Path $HOME ".dws\skill-backups"
    $stamp = [DateTime]::UtcNow.ToString('yyyyMMdd-HHmmss')
    $name = Get-DevSkillBackupName $path
    $backupRoot = Join-Path $backupBase $stamp
    $target = Join-Path $backupRoot $name
    $i = 1
    while (Test-PathLexically $target) {
        $backupRoot = Join-Path $backupBase "$stamp-$i"
        $target = Join-Path $backupRoot $name
        $i++
        if ($i -gt 1000) { throw "备份目录冲突无法解决，保留原目录: $path" }
    }
    New-Item -ItemType Directory -Path $backupRoot -Force -ErrorAction Stop | Out-Null
    # Stamp ownership immediately after creating the stamp root and before
    # any skill directory moves into it, so an interrupted backup can never
    # leave an unmarked (never-prunable) stamp behind.
    try {
        Write-DevSkillBackupMarker $backupRoot
    } catch {
        # The removal stays non-recursive so a pre-existing non-empty root
        # (foreign data) is never destroyed; a failed marker write must not
        # leave an empty unowned stamp root behind either.
        Remove-Item -LiteralPath $backupRoot -Force -ErrorAction SilentlyContinue
        throw
    }
    Move-DevSkillPathRecoverably $path $target
    if ($null -ne $BackupPath) { $BackupPath.Value = $target }
    try {
        $script:DevSkillBackupRootsThisRun.Add([System.IO.Path]::GetFullPath($backupRoot)) | Out-Null
        Remove-OldDevSkillBackups
    } catch {
        Say "⚠️ 旧备份清理失败（备份本身已成功）: $($_.Exception.Message)"
    }
}

function Get-DevSkillLinkSignature([string]$Path) {
    try { $item = Get-Item -LiteralPath $Path -Force -ErrorAction Stop } catch { return $null }
    if (!$item.LinkType) { return $null }
    return ([string]$item.LinkType + "`0" + ((@($item.Target) | ForEach-Object { [string]$_ }) -join "`0"))
}

function New-DevRollbackFailure([string]$Message) {
    $failure = [System.InvalidOperationException]::new($Message)
    $failure.Data["DWSRollbackFailed"] = $true
    return $failure
}

function Remove-VerifiedDevSkillPublication(
    [string]$Destination,
    [string]$PublishedSource,
    [string]$PublishedLinkSignature
) {
    $parent = Split-Path $Destination -Parent
    $quarantine = Join-Path $parent (".dws-dev-rollback-" + [guid]::NewGuid().ToString("N"))
    Move-DevSkillPath $Destination $quarantine
    try {
        if (![string]::IsNullOrWhiteSpace($PublishedLinkSignature)) {
            if ((Get-DevSkillLinkSignature $quarantine) -ne $PublishedLinkSignature) {
                throw "published Skill link was replaced by another process: $Destination"
            }
        } elseif (![string]::IsNullOrWhiteSpace($PublishedSource)) {
            Assert-DevSkillPathCopy $PublishedSource $quarantine
        } else {
            throw "Skill restore destination is occupied by another process: $Destination"
        }
        Remove-DevSkillPathLexically $quarantine
    } catch {
        $failure = $_
        if (Test-PathLexically $quarantine) {
            try {
                if (Test-PathLexically $Destination) { throw "original path is occupied" }
                Move-DevSkillPath $quarantine $Destination
            } catch {
                throw "$failure; concurrent object retained at $quarantine`: $_"
            }
        }
        throw $failure
    }
}

function Restore-DevSkillBackup(
    [string]$Destination,
    [string]$Backup,
    [string]$PublishedSource = "",
    [string]$PublishedLinkSignature = ""
) {
    if (Test-PathLexically $Destination) {
        Remove-VerifiedDevSkillPublication $Destination $PublishedSource $PublishedLinkSignature
    }
    if (![string]::IsNullOrWhiteSpace($Backup)) {
        Move-DevSkillPathRecoverably $Backup $Destination
    }
}

function Publish-DevSkillCopy([string]$Source, [string]$Destination) {
    $parent = Split-Path $Destination -Parent
    New-Item -ItemType Directory -Path $parent -Force -ErrorAction Stop | Out-Null
    $stageRoot = Join-Path $parent (".dws-dev-copy-" + [guid]::NewGuid().ToString("N"))
    $stage = Join-Path $stageRoot "payload"
    $backup = ""
    $publishedSource = ""
    New-Item -ItemType Directory -Path $stageRoot -ErrorAction Stop | Out-Null
    try {
        Copy-DevSkillPathLexically $Source $stage
        Assert-DevSkillPathCopy $Source $stage
        Backup-DevSkill $Destination ([ref]$backup)
        try {
            Move-DevSkillPath $stage $Destination
            $publishedSource = $Source
            Assert-DevSkillPathCopy $Source $Destination
        } catch {
            $publishFailure = $_
            try { Restore-DevSkillBackup $Destination $backup $publishedSource "" } catch {
                throw (New-DevRollbackFailure "$publishFailure; Skill rollback failed; backup retained at $backup`: $_")
            }
            throw $publishFailure
        }
    } finally {
        Remove-DevLinkStageRoot $stageRoot | Out-Null
    }
}

function Publish-DevSkillJunction([string]$Target, [string]$Destination) {
    $parent = Split-Path $Destination -Parent
    New-Item -ItemType Directory -Path $parent -Force -ErrorAction Stop | Out-Null
    $stageRoot = Join-Path $parent (".dws-dev-link-" + [guid]::NewGuid().ToString("N"))
    $stage = Join-Path $stageRoot "payload"
    $backup = ""
    $publishedSignature = ""
    New-Item -ItemType Directory -Path $stageRoot -ErrorAction Stop | Out-Null
    try {
        New-Item -ItemType Junction -Path $stage -Target ([System.IO.Path]::GetFullPath($Target)) -ErrorAction Stop | Out-Null
        Backup-DevSkill $Destination ([ref]$backup)
        try {
            Move-DevSkillPath $stage $Destination
            $published = Get-Item -LiteralPath $Destination -Force -ErrorAction Stop
            if (!$published.LinkType) { throw "published Skill path is not a junction: $Destination" }
            $publishedSignature = Get-DevSkillLinkSignature $Destination
            if ([string]::IsNullOrEmpty($publishedSignature)) { throw "published Skill link identity is unavailable: $Destination" }
        } catch {
            $publishFailure = $_
            try { Restore-DevSkillBackup $Destination $backup "" $publishedSignature } catch {
                throw (New-DevRollbackFailure "$publishFailure; Skill rollback failed; backup retained at $backup`: $_")
            }
            throw $publishFailure
        }
    } finally {
        # The staged payload is a junction to the canonical store; it must be
        # removed lexically or Windows PowerShell 5.1 recursion could delete
        # canonical content through the reparse point.
        Remove-DevLinkStageRoot $stageRoot | Out-Null
    }
}

function Get-Arch {
    if ($env:DWS_ARCH -eq "amd64" -or $env:DWS_ARCH -eq "arm64") { return $env:DWS_ARCH }
    try {
        switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture) {
            "X64"   { return "amd64" }
            "Arm64" { return "arm64" }
        }
    } catch {}
    switch ($env:PROCESSOR_ARCHITECTURE) {
        "AMD64" { return "amd64" }
        "ARM64" { return "arm64" }
        default { Die "Could not detect architecture. Set DWS_ARCH to amd64 or arm64." }
    }
}

function Assert-DevReleaseAssetChecksum([string]$AssetPath, [string]$AssetName, [string]$TempDir) {
    $checksums = Join-Path $TempDir "checksums.txt"
    if (!(Test-Path -LiteralPath $checksums -PathType Leaf)) {
        try {
            Invoke-WebRequest -Uri "https://github.com/$Repo/releases/download/$Version/checksums.txt" `
                -OutFile $checksums -UseBasicParsing -ErrorAction Stop
        } catch {
            Die "Could not download checksums.txt; refusing unverified release assets."
        }
    }
    $line = Get-Content -LiteralPath $checksums | Where-Object {
        $_ -match "^[0-9A-Fa-f]{64}[ ]+[*]?$([regex]::Escape($AssetName))$"
    } | Select-Object -First 1
    if (-not $line) { Die "$AssetName is missing from checksums.txt." }
    $expected = ($line -split '\s+')[0].ToLowerInvariant()
    $actual = (Get-FileHash -LiteralPath $AssetPath -Algorithm SHA256 -ErrorAction Stop).Hash.ToLowerInvariant()
    if ($actual -ne $expected) { Die "SHA256 checksum mismatch for $AssetName." }
    Say "SHA256 checksum verified: $AssetName"
}

# Read the releases list (newest first) and take the top tag, so this also works
# if a release is ever published as a prerelease (which /releases/latest skips).
if (-not $Version) {
    try {
        $rel = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases?per_page=1" `
            -Headers @{ "User-Agent" = "dws-devapp-installer" } -UseBasicParsing
        $Version = $rel[0].tag_name
    } catch {}
    if (-not $Version) { Die "No release found on $Repo. Set DEVAPP_VERSION to a published release tag." }
}

$arch = Get-Arch
$tmp  = Join-Path $env:TEMP ("dws-dev-" + [System.Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $tmp -Force | Out-Null

Write-Host ""
Say "dws dev installer (Windows, pre-built binary)"
Say "Repo:    $Repo"
Say "Version: $Version"
Say "Target:  windows/$arch"
Write-Host ""

# 1) binary
$asset = "dws-windows-$arch.zip"
$zip   = Join-Path $tmp $asset
Say "Downloading $asset ..."
try {
    Invoke-WebRequest -Uri "https://github.com/$Repo/releases/download/$Version/$asset" `
        -OutFile $zip -UseBasicParsing
} catch { Die "Binary download failed - does release $Version have $asset?" }
Assert-DevReleaseAssetChecksum $zip $asset $tmp

Expand-Archive -Path $zip -DestinationPath $tmp -Force
$exe = Get-ChildItem -Path $tmp -Recurse -Filter "dws.exe" | Select-Object -First 1
if (-not $exe) { Die "dws.exe not found inside $asset" }
if (-not (Test-Path $InstallDir)) { New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null }
Copy-Item -Path $exe.FullName -Destination (Join-Path $InstallDir "dws.exe") -Force
Say "Binary -> $InstallDir\dws.exe"

# 2) dev skill from the release's skills bundle
if (-not $NoSkills) {
    try {
        $skzip = Join-Path $tmp "dws-skills.zip"
        Invoke-WebRequest -Uri "https://github.com/$Repo/releases/download/$Version/dws-skills.zip" `
            -OutFile $skzip -UseBasicParsing
        Assert-DevReleaseAssetChecksum $skzip "dws-skills.zip" $tmp
        $skdir = Join-Path $tmp "sk"
        Expand-Archive -Path $skzip -DestinationPath $skdir -Force

        $src = $null
        foreach ($c in @("multi\$SkillName", "skills\multi\$SkillName", "$SkillName")) {
            $p = Join-Path $skdir $c
            if (Test-Path (Join-Path $p "SKILL.md")) { $src = $p; break }
        }
        if ($src) {
            # cache so `dws skill setup --mode multi` can find a source later
            $cache = Join-Path $HOME ".dws\skills\multi\$SkillName"
            Publish-DevSkillCopy $src $cache

            $canonicalBase = Join-Path $HOME ".agents\skills"
            $canonical = Join-Path $canonicalBase $SkillName
            Publish-DevSkillCopy $src $canonical
            $installed = 1
            $failed = 0
            $retireFailed = 0
            $seen = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
            foreach ($row in @($AgentRegistryRows) + @($LegacyAgentCleanupRows)) {
                $parts = $row.Split('|'); $universal = $parts[1] -eq "1"
                $base = Resolve-AgentBase $row
                if ([string]::IsNullOrWhiteSpace($base)) { continue }
                $baseKey = [System.IO.Path]::GetFullPath($base).TrimEnd([char[]]@('\', '/'))
                if (!$seen.Add($baseKey) -or $baseKey -eq [System.IO.Path]::GetFullPath($canonicalBase).TrimEnd([char[]]@('\', '/'))) { continue }
                if (!(Test-AgentBaseDetected $parts[0] $base)) { continue }
                $dest = Join-Path $base $SkillName
                if ($universal) {
                    # Cleanup-only migration: universal Agents read the
                    # canonical store directly, so nothing is installed here. A
                    # retire failure only leaves an obsolete copy behind and
                    # must never fail an otherwise complete install.
                    try { Backup-DevSkill $dest } catch {
                        Say "⚠️ Agent Skill 旧副本备份失败，保留原目录（不影响本次安装）: $dest ($($_.Exception.Message))"
                        $retireFailed++
                    }
                    continue
                }
                # Per-agent degrade like install.sh: a failed link→copy fallback
                # skips that agent loudly instead of aborting the whole loop.
                try {
                    New-Item -ItemType Directory -Path $base -Force -ErrorAction Stop | Out-Null
                    try { Publish-DevSkillJunction $canonical $dest }
                    catch {
                        # A failed rollback leaves the old backup as the only
                        # trusted copy. Do not run a second publisher over that
                        # state; surface the failure and continue other agents.
                        if ($_.Exception.Data["DWSRollbackFailed"]) { throw }
                        Publish-DevSkillCopy $canonical $dest
                    }
                } catch {
                    Say "⚠️ Agent Skill 目标安装失败，已跳过: $dest ($($_.Exception.Message))"
                    $failed++
                    continue
                }
                $installed++
            }
            if ($failed -gt 0) { Die "有 $failed 个 Agent 目标安装 $SkillName 失败（其余目标已安装）" }
            if ($retireFailed -gt 0) {
                Say "⚠️ 有 $retireFailed 个 Agent 旧副本未能迁移（安装已完成，可稍后手动删除）"
            }
            Say "Skill dingtalk-misc -> $installed agent dir(s)"
        } else {
            Say "(dingtalk-misc not found in skills bundle; skipped)"
        }
    } catch {
        Die "Skill install failed: $($_.Exception.Message)"
    }
}

Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue

Write-Host ""
Say "Done. Next steps:"
Say "  dws version"
Say "  dws auth login"
Say "  dws dev --help --format json"
Write-Host ""
if (($env:Path -split ';') -notcontains $InstallDir) {
    Say "Note: $InstallDir is not on PATH. Add it (new terminal after):"
    Say "  [Environment]::SetEnvironmentVariable('Path', `"`$env:Path;$InstallDir`", 'User')"
}
