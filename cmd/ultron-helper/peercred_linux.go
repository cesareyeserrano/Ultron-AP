//go:build linux

package main

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// getPeerUID returns the UID of the process at the other end of a Unix
// socket connection via the SO_PEERCRED socket option. The lookup happens
// at the kernel level — the caller cannot spoof it — so this is a hard
// authentication step on top of the socket's filesystem permissions.
//
// @aitri-trace BG-021 BL-013
func getPeerUID(c net.Conn) (uint32, error) {
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return 0, fmt.Errorf("peercred: connection is not a unix socket (%T)", c)
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return 0, fmt.Errorf("peercred: SyscallConn: %w", err)
	}
	var (
		ucred   *unix.Ucred
		credErr error
	)
	ctlErr := raw.Control(func(fd uintptr) {
		ucred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if ctlErr != nil {
		return 0, fmt.Errorf("peercred: control: %w", ctlErr)
	}
	if credErr != nil {
		return 0, fmt.Errorf("peercred: getsockopt: %w", credErr)
	}
	return ucred.Uid, nil
}

// peerCredSupported reports that SO_PEERCRED enforcement is compiled in for
// this build. On Linux this is always true; the non-linux stub returns false
// so callers can degrade gracefully (warn-and-allow) on developer machines.
const peerCredSupported = true
