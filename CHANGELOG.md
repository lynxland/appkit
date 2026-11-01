# Changelog — lynx-start

lynx-start（v0.5.0 起为仓库根 module）对外做 semver 承诺:破坏性变更升主版本
(0.x 阶段升 minor),向后兼容追加升 patch。条目按发布时间倒序。
v0.5.0 前的条目是 kernel 子 module 谱系(`github.com/lynx-go/lynx-start/kernel`,
tag `kernel/v*`);模块与 CLI 的历史 changelog 随分发迁至 lynx-go/lynx-mods。

## contrib/bunx v0.2.1（2026-11-01）

- testkit 修正两处让联动测试真正跑通的面：① **Services 驱动**——agg.Services
  （事件订阅者等 lynx.Service）逐个 Init 后以 Start 阻塞协程运行
  （watcher 在 Start 里订阅，此前无人驱动）；② **Events 复合发布**——
  publishTelescope（先 BusPublisher 投递总线使订阅者联动，后
  CapturePublisher 记录供断言；此前只捕获不投递，watcher 收不到事件）。

## contrib/bunx v0.2.0（2026-11-01）

- **新子包 `testkit` 进程内全链测试夹具（ADR-0009 D13 蓝图桶⑦，fleetly
  apitest 蓝本）**：一组 InstalledModule 在隔离测试库（真 PG 门控）上
  装配——Deps 组装（CapturePublisher/内存 bus/viper Config）→
  compose.AssembleAll → policy 集 → chain.DefaultChain →
  grpcapiserver.New（fail-closed 校验全继承）→ GRPCServer()/
  HTTPHandler() 逃生口经 bufconn/httptest **无端口**起服。测试拿到真实
  gRPC ClientConn 与 REST base URL；`Eventually` 提供内存总线异步投递
  的确定性等待。定位于 contrib（偏向装配层的测试面：真 PG+bun 适配，
  root 不反向引用）。

## v0.5.8 (2026-11-01)

- **新电池 `periodic` 周期作业骨架（ADR-0009 D13 考古桶⑧，torchwood
  Periodic 泛化——其 worker 侧 12+ ticker 循环的单一实现，fleetly
  engine.Loop 的 panic 恢复语义并入）**：`Periodic`（实现 lynx.Service，
  可直挂 Assembly.Services）——ticker+select 循环、RunAtStart（重启补
  窗口）/After（首轮延迟）、单轮预算
  `WithTimeout(WithoutCancel(ctx), budget)`（**优雅关停不腰斩在途轮次**，
  轮次在预算边界自然收束，幂等由下轮收敛）、单轮 panic 恢复续跑
  （含栈日志）、关停期失败日志抑制。`RunnerService`（torchwood bootkit
  泛化）：把 Run(ctx) error 型后台循环包成 lynx.Service——Start 阻塞
  actor 契约、Stop 取消+有界排水。宿主电池层位（禁 grpcapi/uptrace）。

## v0.5.7 (2026-11-01)

- ratelimit：`Options.Key` 返回空串的语义定为**跳过限流**（未登记方法
  不限流——方法级选配的惯用形态；v0.5.6 行为是共享空桶，属语义缺陷，
  无消费者窗口内立即修正）。与 grpcapi 参考实现的「空串=默认键」有意
  偏离，已文档化。

## v0.5.6 (2026-11-01)

- **新电池 `ratelimit`（ADR-0009 D13 蓝图桶⑥）**：grpcapi 仅有固定窗口
  参考实现，本电池补齐——`TokenBucket`（burst+平滑补充，突发友好、
  长期均值收敛补充率）与 `SlidingWindow`（精确时间戳环，消除固定窗口
  边界突刺），`Limiter` 端口（Decision 含 Limit/Remaining 供
  X-RateLimit-* 投影）+ 惰性清扫无后台协程。Stage 经
  interceptor.RateLimitStage 包装：命名/Order 40 槽位/
  ResourceExhausted+RetryInfo（gateway→Retry-After）继承 grpcapi 冻结
  语义。分布式限流走端口自带适配（fail-open/closed 由适配器自决，
  torchwood API key 失败节流教训）。

## v0.5.5 (2026-11-01)

- **新电池 `idem` 幂等执法拦截器（ADR-0009 D13 考古桶⑤，证据 2×：
  fleetly application/idem、torchwood 域内手搓）**：grpcapi 依赖电池
  （interceptor.Stage，Order 45——authn 后 authz 前，fleetly 链位同构）。
  `Store` 端口（Claim 原子裁决：Claimed/Replayed/InProgress/Mismatch）
  + `MemStore`（单进程，惰性清扫）+ 指纹 sha256(FullMethod+确定性
  proto 序列化) + principal 作用域（匿名回退 anon）+ 失败 Release 可
  重试 + 回放经 dynamicpb 从缓存字节重建响应（进程内全局 proto 注册表；
  空字节体如 google.protobuf.Empty 合法）。claim/replay 窗口缺省取
  fleetly 实证值 90s/24h；分布式声明走 Store 适配（Redis，torchwood
  semaphore 路线）。仅 unary；stream 直通（幂等语义不成立）。

## v0.5.4 (2026-11-01)

- **新电池 `row` 行规约（ADR-0009 D12，dogfood 首循环④收官，证据 2×：
  fleetly FormatTime 单源+FakeClock 黄金时间戳+tombstone never-reuse、
  torchwood nullzero 约定）**：`Stamps`（created_at/updated_at 列声明，
  嵌入即满足 `Stamper`）+ `Tombstone`（deleted_at：非零=删、永不复用、
  MarkDeleted 幂等不覆写首次时刻）+ `FormatTime/ParseTime`（规范文本
  形态：RFC3339/UTC/秒精度——TEXT 存储单源与 crud keyset 游标时间轴
  编码，字典序=时间序）。contrib/bunx 新增 `StampInsert/StampUpdate`
  （显式戳记、时钟注入、非零不覆写）与 `SoftDelete/ExcludeTombstoned`。
  **刻意不做魔法钩子**：bun 的 DB 级 QueryHook 拿不到模型、模型级
  BeforeAppendModel 会把 ORM 词汇渗进中立契约、隐式时间源不可追溯——
  显式一句式与 torchwood 列白名单/fleetly commitTransition 同哲学。

## v0.5.3 (2026-11-01)

- **新电池 `occ` 乐观并发/状态机 CAS（ADR-0009 D12，dogfood 首循环③，证据
  2×：torchwood expected_version、fleetly Transit）**：`Next`（版本从 1 起
  单调）、`Check`/`Expect`（失配→携带两侧值的 ErrConflict 包裹，与 storeerr
  咬合）、`Transition`（From 起点集/To 目标；空 From=任意起点；非法迁移→
  ErrConflict）。contrib/bunx 新增 `IfVersion`（WHERE version=expected +
  自增）、`IfState`（Transit 形态守卫 UPDATE）、`MapGuardedUpdate`
  （0 行→ErrConflict，驱动错误经 MapError）。

## contrib/bunx v0.1.2 (2026-11-01)

- 新增 `MapError`/`Classify`（storeerr 的 pgdriver 适配，见根 v0.5.2 条目）；
  require lynx-start v0.5.2。

## contrib/bunx v0.1.3 (2026-11-01)

- 新增 `IfVersion`/`IfState`/`MapGuardedUpdate`（occ 适配，见根 v0.5.3 条目）；
  require lynx-start v0.5.3。

## contrib/bunx v0.1.4 (2026-11-01)

- 新增 `StampInsert/StampUpdate/SoftDelete/ExcludeTombstoned`（row 行规约
  适配，见根 v0.5.4 条目）；require lynx-start v0.5.4。

## v0.5.2 (2026-11-01)

- **新电池 `storeerr` 存储错误归一（ADR-0009 D12，dogfood 首循环②，证据 3×）**：
  领域哨兵集（AlreadyExists/NotFound/Conflict/FKViolation/Busy）+ `Wrap`
  （多 %w 双保留：哨兵与驱动原因同时 errors.Is 命中）+ 谓词族 +
  `Classify`（哨兵语义优先，驱动分类器显式注入——无全局注册表；
  唯一冲突类=永久、资源忙类=瞬时）。contrib/bunx 新增 pgdriver 适配：
  `MapError`（SQLSTATE→哨兵，含 ErrNoRows→NotFound；pgdriver.Error 值类型
  As 坑收口）与 `Classify`（SQLSTATE 白名单，瞬时集源自 torchwood 重试
  经验：连接类/死锁/序列化失败/资源忙）。occ 电池（循环③）将复用
  ErrConflict。

## v0.5.1 (2026-11-01)

- **crud v2 多模式分页框架（ADR-0009 D11 定典落地，dogfood 首循环①）**：
  `Spec`（方法级声明：开放模式/排序白名单/limit 钳制/唯一决胜列）+
  `Parse[F]`（模式裁决：After→keyset、Offset→offset、首页按已开放优先级；
  决胜列强制追加、白名单 fail-closed）+ `Cursor`（keyset 游标=编码
  「排序键值向量+order 摘要+filter 摘要」，encode-not-sign 无密钥仪式，
  换序/换滤自失效）+ `PagePlan`（执行面，适配层消费）+ `ClampLimit`/
  `HasMore`；`Predicates` 最小 typed 谓词集（等值/IN/区间，AND）实现
  FilterDigester。contrib/bunx 新增 `ApplyPage`（三模式 SQL 形态；keyset
  用展开 OR 谓词，混合方向排序正确）。组合矩阵测试覆盖
  三模式×filter（绑定/未绑定）×orderby（默认/显式/决胜列去重）。
- **v1 分页 API 标记 Deprecated**（ListParams/ParseListParams/
  FinalizeOffsetPage/SliceOffsetPage/FilterDigest/InitPageTokenSigning/
  PageTokenData/TimeCursor）：保持编译兼容，随模块换装 v2 退场。

## v0.5.0 (2026-10-31)

**定位调整(ADR-0009):仓库转型为「lynx 框架的 web 开发基础库」,模块分发
(registry/CLI/scaffold)整体迁往 lynx-go/lynx-mods。**

- **破坏:module path 变更**——kernel 升为仓库根 module
  `github.com/lynx-go/lynx-start`(根 tag `v*`;`kernel/v*` 成为历史),
  根包名 `kernel` 不变;全部子包 import 路径 `…/kernel/<sub>` → `…/<sub>`。
- **破坏:根包纯契约化(D6)**——`ValidateModules/AssembleAll/PolicyFiles/
  SortModules/Descriptors` 移至新包 `compose`;`DefaultChain` 移至新包 `chain`;
  根包仅余契约类型与 `ValidateName/KeyOrName` 规则函数。
- **破坏:ORM 无关化(D5)**——`Deps.DB` 由 `*bun.DB` 改 `any`(ORM 中立
  句柄);bun 全家(bunx/uow.buntx/testutil)下沉新子 module
  `contrib/bunx`(独立 tag,谱系承接 kernel v0.4.0 资产,提供
  `DB()/MustDB()` 类型化出口;bun 钉 v1.3.0);
  `OrderClause/ParseOrderBy/SortedFields` 上移核心 `crud`。
- grpcapi 统一 v0.3.0→v0.4.0(修版本分叉);goosekit 以「派生表名安全
  标识符」就近防御替代对根契约的依赖(保持严格中立电池)。
- 新增 `layers_guard_test`:电池分层机器固化(严格中立集零
  lynx/grpcapi/uptrace;根 go.mod 永不出现 ORM)。
- 文档:ADR-0009(全决策账本 D1-D15)、battery-roadmap(三桶纪律)、
  extending(接缝清单)、api-shapes(D14 骨架)、UPGRADING/SECURITY/README。

## v0.4.0 (2026-10-25)

- **testutil 共享 fake(ADR-0005 路线 A)**:`CapturePublisher`(与
  BusPublisher 同语义——信封不合法即拒绝;Fail 标志测错误路径)与
  `InlineRunner`(uow.Runner 直通)。模块测试不再各自手抄(fake 漏
  Validate 曾让非法事件名在测试期漏网)。
- **testutil.AssertDepsAtAPILayer 依赖护栏**:断言 kernel 根包
  (Deps/Assembly 所在)未被指定目录 import——Deps 只允许 api 装配层,
  domain/app/infra 一律走子包端口;「Deps 子集化」用静态护栏替代 DI 框架。

## v0.3.0 (2026-10-25)

- **`DefaultChain(logger, set, agg, authnOpts)`(additive)**:把「模块贡献
  Stage 时必须复刻 server 默认链」的知识收进 kernel——recovery/requestid/
  clientinfo/authn/authz/validate 基础段 + 模块 Stage 按 Order 稳定排序;
  无 Stage 时返回 (nil, nil) 沿用 server.New 自建链。修复可达性:该逻辑
  此前住在 scaffold(生成后归用户的静态文件),漏一段即认证静默丢失且
  无法远程修复——按 ADR-0003「出错要能远程修」的界归 kernel。

## v0.2.0 (2026-10-25)

- **事件目录契约(additive)**:`Module` 新增 `PublishedEvents` / `ConsumedEvents`;
  `ValidateModules` 启动期交叉校验——发布名合法且首段归属本模块、无重复发布、
  消费侧每个事件都有已装发布者(typo / 上游改名 / 漏装发布方 fail-closed)。
- **`password.DummyHash()`**:惰性生成与当前参数同参的 Argon2id 哑靶,供登录
  时序对齐使用——kernel 升参数后哑靶恒同参,修复模块内写死常量导致的
  时序对齐静默失效。
- demo/identity 模块随本版改用事件常量与 DummyHash(identity v0.1.1 起要求
  `kernel >= v0.2.0`)。

## v0.1.0 (2026-10-25)

初始发布——v0.1.0 候选包全量:

- `module` / 根包:装配协议(`Module` 静态描述 + `Assemble(Deps) (Assembly, error)`
  动态装配 + `AssembleAll` 聚合)、依赖 DAG(Kahn)与事件目录启动期校验、
  `ValidateName`;
- `errcode`:按模块登记的错误码区段与 ErrorInfo 形态;
- `contexts`:Principal 语境;
- `events`:事件信封(Envelope)、发布端口(Publisher)、名字约束(中划线段式);
- `uow` + `uow/buntx`:事务传播端口与 bun 适配(ctx 携带事务连接);
- `crud`:分页方言(HMAC token + offset/时间游标)、AIP-132 排序白名单;
- `goosekit`:per-module 迁移隔离(独立 goose 版本表 `goose_<module>_db`);
- `secretkey`:主密钥 purpose 派生;
- `clock`:时间源(Fake 注入);
- `idgen`:随机 ID / 前缀 ID;
- `password`:Argon2id(t=3 / m=64MiB / p=4);
- `jwt`:HS256 签发/验证(purpose 隔离);
- `bunx`:连接引导 + 泛型列表执行;
- `configkit`:配置段加载(`LoadSection`);
- `testutil`:隔离测试库基座(建库 → 迁移链 → FORCE DROP,DB 门控跳过)。
