package ups

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"
)

// captureLogs redirects the package logger to a slice of rendered lines.
// Call BEFORE NewPoller (the poller captures the logger at construction).
func captureLogs(t *testing.T) *[]string {
	t.Helper()
	var logs []string
	orig := logger
	logger = func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }
	t.Cleanup(func() { logger = orig })
	return &logs
}

type alwaysFail struct{}

func (alwaysFail) List(context.Context) (map[string]string, error) { return nil, errors.New("down") }
func (alwaysFail) Close() error                                    { return nil }

type failThenOK struct {
	n, failN int
}

func (c *failThenOK) List(context.Context) (map[string]string, error) {
	c.n++
	if c.n <= c.failN {
		return nil, errors.New("down")
	}
	return mockVars("OL"), nil
}
func (c *failThenOK) Close() error { return nil }

func countContains(logs []string, sub string) (n int) {
	for _, l := range logs {
		if strings.Contains(l, sub) {
			n++
		}
	}
	return
}

// TC-UPS-047h (NFR-020): reconnect logging is bounded while the UPS stays down.
func TestTC_UPS_047h_BoundedReconnectLogging(t *testing.T) {
	// @aitri-tc TC-UPS-047h
	logs := captureLogs(t)
	p := NewPoller(alwaysFail{}, Config{PollInterval: time.Millisecond, UnreachableTimeout: time.Millisecond, BattLowV: 21, BattHighV: 27.4})
	for i := 0; i < 100; i++ {
		p.pollOnce(context.Background())
	}
	n := countContains(*logs, "unreachable")
	if n >= 20 {
		t.Fatalf("expected bounded reconnect logging (<20) for 100 down-polls, got %d lines", n)
	}
	if n == 0 {
		t.Fatal("expected at least one unreachable log line")
	}
}

// TC-UPS-048e (NFR-020): recovery is logged exactly once.
func TestTC_UPS_048e_RecoveryLoggedOnce(t *testing.T) {
	// @aitri-tc TC-UPS-048e
	logs := captureLogs(t)
	p := NewPoller(&failThenOK{failN: 3}, Config{PollInterval: time.Millisecond, UnreachableTimeout: time.Millisecond, BattLowV: 21, BattHighV: 27.4})
	for i := 0; i < 10; i++ { // 3 fail → unreachable, then recovers and stays up
		p.pollOnce(context.Background())
	}
	if n := countContains(*logs, "reachable again"); n != 1 {
		t.Fatalf("expected exactly 1 recovery log line, got %d: %v", n, *logs)
	}
}

// TC-UPS-049f (NFR-020): log lines carry a timestamp and the ups context.
func TestTC_UPS_049f_TimestampedLines(t *testing.T) {
	// @aitri-tc TC-UPS-049f
	logs := captureLogs(t)
	p := NewPoller(alwaysFail{}, Config{PollInterval: time.Millisecond, UnreachableTimeout: time.Millisecond, BattLowV: 21, BattHighV: 27.4})
	p.pollOnce(context.Background())

	tsRe := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`)
	found := false
	for _, l := range *logs {
		if tsRe.MatchString(l) && strings.Contains(l, "ups") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a timestamped 'ups' log line, got %v", *logs)
	}
}
