# GameDepot v0.6-cmd-core

This version intentionally pauses the UE plugin line and returns to a stable command-line-first Core.
The goal is to validate the core workflow before reintroducing daemon/tasks and UE UI.

## What changed

- All Git operations use `git -C <ProjectRoot> ...` instead of relying on the process working directory.
- Project config now has a managed `git:` section with remote/upstream/branch settings.
- New `git-config` command for remote/upstream setup.
- `sync` now supports the team workflow: Git fetch/pull first, then blob sync.
- `submit` supports `--push` to push Git commits after uploading blobs and committing manifest changes.
- New `asset-status` command with recoverability states.
- New `repair-current-blob` command for re-uploading a missing current blob when the local file still matches the manifest SHA.
- New `gc-impact` command to preview whether GC candidates would break current versions or historical restore.

## Build

```powershell
cd .\GameDepot

go test ./...
go build -o gamedepot.exe .\cmd\gamedepot

cd ..
```

## Configure Aliyun OSS

```powershell
.\GameDepot\gamedepot.exe config add-oss aliyun-oss `
  --region cn-shenzhen `
  --bucket "lsq"

.\GameDepot\gamedepot.exe config set-credentials aliyun-oss `
  --access-key-id <AccessKeyId> `
  --access-key-secret <AccessKeySecret>
```

## Command-first smoke flow

Assume:

```text
.
├─ GameDepot
└─ GameTest
```

```powershell
Remove-Item -Recurse -Force .\GameTest -ErrorAction SilentlyContinue
mkdir .\GameTest
cd .\GameTest

git init
git config user.email "test@example.com"
git config user.name "GameDepot Test"

..\GameDepot\gamedepot.exe init --project GameTest --template ue5
..\GameDepot\gamedepot.exe config project-use aliyun-oss
..\GameDepot\gamedepot.exe doctor
..\GameDepot\gamedepot.exe store check
```

Create fake UE-style files:

```powershell
New-Item -ItemType Directory -Force Content\Maps | Out-Null
New-Item -ItemType Directory -Force Content\Characters | Out-Null
New-Item -ItemType Directory -Force Config | Out-Null

"fake map binary v1" | Out-File -Encoding utf8 Content\Maps\Main.umap
"fake hero asset v1" | Out-File -Encoding utf8 Content\Characters\Hero.uasset
"[/Script/EngineSettings.GameMapsSettings]" | Out-File -Encoding utf8 Config\DefaultGame.ini
```

Submit and verify:

```powershell
..\GameDepot\gamedepot.exe classify --all
..\GameDepot\gamedepot.exe submit -m "initial fake UE project"
..\GameDepot\gamedepot.exe verify --remote-only
```

Check recoverability:

```powershell
..\GameDepot\gamedepot.exe asset-status Content\Maps\Main.umap
..\GameDepot\gamedepot.exe asset-status Content\Characters\Hero.uasset
..\GameDepot\gamedepot.exe asset-status --json
```

Restore:

```powershell
Remove-Item Content\Maps\Main.umap
..\GameDepot\gamedepot.exe restore Content\Maps\Main.umap
Get-Content Content\Maps\Main.umap
```


Second version:

```powershell
"fake hero asset v2" | Out-File -Encoding utf8 Content\Characters\Hero.uasset
..\GameDepot\gamedepot.exe submit -m "update hero asset"
..\GameDepot\gamedepot.exe history Content\Characters\Hero.uasset
..\GameDepot\gamedepot.exe asset-status Content\Characters\Hero.uasset
```

GC preview:

```powershell
..\GameDepot\gamedepot.exe gc --dry-run
..\GameDepot\gamedepot.exe gc-impact --dry-run
```

## Git remote / upstream

Set the team remote:

```powershell
..\GameDepot\gamedepot.exe git-config set-remote `
  --name origin `
  --url <your-git-repo-url>
```

Optional upstream:

```powershell
..\GameDepot\gamedepot.exe git-config set-upstream `
  --name upstream `
  --url <your-upstream-url>
```

Show and test:

```powershell
..\GameDepot\gamedepot.exe git-config show
..\GameDepot\gamedepot.exe git-config test
```

Submit and push:

```powershell
..\GameDepot\gamedepot.exe submit -m "update assets" --push
```

Sync with Git pull + blob download:

```powershell
..\GameDepot\gamedepot.exe sync --force
```

Blob-only sync:

```powershell
..\GameDepot\gamedepot.exe sync --force --no-pull
```

## Recoverability states

`asset-status` reports:

- `restorable`: current version exists in store and can be restored.
- `local_only`: local file exists but is not uploaded/currently differs from manifest.
- `current_blob_missing`: current manifest SHA is missing from store; can be repaired if local file matches manifest SHA.
- `history_broken`: current version is restorable, but at least one historical blob is missing.
- `lost`: local file is missing and current remote blob is missing.

## Repair current blob

If `asset-status` says `current_blob_missing` and the local file still matches the manifest SHA:

```powershell
..\GameDepot\gamedepot.exe repair-current-blob Content\Characters\Hero.uasset
```

## Next step

After this command-line Core is stable, the next layer should be a daemon/task API that wraps the same core operations. UE should only call the daemon and should not duplicate Git/OSS logic.

## v0.6.1 cmd-smoke-test

`cmd-smoke-test` is the full command-line regression test for the cmd-first core workflow. It does not require Unreal Engine or Aliyun OSS. It creates an isolated global config directory, a shared local blob store, a bare Git `origin`, a bare Git `upstream`, a simulated UE project, and a peer clone.

It covers:

- `git-config set-remote`, `set-upstream`, `show`, `test`
- shared local store profile
- UE5 rule classification
- `submit --push`
- `sync --force` with Git pull + blob restore
- `sync --force --no-pull`
- peer clone restore from blob store
- `asset-status`, including recursive JSON output
- restore dirty-file protection and `--force`
- current blob deletion + `repair-current-blob`
- `gc --dry-run`
- `gc-impact --dry-run`, JSON, and `--protect-all-tags`

Run:

```powershell
.\GameDepot\gamedepot.exe cmd-smoke-test `
  --workspace .\GameDepot_CmdCoreSmokeWorkspace `
  --report .\gamedepot_cmd_core_smoke_report.md
```

Expected terminal result:

```text
Cmd core smoke test result: PASS
```

Open the report:

```powershell
notepad .\gamedepot_cmd_core_smoke_report.md
```

# v0.7 UE-specific API layer

This version keeps UE plugin work paused and exposes a UE-first daemon API under:

```text
/api/ue/v1/...
```

The daemon wraps the already-tested cmd-core operations. UE should treat the daemon as the single backend and should not call Git, OSS, manifest, or CLI subcommands directly.

## Build

```powershell
cd .\GameDepot
go build -o gamedepot.exe .\cmd\gamedepot
cd ..
```

## Start daemon with automatic port

Run this inside or against an initialized GameDepot project:

```powershell
.\GameDepot\gamedepot.exe daemon --root .\GameTest --addr 127.0.0.1:0
```

The daemon writes runtime connection info to:

```text
<GameProject>/.gamedepot/runtime/daemon.json
```

## UE API smoke test

```powershell
.\GameDepot\gamedepot.exe ue-api-smoke-test `
  --workspace .\GameDepot_UEAPISmokeWorkspace `
  --report .\gamedepot_ue_api_smoke_report.md
```

The smoke test automatically creates:

```text
GameDepot_UEAPISmokeWorkspace/
  _global_config/
  _shared_blobs/
  _git_remote/<project>.git
  SimUEAPIProject/
```

It validates:

```text
GET  /api/ue/v1/health
GET  /api/ue/v1/overview
GET  /api/ue/v1/settings
POST /api/ue/v1/git/test
POST /api/ue/v1/store/test
POST /api/ue/v1/assets/status
POST /api/ue/v1/assets/history
POST /api/ue/v1/project/gc-preview
POST /api/ue/v1/project/verify task
POST /api/ue/v1/project/sync task
POST /api/ue/v1/project/submit task
POST /api/ue/v1/assets/restore task
POST /api/ue/v1/assets/repair-current-blob
POST /api/ue/v1/admin/shutdown
```

## Key API groups

```text
Lifecycle
  GET  /api/ue/v1/health
  GET  /api/ue/v1/overview
  POST /api/ue/v1/admin/shutdown

Settings
  GET  /api/ue/v1/settings
  POST /api/ue/v1/settings
  GET  /api/ue/v1/rules
  POST /api/ue/v1/rules/upsert
  POST /api/ue/v1/git/test
  POST /api/ue/v1/store/test

Tasks
  POST /api/ue/v1/tasks
  GET  /api/ue/v1/tasks
  GET  /api/ue/v1/tasks/{task_id}
  POST /api/ue/v1/tasks/{task_id}/cancel

Project
  POST /api/ue/v1/project/sync
  POST /api/ue/v1/project/submit
  POST /api/ue/v1/project/verify
  POST /api/ue/v1/project/gc-preview

Assets
  POST /api/ue/v1/assets/status
      POST /api/ue/v1/assets/restore
  POST /api/ue/v1/assets/repair-current-blob
  POST /api/ue/v1/assets/history
  POST /api/ue/v1/assets/submit

Map
  POST /api/ue/v1/map/status
```

## v0.8 UE API Plugin

This version keeps the v0.7 UE-specific daemon API and adds a fresh Unreal Editor plugin client.

Design rules:

- Unreal talks only to `/api/ue/v1/*`.
- Full daemon request/response JSON is written to `Saved/Logs/GameDepotUE_API.log` by the plugin.
- Daemon-side API JSONL is written to `.gamedepot/logs/ue-api.jsonl`.
- UE popups are intentionally friendly; detailed errors go to Output Log and the JSON log files.

Build both CLI and hidden daemon executable on Windows:

```powershell
go build -o gamedepot.exe .\cmd\gamedepot
go build -ldflags="-H windowsgui" -o gamedepotd.exe .\cmd\gamedepot
```

Install the plugin into a UE project:

```powershell
.\GameDepot\gamedepot.exe ue-plugin install --project .\UEProject --overwrite
.\GameDepot\gamedepot.exe ue-plugin verify --project .\UEProject
.\GameDepot\gamedepot.exe ue-plugin diagnose --project .\UEProject
.\GameDepot\gamedepot.exe ue-plugin write-ubt-config --project .\UEProject --low-memory
.\GameDepot\gamedepot.exe ue-plugin verify --project .\UEProject
```

The plugin adds a small GameDepot toolbar and a Content Browser context menu. It auto-starts `gamedepotd.exe daemon --root <ProjectRoot> --addr 127.0.0.1:0`, reads `.gamedepot/runtime/daemon.json`, and calls the UE API.

## v0.10 Strategy B: Content-only GameDepot rules

GameDepot now owns only `Content/**`. Binary assets under `Content` are routed to the blob store and written into `depot/manifests/main.gdmanifest.json`; small text/data files under `Content` can still be routed to Git by rule. Everything outside `Content` — `Plugins/**`, `Source/**`, `Config/**`, `Docs/**`, `.uproject`, and normal project files — is staged directly by Git during `submit` and is not written into the GameDepot manifest.

Unknown files under `Content/**` are still treated as `review`, and `submit` fails until a Content rule is added or `--allow-unmanaged` is used. Unknown files outside `Content/**` are native Git files, not GameDepot review blockers.

Rule priority inside `Content/**` is:

```text
protected runtime ignores > manual Content rules > built-in UE5 Content rules > review
```

Useful commands:

```powershell
# Show all rules
.\gamedepot.exe rules list

# Force one selected path into OSS/blob storage
.\gamedepot.exe rules set --mode blob --kind manual_blob Content\Weird\Foo.custom

# Force one path into Git
.\gamedepot.exe rules set --mode git --kind manual_git Content\Data\SmallTable.json

# Ignore one path
.\gamedepot.exe rules set --mode ignore --kind manual_ignore Content\Temp\Scratch.uasset

# Apply to a Content directory
.\gamedepot.exe rules set --mode blob --scope directory Content\Imported\VendorPack

# Apply to an extension within the Content tree
.\gamedepot.exe rules set --mode blob --scope extension Content\Imported\sample.abc
```

Emergency bypass:

```powershell
.\gamedepot.exe submit -m "submit known files only" --allow-unmanaged
```

The Unreal Content Browser context menu also has:

- `Set Rule: Blob / OSS`
- `Set Rule: Git`
- `Set Rule: Ignore`

These add exact-path manual rules to `.gamedepot/config.yaml`, before the default UE rules. Use `Blob / OSS` for normal `.uasset` and `.umap` files; use `Git` only for small text-like data files.

## Git / OSS routing model in v0.8

GameDepot v0.8 treats Git as the version authority and stores the manifest in Git. The manifest is now a per-version storage routing table: for each managed file it records whether that version stores the file body in Git or in the blob store.

A single path may move in either direction across commits:

```text
commit A: Content/Hero.uasset -> storage=git
commit B: Content/Hero.uasset -> storage=blob sha256=...
commit C: Content/Hero.uasset -> storage=git
```

The invariant is that one commit has only one authoritative source for a path:

- `storage=git`: the file body is tracked by Git.
- `storage=blob`: the file body is stored in OSS/S3/local blob store and Git tracks only the manifest reference.
- `deleted=true`: the path was managed before but is deleted in this manifest version.

`rules` decide how the next submit should classify files. The checked-out `manifest` decides how the current version should be restored.

### Submit transitions

`gamedepot submit` now compares the previous manifest with the current rules and performs precise Git index changes:

| Previous manifest | Current rule | Submit behavior |
|---|---|---|
| git | git | `git add -f <path>`, manifest keeps `storage=git` |
| git | blob | upload file to blob store, manifest writes `storage=blob`, `git rm --cached <path>` |
| blob | blob | upload new content if hash changed, manifest updates `sha256` |
| blob | git | ensure local file exists, `git add -f <path>`, manifest writes `storage=git` |
| none | git | `git add -f <path>`, manifest writes `storage=git` |
| none | blob | upload file, manifest writes `storage=blob` |
| any | review | submit fails unless `--allow-unmanaged` is used |

### Checkout and sync

Use GameDepot checkout when moving between versions with different Git/blob ownership:

```powershell
gamedepot checkout <git-ref>
```

It removes known current blob-managed local files when safe, runs `git checkout`, reloads the checked-out config/manifest, then restores all `storage=blob` files from the blob store.

If you already ran raw Git commands, use:

```powershell
gamedepot sync
```

`sync` reads the currently checked-out manifest and restores only `storage=blob` entries. `storage=git` entries are left to Git.

### GC rule

Blob garbage collection must preserve every blob referenced by protected Git manifest versions, not just the current worktree. v0.8 GC counts only manifest entries with `storage=blob`.

## Content Browser Git / OSS status columns

The Unreal plugin now adds **Tools > GameDepot > Asset Status Browser** and a toolbar button named **Asset Status**.

This opens a GameDepot-specific Content Browser asset picker in Columns view. It adds these columns:

- `GD Storage`: current manifest route for this Git version: `Git`, `OSS`, `New`, or unknown.
- `GD Sync`: local working-tree state compared with the current manifest route, such as `Synced`, `Modified`, `Missing Local`, `OSS/Git Conflict`, or `New / Unsubmitted`.
- `GD Remote`: for OSS/blob-managed assets, whether the current blob exists in the configured store.
- `GD Rule`: the rule mode that will be used by the next submit.
- `GD Message`: diagnostic text explaining the current state.

Use **Refresh GameDepot Status** in the tab, or **Tools > GameDepot > Refresh Asset Status Cache**, after changing assets, syncing, submitting, or changing rules.

The regular UE Content Browser context menu is still available for selected assets. The status browser is a separate Content Browser view because Unreal's normal asset-browser status overlay is tied to the editor source-control provider; GameDepot keeps its Git/OSS routing model in the daemon and manifest instead of replacing UE's built-in provider.

## v0.8 status browser hotfix

This build fixes two Unreal Editor integration issues:

- The UE plugin now rejects stale daemon runtimes whose version does not match the plugin build. If an older daemon is found, the plugin runs `gamedepot daemon stop --root <project> --kill`, switches the next launch to an auto port, and starts the bundled daemon again. This prevents `/api/ue/v1/rules/upsert` from returning 404 because UE is still connected to an old daemon.
- The Asset Status Browser now includes a guaranteed visible status table above the asset picker, in addition to the Content Browser column-view custom columns. The asset picker settings name was reset to `GameDepotAssetStatusBrowser_v2` so old column layout state will not hide the GameDepot columns.

Manual stale-daemon cleanup:

```powershell
gamedepot daemon stop --root "C:\Path\To\UEProject" --kill
```

Then reopen UE or click any GameDepot action to auto-start the new daemon.

## v0.8 API smoke test

After the v0.8 Git/OSS routing core is built, run the daemon/API smoke before wiring the UE plugin to the real daemon:

```powershell
go build -o gamedepot.exe .\cmd\gamedepot
.\gamedepot.exe v08-core-smoke-test
.\gamedepot.exe v08-api-smoke-test
```

`v08-api-smoke-test` creates an isolated fake UE project, starts the daemon on an automatic local port, and verifies the HTTP contract used by the UE UI:

- rules list/upsert/delete/reorder
- asset status for new, review, Git-routed, and OSS-routed files
- submit task failure on review files
- Git -> OSS and OSS -> Git submit transitions
- mixed Git/OSS history listing
- restore from Git history and OSS history
- revert unsubmitted changes
- lightweight overview and sync task
