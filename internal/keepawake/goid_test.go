package keepawake

import (
	"runtime"
	"strings"
)

// goroutineID reads the current goroutine's number from its stack header ("goroutine 12 [running]:").
func goroutineID() string {
	var b [64]byte
	s := string(b[:runtime.Stack(b[:], false)])
	s = strings.TrimPrefix(s, "goroutine ")
	return s[:strings.IndexByte(s, ' ')]
}
