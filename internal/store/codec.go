package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const encodedPrefix = "enc:v1:"

// codec encrypts the provider key with AES-256-GCM when a secret is configured.
// A nil aead keeps values as plaintext so a database created without APP_SECRET
// remains readable.
type codec struct {
	aead cipher.AEAD
}

func newCodec(secret string) (*codec, error) {
	if secret == "" {
		return &codec{}, nil
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &codec{aead: aead}, nil
}

func (c *codec) seal(value string) (string, error) {
	if c.aead == nil {
		return value, nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(value), nil)
	return encodedPrefix + base64.RawStdEncoding.EncodeToString(sealed), nil
}

func (c *codec) open(value string) (string, error) {
	if !strings.HasPrefix(value, encodedPrefix) {
		return value, nil
	}
	if c.aead == nil {
		return "", errors.New("API key is encrypted; set APP_SECRET to the value used when it was saved")
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, encodedPrefix))
	if err != nil {
		return "", fmt.Errorf("decode API key: %w", err)
	}
	size := c.aead.NonceSize()
	if len(raw) < size {
		return "", errors.New("decode API key: payload is too short")
	}
	plain, err := c.aead.Open(nil, raw[:size], raw[size:], nil)
	if err != nil {
		return "", errors.New("decrypt API key: APP_SECRET does not match")
	}
	return string(plain), nil
}
