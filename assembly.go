package appkit

import (
	"github.com/lynx-go/grpcapi/authn"
	"github.com/lynx-go/grpcapi/interceptor"
	"github.com/lynx-go/grpcapi/server"
	"github.com/lynx-go/lynx"
	"github.com/lynx-go/lynx/contrib/schedule"
)

// Assembly 是模块的动态装配产物：需要构造依赖图才能产生的贡献面。
// 静态贡献（策略描述符、迁移、依赖声明）在 Module 上。
type Assembly struct {
	// Registrations 是模块的 gRPC/HTTP 服务注册（grpcapi）。
	Registrations []server.Registration
	// Services 是模块贡献的 lynx.Service（事件订阅、后台作业等）。
	Services []lynx.Service
	// Tasks 是模块贡献的定时任务，应用聚合进唯一 Scheduler。
	Tasks []schedule.Task
	// Authenticators 是模块贡献的认证器（如 session/apikey 模块）。
	Authenticators []authn.Authenticator
	// Enrichers 是模块贡献的认证后增强器（如 access 模块往 Principal
	// 注入 roles/permissions claims——proto 策略的 permissions/roles
	// 声明由此生效）。
	Enrichers []authn.Enricher
	// Stages 是模块贡献的拦截器槽位（audit=50/ratelimit=40/usage=60）。
	Stages []interceptor.Stage
	// ExtraRoutes 是模块的手写 HTTP 路由。
	ExtraRoutes []server.ExtraRoute
}

// InstalledModule 是应用已装模块的完整形态：静态描述 + 动态装配入口。
// 应用侧 internal/modules/modules.go 的 Installed 列表登记全部已装模块，
// lynx-mods add 负责追加登记。
type InstalledModule struct {
	Module Module
	// Assemble 构造模块的动态装配产物；每个应用进程调用一次。
	Assemble func(Deps) (Assembly, error)
}

// Aggregated 是全部已装模块装配产物的聚合，main.go 直接消费
// （聚合过程见 compose.AssembleAll）。
type Aggregated struct {
	Registrations  []server.Registration
	Services       []lynx.Service
	Tasks          []schedule.Task
	Authenticators []authn.Authenticator
	Enrichers      []authn.Enricher
	Stages         []interceptor.Stage
	ExtraRoutes    []server.ExtraRoute
}
