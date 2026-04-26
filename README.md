# GameDepot v0.7

GameDepot is a Git + object-store workflow prototype for small UE-style game projects.

v0.7 adds a PySide desktop GUI on top of the v0.6 local HTTP API daemon while keeping all existing CLI, Aliyun OSS, GC, and team smoke-test capabilities. The HTTP API remains local-first: it binds to `127.0.0.1` by default and can optionally require a bearer token.

Main v0.7 additions:

- `gui`: launch the PySide desktop client.
- `daemon`/`serve`: start a local HTTP API server.
- `api-smoke-test`: create a simulated UE project and exercise the HTTP API end-to-end.
- JSON API endpoints for status, classify, manifest, store check, locks, submit, sync, restore, verify, history, GC, and delete-version.

Existing v0.5.x features remain:

- `config add-oss`: convenience profile command for Alibaba Cloud OSS S3-compatible endpoints.
- `remote-smoke-test`: simulated UE project smoke test against a real store profile such as Aliyun OSS.
- `team-smoke-test`: two-client Alice/Bob collaboration smoke test using a local bare Git remote plus a shared object store profile.
- `delete-version`: manually delete a specific historical blob version.
- `gc`: list and optionally delete unreferenced blob objects.
- deletion log: executed deletes append JSONL records under `.gamedepot/logs/deletions.jsonl`.
- `smoke-test` now exercises delete-version and GC flows.

`gc` and `delete-version` are **dry-run by default**. They only delete when you pass `--execute`.

A project repository does not store access keys. It only stores:

```yaml
store:
  profile: aliyun-oss
  prefix: projects/PartyGame/blobs
```

The real endpoint, bucket, credentials, and lock user name live in the user's global GameDepot config directory.

---

## Build

From the parent directory that contains `GameDepot`:

```powershell
cd .\GameDepot

go fmt ./...
go test ./...
go build -o gamedepot.exe .\cmd\gamedepot

cd ..
```

---

## Local simulated UE smoke test

This does not require Unreal Engine. It creates fake `.umap`, `.uasset`, `.xlsx`, `.blend`, and `.zip` files and runs the full workflow.

```powershell
.\GameDepot\gamedepot.exe smoke-test `
  --workspace .\GameDepot_SmokeWorkspace `
  --report .\gamedepot_smoke_report.md
```

The smoke test runs:

```text
git init
init --template ue5
config user
doctor
store check
classify/status
lock/locks
submit/verify
restore dirty protection
second submit
history
delete-version dry-run/execute
gc dry-run, including protected tag check
sync/unlock/final verify
```

---

## Local HTTP API daemon

Start the daemon from inside a GameDepot project:

```powershell
..\GameDepot\gamedepot.exe daemon --addr 127.0.0.1:17320 --root .
```

With token auth:

```powershell
..\GameDepot\gamedepot.exe daemon --addr 127.0.0.1:17320 --root . --token dev-token
```

Example API calls:

```powershell
Invoke-RestMethod http://127.0.0.1:17320/api/v1/health
Invoke-RestMethod http://127.0.0.1:17320/api/v1/status
Invoke-RestMethod -Method Post http://127.0.0.1:17320/api/v1/verify -Body "{}" -ContentType "application/json"
```

With token auth:

```powershell
$headers = @{ Authorization = "Bearer dev-token" }
Invoke-RestMethod -Headers $headers http://127.0.0.1:17320/api/v1/health
```

Important endpoints:

```text
GET  /api/v1/health
GET  /api/v1/version
GET  /api/v1/status
GET  /api/v1/classify?all=true&target=Content
GET  /api/v1/manifest
GET  /api/v1/locks
GET  /api/v1/history?path=Content/Characters/Hero.uasset
GET  /api/v1/store
POST /api/v1/store/check
POST /api/v1/submit          { "message": "..." }
POST /api/v1/sync            { "force": true }
POST /api/v1/restore         { "path": "Content/Maps/Main.umap", "force": true }
POST /api/v1/lock            { "path": "Content/Maps/Main.umap", "note": "editing" }
POST /api/v1/unlock          { "path": "Content/Maps/Main.umap" }
POST /api/v1/verify          { "remote_only": true }
POST /api/v1/gc              { "dry_run": true, "json": true }
POST /api/v1/delete-version  { "path": "...", "sha256": "...", "dry_run": true }
```

### API smoke test

This does not require Unreal Engine. It starts the API handler in-process, creates a simulated UE project, and exercises the HTTP endpoints.

```powershell
.\GameDepot\gamedepot.exe api-smoke-test `
  --workspace .\GameDepot_APISmokeWorkspace `
  --report .\gamedepot_api_smoke_report.md
```

Open the report:

```powershell
notepad .\gamedepot_api_smoke_report.md
```

---

## Aliyun OSS setup

Create an OSS bucket in the Aliyun console first. Use a RAM AccessKey with permission to put, get, list, and delete objects in that bucket or project prefix.

Add an OSS profile. Both argument orders are supported in v0.5.1:

```powershell
.\GameDepot\gamedepot.exe config add-oss aliyun-oss `
  --region cn-hangzhou `
  --bucket your-bucket-name
```

```powershell
.\GameDepot\gamedepot.exe config add-oss `
  --region cn-hangzhou `
  --bucket your-bucket-name `
  aliyun-oss
```

This creates an S3-compatible OSS profile with:

```text
endpoint: https://s3.oss-cn-hangzhou.aliyuncs.com
force_path_style: false
```

Set credentials. Both argument orders are supported:

```powershell
.\GameDepot\gamedepot.exe config set-credentials aliyun-oss `
  --access-key-id YOUR_ACCESS_KEY_ID `
  --access-key-secret YOUR_ACCESS_KEY_SECRET
```

```powershell
.\GameDepot\gamedepot.exe config set-credentials `
  --access-key-id YOUR_ACCESS_KEY_ID `
  --access-key-secret YOUR_ACCESS_KEY_SECRET `
  aliyun-oss
```

Check profiles:

```powershell
.\GameDepot\gamedepot.exe config profiles
```

---

## Aliyun OSS remote smoke test

This uses your real global profile and credentials. It does **not** isolate `GAMEDEPOT_CONFIG_DIR`.

```powershell
.\GameDepot\gamedepot.exe remote-smoke-test `
  --profile aliyun-oss `
  --workspace .\GameDepot_OSS_SmokeWorkspace `
  --report .\gamedepot_oss_smoke_report.md `
  --project SimUEProjectOSS
```

The remote smoke test creates a simulated UE-style project, switches it to the `aliyun-oss` profile, uploads blobs to OSS, verifies remote blobs, restores files from OSS, tests locks, runs `delete-version`, runs `gc --dry-run`, and writes a markdown report.

Because the project name becomes part of the store prefix, use a unique `--project` value when repeating remote tests if you want separate prefixes.

---

## Aliyun OSS team smoke test

This is the most important collaboration test. It creates Alice and Bob workspaces, a local bare Git remote, and uses your real OSS profile as the shared blob/lock store.

```powershell
.\GameDepot\gamedepot.exe team-smoke-test `
  --profile aliyun-oss `
  --workspace .\GameDepot_TeamSmokeWorkspace `
  --report .\gamedepot_team_smoke_report.md
```

It verifies: Alice submit -> Bob clone and sync, lock conflict, Alice edits map -> Bob pulls and syncs, Bob edits asset -> Alice pulls and syncs, remote-only verify, and GC dry-run with tag protection.

---

## Manual OSS project test

Inside an existing GameDepot project:

```powershell
..\GameDepot\gamedepot.exe config project-use aliyun-oss
..\GameDepot\gamedepot.exe store info
..\GameDepot\gamedepot.exe store check
..\GameDepot\gamedepot.exe submit -m "test aliyun oss"
..\GameDepot\gamedepot.exe verify --remote-only
```

---

## GC and delete-version

Show candidates only:

```powershell
..\GameDepot\gamedepot.exe gc --dry-run
```

Protect a milestone tag while scanning:

```powershell
..\GameDepot\gamedepot.exe gc --dry-run --protect-tag milestone-v0.1
```

Protect all Git tags:

```powershell
..\GameDepot\gamedepot.exe gc --dry-run --protect-all-tags
```

Actually delete GC candidates:

```powershell
..\GameDepot\gamedepot.exe gc --execute
```

Delete one historical blob version:

```powershell
..\GameDepot\gamedepot.exe delete-version Content\Characters\Hero.uasset `
  --sha256 FULL_64_CHAR_SHA256 `
  --execute
```

Deleting the current manifest version is refused unless you pass `--force-current`.

Executed deletions are logged to:

```text
.gamedepot/logs/deletions.jsonl
```

---

## MinIO test

Start MinIO:

```powershell
docker run -p 9000:9000 -p 9001:9001 `
  -e "MINIO_ROOT_USER=minioadmin" `
  -e "MINIO_ROOT_PASSWORD=minioadmin" `
  quay.io/minio/minio server /data --console-address ":9001"
```

Create bucket `gamedepot-test` in the MinIO console, then:

```powershell
.\GameDepot\gamedepot.exe config add-s3 minio-local `
  --endpoint http://127.0.0.1:9000 `
  --region us-east-1 `
  --bucket gamedepot-test `
  --force-path-style

.\GameDepot\gamedepot.exe config set-credentials minio-local `
  --access-key-id minioadmin `
  --access-key-secret minioadmin

.\GameDepot\gamedepot.exe remote-smoke-test `
  --profile minio-local `
  --workspace .\GameDepot_MinIO_SmokeWorkspace `
  --report .\gamedepot_minio_smoke_report.md
```

---

## Main commands

```text
gamedepot init --project my-game [--template ue5]
gamedepot doctor
gamedepot config path
gamedepot config user [--name <name>] [--email <email>]
gamedepot config profiles
gamedepot config add-local <name> [--path <path>]
gamedepot config add-oss <name> --region <region> --bucket <bucket> [--internal]
gamedepot config add-s3 <name> --endpoint <url> --bucket <bucket> [--region <region>] [--force-path-style]
gamedepot config set-credentials <profile> [--access-key-id <id>] [--access-key-secret <secret>]
gamedepot config use <profile>
gamedepot config project-use <profile>
gamedepot smoke-test [--workspace <dir>] [--report <file>]
gamedepot remote-smoke-test --profile <profile> [--workspace <dir>] [--report <file>]
gamedepot team-smoke-test --profile <profile> [--workspace <dir>] [--report <file>]
gamedepot daemon [--addr 127.0.0.1:17320] [--root .] [--token <token>]
gamedepot gui [--root .] [--addr 127.0.0.1:17320] [--token <token>]
gamedepot api-smoke-test [--workspace <dir>] [--report <file>] [--profile <profile>]
gamedepot store info
gamedepot store check
gamedepot classify [path] [--json] [--all]
gamedepot status [--json]
gamedepot submit -m "update assets"
gamedepot sync [--force]
gamedepot verify [--local-only] [--remote-only]
gamedepot ls [--all]
gamedepot history <path>
gamedepot restore <path> [--sha256 <sha256>] [--force]
gamedepot lock <path> [--note <text>] [--force]
gamedepot unlock <path> [--force]
gamedepot locks [--json]
gamedepot delete-version <path> --sha256 <sha256> [--dry-run|--execute] [--force-current]
gamedepot gc [--dry-run|--execute] [--protect-tag <tag>] [--protect-all-tags] [--json]
```

---

## PySide GUI

v0.7 includes a lightweight desktop client under `gui/`. It talks to the local HTTP API daemon instead of reimplementing Git, OSS, manifest, lock, or GC logic.

Install the GUI dependency:

```powershell
cd .\GameDepot
python -m pip install -r .\gui\requirements.txt
```

Launch from the GameDepot repository root:

```powershell
.\gamedepot.exe gui --root ..\GameTest
```

Or launch the Python entry directly:

```powershell
python .\gui\run_gui.py `
  --gamedepot-exe .\gamedepot.exe `
  --project-root ..\GameTest `
  --addr 127.0.0.1:17320
```

The GUI can start and stop the daemon for the selected project root. Current tabs:

```text
Dashboard: health, status, store info/check, verify, sync
Files: classify, restore
Locks: locks, lock, unlock
Submit: submit, git status
GC: gc dry-run/execute switch, delete-version dry-run/execute switch
```

Recommended GUI workflow:

```text
1. Select gamedepot.exe
2. Select the GameDepot project root
3. Click Start daemon
4. Click Health / Status
5. Use Classify, Lock, Submit, Sync, Restore, Verify, and GC from the tabs
```

The GUI is intentionally thin. If a button fails, the log panel shows the API error returned by Core.
