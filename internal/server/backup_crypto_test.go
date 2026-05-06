package server

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func writeTempFile(t *testing.T, content []byte) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "in.bin")
	if err := os.WriteFile(p, content, 0o600); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	return p
}

func roundTrip(t *testing.T, plain []byte) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	enc := filepath.Join(dir, "enc")
	dec := filepath.Join(dir, "dec")
	if err := os.WriteFile(src, plain, 0o600); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		t.Fatal(err)
	}
	if err := encryptFileAESGCM(src, enc, key); err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if err := decryptFileAESGCM(enc, dec, key); err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	got, err := os.ReadFile(dec)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("round-trip mismatch: got %d bytes, want %d", len(got), len(plain))
	}
}

func TestBackupCrypto_RoundTrip_Empty(t *testing.T) {
	roundTrip(t, []byte{})
}

func TestBackupCrypto_RoundTrip_TinyShortChunk(t *testing.T) {
	roundTrip(t, []byte("hello"))
}

func TestBackupCrypto_RoundTrip_ExactlyOneChunk(t *testing.T) {
	plain := bytes.Repeat([]byte{0xAB}, backupChunkSize)
	roundTrip(t, plain)
}

func TestBackupCrypto_RoundTrip_ExactMultipleChunks(t *testing.T) {
	// Triggers the "n_full=full, errCur=nil; next read returns EOF" path
	// where the final chunk is full-sized rather than short.
	plain := bytes.Repeat([]byte{0xCD}, backupChunkSize*3)
	roundTrip(t, plain)
}

func TestBackupCrypto_RoundTrip_PartialFinalChunk(t *testing.T) {
	plain := bytes.Repeat([]byte{0x42}, backupChunkSize*2+123)
	roundTrip(t, plain)
}

func TestBackupCrypto_RoundTrip_LargerThanRAMShape(t *testing.T) {
	// Not actually RAM-stressing in tests, but verifies the streaming
	// path handles many chunks. 17 chunks chosen as a non-power-of-two
	// to catch off-by-one in counter/AAD wiring.
	plain := make([]byte, backupChunkSize*17+7)
	if _, err := io.ReadFull(rand.Reader, plain); err != nil {
		t.Fatal(err)
	}
	roundTrip(t, plain)
}

func TestBackupCrypto_RejectsWrongKey(t *testing.T) {
	dir := t.TempDir()
	src := writeTempFile(t, []byte("the database content"))
	enc := filepath.Join(dir, "enc")
	dec := filepath.Join(dir, "dec")
	keyA := bytes.Repeat([]byte{0x01}, 32)
	keyB := bytes.Repeat([]byte{0x02}, 32)
	if err := encryptFileAESGCM(src, enc, keyA); err != nil {
		t.Fatal(err)
	}
	err := decryptFileAESGCM(enc, dec, keyB)
	if err == nil {
		t.Fatal("decrypt with wrong key must fail")
	}
	if _, statErr := os.Stat(dec); statErr == nil {
		t.Fatal("partial dec file must be cleaned up on error")
	}
}

func TestBackupCrypto_RejectsTamperedHeader(t *testing.T) {
	dir := t.TempDir()
	src := writeTempFile(t, []byte("payload"))
	enc := filepath.Join(dir, "enc")
	key := bytes.Repeat([]byte{0x33}, 32)
	if err := encryptFileAESGCM(src, enc, key); err != nil {
		t.Fatal(err)
	}
	// Flip a byte in the magic.
	blob, err := os.ReadFile(enc)
	if err != nil {
		t.Fatal(err)
	}
	blob[0] ^= 0xFF
	if err := os.WriteFile(enc, blob, 0o600); err != nil {
		t.Fatal(err)
	}
	dec := filepath.Join(dir, "dec")
	err = decryptFileAESGCM(enc, dec, key)
	if !errors.Is(err, errBackupUnknownMagic) {
		t.Fatalf("expected magic mismatch, got %v", err)
	}
}

func TestBackupCrypto_RejectsTamperedCiphertext(t *testing.T) {
	dir := t.TempDir()
	src := writeTempFile(t, bytes.Repeat([]byte{0x77}, backupChunkSize+5))
	enc := filepath.Join(dir, "enc")
	key := bytes.Repeat([]byte{0x33}, 32)
	if err := encryptFileAESGCM(src, enc, key); err != nil {
		t.Fatal(err)
	}
	blob, err := os.ReadFile(enc)
	if err != nil {
		t.Fatal(err)
	}
	// Flip a byte well inside the first ciphertext chunk (past header
	// + chunk frame). GCM tag should reject.
	blob[backupHeaderLen+5+50] ^= 0x80
	if err := os.WriteFile(enc, blob, 0o600); err != nil {
		t.Fatal(err)
	}
	dec := filepath.Join(dir, "dec")
	if err := decryptFileAESGCM(enc, dec, key); err == nil {
		t.Fatal("expected GCM tag to reject tampered ciphertext")
	}
}

func TestBackupCrypto_RejectsTruncated(t *testing.T) {
	dir := t.TempDir()
	src := writeTempFile(t, bytes.Repeat([]byte{0x55}, backupChunkSize*2))
	enc := filepath.Join(dir, "enc")
	key := bytes.Repeat([]byte{0x33}, 32)
	if err := encryptFileAESGCM(src, enc, key); err != nil {
		t.Fatal(err)
	}
	blob, err := os.ReadFile(enc)
	if err != nil {
		t.Fatal(err)
	}
	// Cut the file in the middle of the second chunk.
	if err := os.WriteFile(enc, blob[:len(blob)-100], 0o600); err != nil {
		t.Fatal(err)
	}
	dec := filepath.Join(dir, "dec")
	err = decryptFileAESGCM(enc, dec, key)
	if err == nil {
		t.Fatal("expected truncation to be detected")
	}
}

func TestBackupCrypto_OutputUsesV2Magic(t *testing.T) {
	dir := t.TempDir()
	src := writeTempFile(t, []byte("data"))
	enc := filepath.Join(dir, "enc")
	key := bytes.Repeat([]byte{0x09}, 32)
	if err := encryptFileAESGCM(src, enc, key); err != nil {
		t.Fatal(err)
	}
	blob, err := os.ReadFile(enc)
	if err != nil {
		t.Fatal(err)
	}
	if string(blob[:10]) != backupMagicV2 {
		t.Fatalf("expected magic %q, got %q", backupMagicV2, blob[:10])
	}
}
