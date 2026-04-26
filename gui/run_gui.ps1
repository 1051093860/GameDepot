param(
  [string]$GameDepotExe = "..\gamedepot.exe",
  [string]$ProjectRoot = ".",
  [string]$Addr = "127.0.0.1:17320",
  [string]$Token = ""
)
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$argsList = @("$ScriptDir\run_gui.py", "--gamedepot-exe", $GameDepotExe, "--project-root", $ProjectRoot, "--addr", $Addr)
if ($Token -ne "") { $argsList += @("--token", $Token) }
python @argsList
