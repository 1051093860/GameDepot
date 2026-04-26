from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any, Callable

try:
    from PySide6.QtCore import QObject, QRunnable, Qt, QThreadPool, Signal, Slot
    from PySide6.QtWidgets import (
        QApplication,
        QCheckBox,
        QFileDialog,
        QFormLayout,
        QGridLayout,
        QGroupBox,
        QHBoxLayout,
        QLabel,
        QLineEdit,
        QMainWindow,
        QMessageBox,
        QPushButton,
        QPlainTextEdit,
        QTabWidget,
        QVBoxLayout,
        QWidget,
    )
except ModuleNotFoundError as exc:  # pragma: no cover - user-facing import error
    print("PySide6 is not installed. Install it with: python -m pip install -r gui/requirements.txt", file=sys.stderr)
    raise SystemExit(2) from exc

from .api import ApiResult, GameDepotApiClient
from .daemon import DaemonProcess, DaemonSpec, default_gamedepot_exe


class WorkerSignals(QObject):
    finished = Signal(str, object)
    failed = Signal(str, str)


class ApiWorker(QRunnable):
    def __init__(self, name: str, fn: Callable[[], ApiResult]) -> None:
        super().__init__()
        self.name = name
        self.fn = fn
        self.signals = WorkerSignals()

    @Slot()
    def run(self) -> None:
        try:
            result = self.fn()
        except Exception as exc:  # noqa: BLE001 - GUI should show any unexpected error.
            self.signals.failed.emit(self.name, str(exc))
            return
        self.signals.finished.emit(self.name, result)


def pretty(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, str):
        return value
    return json.dumps(value, ensure_ascii=False, indent=2)


class MainWindow(QMainWindow):
    def __init__(self, args: argparse.Namespace) -> None:
        super().__init__()
        self.setWindowTitle("GameDepot GUI v0.7")
        self.resize(1180, 780)
        self.pool = QThreadPool.globalInstance()
        self.daemon = DaemonProcess()

        self.exe_edit = QLineEdit(args.gamedepot_exe or default_gamedepot_exe())
        self.root_edit = QLineEdit(str(Path(args.project_root or ".").resolve()))
        self.addr_edit = QLineEdit(args.addr or "127.0.0.1:17320")
        self.token_edit = QLineEdit(args.token or "")
        self.token_edit.setEchoMode(QLineEdit.Password)

        self.log = QPlainTextEdit()
        self.log.setReadOnly(True)
        self.log.setMaximumBlockCount(2000)

        central = QWidget()
        root_layout = QVBoxLayout(central)
        root_layout.addWidget(self._connection_box())

        self.tabs = QTabWidget()
        self.tabs.addTab(self._dashboard_tab(), "Dashboard")
        self.tabs.addTab(self._files_tab(), "Files")
        self.tabs.addTab(self._locks_tab(), "Locks")
        self.tabs.addTab(self._submit_tab(), "Submit")
        self.tabs.addTab(self._gc_tab(), "GC")
        root_layout.addWidget(self.tabs, 1)

        root_layout.addWidget(QLabel("Log"))
        root_layout.addWidget(self.log, 1)
        self.setCentralWidget(central)

    def closeEvent(self, event) -> None:  # type: ignore[override]
        self.daemon.stop()
        event.accept()

    def client(self) -> GameDepotApiClient:
        return GameDepotApiClient(base_url=f"http://{self.addr_edit.text().strip()}", token=self.token_edit.text().strip())

    def append(self, text: str) -> None:
        self.log.appendPlainText(text.rstrip())
        self.log.appendPlainText("")

    def _connection_box(self) -> QGroupBox:
        box = QGroupBox("Connection")
        layout = QGridLayout(box)
        layout.addWidget(QLabel("gamedepot exe"), 0, 0)
        layout.addWidget(self.exe_edit, 0, 1)
        browse_exe = QPushButton("Browse")
        browse_exe.clicked.connect(self.browse_exe)
        layout.addWidget(browse_exe, 0, 2)

        layout.addWidget(QLabel("project root"), 1, 0)
        layout.addWidget(self.root_edit, 1, 1)
        browse_root = QPushButton("Browse")
        browse_root.clicked.connect(self.browse_root)
        layout.addWidget(browse_root, 1, 2)

        layout.addWidget(QLabel("addr"), 2, 0)
        layout.addWidget(self.addr_edit, 2, 1)
        layout.addWidget(QLabel("token"), 3, 0)
        layout.addWidget(self.token_edit, 3, 1)

        start = QPushButton("Start daemon")
        start.clicked.connect(self.start_daemon)
        stop = QPushButton("Stop daemon")
        stop.clicked.connect(self.stop_daemon)
        health = QPushButton("Health")
        health.clicked.connect(lambda: self.call("health", lambda: self.client().get("/api/v1/health")))
        row = QHBoxLayout()
        row.addWidget(start)
        row.addWidget(stop)
        row.addWidget(health)
        row.addStretch(1)
        layout.addLayout(row, 4, 1, 1, 2)
        return box

    def _dashboard_tab(self) -> QWidget:
        w = QWidget()
        layout = QVBoxLayout(w)
        row = QHBoxLayout()
        buttons = [
            ("Status", lambda: self.client().get("/api/v1/status")),
            ("Store info", lambda: self.client().get("/api/v1/store")),
            ("Store check", lambda: self.client().post("/api/v1/store/check", {})),
            ("Verify", lambda: self.client().post("/api/v1/verify", {})),
            ("Verify remote only", lambda: self.client().post("/api/v1/verify", {"remote_only": True})),
            ("Sync", lambda: self.client().post("/api/v1/sync", {"force": self.sync_force_check.isChecked()})),
        ]
        self.sync_force_check = QCheckBox("force sync")
        for label, fn in buttons:
            b = QPushButton(label)
            b.clicked.connect(lambda _=False, label=label, fn=fn: self.call(label, fn))
            row.addWidget(b)
        row.addWidget(self.sync_force_check)
        row.addStretch(1)
        layout.addLayout(row)
        self.dashboard_output = QPlainTextEdit()
        self.dashboard_output.setReadOnly(True)
        layout.addWidget(self.dashboard_output, 1)
        return w

    def _files_tab(self) -> QWidget:
        w = QWidget()
        layout = QVBoxLayout(w)
        form = QFormLayout()
        self.classify_target = QLineEdit("Content")
        self.classify_all = QCheckBox("all")
        form.addRow("classify target", self.classify_target)
        form.addRow("options", self.classify_all)
        layout.addLayout(form)
        row = QHBoxLayout()
        classify_btn = QPushButton("Classify")
        classify_btn.clicked.connect(self.classify)
        row.addWidget(classify_btn)
        layout.addLayout(row)
        self.files_output = QPlainTextEdit()
        self.files_output.setReadOnly(True)
        layout.addWidget(self.files_output, 1)

        restore_box = QGroupBox("Restore")
        restore_layout = QFormLayout(restore_box)
        self.restore_path = QLineEdit("Content/Maps/Main.umap")
        self.restore_sha = QLineEdit("")
        self.restore_force = QCheckBox("force")
        restore_btn = QPushButton("Restore")
        restore_btn.clicked.connect(self.restore)
        restore_layout.addRow("path", self.restore_path)
        restore_layout.addRow("sha256 optional", self.restore_sha)
        restore_layout.addRow("options", self.restore_force)
        restore_layout.addRow(restore_btn)
        layout.addWidget(restore_box)
        return w

    def _locks_tab(self) -> QWidget:
        w = QWidget()
        layout = QVBoxLayout(w)
        row = QHBoxLayout()
        refresh = QPushButton("Refresh locks")
        refresh.clicked.connect(lambda: self.call("locks", lambda: self.client().get("/api/v1/locks")))
        row.addWidget(refresh)
        row.addStretch(1)
        layout.addLayout(row)
        self.locks_output = QPlainTextEdit()
        self.locks_output.setReadOnly(True)
        layout.addWidget(self.locks_output, 1)

        form = QFormLayout()
        self.lock_path = QLineEdit("Content/Maps/Main.umap")
        self.lock_note = QLineEdit("editing from GUI")
        self.lock_force = QCheckBox("force")
        form.addRow("path", self.lock_path)
        form.addRow("note", self.lock_note)
        form.addRow("options", self.lock_force)
        layout.addLayout(form)
        row2 = QHBoxLayout()
        lock_btn = QPushButton("Lock")
        lock_btn.clicked.connect(self.lock_file)
        unlock_btn = QPushButton("Unlock")
        unlock_btn.clicked.connect(self.unlock_file)
        row2.addWidget(lock_btn)
        row2.addWidget(unlock_btn)
        row2.addStretch(1)
        layout.addLayout(row2)
        return w

    def _submit_tab(self) -> QWidget:
        w = QWidget()
        layout = QVBoxLayout(w)
        self.submit_msg = QLineEdit("update assets from GUI")
        submit_btn = QPushButton("Submit")
        submit_btn.clicked.connect(self.submit)
        git_status_btn = QPushButton("Git status")
        git_status_btn.clicked.connect(lambda: self.call("git status", lambda: self.client().get("/api/v1/git/status")))
        row = QHBoxLayout()
        row.addWidget(QLabel("message"))
        row.addWidget(self.submit_msg, 1)
        row.addWidget(submit_btn)
        row.addWidget(git_status_btn)
        layout.addLayout(row)
        self.submit_output = QPlainTextEdit()
        self.submit_output.setReadOnly(True)
        layout.addWidget(self.submit_output, 1)
        return w

    def _gc_tab(self) -> QWidget:
        w = QWidget()
        layout = QVBoxLayout(w)
        self.gc_dry_run = QCheckBox("dry run")
        self.gc_dry_run.setChecked(True)
        self.gc_json = QCheckBox("json")
        self.gc_json.setChecked(True)
        self.gc_protect_all = QCheckBox("protect all tags")
        gc_btn = QPushButton("Run GC")
        gc_btn.clicked.connect(self.gc)
        row = QHBoxLayout()
        row.addWidget(self.gc_dry_run)
        row.addWidget(self.gc_json)
        row.addWidget(self.gc_protect_all)
        row.addWidget(gc_btn)
        row.addStretch(1)
        layout.addLayout(row)
        self.gc_output = QPlainTextEdit()
        self.gc_output.setReadOnly(True)
        layout.addWidget(self.gc_output, 1)

        box = QGroupBox("Delete one historical version")
        form = QFormLayout(box)
        self.del_path = QLineEdit("Content/Characters/Hero.uasset")
        self.del_sha = QLineEdit("")
        self.del_execute = QCheckBox("execute")
        del_btn = QPushButton("Delete version")
        del_btn.clicked.connect(self.delete_version)
        form.addRow("path", self.del_path)
        form.addRow("sha256", self.del_sha)
        form.addRow("options", self.del_execute)
        form.addRow(del_btn)
        layout.addWidget(box)
        return w

    def browse_exe(self) -> None:
        path, _ = QFileDialog.getOpenFileName(self, "Select gamedepot executable", str(Path(self.exe_edit.text()).parent))
        if path:
            self.exe_edit.setText(path)

    def browse_root(self) -> None:
        path = QFileDialog.getExistingDirectory(self, "Select GameDepot project root", self.root_edit.text())
        if path:
            self.root_edit.setText(path)

    def start_daemon(self) -> None:
        try:
            self.daemon.start(DaemonSpec(
                gamedepot_exe=self.exe_edit.text().strip(),
                project_root=self.root_edit.text().strip(),
                addr=self.addr_edit.text().strip(),
                token=self.token_edit.text().strip(),
            ))
        except Exception as exc:  # noqa: BLE001
            QMessageBox.critical(self, "Daemon start failed", str(exc))
            return
        self.append(f"Started daemon at http://{self.addr_edit.text().strip()} for {self.root_edit.text().strip()}")

    def stop_daemon(self) -> None:
        self.daemon.stop()
        self.append("Stopped daemon")

    def call(self, name: str, fn: Callable[[], ApiResult]) -> None:
        self.append(f"> {name}")
        worker = ApiWorker(name, fn)
        worker.signals.finished.connect(self.on_api_finished)
        worker.signals.failed.connect(self.on_api_failed)
        self.pool.start(worker)

    @Slot(str, object)
    def on_api_finished(self, name: str, result: ApiResult) -> None:
        text = result.output.strip() or pretty(result.data) or result.raw
        if result.ok:
            self.append(f"PASS {name}\n{text}")
        else:
            self.append(f"FAIL {name}\n{result.error}\n{text}")
        current = self.tabs.currentWidget()
        if name.startswith("Status") or name.startswith("Store") or name.startswith("Verify") or name.startswith("Sync") or name == "health":
            self.dashboard_output.setPlainText(text or result.error)
        elif name.startswith("classify"):
            self.files_output.setPlainText(text or result.error)
        elif name.startswith("locks") or name in {"lock", "unlock"}:
            self.locks_output.setPlainText(text or result.error)
        elif name.startswith("submit") or name == "git status":
            self.submit_output.setPlainText(text or result.error)
        elif name.startswith("gc") or name.startswith("delete-version"):
            self.gc_output.setPlainText(text or result.error)
        elif current:
            pass

    @Slot(str, str)
    def on_api_failed(self, name: str, error: str) -> None:
        self.append(f"ERROR {name}\n{error}")

    def classify(self) -> None:
        self.call("classify", lambda: self.client().get("/api/v1/classify", {
            "target": self.classify_target.text().strip(),
            "all": "true" if self.classify_all.isChecked() else "false",
        }))

    def restore(self) -> None:
        self.call("restore", lambda: self.client().post("/api/v1/restore", {
            "path": self.restore_path.text().strip(),
            "sha256": self.restore_sha.text().strip(),
            "force": self.restore_force.isChecked(),
        }))

    def lock_file(self) -> None:
        self.call("lock", lambda: self.client().post("/api/v1/lock", {
            "path": self.lock_path.text().strip(),
            "note": self.lock_note.text().strip(),
            "force": self.lock_force.isChecked(),
        }))

    def unlock_file(self) -> None:
        self.call("unlock", lambda: self.client().post("/api/v1/unlock", {
            "path": self.lock_path.text().strip(),
            "force": self.lock_force.isChecked(),
        }))

    def submit(self) -> None:
        msg = self.submit_msg.text().strip()
        self.call("submit", lambda: self.client().post("/api/v1/submit", {"message": msg}))

    def gc(self) -> None:
        self.call("gc", lambda: self.client().post("/api/v1/gc", {
            "dry_run": self.gc_dry_run.isChecked(),
            "json": self.gc_json.isChecked(),
            "protect_all_tags": self.gc_protect_all.isChecked(),
        }))

    def delete_version(self) -> None:
        self.call("delete-version", lambda: self.client().post("/api/v1/delete-version", {
            "path": self.del_path.text().strip(),
            "sha256": self.del_sha.text().strip(),
            "dry_run": not self.del_execute.isChecked(),
            "json": True,
        }))


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="GameDepot PySide GUI")
    parser.add_argument("--gamedepot-exe", default=default_gamedepot_exe())
    parser.add_argument("--project-root", default=".")
    parser.add_argument("--addr", default="127.0.0.1:17320")
    parser.add_argument("--token", default="")
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(list(sys.argv[1:] if argv is None else argv))
    app = QApplication(sys.argv[:1])
    app.setApplicationName("GameDepot")
    win = MainWindow(args)
    win.show()
    return app.exec()
