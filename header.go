package edifact

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

// Accessors over the service segment headers. Each reads its position out of
// the stored segment, so a position no accessor names is still carried.

// UNB, elements 1 to 11.

// Charset returns the syntax level UNB names. It reports false when UNB names
// none, or one this package does not support, such as UNOD. The identifier as
// written stays readable through Header().Component(1, 1).
func (ix *Interchange) Charset() (Charset, bool) {
	return charsetNamed(ix.header.Component(1, 1))
}

// SetCharset writes the syntax identifier. Write refuses the zero Charset as a
// missing mandatory element.
func (ix *Interchange) SetCharset(c Charset) { ix.Header().Set(1, 1, c.id) }

// SyntaxVersion returns the ISO 9735 edition UNB names, 1 to 4, or 0 when it
// names none or another value.
func (ix *Interchange) SyntaxVersion() int {
	v, err := strconv.Atoi(ix.header.Component(1, 2))
	if err != nil || v < 1 || v > 4 {
		return 0
	}
	return v
}

// ErrSyntaxVersion reports a syntax version other than 1 to 4.
var ErrSyntaxVersion = errors.New("edifact: syntax version must be 1 to 4")

// SetSyntaxVersion writes the ISO 9735 edition. It returns ErrSyntaxVersion,
// and writes nothing, for a version other than 1 to 4.
func (ix *Interchange) SetSyntaxVersion(v int) error {
	if v < 1 || v > 4 {
		return fmt.Errorf("%w: %d", ErrSyntaxVersion, v)
	}
	ix.Header().Set(1, 2, strconv.Itoa(v))
	return nil
}

// Sender returns the interchange sender identification.
func (ix *Interchange) Sender() string { return ix.header.Component(2, 1) }

// Recipient returns the interchange recipient identification.
func (ix *Interchange) Recipient() string { return ix.header.Component(3, 1) }

// Date returns the date of preparation, as YYMMDD or CCYYMMDD.
func (ix *Interchange) Date() string { return ix.header.Component(4, 1) }

// Time returns the time of preparation, as HHMM.
func (ix *Interchange) Time() string { return ix.header.Component(4, 2) }

// ControlRef returns the interchange control reference, which UNZ repeats.
func (ix *Interchange) ControlRef() string { return ix.header.Element(5) }

// ApplicationRef returns the application reference.
func (ix *Interchange) ApplicationRef() string { return ix.header.Element(7) }

// AgreementID returns the interchange agreement identifier.
func (ix *Interchange) AgreementID() string { return ix.header.Element(10) }

// IsTest reports whether the test indicator is set.
func (ix *Interchange) IsTest() bool { return ix.header.Element(11) == "1" }

// SetApplicationRef writes the application reference.
func (ix *Interchange) SetApplicationRef(ref string) { ix.Header().Set(7, 1, ref) }

// SetAgreementID writes the interchange agreement identifier.
func (ix *Interchange) SetAgreementID(id string) { ix.Header().Set(10, 1, id) }

// SetSender writes the sender identification and its qualifier.
func (ix *Interchange) SetSender(id, qualifier string) {
	ix.Header().Set(2, 1, id)
	ix.Header().Set(2, 2, qualifier)
}

// SetRecipient writes the recipient identification and its qualifier.
func (ix *Interchange) SetRecipient(id, qualifier string) {
	ix.Header().Set(3, 1, id)
	ix.Header().Set(3, 2, qualifier)
}

// SetDate writes the date of preparation, as YYMMDD or CCYYMMDD.
func (ix *Interchange) SetDate(date string) { ix.Header().Set(4, 1, date) }

// SetTime writes the time of preparation, as HHMM.
func (ix *Interchange) SetTime(time string) { ix.Header().Set(4, 2, time) }

// DateTime returns the date and time of preparation. UNB carries no time zone,
// so the result is in UnknownTimezone. A six-digit date is read as YYMMDD and
// an eight-digit one as CCYYMMDD.
func (ix *Interchange) DateTime() (time.Time, error) {
	return preparedAt(ix.Date(), ix.Time())
}

// SetDateTime writes t's date and time of preparation: the date as CCYYMMDD in
// syntax version 4 and YYMMDD below it, and the time as HHMM. t's clock is
// written as it is. Set the syntax version first, since Write refuses a date
// whose length does not match it.
func (ix *Interchange) SetDateTime(t time.Time) {
	date, hhmm := preparedValues(ix.SyntaxVersion(), t)
	ix.SetDate(date)
	ix.SetTime(hhmm)
}

// SetControlRef writes the interchange control reference.
func (ix *Interchange) SetControlRef(ref string) { ix.Header().Set(5, 1, ref) }

// SetTest marks the interchange as test data.
func (ix *Interchange) SetTest(test bool) {
	if test {
		ix.Header().Set(11, 1, "1")
		return
	}
	ix.Header().Set(11, 1, "")
}

// UNG, elements 1 to 8.

// MessageType returns the message type the functional group names. Below
// syntax version 4, every message in the functional group must be of this type.
func (g *FunctionalGroup) MessageType() string { return g.ung().Element(1) }

// Sender returns the functional group's sender identification.
func (g *FunctionalGroup) Sender() string { return g.ung().Component(2, 1) }

// Recipient returns the functional group's recipient identification.
func (g *FunctionalGroup) Recipient() string { return g.ung().Component(3, 1) }

// ControlRef returns the functional group's reference, which UNE repeats.
func (g *FunctionalGroup) ControlRef() string { return g.ung().Element(5) }

// Agency returns the controlling agency.
func (g *FunctionalGroup) Agency() string { return g.ung().Element(6) }

// Version returns the message version number.
func (g *FunctionalGroup) Version() string { return g.ung().Component(7, 1) }

// Release returns the message release number.
func (g *FunctionalGroup) Release() string { return g.ung().Component(7, 2) }

// Date returns the date of preparation, as YYMMDD or CCYYMMDD.
func (g *FunctionalGroup) Date() string { return g.ung().Component(4, 1) }

// Time returns the time of preparation, as HHMM.
func (g *FunctionalGroup) Time() string { return g.ung().Component(4, 2) }

// SetMessageType writes the message type the functional group names.
func (g *FunctionalGroup) SetMessageType(t string) { g.Header().Set(1, 1, t) }

// SetSender writes the functional group's sender identification and its
// qualifier.
func (g *FunctionalGroup) SetSender(id, qualifier string) {
	g.Header().Set(2, 1, id)
	g.Header().Set(2, 2, qualifier)
}

// SetRecipient writes the functional group's recipient identification and its
// qualifier.
func (g *FunctionalGroup) SetRecipient(id, qualifier string) {
	g.Header().Set(3, 1, id)
	g.Header().Set(3, 2, qualifier)
}

// SetDate writes the date of preparation, as YYMMDD or CCYYMMDD.
func (g *FunctionalGroup) SetDate(date string) { g.Header().Set(4, 1, date) }

// SetTime writes the time of preparation, as HHMM.
func (g *FunctionalGroup) SetTime(time string) { g.Header().Set(4, 2, time) }

// DateTime returns the date and time of preparation, read as
// Interchange.DateTime reads them.
func (g *FunctionalGroup) DateTime() (time.Time, error) {
	return preparedAt(g.Date(), g.Time())
}

// SetDateTime writes t's date and time of preparation, in the formats the
// interchange's syntax version requires, as Interchange.SetDateTime does.
func (g *FunctionalGroup) SetDateTime(t time.Time) {
	version := 0
	if g.ix != nil {
		version = g.ix.SyntaxVersion()
	}
	date, hhmm := preparedValues(version, t)
	g.SetDate(date)
	g.SetTime(hhmm)
}

// SetControlRef writes the functional group's reference.
func (g *FunctionalGroup) SetControlRef(ref string) { g.Header().Set(5, 1, ref) }

// SetAgency writes the controlling agency.
func (g *FunctionalGroup) SetAgency(a string) { g.Header().Set(6, 1, a) }

// SetVersion writes the message version number.
func (g *FunctionalGroup) SetVersion(version string) { g.Header().Set(7, 1, version) }

// SetRelease writes the message release number.
func (g *FunctionalGroup) SetRelease(release string) { g.Header().Set(7, 2, release) }

// UNH, elements 1 to 4.

// Ref returns the message reference number, which UNT repeats.
func (m *Message) Ref() string { return m.unh().Element(1) }

// SetRef writes the message reference number.
func (m *Message) SetRef(ref string) { m.Header().Set(1, 1, ref) }

// Key returns the message identifier, read from UNH rather than held apart from
// it. A nil m gives a zero Key.
func (m *Message) Key() Key {
	if m == nil {
		return Key{}
	}
	return Key{
		Type:        Is(m.unh().Component(2, 1)),
		Version:     Is(m.unh().Component(2, 2)),
		Release:     Is(m.unh().Component(2, 3)),
		Agency:      Is(m.unh().Component(2, 4)),
		Association: Is(m.unh().Component(2, 5)),
	}
}

// SetKey writes the message identifier.
func (m *Message) SetKey(k Key) {
	for i, f := range k.fields() {
		v := ""
		if f != nil {
			v = *f
		}
		m.Header().Set(2, i+1, v)
	}
}

// ung returns the UNG segment for reading, or nil. Segment reads are nil-safe.
func (g *FunctionalGroup) ung() *Segment {
	if g == nil {
		return nil
	}
	return g.header
}

// unh returns the UNH segment for reading, or nil. Segment reads are nil-safe.
func (m *Message) unh() *Segment {
	if m == nil {
		return nil
	}
	return m.header
}

// preparedAt reads a date and time of preparation, choosing YYMMDD or
// CCYYMMDD by the date's length.
func preparedAt(date, hhmm string) (time.Time, error) {
	switch len(date) {
	case 0:
		return time.Time{}, ErrEmpty
	case 6:
		return ParseTime(date+hhmm, "201")
	case 8:
		return ParseTime(date+hhmm, "203")
	}
	return time.Time{}, fmt.Errorf("%w: date of preparation %q is neither YYMMDD nor CCYYMMDD", ErrDateFormat, date)
}

// preparedValues renders t as a date and time of preparation for a syntax
// version.
func preparedValues(version int, t time.Time) (date, hhmm string) {
	if version == 4 {
		return t.Format("20060102"), t.Format("1504")
	}
	return t.Format("060102"), t.Format("1504")
}

// preparedDateLength is the length of the date of preparation in a syntax
// version: CCYYMMDD in version 4 and YYMMDD below it.
func preparedDateLength(version int) int {
	if version == 4 {
		return 8
	}
	return 6
}
