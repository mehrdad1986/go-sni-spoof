//go:build windows

package spoof

import (
	"syscall"
)

func setSockOptTTL(fd uintptr, ttl int) error {
	return syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, syscall.IP_TTL, ttl)
}

