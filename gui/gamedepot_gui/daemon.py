from __future__ import annotations

import os
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Optional


@dataclass
class DaemonSpec:
    gamedepot_exe: str
    project_root: str
    addr: str = "127.0.0.1:17320"
    token: str = ""


class DaemonProcess:
    def __init__(self) -> None:
        self.process: Optional[subprocess.Popen[str]] = None
        self.spec: Optional[DaemonSpec] = None

    def is_running(self) -> bool:
        return self.process is not None and self.process.poll() is None

    def start(self, spec: DaemonSpec) -> None:
        if self.is_running():
            return
        exe = str(Path(spec.gamedepot_exe).resolve())
        root = str(Path(spec.project_root).resolve())
        args = [exe, "daemon", "--addr", spec.addr, "--root", root]
        if spec.token:
            args.extend(["--token", spec.token])
        creationflags = 0
        if os.name == "nt":
            creationflags = subprocess.CREATE_NEW_PROCESS_GROUP  # type: ignore[attr-defined]
        self.process = subprocess.Popen(
            args,
            cwd=root,
            stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            encoding="utf-8",
            errors="replace",
            creationflags=creationflags,
        )
        self.spec = spec

    def stop(self) -> None:
        if not self.process:
            return
        if self.process.poll() is None:
            self.process.terminate()
            try:
                self.process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self.process.kill()
                self.process.wait(timeout=5)
        self.process = None

    def read_available_output(self) -> str:
        if not self.process or not self.process.stdout:
            return ""
        # Avoid blocking. This is intentionally conservative; the API itself is the source of truth.
        return ""


def default_gamedepot_exe() -> str:
    here = Path(__file__).resolve()
    candidates = [
        here.parents[2] / "gamedepot.exe",
        here.parents[2] / "gamedepot",
        here.parents[3] / "gamedepot.exe",
        here.parents[3] / "gamedepot",
    ]
    for c in candidates:
        if c.exists():
            return str(c)
    return "gamedepot.exe" if sys.platform.startswith("win") else "gamedepot"
