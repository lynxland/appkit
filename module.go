package appkit

import (
	"fmt"
	"io/fs"

	"github.com/lynxland/grpcapi/server"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Module 是模块的静态装配贡献面（装配协议的静态半边；动态半边见
// InstalledModule/Assembly）。
//
// 模块源码暴露两样东西：`Module()`（本结构：静态描述）与
// `Assemble(appkit.Deps) (appkit.Assembly, error)`（动态装配）。应用侧
// `internal/modules/modules.go` 的 Installed 列表登记两者，main.go 经
// compose.AssembleAll 聚合。
type Module struct {
	// Name 是模块唯一名：小写字母/数字/中划线，同时用作配置段名、迁移目录名
	// 与错误码区段名。
	Name string

	// Description 一句话用途说明（registry 展示用）。
	Description string

	// DependsOn 声明依赖的其他模块名（domain 级 DAG）。安装器与 doctor 校验
	// 无环；被依赖模块的 domain 包是唯一允许 import 的跨模块包。
	DependsOn []string

	// Policies 是模块 proto 的 FileDescriptor（生成物自带），grpcapi
	// policy.Build(policy.FromFiles(...)) 的输入。
	Policies []protoreflect.FileDescriptor

	// Migrations 是模块迁移目录（embed.FS，root 指向该模块的迁移文件），
	// goosekit 以独立版本表执行（契约④：迁移链隔离）。
	Migrations fs.FS

	// ExtraRoutes 是模块的静态手写 HTTP 路由贡献（依赖注入构造的经 Assembly）。
	ExtraRoutes []server.ExtraRoute

	// Config 声明模块的配置段。
	Config ConfigSpec

	// PublishedEvents 是本模块发布的事件目录（契约②：字符串目录
	// "<module>.<noun>.<verb>"，与 item.json 的 publishedEvents 一致）。
	// 声明后由 compose.ValidateModules 交叉校验：名字合法、首段归属本模块、
	// 任何模块的 ConsumedEvents 都能在此找到发布者——typo/改名/漏装
	// 在启动期 fail-closed，而不是运行期静默不触发。
	PublishedEvents []string

	// ConsumedEvents 是本模块订阅的跨模块事件目录（消费方按契约②不
	// import 发布者代码，字符串在此登记以换取启动期校验）。
	ConsumedEvents []string
}

// ConfigSpec 声明模块配置段：模块在自己的 Init 里以
// `UnmarshalKey(spec.Key, &cfg, lynx.WithErrorUnused())` 解码自有 struct
// （ADR-0004 契约③：配置自治，无全局 config.proto）。
type ConfigSpec struct {
	// Key 是配置段名；空则默认为模块名。
	Key string
	// Defaults 是段内默认值（安装器写入 config.yaml.template 片段时参考）。
	Defaults map[string]any
}

// KeyOrName 返回生效的配置段名。
func (s ConfigSpec) KeyOrName(name string) string {
	if s.Key != "" {
		return s.Key
	}
	return name
}

// ValidateName 校验模块名合法性：非空、仅小写字母/数字/中划线。
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("module name is empty")
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return fmt.Errorf("module name %q: only lowercase letters, digits and '-' are allowed", name)
		}
	}
	return nil
}
