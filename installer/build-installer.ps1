param(
  [string]$Version = '0.10.0',
  [string]$InnoSetupCompiler = 'C:\Program Files (x86)\Inno Setup 6\ISCC.exe'
)

$root = Split-Path -Parent $PSScriptRoot
$distRoot = Join-Path $root 'dist\gamedepot'
$exe = Join-Path $distRoot 'gamedepot.exe'
$plugins = Join-Path $distRoot 'plugins'
$iss = Join-Path $PSScriptRoot 'GameDepotInstaller.iss'

if (-not (Test-Path $InnoSetupCompiler)) {
  throw "ISCC not found: $InnoSetupCompiler"
}
if (-not (Test-Path $exe)) {
  throw "Missing: $exe"
}
if (-not (Test-Path $plugins)) {
  throw "Missing: $plugins"
}

& $InnoSetupCompiler "/DMyAppVersion=$Version" $iss
if ($LASTEXITCODE -ne 0) {
  throw "Inno Setup compile failed with exit code $LASTEXITCODE"
}

Write-Host "Installer built at: $(Join-Path $root 'dist\installer')"
