from __future__ import annotations

import os
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Dict, Iterable, Optional

try:
    import yaml
except Exception:  # pragma: no cover
    yaml = None


_ENV_RE = re.compile(r"\$\{([A-Za-z_][A-Za-z0-9_]*)\}")


def expand_env(value: Any) -> Any:
    """Expand ${VAR}; also accept a whole-value $VAR for hand-written configs."""
    if not isinstance(value, str):
        return value
    if value.startswith("$") and value.count("$") == 1 and "{" not in value:
        return os.environ.get(value[1:], "")

    def repl(match: re.Match[str]) -> str:
        return os.environ.get(match.group(1), "")

    return _ENV_RE.sub(repl, value)


def _read_simple_yaml(path: Path) -> Dict[str, Any]:
    """Tiny fallback when PyYAML is unavailable. Only supports top-level key: value."""
    data: Dict[str, Any] = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith("#") or ":" not in stripped:
            continue
        key, value = stripped.split(":", 1)
        value = value.strip().strip('"').strip("'")
        data[key.strip()] = value
    return data


def _quote_yaml_glob_scalars(text: str) -> str:
    """
    GameDepot configs may contain unquoted glob patterns such as:

      - **/*
      - *.uasset

    Strict YAML treats a leading '*' as an alias token. Go yaml parsers are often
    more permissive, so this GUI repairs those lines before handing them to PyYAML.
    """
    out: list[str] = []
    list_pat = re.compile(r"^(\s*-\s+)([^'\"\s][^#\r\n]*?)(\s*(?:#.*)?)$")
    map_pat = re.compile(r"^(\s*[^:#\r\n][^:\r\n]*:\s+)([^'\"\s][^#\r\n]*?)(\s*(?:#.*)?)$")

    def needs_quote(value: str) -> bool:
        v = value.strip()
        return v.startswith("*")

    def quote_value(value: str) -> str:
        v = value.strip()
        return '"' + v.replace('\\', '\\\\').replace('"', '\\"') + '"'

    for line in text.splitlines():
        fixed = line
        m = list_pat.match(line)
        if m and needs_quote(m.group(2)):
            fixed = f"{m.group(1)}{quote_value(m.group(2))}{m.group(3)}"
        else:
            m = map_pat.match(line)
            if m and needs_quote(m.group(2)):
                fixed = f"{m.group(1)}{quote_value(m.group(2))}{m.group(3)}"
        out.append(fixed)
    # Preserve trailing newline if present.
    return "\n".join(out) + ("\n" if text.endswith("\n") else "")


def _read_yaml(path: Path) -> Dict[str, Any]:
    if not path.exists():
        return {}
    if yaml is not None:
        text = path.read_text(encoding="utf-8")
        try:
            loaded = yaml.safe_load(text) or {}
        except Exception:
            fixed = _quote_yaml_glob_scalars(text)
            loaded = yaml.safe_load(fixed) or {}
        return loaded if isinstance(loaded, dict) else {}
    return _read_simple_yaml(path)


def _norm_key(value: str) -> str:
    """Normalize yaml/go-ish keys: access_key_id, accessKeyID, AccessKeyId -> accesskeyid."""
    return re.sub(r"[^a-z0-9]", "", value.lower())


def _lookup_child(data: Any, key: str) -> Any:
    if not isinstance(data, dict):
        return None
    wanted = _norm_key(key)
    for k, v in data.items():
        if _norm_key(str(k)) == wanted:
            return v
    return None


def _deep_get(data: Dict[str, Any], dotted_paths: Iterable[str]) -> Any:
    for path in dotted_paths:
        cur: Any = data
        ok = True
        for part in path.split("."):
            cur = _lookup_child(cur, part)
            if cur is None:
                ok = False
                break
        if ok and cur not in (None, ""):
            return cur
    return None


def _first(data: Dict[str, Any], *paths: str) -> Any:
    return _deep_get(data, paths)


def _env_from(data: Dict[str, Any], *paths: str) -> str:
    """Read env-var-name fields, e.g. access_key_id_env: ALIYUN_OSS_ACCESS_KEY_ID."""
    name = _first(data, *paths)
    if isinstance(name, str) and name:
        return os.environ.get(name, "")
    return ""


def _truthy(value: Any) -> bool:
    if isinstance(value, bool):
        return value
    if isinstance(value, str):
        return value.strip().lower() in {"1", "true", "yes", "y", "on"}
    return bool(value)


@dataclass
class AppConfig:
    # Store/provider settings.
    provider: str = "local"
    root: str = ".gamedepot/blob-store"
    endpoint: str = ""
    bucket: str = ""
    access_key_id: str = ""
    access_key_secret: str = ""
    region: str = ""
    # Empty key_template means use the Go GameDepot sharded layout:
    #   [prefix/]sha256/aa/bb/<sha>.blob
    # Set key_template only for legacy/custom stores.
    key_template: str = ""
    key_prefix: str = ""
    force_path_style: bool = False

    # Pointer-ref settings.
    ref_root: str = "depot/refs"
    ref_suffix: str = ".gdref"
    logical_prefix: str = ""

    # Informational only.
    source_path: str = field(default="", repr=False)
    profile_name: str = field(default="", repr=False)
    credentials_source_path: str = field(default="", repr=False)

    @classmethod
    def load(cls, repo_root: Path) -> "AppConfig":
        """
        Config priority:

        1. .gamedepot/config.yaml                     # project config
        2. GameDepot global config/credentials.yaml   # created by Go CLI
        3. .oss-manager.yaml                         # optional GUI override

        The tool intentionally does not call the Go executable. It only reads files.
        """
        repo_root = repo_root.resolve()
        gd_path = repo_root / ".gamedepot" / "config.yaml"
        override_path = repo_root / ".oss-manager.yaml"

        raw: Dict[str, Any] = {}
        sources: list[str] = []
        cred_sources: list[str] = []

        global_config_path, global_credentials_path = _find_global_gamedepot_paths()
        global_raw = _read_yaml(global_config_path) if global_config_path else {}
        global_creds = _read_yaml(global_credentials_path) if global_credentials_path else {}

        if global_raw:
            raw.update(_flatten_gamedepot_config(global_raw, global_creds))
            sources.append(str(global_config_path))
            if global_creds:
                cred_sources.append(str(global_credentials_path))

        if gd_path.exists():
            project_raw = _read_yaml(gd_path)
            # Project config wins for repo-specific fields, but may still reference
            # credentials/profiles from the global GameDepot config.
            raw.update(_flatten_gamedepot_config(project_raw, global_creds, fallback_config=global_raw))
            sources.append(str(gd_path))

        # Keep the old file as a small optional override for this GUI only. This does not
        # replace the Go config; it only patches missing/changed fields when present.
        if override_path.exists():
            user_raw = _read_yaml(override_path)
            raw.update(_flatten_oss_manager_config(user_raw))
            sources.append(str(override_path))

        cfg = cls()
        for key in cfg.__dataclass_fields__.keys():
            if key in raw and key not in {"source_path", "credentials_source_path"}:
                setattr(cfg, key, expand_env(raw[key]))
        cfg.provider = _normalize_provider(cfg.provider, cfg)
        cfg.key_template = _normalize_key_template(cfg.key_template)
        cfg.key_prefix = _clean_object_prefix(cfg.key_prefix)
        cfg.ref_root = str(cfg.ref_root or "depot/refs").replace("\\", "/").strip("/")
        cfg.ref_suffix = str(cfg.ref_suffix or ".gdref")
        if not cfg.ref_suffix.startswith("."):
            cfg.ref_suffix = "." + cfg.ref_suffix
        cfg.force_path_style = _truthy(cfg.force_path_style)
        cfg.source_path = " + ".join(sources) if sources else "defaults"
        cfg.credentials_source_path = " + ".join(cred_sources)
        return cfg

    def derive_key(self, sha256: str) -> str:
        """Return the exact object key used by the Go GameDepot store.

        Go GameDepot writes blobs as:
          sha256/<first-2>/<next-2>/<sha>.blob
        and S3 optionally prepends the configured profile prefix.
        """
        sha256 = str(sha256 or "").strip().lower()
        if not sha256:
            return ""
        if self.key_template:
            return self.key_template.format(
                sha256=sha256,
                sha256_2=sha256[:2],          # kept for old templates
                sha256_4=sha256[:4],          # kept for old templates
                sha256_0_2=sha256[:2],
                sha256_2_4=sha256[2:4],
            ).replace("\\", "/").lstrip("/")
        if len(sha256) < 4:
            key = f"sha256/{sha256}.blob"
        else:
            key = f"sha256/{sha256[:2]}/{sha256[2:4]}/{sha256}.blob"
        return _join_object_key(self.key_prefix, key)


def _find_global_gamedepot_paths() -> tuple[Optional[Path], Optional[Path]]:
    """Find Go GameDepot's global config/credentials without invoking gamedepot."""
    candidates: list[Path] = []
    # Go GameDepot currently uses the Windows roaming path. Check these env vars
    # even when tests run on non-Windows systems.
    appdata = os.environ.get("APPDATA")
    localappdata = os.environ.get("LOCALAPPDATA")
    if appdata:
        candidates.append(Path(appdata) / "GameDepot")
    if localappdata:
        candidates.append(Path(localappdata) / "GameDepot")
    xdg = os.environ.get("XDG_CONFIG_HOME")
    if xdg:
        candidates.append(Path(xdg) / "GameDepot")
    home = Path.home()
    candidates.extend([
        home / ".config" / "GameDepot",
        home / ".gamedepot",
        home / "AppData" / "Roaming" / "GameDepot",
    ])

    seen: set[Path] = set()
    for base in candidates:
        base = base.expanduser()
        if base in seen:
            continue
        seen.add(base)
        cfg = base / "config.yaml"
        cred = base / "credentials.yaml"
        if cfg.exists() or cred.exists():
            return (cfg if cfg.exists() else None, cred if cred.exists() else None)
    return None, None


def _normalize_provider(provider: Any, cfg: AppConfig) -> str:
    value = str(provider or "").strip().lower().replace("_", "-")
    aliases = {
        "filesystem": "local",
        "file": "local",
        "fs": "local",
        "localdir": "local",
        "local-dir": "local",
        "aliyunoss": "aliyun",
        "aliyun-oss": "aliyun",
        "ali-oss": "aliyun",
        "aws-s3": "s3",
        "s3-compatible": "s3",
    }
    value = aliases.get(value, value)
    if value:
        return value
    if cfg.bucket and cfg.endpoint and "aliyun" in cfg.endpoint.lower():
        # Old native OSS configs often omit provider but use an aliyun endpoint.
        return "aliyun"
    if cfg.bucket:
        return "s3"
    return "local"


def _clean_object_prefix(value: Any) -> str:
    text = str(value or "").replace("\\", "/").strip().strip("/")
    return "" if text == "." else text


def _join_object_key(*parts: str) -> str:
    cleaned = [str(p).replace("\\", "/").strip("/") for p in parts if str(p or "").strip("/")]
    return "/".join(cleaned)


def _normalize_key_template(value: Any) -> str:
    text = str(value or "").replace("\\", "/").strip().strip("/")
    # Empty means: use Go GameDepot's built-in sharded sha256 layout.
    if not text or text == ".":
        return ""
    # Treat explicit custom strings as templates. If they are a literal prefix,
    # keep compatibility by appending {sha256}; GameDepot profile prefix is handled
    # separately through key_prefix and must not come through here.
    if "{" not in text:
        text = text.rstrip("/") + "/{sha256}"
    return text


def _flatten_oss_manager_config(data: Dict[str, Any]) -> Dict[str, Any]:
    """Old standalone config shape; also tolerant to nested sections."""
    out: Dict[str, Any] = {}
    for field_name in AppConfig.__dataclass_fields__.keys():
        if field_name in {"source_path", "credentials_source_path"}:
            continue
        val = _first(data, field_name)
        if val is not None:
            out[field_name] = val
    out.update(_flatten_gamedepot_config(data))
    return out


def _select_profile(data: Dict[str, Any], credentials: Optional[Dict[str, Any]], fallback_config: Optional[Dict[str, Any]]) -> Dict[str, Any]:
    """Import Go global-style profiles/default_profile plus split credentials.yaml."""
    merged_sources = [data]
    if fallback_config:
        merged_sources.append(fallback_config)

    profile_name = None
    for src in merged_sources:
        profile_name = _first(src, "profile", "active_profile", "activeProfile", "default_profile", "defaultProfile", "storage.profile", "remote.profile")
        if profile_name:
            break
    if not profile_name:
        return {}
    profile_name = str(profile_name)

    profile: Any = None
    for src in merged_sources:
        profiles = _lookup_child(src, "profiles")
        if isinstance(profiles, dict):
            # Profile names are user-facing; do not normalize by removing '-'. Prefer exact,
            # then canonical fallback.
            profile = profiles.get(profile_name)
            if profile is None:
                wanted = _norm_key(profile_name)
                for k, v in profiles.items():
                    if _norm_key(str(k)) == wanted:
                        profile = v
                        break
        if isinstance(profile, dict):
            break
    if not isinstance(profile, dict):
        return {"profile_name": profile_name}

    out: Dict[str, Any] = dict(profile)
    out["profile_name"] = profile_name

    creds_section: Any = None
    if isinstance(credentials, dict):
        creds_section = _lookup_child(credentials, "credentials") or credentials
    if isinstance(creds_section, dict):
        cred = creds_section.get(profile_name)
        if cred is None:
            wanted = _norm_key(profile_name)
            for k, v in creds_section.items():
                if _norm_key(str(k)) == wanted:
                    cred = v
                    break
        if isinstance(cred, dict):
            out.update(cred)
    return out


def _flatten_gamedepot_config(
    data: Dict[str, Any],
    credentials: Optional[Dict[str, Any]] = None,
    fallback_config: Optional[Dict[str, Any]] = None,
) -> Dict[str, Any]:
    """
    Best-effort importer for Go-created GameDepot yaml.

    Accepts:
    - project .gamedepot/config.yaml
    - global AppData/Roaming/GameDepot/config.yaml with default_profile/profiles
    - AppData/Roaming/GameDepot/credentials.yaml with credentials[profile]
    - snake_case or camelCase fields
    """
    out: Dict[str, Any] = {}

    prof = _select_profile(data, credentials, fallback_config)
    source_data: Dict[str, Any] = dict(data)
    # Profile values should override generic global values, but direct project fields
    # can override the selected profile after this function returns via raw.update().
    if prof:
        source_data.update(prof)
        out["profile_name"] = prof.get("profile_name", "")

    provider = _first(
        source_data,
        "provider",
        "type",
        "backend",
        "storage.provider",
        "storage.type",
        "storage.backend",
        "store.provider",
        "store.type",
        "object_store.provider",
        "object_store.type",
        "objectStore.provider",
        "objectStore.type",
        "oss.provider",
        "oss.type",
        "remote.provider",
        "remote.type",
    )
    if provider is not None:
        out["provider"] = provider

    endpoint = _first(
        source_data,
        "endpoint",
        "endpoint_url",
        "endpointURL",
        "storage.endpoint",
        "storage.endpoint_url",
        "storage.oss.endpoint",
        "store.endpoint",
        "object_store.endpoint",
        "object_store.endpoint_url",
        "objectStore.endpoint",
        "objectStore.endpointURL",
        "oss.endpoint",
        "aliyun.endpoint",
        "s3.endpoint",
        "remote.endpoint",
    )
    if endpoint is not None:
        out["endpoint"] = endpoint

    bucket = _first(
        source_data,
        "bucket",
        "bucket_name",
        "bucketName",
        "storage.bucket",
        "storage.oss.bucket",
        "store.bucket",
        "object_store.bucket",
        "object_store.bucket_name",
        "objectStore.bucket",
        "objectStore.bucketName",
        "oss.bucket",
        "aliyun.bucket",
        "s3.bucket",
        "remote.bucket",
    )
    if bucket is not None:
        out["bucket"] = bucket

    root = _first(
        source_data,
        "root",
        "path",
        "local_root",
        "localRoot",
        "local_dir",
        "localDir",
        "blob_root",
        "blobRoot",
        "storage.root",
        "storage.local_root",
        "storage.local.dir",
        "storage.local.root",
        "storage.localDir",
        "store.root",
        "object_store.root",
        "objectStore.root",
        "local.root",
        "local.dir",
    )
    if root is not None:
        out["root"] = root

    region = _first(
        source_data,
        "region",
        "storage.region",
        "storage.oss.region",
        "store.region",
        "object_store.region",
        "objectStore.region",
        "oss.region",
        "aliyun.region",
        "s3.region",
        "remote.region",
    )
    if region is not None:
        out["region"] = region

    access_key_id = _first(
        source_data,
        "access_key_id",
        "accessKeyId",
        "accessKeyID",
        "ak",
        "access_id",
        "storage.access_key_id",
        "storage.accessKeyId",
        "storage.oss.access_key_id",
        "object_store.access_key_id",
        "objectStore.accessKeyId",
        "oss.access_key_id",
        "oss.accessKeyId",
        "aliyun.access_key_id",
        "s3.access_key_id",
        "remote.access_key_id",
    ) or _env_from(
        source_data,
        "access_key_id_env",
        "accessKeyIdEnv",
        "ak_env",
        "storage.access_key_id_env",
        "storage.oss.access_key_id_env",
        "object_store.access_key_id_env",
        "oss.access_key_id_env",
        "aliyun.access_key_id_env",
        "s3.access_key_id_env",
    )
    if access_key_id:
        out["access_key_id"] = access_key_id

    access_key_secret = _first(
        source_data,
        "access_key_secret",
        "accessKeySecret",
        "access_key",
        "secret_key",
        "secretKey",
        "sk",
        "storage.access_key_secret",
        "storage.accessKeySecret",
        "storage.oss.access_key_secret",
        "object_store.access_key_secret",
        "objectStore.accessKeySecret",
        "oss.access_key_secret",
        "oss.accessKeySecret",
        "aliyun.access_key_secret",
        "s3.access_key_secret",
        "remote.access_key_secret",
    ) or _env_from(
        source_data,
        "access_key_secret_env",
        "accessKeySecretEnv",
        "secret_key_env",
        "sk_env",
        "storage.access_key_secret_env",
        "storage.oss.access_key_secret_env",
        "object_store.access_key_secret_env",
        "oss.access_key_secret_env",
        "aliyun.access_key_secret_env",
        "s3.access_key_secret_env",
    )
    if access_key_secret:
        out["access_key_secret"] = access_key_secret

    key_template = _first(
        source_data,
        "key_template",
        "keyTemplate",
        "object_key_template",
        "objectKeyTemplate",
        "blob_key_template",
        "blobKeyTemplate",
        "storage.key_template",
        "storage.oss.key_template",
        "store.key_template",
        "object_store.key_template",
        "objectStore.keyTemplate",
        "objectStore.objectKeyTemplate",
        "objectStore.blobKeyTemplate",
        "oss.key_template",
        "aliyun.key_template",
        "s3.key_template",
    )
    key_prefix = _first(
        source_data,
        "key_prefix",
        "keyPrefix",
        "object_prefix",
        "objectPrefix",
        "blob_prefix",
        "blobPrefix",
        "prefix",
        "storage.key_prefix",
        "storage.object_prefix",
        "storage.blob_prefix",
        "storage.oss.key_prefix",
        "object_store.key_prefix",
        "objectStore.keyPrefix",
        "oss.key_prefix",
        "aliyun.key_prefix",
        "s3.key_prefix",
        "s3.prefix",
        "remote.prefix",
        "store.prefix",
    )
    if key_template is not None:
        out["key_template"] = key_template
    if key_prefix is not None:
        # Go S3BlobStore prefixes the sharded blob key, it does not replace the
        # blob key with prefix/<sha>.
        out["key_prefix"] = key_prefix

    ref_root = _first(
        source_data,
        "ref_root",
        "refRoot",
        "refs_root",
        "refsRoot",
        "pointer_ref_root",
        "pointerRefRoot",
        "pointer_refs.root",
        "pointerRefs.root",
        "refs.root",
        "depot.refs_root",
        "depot.ref_root",
    )
    if ref_root is not None:
        out["ref_root"] = ref_root

    ref_suffix = _first(
        source_data,
        "ref_suffix",
        "refSuffix",
        "pointer_ref_suffix",
        "pointerRefSuffix",
        "pointer_refs.suffix",
        "pointerRefs.suffix",
        "refs.suffix",
    )
    if ref_suffix is not None:
        out["ref_suffix"] = ref_suffix

    logical_prefix = _first(
        source_data,
        "logical_prefix",
        "logicalPrefix",
    )
    if logical_prefix is not None:
        out["logical_prefix"] = logical_prefix

    force_path_style = _first(
        source_data,
        "force_path_style",
        "forcePathStyle",
        "path_style",
        "pathStyle",
        "s3.force_path_style",
        "s3.forcePathStyle",
    )
    if force_path_style is not None:
        out["force_path_style"] = force_path_style

    # Infer provider when the config omits an explicit provider.
    if "provider" not in out:
        use_oss = _first(source_data, "use_oss", "useOSS", "storage.use_oss", "storage.useOSS")
        if _truthy(use_oss) or out.get("bucket"):
            endpoint_text = str(out.get("endpoint", "")).lower()
            out["provider"] = "aliyun" if "aliyun" in endpoint_text and "s3." not in endpoint_text else "s3"
        elif out.get("root"):
            out["provider"] = "local"

    return out
