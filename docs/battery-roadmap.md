# 电池路线图（Battery Roadmap）

lynx-start 作为 lynx 框架的 web 开发基础库，其电池沉淀遵循 ADR-0009 的
三桶纪律：**考古桶**（dogfood 项目重复手搓的证据，最强信号）／**蓝图桶**
（通用 web 基建缺口，须有消费者排期才动工）／**投机桶**（只记触发器，
不预建）。每个电池走独立循环：**提取泛化 → 入 lynx-start（含测试）→
dogfood 换 import 采纳 → 版本化发布**。

## 准入与弃位纪律

- **准入门槛**（roadmap → 核心）：测试、dogfood 采纳、文档三件套齐才毕业。
- **弃位流程**（核心 → 移除）：先标记 `// Deprecated:`（附替代方案），
  保留一个 minor 周期后移除（学 grpcapi compat 垫片节奏）。
- **分层约束**（ADR-0009 D6）：哨兵/规约/纯逻辑在核心；驱动映射与 SQL
  编译在 `contrib/`；`layers_guard_test.go` 机器固化。

## 考古桶（dogfood 重复证据）

| 优先 | 电池 | 证据 | 形态 |
|---|---|---|---|
| ① | **crud v2 多模式分页**（D11）✅ 核心已落地（v0.5.1：Spec/Parse[Filter]/Cursor/Predicates + bunx.ApplyPage，v1 标 Deprecated） | torchwood pkg/crud 同源逐字 + fleetly ADR-0026 反向选择 keyset | 剩余：模块面换装（notification/audit 等列表方法迁 v2，随模块版本）。**torchwood 已采纳**（2026-11-01：pkg/crud 中立基元转发层消费 v0.5.1；签名机械暂留本地随 v2 退役） |
| ② | **storeerr 存储错误归一**（D12）✅ 核心已落地（v0.5.2：哨兵集/Wrap 多 %w/Classify 显式注入分类器）+ bunx 适配（v0.1.2：MapError/Classify，pgdriver 值类型坑收口） | **3×**：模块 pgdriver 23505 值类型坑、torchwood MapDocumentDBError、fleetly IsUniqueViolation 族 | **模块试点已落地**（2026-11-01：identity/organization v0.2.1 换装 IsAlreadyExists(bunx.MapError)，真库负路径+完整 e2e 绿）。剩余：其余六模块随动；torchwood docdb_errors 换映射底座 |
| ③ | **occ 乐观并发/状态机 CAS** | 2×：torchwood current_version+expected_version 字段违规回传、fleetly Transit（state∈from + generation 单调 + superseded_by） | version 列规约 + IfVersion 更新 helper + ErrConflict（与 storeerr 哨兵咬合）；写侧与 crud 读侧对偶 |
| ④ | **行规约（时间/审计/tombstone 列）** | 2×：fleetly FormatTime 单源+FakeClock 黄金时间戳+tombstone never-reuse、torchwood nullzero+审计表 | 核心=规约类型+时钟格式约定（RFC3339 UTC）；contrib/bunx=BeforeAppendModel hooks 自动维护 |
| ⑤ | **idem 幂等执法拦截器** | 2×：fleetly application/idem（claim/replay/fingerprint/principal 作用域）、torchwood documents 域内手搓 | grpcapi Stage 形态；Idempotency-Key 头协议 |
| ⑥ | **periodic + RunnerService** | torchwood Periodic（WithoutCancel 预算语义+RunAtStart，15+ 循环单一实现）+ fleetly engine.Loop 单写者骨架 | `periodic` 电池 + Run(ctx)→lynx.Service actor 壳 |
| ⑦ | **errcode v2 + errstatus** | 两家富处互补：fleetly 注册表 fail-fast+golden+usage 反扫+apperr/apperrstatus 分层 ⊕ torchwood ErrorInfo detail/字段违规/OCC metadata 投影 | v2 注册表 + gRPC status 投影伴生包 + **i18n 消息目录**（错误码为键，locale 目录，开源基建面向第三方） |
| ⑧ | **outbox 骨架** | 2×：torchwood PG LISTEN/NOTIFY+SKIP LOCKED+死信/重领/保留窗口全套、fleetly SQLite 事实流+SSE | 可插 dialect/transport 的事务性 outbox；含变更流读侧（after 订阅/断档 410/补偿轮询）与信封 Seq/TransactionID 批标识；**events.Publisher 双实现桥接指引**（进程内 bus / outbox→watermill）随之成文；「四件一拍」写组合（D12）定典入 uow 文档 |
| ⑨ | **守卫测试模板** | fleetly internal/guards ~60 个、torchwood import guard 家族 | testutil 守卫模板（import/uow/分层/租户过滤）；本仓 layers_guard_test.go 是首个自用样例 |
| ⑩ | **goosekit 全局单表模式** | fleetly 单链单库（解锁 fleetly 采纳） | goosekit 支持非 per-module 的全局版本表模式 |

分发器改进（随循环暴露）：add 安装期自动应用 item.json goDependencies（现为打印提示，离线友好优先——模块源码用到新电池版本时地板不抬会编译失败，本循环已靠抬 init 地板修复）。第二梯队（单侧成熟，等第二消费者）：semaphore（Redis SET-NX+Lua 防误删；
与 lock 同族）、secretbox（enc:v1 透明列加密，与 secretkey 互补）、
idgen 增量（snowflake 时钟回拨自旋/APIKeySecret/NormalizeStrategy）、
query Filter AST 裁剪版（纯组合器内核，DSL/pgvector 留应用）、testutil
增量（advisory-lock 并行安全+SQLSTATE 瞬时重试+Eventually）、FakeClock
hermetic 夹具（temp 真库+假时钟）、upsert/批量写 helper、Queue 端口
（Enqueue/Dequeue/Ack/Trim at-least-once）。

## 蓝图桶（通用 web 基建缺口）

| 电池 | 理由 | 备注 |
|---|---|---|
| ratelimit 限流 | grpcapi 只有「参考实现」槽位；fleetly/torchwood 各有实现 | 令牌桶+滑动窗口；进程内实现+分布式接口 |
| testkit 进程内全链 harness | fleetly apitest（bufconn+fake providers，1171 行蓝本）；模块生态刚需 | bufconn gRPC+REST 夹具+fake Deps 构造集+golden 工具；examples/ 活文档随此起步 |
| webhook 出站交付 | torchwood webhooks 蓝本（签名+重试）；Stripe 式平台经典件 | HMAC 签名+退避重试+端点管理，骑 outbox/queue |
| objectstore/blob 端口 | torchwood 对象存储+fleetly 内容寻址上传（双侧弱证据） | Put/Get/Presign/Delete 端口+mem 实现起步 |
| cache 端口+singleflight | web 基建标配 | **触发器=首个消费者**（纪律优先于完备） |
| lock 分布式锁 | torchwood advisory-lock 经验 | 与 semaphore 同族电池 |
| 观测约定 | torchwood outbox 指标全家 | 模块贡献 metrics 命名规约+contrib/bunx 查询 otel hook |

## 投机桶（只记触发器，不预建）

- OAuth2/OIDC 客户端 + TOTP —— 触发：identity 模块社交登录/MFA 需求。
- usage/quota 计量 —— 触发：首个计量计费模块。
- feature flags —— 触发：首个特性门控需求。
- email/短信通道端口 —— 触发：notification 模块外发需求。
- 搜索/全文/向量检索 —— 永属应用层，不上提。

## 引用不重建（生态已覆盖）

重试/断路器（lynx client/http）、生命周期/健康/排水（lynx）、选主
（lynx contrib/cluster）、跨进程 bus（lynx contrib/watermill）、
csrf/cors/SSE/WS/OpenAPI（grpcapi）、配置加载（lynx+configkit）、
DI（手写装配，ADR-0005）、CLI 框架（lynx-go/commands，peer——归属
revisit：若 lynx-start 应用 CLI 供给链成型再议并入）。

## 明确不上提（项目特有）

torchwood：ident schema 命名、ExecIdentity/RLS 变色龙、runbook、实时
WS 面。fleetly：bundle UoW 形态（ADR-0080 架构选择，记 uow 第二风格
revisit 候选）、seq 可读 ID（平台语义）、SSE ticket、k3s/swarm providers。

## dogfood 分工

- **lynx-start-demo**：金丝雀——每次契约/电池变更的第一个应用面验证。
- **torchwood**：PG+bun 路径（contrib/bunx 主消费者）+ grpcapi v1.0
  验收场——crud/storeerr/occ/periodic/outbox 的主力验证项目。
- **fleetly**：SQLite+database/sql 路径——核心不绑 bun 的酸测场
  （`go list -deps` 恒无 uptrace）；errcode v2/goosekit 全局单表/
  FakeClock 夹具的供给方与采纳方。
