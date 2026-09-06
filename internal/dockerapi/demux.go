// Module:       internal/dockerapi (demux)
// Purpose:      De-multiplex Docker's framed log stream. This replaces
//
//	stdcopy.StdCopy from the SDK, which was the only piece of SDK
//	logic that was not simply an HTTP GET.
//
// Dependencies: standard library only (encoding/binary).
//
// Wire format: each frame is an 8-byte header followed by its payload.
//
//	byte 0    stream id (0 stdin, 1 stdout, 2 stderr)
//	bytes 1-3 zero padding
//	bytes 4-7 payload length, big-endian uint32
//
// A container created with a TTY writes unframed plain text instead, so a
// buffer that does not look framed is returned as-is rather than dropped.
//
// @aitri-trace FR-095, AC-095-001, TC-DVH-074e
package dockerapi

import "encoding/binary"

const frameHeaderLen = 8

// Demux flattens Docker's framed stream into the combined output, preserving
// the order the daemon wrote it in. stdout and stderr are interleaved exactly
// as they arrived, which is what an operator reading a log wants.
//
// It is deliberately total: a truncated header or a payload shorter than its
// declared length yields whatever was readable rather than an error. A parse
// failure must degrade legibility, never drop the redaction step that runs
// after it.
//
// Params:
//   - b: the raw body of a logs request.
//
// Returns the de-multiplexed bytes; b unchanged when it is not framed.
func Demux(b []byte) []byte {
	if !looksFramed(b) {
		return b
	}
	out := make([]byte, 0, len(b))
	for len(b) >= frameHeaderLen {
		n := binary.BigEndian.Uint32(b[4:frameHeaderLen])
		b = b[frameHeaderLen:]
		if uint64(n) > uint64(len(b)) {
			// Payload truncated mid-frame: take what is there and stop.
			out = append(out, b...)
			return out
		}
		out = append(out, b[:n]...)
		b = b[n:]
	}
	// A trailing partial header is dropped: it carries no payload.
	return out
}

// looksFramed reports whether b starts with a plausible frame header: a known
// stream id and zeroed padding. Plain TTY output essentially never begins with
// that byte pattern.
func looksFramed(b []byte) bool {
	if len(b) < frameHeaderLen {
		return false
	}
	if b[0] > 2 {
		return false
	}
	return b[1] == 0 && b[2] == 0 && b[3] == 0
}
