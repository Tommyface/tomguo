# 来源说明

## 上游

本仓库改编自 [`Topkill/guoapp5`](https://github.com/Topkill/guoapp5)。
上游仓库未声明开源许可，其代码著作权归上游作者所有。

## 本仓库的改动

相对上游，本仓库新增或重写了：

| 领域 | 内容 |
|---|---|
| 模块系统 | `native/core/module_*.go`（约 2800 行）：goja 运行时、host 能力注入、模块安装与清单 |
| 网络层 | DoH 解析、IP 钉选、CDN 多线路、请求限速与诊断 |
| 界面 | 站源模块管理页、启用开关、同名换源、播放器适配 |
| 构建 | GitHub Actions 出包、版本号与发布流程 |

## 站源解析

站源解析实现（`provider_*.go` 与站源模块）**不在本仓库**，单独维护在私有仓库
`Tommyface/tomguo-modules`。

本仓库只包含：

- 模块运行时（如何加载与调用模块）
- `host` 能力接口（模块能用什么）
- `jsexample` 示例模块（假数据，不访问网络，用于开发调试）

## 第三方

| 组件 | 许可 |
|---|---|
| Flutter / Dart | BSD-3-Clause |
| media_kit | MIT |
| goja | MIT |
| tls-client | BSD-3-Clause |
| 其他 Go 依赖 | 见 `native/go.sum` 对应模块 |
