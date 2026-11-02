# appkit

[![CI](https://github.com/lynxland/appkit/actions/workflows/ci.yml/badge.svg)](https://github.com/lynxland/appkit/actions/workflows/ci.yml)

**lynx 框架的 web 开发基础库**：以装配契约（Module/Assembly/Deps）与电池子包
（事务/分页/错误码/事件/迁移/鉴权原语…）规范基于 lynx 的后端应用的统一开发
范式——自有项目的公共基座，也是第三方可用的开源基建。

**The web foundation library for the lynx framework**: assembly contracts plus
batteries (transactions, pagination, error codes, events, migrations, auth
primitives…) that standardize how lynx-based Go backends are built — the shared
base for our own projects and open-source infrastructure for anyone.

> 找「把业务模块以源码装进应用」的 shadcn 式分发工具（init/add/update）？
> 它已迁至姊妹项目 **[lynxland/lynx-mods](https://github.com/lynxland/lynx-mods)**
> （CLI、registry、8 个业务模块）。本仓只保留范式基座：模块实现的是这里的契约。
> Looking for the shadcn-style module distribution (init/add/update)?
> It moved to **lynx-mods**. This repo keeps the paradigm base: modules
> distributed there implement the contracts defined here.

## 生态四仓（The ecosystem）

```
lynx （宿主框架：生命周期/传输/配置/事件总线）
  └─ appkit （本仓：web 开发基础库 = 契约 + 电池 + contrib 适配）   ← semver
       ├─ 依赖 grpcapi （peer：proto-first API 规范层，冻结 SPI）
       └─ lynx-mods （模块分发：registry + 安装 CLI + 8 业务模块）→ 依赖本仓契约
```

dogfood 序列：appkit-demo（金丝雀）→ torchwood（PG+bun 路径）→
fleetly（SQLite+database/sql 路径，验证核心不绑 ORM）。

## 结构（What's inside）

| 包 | 职责 | 分层 |
|---|---|---|
| `appkit`（根包） | 装配契约：Module/Deps/Assembly/InstalledModule/Aggregated——纯契约，无实现 | 契约层（嵌 grpcapi 冻结 SPI + lynx 宿主类型） |
| `compose` / `chain` | 装配协议实现：DAG 校验/事件目录交叉校验/聚合；grpcapi 标准拦截链组装 | 实现 |
| `uow` `crud` `clock` `errcode` `idgen` `jwt` `password` `secretkey` `goosekit` | 公共模式电池：事务端口、分页与排序基元、错误码区段、迁移隔离… | **严格中立**（零 lynx/grpcapi/ORM 依赖，守卫测试固化） |
| `configkit` `events` `contexts` | 配置分段、事件信封、Principal 语境 | 宿主/grpcapi 依赖 |
| `contrib/bunx` | bun 适配：OpenDB、Deps.DB 类型化出口（DB/MustDB）、事务实现、PG 测试基建 | 可选实现（独立子 module，独立 tag） |

**核心不绑 ORM**：`Deps.DB` 是 ORM 中立句柄（`any`），bun 只是 contrib 里的
默认适配——换 ORM 的项目实现 `uow.Runner` + 自带 contrib 式适配器即可
（见 [docs/extending.md](docs/extending.md)）。根 module 的 go.mod 与依赖树
零 `uptrace/bun`，由分层守卫测试机器证明。

## 快速上手（Quick start）

```go
import (
    "github.com/lynxland/appkit"
    "github.com/lynxland/appkit/compose"
)

// 应用组合根：登记已装模块，聚合装配产物
agg, err := compose.AssembleAll(deps, modules.Installed)
```

模块的分发安装（init/add/update/doctor）见
[lynx-mods](https://github.com/lynxland/lynx-mods)；写业务模块见其
module-authoring 指南；改名迁移（lynx-start → appkit，v0.7.0）与历史升级见
[UPGRADING.md](UPGRADING.md)。

## 开放性（Opinionated defaults, open seams）

我们带偏向（proto-first、bun 首选、goose、AIP 风格）作零配置默认；但每个
偏向都有公共接缝，第三方在自己仓库即可替换——永不需 fork：

| 偏向 | 接缝 |
|---|---|
| ORM=bun | `Deps.DB any` + `uow.Runner` + crud 中立基元 → 自写适配器 |
| 迁移执行器=goose | `Module.Migrations` 只是 `fs.FS` → 自写执行器 |
| API 面=grpcapi | 范式词汇（已声明边界）：非 proto-first 项目消费电池层 |
| 分页=crud | 库级辅助件而非契约强制 → 自带实现不违范式 |
| 分发=lynx-mods | git registry 协议（`--registry` 任意 git URL）→ 自建私有 registry |

完整接缝清单与适配器编写指南：[docs/extending.md](docs/extending.md)。
电池沉淀路线（数据访问族/幂等/outbox/错误码 v2…）：[docs/battery-roadmap.md](docs/battery-roadmap.md)。
API wire 形态规范：[docs/api-shapes.md](docs/api-shapes.md)。

## 非目标（Non-goals）

- 不做运行时框架——生命周期/传输/服务发现归 [lynx](https://github.com/lynx-go/lynx)。
- 不做 API 规范层——policy/网关/鉴权拦截归 [grpcapi](https://github.com/lynx-go/grpcapi)。
- 核心不绑单一 ORM/数据库方言（bun 适配在 contrib，其他自带）。
- 不做跨进程事件/分布式（outbox 在路线图，跨进程 bus 用 lynx watermill）。
- 根包无租户预设（软多租户横切约定见 lynx-mods ADR）。

Apache-2.0。破坏性变更遵守 [UPGRADING.md](UPGRADING.md)；安全披露见
[SECURITY.md](SECURITY.md)。
