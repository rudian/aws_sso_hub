//go:build darwin

package update

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <string.h>

static CFMutableDictionaryRef kc_query(const char *service, const char *account) {
	CFMutableDictionaryRef q = CFDictionaryCreateMutable(NULL, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFStringRef s = CFStringCreateWithCString(NULL, service, kCFStringEncodingUTF8);
	CFStringRef a = CFStringCreateWithCString(NULL, account, kCFStringEncodingUTF8);
	CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
	CFDictionarySetValue(q, kSecAttrService, s);
	CFDictionarySetValue(q, kSecAttrAccount, a);
	CFRelease(s);
	CFRelease(a);
	return q;
}

// kc_set adds the item, or replaces an existing one (delete, then add). The item is created by this process, so its access
// list trusts this application's signature.
static OSStatus kc_set(const char *service, const char *account, const void *data, int len) {
	CFDataRef d = CFDataCreate(NULL, (const UInt8 *)data, len);
	CFMutableDictionaryRef q = kc_query(service, account);
	CFMutableDictionaryRef add = CFDictionaryCreateMutableCopy(NULL, 0, q);
	CFDictionarySetValue(add, kSecValueData, d);
	OSStatus st = SecItemAdd(add, NULL);
	if (st == errSecDuplicateItem) {
		// Never update in place: the existing item may have been created by another process with its own access
		// list. Delete it and add a fresh one, so the access list is always the one this process creates.
		st = SecItemDelete(q);
		if (st == errSecSuccess || st == errSecItemNotFound) {
			st = SecItemAdd(add, NULL);
		}
	}
	CFRelease(add);
	CFRelease(q);
	CFRelease(d);
	return st;
}

// kc_get returns the secret in a malloc'd buffer the caller frees.
static OSStatus kc_get(const char *service, const char *account, void **out, int *outlen) {
	CFMutableDictionaryRef q = kc_query(service, account);
	CFDictionarySetValue(q, kSecReturnData, kCFBooleanTrue);
	CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
	CFTypeRef res = NULL;
	OSStatus st = SecItemCopyMatching(q, &res);
	CFRelease(q);
	if (st == errSecSuccess && res != NULL) {
		CFDataRef d = (CFDataRef)res;
		CFIndex n = CFDataGetLength(d);
		void *buf = malloc(n > 0 ? n : 1);
		memcpy(buf, CFDataGetBytePtr(d), n);
		*out = buf;
		*outlen = (int)n;
		CFRelease(res);
	}
	return st;
}

static OSStatus kc_delete(const char *service, const char *account) {
	CFMutableDictionaryRef q = kc_query(service, account);
	OSStatus st = SecItemDelete(q);
	CFRelease(q);
	return st;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type keychainBackend struct{ service, account string }

func platformBackend(service, account string) secretBackend {
	return keychainBackend{service: service, account: account}
}

func (k keychainBackend) names() (*C.char, *C.char) {
	return C.CString(k.service), C.CString(k.account)
}

func (k keychainBackend) read() ([]byte, bool, error) {
	s, a := k.names()
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(a))
	var out unsafe.Pointer
	var n C.int
	st := C.kc_get(s, a, &out, &n)
	switch st {
	case C.errSecSuccess:
		defer C.free(out)
		return C.GoBytes(out, n), true, nil
	case C.errSecItemNotFound:
		return nil, false, nil
	}
	return nil, false, fmt.Errorf("keychain read failed (status %d)", int(st))
}

func (k keychainBackend) write(secret []byte) error {
	s, a := k.names()
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(a))
	var p unsafe.Pointer
	if len(secret) > 0 {
		p = unsafe.Pointer(&secret[0])
	}
	if st := C.kc_set(s, a, p, C.int(len(secret))); st != C.errSecSuccess {
		return fmt.Errorf("keychain write failed (status %d)", int(st))
	}
	return nil
}

func (k keychainBackend) remove() error {
	s, a := k.names()
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(a))
	if st := C.kc_delete(s, a); st != C.errSecSuccess && st != C.errSecItemNotFound {
		return fmt.Errorf("keychain delete failed (status %d)", int(st))
	}
	return nil
}
