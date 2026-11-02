// Package compose 承载模块装配协议的实现（ADR-0009 D6：根包只留契约）：
// 启动前校验（名字唯一、依赖 DAG、事件目录交叉校验）与已装模块的逐个
// 装配聚合。契约类型（Module/Deps/Assembly/InstalledModule/Aggregated）
// 定义在根包 appkit；本包是纯算法实现——不 import 任何具体技术栈适配
// （grpcapi 拦截链组装见 chain 包）。
package compose

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lynxland/appkit"
	"github.com/lynxland/appkit/events"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// ValidateModules 对已装模块集合做启动前校验：名字合法且唯一、DependsOn
// 指向已装模块、依赖图无环（domain 级 DAG 契约）。校验失败应在组合根即告警，
// doctor 命令复用同一入口。
func ValidateModules(mods []appkit.Module) error {
	seen := make(map[string]bool, len(mods))
	for _, m := range mods {
		if err := appkit.ValidateName(m.Name); err != nil {
			return err
		}
		if seen[m.Name] {
			return fmt.Errorf("duplicate module name %q", m.Name)
		}
		seen[m.Name] = true
	}
	// Kahn 拓扑校验：所有声明的依赖必须已装，且不存在环。
	deps := make(map[string][]string, len(mods))
	indeg := make(map[string]int, len(mods))
	for _, m := range mods {
		for _, d := range m.DependsOn {
			if !seen[d] {
				return fmt.Errorf("module %q depends on %q which is not installed", m.Name, d)
			}
			deps[d] = append(deps[d], m.Name)
			indeg[m.Name]++
		}
	}
	var queue []string
	for _, m := range mods {
		if indeg[m.Name] == 0 {
			queue = append(queue, m.Name)
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, next := range deps[cur] {
			indeg[next]--
			if indeg[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	for name, d := range indeg {
		if d > 0 {
			return fmt.Errorf("module dependency cycle involving %q", name)
		}
	}

	// 事件目录交叉校验（契约②安全网）：发布侧名字合法且归属本模块、
	// 无重复发布；消费侧每个事件都有已装发布者。字符串目录的 typo 与
	// 跨版本改名在此暴露（启动期 fail-closed），代价零——未声明目录的
	// 模块不受影响。
	published := make(map[string]string, len(mods))
	for _, m := range mods {
		for _, name := range m.PublishedEvents {
			if !events.NameRe.MatchString(name) {
				return fmt.Errorf("module %q publishes invalid event name %q", m.Name, name)
			}
			if owner := strings.SplitN(name, ".", 2)[0]; owner != m.Name {
				return fmt.Errorf("module %q publishes %q: name must be prefixed by module name", m.Name, name)
			}
			if prev, dup := published[name]; dup {
				return fmt.Errorf("event %q published by both %q and %q", name, prev, m.Name)
			}
			published[name] = m.Name
		}
	}
	for _, m := range mods {
		for _, name := range m.ConsumedEvents {
			if !events.NameRe.MatchString(name) {
				return fmt.Errorf("module %q consumes invalid event name %q", m.Name, name)
			}
			if _, ok := published[name]; !ok {
				return fmt.Errorf("module %q consumes %q which no installed module publishes", m.Name, name)
			}
		}
	}
	return nil
}

// PolicyFiles 汇总全部模块的策略描述符（policy.FromFiles 的输入）。
func PolicyFiles(mods []appkit.Module) []protoreflect.FileDescriptor {
	var out []protoreflect.FileDescriptor
	for _, m := range mods {
		out = append(out, m.Policies...)
	}
	return out
}

// SortModules 按名字稳定排序（装配与展示的一致性）。
func SortModules(mods []appkit.Module) {
	sort.Slice(mods, func(i, j int) bool { return mods[i].Name < mods[j].Name })
}

// Descriptors 抽取已装模块的静态描述（PolicyFiles 等静态聚合的输入）。
func Descriptors(mods []appkit.InstalledModule) []appkit.Module {
	out := make([]appkit.Module, 0, len(mods))
	for _, m := range mods {
		out = append(out, m.Module)
	}
	return out
}

// AssembleAll 校验已装模块集合（唯一性、依赖 DAG）并逐个装配、聚合。
// 任何模块装配失败即整体失败（fail-fast：模块是应用的一部分，不允许
// 半可用状态）。
func AssembleAll(deps appkit.Deps, mods []appkit.InstalledModule) (*appkit.Aggregated, error) {
	descriptors := make([]appkit.Module, 0, len(mods))
	for _, m := range mods {
		if m.Assemble == nil {
			return nil, fmt.Errorf("module %q: nil Assemble", m.Module.Name)
		}
		descriptors = append(descriptors, m.Module)
	}
	if err := ValidateModules(descriptors); err != nil {
		return nil, err
	}
	agg := &appkit.Aggregated{}
	for _, m := range mods {
		a, err := m.Assemble(deps)
		if err != nil {
			return nil, fmt.Errorf("assemble module %q: %w", m.Module.Name, err)
		}
		agg.Registrations = append(agg.Registrations, a.Registrations...)
		agg.Services = append(agg.Services, a.Services...)
		agg.Tasks = append(agg.Tasks, a.Tasks...)
		agg.Authenticators = append(agg.Authenticators, a.Authenticators...)
		agg.Enrichers = append(agg.Enrichers, a.Enrichers...)
		agg.Stages = append(agg.Stages, a.Stages...)
		agg.ExtraRoutes = append(agg.ExtraRoutes, a.ExtraRoutes...)
	}
	return agg, nil
}
