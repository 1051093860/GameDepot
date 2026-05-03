from __future__ import annotations

import json
import re
from typing import Any, Dict

from .config import AppConfig
from .models import BlobRef

try:
    import yaml
except Exception:  # pragma: no cover
    yaml = None

_SHA_RE = re.compile(r"\b[a-fA-F0-9]{64}\b")
_KEY_PATTERNS = [
    re.compile(r"(?:oss_key|ossKey|object_key|objectKey|blob_key|blobKey|remote_key|remoteKey|key)\s*[:=]\s*[\"']?([^\"'\s,]+)", re.I),
    re.compile(r"(?:path|blob)\s*[:=]\s*[\"']?([^\"'\s,]+)", re.I),
]
_SIZE_RE = re.compile(r"(?:size|bytes|size_bytes|sizeBytes|object_size|objectSize)\s*[:=]\s*(\d+)", re.I)


def parse_blob_ref(text: str, cfg: AppConfig) -> BlobRef:
    if not text.strip():
        return BlobRef(parse_error="empty ref")

    # JSON first.
    try:
        data = json.loads(text)
        if isinstance(data, dict):
            return _from_mapping(data, cfg, "json")
    except Exception:
        pass

    # YAML second.
    if yaml is not None:
        try:
            data = yaml.safe_load(text)
            if isinstance(data, dict):
                return _from_mapping(data, cfg, "yaml")
        except Exception:
            pass

    # Very permissive fallback for key=value / raw text.
    sha = ""
    m = _SHA_RE.search(text)
    if m:
        sha = m.group(0).lower()

    key = ""
    for pattern in _KEY_PATTERNS:
        m = pattern.search(text)
        if m:
            candidate = m.group(1).strip().strip(",")
            # Avoid treating a sha as a key if it is just the hash field.
            if not _SHA_RE.fullmatch(candidate):
                key = candidate
                break

    size = None
    m = _SIZE_RE.search(text)
    if m:
        try:
            size = int(m.group(1))
        except Exception:
            size = None

    if sha and not key:
        key = cfg.derive_key(sha)
    return BlobRef(sha256=sha, key=key, size=size, raw_format="text")


def _from_mapping(data: Dict[str, Any], cfg: AppConfig, fmt: str) -> BlobRef:
    flat = _flatten(data)
    sha = _pick(flat, [
        "sha256", "sha", "hash", "object_sha256", "objectHash", "blob_sha256", "blobHash",
        "digest", "content_sha256", "contentHash", "oid",
    ])
    if sha:
        m = _SHA_RE.search(str(sha))
        sha = m.group(0).lower() if m else str(sha).strip().lower()
    else:
        sha = ""

    # GameDepot .gdref files contain `path`, but that is the asset path
    # (Content/...), not the OSS object key. Only explicit object-key fields are
    # treated as object keys; otherwise derive the key from oid/sha256 exactly the
    # same way the Go store does.
    key = _pick(flat, [
        "key", "oss_key", "ossKey", "object_key", "objectKey", "blob_key", "blobKey",
        "remote_key", "remoteKey", "object_path", "objectPath",
    ])
    key = str(key).strip() if key else ""

    size_raw = _pick(flat, ["size", "bytes", "size_bytes", "sizeBytes", "object_size", "objectSize"])
    size = None
    if size_raw not in (None, ""):
        try:
            size = int(size_raw)
        except Exception:
            size = None

    if sha and not key:
        key = cfg.derive_key(sha)
    return BlobRef(sha256=sha, key=key, size=size, raw_format=fmt)


def _canon(name: str) -> str:
    return re.sub(r"[^a-z0-9]", "", name.lower())


def _flatten(data: Dict[str, Any], prefix: str = "") -> Dict[str, Any]:
    out: Dict[str, Any] = {}
    for k, v in data.items():
        key = f"{prefix}.{k}" if prefix else str(k)
        out[key] = v
        if isinstance(v, dict):
            out.update(_flatten(v, key))
    return out


def _pick(flat: Dict[str, Any], names: list[str]) -> Any:
    # Exact lower/suffix match first.
    lower = {k.lower(): v for k, v in flat.items()}
    for name in names:
        lname = name.lower()
        if lname in lower:
            return lower[lname]
    for name in names:
        lname = name.lower()
        for k, v in lower.items():
            if k.endswith("." + lname):
                return v

    # Canonical match allows objectKey/object_key/object-key and nested suffixes.
    canonical_names = {_canon(name) for name in names}
    for k, v in flat.items():
        last = k.split(".")[-1]
        if _canon(last) in canonical_names:
            return v
    return None
