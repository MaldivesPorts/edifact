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
// UNOW, and as ISO 8859-1 bytes for UNOC. With fit, characters outside the
// charset are first converted where fit knows how. It reports the first
// character still outside the charset, or invalid UTF-8.
func (c Charset) encode(v string, fit bool) (string, error) {
	if c == (Charset{}) {
		return v, nil
	}
	if !utf8.ValidString(v) {
		return "", fmt.Errorf("%w: %q is not valid UTF-8", ErrCharset, v)
	}
	if fit {
		v = c.fit(v)
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

// fit converts the characters of v that are outside the charset where it can:
// an accented Latin-1 letter to its base letter, and for UNOA a lower case
// letter to upper case. Other characters are left for encode to refuse.
func (c Charset) fit(v string) string {
	var sb strings.Builder
	for _, r := range v {
		if c.allows(r) {
			sb.WriteRune(r)
			continue
		}
		s := string(r)
		if base, ok := latin1Base[r]; ok {
			s = base
		}
		if c == UNOA {
			s = strings.ToUpper(s)
		}
		sb.WriteString(s)
	}
	return sb.String()
}

// latin1Base maps each ISO 8859-1 letter from 0xC0 to 0xFF to its base
// letters.
var latin1Base = map[rune]string{
	'À': "A", 'Á': "A", 'Â': "A", 'Ã': "A", 'Ä': "A", 'Å': "A", 'Æ': "AE",
	'Ç': "C", 'È': "E", 'É': "E", 'Ê': "E", 'Ë': "E",
	'Ì': "I", 'Í': "I", 'Î': "I", 'Ï': "I", 'Ð': "D", 'Ñ': "N",
	'Ò': "O", 'Ó': "O", 'Ô': "O", 'Õ': "O", 'Ö': "O", 'Ø': "O",
	'Ù': "U", 'Ú': "U", 'Û': "U", 'Ü': "U", 'Ý': "Y", 'Þ': "TH", 'ß': "ss",
	'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a", 'æ': "ae",
	'ç': "c", 'è': "e", 'é': "e", 'ê': "e", 'ë': "e",
	'ì': "i", 'í': "i", 'î': "i", 'ï': "i", 'ð': "d", 'ñ': "n",
	'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o", 'ø': "o",
	'ù': "u", 'ú': "u", 'û': "u", 'ü': "u", 'ý': "y", 'þ': "th", 'ÿ': "y",
}

// decode returns v as read in the charset, decoded from ISO 8859-1 for UNOC,
// and a description of the first character outside the charset, or "" when
// there is none.
func (c Charset) decode(v string) (string, string) {
	switch c {
	case UNOA, UNOB:
		for i := 0; i < len(v); i++ {
			if b := v[i]; b >= 0x80 {
				return v, fmt.Sprintf("byte 0x%02X is not in %s", b, c)
			} else if !c.allows(rune(b)) {
				return v, fmt.Sprintf("%q (U+%04X) is not in %s", rune(b), b, c)
			}
		}
	case UNOC:
		var sb strings.Builder
		problem := ""
		for i := 0; i < len(v); i++ {
			r := rune(v[i])
			if problem == "" && !c.allows(r) {
				problem = fmt.Sprintf("byte 0x%02X is not in %s", v[i], c)
			}
			sb.WriteRune(r)
		}
		return sb.String(), problem
	case UNOW:
		if !utf8.ValidString(v) {
			return v, "value is not valid UTF-8"
		}
	}
	return v, ""
}

// decodeValues decodes every value of s in place, and describes the first
// character outside the charset, naming its position, or returns "".
func (c Charset) decodeValues(s *Segment) string {
	problem := ""
	for i, e := range s.elements {
		for r, comps := range e {
			for j, v := range comps {
				decoded, p := c.decode(v)
				s.elements[i][r][j] = decoded
				if problem == "" && p != "" {
					problem = fmt.Sprintf("element %d component %d: %s", i+1, j+1, p)
					if r > 0 {
						problem = fmt.Sprintf("element %d repetition %d component %d: %s", i+1, r+1, j+1, p)
					}
				}
			}
		}
	}
	return problem
}
