package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

const blockSize = 4 * 1024 * 1024

// DropboxContentHash implements Dropbox's official content_hash algorithm:
// SHA256(concat(SHA256(each 4MiB block))).
func DropboxContentHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil { return "", err }
	defer f.Close()
	outer := sha256.New()
	buf := make([]byte, blockSize)
	for {
		n, err := io.ReadFull(f, buf)
		if err == io.EOF { break }
		if err == io.ErrUnexpectedEOF {
			inner := sha256.Sum256(buf[:n])
			outer.Write(inner[:])
			break
		}
		if err != nil { return "", err }
		inner := sha256.Sum256(buf[:n])
		outer.Write(inner[:])
	}
	return hex.EncodeToString(outer.Sum(nil)), nil
}
