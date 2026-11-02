// Package contexts 提供模块中立的请求语境助手（契约⑥：无租户预设）。
//
// Principal 的权威来源是 grpcapi 的 principal.With/From（authn 拦截器注入）；
// 本包只做模块侧的顺手封装：取主体、取主体 ID、以及无主体时的哨兵语义。
// 租户/组织等业务语境将来由多租户模块经 claim + 拦截器扩展，不进 Kernel。
package contexts

import (
	"context"

	"github.com/lynxland/grpcapi/principal"
)

// FromPrincipal 返回 ctx 中的 Principal；未认证请求 ok=false（匿名面）。
func FromPrincipal(ctx context.Context) (*principal.Principal, bool) {
	return principal.From(ctx)
}

// SubjectID 返回主体的稳定 ID；无主体时返回空串（调用方按匿名语义处理）。
func SubjectID(ctx context.Context) string {
	p, ok := principal.From(ctx)
	if !ok || p == nil {
		return ""
	}
	return p.ID
}

// IsAnonymous 报告请求是否无认证主体。
func IsAnonymous(ctx context.Context) bool {
	_, ok := principal.From(ctx)
	return !ok
}
