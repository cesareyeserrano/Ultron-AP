// Module:       internal/ups
// Purpose:      Read-only NUT protocol client over TCP (FR-016, RS-1/RS-2).
// Dependencies: standard library only (net, bufio) — NO os/exec, NO privileged helper.
package ups

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// nutDeadline bounds a single poll's socket I/O so a stuck upsd cannot hang the
// poller.
const nutDeadline = 5 * time.Second

// Client reads UPS variables. It is deliberately READ-ONLY: there is no method
// that could issue a SET, INSTCMD, shutdown or load.off command (NFR-018). The
// only implementations are the real TCP client and the dev/test mock.
type Client interface {
	// List returns the current UPS variable map (LIST VAR), or an error when
	// the UPS/NUT server is unreachable.
	List(ctx context.Context) (map[string]string, error)
	// Close releases any resources. Safe to call on an idle client.
	Close() error
}

// tcpClient speaks the NUT protocol to a real upsd. Each List opens a short-
// lived TCP connection (ADR-01: "one short-lived TCP request per poll"), so
// there is no persistent socket to leak and reconnection is implicit.
type tcpClient struct {
	cfg Config
}

// NewClient returns the real TCP client, or the mock client when cfg.Mock is
// set (dev only — never set in the production unit, NFR-022).
func NewClient(cfg Config) Client {
	if cfg.Mock != "" {
		return newMockClient(cfg.Mock)
	}
	return &tcpClient{cfg: cfg}
}

// List dials the NUT server, optionally authenticates with the read-only user,
// issues LIST VAR and parses the reply into a variable map.
//
// ctx: cancels the dial and I/O. Returns the variable map or a connection/
// protocol error (which the poller turns into the "unreachable" state).
// @aitri-trace FR-016 US-016 AC-016-001 TC-UPS-001h
func (c *tcpClient) List(ctx context.Context) (map[string]string, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", c.cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("ups: dial %s: %w", c.cfg.Addr, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(nutDeadline))

	r := bufio.NewReader(conn)

	// Optional authentication with the dedicated read-only NUT user (RS-1).
	if c.cfg.User != "" {
		if err := authCmd(conn, r, "USERNAME "+c.cfg.User); err != nil {
			return nil, err
		}
		if err := authCmd(conn, r, "PASSWORD "+c.cfg.Pass); err != nil {
			return nil, err
		}
	}

	if _, err := fmt.Fprintf(conn, "LIST VAR %s\n", c.cfg.UPSName); err != nil {
		return nil, fmt.Errorf("ups: write LIST VAR: %w", err)
	}
	return parseListVar(r, c.cfg.UPSName)
}

// Close is a no-op: List uses short-lived connections.
func (c *tcpClient) Close() error { return nil }

// authCmd sends a single auth command and requires an "OK" response. Only
// USERNAME/PASSWORD go through here — never a state-changing command.
func authCmd(conn net.Conn, r *bufio.Reader, cmd string) error {
	if _, err := fmt.Fprintf(conn, "%s\n", cmd); err != nil {
		return fmt.Errorf("ups: write auth: %w", err)
	}
	line, err := r.ReadString('\n')
	if err != nil {
		return fmt.Errorf("ups: read auth response: %w", err)
	}
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "OK") {
		return fmt.Errorf("ups: auth rejected: %s", line)
	}
	return nil
}

// parseListVar reads a NUT "LIST VAR" response into a map[var]value.
//
// The expected wire form is:
//
//	BEGIN LIST VAR <ups>
//	VAR <ups> <name> "<value>"
//	...
//	END LIST VAR <ups>
//
// An "ERR ..." line is returned as an error. Values are NUT-unquoted.
func parseListVar(r *bufio.Reader, upsName string) (map[string]string, error) {
	vars := map[string]string{}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("ups: read LIST VAR: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, "ERR"):
			return nil, fmt.Errorf("ups: server error: %s", strings.TrimSpace(line))
		case strings.HasPrefix(line, "BEGIN LIST VAR"):
			continue
		case strings.HasPrefix(line, "END LIST VAR"):
			return vars, nil
		case strings.HasPrefix(line, "VAR "):
			name, value, ok := parseVarLine(line, upsName)
			if ok {
				vars[name] = value
			}
		default:
			// Ignore unrecognised lines defensively rather than failing the poll.
		}
	}
}

// parseVarLine parses one 'VAR <ups> <name> "<value>"' line. Returns the
// variable name, its unquoted value, and ok=false when the line is malformed.
func parseVarLine(line, upsName string) (name, value string, ok bool) {
	// Split into: VAR, <ups>, <name>, "<value...>"
	parts := strings.SplitN(line, " ", 4)
	if len(parts) < 4 || parts[0] != "VAR" || parts[1] != upsName {
		return "", "", false
	}
	return parts[2], nutUnquote(parts[3]), true
}

// nutUnquote strips the surrounding quotes from a NUT value and unescapes the
// backslash-escaped quote and backslash sequences NUT uses.
func nutUnquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\\`, `\`)
	return s
}
