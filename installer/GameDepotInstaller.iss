; GameDepot Windows Installer (Inno Setup)
; Build with: iscc installer\GameDepotInstaller.iss

#define MyAppName "GameDepot"
#define MyAppVersion "0.10.0"
#define MyAppPublisher "GameDepot"
#define MyAppExeName "gamedepot.exe"
#define SourceRoot "..\\dist\\gamedepot"

[Setup]
AppId={{A8A7BB7E-6634-4DD2-8E74-7BDBFA62E51D}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
DefaultDirName={autopf}\\{#MyAppName}
DefaultGroupName={#MyAppName}
DisableProgramGroupPage=yes
OutputDir=..\\dist\\installer
OutputBaseFilename=GameDepot-Setup-{#MyAppVersion}
Compression=lzma
SolidCompression=yes
WizardStyle=modern
ArchitecturesInstallIn64BitMode=x64compatible
PrivilegesRequired=admin

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Files]
Source: "{#SourceRoot}\\{#MyAppExeName}"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceRoot}\\plugins\\*"; DestDir: "{app}\\plugins"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{autoprograms}\\{#MyAppName}"; Filename: "{app}\\{#MyAppExeName}"
Name: "{autodesktop}\\{#MyAppName}"; Filename: "{app}\\{#MyAppExeName}"; Tasks: desktopicon

[Tasks]
Name: "desktopicon"; Description: "Create a desktop shortcut"; GroupDescription: "Additional icons:"; Flags: unchecked

[Run]
Filename: "{app}\\{#MyAppExeName}"; Description: "Launch {#MyAppName}"; Flags: nowait postinstall skipifsilent

[Code]
function AddDirToUserPath(Path: string): Boolean;
var
  OrigPath: string;
  NewPath: string;
begin
  Result := False;
  if not RegQueryStringValue(HKCU, 'Environment', 'Path', OrigPath) then
    OrigPath := '';

  if Pos(';' + Lowercase(Path) + ';', ';' + Lowercase(OrigPath) + ';') > 0 then
  begin
    Result := True;
    exit;
  end;

  if OrigPath = '' then
    NewPath := Path
  else
    NewPath := OrigPath + ';' + Path;

  Result := RegWriteStringValue(HKCU, 'Environment', 'Path', NewPath);
end;

function RemoveDirFromUserPath(Path: string): Boolean;
var
  OrigPath: string;
  Parts: TArrayOfString;
  i: Integer;
  NewPath: string;
begin
  Result := False;
  if not RegQueryStringValue(HKCU, 'Environment', 'Path', OrigPath) then
  begin
    Result := True;
    exit;
  end;

  NewPath := '';
  ExtractStrings([';'], [], OrigPath, Parts);
  for i := 0 to GetArrayLength(Parts) - 1 do
  begin
    if Lowercase(Trim(Parts[i])) <> Lowercase(Path) then
    begin
      if NewPath = '' then
        NewPath := Trim(Parts[i])
      else
        NewPath := NewPath + ';' + Trim(Parts[i]);
    end;
  end;

  Result := RegWriteStringValue(HKCU, 'Environment', 'Path', NewPath);
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
  begin
    if not AddDirToUserPath(ExpandConstant('{app}')) then
      MsgBox('Failed to update user PATH. Please add "' + ExpandConstant('{app}') + '" manually.', mbError, MB_OK);
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usUninstall then
    RemoveDirFromUserPath(ExpandConstant('{app}'));
end;
