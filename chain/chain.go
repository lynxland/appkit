// Package chain 组装 grpcapi 标准拦截链与模块贡献 Stage（ADR-0009 D6：
// 实现归子包，根包只留契约）。任何模块贡献拦截器 Stage（如 audit 的
// Order=50）时，server.New 的默认链会被完全接管——此处把必须复刻的基础段
// （recovery/requestid/clientinfo/authn/authz/validate）收编为一次调用，
// 应用侧一行装配。漏一段基础段的后果是认证与权限判定静默丢失，这类 bug
// 属于「用户不想改、出错要能远程修」，按 ADR-0003 的分界归本库。
package chain

import (
	"log/slog"
	"sort"

	"github.com/lynxland/appkit"
	grpcapiauthn "github.com/lynxland/grpcapi/authn"
	grpcapiauthz "github.com/lynxland/grpcapi/authz"
	grpcapiinterceptor "github.com/lynxland/grpcapi/interceptor"
	grpcapipolicy "github.com/lynxland/grpcapi/policy"
)

// DefaultChain 构建「标准链 + 模块贡献 Stage」。无 Stage 时返回
// (nil, nil)——沿用 server.New 自建默认链。
//
// authnOpts 传应用侧的认证选项（Enrichers 等）；IsAnonymous 由本函数按
// 策略集覆写。AssembleWith 要求 Order 单调不减（sort.SliceStable 保证；
// 模块 Stage 落在 clientinfo 与 authz/validate 之间，见 audit=50）。
func DefaultChain(logger *slog.Logger, set *grpcapipolicy.Set, agg *appkit.Aggregated, authnOpts grpcapiauthn.Options) (*grpcapiinterceptor.Chain, error) {
	if agg == nil || len(agg.Stages) == 0 {
		return nil, nil
	}
	opts := authnOpts
	opts.IsAnonymous = set.IsAnonymous
	authenticator := grpcapiauthn.New(grpcapiauthn.Chain(agg.Authenticators), opts)
	decider, err := grpcapiauthz.NewRequirementDecider(set)
	if err != nil {
		return nil, err
	}
	authorizer, err := grpcapiauthz.NewInterceptor(set, decider, grpcapiauthz.InterceptorOptions{})
	if err != nil {
		return nil, err
	}
	stages := append([]grpcapiinterceptor.Stage{
		grpcapiinterceptor.RecoveryStage(grpcapiinterceptor.RecoveryOptions{Logger: logger}),
		grpcapiinterceptor.RequestIDStage(grpcapiinterceptor.RequestIDOptions{Logger: logger}),
		grpcapiinterceptor.ClientInfoStage(grpcapiinterceptor.NewClientInfo(grpcapiinterceptor.ClientInfoConfig{})),
		grpcapiinterceptor.AuthnStage(authenticator.Unary(), authenticator.Stream()),
		grpcapiinterceptor.AuthzStage(authorizer.Unary(), authorizer.Stream()),
		grpcapiinterceptor.ValidateStage(grpcapiinterceptor.NewValidate()),
	}, agg.Stages...)
	sort.SliceStable(stages, func(i, j int) bool { return stages[i].Order < stages[j].Order })
	return grpcapiinterceptor.Assemble(stages...)
}
