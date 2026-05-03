from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional


@dataclass(frozen=True)
class BlobRef:
    sha256: str = ""
    key: str = ""
    size: Optional[int] = None
    raw_format: str = "unknown"
    parse_error: str = ""

    @property
    def valid(self) -> bool:
        return bool(self.sha256 or self.key)


@dataclass
class HistoryEntry:
    logical_path: str
    ref_path: str
    commit: str
    short_commit: str
    date: str
    author: str
    subject: str
    blob: BlobRef = field(default_factory=BlobRef)
    exists: Optional[bool] = None
    ref_content_missing: bool = False


@dataclass
class FileEntry:
    logical_path: str
    ref_path: str
    in_head: bool = False
    in_worktree: bool = False
    current_blob: BlobRef = field(default_factory=BlobRef)
    current_exists: Optional[bool] = None
    history_count: int = 0

    @property
    def display_name(self) -> str:
        return Path(self.logical_path).name

    @property
    def parent_parts(self) -> tuple[str, ...]:
        parts = Path(self.logical_path).parts
        return tuple(parts[:-1])


@dataclass(frozen=True)
class UnknownBlob:
    """An object that exists in the configured OSS/prefix space but is not
    referenced by any scanned GameDepot commit/ref history.
    """

    key: str
    sha256: str = ""
    size: Optional[int] = None

    @property
    def blob(self) -> BlobRef:
        return BlobRef(sha256=self.sha256, key=self.key, size=self.size, raw_format="oss-list")
