package errcode

import (
	"strings"
	"testing"
)

func newTestRegistry() *Registry {
	r := NewRegistry()
	r.MustRegister(Detail{
		Info:       Info{Domain: "identity", Reason: "INVALID_CREDENTIALS", Message: "invalid email or password"},
		Suggestion: "Check the email and password, or reset the password.",
		DocsURL:    "https://docs.example.dev/errors/identity/INVALID_CREDENTIALS",
	}, map[string]string{
		"zh-CN": "邮箱或密码不正确",
	})
	r.MustRegister(Detail{
		Info: Info{Domain: "organization", Reason: "SLUG_TAKEN", Message: "slug is already taken"},
	}, nil)
	return r
}

func TestRegistryFailFast(t *testing.T) {
	r := NewRegistry()
	mustPanic(t, "empty reason", func() {
		r.MustRegister(Detail{Info: Info{Domain: "d", Reason: "", Message: "m"}}, nil)
	})
	mustPanic(t, "non UPPER_SNAKE", func() {
		r.MustRegister(Detail{Info: Info{Domain: "d", Reason: "not_upper", Message: "m"}}, nil)
	})
	mustPanic(t, "no message at all", func() {
		r.MustRegister(Detail{Info: Info{Domain: "d", Reason: "X"}}, nil)
	})
	// 同 canonical 重复 → panic（登记第二条）。
	r.MustRegister(Detail{Info: Info{Domain: "d", Reason: "DUP", Message: "m"}}, nil)
	mustPanic(t, "duplicate canonical", func() {
		r.MustRegister(Detail{Info: Info{Domain: "d", Reason: "DUP", Message: "m2"}}, nil)
	})
}

func mustPanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s: want panic", name)
		}
	}()
	fn()
}

func TestRegistryMessageFallbackChain(t *testing.T) {
	r := newTestRegistry()
	cases := []struct {
		locale, canonical, want string
	}{
		{"zh-CN", "identity/INVALID_CREDENTIALS", "邮箱或密码不正确"},
		{"en", "identity/INVALID_CREDENTIALS", "invalid email or password"}, // default locale 消息
		{"fr", "identity/INVALID_CREDENTIALS", "invalid email or password"}, // 无 fr → default
		{"zh-CN", "organization/SLUG_TAKEN", "slug is already taken"},       // 无目录 → Message
		{"zh-CN", "ghost/UNKNOWN", "ghost/UNKNOWN"},                         // 未登记 → canonical
	}
	for _, tc := range cases {
		if got := r.Message(tc.canonical, tc.locale); got != tc.want {
			t.Errorf("Message(%s, %s) = %q, want %q", tc.canonical, tc.locale, got, tc.want)
		}
	}
}

func TestRegistryGoldenDeterministic(t *testing.T) {
	r := newTestRegistry()
	g1, g2 := r.Golden(), r.Golden()
	if g1 != g2 {
		t.Fatal("golden not deterministic")
	}
	lines := strings.Split(strings.TrimRight(g1, "\n"), "\n")
	if len(lines) != 3 { // identity 一行 + zh-CN 一行 + organization 一行
		t.Fatalf("golden lines = %d:\n%s", len(lines), g1)
	}
	if !strings.Contains(lines[0], "identity/INVALID_CREDENTIALS\tinvalid email or password") {
		t.Errorf("line0 = %q", lines[0])
	}
	if !strings.Contains(lines[0], "suggest: ") || !strings.Contains(lines[0], "docs: ") {
		t.Errorf("enriched fields missing: %q", lines[0])
	}
	if !strings.Contains(g1, "identity/INVALID_CREDENTIALS.zh-CN\t邮箱或密码不正确") {
		t.Errorf("locale line missing:\n%s", g1)
	}
	// 顺序确定性：identity < organization。
	if strings.Index(g1, "identity/") > strings.Index(g1, "organization/") {
		t.Error("golden not canonical-sorted")
	}
}

func TestRegistryLookupAndRegistered(t *testing.T) {
	r := newTestRegistry()
	if d, ok := r.Lookup("identity/INVALID_CREDENTIALS"); !ok || d.Suggestion == "" || d.DocsURL == "" {
		t.Fatalf("lookup: %+v ok=%v", d, ok)
	}
	if _, ok := r.Lookup("nope/X"); ok {
		t.Error("unknown canonical must not be found")
	}
	if got := r.Registered(); len(got) != 2 || got[0] != "identity/INVALID_CREDENTIALS" {
		t.Errorf("registered = %v", got)
	}
	if r.Count() != 2 {
		t.Errorf("count = %d", r.Count())
	}
}
