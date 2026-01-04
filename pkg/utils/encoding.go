package utils

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

const scowEncPrefix = "scow-enc-"

// CustomEscape escapes non-UTF8 bytes and the '%' character in a string.
//
// Principle:
// It iterates over the string rune by rune.
// 1. If it encounters a valid UTF-8 rune that is NOT '%', it writes it as is.
// 2. If it encounters the '%' character, it escapes it as "%25" to avoid ambiguity with escape sequences.
// 3. If it encounters an invalid UTF-8 byte (RuneError with width 1), it escapes it as "%XX" (hexadecimal representation).
//
// This ensures that the resulting string is always a valid UTF-8 string, with invalid bytes preserved in an escaped format.
func CustomEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, width := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && width == 1 {
			// Invalid byte
			fmt.Fprintf(&b, "%%%02X", s[i])
			i++
			continue
		}
		// Valid rune
		if r == '%' {
			b.WriteString("%25")
		} else {
			b.WriteRune(r)
		}
		i += width
	}
	return b.String()
}

// CustomUnescape unescapes a string escaped by CustomEscape.
//
// Principle:
// It iterates over the string byte by byte.
// 1. If it encounters a '%' character followed by two hex digits (e.g., "%XX"), it parses the hex value and writes the corresponding byte.
// 2. If the '%' is not followed by valid hex digits, or if it's just a regular character, it writes the byte as is.
//
// This reverses the CustomEscape process, restoring original non-UTF8 bytes and '%' characters.
func CustomUnescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '%' && i+2 < len(s) {
			val, err := strconv.ParseInt(s[i+1:i+3], 16, 32)
			if err == nil {
				b.WriteByte(byte(val))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// EncodeNonUtf8Name encodes a filename if it contains non-UTF8 characters or starts with the specific prefix.
// It uses CustomEscape to handle invalid bytes and adds a prefix to identify encoded names.
func EncodeNonUtf8Name(name string) string {
	if utf8.ValidString(name) && !strings.HasPrefix(name, scowEncPrefix) {
		return name
	}
	return scowEncPrefix + CustomEscape(name)
}

// DecodePath decodes a path where some parts might have been encoded by EncodeNonUtf8Name.
// It splits the path by '/' (Linux separator), checks each part for the prefix, and unescapes if necessary.
func DecodePath(path string) string {
	decodePart := func(part string) string {
		if strings.HasPrefix(part, scowEncPrefix) {
			encoded := strings.TrimPrefix(part, scowEncPrefix)
			return CustomUnescape(encoded)
		}
		return part
	}

	// Split by / (Linux separator)
	parts := strings.Split(path, "/")
	for i, part := range parts {
		parts[i] = decodePart(part)
	}
	return strings.Join(parts, "/")
}
