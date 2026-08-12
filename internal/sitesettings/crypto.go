package sitesettings

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

const (
	secretVersion   = 1
	secretAlgorithm = "AES-256-GCM"
	masterKeyBytes  = 32
)

type secretEnvelope struct {
	Version    int    `json:"version"`
	Algorithm  string `json:"algorithm"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type secretCodec struct {
	store MasterKeyStore
	mu    sync.Mutex
	key   []byte
}

func newSecretCodec(store MasterKeyStore) *secretCodec {
	if store == nil {
		store = NewOSMasterKeyStore()
	}
	return &secretCodec{store: store}
}

func (codec *secretCodec) encrypt(ctx context.Context, settingKey, value string, mayCreateKey bool) (string, error) {
	key, err := codec.masterKey(ctx, mayCreateKey)
	if err != nil {
		return "", err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", settingError(CodeSecretUnavailable, "无法加密安全设置", "系统随机数生成器不可用。", err)
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(value), secretAAD(settingKey))
	envelope, err := json.Marshal(secretEnvelope{
		Version: secretVersion, Algorithm: secretAlgorithm,
		Nonce: base64.RawStdEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawStdEncoding.EncodeToString(ciphertext),
	})
	if err != nil {
		return "", settingError(CodeSecretUnavailable, "无法加密安全设置", "密文信封编码失败。", err)
	}
	return string(envelope), nil
}

func (codec *secretCodec) decrypt(ctx context.Context, settingKey, raw string) (string, error) {
	key, err := codec.masterKey(ctx, false)
	if err != nil {
		return "", err
	}
	var envelope secretEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil || envelope.Version != secretVersion || envelope.Algorithm != secretAlgorithm {
		return "", settingError(CodeSecretUnavailable, "安全设置不可用", "密文格式或版本不受支持。", err)
	}
	nonce, nonceErr := base64.RawStdEncoding.DecodeString(envelope.Nonce)
	ciphertext, ciphertextErr := base64.RawStdEncoding.DecodeString(envelope.Ciphertext)
	if nonceErr != nil || ciphertextErr != nil {
		return "", settingError(CodeSecretUnavailable, "安全设置不可用", "密文编码无效。", errors.Join(nonceErr, ciphertextErr))
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, secretAAD(settingKey))
	if err != nil {
		return "", settingError(CodeSecretUnavailable, "安全设置不可用", "Keychain 根密钥与密文不匹配。", err)
	}
	return string(plaintext), nil
}

func (codec *secretCodec) masterKey(ctx context.Context, mayCreate bool) ([]byte, error) {
	codec.mu.Lock()
	defer codec.mu.Unlock()
	if len(codec.key) == masterKeyBytes {
		return append([]byte(nil), codec.key...), nil
	}
	encoded, err := codec.store.Get(ctx)
	if err == nil {
		decoded, decodeErr := base64.RawStdEncoding.DecodeString(encoded)
		if decodeErr == nil && len(decoded) == masterKeyBytes {
			codec.key = append([]byte(nil), decoded...)
			return append([]byte(nil), codec.key...), nil
		}
		if !mayCreate {
			return nil, settingError(CodeSecretUnavailable, "安全设置不可用", "Keychain 中的 AES-256-GCM 根密钥无效。", decodeErr)
		}
	} else if !errors.Is(err, ErrMasterKeyNotFound) || !mayCreate {
		return nil, settingError(CodeSecretUnavailable, "无法访问安全设置", "Keychain 中的 AES 根密钥不可用。", err)
	}
	key := make([]byte, masterKeyBytes)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, settingError(CodeSecretUnavailable, "无法创建安全设置", "系统随机数生成器不可用。", err)
	}
	if err := codec.store.Set(ctx, base64.RawStdEncoding.EncodeToString(key)); err != nil {
		return nil, settingError(CodeSecretUnavailable, "无法创建安全设置", "AES 根密钥无法写入 Keychain。", err)
	}
	codec.key = append([]byte(nil), key...)
	return append([]byte(nil), key...), nil
}

func (codec *secretCodec) clear() {
	codec.mu.Lock()
	defer codec.mu.Unlock()
	for index := range codec.key {
		codec.key[index] = 0
	}
	codec.key = nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, settingError(CodeSecretUnavailable, "安全设置不可用", "无法初始化 AES-256。", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, settingError(CodeSecretUnavailable, "安全设置不可用", "无法初始化 AES-GCM。", err)
	}
	return gcm, nil
}

func secretAAD(settingKey string) []byte {
	return []byte(fmt.Sprintf("lumi:site-setting:%d:%s", secretVersion, settingKey))
}
