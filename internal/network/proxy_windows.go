package network

import (
	"syscall"
	"unsafe"
)

var (
	winhttp      = syscall.NewLazyDLL("winhttp.dll")
	getUserProxy = winhttp.NewProc("WinHttpGetIEProxyConfigForCurrentUser")
	globalFree   = syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalFree")
)

type userProxyConfig struct {
	AutoDetect                   int32
	AutoConfigURL, Proxy, Bypass *uint16
}

// This API reads the active user's settings without changing them. The three
// returned strings must be released with GlobalFree, as documented by WinHTTP.
// https://learn.microsoft.com/windows/win32/api/winhttp/nf-winhttp-winhttpgetieproxyconfigforcurrentuser
func systemProxy() (string, string) {
	var config userProxyConfig
	ok, _, _ := getUserProxy.Call(uintptr(unsafe.Pointer(&config)))
	defer func() {
		for _, p := range []*uint16{config.AutoConfigURL, config.Proxy, config.Bypass} {
			if p != nil {
				_, _, _ = globalFree.Call(uintptr(unsafe.Pointer(p)))
			}
		}
	}()
	if ok == 0 {
		return "", ""
	}
	return proxyString(config.Proxy), proxyString(config.Bypass)
}

func proxyString(p *uint16) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*uint16)(unsafe.Add(unsafe.Pointer(p), uintptr(n)*2)) != 0 {
		n++
	}
	return syscall.UTF16ToString(unsafe.Slice(p, n))
}
