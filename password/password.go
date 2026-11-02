// Package password 提供 Argon2id 哈希与验证（torchwood pkg/password 移植，
// 参数同源：t=3 / m=64MiB / p=4 / key=32B / salt=16B）。
// PHC 字符串格式：$argon2id$v=19$m=…,t=…,p=…$salt$hash。
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

const (
	timeCost    = 3
	memoryCost  = 64 * 1024
	parallelism = 4
	keyLen      = 32
	saltLen     = 16
)

// Hash 返回 Argon2id PHC 字符串。
func Hash(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, timeCost, memoryCost, parallelism, keyLen)
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", memoryCost, timeCost, parallelism, b64Salt, b64Hash), nil
}

// Verify 恒定时间校验。格式非法返回 error；密码不匹配返回 (false, nil)。
func Verify(password, encodedHash string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, fmt.Errorf("invalid hash format")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, err
	}
	var memory, time, threads uint32
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false, err
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}
	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}
	actualHash := argon2.IDKey([]byte(password), salt, time, memory, uint8(threads), uint32(len(expectedHash)))
	return subtle.ConstantTimeCompare(actualHash, expectedHash) == 1, nil
}

// dummy 存放惰性生成的哑靶散列（DummyHash 首次调用时生成一次）。
var (
	dummyOnce sync.Once
	dummy     string
	dummyErr  error
)

// DummyHash 返回一个与当前参数恒同参的哑靶 Argon2id PHC 字符串，供登录
// 时序对齐使用（邮箱不存在时也跑一次等价验证，避免响应时间泄露账号
// 存在性）。散列由本包参数惰性生成——参数升级后哑靶自动同参，不会与
// 真实验证产生可观测的时间差；调用方不应把返回值当常量缓存跨进程使用。
func DummyHash() (string, error) {
	dummyOnce.Do(func() {
		dummy, dummyErr = Hash("appkit-dummy-target")
	})
	return dummy, dummyErr
}
