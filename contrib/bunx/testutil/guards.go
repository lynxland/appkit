// Package testutil —— 依赖护栏(ADR-0005)。
package testutil

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// appkitRootPath 是根包(Deps/Assembly 所在)的 import 路径。
const appkitRootPath = "github.com/lynxland/appkit"

// AssertDepsAtAPILayer 断言 appkit 根包未被 dirs 内的代码 import——
// Deps 只允许出现在 api 装配层(internal/api/<m>/module.go);domain/app/infra
// 一律经子包端口(uow/events/crud/…)消费。「Deps 子集化」用静态护栏表达,
// 替代 DI 框架的依赖图约束。
func AssertDepsAtAPILayer(t testing.TB, dirs ...string) {
	t.Helper()
	fset := token.NewFileSet()
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, imp := range file.Imports {
				if strings.Trim(imp.Path.Value, `"`) == appkitRootPath {
					t.Errorf("%s 违反 ADR-0005:appkit 根包(Deps)只允许 api 装配层 import(domain/app/infra 走子包端口)", path)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
}
