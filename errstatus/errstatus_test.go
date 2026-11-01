package errstatus

import (
	"errors"
	"testing"

	"github.com/lynx-go/lynx-start/errcode"
	"github.com/lynx-go/lynx-start/storeerr"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func testRegistry() *errcode.Registry {
	r := errcode.NewRegistry()
	r.MustRegister(errcode.Detail{
		Info:       errcode.Info{Domain: "identity", Reason: "INVALID_CREDENTIALS", Message: "invalid email or password"},
		Suggestion: "Check the email and password.",
		DocsURL:    "https://docs.example.dev/errors/identity/INVALID_CREDENTIALS",
	}, map[string]string{"zh-CN": "邮箱或密码不正确"})
	return r
}

func infoDetail(t *testing.T, err error) *errdetails.ErrorInfo {
	t.Helper()
	for _, d := range status.Convert(err).Details() {
		if ei, ok := d.(*errdetails.ErrorInfo); ok {
			return ei
		}
	}
	t.Fatalf("ErrorInfo missing in %v", err)
	return nil
}

func TestProjectErrcodeWithRegistry(t *testing.T) {
	r := testRegistry()
	info := errcode.Info{Domain: "identity", Reason: "INVALID_CREDENTIALS", Message: "invalid email or password"}

	// en：default locale + 元数据。
	err := Project(errors.Join(errors.New("outer"), info), Options{Registry: r})
	if code := status.Code(err); code != codes.InvalidArgument {
		t.Fatalf("code = %v", code)
	}
	ei := infoDetail(t, err)
	if ei.Domain != "identity" || ei.Reason != "INVALID_CREDENTIALS" {
		t.Fatalf("domain/reason = %s/%s", ei.Domain, ei.Reason)
	}
	if ei.Metadata["suggestion"] == "" || ei.Metadata["docs_url"] == "" || ei.Metadata["message"] != "invalid email or password" {
		t.Errorf("metadata = %v", ei.Metadata)
	}

	// zh-CN：注册表消息回退链生效。
	err = Project(info, Options{Registry: r, Locale: "zh-CN"})
	ei = infoDetail(t, err)
	if ei.Metadata["message"] != "邮箱或密码不正确" {
		t.Errorf("zh-CN message = %q", ei.Metadata["message"])
	}
}

func TestProjectWithoutRegistry(t *testing.T) {
	info := errcode.Info{Domain: "organization", Reason: "SLUG_TAKEN", Message: "slug is already taken"}
	err := Project(info, Options{})
	ei := infoDetail(t, err)
	if ei.Domain != "organization" || ei.Reason != "SLUG_TAKEN" {
		t.Fatalf("%+v", ei)
	}
	if _, has := ei.Metadata["suggestion"]; has {
		t.Error("no registry → no suggestion metadata")
	}
}

func TestProjectSentinels(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"already-exists", storeerr.ErrAlreadyExists, codes.AlreadyExists},
		{"not-found", storeerr.ErrNotFound, codes.NotFound},
		{"conflict", storeerr.ErrConflict, codes.FailedPrecondition},
		{"foreign-key", storeerr.ErrForeignKey, codes.FailedPrecondition},
		{"busy", storeerr.ErrBusy, codes.Unavailable},
		{"wrapped-conflict", errors.Join(errors.New("x"), storeerr.Wrap(storeerr.ErrConflict, errors.New("40001"), "")), codes.FailedPrecondition},
	}
	for _, tc := range cases {
		if got := status.Code(Project(tc.err, Options{})); got != tc.want {
			t.Errorf("%s: code = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestProjectViolations(t *testing.T) {
	info := errcode.Info{Domain: "task", Reason: "INVALID_TITLE", Message: "title is invalid"}
	err := Project(info, Options{},
		Violation{Field: "title", Description: "must not be empty"},
		Violation{Field: "ops[3].expected_version", Description: "stale version"},
	)
	st := status.Convert(err)
	var br *errdetails.BadRequest
	for _, d := range st.Details() {
		if b, ok := d.(*errdetails.BadRequest); ok {
			br = b
		}
	}
	if br == nil || len(br.FieldViolations) != 2 {
		t.Fatalf("violations = %+v", br)
	}
	if br.FieldViolations[1].Field != "ops[3].expected_version" {
		t.Errorf("field = %q", br.FieldViolations[1].Field)
	}
}

func TestProjectUnknownAndNil(t *testing.T) {
	if Project(nil, Options{}) != nil {
		t.Fatal("nil must project to nil")
	}
	err := Project(errors.New("mystery"), Options{})
	if status.Code(err) != codes.Unknown || status.Convert(err).Message() != "mystery" {
		t.Fatalf("unknown: %v", err)
	}
}

func TestBusyCarriesRetryInfo(t *testing.T) {
	err := Project(storeerr.ErrBusy, Options{})
	found := false
	for _, d := range status.Convert(err).Details() {
		if _, ok := d.(*errdetails.RetryInfo); ok {
			found = true
		}
	}
	if !found {
		t.Error("busy must carry RetryInfo")
	}
}
