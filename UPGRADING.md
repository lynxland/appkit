# 升级指南（UPGRADING）

## lynx-start v0.6.x（github.com/lynx-go/lynx-start）→ appkit v0.7.0（github.com/lynxland/appkit）

v0.7.0 是改名迁移（ADR-0010）：module path、根包名、测试 env 三变更。

### 1. import 路径与限定符

| 旧（lynx-start v0.6.x） | 新（appkit v0.7.0） |
|---|---|
| `kernel "github.com/lynx-go/lynx-start"`（显式别名） | `"github.com/lynxland/appkit"`（免别名） |
| `github.com/lynx-go/lynx-start/<子包>` | `github.com/lynxland/appkit/<子包>`（同名平移） |
| `github.com/lynx-go/lynx-start/contrib/bunx` | `github.com/lynxland/appkit/contrib/bunx` |
| `kernel.Deps` / `kernel.Module` 等 `kernel.` 限定符 | `appkit.Deps` / `appkit.Module` 等 `appkit.`（词边界替换，不伤 goosekit/configkit 等标识符） |

### 2. go.mod

```sh
go mod edit -droprequire github.com/lynx-go/lynx-start \
            -droprequire github.com/lynx-go/lynx-start/contrib/bunx \
            -require github.com/lynxland/appkit@v0.7.0 \
            -require github.com/lynxland/appkit/contrib/bunx@v0.4.0
go mod tidy
```

（用 bun 适配的应用才需要第二条 require。）

### 3. 测试 env

`LYNXSTART_TEST_PG_DSN` → `APPKIT_TEST_PG_DSN`（无兼容期，ADR-0010 D4）
——本仓与 lynx-mods 的 CI 同窗切换。

### 4. 分发侧

`lynx-mods` CLI 的 init/add 自 appkit 地板 v0.7.0 起生成新路径；已装
应用 in-place 迁移＝本节 1/2 的替换 + tidy，`lynx-mods update/doctor`
不受影响（模块源码前缀 `lynx-mods/modules/` 与本次根 module 改名无关）。

旧 tag（`github.com/lynx-go/lynx-start@v0.6.2` 及以前）经 module proxy
缓存长期可得，不急的下游可缓迁。

---

## kernel v0.4.0（github.com/lynx-go/lynx-start/kernel）→ lynx-start v0.5.0（github.com/lynx-go/lynx-start）

v0.5.0 是一次破坏性重排（0.x 阶段 minor 破坏承诺内）：仓库转型为
web 开发基础库，kernel 升为仓库根 module（ADR-0009）。

### 1. import 路径

| 旧（kernel v0.4.0） | 新（v0.5.0） |
|---|---|
| `github.com/lynx-go/lynx-start/kernel`（根包） | `github.com/lynx-go/lynx-start` |
| `github.com/lynx-go/lynx-start/kernel/uow` | `github.com/lynx-go/lynx-start/uow` |
| `github.com/lynx-go/lynx-start/kernel/crud` | `github.com/lynx-go/lynx-start/crud`（新增 OrderClause/ParseOrderBy/SortedFields） |
| `github.com/lynx-go/lynx-start/kernel/bunx` | `github.com/lynx-go/lynx-start/contrib/bunx` |
| `github.com/lynx-go/lynx-start/kernel/uow/buntx` | `github.com/lynx-go/lynx-start/contrib/bunx/buntx` |
| `github.com/lynx-go/lynx-start/kernel/testutil` | `github.com/lynx-go/lynx-start/contrib/bunx/testutil` |
| 其余 `…/kernel/<子包>` | `…/lynx-start/<子包>`（同名平移） |

包名与限定符**零变化**（根包仍叫 `kernel`，`kernel.Deps`/`kernel.Module`
照写——goimports 可能猜错包名，显式别名 `kernel "github.com/lynx-go/lynx-start"` 最稳）。

### 2. 实现函数换包（根包纯契约化）

| 旧 | 新 |
|---|---|
| `kernel.ValidateModules` | `compose.ValidateModules` |
| `kernel.AssembleAll` | `compose.AssembleAll` |
| `kernel.PolicyFiles` / `kernel.SortModules` / `kernel.Descriptors` | `compose.` 同名 |
| `kernel.DefaultChain` | `chain.DefaultChain` |

### 3. Deps.DB 类型变化（ORM 无关化）

`kernel.Deps.DB` 由 `*bun.DB` 改为 `any`。bun 应用的装配处加一次类型化
出口；仓储构造调用点：

```go
// 旧
repo := infraidentity.NewRepository(deps.DB)
// 新
repo := infraidentity.NewRepository(bunx.MustDB(deps)) // import …/contrib/bunx
```

应用 bootstrap 构造 `Deps{DB: db}` 处不变（*bun.DB 装入 any 字段）。

### 4. go.mod

```sh
go mod edit -droprequire github.com/lynx-go/lynx-start/kernel \
            -require github.com/lynx-go/lynx-start@v0.5.0 \
            -require github.com/lynx-go/lynx-start/contrib/bunx@v0.1.0
go mod tidy
```

grpcapi 统一到 v0.4.0；bun 保持 v1.3.0。

### 5. 模块分发侧（实战版，金丝雀 lynx-start-demo 已走通）

安装 CLI 更名 **lynx-mods**（`go install github.com/lynx-go/lynx-mods/cli/cmd/lynx-mods@latest`），
registry 迁至 lynx-go/lynx-mods。已装模块的应用 in-place 迁移步骤：

```bash
# ① 应用侧锚点/文件换代（add 依赖这些锚点）
rm -rf .lynx-start lynx-start.json
sed -i 's/lynx-start:config-fragment/lynx-mods:config-fragment/' configs/config.yaml*
cp <lynx-mods 仓>/scaffold/modules/modules.go modules/modules.go   # 新模板（空登记表）
printf '{"schema":1,"name":"<app>","modulePath":"<module-path>","registry":"<lynx-mods>","modules":[]}\n' > lynx-mods.json

# ② 按依赖序全量重装（-o 重建基线/桥接包/登记）
for m in demo identity session access audit organization notification task; do
  lynx-mods add "$m" -o --registry <lynx-mods 仓>
done

# ③ 老一代 scaffold 生成的 cmd/server/main.go 需同步新版（chain.DefaultChain
#    接管默认链组装；旧形态 AssembleWith(DefaultChainOptions, agg.Stages...)
#    在装 Stage 模块时启动即失败）

# ④ generate → tidy → build → vet → test（PG 门控设 LYNXSTART_TEST_PG_DSN）
lynx-mods generate && go mod tidy && go build ./... && go vet ./... && go test ./...
lynx-mods doctor --registry <lynx-mods 仓>
```

注意：`cmd/migrate/main.go` 等换装后可能出现未使用的 `kernel` import（删之）；
.env/config.yaml 的 DSN 不受影响。
