package edifact

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Charset is a syntax level this package reads and writes, named by the syntax
// identifier in UNB. Only the package's own values exist: UNOA, UNOB, UNOC and
// UNOW.
type Charset struct{ id string }

var (
	// UNOA is level A: upper case letters, digits, space and some punctuation.
	UNOA = Charset{"UNOA"}
	// UNOB is level B: level A and lower case letters.
	UNOB = Charset{"UNOB"}
	// UNOC is level C, the ISO 8859-1 character set.
	UNOC = Charset{"UNOC"}
	// UNOW is level W, UTF-8. It needs syntax version 4.
	UNOW = Charset{"UNOW"}
)

// String returns the syntax identifier, such as "UNOA", or "" for the zero
// Charset.
func (c Charset) String() string { return c.id }

// charsetNamed returns the Charset whose syntax identifier is id.
func charsetNamed(id string) (Charset, bool) {
	for _, c := range []Charset{UNOA, UNOB, UNOC, UNOW} {
		if c.id == id {
			return c, true
		}
	}
	return Charset{}, false
}

// ErrCharset reports a value holding a character outside the interchange's
// charset.
var ErrCharset = errors.New("edifact: character outside the charset")

// levelAPunctuation is the punctuation in level A and level B, from
// ISO 9735:1988 clause 5. The four reserved as default service characters are
// included, since Write releases them where they clash.
const levelAPunctuation = " .,-()/='+:?!\"%&*;<>"

// allows reports whether r is in the charset.
func (c Charset) allows(r rune) bool {
	switch c {
	case UNOA:
		return r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune(levelAPunctuation, r)
	case UNOB:
		return r >= 'a' && r <= 'z' || UNOA.allows(r)
	case UNOC:
		return r >= 0x20 && r <= 0x7e || r >= 0xa0 && r <= 0xff
	case UNOW:
		return true
	}
	return true
}

// encode returns v as the charset writes it: unchanged for UNOA, UNOB and
// UNOW, and as ISO 8859-1 bytes for UNOC. It reports the first character
// outside the charset, or invalid UTF-8.
func (c Charset) encode(v string) (string, error) {
	if c == (Charset{}) {
		return v, nil
	}
	if !utf8.ValidString(v) {
		return "", fmt.Errorf("%w: %q is not valid UTF-8", ErrCharset, v)
	}
	for _, r := range v {
		if !c.allows(r) {
			return "", fmt.Errorf("%w: %q (U+%04X) is not in %s", ErrCharset, r, r, c)
		}
	}
	if c != UNOC {
		return v, nil
	}
	b := make([]byte, 0, len(v))
	for _, r := range v {
		b = append(b, byte(r))
	}
	return string(b), nil
}
