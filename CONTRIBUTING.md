# 贡献指南 / Contributing

本仓是 **lynx 框架的 web 开发基础库**（契约 + 电池 + contrib 适配）。
模块与 registry 的贡献指南已随分发迁移至
[lynxland/lynx-mods](https://github.com/lynxland/lynx-mods) 的 CONTRIBUTING。

## 电池准入三件套（ADR-0009 D13）

一个机制从 roadmap 进入核心，需要齐三件：

1. **测试**：含错误注入/负路径；DB 相关须有真库门控测试
   （`APPKIT_TEST_PG_DSN`，无 DSN 静默 skip 是已知陷阱——本地复核必须设 DSN 实跑）。
2. **dogfood 采纳**：至少一个 dogfood 项目（demo/torchwood/fleetly）换 import 实装。
3. **文档**：godoc 包注释 + 必要的 Example 基线；范式变更走 ADR。

弃位流程：先 `// Deprecated:`（附替代）保留一个 minor 周期再移除。

## 分层纪律（改代码前必读）

- 根包 = 纯契约（类型 + ValidateName 级规则函数）；实现进 `compose/`/`chain/`。
- 严格中立电池（uow/crud/clock/errcode/idgen/jwt/password/secretkey/goosekit）
  禁 import lynx/grpcapi/uptrace/根契约；宿主电池（configkit/events）禁
  grpcapi/uptrace——`layers_guard_test.go` 机器固化，新增包时同步登记。
- 根 go.mod 永不出现 ORM（bun 只在 contrib/bunx，钉 v1.3.0）。

## 本地验证（与 CI 同构）

```bash
mise install
mise run lint        # golangci-lint + gofmt 漂移（大改动后建议 --new=false 清缓存复核）
mise run test        # 需设 APPKIT_TEST_PG_DSN 才会真跑 PG 门控测试
```

## 发布 / Releasing

- 根 module：破坏性变更（0.x 阶段）升 minor，`CHANGELOG.md` 加
  `## vX.Y.Z` 小节，`git tag vX.Y.Z`；向后兼容追加升 patch。
- contrib 子模块（如 contrib/bunx）：独立 tag `contrib/bunx/vX.Y.Z`，
  CHANGELOG 加 `## contrib/bunx vX.Y.Z` 小节；require 的根版本 tag 必须
  已推送（发版时序：根 tag → contrib tidy → contrib tag）。
- 升级指南：破坏性变更同步 `UPGRADING.md`（金丝雀 demo 迁移实证后回填）。
- 文档双语并行：中文为源，英文镜像；两版分歧时以中文为准并尽快同步。

## 安全 / Security

见 [SECURITY.md](SECURITY.md)：私密漏洞报告，勿开公开 issue。安全敏感
实现优先下沉核心（semver 推修复）；模块侧残余由 lynx-mods 的 advisories
通道兜底。
