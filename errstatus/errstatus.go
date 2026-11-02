// Package errstatus 是错误传输投影（ADR-0009 D14 定典：gRPC ErrorInfo
// detail 是权威投影，envelope 仅 SDK/CLI 便利形态）。核心电池层位
// （grpcapi 词汇）：把 errcode.Registry 中的 Detail / storeerr 哨兵 /
// 字段违规投影为 gRPC status（ErrorInfo + BadRequest FieldViolation +
// RetryInfo），供 app 层传输边界统一调用——取代每个模块手抄
// status.New + WithDetails 的样板（torchwood docdb_errors 与 fleetly
// apperrstatus 的合流）。
package errstatus

import (
	"errors"
	"github.com/lynxland/appkit/errcode"
	"github.com/lynxland/appkit/storeerr"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

// Violation 是一条字段违规（google.rpc.BadRequest.FieldViolation 对应）。
type Violation struct {
	Field       string
	Description string
}

// Locale 是消息 locale 选择器（nil → DefaultLocale；从请求 Accept-Language
// 或客户端配置解析后传入）。
type Locale struct{ Tag string }

// Options 是投影选项。
type Options struct {
	// Registry 是错误码注册表（可空——未提供时 canonical 码仍投影
	// ErrorInfo reason，只是没有 Suggestion/DocsURL 元数据）。
	Registry *errcode.Registry
	// Locale 是消息 locale（空 → errcode.DefaultLocale）。
	Locale string
	// CodeMap 是区段码→gRPC code 的应用级覆盖表（reason→code）；
	// 未命中时区段码默认 InvalidArgument。模块的语义码表在此声明，
	// 投影底座统一负责 detail 挂载。
	CodeMap map[string]codes.Code
}

// Project 把领域错误投影为 gRPC status：
//   - errcode.Info（含 v1 区段哨兵）：canonical → ErrorInfo{domain, reason}
//   - Suggestion/DocsURL 元数据（注册表在场时）；
//   - storeerr/occ 哨兵：语义码映射（AlreadyExists→AlreadyExists、
//     NotFound→NotFound、Conflict/ErrForeignKey→FailedPrecondition）；
//   - Violations：BadRequest detail 逐条附带；
//   - 其他错误：Unknown + 原样 message（Internal 语义由调用方提升——
//     框架不吞栈，透传字符串）。
//
// gateway 侧无需额外接线：grpcapi DefaultErrorEncoder 已把 ErrorInfo/
// BadRequest/RetryInfo detail 转 JSON 信封与 HTTP 头。
func Project(err error, opts Options, violations ...Violation) error {
	if err == nil {
		return nil
	}
	st := project(err, opts)
	if len(violations) > 0 {
		br := &errdetails.BadRequest{}
		for _, v := range violations {
			br.FieldViolations = append(br.FieldViolations, &errdetails.BadRequest_FieldViolation{
				Field:       v.Field,
				Description: v.Description,
			})
		}
		if withDetails, derr := st.WithDetails(br); derr == nil {
			st = withDetails
		}
	}
	return st.Err()
}

func project(err error, opts Options) *status.Status {
	// ① errcode 区段哨兵（v1 Info / v2 Detail 同走 ErrorInfo）。
	if info, ok := errAsErrcode(err); ok {
		locale := opts.Locale
		if locale == "" {
			locale = errcode.DefaultLocale
		}
		metadata := map[string]string{}
		if opts.Registry != nil {
			if d, found := opts.Registry.Lookup(info.Canonical()); found {
				if d.Suggestion != "" {
					metadata["suggestion"] = d.Suggestion
				}
				if d.DocsURL != "" {
					metadata["docs_url"] = d.DocsURL
				}
				metadata["message"] = opts.Registry.Message(info.Canonical(), locale)
			}
		}
		code := codes.InvalidArgument
		if opts.CodeMap != nil {
			if c, ok := opts.CodeMap[info.Reason]; ok {
				code = c
			}
		}
		st := status.New(code, messageOr(err, info))
		if withDetails, derr := st.WithDetails(&errdetails.ErrorInfo{
			Reason:   info.Reason,
			Domain:   info.Domain,
			Metadata: metadata,
		}); derr == nil {
			st = withDetails
		}
		return st
	}

	// ② storeerr/occ 哨兵 → 语义码。
	switch {
	case storeerr.IsAlreadyExists(err):
		return status.New(codes.AlreadyExists, err.Error())
	case storeerr.IsNotFound(err):
		return status.New(codes.NotFound, err.Error())
	case storeerr.IsConflict(err):
		return status.New(codes.FailedPrecondition, err.Error())
	case storeerr.IsForeignKey(err):
		return status.New(codes.FailedPrecondition, err.Error())
	case storeerr.IsBusy(err):
		st := status.New(codes.Unavailable, err.Error())
		if withDetails, derr := st.WithDetails(&errdetails.RetryInfo{
			RetryDelay: durationpb.New(0),
		}); derr == nil {
			st = withDetails
		}
		return st
	}

	// ③ 未识别：透传字符串（code=Unknown；调用方可选择提升 Internal）。
	return status.New(codes.Unknown, err.Error())
}

// errAsErrcode 从错误链中提取 errcode.Info（哨兵或包裹；errors.As 原生
// 穿越 errors.Join 的多路 Unwrap 树）。
func errAsErrcode(err error) (errcode.Info, bool) {
	var info errcode.Info
	if errors.As(err, &info) {
		return info, true
	}
	return errcode.Info{}, false
}

func messageOr(err error, info errcode.Info) string {
	if info.Message != "" {
		return info.Message
	}
	return err.Error()
}
