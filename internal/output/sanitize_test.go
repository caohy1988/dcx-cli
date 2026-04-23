package output

import "testing"

func TestSanitize_PreservesNormalText(t *testing.T) {
	input := "Hello, world! This is normal text."
	if got := Sanitize(input); got != input {
		t.Errorf("Sanitize(%q) = %q, want unchanged", input, got)
	}
}

func TestSanitize_PreservesNewlinesAndTabs(t *testing.T) {
	input := "line1\nline2\ttab"
	if got := Sanitize(input); got != input {
		t.Errorf("Sanitize(%q) = %q, want unchanged", input, got)
	}
}

func TestSanitize_StripsANSIEscapes(t *testing.T) {
	input := "\x1b[31mRED\x1b[0m normal"
	want := "RED normal"
	if got := Sanitize(input); got != want {
		t.Errorf("Sanitize(%q) = %q, want %q", input, got, want)
	}
}

func TestSanitize_StripsANSIBold(t *testing.T) {
	input := "\x1b[1;33mWARNING\x1b[0m"
	want := "WARNING"
	if got := Sanitize(input); got != want {
		t.Errorf("Sanitize(%q) = %q, want %q", input, got, want)
	}
}

func TestSanitize_StripsControlChars(t *testing.T) {
	input := "before\x00\x01\x02\x03after"
	want := "beforeafter"
	if got := Sanitize(input); got != want {
		t.Errorf("Sanitize(%q) = %q, want %q", input, got, want)
	}
}

func TestSanitize_StripsBidiOverrides(t *testing.T) {
	// U+202A (LRE), U+202E (RLO)
	input := "normal\u202Areversed\u202Etext"
	want := "normalreversedtext"
	if got := Sanitize(input); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSanitize_StripsBidiIsolates(t *testing.T) {
	// U+2066 (LRI), U+2069 (PDI)
	input := "before\u2066isolated\u2069after"
	want := "beforeisolatedafter"
	if got := Sanitize(input); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSanitize_StripsZeroWidth(t *testing.T) {
	// U+200B (ZWSP), U+FEFF (BOM)
	input := "zero\u200Bwidth\uFEFFspace"
	want := "zerowidthspace"
	if got := Sanitize(input); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSanitize_PreservesUTF8(t *testing.T) {
	input := "日本語テスト émojis 🎉"
	if got := Sanitize(input); got != input {
		t.Errorf("Sanitize(%q) = %q, want unchanged", input, got)
	}
}

func TestSanitize_EmptyString(t *testing.T) {
	if got := Sanitize(""); got != "" {
		t.Errorf("Sanitize(\"\") = %q, want empty", got)
	}
}

func TestSanitize_DELCharacter(t *testing.T) {
	input := "before\x7Fafter"
	want := "beforeafter"
	if got := Sanitize(input); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
