# GameDepot pointer-refs prototype

This build switches GameDepot's core asset model from one large manifest file to per-asset pointer refs.

## Core model

```text
Content/**                         real UE content files, ignored by Git
depot/refs/Content/**/*.gdref      Git-tracked pointer refs, one file per Content asset
.gamedepot/state/local-index.json  local Base state for safe Base/Local/Remote checks
.gamedepot/remote_blobs            local test blob store, or replace with OSS/S3 profile
```

Rules are no longer user-facing in the main workflow:

```text
Content/**     => GameDepot blob storage + pointer refs
Non-Content/** => normal Git
```

A `.gdref` file looks like this:

```json
{
  "version": 1,
  "path": "Content/Game/Props/Chair.uasset",
  "storage": "blob",
  "oid": "sha256:...",
  "size": 183920,
  "kind": "content_asset"
}
```

## Main commands

```powershell
gamedepot init [.] --remote <git-url> --branch <branch> [--no-plugin]
gamedepot clone [--branch <branch>] <git-url> [dir]
gamedepot update [--force]
gamedepot publish -m "message" [--dry-run]
gamedepot status [--json]
gamedepot verify
gamedepot smoke-test
```

Removed legacy commands:

```text
pull / sync / submit / push / rules
```

Use `update` for project refresh and `publish` for upload + Git commit + push.

## Safe update/publish behavior

GameDepot now tracks three values for each Content file:

```text
Base   = .gamedepot/state/local-index.json
Local  = current Content file hash
Remote = depot/refs/**/*.gdref oid
```

Update behavior:

```text
Local == Base, Remote changed    => download Remote
Local changed, Remote == Base    => keep local change
Local changed, Remote changed    => conflict; do not overwrite
--force                          => discard local Content conflict and materialize Remote
```

Publish behavior:

```text
Base == Remote, Local changed    => upload blob and update .gdref
Local == Remote                  => unchanged
Remote changed while Local stale => block; run update first
Local and Remote both changed    => conflict
```

This is intended to avoid the previous failure mode where B publishes Asset2 and accidentally rolls A's Asset1 back because B's local Asset1 was stale.

## Build and test

```powershell
go test ./...
go build -o gamedepot.exe .\cmd\gamedepot
.\gamedepot.exe smoke-test --workspace .\_smoke --clean --keep
```

The smoke test creates a local bare Git remote, three working clones, and a shared local blob store. It validates:

```text
init -> publish -> clone/update
A modifies Asset1 while B modifies Asset2
B update keeps local Asset2 and downloads A Asset1
B publish does not roll A Asset1 back
same-asset concurrent edits are blocked as conflicts
```

## Minimal flow

Local-only prototype:

```powershell
git init
git config user.email "test@example.com"
git config user.name "GameDepot Test"

"{}" | Out-File -Encoding utf8 TestProject.uproject
gamedepot init --no-plugin

New-Item -ItemType Directory -Force Content\Maps | Out-Null
"fake map binary v1" | Out-File -Encoding utf8 Content\Maps\Main.umap

gamedepot publish -m "initial content"
gamedepot status
gamedepot verify
gamedepot smoke-test
```

Initialize directly against a new empty GitHub repository:

```powershell
"{}" | Out-File -Encoding utf8 TestProject.uproject
gamedepot init --remote https://github.com/<org>/<repo>.git --branch main --no-plugin
gamedepot publish -m "initial GameDepot project"
```

For a new clone of an existing GameDepot repository:

```powershell
gamedepot clone --branch main https://github.com/<org>/<repo>.git ProjectB
cd ProjectB
gamedepot update
```

`gamedepot clone` also handles an empty remote repository. If the clone does not contain a `.uproject` yet, it leaves the Git checkout ready and prints a note to create/copy the UE project and then run `gamedepot init`.

## Notes

- `depot/manifests/main.gdmanifest.json` is no longer created by `init`.
- Legacy top-level commands `pull`, `sync`, `submit`, `push`, and `rules` have been removed from the CLI. Use `update` / `publish` with pointer refs.
- `init --remote <url> --branch <name>` configures Git remote/upstream metadata before the first push, which is useful for new empty GitHub repositories.
- `clone --branch <name>` first checks whether the remote branch exists; for empty repositories it falls back to normal clone and sets the local initial branch.
- UE plugin UI has not been fully redesigned in this pass; it should next call the new core operations rather than duplicating Git/OSS logic.
