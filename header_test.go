package edifact

import (
	"errors"
	"strings"
	"testing"
	"time"
)

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
	ix.SetDate("260921")
	ix.SetTime("1030")
	ix.SetControlRef("CTRL001")

	if c, ok := ix.Charset(); !ok || c != UNOA || ix.SyntaxVersion() != 2 {
		t.Errorf("syntax = %v:%d", c, ix.SyntaxVersion())
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
	ix.SetCharset(UNOC)
	ix.SetSyntaxVersion(4)
	ix.SetApplicationRef("APPREF")
	ix.SetAgreementID("AGREE01")

	if c, ok := ix.Charset(); !ok || c != UNOC || ix.SyntaxVersion() != 4 {
		t.Errorf("syntax = %v:%d", c, ix.SyntaxVersion())
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
	g.SetDate("260921")
	g.SetTime("1030")
	g.SetControlRef("GRP1")
	g.SetAgency("UN")
	g.SetVersion("D")
	g.SetRelease("95B")

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

func TestDateTimeFollowsTheSyntaxVersion(t *testing.T) {
	when := time.Date(2026, 1, 2, 15, 4, 59, 0, time.UTC)

	ix := NewInterchange()
	ix.SetDateTime(when)
	if ix.Date() != "260102" || ix.Time() != "1504" {
		t.Errorf("version 2: date %q, time %q", ix.Date(), ix.Time())
	}

	ix.SetCharset(UNOA)
	ix.SetSyntaxVersion(4)
	ix.SetDateTime(when)
	g := ix.AddFunctionalGroup("TSTMSG")
	g.SetDateTime(when)
	if ix.Date() != "20260102" || g.Date() != "20260102" || g.Time() != "1504" {
		t.Errorf("version 4: UNB date %q, UNG date %q, UNG time %q", ix.Date(), g.Date(), g.Time())
	}

	for _, got := range []func() (time.Time, error){ix.DateTime, g.DateTime} {
		read, err := got()
		if err != nil {
			t.Fatal(err)
		}
		want := time.Date(2026, 1, 2, 15, 4, 0, 0, UnknownTimezone)
		if !read.Equal(want) || read.Location() != UnknownTimezone {
			t.Errorf("DateTime = %v, want %v", read, want)
		}
	}
}

func TestDateTimeRejectsOtherLengths(t *testing.T) {
	ix := NewInterchange()
	if _, err := ix.DateTime(); !errors.Is(err, ErrEmpty) {
		t.Errorf("no date: err = %v, want ErrEmpty", err)
	}
	ix.SetDate("2601")
	ix.SetTime("1200")
	if _, err := ix.DateTime(); !errors.Is(err, ErrDateFormat) {
		t.Errorf("four-digit date: err = %v, want ErrDateFormat", err)
	}
}

func TestWriteRefusesADateOfTheWrongLength(t *testing.T) {
	ix := completeInterchange()
	completeMessage(ix, nil, "M1").Add("BGM", "", "REF1")
	ix.SetCharset(UNOA)
	ix.SetSyntaxVersion(4)

	var sb strings.Builder
	err := Write(&sb, ix)
	if !errors.Is(err, ErrHeaderDate) || sb.Len() != 0 {
		t.Fatalf("err = %v, wrote %d bytes, want ErrHeaderDate and nothing written", err, sb.Len())
	}

	ix.SetDate("20260101")
	if err := Write(&sb, ix); err != nil {
		t.Errorf("eight digits in version 4: %v", err)
	}
}

func TestCharsetAndSyntaxVersion(t *testing.T) {
	ix := NewInterchange()
	ix.SetCharset(UNOW)
	if err := ix.SetSyntaxVersion(4); err != nil {
		t.Fatal(err)
	}
	if c, ok := ix.Charset(); !ok || c != UNOW || ix.SyntaxVersion() != 4 {
		t.Errorf("charset %v, %v, version %d", c, ok, ix.SyntaxVersion())
	}
	if got := ix.Header().Component(1, 1) + ":" + ix.Header().Component(1, 2); got != "UNOW:4" {
		t.Errorf("UNB element 1 = %q", got)
	}

	for _, v := range []int{0, 5, -1} {
		if err := ix.SetSyntaxVersion(v); !errors.Is(err, ErrSyntaxVersion) {
			t.Errorf("version %d: err = %v, want ErrSyntaxVersion", v, err)
		}
	}
	if ix.SyntaxVersion() != 4 {
		t.Errorf("a refused version was written: %d", ix.SyntaxVersion())
	}

	ix.Header().Set(1, 1, "UNOD")
	ix.Header().Set(1, 2, "X")
	if c, ok := ix.Charset(); ok || c != (Charset{}) {
		t.Errorf("UNOD: charset %v, %v, want the zero Charset and false", c, ok)
	}
	if ix.SyntaxVersion() != 0 {
		t.Errorf("version X read as %d, want 0", ix.SyntaxVersion())
	}
}

func TestWriteRefusesTheZeroCharset(t *testing.T) {
	ix := completeInterchange()
	completeMessage(ix, nil, "M1").Add("BGM", "", "REF1")
	ix.SetCharset(Charset{})
	if err := Write(&strings.Builder{}, ix); !errors.Is(err, ErrIncompleteHeader) || !strings.Contains(err.Error(), "syntax identifier") {
		t.Errorf("err = %v, want ErrIncompleteHeader naming the syntax identifier", err)
	}
}
