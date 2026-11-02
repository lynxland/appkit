# 扩展与替换指南（Extending appkit）

appkit 带偏向（proto-first API 面、bun 首选、goose 迁移、AIP 风格
分页…）作为零配置默认，但**每个偏向都有公共接缝**——你可以在自己的
仓库实现替换或扩展，永不需要 fork 本库（ADR-0009 D9）。

## 接缝总表

| 偏向 | 默认 | 接缝 | 替换姿势 |
|---|---|---|---|
| ORM | `contrib/bunx` | `appkit.Deps.DB` 是 `any`（ORM 中立句柄）+ `uow.Runner/Isolator` 接口 + `crud` 中立分页/排序基元 | 自写 contrib 式适配器（见下节） |
| 迁移执行器 | `goosekit`（per-module 独立版本表） | `appkit.Module.Migrations` 只是中立的 `fs.FS` | 自写执行器消费同一 FS 约定 |
| API 面 | grpcapi 冻结 SPI（嵌入契约层） | 范式词汇本身——**已声明的边界**：非 proto-first 项目请消费电池层（uow/crud/errcode/…均为独立可用的中立件） | 装配面中立化（Faces 挂点）记 revisit：出现真实非 proto-first 模块需求或 ConnectRPC 等新范式时再议 |
| 事件 | `events.Publisher` + lynx eventbus | Publisher 是本仓接口 | 实现接口（outbox/跨进程形态见 battery-roadmap ⑧） |
| 分页 | crud（多模式框架，D11） | crud 是**库级辅助件而非契约强制** | 自带分页实现（如 fleetly 的裸 keyset）不违范式 |
| ID/时钟/密码/加密 | 电池（选装） | 电池即选装 | 不用即可/自带实现 |
| 契约稳定性 | — | 根包 semver 承诺 | 第三方可安全依赖与锁定版本 |

## 写一个 ORM 适配器（contrib 式）

以 contrib/bunx 为参照，一个新 ORM 适配器（如 sqlc/GORM/裸 database/sql）
只需要：

1. **独立 module**：`yourrepo/yourormx`，require
   `github.com/lynxland/appkit vX.Y.Z`。
2. **类型化出口**：实现 `DB(deps appkit.Deps) (*your.DB, error)` 访问器
   （约定 `Deps.DB` 装载你的句柄类型）；参考 contrib/bunx 的
   `DB()/MustDB()`。
3. **事务实现**：实现 `uow.Runner`（必要时 `uow.Isolator`）——把
   `Run(ctx, fn)` 的事务注入 ctx，使仓储层经 ctx 取事务（domain 端口
   不得把驱动类型写进签名）。
4. **列表执行**（可选）：参照 `bunx.List` 把 `crud.ListParams`/排序
   基元编译成你的查询形态。
5. **测试基建**（可选）：PG/SQLite 门控测试夹具，参考
   `contrib/bunx/testutil`（env `APPKIT_TEST_PG_DSN`）。

## 分层承诺（对你的约束，也是对你的保护）

- 严格中立电池（uow/crud/clock/errcode/idgen/jwt/password/secretkey/
  goosekit）永不 import lynx/grpcapi/uptrace——`layers_guard_test.go`
  机器固化，非 bun 应用可放心消费。
- 宿主电池（configkit/events）绑 lynx；contexts 让渡 principal 权威给
  grpcapi——依赖形态见各包 godoc。

## 自建模块分发

appkit 只定义契约；模块分发（registry/add/update）是姊妹项目
**lynx-go/lynx-mods**。registry 是 git 协议：`--registry` 接受任意 git
URL（https/git@/ssh/file），第三方可自建私有 registry，模块经
kernelConstraint 声明对契约版本的约束。

## 贡献回上游

电池准入三件套（测试+dogfood 采纳+文档，见 docs/battery-roadmap.md
「准入与弃位纪律」）；范式级变更走 ADR（docs/adr/）。
