package server

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"
)

func backupKeyFromRef(ref string) ([]byte, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("backup encryption key reference is empty")
	}
	raw := ref
	if strings.HasPrefix(ref, "env:") {
		name := strings.TrimPrefix(ref, "env:")
		raw = strings.TrimSpace(os.Getenv(name))
		if raw == "" {
			return nil, fmt.Errorf("backup encryption key env %q is empty", name)
		}
	}
	sum := sha256.Sum256([]byte(raw))
	return sum[:], nil
}

func encryptFileAESGCM(srcPath, dstPath string, key []byte) error {
	plain, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	enc := gcm.Seal(nil, nonce, plain, nil)
	blob := append([]byte("ULTRONENC1"), append(nonce, enc...)...)
	return os.WriteFile(dstPath, blob, 0o600)
}
