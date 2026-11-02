# ADR-0010: 项目更名 lynx-start → appkit

日期：2026-11-02
状态：已接受

## 背景与决策

ADR-0009 定位调整后，本仓是「lynx 框架的 web 开发基础库」——契约 +
电池 + contrib 适配，不再是起手模板仓。「lynx-start」的名字停在起手
语义上，与实际供给（每个项目每天 import 的基座）不符。项目更名
**appkit**：module path、根包名、测试 env 一并切换，全生态（lynx-mods /
appkit-demo / torchwood）同一变更窗完成，不留半改名状态。

## 决策账本

- **D1 module path `github.com/lynxland/appkit`**：GitHub org 已更名
  `lynx-go` → `lynxland`，新 module path 用新 org（仓更名后旧 URL 自动
  重定向）。嵌套子模块同步：`github.com/lynxland/appkit/contrib/bunx`。
  依赖（lynx / grpcapi）的 module path 仍为 `github.com/lynx-go/*`——
  那是各仓自己的迁移决策，其已发布版本经 proxy 缓存可得，本仓不代改。
- **D2 根包名 `kernel` → `appkit`（推翻 ADR-0009 D2「包名不变」）**：
  当初保留 kernel 是为了让路径迁移零限定符替换；本次改名本身已强制
  下游动每一行 import，是替换限定符的最便宜时机，一并切齐可永久免去
  `kernel "…/appkit"` 显式别名（goimports 猜包名之税）。当初的 sed
  误伤顾虑（`kernelConstraint`）不成立：该字段属 lynx-mods item
  schema，不在本仓替换范围。
- **D3 版本谱系**：新路径自 **v0.7.0** 起（0.x 阶段 minor 破坏承诺），
  contrib/bunx 自 **v0.4.0** 起。旧 tag（`github.com/lynx-go/lynx-start`
  谱系全部 40 个）经 module proxy 缓存长期可得，缓迁的下游不受影响；
  GitHub 仓更名后 direct 模式下旧路径 go-get 重定向至新仓而 module
  path 不匹配、解析失效——预期内，全生态已同窗切换。
- **D4（修订 ADR-0009 D80/D10「env 名不改」）**：`LYNXSTART_TEST_PG_DSN`
  → `APPKIT_TEST_PG_DSN`，**无兼容期**——消费方（本仓 CI、lynx-mods
  CI 与文档）同一变更窗切换。当初「跨仓协调成本」在全生态一次切的
  前提下消失。
- **D5 `lynxstart-page-token-v1` HMAC 派生 purpose 永久保留**：改值即
  作废在途已签名分页 token；该机制已 Deprecated（D11 多模式框架替代），
  值本身只是密钥派生盐，保留旧名无暴露成本。
- **D6 bunx 对根模块的 require 过渡**：改名提交中省略该行、v0.7.0
  tag 发布后由 `go mod tidy` 补回——workspace 成员 require 一个不可
  解析的版本会令 go.work 联编失败（实测），省略行是唯一不引入
  replace 指令的干净过渡（本仓教义：无 replace）。

## 生态影响

| 仓 | 变更 |
|---|---|
| lynx-mods | CLI 版本解析/init 生成物/地板常量、scaffold 模板、8 模块 require+import；`lynx-start/modules/` 双前缀兼容**原样保留**（指向本仓历史 tag 的安装面，与根 module 改名无关） |
| lynx-start-demo | 更名 **appkit-demo**（module path `github.com/lynxland/appkit-demo` 一并），require/import 全量切换 |
| torchwood | require + `pkg/crud` 转发层 import 切换 |
| uikit | 设计文档 2026-10-31 补记属历史记述，不改 |

历史文档（CHANGELOG 旧条目、UPGRADING 旧节、ADR-0001/0003/0005/0009、
git tag 与提交信息）原样保留；本 ADR 是改名的定典与教义修订记录。
