#!/usr/bin/env python3
"""生成剧集快照：index.json + sources/<id>.json（明文，sha256 校验）。

用法：
  python3 scripts/build_catalog_snapshot.py --probe PROBE --data-dir DIR \
      --sources chaoliu,heitao-guoapp --out /path/to/catalog

输出目录直接作为 tomguo-app 仓库的 tomguo/catalog/ 提交到 main。
客户端读 https://fastly.jsdelivr.net/gh/Tommyface/tomguo-app@main/tomguo/catalog/index.json
"""
from __future__ import annotations
import argparse, hashlib, json, subprocess, time
from pathlib import Path

MAX_SOURCE_BYTES = 24 << 20
MAX_INDEX_BYTES = 256 << 1024
MAX_TOTAL_BYTES = 32 << 20


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument('--probe', required=True)
    ap.add_argument('--data-dir', required=True)
    ap.add_argument('--sources', required=True, help='逗号分隔的站源 id')
    ap.add_argument('--out', required=True)
    ap.add_argument('--pages', type=int, default=5)
    args = ap.parse_args()

    root = Path(args.out)
    (root / 'sources').mkdir(parents=True, exist_ok=True)
    generated = time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())
    index = {'version': 1, 'generatedAt': generated, 'sources': {}}
    total = 0
    for src in [s.strip() for s in args.sources.split(',') if s.strip()]:
        items, seen = [], set()
        for page in range(1, args.pages + 1):
            spec = json.dumps({'action': 'catalog', 'source': src, 'page': page}, ensure_ascii=False)
            try:
                out = subprocess.run(
                    [args.probe, args.data_dir, spec],
                    capture_output=True, text=True, timeout=90,
                ).stdout.strip().splitlines()[-1]
                data = json.loads(out).get('data') or {}
            except Exception:
                break
            batch = data.get('items') or []
            if not batch:
                break
            for item in batch:
                ident = item.get('id')
                if ident and ident not in seen:
                    seen.add(ident)
                    items.append(item)
            if not data.get('hasMore') and len(batch) < 20:
                break
        if not items:
            print(f'{src}: empty, skipped')
            continue
        raw = json.dumps(
            {'source': src, 'generatedAt': generated, 'items': items},
            ensure_ascii=False, separators=(',', ':'),
        ).encode()
        if len(raw) > MAX_SOURCE_BYTES or total + len(raw) > MAX_TOTAL_BYTES:
            print(f'{src}: exceeds size limit, skipped')
            continue
        (root / 'sources' / f'{src}.json').write_bytes(raw)
        index['sources'][src] = {
            'file': f'sources/{src}.json',
            'sha256': hashlib.sha256(raw).hexdigest(),
            'count': len(items),
            'bytes': len(raw),
        }
        total += len(raw)
        print(f'{src}: {len(items)} items, {len(raw)} bytes')
    idx_raw = json.dumps(index, ensure_ascii=False, indent=2).encode()
    if len(idx_raw) > MAX_INDEX_BYTES:
        raise SystemExit('index exceeds 256KiB')
    (root / 'index.json').write_bytes(idx_raw)
    print(f'index: {len(index["sources"])} sources, {len(idx_raw)} bytes')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
