# 推送到新仓库

`Tommyface/tomguo` 已重建为**非 fork** 的空仓库，但当前 GitHub token 是细粒度
（按仓库 ID 授权），无法写入新建仓库。请用下面任一方式推送本地已清理的历史。

## 方式一：本地 git push（推荐）

仓库已备好干净的 17 个提交（旧提交里的密钥已全部替换为 `REDACTED_*`）。

```bash
cd /root/workspace/tomguo
git push -u origin main --force
```

首次会要求凭据，用 GitHub 用户名 + Personal Access Token（需 `repo` 全权限）。

## 方式二：从 bundle 恢复

`/sdcard/Download/DSH-Go/tomguo-clean.bundle` 是完整仓库包（1.0 MB，17 提交）。

```bash
git clone tomguo-clean.bundle tomguo
cd tomguo
git remote set-url origin https://github.com/Tommyface/tomguo.git
git push -u origin main
```

## 方式三：给现有 token 加权限

去 https://github.com/settings/tokens 编辑该 token：

- Repository access 改为 **All repositories**，或把 `Tommyface/tomguo` 加进列表
- Permissions → **Contents: Read and write**

改完把 token 发我，我直接推。

## 推送后要做的事

1. **Actions 会失效**：新仓库需要重新启用 workflow。
   仓库 → Settings → Actions → General → 允许所有 action。
2. **Secrets 需重设**：如果 workflow 用到 `secrets.*`，在新仓库补上。
3. **旧 fork 已删除**：`tomguo-fork-backup`（含旧历史密钥）已删掉。
   上游 `Topkill/guoapp5` 仍在，但本仓已不再是它的 fork。

## 现在的仓库结构

| 仓库 | 可见性 | 内容 |
|---|---|---|
| `Tommyface/tomguo` | public（空，待推） | 应用骨架：模块运行时、网络层、界面、构建 |
| `Tommyface/tomguo-modules` | **private** | 四个站源模块（含密钥）+ `build.py` + `dist/*.zip` |

## 站源模块怎么装

私有仓 `dist/` 下有现成的 zip：

```
dist/huangguo-official.zip   6.5 KB
dist/huangdou.zip            4.4 KB
dist/huanggua.zip            2.9 KB
dist/heitao.zip              3.2 KB
dist/index.json              模块清单（含 sha256）
```

手机上下载后，设置 → 站源模块 → `+` 选 zip 安装。

因为仓库是 private，raw 链接需要 token，所以**不做自动市场**，手动装 zip。
