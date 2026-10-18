package edifact

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func mkSeg(tag string, elems ...string) *Segment {
	s := &Segment{Tag: tag}
	for i, e := range elems {
		s.Set(i+1, 1, e)
	}
	return s
}

// completeInterchange returns an interchange whose UNB carries every mandatory
// element.
func completeInterchange() *Interchange {
	ix := NewInterchange()
	ix.SetSender("S", "")
	ix.SetRecipient("R", "")
	ix.SetDate("260101")
	ix.SetTime("1200")
	ix.SetControlRef("C1")
	return ix
}

// testKey returns a Key with every mandatory UNH component set.
func testKey() Key {
	return Key{Type: Is("TSTMSG"), Version: Is("D"), Release: Is("95B"), Agency: Is("UN")}
}

// completeGroup adds a functional group whose UNG carries every component
// mandatory below syntax version 4.
func completeGroup(ix *Interchange, ref string) *FunctionalGroup {
	g := ix.AddFunctionalGroup("TSTMSG")
	g.SetSender("AS", "")
	g.SetRecipient("AR", "")
	g.SetDate("260101")
	g.SetTime("1200")
	g.SetControlRef(ref)
	g.SetAgency("UN")
	g.SetVersion("D")
	g.SetRelease("95B")
	return g
}

// completeMessage adds a message to g whose UNH carries every mandatory
// component.
func completeMessage(ix *Interchange, g *FunctionalGroup, ref string) *Message {
	m := ix.AddMessageTo(g, testKey())
	m.SetRef(ref)
	return m
}

func feed(t *testing.T, segs ...*Segment) (*Interchange, []Diagnostic, error) {
	t.Helper()
	a := newAssembler()
	for _, s := range segs {
		if err := a.add(s); err != nil {
			return nil, nil, err
		}
	}
	return a.done()
}

// unb returns a UNB carrying every mandatory component.
func unb(sender, recipient, ref string) *Segment {
	s := mkSeg("UNB", "", sender, recipient, "", ref)
	s.Set(1, 1, "UNOA")
	s.Set(1, 2, "2")
	s.Set(4, 1, "260101")
	s.Set(4, 2, "1200")
	return s
}

// ungSeg returns a UNG carrying every component mandatory below syntax
// version 4.
func ungSeg(ref string) *Segment {
	s := mkSeg("UNG", "TSTMSG", "SENDER", "RECIP", "", ref, "UN")
	s.Set(4, 1, "260101")
	s.Set(4, 2, "1200")
	s.Set(7, 1, "D")
	s.Set(7, 2, "95B")
	return s
}

func unh(ref, msgType string) *Segment {
	s := &Segment{Tag: "UNH"}
	s.Set(1, 1, ref)
	s.Set(2, 1, msgType)
	s.Set(2, 2, "D")
	s.Set(2, 3, "95B")
	s.Set(2, 4, "UN")
	s.Set(2, 5, "AAA01")
	return s
}

func TestAssembleOneMessage(t *testing.T) {
	ix, diags, err := feed(t,
		unb("SENDER", "RECIP", "CTRL1"),
		unh("MSG1", "TSTMSG"),
		mkSeg("BGM", "", "REF001"),
		mkSeg("DTM", "137"),
		mkSeg("UNT", "4", "MSG1"),
		mkSeg("UNZ", "1", "CTRL1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("diagnostics from a well-formed interchange: %v", diags)
	}
	if len(ix.Messages()) != 1 || len(ix.Messages()[0].Segments()) != 2 {
		t.Fatalf("got %d messages, first holding %d segments", len(ix.Messages()), len(ix.Messages()[0].Segments()))
	}
	if got := ix.Messages()[0].Key(); *got.Type != "TSTMSG" {
		t.Errorf("Key().Type = %q", *got.Type)
	}
	if ix.Sender() != "SENDER" || ix.ControlRef() != "CTRL1" {
		t.Errorf("UNB not carried: %q %q", ix.Sender(), ix.ControlRef())
	}
}

func TestAssembleCountsAndRefs(t *testing.T) {
	cases := []struct {
		name string
		unt  *Segment
		unz  *Segment
		want string
	}{
		{"unt count wrong", mkSeg("UNT", "9", "MSG1"), mkSeg("UNZ", "1", "CTRL1"), "count is 9, counted 4"},
		{"unt ref wrong", mkSeg("UNT", "4", "NOPE"), mkSeg("UNZ", "1", "CTRL1"), `"NOPE" does not match "MSG1"`},
		{"unz count wrong", mkSeg("UNT", "4", "MSG1"), mkSeg("UNZ", "7", "CTRL1"), "count is 7, counted 1"},
		{"unz ref wrong", mkSeg("UNT", "4", "MSG1"), mkSeg("UNZ", "1", "NOPE"), `"NOPE" does not match "CTRL1"`},
		{"unt not a number", mkSeg("UNT", "many", "MSG1"), mkSeg("UNZ", "1", "CTRL1"), `"many" is not a number`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, diags, err := feed(t,
				unb("SENDER", "RECIP", "CTRL1"),
				unh("MSG1", "TSTMSG"), mkSeg("BGM"), mkSeg("DTM", "137"),
				c.unt, c.unz,
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(diags) != 1 || diags[0].Severity != Error {
				t.Fatalf("diags = %v", diags)
			}
			if !strings.Contains(diags[0].Message, c.want) {
				t.Errorf("message = %q, want it to mention %q", diags[0].Message, c.want)
			}
		})
	}
}

func TestAssembleTolleratesPaddedCount(t *testing.T) {
	_, diags, err := feed(t,
		unb("S", "R", "CTRL1"),
		unh("MSG1", "TSTMSG"), mkSeg("BGM"), mkSeg("DTM", "137"),
		mkSeg("UNT", "000004", "MSG1"),
		mkSeg("UNZ", "1", "CTRL1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Errorf("a zero-padded count was rejected: %v", diags)
	}
}

func TestAssembleFunctionalGroups(t *testing.T) {
	ung := ungSeg("GRP1")
	ix, diags, err := feed(t,
		unb("SENDER", "RECIP", "CTRL1"),
		ung,
		unh("MSG1", "TSTMSG"), mkSeg("BGM"), mkSeg("UNT", "3", "MSG1"),
		unh("MSG2", "TSTMSG"), mkSeg("BGM"), mkSeg("UNT", "3", "MSG2"),
		mkSeg("UNE", "2", "GRP1"),
		mkSeg("UNZ", "1", "CTRL1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("diags = %v", diags)
	}
	if len(ix.Groups()) != 1 || len(ix.Messages()) != 2 {
		t.Fatalf("%d groups, %d messages", len(ix.Groups()), len(ix.Messages()))
	}
	for i, m := range ix.Messages() {
		if m.FunctionalGroup() != ix.Groups()[0] {
			t.Errorf("message %d does not point at the group", i)
		}
	}
	if !ix.Grouped() {
		t.Error("Grouped() false with every message in a group")
	}
	// UNZ counts groups, not messages, once groups are used.
	if _, diags, _ := feed(t,
		unb("S", "R", "CTRL1"), ung,
		unh("MSG1", "TSTMSG"), mkSeg("BGM"), mkSeg("UNT", "3", "MSG1"),
		mkSeg("UNE", "1", "GRP1"),
		mkSeg("UNZ", "2", "CTRL1"),
	); len(diags) != 1 {
		t.Errorf("UNZ counting messages instead of groups went unreported: %v", diags)
	}
}

func TestAssembleStructuralErrors(t *testing.T) {
	cases := []struct {
		name string
		segs []*Segment
		want error
	}{
		{"no UNB", []*Segment{mkSeg("BGM")}, ErrNoUNB},
		{"second UNB", []*Segment{mkSeg("UNB"), mkSeg("UNB")}, ErrUnexpected},
		{"UNT with no UNH", []*Segment{mkSeg("UNB"), mkSeg("UNT", "1", "X")}, ErrUnexpected},
		{"UNH inside UNH", []*Segment{mkSeg("UNB"), unh("A", "T"), unh("B", "T")}, ErrUnexpected},
		{"data outside a message", []*Segment{mkSeg("UNB"), mkSeg("BGM")}, ErrUnexpected},
		{"UNE with no UNG", []*Segment{mkSeg("UNB"), mkSeg("UNE", "1", "G")}, ErrUnexpected},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, _, err := feed(t, c.segs...); !errors.Is(err, c.want) {
				t.Errorf("err = %v, want %v", err, c.want)
			}
		})
	}

	a := newAssembler()
	_ = a.add(mkSeg("UNB"))
	_ = a.add(unh("MSG1", "TSTMSG"))
	if _, _, err := a.done(); !errors.Is(err, ErrUnclosed) {
		t.Errorf("done with an open message: %v", err)
	}
}

func TestAssembleMissingUNZIsAWarning(t *testing.T) {
	_, diags, err := feed(t,
		unb("S", "R", "CTRL1"),
		unh("MSG1", "TSTMSG"), mkSeg("BGM"), mkSeg("UNT", "3", "MSG1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 1 || diags[0].Severity != Warning || diags[0].Tag != "UNZ" {
		t.Fatalf("diags = %v, want one UNZ warning", diags)
	}
}

func TestEnvelopeSegmentsComputesTrailers(t *testing.T) {
	ix := completeInterchange()
	ix.SetControlRef("CTRL1")
	m := ix.AddMessage(testKey())
	m.SetRef("MSG1")
	m.Add("BGM", "", "REF001")
	m.Add("DTM", "137")

	segs, err := envelopeSegments(ix, nil)
	if err != nil {
		t.Fatal(err)
	}
	var tags []string
	for _, s := range segs {
		tags = append(tags, s.Tag)
	}
	if strings.Join(tags, " ") != "UNB UNH BGM DTM UNT UNZ" {
		t.Fatalf("order = %v", tags)
	}
	unt := segs[4]
	if unt.Element(1) != "4" || unt.Element(2) != "MSG1" {
		t.Errorf("UNT = %q %q, want 4 MSG1", unt.Element(1), unt.Element(2))
	}
	unz := segs[5]
	if unz.Element(1) != "1" || unz.Element(2) != "CTRL1" {
		t.Errorf("UNZ = %q %q", unz.Element(1), unz.Element(2))
	}
}

func TestEnvelopeSegmentsGrouped(t *testing.T) {
	ix := completeInterchange()
	ix.SetControlRef("CTRL1")
	g := completeGroup(ix, "GRP1")
	for _, ref := range []string{"MSG1", "MSG2"} {
		m := ix.AddMessageTo(g, testKey())
		m.SetRef(ref)
		m.Add("BGM", "", "REF1")
	}

	segs, err := envelopeSegments(ix, nil)
	if err != nil {
		t.Fatal(err)
	}
	var tags []string
	for _, s := range segs {
		tags = append(tags, s.Tag)
	}
	if strings.Join(tags, " ") != "UNB UNG UNH BGM UNT UNH BGM UNT UNE UNZ" {
		t.Fatalf("order = %v", tags)
	}
	if une := segs[8]; une.Element(1) != "2" || une.Element(2) != "GRP1" {
		t.Errorf("UNE = %q %q, want 2 GRP1", une.Element(1), une.Element(2))
	}
	if unz := segs[9]; unz.Element(1) != "1" {
		t.Errorf("UNZ counts %q, want 1 group", unz.Element(1))
	}
}

func TestEnvelopeSegmentsRejectsMixedGrouping(t *testing.T) {
	ix := NewInterchange()
	g := ix.AddFunctionalGroup("TSTMSG")
	ix.AddMessageTo(g, Key{Type: Is("TSTMSG")})
	ix.AddMessage(Key{Type: Is("TSTMSG")}) // not in a group

	if _, err := envelopeSegments(ix, nil); err == nil {
		t.Error("mixed grouping accepted")
	}
}

func tags(segs []*Segment) string {
	var out []string
	for _, s := range segs {
		switch s.Tag {
		case "UNG", "UNE", "UNH", "UNZ":
			out = append(out, s.Tag+":"+s.Element(map[string]int{"UNG": 5, "UNE": 2, "UNH": 1, "UNZ": 1}[s.Tag]))
		}
	}
	return strings.Join(out, " ")
}

func TestEnvelopeSegmentsReordersGroups(t *testing.T) {
	ix := completeInterchange()
	ix.SetControlRef("C1")
	var gs []*FunctionalGroup
	for _, ref := range []string{"G1", "G2", "G3"} {
		g := completeGroup(ix, ref)
		gs = append(gs, g)
	}
	for i, g := range []*FunctionalGroup{gs[0], gs[1], gs[0], gs[2]} { // G1 not contiguous
		m := ix.AddMessageTo(g, testKey())
		m.SetRef(fmt.Sprintf("M%d", i+1))
		m.Add("BGM")
	}

	segs, err := envelopeSegments(ix, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "UNG:G1 UNH:M1 UNH:M3 UNE:G1 UNG:G2 UNH:M2 UNE:G2 UNG:G3 UNH:M4 UNE:G3 UNZ:3"
	if got := tags(segs); got != want {
		t.Fatalf("order:\n got %s\nwant %s", got, want)
	}

	counts := map[string]string{}
	for _, s := range segs {
		if s.Tag == "UNE" {
			counts[s.Element(2)] = s.Element(1)
		}
	}
	if counts["G1"] != "2" || counts["G2"] != "1" || counts["G3"] != "1" {
		t.Errorf("UNE counts = %v, want G1:2 G2:1 G3:1", counts)
	}

	// What is written reads back to the same groups.
	var sb strings.Builder
	if err := Write(&sb, ix); err != nil {
		t.Fatal(err)
	}
	back, err := Read(strings.NewReader(sb.String()))
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Diagnostics) != 0 || len(back.Groups()) != 3 {
		t.Errorf("re-read: %d groups, diagnostics %v", len(back.Groups()), back.Diagnostics)
	}
}

func TestEnvelopeSegmentsRejectsInconsistentGroups(t *testing.T) {
	ix := NewInterchange()
	other := NewInterchange().AddFunctionalGroup("TSTMSG")
	ix.AddMessageTo(other, Key{Type: Is("TSTMSG")}) // not in ix.Groups()
	if _, err := envelopeSegments(ix, nil); err == nil {
		t.Error("a message naming an unknown group was written")
	}

	ix = NewInterchange()
	ix.AddFunctionalGroup("TSTMSG")
	ix.AddMessage(Key{Type: Is("TSTMSG")}) // outside the declared group
	if _, err := envelopeSegments(ix, nil); err == nil {
		t.Error("groups declared alongside an ungrouped message were written")
	}
}

func TestReadReportsMixedGrouping(t *testing.T) {
	src := "UNB+UNOA:2+S+R+260101:1200+C1'" +
		"UNH+M1+TSTMSG:D:95B:UN'BGM'UNT+3+M1'" + // outside any group
		"UNG+TSTMSG+S+R+260101:1200+G1+UN+D:95B'" +
		"UNH+M2+TSTMSG:D:95B:UN'BGM'UNT+3+M2'" +
		"UNE+1+G1'" +
		"UNZ+1+C1'"

	ix, err := Read(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.Diagnostics) != 1 || ix.Diagnostics[0].Severity != Error ||
		!strings.Contains(ix.Diagnostics[0].Message, "1 messages are in a functional group and 1 are not") {
		t.Fatalf("diagnostics = %v", ix.Diagnostics)
	}

	var sb strings.Builder
	if err := Write(&sb, ix); err == nil {
		t.Error("Write accepted the mixed interchange")
	}
}

func TestWriteRefusesAnIncompleteUNB(t *testing.T) {
	var sb strings.Builder
	err := Write(&sb, NewInterchange())
	if !errors.Is(err, ErrIncompleteHeader) {
		t.Fatalf("err = %v, want ErrIncompleteHeader", err)
	}
	want := "sender, recipient, date of preparation, time of preparation, control reference"
	if !strings.HasSuffix(err.Error(), want) {
		t.Errorf("err = %v, want it to name %s", err, want)
	}
	if sb.Len() != 0 {
		t.Errorf("wrote %q before failing", sb.String())
	}

	ix := completeInterchange()
	ix.Header().Set(1, 1, "")
	ix.Header().Set(1, 2, "")
	if err := Write(&sb, ix); err == nil || !strings.HasSuffix(err.Error(), "syntax identifier, syntax version") {
		t.Errorf("err = %v, want the syntax identifier and version named", err)
	}
}

func TestWriteRefusesIncompleteHeaders(t *testing.T) {
	cases := []struct {
		name  string
		build func(*Interchange)
		want  string
	}{
		{"UNH", func(ix *Interchange) {
			ix.AddMessage(Key{Type: Is("TSTMSG")}).SetRef("M1")
		}, "UNH of message 1 (M1): message version, message release, controlling agency"},
		{"UNH transfer status", func(ix *Interchange) {
			completeMessage(ix, nil, "M1").Header().Set(4, 2, "C")
		}, "UNH of message 1 (M1): sequence of transfers"},
		{"UNG below version 4", func(ix *Interchange) {
			g := ix.AddFunctionalGroup("TSTMSG")
			g.SetControlRef("G1")
			completeMessage(ix, g, "M1")
		}, "UNG of functional group 1 (G1): application sender identification, application recipient identification, " +
			"date of preparation, time of preparation, controlling agency, message version, message release"},
		{"UNG version 4", func(ix *Interchange) {
			ix.SetCharset(UNOA)
			ix.SetSyntaxVersion(4)
			ix.SetDate("20260101")
			g := ix.AddFunctionalGroup("TSTMSG")
			g.SetDate("20260101")
			completeMessage(ix, g, "M1")
		}, "UNG of functional group 1 (): time of preparation, functional group reference"},
		{"UNB recipient reference", func(ix *Interchange) {
			ix.Header().Set(6, 2, "AA")
		}, "UNB: recipient reference"},
	}
	for _, c := range cases {
		ix := completeInterchange()
		c.build(ix)
		var sb strings.Builder
		err := Write(&sb, ix)
		if !errors.Is(err, ErrIncompleteHeader) || !strings.HasSuffix(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to end %q", c.name, err, c.want)
		}
		if sb.Len() != 0 {
			t.Errorf("%s: wrote %q before failing", c.name, sb.String())
		}
	}
}

func TestWriteAcceptsAMinimalVersion4Group(t *testing.T) {
	ix := completeInterchange()
	ix.SetCharset(UNOA)
	ix.SetSyntaxVersion(4)
	ix.SetDate("20260101")
	g := ix.AddFunctionalGroup("")
	g.SetControlRef("G1")
	completeMessage(ix, g, "M1").Add("BGM", "", "REF1")

	if err := Write(&strings.Builder{}, ix); err != nil {
		t.Error(err)
	}
}

func TestUNEInsideAMessageSaysSo(t *testing.T) {
	_, _, err := feed(t, unb("S", "R", "C1"), ungSeg("G1"), unh("M1", "TSTMSG"), mkSeg("UNE", "1", "G1"))
	if !errors.Is(err, ErrUnexpected) || !strings.Contains(err.Error(), "UNE inside an open message") {
		t.Errorf("err = %v, want UNE inside an open message", err)
	}
}

func TestWriteRefusesEnvelopeSegmentsInsideAMessage(t *testing.T) {
	for _, tag := range []string{"UNA", "UNB", "UNG", "UNH", "UNT", "UNE", "UNZ"} {
		ix := completeInterchange()
		m := completeMessage(ix, nil, "M1")
		m.Add("BGM", "", "REF1")
		m.Add(tag, "1", "X")
		var sb strings.Builder
		err := Write(&sb, ix)
		if !errors.Is(err, ErrUnexpected) || !strings.Contains(err.Error(), tag+" inside message 1 (M1)") {
			t.Errorf("%s: err = %v", tag, err)
		}
		if sb.Len() != 0 {
			t.Errorf("%s: wrote %q", tag, sb.String())
		}
	}

	ix := completeInterchange()
	m := completeMessage(ix, nil, "M1")
	m.Add("BGM", "", "REF1")
	m.Add("UNS", "D")
	if err := Write(&strings.Builder{}, ix); err != nil {
		t.Errorf("UNS refused: %v", err)
	}
}

func TestWriteRefusesEmptyEnvelopes(t *testing.T) {
	cases := map[string]func(*Interchange){
		"the interchange has no messages": func(ix *Interchange) {},
		"functional group 2 (G2) has no messages": func(ix *Interchange) {
			completeMessage(ix, completeGroup(ix, "G1"), "M1").Add("BGM", "", "REF1")
			completeGroup(ix, "G2")
		},
		"message 1 (M1) has no segments": func(ix *Interchange) {
			completeMessage(ix, nil, "M1")
		},
	}
	for want, build := range cases {
		ix := completeInterchange()
		build(ix)
		var sb strings.Builder
		err := Write(&sb, ix)
		if !errors.Is(err, ErrNoContent) || !strings.HasSuffix(err.Error(), want) {
			t.Errorf("err = %v, want %q", err, want)
		}
		if sb.Len() != 0 {
			t.Errorf("%s: wrote %q", want, sb.String())
		}
	}
}

func TestReadReportsEmptyEnvelopes(t *testing.T) {
	cases := map[string]string{
		"error: segment 1, UNB: interchange has no messages": "UNB+UNOA:2+S+R+260101:1200+C1'UNZ+0+C1'",
		"error: segment 3, UNG: functional group has no messages": "UNB+UNOA:2+S+R+260101:1200+C1'" +
			"UNG+TSTMSG+AS+AR+260101:1200+G1+UN+D:95B'UNE+0+G1'UNZ+1+C1'",
		"error: message 1 (M1), segment 2, UNT: message has no segments between UNH and UNT": "UNB+UNOA:2+S+R+260101:1200+C1'" +
			"UNH+M1+TSTMSG:D:95B:UN'UNT+2+M1'UNZ+1+C1'",
	}
	for want, src := range cases {
		ix, err := Read(strings.NewReader(src))
		if err != nil {
			t.Fatalf("%s: %v", want, err)
		}
		found := false
		for _, d := range ix.Diagnostics {
			found = found || d.String() == want
		}
		if !found {
			t.Errorf("want %q in %v", want, ix.Diagnostics)
		}
	}
}

func TestMessagesMustMatchTheirFunctionalGroup(t *testing.T) {
	group := func(version, unh string) string {
		date := "260101"
		if version == "4" {
			date = "20260101"
		}
		return "UNB+UNOA:" + version + "+S+R+" + date + ":1200+C1'" +
			"UNG+BAPLIE+AS+AR+" + date + ":1200+G1+UN+D:95B'" + unh + "BGM++REF1'UNT+3+M1'UNE+1+G1'UNZ+1+C1'"
	}
	cases := []struct{ name, src, want string }{
		{"other type", group("2", "UNH+M1+MOVINS:D:95B:UN'"), "message type MOVINS differs from the functional group's BAPLIE"},
		{"other version", group("2", "UNH+M1+BAPLIE:S:95B:UN'"), "message version S differs from the functional group's D"},
		{"other release", group("2", "UNH+M1+BAPLIE:D:96A:UN'"), "message release 96A differs from the functional group's 95B"},
		{"other agency", group("2", "UNH+M1+BAPLIE:D:95B:XX'"), "message controlling agency XX differs from the functional group's UN"},
		{"version 4", group("4", "UNH+M1+MOVINS:D:95B:UN'"), ""},
		{"same", group("2", "UNH+M1+BAPLIE:D:95B:UN'"), ""},
	}
	for _, c := range cases {
		ix, err := Read(strings.NewReader(c.src))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		var got string
		for _, d := range ix.Diagnostics {
			if d.Tag == "UNH" && d.Severity == Error {
				got = d.Message
			}
		}
		if got != c.want {
			t.Errorf("%s: diagnostic %q, want %q", c.name, got, c.want)
		}

		var sb strings.Builder
		err = Write(&sb, ix, AllowIncompleteHeaders())
		if c.want == "" && err != nil {
			t.Errorf("%s: write: %v", c.name, err)
		}
		if c.want != "" && (!errors.Is(err, ErrGroupMismatch) || sb.Len() != 0) {
			t.Errorf("%s: write err = %v, wrote %d bytes", c.name, err, sb.Len())
		}
	}
}

func TestUNHRulesFollowTheSyntaxVersion(t *testing.T) {
	for _, c := range []struct {
		version int
		ok      bool
	}{{1, true}, {2, false}, {3, false}, {4, false}} {
		ix := completeInterchange()
		ix.SetSyntaxVersion(c.version)
		m := ix.AddMessage(Key{Type: Is("BAPLIE"), Version: Is("1")})
		m.SetRef("M1")
		m.Add("BGM", "", "REF1")

		err := Write(&strings.Builder{}, ix)
		if c.ok && err != nil {
			t.Errorf("version %d: %v", c.version, err)
		}
		if !c.ok && (!errors.Is(err, ErrIncompleteHeader) || !strings.HasSuffix(err.Error(), "message release, controlling agency")) {
			t.Errorf("version %d: err = %v", c.version, err)
		}
	}

	src := "UNB+UNOA:1+S+R+260101:1200+C1'UNH+M1+BAPLIE:1'BGM++REF1'UNT+3+M1'UNZ+1+C1'"
	ix, err := Read(strings.NewReader(src))
	if err != nil || len(ix.Diagnostics) != 0 {
		t.Errorf("version 1 read: %v %v", err, ix.Diagnostics)
	}
}
