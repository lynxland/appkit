package configkit

import (
	"testing"

	"github.com/lynxland/lynx/lynxtest"
)

type demoCfg struct {
	Limit int    `mapstructure:"limit"`
	Name  string `mapstructure:"name"`
}

func TestLoadSectionDefaultsAndOverride(t *testing.T) {
	ctx := lynxtest.NewContext(t, lynxtest.ContextWithConfigMap(map[string]any{
		"demo": map[string]any{"limit": 7},
	}))
	var cfg demoCfg
	err := LoadSection(ctx.Config(), "demo", func() {
		cfg.Limit = 5
		cfg.Name = "default-name"
	}, &cfg)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Limit != 7 {
		t.Errorf("override limit = %d, want 7", cfg.Limit)
	}
	if cfg.Name != "default-name" {
		t.Errorf("default name = %q, want default-name", cfg.Name)
	}
}

func TestLoadSectionUnknownKeyFails(t *testing.T) {
	ctx := lynxtest.NewContext(t, lynxtest.ContextWithConfigMap(map[string]any{
		"demo": map[string]any{"limitt": 7}, // 拼写错误
	}))
	var cfg demoCfg
	if err := LoadSection(ctx.Config(), "demo", nil, &cfg); err == nil {
		t.Fatal("unknown key: want error (WithErrorUnused)")
	}
}

func TestLoadSectionMissingSectionKeepsDefaults(t *testing.T) {
	ctx := lynxtest.NewContext(t, lynxtest.ContextWithConfigMap(map[string]any{}))
	var cfg demoCfg
	if err := LoadSection(ctx.Config(), "demo", func() { cfg.Limit = 3 }, &cfg); err != nil {
		t.Fatalf("missing section: %v", err)
	}
	if cfg.Limit != 3 {
		t.Fatalf("defaults lost: %+v", cfg)
	}
}

func TestRequireHelpers(t *testing.T) {
	if err := RequireString("x.y", ""); err == nil {
		t.Error("empty: want error")
	}
	if err := RequireString("x.y", "v"); err != nil {
		t.Errorf("non-empty: %v", err)
	}
	if err := RequirePositive("x.n", 0); err == nil {
		t.Error("zero: want error")
	}
}
