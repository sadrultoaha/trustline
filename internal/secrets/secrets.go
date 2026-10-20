// Package secrets encrypts integration credentials at rest with AES-256-GCM.
// The key is derived from ENCRYPTION_KEY with SHA-256 so any sufficiently long
// secret string works; ciphertexts carry a version byte for future rotation.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
)

const version byte = 1

type Box struct{ aead cipher.AEAD }

func New(key string) (*Box, error) {
	if len(key) < 16 {
		return nil, errors.New("encryption key must be at least 16 characters")
	}
	k := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(k[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext; aad binds the ciphertext to a context (e.g. integration id).
func (b *Box) Seal(plaintext, aad []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := append([]byte{version}, nonce...)
	return b.aead.Seal(out, nonce, plaintext, aad), nil
}

func (b *Box) Open(ciphertext, aad []byte) ([]byte, error) {
	ns := b.aead.NonceSize()
	if len(ciphertext) < 1+ns || ciphertext[0] != version {
		return nil, errors.New("secrets: malformed ciphertext")
	}
	pt, err := b.aead.Open(nil, ciphertext[1:1+ns], ciphertext[1+ns:], aad)
	if err != nil {
		return nil, fmt.Errorf("secrets: decrypt: %w", err)
	}
	return pt, nil
}

func (b *Box) SealJSON(v any, aad []byte) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return b.Seal(raw, aad)
}

func (b *Box) OpenJSON(ct, aad []byte, v any) error {
	pt, err := b.Open(ct, aad)
	if err != nil {
		return err
	}
	return json.Unmarshal(pt, v)
}
