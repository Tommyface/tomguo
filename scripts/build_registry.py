#!/usr/bin/env python3
"""从站源模块目录生成模块清单（registry）。

产物：
  registry.json        非成人条目（可公开发布）
  registry-adult.json  成人条目（需单独授权）
  registry-all.json    全量（内部自用，不发布）

用法：
  python3 scripts/build_registry.py modules/ --out dist/ --base-url https://cdn.example.com/tomguo
  python3 scripts/build_registry.py modules/ --out dist/ --base-url ... --check   # CI 校验产物是否最新

规则：
  - manifest 必须含 id / name / version / runtime / api / adult；main.js ≤ 4 MB；
  - mainSha256 必须与 main.js 实际字节一致，不一致直接失败（先跑 fill_module_digest.py）；
  - 输出按 id 排序，保证同输入产出同字节（可放进 git 比对）。
"""

import argparse
import hashlib
import json
import pathlib
import re
import sys

ID_PATTERN = re.compile(r"^[a-z0-9_\-]{1,32}$")
ENTRY_BYTES = 4 << 20
CAPABILITY_HINT = {
    "search", "suggestions", "recommendations", "danmaku",
    "rankings", "cover", "metadata", "catalogState",
}
REGISTRY_API = 2
REGISTRY_SCHEMA = 2

# 源状态：active 可装；deprecated 可装但提示；retired/blocked 不可装。
SOURCE_STATES = ("active", "deprecated", "retired", "blocked")


def find_modules(root: pathlib.Path):
    if (root / "manifest.json").exists() and (root / "main.js").exists():
        return [root]
    return sorted({m.parent for m in root.rglob("manifest.json") if (m.parent / "main.js").exists()})


def read_module(directory: pathlib.Path):
    manifest_path = directory / "manifest.json"
    entry_path = directory / "main.js"
    try:
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as error:
        raise ValueError(f"{manifest_path} 不是合法 JSON：{error}")
    code = entry_path.read_bytes()
    if len(code) > ENTRY_BYTES:
        raise ValueError(f"{directory}: main.js 超过 4 MB")
    module_id = str(manifest.get("id") or "").strip().lower()
    if not ID_PATTERN.match(module_id):
        raise ValueError(f"{directory}: 模块标识不合法「{module_id}」")
    if directory.name != module_id:
        raise ValueError(f"{directory}: 目录名与 id「{module_id}」不一致")
    if not str(manifest.get("name") or "").strip():
        raise ValueError(f"{module_id}: 缺少名称")
    if manifest.get("runtime") != "js":
        raise ValueError(f"{module_id}: runtime 必须是 js")
    if manifest.get("api") != 1:
        raise ValueError(f"{module_id}: api 必须为 1")
    adult = manifest.get("adult")
    if not isinstance(adult, bool):
        raise ValueError(f"{module_id}: adult 必须是布尔值")
    capabilities = manifest.get("capabilities") or []
    if not isinstance(capabilities, list) or any(not isinstance(item, str) for item in capabilities):
        raise ValueError(f"{module_id}: capabilities 必须是字符串数组")
    for name in capabilities:
        if name not in CAPABILITY_HINT:
            print(f"[提示] {module_id}: 声明了当前未收录的能力「{name}」，仍会写入清单")
    declared = str(manifest.get("mainSha256") or "").strip().lower()
    actual = hashlib.sha256(code).hexdigest()
    if declared != actual:
        raise ValueError(f"{module_id}: mainSha256 与 main.js 不一致，请先运行 fill_module_digest.py")
    return {
        "id": module_id,
        "name": str(manifest.get("name")).strip(),
        "version": str(manifest.get("version") or "1.0.0").strip(),
        "description": str(manifest.get("description") or "").strip(),
        "minApi": int(manifest.get("minApi") or 0),
        "adult": adult,
        "capabilities": capabilities,
        "hosts": manifest.get("hosts") or [],
        "mainSha256": actual,
        "zip": "",
        "size": len(code),
        # schema 2：可新增性由清单声明，客户端按此渲染安装按钮。
        "available": True,
        "state": "active",
        "reason": "",
        "minAppVersion": "",
        "replacedBy": "",
    }


def with_zip(entry: dict, base_url: str) -> dict:
    base = base_url.rstrip("/")
    entry = dict(entry)
    entry["zip"] = f"{base}/{entry['id']}-{entry['version']}.zip"
    return entry


def dump(path: pathlib.Path, list_name: str, description: str, entries) -> str:
    payload = {
        "name": list_name,
        "description": description,
        "api": REGISTRY_API,
        "schema": REGISTRY_SCHEMA,
        "plugins": entries,
    }
    text = json.dumps(payload, ensure_ascii=False, indent=2) + "\n"
    path.write_text(text, encoding="utf-8")
    return text


def main() -> int:
    parser = argparse.ArgumentParser(description="生成站源模块清单")
    parser.add_argument("modules", help="模块根目录")
    parser.add_argument("--out", default="dist", help="产物目录，默认 dist/")
    parser.add_argument("--base-url", default="https://example.com/tomguo", help="模块 zip 的下载前缀")
    parser.add_argument("--check", action="store_true", help="只比对产物是否最新，不写盘")
    parser.add_argument("--state", action="append", default=[], metavar="ID=STATE",
                        help="声明某源状态，如 --state guipian=retired（可重复）")
    parser.add_argument("--available", action="append", default=[], metavar="ID=BOOL",
                        help="声明某源能否新增安装，如 --available guipian=false")
    parser.add_argument("--reason", action="append", default=[], metavar="ID=TEXT",
                        help="声明不可安装原因，如 --reason guipian=站点已关停")
    parser.add_argument("--min-app-version", action="append", default=[], metavar="ID=VER",
                        help="声明最低 App 版本，如 --min-app-version jinpai=0.4.0")
    parser.add_argument("--replaced-by", action="append", default=[], metavar="ID=TARGET",
                        help="声明退役后建议迁移到哪个模块")
    options = parser.parse_args()

    root = pathlib.Path(options.modules)
    out = pathlib.Path(options.out)
    if not root.is_dir():
        print(f"[失败] 模块目录不存在：{root}")
        return 1

    entries = []
    errors = 0
    for directory in find_modules(root):
        try:
            entries.append(with_zip(read_module(directory), options.base_url))
        except ValueError as error:
            print(f"[失败] {error}")
            errors += 1
    if errors:
        print(f"\n校验失败：{errors} 个模块不合法，未生成清单")
        return 1

    def parse_assignments(values, label):
        parsed = {}
        for item in values:
            if "=" not in item:
                print(f"[失败] --{label} 需要 ID=值 形式：{item}")
                return None
            key, value = item.split("=", 1)
            parsed[key.strip().lower()] = value.strip()
        return parsed

    states = parse_assignments(options.state, "state")
    availables = parse_assignments(options.available, "available")
    reasons = parse_assignments(options.reason, "reason")
    min_versions = parse_assignments(options.min_app_version, "min-app-version")
    replacements = parse_assignments(options.replaced_by, "replaced-by")
    if any(value is None for value in (states, availables, reasons, min_versions, replacements)):
        return 1

    ids = {entry["id"] for entry in entries}
    for field, values in (("state", states), ("available", availables),
                          ("reason", reasons), ("minAppVersion", min_versions),
                          ("replacedBy", replacements)):
        unknown = sorted(set(values) - ids)
        if unknown:
            print(f"[失败] --{field} 引用了不存在的模块：{'、'.join(unknown)}")
            return 1

    for entry in entries:
        module_id = entry["id"]
        if module_id in states:
            state = states[module_id]
            if state not in SOURCE_STATES:
                print(f"[失败] {module_id}: state 只能是 {'/'.join(SOURCE_STATES)}，收到 {state}")
                return 1
            entry["state"] = state
        if module_id in availables:
            value = availables[module_id].lower()
            if value not in ("true", "false"):
                print(f"[失败] {module_id}: available 只能是 true/false，收到 {value}")
                return 1
            entry["available"] = value == "true"
        if module_id in reasons:
            entry["reason"] = reasons[module_id]
        if module_id in min_versions:
            entry["minAppVersion"] = min_versions[module_id]
        if module_id in replacements:
            entry["replacedBy"] = replacements[module_id]

        # 不可安装必须说明理由；退役/停用即使可安装也应说明。
        if (not entry["available"] or entry["state"] in ("retired", "blocked")) and not entry["reason"]:
            print(f"[失败] {module_id}: available=false 或 state=retired/blocked 时必须带 --reason")
            return 1
        if entry["replacedBy"] and entry["replacedBy"] not in ids:
            print(f"[失败] {module_id}: replacedBy 指向不存在的模块 {entry['replacedBy']}")
            return 1

    entries.sort(key=lambda item: item["id"])
    vod = [item for item in entries if not item["adult"]]
    adult = [item for item in entries if item["adult"]]
    outputs = {
        "registry.json": ("影视与短剧源", "不含成人内容，可公开发布。", vod),
        "registry-adult.json": ("成人源", "默认隐藏，需在设置中开启后再导入。", adult),
        "registry-all.json": ("全量源", "内部自用，不对外发布。", entries),
    }

    changed = 0
    for name, (list_name, description, items) in outputs.items():
        path = out / name
        if options.check:
            expected = json.dumps(
                {"name": list_name, "description": description, "api": REGISTRY_API, "schema": REGISTRY_SCHEMA, "plugins": items},
                ensure_ascii=False, indent=2,
            ) + "\n"
            if not path.exists():
                print(f"[失败] {path} 不存在")
                changed += 1
            elif path.read_text(encoding="utf-8") != expected:
                print(f"[失败] {path} 与模块目录不一致，请重新生成")
                changed += 1
            else:
                print(f"[通过] {path}")
        else:
            out.mkdir(parents=True, exist_ok=True)
            dump(path, list_name, description, items)
            print(f"[写入] {path}（{len(items)} 条）")

    if options.check:
        print(f"\n校验完成：不一致 {changed} 份")
        return 1 if changed else 0
    print(f"\n完成：共 {len(entries)} 个模块（影视 {len(vod)} / 成人 {len(adult)}）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
