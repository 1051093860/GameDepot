# GameDepot UE Plugin - Daemon Provider Full Step

This build keeps the validated mock UI layout, but connects all main actions to the GameDepot daemon when `MockMode=false`.

Connected areas:

- Toolbar: Status / Config / Sync / Submit / OK / Error / Update
- Configuration Manager: reads daemon config and rules, saves config, upserts rules
- Asset Status Browser: loads `/api/ue/v1/assets/status`
- Content Browser context menu:
  - Show in GameDepot Status
  - Show History Versions
  - Revert Unsubmitted Changes
  - Set Rule: OSS / Git / Ignore
- History dialog:
  - Reads `/api/ue/v1/assets/history`
  - Restore selected version through `/api/ue/v1/assets/restore`
- Submit / Sync:
  - Starts daemon task and polls `/api/ue/v1/tasks/{id}`

Install:

1. Replace `UEProject/Plugins/GameDepotUE` with this folder.
2. Make sure the Go side is built and `v08-core-smoke-test` pass.
3. Build `gamedepot.exe` and put its directory in the system `PATH`, or set `GameDepotExecutable` in `Config/DefaultGameDepotUE.ini`.

Default executable discovery tries:

- `GameDepotExecutable` setting; this may be an absolute path or a binary name resolvable from `PATH`
- `gamedepot.exe` from the system `PATH`
- `gamedeport.exe` from the system `PATH` as a compatibility fallback for the old typo
- `<UEProject>/../GameDepot/gamedepot.exe`
- `<UEProject>/../gamedepot.exe`
- `<UEProject>/gamedepot.exe`

Mock fallback:

- Set `MockMode=true` in `DefaultGameDepotUE.ini` to use UI-only deterministic mock data.
