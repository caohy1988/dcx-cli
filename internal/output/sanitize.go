package output

import "unicode"

// Sanitize strips dangerous characters from untrusted text before
// rendering to the terminal. Applied at render time only — never
// mutates underlying data structures.
//
// Strips:
//   - ASCII control characters except \n and \t (preserves readability)
//   - ANSI escape sequences (prevents terminal injection via \x1b[...)
//   - Bidi overrides (U+202A-U+202E, U+2066-U+2069)
//   - Zero-width characters (U+200B-U+200F, U+FEFF)
//
// JSON and json-minified output is NOT sanitized — it is
// machine-consumed and must preserve raw API values.
func Sanitize(s string) string {
	result := make([]byte, 0, len(s))
	inEscape := false

	for i := 0; i < len(s); i++ {
		b := s[i]

		// Track ANSI escape sequences: \x1b[ ... final byte
		if b == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			inEscape = true
			i++ // skip '['
			continue
		}
		if inEscape {
			// ANSI CSI sequences end with a letter (0x40-0x7E)
			if b >= 0x40 && b <= 0x7E {
				inEscape = false
			}
			continue
		}

		// Preserve newlines and tabs.
		if b == '\n' || b == '\t' {
			result = append(result, b)
			continue
		}

		// Strip ASCII control characters.
		if b < 0x20 || b == 0x7F {
			continue
		}

		// For multi-byte UTF-8, check for dangerous Unicode.
		if b >= 0x80 {
			r, size := decodeRune(s[i:])
			if isDangerousUnicode(r) {
				i += size - 1
				continue
			}
			result = append(result, s[i:i+size]...)
			i += size - 1
			continue
		}

		result = append(result, b)
	}

	return string(result)
}

// isDangerousUnicode returns true for characters that should be stripped
// from terminal output.
func isDangerousUnicode(r rune) bool {
	// Bidi overrides: U+202A-U+202E
	if r >= 0x202A && r <= 0x202E {
		return true
	}
	// Bidi isolates: U+2066-U+2069
	if r >= 0x2066 && r <= 0x2069 {
		return true
	}
	// Zero-width characters
	switch r {
	case 0x200B, 0x200C, 0x200D, 0x200E, 0x200F, 0xFEFF:
		return true
	}
	// Other control characters
	if unicode.IsControl(r) && r != '\n' && r != '\t' {
		return true
	}
	return false
}

// decodeRune decodes a single UTF-8 rune from s.
// Returns the rune and its byte length.
func decodeRune(s string) (rune, int) {
	if len(s) == 0 {
		return 0, 0
	}
	b := s[0]
	switch {
	case b < 0xC0:
		return rune(b), 1
	case b < 0xE0:
		if len(s) < 2 {
			return unicode.ReplacementChar, 1
		}
		return rune(b&0x1F)<<6 | rune(s[1]&0x3F), 2
	case b < 0xF0:
		if len(s) < 3 {
			return unicode.ReplacementChar, 1
		}
		return rune(b&0x0F)<<12 | rune(s[1]&0x3F)<<6 | rune(s[2]&0x3F), 3
	default:
		if len(s) < 4 {
			return unicode.ReplacementChar, 1
		}
		return rune(b&0x07)<<18 | rune(s[1]&0x3F)<<12 | rune(s[2]&0x3F)<<6 | rune(s[3]&0x3F), 4
	}
}
