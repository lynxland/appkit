package kernel_test

// 分层守卫（ADR-0009 D6/D9）：电池分层的机器固化，学 fleetly guards 范式。
// 消费分层：电池层（严格中立）⊂ 契约层（grpcapi 冻结词汇+宿主类型）⊂ 偏向装配层。
//
//	严格中立电池禁 lynx / grpcapi / uptrace 直接 import：
//	  uow crud storeerr occ row clock errcode idgen jwt password secretkey goosekit
//	宿主电池（配置/事件绑定 lynx）禁 grpcapi / uptrace：configkit events
//	grpcapi 电池（principal 让渡）禁 uptrace：contexts
//	compose 是纯算法：禁 grpcapi / uptrace
//	chain 是 grpcapi 组装：禁 uptrace
//	根包契约：禁 uptrace（grpcapi 是冻结 SPI 词汇，lynx 是宿主身份）
//	根 go.mod 永不出现 uptrace（「核心不绑 ORM」的机器证明）

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// forbiddenImports 按包声明禁止的直接 import 前缀。
var forbiddenImports = map[string][]string{
	// 严格中立电池
	"uow":       {"github.com/lynx-go/lynx", "github.com/lynx-go/grpcapi", "uptrace"},
	"crud":      {"github.com/lynx-go/lynx", "github.com/lynx-go/grpcapi", "uptrace"},
	"storeerr":  {"github.com/lynx-go/lynx", "github.com/lynx-go/grpcapi", "uptrace"},
	"occ":       {"github.com/lynx-go/lynx", "github.com/lynx-go/grpcapi", "uptrace"},
	"row":       {"github.com/lynx-go/lynx", "github.com/lynx-go/grpcapi", "uptrace"},
	"clock":     {"github.com/lynx-go/lynx", "github.com/lynx-go/grpcapi", "uptrace"},
	"errcode":   {"github.com/lynx-go/lynx", "github.com/lynx-go/grpcapi", "uptrace"},
	"idgen":     {"github.com/lynx-go/lynx", "github.com/lynx-go/grpcapi", "uptrace"},
	"jwt":       {"github.com/lynx-go/lynx", "github.com/lynx-go/grpcapi", "uptrace"},
	"password":  {"github.com/lynx-go/lynx", "github.com/lynx-go/grpcapi", "uptrace"},
	"secretkey": {"github.com/lynx-go/lynx", "github.com/lynx-go/grpcapi", "uptrace"},
	"goosekit":  {"github.com/lynx-go/lynx", "github.com/lynx-go/grpcapi", "uptrace", "github.com/lynx-go/lynx-start"}, // 也不许 import 根契约（保持可被任意迁移执行方独立消费）
	// 宿主电池
	"configkit": {"github.com/lynx-go/grpcapi", "uptrace"},
	"events":    {"github.com/lynx-go/grpcapi", "uptrace"},
	// grpcapi 电池
	"contexts": {"uptrace"},
	"idem":     {"uptrace"},
	// 实现
	"compose": {"github.com/lynx-go/grpcapi", "uptrace"},
	"chain":   {"uptrace"},
	// 根包（目录 "." = package kernel 契约文件）
	".": {"uptrace"},
}

func TestLayerGuard(t *testing.T) {
	fset := token.NewFileSet()
	for dir, forbidden := range forbiddenImports {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			for _, imp := range file.Imports {
				p := strings.Trim(imp.Path.Value, `"`)
				for _, bad := range forbidden {
					// 斜杠边界匹配：`…/lynx` 不误伤 `…/lynx-start`
					if p == bad || strings.HasPrefix(p, bad+"/") {
						t.Errorf("%s 违反分层（ADR-0009 D6）：禁止 import %s", path, p)
					}
				}
			}
		}
	}
}

// TestRootGoModNoORM 断言根 module 的 go.mod 永不依赖任何 ORM（bun 只准
// 出现在 contrib/ 适配子模块）。
func TestRootGoModNoORM(t *testing.T) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"uptrace", "gorm.io", "entgo.io"} {
		if strings.Contains(string(data), banned) {
			t.Errorf("根 go.mod 出现 ORM 依赖 %q——ORM 只准出现在 contrib/ 适配子模块", banned)
		}
	}
}
