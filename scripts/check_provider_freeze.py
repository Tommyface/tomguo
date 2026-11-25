#!/usr/bin/env python3
"""S0 冻结检查：内置站源实现不得再增长。

规则（重构方案 v2 · S0）：
  1. native/core/provider_*.go 的文件数不得超过基线；
  2. 内置源清单（source_registry.go 的 builtinSourceSpecs 条目数）不得超过基线；
  3. 新增源必须以模块形式提交（modules/<id>/{manifest.json,main.js}）。

基线文件：native/core/provider_freeze_baseline.json
更新基线：python3 scripts/check_provider_freeze.py --update

在 CI 里作为独立步骤运行，失败即拒绝合并。
"""

import argparse
import json
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
CORE = ROOT / "native" / "core"
BASELINE = CORE / "provider_freeze_baseline.json"
SPEC_FILE = CORE / "source_registry.go"


def provider_files():
    return sorted(p.name for p in CORE.glob("provider_*.go") if not p.name.endswith("_test.go"))


def builtin_spec_count():
    text = SPEC_FILE.read_text(encoding="utf-8")
    # 只统计 builtinSourceSpecs 字面量里的条目：每条形如 {ID: sourceXxx, ...}
    start = text.find("var builtinSourceSpecs = []sourceSpec{")
    if start < 0:
        raise SystemExit("找不到 builtinSourceSpecs 定义，检查 source_registry.go")
    end = text.find("\n}", start)
    block = text[start:end]
    return len(re.findall(r"\{ID:\s*source", block))


def current():
    return {"providers": provider_files(), "builtinSpecs": builtin_spec_count()}


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--update", action="store_true", help="用当前状态重写基线（仅重构阶段使用）")
    options = parser.parse_args()

    state = current()
    if options.update:
        BASELINE.write_text(json.dumps(state, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        print(f"[基线已更新] provider 文件 {len(state['providers'])} 个，内置源 {state['builtinSpecs']} 个")
        return 0

    if not BASELINE.exists():
        print("[失败] 缺少基线文件，先运行 --update")
        return 1
    baseline = json.loads(BASELINE.read_text(encoding="utf-8"))

    failures = []
    added = [name for name in state["providers"] if name not in baseline["providers"]]
    if added:
        failures.append(
            "新增了内置站源实现文件：" + "、".join(added)
            + "\n  → 新站源必须做成 zip 模块（modules/<id>/），不要改 native/core/"
        )
    if state["builtinSpecs"] > baseline["builtinSpecs"]:
        failures.append(
            f"内置源清单从 {baseline['builtinSpecs']} 增长到 {state['builtinSpecs']}"
            "\n  → 新站源必须做成 zip 模块，不要往 builtinSourceSpecs 里加"
        )
    if state["builtinSpecs"] < baseline["builtinSpecs"]:
        print(f"[提示] 内置源已减少到 {state['builtinSpecs']} 个，重构有进展；")
        print("       完成迁移后请运行 python3 scripts/check_provider_freeze.py --update 收紧基线")

    if failures:
        print("[失败] 内置站源冻结被破坏：")
        for item in failures:
            print("  - " + item)
        return 1
    print(f"[通过] 内置站源未增长：provider 文件 {len(state['providers'])} 个，内置源 {state['builtinSpecs']} 个")
    return 0


if __name__ == "__main__":
    sys.exit(main())
