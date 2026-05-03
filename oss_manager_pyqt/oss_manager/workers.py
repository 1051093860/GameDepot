from __future__ import annotations

import re
from typing import Dict, List, Optional

from .models import BlobRef, FileEntry, HistoryEntry, UnknownBlob
from .qt_compat import QtCore, Signal


def normalize_object_key(key: str) -> str:
    return str(key or "").replace("\\", "/").strip().lstrip("/")


_SHA_BLOB_RE = re.compile(r"(?:^|/)sha256/[0-9a-fA-F]{2}/[0-9a-fA-F]{2}/([0-9a-fA-F]{64})\.blob$")


def sha_from_blob_key(key: str) -> str:
    m = _SHA_BLOB_RE.search(normalize_object_key(key))
    return m.group(1).lower() if m else ""


def blob_cache_key(blob: BlobRef) -> str:
    return normalize_object_key(blob.key) or str(blob.sha256 or "").strip().lower()


class CommitScanWorker(QtCore.QObject):
    """Scan Git history/ref files only. Does not touch OSS/network."""

    progress = Signal(str, int, int)
    finished = Signal(list)
    failed = Signal(str)

    def __init__(self, git):
        super().__init__()
        self.git = git

    def run(self) -> None:
        try:
            self.progress.emit("正在扫描commit：收集历史 ref 文件...", 0, 0)
            files: List[FileEntry] = self.git.build_file_entries(lambda m, c, t: self.progress.emit(m, c, t))
            self.progress.emit(f"正在扫描commit：完成，共 {len(files)} 个文件", len(files), len(files))
            self.finished.emit(files)
        except Exception as exc:
            self.failed.emit(str(exc))


class OssScanWorker(QtCore.QObject):
    """Check current/latest object presence in OSS in the background."""

    progress = Signal(str, int, int)
    file_status = Signal(str, object, str)  # logical_path, exists, cache_key
    finished = Signal(dict)
    failed = Signal(str)

    def __init__(self, files: List[FileEntry], store, exists_cache: Optional[Dict[str, bool]] = None):
        super().__init__()
        self.files = files
        self.store = store
        self.exists_cache: Dict[str, bool] = dict(exists_cache or {})

    def run(self) -> None:
        try:
            pending = [f for f in self.files if f.current_blob.valid]
            total = len(pending)
            if total == 0:
                self.progress.emit("正在扫描oss：没有可检查的对象", 0, 0)
                self.finished.emit(self.exists_cache)
                return

            for i, entry in enumerate(pending, 1):
                blob = entry.current_blob
                key = blob_cache_key(blob)
                self.progress.emit(f"正在扫描oss：{i}/{total}  {entry.logical_path}", i, total)
                if key in self.exists_cache:
                    exists = self.exists_cache[key]
                else:
                    val = self.store.exists(blob)
                    exists = val
                    if val is not None:
                        self.exists_cache[key] = val
                self.file_status.emit(entry.logical_path, exists, key)
            self.finished.emit(self.exists_cache)
        except Exception as exc:
            self.failed.emit(str(exc))


class HistoryLoadWorker(QtCore.QObject):
    """Load Git commit history for one file only. OSS state stays unknown."""

    progress = Signal(str, int, int)
    finished = Signal(str, list)
    failed = Signal(str)

    def __init__(self, logical_path: str, git):
        super().__init__()
        self.logical_path = logical_path
        self.git = git

    def run(self) -> None:
        try:
            self.progress.emit(f"正在扫描commit：{self.logical_path}", 0, 0)
            history: List[HistoryEntry] = self.git.history_for_file(self.logical_path, lambda m, c, t: self.progress.emit(m, c, t))
            self.progress.emit(f"正在扫描commit：历史完成，共 {len(history)} 个提交", len(history), len(history))
            self.finished.emit(self.logical_path, history)
        except Exception as exc:
            self.failed.emit(str(exc))


class HistoryOssWorker(QtCore.QObject):
    """Check historical blob presence in OSS in the background."""

    progress = Signal(str, int, int)
    history_status = Signal(str, str, object, str)  # logical_path, commit, exists, cache_key
    finished = Signal(str, dict)
    failed = Signal(str)

    def __init__(self, logical_path: str, history: List[HistoryEntry], store, exists_cache: Optional[Dict[str, bool]] = None):
        super().__init__()
        self.logical_path = logical_path
        self.history = history
        self.store = store
        self.exists_cache: Dict[str, bool] = dict(exists_cache or {})

    def run(self) -> None:
        try:
            pending = [h for h in self.history if h.blob.valid]
            total = len(pending)
            if total == 0:
                self.progress.emit("正在扫描oss：该文件没有可检查的历史对象", 0, 0)
                self.finished.emit(self.logical_path, self.exists_cache)
                return
            for i, item in enumerate(pending, 1):
                blob = item.blob
                key = blob_cache_key(blob)
                self.progress.emit(f"正在扫描oss：历史 {i}/{total}", i, total)
                if key in self.exists_cache:
                    exists = self.exists_cache[key]
                else:
                    val = self.store.exists(blob)
                    exists = val
                    if val is not None:
                        self.exists_cache[key] = val
                self.history_status.emit(self.logical_path, item.commit, exists, key)
            self.finished.emit(self.logical_path, self.exists_cache)
        except Exception as exc:
            self.failed.emit(str(exc))


# Backward-compatible aliases for older imports/scripts.
ScanWorker = CommitScanWorker
HistoryWorker = HistoryLoadWorker


class ReferenceIndexWorker(QtCore.QObject):
    progress = Signal(str)
    finished = Signal(dict)
    failed = Signal(str)

    def __init__(self, git):
        super().__init__()
        self.git = git

    def run(self) -> None:
        try:
            index: dict[str, list[tuple[str, str]]] = {}
            refs = sorted(self.git.list_all_ref_paths())
            total = max(1, len(refs))
            for i, ref_path in enumerate(refs, 1):
                logical = self.git.ref_to_logical(ref_path)
                if not logical:
                    continue
                self.progress.emit(f"Building reference index {i}/{total}")
                for h in self.git.history_for_file(logical):
                    key = blob_cache_key(h.blob)
                    if key:
                        index.setdefault(key, []).append((logical, h.commit))
            self.finished.emit(index)
        except Exception as exc:
            self.failed.emit(str(exc))


class UnknownBlobScanWorker(QtCore.QObject):
    """List objects in the configured OSS/prefix space and find objects that are
    not referenced by any GameDepot commit/ref history.
    """

    progress = Signal(str, int, int)
    found = Signal(object)  # UnknownBlob
    finished = Signal(list)
    failed = Signal(str)

    def __init__(self, files: List[FileEntry], git, store, key_prefix: str = ""):
        super().__init__()
        self.files = files
        self.git = git
        self.store = store
        self.key_prefix = normalize_object_key(key_prefix)

    def run(self) -> None:
        try:
            referenced: set[str] = set()
            total_files = len(self.files)
            self.progress.emit("正在扫描commit：建立 blob 引用索引 0/%d" % total_files, 0, total_files)
            for i, entry in enumerate(self.files, 1):
                # Protect the current/latest visible blob too. This avoids deleting
                # a freshly uploaded but not-yet-committed object.
                k = blob_cache_key(entry.current_blob)
                if k:
                    referenced.add(k)
                for h in self.git.history_for_file(entry.logical_path):
                    k = blob_cache_key(h.blob)
                    if k:
                        referenced.add(k)
                if i == 1 or i % 25 == 0 or i == total_files:
                    self.progress.emit(f"正在扫描commit：建立 blob 引用索引 {i}/{total_files}", i, total_files)

            self.progress.emit("正在扫描oss：列出 prefix 空间对象...", 0, 0)
            keys = [normalize_object_key(k) for k in self.store.list_objects(self.key_prefix)]
            keys = sorted(k for k in keys if k)
            total = len(keys)
            unknown: list[UnknownBlob] = []
            for i, key in enumerate(keys, 1):
                # Only classify GameDepot blob-layout objects as unknown blobs.
                # Non-blob metadata or unrelated files in the same prefix are ignored.
                if not sha_from_blob_key(key):
                    continue
                if key not in referenced:
                    item = UnknownBlob(key=key, sha256=sha_from_blob_key(key))
                    unknown.append(item)
                    self.found.emit(item)
                if i == 1 or i % 100 == 0 or i == total:
                    self.progress.emit(f"正在扫描oss：未知 blob {i}/{total}", i, total)
            self.finished.emit(unknown)
        except Exception as exc:
            self.failed.emit(str(exc))
