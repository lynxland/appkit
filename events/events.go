// Package events 定义模块间事件通信的信封与命名约定（ADR-0004 契约②）。
//
// 事件名是字符串目录："<module>.<noun>.<verb>"（如 "users.user.created"），
// 随模块源码发布（item.json 的 publishedEvents）。模块禁止 import 他模块
// domain 包来取事件名——跨模块消费靠目录字符串 + 本包的 Publisher 端口
// （torchwood 事件目录反环规则的移植）。传输由 lynx eventbus 承担，
// BusPublisher 把信封编码进 bus。
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/lynxland/lynx/eventbus"
)

// NameRe 约束事件名形态：小写段以点分隔，首段是模块名。
var NameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*(\.[a-z][a-z0-9-]*){2,}$`)

// Envelope 是事件信封：与传输解耦的最小事实集。Payload 是 JSON 编码的
// 模块自定义载荷（消费方按目录文档解码，不共享 Go 类型）。
type Envelope struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Module     string          `json:"module"`
	OccurredAt time.Time       `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	// Aggregate 是可选聚合轴（如 "organization/<id>"；outbox 列级存储
	// 便于按轴订阅/过滤，fleetly team 轴对应）。可空。
	Aggregate string `json:"aggregate,omitempty"`
}

// TopicFor 返回事件名对应的 bus topic（模块前缀收敛，便于通配订阅：
// topic = "module.<name>"，事件全集按模块分 topic）。
func TopicFor(name string) string {
	return "module." + strings.SplitN(name, ".", 2)[0]
}

// Validate 校验信封：事件名合法、Module 与事件名首段一致。
func (e *Envelope) Validate() error {
	if !NameRe.MatchString(e.Name) {
		return fmt.Errorf("events: invalid name %q", e.Name)
	}
	if e.Module == "" {
		e.Module = strings.SplitN(e.Name, ".", 2)[0]
	}
	if e.Module != strings.SplitN(e.Name, ".", 2)[0] {
		return fmt.Errorf("events: name %q not prefixed by module %q", e.Name, e.Module)
	}
	return nil
}

// Publisher 是模块发布事件的端口（domain/app 层依赖此接口，不依赖 bus）。
type Publisher interface {
	Publish(ctx context.Context, ev Envelope) error
}

// BusPublisher 把事件发布到 lynx eventbus：topic=TopicFor(name)，payload=
// JSON(Envelope)。跨进程投递（watermill/kafka）由 bus 配置决定，与本端口无关。
type BusPublisher struct {
	bus eventbus.Bus
}

// NewBusPublisher 绑定 bus（应用侧 wire 提供 app.Bus() 的产物）。
func NewBusPublisher(bus eventbus.Bus) *BusPublisher {
	return &BusPublisher{bus: bus}
}

// Publish 实现 Publisher。
func (p *BusPublisher) Publish(ctx context.Context, ev Envelope) error {
	if err := ev.Validate(); err != nil {
		return err
	}
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = time.Now().UTC()
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("events: marshal envelope: %w", err)
	}
	_, err = p.bus.Publish(ctx, TopicFor(ev.Name), json.RawMessage(raw))
	return err
}
