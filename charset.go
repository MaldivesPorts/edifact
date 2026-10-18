package edifact

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
