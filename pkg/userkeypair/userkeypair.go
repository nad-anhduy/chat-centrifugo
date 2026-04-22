package userkeypair

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
)

const rsaBits = 2048

// GenerateRSA2048PEM returns PKIX SPKI public PEM and PKCS#8 private PEM (Web Crypto compatible).
func GenerateRSA2048PEM() (publicPEM string, privatePEM string, err error) {
	priv, err := rsa.GenerateKey(rand.Reader, rsaBits)
	if err != nil {
		return "", "", err
	}

	pubBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return "", "", err
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})

	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", "", err
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})

	return string(pubPEM), string(privPEM), nil
}

func deriveAES256Key(masterSecret string) []byte {
	// If exactly 64 hex chars, treat as 32-byte key material.
	if len(masterSecret) == 64 {
		if b, err := hex.DecodeString(masterSecret); err == nil && len(b) == 32 {
			return b
		}
	}
	sum := sha256.Sum256([]byte(masterSecret))
	return sum[:]
}

// EncryptPEMWithMasterKey encrypts plaintext (e.g. PKCS#8 PEM) using AES-256-GCM.
// Wire format: base64(nonce12 || ciphertext||tag).
func EncryptPEMWithMasterKey(masterSecret, plaintext string) (string, error) {
	if masterSecret == "" {
		return "", errors.New("MASTER_KEY is empty")
	}
	key := deriveAES256Key(masterSecret)
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
		return "", err
	}
	out := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

// DecryptPEMWithMasterKey reverses EncryptPEMWithMasterKey.
func DecryptPEMWithMasterKey(masterSecret, blob string) (string, error) {
	if masterSecret == "" {
		return "", errors.New("MASTER_KEY is empty")
	}
	raw, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		return "", fmt.Errorf("decode private key blob: %w", err)
	}
	key := deriveAES256Key(masterSecret)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := gcm.NonceSize()
	if len(raw) < ns {
		return "", errors.New("private key blob too short")
	}
	nonce, ct := raw[:ns], raw[ns:]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt private key: %w", err)
	}
	return string(pt), nil
}
