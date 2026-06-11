//go:build linux

package proxy

import (
	"net"
	"syscall"
)

const soBindToDevice = 25 // SO_BINDTODEVICE on Linux

func applyBind(dialer *net.Dialer, bindIP, bindIface string) {
	if bindIface != "" {
		dialer.Control = func(network, address string, c syscall.RawConn) error {
			var sockErr error
			err := c.Control(func(fd uintptr) {
				sockErr = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, soBindToDevice, bindIface)
			})
			if err != nil {
				return err
			}
			return sockErr
		}
	} else if bindIP != "" {
		dialer.LocalAddr = &net.TCPAddr{IP: net.ParseIP(bindIP)}
	}
}
