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

# Agent skill base directories (same order as build/npm/install.js AGENT_DIRS).
$AgentDirs = @(
    ".agents\skills",
    ".claude\skills",
    ".cursor\skills",
    ".qoder\skills",
    ".qoderwork\skills",
    ".gemini\skills",
    ".codex\skills",
    ".github\skills",
    ".windsurf\skills",
    ".augment\skills",
    ".cline\skills",
    ".amp\skills",
    ".kiro\skills",
    ".trae\skills",
    ".openclaw\skills",
    ".hermes\skills"
)

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
    Move-Item -LiteralPath $Source -Destination $Destination -ErrorAction Stop
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
    if (!(Test-Path $Dir -PathType Container)) { return $true }
    $backupRoot = Join-Path $HOME ".dws\skill-backups"
    $stamp = [DateTime]::UtcNow.ToString("yyyyMMdd-HHmmss")
    $name = Split-Path $Dir -Leaf
    $target = Join-Path (Join-Path $backupRoot $stamp) $name
    $i = 1
    while (Test-Path $target) {
        $target = Join-Path (Join-Path $backupRoot "$stamp-$i") $name
        $i++
        if ($i -gt 1000) {
            Write-Say "⚠️  备份目录冲突，保留原目录 $Dir"
            return $false
        }
    }
    try {
        New-Item -ItemType Directory -Path (Split-Path $target -Parent) -Force -ErrorAction Stop | Out-Null
        Move-SkillPath -Source $Dir -Destination $target
    } catch {
        Write-Say "⚠️  备份失败，保留原目录 $Dir"
        return $false
    }
    if ($null -ne $BackupPath) { $BackupPath.Value = $target }
    Write-Say "  × 已备份并移除 $Dir → $target"
    return $true
}

function Restore-MultiSkillSet {
    param(
        [array]$Published,
        [array]$Backups
    )
    $ok = $true
    for ($i = $Published.Count - 1; $i -ge 0; $i--) {
        try {
            if (Test-Path $Published[$i]) {
                Remove-Item -LiteralPath $Published[$i] -Recurse -Force -ErrorAction Stop
            }
        } catch {
            Write-Say "⚠️  无法移除失败发布目录 $($Published[$i]): $_"
            $ok = $false
        }
    }
    for ($i = $Backups.Count - 1; $i -ge 0; $i--) {
        $item = $Backups[$i]
        try {
            if (Test-Path $item.Original) {
                throw "恢复目标仍存在"
            }
            New-Item -ItemType Directory -Path (Split-Path $item.Original -Parent) -Force -ErrorAction Stop | Out-Null
            Move-SkillPath -Source $item.Backup -Destination $item.Original
        } catch {
            Write-Say "⚠️  无法恢复原 Skill $($item.Original)；备份保留于 $($item.Backup): $_"
            $ok = $false
        }
    }
    return $ok
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

        # Download and verify SHA256 checksum
        if ($GiteeRepo -ne "") { $checksumUrl = Get-GiteeAssetUrl "checksums.txt" } else { $checksumUrl = "https://github.com/$Repo/releases/download/$Version/checksums.txt" }
        try {
            $checksumPath = Join-Path $tmpDir "checksums.txt"
            Invoke-WebRequest -Uri $checksumUrl -OutFile $checksumPath -UseBasicParsing
            $checksumContent = Get-Content $checksumPath
            $expectedLine = $checksumContent | Where-Object { $_ -match [regex]::Escape($archiveName) }
            if ($expectedLine) {
                $expected = ($expectedLine -split '\s+')[0]
                $actual = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash.ToLower()
                if ($actual -ne $expected.ToLower()) {
                    Write-Err "SHA256 checksum mismatch! Expected $expected, got $actual. Aborting."
                }
                Write-Say "✅ SHA256 checksum verified"
            } else {
                Write-Say "⚠️  Archive not found in checksums.txt; skipping verification"
            }
        } catch {
            Write-Say "⚠️  Could not download checksums.txt; skipping verification"
        }

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

        $destBin = Join-Path $InstallDir "${BinName}.exe"
        Copy-Item -Path $binFile.FullName -Destination $destBin -Force

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

        $published += $dest
        Move-SkillPath -Source $stagedSkill -Destination $dest
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
    $attempted = 0
    $failed = 0
    for ($i = 0; $i -lt $AgentDirs.Count; $i++) {
        $agentDir = $AgentDirs[$i]
        $baseDir = Join-Path $Root $agentDir
        $parentGate = Split-Path $baseDir -Parent
        if ($i -gt 0 -and !(Test-Path $parentGate)) {
            continue
        }
        $attempted++
        if ($Root -eq $HOME) {
            $label = "~\$agentDir\$SkillName"
        } else {
            $label = Join-Path $Root (Join-Path $agentDir $SkillName)
        }
        $copied = Install-MonoToBase -SkillSrc $SkillSrc -BaseDir $baseDir -Label $label
        if ($copied) {
            $installed++
        } else {
            $failed++
        }
    }
    if ($attempted -eq 0) {
        $fallback = Join-Path (Join-Path $Root ".agents\skills") $SkillName
        if ($Root -eq $HOME) {
            $flabel = "~\.agents\skills\$SkillName"
        } else {
            $flabel = Join-Path $Root (Join-Path ".agents\skills" $SkillName)
        }
        if (Install-MonoToBase -SkillSrc $SkillSrc -BaseDir (Split-Path $fallback -Parent) -Label $flabel) {
            $installed++
        } else {
            $failed++
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
    $attempted = 0
    $failed = 0
    for ($i = 0; $i -lt $AgentDirs.Count; $i++) {
        $agentDir = $AgentDirs[$i]
        $baseDir = Join-Path $Root $agentDir
        $parentGate = Split-Path $baseDir -Parent
        if ($i -gt 0 -and !(Test-Path $parentGate)) {
            continue
        }
        $attempted++
        if (Install-MultiToBase -MultiSrc $MultiSrc -BaseDir $baseDir -Root $Root -AgentDir $agentDir) {
            $installed++
        } else {
            Write-Say "⚠️  跳过 $baseDir（备份失败，未安装 multi）"
            $failed++
        }
    }
    if ($attempted -eq 0) {
        if (Install-MultiToBase -MultiSrc $MultiSrc -BaseDir (Join-Path $Root ".agents\skills") -Root $Root -AgentDir ".agents\skills") {
            $installed++
        } else {
            $failed++
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
            $published += $dest
            Move-SkillPath -Source (Join-Path $stageRoot $skillDir.Name) -Destination $dest
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
    try {
        & go build -ldflags="-s -w" -o $tmpBin "$Root/cmd"
        $destBin = Join-Path $InstallDir "${BinName}.exe"
        Copy-Item -Path $tmpBin -Destination $destBin -Force
        Write-Say "✅ Binary installed:"
        Write-Say "   → $destBin"
    } finally {
        Remove-Item -Path $tmpBin -Force -ErrorAction SilentlyContinue
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
