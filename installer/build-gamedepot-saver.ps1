param(
    [string]$RepoRoot = "",
    [switch]$Console,
    [switch]$Clean
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($RepoRoot)) {
    $ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
    $RepoRoot = Split-Path -Parent $ScriptDir
}

$Args = @("-RepoRoot", $RepoRoot, "-SkipGoBuild", "-SkipInstaller")
if ($Console) { $Args += "-ConsoleSaver" }
if ($Clean) { $Args += "-Clean" }

& (Join-Path $RepoRoot "installer\build-installer.ps1") @Args
