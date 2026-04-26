# GameDepot GUI v0.7

A thin PySide6 desktop client for GameDepot Core. It starts or connects to `gamedepot daemon` and calls the local HTTP API.

## Install

```powershell
cd .\GameDepot
python -m pip install -r .\gui\requirements.txt
```

## Launch

```powershell
.\gamedepot.exe gui --root ..\GameTest
```

Direct Python launch:

```powershell
python .\gui\run_gui.py `
  --gamedepot-exe .\gamedepot.exe `
  --project-root ..\GameTest
```

## Notes

- The GUI does not access OSS or Git directly.
- All project operations go through the local API daemon.
- The daemon should stay on `127.0.0.1` unless you set a bearer token.
