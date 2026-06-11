//go:build !linux && !windows

package proxy

import "net"

func applyBind(dialer *net.Dialer, bindIP, bindIface string) {
	if bindIP != "" {
		dialer.LocalAddr = &net.TCPAddr{IP: net.ParseIP(bindIP)}
	}
}
