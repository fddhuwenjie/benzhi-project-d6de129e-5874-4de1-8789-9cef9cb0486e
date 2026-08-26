package main

import "strings"

const defaultListenAddress = "127.0.0.1:19081"

func normalizeListenAddress(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return defaultListenAddress
	}
	if !strings.Contains(addr, ":") {
		return "127.0.0.1:" + addr
	}
	return addr
}
