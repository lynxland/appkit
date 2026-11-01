package errcode

import (
	"fmt"
	"sort"
	"strings"
)

// Detail 是一条错误码的富信息（v2：fleetly 注册表形态的证据面并入——
// Suggestion 行动建议与 DocsURL 文档链接是其独有增量，torchwood 的
// retryable/metadata 投影归 errstatus）。
type Detail struct {
	Info
	// Suggestion 是行动建议（API 消费者的下一步；可空）。
	Suggestion string
	// DocsURL 是文档链接（可空；约定 https://<host>/errors/<canonical>）。
	DocsURL string
}

// Locale 是 i18n 消息目录（D14：错误码为键，locale → 消息文本）。
// 缺失回退链：locale → DefaultLocale → Info.Message。
const DefaultLocale = "en"

// Registry 是 v2 注册表：登记期 fail-fast（空 reason/重复 canonical/
// 缺 Message）+ locale 目录 + golden 快照导出。零值不可用，经
// NewRegistry 构造；进程内单实例（模块生态约定一个应用一个注册表）。
type Registry struct {
	details  map[string]Detail // canonical → detail
	messages map[string]map[string]string
}

// NewRegistry 构造空注册表。
func NewRegistry() *Registry {
	return &Registry{
		details:  map[string]Detail{},
		messages: map[string]map[string]string{},
	}
}

// MustRegister 登记（fail-fast panic：供模块包初始化期使用，形态对齐
// fleetly 注册表的启动期断言）：
//   - Info 为空 reason/非 UPPER_SNAKE reason → panic
//   - canonical 重复 → panic（跨模块撞名在此显形）
//   - Message 为空且无任何 locale 消息 → panic（错误码必须有可读说明）
func (r *Registry) MustRegister(d Detail, messages map[string]string) {
	if d.Reason == "" {
		panic(fmt.Sprintf("errcode: empty reason in domain %q", d.Domain))
	}
	if d.Reason != strings.ToUpper(d.Reason) || strings.ContainsAny(d.Reason, " -") {
		panic(fmt.Sprintf("errcode: reason %q must be UPPER_SNAKE", d.Reason))
	}
	canonical := d.Canonical()
	if _, dup := r.details[canonical]; dup {
		panic(fmt.Sprintf("errcode: duplicate canonical %q", canonical))
	}
	if d.Message == "" && len(messages) == 0 {
		panic(fmt.Sprintf("errcode: %s has no Message and no locale messages", canonical))
	}
	r.details[canonical] = d
	if len(messages) > 0 {
		r.messages[canonical] = map[string]string{}
		for locale, msg := range messages {
			r.messages[canonical][locale] = msg
		}
	}
}

// Lookup 按 canonical 查找（未登记返回 false——消费侧自行兜底）。
func (r *Registry) Lookup(canonical string) (Detail, bool) {
	d, ok := r.details[canonical]
	return d, ok
}

// Message 按 locale 取消息：locale → DefaultLocale → Detail.Message →
// canonical（四级回退，永不空串）。
func (r *Registry) Message(canonical, locale string) string {
	if msgs, ok := r.messages[canonical]; ok {
		if msg, ok := msgs[locale]; ok && msg != "" {
			return msg
		}
		if msg, ok := msgs[DefaultLocale]; ok && msg != "" {
			return msg
		}
	}
	if d, ok := r.details[canonical]; ok && d.Message != "" {
		return d.Message
	}
	return canonical
}

// Golden 导出确定性快照（canonical 升序；locale 消息键升序）——提交入库
// 的 golden 文件由此生成，漂移即审（fleetly codes.golden 对应物；用法：
// go test 内比对 r.Golden() 与 testdata/codes.golden 内容）。
func (r *Registry) Golden() string {
	canon := make([]string, 0, len(r.details))
	for c := range r.details {
		canon = append(canon, c)
	}
	sort.Strings(canon)
	var b strings.Builder
	for _, c := range canon {
		d := r.details[c]
		fmt.Fprintf(&b, "%s\t%s", c, r.Message(c, DefaultLocale))
		if d.Suggestion != "" {
			fmt.Fprintf(&b, "\tsuggest: %s", d.Suggestion)
		}
		if d.DocsURL != "" {
			fmt.Fprintf(&b, "\tdocs: %s", d.DocsURL)
		}
		// 非 default locale 逐行列出（键升序）。
		if msgs := r.messages[c]; len(msgs) > 0 {
			locales := make([]string, 0, len(msgs))
			for l := range msgs {
				if l != DefaultLocale {
					locales = append(locales, l)
				}
			}
			sort.Strings(locales)
			for _, l := range locales {
				fmt.Fprintf(&b, "\n%s.%s\t%s", c, l, msgs[l])
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// Registered 返回全部 canonical（升序）——usage 反扫（零引用码即红）
// 与 guard 断言的消费面。
func (r *Registry) Registered() []string {
	canon := make([]string, 0, len(r.details))
	for c := range r.details {
		canon = append(canon, c)
	}
	sort.Strings(canon)
	return canon
}

// Count 返回已登记条数。
func (r *Registry) Count() int { return len(r.details) }
