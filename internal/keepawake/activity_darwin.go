//go:build darwin && cgo

package keepawake

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Foundation
#include <stdint.h>
#include <stdlib.h>

uint64_t keepawake_begin(const char *reason);
void keepawake_end(uint64_t handle);
*/
import "C"

import (
	"errors"
	"unsafe"
)

type nsActivity struct{}

// NewActivityAPI is the real NSProcessInfo activity.
func NewActivityAPI() ActivityAPI { return nsActivity{} }

func (nsActivity) Begin(reason string) (uint64, error) {
	cs := C.CString(reason)
	defer C.free(unsafe.Pointer(cs))
	h := uint64(C.keepawake_begin(cs))
	if h == 0 {
		return 0, errors.New("NSProcessInfo refused the activity")
	}
	return h, nil
}

func (nsActivity) End(h uint64) { C.keepawake_end(C.uint64_t(h)) }
