//go:build windows

package netx

import (
	"context"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// systemConfig reads the WinINET settings of the user (what browsers use) and
// falls back to the WinHTTP default. It starts no child process, so no console
// window flashes and localized tool output is no issue.
func systemConfig(context.Context) (Config, bool) {
	if c, ok := wininetConfig(); ok && (!c.Empty() || c.PAC != "") {
		return c, true
	}
	if c, ok := winhttpConfig(); ok {
		return c, true
	}
	return Config{}, false
}

func wininetConfig() (Config, bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return Config{}, false
	}
	defer k.Close()
	enable, _, _ := k.GetIntegerValue("ProxyEnable")
	server, _, _ := k.GetStringValue("ProxyServer")
	override, _, _ := k.GetStringValue("ProxyOverride")
	pac, _, _ := k.GetStringValue("AutoConfigURL")
	return WinInternetConfig(enable == 1, server, override, pac), true
}

type winhttpProxyInfo struct {
	accessType  uint32
	proxy       *uint16
	proxyBypass *uint16
}

func winhttpConfig() (Config, bool) {
	dll := windows.NewLazySystemDLL("winhttp.dll")
	get := dll.NewProc("WinHttpGetDefaultProxyConfiguration")
	if get.Find() != nil {
		return Config{}, false
	}
	var info winhttpProxyInfo
	if r, _, _ := get.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		return Config{}, false
	}
	free := windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalFree")
	for _, p := range []*uint16{info.proxy, info.proxyBypass} {
		if p != nil {
			defer free.Call(uintptr(unsafe.Pointer(p)))
		}
	}
	const accessNamedProxy = 3
	if info.accessType != accessNamedProxy {
		return Config{}, false
	}
	return WinHTTPConfig(windows.UTF16PtrToString(info.proxy), windows.UTF16PtrToString(info.proxyBypass)), true
}
