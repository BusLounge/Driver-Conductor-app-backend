package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

// getEncryptionKey reads ENCRYPTION_KEY from environment and returns 32-byte key.
// It accepts raw 32-byte string, hex-encoded, or base64-encoded key values.
func getEncryptionKey() ([]byte, error) {
    raw := os.Getenv("ENCRYPTION_KEY")
    if raw == "" {
        return nil, errors.New("ENCRYPTION_KEY not set")
    }

    // Try base64
    if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil && len(decoded) == 32 {
        return decoded, nil
    }

    // Try hex
    if decoded, err := hex.DecodeString(raw); err == nil && len(decoded) == 32 {
        return decoded, nil
    }

    // Raw bytes
    if len(raw) == 32 {
        return []byte(raw), nil
    }

    return nil, fmt.Errorf("ENCRYPTION_KEY must be 32 bytes (raw) or 32-byte key encoded in base64/hex")
}

// EncryptString encrypts plaintext using AES-256-GCM and returns base64(ciphertext), base64(nonce)
func EncryptString(plaintext string) (string, string, error) {
    key, err := getEncryptionKey()
    if err != nil {
        return "", "", err
    }

    block, err := aes.NewCipher(key)
    if err != nil {
        return "", "", fmt.Errorf("failed to create cipher: %w", err)
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", "", fmt.Errorf("failed to create gcm: %w", err)
    }

    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return "", "", fmt.Errorf("failed to generate nonce: %w", err)
    }

    ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)

    return base64.StdEncoding.EncodeToString(ciphertext), base64.StdEncoding.EncodeToString(nonce), nil
}

// DecryptString decrypts base64(ciphertext) with base64(nonce) and returns plaintext
func DecryptString(ciphertextB64, nonceB64 string) (string, error) {
    key, err := getEncryptionKey()
    if err != nil {
        return "", err
    }

    ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
    if err != nil {
        return "", fmt.Errorf("failed to decode ciphertext: %w", err)
    }

    nonce, err := base64.StdEncoding.DecodeString(nonceB64)
    if err != nil {
        return "", fmt.Errorf("failed to decode nonce: %w", err)
    }

    block, err := aes.NewCipher(key)
    if err != nil {
        return "", fmt.Errorf("failed to create cipher: %w", err)
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", fmt.Errorf("failed to create gcm: %w", err)
    }

    plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
    if err != nil {
        return "", fmt.Errorf("decryption failed: %w", err)
    }

    return string(plaintext), nil
}
