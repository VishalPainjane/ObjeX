package auth

import (
	"strings"
	"unicode/utf8"
)

// UriEncodeStrict encodes per RFC 3986 unreserved: A-Z a-z 0-9 - _ . ~
func UriEncodeStrict(value string) string {
	var b strings.Builder
	for _, c := range value {
		if isUnreserved(c) {
			b.WriteRune(c)
		} else {
			buf := make([]byte, 4)
			n := utf8.EncodeRune(buf, c)
			for i := 0; i < n; i++ {
				b.WriteString("%")
				b.WriteString(hexUpper(buf[i]))
			}
		}
	}
	return b.String()
}

func isUnreserved(c rune) bool {
	return (c >= 'A' && c <= 'Z') ||
		(c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_' || c == '.' || c == '~'
}

func hexUpper(b byte) string {
	const hextable = "0123456789ABCDEF"
	return string([]byte{hextable[b>>4], hextable[b&0x0f]})
}

// CollapseSpaces collapses runs of whitespace to a single space (AWS Trimall).
func CollapseSpaces(value string) string {
	if !strings.Contains(value, "  ") {
		return value
	}
	var result strings.Builder
	prevSpace := false
	for _, c := range value {
		if c == ' ' {
			if !prevSpace {
				result.WriteRune(c)
			}
			prevSpace = true
		} else {
			result.WriteRune(c)
			prevSpace = false
		}
	}
	return result.String()
}
