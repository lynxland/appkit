# API 形态规范（Wire Shapes）——全文（D14 定稿，随 errcode v2/errstatus 循环落地）

> 机器校验两级已就绪：句法级经 buf lint/protovalidate 配置（模块 buf.yaml）；
> 语义级 preset 由各 proto 的 lint 断言承载（本仓形态约束以文档+评审执行，
> grpcapi guard preset 待其 v0.5 冻结面后接入）。

本规范约束 proto-first API 的 **wire 形态**（字段与消息的表达约定），适用
于 lynx 生态全部模块与应用。传输层协议（policy/网关/拦截）归 grpcapi。

## 定典

### 标识与时间
- **ID**：wire 上一律 `string`、opaque——禁裸 int64 上 wire（JS 精度坑）；
  可读性（如 `web-42`）是服务端选择，客户端禁解析其语义。
- **时间**：`google.protobuf.Timestamp`（UTC）；存储形态属行规约
  （row 包），不属 wire 形态。

### 枚举与零值
- 枚举必含 `*_UNSPECIFIED = 0`。
- 「显式 false/零值」语义用 `optional`。
- protojson 零值省略是特性不是 bug（消费端按缺省处理）——依赖显式
  零值表达的场景一律 optional。

### 错误体（errstatus 的权威投影）
- **gRPC ErrorInfo detail 是权威投影**：Domain=模块区段、Reason=
  UPPER_SNAKE、Metadata{message[locale 回退链], suggestion, docs_url}。
- 字段违规：BadRequest FieldViolation 逐条（proto envelope 形态仅
  SDK/CLI 便利投影）。
- 哨兵语义码：storeerr.ErrAlreadyExists→AlreadyExists、ErrNotFound→
  NotFound、ErrConflict/ErrForeignKey→FailedPrecondition、ErrBusy→
  Unavailable+RetryInfo。
- 错误码注册表：errcode.Registry v2——登记期 fail-fast（空 reason/
  重复 canonical/缺消息）、locale 目录四级回退（locale→en→Message→
  canonical）、golden 快照漂移即审。

### list 响应
- 统一 meta：`has_more` + after 游标回显；`total_size` 可选、**禁默认
  count**（代价警示）。
- 形态与 crud v2 三模式联动（bounded/offset/keyset），分页字段即
  RawParams 映射。

### 动作响应与更新面
- 幂等动作用带计数/状态的结构优于 Empty（MarkAllRead 返回条数）。
- gateway 映射成文：DELETE+Empty → HTTP 200（非 204）。
- 更新面显式动词优先（ChangePassword/SetXxx）；FieldMask 记 revisit。

### 资源引用与命名
- 路由用资源名（`/v1/organizations/{id}/tasks`，ADR-0008）；字段用
  裸 ID（`organization_id`）。分工已定，勿混用。
- 消息命名正向化：资源直出（`Organization`）用于单资源响应；
  `XxxRequest/XxxResponse` 用于动作面；buf lint 的 except 已在模块
  buf.yaml 固化为正向规则。

## 机器校验

1. **句法级**：buf lint STANDARD + protovalidate（模块 buf.yaml 已配置）。
2. **语义级**：本规范条目在模块 policy_test 与 e2e 中断言（如 ID 必须
   string 的负路径、枚举 UNSPECIFIED 的存在性）；grpcapi guard preset
   在其冻结面开放后接入（v0.5 后续）。
