#ifndef MyAppVersion
  #define MyAppVersion "0.1.0"
#endif

#ifndef SourceDir
  #define SourceDir "..\dist\payload"
#endif

#ifndef OutputDir
  #define OutputDir "..\dist\installer"
#endif

#define MyAppName "GameDepot"
#define MyAppExeName "gamedepot.exe"

[Setup]
AppId={{F2D1D4E2-1A2E-4E3A-9D6D-7A8E9C03B901}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher=GameDepot
DefaultDirName={localappdata}\GameDepot
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
OutputDir={#OutputDir}
OutputBaseFilename=GameDepotSetup_{#MyAppVersion}_x64
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
SetupLogging=yes
UninstallDisplayIcon={app}\{#MyAppExeName}
CloseApplications=yes
RestartApplications=no
UsePreviousAppDir=no
DisableDirPage=no
ChangesEnvironment=yes

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Files]
Source: "{#SourceDir}\gamedepot.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceDir}\plugins\*"; DestDir: "{app}\plugins"; Flags: ignoreversion recursesubdirs createallsubdirs

[InstallDelete]
Type: files; Name: "{app}\gamedepot.exe"
Type: filesandordirs; Name: "{app}\plugins"

[UninstallDelete]
Type: filesandordirs; Name: "{app}\plugins"
Type: files; Name: "{app}\install.json"

[Code]

var
  ExistingExePath: string;
  ExistingExeDir: string;
  ExistingExeCount: Integer;

function ExpandEnvVars(S: string): string;
var
  I, J: Integer;
  VarName, VarValue: string;
begin
  Result := '';
  I := 1;

  while I <= Length(S) do
  begin
    if S[I] = '%' then
    begin
      J := I + 1;

      while (J <= Length(S)) and (S[J] <> '%') do
        J := J + 1;

      if J <= Length(S) then
      begin
        VarName := Copy(S, I + 1, J - I - 1);
        VarValue := GetEnv(VarName);

        if VarValue <> '' then
          Result := Result + VarValue
        else
          Result := Result + Copy(S, I, J - I + 1);

        I := J + 1;
      end
      else
      begin
        Result := Result + S[I];
        I := I + 1;
      end;
    end
    else
    begin
      Result := Result + S[I];
      I := I + 1;
    end;
  end;
end;

function NormalizeDir(S: string): string;
begin
  S := Trim(RemoveQuotes(S));
  S := ExpandEnvVars(S);
  StringChangeEx(S, '/', '\', True);

  while (Length(S) > 3) and (S[Length(S)] = '\') do
    Delete(S, Length(S), 1);

  Result := LowerCase(S);
end;

function NextPathPart(var PathList: string): string;
var
  P: Integer;
begin
  P := Pos(';', PathList);

  if P = 0 then
  begin
    Result := Trim(PathList);
    PathList := '';
  end
  else
  begin
    Result := Trim(Copy(PathList, 1, P - 1));
    Delete(PathList, 1, P);
  end;
end;

function PathContainsDir(PathValue: string; Dir: string): Boolean;
var
  Part: string;
  Target: string;
begin
  Result := False;
  Target := NormalizeDir(Dir);

  while PathValue <> '' do
  begin
    Part := NextPathPart(PathValue);

    if NormalizeDir(Part) = Target then
    begin
      Result := True;
      Exit;
    end;
  end;
end;

function FindGameDepotInPath(): Boolean;
var
  PathValue: string;
  Part: string;
  Dir: string;
  Candidate: string;
begin
  Result := False;
  ExistingExePath := '';
  ExistingExeDir := '';
  ExistingExeCount := 0;

  PathValue := GetEnv('PATH');

  while PathValue <> '' do
  begin
    Part := NextPathPart(PathValue);
    Dir := Trim(RemoveQuotes(ExpandEnvVars(Part)));

    if Dir <> '' then
    begin
      Candidate := AddBackslash(Dir) + '{#MyAppExeName}';

      if FileExists(Candidate) then
      begin
        ExistingExeCount := ExistingExeCount + 1;

        if ExistingExePath = '' then
        begin
          ExistingExePath := Candidate;
          ExistingExeDir := ExtractFileDir(Candidate);
          Result := True;
        end;
      end;
    end;
  end;
end;

function JsonEscape(S: string): string;
begin
  StringChangeEx(S, '\', '\\', True);
  StringChangeEx(S, '"', '\"', True);
  Result := S;
end;

procedure EnsureUserPathContains(Dir: string);
var
  OldPath: string;
  NewPath: string;
begin
  if not RegQueryStringValue(HKEY_CURRENT_USER, 'Environment', 'Path', OldPath) then
    OldPath := '';

  if PathContainsDir(OldPath, Dir) then
    Exit;

  if OldPath = '' then
    NewPath := Dir
  else
    NewPath := Dir + ';' + OldPath;

  RegWriteStringValue(HKEY_CURRENT_USER, 'Environment', 'Path', NewPath);
end;

procedure RemoveUserPathDir(Dir: string);
var
  OldPath: string;
  NewPath: string;
  Part: string;
  Target: string;
begin
  if Dir = '' then
    Exit;

  if not RegQueryStringValue(HKEY_CURRENT_USER, 'Environment', 'Path', OldPath) then
    Exit;

  Target := NormalizeDir(Dir);
  NewPath := '';

  while OldPath <> '' do
  begin
    Part := NextPathPart(OldPath);

    if (Part <> '') and (NormalizeDir(Part) <> Target) then
    begin
      if NewPath = '' then
        NewPath := Part
      else
        NewPath := NewPath + ';' + Part;
    end;
  end;

  RegWriteStringValue(HKEY_CURRENT_USER, 'Environment', 'Path', NewPath);
end;

procedure WriteInstallManifest();
var
  AppDir: string;
  ManifestPath: string;
  Json: AnsiString;
begin
  AppDir := ExpandConstant('{app}');
  ManifestPath := AppDir + '\install.json';

  Json :=
    '{' + #13#10 +
    '  "app": "GameDepot",' + #13#10 +
    '  "version": "{#MyAppVersion}",' + #13#10 +
    '  "install_root": "' + JsonEscape(AppDir) + '",' + #13#10 +
    '  "exe_path": "' + JsonEscape(AppDir + '\{#MyAppExeName}') + '",' + #13#10 +
    '  "plugins_dir": "' + JsonEscape(AppDir + '\plugins') + '",' + #13#10 +
    '  "unreal_plugin_dir": "' + JsonEscape(AppDir + '\plugins\unreal\GameDepotUE') + '",' + #13#10 +
    '  "path_added": true,' + #13#10 +
    '  "layout": "portable-root-v1"' + #13#10 +
    '}';

  SaveStringToFile(ManifestPath, Json, False);
end;

procedure DeleteOldGameDepotFiles();
var
  OldExe: string;
  OldPluginsDir: string;
  NewAppDir: string;
begin
  if ExistingExeDir = '' then
    Exit;

  NewAppDir := ExpandConstant('{app}');

  if NormalizeDir(ExistingExeDir) = NormalizeDir(NewAppDir) then
  begin
    Log('Existing GameDepot path is same as new install dir. Skip old file deletion.');
    Exit;
  end;

  OldExe := AddBackslash(ExistingExeDir) + '{#MyAppExeName}';
  OldPluginsDir := AddBackslash(ExistingExeDir) + 'plugins';

  Log('Deleting old GameDepot files from: ' + ExistingExeDir);

  if DirExists(OldPluginsDir) then
  begin
    if not DelTree(OldPluginsDir, True, True, True) then
    begin
      MsgBox(
        '旧 plugins 目录删除失败：' + #13#10 +
        OldPluginsDir + #13#10#13#10 +
        '可能是 UE 编辑器、GameDepot daemon 或其他程序仍在使用。' + #13#10 +
        '你可以稍后手动删除这个目录。',
        mbError,
        MB_OK
      );
    end;
  end;

  if FileExists(OldExe) then
  begin
    if not DeleteFile(OldExe) then
    begin
      MsgBox(
        '旧 gamedepot.exe 删除失败：' + #13#10 +
        OldExe + #13#10#13#10 +
        '可能是 GameDepot daemon、UE 编辑器或命令行仍在运行。' + #13#10 +
        '你可以稍后手动删除这个文件。',
        mbError,
        MB_OK
      );
    end;
  end;

  RemoveUserPathDir(ExistingExeDir);
end;

function InitializeSetup(): Boolean;
var
  MessageText: string;
begin
  Result := True;

  if FindGameDepotInPath() then
  begin
    MessageText :=
      '发现 PATH 中已经存在 GameDepot，将视为旧安装：' + #13#10#13#10 +
      ExistingExePath + #13#10#13#10 +
      '本次安装将使用新目录：' + #13#10 +
      ExpandConstant('{localappdata}\GameDepot') + #13#10#13#10 +
      '安装成功后会清理：' + #13#10 +
      '- 旧 gamedepot.exe' + #13#10 +
      '- 旧 plugins 目录' + #13#10 +
      '- 用户 PATH 中的旧目录' + #13#10#13#10;

    if ExistingExeCount > 1 then
    begin
      MessageText :=
        MessageText +
        '注意：PATH 中发现了多个 gamedepot.exe。' + #13#10 +
        '安装器只会自动处理 PATH 优先级最高的那个。' + #13#10#13#10;
    end;

    MessageText := MessageText + '是否继续安装？';

    if MsgBox(MessageText, mbConfirmation, MB_YESNO) <> IDYES then
    begin
      Result := False;
      Exit;
    end;
  end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
var
  ExePath: string;
begin
  if CurStep = ssPostInstall then
  begin
    EnsureUserPathContains(ExpandConstant('{app}'));
    WriteInstallManifest();

    ExePath := ExpandConstant('{app}\{#MyAppExeName}');

    if not FileExists(ExePath) then
    begin
      MsgBox(
        'GameDepot 安装失败：未找到 gamedepot.exe。' + #13#10#13#10 +
        ExePath,
        mbError,
        MB_OK
      );
    end
    else
    begin
      DeleteOldGameDepotFiles();
    end;
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then
  begin
    RemoveUserPathDir(ExpandConstant('{app}'));
  end;
end;