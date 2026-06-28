// Package secretbox provides AES-256-GCM authenticated encryption for
// storing third-party credentials (e.g. API keys) at rest in the database.
// The key is loaded once from the environment and never persisted; the
// ciphertext is safe to store in any column.
//
// Wire format: base64(nonce || ciphertext || tag)
// nonce is 12 random bytes; the GCM tag is appended by cipher.Seal.
// A fresh nonce is generated per Seal call — never reuse.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

const (
	keySize   = 32 // AES-256
	nonceSize = 12 // GCM standard nonce
)

// ErrTampered is returned by Open when authentication fails.
var ErrTampered = errors.New("secretbox: authentication failed — ciphertext may be tampered or key mismatch")

// ErrInvalidKey is returned when the key length is not exactly 32 bytes.
var ErrInvalidKey = errors.New("secretbox: key must be exactly 32 bytes (AES-256)")

// Box holds an AES-256-GCM AEAD. Construct with New.
type Box struct {
	aead cipher.AEAD
}

// New constructs a Box from a 32-byte key.
func New(key []byte) (*Box, error) {
	if len(key) != keySize {
		return nil, ErrInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secretbox: create GCM: %w", err)
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts and authenticates plaintext, returning base64(nonce||ct||tag).
// Each call generates a fresh random nonce.
func (b *Box) Seal(plaintext []byte) (string, error) {
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("secretbox: generate nonce: %w", err)
	}
	// cipher.Seal appends ciphertext+tag to nonce in-place.
	out := b.aead.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Open decrypts and authenticates a value produced by Seal.
// Returns ErrTampered if authentication fails.
func (b *Box) Open(encoded string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("secretbox: decode base64: %w", err)
	}
	if len(raw) < nonceSize {
		return nil, ErrTampered
	}
	nonce := raw[:nonceSize]
	ct := raw[nonceSize:]
	plaintext, err := b.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		// GCM authentication failure — surface as ErrTampered, not the raw crypto error.
		return nil, ErrTampered
	}
	return plaintext, nil
}
