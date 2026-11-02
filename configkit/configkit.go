// Package configkit 统一「配置自治」的加载姿势（ADR-0004 契约③）：
// 模块在自己的 Init 里 LoadSection——先落默认值再 UnmarshalKey 覆盖，
// WithErrorUnused 拼写错误 fail-fast。
package configkit

import (
	"fmt"

	"github.com/lynxland/lynx"
)

// LoadSection 把 c 的 key 段解码进 out。defaults 先执行（落零值缺省），
// 随后 UnmarshalKey 覆盖；未识别键返回错误（WithErrorUnused）。
func LoadSection(c lynx.Config, key string, defaults func(), out any) error {
	if defaults != nil {
		defaults()
	}
	if err := c.UnmarshalKey(key, out, lynx.WithErrorUnused()); err != nil {
		return fmt.Errorf("config section %q: %w", key, err)
	}
	return nil
}

// RequireString 校验必填字符串配置项（空串/空白即错误）。
func RequireString(what, v string) error {
	if v == "" {
		return fmt.Errorf("config: %s is required", what)
	}
	return nil
}

// RequirePositive 校验必填正数配置项。
func RequirePositive(what string, v int) error {
	if v <= 0 {
		return fmt.Errorf("config: %s must be positive", what)
	}
	return nil
}
