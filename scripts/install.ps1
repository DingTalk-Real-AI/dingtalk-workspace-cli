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
#   DWS_SKILL_MODE    — mono | multi (default: prompt if TTY, else mono)
#   DWS_GITEE_REPO    — "owner/repo" on Gitee; resolve version + assets via the
#                       Gitee API instead of GitHub (China mirror)
#
# Agent skills paths follow build/npm/install.js AGENT_DIRS (order and entries must match).

$ErrorActionPreference = "Stop"

$Repo = "DingTalk-Real-AI/dingtalk-workspace-cli"
$BinName = "dws"
# China mirror: Gitee repo "owner/repo". When set, version + asset URLs resolve via the Gitee API.
$GiteeRepo = if ($env:DWS_GITEE_REPO) { $env:DWS_GITEE_REPO } else { "" }
$InstallDir = if ($env:DWS_INSTALL_DIR) { $env:DWS_INSTALL_DIR } else { Join-Path $HOME ".local\bin" }
$Version = if ($env:DWS_VERSION) { $env:DWS_VERSION } else { "latest" }
$NoSkills = $env:DWS_NO_SKILLS -eq "1"
$SkillsOnly = $env:DWS_SKILLS_ONLY -eq "1"
$SkillName = "dws"
$SkillMode = ""

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

function Copy-SkillToDir {
    param([string]$SkillSrc, [string]$Dest, [string]$Label)

    # Remove existing installation
    if (Test-Path $Dest) {
        Remove-Item -Path $Dest -Recurse -Force
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
}

function Copy-SkillToDirSummary {
    param([string]$SkillSrc, [string]$Dest, [string]$Label)

    if (Test-Path $Dest) {
        Remove-Item -Path $Dest -Recurse -Force
    }

    $fileCount = Copy-DirRecursive -Source $SkillSrc -Destination $Dest
    Write-Say "✅ Skills → $Label ($fileCount files)"
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
#   2. Interactive prompt when both stdin and stdout are TTYs (default: mono)
#   3. Fallback: mono (non-TTY without env var, e.g. irm | iex)
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
        Write-Say "  1) mono                  — install one bundled dws skill (stable / recommended)"
        Write-Say "  2) multi 🧪 EXPERIMENTAL — split each product into its own skill (preview; run 'dws skill setup --mode multi' afterwards)"
        Write-Say "     ⚠ multi is not yet stable — interface, naming and cross-skill references may change"
        $choice = Read-Host "  Choice [1]"
        switch ($choice) {
            ""      { $script:SkillMode = "mono" }
            "1"     { $script:SkillMode = "mono" }
            "mono"  { $script:SkillMode = "mono" }
            "2"     { $script:SkillMode = "multi" }
            "multi" { $script:SkillMode = "multi" }
            default {
                Write-Say "Unrecognized choice '$choice', defaulting to mono."
                $script:SkillMode = "mono"
            }
        }
        Write-Say "Skill mode: $SkillMode"
        return
    }

    $script:SkillMode = "mono"
}

function Write-MultiModeNotice {
    Write-Say ""
    Write-Say "🧪 Skill mode: multi (EXPERIMENTAL / preview) — automatic skill install skipped."
    Write-Say "   ⚠ multi is not yet stable. 20 product-scoped skills pass dispatch verifier,"
    Write-Say "     but interface, naming and cross-skill references may change in future releases."
    Write-Say "     For production / shared environments, use mono mode (--mode mono)."
    Write-Say ""
    Write-Say "   To install split skills, run:"
    Write-Say "     $BinName skill setup --mode multi"
    Write-Say "   (One skill per product family; requires the dws binary installed above.)"
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

    if (!(Test-Path $skillSrc)) {
        Write-Say "⚠️  Local skills directory not found: $skillSrc"
        Write-Say "   Skipping skills installation."
        return
    }

    Write-Say ""
    Write-Say "📦 Installing agent skills from local source: $skillSrc"

    Install-SkillsToHomes -SkillSrc $skillSrc -Root $HOME

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

    if (!(Test-Path $Source)) { return }

    $cacheDir = Join-Path $HOME ".dws\skills\multi"
    if (Test-Path $cacheDir) {
        Remove-Item -Path $cacheDir -Recurse -Force
    }
    New-Item -ItemType Directory -Path $cacheDir -Force | Out-Null
    $count = Copy-DirRecursive -Source $Source -Destination $cacheDir
    Write-Say "✅ Cached multi skills → $cacheDir ($count files)"
}

function Cache-MonoSkills {
    param([string]$Source)

    if (!(Test-Path $Source)) { return }

    $cacheDir = Join-Path $HOME ".dws\skills\mono"
    if (Test-Path $cacheDir) {
        Remove-Item -Path $cacheDir -Recurse -Force
    }
    New-Item -ItemType Directory -Path $cacheDir -Force | Out-Null
    Copy-DirRecursive -Source $Source -Destination $cacheDir | Out-Null
}

function Install-SkillsToHomes {
    param(
        [string]$SkillSrc,
        [string]$Root = $HOME
    )

    $installed = 0
    for ($i = 0; $i -lt $AgentDirs.Count; $i++) {
        $agentDir = $AgentDirs[$i]
        $baseDir = Join-Path $Root $agentDir
        $parentGate = Split-Path $baseDir -Parent
        if ($i -gt 0 -and !(Test-Path $parentGate)) {
            continue
        }
        $dest = Join-Path $baseDir $SkillName
        if ($Root -eq $HOME) {
            $label = "~\$agentDir\$SkillName"
        } else {
            $label = Join-Path $Root (Join-Path $agentDir $SkillName)
        }
        if ($installed -eq 0) {
            Copy-SkillToDir -SkillSrc $SkillSrc -Dest $dest -Label $label
        } else {
            Copy-SkillToDirSummary -SkillSrc $SkillSrc -Dest $dest -Label $label
        }
        $installed++
    }
    if ($installed -eq 0) {
        $fallback = Join-Path (Join-Path $Root ".agents\skills") $SkillName
        if ($Root -eq $HOME) {
            $flabel = "~\.agents\skills\$SkillName"
        } else {
            $flabel = Join-Path $Root (Join-Path ".agents\skills" $SkillName)
        }
        Copy-SkillToDir -SkillSrc $SkillSrc -Dest $fallback -Label $flabel
    }
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

        Install-SkillsToHomes -SkillSrc $skillSrc -Root $HOME

        # Cache the multi/ tree (and a mono copy) under ~/.dws/skills so that
        # subsequent `dws skill setup --mode multi|mono` can find a source.
        $multiRoot = Join-Path $extractRoot "multi"
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

if (!$NoSkills) {
    Resolve-SkillMode
}

if ($SourceRoot -and !$SkillsOnly -and ($Version -eq "latest")) {
    Install-BinaryFromSource -Root $SourceRoot
    if (!$NoSkills) {
        if ($SkillMode -eq "multi") {
            Write-MultiModeNotice
        } else {
            Install-SkillsLocal -Root $SourceRoot
        }
    }
} elseif ($SkillsOnly) {
    if ($SkillMode -eq "multi") {
        Write-MultiModeNotice
    } else {
        Install-Skills
    }
} elseif ($NoSkills) {
    Install-Binary
} else {
    Install-Binary
    if ($SkillMode -eq "multi") {
        Write-MultiModeNotice
    } else {
        Install-Skills
    }
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
