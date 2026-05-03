param(
    [string]$Version = "1.1.0",

    [string]$RepoRoot = "",

    [string]$ISCCPath = "",

    [switch]$SkipGoBuild,

    [switch]$SkipSaverBuild,

    [switch]$SkipInstaller,

    [switch]$ConsoleSaver,

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

function Require-Command {
    param(
        [string]$Name,
        [string]$InstallHint
    )

    $cmd = Get-Command $Name -ErrorAction SilentlyContinue
    if (!$cmd) {
        throw "鎵句笉鍒?$Name銆?InstallHint"
    }
    return $cmd.Source
}

function Assert-GoMinVersion {
    param([string]$MinVersion = "1.21")

    $goVersionText = (& go version)
    if ($LASTEXITCODE -ne 0) {
        throw "go version failed."
    }

    Write-Host "Go:  $goVersionText"

    if ($goVersionText -notmatch "go([0-9]+)\.([0-9]+)") {
        Write-Warning "Unable to parse Go version; continuing build."
        return
    }

    $major = [int]$Matches[1]
    $minor = [int]$Matches[2]

    if (($major -lt 1) -or (($major -eq 1) -and ($minor -lt 21))) {
        throw "Go version is too low: $goVersionText. Please install Go 1.21 or newer."
    }
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

    throw "ISCC.exe not found. Install Inno Setup 6, or pass -ISCCPath to specify ISCC.exe."
}

function Copy-DirectoryClean {
    param(
        [string]$Source,
        [string]$Destination
    )

    if (!(Test-Path $Source)) {
        throw "婧愮洰褰曚笉瀛樺湪锛?Source"
    }

    if (Test-Path $Destination) {
        Remove-Item -Recurse -Force $Destination
    }

    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Destination) | Out-Null
    Copy-Item -Recurse -Force $Source $Destination
}

function Invoke-External {
    param(
        [string]$DisplayName,
        [scriptblock]$Command
    )

    Write-Host ""
    Write-Host "== $DisplayName =="
    & $Command
    if ($LASTEXITCODE -ne 0) {
        throw "$DisplayName failed with exit code $LASTEXITCODE"
    }
}

if ([string]::IsNullOrWhiteSpace($RepoRoot)) {
    $ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
    $RepoRoot = Split-Path -Parent $ScriptDir
}

$RepoRoot = Resolve-FullPath $RepoRoot

$DistDir = Join-Path $RepoRoot "dist"
$PayloadDir = Join-Path $DistDir "payload"
$InstallerOutDir = Join-Path $DistDir "installer"
$BuildDir = Join-Path $DistDir "build"

$GameDepotExeOut = Join-Path $PayloadDir "gamedepot.exe"
$SaverExeOut = Join-Path $PayloadDir "gamedepot-saver.exe"
$PluginsOutDir = Join-Path $PayloadDir "plugins"

$InnoScript = Join-Path $RepoRoot "installer\GameDepot.iss"
$GameDepotUEPluginDir = Join-Path $RepoRoot "plugins\unreal\GameDepotUE"
$GameDepotUEPayloadDir = Join-Path $PluginsOutDir "unreal\GameDepotUE"
$SaverProjectDir = Join-Path $RepoRoot "oss_manager_pyqt"

if (!(Test-Path $InnoScript)) {
    throw "鎵句笉鍒?Inno Setup 鑴氭湰锛?InnoScript"
}

if (!(Test-Path $GameDepotUEPluginDir)) {
    throw "鎵句笉鍒?GameDepotUE 鎻掍欢鐩綍锛?GameDepotUEPluginDir"
}

$UPluginFile = Join-Path $GameDepotUEPluginDir "GameDepotUE.uplugin"
if (!(Test-Path $UPluginFile)) {
    throw "鎵句笉鍒?UE 鎻掍欢鎻忚堪鏂囦欢锛?UPluginFile"
}

if (!(Test-Path $SaverProjectDir)) {
    throw "鎵句笉鍒?OSS 绠＄悊鍣ㄧ洰褰曪細$SaverProjectDir"
}

Write-Host "== GameDepot Full Installer Build =="
Write-Host "RepoRoot:             $RepoRoot"
Write-Host "Version:              $Version"
Write-Host "PayloadDir:           $PayloadDir"
Write-Host "InstallerOutDir:      $InstallerOutDir"
Write-Host "UE Plugin Source:     $GameDepotUEPluginDir"
Write-Host "UE Plugin Payload:    $GameDepotUEPayloadDir"
Write-Host "OSS Manager Source:   $SaverProjectDir"
Write-Host ""

if ($Clean -and (Test-Path $DistDir)) {
    Write-Host "Cleaning dist..."
    Remove-Item -Recurse -Force $DistDir
}

New-Item -ItemType Directory -Force -Path $PayloadDir | Out-Null
New-Item -ItemType Directory -Force -Path $InstallerOutDir | Out-Null
New-Item -ItemType Directory -Force -Path $BuildDir | Out-Null

Write-Host "Checking toolchain..."
if (!$SkipGoBuild) {
    Require-Command "go" "Please install Go 1.21+ and ensure go.exe is in PATH." | Out-Null
    Assert-GoMinVersion "1.21"
}
else {
    Write-Host "Go:  skipped by -SkipGoBuild"
}

if (!$SkipSaverBuild) {
    Require-Command "uv" 'Please install uv: powershell -ExecutionPolicy ByPass -c "irm https://astral.sh/uv/install.ps1 | iex"' | Out-Null
    $uvVersionText = (& uv --version)
    if ($LASTEXITCODE -ne 0) {
        throw "uv --version failed."
    }
    Write-Host "uv:  $uvVersionText"
}
else {
    Write-Host "uv:  skipped by -SkipSaverBuild"
}

if (!$SkipInstaller) {
    $ISCC = Find-ISCC -ExplicitPath $ISCCPath
    Write-Host "ISCC: $ISCC"
}
else {
    Write-Host "ISCC: skipped by -SkipInstaller"
}

if (!$SkipGoBuild) {
    if (Test-Path $GameDepotExeOut) {
        Remove-Item -Force $GameDepotExeOut
    }

    $oldGOOS = $env:GOOS
    $oldGOARCH = $env:GOARCH
    $oldCGO = $env:CGO_ENABLED

    try {
        $env:GOOS = "windows"
        $env:GOARCH = "amd64"
        $env:CGO_ENABLED = "0"

        Push-Location $RepoRoot
        Invoke-External "Building gamedepot.exe" {
            & go build `
                -trimpath `
                -ldflags="-s -w" `
                -o $GameDepotExeOut `
                ./cmd/gamedepot
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
        $env:CGO_ENABLED = $oldCGO
    }
}
else {
    Write-Host "SkipGoBuild enabled. Using existing exe: $GameDepotExeOut"
}

if (!(Test-Path $GameDepotExeOut)) {
    throw "gamedepot.exe 鏋勫缓澶辫触鎴栦笉瀛樺湪锛?GameDepotExeOut"
}

if (!$SkipSaverBuild) {
    if (Test-Path $SaverExeOut) {
        Remove-Item -Force $SaverExeOut
    }

    $PyInstallerWork = Join-Path $BuildDir "pyinstaller-work"
    $PyInstallerSpec = Join-Path $BuildDir "pyinstaller-spec"
    if (Test-Path $PyInstallerWork) { Remove-Item -Recurse -Force $PyInstallerWork }
    if (Test-Path $PyInstallerSpec) { Remove-Item -Recurse -Force $PyInstallerSpec }
    New-Item -ItemType Directory -Force -Path $PyInstallerWork | Out-Null
    New-Item -ItemType Directory -Force -Path $PyInstallerSpec | Out-Null

    $WindowMode = if ($ConsoleSaver) { "--console" } else { "--windowed" }

    Push-Location $SaverProjectDir
    try {
        Invoke-External "uv sync for gamedepot-saver" {
            & uv sync --all-extras
        }

        $PyInstallerArgs = @(
            "run",
            "--with", "pyinstaller",
            "python", "-m", "PyInstaller",
            "--noconfirm",
            "--clean",
            "--onefile",
            $WindowMode,
            "--name", "gamedepot-saver",
            "--distpath", $PayloadDir,
            "--workpath", $PyInstallerWork,
            "--specpath", $PyInstallerSpec,
            "--collect-submodules", "boto3",
            "--collect-submodules", "botocore",
            "--collect-submodules", "s3transfer",
            "--collect-submodules", "oss2",
            "oss_manager\__main__.py"
        )

        Invoke-External "Building gamedepot-saver.exe" {
            & uv @PyInstallerArgs
        }
    }
    catch {
        throw
    }
    finally {
        Pop-Location
    }
}
else {
    Write-Host "SkipSaverBuild enabled. Using existing exe: $SaverExeOut"
}

if (!(Test-Path $SaverExeOut)) {
    throw "gamedepot-saver.exe 鏋勫缓澶辫触鎴栦笉瀛樺湪锛?SaverExeOut"
}

Write-Host ""
Write-Host "== Copying plugins =="
if (Test-Path $PluginsOutDir) {
    Remove-Item -Recurse -Force $PluginsOutDir
}
New-Item -ItemType Directory -Force -Path $PluginsOutDir | Out-Null
Copy-DirectoryClean -Source $GameDepotUEPluginDir -Destination $GameDepotUEPayloadDir

Write-Host ""
Write-Host "Payload prepared:"
Write-Host "  $GameDepotExeOut"
Write-Host "  $SaverExeOut"
Write-Host "  $GameDepotUEPayloadDir"

if ($SkipInstaller) {
    Write-Host ""
    Write-Host "SkipInstaller enabled. Payload build complete."
    return
}

Write-Host ""
Write-Host "== Building Inno Setup installer =="
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
Write-Host "Installed commands after setup:"
Write-Host "  gamedepot version"
Write-Host "  gamedepot-saver ."
