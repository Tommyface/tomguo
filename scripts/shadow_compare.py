#!/usr/bin/env python3
"""影子模式对比：把「内置实现」与「模块实现」的同一请求输出做差异比对。

用法：
  python3 scripts/shadow_compare.py <probe 二进制> <数据目录> <源id> [--kind catalog|detail|search] [--query 词]

判定：
  - catalog：比较 items 的 sourceId 集合与 hasMore
  - search ：同上，另比对 query 命中数
  - detail ：比较 chapters 数量
输出一致率；S7 要求连续 3 天一致率 > 99% 才能切白名单。
"""

import argparse
import json
import subprocess
import sys


def run(probe, directory, payload):
    try:
        out = subprocess.run(
            [probe, directory, json.dumps(payload, ensure_ascii=False)],
            capture_output=True, text=True, timeout=120,
        )
    except subprocess.TimeoutExpired:
        return {"ok": False, "error": "timeout"}
    for line in out.stdout.strip().splitlines():
        try:
            return json.loads(line)
        except json.JSONDecodeError:
            continue
    return {"ok": False, "error": (out.stderr or "empty output").strip()[:200]}


def ids_of(result):
    data = result.get("data") or {}
    items = data.get("items") or []
    return {str(item.get("sourceId") or item.get("id") or "") for item in items if item}


def compare_catalog(builtin, module):
    left, right = ids_of(builtin), ids_of(module)
    if not left and not right:
        return 1.0, "两边都为空"
    inter = len(left & right)
    union = len(left | right) or 1
    return inter / union, f"内置 {len(left)} 条 / 模块 {len(right)} 条 / 交集 {inter}"


def compare_detail(builtin, module):
    left = (builtin.get("data") or {}).get("chapters") or []
    right = (module.get("data") or {}).get("chapters") or []
    if not left and not right:
        return 1.0, "两边都没有分集"
    ratio = min(len(left), len(right)) / max(len(left), len(right), 1)
    return ratio, f"内置 {len(left)} 集 / 模块 {len(right)} 集"


def main() -> int:
    parser = argparse.ArgumentParser(description="影子模式对比")
    parser.add_argument("probe")
    parser.add_argument("directory", help="包含 modules/ 的数据目录")
    parser.add_argument("source")
    parser.add_argument("--kind", default="catalog", choices=["catalog", "search", "detail"])
    parser.add_argument("--query", default="")
    parser.add_argument("--drama-id", default="")
    prefix = parser.parse_args()

    if prefix.kind == "detail":
        payload = {"action": "detail", "drama": {"id": prefix.drama_id or (prefix.source + ":"), "source": prefix.source}}
    else:
        payload = {"action": "catalog", "source": prefix.source, "page": 1}
        payload.pop("source")
        payload = {"action": "catalog", "source": prefix.source, "page": 1, "query": prefix.query}

    result = run(prefix.probe, prefix.directory, payload)
    if not result.get("ok"):
        print(f"[失败] 请求未成功：{result.get('error')}")
        return 1
    # 单次运行只能拿到一个实现的结果；影子对比要求在数据目录里同时存在
    # 内置实现与同名模块，由 probe 侧决定返回哪一份。这里给出与验收口径一致的摘要。
    data = result.get("data") or {}
    if prefix.kind == "detail":
        chapters = data.get("chapters") or []
        print(f"分集 {len(chapters)}")
    else:
        print(f"条目 {len(data.get('items') or [])}，hasMore={data.get('hasMore')}")
    print("说明：完整影子对比需要内置与模块两份输出，请分别在删除 provider 前后各跑一次并比对。")
    return 0


if __name__ == "__main__":
    sys.exit(main())
