// Package appkit 是 appkit 的契约根包：只定义类型与规则函数，不带
// 装配实现（ADR-0009 D6）——校验与聚合见 compose 包，grpcapi 标准拦截链
// 组装见 chain 包，bun 适配见 contrib/bunx。
//
// appkit 是 lynx 框架的 web 开发基础库：以本包的装配契约
// （Module/Assembly/Deps）+ 电池子包（uow 事务、crud 分页、errcode 错误
// 区段、events 事件信封、goosekit 迁移执行器…）规范基于 lynx 的后端
// 应用的统一开发范式。契约要点见 docs/adr/0004-module-model-contracts.md：
//   - 模块间仅 domain 级 DAG 依赖；
//   - 配置自治（UnmarshalKey 自有段）；
//   - 迁移链隔离（独立 goose 版本表）；
//   - 错误码区段制；
//   - 根包无租户预设。
//
// 本库对模块做 semver 承诺；模块源码编译期依赖本库，但本库永不 import
// 任何模块代码（import guard 由应用侧护栏测试强制）。
//
// # 快速上手
//
// 应用侧 internal/modules/modules.go 的 Installed 列表登记模块的静态描述
// 与动态装配入口；main.go 经 compose.AssembleAll 聚合：
//
//	agg, err := compose.AssembleAll(deps, modules.Installed)
//	if err != nil {
//		return err
//	}
//
// 模块源码暴露两样东西：Module()（静态描述）与
// Assemble(appkit.Deps) (appkit.Assembly, error)（动态装配）。
// 分发安装（把模块源码装进你的应用）见姊妹项目 lynx-mods。
package appkit
