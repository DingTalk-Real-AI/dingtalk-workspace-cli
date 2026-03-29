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

$ErrorActionPreference = "Stop"

$Repo = "DingTalk-Real-AI/dingtalk-workspace-cli"
$BinName = "dws"
$InstallDir = if ($env:DWS_INSTALL_DIR) { $env:DWS_INSTALL_DIR } else { Join-Path $HOME ".local\bin" }
$Version = if ($env:DWS_VERSION) { $env:DWS_VERSION } else { "latest" }
$NoSkills = $env:DWS_NO_SKILLS -eq "1"
$SkillsOnly = $env:DWS_SKILLS_ONLY -eq "1"
$SkillName = "dws"

# Agent directory to install skills into — most agents can fall back to .agents\skills
$AgentDir = ".agents\skills"

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

function Resolve-LatestVersion {
    if ($Version -eq "latest") {
        try {
            $response = Invoke-WebRequest -Uri "https://github.com/$Repo/releases/latest" `
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
            $response = Invoke-WebRequest -Uri "https://github.com/$Repo/releases/latest" `
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

# ── Install Binary ───────────────────────────────────────────────────────────

function Install-Binary {
    $arch = Get-Arch
    Resolve-LatestVersion

    $archiveName = "${BinName}-windows-${arch}.zip"
    $downloadUrl = "https://github.com/$Repo/releases/download/$Version/$archiveName"

    Write-Say "⬇  Downloading $BinName $Version (windows/$arch)..."

    $tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) "dws-install-$PID"
    New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

    try {
        $archivePath = Join-Path $tmpDir $archiveName
        Invoke-WebRequest -Uri $downloadUrl -OutFile $archivePath -UseBasicParsing

        # Download and verify SHA256 checksum
        $checksumUrl = "https://github.com/$Repo/releases/download/$Version/checksums.txt"
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
    $skillSrc = Join-Path $Root "skills"

    if (!(Test-Path $skillSrc)) {
        Write-Say "⚠️  Local skills directory not found: $skillSrc"
        Write-Say "   Skipping skills installation."
        return
    }

    Write-Say ""
    Write-Say "📦 Installing agent skills from local source: $skillSrc"

    $dest = Join-Path (Join-Path $HOME $AgentDir) $SkillName
    $label = "~\$AgentDir\$SkillName"
    Copy-SkillToDir -SkillSrc $skillSrc -Dest $dest -Label $label
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

    $zipUrl = "https://github.com/$Repo/releases/download/$Version/dws-skills.zip"

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

        $skillSrc = $extractRoot
        if (Test-Path (Join-Path $extractRoot "$SkillName\SKILL.md")) {
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

        $dest = Join-Path (Join-Path $HOME $AgentDir) $SkillName
        $label = "~\$AgentDir\$SkillName"
        Copy-SkillToDir -SkillSrc $skillSrc -Dest $dest -Label $label
    } finally {
        Remove-Item -Path $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# ── Main ─────────────────────────────────────────────────────────────────────

$SourceRoot = Resolve-SourceRoot

Write-Banner

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
