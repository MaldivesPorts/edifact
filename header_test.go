package edifact

import "testing"

func TestUNBCarriesPositionsNoAccessorNames(t *testing.T) {
	ix := NewInterchange()
	// Element 2 component 3 is the sender internal identification, and element 3
	// component 3 the routing address. No accessor names either.
	ix.Header().Set(2, 1, "SENDERID")
	ix.Header().Set(2, 3, "INTERNAL")
	ix.Header().Set(3, 3, "ROUTING")

	if got := ix.Sender(); got != "SENDERID" {
		t.Errorf("Sender() = %q", got)
	}
	if got := ix.Header().Component(2, 3); got != "INTERNAL" {
		t.Errorf("unnamed component lost: %q", got)
	}
	if got := ix.Header().Component(3, 3); got != "ROUTING" {
		t.Errorf("unnamed component lost: %q", got)
	}
}

func TestInterchangeAccessors(t *testing.T) {
	ix := NewInterchange()
	ix.SetSender("TERMINAL", "ZZZ")
	ix.SetRecipient("PARTNER", "")
	ix.SetPreparedDatetime("260921", "1030")
	ix.SetControlRef("CTRL001")

	if ix.SyntaxIdentifier() != "UNOA" || ix.SyntaxVersion() != "2" {
		t.Errorf("syntax = %q:%q", ix.SyntaxIdentifier(), ix.SyntaxVersion())
	}
	if ix.Sender() != "TERMINAL" || ix.Header().Component(2, 2) != "ZZZ" {
		t.Error("sender not written")
	}
	if ix.Recipient() != "PARTNER" {
		t.Error("recipient not written")
	}
	if ix.Date() != "260921" || ix.Time() != "1030" {
		t.Errorf("prepared = %q %q", ix.Date(), ix.Time())
	}
	if ix.ControlRef() != "CTRL001" {
		t.Errorf("ControlRef() = %q", ix.ControlRef())
	}
	if ix.IsTest() {
		t.Error("IsTest() true by default")
	}
	ix.SetTest(true)
	if !ix.IsTest() {
		t.Error("SetTest(true) did not take")
	}
}

func TestMessageKeyRoundTripsThroughUNH(t *testing.T) {
	ix := NewInterchange()
	k := Key{Type: Is("TSTMSG"), Version: Is("D"), Release: Is("95B"), Agency: Is("UN"), Association: Is("AAA01")}
	m := ix.AddMessage(k)

	if got := m.Key(); !got.Equal(k) {
		t.Errorf("Key() = %s, want %s", got, k)
	}
	if got := m.Header().Component(2, 1); got != "TSTMSG" {
		t.Errorf("UNH element 2 component 1 = %q", got)
	}

	// Key is derived, so writing the segment changes it with no chance of drift.
	m.Header().Set(2, 5, "AAA02")
	if got := *m.Key().Association; got != "AAA02" {
		t.Errorf("Key() did not follow the segment: %q", got)
	}
}

func TestFunctionalGroupAccessors(t *testing.T) {
	ix := NewInterchange()
	g := ix.AddFunctionalGroup("TSTMSG")
	g.SetControlRef("GRP1")
	g.Header().Set(7, 1, "D")
	g.Header().Set(7, 2, "95B")

	if g.MessageType() != "TSTMSG" {
		t.Errorf("MessageType() = %q", g.MessageType())
	}
	if g.ControlRef() != "GRP1" {
		t.Errorf("ControlRef() = %q", g.ControlRef())
	}
	if g.Version() != "D" || g.Release() != "95B" {
		t.Errorf("version/release = %q/%q", g.Version(), g.Release())
	}
	if len(ix.Groups()) != 1 {
		t.Errorf("Groups has %d entries", len(ix.Groups()))
	}
}

func TestInterchangeSetterCoverage(t *testing.T) {
	ix := NewInterchange()
	ix.SetSyntax("UNOC", "4")
	ix.SetApplicationRef("APPREF")
	ix.SetAgreementID("AGREE01")

	if ix.SyntaxIdentifier() != "UNOC" || ix.SyntaxVersion() != "4" {
		t.Errorf("syntax = %q:%q", ix.SyntaxIdentifier(), ix.SyntaxVersion())
	}
	if ix.ApplicationRef() != "APPREF" {
		t.Errorf("ApplicationRef() = %q", ix.ApplicationRef())
	}
	if ix.AgreementID() != "AGREE01" {
		t.Errorf("AgreementID() = %q", ix.AgreementID())
	}
}

func TestFunctionalGroupSetterCoverage(t *testing.T) {
	ix := NewInterchange()
	g := ix.AddFunctionalGroup("TSTMSG")
	g.SetSender("TERMINAL", "ZZZ")
	g.SetRecipient("PARTNER", "")
	g.SetPreparedDatetime("260921", "1030")
	g.SetControlRef("GRP1")
	g.SetAgency("UN")
	g.SetVersionRelease("D", "95B")

	for _, c := range []struct{ name, got, want string }{
		{"MessageType", g.MessageType(), "TSTMSG"},
		{"Sender", g.Sender(), "TERMINAL"},
		{"Recipient", g.Recipient(), "PARTNER"},
		{"Date", g.Date(), "260921"},
		{"Time", g.Time(), "1030"},
		{"ControlRef", g.ControlRef(), "GRP1"},
		{"Agency", g.Agency(), "UN"},
		{"Version", g.Version(), "D"},
		{"Release", g.Release(), "95B"},
	} {
		if c.got != c.want {
			t.Errorf("%s() = %q, want %q", c.name, c.got, c.want)
		}
	}
	if got := g.Header().Component(2, 2); got != "ZZZ" {
		t.Errorf("sender qualifier = %q", got)
	}
}

func TestEmptyQualifierClearsTheOldOne(t *testing.T) {
	ix := NewInterchange()
	ix.SetSender("A", "ZZ")
	ix.SetSender("B", "")
	ix.SetRecipient("C", "ZZ")
	ix.SetRecipient("D", "")
	g := ix.AddFunctionalGroup("TSTMSG")
	g.SetSender("E", "ZZ")
	g.SetSender("F", "")
	g.SetRecipient("G", "ZZ")
	g.SetRecipient("H", "")

	for name, s := range map[string]*Segment{"UNB": ix.Header(), "UNG": g.Header()} {
		if s.Component(2, 2) != "" || s.Component(3, 2) != "" {
			t.Errorf("%s kept a qualifier: %v %v", name, s.Components(2), s.Components(3))
		}
	}
	if ix.Sender() != "B" || ix.Recipient() != "D" || g.Sender() != "F" || g.Recipient() != "H" {
		t.Error("identifications not replaced")
	}
}
