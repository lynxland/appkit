# 安全策略

## 报告漏洞

**请勿通过公开 issue 报告安全漏洞。** 使用 GitHub 私密漏洞报告
（Security → Report a vulnerability），或联系维护者。我们会在 48 小时
内确认收到，并在修复发布前与你同步进展。

## 支持范围

- 本仓（appkit 核心 + contrib 子模块）的全部代码。
- 模块源码（lynx-go/lynx-mods 的 registry 内容）的安全公告走该仓的
  advisories 通道（`registry/advisories.json` + `lynx-mods audit`）。

## 供应链

- CI 含 govulncheck 与 gosec 级扫描对齐生态位（grpcapi 同款基线）。
- 发布物仅为 git tag（Go module proxy 分发），无二进制产物。
- 依赖升级经 MVS 钉版；冻结面（grpcapi SPI）依赖只在 additive 变更时抬升。
