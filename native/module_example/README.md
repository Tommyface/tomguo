# 站源模块示例

这里只有 `jsexample`：假数据模块，不访问网络，用于开发调试界面与模块契约。

**真实站源模块不在本仓库**，单独维护在私有仓库 `Tommyface/tomguo-modules`。

## 模块契约

`manifest.json`：

```json
{
  "id": "jsexample",
  "name": "JS 示例",
  "version": "1.0.0",
  "description": "假数据示例模块，不访问网络",
  "runtime": "js",
  "api": 1,
  "adult": false,
  "hosts": [],
  "capabilities": ["search"],
  "allowEmptyCatalog": true
}
```

字段说明：

| 字段 | 必填 | 说明 |
|---|---|---|
| `id` | ✅ | 小写字母、数字、下划线、连字符，最长 32 |
| `name` | ✅ | 显示名 |
| `version` | — | 三段版本号 |
| `runtime` | ✅ | 固定 `js` |
| `api` | ✅ | 模块接口版本，当前 `1` |
| `adult` | ✅ | 是否成人内容 |
| `hosts` | — | 会访问的域名，仅作说明 |
| `capabilities` | — | `search` / `cover` / `recommendations` 等 |
| `allowEmptyCatalog` | — | 允许空分类（示例模块用） |

`main.js` 导出：

```js
module.exports = {
  categories(),                 // [{id, name}]
  catalogPage(page, category),  // {items, hasMore}
  search(page, query),          // {items, hasMore}
  detail(sourceId),             // {drama, chapters}
  resolveMedia(task),           // {url, referer}
};
```

## 调试

把模块文件夹放进 App 数据目录的 `modules/` 下，或在「设置 → 站源模块 → +」装 zip。
