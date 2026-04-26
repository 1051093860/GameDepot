# GameDepot v0.1.1

GameDepot is a small Git + object-store asset collaboration prototype for game projects.

This build still uses a local directory as a fake object store:

```text
.gamedepot/remote_blobs/
```

## What v0.1.1 adds

- Rule-driven tracking prototype: files can be classified as `blob`, `git`, or `ignore` in `.gamedepot/config.yaml`.
- Safer path handling for `history`, `restore`, and `sync`.
- `sync --force` and default overwrite protection for local unsubmitted blob edits.
- `doctor` command for project/config/store checks.
- `verify` command for manifest/blob integrity checks.
- `ls` command for listing manifest entries.
- Unit/integration test skeletons for the current core.

## Commands

```bash
gamedepot init --project my-game
gamedepot doctor
gamedepot status
gamedepot status --json
gamedepot submit -m "update assets"
gamedepot sync
gamedepot sync --force
gamedepot verify
gamedepot ls
gamedepot ls --all
gamedepot history External/Planning/test.txt
gamedepot restore External/Planning/test.txt --sha256 <sha256>
gamedepot restore External/Planning/test.txt --sha256 <sha256> --force
```

## Default rule idea

The generated `.gamedepot/config.yaml` now contains a `rules:` section. The first matching rule wins.

Common default examples:

```yaml
rules:
  - pattern: Content/**/*.uasset
    mode: blob
    kind: unreal_asset
  - pattern: Content/**/*.umap
    mode: blob
    kind: unreal_map
  - pattern: External/Planning/**/*.xlsx
    mode: blob
    kind: planning_excel
  - pattern: External/Art/source/**
    mode: blob
    kind: art_source
  - pattern: External/SharedTools/**/*.zip
    mode: blob
    kind: tool_package
  - pattern: External/Tech/**/*.py
    mode: git
    kind: script
  - pattern: External/WebLinks/**/*.url
    mode: git
    kind: shortcut
```

## Build

```bash
go fmt ./...
go test ./...
go build -o gamedepot ./cmd/gamedepot
```

On Windows:

```powershell
go fmt ./...
go test ./...
go build -o gamedepot.exe .\cmd\gamedepot
```

## Quick test on Windows PowerShell

Assume the GameDepot source is at `C:\Dev\GameDepot`.

```powershell
cd C:\Users\YourName\Desktop
mkdir GDTest
cd GDTest

git init
git config user.email "test@example.com"
git config user.name "test"

C:\Dev\GameDepot\gamedepot.exe init --project test-game
C:\Dev\GameDepot\gamedepot.exe doctor

"hello gamedepot" | Out-File -Encoding utf8 External\Planning\test.txt
C:\Dev\GameDepot\gamedepot.exe status
C:\Dev\GameDepot\gamedepot.exe submit -m "add test file"
C:\Dev\GameDepot\gamedepot.exe verify
C:\Dev\GameDepot\gamedepot.exe ls

"dirty local edit" | Out-File -Encoding utf8 External\Planning\test.txt
C:\Dev\GameDepot\gamedepot.exe sync
# Expected: refusing to overwrite local unsubmitted change

C:\Dev\GameDepot\gamedepot.exe sync --force
Get-Content External\Planning\test.txt
# Expected: hello gamedepot
```

## Important limitation

This is still a local prototype. The object store provider is not yet real OSS/S3. The next version should add an S3-compatible provider for Aliyun OSS / Tencent COS / Huawei OBS / MinIO.
