//go:build windows

package proxy

import (
	"net"
)

func applyBind(dialer *net.Dialer, bindIP, bindIface string) {
	if bindIP != "" {
		dialer.LocalAddr = &net.TCPAddr{IP: net.ParseIP(bindIP)}
	}
	// Windows doesn't easily support binding to interface by name.
	// Use bind_ip instead.
	if bindIface != "" {
		// Fall back to bind_ip if set or just ignore; interface binding 
		// on Windows requires LSP/WFP which is beyond this scope.
		// Logging would be nice but we don't have a logger here.
	}
}
