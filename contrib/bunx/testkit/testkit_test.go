package testkit_test

import (
	"testing"
	"testing/fstest"

	"github.com/lynxland/appkit"
	"github.com/lynxland/appkit/compose"
	"github.com/lynxland/appkit/contrib/bunx/testkit"
	"github.com/lynxland/appkit/events"
)

// demoModule 是最小契约模块：零迁移、零策略文件（无 proto）——验证
// Harness 的自举面（装配/DB/事件捕获/拨号），业务级全链见 demo 仓的
// 采纳测试。
func demoModule() appkit.InstalledModule {
	m := appkit.Module{Name: "demo", Description: "testkit self-test"}
	return appkit.InstalledModule{
		Module:   m,
		Assemble: func(appkit.Deps) (appkit.Assembly, error) { return appkit.Assembly{}, nil },
	}
}

// migrationModule 携带一条迁移链，验证 Chain 派生与 goosekit 就绪。
func migrationModule() appkit.InstalledModule {
	migrations := fstest.MapFS{
		"00001_init.sql": &fstest.MapFile{Data: []byte("-- +goose Up\nCREATE TABLE IF NOT EXISTS testkit_things (id text PRIMARY KEY);")},
	}
	m := appkit.Module{Name: "mig", Migrations: migrations}
	return appkit.InstalledModule{
		Module:   m,
		Assemble: func(appkit.Deps) (appkit.Assembly, error) { return appkit.Assembly{}, nil },
	}
}

func TestHarnessBoots(t *testing.T) {
	h := testkit.New(t, testkit.Options{
		Installed: []appkit.InstalledModule{demoModule(), migrationModule()},
		Config:    map[string]any{"demo.greeting": "hi"},
		Gateway:   true,
	})
	if h.Conn == nil || h.HTTPURL == "" || h.DB == nil {
		t.Fatalf("harness fields incomplete: %+v", h)
	}
	// 迁移链已就绪：表可 DDL 探测（空表 SELECT no-rows 与 relation 缺失
	// 需区分——用 Exec 幂等探测）。
	if _, err := h.DB.NewRaw("SELECT 1 FROM testkit_things LIMIT 1").Exec(t.Context()); err != nil {
		t.Fatalf("migration chain not applied: %v", err)
	}
	// 事件捕获器可发布可断言（信封字段形态同模块真实用法）。
	if err := h.Deps.Events.Publish(t.Context(), events.Envelope{
		ID: "evt-1", Name: "demo.event.happened", Module: "demo", Payload: []byte(`{}`),
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if got := h.Events.Names(); len(got) != 1 || got[0] != "demo.event.happened" {
		t.Fatalf("captured = %v", got)
	}
	// compose 校验面在 Harness 内已被调用（重复调用保持幂等语义）。
	if err := compose.ValidateModules([]appkit.Module{demoModule().Module}); err != nil {
		t.Fatal(err)
	}
}
