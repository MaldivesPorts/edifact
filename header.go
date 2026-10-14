package edifact

// Accessors over the service segment headers. Each reads its position out of
// the stored segment, so a position no accessor names is still carried.

// UNB, elements 1 to 11.

// SyntaxIdentifier returns the character repertoire, such as UNOA.
func (ix *Interchange) SyntaxIdentifier() string { return ix.header.Component(1, 1) }

// SyntaxVersion returns the ISO 9735 edition, "1" through "4".
func (ix *Interchange) SyntaxVersion() string { return ix.header.Component(1, 2) }

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

// SetSyntax writes the character repertoire and the ISO 9735 edition.
func (ix *Interchange) SetSyntax(identifier, version string) {
	ix.Header().Set(1, 1, identifier)
	ix.Header().Set(1, 2, version)
}

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

// SetPreparedDatetime writes the date and time of preparation.
func (ix *Interchange) SetPreparedDatetime(date, time string) {
	ix.Header().Set(4, 1, date)
	ix.Header().Set(4, 2, time)
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

// SetPreparedDatetime writes the date and time of preparation.
func (g *FunctionalGroup) SetPreparedDatetime(date, time string) {
	g.Header().Set(4, 1, date)
	g.Header().Set(4, 2, time)
}

// SetControlRef writes the functional group's reference.
func (g *FunctionalGroup) SetControlRef(ref string) { g.Header().Set(5, 1, ref) }

// SetAgency writes the controlling agency.
func (g *FunctionalGroup) SetAgency(a string) { g.Header().Set(6, 1, a) }

// SetVersionRelease writes the message version and release numbers, which name
// the directory together.
func (g *FunctionalGroup) SetVersionRelease(version, release string) {
	g.Header().Set(7, 1, version)
	g.Header().Set(7, 2, release)
}

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
