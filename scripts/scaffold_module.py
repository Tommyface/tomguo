#!/usr/bin/env python3
"""为内置站源生成 zip 模块骨架（S7 迁移工具）。

用法：
  python3 scripts/scaffold_module.py <source-id> [--out modules/]

生成 <out>/<id>/manifest.json 与 main.js：
  - manifest 里 api=1、adult=false，hosts 留空待填；
  - main.js 提供五个导出的空实现与解析辅助模板。

迁移分五步（见 docs/模块迁移手册.md）：
  1. 生成骨架  2. 填写抓取与解析  3. 影子对比  4. 切白名单  5. 删除 provider 文件
"""

import argparse
import json
import pathlib
import sys

# 每个源的元信息：中文名、站点域名、默认成人、分类。
# 新增源必须同时出现在这里与 native/core/source_registry.go，
# 迁移完成后从两边一起删除。
SOURCES = {
    "guipian":     ("鬼片", False, ["https://www.guipianwu.com"]),
    "hanxiaoquan": ("韩小圈", False, ["https://www.jennyhow.com"]),
    "sorani":      ("青空次元", False, ["https://sorani.net"]),
    "mxdm":        ("MX动漫", False, ["https://www.dcc3.com"]),
    "jcydm":       ("囧次元", False, ["https://www.jcydm1.com"]),
    "huaguo":      ("花果", False, ["https://www.zywest263.com"]),
    "wangguo":     ("网果", False, ["https://www.duanju2.com"]),
    "faguo":       ("发果", False, ["https://www.xzyx168.com"]),
    "wuguo":       ("伍果", False, ["https://www.duanju55.com"]),
    "ezdmw":       ("E站动漫", False, ["https://www.ezdmw.org"]),
    "hema":        ("河马剧场", False, ["https://www.kuaikaw.cn"]),
    "chengguo":    ("橙果", False, ["https://www.chengguodj.com"]),
    "huakui":      ("花魁", False, ["https://huakui.net"]),
    "huangju":     ("剧果", False, ["https://www.huangju.net"]),
    "yeguo":       ("野果", False, ["https://www.ygdj7.com"]),
    "dsd":         ("DSD", False, ["https://www.dsd.com.se"]),
    "cloudfront":  ("CloudFront", False, []),
    "51duanju":    ("51短剧", False, ["https://www.51duanju.com"]),
    "hongguo":     ("红果", False, ["https://www.hongguoduanju.com"]),
    "huangdou":    ("黄豆", True, ["https://tideember.cc"]),
    "huangguoai":  ("黄果", True, ["https://huangguoai.com"]),
    "huangguo-video": ("黄果视频", True, ["https://huangguo.video"]),
    "huangguo-official": ("黄果官网", True, ["https://falpius.com"]),
    "huangguo-main":  ("黄果主站", True, ["https://falpius.com"]),
    "huangguo-selected": ("黄果精选", True, ["https://falpius.com"]),
    "huangguo-welfare":  ("黄果福利", True, ["https://falpius.com"]),
}

MODULE_TEMPLATE = '''\
'use strict';
// {name}（{id}）：迁移自 native/core/provider_*.go。
// 站点：{hosts}
//
// 导出为 async（宿主 invoker 会等待 Promise）。若目标客户端仍只支持同步模块，
// 把 async/await 去掉即可——invoker 对两种写法都兼容。
//
// 迁移检查清单：
//   [ ] catalogPage 能返回非空 items，字段含 id/source/sourceId/title/cover
//   [ ] search 能按关键词返回结果（无搜索能力时返回空列表而不是抛错）
//   [ ] detail 的 chapters 数量与站点一致
//   [ ] resolveMedia 返回可 HEAD 200 的地址
//   [ ] 所有出错路径抛 Error（带可读中文），不要返回 null

var SITE = '{site}';
var UA = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36';

var CATEGORIES = [
  {{ id: '', name: '全部' }},
];

function fetchText(url, referer) {{
  var response = host.fetch({{
    url: url,
    method: 'GET',
    headers: {{ 'User-Agent': UA, Referer: referer || SITE + '/' }},
    timeoutMs: 12000,
  }});
  if (!response || response.status < 200 || response.status >= 400) {{
    throw new Error('请求失败：' + (response ? response.status : 'no response'));
  }}
  if (response.body) return String(response.body);
  if (response.bodyBase64) return String(host.base64Decode(response.bodyBase64));
  return '';
}}

// TODO: 按站点结构实现解析。
function parseCards(html) {{
  var items = [];
  return items;
}}

exports.categories = async function () {{
  return CATEGORIES;
}};

exports.catalogPage = async function (page, category) {{
  // TODO: 拼接分类分页地址并解析
  throw new Error('{id} 目录尚未实现');
}};

exports.search = async function (page, query) {{
  // TODO: 拼接搜索地址并解析；站点不支持搜索时返回空列表
  return {{ items: [], hasMore: false }};
}};

exports.detail = async function (sourceId) {{
  var id = String(sourceId || '').trim();
  if (!id) throw new Error('剧集标识无效');
  // TODO: 抓详情页、解析剧集列表
  throw new Error('{id} 详情尚未实现');
}};

exports.resolveMedia = async function (task) {{
  // TODO: 按章节抓播放页，提取 m3u8/mp4
  throw new Error('{id} 播放地址尚未实现');
}};
'''


def main() -> int:
    parser = argparse.ArgumentParser(description="生成站源模块骨架")
    parser.add_argument("source", help="内置源 id，如 jcydm")
    parser.add_argument("--out", default="modules", help="输出根目录，默认 modules/")
    parser.add_argument("--force", action="store_true", help="覆盖已存在的目录")
    options = parser.parse_args()

    source = options.source.strip().lower()
    if source not in SOURCES:
        print(f"[失败] 未收录的源 {source}；已收录：{', '.join(sorted(SOURCES))}")
        return 1
    name, adult, hosts = SOURCES[source]
    root = pathlib.Path(options.out) / source
    if root.exists() and not options.force:
        print(f"[失败] {root} 已存在；加 --force 覆盖")
        return 1
    root.mkdir(parents=True, exist_ok=True)

    manifest = {
        "id": source,
        "name": name,
        "version": "1.0.0",
        "description": f"{name}：迁移自内置实现。",
        "runtime": "js",
        "api": 1,
        "adult": adult,
        "hosts": [h.replace("https://", "") for h in hosts],
        "capabilities": ["cover"],
        "allowEmptyCatalog": True,
        "mainSha256": "",
    }
    (root / "manifest.json").write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    (root / "main.js").write_text(
        MODULE_TEMPLATE.format(
            id=source, name=name, site=hosts[0] if hosts else "", hosts="、".join(hosts) or "待补"
        ),
        encoding="utf-8",
    )
    print(f"[生成] {root}/manifest.json")
    print(f"[生成] {root}/main.js")
    print("下一步：填好实现后运行 python3 scripts/fill_module_digest.py " + str(root))
    return 0


if __name__ == "__main__":
    sys.exit(main())
