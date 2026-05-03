from __future__ import annotations

import subprocess
import sys
from pathlib import Path
from typing import Callable, Iterable, List, Optional, Set

from .config import AppConfig
from .models import FileEntry, HistoryEntry
from .ref_parser import parse_blob_ref

ProgressCallback = Callable[[str, int, int], None]


class GitError(RuntimeError):
    pass


class GitBackend:
    def __init__(self, repo_root: Path, cfg: AppConfig):
        self.repo_root = repo_root.resolve()
        self.cfg = cfg

    @classmethod
    def discover(cls, path: Path, cfg: AppConfig) -> "GitBackend":
        path = path.resolve()
        proc = subprocess.run(
            ["git", "-C", str(path), "rev-parse", "--show-toplevel"],
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            encoding="utf-8",
            errors="replace",
            **cls._subprocess_ui_kwargs(),
        )
        if proc.returncode != 0:
            raise GitError(proc.stderr.strip() or f"not a git repository: {path}")
        return cls(Path(proc.stdout.strip()), cfg)

    def git(self, args: Iterable[str], check: bool = True) -> str:
        proc = subprocess.run(
            ["git", "-C", str(self.repo_root), *args],
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            encoding="utf-8",
            errors="replace",
            **self._subprocess_ui_kwargs(),
        )
        if check and proc.returncode != 0:
            raise GitError(proc.stderr.strip() or "git failed")
        return proc.stdout

    def ref_to_logical(self, ref_path: str) -> Optional[str]:
        p = ref_path.replace("\\", "/").lstrip("/")
        root = self.cfg.ref_root.replace("\\", "/").strip("/")
        suffix = self.cfg.ref_suffix
        if root and not p.startswith(root + "/"):
            return None
        rel = p[len(root):].lstrip("/") if root else p
        if suffix and not rel.endswith(suffix):
            return None
        if suffix:
            rel = rel[: -len(suffix)]
        logical = (self.cfg.logical_prefix.strip("/") + "/" + rel) if self.cfg.logical_prefix else rel
        return logical.replace("\\", "/")

    def logical_to_ref(self, logical_path: str) -> str:
        logical = logical_path.replace("\\", "/").lstrip("/")
        prefix = self.cfg.logical_prefix.strip("/")
        if prefix and logical.startswith(prefix + "/"):
            logical = logical[len(prefix) + 1:]
        root = self.cfg.ref_root.replace("\\", "/").strip("/")
        return f"{root}/{logical}{self.cfg.ref_suffix}" if root else f"{logical}{self.cfg.ref_suffix}"

    def _emit(self, cb: Optional[ProgressCallback], message: str, current: int = 0, total: int = 0) -> None:
        if cb:
            cb(message, current, total)

    def _commit_count(self) -> int:
        out = self.git(["rev-list", "--all", "--count"], check=False).strip()
        try:
            return max(0, int(out))
        except Exception:
            return 0

    def _iter_log_name_status(self, progress: Optional[ProgressCallback] = None) -> tuple[Set[str], dict[str, str]]:
        """Return (all historical refs, newest commit with actual ref content).

        This replaces the previous N-files x git-log implementation.  We scan Git's
        name-status stream once, which gives real progress by commit and avoids
        running one `git log` per deleted file.
        """
        total_commits = self._commit_count()
        marker = "__GD_COMMIT__"
        cmd = [
            "git", "-C", str(self.repo_root),
            "log", "--all", "--name-status", f"--format=format:{marker}%H",
        ]
        proc = subprocess.Popen(
            cmd,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            encoding="utf-8",
            errors="replace",
            **self._subprocess_ui_kwargs(),
        )
        assert proc.stdout is not None

        paths: Set[str] = set()
        latest_content_commit: dict[str, str] = {}
        current_commit = ""
        commit_i = 0
        last_emit = 0
        self._emit(progress, "姝ｅ湪鎵弿commit锛氳鍙?Git 鏃ュ織...", 0, total_commits)

        for raw in proc.stdout:
            line = raw.rstrip("\n")
            if not line:
                continue
            if line.startswith(marker):
                current_commit = line[len(marker):].strip()
                commit_i += 1
                # Emitting on every commit in a huge repo can itself become slow.
                if commit_i == 1 or commit_i - last_emit >= 25 or commit_i == total_commits:
                    self._emit(progress, f"姝ｅ湪鎵弿commit锛歿commit_i}/{total_commits or '?'}", commit_i, total_commits)
                    last_emit = commit_i
                continue

            parts = line.split("\t")
            if not parts or not current_commit:
                continue
            status = parts[0]
            status_head = status[:1]

            candidate_paths: list[tuple[str, bool]] = []  # (path, has_content_at_current_commit)
            if status_head in {"R", "C"} and len(parts) >= 3:
                old_path = parts[-2].replace("\\", "/")
                new_path = parts[-1].replace("\\", "/")
                candidate_paths.append((old_path, False))
                candidate_paths.append((new_path, True))
            elif len(parts) >= 2:
                p = parts[-1].replace("\\", "/")
                candidate_paths.append((p, status_head != "D"))

            for path, has_content in candidate_paths:
                if self.ref_to_logical(path):
                    paths.add(path)
                    if has_content and path not in latest_content_commit:
                        latest_content_commit[path] = current_commit

        stderr = proc.stderr.read() if proc.stderr is not None else ""
        ret = proc.wait()
        if ret != 0:
            # Empty new repositories can fail to log; treat as no historical refs.
            if "does not have any commits" not in stderr and "bad default revision" not in stderr:
                raise GitError(stderr.strip() or "git log failed")
        self._emit(progress, f"姝ｅ湪鎵弿commit锛欸it 鏃ュ織瀹屾垚锛屽彂鐜?{len(paths)} 涓巻鍙?ref", total_commits, total_commits)
        return paths, latest_content_commit

    def _current_head_ref_paths(self) -> Set[str]:
        paths: Set[str] = set()
        root = self.cfg.ref_root.replace("\\", "/").strip("/")
        # Refs tracked by HEAD.
        out = self.git(["ls-tree", "-r", "--name-only", "HEAD", "--", root], check=False)
        for line in out.splitlines():
            p = line.strip().replace("\\", "/")
            if self.ref_to_logical(p):
                paths.add(p)
        return paths

    def _worktree_ref_paths(self) -> Set[str]:
        paths: Set[str] = set()
        root = self.repo_root / self.cfg.ref_root
        if root.exists():
            pattern = f"*{self.cfg.ref_suffix}" if self.cfg.ref_suffix else "*"
            for path in root.rglob(pattern):
                if path.is_file():
                    rel = path.relative_to(self.repo_root).as_posix()
                    if self.ref_to_logical(rel):
                        paths.add(rel)
        # Also include tracked paths even if the working tree file is temporarily absent.
        out = self.git(["ls-files", "--", self.cfg.ref_root], check=False)
        for line in out.splitlines():
            p = line.strip().replace("\\", "/")
            if self.ref_to_logical(p):
                paths.add(p)
        out = self.git(["ls-files", "--others", "--exclude-standard", "--", self.cfg.ref_root], check=False)
        for line in out.splitlines():
            p = line.strip().replace("\\", "/")
            if self.ref_to_logical(p):
                paths.add(p)
        return paths

    def list_all_ref_paths(self, progress: Optional[ProgressCallback] = None) -> Set[str]:
        paths, _ = self._iter_log_name_status(progress)
        paths.update(self._current_head_ref_paths())
        paths.update(self._worktree_ref_paths())
        return paths

    def path_exists_in_head(self, ref_path: str) -> bool:
        proc = subprocess.run(
            ["git", "-C", str(self.repo_root), "cat-file", "-e", f"HEAD:{ref_path}"],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            **self._subprocess_ui_kwargs(),
        )
        return proc.returncode == 0

    def latest_commit_for_ref(self, ref_path: str) -> Optional[str]:
        out = self.git(["log", "--all", "-n", "1", "--format=%H", "--", ref_path], check=False)
        for line in out.splitlines():
            line = line.strip()
            if line:
                return line
        return None

    def read_file_at_head_or_worktree(self, ref_path: str) -> Optional[str]:
        work = self.repo_root / ref_path
        if work.exists():
            try:
                return work.read_text(encoding="utf-8")
            except UnicodeDecodeError:
                return work.read_text(encoding="utf-8", errors="replace")
        return self.read_file_at_commit("HEAD", ref_path)

    def read_file_at_commit(self, commit: str, ref_path: str) -> Optional[str]:
        proc = subprocess.run(
            ["git", "-C", str(self.repo_root), "show", f"{commit}:{ref_path}"],
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            encoding="utf-8",
            errors="replace",
            **self._subprocess_ui_kwargs(),
        )
        if proc.returncode != 0:
            return None
        return proc.stdout

    def build_file_entries(self, progress: Optional[ProgressCallback] = None) -> List[FileEntry]:
        entries: List[FileEntry] = []
        historical_paths, latest_content_commit = self._iter_log_name_status(progress)
        head_paths = self._current_head_ref_paths()
        worktree_paths = self._worktree_ref_paths()
        all_paths = sorted(historical_paths | head_paths | worktree_paths)
        total = len(all_paths)
        self._emit(progress, f"姝ｅ湪鎵弿commit锛氳В鏋?ref 鍐呭 0/{total}", 0, total)

        for i, ref_path in enumerate(all_paths, 1):
            logical = self.ref_to_logical(ref_path)
            if not logical:
                continue
            in_head = ref_path in head_paths
            ref_in_worktree = (self.repo_root / ref_path).exists()
            in_worktree = (self.repo_root / logical).exists()

            current_text: Optional[str] = None
            if ref_in_worktree:
                current_text = self.read_file_at_head_or_worktree(ref_path)
            elif in_head:
                current_text = self.read_file_at_commit("HEAD", ref_path)
            else:
                commit = latest_content_commit.get(ref_path)
                if commit:
                    current_text = self.read_file_at_commit(commit, ref_path)

            current_blob = parse_blob_ref(current_text or "", self.cfg)
            entries.append(FileEntry(
                logical_path=logical,
                ref_path=ref_path,
                in_head=in_head,
                in_worktree=in_worktree,
                current_blob=current_blob,
            ))
            if i == 1 or i % 50 == 0 or i == total:
                self._emit(progress, f"姝ｅ湪鎵弿commit锛氳В鏋?ref 鍐呭 {i}/{total}", i, total)

        self._emit(progress, f"姝ｅ湪鎵弿commit锛氬畬鎴愶紝鍏?{len(entries)} 涓枃浠?, total, total)
        return entries

    def history_for_file(self, logical_path: str, progress: Optional[ProgressCallback] = None) -> List[HistoryEntry]:
        ref_path = self.logical_to_ref(logical_path)
        fmt = "%H%x09%h%x09%ad%x09%an%x09%s"
        out = self.git([
            "log", "--all", "--date=iso", f"--format={fmt}", "--", ref_path
        ], check=False)
        log_rows: list[tuple[str, str, str, str, str]] = []
        for line in out.splitlines():
            if not line.strip():
                continue
            parts = line.split("\t", 4)
            if len(parts) < 5:
                continue
            log_rows.append((parts[0], parts[1], parts[2], parts[3], parts[4]))

        total = len(log_rows)
        self._emit(progress, f"姝ｅ湪鎵弿commit锛氳鍙栧巻鍙?ref 0/{total}", 0, total)
        entries: List[HistoryEntry] = []
        for i, (commit, short, date, author, subject) in enumerate(log_rows, 1):
            content = self.read_file_at_commit(commit, ref_path)
            missing = content is None
            blob = parse_blob_ref(content or "", self.cfg)
            entries.append(HistoryEntry(
                logical_path=logical_path,
                ref_path=ref_path,
                commit=commit,
                short_commit=short,
                date=date,
                author=author,
                subject=subject,
                blob=blob,
                ref_content_missing=missing,
            ))
            if i == 1 or i % 25 == 0 or i == total:
                self._emit(progress, f"姝ｅ湪鎵弿commit锛氳鍙栧巻鍙?ref {i}/{total}", i, total)
        return entries

    def remove_working_file_and_ref(self, logical_path: str) -> list[str]:
        removed: list[str] = []
        for rel in [logical_path, self.logical_to_ref(logical_path)]:
            p = self.repo_root / rel
            if p.exists() and p.is_file():
                p.unlink()
                removed.append(rel)
        return removed

    @staticmethod
    def _subprocess_ui_kwargs() -> dict:
        if sys.platform != "win32":
            return {}
        startupinfo = subprocess.STARTUPINFO()
        startupinfo.dwFlags |= subprocess.STARTF_USESHOWWINDOW
        return {
            "startupinfo": startupinfo,
            "creationflags": subprocess.CREATE_NO_WINDOW,
        }

