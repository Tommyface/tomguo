#!/usr/bin/env python3
"""影子对比运行器：对同一源分别用内置实现与模块实现请求，输出结构化对比。

用法：
  python3 scripts/shadow_compare_run.py <probe 二进制> <数据目录（含 modules/）> <源id> [more action JSON]

内置侧通过 {"action":"shadow","source":"<源id>","command":"builtin"} 切换；
模块侧通过 {"action":"shadow","source":"<源id>","command":"module"} 切回。

对比字段：
  - catalog/search：sourceId 集合、条数、hasMore
  - detail：分集数
  - resolve：URL 非空
"""

import json
import subprocess
import sys


def call(probe, directory, payload, seq):
    spec = dict(payload)
    spec.update({"directory": directory, "session": "shadow", "sequence": seq})
    out = subprocess.run([probe, directory, json.dumps(spec, ensure_ascii=False)],
                         capture_output=True, text=True, timeout=180)
    for line in out.stdout.strip().splitlines():
        try:
            return json.loads(line)
        except json.JSONDecodeError:
            continue
    return {"ok": False, "error": (out.stderr or "no output").strip()[:200]}


def summary(result):
    data = result.get("data") or {}
    items = data.get("items") or []
    chapters = data.get("chapters") or []
    if items:
        return {"items": len(items), "ids": sorted(str(x.get("sourceId") or x.get("id")) for x in items),
                "hasMore": data.get("hasMore")}
    if chapters:
        return {"chapters": len(chapters)}
    if data.get("url"):
        return {"url": True}
    return {"error": result.get("error", "empty")}


def compare(payload, builtin, module):
    left, right = summary(builtin), summary(module)
    ids_l, ids_r = set(left.get("ids", [])), set(right.get("ids", []))
    ratio = (len(ids_l & ids_r) / len(ids_l | ids_r)) if (ids_l or ids_r) else 1.0
    return {"builtin": left, "module": right, "id_overlap": round(ratio, 4),
            "pass": ratio >= 0.99 and left.get("error") == right.get("error")}


def main():
    if len(sys.argv) < 4:
        print("用法: shadow_compare_run.py <probe> <dir> <source> [action-json]")
        return 2
    probe, directory, source = sys.argv[1:4]
    payload = json.loads(sys.argv[4]) if len(sys.argv) > 4 else {"action": "catalog", "source": source, "page": 1}
    payload.setdefault("source", source)

    seq = 1
    seq += 1
    call(probe, directory, {"action": "shadow", "source": source, "command": "builtin"}, seq)
    builtin = call(probe, directory, payload, seq + 1)
    seq += 2
    call(probe, directory, {"action": "shadow", "source": source, "command": "module"}, seq)
    module = call(probe, directory, payload, seq + 1)

    result = {"source": source, "action": payload.get("action"), **compare(payload, builtin, module)}
    print(json.dumps(result, ensure_ascii=False))
    return 0 if result["pass"] else 1


if __name__ == "__main__":
    sys.exit(main())
