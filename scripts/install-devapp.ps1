# Copyright 2026 Alibaba Group
# Licensed under the Apache License, Version 2.0
#
# One-command installer for the dws Dev preview on native Windows (PowerShell).
# Downloads the dev binary (dws.exe) + dingtalk-dev skill from the fork's GitHub Releases.
#
# Usage:
#   irm https://raw.githubusercontent.com/wxianfeng/dingtalk-workspace-cli/feat/dws-devapp/scripts/install-devapp.ps1 | iex
#
# Env (all optional):
#   DEVAPP_REPO      fork holding dev releases (default: wxianfeng/dingtalk-workspace-cli)
#   DEVAPP_VERSION   pin a dev release tag (default: latest release on the fork)
#   DWS_ARCH         architecture override (amd64 or arm64)
#   DWS_INSTALL_DIR  binary dir (default: ~/.local/bin)
#   DWS_NO_SKILLS    set 1 to skip the dev skill

$ErrorActionPreference = "Stop"

$Repo       = if ($env:DEVAPP_REPO) { $env:DEVAPP_REPO } else { "wxianfeng/dingtalk-workspace-cli" }
$Version    = $env:DEVAPP_VERSION
$InstallDir = if ($env:DWS_INSTALL_DIR) { $env:DWS_INSTALL_DIR } else { Join-Path $HOME ".local\bin" }
$NoSkills   = $env:DWS_NO_SKILLS -eq "1"
$SkillName  = "dingtalk-dev"

function Say($m) { Write-Host "  $m" }
function Die($m) { Write-Host "  X $m" -ForegroundColor Red; exit 1 }

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

# GitHub's /releases/latest excludes prereleases; read the releases list (newest
# first) and take the top tag — the dev preview is published as a prerelease.
if (-not $Version) {
    try {
        $rel = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases?per_page=1" `
            -Headers @{ "User-Agent" = "dws-devapp-installer" } -UseBasicParsing
        $Version = $rel[0].tag_name
    } catch {}
    if (-not $Version) { Die "No release found on $Repo. Push a dev tag (e.g. v1.0.39-dev.1) to trigger CI, or set DEVAPP_VERSION." }
}

$arch = Get-Arch
$tmp  = Join-Path $env:TEMP ("dws-dev-" + [System.Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $tmp -Force | Out-Null

Write-Host ""
Say "dws Dev preview installer (Windows, pre-built binary)"
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
            if (Test-Path $cache) { Remove-Item -Recurse -Force $cache }
            New-Item -ItemType Directory -Path $cache -Force | Out-Null
            Copy-Item -Path "$src\*" -Destination $cache -Recurse -Force

            $agentDirs = @(
                ".agents\skills", ".claude\skills", ".cursor\skills", ".qoder\skills", ".qoderwork\skills",
                ".gemini\skills", ".codex\skills", ".github\skills", ".windsurf\skills", ".augment\skills",
                ".cline\skills", ".amp\skills", ".kiro\skills", ".trae\skills", ".openclaw\skills",
                ".hermes\skills", ".config\opencode\skills"
            )
            $installed = 0; $idx = 0
            foreach ($d in $agentDirs) {
                $base   = Join-Path $HOME $d
                $parent = Split-Path $base -Parent
                if ($idx -gt 0 -and -not (Test-Path $parent)) { $idx++; continue }
                $dest = Join-Path $base $SkillName
                if (Test-Path $dest) { Remove-Item -Recurse -Force $dest }
                New-Item -ItemType Directory -Path $dest -Force | Out-Null
                Copy-Item -Path "$src\*" -Destination $dest -Recurse -Force
                $installed++; $idx++
            }
            Say "Skill dingtalk-dev -> $installed agent dir(s)"
        } else {
            Say "(dingtalk-dev not found in skills bundle; skipped)"
        }
    } catch {
        Say "(skill install skipped: $($_.Exception.Message))"
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
