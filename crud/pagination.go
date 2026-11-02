// Package crud 提供列表查询的通用分页方言（torchwood pkg/crud 移植）：
//   - AIP-158 offset 分页（page_size/page_token 双向 token + 终止契约，list.go）；
//   - HMAC 签名的页 token（本文件，防伪造 offset 与跨环境重放）；
//   - 时间 keyset 游标（timecursor.go，方向化前缀）。
package crud

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lynxland/appkit/secretkey"
)

const (
	// PageTokenVersion 是页 token 格式版本。
	PageTokenVersion = "v1"

	// DefaultTokenTTL 是页 token 默认有效期。
	DefaultTokenTTL = 24 * time.Hour

	// TokenModeLegacy 表示 offset 型 token。
	TokenModeLegacy = "legacy"

	// MaxQueryOffset 拒绝超深分页 token，防打爆数据库。
	MaxQueryOffset = 10000

	pageTokenKeyPurpose = "lynxstart-page-token-v1"
)

// pageTokenSecret 进程级签名密钥（HMAC-SHA256，空串=未启用）。组合根启动期
// 经 InitPageTokenSigning 注入（server 与 worker 都必须调用）；未注入时编码
// 退化为未签名 token、带签名的 token 解码被拒——灰度发布期间旧 token 兼容，
// 新签发 token 在任何配置正确的进程中都可验证。
var pageTokenSecret atomic.Value // string

// pageTokenMarshal 是编码链路的序列化入口（var 仅为测试注入必败 marshaler）。
var pageTokenMarshal = json.Marshal

// InitPageTokenSigning 启用页 token 的 HMAC 签名与验签。master 为部署主密钥；
// 实际密钥经 secretkey.Derive(master, purpose) 派生，与其他用途域隔离。
// Deprecated: v1 分页形态。ADR-0009 D11 已定典多模式框架（Spec/Parse/
// Cursor/ApplyPage）：bounded/offset/keyset 三模式 + 白名单排序 + 编码
// 游标（encode-not-sign，无密钥仪式）。新代码一律使用 v2；本组 API 保持
// 编译兼容，随模块换装退场。
func InitPageTokenSigning(master string) error {
	if err := secretkey.MustValidate(master, "page token signing"); err != nil {
		return err
	}
	pageTokenSecret.Store(secretkey.DeriveHex(master, pageTokenKeyPurpose))
	return nil
}

// PageTokenSigningEnabled 报告当前进程是否已启用签名（测试与启动断言用）。
func PageTokenSigningEnabled() bool { return pageTokenKey() != "" }

func pageTokenKey() string {
	if v, ok := pageTokenSecret.Load().(string); ok {
		return v
	}
	return ""
}

// seal 为 token 附加 HMAC 签名（若已启用签名）。签名失败仅可能因内部状态
// 异常，此时退化为未签名并在解码侧被拒（fail-closed），不影响可用性。
func seal(data *PageTokenData) {
	key := pageTokenKey()
	if key == "" {
		return
	}
	if sig, err := SignPageToken(data, key); err == nil {
		data.Sig = sig
	}
}

// Deprecated: v1 分页形态。ADR-0009 D11 已定典多模式框架（Spec/Parse/
// Cursor/ApplyPage）：bounded/offset/keyset 三模式 + 白名单排序 + 编码
// 游标（encode-not-sign，无密钥仪式）。新代码一律使用 v2；本组 API 保持
// 编译兼容，随模块换装退场。
// PageTokenData 是页 token 的内部结构（AIP-158）。
type PageTokenData struct {
	Version      string    `json:"v"`
	Mode         string    `json:"mode,omitempty"`
	Offset       int       `json:"offset"`
	Created      time.Time `json:"created"`
	PageSize     int32     `json:"page_size"`
	OrderBy      string    `json:"order_by,omitempty"`
	FilterDigest string    `json:"filter_digest,omitempty"`
	TTL          int64     `json:"ttl,omitempty"`
	Sig          string    `json:"sig,omitempty"`
}

// EncodePageToken 从 offset 编码页 token（已启用签名时附带 HMAC，客户端无法
// 伪造偏移）。
func EncodePageToken(offset int) (string, error) {
	data := PageTokenData{
		Version: PageTokenVersion,
		Mode:    TokenModeLegacy,
		Offset:  offset,
		Created: time.Now().UTC(),
	}
	seal(&data)
	return encodeTokenData(data)
}

// SignPageToken 计算 token 载荷的 HMAC-SHA256 签名。
func SignPageToken(data *PageTokenData, secret string) (string, error) {
	if data == nil {
		return "", fmt.Errorf("page token data is nil")
	}
	if strings.TrimSpace(secret) == "" {
		return "", fmt.Errorf("page token secret is required")
	}
	canonical := *data
	canonical.Sig = ""
	raw, err := pageTokenMarshal(canonical)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// VerifyPageTokenSignature 校验页 token 签名。
func VerifyPageTokenSignature(data *PageTokenData, secret string) error {
	if data == nil {
		return fmt.Errorf("page token data is nil")
	}
	if data.Sig == "" {
		return fmt.Errorf("page token signature missing")
	}
	expected, err := SignPageToken(data, secret)
	if err != nil {
		return err
	}
	if !hmac.Equal([]byte(expected), []byte(data.Sig)) {
		return fmt.Errorf("invalid page token signature")
	}
	return nil
}

func encodeTokenData(data PageTokenData) (string, error) {
	jsonBytes, err := pageTokenMarshal(data)
	if err != nil {
		return "", fmt.Errorf("marshal page token data: %w", err)
	}
	return base64.URLEncoding.EncodeToString(jsonBytes), nil
}

// DecodePageToken 解码页 token 返回 offset。仅接受 base64 JSON 格式；带 Sig
// 的 token 必须能被当前进程密钥验证；启用签名的进程拒绝无签名 token
// （fail-closed）。
func DecodePageToken(token string) (int, error) {
	if token == "" {
		return 0, nil
	}
	data, err := decodeTokenData(token)
	if err != nil {
		return 0, fmt.Errorf("invalid page token format")
	}
	if err := verifyTokenAuthenticity(&data); err != nil {
		return 0, err
	}
	if data.Version != PageTokenVersion {
		return 0, fmt.Errorf("invalid page token version: %s", data.Version)
	}
	ttl := DefaultTokenTTL
	if data.TTL > 0 {
		ttl = time.Duration(data.TTL) * time.Second
	}
	if time.Since(data.Created) > ttl {
		return 0, fmt.Errorf("page token has expired")
	}
	return data.Offset, nil
}

// verifyTokenAuthenticity：未启用签名（key 为空）时跳过校验保持历史语义；
// 已启用签名时无签名/篡改/跨环境 token 一律拒绝。
func verifyTokenAuthenticity(data *PageTokenData) error {
	key := pageTokenKey()
	if key == "" {
		return nil
	}
	if data.Sig == "" {
		return fmt.Errorf("page token signature missing: token signing is enforced on this deployment")
	}
	return VerifyPageTokenSignature(data, key)
}

func decodeTokenData(token string) (PageTokenData, error) {
	var data PageTokenData
	jsonBytes, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return data, fmt.Errorf("failed to decode page token: %w", err)
	}
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		return data, fmt.Errorf("failed to parse page token data: %w", err)
	}
	return data, nil
}

// DecodePageTokenFull 解码页 token 返回完整元数据。
func DecodePageTokenFull(token string) (*PageTokenData, error) {
	if token == "" {
		return nil, fmt.Errorf("empty page token")
	}
	data, err := decodeTokenData(token)
	if err != nil {
		return nil, err
	}
	if err := verifyTokenAuthenticity(&data); err != nil {
		return nil, err
	}
	if data.Version != PageTokenVersion {
		return nil, fmt.Errorf("invalid page token version: %s", data.Version)
	}
	ttl := DefaultTokenTTL
	if data.TTL > 0 {
		ttl = time.Duration(data.TTL) * time.Second
	}
	if time.Since(data.Created) > ttl {
		return nil, fmt.Errorf("page token has expired")
	}
	if data.Mode == "" {
		data.Mode = TokenModeLegacy
	}
	return &data, nil
}
