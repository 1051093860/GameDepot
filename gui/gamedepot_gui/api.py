from __future__ import annotations

import json
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from typing import Any, Optional


@dataclass
class ApiResult:
    ok: bool
    data: Any = None
    output: str = ""
    error: str = ""
    raw: str = ""
    status: int = 0


class GameDepotApiClient:
    def __init__(self, base_url: str = "http://127.0.0.1:17320", token: str = "", timeout: float = 120.0) -> None:
        self.base_url = base_url.rstrip("/")
        self.token = token
        self.timeout = timeout

    def _headers(self) -> dict[str, str]:
        headers = {"Accept": "application/json"}
        if self.token:
            headers["Authorization"] = f"Bearer {self.token}"
        return headers

    def get(self, path: str, query: Optional[dict[str, Any]] = None) -> ApiResult:
        url = self.base_url + path
        if query:
            pairs = {k: v for k, v in query.items() if v is not None and v != ""}
            if pairs:
                url += "?" + urllib.parse.urlencode(pairs)
        return self._request("GET", url, None)

    def post(self, path: str, body: Optional[dict[str, Any]] = None) -> ApiResult:
        url = self.base_url + path
        payload = json.dumps(body or {}).encode("utf-8")
        return self._request("POST", url, payload)

    def _request(self, method: str, url: str, payload: Optional[bytes]) -> ApiResult:
        headers = self._headers()
        if payload is not None:
            headers["Content-Type"] = "application/json"
        req = urllib.request.Request(url, data=payload, headers=headers, method=method)
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                raw = resp.read().decode("utf-8", errors="replace")
                return self._parse(raw, resp.status)
        except urllib.error.HTTPError as e:
            raw = e.read().decode("utf-8", errors="replace")
            parsed = self._parse(raw, e.code)
            if parsed.error:
                return parsed
            return ApiResult(ok=False, error=str(e), raw=raw, status=e.code)
        except Exception as e:  # noqa: BLE001 - GUI needs a readable error box.
            return ApiResult(ok=False, error=str(e), status=0)

    @staticmethod
    def _parse(raw: str, status: int) -> ApiResult:
        try:
            obj = json.loads(raw) if raw.strip() else {}
        except json.JSONDecodeError:
            return ApiResult(ok=False, error="response is not JSON", raw=raw, status=status)
        return ApiResult(
            ok=bool(obj.get("ok")),
            data=obj.get("data"),
            output=obj.get("output") or "",
            error=obj.get("error") or "",
            raw=raw,
            status=status,
        )
