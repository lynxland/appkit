// Package blob 是对象存储端点电池（ADR-0009 D13 蓝图桶，证据 2×：
// torchwood MemObjectStore+MinIO、fleetly 内容寻址 upload）：Store 端口
// （Put/Get/Delete/List）+ 单进程内存实现起步。S3/MinIO 适配走 Store
// 端口自带实现（aws-sdk-go-v2 或 minio-go；首个消费者时动工——
// D13 触发器纪律）。
//
// Key 约定：无前导斜杠的分层路径（如 "org/<id>/avatar/128.png"）；
// Content-Addressed 场景（fleetly 形态）由调用方自行哈希为 Key。
package blob

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrNotFound 是对象缺失哨兵（与 storeerr.ErrNotFound 语义一致但独立
// ——blob 域内自用，避免核心包反向依赖）。
var ErrNotFound = errors.New("blob: object not found")

// Object 是存储的对象（Get 返回全量；List 仅元信息，Data 为空）。
type Object struct {
	Key         string
	Data        []byte
	ContentType string
	Size        int64
	CreatedAt   time.Time
}

// Store 是对象存储端口。
type Store interface {
	// Put 写入或覆写对象（Key 唯一——同 Key 后写覆盖前写）。
	Put(ctx context.Context, obj Object) error
	// Get 按 Key 取对象（含 Data）；缺失返回 ErrNotFound。
	Get(ctx context.Context, key string) (Object, error)
	// Delete 按 Key 删除（幂等：不存在不报错）。
	Delete(ctx context.Context, key string) error
	// List 按前缀列出对象元信息（不含 Data；字典序）。
	List(ctx context.Context, prefix string) ([]Object, error)
}

// MemStore 是单进程内存实现（测试/开发/小规模内嵌场景；分布式用
// S3/MinIO 适配）。
type MemStore struct {
	mu      sync.RWMutex
	objects map[string]Object
}

// NewMemStore 构造。
func NewMemStore() *MemStore { return &MemStore{objects: map[string]Object{}} }

// Put 实现 Store。
func (s *MemStore) Put(_ context.Context, obj Object) error {
	if obj.Key == "" {
		return errors.New("blob: empty key")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stored := Object{Key: obj.Key, Data: obj.Data, ContentType: obj.ContentType, Size: int64(len(obj.Data)), CreatedAt: time.Now().UTC()}
	s.objects[obj.Key] = stored
	return nil
}

// Get 实现 Store。
func (s *MemStore) Get(_ context.Context, key string) (Object, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	obj, ok := s.objects[key]
	if !ok {
		return Object{}, ErrNotFound
	}
	return obj, nil
}

// Delete 实现 Store。
func (s *MemStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}

// List 实现 Store（仅元信息）。
func (s *MemStore) List(_ context.Context, prefix string) ([]Object, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Object
	for k, obj := range s.objects {
		if strings.HasPrefix(k, prefix) {
			meta := obj
			meta.Data = nil
			out = append(out, meta)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}
