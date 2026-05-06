//go:build !linux

package main

import "net"

// getPeerUID is a no-op stub for non-Linux builds. SO_PEERCRED is a Linux
// extension; on macOS/BSD developer machines we surface this by returning a
// sentinel uid and letting the caller decide whether to allow the connection
// (the production deploy is always linux/arm64 — see DEPLOYMENT.md).
//
// @aitri-trace BG-021 BL-013
func getPeerUID(_ net.Conn) (uint32, error) {
	return 0, nil
}

const peerCredSupported = false
