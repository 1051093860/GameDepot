from __future__ import annotations

import sys
from pathlib import Path
from typing import Dict, List, Optional

from .config import AppConfig
from .git_backend import GitBackend
from .models import BlobRef, FileEntry, HistoryEntry, UnknownBlob
from .oss_backends import ObjectStore, create_store
from .qt_compat import QtCore, QtGui, QtWidgets
from .workers import CommitScanWorker, HistoryLoadWorker, HistoryOssWorker, OssScanWorker, UnknownBlobScanWorker, blob_cache_key


GREEN = QtGui.QColor(34, 139, 34)
RED = QtGui.QColor(190, 40, 40)
GRAY = QtGui.QColor(130, 130, 130)
BLUE = QtGui.QColor(30, 90, 180)
ORANGE = QtGui.QColor(210, 120, 20)
YELLOW = QtGui.QColor(210, 170, 20)

ROLE_LOGICAL_PATH = int(QtCore.Qt.ItemDataRole.UserRole) + 1
ROLE_ROW_INDEX = int(QtCore.Qt.ItemDataRole.UserRole) + 2
ROLE_META_KIND = int(QtCore.Qt.ItemDataRole.UserRole) + 3
ROLE_BLOB_KEY = int(QtCore.Qt.ItemDataRole.UserRole) + 4


APP_QSS = """
QMainWindow, QWidget#AppRoot {
    background: #15181d;
    color: #e7ebf0;
    font-family: "Microsoft YaHei UI", "Segoe UI", sans-serif;
    font-size: 12px;
}
QFrame#HeaderPanel {
    background: qlineargradient(x1:0, y1:0, x2:1, y2:0, stop:0 #222a35, stop:1 #1a2029);
    border: 1px solid #313b49;
    border-radius: 12px;
}
QFrame#Panel {
    background: #1d222a;
    border: 1px solid #303844;
    border-radius: 10px;
}
QLabel#AppTitle {
    color: #f4f7fb;
    font-size: 20px;
    font-weight: 700;
}
QLabel#AppSubtitle {
    color: #9ca8b7;
    font-size: 12px;
}
QLabel#PanelTitle {
    color: #dfe6ef;
    font-weight: 700;
    font-size: 13px;
}
QLabel#MutedText {
    color: #94a0af;
}
QLabel#PathText {
    color: #b8c3d2;
    font-family: Consolas, "Cascadia Mono", monospace;
    font-size: 11px;
}
QLabel#Chip {
    background: #252c36;
    border: 1px solid #384353;
    border-radius: 9px;
    padding: 4px 10px;
    color: #dce4ee;
}
QLabel#ChipGreen {
    background: rgba(40, 150, 92, 0.18);
    border: 1px solid rgba(40, 150, 92, 0.55);
    border-radius: 9px;
    padding: 4px 10px;
    color: #b9f3cf;
}
QLabel#ChipRed {
    background: rgba(218, 71, 71, 0.18);
    border: 1px solid rgba(218, 71, 71, 0.55);
    border-radius: 9px;
    padding: 4px 10px;
    color: #ffc4c4;
}
QLabel#ChipGray {
    background: rgba(140, 150, 163, 0.16);
    border: 1px solid rgba(140, 150, 163, 0.45);
    border-radius: 9px;
    padding: 4px 10px;
    color: #c9d0da;
}
QLabel#ChipOrange {
    background: rgba(224, 144, 49, 0.18);
    border: 1px solid rgba(224, 144, 49, 0.55);
    border-radius: 9px;
    padding: 4px 10px;
    color: #ffd49a;
}
QPushButton {
    background: #2a3240;
    border: 1px solid #3b4656;
    border-radius: 8px;
    padding: 7px 14px;
    color: #e8edf4;
}
QPushButton:hover {
    background: #344052;
    border-color: #526176;
}
QPushButton:pressed {
    background: #202733;
}
QPushButton:disabled {
    color: #7c8794;
    background: #20252d;
    border-color: #2a313a;
}
QPushButton#PrimaryButton {
    background: #2d6cdf;
    border: 1px solid #3a7cf1;
    color: #ffffff;
    font-weight: 700;
}
QPushButton#PrimaryButton:hover {
    background: #3678ee;
}
QTreeWidget, QTableWidget {
    background: #171b21;
    border: 1px solid #2c333d;
    border-radius: 8px;
    alternate-background-color: #1b2027;
    gridline-color: #2b323c;
    color: #e7ebf0;
    selection-background-color: #29466f;
    selection-color: #ffffff;
}
QTreeWidget::item, QTableWidget::item {
    padding: 5px 6px;
}
QTreeWidget::item:selected {
    background: #29466f;
    border-radius: 4px;
}
QHeaderView::section {
    background: #252b34;
    color: #d5dde8;
    border: 0px;
    border-right: 1px solid #343c47;
    border-bottom: 1px solid #343c47;
    padding: 6px 8px;
    font-weight: 700;
}
QScrollBar:vertical, QScrollBar:horizontal {
    background: #171b21;
    border: none;
    width: 10px;
    height: 10px;
    margin: 0px;
}
QScrollBar::handle:vertical, QScrollBar::handle:horizontal {
    background: #3a4350;
    border-radius: 5px;
    min-height: 24px;
    min-width: 24px;
}
QScrollBar::handle:hover {
    background: #515d6d;
}
QScrollBar::add-line, QScrollBar::sub-line {
    width: 0px;
    height: 0px;
}
QMenu {
    background: #20262f;
    color: #e7ebf0;
    border: 1px solid #3a4350;
    border-radius: 8px;
    padding: 6px;
}
QMenu::item {
    padding: 7px 28px 7px 12px;
    border-radius: 6px;
}
QMenu::item:selected {
    background: #29466f;
}
QStatusBar {
    background: #111419;
    border-top: 1px solid #2b323c;
    color: #c2ccd8;
}
QProgressBar {
    background: #20262f;
    border: 1px solid #384353;
    border-radius: 5px;
    height: 10px;
    text-align: center;
}
QProgressBar::chunk {
    background: #3a7cf1;
    border-radius: 4px;
}
QSplitter::handle {
    background: #15181d;
}
QSplitter::handle:horizontal {
    width: 8px;
}
"""


class MainWindow(QtWidgets.QMainWindow):
    def __init__(self, start_path: Path):
        super().__init__()
        self.setWindowTitle("OSS Manager")
        self.resize(1280, 760)

        self.input_path = start_path.resolve()
        self.cfg: Optional[AppConfig] = None
        self.git: Optional[GitBackend] = None
        self.store: Optional[ObjectStore] = None
        self.files: List[FileEntry] = []
        self.file_by_logical: Dict[str, FileEntry] = {}
        self.tree_items_by_logical: Dict[str, QtWidgets.QTreeWidgetItem] = {}
        self.current_history: List[HistoryEntry] = []
        self.history_row_by_commit: Dict[str, int] = {}
        self.exists_cache: Dict[str, bool] = {}
        self.commit_thread: Optional[QtCore.QThread] = None
        self.file_oss_thread: Optional[QtCore.QThread] = None
        self.history_thread: Optional[QtCore.QThread] = None
        self.history_oss_thread: Optional[QtCore.QThread] = None
        self.unknown_blob_thread: Optional[QtCore.QThread] = None
        self.unknown_blobs: Dict[str, UnknownBlob] = {}
        self.unknown_blob_root_item: Optional[QtWidgets.QTreeWidgetItem] = None
        self.unknown_blob_group_item: Optional[QtWidgets.QTreeWidgetItem] = None
        # Keep explicit Python references to worker QObjects.
        # Without these references PyQt may garbage-collect a worker before the
        # newly-created QThread starts, which leaves the UI stuck at
        # "正在扫描commit..." with an indefinite progress bar.
        self.commit_worker: Optional[CommitScanWorker] = None
        self.file_oss_worker: Optional[OssScanWorker] = None
        self.history_worker: Optional[HistoryLoadWorker] = None
        self.history_oss_worker: Optional[HistoryOssWorker] = None
        self.unknown_blob_worker: Optional[UnknownBlobScanWorker] = None
        self.reference_index: Optional[dict[str, list[tuple[str, str]]]] = None

        self._build_ui()
        self._open_repo(self.input_path)

    def _build_ui(self) -> None:
        self.setMinimumSize(1040, 640)
        self.setStyleSheet(APP_QSS)
        self._init_icons()

        central = QtWidgets.QWidget(self)
        central.setObjectName("AppRoot")
        layout = QtWidgets.QVBoxLayout(central)
        layout.setContentsMargins(12, 12, 12, 10)
        layout.setSpacing(10)

        header = QtWidgets.QFrame()
        header.setObjectName("HeaderPanel")
        header_layout = QtWidgets.QVBoxLayout(header)
        header_layout.setContentsMargins(14, 12, 14, 12)
        header_layout.setSpacing(8)

        title_row = QtWidgets.QHBoxLayout()
        title_block = QtWidgets.QVBoxLayout()
        title_block.setSpacing(2)
        title = QtWidgets.QLabel("GameDepot OSS Manager")
        title.setObjectName("AppTitle")
        subtitle = QtWidgets.QLabel("检查 Git 历史中的 pointer refs、OSS blob 存在性，以及可清理的未知 blob")
        subtitle.setObjectName("AppSubtitle")
        title_block.addWidget(title)
        title_block.addWidget(subtitle)
        title_row.addLayout(title_block, 1)
        self.refresh_button = QtWidgets.QPushButton("刷新")
        self.refresh_button.setObjectName("PrimaryButton")
        self.refresh_button.setMinimumWidth(96)
        self.refresh_button.clicked.connect(self.refresh)
        title_row.addWidget(self.refresh_button, 0, QtCore.Qt.AlignmentFlag.AlignTop)
        header_layout.addLayout(title_row)

        info_row = QtWidgets.QHBoxLayout()
        info_row.setSpacing(10)
        self.repo_label = QtWidgets.QLabel("Repository: -")
        self.repo_label.setObjectName("PathText")
        self.provider_label = QtWidgets.QLabel("Provider: -")
        self.provider_label.setObjectName("PathText")
        self.repo_label.setTextInteractionFlags(QtCore.Qt.TextInteractionFlag.TextSelectableByMouse)
        self.provider_label.setTextInteractionFlags(QtCore.Qt.TextInteractionFlag.TextSelectableByMouse)
        info_row.addWidget(self.repo_label, 1)
        info_row.addWidget(self.provider_label, 1)
        header_layout.addLayout(info_row)

        chip_row = QtWidgets.QHBoxLayout()
        chip_row.setSpacing(8)
        self.total_chip = self._make_chip("资产 0", "Chip")
        self.green_chip = self._make_chip("绿色 0", "ChipGreen")
        self.red_chip = self._make_chip("红色 0", "ChipRed")
        self.gray_chip = self._make_chip("灰色 0", "ChipGray")
        self.unknown_chip = self._make_chip("未知 Blob 0", "ChipOrange")
        for chip in [self.total_chip, self.green_chip, self.red_chip, self.gray_chip, self.unknown_chip]:
            chip_row.addWidget(chip)
        chip_row.addStretch(1)
        header_layout.addLayout(chip_row)
        layout.addWidget(header)

        splitter = QtWidgets.QSplitter(QtCore.Qt.Orientation.Horizontal)
        splitter.setChildrenCollapsible(False)
        layout.addWidget(splitter, 1)

        left_panel = QtWidgets.QFrame()
        left_panel.setObjectName("Panel")
        left_layout = QtWidgets.QVBoxLayout(left_panel)
        left_layout.setContentsMargins(10, 10, 10, 10)
        left_layout.setSpacing(8)
        left_title_row = QtWidgets.QHBoxLayout()
        left_title = QtWidgets.QLabel("资产树 / 历史残留")
        left_title.setObjectName("PanelTitle")
        left_hint = QtWidgets.QLabel("右键执行清理或提取")
        left_hint.setObjectName("MutedText")
        left_title_row.addWidget(left_title)
        left_title_row.addStretch(1)
        left_title_row.addWidget(left_hint)
        left_layout.addLayout(left_title_row)

        self.tree = QtWidgets.QTreeWidget()
        self.tree.setHeaderLabels(["文件 / 历史中存在的文件"])
        self.tree.setContextMenuPolicy(QtCore.Qt.ContextMenuPolicy.CustomContextMenu)
        self.tree.customContextMenuRequested.connect(self._show_file_menu)
        self.tree.itemSelectionChanged.connect(self._on_file_selected)
        self.tree.setUniformRowHeights(True)
        self.tree.setAnimated(True)
        self.tree.setIndentation(18)
        self.tree.setAlternatingRowColors(True)
        self.tree.header().setStretchLastSection(True)
        left_layout.addWidget(self.tree, 1)
        splitter.addWidget(left_panel)

        right_panel = QtWidgets.QFrame()
        right_panel.setObjectName("Panel")
        right_layout = QtWidgets.QVBoxLayout(right_panel)
        right_layout.setContentsMargins(10, 10, 10, 10)
        right_layout.setSpacing(8)
        right_title_row = QtWidgets.QHBoxLayout()
        history_title = QtWidgets.QLabel("历史版本")
        history_title.setObjectName("PanelTitle")
        self.file_title = QtWidgets.QLabel("选择左侧文件查看历史")
        self.file_title.setObjectName("MutedText")
        self.file_title.setTextInteractionFlags(QtCore.Qt.TextInteractionFlag.TextSelectableByMouse)
        right_title_row.addWidget(history_title)
        right_title_row.addSpacing(12)
        right_title_row.addWidget(self.file_title, 1)
        right_layout.addLayout(right_title_row)

        self.table = QtWidgets.QTableWidget(0, 7)
        self.table.setHorizontalHeaderLabels(["Commit", "Date", "Author", "OSS", "Size", "SHA256 / Key", "Subject"])
        self.table.horizontalHeader().setStretchLastSection(True)
        self.table.horizontalHeader().setSectionResizeMode(0, QtWidgets.QHeaderView.ResizeMode.ResizeToContents)
        self.table.horizontalHeader().setSectionResizeMode(1, QtWidgets.QHeaderView.ResizeMode.ResizeToContents)
        self.table.horizontalHeader().setSectionResizeMode(2, QtWidgets.QHeaderView.ResizeMode.ResizeToContents)
        self.table.horizontalHeader().setSectionResizeMode(3, QtWidgets.QHeaderView.ResizeMode.ResizeToContents)
        self.table.horizontalHeader().setSectionResizeMode(4, QtWidgets.QHeaderView.ResizeMode.ResizeToContents)
        self.table.horizontalHeader().setSectionResizeMode(5, QtWidgets.QHeaderView.ResizeMode.Interactive)
        self.table.setColumnWidth(5, 280)
        self.table.setSelectionBehavior(QtWidgets.QAbstractItemView.SelectionBehavior.SelectRows)
        self.table.setSelectionMode(QtWidgets.QAbstractItemView.SelectionMode.SingleSelection)
        self.table.setEditTriggers(QtWidgets.QAbstractItemView.EditTrigger.NoEditTriggers)
        self.table.setContextMenuPolicy(QtCore.Qt.ContextMenuPolicy.CustomContextMenu)
        self.table.customContextMenuRequested.connect(self._show_history_menu)
        self.table.setAlternatingRowColors(True)
        self.table.setShowGrid(False)
        self.table.verticalHeader().setVisible(False)
        right_layout.addWidget(self.table, 1)
        splitter.addWidget(right_panel)
        splitter.setSizes([430, 850])

        legend_row = QtWidgets.QHBoxLayout()
        legend_row.setSpacing(8)
        legend_row.addWidget(self._make_chip("灰色：未知 / 待扫描", "ChipGray"))
        legend_row.addWidget(self._make_chip("绿色：存在于 OSS", "ChipGreen"))
        legend_row.addWidget(self._make_chip("红色：OSS 缺失", "ChipRed"))
        legend_row.addWidget(self._make_chip("黄色文件夹：混合状态", "ChipOrange"))
        legend_row.addStretch(1)
        layout.addLayout(legend_row)

        self.setCentralWidget(central)

        self.status_text = QtWidgets.QLabel("Ready")
        self.status_text.setObjectName("MutedText")
        self.status_progress = QtWidgets.QProgressBar()
        self.status_progress.setMaximumWidth(220)
        self.status_progress.setVisible(False)
        self.statusBar().addPermanentWidget(self.status_text, 1)
        self.statusBar().addPermanentWidget(self.status_progress)
        self._set_idle_status("Ready")

    def _init_icons(self) -> None:
        style = self.style()
        self.folder_icon = style.standardIcon(QtWidgets.QStyle.StandardPixmap.SP_DirIcon)
        self.file_icon = style.standardIcon(QtWidgets.QStyle.StandardPixmap.SP_FileIcon)
        self.meta_icon = style.standardIcon(QtWidgets.QStyle.StandardPixmap.SP_FileDialogDetailedView)
        self.warning_icon = style.standardIcon(QtWidgets.QStyle.StandardPixmap.SP_MessageBoxWarning)

    def _make_chip(self, text: str, object_name: str = "Chip") -> QtWidgets.QLabel:
        label = QtWidgets.QLabel(text)
        label.setObjectName(object_name)
        label.setMinimumHeight(26)
        return label

    def _update_summary_chips(self) -> None:
        total = len(self.files)
        green = red = gray = 0
        for entry in self.files:
            if entry.current_blob.valid and entry.current_exists is True:
                green += 1
            elif entry.current_blob.valid and entry.current_exists is False:
                red += 1
            else:
                gray += 1
        if hasattr(self, "total_chip"):
            self.total_chip.setText(f"资产 {total}")
            self.green_chip.setText(f"绿色 {green}")
            self.red_chip.setText(f"红色 {red}")
            self.gray_chip.setText(f"灰色 {gray}")
            self.unknown_chip.setText(f"未知 Blob {len(self.unknown_blobs)}")

    def _set_progress_status(self, message: str, current: int = 0, total: int = 0) -> None:
        self.statusBar().showMessage(message)
        self.status_text.setText(message)
        self.status_progress.setVisible(True)
        if total > 0:
            self.status_progress.setRange(0, total)
            self.status_progress.setValue(max(0, min(current, total)))
        else:
            # Unknown duration: animated/busy progress bar.
            self.status_progress.setRange(0, 0)

    def _set_idle_status(self, message: str) -> None:
        self.statusBar().showMessage(message)
        self.status_text.setText(message)
        self.status_progress.setVisible(False)

    def _clear_commit_worker(self) -> None:
        self.commit_worker = None
        self.commit_thread = None

    def _clear_file_oss_worker(self) -> None:
        self.file_oss_worker = None
        self.file_oss_thread = None

    def _clear_history_worker(self) -> None:
        self.history_worker = None
        self.history_thread = None

    def _clear_history_oss_worker(self) -> None:
        self.history_oss_worker = None
        self.history_oss_thread = None

    def _clear_unknown_blob_worker(self) -> None:
        self.unknown_blob_worker = None
        self.unknown_blob_thread = None

    def _open_repo(self, path: Path) -> None:
        try:
            self.cfg = AppConfig.load(path)
            self.git = GitBackend.discover(path, self.cfg)
            # Reload config from actual repo root as caller may pass subdir.
            self.cfg = AppConfig.load(self.git.repo_root)
            self.git.cfg = self.cfg
            self.store = create_store(self.git.repo_root, self.cfg)
            self.repo_label.setText(f"Repo  {self.git.repo_root}")
            self.provider_label.setText(f"Store  {self.store.describe()}    Config  {self.cfg.source_path}")
            self.refresh()
        except Exception as exc:
            QtWidgets.QMessageBox.critical(self, "打开失败", str(exc))

    def refresh(self) -> None:
        if not self.git:
            return
        self.refresh_button.setEnabled(False)
        self.tree.clear()
        self.table.setRowCount(0)
        self.file_title.setText("正在扫描commit...")
        self.reference_index = None
        self.files = []
        self.file_by_logical = {}
        self.tree_items_by_logical = {}
        self.current_history = []
        self.history_row_by_commit = {}
        self.unknown_blobs = {}
        self.unknown_blob_root_item = None
        self.unknown_blob_group_item = None
        self._update_summary_chips()

        thread = QtCore.QThread(self)
        worker = CommitScanWorker(self.git)
        worker.moveToThread(thread)
        thread.started.connect(worker.run)
        worker.progress.connect(self._set_progress_status)
        worker.finished.connect(self._commit_scan_finished)
        worker.failed.connect(self._worker_failed)
        worker.finished.connect(thread.quit)
        worker.failed.connect(thread.quit)
        worker.finished.connect(worker.deleteLater)
        worker.failed.connect(worker.deleteLater)
        thread.finished.connect(thread.deleteLater)
        self.commit_thread = thread
        self.commit_worker = worker
        thread.finished.connect(self._clear_commit_worker)
        self._set_progress_status("正在扫描commit...", 0, 0)
        thread.start()

    def _commit_scan_finished(self, files: list) -> None:
        self.files = files
        self.file_by_logical = {f.logical_path: f for f in self.files}
        self._update_summary_chips()
        self._populate_tree()
        self.file_title.setText(f"共 {len(self.files)} 个历史文件，OSS 状态后台扫描中...")
        self._start_file_oss_scan()

    def _start_file_oss_scan(self) -> None:
        if not self.store:
            self.refresh_button.setEnabled(True)
            self._set_idle_status("commit扫描完成；未配置OSS")
            return
        thread = QtCore.QThread(self)
        worker = OssScanWorker(self.files, self.store, self.exists_cache)
        worker.moveToThread(thread)
        thread.started.connect(worker.run)
        worker.progress.connect(self._set_progress_status)
        worker.file_status.connect(self._file_oss_status)
        worker.finished.connect(self._file_oss_finished)
        worker.failed.connect(self._worker_failed)
        worker.finished.connect(thread.quit)
        worker.failed.connect(thread.quit)
        worker.finished.connect(worker.deleteLater)
        worker.failed.connect(worker.deleteLater)
        thread.finished.connect(thread.deleteLater)
        self.file_oss_thread = thread
        self.file_oss_worker = worker
        thread.finished.connect(self._clear_file_oss_worker)
        self._set_progress_status("正在扫描oss...", 0, 0)
        thread.start()

    def _file_oss_status(self, logical: str, exists: object, key: str) -> None:
        if isinstance(exists, bool) and key:
            self.exists_cache[key] = exists
        entry = self.file_by_logical.get(logical)
        if entry is not None:
            entry.current_exists = exists if isinstance(exists, bool) else None
        item = self.tree_items_by_logical.get(logical)
        if item is not None and entry is not None:
            item.setForeground(0, QtGui.QBrush(self._file_color(entry)))
            item.setToolTip(0, self._file_tooltip(entry))
        self._refresh_folder_colors()
        self._update_summary_chips()

    def _file_oss_finished(self, cache: dict) -> None:
        self.exists_cache.update(cache)
        self._refresh_tree_colors_from_cache()
        self._start_unknown_blob_scan()

    def _start_unknown_blob_scan(self) -> None:
        if not self.git or not self.store:
            self.refresh_button.setEnabled(True)
            self._set_idle_status(f"OSS扫描完成：{len(self.files)} 个文件")
            return
        thread = QtCore.QThread(self)
        key_prefix = self.cfg.key_prefix if self.cfg else ""
        worker = UnknownBlobScanWorker(self.files, self.git, self.store, key_prefix)
        worker.moveToThread(thread)
        thread.started.connect(worker.run)
        worker.progress.connect(self._set_progress_status)
        worker.found.connect(self._unknown_blob_found)
        worker.finished.connect(self._unknown_blob_scan_finished)
        worker.failed.connect(self._worker_failed)
        worker.finished.connect(thread.quit)
        worker.failed.connect(thread.quit)
        worker.finished.connect(worker.deleteLater)
        worker.failed.connect(worker.deleteLater)
        thread.finished.connect(thread.deleteLater)
        self.unknown_blob_thread = thread
        self.unknown_blob_worker = worker
        thread.finished.connect(self._clear_unknown_blob_worker)
        self._set_progress_status("正在扫描oss：未知 blob...", 0, 0)
        thread.start()

    def _unknown_blob_found(self, unknown: object) -> None:
        if isinstance(unknown, UnknownBlob):
            if unknown.key in self.unknown_blobs:
                return
            self.unknown_blobs[unknown.key] = unknown
            self._append_unknown_blob_item(unknown)
            self._update_summary_chips()

    def _unknown_blob_scan_finished(self, unknown: list) -> None:
        self.unknown_blobs = {u.key: u for u in unknown if isinstance(u, UnknownBlob)}
        self._rebuild_unknown_blob_nodes()
        self._update_summary_chips()
        self.refresh_button.setEnabled(True)
        self._set_idle_status(f"OSS扫描完成：{len(self.files)} 个文件；未知 blob {len(self.unknown_blobs)} 个")

    def _worker_failed(self, message: str) -> None:
        self.refresh_button.setEnabled(True)
        self._set_idle_status("Failed")
        QtWidgets.QMessageBox.critical(self, "操作失败", message)

    def _populate_tree(self) -> None:
        self.tree.clear()
        self.tree_items_by_logical = {}
        nodes: Dict[str, QtWidgets.QTreeWidgetItem] = {}
        for entry in sorted(self.files, key=lambda e: e.logical_path.lower()):
            parts = entry.logical_path.replace("\\", "/").split("/")
            parent_item = self.tree.invisibleRootItem()
            prefix = ""
            for part in parts[:-1]:
                prefix = f"{prefix}/{part}" if prefix else part
                if prefix not in nodes:
                    item = QtWidgets.QTreeWidgetItem([part])
                    item.setIcon(0, self.folder_icon)
                    item.setForeground(0, QtGui.QBrush(GRAY))
                    parent_item.addChild(item)
                    nodes[prefix] = item
                parent_item = nodes[prefix]
            leaf = QtWidgets.QTreeWidgetItem([parts[-1]])
            leaf.setIcon(0, self.file_icon)
            leaf.setData(0, ROLE_LOGICAL_PATH, entry.logical_path)
            leaf.setToolTip(0, self._file_tooltip(entry))
            leaf.setForeground(0, QtGui.QBrush(self._file_color(entry)))
            parent_item.addChild(leaf)
            self.tree_items_by_logical[entry.logical_path] = leaf
        self._rebuild_unknown_blob_nodes()
        self._refresh_folder_colors()
        self.tree.expandToDepth(1)

    def _ensure_unknown_blob_nodes(self) -> tuple[QtWidgets.QTreeWidgetItem, QtWidgets.QTreeWidgetItem]:
        if self.unknown_blob_root_item is None:
            root = QtWidgets.QTreeWidgetItem(["[Meta]"])
            root.setIcon(0, self.meta_icon)
            root.setForeground(0, QtGui.QBrush(BLUE))
            root.setData(0, ROLE_META_KIND, "meta_root")
            self.tree.invisibleRootItem().addChild(root)
            self.unknown_blob_root_item = root
        if self.unknown_blob_group_item is None:
            group = QtWidgets.QTreeWidgetItem(["未知 Blob (0)"])
            group.setIcon(0, self.warning_icon)
            group.setForeground(0, QtGui.QBrush(ORANGE))
            group.setData(0, ROLE_META_KIND, "unknown_blob_group")
            group.setToolTip(0, "存在于当前项目 OSS/prefix 空间，但没有任何 commit/ref 历史引用的 blob。")
            self.unknown_blob_root_item.addChild(group)
            self.unknown_blob_group_item = group
        return self.unknown_blob_root_item, self.unknown_blob_group_item

    def _rebuild_unknown_blob_nodes(self) -> None:
        if not self.unknown_blobs:
            # Keep an empty meta group visible so the scan result is explicit.
            _, group = self._ensure_unknown_blob_nodes()
            group.takeChildren()
            group.setText(0, "未知 Blob (0)")
            group.setToolTip(0, "没有发现存在于 OSS/prefix 空间但未被 commit 引用的 blob。")
            return
        _, group = self._ensure_unknown_blob_nodes()
        group.takeChildren()
        group.setText(0, f"未知 Blob ({len(self.unknown_blobs)})")
        group.setToolTip(0, "存在于当前项目 OSS/prefix 空间，但没有任何 commit/ref 历史引用的 blob。右键可一键删除。")
        for key, unknown in sorted(self.unknown_blobs.items()):
            label = unknown.sha256[:12] + "…" if unknown.sha256 else key
            item = QtWidgets.QTreeWidgetItem([label])
            item.setIcon(0, self.warning_icon)
            item.setForeground(0, QtGui.QBrush(ORANGE))
            item.setData(0, ROLE_META_KIND, "unknown_blob")
            item.setData(0, ROLE_BLOB_KEY, key)
            item.setToolTip(0, f"未知 blob\nkey: {key}\nsha256: {unknown.sha256 or '-'}\n未被任何 commit/ref 历史引用。")
            group.addChild(item)
        group.setExpanded(True)

    def _append_unknown_blob_item(self, unknown: UnknownBlob) -> None:
        _, group = self._ensure_unknown_blob_nodes()
        group.setText(0, f"未知 Blob ({len(self.unknown_blobs)})")
        group.setToolTip(0, "存在于当前项目 OSS/prefix 空间，但没有任何 commit/ref 历史引用的 blob。右键可一键删除。")
        label = unknown.sha256[:12] + "…" if unknown.sha256 else unknown.key
        item = QtWidgets.QTreeWidgetItem([label])
        item.setIcon(0, self.warning_icon)
        item.setForeground(0, QtGui.QBrush(ORANGE))
        item.setData(0, ROLE_META_KIND, "unknown_blob")
        item.setData(0, ROLE_BLOB_KEY, unknown.key)
        item.setToolTip(0, f"未知 blob\nkey: {unknown.key}\nsha256: {unknown.sha256 or '-'}\n未被任何 commit/ref 历史引用。")
        group.addChild(item)
        group.setExpanded(True)

    def _refresh_folder_colors(self) -> None:
        """Aggregate normal folder colors from child states.

        File leaves keep their direct OSS colors:
        - gray: unknown / pending / invalid ref
        - green: blob exists
        - red: blob missing

        Folder colors are intentionally conservative:
        - green iff every child subtree is green
        - gray iff every child subtree is gray
        - yellow for mixed / problematic / partially-scanned subtrees
        - any yellow child makes the parent yellow

        The [Meta] subtree keeps its own orange color and is not folded into
        Content folder state.
        """
        root = self.tree.invisibleRootItem()
        for i in range(root.childCount()):
            child = root.child(i)
            if child.data(0, ROLE_META_KIND):
                self._restore_meta_colors(child)
                continue
            self._folder_state(child)

    def _restore_meta_colors(self, item: QtWidgets.QTreeWidgetItem) -> None:
        kind = item.data(0, ROLE_META_KIND)
        if kind == "meta_root":
            item.setForeground(0, QtGui.QBrush(BLUE))
        elif kind:
            item.setForeground(0, QtGui.QBrush(ORANGE))
        for i in range(item.childCount()):
            self._restore_meta_colors(item.child(i))

    def _folder_state(self, item: QtWidgets.QTreeWidgetItem) -> str:
        if item.data(0, ROLE_META_KIND):
            self._restore_meta_colors(item)
            return "yellow"

        logical = item.data(0, ROLE_LOGICAL_PATH)
        if logical:
            entry = self.file_by_logical.get(str(logical))
            if entry is None or not entry.current_blob.valid or entry.current_exists is None:
                return "gray"
            if entry.current_exists is True:
                return "green"
            return "red"

        child_states: list[str] = []
        for i in range(item.childCount()):
            child = item.child(i)
            if child.data(0, ROLE_META_KIND):
                self._restore_meta_colors(child)
                continue
            child_states.append(self._folder_state(child))

        if not child_states:
            state = "gray"
        elif all(s == "green" for s in child_states):
            state = "green"
        elif all(s == "gray" for s in child_states):
            state = "gray"
        else:
            state = "yellow"

        item.setForeground(0, QtGui.QBrush(self._folder_color_from_state(state)))
        item.setToolTip(0, self._folder_tooltip_from_state(state))
        return state

    def _folder_color_from_state(self, state: str) -> QtGui.QColor:
        if state == "green":
            return GREEN
        if state == "gray":
            return GRAY
        return YELLOW

    def _folder_tooltip_from_state(self, state: str) -> str:
        if state == "green":
            return "文件夹状态：全部子项目存在于 OSS。"
        if state == "gray":
            return "文件夹状态：全部子项目未知/待扫描/无可解析对象。"
        return "文件夹状态：混合/部分缺失/部分待扫描；存在黄色子项目会向上传播为黄色。"

    def _file_color(self, entry: FileEntry) -> QtGui.QColor:
        if not entry.current_blob.valid:
            return GRAY
        if entry.current_exists is True:
            return GREEN
        if entry.current_exists is False:
            return RED
        return GRAY

    def _file_tooltip(self, entry: FileEntry) -> str:
        status = "in HEAD" if entry.in_head else "deleted from HEAD, exists in history"
        exists = "unknown/pending" if entry.current_exists is None else str(entry.current_exists)
        return f"{entry.logical_path}\nref: {entry.ref_path}\n{status}\nOSS exists: {exists}\nkey: {entry.current_blob.key}\nsha256: {entry.current_blob.sha256}"

    def _selected_logical_path(self) -> Optional[str]:
        item = self.tree.currentItem()
        if not item:
            return None
        if item.data(0, ROLE_META_KIND):
            return None
        value = item.data(0, ROLE_LOGICAL_PATH)
        return str(value) if value else None

    def _on_file_selected(self) -> None:
        item = self.tree.currentItem()
        if item and item.data(0, ROLE_META_KIND):
            self._show_meta_selection(item)
            return
        logical = self._selected_logical_path()
        if not logical or not self.git:
            return
        self.file_title.setText(logical)
        self.table.setRowCount(0)
        self.current_history = []
        self.history_row_by_commit = {}

        thread = QtCore.QThread(self)
        worker = HistoryLoadWorker(logical, self.git)
        worker.moveToThread(thread)
        thread.started.connect(worker.run)
        worker.progress.connect(self._set_progress_status)
        worker.finished.connect(self._history_loaded)
        worker.failed.connect(self._worker_failed)
        worker.finished.connect(thread.quit)
        worker.failed.connect(thread.quit)
        worker.finished.connect(worker.deleteLater)
        worker.failed.connect(worker.deleteLater)
        thread.finished.connect(thread.deleteLater)
        self.history_thread = thread
        self.history_worker = worker
        thread.finished.connect(self._clear_history_worker)
        self._set_progress_status(f"正在扫描commit：{logical}", 0, 0)
        thread.start()

    def _history_loaded(self, logical: str, history: list) -> None:
        if logical != self._selected_logical_path():
            return
        self.current_history = history
        self.history_row_by_commit = {h.commit: i for i, h in enumerate(history)}
        if logical in self.file_by_logical:
            self.file_by_logical[logical].history_count = len(history)
        self._populate_history(history)
        self.file_title.setText(f"{logical}  |  {len(history)} 个提交，OSS 状态后台扫描中...")
        self._start_history_oss_scan(logical, history)

    def _start_history_oss_scan(self, logical: str, history: list) -> None:
        if not self.store:
            self._set_idle_status(f"History loaded: {len(history)} commits")
            return
        thread = QtCore.QThread(self)
        worker = HistoryOssWorker(logical, history, self.store, self.exists_cache)
        worker.moveToThread(thread)
        thread.started.connect(worker.run)
        worker.progress.connect(self._set_progress_status)
        worker.history_status.connect(self._history_oss_status)
        worker.finished.connect(self._history_oss_finished)
        worker.failed.connect(self._worker_failed)
        worker.finished.connect(thread.quit)
        worker.failed.connect(thread.quit)
        worker.finished.connect(worker.deleteLater)
        worker.failed.connect(worker.deleteLater)
        thread.finished.connect(thread.deleteLater)
        self.history_oss_thread = thread
        self.history_oss_worker = worker
        thread.finished.connect(self._clear_history_oss_worker)
        self._set_progress_status("正在扫描oss：历史版本...", 0, 0)
        thread.start()

    def _history_oss_status(self, logical: str, commit: str, exists: object, key: str) -> None:
        if logical != self._selected_logical_path():
            return
        if isinstance(exists, bool) and key:
            self.exists_cache[key] = exists
        row = self.history_row_by_commit.get(commit)
        if row is None or row >= len(self.current_history):
            return
        h = self.current_history[row]
        h.exists = exists if isinstance(exists, bool) else None
        self._update_history_row(row, h)

    def _history_oss_finished(self, logical: str, cache: dict) -> None:
        self.exists_cache.update(cache)
        if logical != self._selected_logical_path():
            return
        self._set_idle_status(f"历史OSS扫描完成：{len(self.current_history)} commits")
        # File tree may benefit from cache populated by history scan.
        self._refresh_tree_colors_from_cache()

    def _populate_history(self, history: List[HistoryEntry]) -> None:
        self.table.setRowCount(len(history))
        self.history_row_by_commit = {}
        for row, h in enumerate(history):
            self.history_row_by_commit[h.commit] = row
            key = blob_cache_key(h.blob)
            if key in self.exists_cache:
                h.exists = self.exists_cache[key]
            self._update_history_row(row, h)
        self.table.resizeRowsToContents()

    def _update_history_row(self, row: int, h: HistoryEntry) -> None:
        exists_text = "存在" if h.exists is True else "缺失" if h.exists is False else "未知/待扫描"
        size_text = "" if h.blob.size is None else str(h.blob.size)
        key_text = h.blob.sha256 or h.blob.key
        if h.blob.sha256 and h.blob.key:
            key_text = f"{h.blob.sha256[:12]}…  {h.blob.key}"
        values = [h.short_commit, h.date, h.author, exists_text, size_text, key_text, h.subject]
        for col, value in enumerate(values):
            item = self.table.item(row, col)
            if item is None:
                item = QtWidgets.QTableWidgetItem(value)
                self.table.setItem(row, col, item)
            else:
                item.setText(value)
            if col == 0:
                item.setData(ROLE_ROW_INDEX, row)
                item.setToolTip(h.commit)
            item.setForeground(QtGui.QBrush(self._history_color(h)))

    def _history_color(self, h: HistoryEntry) -> QtGui.QColor:
        if not h.blob.valid:
            return GRAY
        if h.exists is True:
            return GREEN
        if h.exists is False:
            return RED
        return GRAY

    def _show_meta_selection(self, item: QtWidgets.QTreeWidgetItem) -> None:
        kind = item.data(0, ROLE_META_KIND)
        self.current_history = []
        self.history_row_by_commit = {}
        self.table.setRowCount(0)
        if kind == "unknown_blob_group":
            self.file_title.setText(f"[Meta] 未知 Blob：{len(self.unknown_blobs)} 个，可右键一键删除")
            self.table.setRowCount(len(self.unknown_blobs))
            for row, unknown in enumerate(sorted(self.unknown_blobs.values(), key=lambda u: u.key)):
                values = ["META", "", "", "存在", "", unknown.sha256 or unknown.key, "未知 Blob：OSS 中存在，但无 commit/ref 引用"]
                for col, value in enumerate(values):
                    cell = QtWidgets.QTableWidgetItem(value)
                    cell.setForeground(QtGui.QBrush(ORANGE))
                    cell.setToolTip(unknown.key)
                    self.table.setItem(row, col, cell)
            return
        if kind == "unknown_blob":
            key = str(item.data(0, ROLE_BLOB_KEY) or "")
            unknown = self.unknown_blobs.get(key)
            self.file_title.setText(f"[Meta] 未知 Blob：{key}")
            self.table.setRowCount(1)
            if unknown:
                values = ["META", "", "", "存在", "", unknown.sha256 or unknown.key, "未知 Blob：OSS 中存在，但无 commit/ref 引用"]
                for col, value in enumerate(values):
                    cell = QtWidgets.QTableWidgetItem(value)
                    cell.setForeground(QtGui.QBrush(ORANGE))
                    cell.setToolTip(unknown.key)
                    self.table.setItem(0, col, cell)
            return
        self.file_title.setText("[Meta]")

    def _show_file_menu(self, pos: QtCore.QPoint) -> None:
        item = self.tree.itemAt(pos)
        if item is not None:
            self.tree.setCurrentItem(item)
            meta_kind = item.data(0, ROLE_META_KIND)
            if meta_kind:
                self._show_meta_menu(item, pos)
                return
        logical = self._selected_logical_path()
        if not logical:
            return
        menu = QtWidgets.QMenu(self)
        act_delete = menu.addAction("删除文件")
        act_keep = menu.addAction("仅保留当前版本")
        act_extract = menu.addAction("提取当前版本到...")
        action = menu.exec(self.tree.viewport().mapToGlobal(pos))
        if action == act_delete:
            self._delete_file(logical)
        elif action == act_keep:
            self._keep_current_only(logical)
        elif action == act_extract:
            self._extract_current(logical)

    def _show_history_menu(self, pos: QtCore.QPoint) -> None:
        row = self.table.rowAt(pos.y())
        if row < 0 or row >= len(self.current_history):
            return
        self.table.selectRow(row)
        menu = QtWidgets.QMenu(self)
        act_delete = menu.addAction("删除选中历史 OSS blob")
        act_extract = menu.addAction("提取到...")
        action = menu.exec(self.table.viewport().mapToGlobal(pos))
        if action == act_delete:
            self._delete_history_blob(row)
        elif action == act_extract:
            self._extract_history(row)

    def _show_meta_menu(self, item: QtWidgets.QTreeWidgetItem, pos: QtCore.QPoint) -> None:
        kind = item.data(0, ROLE_META_KIND)
        menu = QtWidgets.QMenu(self)
        if kind == "unknown_blob_group":
            act_delete_all = menu.addAction("一键删除所有未知 Blob")
            action = menu.exec(self.tree.viewport().mapToGlobal(pos))
            if action == act_delete_all:
                self._delete_unknown_blobs(list(self.unknown_blobs.values()))
            return
        if kind == "unknown_blob":
            key = str(item.data(0, ROLE_BLOB_KEY) or "")
            unknown = self.unknown_blobs.get(key)
            act_delete = menu.addAction("删除这个未知 Blob")
            act_delete_all = menu.addAction("一键删除所有未知 Blob")
            action = menu.exec(self.tree.viewport().mapToGlobal(pos))
            if action == act_delete and unknown:
                self._delete_unknown_blobs([unknown])
            elif action == act_delete_all:
                self._delete_unknown_blobs(list(self.unknown_blobs.values()))

    def _delete_unknown_blobs(self, items: list[UnknownBlob]) -> None:
        if not self.store or not items:
            return
        msg = f"将从 OSS 删除 {len(items)} 个未知 blob。\n\n这些对象存在于当前项目 prefix 空间，但没有任何 commit/ref 历史引用。"
        if len(items) <= 5:
            msg += "\n\n" + "\n".join(u.key for u in items)
        else:
            msg += "\n\n" + "\n".join(u.key for u in items[:5]) + f"\n... and {len(items)-5} more"
        reply = QtWidgets.QMessageBox.question(self, "确认删除未知 Blob", msg)
        if reply != QtWidgets.QMessageBox.StandardButton.Yes:
            return
        deleted = 0
        failed: list[str] = []
        QtWidgets.QApplication.setOverrideCursor(QtCore.Qt.CursorShape.WaitCursor)
        try:
            total = len(items)
            for i, unknown in enumerate(items, 1):
                self._set_progress_status(f"正在删除未知 blob：{i}/{total}", i, total)
                try:
                    self.store.delete(unknown.blob)
                    self.unknown_blobs.pop(unknown.key, None)
                    self.exists_cache[unknown.key] = False
                    deleted += 1
                except Exception as exc:
                    failed.append(f"{unknown.key}: {exc}")
        finally:
            QtWidgets.QApplication.restoreOverrideCursor()
        self._rebuild_unknown_blob_nodes()
        self._update_summary_chips()
        self._set_idle_status(f"未知 blob 删除完成：删除 {deleted} 个，失败 {len(failed)} 个")
        text = f"删除 {deleted} 个；失败 {len(failed)} 个。"
        if failed:
            text += "\n\n" + "\n".join(failed[:10])
        QtWidgets.QMessageBox.information(self, "删除完成", text)

    def _delete_file(self, logical: str) -> None:
        if not self.git:
            return
        reply = QtWidgets.QMessageBox.question(
            self,
            "确认删除文件",
            f"删除工作区文件和当前 ref？\n\n{logical}\n\n不会删除 OSS 历史 blob，也不会自动提交 Git。",
        )
        if reply != QtWidgets.QMessageBox.StandardButton.Yes:
            return
        try:
            removed = self.git.remove_working_file_and_ref(logical)
            QtWidgets.QMessageBox.information(self, "已删除", "删除：\n" + "\n".join(removed or ["没有找到可删除的工作区文件/ref"]))
            self.refresh()
        except Exception as exc:
            QtWidgets.QMessageBox.critical(self, "删除失败", str(exc))

    def _extract_current(self, logical: str) -> None:
        entry = self.file_by_logical.get(logical)
        if not entry or not entry.current_blob.valid:
            QtWidgets.QMessageBox.warning(self, "无法提取", "当前/最近版本没有可解析的 OSS blob。")
            return
        default = Path(logical).name
        target, _ = QtWidgets.QFileDialog.getSaveFileName(self, "提取当前版本到", default)
        if target:
            self._download_blob(entry.current_blob, Path(target))

    def _extract_history(self, row: int) -> None:
        h = self.current_history[row]
        if not h.blob.valid:
            QtWidgets.QMessageBox.warning(self, "无法提取", "该历史版本没有可解析的 OSS blob。")
            return
        default = f"{Path(h.logical_path).name}.{h.short_commit}"
        target, _ = QtWidgets.QFileDialog.getSaveFileName(self, "提取历史版本到", default)
        if target:
            self._download_blob(h.blob, Path(target))

    def _download_blob(self, blob: BlobRef, target: Path) -> None:
        if not self.store:
            return
        try:
            self.store.download(blob, target)
            QtWidgets.QMessageBox.information(self, "提取完成", str(target))
        except Exception as exc:
            QtWidgets.QMessageBox.critical(self, "提取失败", str(exc))

    def _delete_history_blob(self, row: int) -> None:
        if not self.store:
            return
        h = self.current_history[row]
        if not h.blob.valid:
            QtWidgets.QMessageBox.warning(self, "无法删除", "该历史版本没有可解析的 OSS blob。")
            return
        key = blob_cache_key(h.blob)
        ref_count = self._reference_count(key)
        extra = ""
        if ref_count > 1:
            extra = f"\n\n警告：扫描到该 blob 被 {ref_count} 个历史记录引用。删除会影响这些版本恢复。"
        reply = QtWidgets.QMessageBox.question(
            self,
            "确认删除历史 blob",
            f"删除 OSS blob？\n\nfile: {h.logical_path}\ncommit: {h.short_commit}\nkey: {h.blob.key}\nsha256: {h.blob.sha256}{extra}",
        )
        if reply != QtWidgets.QMessageBox.StandardButton.Yes:
            return
        try:
            self.store.delete(h.blob)
            self.exists_cache[key] = False
            h.exists = False
            self._update_history_row(row, h)
            self._refresh_tree_colors_from_cache()
            QtWidgets.QMessageBox.information(self, "已删除", h.blob.key or h.blob.sha256)
        except Exception as exc:
            QtWidgets.QMessageBox.critical(self, "删除失败", str(exc))

    def _keep_current_only(self, logical: str) -> None:
        if not self.git or not self.store:
            return
        entry = self.file_by_logical.get(logical)
        if not entry or not entry.current_blob.valid:
            QtWidgets.QMessageBox.warning(self, "无法执行", "该文件没有当前/最近 OSS 版本。")
            return
        current_key = blob_cache_key(entry.current_blob)
        try:
            history = self.git.history_for_file(logical)
            ref_index = self._build_reference_index_sync()
            current_keys = {blob_cache_key(f.current_blob) for f in self.files if f.current_blob.valid}
            candidates: Dict[str, BlobRef] = {}
            skipped_shared: list[str] = []
            for h in history:
                k = blob_cache_key(h.blob)
                if not k or k == current_key:
                    continue
                if k in current_keys:
                    skipped_shared.append(k)
                    continue
                refs = ref_index.get(k, [])
                if any(ref_logical != logical for ref_logical, _ in refs):
                    skipped_shared.append(k)
                    continue
                candidates[k] = h.blob
            if not candidates:
                QtWidgets.QMessageBox.information(self, "无需清理", "没有可安全删除的旧版本 blob。")
                return
            msg = f"将删除 {len(candidates)} 个旧版本 blob，只保留当前/最近版本。"
            if skipped_shared:
                msg += f"\n\n有 {len(set(skipped_shared))} 个 blob 被当前版本或其它文件引用，已自动跳过。"
            msg += "\n\n该操作不会修改 Git 历史，但旧版本可能无法从 OSS 恢复。"
            reply = QtWidgets.QMessageBox.question(self, "确认仅保留当前版本", msg)
            if reply != QtWidgets.QMessageBox.StandardButton.Yes:
                return
            deleted = 0
            missing = 0
            failed: list[str] = []
            for k, blob in candidates.items():
                try:
                    exists = self.store.exists(blob)
                    if exists is False:
                        missing += 1
                        self.exists_cache[k] = False
                        continue
                    self.store.delete(blob)
                    self.exists_cache[k] = False
                    deleted += 1
                except Exception as exc:
                    failed.append(f"{k}: {exc}")
            self._on_file_selected()
            self._refresh_tree_colors_from_cache()
            text = f"删除 {deleted} 个；已缺失 {missing} 个；失败 {len(failed)} 个。"
            if failed:
                text += "\n\n" + "\n".join(failed[:10])
            QtWidgets.QMessageBox.information(self, "清理完成", text)
        except Exception as exc:
            QtWidgets.QMessageBox.critical(self, "清理失败", str(exc))

    def _reference_count(self, key: str) -> int:
        if not key:
            return 0
        index = self._build_reference_index_sync()
        return len(index.get(key, []))

    def _build_reference_index_sync(self) -> dict[str, list[tuple[str, str]]]:
        if self.reference_index is not None:
            return self.reference_index
        if not self.git:
            return {}
        QtWidgets.QApplication.setOverrideCursor(QtCore.Qt.CursorShape.WaitCursor)
        self._set_progress_status("Building reference index...", 0, 0)
        try:
            index: dict[str, list[tuple[str, str]]] = {}
            refs = sorted(self.git.list_all_ref_paths())
            total = max(1, len(refs))
            for i, ref_path in enumerate(refs, 1):
                self._set_progress_status(f"Building reference index {i}/{total}", i, total)
                logical = self.git.ref_to_logical(ref_path)
                if not logical:
                    continue
                for h in self.git.history_for_file(logical):
                    k = blob_cache_key(h.blob)
                    if k:
                        index.setdefault(k, []).append((logical, h.commit))
            self.reference_index = index
            self._set_idle_status("Reference index ready")
            return index
        finally:
            QtWidgets.QApplication.restoreOverrideCursor()

    def _refresh_tree_colors_from_cache(self) -> None:
        for entry in self.files:
            if entry.current_blob.valid:
                k = blob_cache_key(entry.current_blob)
                if k in self.exists_cache:
                    entry.current_exists = self.exists_cache[k]
        for logical, item in self.tree_items_by_logical.items():
            entry = self.file_by_logical.get(logical)
            if entry is not None:
                item.setForeground(0, QtGui.QBrush(self._file_color(entry)))
                item.setToolTip(0, self._file_tooltip(entry))
        self._refresh_folder_colors()


def run_app(path: Path) -> int:
    app = QtWidgets.QApplication(sys.argv)
    app.setApplicationName("OSS Manager")
    win = MainWindow(path)
    win.show()
    return app.exec()
