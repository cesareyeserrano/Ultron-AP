// Package markdown implements escaping for Telegram MarkdownV2 message bodies.
//
// Trusted developer-authored substrings (template literals) MUST NOT be passed
// through EscapeV2 because that would defeat the formatting; only untrusted
// user/environment-controlled fields (hostnames, unit names, container names,
// journal/log lines, image references, process names) are escaped.
//
// Reference: https://core.telegram.org/bots/api#markdownv2-style
package markdown

import "strings"

// telegramSpecialV2 lists every character Telegram's MarkdownV2 grammar treats
// as a formatting marker. Any of these inside an interpolated value must be
// preceded with a backslash to render as a literal.
var telegramSpecialV2 = [...]bool{
	'_': true, '*': true, '[': true, ']': true,
	'(': true, ')': true, '~': true, '`': true,
	'>': true, '#': true, '+': true, '-': true,
	'=': true, '|': true, '{': true, '}': true,
	'.': true, '!': true,
}

// EscapeV2 returns s with every Telegram MarkdownV2 special character preceded
// by a backslash. Pass only untrusted fields through this helper — never pass
// developer-authored template fragments, since that would prevent intended
// formatting (bold, link, code) from rendering.
func EscapeV2(s string) string {
	if s == "" {
		return ""
	}
	// Worst case: every byte is special and gets a backslash prepended.
	var b strings.Builder
	b.Grow(len(s) + len(s)/4)
	for _, r := range s {
		if r >= 0 && int(r) < len(telegramSpecialV2) && telegramSpecialV2[r] {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
