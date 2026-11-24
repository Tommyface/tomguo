#!/usr/bin/env python3
"""把本地 catalog 目录发布到 Tommyface/tomguo-app 的 tomguo/catalog/。

用法：GH_TOKEN=<PAT> python3 scripts/publish_catalog_snapshot.py /path/to/catalog
"""
from __future__ import annotations

import base64
import json
import os
import pathlib
import sys
import urllib.error
import urllib.request

REPO = "Tommyface/tomguo-app"


def call(path: str, data: dict | None = None, method: str | None = None) -> dict:
    body = json.dumps(data).encode() if data is not None else None
    req = urllib.request.Request(
        f"https://api.github.com/repos/{REPO}/contents/{path}",
        data=body,
        method=method,
        headers={
            "Authorization": "Bearer " + os.environ["GH_TOKEN"],
            "Accept": "application/vnd.github+json",
            "User-Agent": "catalog-snapshot-publisher",
        },
    )
    return json.loads(urllib.request.urlopen(req, timeout=40).read())


def main() -> int:
    root = pathlib.Path(sys.argv[1])
    for file in sorted(root.rglob("*.json")):
        rel = "tomguo/catalog/" + str(file.relative_to(root))
        sha = None
        try:
            sha = call(rel).get("sha")
        except urllib.error.HTTPError:
            pass
        payload = {
            "message": "chore: refresh catalog snapshot",
            "content": base64.b64encode(file.read_bytes()).decode(),
            "branch": "main",
        }
        if sha:
            payload["sha"] = sha
        call(rel, payload, method="PUT")
        print("published", rel)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
