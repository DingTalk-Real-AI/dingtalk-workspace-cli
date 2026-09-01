# Copyright 2026 Alibaba Group
# Licensed under the Apache License, Version 2.0
#
# Installer for dws (DingTalk Workspace CLI) on Windows.
# Downloads the pre-built binary from GitHub Releases and installs agent skills.
# No Go, Node.js, or other dependencies required.
#
# Usage (from an existing PowerShell session):
#   irm https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install.ps1 | iex
#
# If you are launching from Win+R or cmd.exe and want the window to stay open:
#   powershell -NoExit -ExecutionPolicy Bypass -Command "irm https://raw.githubusercontent.com/DingTalk-Real-AI/dingtalk-workspace-cli/main/scripts/install.ps1 | iex"
#
# Environment variables (all optional):
#   DWS_INSTALL_DIR   — where to put the binary       (default: ~/.local/bin)
#   DWS_VERSION       — version to install             (default: latest)
#   DWS_ARCH          — architecture override          (amd64 or arm64)
#   DWS_NO_SKILLS     — set to 1 to skip skills install
#   DWS_SKILLS_ONLY   — set to 1 to install only skills
#   DWS_SKILL_MODE    — mono | multi (default: prompt if TTY, else multi)
#   DWS_GITEE_REPO    — "owner/repo" on Gitee; resolve version + assets via the
#                       Gitee API instead of GitHub (China mirror)
#
# Agent skills paths follow build/npm/install.js AGENT_DIRS (order and entries must match).

$ErrorActionPreference = "Stop"

$Repo = "DingTalk-Real-AI/dingtalk-workspace-cli"
$BinName = "dws"
# GitHub "latest release" URL; Resolve-LatestVersion follows its redirect to get the tag.
$LatestUrl = "https://github.com/$Repo/releases/latest"
# China mirror: Gitee repo "owner/repo". When set, version + asset URLs resolve via the Gitee API.
$GiteeRepo = if ($env:DWS_GITEE_REPO) { $env:DWS_GITEE_REPO } else { "" }
# Auto-fallback Gitee mirror used when GitHub is unreachable (see Resolve-Source).
$GiteeFallbackRepo = if ($env:DWS_GITEE_FALLBACK_REPO) { $env:DWS_GITEE_FALLBACK_REPO } else { "DingTalk-Real-AI/dingtalk-workspace-cli" }
$InstallDir = if ($env:DWS_INSTALL_DIR) { $env:DWS_INSTALL_DIR } else { Join-Path $HOME ".local\bin" }
$Version = if ($env:DWS_VERSION) { $env:DWS_VERSION } else { "latest" }
$NoSkills = $env:DWS_NO_SKILLS -eq "1"
$SkillsOnly = $env:DWS_SKILLS_ONLY -eq "1"
$SkillName = "dws"
$SkillMode = ""
$SkillStateRoot = if ($env:DWS_CONFIG_DIR) { $env:DWS_CONFIG_DIR } else { Join-Path $HOME ".dws" }
$ManagedSkillDigestScope = "skill-directory-v1"
$LegacyOfficialMultiSkills = @(
    "dingtalk-agoal", "dingtalk-aiapp", "dingtalk-aisearch", "dingtalk-aitable",
    "dingtalk-attendance", "dingtalk-calendar", "dingtalk-chat", "dingtalk-contact",
    "dingtalk-dev", "dingtalk-devapp", "dingtalk-devdoc", "dingtalk-ding",
    "dingtalk-doc", "dingtalk-drive", "dingtalk-event", "dingtalk-hrbrain",
    "dingtalk-live", "dingtalk-mail", "dingtalk-markdown", "dingtalk-minutes",
    "dingtalk-misc", "dingtalk-oa", "dingtalk-pat", "dingtalk-profile",
    "dingtalk-report", "dingtalk-shared", "dingtalk-sheet", "dingtalk-skill",
    "dingtalk-todo", "dingtalk-wiki", "dws-shared"
)

# Agent registry synchronized with vercel-labs/skills agents.ts (c6f69c6).
# Universal agents read the canonical store directly; their separate historical
# global directories are cleanup-only. Non-universal agents get junctions to
# canonical (or complete copies when junction creation is unavailable).
$AgentRegistry = @(
    [pscustomobject]@{ Id = "aider-desk"; Universal = $false; Dir = ".aider-desk\skills" },
    [pscustomobject]@{ Id = "amp"; Universal = $true; Dir = ".config\agents\skills" },
    [pscustomobject]@{ Id = "antigravity"; Universal = $true; Dir = ".gemini\antigravity\skills" },
    [pscustomobject]@{ Id = "antigravity-cli"; Universal = $true; Dir = ".gemini\antigravity-cli\skills" },
    [pscustomobject]@{ Id = "astrbot"; Universal = $false; Dir = ".astrbot\data\skills" },
    [pscustomobject]@{ Id = "autohand-code"; Universal = $false; Dir = ".autohand\skills" },
    [pscustomobject]@{ Id = "augment"; Universal = $false; Dir = ".augment\skills" },
    [pscustomobject]@{ Id = "bob"; Universal = $false; Dir = ".bob\skills" },
    [pscustomobject]@{ Id = "claude-code"; Universal = $false; Dir = ".claude\skills" },
    [pscustomobject]@{ Id = "openclaw"; Universal = $false; Dir = ".openclaw\skills" },
    [pscustomobject]@{ Id = "cline"; Universal = $true; Dir = ".agents\skills" },
    [pscustomobject]@{ Id = "codearts-agent"; Universal = $false; Dir = ".codeartsdoer\skills" },
    [pscustomobject]@{ Id = "codebuddy"; Universal = $false; Dir = ".codebuddy\skills" },
    [pscustomobject]@{ Id = "codemaker"; Universal = $false; Dir = ".codemaker\skills" },
    [pscustomobject]@{ Id = "codestudio"; Universal = $false; Dir = ".codestudio\skills" },
    [pscustomobject]@{ Id = "codex"; Universal = $true; Dir = ".codex\skills" },
    [pscustomobject]@{ Id = "command-code"; Universal = $false; Dir = ".commandcode\skills" },
    [pscustomobject]@{ Id = "continue"; Universal = $false; Dir = ".continue\skills" },
    [pscustomobject]@{ Id = "cortex"; Universal = $false; Dir = ".snowflake\cortex\skills" },
    [pscustomobject]@{ Id = "crush"; Universal = $false; Dir = ".config\crush\skills" },
    [pscustomobject]@{ Id = "cursor"; Universal = $true; Dir = ".cursor\skills" },
    [pscustomobject]@{ Id = "deepagents"; Universal = $true; Dir = ".deepagents\agent\skills" },
    [pscustomobject]@{ Id = "devin"; Universal = $false; Dir = ".config\devin\skills" },
    [pscustomobject]@{ Id = "dexto"; Universal = $true; Dir = ".agents\skills" },
    [pscustomobject]@{ Id = "droid"; Universal = $false; Dir = ".factory\skills" },
    [pscustomobject]@{ Id = "eve"; Universal = $false; Dir = $null },
    [pscustomobject]@{ Id = "firebender"; Universal = $true; Dir = ".firebender\skills" },
    [pscustomobject]@{ Id = "forgecode"; Universal = $false; Dir = ".forge\skills" },
    [pscustomobject]@{ Id = "gemini-cli"; Universal = $true; Dir = ".gemini\skills" },
    [pscustomobject]@{ Id = "github-copilot"; Universal = $true; Dir = ".copilot\skills" },
    [pscustomobject]@{ Id = "goose"; Universal = $false; Dir = ".config\goose\skills" },
    [pscustomobject]@{ Id = "grok"; Universal = $false; Dir = ".grok\skills" },
    [pscustomobject]@{ Id = "hermes-agent"; Universal = $false; Dir = ".hermes\skills" },
    [pscustomobject]@{ Id = "inference-sh"; Universal = $false; Dir = ".inferencesh\skills" },
    [pscustomobject]@{ Id = "jazz"; Universal = $false; Dir = ".jazz\skills" },
    [pscustomobject]@{ Id = "junie"; Universal = $false; Dir = ".junie\skills" },
    [pscustomobject]@{ Id = "iflow-cli"; Universal = $false; Dir = ".iflow\skills" },
    [pscustomobject]@{ Id = "kilo"; Universal = $false; Dir = ".kilocode\skills" },
    [pscustomobject]@{ Id = "kimchi"; Universal = $false; Dir = ".config\kimchi\harness\skills" },
    [pscustomobject]@{ Id = "kimi-code-cli"; Universal = $true; Dir = ".agents\skills" },
    [pscustomobject]@{ Id = "kiro-cli"; Universal = $false; Dir = ".kiro\skills" },
    [pscustomobject]@{ Id = "kode"; Universal = $false; Dir = ".kode\skills" },
    [pscustomobject]@{ Id = "lingma"; Universal = $false; Dir = ".lingma\skills" },
    [pscustomobject]@{ Id = "loaf"; Universal = $true; Dir = ".agents\skills" },
    [pscustomobject]@{ Id = "mcpjam"; Universal = $false; Dir = ".mcpjam\skills" },
    [pscustomobject]@{ Id = "minimax-code"; Universal = $false; Dir = ".minimax\skills" },
    [pscustomobject]@{ Id = "mistral-vibe"; Universal = $false; Dir = ".vibe\skills" },
    [pscustomobject]@{ Id = "moxby"; Universal = $false; Dir = ".moxby\skills" },
    [pscustomobject]@{ Id = "mux"; Universal = $false; Dir = ".mux\skills" },
    [pscustomobject]@{ Id = "opencode"; Universal = $true; Dir = ".config\opencode\skills" },
    [pscustomobject]@{ Id = "openhands"; Universal = $false; Dir = ".openhands\skills" },
    [pscustomobject]@{ Id = "ona"; Universal = $false; Dir = ".ona\skills" },
    [pscustomobject]@{ Id = "pi"; Universal = $false; Dir = ".pi\agent\skills" },
    [pscustomobject]@{ Id = "qoder"; Universal = $false; Dir = ".qoder\skills" },
    [pscustomobject]@{ Id = "qoder-cn"; Universal = $false; Dir = ".qoder-cn\skills" },
    [pscustomobject]@{ Id = "qwen-code"; Universal = $false; Dir = ".qwen\skills" },
    [pscustomobject]@{ Id = "replit"; Universal = $true; Dir = ".config\agents\skills" },
    [pscustomobject]@{ Id = "reasonix"; Universal = $false; Dir = ".reasonix\skills" },
    [pscustomobject]@{ Id = "rovodev"; Universal = $false; Dir = ".rovodev\skills" },
    [pscustomobject]@{ Id = "roo"; Universal = $false; Dir = ".roo\skills" },
    [pscustomobject]@{ Id = "tabnine-cli"; Universal = $false; Dir = ".tabnine\agent\skills" },
    [pscustomobject]@{ Id = "terramind"; Universal = $false; Dir = ".terramind\skills" },
    [pscustomobject]@{ Id = "tinycloud"; Universal = $false; Dir = ".tinycloud\skills" },
    [pscustomobject]@{ Id = "trae"; Universal = $false; Dir = ".trae\skills" },
    [pscustomobject]@{ Id = "trae-cn"; Universal = $false; Dir = ".trae-cn\skills" },
    [pscustomobject]@{ Id = "warp"; Universal = $true; Dir = ".agents\skills" },
    [pscustomobject]@{ Id = "windsurf"; Universal = $false; Dir = ".codeium\windsurf\skills" },
    [pscustomobject]@{ Id = "zed"; Universal = $true; Dir = ".agents\skills" },
    [pscustomobject]@{ Id = "zcode"; Universal = $false; Dir = ".zcode\skills" },
    [pscustomobject]@{ Id = "zencoder"; Universal = $false; Dir = ".zencoder\skills" },
    [pscustomobject]@{ Id = "zenflow"; Universal = $false; Dir = ".zencoder\skills" },
    [pscustomobject]@{ Id = "neovate"; Universal = $false; Dir = ".neovate\skills" },
    [pscustomobject]@{ Id = "pochi"; Universal = $false; Dir = ".pochi\skills" },
    [pscustomobject]@{ Id = "promptscript"; Universal = $true; Dir = $null },
    [pscustomobject]@{ Id = "adal"; Universal = $false; Dir = ".adal\skills" },
    [pscustomobject]@{ Id = "universal"; Universal = $true; Dir = ".config\agents\skills" }
)

# DWS compatibility targets that are not part of the upstream registry.
# Qoderwork remains a non-universal install target; the other entries are
# migration cleanup targets only and intentionally do not count as agents.
$LegacyAgentCleanupTargets = @(
    [pscustomobject]@{ Id = "dws-qoderwork"; Universal = $false; Dir = ".qoderwork\skills" },
    [pscustomobject]@{ Id = "dws-legacy-github"; Universal = $true; Dir = ".github\skills" },
    [pscustomobject]@{ Id = "dws-legacy-amp"; Universal = $true; Dir = ".amp\skills" },
    [pscustomobject]@{ Id = "dws-legacy-cline"; Universal = $true; Dir = ".cline\skills" },
    [pscustomobject]@{ Id = "dws-legacy-windsurf"; Universal = $true; Dir = ".windsurf\skills" }
)

# Kept as a compatibility surface for policy tests and downstream packagers.
$AgentDirs = @($AgentRegistry | Where-Object { $null -ne $_.Dir } | ForEach-Object { $_.Dir })

# ── Helpers ──────────────────────────────────────────────────────────────────

function Write-Say {
    param([string]$Message)
    Write-Host "  $Message"
}

function Write-Err {
    param([string]$Message)
    Write-Host "  ❌ $Message" -ForegroundColor Red
    exit 1
}

# A dingtalk-* prefix alone is not ownership evidence: market/user skills may
# use it too. Ownership comes from the centralized skills-state.json.
function Test-ManagedMultiSkillDir {
    param([string]$Dir)
    $name = Split-Path $Dir -Leaf
    if ($LegacyOfficialMultiSkills -contains $name) { return $true }
    $statePath = Join-Path $SkillStateRoot "skills-state.json"
    if (!(Test-Path $statePath -PathType Leaf)) { return $false }
    try {
        $state = Get-Content -Path $statePath -Raw | ConvertFrom-Json -ErrorAction Stop
        return @($state.managed_skills | Where-Object { $_.name -eq $name }).Count -gt 0
    } catch {
        return $false
    }
}

function Get-SkillDirectoryDigest {
    param([string]$Dir)
    $root = [System.IO.Path]::GetFullPath($Dir).TrimEnd([char[]]@('\', '/'))
    $files = @(
        Get-ChildItem -Path $root -Recurse -File -Force |
            ForEach-Object {
                [pscustomobject]@{
                    Relative = $_.FullName.Substring($root.Length).TrimStart([char[]]@('\', '/')).Replace('\', '/')
                    FullName = $_.FullName
                }
            } |
            Sort-Object -Property Relative
    )
    $stream = [System.IO.MemoryStream]::new()
    $sha = [System.Security.Cryptography.SHA256]::Create()
    try {
        foreach ($file in $files) {
            $pathBytes = [System.Text.Encoding]::UTF8.GetBytes($file.Relative)
            $stream.Write($pathBytes, 0, $pathBytes.Length)
            $stream.WriteByte(0)
            $content = [System.IO.File]::ReadAllBytes($file.FullName)
            $stream.Write($content, 0, $content.Length)
            $stream.WriteByte(0)
        }
        $hash = $sha.ComputeHash($stream.ToArray())
        return "sha256:" + ([System.BitConverter]::ToString($hash).Replace("-", "").ToLowerInvariant())
    } finally {
        $sha.Dispose()
        $stream.Dispose()
    }
}

function Write-SkillsState {
    param([string]$MultiSrc)
    $stateDir = $SkillStateRoot
    New-Item -ItemType Directory -Path $stateDir -Force | Out-Null
    $versionValue = if ([string]::IsNullOrWhiteSpace($Version)) { "unknown" } else { $Version }
    $skills = @(Get-ChildItem -Path $MultiSrc -Directory | Where-Object {
        Test-Path (Join-Path $_.FullName "SKILL.md")
    } | Sort-Object -Property Name)
    $names = @($skills | ForEach-Object { $_.Name })
    $managed = @($skills | ForEach-Object {
        [ordered]@{
            name = $_.Name
            version = $versionValue
            source = "install.ps1"
            digest = Get-SkillDirectoryDigest -Dir $_.FullName
            digest_scope = $ManagedSkillDigestScope
        }
    })
    $state = [ordered]@{
        version = $versionValue
        official_skills = $names
        updated_skills = $names
        managed_skills = $managed
        updated_at = [DateTime]::UtcNow.ToString("yyyy-MM-ddTHH:mm:ssZ")
    }
    $statePath = Join-Path $stateDir "skills-state.json"
    $tempPath = Join-Path $stateDir (".skills-state-" + [guid]::NewGuid().ToString("N") + ".tmp")
    $backupPath = Join-Path $stateDir (".skills-state-" + [guid]::NewGuid().ToString("N") + ".previous")
    try {
        [System.IO.File]::WriteAllText($tempPath, (($state | ConvertTo-Json -Depth 5) + "`n"), [System.Text.UTF8Encoding]::new($false))
        if (Test-Path $statePath -PathType Leaf) {
            [System.IO.File]::Replace($tempPath, $statePath, $backupPath, $true)
            Remove-Item -LiteralPath $backupPath -Force -ErrorAction SilentlyContinue
        } else {
            Move-Item -LiteralPath $tempPath -Destination $statePath
        }
    } finally {
        if (Test-Path $tempPath) { Remove-Item -LiteralPath $tempPath -Force -ErrorAction SilentlyContinue }
    }
}

# Central move seam for transactional Skill publication. Tests replace this
# function to inject backup/publish failures without relying on ACL behavior.
function Move-SkillPath {
    param(
        [string]$Source,
        [string]$Destination
    )
    if (Test-SkillPathLexically -Path $Destination) {
        throw "Skill move destination already exists: $Destination"
    }
    $sourceItem = Get-Item -LiteralPath $Source -Force -ErrorAction Stop
    if ($sourceItem.PSIsContainer) {
        # Move-Item treats an existing directory as a container and nests the
        # source below it. Directory.Move has the exact rename semantics this
        # transaction requires: an occupied destination fails without touching
        # either path.
        [System.IO.Directory]::Move($Source, $Destination)
    } else {
        # File.Move replaces an occupied dest on Windows. Copy without
        # overwrite refuses that dest, then the source is removed only after
        # the exclusive copy succeeds.
        [System.IO.File]::Copy($Source, $Destination, $false)
        try {
            Remove-SkillPathLexically -Path $Source
        } catch {
            $removeErr = $_
            try {
                Remove-SkillPathLexically -Path $Destination
            } catch {
                throw "Skill move state uncertain; source $Source and dest $Destination retained: $removeErr; retract failed: $_"
            }
            throw $removeErr
        }
    }
}

function Test-CrossDeviceMoveError {
    param([System.Management.Automation.ErrorRecord]$Record)
    $exception = $Record.Exception
    while ($null -ne $exception) {
        # Win32 ERROR_NOT_SAME_DEVICE is 17 (0x11). Move-Item surfaces it as
        # the low word of an IOException HRESULT.
        if (($exception.HResult -band 0xffff) -eq 17) { return $true }
        $exception = $exception.InnerException
    }
    return $false
}

function Copy-SkillPathMetadata {
    param($SourceItem, [string]$Destination)
    (Get-Item -LiteralPath $Destination -Force -ErrorAction Stop).Attributes = $SourceItem.Attributes
    $nativeWindows = $env:OS -eq "Windows_NT" -or $PSVersionTable.PSEdition -eq "Desktop"
    if ($nativeWindows) {
        Set-Acl -LiteralPath $Destination -AclObject (Get-Acl -LiteralPath $SourceItem.FullName -ErrorAction Stop) -ErrorAction Stop
    } else {
        $mode = [System.IO.File]::GetUnixFileMode($SourceItem.FullName)
        [System.IO.File]::SetUnixFileMode($Destination, $mode)
    }
}

function Get-SkillPathPermissionFingerprint {
    param([string]$Path)
    $nativeWindows = $env:OS -eq "Windows_NT" -or $PSVersionTable.PSEdition -eq "Desktop"
    if ($nativeWindows) {
        return (Get-Acl -LiteralPath $Path -ErrorAction Stop).Sddl
    }
    return [string][System.IO.File]::GetUnixFileMode($Path)
}

function Copy-SkillPathLexically {
    param([string]$Source, [string]$Destination)
    $item = Get-Item -LiteralPath $Source -Force -ErrorAction Stop
    if ($item.LinkType) {
        $itemType = if ($item.LinkType -eq "Junction") { "Junction" } else { "SymbolicLink" }
        New-Item -ItemType $itemType -Path $Destination -Target $item.Target -ErrorAction Stop | Out-Null
        return
    }
    if ($item.PSIsContainer) {
        New-Item -ItemType Directory -Path $Destination -ErrorAction Stop | Out-Null
        foreach ($child in @(Get-ChildItem -LiteralPath $Source -Force -ErrorAction Stop)) {
            Copy-SkillPathLexically -Source $child.FullName -Destination (Join-Path $Destination $child.Name)
        }
        Copy-SkillPathMetadata -SourceItem $item -Destination $Destination
        return
    }
    if ($item -isnot [System.IO.FileInfo]) {
        throw "不支持复制特殊 Skill 路径 $Source"
    }
    [System.IO.File]::Copy($Source, $Destination, $false)
    Copy-SkillPathMetadata -SourceItem $item -Destination $Destination
}

function Assert-SkillPathCopy {
    param([string]$Source, [string]$Destination)
    $sourceItem = Get-Item -LiteralPath $Source -Force -ErrorAction Stop
    $destinationItem = Get-Item -LiteralPath $Destination -Force -ErrorAction Stop
    if ([bool]$sourceItem.LinkType -ne [bool]$destinationItem.LinkType -or
        $sourceItem.PSIsContainer -ne $destinationItem.PSIsContainer) {
        throw "Skill 路径类型不一致: $Source != $Destination"
    }
    if ($sourceItem.LinkType) {
        if ($sourceItem.LinkType -ne $destinationItem.LinkType -or
            ($sourceItem.Target -join "`0") -ne ($destinationItem.Target -join "`0")) {
            throw "Skill 链接目标不一致: $Source != $Destination"
        }
        return
    }
    $nativeWindows = $env:OS -eq "Windows_NT" -or $PSVersionTable.PSEdition -eq "Desktop"
    if (-not $nativeWindows) {
        if ((Get-SkillPathPermissionFingerprint -Path $Source) -ne
            (Get-SkillPathPermissionFingerprint -Path $Destination)) {
            throw "Skill 路径权限不一致: $Source != $Destination"
        }
    }
    if ($sourceItem.PSIsContainer) {
        $sourceChildren = @(Get-ChildItem -LiteralPath $Source -Force -ErrorAction Stop | Sort-Object -Property Name)
        $destinationChildren = @(Get-ChildItem -LiteralPath $Destination -Force -ErrorAction Stop | Sort-Object -Property Name)
        if ($sourceChildren.Count -ne $destinationChildren.Count) {
            throw "Skill 目录项数量不一致: $Source != $Destination"
        }
        for ($i = 0; $i -lt $sourceChildren.Count; $i++) {
            if ($sourceChildren[$i].Name -ne $destinationChildren[$i].Name) {
                throw "Skill 目录项不一致: $Source != $Destination"
            }
            Assert-SkillPathCopy -Source $sourceChildren[$i].FullName -Destination $destinationChildren[$i].FullName
        }
        return
    }
    if ($sourceItem.Length -ne $destinationItem.Length) {
        throw "Skill 文件大小不一致: $Source != $Destination"
    }
    $sourceHash = (Get-FileHash -LiteralPath $Source -Algorithm SHA256 -ErrorAction Stop).Hash
    $destinationHash = (Get-FileHash -LiteralPath $Destination -Algorithm SHA256 -ErrorAction Stop).Hash
    if ($sourceHash -ne $destinationHash) {
        throw "Skill 文件内容摘要不一致: $Source != $Destination"
    }
}

function Remove-SkillPathLexically {
    param([string]$Path)
    $item = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
    if ($item.PSIsContainer -and !$item.LinkType) {
        Remove-Item -LiteralPath $Path -Recurse -Force -ErrorAction Stop
    } else {
        Remove-Item -LiteralPath $Path -Force -ErrorAction Stop
    }
}

# Removes a staging directory that may hold unpublished junction/symlink
# children. Every link child must be removed non-recursively first: Windows
# PowerShell 5.1 (the advertised irm|iex surface) can follow reparse points
# during Remove-Item -Recurse and would delete the canonical store's contents
# through the staged link. CI runs pwsh 7, which masks this behavior.
function Remove-LinkStageRoot {
    param([string]$StageRoot)
    if (!(Test-Path -LiteralPath $StageRoot)) { return $true }
    $ok = $true
    foreach ($child in @(Get-ChildItem -LiteralPath $StageRoot -Force -ErrorAction SilentlyContinue)) {
        try { Remove-SkillPathLexically -Path $child.FullName } catch { $ok = $false }
    }
    try {
        Remove-Item -LiteralPath $StageRoot -Force -ErrorAction Stop
    } catch {
        $ok = $false
    }
    return $ok
}

# Resolves a path to its physical location, dereferencing junction/symlink
# reparse points at any depth. Resolve-Path alone is lexical and never
# recognizes DWS's own junctions, so every rerun would back them up and
# recreate them. Mirrors EvalSymlinks (Go), realpathSync (npm), and cd -P
# (shell) on the other installer surfaces.
function Get-PhysicalSkillPath {
    param([string]$Path, [int]$Depth = 0)
    if ($Depth -gt 40) { return $null }
    $parent = Split-Path $Path -Parent
    if ([string]::IsNullOrWhiteSpace($parent)) { return $Path }
    $parentPhysical = Get-PhysicalSkillPath -Path $parent -Depth ($Depth + 1)
    if ($null -eq $parentPhysical) { return $null }
    $leaf = Split-Path $Path -Leaf
    if ([string]::IsNullOrEmpty($leaf)) { return $parentPhysical }
    $candidate = Join-Path $parentPhysical $leaf
    try { $item = Get-Item -LiteralPath $candidate -Force -ErrorAction Stop } catch { return $null }
    $target = @($item.Target) | Where-Object { $_ } | Select-Object -First 1
    if ($item.LinkType -and $target) {
        $targetPath = ""
        if ([System.IO.Path]::IsPathRooted([string]$target)) {
            $targetPath = [string]$target
        } else {
            $targetPath = Join-Path $parentPhysical ([string]$target)
        }
        # Resolve the target recursively as well. A custom HOME or canonical
        # root can itself sit below another junction, which must compare equal
        # to the fully physical path just like EvalSymlinks/realpath do.
        return Get-PhysicalSkillPath -Path ([System.IO.Path]::GetFullPath($targetPath)) -Depth ($Depth + 1)
    }
    return $candidate
}

# Same-volume moves remain atomic. For a cross-volume backup/restore, stage on
# the destination filesystem, copy links lexically, verify, publish, then
# remove the source. Before source removal every failure keeps the original;
# a removal failure deliberately leaves both verified copies and fails loud.
function Move-SkillPathRecoverably {
    param([string]$Source, [string]$Destination)
    if (Test-SkillPathLexically -Path $Destination) { throw "移动目标已存在: $Destination" }
    $destinationParent = Split-Path $Destination -Parent
    New-Item -ItemType Directory -Path $destinationParent -Force -ErrorAction Stop | Out-Null
    try {
        Move-SkillPath -Source $Source -Destination $Destination
        return
    } catch {
        if (!(Test-CrossDeviceMoveError -Record $_)) { throw }
    }

    $stageRoot = Join-Path $destinationParent ("." + (Split-Path $Destination -Leaf) + ".cross-device-" + [guid]::NewGuid().ToString("N"))
    $stage = Join-Path $stageRoot "payload"
    New-Item -ItemType Directory -Path $stageRoot -ErrorAction Stop | Out-Null
    $published = $false
    try {
        Copy-SkillPathLexically -Source $Source -Destination $stage
        Assert-SkillPathCopy -Source $Source -Destination $stage
        Move-SkillPath -Source $stage -Destination $Destination
        $published = $true
        $publication = [pscustomobject]@{ Path = $Destination; Source = $Source }
        try {
            Assert-SkillPathCopy -Source $Source -Destination $Destination
            if (!(Remove-LinkStageRoot -StageRoot $stageRoot)) {
                throw "Skill staging 清理失败: $stageRoot"
            }
        } catch {
            $postErr = $_
            try {
                Remove-PublishedSkillPathSafely -Record $publication
            } catch {
                throw "Skill 移动状态不确定：$postErr；撤回目标 $Destination 失败: $_；源 $Source 与目标 $Destination 均保留"
            }
            throw "Skill 移动失败，目标已撤回，原路径保留 ${Source}: $postErr"
        }
        try {
            Remove-SkillPathLexically -Path $Source
        } catch {
            throw "Skill 目标已发布但源路径删除失败（源 $Source 与目标 $Destination 均保留）: $_"
        }
        if (Test-SkillPathLexically -Path $Source) {
            throw "Skill 目标已发布但源路径仍存在（源 $Source 与目标 $Destination 均保留）"
        }
    } catch {
        $failure = $_
        if (Test-Path -LiteralPath $stageRoot) {
            if (!(Remove-LinkStageRoot -StageRoot $stageRoot)) {
                if (-not $published) {
                    throw "$failure；跨设备 Skill staging 清理失败 $stageRoot（备份与原路径均保留）"
                }
            }
        }
        throw $failure
    }
}

function Get-Arch {
    # Allow manual override via environment variable
    if ($env:DWS_ARCH) {
        $override = $env:DWS_ARCH.ToLower()
        if ($override -eq "amd64" -or $override -eq "arm64") {
            return $override
        }
        Write-Err "Invalid DWS_ARCH value '$env:DWS_ARCH'. Must be 'amd64' or 'arm64'."
    }

    # Method 1: Try RuntimeInformation (available in .NET Core / PowerShell 6+)
    try {
        $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
        if ($arch) {
            switch ($arch.ToString()) {
                "X64"   { return "amd64" }
                "Arm64" { return "arm64" }
            }
        }
    } catch {}

    # Method 2: Check PROCESSOR_ARCHITECTURE environment variable (Windows)
    $envArch = $env:PROCESSOR_ARCHITECTURE
    if ($envArch) {
        switch ($envArch.ToUpper()) {
            "AMD64" { return "amd64" }
            "ARM64" { return "arm64" }
            "X86"   {
                # 32-bit process on 64-bit OS?
                $realArch = $env:PROCESSOR_ARCHITEW6432
                if ($realArch) {
                    switch ($realArch.ToUpper()) {
                        "AMD64" { return "amd64" }
                        "ARM64" { return "arm64" }
                    }
                }
                Write-Err "32-bit Windows is not supported"
            }
        }
    }

    # Method 3: Try WMI query as last resort
    try {
        $cpu = Get-WmiObject -Class Win32_Processor -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($cpu) {
            switch ($cpu.Architecture) {
                9 { return "amd64" }  # x64
                12 { return "arm64" } # ARM64
            }
        }
    } catch {}

    Write-Err "Unsupported architecture: Could not detect system architecture. Please set DWS_ARCH environment variable to 'amd64' or 'arm64'."
}

function Invoke-GiteeApi {
    param([string]$Uri)
    # Gitee's gateway returns sporadic 502/503, so retry a few times before failing.
    for ($i = 1; $i -le 4; $i++) {
        try {
            return Invoke-RestMethod -Uri $Uri -UseBasicParsing
        } catch {
            if ($i -eq 4) { throw }
            Start-Sleep -Seconds 2
        }
    }
}

function Get-GiteeAssetUrl {
    param([string]$Name)
    # Resolve a release asset's download URL by name via the Gitee API
    # (Gitee attachment URLs carry an unstable numeric id, so never template them).
    $rel = Invoke-GiteeApi "https://gitee.com/api/v5/repos/$GiteeRepo/releases/tags/$Version"
    foreach ($a in $rel.assets) {
        if ($a.name -eq $Name) { return $a.browser_download_url }
    }
    return ""
}

function Assert-ReleaseAssetChecksum {
    param([string]$AssetPath, [string]$AssetName, [string]$TempDir)
    if ($GiteeRepo -ne "") { $checksumUrl = Get-GiteeAssetUrl "checksums.txt" } else { $checksumUrl = "https://github.com/$Repo/releases/download/$Version/checksums.txt" }
    if (-not $checksumUrl) { Write-Err "Could not resolve checksums.txt for $Version." }
    $checksumPath = Join-Path $TempDir "checksums.txt"
    try {
        Invoke-WebRequest -Uri $checksumUrl -OutFile $checksumPath -UseBasicParsing -ErrorAction Stop
    } catch {
        Write-Err "Could not download checksums.txt for $Version; refusing unverified $AssetName."
    }
    $expectedLine = Get-Content -LiteralPath $checksumPath | Where-Object {
        $_ -match "^[0-9A-Fa-f]{64}[ ]+[*]?$([regex]::Escape($AssetName))$"
    } | Select-Object -First 1
    if (-not $expectedLine) { Write-Err "$AssetName is missing from checksums.txt." }
    $expected = ($expectedLine -split '\s+')[0].ToLowerInvariant()
    $actual = (Get-FileHash -LiteralPath $AssetPath -Algorithm SHA256 -ErrorAction Stop).Hash.ToLowerInvariant()
    if ($actual -ne $expected) { Write-Err "SHA256 checksum mismatch for $AssetName. Expected $expected, got $actual." }
    Write-Say "✅ SHA256 checksum verified: $AssetName"
}

function Resolve-Source {
    # Explicit DWS_GITEE_REPO wins; else probe GitHub and fall back to Gitee when unreachable.
    if ($GiteeRepo -ne "") { return }
    if ($env:DWS_NO_FALLBACK -eq "1") { return }
    try {
        Invoke-WebRequest -Uri "https://github.com/$Repo/releases/latest" -Method Head `
            -TimeoutSec 12 -UseBasicParsing -ErrorAction Stop 2>$null | Out-Null
        return
    } catch {
        $script:GiteeRepo = $GiteeFallbackRepo
        Write-Say "⚠ GitHub 不可达，自动切换国内 Gitee 镜像: $script:GiteeRepo"
    }
}

function Resolve-LatestVersion {
    if ($Version -eq "latest") {
        if ($GiteeRepo -ne "") {
            try {
                # Gitee's /releases/latest and /releases endpoints are unreliable
                # (404 / empty even when releases exist), so resolve the newest
                # vN.N.N tag from the git tags endpoint instead.
                $tags = Invoke-GiteeApi "https://gitee.com/api/v5/repos/$GiteeRepo/tags"
                $latest = $tags.name |
                    Where-Object { $_ -match '^v\d+\.\d+\.\d+$' } |
                    ForEach-Object { [version]($_.TrimStart('v')) } |
                    Sort-Object | Select-Object -Last 1
                if ($latest) { $script:Version = "v$latest"; return }
            } catch {}
            Write-Err "Could not determine the latest Gitee version. Set `$env:DWS_VERSION explicitly."
            return
        }
        try {
            $response = Invoke-WebRequest -Uri $LatestUrl `
                -MaximumRedirection 0 -ErrorAction SilentlyContinue -UseBasicParsing 2>$null
        } catch {
            if ($_.Exception.Response.Headers.Location) {
                $location = $_.Exception.Response.Headers.Location.ToString()
                $script:Version = ($location -split "/tag/")[-1].Trim()
                return
            }
        }

        # Fallback: parse the redirect from the response
        try {
            $response = Invoke-WebRequest -Uri $LatestUrl `
                -UseBasicParsing -ErrorAction Stop
            if ($response.BaseResponse.ResponseUri) {
                $script:Version = ($response.BaseResponse.ResponseUri.ToString() -split "/tag/")[-1].Trim()
                return
            }
            if ($response.BaseResponse.RequestMessage.RequestUri) {
                $script:Version = ($response.BaseResponse.RequestMessage.RequestUri.ToString() -split "/tag/")[-1].Trim()
                return
            }
        } catch {}

        Write-Err "Could not determine the latest version. Set `$env:DWS_VERSION explicitly."
    }
}

function Copy-DirRecursive {
    param([string]$Source, [string]$Destination)
    if (!(Test-Path $Destination)) {
        New-Item -ItemType Directory -Path $Destination -Force | Out-Null
    }
    $count = 0
    Get-ChildItem -Path $Source -Force | ForEach-Object {
        $destPath = Join-Path $Destination $_.Name
        if ($_.PSIsContainer) {
            $count += Copy-DirRecursive -Source $_.FullName -Destination $destPath
        } else {
            Copy-Item -Path $_.FullName -Destination $destPath -Force
            $count++
        }
    }
    return $count
}

function Publish-SkillCache {
    param([string]$Source, [string]$CacheDir)

    $cacheParent = Split-Path $CacheDir -Parent
    $cacheName = Split-Path $CacheDir -Leaf
    New-Item -ItemType Directory -Path $cacheParent -Force -ErrorAction Stop | Out-Null
    $stagedDir = Join-Path $cacheParent ".$cacheName.tmp-$([Guid]::NewGuid().ToString('N'))"
    $rollbackDir = ""
    $published = $false
    New-Item -ItemType Directory -Path $stagedDir -Force -ErrorAction Stop | Out-Null

    try {
        $count = Copy-DirRecursive -Source $Source -Destination $stagedDir
        if (Test-Path $CacheDir) {
            $rollbackDir = Join-Path $cacheParent ".$cacheName.old-$([Guid]::NewGuid().ToString('N'))"
            Move-Item -Path $CacheDir -Destination $rollbackDir -ErrorAction Stop
        }
        try {
            Move-Item -Path $stagedDir -Destination $CacheDir -ErrorAction Stop
            $published = $true
        } catch {
            $publishError = $_
            if ($rollbackDir) {
                try {
                    Move-Item -Path $rollbackDir -Destination $CacheDir -ErrorAction Stop
                    $rollbackDir = ""
                } catch {
                    throw "Skill 缓存发布失败: $publishError；原缓存恢复也失败，恢复目录: $rollbackDir；错误: $_"
                }
            }
            throw $publishError
        }
        if ($rollbackDir -and (Test-Path $rollbackDir)) {
            Remove-Item -Path $rollbackDir -Recurse -Force -ErrorAction SilentlyContinue
            if (Test-Path $rollbackDir) {
                Write-Say "⚠️ 新缓存已生效，但旧缓存清理失败: $rollbackDir"
            }
            $rollbackDir = ""
        }
        return $count
    } finally {
        if (!$published -and (Test-Path $stagedDir)) {
            Remove-Item -Path $stagedDir -Recurse -Force -ErrorAction SilentlyContinue
        }
    }
}

# Get-SkillBackupName encodes the HOME-relative path of a backed-up Skill
# directory ('.codex\skills\dingtalk-chat' → '.codex-skills-dingtalk-chat') so
# copies retired from different Agent roots stay distinguishable inside one
# stamp. Mirrors build/npm/install.js and internal/upgrade/paths.go. Paths
# outside HOME fall back to the bare leaf.
function Get-SkillBackupName {
    param([string]$Dir)
    $leaf = Split-Path $Dir -Leaf
    try {
        $full = [System.IO.Path]::GetFullPath($Dir)
        $root = [System.IO.Path]::GetFullPath($HOME).TrimEnd([char[]]@('\', '/'))
    } catch {
        return $leaf
    }
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

# SkillBackupKeep bounds $HOME\.dws\skill-backups growth: only the newest
# stamped backup directories are kept, matching skillBackupKeep and
# pruneSkillBackups in internal/upgrade/paths.go.
$SkillBackupKeep = 5
$script:SkillBackupRootsThisRun = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)

# Ownership marker: every stamp root DWS creates carries .dws-skill-backup
# with exactly "dws skill backup v1" + LF — the same bytes internal/upgrade/
# paths.go, install.sh, and build/npm/install.js write. A stamp-shaped name
# alone is not ownership proof, so pruning only deletes directories whose
# marker content verifies.
$SkillBackupMarkerFile = ".dws-skill-backup"
$SkillBackupMarkerBody = "dws skill backup v1"

# Write-SkillBackupMarker stamps a freshly created stamp root as DWS-owned.
# [IO.File]::WriteAllText pins the exact LF-terminated bytes (Set-Content
# would append a platform newline on Windows PowerShell 5.1).
function Write-SkillBackupMarker {
    param([string]$Root)
    [System.IO.File]::WriteAllText(
        (Join-Path $Root $SkillBackupMarkerFile),
        "$SkillBackupMarkerBody`n",
        [System.Text.UTF8Encoding]::new($false))
}

# Test-SkillBackupMarker reports whether a stamp root carries the ownership
# marker. The check normalizes CRLF→LF and drops trailing newlines before
# comparing, so this surface accepts every surface's exact-LF bytes (writer
# and checker agree); any other content, or a missing/unreadable marker,
# means foreign data.
function Test-SkillBackupMarker {
    param([string]$Dir)
    try {
        $marker = Join-Path $Dir $SkillBackupMarkerFile
        if (![System.IO.File]::Exists($marker)) { return $false }
        $body = [System.IO.File]::ReadAllText($marker).Replace("`r`n", "`n").TrimEnd("`r", "`n")
        return ($body -eq $SkillBackupMarkerBody)
    } catch {
        return $false
    }
}

# Removes a whole stamp root child-first without ever following a reparse
# point: link children are deleted non-recursively, real directories are
# recursed the same way, and only an emptied directory is removed. Backup
# trees can contain junctions/symlinks (victims are collected before the
# physical-equality filter), and Windows PowerShell 5.1 can follow reparse
# points during Remove-Item -Recurse — the invariant Remove-LinkStageRoot
# enforces for staging roots, applied at every depth here.
function Remove-SkillBackupTreeLexically {
    param([string]$Path)
    $item = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
    if ($item.PSIsContainer -and !$item.LinkType) {
        foreach ($child in @(Get-ChildItem -LiteralPath $Path -Force -ErrorAction Stop)) {
            Remove-SkillBackupTreeLexically -Path $child.FullName
        }
    }
    Remove-Item -LiteralPath $Path -Force -ErrorAction Stop
}

# Remove-OldSkillBackups deletes the oldest stamped backup directories once
# more than $SkillBackupKeep remain. Stamps sort lexicographically in
# chronological order, so name order is age order. Roots created during this
# run are never pruned: an in-flight transaction still needs them to roll
# back. Best-effort — a prune failure never fails the install.
function Remove-OldSkillBackups {
    $root = Join-Path $HOME ".dws\skill-backups"
    if (!(Test-Path -LiteralPath $root -PathType Container)) { return }
    # Only directories whose names match the DWS backup stamp format (UTC
    # yyyyMMdd-HHmmss, optional -N collision suffix) AND whose ownership
    # marker verifies are candidates; anything else is foreign data —
    # preserved and never counted against $SkillBackupKeep.
    $dirs = @(Get-ChildItem -LiteralPath $root -Directory -Force -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -match '^[0-9]{8}-[0-9]{6}(-[0-9]+)?$' -and (Test-SkillBackupMarker -Dir $_.FullName) } |
        Sort-Object -Property Name)
    $excess = $dirs.Count - $SkillBackupKeep
    foreach ($dir in $dirs) {
        if ($excess -le 0) { break }
        if ($script:SkillBackupRootsThisRun.Contains($dir.FullName)) { continue }
        try { Remove-SkillBackupTreeLexically -Path $dir.FullName } catch { }
        $excess--
    }
}

# Backup-SkillDir moves $Dir into $HOME\.dws\skill-backups\<stamp>\<name>
# instead of destroying it (non-interactive installs cannot confirm, so
# removals must stay reversible). Missing paths are a no-op success. On any
# backup failure the directory is left in place and $false is returned so
# callers skip that target rather than silently deleting data.
function Backup-SkillDir {
    param(
        [string]$Dir,
        [ref]$BackupPath
    )
    if ($null -ne $BackupPath) { $BackupPath.Value = "" }
    if (!(Test-SkillPathLexically -Path $Dir)) { return $true }
    $backupRoot = Join-Path $HOME ".dws\skill-backups"
    $stamp = [DateTime]::UtcNow.ToString("yyyyMMdd-HHmmss")
    $name = Get-SkillBackupName -Dir $Dir
    $targetRoot = Join-Path $backupRoot $stamp
    $target = Join-Path $targetRoot $name
    $i = 1
    # Bump not only when <stamp>\<name> is taken but also when the stamp root
    # itself exists without a verified ownership marker: a same-second
    # foreign directory must never be stamped DWS-owned and pruned later. A
    # marker-verified root from this run's same second stays reusable.
    while ((Test-SkillPathLexically -Path $target) -or
        ((Test-SkillPathLexically -Path $targetRoot) -and !(Test-SkillBackupMarker -Dir $targetRoot))) {
        $targetRoot = Join-Path $backupRoot "$stamp-$i"
        $target = Join-Path $targetRoot $name
        $i++
        if ($i -gt 1000) {
            Write-Say "⚠️  备份目录冲突，保留原目录 $Dir"
            return $false
        }
    }
    try {
        New-Item -ItemType Directory -Path (Split-Path $target -Parent) -Force -ErrorAction Stop | Out-Null
        # Stamp ownership immediately after creating the stamp root and
        # before any skill directory moves into it, so an interrupted backup
        # can never leave an unmarked (never-prunable) stamp behind.
        Write-SkillBackupMarker -Root $targetRoot
    } catch {
        # The removal stays non-recursive so a pre-existing non-empty root
        # (foreign data) is never destroyed; a failed marker write must not
        # leave an empty unowned stamp root behind either.
        Remove-Item -LiteralPath $targetRoot -Force -ErrorAction SilentlyContinue
        Write-Say "⚠️  备份失败，保留原目录 $Dir`: $_"
        return $false
    }
    try {
        Move-SkillPathRecoverably -Source $Dir -Destination $target
    } catch {
        Write-Say "⚠️  备份失败，保留原目录 $Dir`: $_"
        return $false
    }
    if ($null -ne $BackupPath) { $BackupPath.Value = $target }
    try {
        $script:SkillBackupRootsThisRun.Add([System.IO.Path]::GetFullPath($targetRoot)) | Out-Null
        Remove-OldSkillBackups
    } catch {
        Write-Say "⚠️  旧备份清理失败（备份本身已成功）: $_"
    }
    Write-Say "  × 已备份并移除 $Dir → $target"
    return $true
}

function Get-SkillLinkSignature {
    param([string]$Path)
    try {
        $item = Get-Item -LiteralPath $Path -Force -ErrorAction Stop
    } catch {
        return $null
    }
    if (!$item.LinkType) { return $null }
    $targets = @($item.Target) | ForEach-Object { [string]$_ }
    return ([string]$item.LinkType + "`0" + ($targets -join "`0"))
}

function New-PublishedSkillLinkRecord {
    param([string]$Path)
    $signature = Get-SkillLinkSignature -Path $Path
    if ([string]::IsNullOrEmpty($signature)) {
        throw "published Skill path is not a link: $Path"
    }
    return [pscustomobject]@{ Path = $Path; LinkSignature = $signature }
}

function Remove-PublishedSkillPathSafely {
    param($Record)
    $path = [string]$Record.Path
    $parent = Split-Path $path -Parent
    $quarantine = Join-Path $parent (".dws-link-rollback-" + [guid]::NewGuid().ToString("N"))

    # Claim the current directory entry with an exact same-filesystem rename.
    # If another process replaced our link, verification below fails and that
    # object is moved back instead of ever being recursively removed.
    Move-SkillPath -Source $path -Destination $quarantine
    try {
        if ($Record.LinkSignature) {
            if ((Get-SkillLinkSignature -Path $quarantine) -ne [string]$Record.LinkSignature) {
                throw "发布链接已被其他进程替换，拒绝删除"
            }
        } elseif ($Record.Source) {
            Assert-SkillPathCopy -Source ([string]$Record.Source) -Destination $quarantine
        } else {
            throw "发布路径缺少事务身份，拒绝删除"
        }
        Remove-SkillPathLexically -Path $quarantine
    } catch {
        $failure = $_
        if (Test-SkillPathLexically -Path $quarantine) {
            try {
                if (Test-SkillPathLexically -Path $path) { throw "原路径已被占用" }
                Move-SkillPath -Source $quarantine -Destination $path
            } catch {
                throw "$failure；并发对象保留于 $quarantine`: $_"
            }
        }
        throw $failure
    }
}

function Restore-MultiSkillSet {
    param(
        [array]$Published,
        [array]$Backups
    )
    $ok = $true
    for ($i = $Published.Count - 1; $i -ge 0; $i--) {
        $publishedItem = $Published[$i]
        $publishedPath = if ($publishedItem -is [string]) { $publishedItem } else { [string]$publishedItem.Path }
        try {
            if (Test-SkillPathLexically -Path $publishedPath) {
                if (!($publishedItem -is [string])) {
                    Remove-PublishedSkillPathSafely -Record $publishedItem
                } else {
                    Remove-SkillPathLexically -Path $publishedPath
                }
            }
        } catch {
            Write-Say "⚠️  无法移除失败发布目录 $publishedPath`: $_"
            $ok = $false
        }
    }
    for ($i = $Backups.Count - 1; $i -ge 0; $i--) {
        $item = $Backups[$i]
        try {
            if (Test-SkillPathLexically -Path $item.Original) {
                throw "恢复目标仍存在"
            }
            New-Item -ItemType Directory -Path (Split-Path $item.Original -Parent) -Force -ErrorAction Stop | Out-Null
            Move-SkillPathRecoverably -Source $item.Backup -Destination $item.Original
        } catch {
            Write-Say "⚠️  无法恢复原 Skill $($item.Original)；备份保留于 $($item.Backup): $_"
            $ok = $false
        }
    }
    return $ok
}

function Test-SkillPathLexically {
    param([string]$Path)
    if ([string]::IsNullOrWhiteSpace($Path)) { return $false }
    try {
        Get-Item -LiteralPath $Path -Force -ErrorAction Stop | Out-Null
        return $true
    } catch {
        $parent = Split-Path $Path -Parent
        $leaf = Split-Path $Path -Leaf
        if ([string]::IsNullOrWhiteSpace($parent) -or !(Test-Path $parent -PathType Container)) { return $false }
        return $null -ne (Get-ChildItem -LiteralPath $parent -Force -ErrorAction SilentlyContinue |
            Where-Object { $_.Name -eq $leaf } | Select-Object -First 1)
    }
}

function Resolve-AgentSkillBase {
    param([string]$Root, $Agent)
    if ($null -eq $Agent.Dir) { return $null }
    switch ($Agent.Id) {
        "autohand-code" { if ($env:AUTOHAND_HOME) { return (Join-Path $env:AUTOHAND_HOME "skills") } }
        "claude-code" { if ($env:CLAUDE_CONFIG_DIR) { return (Join-Path $env:CLAUDE_CONFIG_DIR "skills") } }
        "codex" { if ($env:CODEX_HOME) { return (Join-Path $env:CODEX_HOME "skills") } }
        "grok" { if ($env:GROK_HOME) { return (Join-Path $env:GROK_HOME "skills") } }
        "hermes-agent" { if ($env:HERMES_HOME) { return (Join-Path $env:HERMES_HOME "skills") } }
        "mistral-vibe" { if ($env:VIBE_HOME) { return (Join-Path $env:VIBE_HOME "skills") } }
        "openclaw" {
            foreach ($name in @(".openclaw", ".clawdbot", ".moltbot")) {
                $candidate = Join-Path $Root $name
                if (Test-Path $candidate -PathType Container) { return (Join-Path $candidate "skills") }
            }
        }
        { $_ -in @("amp", "replit", "universal") } {
            $configHome = if ($env:XDG_CONFIG_HOME) { $env:XDG_CONFIG_HOME } else { Join-Path $Root ".config" }
            return (Join-Path $configHome "agents\skills")
        }
        { $_ -in @("crush", "devin", "goose", "opencode") } {
            $configHome = if ($env:XDG_CONFIG_HOME) { $env:XDG_CONFIG_HOME } else { Join-Path $Root ".config" }
            $child = switch ($Agent.Id) { "crush" { "crush\skills" }; "devin" { "devin\skills" }; "goose" { "goose\skills" }; default { "opencode\skills" } }
            return (Join-Path $configHome $child)
        }
        "kimchi" {
            $configHome = if ($env:XDG_CONFIG_HOME) { $env:XDG_CONFIG_HOME } else { Join-Path $Root ".config" }
            return (Join-Path $configHome "kimchi\harness\skills")
        }
    }
    return (Join-Path $Root $Agent.Dir)
}

function Test-AgentSkillBaseDetected {
    param([string]$BaseDir, $Agent)
    $parent = Split-Path $BaseDir -Parent
    switch ($Agent.Id) {
        "kimchi" { return Test-Path (Split-Path $parent -Parent) -PathType Container }
        "tabnine-cli" { return Test-Path (Split-Path $parent -Parent) -PathType Container }
        "zcode" { return (Test-Path $parent -PathType Container) -or (Test-Path "/Applications/ZCode.app" -PathType Container) }
        "minimax-code" { return (Test-Path $parent -PathType Container) -or (Test-Path "/Applications/MiniMax Code.app" -PathType Container) }
        default { return Test-Path $parent -PathType Container }
    }
}

function Test-SamePhysicalSkillRoot {
    param([string]$Left, [string]$Right)
    if (!(Test-Path $Left) -or !(Test-Path $Right)) { return $false }
    $leftPhysical = Get-PhysicalSkillPath -Path $Left
    $rightPhysical = Get-PhysicalSkillPath -Path $Right
    if ($null -eq $leftPhysical -or $null -eq $rightPhysical) { return $false }
    return $leftPhysical.TrimEnd([char[]]@('\', '/')) -ieq $rightPhysical.TrimEnd([char[]]@('\', '/'))
}

function Move-AgentSkillRootToBackup {
    param([string]$Root, [string]$BaseDir)

    $victims = [System.Collections.Generic.List[string]]::new()
    $victims.Add((Join-Path $baseDir $SkillName))
    foreach ($existing in Get-ChildItem -Path $baseDir -Force -ErrorAction SilentlyContinue) {
        if (Test-ManagedMultiSkillDir -Dir $existing.FullName) {
            $victims.Add($existing.FullName)
        }
    }
    $backups = @()
    try {
        $seen = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
        foreach ($victim in $victims) {
            if (!$seen.Add($victim)) { continue }
            $backupPath = ""
            if (!(Backup-SkillDir -Dir $victim -BackupPath ([ref]$backupPath))) {
                throw "Agent Skill 旧副本备份失败: $victim"
            }
            if ($backupPath) {
                $backups += [pscustomobject]@{ Original = $victim; Backup = $backupPath }
            }
        }
        return $true
    } catch {
        Restore-MultiSkillSet -Published @() -Backups $backups | Out-Null
        Write-Say "⚠️  Agent Skill 旧副本迁移失败，已回滚: $_"
        return $false
    }
}

function Publish-CanonicalSkillLinks {
    param([string]$Root, [string]$BaseDir, [string]$Mode, [string[]]$BundleNames = @())
    $canonical = Join-Path $Root ".agents\skills"
    if (!(Test-Path $BaseDir)) { New-Item -ItemType Directory -Path $BaseDir -Force | Out-Null }
    if (Test-SamePhysicalSkillRoot -Left $BaseDir -Right $canonical) { return $true }
    $stageRoot = Join-Path $BaseDir (".dws-link-set-" + [guid]::NewGuid().ToString("N"))
    $backups = @()
    $published = @()
    try {
        New-Item -ItemType Directory -Path $stageRoot -Force -ErrorAction Stop | Out-Null
        if ($Mode -eq "mono") {
            $names = @($SkillName)
        } elseif ($BundleNames.Count -gt 0) {
            # The canonical store is SHARED: enumerate the link set from the
            # installed bundle, never from ~\.agents\skills, or third-party and
            # user skills would be republished into every Agent root.
            $names = @($BundleNames)
        } else {
            # No bundle list means we cannot tell DWS skills apart from the
            # user's own entries in the shared store. Fail this Agent (the
            # caller degrades to a copy install) instead of guessing, matching
            # link_canonical_skills_to_base in scripts/install-skills.sh.
            throw "multi mode requires the installed bundle skill names"
        }
        $publishNames = @()
        foreach ($name in $names) {
            if (Test-SamePhysicalSkillRoot -Left (Join-Path $BaseDir $name) -Right (Join-Path $canonical $name)) { continue }
            $absoluteTarget = [System.IO.Path]::GetFullPath((Join-Path $canonical $name))
            New-Item -ItemType Junction -Path (Join-Path $stageRoot $name) -Target $absoluteTarget -ErrorAction Stop | Out-Null
            $publishNames += $name
        }
        $victims = [System.Collections.Generic.List[string]]::new()
        $victims.Add((Join-Path $BaseDir $SkillName))
        foreach ($existing in Get-ChildItem -Path $BaseDir -Force -ErrorAction SilentlyContinue) {
            if ($existing.FullName -eq $stageRoot) { continue }
            if (Test-ManagedMultiSkillDir -Dir $existing.FullName) { $victims.Add($existing.FullName) }
        }
        # Every published name replaces whatever occupies its destination, even
        # when that copy predates central ownership metadata; the loop below
        # still skips destinations that are already correct links.
        foreach ($name in $names) {
            $victims.Add((Join-Path $BaseDir $name))
        }
        $seen = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
        foreach ($victim in $victims) {
            if (!$seen.Add($victim)) { continue }
            if (Test-SamePhysicalSkillRoot -Left $victim -Right (Join-Path $canonical (Split-Path $victim -Leaf))) { continue }
            $backupPath = ""
            if (!(Backup-SkillDir -Dir $victim -BackupPath ([ref]$backupPath))) { throw "Skill 备份失败: $victim" }
            if ($backupPath) { $backups += [pscustomobject]@{ Original = $victim; Backup = $backupPath } }
        }
        foreach ($name in $publishNames) {
            $dest = Join-Path $BaseDir $name
            Move-SkillPath -Source (Join-Path $stageRoot $name) -Destination $dest
            # Only record a publication after the staged junction has occupied
            # the exact destination. Rollback also checks this identity before
            # deleting, so a concurrent user directory is never removed.
            $published += New-PublishedSkillLinkRecord -Path $dest
            Write-Say "↪ Skills → $dest"
        }
        return $true
    } catch {
        Restore-MultiSkillSet -Published $published -Backups $backups | Out-Null
        Write-Say "⚠️  Skill 链接发布失败，已回滚: $BaseDir ($_)"
        return $false
    } finally {
        Remove-LinkStageRoot -StageRoot $stageRoot | Out-Null
    }
}

function Copy-SkillToDir {
    param([string]$SkillSrc, [string]$Dest, [string]$Label)

    # Refreshing an existing skill: back it up first; on backup failure keep
    # the user's copy and skip this target.
    if (!(Backup-SkillDir -Dir $Dest)) {
        Write-Say "⚠️  跳过 $Dest（保留原目录）"
        return $false
    }

    $fileCount = Copy-DirRecursive -Source $SkillSrc -Destination $Dest
    Write-Say "✅ Skills → $Label ($fileCount files)"

    # List top-level contents for visibility
    Get-ChildItem -Path $Dest | ForEach-Object {
        if ($_.PSIsContainer) {
            $subCount = (Get-ChildItem -Path $_.FullName -Recurse -File).Count
            Write-Say "   📁 $($_.Name)/ ($subCount files)"
        } else {
            Write-Say "   📄 $($_.Name)"
        }
    }
    return $true
}

function Copy-SkillToDirSummary {
    param([string]$SkillSrc, [string]$Dest, [string]$Label)

    if (!(Backup-SkillDir -Dir $Dest)) {
        Write-Say "⚠️  跳过 $Dest（保留原目录）"
        return $false
    }

    $fileCount = Copy-DirRecursive -Source $SkillSrc -Destination $Dest
    Write-Say "✅ Skills → $Label ($fileCount files)"
    return $true
}

function Resolve-SourceRoot {
    $scriptPath = $PSScriptRoot
    if (-not $scriptPath) { return $null }
    $candidateRoot = Split-Path $scriptPath -Parent
    if ((Test-Path (Join-Path $candidateRoot "go.mod")) -and (Test-Path (Join-Path $candidateRoot "cmd"))) {
        return $candidateRoot
    }
    return $null
}

# ── Banner ───────────────────────────────────────────────────────────────────

function Write-Banner {
    Write-Host ""
    Write-Say "┌──────────────────────────────────────┐"
    Write-Say "│     DWS Installer                    │"
    Write-Say "│     DingTalk Workspace CLI            │"
    Write-Say "└──────────────────────────────────────┘"
    Write-Host ""
}

# ── Skill Mode Resolution ────────────────────────────────────────────────────
#
# Priority (highest first):
#   1. DWS_SKILL_MODE env var (mono | multi, case-insensitive)
#   2. Interactive prompt when both stdin and stdout are TTYs (default: multi)
#   3. Fallback: multi (non-TTY without env var, e.g. irm | iex)
function Resolve-SkillMode {
    if ($env:DWS_SKILL_MODE) {
        $normalized = $env:DWS_SKILL_MODE.ToLower()
        if ($normalized -eq "mono" -or $normalized -eq "multi") {
            $script:SkillMode = $normalized
            Write-Say "Skill mode: $SkillMode (from DWS_SKILL_MODE)"
            return
        }
        Write-Err "Invalid DWS_SKILL_MODE='$($env:DWS_SKILL_MODE)'. Use 'mono' or 'multi'."
    }

    $isInteractive = $false
    try {
        $isInteractive = ([Console]::IsInputRedirected -eq $false) -and ([Console]::IsOutputRedirected -eq $false)
    } catch {
        $isInteractive = $false
    }

    if ($isInteractive) {
        Write-Host ""
        Write-Say "Select skill installation mode:"
        Write-Say "  1) multi (default) — split each product into its own skill (dingtalk-*)"
        Write-Say "  2) mono            — install one bundled dws skill (legacy)"
        $choice = Read-Host "  Choice [1]"
        switch ($choice) {
            ""      { $script:SkillMode = "multi" }
            "1"     { $script:SkillMode = "multi" }
            "multi" { $script:SkillMode = "multi" }
            "2"     { $script:SkillMode = "mono" }
            "mono"  { $script:SkillMode = "mono" }
            default {
                Write-Say "Unrecognized choice '$choice', defaulting to multi."
                $script:SkillMode = "multi"
            }
        }
        Write-Say "Skill mode: $SkillMode"
        return
    }

    $script:SkillMode = "multi"
}

# ── Install Binary ───────────────────────────────────────────────────────────

function Publish-RuntimePayload {
    param([string]$Source)

    if (!(Test-Path -LiteralPath (Join-Path $Source "manifest.json") -PathType Leaf)) {
        Write-Err "Runtime payload manifest is missing."
    }
    $psPath = Join-Path $Source "ps"
    $psCount = @(Get-ChildItem -LiteralPath $psPath -File -Recurse -ErrorAction SilentlyContinue).Count
    if ($psCount -ne 123) {
        Write-Err "Runtime payload is incomplete (expected 123 ps files, found $psCount)."
    }
    $libraryCount = @(Get-ChildItem -LiteralPath $Source -File -Filter "*.dll" -ErrorAction SilentlyContinue).Count
    if ($libraryCount -ne 1) {
        Write-Err "Runtime payload must contain exactly one target library."
    }

    $payloadParent = Join-Path $InstallDir ".dws-runtime"
    New-Item -ItemType Directory -Path $payloadParent -Force | Out-Null
    $payloadStage = Join-Path $payloadParent ".20260825.tmp-$PID"
    $payloadTarget = Join-Path $payloadParent "20260825"
    $payloadBackup = Join-Path $payloadParent ".20260825.old-$PID"
    Remove-Item -LiteralPath $payloadStage -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $payloadBackup -Recurse -Force -ErrorAction SilentlyContinue
    Copy-Item -LiteralPath $Source -Destination $payloadStage -Recurse -Force

    if (Test-Path -LiteralPath $payloadTarget) {
        Move-Item -LiteralPath $payloadTarget -Destination $payloadBackup -Force
    }
    try {
        Move-Item -LiteralPath $payloadStage -Destination $payloadTarget -Force
    } catch {
        if (Test-Path -LiteralPath $payloadBackup) {
            Move-Item -LiteralPath $payloadBackup -Destination $payloadTarget -Force
        }
        throw
    }
    Remove-Item -LiteralPath $payloadBackup -Recurse -Force -ErrorAction SilentlyContinue
}

function Publish-RuntimePayloadIfPresent {
    param([string]$Source)

    if (!(Test-Path -LiteralPath $Source -PathType Container)) {
        Write-Say "Runtime payload is not included in this archive; continuing with binary-only installation."
        return
    }
    Publish-RuntimePayload -Source $Source
}

function Install-Binary {
    $arch = Get-Arch
    Resolve-LatestVersion

    $archiveName = "${BinName}-windows-${arch}.zip"
    if ($GiteeRepo -ne "") { $downloadUrl = Get-GiteeAssetUrl $archiveName } else { $downloadUrl = "https://github.com/$Repo/releases/download/$Version/$archiveName" }
    if (-not $downloadUrl) { Write-Err "Could not resolve download URL for $archiveName (version $Version)." }

    Write-Say "⬇  Downloading $BinName $Version (windows/$arch)..."

    $tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) "dws-install-$PID"
    New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

    try {
        $archivePath = Join-Path $tmpDir $archiveName
        Invoke-WebRequest -Uri $downloadUrl -OutFile $archivePath -UseBasicParsing

        Assert-ReleaseAssetChecksum -AssetPath $archivePath -AssetName $archiveName -TempDir $tmpDir

        Write-Say "📦 Extracting..."
        Expand-Archive -Path $archivePath -DestinationPath $tmpDir -Force

        # Create install directory
        if (!(Test-Path $InstallDir)) {
            New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        }

        # Find the binary
        $binFile = Get-ChildItem -Path $tmpDir -Recurse -Filter "${BinName}.exe" | Select-Object -First 1
        if ($null -eq $binFile) {
            Write-Err "Could not find ${BinName}.exe in the downloaded archive."
        }

        $payloadSource = Join-Path $binFile.Directory.FullName ".dws-runtime\20260825"
        Publish-RuntimePayloadIfPresent -Source $payloadSource

        $destBin = Join-Path $InstallDir "${BinName}.exe"
        $stagedBin = Join-Path $InstallDir ".${BinName}.tmp-$PID.exe"
        Copy-Item -Path $binFile.FullName -Destination $stagedBin -Force
        Move-Item -Path $stagedBin -Destination $destBin -Force

        Write-Say "✅ Binary installed:"
        Write-Say "   → $destBin"

        # Check if install dir is in PATH
        $userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
        if ($userPath -notlike "*$InstallDir*") {
            Write-Say ""
            Write-Say "⚠️  $InstallDir is not in your PATH."
            Write-Say "   Adding to user PATH..."
            [Environment]::SetEnvironmentVariable("PATH", "$InstallDir;$userPath", "User")
            $env:PATH = "$InstallDir;$env:PATH"
            Write-Say "   ✅ Added to PATH. Restart your terminal for changes to take effect."
        }
    } finally {
        Remove-Item -Path $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# ── Install Skills from Local Source ──────────────────────────────────────────

function Install-SkillsLocal {
    param([string]$Root)
    $skillSrc = Join-Path (Join-Path $Root "skills") "mono"
    $multiSrc = Join-Path (Join-Path $Root "skills") "multi"

    if ($SkillMode -eq "multi" -and (Test-MultiTreeHasSkills $multiSrc)) {
        Write-Say ""
        Write-Say "📦 Installing agent skills (multi) from local source: $multiSrc"
        if (!(Install-MultiSkillsToHomes -MultiSrc $multiSrc -Root $HOME)) {
            throw "multi Skill installation failed"
        }
    } else {
        if ($SkillMode -eq "multi") {
            Write-Say "⚠️  multi skill tree not found or empty at $multiSrc; falling back to mono."
        }
        if (!(Test-Path $skillSrc)) {
            Write-Say "⚠️  Local skills directory not found: $skillSrc"
            Write-Say "   Skipping skills installation."
            return
        }

        Write-Say ""
        Write-Say "📦 Installing agent skills from local source: $skillSrc"

        if (!(Install-SkillsToHomes -SkillSrc $skillSrc -Root $HOME)) {
            throw "mono Skill installation failed"
        }
    }

    if (Test-Path $multiSrc) {
        Cache-MultiSkills -Source $multiSrc
    }
    Cache-MonoSkills -Source $skillSrc
}

# Cache-MultiSkills mirrors install.sh cache_multi_skills: copies the multi/
# tree to ~/.dws/skills/multi/ so `dws skill setup --mode multi` can find a
# source without needing the source checkout or a re-download.
function Cache-MultiSkills {
    param([string]$Source)

    # Never let an empty/corrupt multi\ tree wipe a previously good cache.
    if (!(Test-MultiTreeHasSkills $Source)) { return }

    $cacheDir = Join-Path $HOME ".dws\skills\multi"
    try {
        $count = Publish-SkillCache -Source $Source -CacheDir $cacheDir
        Write-Say "✅ Cached multi skills → $cacheDir ($count files)"
    } catch {
        Write-Say "⚠️ Multi Skill 缓存刷新失败，未覆盖原缓存: $cacheDir ($_)"
    }
}

function Cache-MonoSkills {
    param([string]$Source)

    # Only refresh when the new bundle actually carries a mono tree — a
    # multi-only bundle must never wipe a previously good mono cache.
    if (!(Test-Path (Join-Path $Source "SKILL.md"))) { return }

    $cacheDir = Join-Path $HOME ".dws\skills\mono"
    try {
        Publish-SkillCache -Source $Source -CacheDir $cacheDir | Out-Null
    } catch {
        Write-Say "⚠️ Mono Skill 缓存刷新失败，未覆盖原缓存: $cacheDir ($_)"
    }
}

function Install-MonoToBase {
    param(
        [string]$SkillSrc,
        [string]$BaseDir,
        [string]$Label
    )

    if (!(Test-Path $BaseDir)) {
        New-Item -ItemType Directory -Path $BaseDir -Force | Out-Null
    }
    $stageRoot = Join-Path $BaseDir (".dws-mono-set-" + [guid]::NewGuid().ToString("N"))
    $stagedSkill = Join-Path $stageRoot $SkillName
    $dest = Join-Path $BaseDir $SkillName
    $backups = @()
    $published = @()
    try {
        # Stage the complete mono tree before moving any Agent-visible
        # directory, including every mutually-exclusive managed multi Skill.
        New-Item -ItemType Directory -Path $stageRoot -Force -ErrorAction Stop | Out-Null
        Copy-DirRecursive -Source $SkillSrc -Destination $stagedSkill | Out-Null

        $victims = [System.Collections.Generic.List[string]]::new()
        $victims.Add($dest)
        foreach ($existing in Get-ChildItem -Path $BaseDir -Directory -ErrorAction SilentlyContinue) {
            if ($existing.FullName -eq $stageRoot) { continue }
            if (Test-ManagedMultiSkillDir -Dir $existing.FullName) {
                $victims.Add($existing.FullName)
            }
        }

        $seen = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
        foreach ($victim in $victims) {
            if (!$seen.Add($victim)) { continue }
            $backupPath = ""
            if (!(Backup-SkillDir -Dir $victim -BackupPath ([ref]$backupPath))) {
                throw "Skill 备份失败: $victim"
            }
            if ($backupPath) {
                $backups += [pscustomobject]@{ Original = $victim; Backup = $backupPath }
            }
        }

        Move-SkillPath -Source $stagedSkill -Destination $dest
        $published += [pscustomobject]@{ Path = $dest; Source = $SkillSrc }
        Assert-SkillPathCopy -Source $SkillSrc -Destination $dest
    } catch {
        $transactionError = $_
        if (!(Restore-MultiSkillSet -Published $published -Backups $backups)) {
            Write-Say "⚠️  原 Skill 集合自动恢复不完整，请检查上方备份路径"
        }
        Write-Say "⚠️  mono Skill 集合发布失败，目标已回滚: $BaseDir ($transactionError)"
        return $false
    } finally {
        if (Test-Path $stageRoot) {
            Remove-Item -LiteralPath $stageRoot -Recurse -Force -ErrorAction SilentlyContinue
        }
    }

    $fileCount = (Get-ChildItem -Path $dest -Recurse -File).Count
    Write-Say "✅ Skills → $Label ($fileCount files)"
    return $true
}

function Install-SkillsToHomes {
    param(
        [string]$SkillSrc,
        [string]$Root = $HOME
    )

    $installed = 0
    $attempted = 1
    $failed = 0
    $canonical = Join-Path $Root ".agents\skills"
    if (Install-MonoToBase -SkillSrc $SkillSrc -BaseDir $canonical -Label "~\.agents\skills\$SkillName") { $installed++ } else { $failed++ }
    if ($installed -eq 0) { return $false }
    $seenBases = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
    foreach ($agent in @($AgentRegistry) + @($LegacyAgentCleanupTargets)) {
        $baseDir = Resolve-AgentSkillBase -Root $Root -Agent $agent
        if ([string]::IsNullOrWhiteSpace($baseDir)) { continue }
        $baseKey = [System.IO.Path]::GetFullPath($baseDir).TrimEnd([char[]]@('\', '/'))
        if (!$seenBases.Add($baseKey)) { continue }
        if (!(Test-AgentSkillBaseDetected -BaseDir $baseDir -Agent $agent)) { continue }
        if (Test-SamePhysicalSkillRoot -Left $baseDir -Right $canonical) { continue }
        $attempted++
        if ($agent.Universal) {
            # Cleanup-only migration: universal Agents read the canonical store
            # directly, so nothing is installed here. Leaving an obsolete copy
            # behind is a warning, never a reason to fail an install whose
            # canonical store and links all succeeded.
            if (!(Move-AgentSkillRootToBackup -Root $Root -BaseDir $baseDir)) {
                Write-Say "⚠️  Agent Skill 旧副本迁移失败（不影响本次安装）: $baseDir"
            }
            continue
        }
        if (Publish-CanonicalSkillLinks -Root $Root -BaseDir $baseDir -Mode "mono") {
            $installed++
        } else {
            if (Install-MonoToBase -SkillSrc $SkillSrc -BaseDir $baseDir -Label (Join-Path $baseDir $SkillName)) {
                Write-Say "ℹ️  $baseDir 已自动使用兼容方式安装，可正常使用"
                $installed++
            } else { $failed++ }
        }
    }
    if ($installed -eq 0) {
        Write-Say "⚠️  未安装任何 mono Skill：所有检测到的 Agent 目标均失败"
        return $false
    }
    if ($failed -gt 0) {
        Write-Say "⚠️  有 $failed 个 Agent 目标安装 mono Skill 失败"
        return $false
    }
    Remove-Item -LiteralPath (Join-Path $SkillStateRoot "skills-state.json") -Force -ErrorAction SilentlyContinue
    Write-Say "✅ DWS Skills 安装完成"
    Write-Say "   统一安装位置：$canonical"
    Write-Say "   已自动适配本机上检测到的 Agent"
    Write-Say "ℹ️  下一步：请重启已打开的 Agent，使新 Skills 生效"
    return $true
}

# Test-MultiTreeHasSkills returns $true only when the multi bundle directory
# contains at least one product skill (a subdirectory with a SKILL.md). An
# empty or corrupt multi\ tree must never select the multi branch: installing
# it would delete existing dws\ + dingtalk-* skills and lay down nothing.
function Test-MultiTreeHasSkills {
    param([string]$MultiSrc)
    if (!(Test-Path $MultiSrc)) { return $false }
    foreach ($dir in Get-ChildItem -Path $MultiSrc -Directory -ErrorAction SilentlyContinue) {
        if (Test-Path (Join-Path $dir.FullName "SKILL.md")) { return $true }
    }
    return $false
}

# Install the multi skill bundle (one subdirectory per product skill) into all
# agent homes as sibling directories, mirroring `dws skill setup --mode multi`.
# Mutual exclusion: the mono leftover (<home>\dws) and stale DWS-managed Skills
# not present in the new bundle are removed first.
function Install-MultiSkillsToHomes {
    param(
        [string]$MultiSrc,
        [string]$Root = $HOME
    )

    $installed = 0
    $attempted = 1
    $failed = 0
    $canonical = Join-Path $Root ".agents\skills"
    # The link set must come from the installed bundle, not from the shared
    # canonical store (which may also hold user/third-party skills).
    $bundleNames = @(Get-ChildItem -Path $MultiSrc -Directory -ErrorAction SilentlyContinue | Where-Object {
        Test-Path (Join-Path $_.FullName "SKILL.md")
    } | ForEach-Object { $_.Name })
    if (Install-MultiToBase -MultiSrc $MultiSrc -BaseDir $canonical -Root $Root -AgentDir ".agents\skills") { $installed++ } else { $failed++ }
    if ($installed -eq 0) { return $false }
    $seenBases = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
    foreach ($agent in @($AgentRegistry) + @($LegacyAgentCleanupTargets)) {
        $baseDir = Resolve-AgentSkillBase -Root $Root -Agent $agent
        if ([string]::IsNullOrWhiteSpace($baseDir)) { continue }
        $baseKey = [System.IO.Path]::GetFullPath($baseDir).TrimEnd([char[]]@('\', '/'))
        if (!$seenBases.Add($baseKey)) { continue }
        if (!(Test-AgentSkillBaseDetected -BaseDir $baseDir -Agent $agent)) { continue }
        if (Test-SamePhysicalSkillRoot -Left $baseDir -Right $canonical) { continue }
        $attempted++
        if ($agent.Universal) {
            # Cleanup-only migration (see Install-SkillsToHomes): a retire
            # failure must not fail an otherwise complete install.
            if (!(Move-AgentSkillRootToBackup -Root $Root -BaseDir $baseDir)) {
                Write-Say "⚠️  Agent Skill 旧副本迁移失败（不影响本次安装）: $baseDir"
            }
            continue
        }
        if (Publish-CanonicalSkillLinks -Root $Root -BaseDir $baseDir -Mode "multi" -BundleNames $bundleNames) {
            $installed++
        } else {
            if (Install-MultiToBase -MultiSrc $MultiSrc -BaseDir $baseDir -Root $Root -AgentDir $agent.Dir) {
                Write-Say "ℹ️  $baseDir 已自动使用兼容方式安装，可正常使用"
                $installed++
            } else { $failed++ }
        }
    }
    if ($installed -eq 0) {
        Write-Say "⚠️  未安装任何 multi Skill：所有检测到的 Agent 目标均失败"
        return $false
    }
    if ($failed -gt 0) {
        Write-Say "⚠️  有 $failed 个 Agent 目标安装 multi Skill 失败"
        return $false
    }
    Write-SkillsState -MultiSrc $MultiSrc
    Write-Say "✅ DWS Skills 安装完成"
    Write-Say "   统一安装位置：$canonical"
    Write-Say "   已自动适配本机上检测到的 Agent"
    Write-Say "ℹ️  下一步：请重启已打开的 Agent，使新 Skills 生效"
    return $true
}

function Install-MultiToBase {
    param(
        [string]$MultiSrc,
        [string]$BaseDir,
        [string]$Root,
        [string]$AgentDir
    )

    if (!(Test-Path $BaseDir)) {
        New-Item -ItemType Directory -Path $BaseDir -Force | Out-Null
    }

    $skillDirs = @(Get-ChildItem -Path $MultiSrc -Directory | Where-Object {
        Test-Path (Join-Path $_.FullName "SKILL.md")
    })
    $stageRoot = Join-Path $BaseDir (".dws-multi-set-" + [guid]::NewGuid().ToString("N"))
    $backups = @()
    $published = @()
    try {
        # Stage the complete replacement before moving any Agent-visible
        # directory. Copy failures therefore leave the old set untouched.
        New-Item -ItemType Directory -Path $stageRoot -Force -ErrorAction Stop | Out-Null
        foreach ($skillDir in $skillDirs) {
            Copy-DirRecursive -Source $skillDir.FullName -Destination (Join-Path $stageRoot $skillDir.Name) | Out-Null
        }

        $victims = [System.Collections.Generic.List[string]]::new()
        $victims.Add((Join-Path $BaseDir $SkillName))

        # Include stale, proven DWS-managed skills in the same transaction.
        foreach ($existing in Get-ChildItem -Path $BaseDir -Directory -ErrorAction SilentlyContinue) {
            if ($existing.FullName -eq $stageRoot) { continue }
            if ((Test-ManagedMultiSkillDir -Dir $existing.FullName) -and
                !(Test-Path (Join-Path (Join-Path $MultiSrc $existing.Name) "SKILL.md"))) {
                $victims.Add($existing.FullName)
            }
        }
        if (!(Test-Path (Join-Path (Join-Path $MultiSrc "dws-shared") "SKILL.md"))) {
            $victims.Add((Join-Path $BaseDir "dws-shared"))
        }
        foreach ($skillDir in $skillDirs) {
            $victims.Add((Join-Path $BaseDir $skillDir.Name))
        }

        $seen = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
        foreach ($victim in $victims) {
            if (!$seen.Add($victim)) { continue }
            $backupPath = ""
            if (!(Backup-SkillDir -Dir $victim -BackupPath ([ref]$backupPath))) {
                throw "Skill 备份失败: $victim"
            }
            if ($backupPath) {
                $backups += [pscustomobject]@{ Original = $victim; Backup = $backupPath }
            }
        }

        foreach ($skillDir in $skillDirs) {
            $dest = Join-Path $BaseDir $skillDir.Name
            Move-SkillPath -Source (Join-Path $stageRoot $skillDir.Name) -Destination $dest
            $published += [pscustomobject]@{ Path = $dest; Source = $skillDir.FullName }
            Assert-SkillPathCopy -Source $skillDir.FullName -Destination $dest
        }
    } catch {
        $transactionError = $_
        if (!(Restore-MultiSkillSet -Published $published -Backups $backups)) {
            Write-Say "⚠️  原 Skill 集合自动恢复不完整，请检查上方备份路径"
        }
        Write-Say "⚠️  multi Skill 集合发布失败，目标已回滚: $BaseDir ($transactionError)"
        return $false
    } finally {
        if (Test-Path $stageRoot) {
            Remove-Item -LiteralPath $stageRoot -Recurse -Force -ErrorAction SilentlyContinue
        }
    }

    $count = $skillDirs.Count

    if ($Root -eq $HOME) {
        $label = "~\$AgentDir\"
    } else {
        $label = Join-Path $Root $AgentDir
    }
    Write-Say "✅ Skills → $label ($count product skills)"
    return $true
}

# ── Install Binary from Source ───────────────────────────────────────────────

function Install-BinaryFromSource {
    param([string]$Root)

    if (!(Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Err "Missing required command: go"
    }

    Write-Say "Installing dws from source checkout: $Root"
    Write-Say "Install dir: $InstallDir"

    if (!(Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }

    $tmpBin = Join-Path ([System.IO.Path]::GetTempPath()) "dws-build-$PID.exe"
    $tmpPayloadRoot = Join-Path ([System.IO.Path]::GetTempPath()) "dws-runtime-build-$PID"
    try {
        & go build -ldflags="-s -w" -o $tmpBin "$Root/cmd"
        if ($LASTEXITCODE -ne 0) { throw "go build failed with exit code $LASTEXITCODE" }

        $targetArch = Get-Arch
        $payloadSource = Join-Path $Root "third_party\runtimepayload\20260825\windows\$targetArch\x7k2m9p4q1w864.dll"
        $psSource = Join-Path $Root "third_party\runtimepayload\20260825\ps"
        $preparedPayload = Join-Path $tmpPayloadRoot "20260825"
        New-Item -ItemType Directory -Path (Join-Path $preparedPayload "ps") -Force | Out-Null
        $preparedLibrary = Join-Path $preparedPayload "x7k2m9p4q1w864.dll"
        Copy-Item -LiteralPath $payloadSource -Destination $preparedLibrary -Force
        Copy-Item -Path (Join-Path $psSource "*") -Destination (Join-Path $preparedPayload "ps") -Recurse -Force
        $manifest = [ordered]@{
            format_version = 1
            payload_version = "20260825"
            target = "windows/$targetArch"
            library = "x7k2m9p4q1w864.dll"
            library_sha256 = (Get-FileHash -LiteralPath $preparedLibrary -Algorithm SHA256).Hash.ToLowerInvariant()
            ps_file_count = 123
            ps_manifest_sha256 = "45ae147697c1f8683df3f232d0ba792b807179bbe22fdac8225a0cf25fc33e7e"
        }
        $manifest | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $preparedPayload "manifest.json") -Encoding UTF8
        Publish-RuntimePayload -Source $preparedPayload

        $destBin = Join-Path $InstallDir "${BinName}.exe"
        $stagedBin = Join-Path $InstallDir ".${BinName}.tmp-$PID.exe"
        Copy-Item -LiteralPath $tmpBin -Destination $stagedBin -Force
        Move-Item -LiteralPath $stagedBin -Destination $destBin -Force
        Write-Say "✅ Binary installed:"
        Write-Say "   → $destBin"
    } finally {
        Remove-Item -Path $tmpBin -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $tmpPayloadRoot -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# ── Install Skills from Remote ───────────────────────────────────────────────

function Install-Skills {
    Write-Say ""
    Write-Say "📦 Installing agent skills from GitHub Releases..."
    Resolve-LatestVersion

    if ($GiteeRepo -ne "") { $zipUrl = Get-GiteeAssetUrl "dws-skills.zip" } else { $zipUrl = "https://github.com/$Repo/releases/download/$Version/dws-skills.zip" }
    if (-not $zipUrl) { Write-Err "Could not resolve download URL for dws-skills.zip (version $Version)." }

    $tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) "dws-skills-$PID"
    New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

    try {
        $zipPath = Join-Path $tmpDir "repo.zip"
        try {
            Invoke-WebRequest -Uri $zipUrl -OutFile $zipPath -UseBasicParsing
        } catch {
            Write-Say "⚠️  Release asset download failed. Trying local source..."
            $localRoot = Resolve-SourceRoot
            if ($localRoot) {
                Install-SkillsLocal -Root $localRoot
                return
            } else {
                Write-Err "Cannot download skills from GitHub and no local source checkout found."
            }
        }
        Assert-ReleaseAssetChecksum -AssetPath $zipPath -AssetName "dws-skills.zip" -TempDir $tmpDir

        $extractRoot = Join-Path $tmpDir "skills"
        Expand-Archive -Path $zipPath -DestinationPath $extractRoot -Force

        # Prefer the explicit mono/ subtree; fall back to legacy nested or zip root.
        $skillSrc = $extractRoot
        $monoRoot = Join-Path $extractRoot "mono"
        if ((Test-Path $monoRoot) -and (Test-Path (Join-Path $monoRoot "SKILL.md"))) {
            $skillSrc = $monoRoot
        } elseif (Test-Path (Join-Path $extractRoot "$SkillName\SKILL.md")) {
            $skillSrc = Join-Path $extractRoot $SkillName
        }

        # Multi first: a release may ship only the multi\ tree without the
        # root mono copy, so the mono SKILL.md gate must never block a multi
        # install. An empty/corrupt multi\ tree (no *\SKILL.md) falls back to
        # mono with a warning — installing it would wipe existing skills and
        # lay down nothing.
        $multiRoot = Join-Path $extractRoot "multi"
        if ($SkillMode -eq "multi" -and (Test-MultiTreeHasSkills $multiRoot)) {
            if (!(Install-MultiSkillsToHomes -MultiSrc $multiRoot -Root $HOME)) {
                throw "multi Skill installation failed"
            }
        } else {
            if ($SkillMode -eq "multi") {
                Write-Say "⚠️  multi skill tree not found or empty in release asset; falling back to mono."
            }
            if (!(Test-Path (Join-Path $skillSrc "SKILL.md"))) {
                Write-Say "⚠️  Skills not found in release asset. Trying local source..."
                $localRoot = Resolve-SourceRoot
                if ($localRoot) {
                    Install-SkillsLocal -Root $localRoot
                    return
                }
                Write-Say "⚠️  No local source found either. Skipping skills installation."
                return
            }
            if (!(Install-SkillsToHomes -SkillSrc $skillSrc -Root $HOME)) {
                throw "mono Skill installation failed"
            }
        }

        # Cache the multi/ tree (and a mono copy) under ~/.dws/skills so that
        # subsequent `dws skill setup --mode multi|mono` can find a source.
        if (Test-Path $multiRoot) {
            Cache-MultiSkills -Source $multiRoot
        }
        Cache-MonoSkills -Source $skillSrc
    } finally {
        Remove-Item -Path $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# ── Main ─────────────────────────────────────────────────────────────────────

$SourceRoot = Resolve-SourceRoot

Write-Banner

# Pick GitHub vs Gitee mirror (auto-fallback when GitHub is unreachable).
# Skipped when installing from a local source checkout (no download needed).
if (-not $SourceRoot) { Resolve-Source }

if (!$NoSkills) {
    Resolve-SkillMode
}

if ($SourceRoot -and !$SkillsOnly -and ($Version -eq "latest")) {
    Install-BinaryFromSource -Root $SourceRoot
    if (!$NoSkills) {
        Install-SkillsLocal -Root $SourceRoot
    }
} elseif ($SkillsOnly) {
    Install-Skills
} elseif ($NoSkills) {
    Install-Binary
} else {
    Install-Binary
    Install-Skills
}

Write-Host ""
Write-Say "🎉 Installation complete!"
Write-Say ""
Write-Say "Next steps:"
if (!$SkillsOnly) {
    Write-Say "  $BinName version          # verify installation"
    Write-Say "  $BinName auth login       # authenticate with DingTalk"
}
Write-Say "  $BinName --help           # explore commands"
Write-Host ""
