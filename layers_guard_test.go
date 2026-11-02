package appkit_test

// 分层守卫（ADR-0009 D6/D9）：电池分层的机器固化，学 fleetly guards 范式。
// 消费分层：电池层（严格中立）⊂ 契约层（grpcapi 冻结词汇+宿主类型）⊂ 偏向装配层。
//
//	严格中立电池禁 lynx / grpcapi / uptrace 直接 import：
//	  uow crud storeerr occ row outbox lock blob cache semaphore query queue
//	  clock errcode idgen jwt password secretkey goosekit
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
	"uow":       {"github.com/lynxland/lynx", "github.com/lynxland/grpcapi", "uptrace"},
	"crud":      {"github.com/lynxland/lynx", "github.com/lynxland/grpcapi", "uptrace"},
	"storeerr":  {"github.com/lynxland/lynx", "github.com/lynxland/grpcapi", "uptrace"},
	"occ":       {"github.com/lynxland/lynx", "github.com/lynxland/grpcapi", "uptrace"},
	"row":       {"github.com/lynxland/lynx", "github.com/lynxland/grpcapi", "uptrace"},
	"outbox":    {"github.com/lynxland/lynx", "github.com/lynxland/grpcapi", "uptrace"},
	"lock":      {"github.com/lynxland/lynx", "github.com/lynxland/grpcapi", "uptrace"},
	"blob":      {"github.com/lynxland/lynx", "github.com/lynxland/grpcapi", "uptrace"},
	"cache":     {"github.com/lynxland/lynx", "github.com/lynxland/grpcapi", "uptrace"},
	"semaphore": {"github.com/lynxland/lynx", "github.com/lynxland/grpcapi", "uptrace"},
	"query":     {"github.com/lynxland/lynx", "github.com/lynxland/grpcapi", "uptrace"},
	"queue":     {"github.com/lynxland/lynx", "github.com/lynxland/grpcapi", "uptrace"},
	"clock":     {"github.com/lynxland/lynx", "github.com/lynxland/grpcapi", "uptrace"},
	"errcode":   {"github.com/lynxland/lynx", "github.com/lynxland/grpcapi", "uptrace"},
	"idgen":     {"github.com/lynxland/lynx", "github.com/lynxland/grpcapi", "uptrace"},
	"jwt":       {"github.com/lynxland/lynx", "github.com/lynxland/grpcapi", "uptrace"},
	"password":  {"github.com/lynxland/lynx", "github.com/lynxland/grpcapi", "uptrace"},
	"secretkey": {"github.com/lynxland/lynx", "github.com/lynxland/grpcapi", "uptrace"},
	"goosekit":  {"github.com/lynxland/lynx", "github.com/lynxland/grpcapi", "uptrace", "github.com/lynxland/appkit"}, // 也不许 import 根契约（保持可被任意迁移执行方独立消费）
	// 宿主电池
	"configkit": {"github.com/lynxland/grpcapi", "uptrace"},
	"events":    {"github.com/lynxland/grpcapi", "uptrace"},
	"periodic":  {"github.com/lynxland/grpcapi", "uptrace"},
	// grpcapi 电池
	"contexts":  {"uptrace"},
	"errstatus": {"uptrace"},
	"idem":      {"uptrace"},
	"ratelimit": {"uptrace"},
	// 宿主电池
	// 实现
	"compose": {"github.com/lynxland/grpcapi", "uptrace"},
	"chain":   {"uptrace"},
	// 根包（目录 "." = package appkit 契约文件）
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
					// 斜杠边界匹配：前缀命中必须落在完整路径段边界
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
