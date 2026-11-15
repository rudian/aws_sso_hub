package toolinstall

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"runtime"
)

func goos() string { return runtime.GOOS }

func fileSHA256(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(f, MaxPkgBytes+1)); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}
