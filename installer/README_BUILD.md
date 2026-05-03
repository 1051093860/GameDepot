# GameDepot 本地打包

一键构建内容：

- 检查 `go`，要求 Go 1.21+
- 检查 `uv`
- 构建 `dist/payload/gamedepot.exe`
- 构建 `dist/payload/gamedepot-saver.exe`，也就是 OSS 管理器
- 复制 `plugins/unreal/GameDepotUE`
- 使用 Inno Setup 生成安装程序

## 前置环境

Windows PowerShell 中需要能访问：

```powershell
go version
uv --version
ISCC.exe /?
```

如果没有 uv：

```powershell
powershell -ExecutionPolicy ByPass -c "irm https://astral.sh/uv/install.ps1 | iex"
```

如果没有 Inno Setup，请安装 Inno Setup 6，或构建时传入：

```powershell
-ISCCPath "C:\Program Files (x86)\Inno Setup 6\ISCC.exe"
```

## 一键完整构建

```powershell
Set-ExecutionPolicy -Scope Process Bypass -Force
.\installer\build-installer.ps1 -Clean
```

输出：

```text
dist\payload\gamedepot.exe
dist\payload\gamedepot-saver.exe
dist\installer\GameDepotSetup_<version>_x64.exe
```

## 只构建 payload，不生成安装器

```powershell
.\installer\build-installer.ps1 -Clean -SkipInstaller
```

## 只构建 OSS 管理器

```powershell
.\installer\build-gamedepot-saver.ps1 -Clean
```

## 打包调试版 OSS 管理器

如果双击 `gamedepot-saver.exe` 没反应，用控制台模式查看报错：

```powershell
.\installer\build-gamedepot-saver.ps1 -Clean -Console
.\dist\payload\gamedepot-saver.exe .
```
