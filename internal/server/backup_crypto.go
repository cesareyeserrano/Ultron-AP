package server

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// V2 streaming format. Encrypts the input in fixed-size chunks so peak
// memory stays bounded by chunkSize regardless of the database size —
// the V1 implementation read the whole file into RAM and called
// gcm.Seal on it, which would OOM on the Pi for a database approaching
// available RAM.
//
// File layout:
//
//   header  := magic(10) || chunkSize(4 BE) || nonceBase(8)
//   chunk   := isFinal(1) || ctLen(4 BE) || ciphertext+tag
//   payload := chunk*  (loop until a chunk with isFinal=1 is read)
//
// AAD per chunk binds the chunk index and the final flag so neither
// reordering nor truncation can be hidden — the GCM tag verification
// fails if either is tampered with. Nonce per chunk is nonceBase(8) ||
// counter(4 BE), giving 2^32 chunks per backup before counter wrap
// (with 64 KiB chunks that is 256 TiB — well beyond any practical DB
// size).
const (
	backupMagicV2     = "ULTRONENC2"
	backupChunkSize   = 64 * 1024 // bytes; matches age/secretstream defaults.
	backupHeaderLen   = 10 + 4 + 8
	backupNonceLen    = 12
	backupNonceBaseLen = 8
)

var (
	errBackupUnknownMagic   = errors.New("backup: unknown magic / not an ultron-encrypted file")
	errBackupTruncated      = errors.New("backup: truncated stream (no final chunk seen)")
	errBackupTrailingBytes  = errors.New("backup: trailing bytes after final chunk")
	errBackupInvalidChunk   = errors.New("backup: invalid chunk length")
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

// encryptFileAESGCM streams the plaintext file at srcPath into an
// AES-GCM V2 ciphertext file at dstPath. Peak in-flight memory is
// O(chunkSize) regardless of input size.
func encryptFileAESGCM(srcPath, dstPath string, key []byte) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	closeOK := false
	defer func() {
		if !closeOK {
			_ = dst.Close()
			_ = os.Remove(dstPath)
		}
	}()

	gcm, err := newBackupGCM(key)
	if err != nil {
		return err
	}

	nonceBase := make([]byte, backupNonceBaseLen)
	if _, err := io.ReadFull(rand.Reader, nonceBase); err != nil {
		return fmt.Errorf("backup: nonce generation: %w", err)
	}

	header := make([]byte, 0, backupHeaderLen)
	header = append(header, []byte(backupMagicV2)...)
	header = binary.BigEndian.AppendUint32(header, uint32(backupChunkSize))
	header = append(header, nonceBase...)
	if _, err := dst.Write(header); err != nil {
		return err
	}

	bufA := make([]byte, backupChunkSize)
	bufB := make([]byte, backupChunkSize)
	current := bufA
	next := bufB

	nCur, errCur := io.ReadFull(src, current)
	if errCur == io.EOF {
		// Empty input: emit a single empty final chunk so decryption
		// always sees an isFinal=1 marker.
		if err := writeBackupChunk(dst, gcm, nonceBase, 0, true, nil); err != nil {
			return err
		}
		if err := dst.Close(); err != nil {
			return err
		}
		closeOK = true
		return nil
	}
	if errCur != nil && errCur != io.ErrUnexpectedEOF {
		return errCur
	}

	var counter uint32
	for {
		nNext, errNext := io.ReadFull(src, next)
		isLast := errNext == io.EOF
		if errNext != nil && !isLast && errNext != io.ErrUnexpectedEOF {
			return errNext
		}

		final := isLast || errCur == io.ErrUnexpectedEOF
		if err := writeBackupChunk(dst, gcm, nonceBase, counter, final, current[:nCur]); err != nil {
			return err
		}
		if final {
			break
		}

		counter++
		if counter == 0 {
			return fmt.Errorf("backup: chunk counter overflowed (input too large)")
		}
		current, next = next, current
		nCur, errCur = nNext, errNext
	}

	if err := dst.Close(); err != nil {
		return err
	}
	closeOK = true
	return nil
}

// decryptFileAESGCM reverses encryptFileAESGCM. Streaming, O(chunkSize)
// peak memory. Validates magic, chunk ordering, final-flag, trailing
// bytes — any tampering surfaces as a non-nil error and a torn-down
// destination file.
func decryptFileAESGCM(srcPath, dstPath string, key []byte) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	header := make([]byte, backupHeaderLen)
	if _, err := io.ReadFull(src, header); err != nil {
		return errBackupUnknownMagic
	}
	if string(header[:10]) != backupMagicV2 {
		return errBackupUnknownMagic
	}
	chunkSize := binary.BigEndian.Uint32(header[10:14])
	if chunkSize == 0 || chunkSize > 16*1024*1024 {
		return fmt.Errorf("backup: implausible chunk size %d", chunkSize)
	}
	nonceBase := append([]byte(nil), header[14:14+backupNonceBaseLen]...)

	gcm, err := newBackupGCM(key)
	if err != nil {
		return err
	}

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	closeOK := false
	defer func() {
		if !closeOK {
			_ = dst.Close()
			_ = os.Remove(dstPath)
		}
	}()

	maxCT := int(chunkSize) + gcm.Overhead()
	hdr := make([]byte, 5) // isFinal(1) || ctLen(4)
	ct := make([]byte, maxCT)
	var counter uint32
	for {
		if _, err := io.ReadFull(src, hdr); err != nil {
			return errBackupTruncated
		}
		isFinal := hdr[0] == 1
		ctLen := binary.BigEndian.Uint32(hdr[1:5])
		if ctLen == 0 || int(ctLen) > maxCT {
			return errBackupInvalidChunk
		}
		if _, err := io.ReadFull(src, ct[:ctLen]); err != nil {
			return errBackupTruncated
		}

		nonce := makeBackupNonce(nonceBase, counter)
		aad := makeBackupAAD(counter, isFinal)
		plain, err := gcm.Open(nil, nonce, ct[:ctLen], aad)
		if err != nil {
			return fmt.Errorf("backup: chunk %d: %w", counter, err)
		}
		if _, err := dst.Write(plain); err != nil {
			return err
		}

		if isFinal {
			// any remaining bytes after a final chunk indicate tampering / corruption.
			var probe [1]byte
			if _, err := src.Read(probe[:]); err == nil {
				return errBackupTrailingBytes
			} else if err != io.EOF {
				return err
			}
			break
		}

		counter++
		if counter == 0 {
			return fmt.Errorf("backup: chunk counter overflowed during decrypt")
		}
	}

	if err := dst.Close(); err != nil {
		return err
	}
	closeOK = true
	return nil
}

func newBackupGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm, nil
}

func writeBackupChunk(w io.Writer, gcm cipher.AEAD, nonceBase []byte, counter uint32, isFinal bool, plain []byte) error {
	nonce := makeBackupNonce(nonceBase, counter)
	aad := makeBackupAAD(counter, isFinal)
	ct := gcm.Seal(nil, nonce, plain, aad)
	frame := make([]byte, 5)
	if isFinal {
		frame[0] = 1
	}
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(ct)))
	if _, err := w.Write(frame); err != nil {
		return err
	}
	if _, err := w.Write(ct); err != nil {
		return err
	}
	return nil
}

func makeBackupNonce(base []byte, counter uint32) []byte {
	nonce := make([]byte, backupNonceLen)
	copy(nonce[:backupNonceBaseLen], base)
	binary.BigEndian.PutUint32(nonce[backupNonceBaseLen:], counter)
	return nonce
}

func makeBackupAAD(counter uint32, isFinal bool) []byte {
	aad := make([]byte, 5)
	binary.BigEndian.PutUint32(aad[:4], counter)
	if isFinal {
		aad[4] = 1
	}
	return aad
}
