package edifact

import (
	"errors"
	"fmt"
	"io"
)

// Delimiters are the six characters declared by a UNA segment, in UNA order.
type Delimiters struct {
	// Component separates the components of a composite element, ':' by
	// default.
	Component byte
	// Element separates a segment's elements, '+' by default.
	Element byte
	// Decimal records what a UNA declared. Reading accepts either a dot or a
	// comma whatever it says. Writing declares a comma when Decimal is one, and
	// a dot otherwise.
	Decimal byte
	// Release is zero when the interchange declares no release character,
	// which a UNA does by putting a space in that position.
	Release byte
	// Repetition separates the repetitions of an element in syntax version 4,
	// '*' by default. It is zero below version 4, which has no repetitions.
	Repetition byte
	// Segment ends each segment, '\'' by default.
	Segment byte
}

// StandardDelimiters returns the defaults used by this package.
//   - Repetition is zero. Write uses an asterisk for syntax version 4.
//   - Decimal is '.', the library's own default. ISO 9735 allows a dot or a
//     comma.
func StandardDelimiters() Delimiters {
	return Delimiters{
		Component: ':',
		Element:   '+',
		Decimal:   '.',
		Release:   '?',
		Segment:   '\'',
	}
}

// ErrShortUNA reports a UNA segment with fewer than six characters after the
// tag.
var ErrShortUNA = errors.New("edifact: UNA segment is too short")

// ErrUnreleasable reports a value containing a separator, written to an
// interchange that has no release character to escape it with.
var ErrUnreleasable = errors.New("edifact: value contains a separator and there is no release character")

// ErrNoRepetitionSeparator reports an element with several repetitions, written
// to an interchange that declares no repetition separator.
var ErrNoRepetitionSeparator = errors.New("edifact: element repeats and there is no repetition separator")

// ErrInvalidDelimiters reports a component separator, element separator or
// segment terminator that is zero, or two delimiters that are the same
// character.
var ErrInvalidDelimiters = errors.New("edifact: invalid delimiters")

const unaLen = 9

// ParseUNA reads delimiters from b when it opens with a UNA segment. The second
// result is false when it does not.
func ParseUNA(b []byte) (Delimiters, bool, error) {
	if len(b) < 3 || b[0] != 'U' || b[1] != 'N' || b[2] != 'A' {
		return StandardDelimiters(), false, nil
	}
	if len(b) < unaLen {
		return StandardDelimiters(), true, ErrShortUNA
	}
	d := Delimiters{
		Component:  b[3],
		Element:    b[4],
		Decimal:    b[5],
		Release:    b[6],
		Repetition: b[7],
		Segment:    b[8],
	}
	if d.Release == ' ' {
		d.Release = 0
	}
	if d.Repetition == ' ' {
		d.Repetition = 0
	}
	if err := d.validate(); err != nil {
		return d, true, err
	}
	return d, true, nil
}

// validate reports a required delimiter that is zero, or two delimiters that
// are the same character. Release and repetition may be zero, meaning none.
func (d Delimiters) validate() error {
	named := []struct {
		name     string
		c        byte
		optional bool
	}{
		{"component separator", d.Component, false},
		{"element separator", d.Element, false},
		{"decimal mark", d.Decimal, true},
		{"release character", d.Release, true},
		{"repetition separator", d.Repetition, true},
		{"segment terminator", d.Segment, false},
	}
	for i, a := range named {
		if a.c == 0 {
			if a.optional {
				continue
			}
			return fmt.Errorf("%w: no %s", ErrInvalidDelimiters, a.name)
		}
		for _, b := range named[i+1:] {
			if a.c == b.c {
				return fmt.Errorf("%w: %s and %s are both %q", ErrInvalidDelimiters, a.name, b.name, a.c)
			}
		}
	}
	return nil
}

// isMeta reports whether c must be released to appear in a value.
func (d Delimiters) isMeta(c byte) bool {
	return c == d.Component ||
		c == d.Element ||
		c == d.Segment ||
		(d.Release != 0 && c == d.Release) ||
		(d.Repetition != 0 && c == d.Repetition)
}

// unreleasable reports whether v holds a separator that cannot be escaped,
// because the interchange has no release character.
func (d Delimiters) unreleasable(v string) bool {
	if d.Release != 0 {
		return false
	}
	for i := 0; i < len(v); i++ {
		if d.isMeta(v[i]) {
			return true
		}
	}
	return false
}

// writeUNA emits a service string advice declaring d.
func (d Delimiters) writeUNA(w io.Writer) error {
	rel, rep := d.Release, d.Repetition
	if rel == 0 {
		rel = ' '
	}
	if rep == 0 {
		rep = ' '
	}
	_, err := w.Write([]byte{'U', 'N', 'A', d.Component, d.Element, d.Decimal, rel, rep, d.Segment})
	return err
}
