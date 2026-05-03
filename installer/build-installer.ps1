param(
    [string]$Version = "1.1.0",

    [string]$RepoRoot = "",

    [string]$ISCCPath = "",

    [switch]$SkipGoBuild,

    [switch]$Clean
)

$ErrorActionPreference = "Stop"

function Resolve-FullPath {
    param([string]$Path)

    if ([System.IO.Path]::IsPathRooted($Path)) {
        return [System.IO.Path]::GetFullPath($Path)
    }

    return [System.IO.Path]::GetFullPath((Join-Path (Get-Location) $Path))
}

function Find-ISCC {
    param([string]$ExplicitPath)

    if ($ExplicitPath -and (Test-Path $ExplicitPath)) {
        return (Resolve-FullPath $ExplicitPath)
    }

    $cmd = Get-Command "ISCC.exe" -ErrorAction SilentlyContinue
    if ($cmd) {
        return $cmd.Source
    }

    $candidates = @(
        "C:\Program Files (x86)\Inno Setup 6\ISCC.exe",
        "C:\Program Files\Inno Setup 6\ISCC.exe"
    )

    foreach ($candidate in $candidates) {
        if (Test-Path $candidate) {
            return $candidate
        }
    }

    throw "找不到 ISCC.exe。请安装 Inno Setup 6，或者使用 -ISCCPath 指定 ISCC.exe。"
}

function Copy-DirectoryClean {
    param(
        [string]$Source,
        [string]$Destination
    )

    if (!(Test-Path $Source)) {
        throw "源目录不存在：$Source"
    }

    if (Test-Path $Destination) {
        Remove-Item -Recurse -Force $Destination
    }

    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Destination) | Out-Null
    Copy-Item -Recurse -Force $Source $Destination
}

if ([string]::IsNullOrWhiteSpace($RepoRoot)) {
    $ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
    $RepoRoot = Split-Path -Parent $ScriptDir
}

$RepoRoot = Resolve-FullPath $RepoRoot

$DistDir = Join-Path $RepoRoot "dist"
$PayloadDir = Join-Path $DistDir "payload"
$InstallerOutDir = Join-Path $DistDir "installer"

$GameDepotExeOut = Join-Path $PayloadDir "gamedepot.exe"
$PluginsOutDir = Join-Path $PayloadDir "plugins"

$InnoScript = Join-Path $RepoRoot "installer\GameDepot.iss"

# 仓库内插件目录：
# plugins/unreal/GameDepotUE
$GameDepotUEPluginDir = Join-Path $RepoRoot "plugins\unreal\GameDepotUE"

# payload 内插件目录：
# dist/payload/plugins/unreal/GameDepotUE
$GameDepotUEPayloadDir = Join-Path $PluginsOutDir "unreal\GameDepotUE"

if (!(Test-Path $InnoScript)) {
    throw "找不到 Inno Setup 脚本：$InnoScript"
}

if (!(Test-Path $GameDepotUEPluginDir)) {
    throw "找不到 GameDepotUE 插件目录：$GameDepotUEPluginDir"
}

$UPluginFile = Join-Path $GameDepotUEPluginDir "GameDepotUE.uplugin"
if (!(Test-Path $UPluginFile)) {
    throw "找不到 UE 插件描述文件：$UPluginFile"
}

Write-Host "== GameDepot Installer Build =="
Write-Host "RepoRoot:             $RepoRoot"
Write-Host "Version:              $Version"
Write-Host "PayloadDir:           $PayloadDir"
Write-Host "UE Plugin Source:     $GameDepotUEPluginDir"
Write-Host "UE Plugin Payload:    $GameDepotUEPayloadDir"
Write-Host ""

if ($Clean -and (Test-Path $DistDir)) {
    Write-Host "Cleaning dist..."
    Remove-Item -Recurse -Force $DistDir
}

New-Item -ItemType Directory -Force -Path $PayloadDir | Out-Null
New-Item -ItemType Directory -Force -Path $InstallerOutDir | Out-Null

if (!$SkipGoBuild) {
    Write-Host "Building gamedepot.exe..."

    $oldGOOS = $env:GOOS
    $oldGOARCH = $env:GOARCH

    try {
        $env:GOOS = "windows"
        $env:GOARCH = "amd64"

        Push-Location $RepoRoot

        & go build `
            -trimpath `
            -ldflags="-s -w" `
            -o $GameDepotExeOut `
            ./cmd/gamedepot

        if ($LASTEXITCODE -ne 0) {
            throw "go build failed with exit code $LASTEXITCODE"
        }

        Pop-Location
    }
    catch {
        try { Pop-Location } catch {}
        throw
    }
    finally {
        $env:GOOS = $oldGOOS
        $env:GOARCH = $oldGOARCH
    }
}
else {
    Write-Host "SkipGoBuild enabled. Using existing exe: $GameDepotExeOut"

    if (!(Test-Path $GameDepotExeOut)) {
        throw "SkipGoBuild 已开启，但不存在：$GameDepotExeOut"
    }
}

if (!(Test-Path $GameDepotExeOut)) {
    throw "gamedepot.exe 构建失败或不存在：$GameDepotExeOut"
}

Write-Host "Copying plugins..."

if (Test-Path $PluginsOutDir) {
    Remove-Item -Recurse -Force $PluginsOutDir
}

New-Item -ItemType Directory -Force -Path $PluginsOutDir | Out-Null

Copy-DirectoryClean `
    -Source $GameDepotUEPluginDir `
    -Destination $GameDepotUEPayloadDir

Write-Host "Payload prepared:"
Write-Host "  $GameDepotExeOut"
Write-Host "  $GameDepotUEPayloadDir"
Write-Host ""

$ISCC = Find-ISCC -ExplicitPath $ISCCPath

Write-Host "Using ISCC:"
Write-Host "  $ISCC"
Write-Host ""

Write-Host "Building Inno Setup installer..."

& $ISCC `
    "/DMyAppVersion=$Version" `
    "/DSourceDir=$PayloadDir" `
    "/DOutputDir=$InstallerOutDir" `
    "$InnoScript"

if ($LASTEXITCODE -ne 0) {
    throw "ISCC failed with exit code $LASTEXITCODE"
}

$ExpectedInstaller = Join-Path $InstallerOutDir "GameDepotSetup_${Version}_x64.exe"

Write-Host ""
Write-Host "Installer build complete."

if (Test-Path $ExpectedInstaller) {
    Write-Host "Output:"
    Write-Host "  $ExpectedInstaller"
}
else {
    Write-Host "Output directory:"
    Write-Host "  $InstallerOutDir"
}

Write-Host ""
Write-Host "Next:"
Write-Host "  1. Run installer exe"
Write-Host "  2. Reopen PowerShell"
Write-Host "  3. Test: gamedepot version"