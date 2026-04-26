# GameDepot v0.2

GameDepot is a small-team game project collaboration tool prototype.

Current design:

- Git stores code, config, docs, shortcuts, launchers, and the manifest.
- The blob store stores large binary files by `sha256`.
- `depot/manifests/main.gdmanifest.json` bridges Git history and blob objects.
- Rules in `.gamedepot/config.yaml` decide whether each file is `blob`, `git`, or `ignore`.

v0.2 is still using a local fake object store:

```text
.gamedepot/remote_blobs/
```

The next major step is replacing the local store with OSS / COS / OBS / MinIO through an S3-compatible provider.

---

## Build

```powershell
cd GameDepot
go fmt ./...
go test ./...
go build -o gamedepot.exe .\cmd\gamedepot
```

Linux / macOS:

```bash
cd GameDepot
go fmt ./...
go test ./...
go build -o gamedepot ./cmd/gamedepot
```

---

## Quick test

PowerShell example:

```powershell
cd C:\Users\meloda\Desktop
mkdir GDTest_v02
cd GDTest_v02

git init
git config user.email "test@example.com"
git config user.name "GameDepot Test"

C:\Dev\GameDepot\gamedepot.exe init --project PartyGame --template ue5
C:\Dev\GameDepot\gamedepot.exe doctor
```

Create several test files:

```powershell
New-Item -ItemType Directory -Force Content\Maps | Out-Null
New-Item -ItemType Directory -Force Config | Out-Null
New-Item -ItemType Directory -Force Source\PartyGame | Out-Null
New-Item -ItemType Directory -Force External\Planning | Out-Null
New-Item -ItemType Directory -Force Saved\Logs | Out-Null

"fake map binary" | Out-File -Encoding utf8 Content\Maps\Main.umap
"[/Script/EngineSettings.GameMapsSettings]" | Out-File -Encoding utf8 Config\DefaultGame.ini
"// fake cpp" | Out-File -Encoding utf8 Source\PartyGame\GameMode.cpp
"balance table" | Out-File -Encoding utf8 External\Planning\balance.txt
"runtime log" | Out-File -Encoding utf8 Saved\Logs\Game.log
```

Check classification:

```powershell
C:\Dev\GameDepot\gamedepot.exe classify
C:\Dev\GameDepot\gamedepot.exe classify Content\Maps\Main.umap
C:\Dev\GameDepot\gamedepot.exe classify --json
```

Expected categories:

```text
blob    Content/Maps/Main.umap
git     Config/DefaultGame.ini
git     Source/PartyGame/GameMode.cpp
blob    External/Planning/balance.txt
ignore  Saved/Logs/Game.log
```

Submit:

```powershell
C:\Dev\GameDepot\gamedepot.exe status
C:\Dev\GameDepot\gamedepot.exe submit -m "initial GameDepot v0.2 test"
C:\Dev\GameDepot\gamedepot.exe verify
C:\Dev\GameDepot\gamedepot.exe ls
```

Test sync overwrite protection:

```powershell
"dirty local edit" | Out-File -Encoding utf8 Content\Maps\Main.umap

C:\Dev\GameDepot\gamedepot.exe sync
```

Expected: it refuses to overwrite the local unsubmitted blob change.

Force restore:

```powershell
C:\Dev\GameDepot\gamedepot.exe sync --force
Get-Content Content\Maps\Main.umap
```

---

## Commands

```text
gamedepot init --project my-game [--template ue5]
gamedepot doctor
gamedepot classify [path] [--json] [--all]
gamedepot status [--json]
gamedepot submit -m "update assets"
gamedepot sync [--force]
gamedepot verify
gamedepot ls [--all]
gamedepot history <path>
gamedepot restore <path> --sha256 <sha256> [--force]
```

---

## Rule system

`.gamedepot/config.yaml` contains rules like this:

```yaml
rules:
  - pattern: Content/**/*.uasset
    mode: blob
    kind: unreal_asset

  - pattern: Content/**/*.umap
    mode: blob
    kind: unreal_map

  - pattern: Config/**
    mode: git
    kind: unreal_config

  - pattern: Source/**
    mode: git
    kind: code

  - pattern: Saved/**
    mode: ignore
    kind: unreal_generated
```

The first matching rule wins.

Modes:

```text
blob    hash + upload to blob store + write manifest
git     stage directly into Git
ignore  skipped by GameDepot
```

---

## Default UE5 behavior

Blob-managed:

```text
Content/**/*.uasset
Content/**/*.umap
External/Planning/**/*.xlsx
External/Planning/**/*.xls
External/Planning/**/*.csv
External/Planning/**/*.txt
External/Art/source/**
External/Art/**/*.psd
External/Art/**/*.blend
External/Art/**/*.fbx
External/Art/**/*.png
External/SharedTools/**/*.zip
External/SharedTools/**/*.7z
External/SharedTools/**/*.exe
```

Git-managed:

```text
*.uproject
Config/**
Source/**
Plugins/**/*.uplugin
Plugins/**/Source/**
Docs/**
External/WebLinks/**/*.url
External/Tech/**/*.py
External/Tech/**/*.ps1
External/Tech/**/*.bat
External/Launchers/**/*.bat
External/Launchers/**/*.ps1
External/**/*.md
External/**/*.url
```

Ignored:

```text
.git/**
.gamedepot/cache/**
.gamedepot/tmp/**
.gamedepot/logs/**
.gamedepot/remote_blobs/**
Binaries/**
Build/**
DerivedDataCache/**
Intermediate/**
Saved/**
.vs/**
```

---

## Verify checks

`gamedepot verify` now checks:

- manifest readability
- unsafe paths
- missing blob objects
- blob hash mismatch
- local blob mismatch against manifest
- manifest entries that no longer match blob rules
- blob-managed files accidentally tracked by Git
- ignored files tracked by Git
- git-managed files not yet tracked

If a blob-managed file is already tracked by Git, fix it with:

```bash
git rm --cached -- Content/Maps/Main.umap
gamedepot submit -m "move Main.umap to GameDepot blob store"
```

---

## Suggested next version

v0.3 should add a real object-store provider:

```text
internal/store/s3_store.go
internal/store/provider.go
```

Target configuration:

```yaml
store:
  type: s3
  endpoint: oss-cn-hangzhou.aliyuncs.com
  bucket: gamedepot-test
  region: cn-hangzhou
  prefix: projects/party-game/blobs
```

Secrets should come from environment variables, not config files.
