from __future__ import annotations

import shutil
from abc import ABC, abstractmethod
from pathlib import Path
from typing import Optional, List

from .config import AppConfig
from .models import BlobRef


class OssError(RuntimeError):
    pass


class ObjectStore(ABC):
    @abstractmethod
    def exists(self, blob: BlobRef) -> Optional[bool]:
        raise NotImplementedError

    @abstractmethod
    def download(self, blob: BlobRef, target: Path) -> None:
        raise NotImplementedError

    @abstractmethod
    def delete(self, blob: BlobRef) -> None:
        raise NotImplementedError

    @abstractmethod
    def list_objects(self, prefix: str = "") -> List[str]:
        """Return object keys inside the project/prefix space.

        Keys returned here must be comparable with BlobRef.key. For S3/OSS
        providers this means full bucket object keys including cfg.key_prefix.
        """
        raise NotImplementedError

    @abstractmethod
    def describe(self) -> str:
        raise NotImplementedError


class LocalStore(ObjectStore):
    def __init__(self, repo_root: Path, cfg: AppConfig):
        self.repo_root = repo_root
        self.root = (repo_root / cfg.root).resolve() if not Path(cfg.root).is_absolute() else Path(cfg.root)

    def _path(self, blob: BlobRef) -> Path:
        if not blob.key:
            raise OssError("blob has no key")
        return (self.root / blob.key).resolve()

    def exists(self, blob: BlobRef) -> Optional[bool]:
        if not blob.valid or not blob.key:
            return None
        return self._path(blob).exists()

    def download(self, blob: BlobRef, target: Path) -> None:
        src = self._path(blob)
        if not src.exists():
            raise OssError(f"object missing: {blob.key}")
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(src, target)

    def delete(self, blob: BlobRef) -> None:
        p = self._path(blob)
        if not p.exists():
            raise OssError(f"object already missing: {blob.key}")
        p.unlink()

    def list_objects(self, prefix: str = "") -> List[str]:
        base = self.root
        if prefix:
            base = self.root / prefix.replace("\\", "/")
        if not base.exists():
            return []
        out: List[str] = []
        for path in base.rglob("*"):
            if path.is_file():
                out.append(path.relative_to(self.root).as_posix())
        return sorted(out)

    def describe(self) -> str:
        return f"local:{self.root}"


class AliyunOssStore(ObjectStore):
    def __init__(self, cfg: AppConfig):
        try:
            import oss2  # type: ignore
        except Exception as exc:  # pragma: no cover
            raise OssError("Aliyun OSS provider requires: pip install oss2") from exc
        if not cfg.endpoint or not cfg.bucket:
            raise OssError("aliyun provider requires endpoint and bucket in .gamedepot/config.yaml")
        if not cfg.access_key_id or not cfg.access_key_secret:
            raise OssError("aliyun provider requires access_key_id/access_key_secret, or *_env fields in .gamedepot/config.yaml")
        self.oss2 = oss2
        auth = oss2.Auth(cfg.access_key_id, cfg.access_key_secret)
        self.bucket = oss2.Bucket(auth, cfg.endpoint, cfg.bucket)
        self.bucket_name = cfg.bucket
        self.endpoint = cfg.endpoint

    def exists(self, blob: BlobRef) -> Optional[bool]:
        if not blob.valid or not blob.key:
            return None
        try:
            return bool(self.bucket.object_exists(blob.key))
        except Exception as exc:
            raise OssError(f"OSS exists failed for {blob.key}: {exc}") from exc

    def download(self, blob: BlobRef, target: Path) -> None:
        if not blob.key:
            raise OssError("blob has no key")
        target.parent.mkdir(parents=True, exist_ok=True)
        try:
            self.bucket.get_object_to_file(blob.key, str(target))
        except Exception as exc:
            raise OssError(f"OSS download failed for {blob.key}: {exc}") from exc

    def delete(self, blob: BlobRef) -> None:
        if not blob.key:
            raise OssError("blob has no key")
        try:
            self.bucket.delete_object(blob.key)
        except Exception as exc:
            raise OssError(f"OSS delete failed for {blob.key}: {exc}") from exc

    def list_objects(self, prefix: str = "") -> List[str]:
        try:
            out: List[str] = []
            for obj in self.oss2.ObjectIterator(self.bucket, prefix=prefix or ""):
                key = getattr(obj, "key", "")
                if key:
                    out.append(str(key))
            return sorted(out)
        except Exception as exc:
            raise OssError(f"OSS list failed for prefix {prefix!r}: {exc}") from exc

    def describe(self) -> str:
        return f"aliyun:{self.bucket_name}@{self.endpoint}"


class S3Store(ObjectStore):
    def __init__(self, cfg: AppConfig):
        try:
            import boto3  # type: ignore
            from botocore.exceptions import ClientError  # type: ignore
            from botocore.config import Config as BotoConfig  # type: ignore
        except Exception as exc:  # pragma: no cover
            raise OssError("S3 provider requires: pip install boto3") from exc
        if not cfg.bucket:
            raise OssError("s3 provider requires bucket")
        kwargs = {}
        if cfg.endpoint:
            kwargs["endpoint_url"] = cfg.endpoint
        if cfg.region:
            kwargs["region_name"] = cfg.region
        if cfg.access_key_id and cfg.access_key_secret:
            kwargs["aws_access_key_id"] = cfg.access_key_id
            kwargs["aws_secret_access_key"] = cfg.access_key_secret
        if cfg.force_path_style:
            kwargs["config"] = BotoConfig(s3={"addressing_style": "path"})
        else:
            kwargs["config"] = BotoConfig(s3={"addressing_style": "virtual"})
        self.client = boto3.client("s3", **kwargs)
        self.bucket = cfg.bucket
        self.ClientError = ClientError

    def exists(self, blob: BlobRef) -> Optional[bool]:
        if not blob.valid or not blob.key:
            return None
        try:
            self.client.head_object(Bucket=self.bucket, Key=blob.key)
            return True
        except self.ClientError as exc:
            code = exc.response.get("Error", {}).get("Code", "")
            if str(code) in {"404", "NoSuchKey", "NotFound"}:
                return False
            raise OssError(f"S3 exists failed for {blob.key}: {exc}") from exc

    def download(self, blob: BlobRef, target: Path) -> None:
        if not blob.key:
            raise OssError("blob has no key")
        target.parent.mkdir(parents=True, exist_ok=True)
        try:
            self.client.download_file(self.bucket, blob.key, str(target))
        except Exception as exc:
            raise OssError(f"S3 download failed for {blob.key}: {exc}") from exc

    def delete(self, blob: BlobRef) -> None:
        if not blob.key:
            raise OssError("blob has no key")
        try:
            self.client.delete_object(Bucket=self.bucket, Key=blob.key)
        except Exception as exc:
            raise OssError(f"S3 delete failed for {blob.key}: {exc}") from exc

    def list_objects(self, prefix: str = "") -> List[str]:
        try:
            out: List[str] = []
            kwargs = {"Bucket": self.bucket}
            if prefix:
                kwargs["Prefix"] = prefix
            while True:
                resp = self.client.list_objects_v2(**kwargs)
                for obj in resp.get("Contents", []) or []:
                    key = obj.get("Key")
                    if key:
                        out.append(str(key))
                if not resp.get("IsTruncated"):
                    break
                token = resp.get("NextContinuationToken")
                if not token:
                    break
                kwargs["ContinuationToken"] = token
            return sorted(out)
        except Exception as exc:
            raise OssError(f"S3 list failed for prefix {prefix!r}: {exc}") from exc

    def describe(self) -> str:
        return f"s3:{self.bucket}"


def create_store(repo_root: Path, cfg: AppConfig) -> ObjectStore:
    provider = cfg.provider.lower()
    if provider in {"local", "file", "filesystem"}:
        return LocalStore(repo_root, cfg)
    if provider in {"aliyun", "ali", "oss", "aliyun-oss"}:
        return AliyunOssStore(cfg)
    if provider in {"s3", "cos", "obs", "minio"}:
        return S3Store(cfg)
    raise OssError(f"unknown provider: {cfg.provider}")
