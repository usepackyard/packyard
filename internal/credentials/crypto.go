package credentials

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

const CiphertextPrefix = "v1:"

func DecodeKey(hexKey string) ([]byte, error) {
	key, err := hex.DecodeString(strings.TrimSpace(hexKey))
	if err != nil {
		return nil, fmt.Errorf("decode credentials key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("credentials key must be 32 bytes / 64 hex characters")
	}
	return key, nil
}

func EncryptString(plaintext, hexKey string) (string, error) {
	key, err := DecodeKey(hexKey)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return CiphertextPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

func DecryptString(ciphertext, hexKey string) (string, error) {
	key, err := DecodeKey(hexKey)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(ciphertext, CiphertextPrefix) {
		return "", fmt.Errorf("unsupported ciphertext version")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(ciphertext, CiphertextPrefix))
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, data := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, data, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt credential: %w", err)
	}
	return string(plaintext), nil
}

func TokenPrefix(token string) string {
	if len(token) <= 8 {
		return token
	}
	return token[:8]
}
