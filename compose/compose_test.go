package compose_test

import (
	"io/fs"
	"testing"

	"github.com/lynxland/appkit"
	"github.com/lynxland/appkit/compose"
)

func mod(name string, deps ...string) appkit.Module {
	return appkit.Module{Name: name, DependsOn: deps}
}

func TestValidateModules(t *testing.T) {
	if err := compose.ValidateModules([]appkit.Module{mod("a"), mod("b", "a"), mod("c", "b")}); err != nil {
		t.Fatalf("valid DAG: %v", err)
	}
	if err := compose.ValidateModules([]appkit.Module{mod("a"), mod("a")}); err == nil {
		t.Error("duplicate name: want error")
	}
	if err := compose.ValidateModules([]appkit.Module{mod("a", "missing")}); err == nil {
		t.Error("missing dep: want error")
	}
	if err := compose.ValidateModules([]appkit.Module{mod("a", "b"), mod("b", "a")}); err == nil {
		t.Error("cycle: want error")
	}
	if err := compose.ValidateModules([]appkit.Module{mod("a"), mod("b", "a"), mod("c", "b"), mod("d", "a")}); err != nil {
		t.Errorf("complex DAG: %v", err)
	}
}

func TestValidateModulesEventCatalog(t *testing.T) {
	// 正路径：发布方与消费方目录对得上（跨模块字符串目录，契约②）。
	ok := []appkit.Module{
		{Name: "identity", PublishedEvents: []string{"identity.user.created", "identity.user.password-changed"}},
		{Name: "session", DependsOn: []string{"identity"},
			PublishedEvents: []string{"session.session.created"},
			ConsumedEvents:  []string{"identity.user.password-changed"}},
	}
	if err := compose.ValidateModules(ok); err != nil {
		t.Fatalf("valid catalog: %v", err)
	}

	// 消费的事件没有任何已装模块发布（typo/漏装/上游改名）。
	orphan := []appkit.Module{
		{Name: "identity", PublishedEvents: []string{"identity.user.created"}},
		{Name: "session", DependsOn: []string{"identity"},
			ConsumedEvents: []string{"identity.user.password-changed"}},
	}
	if err := compose.ValidateModules(orphan); err == nil {
		t.Error("consumed without publisher: want error")
	}

	// 发布名首段不属于本模块（目录所有权）。
	owner := []appkit.Module{{Name: "demo", PublishedEvents: []string{"identity.user.created"}}}
	if err := compose.ValidateModules(owner); err == nil {
		t.Error("wrong owner prefix: want error")
	}

	// 名字不符合 NameRe（如事件名用了下划线——运行期 Validate 同样会拒）。
	bad := []appkit.Module{{Name: "demo", PublishedEvents: []string{"demo.item_created"}}}
	if err := compose.ValidateModules(bad); err == nil {
		t.Error("invalid event name: want error")
	}

	// 两个模块声明发布同一事件（撞目录）。
	dup := []appkit.Module{
		{Name: "a", PublishedEvents: []string{"a.x.created"}},
		{Name: "b", PublishedEvents: []string{"a.x.created"}},
	}
	if err := compose.ValidateModules(dup); err == nil {
		t.Error("duplicate published event: want error")
	}
}

func TestPolicyFiles(t *testing.T) {
	mods := []appkit.Module{
		{Name: "a", Policies: nil},
		{Name: "b"},
	}
	if got := len(compose.PolicyFiles(mods)); got != 0 {
		t.Errorf("empty policies: got %d, want 0", got)
	}
	_ = fs.FS(nil) // Migrations 类型存在性编译检查
}
