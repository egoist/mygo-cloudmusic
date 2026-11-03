//go:build !windows

package network

func systemProxy() (string, string) { return "", "" }
