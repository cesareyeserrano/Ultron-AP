package markdown

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestTC_TMU_025h covers FR-025 / AC-025-001: a service named foo_bar.service
// must escape the underscore and the dot so MarkdownV2 parses cleanly.
//
// @aitri-trace FR-025 US-025 AC-025-001 TC-TMU-025h
func TestTC_TMU_025h_ServiceNameUnderscoreAndDotEscaped(t *testing.T) {
	got := EscapeV2("foo_bar.service")
	want := `foo\_bar\.service`
	if got != want {
		t.Fatalf("EscapeV2(%q) = %q; want %q", "foo_bar.service", got, want)
	}
}

// TestTC_TMU_025e covers FR-025 / AC-025-002: a journal line containing
// *danger* must escape the asterisks so MarkdownV2 does not render them as
// bold.
//
// @aitri-trace FR-025 US-025 AC-025-002 TC-TMU-025e
func TestTC_TMU_025e_AsterisksInJournalLineEscaped(t *testing.T) {
	got := EscapeV2("*danger*")
	want := `\*danger\*`
	if got != want {
		t.Fatalf("EscapeV2(%q) = %q; want %q", "*danger*", got, want)
	}
	if strings.Contains(got, "**") {
		t.Fatalf("escaped output unexpectedly contains balanced asterisks: %q", got)
	}
}

// TestTC_TMU_025f confirms every special char in the MarkdownV2 set is
// escaped exactly once. This is the security floor: a missed character
// causes Telegram to reject the message with 400 "can't parse entities".
//
// @aitri-trace FR-025 US-025 AC-025-001 TC-TMU-025f-set
func TestTC_TMU_025f_AllSpecialCharsEscapedExactlyOnce(t *testing.T) {
	specials := "_*[]()~`>#+-=|{}.!"
	got := EscapeV2(specials)
	// Every special char should be preceded by exactly one backslash.
	for i, r := range specials {
		want := "\\" + string(r)
		if !strings.Contains(got, want) {
			t.Errorf("special char %q (idx %d) not escaped in output %q", r, i, got)
		}
	}
	// No double-escaping.
	if strings.Contains(got, `\\`) {
		t.Fatalf("output contains double-escape sequence: %q", got)
	}
}

// TestTC_TMU_025_DotInHostnameEscaped checks the hostname.local case from
// FR-025 / AC-025-003: a host like pi-1.local must escape the dot AND the
// hyphen.
//
// @aitri-trace FR-025 US-025 AC-025-001 TC-TMU-025-host
func TestTC_TMU_025_DotInHostnameEscaped(t *testing.T) {
	got := EscapeV2("pi-1.local")
	want := `pi\-1\.local`
	if got != want {
		t.Fatalf("EscapeV2(%q) = %q; want %q", "pi-1.local", got, want)
	}
}

// TestTC_TMU_025_EmptyAndPlainPassthrough covers boundary inputs: empty string
// and a string with no special characters must round-trip unchanged.
//
// @aitri-trace FR-025 TC-TMU-025-bounds
func TestTC_TMU_025_EmptyAndPlainPassthrough(t *testing.T) {
	if got := EscapeV2(""); got != "" {
		t.Fatalf("EscapeV2(\"\") = %q; want empty", got)
	}
	plain := "abc 123 XYZ"
	if got := EscapeV2(plain); got != plain {
		t.Fatalf("EscapeV2(%q) = %q; want %q (no special chars)", plain, got, plain)
	}
}

// TestTC_TMU_025_UTF8RoundTrip confirms multi-byte runes survive untouched.
// MarkdownV2 has no specials in the BMP outside ASCII; an emoji or accented
// char must not be split or escaped.
//
// @aitri-trace FR-025 TC-TMU-025-utf8
func TestTC_TMU_025_UTF8RoundTrip(t *testing.T) {
	in := "café 🔴 niño"
	got := EscapeV2(in)
	if got != in {
		t.Fatalf("EscapeV2(%q) = %q; want unchanged", in, got)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("EscapeV2 produced invalid UTF-8: %q", got)
	}
}

// TestTC_TMU_025z is a smaller-than-fuzz table covering ASCII printable bytes
// 0x20..0x7E. Every input must produce output where the only difference from
// the input is a backslash inserted before each MarkdownV2 special. This is a
// proxy for FR-025 fuzz coverage; a real -fuzz run lives alongside via
// FuzzEscapeV2 below.
//
// @aitri-trace FR-025 TC-TMU-025z-ascii-table
func TestTC_TMU_025z_AsciiPrintableTable(t *testing.T) {
	for r := byte(0x20); r <= 0x7E; r++ {
		in := string(r)
		got := EscapeV2(in)
		isSpecial := int(r) < len(telegramSpecialV2) && telegramSpecialV2[r]
		if isSpecial {
			want := "\\" + in
			if got != want {
				t.Errorf("EscapeV2(%q) = %q; want %q (special)", in, got, want)
			}
		} else {
			if got != in {
				t.Errorf("EscapeV2(%q) = %q; want %q (non-special)", in, got, in)
			}
		}
	}
}

// FuzzEscapeV2 — a permanent fuzz target for FR-025. Run with:
//
//	go test ./internal/notify/markdown -fuzz=FuzzEscapeV2 -fuzztime=30s
//
// Property: the escaped output must, when every '\X' pair is collapsed back
// to 'X', equal the original input. (Round-trip via the canonical un-escape.)
func FuzzEscapeV2(f *testing.F) {
	for _, seed := range []string{
		"", "abc", "_", "*", "[]", "(){}", "pi-1.local", "café",
		"_*[]()~`>#+-=|{}.!", "mixed_with*specials.AND-text!",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		got := EscapeV2(in)
		// Reverse: drop a backslash whenever it precedes a special char.
		var b strings.Builder
		runes := []rune(got)
		for i := 0; i < len(runes); i++ {
			r := runes[i]
			if r == '\\' && i+1 < len(runes) {
				next := runes[i+1]
				if int(next) < len(telegramSpecialV2) && telegramSpecialV2[next] {
					b.WriteRune(next)
					i++
					continue
				}
			}
			b.WriteRune(r)
		}
		if b.String() != in {
			t.Fatalf("round-trip failed: in=%q escaped=%q reversed=%q", in, got, b.String())
		}
	})
}
